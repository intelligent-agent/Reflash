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
  # Matched anywhere on the line, not at the start: the call now lives inside
  # check_fs, which is defined after the settle it must not run before.
  fsck=$(line_of 'e2fsck ')
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

# --- e2fsck's exit status is a bitmask, not a boolean (#127) ----------------
#
# It returns 1 when it finds errors AND CORRECTS THEM, which is a success. Under
# this script's set -e a bare call aborted the moment it repaired anything, and
# every step after it was skipped in silence: the resize, the UUID
# randomisation, /etc/rebuild-settings, the host keys, and the per-revision
# device tree symlink. An A5 flashed that way boots the generic tree and runs
# its DDR3 at 1.36V, below the 1.425V minimum for the part it is fitted with.

@test "flash-cleanup: e2fsck is not called bare under set -e" {
  # A bare `e2fsck ...` as a whole statement is the defect. It has to be guarded
  # by something that inspects the status - here, check_fs.
  run grep -nE '^[[:space:]]*e2fsck [^|&]*$' "$SCRIPT"
  [ "$status" -ne 0 ] || { echo "unguarded e2fsck call(s):"; echo "$output"; false; }
}

@test "flash-cleanup: a repair (status 1 or 2) is treated as success" {
  # The whole point: 1 and 2 mean corrected, so the script must carry on to the
  # device tree symlink rather than dying before it.
  grep -qE '1\|2\)' "$SCRIPT"
}

@test "flash-cleanup: uncorrected errors still fail" {
  # 4 and above must not be swallowed - a "|| true" here would trade a silent
  # abort for a silent corruption, which is worse.
  run grep -cE 'e2fsck.*\|\|[[:space:]]*true' "$SCRIPT"
  [ "$output" -eq 0 ]
}

@test "flash-cleanup: both partitions are still checked" {
  grep -q 'check_fs "${OUTFILE}p1"' "$SCRIPT"
  grep -q 'check_fs "${OUTFILE}p2"' "$SCRIPT"
}

@test "flash-cleanup: a repair is reported, not passed over in silence" {
  # A flash that needed fsck means the write or the image was interrupted, and
  # that is worth knowing when the same board turns up odd later.
  grep -q 'repaired' "$SCRIPT"
}

@test "flash-cleanup: the device tree symlink is still reached after the check" {
  local fsck symlink
  fsck=$(line_of 'check_fs "${OUTFILE}p1"')
  symlink=$(grep -n 'ln -sf sun50i-a64-recore' "$SCRIPT" | head -1 | cut -d: -f1)
  [ -n "$fsck" ] && [ -n "$symlink" ]
  [ "$fsck" -lt "$symlink" ]
}
