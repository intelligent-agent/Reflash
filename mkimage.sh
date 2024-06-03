#!/bin/bash

set -xeuo pipefail

export ROOTFSDIR=reflash_rootfs
sudo rm -rf "${ROOTFSDIR}"
mkdir -p "${ROOTFSDIR}"

sudo debootstrap --arch=arm64 --foreign --variant=minbase bookworm "${ROOTFSDIR}"/initrd

if [ ! -f rootfs_files/debs/linux-dtb-legacy-sunxi64_23.08.0-trunk_arm64__5.15.127.deb ]; then
    wget -P rootfs_files/debs/ http://feeds.iagent.no/debian/pool/main/linux-dtb-legacy-sunxi64_23.08.0-trunk_arm64__5.15.127.deb
    wget -P rootfs_files/debs/ http://feeds.iagent.no/debian/pool/main/linux-image-legacy-sunxi64_23.08.0-trunk_arm64__5.15.127.deb
fi

sudo cp rootfs_files/debs/* "${ROOTFSDIR}"/initrd

sudo bash -c "echo recore > ${ROOTFSDIR}/initrd/etc/hostname"

sudo chroot "${ROOTFSDIR}"/initrd /bin/bash <<ENDOFDEB
export DEBIAN_FRONTEND="noninteractive"
export TERM=xterm-color
/debootstrap/debootstrap --second-stage
export LC_ALL=C

dpkg -i linux-dtb-legacy-sunxi64_23.08.0-trunk_arm64__5.15.127.deb
dpkg -i linux-image-legacy-sunxi64_23.08.0-trunk_arm64__5.15.127.deb

apt install -y systemd-resolved systemd openssh-server udev kmod fdisk parted ca-certificates xz-utils pv systemd-timesyncd wget wpasupplicant sudo policykit-1 iproute2 --no-install-recommends --no-install-suggests
systemctl enable systemd-networkd
ln -s /lib/systemd/systemd /init

useradd debian -d /home/debian -G tty,dialout -m -s /bin/bash -e -1
echo "debian ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/debian

# Set default passwords
echo 'debian:temppwd' | chpasswd
echo 'root:temppwd' | chpasswd

# Clean up
rm ./*.deb
rm -rf /var/lib/apt/lists/
rm -rf /var/cache/
rm -rf /usr/share/locale/

find /usr/share/doc -depth -type f -print0 ! -name copyright | xargs -0 rm
find /usr/share/doc -empty -print0 | xargs -0 rmdir
rm -rf /usr/share/man/* /usr/share/groff/* /usr/share/info/*
rm -rf /usr/share/lintian/* /usr/share/linda/* /var/cache/man/*

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

systemctl enable wpa_supplicant@wlan0.service --root="${ROOTFSDIR}"/initrd

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
sudo cp bin/prod/* "${ROOTFSDIR}"/initrd/usr/local/bin
sudo mkdir -p "${ROOTFSDIR}"/initrd/mnt/usb

TAG=$(git describe --always --tags)
NAME="reflash-${TAG}"
echo "$NAME" > "$ROOTFSDIR"/initrd/etc/reflash-version

# Move the boot folder outside the rootfs
sudo rm -rf "${ROOTFSDIR}"/boot
sudo mv "${ROOTFSDIR}"/initrd/boot/ "${ROOTFSDIR}"

# Compile and copy extra files
sudo cp rootfs_files/files/* "${ROOTFSDIR}"/boot
mkimage -C none -A arm -T script -d "${ROOTFSDIR}"/boot/boot.cmd "${ROOTFSDIR}"/boot/boot.scr

# Crate initramfs
sudo bash -c "cd '${ROOTFSDIR}/initrd' && find . | cpio -ov --format=newc | gzip -9 >'../initrd.img.gz'" >/dev/null 2>&1

#cd "${ROOTFSDIR}"/initrd; sudo bash -c "find . | cpio -ov --format=newc | gzip -9 >../initrd.img.gz";  cd ../..
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
mv "${ROOTFSDIR}"/reflash.img.xz ./${NAME}.img.xz
