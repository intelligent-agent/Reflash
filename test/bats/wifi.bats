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

# Three different problems used to produce one message. They need three
# different actions from the user, so the log has to tell them apart.

# Stub iwctl so that connect succeeds but no lease ever arrives, with the
# station left in $1 - which is the only thing that says why.
no_lease_in_state() {
  cat > "$SHIMDIR/iwctl" <<EOF
#!/usr/bin/env bash
echo "iwctl \$*" >> "\$CALLS"
if [ "\$1 \$2 \$3" = "device wlan0 show" ]; then echo "Mode station"; fi
if [ "\$1 \$2 \$3" = "station wlan0 show" ]; then echo "      State       $1"; fi
exit 0
EOF
  chmod +x "$SHIMDIR/iwctl"
  cat > "$SHIMDIR/ip" <<'EOF'
#!/usr/bin/env bash
echo "ip $*" >> "$CALLS"
exit 0
EOF
  chmod +x "$SHIMDIR/ip"
}

@test "wifi-connect: associated but no lease is reported as a DHCP problem" {
  with_adapter
  no_lease_in_state connected
  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [ "$status" -eq 1 ]
  [[ "$output" == *"DHCP problem, not a password one"* ]]
}

@test "wifi-connect: never associating points at the passphrase" {
  with_adapter
  no_lease_in_state disconnected
  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [ "$status" -eq 1 ]
  [[ "$output" == *"Never associated"* ]]
  [[ "$output" == *"Check the passphrase"* ]]
}

@test "wifi-connect: still associating is not blamed on DHCP" {
  with_adapter
  no_lease_in_state connecting
  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [ "$status" -eq 1 ]
  [[ "$output" == *"Still trying to associate"* ]]
}

# --- switching back to the hotspot on request (#105) -------------------------

@test "wifi-hotspot: no adapter exits 1" {
  run "$PROD_BIN/wifi-hotspot"
  [ "$status" -eq 1 ]
  [[ "$output" == *"Cannot start the hotspot"* ]]
}

@test "wifi-hotspot: disconnects, then switches to AP mode and starts the profile" {
  with_adapter
  stub_silent iwctl
  run "$PROD_BIN/wifi-hotspot"
  [ "$status" -eq 0 ]
  assert_called_with "station wlan0 disconnect"
  # Stop before start, or start-profile fails with "Object already exists".
  assert_called_with "ap wlan0 stop"
  assert_called_with "device wlan0 set-property Mode ap"
  assert_called_with "ap wlan0 start-profile Recore"
}

@test "wifi-hotspot: a refused mode change is reported, not ignored" {
  with_adapter
  cat > "$SHIMDIR/iwctl" <<'EOF2'
#!/usr/bin/env bash
echo "iwctl $*" >> "$CALLS"
[ "$1 $3 $4" = "device set-property Mode" ] && exit 1
exit 0
EOF2
  chmod +x "$SHIMDIR/iwctl"
  run "$PROD_BIN/wifi-hotspot"
  [ "$status" -eq 1 ]
  [[ "$output" == *"Could not switch"* ]]
}

# --- source routing: reply on the interface the request arrived on -----------
#
# With Ethernet up on the same subnet, replies to connections that arrived on
# wlan0 leave eth0 carrying the WiFi source address. That teaches the AP's
# bridge the address lives on the wire, and it stops forwarding frames for it
# over the air - so the client's ACK never lands, the socket sits in SYN-RECV
# retransmitting SYN-ACK, and the connection hangs rather than failing (#126).
# Measured on an A6: HTTP to the WiFi address went from a 20s timeout to 0.017s.

@test "wifi-connect: installs a source route for the address it just got" {
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
case "$*" in
  "addr show wlan0")            echo "    inet 192.168.1.50/24 brd 192.168.1.255 scope global wlan0" ;;
  *"route show dev wlan0 scope link") echo "192.168.1.0/24 proto kernel scope link src 192.168.1.50" ;;
  *"route show default dev wlan0")    echo "default via 192.168.1.1 dev wlan0" ;;
  *"route get"*)                echo "1.1.1.1 from 192.168.1.50 dev wlan0 table 100" ;;
esac
exit 0
EOF
  chmod +x "$SHIMDIR/ip"

  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [ "$status" -eq 0 ]

  # The rule is what actually redirects the traffic; without it the routes in
  # table 100 are never consulted.
  grep -q "ip rule add from 192.168.1.50 lookup 100" "$CALLS"
  grep -q "ip route add default via 192.168.1.1 dev wlan0 table 100" "$CALLS"
  [[ "$output" == *"Source routing: traffic from 192.168.1.50 now leaves wlan0"* ]]
}

# A silent failure here is the worst outcome: the board looks connected and its
# WiFi address is quietly unusable, which is exactly how #126 presented.
@test "wifi-connect: says so when the source route did not take" {
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
case "$*" in
  "addr show wlan0") echo "    inet 192.168.1.50/24 brd 192.168.1.255 scope global wlan0" ;;
  *"route get"*)     echo "1.1.1.1 from 192.168.1.50 dev eth0" ;;   # still leaving the wrong way
esac
exit 0
EOF
  chmod +x "$SHIMDIR/ip"

  run "$PROD_BIN/wifi-connect" HomeNet hunter2
  [[ "$output" == *"WARNING: could not install source routing"* ]]
}

@test "wifi-hotspot: removes the source route when leaving station mode" {
  with_adapter
  cat > "$SHIMDIR/ip" <<'EOF'
#!/usr/bin/env bash
echo "ip $*" >> "$CALLS"
if [ "$1 $2" = "rule list" ]; then echo "32765:	from 192.168.1.50 lookup 100"; fi
exit 0
EOF
  chmod +x "$SHIMDIR/ip"
  stub_silent iwctl

  run "$PROD_BIN/wifi-hotspot"

  # The address is gone once the interface becomes an AP; a rule naming it is a
  # trap for whoever debugs routing here next.
  grep -q "ip rule del from 192.168.1.50 lookup 100" "$CALLS"
  grep -q "ip route flush table 100" "$CALLS"
}
