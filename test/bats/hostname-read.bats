#!/usr/bin/env bats
load helper
setup() {
 setup_sandbox
 export REFLASH_HOSTNAME_MNT="$SANDBOX/root"
 stub_silent mount
 stub_silent umount
}
teardown() { teardown_sandbox; }
@test "legacy hostname reads in a read-only chroot and unmounts" {
 cat > "$SHIMDIR/chroot" <<'SH'
#!/bin/bash
case "$3" in /etc/rebuild-version) echo rebuild-fluidd-v1.1.0-91-g6056664 ;; /etc/hostname) echo legacy-a8 ;; esac
SH
 chmod +x "$SHIMDIR/chroot"
 run "$PROD_BIN/get-installed-hostname"
 [ "$status" -eq 0 ]
 [ "$output" = legacy-a8 ]
 assert_called_with 'mount -o ro,noload /dev/mmcblk2p2'
 assert_called_with 'umount'
}
@test "legacy hostname refuses unknown images and still unmounts" {
 stub chroot <<<'some-other-image'
 run "$PROD_BIN/get-installed-hostname"
 [ "$status" -eq 3 ]
 assert_called_with 'umount'
}
