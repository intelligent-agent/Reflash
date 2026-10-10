// Countries for the Wi-Fi regulatory domain (#198): ISO 3166 codes, which is
// what the radio's rules are keyed by. Not every country: the ones a printer is
// likely to be in; another can be added here without touching anything else.
export const countries = [
  ["AR", "Argentina"], ["AU", "Australia"], ["AT", "Austria"], ["BE", "Belgium"], ["BR", "Brazil"],
  ["BG", "Bulgaria"], ["CA", "Canada"], ["CL", "Chile"], ["CN", "China"], ["HR", "Croatia"],
  ["CZ", "Czechia"], ["DK", "Denmark"], ["EG", "Egypt"], ["EE", "Estonia"], ["FI", "Finland"],
  ["FR", "France"], ["DE", "Germany"], ["GR", "Greece"], ["HK", "Hong Kong"], ["HU", "Hungary"],
  ["IS", "Iceland"], ["IN", "India"], ["ID", "Indonesia"], ["IE", "Ireland"], ["IL", "Israel"],
  ["IT", "Italy"], ["JP", "Japan"], ["KR", "South Korea"], ["LV", "Latvia"], ["LT", "Lithuania"],
  ["MY", "Malaysia"], ["MX", "Mexico"], ["NL", "Netherlands"], ["NZ", "New Zealand"], ["NO", "Norway"],
  ["PH", "Philippines"], ["PL", "Poland"], ["PT", "Portugal"], ["RO", "Romania"], ["SA", "Saudi Arabia"],
  ["RS", "Serbia"], ["SG", "Singapore"], ["SK", "Slovakia"], ["SI", "Slovenia"], ["ZA", "South Africa"],
  ["ES", "Spain"], ["SE", "Sweden"], ["CH", "Switzerland"], ["TW", "Taiwan"], ["TH", "Thailand"],
  ["TR", "Turkey"], ["UA", "Ukraine"], ["AE", "United Arab Emirates"], ["GB", "United Kingdom"],
  ["US", "United States"], ["VN", "Vietnam"],
].sort((a, b) => a[1].localeCompare(b[1]));

const FALLBACK_ZONES = [
  "Europe/Oslo", "Europe/Stockholm", "Europe/Copenhagen", "Europe/Helsinki", "Europe/London", "Europe/Berlin",
  "Europe/Paris", "Europe/Madrid", "Europe/Rome", "Europe/Amsterdam", "Europe/Warsaw", "Europe/Athens",
  "America/New_York", "America/Chicago", "America/Denver", "America/Los_Angeles", "America/Toronto",
  "America/Sao_Paulo", "Asia/Tokyo", "Asia/Shanghai", "Asia/Singapore", "Asia/Kolkata", "Australia/Sydney",
  "Pacific/Auckland", "Africa/Johannesburg", "UTC",
];

// The browser knows the names; the board checks them against its own zoneinfo.
export function timezones() {
  try {
    const zones = Intl.supportedValuesOf("timeZone");
    if (zones && zones.length) return zones.includes("UTC") ? zones : zones.concat("UTC");
  } catch (e) {
    // An older browser: the short list.
  }
  return FALLBACK_ZONES;
}

// The browser's own zone, to start the list on.
export function browserTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch (e) {
    return "";
  }
}
