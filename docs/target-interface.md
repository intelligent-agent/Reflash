# Target interface v1

How Reflash prepares an image it has just written to the eMMC, without knowing
anything about that image (#179).

Reflash 1.2.0 is meant to be the last Reflash a user flashes. So everything
specific to the operating system being installed - its boot configuration, its
device tree selection, its SSH keys, its screen rotation, its settings file -
lives in the image, in an *installer* the image ships. Reflash only writes the
bytes, runs one generic device step the image asks for, and calls the installer
through the interface below. An image made years after this Reflash can still
be installed by it, as long as it speaks interface 1.

Images without a manifest (Rebuild v1.0.x and v1.1.x) are prepared the old
way, by `flash-cleanup`, `rotate-screen` and `save-settings`. That path is
frozen: it is kept for those images and nothing else.

Everything below is the contract. Changing any of it means interface 2, and
Reflash refusing images that ask for an interface it does not know.

## 1. The manifest

A text file on **partition 1**, at `reflash/manifest`, or at
`boot/reflash/manifest` for an image whose partition 1 is its root
filesystem. Reflash reads the first one it finds.

```
# Reflash target manifest
interface=1
root=2
boot=1
prepare=ext4-grow-root
installer=/usr/lib/reflash/target-installer
log=/var/log.hdd/reflash.log
```

One `key=value` per line. `#` starts a comment line, and blank lines are
ignored. The file is parsed, never sourced or executed.

| key | required | value |
| --- | --- | --- |
| `interface` | yes | `1`. Reflash refuses any other value, before it changes anything. |
| `root` | yes | number of the partition holding the root filesystem, 1-9. |
| `boot` | no | number of a partition to mount at `/boot` inside the root. |
| `prepare` | yes | the device step Reflash runs before the installer (section 2). |
| `installer` | yes | absolute path of the installer inside the root filesystem. Letters, digits, `.`, `_`, `-` and `/` only; no `..`. |
| `log` | no | absolute path inside the root where Reflash copies its own log after preparing. Same characters as `installer`. |

Unknown keys are logged and ignored, so a later image can add optional keys
without breaking this Reflash. A key whose absence would make this Reflash do
the wrong thing is not optional, and belongs in a new interface version.

## 2. Device preparation

Some steps cannot be done by an installer running inside the image, because
the image's root filesystem has to be mounted for the installer to run at all,
and `tune2fs -U` refuses a mounted filesystem. So Reflash does them, as one
named procedure the manifest asks for:

- **`ext4-grow-root`**
  1. Re-read the partition table and wait for the `root` and `boot` nodes.
  2. `e2fsck -f -y` the `boot` and `root` partitions, which must both be ext4.
     Exit status 1 and 2 are repairs and count as success; 4 and above fail
     the install.
  3. Grow partition `root` to the end of the device, and `resize2fs` it. It
     has to be the last partition.
  4. Give `boot` and `root` new random UUIDs, so the eMMC no longer matches the
     image it was written from (or the USB drive, if that was written from the
     same image).
- **`none`**: nothing. The installer gets an untouched device.

An unknown `prepare` value is refused, like an unknown interface.

## 3. Running the installer

Reflash mounts partition `root` read-write at a directory of its choosing,
partition `boot` (if given) at `/boot` under it, and bind-mounts `/dev`,
`/proc` and `/sys`. It then runs the installer with `chroot`, so the installer
runs on the image's own libc and tools, never on Reflash's. Everything is
unmounted afterwards, whether or not the installer succeeded.

```
chroot <root> <installer> prepare
chroot <root> <installer> configure  < settings
```

The environment is cleared and then set to:

| variable | value |
| --- | --- |
| `PATH` | `/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin` |
| `REFLASH_INTERFACE` | `1` |
| `REFLASH_VERSION` | this Reflash's version, for the installer's log only |
| `REFLASH_DEVICE` | the whole device, e.g. `/dev/mmcblk2` |
| `REFLASH_ROOT_DEV` | the root partition, e.g. `/dev/mmcblk2p2` |
| `REFLASH_BOOT_DEV` | the boot partition, or empty when there is none |
| `REFLASH_REVISION` | the board's hardware revision in lower case: `a5`, `a6`, `a7`, `a8`, ... |
| `REFLASH_SERIAL` | the board's serial number, or empty if it could not be read |

`REFLASH_REVISION` and `REFLASH_SERIAL` come from the board's own
configuration (eMMC boot partition 0), which only Reflash can read before the
installed system has booted.

### `prepare`

Run once, straight after the image is written and the device step is done.
Everything the board needs **before its first boot** happens here, above all
choosing the device tree for the revision (section 5).

### `configure`

Run when the user finishes the installation, with their choices on stdin. It
can run more than once, and must give the same result each time.

```
SETTINGS=1
SSH_ENABLED=true
SCREEN_ROTATION=270
WIFI_SSID=Home network
WIFI_PSK=it's-my-wifi
```

One `KEY=VALUE` per line. The value is everything after the first `=` up to
the end of the line, taken literally: no quoting and no escapes. Reflash
refuses a value containing a newline rather than sending it. The installer
ignores keys it does not know, and must never write `WIFI_PSK` to a log.

| key | value |
| --- | --- |
| `SETTINGS` | `1`, the version of this list |
| `SSH_ENABLED` | `true` or `false` |
| `SCREEN_ROTATION` | `0`, `90`, `180` or `270` (degrees clockwise) |
| `WIFI_SSID` | network name, may be empty |
| `WIFI_PSK` | passphrase, may be empty |

### Output and exit status

Exit status 0 is success; anything else fails the installation. Everything the
installer prints goes to Reflash's log. A line starting with `ERROR: ` is a
reason meant for the user, and the last one is shown when the installer fails.

### What the installer may not do

Use the network, install packages, start services, or take more than a few
minutes. It runs on a board that may be in hotspot mode with no internet, and
the user is waiting.

## 4. Repeating a step

`prepare` can run again on a board it has already prepared (for example after a
failed attempt), and `configure` runs again whenever the user changes their
choices. Neither may assume a clean image.

## 5. What the image must leave on partition 1

Reflash's own boot script reads partition 1 of the eMMC before it starts, to
decide whether to raise the DRAM supply: A5 and A6 boards run their DDR3 at
1.5 V, and the universal Reflash device tree would leave them at 1.36 V. It
does that by reading `fdtfile=` from `/armbianEnv.txt` on partition 1 and the
`model` of that device tree.

So after `prepare`, partition 1 must hold `/armbianEnv.txt` with an `fdtfile=`
line naming the device tree for `REFLASH_REVISION`, at `/dtb/<fdtfile>`. This
is the one place the interface reaches into the image's layout. Moving the
decision onto the board configuration itself would remove it, and would be a
change to Reflash's boot script, not to this interface.
