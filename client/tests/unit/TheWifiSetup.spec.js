import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import axios from 'axios'
import TheWifiSetup from '../../src/components/TheWifiSetup.vue'

vi.mock('axios', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))

// Every bug this file pins was in component state, not in markup - so these
// assert on state and on the requests that went out, never on rendered HTML.
// shallowMount keeps the wave-ui children stubbed, which is what makes that
// possible without dragging the whole UI kit into the test.
function mountDialog(open = false) {
  return shallowMount(TheWifiSetup, {
    props: { open },
    global: {
      mocks: {
        $waveui: { theme: 'dark', notify: vi.fn() },
        // mapGetters(["options"]) reaches into a store the tests do not build.
        $store: { getters: { options: {} } },
      },
    },
  })
}

// Default answers for everything the dialog fetches on open, so individual
// tests only have to say what is different.
function stubStatus(wifi = {}, aps = []) {
  axios.get.mockImplementation((url) => {
    if (url === '/api/get_status') {
      return Promise.resolve({
        status: 200,
        data: { network: { ethernet: {}, wifi: { present: true, ...wifi } } },
      })
    }
    if (url === '/api/get_wifi') {
      return Promise.resolve({ status: 200, data: { SSID: '', password: '' } })
    }
    if (url === '/api/wifi_poll_scan') {
      return Promise.resolve({ status: 200, data: aps })
    }
    return Promise.reject(new Error('unexpected GET ' + url))
  })
}

// The dialog fires several requests per open; flush them all.
const settle = async (wrapper, times = 4) => {
  for (let i = 0; i < times; i++) await wrapper.vm.$nextTick()
}

// beginReconnect fires a probe immediately and does not await it. Tests that
// swap the axios mock afterwards must let that first probe land, or its late
// resolution overwrites whatever the swapped mock produced.
const flushProbe = () => vi.advanceTimersByTimeAsync(0)

beforeEach(() => {
  vi.clearAllMocks()
  axios.post.mockResolvedValue({ status: 202 })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('restoring state on open (#105)', () => {
  it('repopulates the saved SSID', async () => {
    // Regression: inputSSID was assigned by getInfo() but never declared in
    // data(), so it was not reactive and nothing was bound to it - the field
    // silently stayed empty and getInfo() was never called at all.
    stubStatus({ mode: 'station', ssid: 'HomeNet' })
    axios.get.mockImplementation((url) => {
      if (url === '/api/get_wifi') {
        return Promise.resolve({ status: 200, data: { SSID: 'HomeNet', password: '' } })
      }
      if (url === '/api/wifi_poll_scan') {
        return Promise.resolve({ status: 200, data: [{ SSID: 'HomeNet', signal: '****' }] })
      }
      return Promise.resolve({
        status: 200,
        data: { network: { ethernet: {}, wifi: { present: true } } },
      })
    })

    const wrapper = mountDialog(false)
    await wrapper.setProps({ open: true })
    await settle(wrapper)

    expect(wrapper.vm.inputSSID).toBe('HomeNet')
  })

  it('repopulates the access point list from the server cache', async () => {
    stubStatus({}, [
      { SSID: 'HomeNet', signal: '****' },
      { SSID: 'CoffeeShop', signal: '**' },
    ])
    const wrapper = mountDialog(false)
    await wrapper.setProps({ open: true })
    await settle(wrapper)

    expect(wrapper.vm.availableAPs.map((a) => a.SSID)).toEqual(['HomeNet', 'CoffeeShop'])
    // The label is what the select renders, and it is built here rather than
    // by the server.
    expect(wrapper.vm.availableAPs[0].label).toBe('HomeNet ****')
  })

  it('reselects the saved network so the dialog returns where it was left', async () => {
    axios.get.mockImplementation((url) => {
      if (url === '/api/get_wifi') {
        return Promise.resolve({ status: 200, data: { SSID: 'HomeNet' } })
      }
      if (url === '/api/wifi_poll_scan') {
        return Promise.resolve({
          status: 200,
          data: [{ SSID: 'Other', signal: '*' }, { SSID: 'HomeNet', signal: '****' }],
        })
      }
      return Promise.resolve({
        status: 200,
        data: { network: { ethernet: {}, wifi: { present: true } } },
      })
    })

    const wrapper = mountDialog(false)
    await wrapper.setProps({ open: true })
    await settle(wrapper)

    expect(wrapper.vm.selected?.SSID).toBe('HomeNet')
  })

  it('leaves the list alone while a scan is in flight', async () => {
    // 204 means "scanning, nothing yet"; overwriting the list with the empty
    // body would blank a list the user is looking at.
    axios.get.mockImplementation((url) => {
      if (url === '/api/wifi_poll_scan') return Promise.resolve({ status: 204, data: '' })
      if (url === '/api/get_wifi') return Promise.resolve({ status: 200, data: { SSID: '' } })
      return Promise.resolve({
        status: 200,
        data: { network: { ethernet: {}, wifi: { present: true } } },
      })
    })

    const wrapper = mountDialog(false)
    wrapper.vm.availableAPs = [{ SSID: 'Kept', signal: '*', label: 'Kept *' }]
    await wrapper.setProps({ open: true })
    await settle(wrapper)

    expect(wrapper.vm.availableAPs.map((a) => a.SSID)).toEqual(['Kept'])
  })
})

describe('adapter presence', () => {
  it('does not conclude the dongle is gone when a request fails', async () => {
    // Regression: the catch block set isWifiPresent = false, so any failed
    // request became a claim about the hardware. Scanning takes the AP down,
    // which is exactly when this used to be polling - the whole dialog body
    // collapsed mid-scan on a board with a dongle fitted.
    stubStatus({ mode: 'station', ssid: 'HomeNet' })
    const wrapper = mountDialog(false)
    await wrapper.setProps({ open: true })
    await settle(wrapper)
    expect(wrapper.vm.isWifiPresent).toBe(true)

    axios.get.mockRejectedValue(new Error('network error'))
    await wrapper.vm.getWifiStatus()
    await settle(wrapper)

    expect(wrapper.vm.isWifiPresent).toBe(true)
  })

  it('stays quiet about a missing dongle until the server has answered', async () => {
    // isWifiPresent starts false, so before the first reply "not present" only
    // means "not asked yet" - rendering the warning then is how it flashed.
    const wrapper = mountDialog(false)
    expect(wrapper.vm.wifiStatusKnown).toBe(false)

    stubStatus({ present: false })
    await wrapper.setProps({ open: true })
    await settle(wrapper)

    expect(wrapper.vm.wifiStatusKnown).toBe(true)
  })
})

describe('reconnect watch after a mode switch', () => {
  beforeEach(() => vi.useFakeTimers())

  it('reports switching, then unreachable, then connected', async () => {
    stubStatus({ mode: 'ap', ssid: 'Recore' })
    const wrapper = mountDialog(true)
    await settle(wrapper)
    wrapper.vm.selected = { SSID: 'HomeNet' }

    await wrapper.vm.startWifiConnect()
    expect(axios.post).toHaveBeenCalledWith('/api/wifi_start_connect', {
      SSID: 'HomeNet',
      password: '',
    })
    expect(wrapper.vm.statusMessage).toContain('switching to HomeNet')
    // Buttons stay locked while the radio is moving.
    expect(wrapper.vm.busy).toBe(true)
    await flushProbe()

    // Board unreachable: the hint to reconnect this computer appears.
    axios.get.mockRejectedValue(new Error('network error'))
    await vi.advanceTimersByTimeAsync(2500)
    expect(wrapper.vm.boardReachable).toBe(false)
    expect(wrapper.vm.reconnecting).toBe(true)

    // Board answers, and agrees it joined the network that was asked for.
    stubStatus({ mode: 'station', ssid: 'HomeNet', ip: '10.0.0.5' })
    await vi.advanceTimersByTimeAsync(2500)

    expect(wrapper.vm.statusMessage).toBe('Connected to HomeNet.')
    expect(wrapper.vm.reconnecting).toBe(false)
    expect(wrapper.vm.boardReachable).toBe(true)
    // The heading and the buttons come back from the same fresh reading.
    expect(wrapper.vm.wifi.ssid).toBe('HomeNet')
    expect(wrapper.vm.busy).toBe(false)
  })

  it('does not call it connected just because the board answered', async () => {
    // Straight after the request the board is still on the old network.
    // Treating a reply as success would announce a connection that has not
    // happened.
    stubStatus({ mode: 'station', ssid: 'OldNet' })
    const wrapper = mountDialog(true)
    await settle(wrapper)
    wrapper.vm.selected = { SSID: 'HomeNet' }

    await wrapper.vm.startWifiConnect()
    await flushProbe()
    await vi.advanceTimersByTimeAsync(2500)

    expect(wrapper.vm.reconnecting).toBe(true)
    expect(wrapper.vm.statusMessage).not.toContain('Connected to')
  })

  it('does not call a connect failed just because the hotspot is still up', async () => {
    // Connecting *from* the hotspot is the common case: the board is serving
    // the AP this page arrived on. The first probe sees an AP it has not torn
    // down yet, and reading that as the #90 fallback declared the attempt
    // failed before the board had done anything.
    stubStatus({ mode: 'ap', ssid: 'Recore' })
    const wrapper = mountDialog(true)
    await settle(wrapper)
    wrapper.vm.selected = { SSID: 'HomeNet' }

    await wrapper.vm.startWifiConnect()
    await flushProbe()
    await vi.advanceTimersByTimeAsync(2500)

    expect(wrapper.vm.statusMessage).not.toContain('Could not join')
    expect(wrapper.vm.reconnecting).toBe(true)

    // ...and once it really has switched, it still reports success.
    stubStatus({ mode: 'station', ssid: 'HomeNet' })
    await vi.advanceTimersByTimeAsync(2500)
    expect(wrapper.vm.statusMessage).toBe('Connected to HomeNet.')
  })

  it('reports the silent fallback to the hotspot as a failure (#90)', async () => {
    // wifi-connect restores the AP on any failure - bad password, AP out of
    // range - and nothing used to say so, leaving the user believing they had
    // joined their network. Starting from station mode, so the move to AP is a
    // real transition rather than a hotspot that was already up.
    stubStatus({ mode: 'station', ssid: 'OldNet' })
    const wrapper = mountDialog(true)
    await settle(wrapper)
    wrapper.vm.selected = { SSID: 'HomeNet' }

    await wrapper.vm.startWifiConnect()
    await flushProbe()
    stubStatus({ mode: 'ap', ssid: 'Recore' })
    await vi.advanceTimersByTimeAsync(2500)

    expect(wrapper.vm.statusMessage).toContain('Could not join HomeNet')
    expect(wrapper.vm.reconnecting).toBe(false)
  })

  it('settles when the hotspot switch completes', async () => {
    stubStatus({ mode: 'station', ssid: 'HomeNet' })
    const wrapper = mountDialog(true)
    await settle(wrapper)

    await wrapper.vm.startHotspot()
    expect(axios.post).toHaveBeenCalledWith('/api/wifi_start_hotspot')
    expect(wrapper.vm.busy).toBe(true)
    await flushProbe()

    stubStatus({ mode: 'ap', ssid: 'Recore', ip: '192.168.50.1' })
    await vi.advanceTimersByTimeAsync(2500)

    expect(wrapper.vm.statusMessage).toBe('Connected to Recore.')
    expect(wrapper.vm.busy).toBe(false)
  })

  it('stops watching when the dialog closes', async () => {
    // The watch is bounded by the dialog, not by a deadline - otherwise it is
    // another loop that runs forever behind a closed dialog (#115).
    stubStatus({ mode: 'station', ssid: 'OldNet' })
    const wrapper = mountDialog(true)
    await settle(wrapper)
    wrapper.vm.selected = { SSID: 'HomeNet' }
    await wrapper.vm.startWifiConnect()
    expect(wrapper.vm.reconnecting).toBe(true)

    await wrapper.setProps({ open: false })
    expect(wrapper.vm.reconnecting).toBe(false)

    const before = axios.get.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(axios.get.mock.calls.length).toBe(before)
  })
})
