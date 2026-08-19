import { describe, it, expect } from 'vitest'
import { networkLines, wifiSummary, signalBars } from '../../src/network'

describe('networkLines', () => {
  const texts = (n) => networkLines(n).map((l) => l.text)

  it('shows ethernet with its IP', () => {
    expect(texts({ ethernet: { up: true, ip: '192.168.1.42' } }))
      .toEqual(['Ethernet (192.168.1.42)'])
  })

  it('shows a WiFi client with its SSID and IP', () => {
    expect(texts({
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '192.168.1.87' },
    })).toEqual(['WiFi - HomeNet (192.168.1.87)'])
  })

  // Flagged, not spelled out in the text - the UI renders it as a badge.
  it('flags the hotspot rather than describing it', () => {
    const [ap] = networkLines({
      wifi: { present: true, mode: 'ap', ssid: 'Recore', ip: '192.168.8.1' },
    })
    expect(ap.text).toBe('WiFi - Recore (192.168.8.1)')
    expect(ap.hotspot).toBe(true)
  })

  it('does not flag a station link as a hotspot', () => {
    const [wifi] = networkLines({
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '10.0.0.2', rssi: -60 },
    })
    expect(wifi.hotspot).toBe(false)
  })

  // The case #112 was about: both up, on the same subnet, and only the default
  // route says which is carrying traffic. Measured on a real board, WiFi won.
  it('marks which of two live transports is actually active', () => {
    const lines = networkLines({
      ethernet: { up: true, ip: '192.168.32.198', active: false },
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '192.168.32.161', active: true },
    })
    expect(lines.map((l) => l.text)).toEqual([
      'Ethernet (192.168.32.198)',
      'WiFi - HomeNet (192.168.32.161)',
    ])
    expect(lines.map((l) => l.active)).toEqual([false, true])
  })

  it('carries the signal strength through as bars', () => {
    const [wifi] = networkLines({
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '10.0.0.2', rssi: -55 },
    })
    expect(wifi.bars).toBe(4)
    expect(wifi.rssi).toBe(-55)
  })

  // The hotspot reading is this board's view of its clients, not link quality.
  it('draws no bar for the hotspot, badge instead', () => {
    const [ap] = networkLines({
      wifi: { present: true, mode: 'ap', ssid: 'Recore', ip: '192.168.8.1', rssi: -40 },
    })
    expect(ap.bars).toBe(null)
    expect(ap.hotspot).toBe(true)
  })

  it('omits ethernet when the cable is out', () => {
    expect(texts({
      ethernet: { up: false, ip: '' },
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '192.168.1.87' },
    })).toEqual(['WiFi - HomeNet (192.168.1.87)'])
  })

  // wifi-connect falls back to the hotspot on any failure (#90), so a present
  // adapter in station mode with no SSID means "did not join anything".
  it('reports not connected when nothing is up', () => {
    expect(texts({
      ethernet: { up: false, ip: '' },
      wifi: { present: true, mode: 'station', ssid: '', ip: '' },
    })).toEqual(['not connected'])
  })

  it('stays silent until the server has answered', () => {
    expect(networkLines({})).toEqual([])
    expect(networkLines(undefined)).toEqual([])
  })

  it('drops the parenthesised IP when there is no lease yet', () => {
    expect(texts({ ethernet: { up: true, ip: '' } })).toEqual(['Ethernet'])
  })
})

describe('signalBars', () => {
  it('maps dBm to 1-4 bars', () => {
    expect(signalBars(-40)).toBe(4)
    expect(signalBars(-55)).toBe(4)
    expect(signalBars(-60)).toBe(3)
    expect(signalBars(-70)).toBe(2)
    expect(signalBars(-85)).toBe(1)
  })

  // A real reading is always negative, so 0 is "could not read it".
  it('treats 0 and missing as unknown, not as a perfect signal', () => {
    expect(signalBars(0)).toBe(null)
    expect(signalBars(undefined)).toBe(null)
  })
})

describe('wifiSummary', () => {
  it('names the network, not the device', () => {
    expect(wifiSummary({ present: true, mode: 'station', ssid: 'HomeNet', ip: '192.168.1.87' }))
      .toBe('Connected to: HomeNet (192.168.1.87)')
  })

  it('says access point when hosting the hotspot', () => {
    expect(wifiSummary({ present: true, mode: 'ap', ssid: 'Recore', ip: '192.168.8.1' }))
      .toBe('Access point: Recore (192.168.8.1)')
  })

  it('says not connected when the adapter has joined nothing', () => {
    expect(wifiSummary({ present: true, mode: 'station', ssid: '', ip: '' }))
      .toBe('Not connected to a network')
  })

  it('is empty with no adapter, so the dialog shows its own warning', () => {
    expect(wifiSummary({ present: false })).toBe('')
    expect(wifiSummary(undefined)).toBe('')
  })
})
