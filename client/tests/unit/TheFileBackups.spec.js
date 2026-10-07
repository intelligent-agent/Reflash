import { describe, it, expect, vi, beforeEach } from 'vitest'
import { shallowMount, flushPromises } from '@vue/test-utils'
import axios from 'axios'
import TheFileBackups from '../../src/components/TheFileBackups.vue'

vi.mock('axios', () => ({ default: { get: vi.fn(), post: vi.fn() } }))

async function mountOpen(options = {}) {
  const dispatch = vi.fn().mockResolvedValue()
  const w = shallowMount(TheFileBackups, {
    props: { open: true },
    global: {
      renderStubDefaultSlot: true,
      mocks: { $store: { getters: { options: { restoreBackup: '', ...options } }, dispatch } },
    },
  })
  await flushPromises()
  return { w, dispatch }
}

describe('TheFileBackups (#175)', () => {
  beforeEach(() => {
    axios.get.mockReset().mockResolvedValue({ data: [{ name: 'rebuild-files-1.tar.gz', size: 4096 }] })
    axios.post.mockReset()
  })

  it('lists the backups on the drive, each downloadable', async () => {
    const { w } = await mountOpen()
    expect(axios.get).toHaveBeenCalledWith('/api/file_backups')
    expect(w.html()).toContain('/api/file_backups/download?name=rebuild-files-1.tar.gz')
  })

  it('makes one and says where it went', async () => {
    const { w } = await mountOpen()
    axios.post.mockResolvedValue({ data: { status: 'OK', name: 'rebuild-files-2.tar.gz' } })
    await w.vm.makeBackup()
    expect(w.vm.message).toBe('Saved as rebuild-files-2.tar.gz')
  })

  it('shows why it could not', async () => {
    const { w } = await mountOpen()
    axios.post.mockResolvedValue({ data: { status: 'ERROR', error: 'does not support backing up' } })
    await w.vm.makeBackup()
    expect(w.vm.message).toBe('does not support backing up')
  })

  it('chooses the one to restore into the next install as an option', async () => {
    const { w, dispatch } = await mountOpen()
    w.vm.chooseRestore('rebuild-files-1.tar.gz')
    expect(dispatch).toHaveBeenCalledWith('setOption', { restoreBackup: 'rebuild-files-1.tar.gz' })
  })
})
