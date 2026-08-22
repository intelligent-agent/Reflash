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

# Every board flashed from an image built on the same cached Armbian rootfs got
# the same SSH host keys. `ssh-keygen -A` only generates keys "if they do not
# already exist", the images do ship them, and flash-cleanup disables Armbian's
# own first-boot regeneration a few lines earlier - so nothing ever replaced
# them. Two boards flashed from rebuild-barebone-891e72c both presented
# SHA256:tADe3k31lQ/ApIZZe6lGD0A3bFrI/argPFJ5aeIlPQo.
@test "flash-cleanup: removes shipped host keys before pre-generating" {
  run grep -c '^rm -f /mnt/emmc/etc/ssh/ssh_host_\*$' "$SCRIPT"
  [ "$status" -eq 0 ]
  [ "$output" -eq 1 ]
}

@test "flash-cleanup: the removal comes before ssh-keygen -A, not after" {
  local rm_line keygen_line
  # Anchored to the commands: both strings also appear in the comment above them.
  rm_line=$(grep -n '^rm -f /mnt/emmc/etc/ssh/ssh_host_\*$' "$SCRIPT" | cut -d: -f1)
  keygen_line=$(grep -n '^ssh-keygen -A ' "$SCRIPT" | cut -d: -f1)
  [ -n "$rm_line" ]
  [ -n "$keygen_line" ]
  # Ordering is the whole point: after the generate, it would delete the fresh
  # keys and leave the board with none.
  [ "$rm_line" -lt "$keygen_line" ]
}

# Guards the assumption the fix rests on. If a future ssh-keygen overwrote
# existing keys, the rm would be redundant - but it does not, and this fails
# loudly if that ever changes.
@test "ssh-keygen -A does not replace keys that already exist" {
  setup_sandbox
  local dir="$SANDBOX/root"
  mkdir -p "$dir/etc/ssh"

  ssh-keygen -A -f "$dir" >/dev/null 2>&1
  local before
  before=$(ssh-keygen -lf "$dir/etc/ssh/ssh_host_ed25519_key.pub" | awk '{print $2}')

  ssh-keygen -A -f "$dir" >/dev/null 2>&1
  local after
  after=$(ssh-keygen -lf "$dir/etc/ssh/ssh_host_ed25519_key.pub" | awk '{print $2}')

  [ "$before" = "$after" ]

  # ...and that deleting first does produce a different key.
  rm -f "$dir"/etc/ssh/ssh_host_*
  ssh-keygen -A -f "$dir" >/dev/null 2>&1
  local regenerated
  regenerated=$(ssh-keygen -lf "$dir/etc/ssh/ssh_host_ed25519_key.pub" | awk '{print $2}')
  [ "$regenerated" != "$after" ]

  teardown_sandbox
}
