#!/usr/bin/env bats
#
# What ends up *in* the image, as opposed to how a helper behaves.
#
# Nothing here checks a built image - that needs a build - so these test the
# inputs instead: the rootfs_files tree and the parts of mkimage.sh that decide
# whether the tree lands intact. The bug that motivates them (#120) was three
# images shipping without usb-gadget-init.sh, which no test suite noticed
# because none of them looked at the image at all.

load helper

MKIMAGE="$BATS_TEST_DIRNAME/../../mkimage.sh"

# --- the tree replaces the here-docs ----------------------------------------

@test "mkimage.sh writes no files into the rootfs inline" {
  # Seventeen here-docs were the arrangement that allowed #120: a file written
  # inside the chroot and deleted further down, invisibly. Config belongs in
  # rootfs_files/, where deleting it is a diff.
  run grep -nE "^cat[[:space:]]*<<" "$MKIMAGE"
  [ "$status" -ne 0 ] || { echo "inline file writes left in mkimage.sh:"; echo "$output"; false; }
}

@test "mkimage.sh copies both rootfs trees in" {
  grep -q 'cp -a rootfs_files/rootfs-preinstall/\. ' "$MKIMAGE"
  grep -q 'cp -a rootfs_files/rootfs/\. ' "$MKIMAGE"
}

@test "the pre-install tree is copied before the chroot runs" {
  # 01-slim's dpkg excludes only apply to packages unpacked after it exists,
  # and apt-get update inside the chroot needs the apt.conf.d files.
  local preinstall chroot_line
  preinstall="$(grep -n 'cp -a rootfs_files/rootfs-preinstall/\.' "$MKIMAGE" | cut -d: -f1)"
  chroot_line="$(grep -n '^sudo chroot' "$MKIMAGE" | cut -d: -f1)"
  [ -n "$preinstall" ] && [ -n "$chroot_line" ]
  [ "$preinstall" -lt "$chroot_line" ]
}

@test "the rootfs tree is copied after usr/local/bin is rebuilt" {
  # This is the ordering that #120 turned on: the rm -rf that rebuilds
  # /usr/local/bin deletes anything the tree put there if it runs afterwards.
  local rm_line cp_line
  rm_line="$(grep -n 'rm -rf .*initrd/usr/local/bin' "$MKIMAGE" | cut -d: -f1)"
  cp_line="$(grep -n 'cp -a rootfs_files/rootfs/\.' "$MKIMAGE" | cut -d: -f1)"
  [ -n "$rm_line" ] && [ -n "$cp_line" ]
  [ "$rm_line" -lt "$cp_line" ]
}

@test "the rootfs tree is copied before the units are enabled" {
  # systemctl enable --root needs the unit file to exist to link it.
  local cp_line enable_line
  cp_line="$(grep -n 'cp -a rootfs_files/rootfs/\.' "$MKIMAGE" | cut -d: -f1)"
  enable_line="$(grep -n 'systemctl enable .* --root=' "$MKIMAGE" | head -1 | cut -d: -f1)"
  [ -n "$cp_line" ] && [ -n "$enable_line" ]
  [ "$cp_line" -lt "$enable_line" ]
}

@test "mkimage.sh fails the build when a rootfs_files entry is missing" {
  # The build log used to print the installed helper list and nothing checked
  # it, so it printed the evidence of #120 for three releases.
  grep -q "ROOTFS CHECK" "$MKIMAGE"
  grep -q "FATAL: files missing from the built rootfs" "$MKIMAGE"
}

# --- what the units point at has to ship ------------------------------------

@test "every /usr/local/bin program a unit runs is in the repo" {
  # The exact shape of #120: usb-gadget-setup.service naming an executable that
  # the image did not contain. Only "reflash" is exempt - it is the Go binary,
  # built rather than committed.
  local prog missing=""
  while read -r prog; do
    [ "$prog" = reflash ] && continue
    [ -x "$PROD_BIN/$prog" ] || [ -x "$ROOTFS_TREE/usr/local/bin/$prog" ] \
      || missing="$missing $prog"
  done < <(grep -rhoE '^Exec(Start|Stop)=/usr/local/bin/[^ ]+' "$ROOTFS_TREE/etc/systemd" \
             | sed 's|.*/||' | sort -u)
  [ -z "$missing" ] || { echo "units run programs that are not in the repo:$missing"; false; }
}

@test "every unit a udev rule wants exists in the tree or is a systemd template" {
  local unit missing=""
  while read -r unit; do
    # serial-getty@.service and friends come from systemd itself.
    case "$unit" in *@*) continue ;; esac
    [ -f "$ROOTFS_TREE/etc/systemd/system/$unit" ] || missing="$missing $unit"
  done < <(grep -rhoE 'SYSTEMD_WANTS\}="[^"]+"' "$ROOTFS_TREE/etc/udev/rules.d" \
             | sed 's/.*="//; s/"$//' | tr ' ' '\n' | sort -u)
  [ -z "$missing" ] || { echo "udev rules want units that do not ship:$missing"; false; }
}

# --- the gadget script ------------------------------------------------------
#
# It lives in rootfs_files/rootfs, not bin/prod: systemd runs it, Reflash never
# does. It was written into the image by the chroot and then deleted by the
# rm -rf that rebuilds /usr/local/bin, so usb-gadget-setup.service failed with
# "Unable to locate executable" on every image built after that change - no
# /dev/ttyGS*, no ttyACM0/1 on the host, no login and no control protocol.

@test "usb-gadget-init.sh ships as an executable script" {
  [ -x "$ROOTFS_TREE/usr/local/bin/usb-gadget-init.sh" ]
  run bash -n "$ROOTFS_TREE/usr/local/bin/usb-gadget-init.sh"
  [ "$status" -eq 0 ]
}

@test "usb-gadget-init.sh sets up both ACM functions" {
  # acm.usb0 carries the login getty, acm.usb1 the control protocol. If these
  # ever collapse to one, flasher-pi and the getty fight over a single tty.
  grep -q "mkdir -p functions/acm.usb0" "$ROOTFS_TREE/usr/local/bin/usb-gadget-init.sh"
  grep -q "mkdir -p functions/acm.usb1" "$ROOTFS_TREE/usr/local/bin/usb-gadget-init.sh"
}

# The escaping it needed as a nested heredoc is wrong in a standalone file:
# \$UDC_NAME would be a literal backslash rather than the variable.
@test "usb-gadget-init.sh carries no leftover heredoc escaping" {
  run grep -c '\\\$' "$ROOTFS_TREE/usr/local/bin/usb-gadget-init.sh"
  [ "$output" -eq 0 ]
}
