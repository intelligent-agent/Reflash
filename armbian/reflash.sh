#!/bin/bash

./compile.sh docker \
BOARD=recore \
BRANCH=current \
RELEASE=bullseye \
BUILD_ONLY=default \
BUILD_DESKTOP=no \
KERNEL_CONFIGURE=no \
WIREGUARD=no \
BOOTSIZE=96 \
BOOTFS_TYPE=ext4 \
CLEAN_LEVEL=image \
COMPRESS_OUTPUTIMAGE=sha,gpg,xz
