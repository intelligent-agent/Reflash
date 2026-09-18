import { describe, it, expect } from 'vitest';
import { selectRebuildImages } from '@/rebuildImages';

// Shaped like the GitHub releases payload, with only the fields the picker uses.
function release(tag, { prerelease = false, draft = false, variants = ['barebone', 'fluidd'] } = {}) {
  return {
    tag_name: tag,
    prerelease,
    draft,
    assets: variants.map((v, i) => ({
      name: `rebuild-${v}-${tag}.img.xz`,
      id: `${tag}-${i}`,
      browser_download_url: `http://example/${v}-${tag}`,
      size: 1000 + i,
    })),
  };
}

const names = (list) => list.map((i) => i.name);

describe('the Rebuild image picker', () => {
  // The order used to be whatever the API handed back. It is the newest image
  // people want, so it belongs at the top whatever GitHub does (#171).
  it('puts the newest release first, whatever order the payload arrives in', () => {
    const out = selectRebuildImages(
      [release('v1.0.2'), release('v1.1.0'), release('v1.0.3')],
      false
    );
    expect(names(out)[0]).toContain('v1.1.0');
    expect(names(out)).toEqual([
      'rebuild-barebone-v1.1.0.img.xz', 'rebuild-fluidd-v1.1.0.img.xz',
      'rebuild-barebone-v1.0.3.img.xz', 'rebuild-fluidd-v1.0.3.img.xz',
      'rebuild-barebone-v1.0.2.img.xz', 'rebuild-fluidd-v1.0.2.img.xz',
    ]);
  });

  // Reflash has not been tested against them and they are not recommended
  // (#172). 1.0.2 is the previous release and the floor.
  it('drops releases older than 1.0.2', () => {
    const out = selectRebuildImages(
      [release('v1.0.2'), release('v1.0.1'), release('v1.0.0'), release('v0.1.0')],
      false
    );
    expect(names(out).every((n) => n.includes('v1.0.2'))).toBe(true);
  });

  it('hides prereleases unless they are asked for', () => {
    const payload = [release('v1.1.0-RC8', { prerelease: true }), release('v1.0.2')];

    expect(names(selectRebuildImages(payload, false)).some((n) => n.includes('RC8'))).toBe(false);
    expect(names(selectRebuildImages(payload, true))[0]).toContain('RC8');
  });

  // A prerelease of a version below the floor is still below the floor.
  it('applies the floor to prereleases too', () => {
    const out = selectRebuildImages([release('v1.0.1-RC1', { prerelease: true })], true);
    expect(out).toEqual([]);
  });

  it('never shows drafts', () => {
    const out = selectRebuildImages([release('v1.2.0', { draft: true }), release('v1.0.2')], true);
    expect(names(out).some((n) => n.includes('v1.2.0'))).toBe(false);
  });

  // The picker is fed by two calls that can both land (created() and
  // checkInternet()), and a duplicated list is a list nobody trusts.
  it('lists each image once even if the payload is repeated', () => {
    const payload = [release('v1.0.2')];
    const out = selectRebuildImages([...payload, ...payload], false);
    expect(names(out)).toEqual([
      'rebuild-barebone-v1.0.2.img.xz',
      'rebuild-fluidd-v1.0.2.img.xz',
    ]);
  });

  // Only Rebuild images belong in a Rebuild picker; a release can carry other
  // assets (checksums, a Reflash image someone attached by hand).
  it('ignores assets that are not Rebuild images', () => {
    const rel = release('v1.0.2');
    rel.assets.push({ name: 'SHA256SUMS', id: 'x', browser_download_url: 'u', size: 1 });
    rel.assets.push({ name: 'reflash-v1.0.2.img.xz', id: 'y', browser_download_url: 'u', size: 1 });
    expect(names(selectRebuildImages([rel], false))).toEqual([
      'rebuild-barebone-v1.0.2.img.xz',
      'rebuild-fluidd-v1.0.2.img.xz',
    ]);
  });

  it('survives a payload that is not a list', () => {
    expect(selectRebuildImages(undefined, false)).toEqual([]);
    expect(selectRebuildImages({ message: 'API rate limit exceeded' }, false)).toEqual([]);
  });

  it('keeps the fields the picker and the download need', () => {
    const [first] = selectRebuildImages([release('v1.0.2')], false);
    expect(first).toEqual({
      name: 'rebuild-barebone-v1.0.2.img.xz',
      id: 'v1.0.2-0',
      url: 'http://example/barebone-v1.0.2',
      size: 1000,
    });
  });
});

// The switch is a question about a list the page already has. Re-fetching to
// answer it would put a GitHub round trip - and its rate limit - behind a
// toggle.
describe('the pre-release switch re-filters what is already loaded', () => {
  it('needs only the cached payload to change the list', () => {
    const payload = [release('v1.1.0-RC8', { prerelease: true }), release('v1.0.2')];

    const hidden = selectRebuildImages(payload, false);
    const shown = selectRebuildImages(payload, true);

    expect(hidden).toHaveLength(2);
    expect(shown).toHaveLength(4);
    expect(names(shown)[0]).toContain('RC8');
  });
});
