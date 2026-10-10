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
log=/var/log/reflash.log
settings=SSH_ENABLED,SCREEN_ROTATION,WIFI_SSID,WIFI_PSK,LOGIN_PASSWORD
actions=settings,backup,restore
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
| `settings` | no | the settings keys `configure` applies (section 3), comma separated. Reflash offers the user only these. Without it: `SSH_ENABLED,SCREEN_ROTATION,WIFI_SSID,WIFI_PSK`. |
| `actions` | no | the optional actions the installer supports (section 3), comma separated: any of `settings`, `backup`, `restore`, `list`, `list-archive`. Without it: none. |

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
ignores keys it does not know, and must never write `WIFI_PSK` or
`LOGIN_PASSWORD` to a log.

| key | value |
| --- | --- |
| `SETTINGS` | `1`, the version of this list |
| `SSH_ENABLED` | `true` or `false` |
| `SCREEN_ROTATION` | `0`, `90`, `180` or `270` (degrees clockwise) |
| `WIFI_SSID` | network name, may be empty |
| `WIFI_PSK` | passphrase, may be empty |
| `LOGIN_PASSWORD` | a new password for the system's login account, or empty to leave the account as it is |
| `ROOT_PASSWORD` | a new password for root, or empty to leave it as it is |
| `WIFI_COUNTRY` | two capital letters, the Wi-Fi regulatory domain; empty puts it back to none |
| `TIMEZONE` | a name like `Europe/Oslo`; empty puts it back to UTC |
| `WIFI_MODE` | `auto` (join the network, hotspot when it is not found), `client` (never the hotspot) or `ap` (always the hotspot); empty is `auto` |
| `HOTSPOT_SSID` | the hotspot's name; empty is the image's own |
| `HOTSPOT_PSK` | the hotspot's password, 8 to 63 characters; empty is the image's own |
| `SOFTWARE_<name>` | `on` or `off`: optional software the image offers (below). Listed in `settings=` as `SOFTWARE`. |

Settings are sent only when the manifest lists them (`settings=`), because an
installer ignores keys it does not know: a choice the image would silently
drop must not be offered at all.

`LOGIN_PASSWORD` is the image's to apply: which account it is, and whether the
password is good enough by the image's own rules. A password it refuses fails
`configure` with an `ERROR: ` line saying why. Once set, the system does not ask
for a new password at its first login. Like `WIFI_PSK`, it is never logged and
never printed back by the `settings` action.

An empty value for `WIFI_COUNTRY`, `TIMEZONE`, `WIFI_MODE`, `HOTSPOT_SSID` and
`HOTSPOT_PSK` means "the image's own": Reflash's Default box (#198). It is sent
when a setting goes back to Default after a change, and a key that was never
changed is not sent at all, so an image left alone behaves exactly as it did.
`ROOT_PASSWORD` cannot be read back, so Reflash puts the factory password
back by sending it. An installer that applies `ROOT_PASSWORD`, `WIFI_COUNTRY`
or `TIMEZONE` may also skip a first-login setup of its own that asks for the
same things, but must not take away what a person debugging the board over
its serial console relies on.

`configure` applies the keys it is given and leaves every other setting as it
is, so changing one choice later (for example from a Reflash booted to repair a
forgotten password) does not reset the rest.

### Optional actions

An installer may support more actions, listed in the manifest's `actions=`.
Reflash calls only those, and an installer given an action it does not
support exits 3 without changing anything - so a wrong manifest costs a
message, not a board.

For these actions stdout carries data, and only stderr goes to Reflash's log.

- **`settings`**: print the system's current settings in the `configure`
  format, read from where the system keeps them, so Reflash can show them
  before the user changes anything. Secrets (`WIFI_PSK`, `LOGIN_PASSWORD`)
  are not printed. Changes nothing.

  With the word `secrets` as its argument (`settings secrets`), the installer
  may also print `WIFI_PSK`, after `WIFI_SSID`. Reflash asks for that when it
  starts (#185), so that the installed system's Wi-Fi network - name and
  passphrase together, a name alone is no network - becomes Reflash's own and
  goes onto the next image. The passphrase stays in Reflash's options, where a
  passphrase typed in Reflash lives too; the installer must not print it to
  stderr, which Reflash logs. An installer that does not know the word ignores
  it and prints no passphrase, and Reflash then leaves its own network alone.
  `LOGIN_PASSWORD` and `ROOT_PASSWORD` cannot be read back and are never
  printed. `HOTSPOT_PSK`, when it is not the default, is printed with the
  same word, for the same reason.

  `settings` also prints `WIFI_COUNTRY`, `TIMEZONE`, `WIFI_MODE` and
  `HOTSPOT_SSID`. A timezone of `Etc/UTC` and a mode of `auto` are the
  defaults and Reflash shows them as that.

  **Optional software** is listed too: `SOFTWARE_LIST=a b` (the names, lower
  case letters, digits and `_`), and for each `SOFTWARE_a=on|off` and
  `SOFTWARE_a_INFO=` a line for people. Nothing is listed when there is
  nothing to offer, and Reflash then shows no section. Installing is
  `configure` with `SOFTWARE_a=on`, and may need the network (see below).
  LED effects are cloned into the printer user's `~/klipper-led_effect` and
  registered with Moonraker's updater. Off removes the module link and updater
  section, leaving the checkout for reuse. Existing standalone module files
  are left alone; the info line identifies software installed separately.
  Restoring config files that reference an unavailable `led_effect` module
  emits a warning. It does not download software as part of restore.

  Reflash keeps `SSH_ENABLED`, `SCREEN_ROTATION` and the Wi-Fi network the same
  as the installed system's: it reads them when it starts, and applies a change
  made in Reflash to the installed system, through `configure`, as soon as it
  is made - only the settings that changed. A change made while an image is
  being written is kept in Reflash and goes onto the new image when the
  install finishes.
- **`backup`**: write the user's own files - configuration, and whatever else
  the image judges worth keeping across a reinstall - to stdout as a
  gzip-compressed tar archive. Reflash stores it as it is; the only look
  inside is at an uploaded file's first tar header, to tell a backup from an
  image. Changes nothing.
  A Rebuild release before this interface (v1.0.x, v1.1.0) has no manifest and
  no installer, so it has no `backup` action; Reflash backs those up itself
  (#187), because they will never change and the image they move to has
  `restore`. It reads the system's second partition read-only, takes the same
  files an installer would (`printer_data/config`, `printer_data/database`,
  OctoPrint's settings, users and data, minus logs and timelapses) and writes
  them in the archive format above under the names a current system uses: a
  release before v1.1 has no `printer` user and keeps everything under
  `debian`'s home, so the paths are renamed to `home/printer/...` and owned by
  `printer`. Only for a release it recognises by `/etc/rebuild-version`; any
  other system without a manifest is "not supported", as before. Checked
  against the v1.0.2 and v1.1.0 fluidd and octoprint images.

- **`list`**: print the files a `backup` would hold, one path per line, as
  `backup` stores them (excluding what it excludes). For the tree in which the
  user picks what to save. Changes nothing.
- **`list-archive`**: the same for an archive given on stdin: only the files a
  `restore` would take, not the manifest. Changes nothing.

  With `list` in `actions=`, `backup` and `restore` also take any number of
  `--include PATH` arguments, paths as `list` prints them (a folder includes
  what is in it). `backup` then holds only those. `restore` puts back only
  those, laid over what is there and leaving everything else as it is; without
  `--include` it replaces, as before. A path the system does not hold is
  refused. Without `list` in `actions=`, Reflash always saves and restores
  everything.

  `restore --merge` puts back everything the archive holds, laid over what is
  there, and removes nothing. Reflash asks for it whenever the installer lists
  `list` and no files were chosen, because its page promises that the rest of
  the config stays: an archive of three files put into a system with eight must
  not take the other five away (it did, with plain `restore`, which replaces
  each folder it holds).
- **`restore`**: read an archive made by `backup` on stdin and put its files
  back. Reflash runs it in two places: after `prepare` and before
  `configure` on a freshly written image, so the user's choices in Reflash
  win over restored ones; and on its own, into the system already on the
  board, when the user installs a backup without an image. The archive may
  come from an older version of the same system, or hold none of this
  system's files; the installer decides what still applies, and says so on
  stderr.

### Output and exit status

Exit status 0 is success, and 3 from an optional action means it is not
supported; anything else fails the installation. Everything the
installer prints goes to Reflash's log. A line starting with `ERROR: ` is a
reason meant for the user, and the last one is shown when the installer fails.

### What the installer may not do

Install packages, start services, or take more than a few minutes. It runs on
a board that may be in hotspot mode with no internet, and the user is waiting.
It does not use the network either, with one exception: **optional software**
that is fetched (`SOFTWARE_<name>=on`, #205), which is cloned with git. Reflash
puts its own `/etc/resolv.conf` in the image for the length of a `configure`
(the image's is usually a link into `/run`, which a chroot does not have) and
puts the image's back after. The installer must check that it can reach the
source before it changes anything, and without a connection must change nothing
and fail with an `ERROR: ` line saying so. Reflash does not offer the choice
without internet, and applies every other setting first.

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
