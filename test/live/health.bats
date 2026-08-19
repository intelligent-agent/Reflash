#!/usr/bin/env bats
#
# Whole-image health: the things that are wrong in a way no single feature test
# would notice - a unit that failed at boot, a helper that was deleted upstream
# but is still installed, a secret in the log.

load helper

setup() { require_board; }

@test "no systemd units failed" {
  run board_ssh "systemctl --failed --no-legend --plain"
  [ "$status" -eq 0 ]
  [ -z "$output" ] || { echo "failed units:"; echo "$output"; false; }
}

@test "the flashing server and ssh are up" {
  [ "$(board_ssh 'systemctl is-active reflash')" = "active" ]
  [ "$(board_ssh 'systemctl is-active ssh')" = "active" ]
}

# The rootfs is the initrd unpacked into a tmpfs capped at half of RAM, and this
# image has historically booted full enough that any write hits ENOSPC - which
# takes out logging, ssh host keys and login, all at once.
@test "the root filesystem has room to write" {
  run board_ssh "df --output=pcent / | tail -1 | tr -dc '0-9'"
  [ "$output" -lt 95 ] || { echo "rootfs ${output}% full"; false; }
}

# runCommand2 logs the argv of anything that fails, and the log is streamed
# straight to the browser. wifi-connect and save-settings take a secret on the
# command line, so their args are redacted - this proves it on a live log.
@test "no WiFi passphrase in the log" {
  run board_ssh "grep -c 'Passphrase=' /var/log/reflash.log || true"
  [ "$output" -eq 0 ] || { echo "a passphrase reached the log"; false; }

  # Assert the placeholder is present rather than that a third field is absent.
  # The old pattern was "wifi-connect <ssid> <something>", and the placeholder
  # is itself a something - so it flagged correctly redacted lines, and only
  # passed on boards where no connection had ever failed.
  run board_ssh "grep 'wifi-connect ' /var/log/reflash.log | grep -vc '<redacted>' || true"
  [ "$output" -eq 0 ] || { board_ssh "grep 'wifi-connect ' /var/log/reflash.log"; false; }
}

# Deleted upstream but still shipped is invisible until someone runs one and
# believes the result. get-setting in particular could never work.
@test "no helper scripts that were deleted upstream" {
  for f in wifi-present is-ssh-enabled set-ssh-enabled get-setting \
           delete-recore-config first-boot save-setting wpa-psk; do
    run board_ssh "test -e /usr/local/bin/$f"
    [ "$status" -ne 0 ] || { echo "stale helper installed: $f"; false; }
  done
}

@test "the helpers the server actually calls are all installed" {
  for f in network-status wifi-connect wifi-bringup wifi-scan wifi-hotspot \
           reflash-ctl get-free-space get-hostnames get-reflash-version \
           get-recore-revision get-recore-serial-number get-emmc-version \
           save-settings mount-unmount-usb reboot-board shutdown-board; do
    run board_ssh "test -x /usr/local/bin/$f"
    [ "$status" -eq 0 ] || { echo "missing helper: $f"; false; }
  done
}

@test "network-status emits valid JSON even as an unprivileged user" {
  # iwctl's D-Bus calls are rejected for non-root, so the mode and SSID come
  # back empty here - but the output still has to parse, because a stray line
  # would take out the whole info panel rather than one field.
  run board_ssh "network-status"
  [ "$status" -eq 0 ]
  echo "$output" | assert_json
}

@test "the image reports a version" {
  run board_ssh "cat /etc/reflash-version"
  [ -n "$output" ]
  echo "# running image: $output" >&3
}
