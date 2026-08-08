# Shared helpers for the Reflash bash-helper test suite (bats-core).
#
# The scripts under bin/prod talk to real hardware/system paths (iwctl, wget,
# /dev/mmcblk2, /mnt/config, ...). These helpers let a test run them in a
# throwaway sandbox by (a) prepending a directory of fake commands ("shims") to
# PATH and (b) pointing the scripts' env seams at the sandbox.

# Absolute path to bin/prod, derived from this file's location.
PROD_BIN="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../bin/prod" && pwd)"

# Create a clean sandbox + an empty shim dir on the front of PATH.
# Sets: SANDBOX, SHIMDIR, CALLS (a log of every shimmed command's argv).
setup_sandbox() {
  SANDBOX="$(mktemp -d)"
  SHIMDIR="$SANDBOX/shims"
  mkdir -p "$SHIMDIR"
  CALLS="$SANDBOX/calls.log"
  : > "$CALLS"
  PATH="$SHIMDIR:$PATH"
  export PATH CALLS
  # Common env seam: keep every script's log inside the sandbox.
  export LOG_FILE="$SANDBOX/reflash.log"
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
