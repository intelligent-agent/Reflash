import { describe, it, expect, vi, beforeEach } from 'vitest';
import App from '@/App.vue';

// App.vue pulls in most of the UI, so these rules are exercised against a
// stand-in `this`, as in InstallGating.spec.js.
const m = App.methods;

// #198: the config to install is chosen under Set up printer, not on the main
// page, so the Install button is about the image alone; the config chosen there
// goes in with it.
describe('Install with a config', () => {
  const visible = (over) => m.isInstallButtonVisibile.call({
    options: { magicmode: false }, flash: { selectedMethod: 0 },
    selectedLocalImage: null, selectedConfig: '', ...over,
  });

  it('appears for an image, and a config chosen elsewhere does not make it appear', () => {
    expect(visible({ selectedLocalImage: 'a.img.xz' })).toBeTruthy();
    expect(visible({ selectedLocalImage: 'a.img.xz', selectedConfig: 'c.tar.gz' })).toBeTruthy();
    expect(visible({ selectedConfig: 'c.tar.gz' })).toBeFalsy();
    expect(visible({})).toBeFalsy();
  });

  it('installs an image, and a backup is still a backup', () => {
    const stand = (over) => ({
      flash: { selectedMethod: 0 }, state: 'IDLE', apiCall: vi.fn(),
      installSelected: vi.fn(), backupSelected: vi.fn(), ...over,
    });
    const image = stand({ selectedLocalImage: 'a.img.xz', selectedConfig: 'c.tar.gz' });
    m.onInstallButtonClick.call(image);
    expect(image.installSelected).toHaveBeenCalled();

    const backup = stand({ flash: { selectedMethod: 1 } });
    m.onInstallButtonClick.call(backup);
    expect(backup.backupSelected).toHaveBeenCalled();
  });

  it('has no config controls of its own any more', () => {
    for (const gone of ['restoreConfigOnly', 'backupConfigFiles', 'isConfigBackup', 'configChoices']) {
      expect(m[gone] || App.computed[gone]).toBeUndefined();
    }
  });
});

describe('the backup name', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 9, 8, 18, 44));
  });

  it('says which board, what was on it, and when', () => {
    const name = (over) => m.defaultBackupName.call({
      emmc_version: 'rebuild-fluidd-v1.1.0-93-gdc43e37', serial_number: '0132',
      ...over,
    });
    expect(name()).toBe('recore-0132-fluidd-2026-10-08-1844');
    expect(name({ emmc_version: 'Unknown', serial_number: 'Unknown' })).toBe('recore-2026-10-08-1844');
  });

  // An edited name is the user's; only the suggested one follows the target.
  it('is replaced only while it is still the suggested one', () => {
    const stand = {
      flash: { selectedMethod: 1 }, backupFile: 'my-own', suggestedBackupName: 'recore-x',
      defaultBackupName: () => 'recore-y',
    };
    m.suggestBackupName.call(stand);
    expect(stand.backupFile).toBe('my-own');
    stand.backupFile = 'recore-x';
    m.suggestBackupName.call(stand);
    expect(stand.backupFile).toBe('recore-y');
  });
});

describe('Local storage, both ways', () => {
  it('tells an image from a config archive by its first bytes', () => {
    const kind = (bytes) => m.uploadKindOf(new Uint8Array(bytes));
    expect(kind([0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00, 1])).toBe('Image');
    expect(kind([0x1f, 0x8b, 8, 0])).toBe('Config archive');
    expect(kind([0x50, 0x4b, 3, 4])).toBe('Not an image or a config archive');
  });

  // Archives are downloaded in the setup panel; this list is the images.
  it('downloads an image from the images', () => {
    expect(m.downloadUrl('a b.img.xz')).toBe('/api/images/download?name=a%20b.img.xz');
  });
});

import { shortName } from '@/App.vue';

// #188: one line in a narrow column, keeping what tells two files apart.
describe('shortName', () => {
  it('leaves a short name alone', () => {
    expect(shortName('fluidd.img.xz')).toBe('fluidd.img.xz');
  });

  it('keeps the start, and the end with the version and extension', () => {
    const s = shortName('rebuild-fluidd-v1.1.0-94-gdf7a98b.img.xz');
    expect(s.length).toBeLessThanOrEqual(32);
    expect(s.startsWith('rebuild-fluid')).toBe(true);
    expect(s.endsWith('94-gdf7a98b.img.xz')).toBe(true);
  });

  it('tells two backups apart by their time', () => {
    const a = shortName('recore-0132-barebone-config-2026-10-08-1844.tar.gz');
    const b = shortName('recore-0132-barebone-config-2026-10-08-2108.tar.gz');
    expect(a).not.toBe(b);
  });
});
