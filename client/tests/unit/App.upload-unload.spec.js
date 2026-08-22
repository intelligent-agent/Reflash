import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import axios from 'axios';
import App from '@/App.vue';

vi.mock('axios');

// App.vue mounts most of the UI, so these drive the two methods directly
// against a stand-in `this`. Both depend only on `ownsUpload` and, for the
// unload handler, navigator.sendBeacon - so a full mount would cost a lot of
// stubbing to test nothing extra.
const cancelUploadOnUnload = App.methods.cancelUploadOnUnload;
const apiCall = App.methods.apiCall;

describe('cancelling an upload when the page goes away', () => {
  let beacon;

  beforeEach(() => {
    vi.clearAllMocks();
    beacon = vi.fn(() => true);
    navigator.sendBeacon = beacon;
  });

  afterEach(() => {
    delete navigator.sendBeacon;
  });

  it('tells the server on the way out, so the drive is not left mounted rw', () => {
    // The whole point of #118: a refresh used to strand the server in
    // UPLOADING until the watchdog gave up on it minutes later.
    const ctx = { ownsUpload: true };
    cancelUploadOnUnload.call(ctx);

    expect(beacon).toHaveBeenCalledWith('/api/upload_cancel');
  });

  // The hazard that makes ownsUpload necessary. `state` is refreshed from
  // get_progress, so a tab that is merely watching also reads UPLOADING -
  // closing it must not cancel the upload another tab is driving.
  it('stays out of it when this tab did not start the upload', () => {
    const ctx = { ownsUpload: false };
    cancelUploadOnUnload.call(ctx);

    expect(beacon).not.toHaveBeenCalled();
  });

  it('gives up ownership so it cannot fire twice', () => {
    const ctx = { ownsUpload: true };
    cancelUploadOnUnload.call(ctx);
    cancelUploadOnUnload.call(ctx);

    expect(ctx.ownsUpload).toBe(false);
    expect(beacon).toHaveBeenCalledTimes(1);
  });

  // sendBeacon is not universal, and an unload handler that throws would
  // block the navigation it is running in.
  it('does nothing rather than throwing where sendBeacon is missing', () => {
    delete navigator.sendBeacon;
    const ctx = { ownsUpload: true };

    expect(() => cancelUploadOnUnload.call(ctx)).not.toThrow();
  });
});

describe('upload ownership ends with the upload', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    axios.put.mockResolvedValue({ data: {} });
  });

  // Otherwise a later refresh would fire upload_cancel at whatever the server
  // had moved on to - an install, or somebody else's upload.
  it.each(['upload_finish', 'upload_magic_finish', 'upload_cancel'])(
    'releases ownership on %s',
    async (call) => {
      const ctx = { ownsUpload: true, $waveui: { notify: vi.fn() } };
      await apiCall.call(ctx, call);
      expect(ctx.ownsUpload).toBe(false);
    }
  );

  it('keeps ownership across calls that do not end the upload', async () => {
    const ctx = { ownsUpload: true, $waveui: { notify: vi.fn() } };
    await apiCall.call(ctx, 'reboot_board');
    expect(ctx.ownsUpload).toBe(true);
  });
});
