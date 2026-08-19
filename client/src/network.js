// Map signal strength in dBm to 1-4 bars, or null when there is nothing to
// show. RSSI is always negative in practice, so 0 (or a missing value) means
// the board could not read it - not a perfect signal.
export function signalBars(rssi) {
  if (!rssi || rssi >= 0) {
    return null;
  }
  if (rssi >= -55) return 4;
  if (rssi >= -65) return 3;
  if (rssi >= -75) return 2;
  return 1;
}

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

// Build the "Network:" rows for the info panel (#117).
//
// One row per active transport, not one "the" connection: the board can be on
// ethernet and WiFi at once, on the same subnet, and then which one is carrying
// the page is genuinely ambiguous (#112). Showing both, and marking which one
// holds the default route, is the honest answer - measured on a real board the
// WiFi won on metric while the cable was plugged in, which is not guessable.
//
// Each row is {text, active, hotspot, bars, rssi}; bars is null unless there is
// a signal reading to draw.
export function networkLines(network) {
  const lines = [];
  const eth = network && network.ethernet;
  const wifi = network && network.wifi;
  const withIp = (ip) => (ip ? ` (${ip})` : "");

  if (eth && eth.up) {
    lines.push({
      text: "Ethernet" + withIp(eth.ip),
      active: !!eth.active,
      hotspot: false,
      bars: null,
      rssi: 0,
    });
  }

  if (wifi && wifi.present && (wifi.mode === "ap" || wifi.ssid)) {
    const hotspot = wifi.mode === "ap";
    lines.push({
      text: "WiFi - " + (wifi.ssid || "Recore") + withIp(wifi.ip),
      active: !!wifi.active,
      // Flagged rather than spelled out in the text: in hotspot mode the user
      // is connected to the board itself, so there is no internet and the
      // github downloads cannot work.
      hotspot,
      // No bar for the hotspot: the reading is this board's view of clients,
      // which is not the "how good is my link" the bar is asking about.
      bars: hotspot ? null : signalBars(wifi.rssi),
      rssi: wifi.rssi || 0,
    });
  }

  // Only claim "not connected" once the server has actually answered. An empty
  // object is "not loaded yet", which is not the same thing, and saying the
  // board is offline on a page it just served would be absurd.
  if (lines.length === 0 && (eth || wifi)) {
    lines.push({ text: "not connected", active: false, hotspot: false, bars: null, rssi: 0 });
  }
  return lines;
}
