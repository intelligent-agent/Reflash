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
}

teardown() { teardown_sandbox; }

with_wifi() { mkdir -p "$SYS_NET/$WIFI_INTERFACE"; }

# operstate is what the script reads to decide the cable is in.
with_eth() {
  mkdir -p "$SYS_NET/$ETH_INTERFACE"
  echo "${1:-up}" > "$SYS_NET/$ETH_INTERFACE/operstate"
}

# Stub `ip` so each interface reports its own address.
stub_ip() {
  cat > "$SHIMDIR/ip" <<EOF
#!/usr/bin/env bash
echo "ip \$*" >> "$CALLS"
case "\$4" in
  eth0)  echo "    inet ${1:-}/24 brd 10.0.0.255 scope global eth0" ;;
  wlan0) echo "    inet ${2:-}/24 brd 10.0.0.255 scope global wlan0" ;;
esac
exit 0
EOF
  chmod +x "$SHIMDIR/ip"
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
  [[ "$output" == *'"ethernet":{"up":false,"ip":""}'* ]]
  [[ "$output" == *'"present":false'* ]]
}

@test "ethernet up reports its IP" {
  with_eth up
  stub_ip "192.168.1.42" ""
  run "$PROD_BIN/network-status"
  [[ "$output" == *'"ethernet":{"up":true,"ip":"192.168.1.42"}'* ]]
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
  [[ "$output" == *'"ethernet":{"up":true,"ip":"192.168.1.42"}'* ]]
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
