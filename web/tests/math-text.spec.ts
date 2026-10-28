import { expect, test } from '@playwright/test'
import { renderMathText } from '../app/utils/math-text'

test('mixed prose, inline math, display math, and escaped dollars', () => {
  // Not String.raw: Bun's transpiler stores non-ASCII template text as \u escapes, which raw strings expose.
  const parts = renderMathText('価格は \\$5。$x^2$ と $$\\frac{a}{b}$$。')
  expect(parts[0]).toEqual({ kind: 'text', text: '価格は $5。' })
  const math = parts.filter(part => part.kind === 'math')
  expect(math.map(part => part.display)).toEqual([false, true])
  expect(math[0]?.html).toContain('class="katex"')
  expect(math[1]?.html).toContain('class="katex-display"')
  expect(math[1]?.html).toContain('<math ')
})

test('escaped dollars inside math do not close the expression', () => {
  const parts = renderMathText(String.raw`$\text{cost: \$5}$`)
  expect(parts).toHaveLength(1)
  expect(parts[0]?.kind).toBe('math')
})

test('invalid, incomplete, and over-budget expressions retain their source', () => {
  for (const source of [String.raw`$\unknownCommand{x}$`, '$unclosed', '$$unclosed', '$' + 'x'.repeat(10_001) + '$', String.raw`$\def\a{\a}\a$`]) {
    expect(renderMathText(source)).toEqual([{ kind: 'text', text: source }])
  }
})

test('ordinary HTML is text and KaTeX cannot create links or images', () => {
  const source = '<img src=x onerror="alert(1)">'
  expect(renderMathText(source)).toEqual([{ kind: 'text', text: source }])
  for (const tex of [String.raw`\href{javascript:alert(1)}{click}`, String.raw`\includegraphics{https://example.com/a.png}`]) {
    const parts = renderMathText(`$${tex}$`)
    for (const part of parts) {
      if (part.kind === 'math') {
        expect(part.html).not.toMatch(/<(?:a|img)\b/i)
      }
    }
  }
})

test('display math is rendered in SSR; invalid content is text; code stays literal', async ({ browser }) => {
  const context = await browser.newContext({ javaScriptEnabled: false, viewport: { width: 320, height: 900 } })
  const page = await context.newPage()
  const response = await page.goto('http://127.0.0.1:13001/problems/22222222-2222-4222-8222-222222222222')
  expect(response?.status()).toBe(200)
  await expect(page.locator('.markdown-body p').first()).toContainText('合計は')
  await expect(page.locator('.katex-display')).toHaveCount(2)
  await expect(page.locator('.katex-display').first()).toBeVisible()
  await expect(page.locator('.markdown-body img, .markdown-body a')).toHaveCount(0)
  await expect(page.getByText('<img src=x onerror="alert(1)">', { exact: true })).toBeVisible()
  await expect(page.locator('.markdown-body')).toContainText(String.raw`\unknownCommand`)
  await expect(page.locator('.markdown-body pre').first()).toHaveText('$literal input$\n')
  await expect(page.locator('pre .katex')).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  const longMath = page.locator('.katex-display').last()
  expect(await longMath.evaluate(element => element.scrollWidth > element.clientWidth)).toBe(true)
  // CSS and font assets must be self-hosted and loaded without page JavaScript.
  expect(await page.locator('.katex').first().evaluate(element => getComputedStyle(element).fontFamily)).toContain('KaTeX_Main')
  await context.close()
})
