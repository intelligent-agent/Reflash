#!/bin/bash

./compile.sh docker \
BOARD=recore \
BRANCH=current \
RELEASE=bullseye \
BUILD_MINIMAL=yes \
BUILD_DESKTOP=no \
KERNEL_ONLY=no \
KERNEL_CONFIGURE=no \
WIREGUARD=no \
BOOTSIZE=96 \
BOOTFS_TYPE="ext4" \
CLEAN_LEVEL="" \
COMPRESS_OUTPUTIMAGE=sha,gpg,xz
