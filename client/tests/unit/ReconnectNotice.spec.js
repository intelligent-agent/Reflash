import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import axios from 'axios'
import ReconnectNotice from '../../src/components/ReconnectNotice.vue'
import { reconnectURL, isIPAddress } from '../../src/network'
vi.mock('axios', () => ({ default: { get: vi.fn() } }))
beforeEach(() => { axios.get.mockReset(); axios.get.mockResolvedValue({ data: { hostname: 'recore-42.local' } }); })
describe('reconnect guidance', () => {
 it('detects IPv4 and IPv6, excluding hostnames', () => {
  expect(isIPAddress('192.168.50.1')).toBe(true)
  expect(isIPAddress('[fe80::1]')).toBe(true)
  expect(isIPAddress('voron.local')).toBe(false)
 })
 it('preserves port and page, rejecting unverified names', () => {
  expect(reconnectURL('http://192.168.50.1:8080/?x=1#setup', 'recore-42.local')).toBe('http://recore-42.local:8080/?x=1#setup')
  for (const name of ['', 'evil.example', 'javascript:alert(1)', 'bad/name.local']) expect(reconnectURL('http://192.168.50.1/', name)).toBe('')
 })
 it('warns before connecting by IP and links to the running board', async () => {
  const w = mount(ReconnectNotice)
  w.vm.locationURL = 'http://192.168.50.1:8080/'
  await flushPromises()
  expect(w.text()).toContain('address may change')
  expect(w.find('a').attributes('href')).toBe('http://recore-42.local:8080/')
 })
 it('hides guidance on a hostname until a disconnected transition', async () => {
  const w = mount(ReconnectNotice, { props: { target: 'Home Wi-Fi' } })
  w.vm.locationURL = 'http://voron.local/'
  await flushPromises()
  expect(w.find('.reconnect-notice').exists()).toBe(false)
  await w.setProps({ active: true })
  expect(w.text()).toContain('Rejoin the Home Wi-Fi network')
 })
 it('provides discovery advice when the resolver is unavailable', async () => {
  axios.get.mockRejectedValue(new Error('offline'))
  const w = mount(ReconnectNotice, { props: { active: true } })
  await flushPromises()
  expect(w.find('a').exists()).toBe(false)
  expect(w.text()).toContain("router's connected-device list")
 })
})
