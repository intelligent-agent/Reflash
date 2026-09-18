import { describe, it, expect, vi, beforeEach } from 'vitest';
import axios from 'axios';
import App from '@/App.vue';

vi.mock('axios');

// Every one of these buttons asks the board to start something. If that request
// does not land, the ones that set the state optimistically used to leave the UI
// on a progress bar for a transfer that never started (#166), and the ones that
// do not set it said nothing at all - including when the board had answered 409
// with a reason worth reading.
function stand(over = {}) {
  return {
    state: 'IDLE',
    ownsUpload: false,
    status: null,
    backupFile: 'backup',
    file: { name: 'image.img.xz', size: 1024 },
    selectedGithubImage: { name: 'image.img.xz', size: 1024, url: 'http://example/image.img.xz' },
    selectedLocalImage: 'image.img.xz',
    checkProgress: vi.fn(),
    resetProgressBars: vi.fn(),
    setProgress: vi.fn(),
    uploadLocalFile: vi.fn(),
    magicUploadLocalFile: vi.fn(),
    $refs: { installprogressbar: { update: vi.fn() } },
    $waveui: { notify: vi.fn() },
    // The real one, not a stub: the failure handling is what is under test
    // here, and on a live component every method sits on the same instance.
    transferFailed: App.methods.transferFailed,
    ...over,
  };
}

const started = [
  ['downloadSelected', 'DOWNLOADING'],
  ['uploadSelected', 'UPLOADING'],
  ['startMagic', 'MAGIC'],
  ['startMagicUpload', 'UPLOADING_MAGIC'],
];

describe('a transfer that cannot be started does not leave the UI pretending (#166)', () => {
  beforeEach(() => vi.clearAllMocks());

  for (const [method, busyState] of started) {
    it(`${method}: puts the state back to IDLE when the request fails`, async () => {
      axios.put.mockRejectedValue(new Error('Network Error'));
      const self = stand();

      await App.methods[method].call(self);

      expect(self.state).toBe('IDLE');
      expect(self.checkProgress).not.toHaveBeenCalled();
      expect(self.$waveui.notify).toHaveBeenCalled();
    });

    it(`${method}: leaves the state alone when the request succeeds`, async () => {
      axios.put.mockResolvedValue({ data: { success: true } });
      const self = stand();

      await App.methods[method].call(self);

      expect(self.state).toBe(busyState);
      expect(self.$waveui.notify).not.toHaveBeenCalled();
    });
  }

  // The uploads also claim ownership of the transfer for this tab, which drives
  // the cancel-on-unload beacon. A start that failed owns nothing.
  it('a failed upload start does not leave this tab owning the upload', async () => {
    axios.put.mockRejectedValue(new Error('Network Error'));
    const self = stand();

    await App.methods.uploadSelected.call(self);

    expect(self.ownsUpload).toBe(false);
  });
});

// start_installation and start_magic answer 409 with a sentence explaining that
// the eMMC stopped responding and the board needs a power cycle (#137). It is
// the most useful thing the server can say, and it was being dropped.
describe('the board\'s refusal reaches the user', () => {
  beforeEach(() => vi.clearAllMocks());

  it('installSelected surfaces the message from a refused install', async () => {
    axios.put.mockRejectedValue({
      response: { status: 409, data: 'The eMMC stopped responding. Power cycle the board and try again.' },
    });
    const self = stand();

    await App.methods.installSelected.call(self);

    expect(self.checkProgress).not.toHaveBeenCalled();
    const said = self.$waveui.notify.mock.calls.map(([msg]) => msg).join(' ');
    expect(said).toContain('eMMC stopped responding');
  });

  it('backupSelected reports a backup that could not be started', async () => {
    axios.put.mockRejectedValue(new Error('Network Error'));
    const self = stand();

    await App.methods.backupSelected.call(self);

    expect(self.checkProgress).not.toHaveBeenCalled();
    expect(self.$waveui.notify).toHaveBeenCalled();
  });
});
