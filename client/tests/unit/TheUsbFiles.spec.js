import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount, flushPromises } from '@vue/test-utils'
import axios from 'axios'
import TheUsbFiles from '../../src/components/TheUsbFiles.vue'

vi.mock('axios', () => ({ default: { get: vi.fn(), put: vi.fn() } }))

// #188: the drive's files in one window - images, then config archives.
function mountOpen() {
  return shallowMount(TheUsbFiles, {
    props: { open: true },
    global: { renderStubDefaultSlot: true, mocks: { $waveui: { theme: 'dark' } } },
  })
}

describe('TheUsbFiles (#188)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    axios.get.mockReset()
    axios.put.mockReset()
    axios.get.mockImplementation((url) => Promise.resolve({
      data: url == '/api/get_status'
        ? { local_images: [
            { name: 'old.img.xz', size: 700 << 20, date: 100 },
            { name: 'new.img.xz', size: 720 << 20, date: 300 },
          ] }
        : [{ name: 'printer-config.tar.gz', size: 12000, date: 200, ok: true }],
    }))
    axios.put.mockImplementation((url, body) => Promise.resolve({
      data: url == '/api/check_file_integrity'
        ? { is_file_ok: body.filename != 'old.img.xz' }
        : { status: 'OK' },
    }))
  })

  it('lists images first, then config archives, each newest first', async () => {
    const w = mountOpen()
    await flushPromises()
    const [images, configs] = w.vm.sections
    expect(images.files.map((f) => f.name)).toEqual(['new.img.xz', 'old.img.xz'])
    expect(configs.files.map((f) => f.name)).toEqual(['printer-config.tar.gz'])
  })

  it('checks each image, and takes the archives\' check from the list', async () => {
    const w = mountOpen()
    await flushPromises()
    const ok = Object.fromEntries(w.vm.files.map((f) => [f.name, f.ok]))
    expect(ok).toEqual({ 'new.img.xz': true, 'old.img.xz': false, 'printer-config.tar.gz': true })
  })

  // Twice to delete, as Delete on the main screen was (#153).
  it('deletes on the second press, and tells the page', async () => {
    const w = mountOpen()
    await flushPromises()
    const f = w.vm.files.find((x) => x.name == 'printer-config.tar.gz')
    await w.vm.remove(f)
    expect(axios.put).not.toHaveBeenCalledWith('/api/delete_image', expect.anything())
    await w.vm.remove(f)
    expect(axios.put).toHaveBeenCalledWith('/api/delete_image', { filename: 'printer-config.tar.gz' })
    expect(w.vm.files.map((x) => x.name)).not.toContain('printer-config.tar.gz')
    expect(w.emitted('changed')).toBeTruthy()
  })

  it('forgets the first press after a few seconds', async () => {
    const w = mountOpen()
    await flushPromises()
    await w.vm.remove(w.vm.files[0])
    vi.advanceTimersByTime(5000)
    expect(w.vm.confirm).toBe('')
  })

  it('says why when the server refuses', async () => {
    const w = mountOpen()
    await flushPromises()
    axios.put.mockResolvedValue({ data: { status: 'ERROR', error: 'busy: UPLOADING' } })
    const f = w.vm.files[0]
    await w.vm.remove(f)
    await w.vm.remove(f)
    expect(w.vm.message).toContain('busy: UPLOADING')
    expect(w.vm.files).toContain(f)
    expect(w.emitted('changed')).toBeFalsy()
  })
})
