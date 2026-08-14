#!/bin/bash

set -xeuo pipefail

KERNEL_DEB=linux-image-current-sunxi64_26.02.0-trunk_arm64__6.12.69.deb
export ROOTFSDIR=reflash_rootfs
sudo rm -rf "${ROOTFSDIR}"
mkdir -p "${ROOTFSDIR}"

sudo debootstrap --arch=arm64 --foreign --variant=minbase trixie "${ROOTFSDIR}"/initrd http://ftp.no.debian.org/debian/

if [ ! -f rootfs_files/debs/${KERNEL_DEB} ]; then
	wget -P rootfs_files/debs/ https://feeds.iagent.no/debian/pool/main/${KERNEL_DEB}
fi
sudo cp rootfs_files/debs/* "${ROOTFSDIR}"/initrd

sudo bash -c "echo recore > ${ROOTFSDIR}/initrd/etc/hostname"

sudo chroot "${ROOTFSDIR}"/initrd /bin/bash <<ENDOFDEB
export DEBIAN_FRONTEND="noninteractive"
export TERM=xterm-color
/debootstrap/debootstrap --second-stage
export LC_ALL=C

mount -t proc proc /proc
mount -t sysfs sys /sys
mount -t devtmpfs dev /dev || mount --bind /dev /dev
mount -t devpts pts /dev/pts

cat <<EOF > /etc/apt/apt.conf.d/01norecommend
APT::Install-Recommends "0";
APT::Install-Suggests "0";
EOF

cat <<EOF > /etc/apt/apt.conf.d/80-retries
Acquire::Retries "3";
Acquire::http::Timeout "10";
Acquire::https::Timeout "10";
EOF

cat <<EOF > /usr/sbin/policy-rc.d
#!/bin/sh
exit 101
EOF

chmod +x /usr/sbin/policy-rc.d

# Never unpack things this image has no use for. Done as dpkg excludes
# rather than deleting afterwards so the files are never written at all -
# that saves build time as well as space, and it also applies to the
# kernel package installed further down.
#
# The kernel package is by far the biggest thing in this image (208MB of
# ~366MB installed), and the initramfs is unpacked into a tmpfs that the
# kernel caps at half of RAM (~367MB on this 1GB board). Without this the
# rootfs boots essentially 100% full, and anything that writes - logging,
# ssh host keys, even login - fails with ENOSPC.
cat <<EOF > /etc/dpkg/dpkg.cfg.d/01-slim
path-exclude /usr/share/doc/*
path-include /usr/share/doc/*/copyright
path-exclude /usr/share/man/*
path-exclude /usr/share/info/*
path-exclude /usr/share/locale/*
path-exclude /usr/share/lintian/*

# Sound: this image never plays audio.
path-exclude /lib/modules/*/kernel/sound/*
path-exclude /lib/modules/*/kernel/drivers/media/*

# Firewalling/QoS/bluetooth: not used while flashing.
#
# net/ipv4/netfilter and net/ipv6/netfilter have to go too, not just
# net/netfilter: x_tables.ko lives in the latter, and leaving ip_tables.ko
# behind without it means every boot logs
#   ip_tables: Unknown symbol xt_compat_unlock (err -2)
# for a module that then fails to load anyway.
path-exclude /lib/modules/*/kernel/net/netfilter/*
path-exclude /lib/modules/*/kernel/net/ipv4/netfilter/*
path-exclude /lib/modules/*/kernel/net/ipv6/netfilter/*
path-exclude /lib/modules/*/kernel/net/bluetooth/*
path-exclude /lib/modules/*/kernel/net/sched/*

# Filesystems we never mount. ext4/vfat/fuse/nls are deliberately kept:
# ext4 for the USB and eMMC partitions, vfat+nls for FAT boot partitions.
path-exclude /lib/modules/*/kernel/fs/xfs/*
path-exclude /lib/modules/*/kernel/fs/btrfs/*
path-exclude /lib/modules/*/kernel/fs/f2fs/*
path-exclude /lib/modules/*/kernel/fs/jfs/*
path-exclude /lib/modules/*/kernel/fs/ocfs2/*
path-exclude /lib/modules/*/kernel/fs/gfs2/*
path-exclude /lib/modules/*/kernel/fs/nfs/*
path-exclude /lib/modules/*/kernel/fs/nfsd/*
path-exclude /lib/modules/*/kernel/fs/smb/*
path-exclude /lib/modules/*/kernel/fs/ceph/*
path-exclude /lib/modules/*/kernel/net/sunrpc/*

# iSCSI target (exporting storage over the network). Note this is NOT
# the USB drive's path - that is usb-storage/uas on top of drivers/scsi,
# both of which are kept.
path-exclude /lib/modules/*/kernel/drivers/target/*
path-exclude /lib/modules/*/kernel/drivers/infiniband/*

# Firmware: default-deny, then allow only what a USB wifi dongle needs.
# Note the path spelling - firmware packages record ./usr/lib/firmware
# while the kernel package records ./lib/modules, so the two need
# different prefixes and an exclude written against the wrong one
# silently matches nothing.
#
# Taken whole these packages are 236MB; the USB-dongle parts are ~15MB.
# The bulk is PCIe/SDIO cards, bluetooth, ethernet NICs and audio
# codecs, none of which exist on this board.
#
# Adding support for another dongle family = add a path-include here.
path-exclude /usr/lib/firmware/*
path-include /usr/lib/firmware/rtw88/*
path-include /usr/lib/firmware/rtlwifi/*
path-include /usr/lib/firmware/ath9k_htc/*
path-include /usr/lib/firmware/mediatek/mt76*
path-include /usr/lib/firmware/mediatek/mt7601u.bin
path-include /usr/lib/firmware/rt2*.bin
path-include /usr/lib/firmware/rt3*.bin
path-include /usr/lib/firmware/rt73.bin
path-include /usr/lib/firmware/regulatory.db*
EOF

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

# wireless-regdb ships regulatory.db-debian and relies on
# update-alternatives to create the regulatory.db symlink the kernel
# actually asks for - and that did not survive the chroot install.
# Create it directly; a missing regulatory.db means the wifi stack falls
# back to the most restrictive channel/power set on every dongle.
for f in regulatory.db regulatory.db.p7s; do
  [ -e /usr/lib/firmware/\$f ] || ln -sf \$f-debian /usr/lib/firmware/\$f
done

dpkg -i ${KERNEL_DEB}

# Rebuild modules.dep against what actually got unpacked - the excludes
# above drop whole subsystems, and a stale dep file would reference
# modules that are not there.
depmod -a \$(ls /lib/modules | head -1)


systemctl enable systemd-networkd
ln -sf /usr/lib/systemd/systemd /init

useradd debian -d /home/debian -G tty,dialout -m -s /bin/bash -e -1
mkdir -p /etc/sudoers.d
echo "debian ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/debian

# Set default passwords
echo 'debian:temppwd' | chpasswd
echo 'root:temppwd' | chpasswd

cat <<EOF > /etc/udev/rules.d/99-recore-otg.rules
# Reflash boots a universal dr_mode=peripheral DTB (model "Recore-all") with no
# Type-C role-switch, so a usb_role role==device event never fires. Bring the
# gadget up when the USB device controller (UDC) appears instead — this works on
# every Recore revision regardless of DTB.
SUBSYSTEM=="udc", ACTION=="add", TAG+="systemd", ENV{SYSTEMD_WANTS}="usb-gadget-setup.service"

# ttyGS0 is owned by the Reflash server's USB control protocol (flasher-pi sees
# it as /dev/ttyACM0), so we do NOT start a login getty on it. Use SSH over the
# network for an interactive console.
EOF

# Careful: this heredoc is nested inside the outer, unquoted ENDOFDEB block
# below (which must stay unquoted, since it expands the KERNEL_DEB variable
# from mkimage.sh's own scope). An unquoted outer heredoc expands its entire
# body - including text that looks like a nested quoted heredoc, and even
# text inside shell comment lines like this one - before any of it reaches
# the chroot'd bash that would otherwise treat this block as fully literal.
# Do not write a bare dollar sign or a backtick anywhere in this comment or
# in the script body below: both trigger expansion (variable substitution or
# command substitution) during that outer pass. Every intentional variable
# or command-substitution reference in the body below is backslash-escaped
# so it survives that pass as literal text instead. Getting this wrong
# previously produced an empty case pattern and an unrolled for-loop list,
# and the gadget never came up.
cat <<'EOF' > /usr/local/bin/usb-gadget-init.sh
#!/bin/bash

GADGET_DIR="/sys/kernel/config/usb_gadget/g1"

case "\$1" in
    start)
        # Pulls in libcomposite + u_serial and registers the usb_gadget configfs subsystem.
        modprobe usb_f_acm || { echo "modprobe usb_f_acm failed" >&2; exit 1; }

        # configfs registration is asynchronous; wait for the gadget subsystem to appear.
        for i in \$(seq 1 50); do
            [ -d /sys/kernel/config/usb_gadget ] && break
            sleep 0.1
        done
        [ -d /sys/kernel/config/usb_gadget ] || { echo "usb_gadget configfs unavailable" >&2; exit 1; }

        # Bind to the first available UDC (musb-hdrc.4.auto on the A64).
        UDC_NAME="\$(ls /sys/class/udc 2>/dev/null | head -1)"
        [ -n "\$UDC_NAME" ] || { echo "no UDC available" >&2; exit 1; }

        mkdir -p \$GADGET_DIR
        cd \$GADGET_DIR

        echo 0x1d6b > idVendor
        echo 0x0104 > idProduct
        echo 0x0200 > bcdUSB

        mkdir -p strings/0x409
        echo "0123456789" > strings/0x409/serialnumber
        echo "Iagent" > strings/0x409/manufacturer
        echo "Recore USB Serial" > strings/0x409/product

        mkdir -p functions/acm.usb0
        mkdir -p configs/c.1/strings/0x409
        echo "Config 1: Serial" > configs/c.1/strings/0x409/configuration

        # Link function to config
        ln -s functions/acm.usb0 configs/c.1/ 2>/dev/null

        # Bind to hardware
        echo "\$UDC_NAME" > UDC
        ;;
    stop)
        if [ -d "\$GADGET_DIR" ]; then
            cd \$GADGET_DIR
            echo "" > UDC
            rm -f configs/c.1/acm.usb0
            [ -d "configs/c.1/strings/0x409" ] && rmdir configs/c.1/strings/0x409
            [ -d "configs/c.1" ] && rmdir configs/c.1
            [ -d "functions/acm.usb0" ] && rmdir functions/acm.usb0
            [ -d "strings/0x409" ] && rmdir strings/0x409
            cd ..
            rmdir g1
        fi
        ;;
esac
EOF

chmod +x /usr/local/bin/usb-gadget-init.sh

cat <<EOF > /etc/systemd/system/usb-gadget-setup.service
[Unit]
Description=USB ConfigFS Gadget Manager

[Service]
Type=oneshot
ExecStart=/usr/local/bin/usb-gadget-init.sh start
ExecStop=/usr/local/bin/usb-gadget-init.sh stop
RemainAfterExit=yes

[Install]
EOF

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

# journald's default is Storage=auto, which writes persistently if
# /var/log/journal exists and volatile (/run) otherwise. This root fs is
# a RAM-backed initramfs capped at half of RAM, while /run is a separate
# and much roomier tmpfs - so the journal belongs there. It already ends
# up in /run today only because that directory happens to be empty; make
# it explicit so a future package cannot silently flip the journal onto
# the space-constrained rootfs.
rm -rf /var/log/journal
mkdir -p /etc/systemd/journald.conf.d
cat <<EOF > /etc/systemd/journald.conf.d/volatile.conf
[Journal]
Storage=volatile
RuntimeMaxUse=16M
EOF

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

ENDOFDEB

cat <<EOF > "${ROOTFSDIR}"/initrd/etc/iwd/main.conf
[General]
EnableNetworkConfiguration=true
EOF

cat <<EOF > "${ROOTFSDIR}"/initrd/etc/systemd/network/20-wired.network
[Match]
Name=eth0

[Network]
DHCP=yes
MulticastDNS=yes
EOF

cat <<EOF > "${ROOTFSDIR}"/initrd/etc/systemd/network/30-wireless.network
[Match]
Name=wlan0

[Network]
DHCP=yes
MulticastDNS=yes
EOF

mkdir -p "${ROOTFSDIR}"/initrd/var/lib/iwd/ap/
cat <<EOF > "${ROOTFSDIR}"/initrd/var/lib/iwd/ap/Recore.ap
[Security]
Passphrase=12345678

[IPv4]
Address=192.168.50.1
Gateway=192.168.50.1
Netmask=255.255.255.0
EOF

cat <<EOF > "${ROOTFSDIR}"/initrd/etc/systemd/network/10-wlan-generic.link
[Match]
Type=wlan

[Link]
Name=wlan0
NamePolicy=keep kernel
EOF

systemctl enable iwd --root="${ROOTFSDIR}"/initrd

mkdir -p "${ROOTFSDIR}"/initrd/etc/systemd/resolved.conf.d/
cat <<EOF > "${ROOTFSDIR}"/initrd/etc/systemd/resolved.conf.d/mdns.conf
[Resolve]
MulticastDNS=yes
EOF

# This board's root fs runs from initrd and doesn't persist writes across
# reboots, so keys generated straight into /etc/ssh would be regenerated
# (and thus change) every boot. ssh-keygen-boot restores/saves them against
# /mnt/usb instead - the one thing that actually persists - so a given board
# keeps a stable identity while still not sharing a key with every other
# image/board (#80). Ordered before both ssh.service and reflash.service so
# it has /mnt/usb to itself - mount-unmount-usb has no locking of its own.
cat <<EOF >"${ROOTFSDIR}"/initrd/etc/systemd/system/ssh-keygen-boot.service
[Unit]
Description=Restore or generate persistent SSH host keys from USB storage (see #80)
Before=ssh.service reflash.service
ConditionPathExists=!/etc/ssh/ssh_host_rsa_key

[Service]
Type=oneshot
ExecStart=/usr/local/bin/ssh-keygen-boot
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

systemctl enable ssh-keygen-boot --root="${ROOTFSDIR}"/initrd

cat <<EOF >"${ROOTFSDIR}"/initrd/etc/systemd/system/reflash.service
[Unit]
Description=Refactor flashing server
After=network.target
Conflicts=getty@tty1.service
Before=getty.target

[Service]
ExecStart=/usr/local/bin/reflash

[Install]
WantedBy=multi-user.target
EOF

systemctl enable reflash --root="${ROOTFSDIR}"/initrd

# Install app
sudo mkdir -p "${ROOTFSDIR}"/initrd/usr/local/bin
sudo cp reflash/reflash "${ROOTFSDIR}"/initrd/usr/local/bin/
sudo mkdir -p "${ROOTFSDIR}"/initrd/usr/local/share/fonts
sudo cp reflash/Roboto-Light.ttf "${ROOTFSDIR}"/initrd/usr/local/share/fonts/
sudo mkdir -p "${ROOTFSDIR}"/initrd/var/www/html/reflash
sudo cp -r client/dist "${ROOTFSDIR}"/initrd/var/www/html/reflash
sudo cp bin/* "${ROOTFSDIR}"/initrd/usr/local/bin
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
