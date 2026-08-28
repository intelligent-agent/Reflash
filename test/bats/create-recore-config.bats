#!/usr/bin/env bats
#
# create-recore-config writes the serial number and calibration file onto the
# eMMC boot partition. Everything it does needs root, loop devices and real
# block devices - so these tests stub those and cover the parts that can go
# wrong independently of the hardware: the serial-number-to-revision mapping,
# the two distinguishable download failures, and the cache invalidation.

load helper

setup() {
  setup_sandbox
  export REFLASH_SYS_BLOCK="$SANDBOX/sys/block"
  export REFLASH_CONFIG_MNT="$SANDBOX/mnt/config"
  mkdir -p "$REFLASH_SYS_BLOCK/mmcblk2boot0" "$REFLASH_CONFIG_MNT" "$REFLASH_CACHE_DIR"

  stub lsblk <<< "mmcblk2"
  stub_silent wget            # --spider succeeds, then the download succeeds
  stub_silent blockdev
  stub losetup <<< "/dev/loop9"
  stub_silent fdisk
  stub_silent mkfs.ext4
  stub_silent mount
  stub_silent umount
  stub_silent dd
  stub_silent sync
}

teardown() { teardown_sandbox; }

# The point of the whole exercise: after provisioning, the getters must not go
# on serving the identity the board had BEFORE it was provisioned. They cache
# for the life of the boot, so nothing else would ever drop it.
@test "create-recore-config: drops the cached identity" {
  echo "a5"   > "$REFLASH_CACHE_DIR/recore-revision"
  echo "0001" > "$REFLASH_CACHE_DIR/recore-serial-number"

  run "$PROD_BIN/create-recore-config" 390
  [ "$status" -eq 0 ]

  [ ! -f "$REFLASH_CACHE_DIR/recore-revision" ]
  [ ! -f "$REFLASH_CACHE_DIR/recore-serial-number" ]
}

# ...and it must not do that on the way out of a FAILED run, or a board whose
# provisioning fell over would start reporting nothing at all.
@test "create-recore-config: leaves the cache alone when it cannot provision" {
  echo "a7" > "$REFLASH_CACHE_DIR/recore-revision"

  run "$PROD_BIN/create-recore-config" 99      # outside every revision range
  [ "$status" -ne 0 ]

  [ -f "$REFLASH_CACHE_DIR/recore-revision" ]
  [ "$(cat "$REFLASH_CACHE_DIR/recore-revision")" = "a7" ]
}

@test "create-recore-config: maps the serial number to a hardware revision" {
  run "$PROD_BIN/create-recore-config" 390
  [ "$status" -eq 0 ]
  [[ "$output" == *"Hardware revision: A7"* ]]
  [[ "$output" == *"Serial number: 0390"* ]]
}

# wget distinguishes "no network at all" (4) from "server said no" (a 404),
# and the user-facing message differs.
@test "create-recore-config: no internet is reported as no internet" {
  stub_silent wget 4
  run "$PROD_BIN/create-recore-config" 390
  [ "$status" -eq 3 ]
  [[ "$output" == *"No internet connection"* ]]
}

@test "create-recore-config: a missing calibration file names the serial number" {
  stub_silent wget 8
  run "$PROD_BIN/create-recore-config" 390
  [ "$status" -eq 2 ]
  [[ "$output" == *"No calibration file found for serial number 0390"* ]]
}
