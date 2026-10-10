import { describe, it, expect, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import axios from 'axios'
import TheOptions from '../../src/components/TheOptions.vue'
// ?raw hands us the file's text at transform time. Reading it with fs and
// import.meta.url instead passed locally and threw ERR_INVALID_ARG_TYPE on the
// CI runner, because that URL resolves differently there - a test that depends
// on how it is run, which is not a property worth having.
import theOptionsSource from '../../src/components/TheOptions.vue?raw'

vi.mock('axios', () => ({ default: { post: vi.fn().mockResolvedValue({}), get: vi.fn() } }))

// mapGetters/mapActions reach into a store the tests do not build, so the
// component gets a stub $store and assertions go against dispatch().
function mountOptions(options = {}) {
  const dispatch = vi.fn().mockResolvedValue()
  const wrapper = shallowMount(TheOptions, {
    props: { open: true },
    global: {
      mocks: {
        $store: {
          getters: { options: { darkmode: false, screenRotation: 0, ...options } },
          dispatch,
        },
      },
    },
  })
  return { wrapper, dispatch }
}

// Only the setOption dispatches; created() fires getOptions too.
const optionPayloads = (dispatch) =>
  dispatch.mock.calls.filter(([action]) => action === 'setOption').map(([, payload]) => payload)

describe('TheOptions', () => {
  // The option key has to be the one the server unmarshals, `screenRotation`.
  // `rotateScreen` is the name of a *different* thing - the /api/rotate_screen
  // endpoint - and Go silently drops unknown fields, so posting it left the
  // rotation at whatever it already was and still returned success.
  //
  // This was latent until #124: setOption used to post the whole cached options
  // object, and w-radios' v-model had already written the new value straight
  // into it, so the correct key rode along by accident. Posting only what
  // changed made the wrong key the only key.
  it('dispatches exactly the one key that changed', () => {
    const { wrapper, dispatch } = mountOptions()

    wrapper.vm.onChange('screenRotation', 270)

    expect(optionPayloads(dispatch)).toEqual([{ screenRotation: 270 }])
  })
})

// Reboot and Shut down sit next to each other and used to fire on one click.
// The two outcomes are not equally cheap: a stray Shut down on a headless board
// needs a person to go and power-cycle it (#163).
describe('TheOptions destructive actions', () => {
  it('does not reboot or shut down on the first click', () => {
    const { wrapper } = mountOptions()

    wrapper.vm.confirmAction('reboot')
    wrapper.vm.confirmAction('shutdown')

    expect(wrapper.emitted('reboot-board')).toBeUndefined()
    expect(wrapper.emitted('shutdown-board')).toBeUndefined()
  })

  it('acts on the second click of the same button', () => {
    const { wrapper } = mountOptions()

    wrapper.vm.confirmAction('reboot')
    wrapper.vm.confirmAction('reboot')

    expect(wrapper.emitted('reboot-board')).toHaveLength(1)
    expect(wrapper.emitted('shutdown-board')).toBeUndefined()
  })

  // Arming one has to disarm the other, or a click meant for Reboot would
  // confirm a Shut down that was armed moments earlier - the precise mistake
  // the confirmation exists to prevent.
  it('arming the other button cancels the pending one', () => {
    const { wrapper } = mountOptions()

    wrapper.vm.confirmAction('reboot')
    wrapper.vm.confirmAction('shutdown')
    wrapper.vm.confirmAction('shutdown')

    expect(wrapper.emitted('reboot-board')).toBeUndefined()
    expect(wrapper.emitted('shutdown-board')).toHaveLength(1)
  })

  it('disarms itself after a few seconds so a stale click does not act', () => {
    vi.useFakeTimers()
    try {
      const { wrapper } = mountOptions()

      wrapper.vm.confirmAction('reboot')
      vi.advanceTimersByTime(5000)
      wrapper.vm.confirmAction('reboot')

      expect(wrapper.emitted('reboot-board')).toBeUndefined()
    } finally {
      vi.useRealTimers()
    }
  })
})

// The drawer and its layout are stubs under shallowMount; rendering their
// default slots puts the panel's own text in front of the assertions.
function mountWithText(options = {}) {
  return shallowMount(TheOptions, {
    props: { open: true },
    global: {
      renderStubDefaultSlot: true,
      mocks: {
        $store: {
          getters: { options: { darkmode: false, screenRotation: 0, ...options } },
          dispatch: vi.fn().mockResolvedValue(),
        },
      },
    },
  })
}

// The password itself is set in its own window (TheLoginPassword, #186); the
// panel opens it and says whether one is set.
describe('TheOptions Advanced (#198)', () => {
  const source = theOptionsSource
  // What moved to Set up printer is not here any more.
  it('points to Set up printer for what moved there, and keeps no buttons for it', () => {
    expect(source).not.toMatch(/open-wifi|open-login-password|open-serial-number/)
    expect(source).toContain('under Set up printer')
  })

  it('keeps pre-releases and the serial number under Advanced', () => {
    const advanced = source.slice(source.indexOf('<details'), source.indexOf('</details>'))
    expect(advanced).toContain('showPrereleases')
    expect(advanced).toContain('serial-number')
    expect(source.slice(0, source.indexOf('<details'))).not.toContain('showPrereleases')
  })

  it('saves the serial number only when it is a changed number', async () => {
    const { wrapper } = mountOptions()
    wrapper.vm.serialSaved = '0482'
    wrapper.vm.serialInput = '0482'
    expect(wrapper.vm.serialDirty).toBe(false)
    wrapper.vm.serialInput = '0483'
    expect(wrapper.vm.serialDirty).toBe(true)
    wrapper.vm.serialInput = '04x3'
    expect(wrapper.vm.serialDirty).toBe(false)

    axios.post.mockResolvedValueOnce({ data: { status: 'OK' } })
    wrapper.vm.serialInput = '0483'
    await wrapper.vm.saveSerial()
    expect(axios.post).toHaveBeenCalledWith('/api/update_config', { snr: 483 })
    expect(wrapper.emitted('serial-saved')).toBeTruthy()
    expect(wrapper.vm.serialDirty).toBe(false)
  })

  it('says why a serial number was not saved', async () => {
    const { wrapper } = mountOptions()
    wrapper.vm.serialInput = '77'
    axios.post.mockResolvedValueOnce({ data: { status: 'ERROR', error: 'eMMC busy' } })
    await wrapper.vm.saveSerial()
    expect(wrapper.vm.serialError).toBe('eMMC busy')
    expect(wrapper.emitted('serial-saved')).toBeFalsy()
  })
})
