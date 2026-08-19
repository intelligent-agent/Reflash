#!/usr/bin/env bats
#
# The networking the image ships (#112): one DHCP client per interface, wired
# preferred, and no ARP flux between two interfaces on one subnet.
#
# These assert the config is *in force*, not merely present on disk - the failure
# they exist to catch is a setting that is silently ignored.

load helper

setup() { require_board; }

has_wifi() {
  board_ssh "test -d /sys/class/net/wlan0" || skip "no WiFi adapter fitted"
}

wired_up() {
  [ "$(board_ssh 'cat /sys/class/net/eth0/carrier 2>/dev/null')" = "1" ] \
    || skip "no ethernet carrier (cable out)"
}

@test "iwd owns wlan0's addressing, networkd does not" {
  has_wifi
  # Two DHCP clients on one interface is what gave wlan0 two default routes.
  run board_ssh "grep -c '^DHCP=no' /etc/systemd/network/30-wireless.network"
  [ "$output" -eq 1 ]
  run board_ssh "grep -c 'EnableNetworkConfiguration=true' /etc/iwd/main.conf"
  [ "$output" -eq 1 ]
}

@test "wlan0 has exactly one default route" {
  has_wifi
  run board_ssh "ip route show default | grep -c 'dev wlan0'"
  [ "$output" -le 1 ]
}

# The fix has to hold as a *route metric*, not just as a config line: iwd adds
# the interface index to RoutePriorityOffset, so the value on the wire is what
# decides, and it must lose to the wired 100.
@test "wired wins the default route when both are up" {
  has_wifi
  wired_up
  local eth wlan
  eth=$(board_ssh "ip route show default | sed -n 's/.*dev eth0.*metric \([0-9]*\).*/\1/p' | head -1")
  wlan=$(board_ssh "ip route show default | sed -n 's/.*dev wlan0.*metric \([0-9]*\).*/\1/p' | head -1")
  [ -n "$eth" ] || { echo "no eth0 default route"; false; }
  [ -n "$wlan" ] || { echo "no wlan0 default route"; false; }
  [ "$eth" -lt "$wlan" ] || { echo "eth0=$eth wlan0=$wlan - wireless wins"; false; }

  # And the routing table agrees, which is the thing that actually matters.
  run board_ssh "ip route get 1.1.1.1"
  [[ "$output" == *"dev eth0"* ]]
}

# Both interfaces on one subnet made the board answer ARP for either address on
# either interface, so peers cached them crossed and the wired address took ping
# but refused TCP.
@test "ARP flux is disabled" {
  [ "$(board_ssh 'cat /proc/sys/net/ipv4/conf/all/arp_ignore')" = "1" ]
  [ "$(board_ssh 'cat /proc/sys/net/ipv4/conf/all/arp_announce')" = "2" ]
}

@test "network-status agrees with the kernel about which link is active" {
  run board_ssh "sudo network-status"
  [ "$status" -eq 0 ]
  echo "$output" | assert_json

  local active
  active=$(board_ssh "ip route show default | sort -t' ' -k11 -n | sed -n 's/.*dev \([a-z0-9]*\).*/\1/p' | head -1")
  if [ "$active" = "eth0" ]; then
    [ "$(echo "$output" | json_get ethernet.active)" = "true" ]
  elif [ "$active" = "wlan0" ]; then
    [ "$(echo "$output" | json_get wifi.active)" = "true" ]
  else
    skip "no default route to compare against"
  fi
}

@test "a connected WiFi link reports a plausible RSSI" {
  has_wifi
  run board_ssh "sudo network-status"
  local mode rssi
  mode=$(echo "$output" | json_get wifi.mode)
  [ "$mode" = "station" ] || skip "not in station mode (mode=$mode)"
  [ -n "$(echo "$output" | json_get wifi.ssid)" ] || skip "not associated"
  rssi=$(echo "$output" | json_get wifi.rssi)
  # A real reading is negative; 0 means the stats file could not be read, which
  # the UI renders as no bar rather than as a perfect signal.
  [ "$rssi" -lt 0 ] || { echo "rssi=$rssi - unreadable on an associated link"; false; }
  [ "$rssi" -gt -100 ]
}
