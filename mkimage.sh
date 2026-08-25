#!/bin/bash

set -xeuo pipefail

# Kernel comes from a Rebuild build, with the hash suffix Armbian stamps on
# the filename stripped off (Armbian appends -S...-P...-C... content hashes
# that change whenever the patches or config change, which would make this
# name unstable).
#
# Matched to Rebuild's pinned kernel (armbian/recore.csc: 6.18.33, edge) so
# both images run the same kernel. Reflash previously used 6.12.69/current,
# which had drifted from Rebuild across three axes at once - kernel version,
# branch, and Armbian release - and meant kernel patches fixed one image but
# not the other. The dw-hdmi HPD debounce is the case in point: it is what
# makes the on-board screen light up during flashing, and it only exists in
# Rebuild's 6.18 patch set.
KERNEL_DEB=linux-image-edge-sunxi64_26.05.0-trunk_arm64__6.18.33.deb
export ROOTFSDIR=reflash_rootfs
sudo rm -rf "${ROOTFSDIR}"
mkdir -p "${ROOTFSDIR}"

sudo debootstrap --arch=arm64 --foreign --variant=minbase trixie "${ROOTFSDIR}"/initrd http://ftp.no.debian.org/debian/

if [ ! -f rootfs_files/debs/${KERNEL_DEB} ]; then
	wget -P rootfs_files/debs/ https://feeds.iagent.no/debian/pool/main/${KERNEL_DEB}
fi
# Name the file rather than globbing. ${KERNEL_DEB} is the only deb that is
# downloaded and the only one dpkg installs, and rootfs_files/debs is gitignored
# - so its contents are whatever happens to be on the machine doing the build,
# which makes a glob here non-deterministic. Copying everything once baked a
# 166MB backup of the kernel deb into the rootfs, because the cleanup afterwards
# only matched *.deb and the backup was named *.deb.upstream-orig.
sudo cp rootfs_files/debs/${KERNEL_DEB} "${ROOTFSDIR}"/initrd

sudo bash -c "echo recore > ${ROOTFSDIR}/initrd/etc/hostname"

# apt/dpkg policy, copied in before anything installs. These four have to be in
# place before the second stage and before apt-get update below - dpkg reads
# 01-slim as it unpacks, so writing it later would mean the base system and
# whatever apt pulled in were unpacked whole. Everything else the image needs
# lives in rootfs_files/rootfs and is copied in after the chroot; see there.
sudo cp -a rootfs_files/rootfs-preinstall/. "${ROOTFSDIR}"/initrd/

sudo chroot "${ROOTFSDIR}"/initrd /bin/bash <<ENDOFDEB
export DEBIAN_FRONTEND="noninteractive"
export TERM=xterm-color
/debootstrap/debootstrap --second-stage
export LC_ALL=C

mount -t proc proc /proc
mount -t sysfs sys /sys
mount -t devtmpfs dev /dev || mount --bind /dev /dev
mount -t devpts pts /dev/pts

# firmware-* live in non-free-firmware, which debootstrap does not
# enable. Without them most wifi dongles fail at probe with "Direct
# firmware load for ... failed" - see the firmware install below.
echo "deb http://ftp.no.debian.org/debian trixie main non-free-firmware" > /etc/apt/sources.list

apt-get update

apt install -y \
systemd-resolved \
systemd \
systemd-timesyncd \
openssh-server \
udev \
kmod \
fdisk \
parted \
xz-utils \
pv \
wget \
sudo \
iproute2 \
e2fsprogs \
libnss-resolve \
ca-certificates \
iwd \
dbus

# Wifi dongle firmware. Nearly every mainline wireless driver (rtw88,
# mt76, ath9k_htc, brcmfmac, rtl8xxxu, ...) loads a blob from
# /lib/firmware when it probes, and this image previously shipped no
# firmware directory at all - so those drivers failed with
#   rtw_8821cu: Direct firmware load for rtw88/rtw8821c_fw.bin failed
#   rtw_8821cu: failed to setup chip information
# and the dongle was dead. The only adapter that worked did so because
# its out-of-tree driver carries firmware inside the module.
#
# wireless-regdb provides regulatory.db, which was also missing and
# failing to load on every boot; it governs which channels and TX powers
# are permitted, so without it a dongle can come up restricted.
#
# Chosen for USB dongles specifically:
#   realtek     - rtw88 (8821cu etc) and rtlwifi (8192cu etc)
#   mediatek    - mt76/mt7601u, and the Ralink rt2x00 blobs live here too
#   ath9k-htc   - the USB Atheros package; firmware-atheros is 117MB of
#                 ath10k/11k/qca for PCIe cards and is deliberately NOT
#                 installed, as none of it applies to a USB dongle
# firmware-brcm80211 is omitted (Broadcom is almost all SDIO/PCIe) as is
# firmware-misc-nonfree (19.5MB, nothing wifi-relevant for us).
# dpkg does not create parent directories for path-included files, and
# the default-deny above excludes /usr/lib/firmware/mediatek itself. The
# rtw88/rtlwifi/ath9k_htc includes end in /* which happens to match the
# directory entry too (fnmatch lets * match the empty string), but
# mediatek/mt76* needs a literal mt76 and so cannot - which made
# firmware-mediatek fail to unpack entirely:
#   unable to create '.../mediatek/mt7601u.bin.dpkg-new': No such file
# Pre-create the directory so the included blobs have somewhere to land.
mkdir -p /usr/lib/firmware/mediatek

apt install -y \
wireless-regdb \
firmware-realtek \
firmware-mediatek \
firmware-ath9k-htc

# wireless-regdb ships the same database twice - regulatory.db-debian and
# regulatory.db-upstream are byte-identical, only the detached .p7s signature
# differs - and update-alternatives prefers the Debian one (priority 100 against
# 50). That is right on a Debian kernel, which has the Debian signing key built
# in. Ours comes from Armbian and carries only sforshee's, so the signature
# check fails and the kernel throws the whole database away:
#   cfg80211: loaded regulatory.db is malformed or signature is missing/invalid
# The board is then stuck in regulatory domain 00 permanently, which costs
# channels and TX power on every dongle. Note the file is still *present* and a
# plain -e test on it passes, which is why the check below resolves the symlink.
update-alternatives --set regulatory.db /lib/firmware/regulatory.db-upstream

dpkg -i ${KERNEL_DEB}

KVER=\$(ls /lib/modules | head -1)

# Recreate the /boot/Image symlink the kernel package's postinst would have
# made ("Armbian: update last-installed kernel symlink to 'Image'").
#
# The postinst does not run here. The kernel deb depends on
# initramfs-tools | linux-initramfs-tool and this minbase chroot has
# neither - deliberately, because Reflash's rootfs IS the initramfs, so
# there is nothing for initramfs-tools to build. dpkg therefore unpacks the
# package but leaves it unconfigured.
#
# boot.cmd loads \${prefix}Image, so without this symlink the image is
# unbootable. That is exactly what happened on the first 6.18 build, and it
# still exited 0: dpkg's failure does not stop this chroot body, which runs
# without set -e.
ln -sf vmlinuz-\${KVER} /boot/Image

# Rebuild modules.dep against what actually got unpacked - the excludes
# above drop whole subsystems, and a stale dep file would reference
# modules that are not there.
depmod -a \${KVER}


systemctl enable systemd-networkd
ln -sf /usr/lib/systemd/systemd /init

useradd debian -d /home/debian -G tty,dialout -m -s /bin/bash -e -1
mkdir -p /etc/sudoers.d
echo "debian ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/debian

# Set default passwords
echo 'debian:temppwd' | chpasswd
echo 'root:temppwd' | chpasswd

# No file is written from inside this chroot any more - see the comment above
# the rootfs_files/rootfs copy below for why. Note the constraint that used to
# apply here, in case anything is ever added back: this here-doc is unquoted, so
# a nested one had to backslash-escape every dollar sign and backtick to survive
# the outer expansion, and getting that wrong produced an empty case pattern and
# an unrolled for-loop list in the gadget script rather than an error.

# Installing openssh-server generated SSH host keys as a side effect - baked
# into this one build, they'd be identical across every image and every
# board flashed from it (#80). Delete them; ssh-keygen-boot.service (set up
# below, outside the chroot) restores or generates them from USB storage
# on boot instead.
rm -f /etc/ssh/ssh_host_*

# Clean up
rm -rf /usr/sbin/policy-rc.d
rm ./*.deb
rm -rf /var/lib/apt/lists/
rm -rf /var/cache/
rm -rf /usr/share/locale/

find /usr/share/doc -depth -type f -print0 ! -name copyright | xargs -0 rm
find /usr/share/doc -empty -print0 | xargs -0 rmdir
rm -rf /usr/share/man/* /usr/share/groff/* /usr/share/info/*
rm -rf /usr/share/lintian/* /usr/share/linda/* /var/cache/man/*
rm -rf /usr/share/zoneinfo/*
rm -rf /lib/udev/hwdb.d/*

# Build-time logs. These record how the image was made and are useless
# once it boots, on a rootfs where free space is the scarce resource.
rm -rf /var/log/apt /var/log/dpkg.log /var/log/bootstrap.log \
       /var/log/alternatives.log /var/log/faillog /var/log/lastlog

# The directory apt just created is what would make journald pick persistent
# storage; the drop-in that pins it volatile is in rootfs_files/rootfs.
rm -rf /var/log/journal

# Report what actually landed, so a mis-typed path-exclude shows up in
# the build log instead of silently shipping the full tree.
echo "=== SIZE REPORT ==="
du -sh /lib/modules /usr/lib/firmware / 2>/dev/null | sed 's/^/  /'
du -sh /usr/lib/firmware/* 2>/dev/null | sed 's/^/    /'
ls /usr/lib/firmware/rtw88/rtw8821c_fw.bin >/dev/null 2>&1 \
  && echo "  rtw8821c_fw.bin: present" || echo "  rtw8821c_fw.bin: MISSING"
ls /usr/lib/firmware/regulatory.db >/dev/null 2>&1 \
  && echo "  regulatory.db: present" || echo "  regulatory.db: MISSING"
ls /usr/lib/firmware/mediatek/mt7601u.bin >/dev/null 2>&1 \
  && echo "  mt7601u.bin: present" || echo "  mt7601u.bin: MISSING"
ls /usr/lib/firmware/ath9k_htc/*.fw >/dev/null 2>&1 \
  && echo "  ath9k_htc fw: present" || echo "  ath9k_htc fw: MISSING"
echo "=== END SIZE REPORT ==="

# Clean up mounts before exiting the chroot
umount /proc
umount /sys
umount /dev/pts
umount /dev

# Fail the build if the firmware install did not actually land.
#
# apt returning non-zero is not enough on its own: this chroot body runs
# without set -e (the outer script's set -e only sees the exit status of
# the whole here-doc, i.e. the last command), so a dpkg unpack failure
# scrolls past and the image ships silently without dongle firmware.
# That is exactly how firmware-mediatek went missing for an entire build
# while make still reported success.
#
# Deliberately placed after the umounts above: exiting non-zero with
# /proc, /sys and /dev still bind-mounted would leave them mounted inside
# ${ROOTFSDIR}, and the next run's "sudo rm -rf ${ROOTFSDIR}" would then
# recurse straight into the host's /dev.
# Fail the build if the kernel did not land properly.
#
# boot.cmd loads Image and uInitrd; without them U-Boot has nothing to boot
# and the failure is invisible until a board is flashed. The kernel package
# is expected to be "unpacked but not configured" here (see the Image
# symlink above), so dpkg's own exit status cannot be used as the check -
# look at the files instead.
BOOT_MISSING=""
for f in /boot/Image /boot/vmlinuz-* ; do
  [ -e "\$f" ] || BOOT_MISSING="\$BOOT_MISSING \$f"
done
[ -e "\$(readlink -f /boot/Image)" ] || BOOT_MISSING="\$BOOT_MISSING /boot/Image(dangling)"
if [ -n "\$BOOT_MISSING" ]; then
  echo "FATAL: kernel install incomplete - missing:\$BOOT_MISSING" >&2
  echo "FATAL: the resulting image would not boot" >&2
  exit 1
fi

FW_MISSING=""
for f in /usr/lib/firmware/rtw88/rtw8821c_fw.bin \
         /usr/lib/firmware/mediatek/mt7601u.bin \
         /usr/lib/firmware/regulatory.db; do
  [ -e "\$f" ] || FW_MISSING="\$FW_MISSING \$f"
done
# Checked as non-empty directories rather than by filename - these
# packages rename their blobs between releases, and a check hard-coded to
# a name that no longer exists would fail the build for the wrong reason.
for d in /usr/lib/firmware/rtlwifi /usr/lib/firmware/ath9k_htc; do
  [ -n "\$(ls -A \$d 2>/dev/null)" ] || FW_MISSING="\$FW_MISSING \$d/"
done
if [ -n "\$FW_MISSING" ]; then
  echo "FATAL: wifi firmware install failed - missing:\$FW_MISSING" >&2
  echo "FATAL: check the dpkg path-include rules and the apt output above" >&2
  exit 1
fi

# The presence check above cannot catch this: the Debian-signed database is a
# perfectly good file that our kernel rejects at load time, leaving the board
# in regulatory domain 00 with no error the build would ever see.
REGDB=\$(readlink -f /usr/lib/firmware/regulatory.db)
case "\$REGDB" in
  *-upstream) ;;
  *) echo "FATAL: regulatory.db resolves to \$REGDB, not the -upstream variant" >&2
     echo "FATAL: our kernel only carries sforshee's key; any other signature is discarded" >&2
     exit 1 ;;
esac

ENDOFDEB

# Install app
#
# The helper directory and the web root are built from scratch rather than
# copied into: the build context is a staging directory that the Makefile
# populates from bin/prod and client/, and "cp" merges rather than replaces - so
# anything deleted upstream survived in the context and kept being installed.
# Eight dead scripts had accumulated that way, one of them (get-setting) removed
# because it could never work. The Makefile clears the staging dir now; this
# refuses to install a stale set even if it is handed one, e.g. when the
# container is run against a context assembled by hand.
sudo rm -rf "${ROOTFSDIR}"/initrd/usr/local/bin "${ROOTFSDIR}"/initrd/var/www/html/reflash
sudo mkdir -p "${ROOTFSDIR}"/initrd/usr/local/bin
sudo cp reflash/reflash "${ROOTFSDIR}"/initrd/usr/local/bin/
sudo mkdir -p "${ROOTFSDIR}"/initrd/usr/local/share/fonts
sudo cp reflash/Roboto-Light.ttf "${ROOTFSDIR}"/initrd/usr/local/share/fonts/
sudo mkdir -p "${ROOTFSDIR}"/initrd/var/www/html/reflash
sudo cp -r client/dist "${ROOTFSDIR}"/initrd/var/www/html/reflash
sudo cp bin/* "${ROOTFSDIR}"/initrd/usr/local/bin

# Everything systemd, udev, sysctl and iwd read at boot: units, network files,
# udev rules, the AP profile, the gadget script. Each one is a plain file in
# rootfs_files/rootfs under its target path, and the comment explaining why it
# says what it says lives in the file itself.
#
# They used to be eighteen here-docs written straight into the rootfs, which
# cost an outage: usb-gadget-init.sh was written by the chroot and then deleted
# again by the "rm -rf usr/local/bin" above, and three images shipped with
# usb-gadget-setup.service failing on "Unable to locate executable" - no
# /dev/ttyGS*, so no USB login and no control protocol on the host, while every
# test suite stayed green (#120).
#
# Two ordering constraints, both load-bearing:
#   - after the rm -rf above, or usr/local/bin/usb-gadget-init.sh is deleted
#     again exactly as it was;
#   - before the systemctl enable calls below, which need the unit files to
#     exist to link them into multi-user.target.wants.
# The verification pass at the end of this script asserts every file in the
# tree made it into the rootfs, so a third way to lose them fails the build
# rather than the boot.
sudo cp -a rootfs_files/rootfs/. "${ROOTFSDIR}"/initrd/

# Deliberately no .network file for wlan0. There used to be one carrying
# DHCP=no + MulticastDNS=yes, on the theory that matching the interface without
# a DHCP client was inert and only bought mDNS. It is not inert: any match makes
# networkd *manage* the link, and a managed link refuses iwd's addressing. iwd
# associated fine and then never got an IPv4 address, so wifi-connect timed out
# after 30s and fell back to the hotspot - every time, on any network.
#
# What it looked like on the board: `networkctl status wlan0` reporting
# "routable (configured)" with only an IPv6 address (DHCP=no still leaves
# IPv6AcceptRA on, so networkd quietly took the v6 side), no IPv4, and iwd
# logging "Failed to modify the DNS entries ... Link wlan0 is managed".
#
# mDNS is not lost with the file gone: etc/systemd/resolved.conf.d/mdns.conf
# sets MulticastDNS=yes globally, and `resolvectl mdns` shows it on for wlan0.

# Enabling is separate from installing the unit file: these three want to start
# at boot without anything pulling them in.
systemctl enable iwd --root="${ROOTFSDIR}"/initrd
systemctl enable ssh-keygen-boot --root="${ROOTFSDIR}"/initrd
systemctl enable reflash --root="${ROOTFSDIR}"/initrd

# The helper set is what most of the image's behaviour hangs off, and a missing
# or surplus script is invisible until something calls it at runtime. Record it
# in the build log so an image can be audited from its own build output.
echo "Installed $(ls "${ROOTFSDIR}"/initrd/usr/local/bin | wc -l) helpers:"
ls "${ROOTFSDIR}"/initrd/usr/local/bin | sort | tr '\n' ' '; echo

# ...and then check, rather than print and hope. Printing the helper list is
# what this was before, and it had been printing the evidence of the missing
# gadget script in every build for three releases without anyone reading it as
# an absence (#120). An assertion cannot be misread.
#
# The expected set is the rootfs_files tree itself, so adding a file there is
# all it takes to have it checked - there is no second list to keep in sync.
# Everything else that has to be in the image and does not come from that tree
# is named explicitly below.
echo "=== ROOTFS CHECK ==="
MISSING=""
while IFS= read -r f; do
	[ -e "${ROOTFSDIR}/initrd/${f}" ] || MISSING="$MISSING /$f"
done < <(cd rootfs_files/rootfs && find . -type f -printf '%P\n')

for f in usr/local/bin/reflash \
	usr/local/bin/ssh-keygen-boot \
	usr/local/share/fonts/Roboto-Light.ttf \
	var/www/html/reflash/dist/index.html; do
	[ -e "${ROOTFSDIR}/initrd/${f}" ] || MISSING="$MISSING /$f"
done

# Present is not the same as enabled: a unit file that nothing pulls into
# multi-user.target is as inert as one that is not there.
#
# Looked up as a .wants symlink under any target rather than by asking
# "systemctl is-enabled". The systemctl in this container is the Python
# reimplementation (Debian's systemctl package, since there is no PID 1 here),
# and its is-enabled answers "disabled" for a unit it has just enabled whenever
# --root was given a relative path - which this one is. The symlink it writes is
# correct; only its own read-back is not.
for u in iwd reflash ssh-keygen-boot; do
	[ -n "$(find "${ROOTFSDIR}"/initrd/etc/systemd/system -path '*.wants/*' -name "$u.service" -print -quit)" ] \
		|| MISSING="$MISSING $u.service(installed but not enabled)"
done

# The gadget script is the one file that is both installed from the tree and
# executed by a unit, so an exec bit lost in transit would fail exactly as a
# missing file did - "Permission denied" instead of "Unable to locate".
[ -x "${ROOTFSDIR}/initrd/usr/local/bin/usb-gadget-init.sh" ] \
	|| MISSING="$MISSING /usr/local/bin/usb-gadget-init.sh(not-executable)"

if [ -n "$MISSING" ]; then
	echo "FATAL: files missing from the built rootfs:$MISSING" >&2
	echo "FATAL: something wrote over or deleted them after they were installed" >&2
	exit 1
fi
echo "  all $(cd rootfs_files/rootfs && find . -type f | wc -l) rootfs_files entries present, units enabled"
echo "=== END ROOTFS CHECK ==="

sudo mkdir -p "${ROOTFSDIR}"/initrd/mnt/usb
sudo mkdir -p "${ROOTFSDIR}"/initrd/mnt/emmc

sudo cp reflash-version "$ROOTFSDIR"/initrd/etc/
NAME="reflash-"$(cat reflash-version  | tr -d '\n')

# Move the boot folder outside the rootfs
sudo rm -rf "${ROOTFSDIR}"/boot
sudo mv "${ROOTFSDIR}"/initrd/boot/ "${ROOTFSDIR}"

# Compile and copy extra files
sudo cp rootfs_files/boot/* "${ROOTFSDIR}"/boot
mkimage -C none -A arm -T script -d "${ROOTFSDIR}"/boot/boot.cmd "${ROOTFSDIR}"/boot/boot.scr

# Copy recore dtb and fixup
mkdir -p "${ROOTFSDIR}"/boot/dtb/allwinner/overlay
sudo cp rootfs_files/dtb/*.dtb "${ROOTFSDIR}"/boot/dtb/allwinner/
sudo cp rootfs_files/dtb/*.scr "${ROOTFSDIR}"/boot/dtb/allwinner/overlay/

# Crate initramfs
sudo bash -c "cd '${ROOTFSDIR}/initrd' && find . | cpio -ov --format=newc | gzip -9 >'../initrd.img.gz'" >/dev/null 2>&1
mkimage -A arm -T ramdisk -C gzip -n uInitrd -d "${ROOTFSDIR}"/initrd.img.gz "${ROOTFSDIR}"/uInitrd
sudo cp "${ROOTFSDIR}"/uInitrd "${ROOTFSDIR}"/boot

# Create new empty image
sudo rm -rf "${ROOTFSDIR}"/reflash.img
truncate -s 250M "${ROOTFSDIR}"/reflash.img
LOOPDEV=$(sudo losetup -f)
sudo losetup -P "${LOOPDEV}" "${ROOTFSDIR}"/reflash.img
printf "g\nn\n\n\n\nw\n" | sudo fdisk "${LOOPDEV}"
sudo mkfs.ext4 -E nodiscard "${LOOPDEV}"p1
mkdir -p "${ROOTFSDIR}"/image
sudo mount "${LOOPDEV}"p1 "${ROOTFSDIR}"/image

# Copy files to new image
sudo cp -r "${ROOTFSDIR}"/boot/* "${ROOTFSDIR}"/image

# Unmount and compress image
sudo umount "${ROOTFSDIR}"/image
sudo losetup -d "${LOOPDEV}"

xz -f -T 0 -k -z "${ROOTFSDIR}"/reflash.img
mv "${ROOTFSDIR}"/reflash.img.xz /output/${NAME}.img.xz

sudo rm -rf "${ROOTFSDIR}"
