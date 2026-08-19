#!/usr/bin/env bats
#
# The consoles a user can actually reach, and the USB gadget behind them.
#
# This is the part that cannot be proven offline: the gadget only exists once
# the UDC binds, so a wrong function order or a missing udev rule shows up here
# and nowhere else.

load helper

setup() { require_board; }

@test "the gadget exposes two ACM functions" {
  run board_ssh "sudo ls /sys/kernel/config/usb_gadget/g1/configs/c.1/"
  [ "$status" -eq 0 ]
  [[ "$output" == *"acm.usb0"* ]]
  [[ "$output" == *"acm.usb1"* ]]
}

@test "both gadget ttys exist" {
  run board_ssh "ls /dev/ttyGS0 /dev/ttyGS1"
  [ "$status" -eq 0 ]
}

# The login goes on the first port because that is the one a person plugging in
# a cable reaches for. If these two ever swap, flasher-pi talks to a login
# prompt and the user gets a raw protocol.
@test "the login getty is on ttyGS0" {
  run board_ssh "systemctl is-active serial-getty@ttyGS0.service"
  [ "$output" = "active" ]
}

@test "the control protocol took ttyGS1, not ttyGS0" {
  run board_ssh "grep -c 'USB control channel open on /dev/ttyGS1' /var/log/reflash.log"
  [ "$output" -ge 1 ]
  run board_ssh "grep -c 'USB control channel open on /dev/ttyGS0' /var/log/reflash.log"
  [ "$output" -eq 0 ]
}

# Reflash draws its status UI straight to /dev/fb0, so a getty on tty1 would
# fight it for the framebuffer. Its absence is why the USB login has to exist.
@test "no getty on tty1, because Reflash owns the framebuffer" {
  run board_ssh "systemctl is-active getty@tty1.service"
  [ "$output" != "active" ]
}

@test "the control socket is served and reachable by a non-root login" {
  run board_ssh "test -S /run/reflash/control.sock && stat -c %a /run/reflash/control.sock"
  [ "$status" -eq 0 ]
  [ "$output" = "666" ]
}

# Driving the protocol from the getty is the whole reason the socket exists.
@test "reflash-ctl speaks the protocol over the socket" {
  run board_ssh "reflash-ctl STATUS"
  [ "$status" -eq 0 ]
  [[ "$output" =~ ^STATE\ [A-Z]+\ PROGRESS\ [0-9]+ ]]

  run board_ssh "reflash-ctl LIST"
  [ "$status" -eq 0 ]
  # LIST terminates with OK; without it a client cannot tell a truncated
  # response from an empty image list.
  [[ "$output" == *"OK"* ]]
}
