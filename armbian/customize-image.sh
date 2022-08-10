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

VERSION="v0.0.6-RC0"
apt install -y nginx gunicorn unzip

cd /usr/src
wget "https://github.com/intelligent-agent/Reflash/releases/download/${VERSION}/reflash.zip"
unzip reflash.zip
cd reflash
chmod +x ./scripts/install_reflash.sh
./scripts/install_reflash.sh

wget https://github.com/intelligent-agent/Recore/raw/master/Device_tree/sun50i-a64-recore-a4.dtb
wget https://github.com/intelligent-agent/Recore/raw/master/Device_tree/sun50i-a64-recore-a5.dtb
wget https://github.com/intelligent-agent/Recore/raw/master/Device_tree/sun50i-a64-recore-a6.dtb

mv sun50i-a64-recore-a*.dtb /boot/dtb/allwinner/
cd /boot/dtb/allwinner/; ln -s sun50i-a64-recore-a6.dtb sun50i-a64-recore.dtb
sh -c 'echo root:kamikaze | chpasswd'
echo "${VERSION}" > /etc/reflash.version
echo "Custom script completed"
