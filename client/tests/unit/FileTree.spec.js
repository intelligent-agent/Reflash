import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import FileTree from '../../src/components/FileTree.vue'

const P = 'home/printer/printer_data/'
const files = [P + 'config/printer.cfg', P + 'config/moonraker.conf', P + 'config/peripherals/probe.cfg', P + 'database/db']

const make = (selected = files) => mount(FileTree, { props: { files, modelValue: selected } })

describe('FileTree', () => {
  it('shows folders and files, with the folder a user made', () => {
    const text = make().text()
    expect(text).toContain('config/')
    expect(text).toContain('peripherals/')
    expect(text).toContain('printer.cfg')
    expect(text).not.toContain('home/')
  })

  it('shows a folder with some of its files as partly chosen', () => {
    const w = make([P + 'config/printer.cfg'])
    const boxes = w.findAll('input[type=checkbox]')
    const config = boxes[0].element
    expect(config.indeterminate).toBe(true)
    expect(config.checked).toBe(false)
  })

  it('choosing a folder chooses every file in it, and again takes them out', async () => {
    const w = make([])
    await w.findAll('input[type=checkbox]')[0].setValue(true)
    expect(w.emitted('update:modelValue')[0][0].sort()).toEqual(
      [P + 'config/moonraker.conf', P + 'config/peripherals/probe.cfg', P + 'config/printer.cfg'].sort())
    const w2 = make(files)
    await w2.findAll('input[type=checkbox]')[0].setValue(false)
    expect(w2.emitted('update:modelValue')[0][0]).toEqual([P + 'database/db'])
  })

  it('has presets for everything, the Klipper files and nothing', async () => {
    const w = make([])
    const buttons = w.findAll('.presets button')
    await buttons[0].trigger('click')
    expect(w.emitted('update:modelValue').pop()[0]).toHaveLength(4)
    await buttons[1].trigger('click')
    expect(w.emitted('update:modelValue').pop()[0].sort()).toEqual([P + 'config/peripherals/probe.cfg', P + 'config/printer.cfg'].sort())
    await buttons[2].trigger('click')
    expect(w.emitted('update:modelValue').pop()[0]).toEqual([])
  })

  it('folds a folder away', async () => {
    const w = make()
    expect(w.text()).toContain('probe.cfg')
    await w.find('.twist[aria-expanded="true"]').trigger('click')
    expect(w.text()).not.toContain('printer.cfg')
  })
})
