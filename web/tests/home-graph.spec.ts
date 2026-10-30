import { expect, test } from '@playwright/test'

const dotFills = (page: import('@playwright/test').Page, selector: string) =>
  page.locator(selector).evaluateAll(dots => [...new Set(dots.map(dot => getComputedStyle(dot).fill))])

test('clicking hero nodes lights them, their lit edges, and a ripple from the node', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'no-preference' })
  await page.goto('/')
  const art = page.locator('.path-art')
  await art.evaluate(svg => svg.getAnimations({ subtree: true }).forEach(animation => animation.finish()))
  const nodes = art.locator('.graph-node')
  const litEdges = art.locator('.answer-path--lit')
  await expect(litEdges).toHaveCount(0)

  await nodes.nth(0).click()
  await expect(art.locator('.graph-node--lit')).toHaveCount(1)
  await expect(art.locator('.graph-node--lit .node-halo')).toHaveCount(1)
  // Both edges to the lit answer path are traced from the clicked node.
  expect(await litEdges.evaluateAll(paths => paths.map(path => path.getAttribute('d')))).toEqual(['M168 104 64 212', 'M168 104 284 184'])
  const rippling = art.locator('.grid-dot--ripple')
  expect(await rippling.count()).toBeGreaterThan(0)
  expect(await rippling.count()).toBeLessThan(255)
  await rippling.evaluateAll(dots => dots.forEach(dot => dot.getAnimations().forEach((animation) => {
    animation.pause()
    animation.currentTime = Number(animation.effect!.getTiming().delay) + 270
  })))
  // Push and tint fade with distance, so the paused ripple shows more than one shade.
  const fills = await dotFills(page, '.grid-dot--ripple')
  expect(fills.length).toBeGreaterThan(1)

  await nodes.nth(1).click()
  await expect(litEdges).toHaveCount(3)
  expect(await litEdges.nth(1).getAttribute('d')).toBe('M292 72 168 104')
  await nodes.nth(0).click()
  await expect(art.locator('.graph-node--lit')).toHaveCount(1)
  await expect(litEdges).toHaveCount(0)

  // Answer nodes stay lit and only send a ripple.
  await nodes.nth(4).click()
  await expect(art.locator('.graph-node--lit')).toHaveCount(1)
  await art.evaluate(svg => svg.getAnimations({ subtree: true }).forEach(animation => animation.finish()))
  expect(await dotFills(page, '.grid-dot')).toEqual(['rgb(220, 226, 232)'])
  await expect(art.locator('.graph-node--lit .quiet-node')).toHaveCSS('fill', 'rgb(24, 121, 78)')
})

test('hero nodes light without motion when reduced motion is requested', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light', reducedMotion: 'reduce' })
  await page.goto('/')
  const art = page.locator('.path-art')
  await art.locator('.graph-node').nth(2).click()
  await expect(art.locator('.answer-path--lit')).toHaveCount(2)
  await expect(art.locator('.graph-node--lit .quiet-node')).toHaveCSS('fill', 'rgb(24, 121, 78)')
  expect(await art.evaluate(svg => svg.getAnimations({ subtree: true }).length)).toBe(0)
})
