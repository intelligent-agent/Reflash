# Reflash

This is a simple Go server that is set up to
get and flash Refactor and Rebuild images

All users should download the latest stable version of
Reflash and use Balena Etcher to flash it to a USB drive. 

More information on the wiki: https://wiki.iagent.no/wiki/Reflash

## Consoles and the control protocol

Reflash draws its status UI straight to `/dev/fb0`, so `reflash.service` carries
`Conflicts=getty@tty1.service` and the attached screen has no login prompt. That
leaves USB as the way in, and the gadget exposes two ACM functions for it:

| function | board | host | serves |
| --- | --- | --- | --- |
| `acm.usb0` | `/dev/ttyGS0` | `/dev/ttyACM0` | login getty (`debian` / `temppwd`) |
| `acm.usb1` | `/dev/ttyGS1` | `/dev/ttyACM1` | control protocol |

Ports are handed out in the order the functions are linked into the config, so
the order in `usb-gadget-init.sh` is what fixes this mapping. The login gets the
first port because that is the one a person plugging in a cable reaches for.

The login matters because the alternatives are not always there: SSH needs a
network, which a board with failed WiFi setup does not have, and without a USB
getty the only remaining console is the UART header - which means opening the
case, exactly when a console is most needed.

### The control protocol

A line-based protocol for flashing without the web UI. **flasher-pi** is the
main consumer: a standalone machine that flashes an image onto a Recore, used in
testing to flash a board for the first time. It talks to `/dev/ttyACM1` and does
not need the board to be on the network - the image is already on the board's
USB drive, so this only lists, starts and polls.

```
LIST          -> "IMG <name> <bytes>" per local image, then OK
STATUS        -> "STATE <state> PROGRESS <pct>"
FLASH <file>  -> starts a file install; "OK flashing <file>" or "ERR ..."
CANCEL        -> cancels an in-progress flash
```

The same protocol is served on a unix socket at `/run/reflash/control.sock`, for
clients on the board itself. `reflash-ctl` is the wrapper:

```
reflash-ctl LIST
reflash-ctl FLASH rebuild-1.2.3.img.xz
reflash-ctl STATUS
reflash-ctl                 # interactive, one command per line
```

Use it from the getty rather than opening `/dev/ttyGS1`, which would take the
tty flasher-pi is talking on. Responses are CRLF-terminated on the serial line
and LF-terminated on the socket; everything else is identical, and all three
transports drive the same state machine as the HTTP API.

## Development
### Create linux image
This will use debootstrap to create a Reflash image
```
make docker
```

### Development
The server only runs on a board - it draws to the framebuffer, mounts the USB
drive and shells out to helpers in `/usr/local/bin`, so there is no local run
target. Work against the test suites, then build an image:

```
make test        # Go, bash helpers (bats) and the Vue client
make test-live   # read-only checks against a running board, over SSH
make build-vue   # rebuild the client bundle on its own
```

### Tests to run on a new version
- [x] A Rebuild file can be uploaded
- [x] A Rebuild file can be installed from github
- [x] A Rebuild file can be downloaded
- [x] A Rebuild file can be installed from local/USB
- [x] A fresh OS comes up as an Access Point
- [x] A Wifi access point can be connected to
- [x] Storing settings works
- [ ] Reflash boots on a board still running the previous **stable** Rebuild

### Booting from the previous stable version

Customers upgrade from the last stable Rebuild release (currently `v1.0.2`, from
2024) rather than from a pre-release, so that is the path worth testing - and it
is not the same as testing on a development board, which already has current
firmware.

It matters because **Reflash runs the U-Boot already on the board's eMMC**, not
anything shipped in the Reflash image. A board that has never been flashed with a
current Rebuild is therefore booting a current Reflash on two-year-old firmware.

To test: flash a board back to the last stable Rebuild, then boot Reflash from USB
and upgrade it to the current release.

Two known problems on that path. Both fix themselves after one successful flash,
because that is what updates U-Boot on the eMMC - but both are what a customer
sees on their *first* attempt:

- **USB boot is unreliable.** The fix for #88 - enabling the USB VBUS regulators
  properly and early, in `u-boot-sunxi64-legacy-3-regulators-enable-boot-on.patch`
  - lives in U-Boot on the eMMC, which the customer does not have yet. Measured on
  the old U-Boot: enumeration degraded on 54% of boots, and `uInitrd` failed to
  load on 21% of USB boots. Symptoms are `EHCI timed out on TD` and
  `Failed to load '/uInitrd'`. The workaround is simply to retry, so when testing
  this, boot Reflash **several times** - a single success proves nothing.
- **The Reflash screen stays dark.** The panel is driven by simpledrm from the
  framebuffer U-Boot hands over, and the old U-Boot has neither the splash patch
  nor the HDMI VBUS patch, so nothing is handed over and `/dev/fb0` does not
  exist. Reflash detects this and skips drawing; the web UI is unaffected.

