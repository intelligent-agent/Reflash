import { describe, it, expect, beforeEach } from 'vitest';
import { shallowMount } from '@vue/test-utils';
import ProgressBar from '@/components/ProgressBar.vue';

const progress = { progress: 50, bandwidth: 1.5, timeStarted: Date.now() };

function mount() {
  return shallowMount(ProgressBar, {
    props: { revision: 'A8' },
    global: {
      mocks: { $store: { getters: { progress } } },
      stubs: { 'w-progress': true, 'w-flex': true, TheMetrics: true }
    }
  });
}

function evtAt(clientX, barTop = 500) {
  return {
    clientX,
    currentTarget: {
      getBoundingClientRect: () => ({ top: barTop, bottom: barTop + 20 })
    }
  };
}

function setBarRect(w, barTop = 500) {
  w.vm.$refs.bar.getBoundingClientRect = () => ({ top: barTop, bottom: barTop + 20 });
}

describe('ProgressBar board metrics hover', () => {
  beforeEach(() => {
    window.innerWidth = 1000;
    window.innerHeight = 800;
  });

  it('is hidden until the progress bar is hovered or tapped', async () => {
    const w = mount();
    expect(w.vm.metricsVisible).toBe(false);
    w.vm.hovering = true;
    await w.vm.$nextTick();
    expect(w.vm.metricsVisible).toBe(true);
    expect(w.find('the-metrics-stub').exists()).toBe(true);
  });

  it('passes the board revision and active state to the metrics panel', async () => {
    const w = mount();
    w.vm.hovering = true;
    await w.vm.$nextTick();
    const panel = w.find('the-metrics-stub');
    expect(panel.attributes('revision')).toBe('A8');
    expect(panel.attributes('active')).toBe('true');
  });

  it('centres the full-width panel where it fits', () => {
    const w = mount();
    setBarRect(w);
    w.vm.place(evtAt(500));
    expect(w.vm.popupLeft).toBe(90);
  });

  it('uses the viewport width without overflowing on a narrow screen', () => {
    window.innerWidth = 600;
    const w = mount();
    setBarRect(w);
    w.vm.place(evtAt(595));
    expect(w.vm.popupLeft).toBe(8);
  });

  it('sits above the bar and flips below it when necessary', () => {
    const w = mount();
    setBarRect(w, 500);
    w.vm.place(evtAt(500, 500));
    expect(w.vm.popupTop).toBe(500 - 420 - 14);

    setBarRect(w, 20);
    w.vm.place(evtAt(500, 20));
    expect(w.vm.popupTop).toBe(20 + 20 + 14);
  });

  it('pins open on tap and closes on a second tap', () => {
    const w = mount();
    setBarRect(w);
    w.vm.togglePin(evtAt(500));
    expect(w.vm.pinned).toBe(true);
    expect(w.vm.metricsVisible).toBe(true);
    w.vm.togglePin(evtAt(500));
    expect(w.vm.pinned).toBe(false);
    expect(w.vm.metricsVisible).toBe(false);
  });

  it('resets by closing the metrics panel', () => {
    const w = mount();
    w.vm.hovering = true;
    w.vm.pinned = true;
    w.vm.reset();
    expect(w.vm.metricsVisible).toBe(false);
  });
});
