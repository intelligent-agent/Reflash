import { describe, it, expect, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import TheOptions from '../../src/components/TheOptions.vue'
// ?raw hands us the file's text at transform time. Reading it with fs and
// import.meta.url instead passed locally and threw ERR_INVALID_ARG_TYPE on the
// CI runner, because that URL resolves differently there - a test that depends
// on how it is run, which is not a property worth having.
import theOptionsSource from '../../src/components/TheOptions.vue?raw'

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

  // Guards the wiring, not the handler. The bug lived in the template's
  // @change argument, so a test that only calls onChange() directly stays
  // green through the entire outage - shallowMount stubs the radio away, so
  // the binding has to be asserted on the source itself.
  it('wires the rotation control to the screenRotation key', () => {
    const binding = theOptionsSource.match(
      /@change="onChange\('([^']+)',\s*options\.screenRotation\)"/)

    expect(binding).not.toBeNull()
    expect(binding[1]).toBe('screenRotation')
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
