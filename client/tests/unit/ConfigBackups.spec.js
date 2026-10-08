import { describe, it, expect, vi, beforeEach } from 'vitest';
import App from '@/App.vue';

// App.vue pulls in most of the UI, so these rules are exercised against a
// stand-in `this`, as in InstallGating.spec.js.
const m = App.methods;

// #184: Install appears for an image, a config, or both.
describe('Install with a config', () => {
  const visible = (over) => m.isInstallButtonVisibile.call({
    options: { magicmode: false }, flash: { selectedMethod: 0 },
    selectedLocalImage: null, selectedConfig: '', ...over,
  });

  it('appears for an image, a config, or both', () => {
    expect(visible({ selectedLocalImage: 'a.img.xz' })).toBeTruthy();
    expect(visible({ selectedConfig: 'c.tar.gz' })).toBeTruthy();
    expect(visible({ selectedLocalImage: 'a.img.xz', selectedConfig: 'c.tar.gz' })).toBeTruthy();
    expect(visible({})).toBeFalsy();
  });

  // There is no image to verify when only a config goes in.
  it('is not gated on integrity for a config alone', () => {
    expect(m.isInstallButtonDisabled.call({
      flash: { selectedMethod: 0 }, state: 'IDLE', imageIntegrity: null,
      selectedLocalImage: null, selectedConfig: 'c.tar.gz', configRestoreBusy: false,
    })).toBe(false);
  });

  it('puts a config alone into the installed system, and installs an image as before', () => {
    const stand = (over) => ({
      flash: { selectedMethod: 0 }, state: 'IDLE', backupTarget: 'eMMC', apiCall: vi.fn(),
      restoreConfigOnly: vi.fn(), installSelected: vi.fn(), ...over,
    });
    const alone = stand({ selectedLocalImage: null, selectedConfig: 'c.tar.gz' });
    m.onInstallButtonClick.call(alone);
    expect(alone.restoreConfigOnly).toHaveBeenCalled();
    expect(alone.installSelected).not.toHaveBeenCalled();

    const both = stand({ selectedLocalImage: 'a.img.xz', selectedConfig: 'c.tar.gz' });
    m.onInstallButtonClick.call(both);
    expect(both.installSelected).toHaveBeenCalled();
    expect(both.restoreConfigOnly).not.toHaveBeenCalled();
  });

  it('backs up the config files when Backup is set to Config files', () => {
    const stand = {
      flash: { selectedMethod: 1 }, state: 'IDLE', backupTarget: 'config',
      configBackupBusy: false, backupConfigFiles: vi.fn(), backupSelected: vi.fn(), apiCall: vi.fn(),
    };
    m.onInstallButtonClick.call(stand);
    expect(stand.backupConfigFiles).toHaveBeenCalled();
    expect(stand.backupSelected).not.toHaveBeenCalled();
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
      backupTarget: 'config', ...over,
    });
    expect(name()).toBe('recore-0132-fluidd-config-2026-10-08-1844');
    expect(name({ backupTarget: 'eMMC' })).toBe('recore-0132-fluidd-2026-10-08-1844');
    expect(name({ emmc_version: 'Unknown', serial_number: 'Unknown' })).toBe('recore-config-2026-10-08-1844');
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

  it('downloads a config archive and an image from their own places', () => {
    const url = (name) => m.downloadUrl.call({ configBackups: [{ name: 'c.tar.gz' }] }, name);
    expect(url('c.tar.gz')).toBe('/api/file_backups/download?name=c.tar.gz');
    expect(url('a b.img.xz')).toBe('/api/images/download?name=a%20b.img.xz');
  });
});
