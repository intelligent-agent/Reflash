#!/usr/bin/env bats
#
# WiFi helper behaviour: graceful no-dongle handling (the fix we want to keep
# locked in) plus the AP/station bring-up and scan-parsing logic.

load helper

setup() {
  setup_sandbox
  export WIFI_INTERFACE="wlan0"
  # Default to "no adapter present": SYS_NET is an empty dir.
  export SYS_NET="$SANDBOX/sys-net"
  mkdir -p "$SYS_NET"
  export IWD_DIR="$SANDBOX/iwd"
  export WIFI_CONFIG_FILE="$SANDBOX/options.cfg"
  # No real delays in tests.
  stub_silent sleep
}

teardown() { teardown_sandbox; }

# Pretend a dongle is fitted: create /sys/class/net/wlan0.
with_adapter() { mkdir -p "$SYS_NET/$WIFI_INTERFACE"; }

# --- graceful degradation when no dongle is fitted -------------------------

@test "wifi-bringup: no adapter exits 0 (board is usable over Ethernet)" {
  run "$PROD_BIN/wifi-bringup"
  [ "$status" -eq 0 ]
  [[ "$output" == *"No WiFi adapter"* ]]
}

@test "wifi-scan: no adapter emits empty result markers and exits 0" {
  run "$PROD_BIN/wifi-scan"
  [ "$status" -eq 0 ]
  [[ "$output" == *"---SCAN_RESULTS_START---"* ]]
  [[ "$output" == *"---SCAN_RESULTS_END---"* ]]
}

@test "wifi-connect: no adapter exits 1 (connection impossible)" {
  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [ "$status" -eq 1 ]
  [[ "$output" == *"Cannot connect"* ]]
}

@test "wifi-connect: missing arguments exits 1 with usage" {
  run "$PROD_BIN/wifi-connect"
  [ "$status" -eq 1 ]
  [[ "$output" == *"Usage:"* ]]
}

# --- bring-up with an adapter present --------------------------------------

@test "wifi-bringup: adapter present, no SSID configured -> starts AP profile" {
  with_adapter
  stub_silent iwctl
  run "$PROD_BIN/wifi-bringup"
  [ "$status" -eq 0 ]
  assert_called_with "device wlan0 set-property Mode ap"
  assert_called_with "ap wlan0 start-profile Recore"
}

@test "wifi-bringup: adapter present, SSID set -> writes iwd profile, station mode" {
  with_adapter
  stub_silent iwctl
  printf "WifiSSID='HomeNet'\nWifiPSK='hunter2'\n" > "$WIFI_CONFIG_FILE"
  run "$PROD_BIN/wifi-bringup"
  [ "$status" -eq 0 ]
  [ -f "$IWD_DIR/HomeNet.psk" ]
  grep -q "Passphrase=hunter2" "$IWD_DIR/HomeNet.psk"
  assert_called_with "device wlan0 set-property Mode station"
}

# --- scan output parsing ----------------------------------------------------

@test "wifi-scan: parses iwctl get-networks into name|security|signal rows" {
  with_adapter
  cat > "$SHIMDIR/iwctl" <<'EOF'
#!/usr/bin/env bash
echo "iwctl $*" >> "$CALLS"
# 'device wlan0 show' -> report station mode so no AP toggling happens.
if [ "$1 $2 $3" = "device wlan0 show" ]; then echo "  Mode  station"; fi
if [ "$1 $2 $3" = "station wlan0 get-networks" ]; then
cat <<'NET'
                    Available networks
      Network name          Security    Signal
-----------------------------------------------
  >   HomeNet               psk         ****
      CoffeeShop            open        **
NET
fi
exit 0
EOF
  chmod +x "$SHIMDIR/iwctl"
  run "$PROD_BIN/wifi-scan"
  [ "$status" -eq 0 ]
  [[ "$output" == *"HomeNet|psk|****"* ]]
  [[ "$output" == *"CoffeeShop|open|**"* ]]
}

# --- full connect happy path ------------------------------------------------

@test "wifi-connect: provisions profile and reports success once DHCP leases" {
  with_adapter
  cat > "$SHIMDIR/iwctl" <<'EOF'
#!/usr/bin/env bash
echo "iwctl $*" >> "$CALLS"
if [ "$1 $2 $3" = "device wlan0 show" ]; then echo "Mode station"; fi
exit 0
EOF
  chmod +x "$SHIMDIR/iwctl"
  cat > "$SHIMDIR/ip" <<'EOF'
#!/usr/bin/env bash
echo "ip $*" >> "$CALLS"
echo "    inet 192.168.1.50/24 brd 192.168.1.255 scope global wlan0"
exit 0
EOF
  chmod +x "$SHIMDIR/ip"
  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [ "$status" -eq 0 ]
  [ -f "$IWD_DIR/HomeNet.psk" ]
  grep -q "Passphrase=hunter2" "$IWD_DIR/HomeNet.psk"
  assert_called_with "station wlan0 connect HomeNet"
  [[ "$output" == *"Connected with IP: 192.168.1.50/24"* ]]
}

# --- connect failure -> hotspot fallback (#90) ------------------------------

@test "wifi-connect: restores hotspot if the connect command keeps failing" {
  with_adapter
  cat > "$SHIMDIR/iwctl" <<'EOF'
#!/usr/bin/env bash
echo "iwctl $*" >> "$CALLS"
if [ "$1 $2 $3" = "device wlan0 show" ]; then echo "Mode station"; fi
if [ "$1 $2 $3" = "station wlan0 connect" ]; then exit 1; fi
exit 0
EOF
  chmod +x "$SHIMDIR/iwctl"
  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [ "$status" -eq 1 ]
  [[ "$output" == *"Connection command failed after 3 attempts."* ]]
  assert_called_with "device wlan0 set-property Mode ap"
  assert_called_with "ap wlan0 start-profile Recore"
}

@test "wifi-connect: restores hotspot if DHCP never leases" {
  with_adapter
  cat > "$SHIMDIR/iwctl" <<'EOF'
#!/usr/bin/env bash
echo "iwctl $*" >> "$CALLS"
if [ "$1 $2 $3" = "device wlan0 show" ]; then echo "Mode station"; fi
exit 0
EOF
  chmod +x "$SHIMDIR/iwctl"
  cat > "$SHIMDIR/ip" <<'EOF'
#!/usr/bin/env bash
echo "ip $*" >> "$CALLS"
# No "inet " line in the output - no lease ever arrives.
exit 0
EOF
  chmod +x "$SHIMDIR/ip"
  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [ "$status" -eq 1 ]
  [[ "$output" == *"Timed out waiting for IP address."* ]]
  assert_called_with "device wlan0 set-property Mode ap"
  assert_called_with "ap wlan0 start-profile Recore"
}

# --- adapter presence (#117 fallout: the dialog vanished mid-scan) -----------

@test "wifi-present: no adapter reports present=false and exits 0" {
  run "$PROD_BIN/wifi-present"
  [ "$status" -eq 0 ]
  [[ "$output" == *'"present": false'* ]]
}

@test "wifi-present: adapter fitted reports the details iwd gives" {
  with_adapter
  stub iwctl <<'OUT'
                    Devices
  Name    Address             Powered  Adapter  Mode
------------------------------------------------------
  wlan0   aa:bb:cc:dd:ee:ff   on       phy0     station
OUT
  run "$PROD_BIN/wifi-present"
  [ "$status" -eq 0 ]
  [[ "$output" == *'"present": true'* ]]
  [[ "$output" == *'"mac": "aa:bb:cc:dd:ee:ff"'* ]]
  [[ "$output" == *'"mode": "station"'* ]]
}

# The actual bug: a mode change makes iwd drop and re-add the device, so
# `iwctl device list` is briefly empty. The dongle has not gone anywhere, and
# the UI polls this every 2s while wifi-scan flips the mode twice.
@test "wifi-present: adapter is still present while iwd shows no device" {
  with_adapter
  stub_silent iwctl
  run "$PROD_BIN/wifi-present"
  [ "$status" -eq 0 ]
  [[ "$output" == *'"present": true'* ]]
  [[ "$output" == *'"name": "wlan0"'* ]]
}

@test "wifi-present: adapter is still present when iwctl fails outright" {
  with_adapter
  stub iwctl 1 <<'OUT'
OUT
  run "$PROD_BIN/wifi-present"
  [ "$status" -eq 0 ]
  [[ "$output" == *'"present": true'* ]]
}
