import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import axios from 'axios'
import TheMetrics from '../../src/components/TheMetrics.vue'

vi.mock('axios')

// A flash on an A5, in the shape the session behind #128 recorded it: the SoC
// warms from 42C to 71C, thermal throttling engages on the way, throughput
// falls as it does, and the DRAM rail sits at 1.36V on a board fitted with
// 1.5V parts.
function flashSamples(n = 12) {
  return Array.from({ length: n }, (_, i) => ({
    t: 1000 + i,
    cpu_temp: 42 + i * 2.7,
    cpu_freq: i > 7 ? 816 : 1104,
    throttle: i > 7 ? 3 : 0,
    vcc_dram: 1.36,
    dirty: 3072,
    bandwidth: 4.6 - i * 0.15,
  }))
}

function mountWith(samples, revision = 'A5') {
  axios.get.mockResolvedValue({ data: { samples, window: 300 } })
  return mount(TheMetrics, { props: { active: true, revision } })
}

// The footer is <min> <note?> <max>, and the note carries a number of its own
// ("expected 1.36V"), so take the first and last spans rather than scraping
// the text.
function axisOf(foot) {
  const spans = foot.findAll('span')
  return [Number(spans[0].text()), Number(spans[spans.length - 1].text())]
}

beforeEach(() => vi.clearAllMocks())

describe('TheMetrics', () => {
  it('says so while it has nothing yet, rather than drawing an empty chart', async () => {
    const w = mountWith([])
    await flushPromises()
    expect(w.text()).toContain('Collecting board metrics')
    expect(w.find('svg.spark').exists()).toBe(false)
  })

  it('draws one panel per measure, each on its own scale', async () => {
    const w = mountWith(flashSamples())
    await flushPromises()
    // Deliberately not one chart with six series: MB/s, C, MHz and volts share
    // no axis and would be meaningless overlaid.
    expect(w.findAll('svg.spark').length).toBe(6)
    expect(w.text()).toContain('SoC temperature')
    expect(w.text()).toContain('Throughput')
    expect(w.text()).toContain('DRAM rail')
  })

  it('shows the latest reading, not the first', async () => {
    const w = mountWith(flashSamples())
    await flushPromises()
    const panel = w.findAll('.panel').find((p) => p.text().includes('SoC temperature'))
    // 42 + 11*2.7 = 71.7 -> 72C at zero digits. Asserted on the value element
    // rather than the panel text: the axis footer legitimately shows 42 as the
    // range minimum, which is the whole point of having a footer.
    expect(panel.find('.value').text()).toContain('72')
    expect(panel.find('.foot').text()).toContain('42')
  })

  it('flags throttling, and never by colour alone', async () => {
    const w = mountWith(flashSamples())
    await flushPromises()
    const panel = w.findAll('.panel').find((p) => p.text().includes('Thermal throttle'))
    const value = panel.find('.value')
    expect(value.classes()).toContain('warn')
    // The warning glyph is in CSS content, so assert the class that carries it
    // plus the label - the point is that the status is not the colour alone.
    expect(panel.text()).toContain('Thermal throttle')
  })

  it('calls out a DRAM rail below what the fitted part needs', async () => {
    // A5 expects 1.5V; this board reports 1.36V, which is the fault that
    // motivated the panel and is invisible without an expectation to compare to.
    const w = mountWith(flashSamples(), 'A5')
    await flushPromises()
    const panel = w.findAll('.panel').find((p) => p.text().includes('DRAM rail'))
    expect(panel.find('.value').classes()).toContain('critical')
    expect(panel.text()).toContain('expected 1.5V')
  })

  it('leaves the same rail alone on a board that wants it', async () => {
    const w = mountWith(flashSamples(), 'A8')
    await flushPromises()
    const panel = w.findAll('.panel').find((p) => p.text().includes('DRAM rail'))
    expect(panel.find('.value').classes()).not.toContain('critical')
    expect(panel.text()).toContain('expected 1.36V')
  })

  it('breaks the line across gaps instead of interpolating through them', async () => {
    // Throughput is absent between flashes. Joining across that would draw a
    // slope through a period when nothing was transferring.
    const gapped = [
      { t: 1, bandwidth: 4.0 },
      { t: 2 },
      { t: 3, bandwidth: 3.0 },
    ]
    const w = mountWith(gapped)
    await flushPromises()
    const panel = w.findAll('.panel').find((p) => p.text().includes('Throughput'))
    const d = panel.find('path.line').attributes('d')
    // Two separate subpaths - a second M rather than one continuous L run.
    expect(d.match(/M/g).length).toBe(2)
  })

  // Both of these were found by rendering the component and looking at it, not
  // by the suite - which passed happily while a 1.36V rail was drawn on an axis
  // running 0.36 to 2.36. An axis that absurd makes the panel worthless, so it
  // is worth asserting on.
  it('scales a flat series proportionally, not by a fixed amount', async () => {
    const flat = Array.from({ length: 5 }, (_, i) => ({ t: i, vcc_dram: 1.36 }))
    const w = mountWith(flat, 'A8')
    await flushPromises()
    const foot = w.findAll('.panel').find((p) => p.text().includes('DRAM rail')).find('.foot')
    const [lo, hi] = axisOf(foot)
    // A rail plotted on a volt-wide axis says nothing about a rail.
    expect(hi - lo).toBeLessThan(0.5)
    expect(lo).toBeLessThan(1.36)
    expect(hi).toBeGreaterThan(1.36)
  })

  it('puts the expected rail voltage inside the range, so a shortfall is visible', async () => {
    // An A5 at 1.36V wanted 1.5V. If the scale covered only the samples, the
    // line would sit mid-panel and the gap this panel exists to show would not
    // be drawn at all.
    const w = mountWith(flashSamples(), 'A5')
    await flushPromises()
    const panel = w.findAll('.panel').find((p) => p.text().includes('DRAM rail'))
    const [lo, hi] = axisOf(panel.find('.foot'))
    expect(lo).toBeLessThan(1.36)
    expect(hi).toBeGreaterThan(1.5)
    expect(panel.find('line.reference').exists()).toBe(true)
  })

  it('renders a flat series as a line rather than dividing by zero', async () => {
    const flat = Array.from({ length: 5 }, (_, i) => ({ t: i, cpu_temp: 50 }))
    const w = mountWith(flat)
    await flushPromises()
    const panel = w.findAll('.panel').find((p) => p.text().includes('SoC temperature'))
    const d = panel.find('path.line').attributes('d')
    expect(d).toBeTruthy()
    expect(d).not.toContain('NaN')
  })

  it('asks only for samples it does not already have', async () => {
    const w = mountWith(flashSamples())
    await flushPromises()
    expect(axios.get).toHaveBeenCalledWith('/api/get_metrics?since=0')

    axios.get.mockResolvedValue({ data: { samples: [], window: 300 } })
    await w.vm.fetch()
    // The newest sample it holds is t=1011, so that is where it resumes.
    expect(axios.get).toHaveBeenLastCalledWith('/api/get_metrics?since=1011')
  })

  it('stops polling when the dialog closes', async () => {
    vi.useFakeTimers()
    try {
      const w = mountWith(flashSamples())
      await flushPromises()

      // Still open: the interval fires and it asks again.
      const beforeTick = axios.get.mock.calls.length
      vi.advanceTimersByTime(2500)
      expect(axios.get.mock.calls.length).toBeGreaterThan(beforeTick)

      // Closed: no reason to keep asking a board mid-flash for numbers nobody
      // is looking at.
      await w.setProps({ active: false })
      const afterClose = axios.get.mock.calls.length
      vi.advanceTimersByTime(10000)
      expect(axios.get.mock.calls.length).toBe(afterClose)
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not plot a field the board never reported', async () => {
    // A board with no devfreq node reports no dram_freq. Absent must read as
    // absent, not as zero.
    const w = mountWith([{ t: 1, cpu_temp: 40 }, { t: 2, cpu_temp: 41 }])
    await flushPromises()
    const panel = w.findAll('.panel').find((p) => p.text().includes('Writeback'))
    expect(panel.find('path.line').exists()).toBe(false)
    expect(panel.text()).toContain('—')
  })
})
