import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import axios from 'axios'
import TheLoginPassword from '../../src/components/TheLoginPassword.vue'

vi.mock('axios', () => ({ default: { post: vi.fn(), get: vi.fn() } }))

function mountDialog(options = {}) {
  const dispatch = vi.fn().mockResolvedValue()
  const wrapper = shallowMount(TheLoginPassword, {
    props: { open: true },
    global: {
      renderStubDefaultSlot: true,
      mocks: {
        $store: { getters: { options: { loginPasswordSet: false, ...options } }, dispatch },
        $waveui: { theme: 'dark' },
      },
    },
  })
  return { wrapper, dispatch }
}

describe('TheLoginPassword (#182, #186)', () => {
  beforeEach(() => axios.post.mockReset())

  // Never through setOption: that keeps what it posts in the page's store.
  it('posts the password straight to the server, empties the fields and rereads', async () => {
    axios.post.mockResolvedValue({ data: { status: 'OK' } })
    const { wrapper, dispatch } = mountDialog()
    wrapper.vm.password = 'correct horse'
    wrapper.vm.passwordAgain = 'correct horse'
    await wrapper.vm.setPassword(wrapper.vm.password)

    expect(axios.post).toHaveBeenCalledWith('/api/set_options', { loginPassword: 'correct horse' })
    expect(wrapper.vm.password).toBe('')
    expect(wrapper.vm.passwordAgain).toBe('')
    expect(dispatch.mock.calls.filter(([a]) => a === 'setOption')).toEqual([])
    expect(dispatch).toHaveBeenCalledWith('getOptions')
  })

  // The window stays open and says how it went.
  it('stays open and says the password was set, or set to the default', async () => {
    axios.post.mockResolvedValue({ data: { status: 'OK' } })
    const { wrapper } = mountDialog()
    await wrapper.vm.setPassword('correct horse')
    expect(wrapper.vm.dialog.show).toBe(true)
    expect(wrapper.vm.messageOk).toBe(true)
    expect(wrapper.vm.message).toContain('Password set for debian')
    await wrapper.vm.setPassword(wrapper.vm.defaultPassword)
    expect(wrapper.vm.message).toContain('the default again: temppwd')
  })

  // "Clear" was never a state of the system: the account always has a
  // password, and the default is the factory one.
  it('Set default sends temppwd, not an empty password', async () => {
    axios.post.mockResolvedValue({ data: { status: 'OK' } })
    const { wrapper } = mountDialog()
    await wrapper.vm.setPassword(wrapper.vm.defaultPassword)
    expect(axios.post).toHaveBeenCalledWith('/api/set_options', { loginPassword: 'temppwd' })
  })

  it('names the user and the default, so nobody has to guess which account', () => {
    const { wrapper } = mountDialog()
    expect(wrapper.text()).toContain('debian')
    expect(wrapper.text()).toContain('temppwd')
    expect(wrapper.text()).toContain('Set default')
    expect(wrapper.text()).not.toContain('Clear')
  })

  // The image's own rules decide: a password it refuses is reported by the
  // installed system's answer, not by a message that was already on the page.
  it('says what the installed system refused', async () => {
    axios.post.mockResolvedValue({ data: { status: 'OK' } })
    const { wrapper } = mountDialog({ settingsSyncBusy: false, settingsSyncError: 'the password is too short: at least 6 characters' })
    await wrapper.vm.setPassword('abc')
    expect(wrapper.vm.messageOk).toBe(false)
    expect(wrapper.vm.message).toContain('at least 6 characters')
  })

  it("shows the server's reason when it fails", async () => {
    axios.post.mockResolvedValue({ data: { status: 'ERROR', error: 'the USB drive is read-only' } })
    const { wrapper } = mountDialog()
    await wrapper.vm.setPassword('correct horse')
    expect(wrapper.vm.messageOk).toBe(false)
    expect(wrapper.vm.message).toContain('the USB drive is read-only')
  })

  it('says so when the two fields differ', async () => {
    const { wrapper } = mountDialog()
    wrapper.vm.password = 'correct horse'
    wrapper.vm.passwordAgain = 'correct hose'
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('The two passwords differ.')
  })

  it('forgets what was typed when it closes', async () => {
    const { wrapper } = mountDialog()
    wrapper.vm.password = 'correct horse'
    wrapper.vm.dialog.show = false
    await wrapper.vm.$nextTick()
    expect(wrapper.vm.password).toBe('')
    expect(wrapper.emitted('close')).toBeTruthy()
  })
})
