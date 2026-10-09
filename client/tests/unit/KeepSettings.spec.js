import { describe, it, expect, vi, beforeEach } from 'vitest';
import axios from 'axios';
import App from '@/App.vue';

// #195: "Keep my settings" is offered when an image is chosen and the
// installed system can say what its settings are. Rules exercised against a
// stand-in `this`, as in ConfigBackups.spec.js.
const c = App.computed;
const m = App.methods;

const now = (current, supported = true) => ({ supported, current });

describe('Keep my settings is offered', () => {
  const offered = (over) => c.keepSettingsOffered.call({
    flash: { selectedMethod: 0 }, selectedLocalImage: 'a.img.xz',
    installedNow: now({ SSH_ENABLED: 'true' }), ...over,
  });

  it('for an image, when the installed system has settings to carry', () => {
    expect(offered({})).toBe(true);
  });

  it('not without an image, not for a backup, not when unsupported or empty', () => {
    expect(offered({ selectedLocalImage: null })).toBe(false);
    expect(offered({ flash: { selectedMethod: 1 } })).toBe(false);
    expect(offered({ installedNow: now({ SSH_ENABLED: 'true' }, false) })).toBe(false);
    expect(offered({ installedNow: now({}) })).toBe(false);
  });
});

describe('What it would carry', () => {
  const summary = (current) => c.keepSettingsSummary.call({ installedNow: now(current) });

  it('says Wi-Fi, rotation and SSH in words', () => {
    expect(summary({ WIFI_SSID: 'home', SCREEN_ROTATION: '270', SSH_ENABLED: 'false' }))
      .toBe('Wi-Fi home, rotation 270°, SSH off');
  });

  it('leaves out what the system does not have, and shows no passphrase', () => {
    expect(summary({ SSH_ENABLED: 'true', WIFI_PSK: 'secret', WIFI_SSID: '' })).toBe('SSH on');
    expect(summary({ SCREEN_ROTATION: '0' })).toBe('rotation 0°');
    expect(summary({ WIFI_PSK: 'secret' })).not.toContain('secret');
  });
});

describe('Reading the installed settings', () => {
  beforeEach(() => vi.restoreAllMocks());

  it('keeps a supported answer', async () => {
    vi.spyOn(axios, 'get').mockResolvedValue({ data: { supported: true, current: { SSH_ENABLED: 'true' } } });
    const self = { installedNow: now({}, false) };
    await m.loadInstalledNow.call(self);
    expect(self.installedNow.current.SSH_ENABLED).toBe('true');
  });

  it('treats unsupported, busy and errors alike: not offered', async () => {
    for (const reply of [
      () => Promise.resolve({ data: { supported: false, reason: 'too old' } }),
      () => Promise.reject(new Error('409 busy')),
    ]) {
      vi.spyOn(axios, 'get').mockImplementation(reply);
      const self = { installedNow: now({ SSH_ENABLED: 'true' }) };
      await m.loadInstalledNow.call(self);
      expect(self.installedNow.supported).toBe(false);
      expect(self.installedNow.current).toEqual({});
    }
  });
});
