# Shared helpers for the live-board test suite.
#
# These run against a real Recore running Reflash, over SSH, from the host.
# Unlike test/bats - which stubs everything and proves the scripts' logic - this
# proves the *image* behaves: the units that should be running are running, the
# gadget enumerated, the endpoints answer, and the config that shipped is the
# config in force.
#
# READ-ONLY BY CONSTRUCTION. Nothing here flashes, switches WiFi mode, writes to
# the eMMC or restarts a service. A board can be left running the suite without
# losing whatever state it is in. If you add a test, keep that true - the point
# is to be able to run this against a board mid-use.
#
# Config:
#   RECORE_HOST  board address        (default recore.local)
#   RECORE_USER  ssh user             (default debian)
#   RECORE_PASS  ssh password         (default temppwd)

RECORE_HOST="${RECORE_HOST:-recore.local}"
RECORE_USER="${RECORE_USER:-debian}"
RECORE_PASS="${RECORE_PASS:-temppwd}"

board_ssh() {
  sshpass -p "$RECORE_PASS" ssh \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    -o ConnectTimeout=10 -o LogLevel=ERROR \
    "$RECORE_USER@$RECORE_HOST" "$@"
}

# GET a Reflash endpoint. Issued from the board itself rather than from the
# host, so the suite does not also depend on which of the board's addresses the
# host happens to be able to route to.
board_api() {
  board_ssh "wget -qO- 'http://localhost$1'"
}

# Skip rather than fail when there is no board: the suite lives alongside the
# offline ones and must not turn a laptop-only `make test` run red.
require_board() {
  command -v sshpass >/dev/null 2>&1 || skip "sshpass not installed"
  board_ssh true >/dev/null 2>&1 || skip "no board reachable at $RECORE_HOST"
}

# Read a dotted path out of JSON on stdin. Prints nothing and fails if absent,
# so `[ "$(... | json_get a.b)" = "x" ]` reads naturally in a test.
json_get() {
  python3 -c "
import json,sys
d=json.load(sys.stdin)
for k in '$1'.split('.'):
    if isinstance(d,list): d=d[int(k)]
    else: d=d[k]
# json.dumps for everything but strings, so booleans come out as JSON's
# true/false rather than Python's True/False - the latter silently fails
# every `= "true"` comparison, and passes every `!= "true"` one.
print(d if isinstance(d,str) else json.dumps(d))
"
}

# Assert stdin is well-formed JSON - worth its own check, because a helper that
# emits a stray line turns into an unparseable body and takes out a whole panel.
assert_json() {
  python3 -c 'import json,sys; json.load(sys.stdin)'
}
