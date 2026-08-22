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
