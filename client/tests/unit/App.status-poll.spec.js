import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import axios from 'axios';
import App from '@/App.vue';

vi.mock('axios');

// Driven against a stand-in `this` rather than a full mount, like
// App.upload-unload.spec.js: getStatus touches four data fields and one timer,
// so mounting the whole UI would cost a lot of stubbing to test nothing extra.
const getStatus = App.methods.getStatus;

function ctx(storage) {
  const c = { localImages: [], bytesAvailable: -1, network: {}, storage };
  c.getStatus = getStatus.bind(c);
  return c;
}

const ok = (storage) => ({
  data: { local_images: [], bytes_available: 1, network: {}, storage },
});

describe('the storage poll', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });
  afterEach(() => vi.useRealTimers());

  // The #131 report: "Preparing drive stuck forever". The drive was not stuck -
  // the polling was. There was no catch, so one rejected request returned
  // before re-arming the timer and nothing else calls getStatus while the board
  // is still starting up. Seen in the browser as NS_BINDING_ABORTED, a request
  // the browser cancelled, which leaves no trace on the server at all.
  it('keeps polling after a request fails', async () => {
    axios.get.mockRejectedValue(new Error('NS_BINDING_ABORTED'));
    const c = ctx('PREPARING');

    await getStatus.call(c);

    expect(vi.getTimerCount()).toBe(1);
  });

  // storage starts as "", so a first poll that failed left it neither
  // PREPARING nor known - and a retry keyed only on PREPARING would never fire.
  it('retries when the very first poll fails, before storage is known', async () => {
    axios.get.mockRejectedValue(new Error('Network Error'));
    const c = ctx('');

    await getStatus.call(c);

    expect(vi.getTimerCount()).toBe(1);
  });

  it('keeps polling while the drive is still being prepared', async () => {
    axios.get.mockResolvedValue(ok('PREPARING'));
    const c = ctx('');

    await getStatus.call(c);

    expect(c.storage).toBe('PREPARING');
    expect(vi.getTimerCount()).toBe(1);
  });

  // The other half of the guarantee: it has to stop, or it polls the board
  // every second for as long as the page is open.
  it('stops once the drive is ready', async () => {
    axios.get.mockResolvedValue(ok('READY'));
    const c = ctx('PREPARING');

    await getStatus.call(c);

    expect(c.storage).toBe('READY');
    expect(vi.getTimerCount()).toBe(0);
  });

  it('stops when the drive has failed, rather than asking forever', async () => {
    axios.get.mockResolvedValue(ok('FAILED'));
    const c = ctx('PREPARING');

    await getStatus.call(c);

    expect(vi.getTimerCount()).toBe(0);
  });
});
