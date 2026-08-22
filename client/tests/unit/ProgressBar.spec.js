import { describe, it, expect, beforeEach } from 'vitest';
import { shallowMount } from '@vue/test-utils';
import ProgressBar from '@/components/ProgressBar.vue';

// The store getter the component maps; the popup logic does not care about the
// values beyond bandwidth, which update() samples into history.
const progress = { progress: 50, bandwidth: 1.5, timeStarted: Date.now() };

// mapGetters reads $store.getters, so mock that rather than overriding the
// component's computed block - Vue Test Utils v2 dropped the `computed`
// mounting option, and passing it replaces the component's own computeds
// wholesale, which makes plotVisible undefined rather than false.
function mount() {
  return shallowMount(ProgressBar, {
    global: {
      mocks: { $store: { getters: { progress } } },
      stubs: { 'w-progress': true, 'w-flex': true }
    }
  });
}

// A mouse event over a bar sitting 200px down a 1000px-wide window.
function evtAt(clientX, barTop = 200) {
  return {
    clientX,
    currentTarget: {
      getBoundingClientRect: () => ({ top: barTop, bottom: barTop + 20 })
    }
  };
}

describe('ProgressBar throughput popup', () => {
  beforeEach(() => {
    window.innerWidth = 1000;
    window.innerHeight = 800;
  });

  it('stays hidden until there are at least two samples', async () => {
    const w = mount();
    w.vm.hovering = true;
    w.vm.history = [1.0];
    await w.vm.$nextTick();
    // One point cannot draw a line, and an empty plot is worse than none.
    expect(w.vm.plotVisible).toBe(false);
    w.vm.history = [1.0, 1.2];
    await w.vm.$nextTick();
    expect(w.vm.plotVisible).toBe(true);
  });

  it('centres on the cursor when there is room', () => {
    const w = mount();
    w.vm.place(evtAt(500));
    expect(w.vm.popupLeft).toBe(500 - 280 / 2);
  });

  it('does not overflow the right edge', () => {
    const w = mount();
    w.vm.place(evtAt(995));
    // 1000 - 280 - 8
    expect(w.vm.popupLeft).toBe(712);
    expect(w.vm.popupLeft + 280).toBeLessThanOrEqual(1000);
  });

  it('does not overflow the left edge', () => {
    const w = mount();
    w.vm.place(evtAt(2));
    expect(w.vm.popupLeft).toBe(8);
  });

  it('sits above the bar, and flips below when there is no room above', () => {
    const w = mount();
    w.vm.place(evtAt(500, 300));
    expect(w.vm.popupTop).toBe(300 - 96 - 14);

    // Bar near the top of the window: above would be off-screen.
    w.vm.place(evtAt(500, 20));
    expect(w.vm.popupTop).toBe(20 + 20 + 14);
  });

  it('pins open on tap and unpins on a second tap', () => {
    const w = mount();
    w.vm.history = [1, 2];
    w.vm.togglePin(evtAt(500));
    expect(w.vm.pinned).toBe(true);
    expect(w.vm.plotVisible).toBe(true);   // visible without hover, for touch
    w.vm.togglePin(evtAt(500));
    expect(w.vm.pinned).toBe(false);
    expect(w.vm.plotVisible).toBe(false);
  });

  it('reset clears history and closes the popup', () => {
    const w = mount();
    w.vm.history = [1, 2];
    w.vm.hovering = true;
    w.vm.pinned = true;
    w.vm.reset();
    expect(w.vm.history).toEqual([]);
    expect(w.vm.plotVisible).toBe(false);
  });
});
