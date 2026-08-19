import { describe, it, expect } from 'vitest'
import { networkLines, wifiSummary } from '../../src/network'

describe('networkLines', () => {
  it('shows ethernet with its IP', () => {
    expect(networkLines({ ethernet: { up: true, ip: '192.168.1.42' } }))
      .toEqual(['Ethernet (192.168.1.42)'])
  })

  it('shows a WiFi client with its SSID and IP', () => {
    expect(networkLines({
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '192.168.1.87' },
    })).toEqual(['WiFi - HomeNet (192.168.1.87)'])
  })

  it('flags the hotspot as having no internet', () => {
    expect(networkLines({
      wifi: { present: true, mode: 'ap', ssid: 'Recore', ip: '192.168.8.1' },
    })).toEqual(['WiFi hotspot - Recore (192.168.8.1) (no internet)'])
  })

  // The case #112 was about: both up, on the same subnet, and it is not
  // knowable which one is carrying the page.
  it('lists both transports when both are up', () => {
    expect(networkLines({
      ethernet: { up: true, ip: '192.168.1.42' },
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '192.168.1.87' },
    })).toEqual(['Ethernet (192.168.1.42)', 'WiFi - HomeNet (192.168.1.87)'])
  })

  it('omits ethernet when the cable is out', () => {
    expect(networkLines({
      ethernet: { up: false, ip: '' },
      wifi: { present: true, mode: 'station', ssid: 'HomeNet', ip: '192.168.1.87' },
    })).toEqual(['WiFi - HomeNet (192.168.1.87)'])
  })

  // wifi-connect falls back to the hotspot on any failure (#90), so a present
  // adapter in station mode with no SSID means "did not join anything".
  it('reports not connected when nothing is up', () => {
    expect(networkLines({
      ethernet: { up: false, ip: '' },
      wifi: { present: true, mode: 'station', ssid: '', ip: '' },
    })).toEqual(['not connected'])
  })

  it('stays silent until the server has answered', () => {
    expect(networkLines({})).toEqual([])
    expect(networkLines(undefined)).toEqual([])
  })

  it('drops the parenthesised IP when there is no lease yet', () => {
    expect(networkLines({ ethernet: { up: true, ip: '' } })).toEqual(['Ethernet'])
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
