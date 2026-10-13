import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { reactive } from 'vue'
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
  options = reactive({ ...base, ...over })
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
    if (url === '/api/get_status') return { data: { network: { wifi: { present: true } } } }
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



  it('starts an option as custom when the printer already has it that way', () => {
    const w = mountSetup({ wifiCountry: 'NO', screenRotation: 90, enableSsh: true, loginPasswordSet: true })
    expect(w.vm.isDef('country')).toBe(false)
    expect(w.vm.isDef('rotation')).toBe(false)
    expect(w.vm.isDef('ssh')).toBe(false)
    expect(w.vm.isDef('sshPassword')).toBe(false)
    expect(w.vm.isDef('timezone')).toBe(true)

  })

  it('touching an option leaves default and sends only that option', async () => {
    const w = mountSetup()
    await w.find('.setup-toggle').trigger('click')
    await w.find('#country').setValue('NO')
    expect(sent()).toEqual([{ wifiCountry: 'NO' }])
    expect(w.vm.isDef('country')).toBe(false)

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

  it('does not offer to install software without internet, and says why; off is always possible', async () => {
    const item = [{ name: 'led_effect', info: 'LED effects', installed: false }]
    const w = mountSetup({ softwareAvailable: item })
    await w.setProps({ internet: false })
    await w.find('.setup-toggle').trigger('click')
    const sw = () => w.find('#setup-software .w-switch, #setup-software w-switch-stub')
    expect(w.find('#setup-software').text()).toContain('needs an internet connection')
    expect(w.find('#setup-software').html()).toMatch(/disabled/)
    // Already on: it can be switched off, and no warning is shown.
    const on = mountSetup({ softwareAvailable: item, software: 'led_effect' })
    await on.setProps({ internet: false })
    await on.find('.setup-toggle').trigger('click')
    expect(on.find('#setup-software').text()).not.toContain('needs an internet connection')
    // With internet, nothing is said.
    const online = mountSetup({ softwareAvailable: item })
    await online.find('.setup-toggle').trigger('click')
    expect(online.find('#setup-software').text()).not.toContain('needs an internet connection')
  })

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

  // The switches are the same Wave switches as in the Options panel, not a
  // look-alike of ours: ours placed its hidden checkbox against the page, and
  // clicking it scrolled the page (#198).
  it('uses the Wave switch, as the Options panel does', () => {
    expect(source).toContain('<w-switch')
    expect(source).not.toMatch(/class="switch"/)
    expect(source).not.toMatch(/\.setup-wrap \.switch/)
  })

  // The part that matters for scrolling: the hidden checkbox Wave puts in a
  // switch is placed against the nearest positioned ancestor.
  it('makes each switch the box its hidden checkbox is placed in', () => {
    const rule = source.match(/\.setup-wrap \.w-switch \{([^}]*)\}/)
    expect(rule && rule[1]).toMatch(/position:\s*relative/)
  })

  // One of the three is always chosen, so the page never reads as having no mode.
  it('always has one Wi-Fi mode chosen, Automatic being the Default', async () => {
    const w = mountSetup()
    await w.find('.setup-toggle').trigger('click')
    await flushPromises()
    const chosen = () => w.findAll('input[name=wifiMode]').filter((r) => r.element.checked).map((r) => r.element.value)
    expect(chosen()).toEqual([''])
    expect(w.findAll('input[name=wifiMode]').map((r) => r.element.parentElement.textContent.trim()))
      .toEqual(['Automatic', 'Client only', 'Access point only'])

    await w.find('input[name=wifiMode][value=ap]').setValue(true)
    expect(sent().pop()).toEqual({ wifiMode: 'ap' })
    expect(w.vm.isDef('wifiMode')).toBe(false)
    expect(w.vm.effectiveMode).toBe('ap')

    // Choosing Automatic is going back to Default.
    await w.find('input[name=wifiMode][value=""]').setValue(true)
    expect(sent().pop()).toEqual({ wifiMode: '' })
    expect(w.vm.isDef('wifiMode')).toBe(true)
  })

  it('shows the mode the printer already has', () => {
    const w = mountSetup({ wifiMode: 'client' })
    expect(w.vm.isDef('wifiMode')).toBe(false)
    expect(w.vm.effectiveMode).toBe('client')
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
    await w.vm.openArchives()
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
    w.findComponent({ name: 'w-select' }).vm.$emit('update:modelValue', 'voron.tar.gz')
    await flushPromises()
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


describe('setup feedback and Wi-Fi modes', () => {
  it('shows both network and hotspot in Automatic and preserves input across modes', async () => {
    const w = mountSetup({ hotspotSSID: 'Voron AP' })
    document.body.appendChild(w.element)
    await w.find('.setup-toggle').trigger('click')
    await flushPromises()
    w.vm.hotspotPsk = 'pending password'
    const network = () => w.findComponent({ name: 'TheWifiSetup' })
    const hotspot = () => w.find('#hotspot-name')
    expect(network().isVisible()).toBe(true)
    expect(hotspot().isVisible()).toBe(true)
    await w.find('input[name=wifiMode][value=client]').setValue(true)
    expect(network().isVisible()).toBe(true)
    expect(hotspot().isVisible()).toBe(false)
    await w.find('input[name=wifiMode][value=ap]').setValue(true)
    expect(network().isVisible()).toBe(false)
    expect(hotspot().isVisible()).toBe(true)
    await w.find('input[name=wifiMode][value=""]').setValue(true)
    expect(network().isVisible()).toBe(true)
    expect(hotspot().isVisible()).toBe(true)
    expect(w.vm.hotspotPsk).toBe('pending password')
    expect(w.vm.hotspotName).toBe('Voron AP')
    w.unmount()
  })

  it('notifies success for the archive that was actually requested', async () => {
    const w = mountSetup({ restoreBackup: 'voron.tar.gz' })
    let resolve
    axios.post.mockImplementation(() => new Promise(r => { resolve = r }))
    const request = w.vm.installNow()
    expect(w.vm.busy.install).toBe(true)
    options.restoreBackup = 'other.tar.gz'
    resolve({ data: { status: 'OK' } })
    await request
    expect(w.vm.message.install).toBe('Installed voron.')
    expect(w.vm.$waveui.notify).toHaveBeenCalledWith('Installed voron.', 'success', 6000)
    expect(w.vm.busy.install).toBe(false)
  })

  it('notifies request errors and always clears the busy state', async () => {
    const w = mountSetup({ restoreBackup: 'voron.tar.gz' })
    axios.post.mockRejectedValue({ response: { data: 'busy: INSTALLING' } })
    await w.vm.installNow()
    expect(w.vm.problem.install).toBe('busy: INSTALLING')
    expect(w.vm.$waveui.notify).toHaveBeenCalledWith('busy: INSTALLING', 'error', 6000)
    expect(w.vm.busy.install).toBe(false)
  })

  it('reports errors in preparing the file selection before sending any request', async () => {
    const w = mountSetup({ restoreBackup: 'voron.tar.gz' })
    w.vm.includeOf = () => { throw new Error('Cannot prepare file selection') }
    await w.vm.installNow()
    expect(w.vm.problem.install).toBe('Cannot prepare file selection')
    expect(w.vm.busy.install).toBe(false)
    expect(axios.post).not.toHaveBeenCalled()
  })
})

describe('hostname', () => {
  it('shows the saved name, normalizes edits and resets to Default', async () => {
    const w = mountSetup({ hostname: 'voron' });
    await w.find('.setup-toggle').trigger('click');
    expect(w.find('#hostname').element.value).toBe('voron');
    await w.find('#hostname').setValue('A5-Printer');
    expect(sent()).toContainEqual({ hostname: 'a5-printer' });
    w.vm.keep('hostname', true);
    expect(sent()).toContainEqual({ hostname: '' });
  });
  it('refuses names that cannot become a local hostname', async () => {
    const w = mountSetup();
    for (const name of ['-printer', 'printer-', 'printer.local', 'two words', 'a'.repeat(64)]) {
      await w.vm.setHostname(name);
      expect(w.vm.problem.hostname).toContain('1 to 63');
    }
    expect(sent()).toEqual([]);
  });
});

describe('Wi-Fi credential guidance (#217)', () => {
 it('identifies client and fallback credentials, including defaults', async () => {
  const w = mountSetup()
  await w.find('.setup-toggle').trigger('click')
  await flushPromises()
  expect(w.text()).toContain('Configure both sets of credentials')
  expect(w.text()).toContain('Client network — router SSID and password')
  expect(w.text()).toContain('Fallback hotspot — printer AP name and password')
  expect(w.find('#hotspot-name').attributes('placeholder')).toBe('Recore')
  expect(w.find('#hotspot-psk').attributes('placeholder')).toBe('12345678')
 })
 it('rejects an empty custom hotspot name without applying it', async () => {
  const w = mountSetup()
  w.vm.hotspotName = '   '
  await w.vm.setHotspotName()
  expect(sent()).toEqual([])
  expect(w.vm.problem.hotspot).toContain('Enter a hotspot name')
 })
})

describe('simplified setup prototype (#216)', () => {
 it('shows effective hostname and timezone without Default checkboxes', async () => {
  const w = mountSetup()
  await w.find('.setup-toggle').trigger('click')
  expect(w.find('#hostname').element.value).toBe('recore')
  expect(w.find('#timezone').element.value).toBe('UTC')
  expect(w.findAll('.keep input')).toHaveLength(0)
 })
 it('hides absent Wi-Fi and discovers a newly connected adapter on Check again', async () => {
  let present = false
  axios.get.mockImplementation(async url => ({ data: url === '/api/get_status' ? { network: { wifi: { present } } } : [] }))
  const w = mountSetup({ wifiMode: 'client', hotspotSSID: 'My AP' })
  await w.find('.setup-toggle').trigger('click')
  await flushPromises()
  expect(w.find('.wifi-settings').exists()).toBe(false)
  expect(w.find('.wifi-check').text()).toContain('No Wi-Fi adapter detected')
  expect(w.find('.wifi-check .error').exists()).toBe(false)
  present = true
  await w.find('.wifi-check button').trigger('click')
  await flushPromises()
  expect(w.find('.wifi-settings').exists()).toBe(true)
  expect(options.wifiMode).toBe('client')
  expect(options.hotspotSSID).toBe('My AP')
  expect(sent()).toEqual([])
 })
 it('keeps setup limited to selecting an archive and opening the manager', async () => {
  const w = mountSetup()
  await w.find('.setup-toggle').trigger('click')
  await flushPromises()
  expect(w.find('#setup-config #save-name').exists()).toBe(false)
  expect(w.find('#setup-config .archives').exists()).toBe(false)
  expect(w.vm.archiveChoices[0]).toEqual({ label: 'Default — image configuration', value: '' })
  await w.find('#setup-config button').trigger('click')
  await flushPromises()
  expect(w.vm.managerOpen).toBe(true)
  expect(w.find('.archive-manager #save-name').exists()).toBe(true)
  expect(w.find('.archive-manager').text()).toContain('Create archive from current image')
 })
 it('selecting the image configuration clears an old archive and partial selection', async () => {
  const w = mountSetup({ restoreBackup: 'voron.tar.gz', restoreInclude: 'printer.cfg' })
  await w.vm.chooseConfig('')
  expect(sent()).toContainEqual({ restoreBackup: '', restoreInclude: '' })
 })
})
