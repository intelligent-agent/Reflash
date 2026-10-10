import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { reactive, nextTick } from 'vue'
import FlashSelector from '../../src/components/FlashSelector.vue'

function setup(magic = false, method = 0) {
  const options = reactive({ magicmode: magic })
  const flash = reactive({ selectedMethod: method })
  const dispatch = vi.fn((action, value) => { flash.selectedMethod = value })
  const w = mount(FlashSelector, { global: { mocks: { $store: { getters: { options, flash }, dispatch } } } })
  return { w, options, flash, dispatch }
}

describe('backup selector in magic mode', () => {
  it('hides the dropdown and clears an existing backup selection on entry', async () => {
    const { w, options, flash } = setup(false, 1)
    expect(w.findComponent({ name: 'w-select' }).exists()).toBe(true)
    options.magicmode = true
    await nextTick()
    expect(w.findComponent({ name: 'w-select' }).exists()).toBe(false)
    expect(w.text()).toBe('Install')
    expect(flash.selectedMethod).toBe(0)
    options.magicmode = false
    await nextTick()
    expect(w.findComponent({ name: 'w-select' }).exists()).toBe(true)
  })

  it('cannot retain a backup selection when loaded in magic mode', () => {
    const { w, flash } = setup(true, 1)
    expect(flash.selectedMethod).toBe(0)
    w.vm.setSelection(1)
    expect(flash.selectedMethod).toBe(0)
  })

  it('allows backup in USB mode without dispatching during render', () => {
    const { w, flash, dispatch } = setup()
    expect(dispatch).not.toHaveBeenCalled()
    w.vm.setSelection(1)
    expect(flash.selectedMethod).toBe(1)
  })
})
