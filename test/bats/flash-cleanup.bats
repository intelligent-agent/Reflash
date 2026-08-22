#!/usr/bin/env bats

load helper

# flash-cleanup mounts real block devices and hardcodes /mnt/emmc and
# /dev/mmcblk2 throughout, with no env seams - so unlike the other suites here,
# these are static checks on the script text rather than runs of it. Retrofitting
# seams to the flash path is a refactor of its own; this at least pins the
# defects that shipped.

setup() { SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../../bin/prod" && pwd)/flash-cleanup"; }

line_of() { grep -n "$1" "$SCRIPT" | head -1 | cut -d: -f1; }

# partprobe returns once the kernel has re-read the partition table, but the
# kernel removes the partition devices first and udev recreates them afterwards.
# e2fsck running in that window dies with "No such file or directory while
# trying to open /dev/mmcblk2p1", and because the script is set -euo pipefail
# that skips the resize, the UUID randomisation and the device tree symlink -
# leaving a board that boots but is subtly wrong. Observed flashing an A5.
@test "flash-cleanup: waits for udev after partprobe, before touching partitions" {
  local partprobe settle fsck
  partprobe=$(line_of '^partprobe ')
  settle=$(line_of 'udevadm settle')
  fsck=$(line_of '^e2fsck ')
  [ -n "$partprobe" ]
  [ -n "$settle" ]
  [ -n "$fsck" ]
  [ "$partprobe" -lt "$settle" ]
  [ "$settle" -lt "$fsck" ]
}

# udevadm settle alone is not enough - it can return before the nodes are back,
# and it is best-effort here (|| true), so the explicit poll is what actually
# guarantees the device exists.
@test "flash-cleanup: polls for both partition nodes, not just settle" {
  run grep -c 'if \[ -b "${OUTFILE}p1" \] && \[ -b "${OUTFILE}p2" \]; then break; fi' "$SCRIPT"
  [ "$status" -eq 0 ]
  [ "$output" -eq 1 ]
}

@test "flash-cleanup: gives up loudly if the nodes never return" {
  run grep -c 'Partition nodes did not reappear' "$SCRIPT"
  [ "$status" -eq 0 ]
  [ "$output" -eq 1 ]
}
