import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import TheInfo from '../../src/components/TheInfo.vue'

// This component is presentational, so unlike TheWifiSetup these assert on what
// is rendered. Kept to text and the presence of the badge/bar elements - not to
// class names or markup shape, which would just restate the template.
function render(network, open = true) {
  return mount(TheInfo, {
    props: {
      open,
      version: 'v1.2.3',
      revision: 'A7',
      serialNumber: 'RC-0042',
      network,
    },
  })
}

describe('TheInfo', () => {
  it('shows the board identity', () => {
    const w = render({})
    expect(w.text()).toContain('Reflash version: v1.2.3')
    expect(w.text()).toContain('Recore revision: A7')
    expect(w.text()).toContain('Recore serial number: RC-0042')
  })

  it('renders nothing when closed', () => {
    const w = render({ ethernet: { up: true, ip: '10.0.0.2' } }, false)
    expect(w.text()).not.toContain('Network:')
  })

  it('lists both transports and marks only the active one', () => {
    const w = render({
      ethernet: { up: true, ip: '192.168.32.198', active: false },
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '192.168.32.161', active: true, rssi: -55 },
    })
    expect(w.text()).toContain('Network: Ethernet (192.168.32.198)')
    expect(w.text()).toContain('Network: WiFi - HomeNet (192.168.32.161)')
    // One badge, on the row that holds the default route (#112).
    expect(w.findAll('.badge')).toHaveLength(1)
    expect(w.find('.badge').text()).toBe('active')
  })

  it('draws a signal bar for a WiFi link, with the reading available on hover', () => {
    const w = render({
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '10.0.0.2', rssi: -60 },
    })
    const signal = w.find('.signal')
    expect(signal.exists()).toBe(true)
    expect(signal.attributes('title')).toBe('-60 dBm')
    // Four segments always drawn, three filled - so it reads as "3 of 4".
    expect(signal.findAll('i')).toHaveLength(4)
    expect(signal.findAll('i.on')).toHaveLength(3)
  })

  it('badges the hotspot instead of drawing a bar', () => {
    const w = render({
      wifi: { present: true, mode: 'ap', ssid: 'Recore', ip: '192.168.50.1', active: true },
    })
    expect(w.text()).toContain('Network: WiFi - Recore (192.168.50.1)')
    expect(w.find('.signal').exists()).toBe(false)
    const badges = w.findAll('.badge').map((b) => b.text())
    expect(badges).toContain('hotspot')
    expect(badges).toContain('active')
  })

  it('draws no bar when the signal could not be read', () => {
    // 0 is "unreadable", not a perfect signal - a missing /proc/net/wireless
    // must not render as full strength.
    const w = render({
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '10.0.0.2', rssi: 0 },
    })
    expect(w.text()).toContain('Network: WiFi - HomeNet')
    expect(w.find('.signal').exists()).toBe(false)
  })

  it('says nothing about the network before the server has answered', () => {
    // An empty object is "not loaded yet"; claiming the board is offline on a
    // page it just served would be absurd.
    const w = render({})
    expect(w.text()).not.toContain('Network:')
  })

  it('reports not connected once the server says nothing is up', () => {
    const w = render({
      ethernet: { up: false, ip: '' },
      wifi: { present: true, mode: 'station', ssid: '', ip: '' },
    })
    expect(w.text()).toContain('Network: not connected')
  })
})
