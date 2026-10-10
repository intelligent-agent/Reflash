#!/usr/bin/env bats

load helper

# target-install and target-manifest: preparing an image through the installer
# it ships (docs/target-interface.md, #179). The eMMC is a directory of plain
# files standing in for partition nodes; mount, the e2fsprogs and chroot are
# shims, and the "installer" is whatever the chroot shim is told to do.

setup() {
  setup_sandbox
  export REFLASH_EMMC="$SANDBOX/dev/mmcblk2"
  export REFLASH_NODE_TEST=-e
  export REFLASH_MANIFEST_MNT="$SANDBOX/p1"
  export REFLASH_TARGET_MNT="$SANDBOX/target"
  mkdir -p "$SANDBOX/dev" "$SANDBOX/p1/reflash" "$SANDBOX/target/usr/lib/reflash"
  : > "${REFLASH_EMMC}p1"
  : > "${REFLASH_EMMC}p2"
  printf '#!/bin/sh\n' > "$SANDBOX/target/usr/lib/reflash/target-installer"
  chmod +x "$SANDBOX/target/usr/lib/reflash/target-installer"

  for c in mount umount sync partprobe udevadm e2fsck parted resize2fs tune2fs flash-cleanup; do
    stub_silent "$c"
  done
  echo 0256 | stub get-recore-serial-number
  echo a5 | stub get-recore-revision
  echo v1.2.0 | stub get-reflash-version

  # The installer, run "inside the image": records its argv, environment and
  # stdin, prints INSTALLER_OUT and exits INSTALLER_RC.
  cat > "$SHIMDIR/chroot" <<EOF
#!/usr/bin/env bash
echo "chroot \$*" >> "$CALLS"
env | grep '^REFLASH_\|^PATH=' | sort > "$SANDBOX/installer.env"
cat > "$SANDBOX/installer.stdin"
printf '%b' "\$(cat "$SANDBOX/installer.out" 2>/dev/null)"
printf '%b' "\$(cat "$SANDBOX/installer.err" 2>/dev/null)" >&2
exit \$(cat "$SANDBOX/installer.rc" 2>/dev/null || echo 0)
EOF
  chmod +x "$SHIMDIR/chroot"
}

teardown() { teardown_sandbox; }

manifest() { printf '%s\n' "$@" > "$SANDBOX/p1/reflash/manifest"; }

v1_manifest() {
  manifest '# Reflash target manifest' interface=1 root=2 boot=1 prepare=ext4-grow-root \
    installer=/usr/lib/reflash/target-installer log=/var/log.hdd/reflash.log
}

# Line number of the first recorded call starting with $1.
call_line() { grep -n "^$1" "$CALLS" | head -1 | cut -d: -f1; }

@test "target-manifest: no manifest prints nothing and succeeds" {
  run "$PROD_BIN/target-manifest"
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "target-manifest: a v1 manifest comes back normalised, unknown keys ignored" {
  manifest interface=1 root=2 future_key=yes prepare=none installer=/usr/lib/x
  run "$PROD_BIN/target-manifest"
  [ "$status" -eq 0 ]
  [[ "$output" == *"ignoring unknown key 'future_key'"* ]]
  [[ "$output" == *$'interface=1\nroot=2\nprepare=none\ninstaller=/usr/lib/x'* ]]
}

@test "target-manifest: found under boot/ when partition 1 is the root" {
  rm -r "$SANDBOX/p1/reflash"
  mkdir -p "$SANDBOX/p1/boot/reflash"
  printf 'interface=1\nroot=1\nprepare=none\ninstaller=/i\n' > "$SANDBOX/p1/boot/reflash/manifest"
  run "$PROD_BIN/target-manifest"
  [ "$status" -eq 0 ]
  [[ "$output" == *"root=1"* ]]
}

@test "target-manifest: refuses what this Reflash cannot honour" {
  manifest interface=2 root=2 prepare=none installer=/i
  run "$PROD_BIN/target-manifest"
  [ "$status" -ne 0 ]
  [[ "$output" == *"interface '2' is not supported"* ]]

  manifest interface=1 root=2 prepare=btrfs-magic installer=/i
  run "$PROD_BIN/target-manifest"
  [ "$status" -ne 0 ]
  [[ "$output" == *"prepare 'btrfs-magic'"* ]]

  manifest interface=1 root=2 prepare=none installer=/usr/../../bin/sh
  run "$PROD_BIN/target-manifest"
  [ "$status" -ne 0 ]

  manifest interface=1 root=2 prepare=none 'installer=usr/lib/x'
  run "$PROD_BIN/target-manifest"
  [ "$status" -ne 0 ]

  manifest interface=1 root=2 boot=2 prepare=none installer=/i
  run "$PROD_BIN/target-manifest"
  [ "$status" -ne 0 ]
}

@test "target-install prepare: an image without a manifest goes to flash-cleanup" {
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -eq 0 ]
  assert_called_with "flash-cleanup a5"
  ! grep -q '^chroot' "$CALLS"
}

@test "target-install configure: an image without a manifest is refused" {
  echo SETTINGS=1 > "$SANDBOX/settings"
  run "$PROD_BIN/target-install" configure "$SANDBOX/settings"
  [ "$status" -ne 0 ]
  ! grep -q '^chroot' "$CALLS"
}

@test "target-install prepare: refuses an empty revision before touching anything" {
  v1_manifest
  run "$PROD_BIN/target-install" prepare ""
  [ "$status" -ne 0 ]
  [ ! -s "$CALLS" ]
}

@test "target-install prepare: an unsupported manifest changes nothing" {
  manifest interface=2 root=2 prepare=ext4-grow-root installer=/usr/lib/reflash/target-installer
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -ne 0 ]
  [[ "$output" == *"cannot be installed by this Reflash"* ]]
  ! grep -qE '^(e2fsck|parted|tune2fs|resize2fs|chroot|flash-cleanup)' "$CALLS"
}

@test "target-install prepare: device step in order, then the installer" {
  v1_manifest
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -eq 0 ]
  assert_called_with "e2fsck -y -f ${REFLASH_EMMC}p1"
  assert_called_with "e2fsck -y -f ${REFLASH_EMMC}p2"
  assert_called_with "parted -s $REFLASH_EMMC resizepart 2 100%"
  assert_called_with "resize2fs ${REFLASH_EMMC}p2"
  assert_called_with "tune2fs -U random ${REFLASH_EMMC}p1"
  assert_called_with "tune2fs -U random ${REFLASH_EMMC}p2"
  assert_called_with "chroot $REFLASH_TARGET_MNT /usr/lib/reflash/target-installer prepare"
  [ "$(call_line partprobe)" -lt "$(call_line e2fsck)" ]
  [ "$(call_line e2fsck)" -lt "$(call_line parted)" ]
  [ "$(call_line parted)" -lt "$(call_line resize2fs)" ]
  [ "$(call_line resize2fs)" -lt "$(call_line tune2fs)" ]
  [ "$(call_line tune2fs)" -lt "$(call_line chroot)" ]
}

@test "target-install prepare: the installer gets exactly the interface's environment" {
  v1_manifest
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -eq 0 ]
  diff - "$SANDBOX/installer.env" <<EOF
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
REFLASH_BOOT_DEV=${REFLASH_EMMC}p1
REFLASH_DEVICE=$REFLASH_EMMC
REFLASH_INTERFACE=1
REFLASH_REVISION=a5
REFLASH_ROOT_DEV=${REFLASH_EMMC}p2
REFLASH_SERIAL=0256
REFLASH_VERSION=v1.2.0
EOF
  [ ! -s "$SANDBOX/installer.stdin" ]
}

@test "target-install prepare: mounts the target and binds /dev /proc /sys, then unmounts" {
  v1_manifest
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -eq 0 ]
  assert_called_with "mount ${REFLASH_EMMC}p2 $REFLASH_TARGET_MNT"
  assert_called_with "mount ${REFLASH_EMMC}p1 $REFLASH_TARGET_MNT/boot"
  assert_called_with "mount --bind /proc $REFLASH_TARGET_MNT/proc"
  assert_called_with "umount $REFLASH_TARGET_MNT/proc"
  assert_called_with "umount $REFLASH_TARGET_MNT/boot"
}

@test "target-install prepare: keeps Reflash's log where the manifest asks" {
  v1_manifest
  echo "[info] earlier line" > "$LOG_FILE"
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -eq 0 ]
  grep -q "earlier line" "$SANDBOX/target/var/log.hdd/reflash.log"
  grep -q "The image is prepared" "$SANDBOX/target/var/log.hdd/reflash.log"
}

@test "target-install prepare: a failing installer fails the install with its reason" {
  v1_manifest
  echo 'working\nERROR: no device tree for a5' > "$SANDBOX/installer.out"
  echo 3 > "$SANDBOX/installer.rc"
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -ne 0 ]
  [[ "$output" == *"installer: working"* ]]
  [[ "$output" == *"installer failed (exit 3): no device tree for a5"* ]]
  # Unmounted on the way out, and the log still kept.
  assert_called_with "umount $REFLASH_TARGET_MNT/boot"
  grep -q "installer failed" "$SANDBOX/target/var/log.hdd/reflash.log"
}

# A shell error is not an ERROR: line; the last thing the installer said is
# still a better reason than the exit status alone (Reflash#184, on A5).
@test "target-install: without an ERROR: line, the installer's last line is the reason" {
  v1_manifest
  printf 'restoring\n/usr/lib/reflash/target-installer: line 329: R: parameter null or not set\n\n' > "$SANDBOX/installer.out"
  echo 1 > "$SANDBOX/installer.rc"
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -ne 0 ]
  [[ "$output" == *"installer failed (exit 1): /usr/lib/reflash/target-installer: line 329: R: parameter null or not set"* ]]
}

@test "target-install prepare: a missing installer is a clear failure" {
  v1_manifest
  rm "$SANDBOX/target/usr/lib/reflash/target-installer"
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -ne 0 ]
  [[ "$output" == *"installer /usr/lib/reflash/target-installer is missing"* ]]
}

@test "target-install prepare: e2fsck repairs pass, uncorrected errors fail" {
  v1_manifest
  stub_silent e2fsck 1
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -eq 0 ]
  [[ "$output" == *"errors found and repaired"* ]]

  : > "$CALLS"
  stub_silent e2fsck 4
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -ne 0 ]
  ! grep -q '^chroot' "$CALLS"
}

@test "target-install prepare: prepare=none leaves the device alone" {
  manifest interface=1 root=2 prepare=none installer=/usr/lib/reflash/target-installer
  run "$PROD_BIN/target-install" prepare a5
  [ "$status" -eq 0 ]
  ! grep -qE '^(e2fsck|parted|tune2fs|resize2fs)' "$CALLS"
  assert_called_with "chroot $REFLASH_TARGET_MNT /usr/lib/reflash/target-installer prepare"
  grep -q '^REFLASH_BOOT_DEV=$' "$SANDBOX/installer.env"
}

@test "target-install configure: settings reach the installer on stdin, device untouched" {
  v1_manifest
  printf 'SETTINGS=1\nWIFI_PSK=hunter2\n' > "$SANDBOX/settings"
  run "$PROD_BIN/target-install" configure "$SANDBOX/settings"
  [ "$status" -eq 0 ]
  assert_called_with "chroot $REFLASH_TARGET_MNT /usr/lib/reflash/target-installer configure"
  diff "$SANDBOX/settings" "$SANDBOX/installer.stdin"
  ! grep -qE '^(e2fsck|parted|tune2fs|resize2fs)' "$CALLS"
  # configure keeps no log on the target: that belongs to a flash.
  [ ! -e "$SANDBOX/target/var/log.hdd/reflash.log" ]
  ! grep -q hunter2 "$LOG_FILE"
}

optional_manifest() {
  manifest interface=1 root=2 boot=1 prepare=ext4-grow-root \
    installer=/usr/lib/reflash/target-installer "$@"
}

@test "target-manifest: settings= and actions= come through; unknown actions and bad lists are dropped" {
  optional_manifest settings=SSH_ENABLED,LOGIN_PASSWORD actions=settings,teleport,backup
  run "$PROD_BIN/target-manifest"
  [ "$status" -eq 0 ]
  [[ "$output" == *"settings=SSH_ENABLED,LOGIN_PASSWORD"* ]]
  [[ "$output" == *"actions=settings,backup"* ]]
  [[ "$output" == *"ignoring unknown action 'teleport'"* ]]

  optional_manifest 'settings=ssh enabled;rm' actions=settings
  run "$PROD_BIN/target-manifest"
  [ "$status" -eq 0 ]
  [[ "$output" != *"settings=ssh"* ]]
  [[ "$output" == *"ignoring settings"* ]]
}

@test "target-install settings: prints only what the installer wrote to stdout" {
  optional_manifest actions=settings
  printf 'SETTINGS=1\\nSSH_ENABLED=true\\n' > "$SANDBOX/installer.out"
  echo 'reading weston.ini' > "$SANDBOX/installer.err"
  run --separate-stderr "$PROD_BIN/target-install" settings
  [ "$status" -eq 0 ]
  [ "$output" = $'SETTINGS=1\nSSH_ENABLED=true' ]
  assert_called_with "chroot $REFLASH_TARGET_MNT /usr/lib/reflash/target-installer settings"
  grep -q "installer: reading weston.ini" "$LOG_FILE"
  ! grep -qE '^(e2fsck|parted|tune2fs)' "$CALLS"
}

@test "target-install settings secrets: the installer is told, and the passphrase stays out of the log" {
  optional_manifest actions=settings
  printf 'SETTINGS=1\\nWIFI_SSID=home\\nWIFI_PSK=hunter2\\n' > "$SANDBOX/installer.out"
  run --separate-stderr "$PROD_BIN/target-install" settings secrets
  [ "$status" -eq 0 ]
  [[ "$output" == *"WIFI_PSK=hunter2"* ]]
  assert_called_with "chroot $REFLASH_TARGET_MNT /usr/lib/reflash/target-installer settings secrets"
  ! grep -q hunter2 "$LOG_FILE"
}

@test "target-install settings: any word but secrets is refused, and plain settings does not ask for them" {
  optional_manifest actions=settings
  run "$PROD_BIN/target-install" settings everything
  [ "$status" -eq 2 ]
  ! grep -q '^chroot' "$CALLS"
  printf 'SETTINGS=1\\n' > "$SANDBOX/installer.out"
  run "$PROD_BIN/target-install" settings
  [ "$status" -eq 0 ]
  assert_called_with "chroot $REFLASH_TARGET_MNT /usr/lib/reflash/target-installer settings"
}

@test "target-install: an optional action the manifest does not list is not supported (3)" {
  optional_manifest actions=settings
  run "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  [ "$status" -eq 3 ]
  ! grep -q '^chroot' "$CALLS"
}

@test "target-install: optional actions on an image without a manifest are not supported (3)" {
  run "$PROD_BIN/target-install" settings
  [ "$status" -eq 3 ]
  ! grep -qE '^(chroot|flash-cleanup)' "$CALLS"
}

@test "target-install backup and restore: the archive goes to the file and comes back on stdin" {
  optional_manifest actions=backup,restore
  printf 'ARCHIVE' > "$SANDBOX/installer.out"
  run "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  [ "$status" -eq 0 ]
  [ "$(cat "$SANDBOX/b.tgz")" = ARCHIVE ]

  : > "$SANDBOX/installer.out"
  run "$PROD_BIN/target-install" restore "$SANDBOX/b.tgz"
  [ "$status" -eq 0 ]
  [ "$(cat "$SANDBOX/installer.stdin")" = ARCHIVE ]
  assert_called_with "chroot $REFLASH_TARGET_MNT /usr/lib/reflash/target-installer restore"
}

@test "target-install: an installer that says 3 to an optional action means not supported" {
  optional_manifest actions=backup
  echo 3 > "$SANDBOX/installer.rc"
  run "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  [ "$status" -eq 3 ]
}

@test "target-install: the installer's lines reach the log while it runs, not when it ends" {
  v1_manifest
  # An installer that says where it is, then hangs until told to go on - as
  # one did on a8 when its eMMC stopped answering.
  cat > "$SHIMDIR/chroot" <<SHIM
#!/usr/bin/env bash
echo "flashing the STM32"
for _ in \$(seq 1 100); do [ -e "$SANDBOX/go" ] && exit 0; sleep 0.1; done
exit 9
SHIM
  chmod +x "$SHIMDIR/chroot"
  "$PROD_BIN/target-install" prepare a5 > "$SANDBOX/out" 2>&1 &
  pid=$!
  for _ in $(seq 1 50); do grep -q "installer: flashing the STM32" "$LOG_FILE" && break; sleep 0.1; done
  grep -q "installer: flashing the STM32" "$LOG_FILE"
  touch "$SANDBOX/go"
  wait "$pid"
}

# --- Rebuild v1.0/v1.1: backed up by Reflash itself (#187) -----------------

# A release with no manifest, laid out as the real ones are: v1.0.x keeps
# everything under debian's home (checked against the v1.0.2 image), v1.1.0
# under a printer user.
legacy_system() {
  local user=$1 version=$2 t="$SANDBOX/target"
  echo "$version" > "$t/etc-rebuild-version.tmp"
  mkdir -p "$t/etc" "$t/home/$user/printer_data/config" "$t/home/$user/printer_data/database" "$t/home/$user/printer_data/logs"
  mv "$t/etc-rebuild-version.tmp" "$t/etc/rebuild-version"
  echo recore > "$t/etc/hostname"
  echo "[printer]" > "$t/home/$user/printer_data/config/printer.cfg"
  echo "[server]" > "$t/home/$user/printer_data/config/moonraker.conf"
  echo "bkp" > "$t/home/$user/printer_data/config/.moonraker.conf.bkp"
  echo "db" > "$t/home/$user/printer_data/database/moonraker-sql.db"
  echo "log" > "$t/home/$user/printer_data/logs/klippy.log"
}

@test "legacy backup: v1.0.2 keeps everything under debian; the archive is renamed to what a current system restores" {
  legacy_system debian rebuild-fluidd-v1.0.2
  run "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  [ "$status" -eq 0 ]
  list=$(tar tzf "$SANDBOX/b.tgz")
  # The manifest first, as the installer writes it.
  [ "$(echo "$list" | head -1)" = rebuild-backup.manifest ]
  [[ "$list" == *"home/printer/printer_data/config/printer.cfg"* ]]
  [[ "$list" == *"home/printer/printer_data/config/moonraker.conf"* ]]
  [[ "$list" == *"home/printer/printer_data/database/moonraker-sql.db"* ]]
  # Nothing under the old user's name, or the new image's restore finds nothing.
  [[ "$list" != *"home/debian"* ]]
  # Not logs, and not Moonraker's own copy of its config.
  [[ "$list" != *klippy.log* ]]
  [[ "$list" != *".moonraker.conf.bkp"* ]]
  # Owned by printer by name, which is how a restore gives them to the new user.
  tar tvzf "$SANDBOX/b.tgz" | grep -q "printer/printer .*printer_data/config/printer.cfg"
}

@test "legacy backup: the manifest says where the files came from, as the installer's does" {
  legacy_system debian rebuild-fluidd-v1.0.2
  "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  m=$(tar xzOf "$SANDBOX/b.tgz" rebuild-backup.manifest)
  [[ "$m" == *"format=1"* ]]
  [[ "$m" == *"rebuild_version=rebuild-fluidd-v1.0.2"* ]]
  [[ "$m" == *"hostname=recore"* ]]
  [[ "$m" == *"board_revision=a5"* ]]
  [[ "$m" == *"board_serial=0256"* ]]
  [[ "$m" == *"reflash_version=v1.2.0"* ]]
  [[ "$m" == *"klipper_version="$'\n'* ]]
  # The paths as the new image will see them.
  [[ "$m" == *"paths=home/printer/printer_data/config home/printer/printer_data/database"* ]]
  [[ "$m" != *"home/debian"* ]]
}

@test "legacy backup: v1.1.0 keeps its printer user's names" {
  legacy_system printer rebuild-fluidd-v1.1.0
  run "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  [ "$status" -eq 0 ]
  list=$(tar tzf "$SANDBOX/b.tgz")
  [[ "$list" == *"home/printer/printer_data/config/printer.cfg"* ]]
  tar tvzf "$SANDBOX/b.tgz" | grep -q "printer/printer .*printer_data/config/moonraker.conf"
}

@test "legacy backup: OctoPrint's settings, users and data, without its timelapses" {
  legacy_system debian rebuild-octoprint-v1.0.2
  o="$SANDBOX/target/home/debian/.octoprint"
  mkdir -p "$o/data/timelapse" "$o/data/gcodeEditor"
  echo c > "$o/config.yaml"; echo u > "$o/users.yaml"
  echo s > "$o/data/gcodeEditor/settings.yaml"; echo t > "$o/data/timelapse/frame.jpg"
  run "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  [ "$status" -eq 0 ]
  list=$(tar tzf "$SANDBOX/b.tgz")
  [[ "$list" == *"home/printer/.octoprint/config.yaml"* ]]
  [[ "$list" == *"home/printer/.octoprint/users.yaml"* ]]
  [[ "$list" == *"home/printer/.octoprint/data/gcodeEditor/settings.yaml"* ]]
  [[ "$list" != *timelapse/frame.jpg* ]]
}

@test "legacy backup: a system with none of the files still makes a backup - the manifest alone" {
  mkdir -p "$SANDBOX/target/etc"
  echo rebuild-barebone-v1.1.0 > "$SANDBOX/target/etc/rebuild-version"
  run "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  [ "$status" -eq 0 ]
  [ "$(tar tzf "$SANDBOX/b.tgz")" = rebuild-backup.manifest ]
}

@test "legacy backup: the system is read, never written - mounted read-only and unmounted after" {
  legacy_system debian rebuild-fluidd-v1.0.2
  "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
  grep -q "^mount -o ro ${REFLASH_EMMC}p2 " "$CALLS"
  [ "$(call_line umount)" -gt "$(call_line mount)" ]
  ! grep -qE '^(e2fsck|parted|tune2fs|resize2fs|chroot)' "$CALLS"
}

@test "legacy backup: a release Reflash does not know is not supported (3), and nothing is written" {
  for v in rebuild-fluidd-v1.9.9 rebuild-fluidd-v2.0.0 "Unknown version" "refactor-v0.9" ""; do
    legacy_system debian "$v"
    rm -f "$SANDBOX/b.tgz"
    run "$PROD_BIN/target-install" backup "$SANDBOX/b.tgz"
    [ "$status" -eq 3 ]
    [ ! -e "$SANDBOX/b.tgz" ]
  done
}

@test "legacy backup: only backup - restore, settings and configure on such a system are unchanged" {
  legacy_system debian rebuild-fluidd-v1.0.2
  echo x > "$SANDBOX/in.tgz"
  run "$PROD_BIN/target-install" restore "$SANDBOX/in.tgz"
  [ "$status" -eq 3 ]
  run "$PROD_BIN/target-install" settings
  [ "$status" -eq 3 ]
}

# ---- #198: include lists and listing ----

@test "target-install backup and restore: --include paths reach the installer as arguments" {
  v1_manifest
  printf 'actions=settings,backup,restore,list,list-archive\n' >> "$SANDBOX/p1/reflash/manifest"
  printf 'ARCHIVE' > "$SANDBOX/installer.out"
  run "$PROD_BIN/target-install" backup "$SANDBOX/out.tgz" --include home/printer/printer_data/config/printer.cfg --include "home/printer/printer_data/config/my files"
  [ "$status" -eq 0 ]
  grep -q 'target-installer backup --include home/printer/printer_data/config/printer.cfg --include home/printer/printer_data/config/my files$' "$CALLS"
  : > "$SANDBOX/in.tgz"
  run "$PROD_BIN/target-install" restore "$SANDBOX/in.tgz" --include home/printer/printer_data/config
  [ "$status" -eq 0 ]
  grep -q 'target-installer restore --include home/printer/printer_data/config$' "$CALLS"
}

@test "target-install: a stray argument is refused, not passed on" {
  v1_manifest
  printf 'actions=backup,restore,list\n' >> "$SANDBOX/p1/reflash/manifest"
  run "$PROD_BIN/target-install" backup "$SANDBOX/out.tgz" --exec /bin/sh
  [ "$status" -ne 0 ]
  [[ "$output" == *"unknown argument"* ]]
  run "$PROD_BIN/target-install" backup "$SANDBOX/out.tgz" --include
  [ "$status" -ne 0 ]
  run "$PROD_BIN/target-install" list --include x
  [ "$status" -ne 0 ]
}

@test "target-install list and list-archive: the paths alone on stdout, no log lines among them" {
  v1_manifest
  printf 'actions=list,list-archive\n' >> "$SANDBOX/p1/reflash/manifest"
  printf 'a/one.cfg\na/two.cfg\n' > "$SANDBOX/installer.out"
  run --separate-stderr "$PROD_BIN/target-install" list
  [ "$status" -eq 0 ]
  [ "$output" = $'a/one.cfg\na/two.cfg' ]
  : > "$SANDBOX/in.tgz"
  run --separate-stderr "$PROD_BIN/target-install" list-archive "$SANDBOX/in.tgz"
  [ "$status" -eq 0 ]
  [ "$output" = $'a/one.cfg\na/two.cfg' ]
  [ "$(cat "$SANDBOX/installer.stdin" | wc -c)" -eq 0 ]
}

@test "target-install list: an image that does not list it exits 3" {
  v1_manifest
  printf 'actions=settings,backup\n' >> "$SANDBOX/p1/reflash/manifest"
  run "$PROD_BIN/target-install" list
  [ "$status" -eq 3 ]
  run "$PROD_BIN/target-install" list-archive "$SANDBOX/in.tgz"
  [ "$status" -ne 0 ]
}

@test "target-manifest: the list actions are known" {
  manifest interface=1 root=2 prepare=none installer=/i actions=settings,list,list-archive,bogus
  run "$PROD_BIN/target-manifest"
  [[ "$output" == *"actions=settings,list,list-archive"* ]]
  [[ "$output" == *"unknown action 'bogus'"* ]]
}
