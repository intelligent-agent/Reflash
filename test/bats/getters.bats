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

@test "usb-ready: not ready while the owning unit is still running" {
  export USB_DEVICE="$SANDBOX/sda2"
  export USB_OWNER_UNIT="fake.service"
  printf '' > "$USB_DEVICE"   # not a block device, but present
  stub systemctl <<'OUT'
activating
OUT
  run "$PROD_BIN/usb-ready"
  [ "$status" -ne 0 ]
}

@test "usb-ready: not ready while the partition does not exist" {
  export USB_DEVICE="$SANDBOX/absent"
  export USB_OWNER_UNIT="fake.service"
  stub systemctl <<'OUT'
active
OUT
  run "$PROD_BIN/usb-ready"
  [ "$status" -ne 0 ]
}

# A unit skipped by its ConditionPathExists reports inactive, never active -
# that must count as "not going to touch the drive", not as "wait forever".
@test "usb-ready: an inactive owner does not block forever" {
  # Needs a real block device: the check is -b, and creating one needs root.
  dev=$(ls /dev/loop0 /dev/sda /dev/nvme0n1 /dev/vda 2>/dev/null | head -1)
  [ -b "$dev" ] || skip "no block device available to point USB_DEVICE at"
  export USB_DEVICE="$dev"
  export USB_OWNER_UNIT="fake.service"
  stub systemctl <<'OUT'
inactive
OUT
  run "$PROD_BIN/usb-ready"
  [ "$status" -eq 0 ]
}
