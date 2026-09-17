import { describe, it, expect, vi, beforeEach } from 'vitest';
import { shallowMount } from '@vue/test-utils';
import axios from 'axios';
import App from '@/App.vue';
import IntegrityChecker from '@/components/IntegrityChecker.vue';

vi.mock('axios');

// App.vue pulls in most of the UI, so the button rule is exercised directly
// against a stand-in `this` - it reads only these four fields.
const isInstallButtonDisabled = App.methods.isInstallButtonDisabled;

function disabled(over) {
  return isInstallButtonDisabled.call({
    flash: { selectedMethod: 0 },
    state: 'IDLE',
    imageIntegrity: true,
    ...over,
  });
}

describe('Install is refused for an image that failed its integrity check', () => {
  // The case that prompted this: an abandoned upload leaves a truncated
  // .img.xz in the list (#118). It shows a red X, and Install used to stay
  // clickable - which flashes an unbootable eMMC.
  it('is disabled when the check failed', () => {
    expect(disabled({ imageIntegrity: false })).toBe(true);
  });

  it('is enabled when the check passed', () => {
    expect(disabled({ imageIntegrity: true })).toBe(false);
  });

  // The spinner is up for a second or two. Enabling during it is how a
  // truncated image gets flashed by someone who clicks quickly.
  it('is disabled while the check is still running', () => {
    expect(disabled({ imageIntegrity: null })).toBe(true);
  });

  // Backups write no image, so there is nothing to verify and nothing to gate.
  it('does not gate backups', () => {
    expect(disabled({ flash: { selectedMethod: 1 }, imageIntegrity: null })).toBe(false);
  });

  // Mid-install the same button means Cancel. Disabling it there would strand
  // the user in an install they cannot stop.
  it.each(['INSTALLING', 'BACKUPING'])('stays clickable as Cancel during %s', (state) => {
    expect(disabled({ state, imageIntegrity: false })).toBe(false);
  });

  // Through a transfer it read "Install" and stayed enabled, and a click sent
  // cancel_installation or cancel_backup at the transfer (#156).
  it.each(['DOWNLOADING', 'UPLOADING', 'MAGIC', 'UPLOADING_MAGIC'])(
    'is disabled during %s, for install and backup alike', (state) => {
      expect(disabled({ state, imageIntegrity: true })).toBe(true);
      expect(disabled({ state, flash: { selectedMethod: 1 } })).toBe(true);
    });

  it.each([
    ['DOWNLOADING', 0], ['UPLOADING', 0], ['UPLOADING', 1], ['MAGIC', 1],
  ])('sends no cancel from a %s (method %i)', (state, selectedMethod) => {
    const apiCall = vi.fn();
    App.methods.onInstallButtonClick.call({
      flash: { selectedMethod }, state, apiCall,
      installSelected: vi.fn(), backupSelected: vi.fn(),
    });
    expect(apiCall).not.toHaveBeenCalled();
  });
});

describe('deleting an image (#153)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });

  function stand(over = {}) {
    return {
      selectedLocalImage: 'truncated.img.xz',
      confirmDelete: false,
      confirmDeleteTimer: null,
      getStatus: vi.fn(),
      $waveui: { notify: vi.fn() },
      ...over,
    };
  }

  it('asks first, and deletes on the second click', async () => {
    axios.put.mockResolvedValue({ data: { status: 'OK' } });
    const self = stand();
    await App.methods.onDeleteImageClick.call(self);
    expect(axios.put).not.toHaveBeenCalled();
    expect(self.confirmDelete).toBe(true);

    await App.methods.onDeleteImageClick.call(self);
    expect(axios.put).toHaveBeenCalledWith('/api/delete_image', { filename: 'truncated.img.xz' });
    expect(self.selectedLocalImage).toBe(null);
    expect(self.getStatus).toHaveBeenCalled();
  });

  it('forgets the first click after a few seconds', async () => {
    const self = stand();
    await App.methods.onDeleteImageClick.call(self);
    vi.advanceTimersByTime(5000);
    expect(self.confirmDelete).toBe(false);
  });

  it('says why when the server refuses', async () => {
    axios.put.mockResolvedValue({ data: { status: 'ERROR', error: 'busy: UPLOADING' } });
    const self = stand({ confirmDelete: true });
    await App.methods.onDeleteImageClick.call(self);
    expect(self.$waveui.notify).toHaveBeenCalledWith('busy: UPLOADING', 'error', 0);
  });

  it('is only offered for an image, while nothing is running', () => {
    const visible = (over) => App.methods.isDeleteButtonVisible.call({
      options: { magicmode: false }, flash: { selectedMethod: 0 },
      selectedLocalImage: 'a.img.xz', state: 'IDLE', ...over,
    });
    expect(visible({})).toBe(true);
    expect(visible({ state: 'UPLOADING' })).toBe(false);
    expect(visible({ selectedLocalImage: null })).toBe(false);
    expect(visible({ options: { magicmode: true } })).toBe(false);
    expect(visible({ flash: { selectedMethod: 1 } })).toBe(false);
  });
});

describe('IntegrityChecker reports its verdict', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  function mount() {
    return shallowMount(IntegrityChecker, {
      global: {
        mocks: { $waveui: { theme: 'dark' } },
        stubs: { 'w-spinner': true },
      },
    });
  }

  it('emits the pass and the fail', async () => {
    axios.put.mockResolvedValue({ data: { is_file_ok: false } });
    const w = mount();

    await w.vm.fileSelected('truncated.img.xz');

    // null first (a check is running, so there is no answer yet), then the
    // verdict - App keeps Install disabled on both.
    expect(w.emitted('integrity')).toEqual([[null], [false]]);
  });

  it('reports a clear when the selection is emptied', async () => {
    const w = mount();
    await w.vm.fileSelected('');
    expect(w.emitted('integrity')).toEqual([[null]]);
    expect(w.vm.spinner_visible).toBe(false);
  });

  // A failed request is not a pass. Reporting null keeps Install disabled
  // rather than letting a network blip open the gate.
  it('does not report a pass when the check itself fails', async () => {
    axios.put.mockRejectedValue(new Error('network'));
    const w = mount();

    await w.vm.fileSelected('something.img.xz');

    expect(w.emitted('integrity')).toEqual([[null], [null]]);
    expect(w.vm.spinner_visible).toBe(false);
  });
});
