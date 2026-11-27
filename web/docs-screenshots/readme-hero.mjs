// Writes docs/readme/hero-{light,dark}.png, the banner at the top of the repository README.
// Run from web/ after the home hero changes: node docs-screenshots/readme-hero.mjs
import { chromium } from 'playwright'

const out = new URL('../../docs/readme/', import.meta.url).pathname
const browser = await chromium.launch()
for (const scheme of ['light', 'dark']) {
  const context = await browser.newContext({ viewport: { width: 1180, height: 800 }, deviceScaleFactor: 2, colorScheme: scheme, locale: 'ja-JP' })
  const page = await context.newPage()
  await page.goto('https://www.share-oj.net/', { waitUntil: 'networkidle' })
  // Transparent, so the banner sits on GitHub's own light and dark backgrounds.
  await page.addStyleTag({ content: 'html, body, #__nuxt, .home, .hero { background: transparent !important; } .hero-actions, .hero-description { display: none !important; }' })
  await page.evaluate(() => document.fonts.ready)
  // The answer path draws itself in; wait until it has reached the goal.
  await page.waitForTimeout(4000)
  const hero = await page.locator('.hero').boundingBox()
  const art = await page.locator('.hero-figure svg').boundingBox()
  // The art keeps empty rows of its own above and below the dots; cut into them.
  const trim = 28
  await page.screenshot({
    path: `${out}hero-${scheme}.png`, omitBackground: true,
    clip: { x: hero.x, y: art.y + trim, width: hero.width, height: art.height - 2 * trim },
  })
  await context.close()
}
await browser.close()
