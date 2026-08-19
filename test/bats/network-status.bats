#!/usr/bin/env bats
#
# network-status reports how the board is reachable, as JSON, for the info
# panel (#117). Both transports are reported independently - they can be up at
# once, and picking a winner would be guessing (#112).

load helper

setup() {
  setup_sandbox
  export WIFI_INTERFACE="wlan0"
  export ETH_INTERFACE="eth0"
  # Default to "no interfaces at all": SYS_NET is an empty dir.
  export SYS_NET="$SANDBOX/sys-net"
  mkdir -p "$SYS_NET"
  export PROC_WIRELESS="$SANDBOX/proc-wireless"
  : > "$PROC_WIRELESS"
  stub_silent ip
}

teardown() { teardown_sandbox; }

with_wifi() { mkdir -p "$SYS_NET/$WIFI_INTERFACE"; }

# operstate is what the script reads to decide the cable is in.
with_eth() {
  mkdir -p "$SYS_NET/$ETH_INTERFACE"
  echo "${1:-up}" > "$SYS_NET/$ETH_INTERFACE/operstate"
}

# Stub `ip` so each interface reports its own address, and `ip route show
# default` replays whatever set_routes wrote.
stub_ip() {
  : > "$SANDBOX/routes"
  cat > "$SHIMDIR/ip" <<EOF
#!/usr/bin/env bash
echo "ip \$*" >> "$CALLS"
if [ "\$1 \$2" = "route show" ]; then cat "$SANDBOX/routes"; exit 0; fi
case "\$4" in
  eth0)  echo "    inet ${1:-}/24 brd 10.0.0.255 scope global eth0" ;;
  wlan0) echo "    inet ${2:-}/24 brd 10.0.0.255 scope global wlan0" ;;
esac
exit 0
EOF
  chmod +x "$SHIMDIR/ip"
}

# Real `ip route show default` output, metrics and all.
set_routes() { printf '%s\n' "$@" > "$SANDBOX/routes"; }

set_rssi() {
  {
    echo "Inter-| sta-|   Quality        |   Discarded packets"
    echo " face | tus | link level noise |  nwid  crypt   frag"
    echo " $WIFI_INTERFACE: 0000   55.  $1.  -256        0      0"
  } > "$PROC_WIRELESS"
}

# Stub `iwctl` with a given mode and connected-network name. The SSID goes
# through a file rather than being interpolated into the stub body: it is
# attacker-shaped test data (quotes, backslashes) and would otherwise break out
# of the generated script's own quoting.
stub_iwctl() {
  printf '%s' "${1:-station}" > "$SANDBOX/mode"
  printf '%s' "${2:-}" > "$SANDBOX/ssid"
  cat > "$SHIMDIR/iwctl" <<EOF
#!/usr/bin/env bash
echo "iwctl \$*" >> "$CALLS"
if [ "\$1 \$3" = "device show" ]; then echo "  Mode    \$(cat "$SANDBOX/mode")"; fi
if [ "\$1 \$3" = "station show" ]; then echo "  Connected network    \$(cat "$SANDBOX/ssid")"; fi
exit 0
EOF
  chmod +x "$SHIMDIR/iwctl"
}

@test "no interfaces at all: valid JSON, everything down" {
  run "$PROD_BIN/network-status"
  [ "$status" -eq 0 ]
  [[ "$output" == *'"ethernet":{"up":false,"ip":"","active":false}'* ]]
  [[ "$output" == *'"present":false'* ]]
}

@test "ethernet up reports its IP" {
  with_eth up
  stub_ip "192.168.1.42" ""
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"ethernet":{"up":true,"ip":"192.168.1.42","active":false}'* ]]
}

@test "ethernet present but cable out is not up" {
  with_eth down
  stub_ip "" ""
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"up":false'* ]]
}

@test "wifi in station mode reports the connected SSID" {
  with_wifi
  stub_ip "" "192.168.1.87"
  stub_iwctl station "HomeNet"
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"mode":"station"'* ]]
  [[ "$output" == *'"ssid":"HomeNet"'* ]]
  [[ "$output" == *'"ip":"192.168.1.87"'* ]]
}

@test "wifi in ap mode reports the hotspot profile" {
  with_wifi
  stub_ip "" "192.168.8.1"
  stub_iwctl ap ""
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"mode":"ap"'* ]]
  [[ "$output" == *'"ssid":"Recore"'* ]]
}

@test "both transports up are both reported (#112)" {
  with_eth up
  with_wifi
  stub_ip "192.168.1.42" "192.168.1.87"
  stub_iwctl station "HomeNet"
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"ethernet":{"up":true,"ip":"192.168.1.42","active":false}'* ]]
  [[ "$output" == *'"ssid":"HomeNet"'* ]]
  [[ "$output" == *'"ip":"192.168.1.87"'* ]]
}

@test "an SSID with a quote does not produce invalid JSON" {
  with_wifi
  stub_ip "" "192.168.1.87"
  stub_iwctl station 'He said "hi"'
  run "$PROD_BIN/network-status"
  # The whole info panel depends on this parsing.
  echo "$output" | python3 -c 'import json,sys; json.load(sys.stdin)'
  [[ "$output" == *'\"hi\"'* ]]
}

# --- which interface actually carries traffic (#112) ------------------------

@test "the default route with the lowest metric is the active one" {
  with_eth up
  with_wifi
  stub_ip "192.168.32.198" "192.168.32.161"
  stub_iwctl station "HomeNet"
  # Measured on a real board: WiFi wins on metric with the cable plugged in.
  set_routes "default via 192.168.32.1 dev wlan0 proto dhcp src 192.168.32.161 metric 304" \
             "default via 192.168.32.1 dev eth0 proto dhcp src 192.168.32.198 metric 1024"
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"ip":"192.168.32.198","active":false'* ]]
  [[ "$output" == *'"active":true}}'* ]]
}

@test "a route with no metric beats one with a metric" {
  with_eth up
  with_wifi
  stub_ip "10.0.0.2" "10.0.0.3"
  stub_iwctl station "HomeNet"
  set_routes "default via 10.0.0.1 dev eth0" \
             "default via 10.0.0.1 dev wlan0 metric 600"
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"active":true}'* ]]
  [[ "$output" == *'"active":false}}'* ]]
}

@test "no default route means nothing is active" {
  with_eth up
  stub_ip "10.0.0.2" ""
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"active":false'* ]]
}

# --- signal strength --------------------------------------------------------

@test "RSSI is read from /proc/net/wireless" {
  with_wifi
  stub_ip "" "10.0.0.3"
  stub_iwctl station "HomeNet"
  set_rssi -55
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"rssi":-55'* ]]
}

# iw is not installed on the image and iwctl's D-Bus calls are rejected for
# non-root, so an unreadable/absent stats file has to degrade quietly.
@test "a missing wireless stats file reports rssi 0, not garbage" {
  with_wifi
  stub_ip "" "10.0.0.3"
  stub_iwctl station "HomeNet"
  rm -f "$PROC_WIRELESS"
  run "$PROD_BIN/network-status"
  [ "$status" -eq 0 ]
  [[ "$output" == *'"rssi":0'* ]]
  echo "$output" | python3 -c 'import json,sys; json.load(sys.stdin)'
}
