#!/usr/bin/env bats

load helper

# expand-usb partitions and formats /dev/sda, hardcoded and with no env seams,
# so - as with flash-cleanup.bats - these are static checks on the script text
# rather than runs of it. Creating a real block device to run against needs
# root and losetup, which the suite does not have. This at least pins the
# defects that shipped.

setup() { SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../../bin/prod" && pwd)/expand-usb"; }

line_of() { grep -n "$1" "$SCRIPT" | head -1 | cut -d: -f1; }

# The bug this suite exists for. A first boot created sda2, failed to format it,
# and every boot after that exited at the guard because the *partition* was
# there - so the drive stayed unformatted for good. The board then has no
# storage, no options.cfg, no ssh host keys, and sits on its own hotspot.
@test "expand-usb: the early exit requires a filesystem, not just a partition" {
  local guard
  guard=$(grep -n 'exists and holds a filesystem' "$SCRIPT" | head -1 | cut -d: -f1)
  [ -n "$guard" ]
  # The condition on that exit has to probe the filesystem, not only -b.
  run sed -n "$((guard - 1))p" "$SCRIPT"
  [[ "$output" == *"FSTYPE"* ]]
}

# The other half: having found no filesystem, it has to go on and make one
# rather than falling out of the script.
@test "expand-usb: a partition with no filesystem is formatted, not skipped" {
  local unformatted mkfs
  unformatted=$(line_of 'has no filesystem')
  mkfs=$(line_of 'mkfs.ext4 ')
  [ -n "$unformatted" ]
  [ -n "$mkfs" ]
  [ "$unformatted" -lt "$mkfs" ]
  # And partitioning has to be confined to the branch taken when the partition
  # was absent, so the "exists but unformatted" path cannot re-create a
  # partition that is already there. Checked by position rather than by reading
  # the branch, because the else-branch text sits between the two markers above
  # while never running on this path.
  local else_line endif fdisk
  else_line=$(grep -n '^else$' "$SCRIPT" | head -1 | cut -d: -f1)
  endif=$(line_of 'end of "the partition had to be created"')
  fdisk=$(line_of 'fdisk /dev/sda')
  [ -n "$else_line" ] && [ -n "$endif" ] && [ -n "$fdisk" ]
  [ "$else_line" -lt "$fdisk" ]
  [ "$fdisk" -lt "$endif" ]
  [ "$endif" -lt "$mkfs" ]
}

# mke2fs opens the device O_EXCL and udev probes a partition the moment it
# appears; losing that race makes mkfs fail instantly, which is how the stick
# got into the state above.
@test "expand-usb: waits for udev to release the new partition before mkfs" {
  local settle mkfs
  settle=$(line_of 'udevadm settle')
  mkfs=$(line_of 'mkfs.ext4 ')
  [ -n "$settle" ]
  [ -n "$mkfs" ]
  [ "$settle" -lt "$mkfs" ]
}

# The failure that started all this left no explanation anywhere: mkfs wrote to
# stderr, stderr went to the journal, and the journal is a ramdisk that the next
# reboot threw away.
@test "expand-usb: mkfs output is kept in the reflash log" {
  run grep -c 'mkfs.ext4 .*>> */var/log/reflash.log' "$SCRIPT"
  [ "$output" -ge 1 ]
}

@test "expand-usb: a failed mkfs says so and stops" {
  local fail
  fail=$(line_of 'Could not create the filesystem')
  [ -n "$fail" ]
  run sed -n "$((fail + 1))p" "$SCRIPT"
  [[ "$output" == *"exit 1"* ]]
}

@test "expand-usb: is valid bash" {
  run bash -n "$SCRIPT"
  [ "$status" -eq 0 ]
}
