# Shared helpers for the Reflash bash-helper test suite (bats-core).
#
# The scripts under bin/prod talk to real hardware/system paths (iwctl, wget,
# /dev/mmcblk2, /mnt/config, ...). These helpers let a test run them in a
# throwaway sandbox by (a) prepending a directory of fake commands ("shims") to
# PATH and (b) pointing the scripts' env seams at the sandbox.

# Absolute paths into the repo, derived from this file's location.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# What the Go server shells out to.
PROD_BIN="$REPO_ROOT/bin/prod"
# What systemd and udev read, copied into the image verbatim (see #120).
ROOTFS_TREE="$REPO_ROOT/rootfs_files/rootfs"

# Create a clean sandbox + an empty shim dir on the front of PATH.
# Sets: SANDBOX, SHIMDIR, CALLS (a log of every shimmed command's argv).
setup_sandbox() {
  SANDBOX="$(mktemp -d)"
  SHIMDIR="$SANDBOX/shims"
  mkdir -p "$SHIMDIR"
  CALLS="$SANDBOX/calls.log"
  : > "$CALLS"
  # Shims first, then the real scripts: on a board every bin/prod script is
  # installed into one directory on PATH, so they can call each other - and
  # since #138 they do, rather than each re-implementing the config read. Stubs
  # still win, being earlier in PATH.
  PATH="$SHIMDIR:$PROD_BIN:$PATH"
  export PATH CALLS
  # Common env seam: keep every script's log inside the sandbox.
  export LOG_FILE="$SANDBOX/reflash.log"
  # And its identity cache, so tests neither write to /run nor inherit a value
  # cached by an earlier test.
  export REFLASH_CACHE_DIR="$SANDBOX/cache"
}

teardown_sandbox() {
  [ -n "${SANDBOX:-}" ] && rm -rf "$SANDBOX"
}

# stub NAME [EXIT_CODE]   (canned stdout read from this function's stdin)
# Installs a fake command on PATH that records its argv to $CALLS, prints the
# heredoc body to stdout, then exits with EXIT_CODE (default 0).
#
#   stub iwctl <<'OUT'
#   Mode station
#   OUT
stub() {
  local name="$1" code="${2:-0}" body
  body="$(cat)"
  {
    echo '#!/usr/bin/env bash'
    echo "echo \"$name \$*\" >> \"$CALLS\""
    echo "cat <<'__STUB_OUT__'"
    printf '%s\n' "$body"
    echo '__STUB_OUT__'
    echo "exit $code"
  } > "$SHIMDIR/$name"
  chmod +x "$SHIMDIR/$name"
}

# stub_stderr NAME [EXIT_CODE]   (canned stderr read from this function's stdin)
# Same as stub, but the heredoc body goes to stderr - for commands like wget
# that write their real diagnostics there, not stdout.
#
#   stub_stderr wget 1 <<'ERR'
#   wget: unable to resolve host address 'example'
#   ERR
stub_stderr() {
  local name="$1" code="${2:-0}" body
  body="$(cat)"
  {
    echo '#!/usr/bin/env bash'
    echo "echo \"$name \$*\" >> \"$CALLS\""
    echo "cat <<'__STUB_ERR__' >&2"
    printf '%s\n' "$body"
    echo '__STUB_ERR__'
    echo "exit $code"
  } > "$SHIMDIR/$name"
  chmod +x "$SHIMDIR/$name"
}

# stub_silent NAME [EXIT_CODE] — a fake command with no stdout (records argv).
# Handy for no-op system tools (mount, sleep, partprobe, ...) and for making a
# command "fail" with a chosen exit code.
stub_silent() {
  local name="$1" code="${2:-0}"
  {
    echo '#!/usr/bin/env bash'
    echo "echo \"$name \$*\" >> \"$CALLS\""
    echo "exit $code"
  } > "$SHIMDIR/$name"
  chmod +x "$SHIMDIR/$name"
}

# assert_called_with SUBSTRING — fail unless some recorded call contains it.
assert_called_with() {
  if ! grep -qF -- "$1" "$CALLS"; then
    echo "expected a call matching: $1" >&2
    echo "recorded calls:" >&2
    cat "$CALLS" >&2
    return 1
  fi
}
