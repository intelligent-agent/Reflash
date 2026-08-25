#!/bin/sh
# Stop the chroot from starting daemons as packages are installed. mkimage.sh
# deletes this again at the end of the chroot, so it never ships in the image.
exit 101
