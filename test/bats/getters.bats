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

# --- the gadget script must actually ship ----------------------------------
#
# It was written into the image by the chroot and then deleted by the rm -rf
# that rebuilds /usr/local/bin, so usb-gadget-setup.service failed with
# "Unable to locate executable" on every image built after that change - no
# /dev/ttyGS*, no ttyACM0/1 on the host, no login and no control protocol.

@test "usb-gadget-init.sh is a helper, not written inside mkimage's chroot" {
  [ -x "$PROD_BIN/usb-gadget-init.sh" ]
  run bash -n "$PROD_BIN/usb-gadget-init.sh"
  [ "$status" -eq 0 ]
}

@test "usb-gadget-init.sh sets up both ACM functions" {
  # acm.usb0 carries the login getty, acm.usb1 the control protocol. If these
  # ever collapse to one, flasher-pi and the getty fight over a single tty.
  grep -q "mkdir -p functions/acm.usb0" "$PROD_BIN/usb-gadget-init.sh"
  grep -q "mkdir -p functions/acm.usb1" "$PROD_BIN/usb-gadget-init.sh"
}

# The escaping it needed as a nested heredoc is wrong in a standalone file:
# \$UDC_NAME would be a literal backslash rather than the variable.
@test "usb-gadget-init.sh carries no leftover heredoc escaping" {
  run grep -c '\\\$' "$PROD_BIN/usb-gadget-init.sh"
  [ "$output" -eq 0 ]
}
