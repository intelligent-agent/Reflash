import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount, flushPromises } from '@vue/test-utils'
import axios from 'axios'
import TheInstalledSettings from '../../src/components/TheInstalledSettings.vue'

vi.mock('axios', () => ({ default: { get: vi.fn(), post: vi.fn() } }))

const installed = {
  supported: true,
  keys: ['LOGIN_PASSWORD', 'SCREEN_ROTATION', 'SSH_ENABLED', 'WIFI_PSK', 'WIFI_SSID'],
  current: { SSH_ENABLED: 'true', SCREEN_ROTATION: '90', WIFI_SSID: 'home' },
}

async function mountOpen(data = installed) {
  axios.get.mockResolvedValue({ data })
  const wrapper = shallowMount(TheInstalledSettings, {
    props: { open: true },
    global: { renderStubDefaultSlot: true },
  })
  await flushPromises()
  return wrapper
}

describe('TheInstalledSettings (#173)', () => {
  beforeEach(() => {
    axios.get.mockReset()
    axios.post.mockReset()
  })

  it('starts from what the installed system has, so nothing is a change yet', async () => {
    const w = await mountOpen()
    expect(axios.get).toHaveBeenCalledWith('/api/installed_settings')
    expect(w.vm.form.ssid).toBe('home')
    expect(w.vm.form.rotation).toBe('90')
    expect(w.vm.form.ssh).toBe(true)
    expect(w.vm.changes).toEqual({})
  })

  // A forgotten password must not reset the Wi-Fi or the rotation.
  it('sends only what was changed', async () => {
    const w = await mountOpen()
    w.vm.form.password = 'correct horse'
    w.vm.form.passwordAgain = 'correct horse'
    axios.post.mockResolvedValue({ data: { status: 'OK' } })
    await w.vm.apply()
    expect(axios.post).toHaveBeenCalledWith('/api/installed_settings', { LOGIN_PASSWORD: 'correct horse' })
    expect(w.vm.form.password).toBe('')
  })

  it('does not send a password typed differently twice', async () => {
    const w = await mountOpen()
    w.vm.form.password = 'correct horse'
    w.vm.form.passwordAgain = 'correct hose'
    expect(w.vm.passwordsDiffer).toBe(true)
    expect(w.vm.changes).toEqual({})
  })

  it('shows the image\'s reason when it cannot be changed from Reflash', async () => {
    const w = await mountOpen({ supported: false, reason: 'too old: reinstall it', keys: [], current: {} })
    expect(w.text()).toContain('too old: reinstall it')
  })

  it('shows the refusal it got back', async () => {
    const w = await mountOpen()
    w.vm.form.ssid = 'other'
    axios.post.mockResolvedValue({ data: { status: 'ERROR', error: 'the password is too short' } })
    await w.vm.apply()
    expect(w.vm.message).toBe('the password is too short')
  })
})
