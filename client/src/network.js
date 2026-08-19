// One-line summary of what the WiFi adapter is doing, for the WiFi dialog.
//
// This used to read `name` out of `iwctl device list`, which is the *device* -
// so the dialog said "Connected as: wlan0" where it meant to name the network.
export function wifiSummary(wifi) {
  if (!wifi || !wifi.present) {
    return "";
  }
  const withIp = wifi.ip ? ` (${wifi.ip})` : "";
  if (wifi.mode === "ap") {
    return `Access point: ${wifi.ssid || "Recore"}${withIp}`;
  }
  if (wifi.ssid) {
    return `Connected to: ${wifi.ssid}${withIp}`;
  }
  return "Not connected to a network";
}

// Build the "Network:" lines for the info panel (#117).
//
// One line per active transport, not one "the" connection: the board can be on
// ethernet and WiFi at once, on the same subnet, and then which one is carrying
// the page is genuinely ambiguous (#112). Showing both is the honest answer.
//
// Kept out of TheInfo.vue so it can be unit-tested: the vitest setup runs in a
// node environment with no @vue/test-utils, so an SFC computed is not reachable.
export function networkLines(network) {
  const lines = [];
  const eth = network && network.ethernet;
  const wifi = network && network.wifi;
  const withIp = (ip) => (ip ? ` (${ip})` : "");

  if (eth && eth.up) {
    lines.push("Ethernet" + withIp(eth.ip));
  }

  if (wifi && wifi.present && wifi.mode === "ap") {
    // Worth spelling out: in hotspot mode the user is connected to the board,
    // so there is no internet and the github downloads cannot work. Saying it
    // here beats letting them find out from a failed download.
    lines.push(
      "WiFi hotspot - " + (wifi.ssid || "Recore") + withIp(wifi.ip) + " (no internet)"
    );
  } else if (wifi && wifi.present && wifi.ssid) {
    lines.push("WiFi - " + wifi.ssid + withIp(wifi.ip));
  }

  // Only claim "not connected" once the server has actually answered. An empty
  // object is "not loaded yet", which is not the same thing, and saying the
  // board is offline on a page it just served would be absurd.
  if (lines.length === 0 && (eth || wifi)) {
    lines.push("not connected");
  }
  return lines;
}
