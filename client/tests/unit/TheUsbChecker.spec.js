import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { shallowMount } from '@vue/test-utils';
import axios from 'axios';
import TheUsbChecker from '@/components/TheUsbChecker.vue';

vi.mock('axios');

// The dialog's own polling uses setTimeout; drive it deterministically rather
// than waiting on real timers.
function mount(options) {
  return shallowMount(TheUsbChecker, {
    props: { open: false },
    global: {
      mocks: { $store: { getters: { options } } },
      stubs: { 'w-dialog': true, 'w-button': true, 'w-progress': true }
    }
  });
}

describe('TheUsbChecker', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('does NOT reboot when the drive is removed - the server does that', async () => {
    // Two mechanisms used to race on this condition, and at 500ms here against
    // the server's tick the browser always won, so checkAutoReboot went
    // untested and stayed broken (#123). The UI must now stand down.
    axios.get.mockResolvedValue({ data: { result: false } });
    const w = mount({ rebootWhenDone: true });

    await w.vm.checkUsbPresent();

    expect(w.emitted('reboot-board')).toBeUndefined();
    // ...but it should show that a reboot is expected, and start watching.
    expect(w.vm.rebootPressed).toBe(true);
    expect(w.vm.serverResponding).toBe(false);
  });

  it('keeps polling while the drive is still in', async () => {
    axios.get.mockResolvedValue({ data: { result: true } });
    const w = mount({ rebootWhenDone: true });

    await w.vm.checkUsbPresent();

    expect(w.emitted('reboot-board')).toBeUndefined();
    expect(w.vm.isUsbPresent).toBe(true);
    expect(w.vm.rebootPressed).toBe(false);
  });

  it('still reboots on the manual button, when reboot-when-done is off', async () => {
    axios.get.mockResolvedValue({ data: { result: false } });
    const w = mount({ rebootWhenDone: false });

    // With the option off the server will not reboot, so the button is the
    // only way out and must keep working.
    w.vm.clickReboot();

    expect(w.emitted('reboot-board')).toHaveLength(1);
    expect(w.vm.rebootPressed).toBe(true);
  });

  it('does not reboot itself when reboot-when-done is off', async () => {
    axios.get.mockResolvedValue({ data: { result: false } });
    const w = mount({ rebootWhenDone: false });

    await w.vm.checkUsbPresent();

    expect(w.emitted('reboot-board')).toBeUndefined();
  });

  it('reports what the user should do at each step', async () => {
    axios.get.mockResolvedValue({ data: { result: true } });
    const w = mount({ rebootWhenDone: true });
    w.vm.isUsbPresent = true;
    expect(w.vm.computeText()).toMatch(/remove usb drive/i);

    w.vm.isUsbPresent = false;
    expect(w.vm.computeText()).toMatch(/rebooting/i);

    const w2 = mount({ rebootWhenDone: false });
    w2.vm.isUsbPresent = false;
    expect(w2.vm.computeText()).toMatch(/ready to reboot/i);
  });
});
