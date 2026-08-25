# Bash helper tests (bats-core)

Unit tests for the helper scripts in `bin/prod/` — the scripts the Go server
shells out to. They run hermetically: no root, no real WiFi adapter, no eMMC.

`image-files.bats` is the odd one out: it tests the image build's inputs
(`rootfs_files/` and the ordering inside `mkimage.sh`) rather than a helper's
behaviour, because the failure it guards against — a file silently missing from
the image — is invisible to every other test here (#120).

## Running

```sh
# Debian/Ubuntu
sudo apt-get install bats

make test-bats        # or: bats test/bats
```

## How it works

Each script reads its hardware/system paths from environment variables that
**default to the on-board locations**, so production behaviour is unchanged:

| Script(s)                 | Seam env vars                                            |
|---------------------------|---------------------------------------------------------|
| `wifi-*`                  | `WIFI_INTERFACE`, `SYS_NET`, `IWD_DIR`, `WIFI_CONFIG_FILE`, `LOG_FILE` |
| `flash-from-url`          | `REFLASH_TARGET`, `REFLASH_CONFIG_DIR`, `REFLASH_EMMC_MNT`, `LOG_FILE` |
| `get-reflash-version`     | `REFLASH_VERSION_FILE`                                   |
| `get-recore-serial-number`| `REFLASH_CONFIG_DIR`                                     |

`helper.bash` sets up a temp sandbox, points those seams at it, and prepends a
shim directory to `PATH`. Tests install fake commands (`iwctl`, `wget`, `ip`,
`mount`, ...) with `stub` / `stub_silent`; every shimmed call is recorded so
`assert_called_with` can check what the script asked the system to do. The
`SYS_NET` seam lets a test simulate "no dongle fitted" (empty dir) vs. "dongle
present" (`with_adapter`).

## Notes

- `flash-from-url`'s full success path mounts real partitions and rewrites
  UUIDs (needs root), so it isn't exercised end-to-end here — the tests cover
  revision parsing and download-failure handling.
- One test is `skip`ped on purpose: it documents the desired fix for issue #59
  (surface the real download error instead of swallowing it). Un-skip it once
  `flash-from-url` no longer redirects the pipeline's stderr to
  `/tmp/recore-flash-progress`.
