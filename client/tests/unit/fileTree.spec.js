import { describe, it, expect } from 'vitest'
import { buildTree, visibleRoots, includeFor, isConfigFile } from '../../src/fileTree'

const P = 'home/printer/printer_data/'
const files = [
  P + 'config/printer.cfg',
  P + 'config/moonraker.conf',
  P + 'config/fluidd.cfg',
  P + 'config/firmware/stm32.config',
  P + 'config/peripherals/probe.cfg',
  P + 'config/peripherals/sensors/chamber.cfg',
  P + 'database/moonraker-sql.db',
  'home/printer/.octoprint/config.yaml',
]

describe('the tree of a config archive (#198)', () => {
  it('starts at config/ and database/, not at the printer\'s home', () => {
    const names = visibleRoots(buildTree(files)).map((n) => n.name)
    expect(names.sort()).toEqual(['.octoprint', 'config', 'database'])
  })

  it('lists a folder the user made, with its subfolders', () => {
    const config = visibleRoots(buildTree(files)).find((n) => n.name === 'config')
    const names = [...config.children.keys()]
    expect(names).toContain('peripherals')
    expect([...config.children.get('peripherals').children.keys()].sort()).toEqual(['probe.cfg', 'sensors'])
    expect(config.children.get('peripherals').files).toHaveLength(2)
  })

  it('gives every folder the files below it', () => {
    const config = visibleRoots(buildTree(files)).find((n) => n.name === 'config')
    expect(config.files).toHaveLength(6)
  })
})

describe('what is handed to the installer', () => {
  it('is nothing at all for everything, so a backup is what it always was', () => {
    expect(includeFor(files, files)).toEqual([])
  })

  it('is null for nothing chosen', () => {
    expect(includeFor(files, [])).toBeNull()
  })

  it('names a folder whole when everything in it is chosen, files otherwise', () => {
    const chosen = files.filter((f) => f.includes('/config/peripherals/') || f.endsWith('printer.cfg'))
    expect(includeFor(files, chosen).sort()).toEqual([P + 'config/peripherals', P + 'config/printer.cfg'].sort())
  })

  it('names the files of a folder only partly chosen', () => {
    const chosen = [P + 'config/peripherals/probe.cfg']
    expect(includeFor(files, chosen)).toEqual([P + 'config/peripherals/probe.cfg'])
  })
})

describe('the config preset', () => {
  it('is all of config/ - Klipper, Moonraker, KlipperScreen, the UI, firmware options, a folder the user made - and not the database', () => {
    expect(files.filter(isConfigFile)).toEqual([
      P + 'config/printer.cfg',
      P + 'config/moonraker.conf',
      P + 'config/fluidd.cfg',
      P + 'config/firmware/stm32.config',
      P + 'config/peripherals/probe.cfg',
      P + 'config/peripherals/sensors/chamber.cfg',
    ])
    expect(isConfigFile(P + 'config/KlipperScreen.conf')).toBe(true)
    expect(isConfigFile(P + 'database/moonraker-sql.db')).toBe(false)
    expect(isConfigFile('home/printer/.octoprint/config.yaml')).toBe(false)
  })
})
