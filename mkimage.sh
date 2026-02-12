#!/bin/bash

set -xeuo pipefail

export ROOTFSDIR=reflash_rootfs
sudo rm -rf "${ROOTFSDIR}"
mkdir -p "${ROOTFSDIR}"

sudo debootstrap --arch=arm64 --foreign --variant=minbase trixie "${ROOTFSDIR}"/initrd http://ftp.no.debian.org/debian/

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
wpasupplicant \
sudo \
iproute2 \
e2fsprogs \
libnss-resolve \
ca-certificates

dpkg -i linux-image-current-sunxi64_26.02.0-trunk_arm64__6.12.69.deb


systemctl enable systemd-networkd
ln -sf /usr/lib/systemd/systemd /init

useradd debian -d /home/debian -G tty,dialout -m -s /bin/bash -e -1
mkdir -p /etc/sudoers.d
echo "debian ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/debian

# Set default passwords
echo 'debian:temppwd' | chpasswd
echo 'root:temppwd' | chpasswd

echo "g_serial" >> /etc/modules
echo "ttyGS0" >> /etc/securetty
systemctl enable serial-getty@ttyGS0.service

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

cat <<EOF > "${ROOTFSDIR}"/initrd/etc/systemd/network/20-wired.network
[Match]
Name=eth0

[Network]
DHCP=yes
MulticastDNS=yes

[Link]
Multicast=yes
EOF

cat <<EOF > "${ROOTFSDIR}"/initrd/etc/systemd/network/30-wireless.network
[Match]
Name=wlan0
[Network]
Address=192.168.50.1/24
DHCPServer=yes
LinkLocalAddressing=yes
MulticastDNS=yes
EOF

cat <<EOF > "${ROOTFSDIR}"/initrd/etc/udev/rules.d/20-wifi.rules
ACTION=="add", SUBSYSTEM=="net", KERNEL=="wlan0", ENV{SYSTEMD_WANTS}+="wpa_supplicant@wlan0.service"
EOF

cat <<EOF > "${ROOTFSDIR}"/initrd/etc/wpa_supplicant/wpa_supplicant-wlan0.conf
ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev
update_config=1
ap_scan=1

network={
    priority=0
    ssid="Recore"
    mode=2
    key_mgmt=WPA-PSK
    psk="12345678"
    frequency=2462
}
EOF

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
