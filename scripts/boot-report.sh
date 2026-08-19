#!/bin/bash
#
# Print where a board's boot time actually went, so the early-UI experiment can
# be judged on numbers instead of impressions.
#
#   scripts/boot-report.sh [host]
#
# Reads three things:
#   - systemd's own accounting, for everything before Reflash starts
#   - the "boot:" lines Reflash logs per startup phase
#   - the mkfs/USB-reset evidence from the kernel, which is what #116 is about
#
# Read-only. Run it after a boot; /var/log/reflash.log is on tmpfs, so the
# numbers are gone once the board reboots.

set -uo pipefail

HOST="${1:-${RECORE_HOST:-recore.local}}"
USER_NAME="${RECORE_USER:-debian}"
PASSWORD="${RECORE_PASS:-temppwd}"

board() {
    sshpass -p "$PASSWORD" ssh \
        -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -o ConnectTimeout=10 -o LogLevel=ERROR \
        "$USER_NAME@$HOST" "$@"
}

if ! board true 2>/dev/null; then
    echo "No board reachable at $HOST (set RECORE_HOST/RECORE_USER/RECORE_PASS)" >&2
    exit 1
fi

echo "=== image ==="
board "cat /etc/reflash-version 2>/dev/null"
board "grep -q REFLASH_EARLY_UI /etc/systemd/system/reflash.service.d/*.conf 2>/dev/null \
       && echo 'early-UI: ON' || echo 'early-UI: off'"

echo
echo "=== to first frame and first byte served ==="
# reflash.service's start timestamp against the phase log tells us how much of
# the delay is systemd getting there versus Reflash's own startup.
board "systemd-analyze time 2>/dev/null | head -2"
board "systemctl show reflash -p ActiveEnterTimestampMonotonic --value 2>/dev/null \
       | awk '{printf \"reflash.service active at %.2fs\n\", \$1/1000000}'"

echo
echo "=== Reflash startup phases ==="
board "grep '^\[info\] boot:' /var/log/reflash.log 2>/dev/null" || echo "  (none - old binary?)"

echo
echo "=== slowest units before Reflash ==="
board "systemd-analyze blame 2>/dev/null | head -8"

echo
echo "=== USB drive health during boot (#116) ==="
board "dmesg 2>/dev/null | grep -c 'reset high-speed USB device' \
       | awk '{print \"  USB resets: \" \$1}'"
board "dmesg 2>/dev/null | grep -iE 'error -110|device descriptor read' | tail -3 | sed 's/^/  /'"
