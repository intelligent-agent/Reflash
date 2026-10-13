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
    if (url === '/api/installed_settings') return { data: { supported: true, keys: ['HOSTNAME'], current: { HOSTNAME: 'installed-printer' } } }
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
    expect(w.vm.isDef('rotation')).toBe(false)
    expect(w.vm.isDef('ssh')).toBe(false)
    expect(w.vm.isDef('sshPassword')).toBe(false)
    expect(w.vm.isDef('timezone')).toBe(true)

  })

  it('touching an option leaves default and sends only that option', async () => {
    const w = mountSetup()
    await w.find('.setup-toggle').trigger('click')
    await w.find('#country').setValue('NO')
    expect(axios.post).toHaveBeenCalledWith('/api/set_options?apply=false', { wifiCountry: 'NO' })

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
    expect(w.vm.rootPw).toBe('rootpw1')
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
    expect(w.find('#manual-wifi-mode').element.value).toBe('auto')
    await w.find('#manual-wifi-mode').setValue('ap')
    await flushPromises()
    expect(axios.post).toHaveBeenCalledWith('/api/wifi_setup', expect.objectContaining({ source: 'manual', action: 'stage', settings: expect.objectContaining({ WIFI_MODE: 'ap' }) }), expect.anything())
    expect(w.vm.effectiveMode).toBe('ap')
  })

  it('shows the mode the printer already has', () => {
    const w = mountSetup({ wifiMode: 'client' })
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
    expect(axios.post).toHaveBeenCalledWith('/api/file_backups', { name: 'mine', include: [] }, expect.anything())
  })

  it('saves only the files chosen in the tree', async () => {
    const w = await open()
    await w.find('.disc').trigger('click')
    await flushPromises()
    w.vm.tree.save.selected = ['home/printer/printer_data/config/printer.cfg']
    axios.post.mockResolvedValue({ data: { status: 'OK', name: 'k.tar.gz' } })
    await w.vm.save()
    expect(axios.post).toHaveBeenCalledWith('/api/file_backups', expect.objectContaining({ include: ['home/printer/printer_data/config/printer.cfg'] }), expect.anything())
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
    expect(axios.post).toHaveBeenCalledWith('/api/file_backups', expect.objectContaining({ include: [] }), expect.anything())
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
    const network = () => w.find('#client-ssid')
    const hotspot = () => w.find('#hotspot-name')
    expect(network().isVisible()).toBe(true)
    expect(hotspot().isVisible()).toBe(true)
    await w.find('#manual-wifi-mode').setValue('client'); await flushPromises()
    expect(network().isVisible()).toBe(true)
    expect(hotspot().isVisible()).toBe(false)
    await w.find('#manual-wifi-mode').setValue('ap'); await flushPromises()
    expect(network().isVisible()).toBe(false)
    expect(hotspot().isVisible()).toBe(true)
    await w.find('#manual-wifi-mode').setValue('auto'); await flushPromises()
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
    expect(axios.post).toHaveBeenCalledWith('/api/set_options?apply=false', { hostname: 'a5-printer' });
    w.vm.keep('hostname', true);
    expect(sent()).toContainEqual({ hostname: '' });
  });
  it('refuses names that cannot become a local hostname', async () => {
    const w = mountSetup();
    for (const name of ['-printer', 'printer-', 'printer.local', 'two words', 'a'.repeat(64)]) {
      await w.vm.setHostname(name);
      expect(w.vm.problem.hostname).toContain('1 to 63');
    }
    expect(axios.post).not.toHaveBeenCalled();
  });
});

describe('Wi-Fi credential guidance (#217)', () => {
 it('identifies client and fallback credentials, including defaults', async () => {
  const w = mountSetup()
  await w.find('.setup-toggle').trigger('click')
  await flushPromises()
  expect(w.text()).toContain('Configure both the client network and fallback hotspot')
  expect(w.text()).toContain('Client SSID')
  expect(w.text()).toContain('Hotspot name')
  expect(w.find('#hotspot-name').element.value).toBe('Recore')
  expect(w.find('#hotspot-psk').element.value).toBe('12345678')
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

describe('hostname Get/Set on the installed image', () => {
 it('Get reads the image hostname and retains it without applying anything to that image', async () => {
  const w = mountSetup({ hostname: 'next-image' })
  axios.post.mockResolvedValue({ data: {} })
  await w.vm.getInstalledHostname()
  expect(w.vm.hostnameDraft).toBe('installed-printer')
  expect(axios.get).toHaveBeenCalledWith('/api/installed_settings', expect.anything())
  expect(axios.post).toHaveBeenCalledWith('/api/set_options?apply=false', { hostname: 'installed-printer' })
  expect(axios.post.mock.calls.some(([url]) => url === '/api/installed_settings')).toBe(false)
 })
 it('Set applies exactly the selected hostname through the installer API', async () => {
  const w = mountSetup()
  w.vm.hostnameDraft = 'a8-printer'
  axios.post.mockResolvedValue({ data: { status: 'OK' } })
  await w.vm.applyHostname()
  expect(axios.post).toHaveBeenCalledWith('/api/installed_settings', { HOSTNAME: 'a8-printer' }, expect.anything())
  expect(w.vm.hostnameMessage).toContain('next boot')
 })
 it('allows Get but disables Set for a legacy image', async () => {
  axios.get.mockImplementation(async () => ({ data: { supported: false, current: { HOSTNAME: 'legacy-a8' }, reason: 'This installed image cannot apply settings.' } }))
  const w = mountSetup()
  await w.vm.checkHostnameSupport()
  expect(w.vm.hostnameCanRead).toBe(true)
  expect(w.vm.hostnameCanSet).toBe(false)
  expect(w.vm.hostnameReason).toContain('cannot apply')
 })
 it('reports an installer refusal instead of success', async () => {
  const w = mountSetup()
  axios.post.mockImplementation(async url => ({ data: url === '/api/installed_settings' ? { status: 'ERROR', error: 'hostname refused' } : {} }))
  await w.vm.applyHostname()
  expect(w.vm.problem.hostname).toBe('Error: hostname refused')
  expect(w.vm.hostnameMessage).not.toContain('Set recore on')
 })
})

describe('factory password suggestions', () => {
 it('prefills documented factory passwords without applying them', async () => {
  const w = mountSetup()
  await w.find('.setup-toggle').trigger('click')
  expect(w.find('#ssh-password').element.value).toBe('temppwd')
  expect(w.find('#root-password').element.value).toBe('temppwd')
  expect(sent()).toEqual([])
  expect(axios.post).not.toHaveBeenCalled()
 })
 it('leaves fields empty when replacement passwords are recorded', () => {
  const w = mountSetup({ loginPasswordSet: true, rootPasswordSet: true })
  expect(w.vm.sshPw).toBe('')
  expect(w.vm.rootPw).toBe('')
 })
})

describe('eMMC location, display, and network controls', () => {
 const current = { SSH_ENABLED: 'true', HOSTNAME: 'recore', WIFI_COUNTRY: 'NO', TIMEZONE: 'Europe/Oslo', SCREEN_ROTATION: '90', HOTSPOT_SSID: 'Printer AP', WIFI_SSID: 'Home network' }
 function stubInstalled() {
  axios.get.mockImplementation(async url => {
   if (url === '/api/installed_settings') return { data: { supported: true, keys: Object.keys(current), current } }
   if (url === '/api/get_status') return { data: { network: { wifi: { present: true } } } }
   if (url === '/api/file_backups') return { data: [] }
   return { data: options }
  })
  axios.post.mockResolvedValue({ data: { status: 'OK' } })
 }
 it.each([
  ['ssh', 'enableSsh', true], ['country', 'wifiCountry', 'NO'], ['timezone', 'timezone', 'Europe/Oslo'],
  ['rotation', 'screenRotation', 90], ['hotspot', 'hotspotSSID', 'Printer AP'], ['client', 'SSID', 'Home network'],
 ])('Get %s reads eMMC and stages it without applying settings', async (name, option, value) => {
  const w = mountSetup(); stubInstalled()
  await w.vm.getInstalledSetting(name)
  expect(w.vm.settingDraft[name]).toBe(value)
  expect(axios.post).toHaveBeenCalledWith('/api/set_options?apply=false', { [option]: value })
  expect(axios.post.mock.calls.some(([url]) => url === '/api/installed_settings')).toBe(false)
 })
 it.each([
  ['ssh', 'SSH_ENABLED', true], ['country', 'WIFI_COUNTRY', 'NO'], ['timezone', 'TIMEZONE', 'Europe/Oslo'],
  ['rotation', 'SCREEN_ROTATION', 90], ['hotspot', 'HOTSPOT_SSID', 'Printer AP'], ['client', 'WIFI_SSID', 'Home network'],
 ])('Set %s applies only its own key', async (name, key, value) => {
  const w = mountSetup(); stubInstalled()
  await w.vm.readInstalledSettings()
  w.vm.settingDraft[name] = value
  if (name === 'hotspot') w.vm.hotspotName = value
  await w.vm.applyInstalledSetting(name)
  expect(axios.post).toHaveBeenCalledWith('/api/installed_settings', { [key]: String(value) }, expect.anything())
  expect(w.vm.settingMessage[name]).toContain('Applied to the installed image')
 })
 it('shows the Wi-Fi source selector and manager only with an adapter', async () => {
  const w = mountSetup(); stubInstalled()
  await w.find('.setup-toggle').trigger('click'); await flushPromises()
  expect(w.find('#wifi-source').exists()).toBe(true)
  expect(w.find('[aria-label="AP name on eMMC"]').exists()).toBe(false)
  w.vm.wifiPresent = false; await w.vm.$nextTick()
  expect(w.find('#wifi-source').exists()).toBe(false)
 })
 it('explains unsupported operations and does not submit them', async () => {
  const w = mountSetup()
  w.vm.installedSettings = { supported: true, keys: ['SCREEN_ROTATION'], current: { SCREEN_ROTATION: '90' } }
  expect(w.vm.canGetSetting('country')).toBe(false)
  expect(w.vm.canSetSetting('country')).toBe(false)
  await w.vm.applyInstalledSetting('country')
  expect(axios.post).not.toHaveBeenCalled()
  expect(w.vm.settingError.country).toContain('cannot be applied')
 })
 it('does not report an installer error as success', async () => {
  const w = mountSetup(); stubInstalled()
  await w.vm.readInstalledSettings()
  axios.post.mockImplementation(async url => ({ data: url === '/api/installed_settings' ? { status: 'ERROR', error: 'setting refused' } : {} }))
  await w.vm.applyInstalledSetting('rotation')
  expect(w.vm.settingError.rotation).toContain('setting refused')
  expect(w.vm.settingMessage.rotation).not.toContain('Applied')
 })
})

describe('options-save conflicts', () => {
 it('waits for a brief USB save then retries Set without leaving a stale error', async () => {
  vi.useFakeTimers()
  try {
   const w = mountSetup()
   w.vm.installedSettings = { supported: true, keys: ['TIMEZONE'], current: { TIMEZONE: 'Europe/Oslo' } }
   w.vm.settingDraft.timezone = 'Europe/Moscow'
   let posts = 0, polls = 0
   axios.post.mockImplementation(async url => {
    if (url === '/api/installed_settings' && ++posts === 1) throw { response: { status: 409, data: 'busy: SAVING\n' } }
    return { data: { status: 'OK' } }
   })
   axios.get.mockImplementation(async url => ({ data: url === '/api/get_progress' ? { state: ++polls === 1 ? 'SAVING' : 'IDLE' } : options }))
   const work = w.vm.applyInstalledSetting('timezone')
   await vi.advanceTimersByTimeAsync(1000)
   await work
   expect(posts).toBe(2)
   expect(w.vm.settingBusy.timezone).toBe('')
   expect(w.vm.settingError.timezone).toBe('')
   expect(w.vm.settingMessage.timezone).toContain('Applied')
  } finally { vi.useRealTimers() }
 })
 it('does not replay a request if an image flash started during the wait', async () => {
  vi.useFakeTimers()
  try {
   const w = mountSetup()
   axios.post.mockRejectedValue({ response: { status: 409, data: 'busy: SAVING' } })
   axios.get.mockResolvedValue({ data: { state: 'INSTALLING' } })
   const work = expect(w.vm.installedSettingsRequest({ TIMEZONE: 'Europe/Moscow' })).rejects.toThrow('INSTALLING')
   await vi.advanceTimersByTimeAsync(300)
   await work
   expect(axios.post).toHaveBeenCalledTimes(1)
  } finally { vi.useRealTimers() }
 })
 it('bounds the wait for a save that never finishes', async () => {
  vi.useFakeTimers()
  try {
   const w = mountSetup()
   axios.get.mockImplementation(async url => {
    if (url === '/api/get_progress') return { data: { state: 'SAVING' } }
    throw { response: { status: 409, data: 'busy: SAVING' } }
   })
   const work = expect(w.vm.installedSettingsRequest()).rejects.toThrow('still being saved')
   await vi.advanceTimersByTimeAsync(21000)
   await work
  } finally { vi.useRealTimers() }
 })
})

describe('retained password inputs', () => {
 it.each(['sshPassword', 'rootPassword'])('keeps %s after applying it', async kind => {
  const w = mountSetup()
  const field = kind === 'rootPassword' ? 'rootPw' : 'sshPw'
  w.vm[field] = 'edited-password'
  w.vm.settle = async () => ''
  await w.vm.setPassword(kind)
  expect(w.vm[field]).toBe('edited-password')
 })
})

describe('archive manager refinements', () => {
 it('prefills an editable name and preserves a name already entered', async () => {
  const w = mountSetup()
  await w.vm.openArchives()
  expect(w.vm.saveName).toMatch(/^installed-printer-config-\d{8}-\d{6}$/)
  w.vm.saveName = 'my-custom-backup'
  await w.vm.openArchives()
  expect(w.vm.saveName).toBe('my-custom-backup')
 })
 it('omits firmware README even when all visible files are selected and Choose files was not opened', async () => {
  const readme = 'home/printer/printer_data/config/firmware/README'
  const firmware = 'home/printer/printer_data/config/firmware/klipper.config'
  const files = [readme, firmware, 'home/printer/printer_data/config/printer.cfg']
  const w = mountSetup()
  axios.get.mockImplementation(async url => ({ data: url === '/api/file_backups/files' ? { supported: true, files } : [] }))
  axios.post.mockResolvedValue({ data: { status: 'OK', name: 'backup.tar.gz' } })
  await w.vm.save()
  expect(w.vm.tree.save.files).not.toContain(readme)
  expect(w.vm.tree.save.files).toContain(firmware)
  const payload = axios.post.mock.calls.find(([url]) => url === '/api/file_backups')[1]
  expect(payload.include.length).toBeGreaterThan(0)
  expect(payload.include.some(path => readme === path || readme.startsWith(path + '/'))).toBe(false)
  expect(payload.include).toContain(firmware)
  expect(w.vm.message.save).toContain('Created backup.tar.gz')
  expect(w.vm.$waveui.notify).not.toHaveBeenCalled()
 })
 it('shows a valid local date and a neutral fallback for missing timestamps', () => {
  const w = mountSetup()
  expect(w.vm.archiveDateTime(1791900000)).toMatch(/^2026-10-13T/)
  expect(w.vm.archiveDate(1791900000)).not.toBe('—')
  expect(w.vm.archiveDate(undefined)).toBe('—')
 })
})

describe('Wi-Fi mode on eMMC', () => {
 it('Get preserves Automatic as an effective value and stages it without applying it', async () => {
  const w = mountSetup()
  axios.get.mockResolvedValue({ data: { supported: true, keys: ['WIFI_MODE'], current: { WIFI_MODE: 'auto' } } })
  axios.post.mockResolvedValue({ data: {} })
  await w.vm.getInstalledSetting('wifiMode')
  expect(w.vm.settingDraft.wifiMode).toBe('auto')
  expect(axios.post).toHaveBeenCalledWith('/api/set_options?apply=false', { wifiMode: '' })
  expect(axios.post.mock.calls.some(([url]) => url === '/api/installed_settings')).toBe(false)
 })
 it('Set applies only the image Wi-Fi mode and does not switch the running radio', async () => {
  const w = mountSetup()
  w.vm.installedSettings = { supported: true, keys: ['WIFI_MODE'], current: {} }
  w.vm.settingDraft.wifiMode = 'client'
  axios.post.mockResolvedValue({ data: { status: 'OK' } })
  await w.vm.applyInstalledSetting('wifiMode')
  expect(axios.post).toHaveBeenCalledWith('/api/installed_settings', { WIFI_MODE: 'client' }, expect.anything())
  expect(axios.post.mock.calls.some(([url]) => url.startsWith('/api/wifi_'))).toBe(false)
 })
})

describe('editable AP factory password', () => {
 it('prefills 12345678, masks it, and allows Show without applying it', async () => {
  const w = mountSetup()
  await w.find('.setup-toggle').trigger('click'); await flushPromises()
  expect(w.find('#hotspot-psk').element.value).toBe('12345678')
  expect(w.find('#hotspot-psk').attributes('type')).toBe('password')
  const show = w.find('#hotspot-psk').element.parentElement.querySelector('button')
  show.click(); await w.vm.$nextTick()
  expect(w.find('#hotspot-psk').attributes('type')).toBe('text')
  expect(axios.post).not.toHaveBeenCalled()
 })
 it('keeps the editable password after Set', async () => {
  const w = mountSetup()
  w.vm.hotspotPsk = 'edited-ap-password'
  w.vm.settle = async () => ''
  await w.vm.setHotspotPassword()
  expect(w.vm.hotspotPsk).toBe('edited-ap-password')
 })
})


describe('separate next-image and live Wi-Fi setup', () => {
 it('copies an eMMC setup without a live connection request', async () => {
  const w = mountSetup(); await w.find('.setup-toggle').trigger('click'); await flushPromises()
  axios.post.mockClear()
  await w.find('#wifi-source').setValue('emmc'); await flushPromises()
  expect(axios.post).toHaveBeenCalledWith('/api/wifi_setup', {source:'emmc', action:'stage', settings:undefined}, expect.anything())
  expect(axios.post.mock.calls.some(([url]) => url.includes('start_connect') || url === '/api/installed_settings')).toBe(false)
 })
 it('keeps live management closed until requested', async () => {
  const w = mountSetup(); await w.find('.setup-toggle').trigger('click'); await flushPromises()
  expect(w.vm.wifiManagerOpen).toBe(false)
  await w.findAll('button').find(b => b.text() === 'Manage Wi-Fi').trigger('click')
  expect(w.vm.wifiManagerOpen).toBe(true)
  expect(axios.post).not.toHaveBeenCalled()
 })
 it('stages manual credentials without testing the network', async () => {
  const w = mountSetup(); await w.find('.setup-toggle').trigger('click'); await flushPromises()
  w.vm.settingDraft.client = 'Offline network'; w.vm.clientPsk = 'manualpassword'
  await w.vm.saveManualWifi()
  expect(axios.post).toHaveBeenCalledWith('/api/wifi_setup', expect.objectContaining({source:'manual',action:'stage',settings:expect.objectContaining({WIFI_SSID:'Offline network',WIFI_PSK:'manualpassword'})}),expect.anything())
  expect(axios.post.mock.calls.some(([url]) => url.includes('start_connect'))).toBe(false)
 })
})
