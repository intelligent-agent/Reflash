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
    expect(axios.post).toHaveBeenCalledWith('/api/set_options', expect.anything())
  })
})
