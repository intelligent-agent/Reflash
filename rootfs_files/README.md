# rootfs_files

Everything `mkimage.sh` copies into the image rather than generates.

| directory | goes to | copied |
| --- | --- | --- |
| `boot/` | the image's boot partition (`boot.cmd`, `armbianEnv.txt`, splash) | after the chroot |
| `dtb/` | `boot/dtb/allwinner/` | after the chroot |
| `debs/` | the kernel package, unpacked in the chroot (gitignored - fetched on demand) | into the chroot |
| `rootfs-preinstall/` | `/`, before anything is installed | after debootstrap's first stage |
| `rootfs/` | `/`, as the last thing written | after the app install |

## `rootfs/` vs `bin/prod`

Both end up in `/usr/local/bin` on the board, and the difference is who runs
them:

- **`bin/prod`** is what the **Go server** shells out to (`wifi-connect`,
  `flash-from-url`, `get-recore-revision`, ...). Its tests live in
  `test/bats/`.
- **`rootfs/`** is what **systemd, udev, sysctl and iwd** read - units, network
  files, udev rules, the AP profile - plus `usb-gadget-init.sh`, which is a
  program only because a unit has to run something. Reflash never calls it.

Each file sits under its target path, so `rootfs/etc/systemd/system/reflash.service`
lands at `/etc/systemd/system/reflash.service`, and the comment explaining why
it says what it says lives in the file itself.

## Adding a file

Drop it in at its target path and commit it. `mkimage.sh` copies the whole tree
and then asserts every file in it made it into the built rootfs, so there is no
list to update - and a file that gets deleted or written over during the build
fails the build rather than the boot (#120).

Two things still have to be done by hand:

- **Enabling a unit.** Installing `foo.service` does not start it at boot; add a
  `systemctl enable foo --root=...` next to the others in `mkimage.sh`.
- **Choosing the tree.** `rootfs-preinstall/` exists for files that have to be
  in place before `apt` runs - the dpkg excludes and apt config. Anything the
  running image reads belongs in `rootfs/`.
