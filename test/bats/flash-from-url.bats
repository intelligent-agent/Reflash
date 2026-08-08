#!/usr/bin/env bats
#
# "Magic" flash: bin/prod/flash-from-url. The full success path rewrites UUIDs
# and mounts real partitions (needs root); these tests cover the parts that
# matter most for the bugs we've seen — revision parsing and what happens when
# the download fails (issue #59).

load helper

setup() {
  setup_sandbox
  export REFLASH_TARGET="$SANDBOX/target.img"      # don't touch /dev/mmcblk2
  export REFLASH_CONFIG_DIR="$SANDBOX/config"
  export REFLASH_EMMC_MNT="$SANDBOX/emmc"
  mkdir -p "$REFLASH_CONFIG_DIR" "$REFLASH_EMMC_MNT"
  printf '{ "Revision": "A5" }\n' > "$REFLASH_CONFIG_DIR/recore.json"
  # Neutralise the system tools the script shells out to before the download.
  stub_silent mount-config
  stub_silent unmount-config
  stub_silent mount      # `mount | grep /dev/mmcblk` -> no matches
  stub_silent umount
  stub_silent pv         # passthrough placeholder
}

teardown() { teardown_sandbox; }

@test "flash-from-url: reads and lower-cases the revision from the config json" {
  # wget fails immediately, so the script stops right after the revision banner
  # and never reaches the root-only partition work.
  stub_silent wget 1
  stub_silent xz 0
  run "$PROD_BIN/flash-from-url" http://example/image.img.xz
  [[ "$output" == *"Found Recore hardware revision a5"* ]]
}

@test "flash-from-url: a failed download makes the script exit non-zero (#59)" {
  stub_silent wget 1
  stub_silent xz 0
  run "$PROD_BIN/flash-from-url" http://example/image.img.xz
  [ "$status" -ne 0 ]
}

# Issue #59: the operator currently sees only "unknown error" because the
# download pipeline's stderr is redirected into /tmp/recore-flash-progress
# instead of being surfaced. Un-skip this once flash-from-url reports the real
# cause on stdout/in the log.
@test "flash-from-url: surfaces the real download error (#59 — pending fix)" {
  skip "flash-from-url still redirects pipeline stderr to /tmp; un-skip when #59 is fixed"
  stub wget 1 <<'ERR'
wget: unable to resolve host address 'example'
ERR
  stub_silent xz 0
  run "$PROD_BIN/flash-from-url" http://example/image.img.xz
  [[ "$output" == *"unable to resolve host"* ]]
}
