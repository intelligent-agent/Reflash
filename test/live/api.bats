#!/usr/bin/env bats
#
# The HTTP API as the browser sees it on a running board.

load helper

setup() { require_board; }

# get_info mounts three partitions to answer, so what must be proven is that it
# is *small* - anything that grew back into it is a thing the UI would fetch on
# every state change.
@test "get_info carries only the boot-static facts" {
  run board_api /api/get_info
  [ "$status" -eq 0 ]
  echo "$output" | assert_json

  for key in reflash_version recore_revision serial_number emmc_version; do
    echo "$output" | json_get "$key" >/dev/null || { echo "missing $key"; false; }
  done
  # These moved to get_status or were deleted outright; if any come back,
  # get_info has silently become pollable again.
  for key in local_images bytes_available network is_ssh_enabled ips; do
    if echo "$output" | json_get "$key" >/dev/null 2>&1; then
      echo "get_info should not carry $key: $output"
      false
    fi
  done
}

@test "get_status carries the parts that change" {
  run board_api /api/get_status
  [ "$status" -eq 0 ]
  echo "$output" | assert_json
  echo "$output" | json_get local_images >/dev/null
  [ "$(echo "$output" | json_get bytes_available)" -gt 0 ]
  echo "$output" | json_get network.ethernet.up >/dev/null
  echo "$output" | json_get network.wifi.present >/dev/null
}

# Both transports report independently, and exactly one can hold the default
# route - that is the whole point of the active flag (#112).
@test "at most one transport is marked active" {
  run board_api /api/get_status
  local eth wifi
  eth=$(echo "$output" | json_get network.ethernet.active)
  wifi=$(echo "$output" | json_get network.wifi.active)
  [ "$eth" != "true" ] || [ "$wifi" != "true" ]
}

# The server holds the passphrase in memory and writes it to disk; it must never
# hand it back to a browser that may be sitting on an open hotspot.
@test "get_wifi never returns the passphrase" {
  run board_api /api/get_wifi
  [ "$status" -eq 0 ]
  echo "$output" | assert_json
  [ "$(echo "$output" | json_get password)" = "" ]
}

# Folded into get_status. A 200 here means an old binary is running, and with it
# the 2s poll that reported the dongle as missing mid-scan.
@test "the old get_wifi_status endpoint is gone" {
  run board_ssh "wget -S -qO- http://localhost/api/get_wifi_status 2>&1 | grep -c '404'"
  [ "$output" -ge 1 ]
}
