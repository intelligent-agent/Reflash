#!/usr/bin/env bats
#
# Small "getter" helpers whose output the Go server parses directly. Locking
# their output format guards against the kind of "Unknown" / blank-field bugs
# seen in the UI.

load helper

setup() { setup_sandbox; }
teardown() { teardown_sandbox; }

@test "get-reflash-version: prints the contents of the version file" {
  export REFLASH_VERSION_FILE="$SANDBOX/reflash-version"
  echo "v1.2.3" > "$REFLASH_VERSION_FILE"
  run "$PROD_BIN/get-reflash-version"
  [ "$status" -eq 0 ]
  [ "$output" = "v1.2.3" ]
}

@test "get-recore-serial-number: prints the serial from the config mount" {
  export REFLASH_CONFIG_DIR="$SANDBOX/config"
  mkdir -p "$REFLASH_CONFIG_DIR"
  echo "RC-0001-XYZ" > "$REFLASH_CONFIG_DIR/serial_number"
  stub_silent mount-config
  stub_silent unmount-config
  run "$PROD_BIN/get-recore-serial-number"
  [ "$status" -eq 0 ]
  [ "$output" = "RC-0001-XYZ" ]
}

@test "get-recore-serial-number: still unmounts when serial_number is missing (#83 fallout)" {
  export REFLASH_CONFIG_DIR="$SANDBOX/config"
  mkdir -p "$REFLASH_CONFIG_DIR"
  # No serial_number file - this is exactly the state a not-yet-provisioned
  # board is in, i.e. the case this script exists to detect.
  stub_silent mount-config
  stub_silent unmount-config
  run "$PROD_BIN/get-recore-serial-number"
  [ "$status" -ne 0 ]
  assert_called_with "unmount-config"
}

# --- usb-ready: Reflash must mount last ------------------------------------
#
# Reflash holds /mnt/usb for the life of the process, so it waits for whatever
# needs the drive read-write to finish rather than racing it.

# systemctl show -p X --value is called twice: ActiveState then ConditionResult.
stub_unit() {
  cat > "$SHIMDIR/systemctl" <<EOF
#!/usr/bin/env bash
echo "systemctl \$*" >> "$CALLS"
case "\$*" in
  *ConditionResult*) echo "${2:-yes}" ;;
  *ActiveState*)     echo "${1:-inactive}" ;;
esac
EOF
  chmod +x "$SHIMDIR/systemctl"
}

with_partition() {
  dev=$(ls /dev/loop0 /dev/sda /dev/nvme0n1 /dev/vda 2>/dev/null | head -1)
  [ -b "$dev" ] || skip "no block device available to point USB_DEVICE at"
  export USB_DEVICE="$dev"
  export USB_OWNER_UNIT="fake.service"
}

@test "usb-ready: false while the owning unit is still running" {
  with_partition
  stub_unit activating yes
  run "$PROD_BIN/usb-ready"
  [ "$status" -eq 0 ]
  [ "$output" = "false" ]
}

# The race this closes: a oneshot reads "inactive" before it starts as well as
# after a Condition skips it. Mounting in that gap let the unit steal the mount.
@test "usb-ready: false while the owning unit has not started yet" {
  with_partition
  stub_unit inactive yes
  run "$PROD_BIN/usb-ready"
  [ "$output" = "false" ]
}

@test "usb-ready: true once the owning unit has completed" {
  with_partition
  stub_unit active yes
  run "$PROD_BIN/usb-ready"
  [ "$output" = "true" ]
}

# A failed unit is not going to touch the drive again either - waiting for it
# would just burn the timeout and then report no drive.
@test "usb-ready: true when the owning unit failed" {
  with_partition
  stub_unit failed yes
  run "$PROD_BIN/usb-ready"
  [ "$output" = "true" ]
}

@test "usb-ready: true when the owning unit was skipped by its condition" {
  with_partition
  stub_unit inactive no
  run "$PROD_BIN/usb-ready"
  [ "$output" = "true" ]
}

@test "usb-ready: false while the partition does not exist" {
  export USB_DEVICE="$SANDBOX/absent"
  export USB_OWNER_UNIT="fake.service"
  stub_unit active yes
  run "$PROD_BIN/usb-ready"
  [ "$output" = "false" ]
}

# The gadget script's own tests moved to image-files.bats when it moved out of
# bin/prod: it is run by systemd, not by the Go server.

# --- the config partition is read once per boot -----------------------------
#
# It is mounted through a transient systemd unit, and two readers of it collide:
# "Unit mnt-config.mount was already loaded or has a fragment file". Every
# caller used to mount it afresh, so a flash and a UI poll overlapped and
# flash-cleanup was refused its revision (#138). What matters is therefore how
# many times the partition is mounted, not just what comes back.

@test "get-recore-revision: mounts the config partition only once" {
  export REFLASH_CONFIG_DIR="$SANDBOX/config"
  mkdir -p "$REFLASH_CONFIG_DIR"
  printf '{ "Revision": "A7" }\n' > "$REFLASH_CONFIG_DIR/recore.json"
  stub_silent mount-config
  stub_silent unmount-config

  for _ in 1 2 3 4 5; do
    run "$PROD_BIN/get-recore-revision"
    [ "$status" -eq 0 ]
    [ "$output" = "a7" ]
  done

  [ "$(grep -c '^mount-config' "$CALLS")" -eq 1 ]
}

@test "get-recore-serial-number: mounts the config partition only once" {
  export REFLASH_CONFIG_DIR="$SANDBOX/config"
  mkdir -p "$REFLASH_CONFIG_DIR"
  echo "0390" > "$REFLASH_CONFIG_DIR/serial_number"
  stub_silent mount-config
  stub_silent unmount-config

  for _ in 1 2 3; do
    run "$PROD_BIN/get-recore-serial-number"
    [ "$status" -eq 0 ]
    [ "$output" = "0390" ]
  done

  [ "$(grep -c '^mount-config' "$CALLS")" -eq 1 ]
}

# A failure must not be cached, or one transient mount error becomes permanent
# for the rest of the boot - which is the fault #138 describes, with a longer
# lifetime.
@test "get-recore-revision: a failed read is retried, not remembered" {
  export REFLASH_CONFIG_DIR="$SANDBOX/config"
  mkdir -p "$REFLASH_CONFIG_DIR"        # no json at all
  stub_silent mount-config
  stub_silent unmount-config

  run "$PROD_BIN/get-recore-revision"; [ "$status" -ne 0 ]
  run "$PROD_BIN/get-recore-revision"; [ "$status" -ne 0 ]

  [ "$(grep -c '^mount-config' "$CALLS")" -eq 2 ]
}

# Concurrent callers are the actual failure mode: get_info is per-request and
# the UI polls it while a flash is running.
@test "get-recore-revision: concurrent callers still mount only once" {
  export REFLASH_CONFIG_DIR="$SANDBOX/config"
  mkdir -p "$REFLASH_CONFIG_DIR"
  printf '{ "Revision": "A7" }\n' > "$REFLASH_CONFIG_DIR/recore.json"
  stub_silent mount-config
  stub_silent unmount-config

  for _ in $(seq 1 8); do "$PROD_BIN/get-recore-revision" >/dev/null & done
  wait

  [ "$(grep -c '^mount-config' "$CALLS")" -eq 1 ]
}

# Not covered here: create-recore-config removes both cache files, but the rest
# of that script needs root, loop devices and a real boot partition, so a test
# would only exercise the rm - which asserts nothing about the code.
