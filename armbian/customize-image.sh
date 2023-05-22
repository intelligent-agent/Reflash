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
    # Disable unique naming scheme
    ln -s /dev/null /etc/systemd/network/99-default.link

    # Install autohotspot script
    cp /tmp/overlay/autohotspot /usr/local/bin
    chmod +x /usr/local/bin/autohotspot

    # Install autohotspot service file
    cp /tmp/overlay/autohotspot.service /etc/systemd/system/

    systemctl enable autohotspot.service
}


install_reflash
install_autohotspot

sh -c 'echo root:kamikaze | chpasswd'

cd /boot
mklost+found
chmod +r /boot/lost+found

# changing dram frequency sems to cause corruption. Set it to max.
echo "class/devfreq/1c62000.dram-controller/governor = performance" > /etc/sysfs.d/dram_governor.conf

echo "Custom script completed"
