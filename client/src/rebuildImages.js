// Turning GitHub's releases payload into the list behind "Choose image to
// Download". Its own module because it is pure - payload in, list out - and a
// pure function is the part of this worth testing.
//
// The floor. Reflash has not been tested against anything older and those
// images are not recommended, so they are not offered (#172). 1.0.2 is the
// release before the current line.
const OLDEST_OFFERED = [1, 0, 2];

// "v1.1.0-RC8" -> [1, 1, 0]. The prerelease suffix is deliberately dropped: a
// release's *version* is what the floor is about, and whether it is a
// prerelease is already a field of its own on the release.
function versionOf(tag) {
  const m = /^v?(\d+)\.(\d+)\.(\d+)/.exec(String(tag || ''));
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null;
}

function compare(a, b) {
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] - b[i];
  }
  return 0;
}

// The list the picker shows, newest first.
//
// Sorted here rather than taken as it arrives: the order used to be whatever
// the API returned, which put the newest image somewhere down the list (#171).
// De-duplicated for the same reason - the picker is fed from two places that
// can both land, and a list with an image in it twice is a list nobody trusts.
export function selectRebuildImages(releases, showPrereleases) {
  if (!Array.isArray(releases)) return [];       // an error body is not a list

  const wanted = releases
    .filter((r) => r && !r.draft)
    .filter((r) => showPrereleases || !r.prerelease)
    .map((r) => ({ release: r, version: versionOf(r.tag_name) }))
    .filter(({ version }) => version && compare(version, OLDEST_OFFERED) >= 0)
    .sort((a, b) => compare(b.version, a.version));

  const seen = new Set();
  const images = [];
  for (const { release } of wanted) {
    const assets = (release.assets || [])
      .filter((a) => a && typeof a.name === 'string' && a.name.startsWith('rebuild-'))
      // Within one release the order is the variants', and GitHub's asset order
      // is upload order - which is not something anyone chose.
      .sort((a, b) => a.name.localeCompare(b.name));
    for (const a of assets) {
      if (seen.has(a.name)) continue;
      seen.add(a.name);
      images.push({ name: a.name, id: a.id, url: a.browser_download_url, size: a.size });
    }
  }
  return images;
}
