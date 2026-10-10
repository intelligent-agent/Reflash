import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import axios from 'axios'
import TheSetup from '../../src/components/TheSetup.vue'
import source from '../../src/components/TheSetup.vue?raw'

vi.mock('axios', () => ({ default: { get: vi.fn(), post: vi.fn(), put: vi.fn() } }))

const base = {
  wifiMode: '', hotspotSSID: '', hotspotPSKSet: false, wifiCountry: '', timezone: '', loginPasswordSet: false,
  rootPasswordSet: false, enableSsh: false, screenRotation: 0, restoreBackup: '', restoreInclude: '', software: '',
  softwareAvailable: [], settingsSyncBusy: false, settingsSyncError: '',
}

let options
let dispatch

function mountSetup(over = {}) {
  options = { ...base, ...over }
  dispatch = vi.fn(async (action, payload) => {
    if (action === 'setOption') Object.assign(options, payload)
  })
  const w = mount(TheSetup, {
    global: {
      mocks: { $store: { getters: { get options() { return options } }, dispatch }, $waveui: { notify: vi.fn() } },
      stubs: { TheWifiSetup: true },
    },
  })
  return w
}

const sent = () => dispatch.mock.calls.filter(([a]) => a === 'setOption').map(([, p]) => p)

beforeEach(() => {
  axios.get.mockReset(); axios.post.mockReset(); axios.put.mockReset()
  axios.get.mockImplementation(async (url) => {
    if (url === '/api/file_backups') return { data: [{ name: 'voron.tar.gz', size: 2048, ok: true }, { name: 'a7.tar.gz', size: 900, ok: true }] }
    if (url.startsWith('/api/file_backups/files')) return { data: { supported: true, files: ['home/printer/printer_data/config/printer.cfg', 'home/printer/printer_data/config/moonraker.conf', 'home/printer/printer_data/database/x.db'] } }
    return { data: options }
  })
})

describe('Set up printer (#198)', () => {
  it('is a bar on the page that opens and closes a panel', async () => {
    const w = mountSetup()
    expect(w.find('.setup-panel').exists()).toBe(false)
    await w.find('.setup-toggle').trigger('click')
    expect(w.find('.setup-panel').exists()).toBe(true)
    expect(w.find('.setup-toggle').attributes('aria-expanded')).toBe('true')
    await w.find('.setup-foot .btn').trigger('click')
    expect(w.find('.setup-panel').exists()).toBe(false)
  })

  it('has the sections the design asks for, and Optional software only when there is some', async () => {
    const w = mountSetup()
    await w.find('.setup-toggle').trigger('click')
    expect(w.findAll('.setup-nav a').map((a) => a.text())).toEqual(['Network', 'Location', 'Accounts', 'Display', 'Config archive'])
    const w2 = mountSetup({ softwareAvailable: [{ name: 'led_effect', info: 'LED effects', installed: false }] })
    await w2.find('.setup-toggle').trigger('click')
    expect(w2.findAll('.setup-nav a').map((a) => a.text())).toContain('Optional software')
  })

  it('starts with everything at default and says so', async () => {
    const w = mountSetup()
    expect(w.find('.setup-meta').text()).toContain('All options at default')
  })

  it('starts an option as custom when the printer already has it that way', () => {
    const w = mountSetup({ wifiCountry: 'NO', screenRotation: 90, enableSsh: true, loginPasswordSet: true })
    expect(w.vm.isDef('country')).toBe(false)
    expect(w.vm.isDef('rotation')).toBe(false)
    expect(w.vm.isDef('ssh')).toBe(false)
    expect(w.vm.isDef('sshPassword')).toBe(false)
    expect(w.vm.isDef('timezone')).toBe(true)
    expect(w.find('.setup-meta').text()).toContain('4 changed from default')
  })

  it('touching an option leaves default and sends only that option', async () => {
    const w = mountSetup()
    await w.find('.setup-toggle').trigger('click')
    await w.find('#country').setValue('NO')
    expect(sent()).toEqual([{ wifiCountry: 'NO' }])
    expect(w.vm.isDef('country')).toBe(false)
    expect(w.find('.setup-meta').text()).toContain('1 changed from default')
  })

  it('ticking Default puts the option back, and Reset all puts back all of them', async () => {
    const w = mountSetup({ wifiCountry: 'NO', timezone: 'Europe/Oslo', screenRotation: 180 })
    await w.find('.setup-toggle').trigger('click')
    const box = w.find('#country').element.closest('.field').querySelector('.keep input')
    box.checked = true
    box.dispatchEvent(new Event('change'))
    await flushPromises()
    expect(sent()).toContainEqual({ wifiCountry: '' })
    dispatch.mockClear()
    await w.find('.setup-meta .link').trigger('click')
    const payloads = sent()
    expect(payloads).toContainEqual({ timezone: '' })
    expect(payloads).toContainEqual({ screenRotation: 0 })
    expect(payloads).toContainEqual({ wifiMode: '' })
    expect(w.find('.setup-meta').text()).toContain('All options at default')
  })

  it('refuses a short password here, before anything is sent', async () => {
    const w = mountSetup()
    await w.find('.setup-toggle').trigger('click')
    await w.find('#ssh-password').setValue('abc')
    await w.vm.setPassword('sshPassword')
    expect(sent()).toEqual([])
    expect(w.text()).toContain('too short')
  })

  it('sends a password once, and shows what the installed system said when it refused', async () => {
    const w = mountSetup()
    await w.find('.setup-toggle').trigger('click')
    await w.find('#root-password').setValue('rootpw1')
    dispatch.mockImplementation(async (action, payload) => {
      if (action === 'setOption') Object.assign(options, payload)
      if (action === 'getOptions') options.settingsSyncError = 'the password is a palindrome'
    })
    await w.vm.setPassword('rootPassword')
    expect(sent()).toEqual([{ rootPassword: 'rootpw1' }])
    expect(w.vm.problem.rootPassword).toBe('the password is a palindrome')
    expect(w.vm.isDef('rootPassword')).toBe(true)
    expect(w.vm.rootPw).toBe('')
  }, 10000)

  it('turns software on and off as a list', async () => {
    const w = mountSetup({ softwareAvailable: [{ name: 'led_effect', info: 'LED effects', installed: false }] })
    await w.find('.setup-toggle').trigger('click')
    w.vm.toggleSoftware('led_effect', true)
    expect(sent()).toEqual([{ software: 'led_effect' }])
    w.vm.toggleSoftware('led_effect', false)
    expect(sent().pop()).toEqual({ software: '' })
    expect(w.find('.setup-meta').text()).not.toContain('changed')
  })

  it('shows what the installed system says about the last change', () => {
    expect(mountSetup({ settingsSyncBusy: true }).find('.setup-meta').text()).toContain('Saving')
    expect(mountSetup({ settingsSyncError: 'no space' }).find('.setup-meta').text()).toContain('no space')
  })

  it('has no Default box on the Wi-Fi network, which it cannot take back', () => {
    expect(source).not.toMatch(/keep\('network'/)
  })
})

describe('Config archives in the panel', () => {
  async function open(over) {
    const w = mountSetup(over)
    await w.find('.setup-toggle').trigger('click')
    await flushPromises()
    return w
  }

  it('lists the archives with Download and Delete, and asks before deleting', async () => {
    const w = await open()
    expect(w.findAll('.archive .name').map((n) => n.text())).toEqual(['voron', 'a7'])
    axios.post.mockResolvedValue({ data: { status: 'OK' } })
    await w.findAll('.archive .act.danger')[0].trigger('click')
    expect(axios.post).not.toHaveBeenCalled()
    expect(w.text()).toContain('Delete voron?')
    await w.find('.archive .act.danger').trigger('click')
    await flushPromises()
    expect(axios.post).toHaveBeenCalledWith('/api/delete_image', { filename: 'voron.tar.gz' })
  })

  it('Keep takes the question back', async () => {
    const w = await open()
    await w.findAll('.archive .act.danger')[0].trigger('click')
    const keep = w.findAll('.archive .act').find((b) => b.text() === 'Keep')
    await keep.trigger('click')
    expect(w.text()).not.toContain('Delete voron?')
  })

  it('saves with a name, and with every file when none were left out', async () => {
    const w = await open()
    axios.post.mockResolvedValue({ data: { status: 'OK', name: 'mine.tar.gz' } })
    await w.find('#save-name').setValue('mine')
    await w.vm.save()
    expect(axios.post).toHaveBeenCalledWith('/api/file_backups', { name: 'mine', include: [] })
  })

  it('saves only the files chosen in the tree', async () => {
    const w = await open()
    await w.find('.disc').trigger('click')
    await flushPromises()
    w.vm.tree.save.selected = ['home/printer/printer_data/config/printer.cfg']
    axios.post.mockResolvedValue({ data: { status: 'OK', name: 'k.tar.gz' } })
    await w.vm.save()
    expect(axios.post).toHaveBeenCalledWith('/api/file_backups', { name: '', include: ['home/printer/printer_data/config/printer.cfg'] })
  })

  it('does not save with no files chosen', async () => {
    const w = await open()
    await w.find('.disc').trigger('click')
    await flushPromises()
    w.vm.tree.save.selected = []
    await w.vm.save()
    expect(axios.post).not.toHaveBeenCalled()
    expect(w.vm.problem.save).toContain('No files')
  })

  it('says why it cannot offer a tree, and then saves the whole config', async () => {
    axios.get.mockImplementation(async (url) => {
      if (url === '/api/file_backups') return { data: [] }
      if (url.startsWith('/api/file_backups/files')) return { data: { supported: false, files: [], reason: 'not supported: it cannot list' } }
      return { data: options }
    })
    const w = await open()
    await w.find('.disc').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('cannot list')
    axios.post.mockResolvedValue({ data: { status: 'OK', name: 'x.tar.gz' } })
    await w.vm.save()
    expect(axios.post).toHaveBeenCalledWith('/api/file_backups', { name: '', include: [] })
  })

  it('chooses the config to install and what of it, for the next install', async () => {
    const w = await open()
    await w.find('#config-install').setValue('voron.tar.gz')
    expect(sent()).toContainEqual({ restoreBackup: 'voron.tar.gz', restoreInclude: '' })
    expect(w.vm.isDef('config')).toBe(false)
    await w.findAll('.disc')[1].trigger('click')
    await flushPromises()
    w.vm.tree.install.selected = ['home/printer/printer_data/config/printer.cfg']
    w.vm.installSelectionChanged()
    expect(sent().pop()).toEqual({ restoreInclude: 'home/printer/printer_data/config/printer.cfg' })
  })

  it('installs the chosen config on its own, with the chosen files', async () => {
    const w = await open({ restoreBackup: 'voron.tar.gz' })
    axios.post.mockResolvedValue({ data: { status: 'OK' } })
    await w.vm.installNow()
    expect(axios.post).toHaveBeenCalledWith('/api/file_backups/restore', { name: 'voron.tar.gz', include: [] })
    expect(w.vm.message.install).toContain('Installed voron')
  })

  it('shows the installed system\'s refusal, not a success', async () => {
    const w = await open({ restoreBackup: 'voron.tar.gz' })
    axios.post.mockResolvedValue({ data: { status: 'ERROR', error: 'busy' } })
    await w.vm.installNow()
    expect(w.vm.problem.install).toBe('busy')
    expect(w.vm.message.install).toBe('')
  })

  it('uploads an archive from this computer and lists it', async () => {
    const w = await open()
    axios.put.mockResolvedValue({ data: {} })
    axios.post.mockResolvedValue({ data: { success: true } })
    const file = new File(['abc'], 'new.tar.gz')
    await w.vm.onUpload({ target: { files: [file], value: 'x' } })
    expect(axios.put).toHaveBeenCalledWith('/api/upload_start', expect.objectContaining({ filename: 'new.tar.gz', size: 3 }))
    expect(axios.post).toHaveBeenCalledWith('/api/upload_chunk', expect.anything(), expect.anything())
    expect(axios.put).toHaveBeenCalledWith('/api/upload_finish')
  })

  it('cancels a failed upload and says so', async () => {
    const w = await open()
    axios.put.mockResolvedValue({ data: {} })
    axios.post.mockRejectedValue(new Error('network'))
    await w.vm.onUpload({ target: { files: [new File(['abc'], 'new.tar.gz')], value: 'x' } })
    expect(axios.put).toHaveBeenCalledWith('/api/upload_cancel')
    expect(w.vm.problem.archives).toContain('upload failed')
  }, 20000)
})
