# Reflash

This is a simple Go server that is set up to
get and flash Refactor and Rebuild images

All users should download the latest stable version of
Reflash and use Balena Etcher to flash it to a USB drive. 

More information on the wiki: https://wiki.iagent.no/wiki/Reflash

## Development
### Create linux image
This will use debootstrap to create a Reflash image
```
make docker
```

### Local development
To start the npm client/Vue frontend
```
make run-go
make dev-client
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

