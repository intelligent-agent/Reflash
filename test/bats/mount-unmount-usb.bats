#!/usr/bin/env bats

load helper

# Every mount of the USB drive goes through mount-unmount-usb. It used to
# unmount and mount again for every mode change, leaving /mnt/usb empty for a
# moment; a config restore reading the drive in that moment failed with "not
# on the USB drive" (#191).

setup() {
  setup_sandbox
  export USB_LOCK_HELD=1 USB_MOUNT_POINT="$SANDBOX/usb" USB_DEVICE=/dev/sda2
  mkdir -p "$USB_MOUNT_POINT"
  stub umount </dev/null
}
teardown() { teardown_sandbox; }

@test "mount-unmount-usb: an already-mounted drive changes mode in place" {
  stub findmnt <<<"/dev/sda2"
  stub mount </dev/null
  run "$PROD_BIN/mount-unmount-usb" mounted rw
  [ "$status" -eq 0 ]
  assert_called_with "mount -o remount,rw $USB_MOUNT_POINT"
  ! grep -q '^umount' "$CALLS"
}

@test "mount-unmount-usb: a drive that is not mounted is mounted" {
  stub findmnt 1 </dev/null
  stub mount </dev/null
  run "$PROD_BIN/mount-unmount-usb" mounted ro
  [ "$status" -eq 0 ]
  assert_called_with "mount /dev/sda2 -r $USB_MOUNT_POINT"
  ! grep -q 'remount' "$CALLS"
}

# Something else on the mount point is not changed in place: it is replaced.
@test "mount-unmount-usb: another device on the mount point is replaced" {
  stub findmnt <<<"/dev/sdb1"
  stub mount </dev/null
  run "$PROD_BIN/mount-unmount-usb" mounted ro
  [ "$status" -eq 0 ]
  assert_called_with "umount -q $USB_MOUNT_POINT"
  assert_called_with "mount /dev/sda2 -r $USB_MOUNT_POINT"
}

# A remount the kernel refuses - a writer still open on a rw->ro change - falls
# back to the old unmount and mount.
@test "mount-unmount-usb: a refused remount falls back to unmount and mount" {
  stub findmnt <<<"/dev/sda2"
  cat > "$SHIMDIR/mount" <<SH
#!/usr/bin/env bash
echo "mount \$*" >> "$CALLS"
case "\$*" in *remount*) exit 32;; esac
exit 0
SH
  chmod +x "$SHIMDIR/mount"
  run "$PROD_BIN/mount-unmount-usb" mounted ro
  [ "$status" -eq 0 ]
  assert_called_with "mount -o remount,ro $USB_MOUNT_POINT"
  assert_called_with "umount -q $USB_MOUNT_POINT"
  assert_called_with "mount /dev/sda2 -r $USB_MOUNT_POINT"
}

@test "mount-unmount-usb: unmounting still unmounts" {
  run "$PROD_BIN/mount-unmount-usb" unmounted
  [ "$status" -eq 0 ]
  assert_called_with "umount -q $USB_MOUNT_POINT"
}
