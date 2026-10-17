import type { Locator, Page } from '@playwright/test'

export type Mark = Locator | { target: Locator | Locator[], pad?: number, badge?: 'top-left' | 'top-right' | 'left' | 'right' | 'bottom' | 'bottom-right' }
type Box = { x: number, y: number, width: number, height: number }

const red = '#e0202f'

// Document coordinates, so marks and clips stay put however the page is scrolled.
function place(target: Locator): Promise<Box> {
  return target.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return { x: rect.x + scrollX, y: rect.y + scrollY, width: rect.width, height: rect.height }
  })
}

// Draws a red frame and a numbered badge on each mark, numbered from 1 in order.
// With a modal dialog open, the marks go inside it: the top layer would cover them otherwise.
export async function annotate(page: Page, marks: Mark[]) {
  const frames: (Box & { pad: number, badge: string })[] = []
  for (const mark of marks) {
    const { target, pad = 4, badge = 'top-left' } = 'target' in mark ? mark : { target: mark }
    // Several locators share one frame around all of them.
    const boxes = await Promise.all((Array.isArray(target) ? target : [target]).map(place))
    const x = Math.min(...boxes.map(b => b.x))
    const y = Math.min(...boxes.map(b => b.y))
    const width = Math.max(...boxes.map(b => b.x + b.width)) - x
    const height = Math.max(...boxes.map(b => b.y + b.height)) - y
    frames.push({ x, y, width, height, pad, badge })
  }
  await page.evaluate(({ frames, red }) => {
    document.querySelector('#docs-annotations')?.remove()
    const modal = document.querySelector('dialog:modal')
    const layer = document.createElement('div')
    layer.id = 'docs-annotations'
    Object.assign(layer.style, { position: modal ? 'fixed' : 'absolute', left: '0', top: '0', width: '0', height: '0', pointerEvents: 'none', zIndex: '2147483647' })
    const [offsetX, offsetY] = modal ? [scrollX, scrollY] : [0, 0]
    frames.forEach((frame, index) => {
      const left = frame.x - frame.pad - offsetX
      const top = frame.y - frame.pad - offsetY
      const width = frame.width + frame.pad * 2
      const height = frame.height + frame.pad * 2
      const outline = document.createElement('div')
      Object.assign(outline.style, {
        position: 'absolute', left: `${left}px`, top: `${top}px`, width: `${width}px`, height: `${height}px`,
        border: `3px solid ${red}`, borderRadius: '6px', boxSizing: 'border-box',
        boxShadow: '0 0 0 2px rgba(255,255,255,.85)',
      })
      const size = 24
      const [x, y] = {
        'top-left': [left - size / 2, top - size / 2],
        'top-right': [left + width - size / 2, top - size / 2],
        'left': [left - size - 6, top + height / 2 - size / 2],
        'right': [left + width + 6, top + height / 2 - size / 2],
        'bottom': [left + width / 2 - size / 2, top + height + 4],
        'bottom-right': [left + width - size, top + height + 4],
      }[frame.badge]!
      const badge = document.createElement('div')
      badge.textContent = String(index + 1)
      Object.assign(badge.style, {
        position: 'absolute', left: `${Math.max(2, x!)}px`, top: `${Math.max(2, y!)}px`,
        width: `${size}px`, height: `${size}px`, borderRadius: '50%', background: red, color: '#fff',
        font: '700 14px/24px system-ui, sans-serif', textAlign: 'center',
        boxShadow: '0 0 0 2px #fff, 0 1px 3px rgba(0,0,0,.3)',
      })
      layer.append(outline, badge)
    })
    ;(modal ?? document.documentElement).append(layer)
  }, { frames, red })
}

// Saves the region covering the given locators, with a margin, or the whole viewport.
export async function shoot(page: Page, name: string, region?: Locator[], margin = 20) {
  let clip: Box | undefined
  if (region) {
    const boxes = await Promise.all(region.map(place))
    const width = await page.evaluate(() => document.documentElement.scrollWidth)
    const left = Math.max(0, Math.min(...boxes.map(b => b.x)) - margin)
    const top = Math.max(0, Math.min(...boxes.map(b => b.y)) - margin)
    const right = Math.min(width, Math.max(...boxes.map(b => b.x + b.width)) + margin)
    const bottom = Math.max(...boxes.map(b => b.y + b.height)) + margin
    clip = { x: left, y: top, width: right - left, height: bottom - top }
  }
  await page.screenshot({ path: `public/images/docs/${name}.png`, clip, fullPage: Boolean(clip), animations: 'disabled', caret: 'hide' })
}
