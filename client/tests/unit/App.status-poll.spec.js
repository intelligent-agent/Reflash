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

// #131: "the text on the screen gets stuck there". A static banner cannot be
// told apart from a hang, so the wait is counted and a drive that runs out the
// clock is named as broken rather than merely absent.
describe('the preparing counter', () => {
  const withStorage = (storage, wait, max) => ({
    data: {
      local_images: [], bytes_available: 1, network: {},
      storage, storage_wait: wait, storage_wait_max: max,
    },
  });

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
  });
  afterEach(() => vi.useRealTimers());

  it('takes the elapsed seconds and the budget from the server', async () => {
    axios.get.mockResolvedValue(withStorage('PREPARING', 12, 180));
    const c = ctx('');
    c.storageWait = 0;
    c.storageWaitMax = 0;

    await getStatus.call(c);

    expect(c.storageWait).toBe(12);
    expect(c.storageWaitMax).toBe(180);
  });

  // An older server does not send these. undefined would render as empty
  // parentheses beside the message.
  it('defaults to zero when the server does not report them', async () => {
    axios.get.mockResolvedValue({
      data: { local_images: [], bytes_available: 1, network: {}, storage: 'PREPARING' },
    });
    const c = ctx('');
    c.storageWait = 99;
    c.storageWaitMax = 99;

    await getStatus.call(c);

    expect(c.storageWait).toBe(0);
    expect(c.storageWaitMax).toBe(0);
  });

  // storageTimedOut is derived, so verify the derivation rather than a field.
  const timedOut = App.computed.storageTimedOut;

  it('calls a drive broken only when it ran out the full budget', () => {
    expect(timedOut.call({ storage: 'FAILED', storageWait: 180, storageWaitMax: 180 })).toBe(true);
  });

  it('does not call an absent drive broken', () => {
    expect(timedOut.call({ storage: 'FAILED', storageWait: 0, storageWaitMax: 180 })).toBe(false);
  });

  it('does not call a drive broken while it is still preparing', () => {
    expect(timedOut.call({ storage: 'PREPARING', storageWait: 180, storageWaitMax: 180 })).toBe(false);
  });
});
