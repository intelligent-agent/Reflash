import { describe, it, expect, vi, beforeEach } from 'vitest'
import axios from 'axios'
import optionsModule from '../../src/store/modules/options'

vi.mock('axios', () => ({
  default: { get: vi.fn(), post: vi.fn() },
}))

describe('options store mutations', () => {
  it('getOptions replaces the options object', () => {
    const state = { options: { stale: true } }
    optionsModule.mutations.getOptions(state, { darkmode: true })
    expect(state.options).toEqual({ darkmode: true })
  })

  it('setOption merges a partial change into options', () => {
    const state = { options: { darkmode: true } }
    optionsModule.mutations.setOption(state, { screenRotation: 90 })
    expect(state.options).toEqual({ darkmode: true, screenRotation: 90 })
  })

  it('setProgress and setBandwidth update the progress sub-state', () => {
    const state = { progress: { progress: 0, bandwidth: 0 } }
    optionsModule.mutations.setProgress(state, { progress: 42 })
    optionsModule.mutations.setBandwidth(state, { bandwidth: 100 })
    expect(state.progress.progress).toBe(42)
    expect(state.progress.bandwidth).toBe(100)
  })

  it('setFlashMethod updates the selected method', () => {
    const state = { flash: { selectedMethod: 0 } }
    optionsModule.mutations.setFlashMethod(state, 2)
    expect(state.flash.selectedMethod).toBe(2)
  })
})

describe('options store actions', () => {
  beforeEach(() => vi.clearAllMocks())

  it('getOptions fetches from the API and commits the response', async () => {
    axios.get.mockResolvedValue({ data: { darkmode: false } })
    const commit = vi.fn()
    await optionsModule.actions.getOptions({ commit })
    expect(axios.get).toHaveBeenCalledWith('/api/get_options')
    expect(commit).toHaveBeenCalledWith('getOptions', { darkmode: false })
  })

  it('setOption commits the change and then persists it to the API', async () => {
    axios.post.mockResolvedValue({})
    const commit = vi.fn()
    await optionsModule.actions.setOption({ commit }, { enableSsh: true })
    expect(commit).toHaveBeenCalledWith('setOption', { enableSsh: true })
    // Not expect.anything(): that is what let #124 through this test. The
    // payload is the whole point.
    expect(axios.post).toHaveBeenCalledWith('/api/set_options', { enableSsh: true })
  })

  // #124. The page caches options once at load and nothing re-fetches, while
  // the server changes them underneath - connecting to WiFi sets SSID and PSK
  // server-side. Posting the whole cached object therefore wrote a stale empty
  // SSID back over working credentials on the next toggle of anything, and the
  // next flash produced an image with no network.
  it('posts only the changed key, never the whole cached object', async () => {
    axios.post.mockResolvedValue({})
    const commit = vi.fn()

    await optionsModule.actions.setOption({ commit }, { screenRotation: 90 })

    const [, body] = axios.post.mock.calls[0]
    expect(Object.keys(body)).toEqual(['screenRotation'])
    expect(body).not.toHaveProperty('SSID')
    expect(body).not.toHaveProperty('PSK')
  })
})
