import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import App from '@/App.vue';
import appSource from '../../src/App.vue?raw';
import theSetupSource from '../../src/components/TheSetup.vue?raw';
import theOptionsSource from '../../src/components/TheOptions.vue?raw';

vi.mock('axios');

// Driven against a stand-in `this`, like the other App specs.

describe('the install bar waits for the operation (#225)', () => {
  const isOperationRunning = App.methods.isOperationRunning;

  it('is free only when IDLE', () => {
    expect(isOperationRunning.call({ state: 'IDLE' })).toBe(false);
  });

  it.each(['DOWNLOADING', 'UPLOADING', 'UPLOADING_MAGIC', 'MAGIC', 'INSTALLING', 'BACKUPING'])(
    'is busy during %s', (state) => {
      expect(isOperationRunning.call({ state })).toBe(true);
    });

  it('binds all three drop-downs to it', () => {
    const bound = appSource.match(/:disabled="isOperationRunning\(\)"/g) || [];
    expect(bound.length).toBe(3);
  });
});

describe('the network rows follow a Wi-Fi change (#223)', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('reads the status now and again while the address arrives', () => {
    const c = { getStatus: vi.fn() };
    App.methods.refreshNetworkSoon.call(c);
    expect(c.getStatus).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(12000);
    expect(c.getStatus).toHaveBeenCalledTimes(4);
  });

  it('polls while the info panel is open and stops when it closes', () => {
    const c = { getStatus: vi.fn(), infoTimer: null };
    const watch = App.watch.openInfo;
    watch.call(c, true);
    expect(c.getStatus).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(10000);
    expect(c.getStatus).toHaveBeenCalledTimes(3);
    watch.call(c, false);
    vi.advanceTimersByTime(20000);
    expect(c.getStatus).toHaveBeenCalledTimes(3);
  });

  it('listens for the Wi-Fi dialog through the setup panel', () => {
    expect(theSetupSource).toContain("$emit('wifi-changed')");
    expect(appSource).toContain('@wifi-changed="refreshNetworkSoon()"');
  });
});

describe('Reflash\'s own Wi-Fi is reachable from Options (#224)', () => {
  it('has a button that asks the page to open the dialog', () => {
    expect(theOptionsSource).toContain("$emit('manage-wifi')");
    expect(appSource).toContain('$refs.setup.openWifi()');
  });

  it('opens the panel and the dialog', () => {
    const c = { expanded: false, wifiManagerOpen: false };
    // methods are plain functions on the options object
    return import('@/components/TheSetup.vue').then((m) => {
      m.default.methods.openWifi.call(c);
      expect(c.expanded).toBe(true);
      expect(c.wifiManagerOpen).toBe(true);
    });
  });
});
