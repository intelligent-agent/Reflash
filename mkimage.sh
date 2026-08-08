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

dpkg -i ${KERNEL_DEB}


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

# Clean up mounts before exiting the chroot
umount /proc
umount /sys
umount /dev/pts
umount /dev

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
