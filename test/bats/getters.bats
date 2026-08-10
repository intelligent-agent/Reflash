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
