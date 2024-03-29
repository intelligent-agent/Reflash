#!/bin/bash

# arguments: $RELEASE $LINUXFAMILY $BOARD $BUILD_DESKTOP
#
# This is the image customization script

# NOTE: It is copied to /tmp directory inside the image
# and executed there inside chroot environment
# so don't reference any files that are not already installed

# NOTE: If you want to transfer files between chroot and host
# userpatches/overlay directory on host is bind-mounted to /tmp/overlay in chroot
# The sd card's root path is accessible via $SDCARD variable.

set -e

RELEASE=$1
LINUXFAMILY=$2
BOARD=$3
BUILD_DESKTOP=$4

install_reflash() {
    cd /usr/src
    cp /tmp/overlay/reflash.tar.gz .
    tar -xf reflash.tar.gz
    cd reflash
    chmod +x ./scripts/install_reflash.sh
    ./scripts/install_reflash.sh
}

install_autohotspot() {
    # Install autohotspot script
    cp /tmp/overlay/autohotspot /usr/local/bin
    chmod +x /usr/local/bin/autohotspot

    # Install autohotspot service file
    cp /tmp/overlay/autohotspot.service /etc/systemd/system/

    systemctl enable autohotspot.service
}

fix_netplan(){
    cat <<- EOF > /etc/netplan/armbian-default.yaml
		network:
		  version: 2
		  renderer: NetworkManager
	EOF
}

enable_ttyGS0(){
    echo ttyGS0 >> /etc/securetty
    systemctl enable serial-getty@ttyGS0.service
}

install_reflash
install_autohotspot
fix_netplan
enable_ttyGS0

sh -c 'echo root:temppwd | chpasswd'

cd /boot
mklost+found
chmod +r /boot/lost+found

echo "Custom script completed"
