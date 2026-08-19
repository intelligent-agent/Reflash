import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import TheLogger from '../../src/components/TheLogger.vue'

// jsdom has no EventSource. This stand-in records the instances created so a
// test can drive onmessage/onerror by hand, which is the only way to reproduce
// a reconnect without a board.
class FakeEventSource {
  constructor(url) {
    this.url = url
    this.closed = false
    this.listeners = {}
    FakeEventSource.instances.push(this)
  }
  addEventListener(name, fn) {
    this.listeners[name] = fn
  }
  close() {
    this.closed = true
  }
  emit(text) {
    this.onmessage({ data: text })
  }
  fail() {
    this.onerror()
  }
  static reset() {
    FakeEventSource.instances = []
  }
  static get latest() {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1]
  }
}
FakeEventSource.instances = []

function mountLogger() {
  return shallowMount(TheLogger, {
    props: { open: true },
    global: { mocks: { $waveui: { theme: 'dark', notify: vi.fn() } } },
  })
}

beforeEach(() => {
  FakeEventSource.reset()
  global.EventSource = FakeEventSource
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('TheLogger', () => {
  it('shows the lines the server streams', () => {
    const w = mountLogger()
    FakeEventSource.latest.emit('[info] one')
    FakeEventSource.latest.emit('[info] two')
    expect(w.vm.log).toEqual(['[info] one', '[info] two'])
  })

  // The bug this pins: streamLog tails from offset 0, so every connection
  // replays the whole file. The board reconnects whenever it changes WiFi mode
  // (#95), and each reconnect used to append another full copy - the log
  // appeared two and three times over on a single boot.
  it('does not duplicate the log when the stream reconnects', async () => {
    const w = mountLogger()
    const first = FakeEventSource.latest
    first.emit('[info] one')
    first.emit('[info] two')

    // WiFi mode switch: the connection dies and the client reconnects.
    first.fail()
    expect(first.closed).toBe(true)
    await vi.advanceTimersByTimeAsync(3500)

    const second = FakeEventSource.latest
    expect(second).not.toBe(first)
    // The server replays from the start, exactly as it did the first time.
    second.emit('[info] one')
    second.emit('[info] two')
    second.emit('[info] three')

    expect(w.vm.log).toEqual(['[info] one', '[info] two', '[info] three'])
  })

  it('reconnects when the stream goes silent, not only on an error', async () => {
    // Confirmed live (#95): the connection can die with no error at all, so a
    // missed heartbeat is what has to catch it.
    const w = mountLogger()
    const first = FakeEventSource.latest
    first.emit('[info] one')

    // Watchdog fires at 30s and closes the dead stream...
    vi.advanceTimersByTime(31000)
    expect(first.closed).toBe(true)

    // ...and reconnect() waits 3s more before opening a new one, which is
    // what clears the buffer for the replay.
    await vi.advanceTimersByTimeAsync(3500)
    expect(FakeEventSource.latest).not.toBe(first)
    expect(w.vm.log).toEqual([])
  })

  it('a heartbeat keeps the connection alive', () => {
    const w = mountLogger()
    const first = FakeEventSource.latest
    first.emit('[info] one')

    // Heartbeats arrive every 15s; the watchdog fires at 30s without one.
    for (let i = 0; i < 3; i++) {
      vi.advanceTimersByTime(15000)
      first.listeners.heartbeat()
    }
    expect(first.closed).toBe(false)
    expect(w.vm.log).toEqual(['[info] one'])
  })
})
