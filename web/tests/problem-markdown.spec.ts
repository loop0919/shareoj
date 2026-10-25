import { expect, test } from '@playwright/test'
import { renderProblemMarkdown } from '../app/utils/problem-markdown'
import { draftErrors } from '../app/utils/problem-draft'

test('content links open separate tabs safely while fragment links stay on the page', () => {
  for (const href of ['/blog/article', 'https://judge.example/blog/article', 'https://example.com', '//example.com']) {
    expect(renderProblemMarkdown(`[link](${href})`))
      .toContain(`href="${href}" target="_blank" rel="noopener noreferrer"`)
  }
  expect(renderProblemMarkdown('[見出し](#section)')).toBe('<p><a href="#section">見出し</a></p>\n')
})

test('copy controls are available for code fences but not input or math', () => {
  for (const language of ['text', 'py', 'cpp', 'javascript', '']) {
    expect(renderProblemMarkdown(`\`\`\`${language}\na < b\n\`\`\``)).toContain('class="code-copy"')
  }
  for (const source of ['```input\n$A$ $B$\n```', '```math\nx^2\n```', '`inline`']) {
    expect(renderProblemMarkdown(source)).not.toContain('class="code-copy"')
  }
})

test('editor previews mark each block with its source lines', () => {
  const source = '# 見出し\n\n段落\n\n```cpp\nint x;\n```\n\n$$\nx^2\n$$\n\n```input\n$N$\n```\n\n:::details 詳細\n- 項目\n:::\n\n```math\ny\n```'
  const html = renderProblemMarkdown(source, { sourceLines: true })
  for (const [tag, start, end] of [
    ['h1', 0, 1], ['p', 2, 3], ['div', 4, 7], ['p', 8, 11], ['div', 12, 15], ['details', 16, 18], ['ul', 17, 18], ['li', 17, 18], ['p', 20, 23],
  ] as const) {
    expect(html).toMatch(new RegExp(`<${tag} data-source-line="${start}" data-source-end="${end}"[ >]`))
  }
  expect(html.match(/data-source-line=/g)).toHaveLength(9)
  expect(renderProblemMarkdown(source)).not.toContain('data-source-')
})

test('copy buttons copy only code text and report clipboard failures', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.goto('/docs/markdown')
  const languages = page.locator('details').filter({ hasText: '対応言語と指定する言語名' })
  await languages.locator('summary').click()
  for (const language of ['Python', 'C++', 'C', 'Rust', 'C#', 'Java', 'Nim', 'Go', 'Haskell', 'JavaScript', 'TypeScript', 'Ruby']) {
    await expect(languages.locator('td').getByText(language, { exact: true })).toBeVisible()
  }
  await expect(page.locator('.guide-result .input-format')).toHaveCount(2)
  await expect(page.locator('.guide-result .code-copy')).toHaveCount(1)
  for (const block of await page.locator('.markdown-body .copyable-code').all()) {
    const source = await block.locator('pre > code').textContent()
    await block.getByRole('button', { name: 'コードをコピー' }).click()
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(source)
  }
  await page.evaluate(() => {
    Object.defineProperty(navigator.clipboard, 'writeText', { value: () => Promise.reject(new Error('denied')) })
  })
  await page.locator('.guide-result .code-copy').click()
  await expect(page.locator('.guide-result [role="status"]').filter({ hasText: 'コピーできませんでした' })).toBeVisible()
})

test('spaces between values in an input format show nearly as wide as \\quad', async ({ page }) => {
  await page.goto('/docs/markdown')
  const gaps = await page.evaluate((blocks) => blocks.map((html) => {
    const box = document.createElement('div')
    box.className = 'markdown-body'
    box.innerHTML = html
    document.querySelector('main')!.prepend(box)
    const block = box.querySelector('.input-format')!
    const [a, b] = [...block.querySelectorAll('.katex-html .mord.mathnormal')].map(glyph => glyph.getBoundingClientRect())
    const gap = (b!.left - a!.right) / parseFloat(getComputedStyle(block).fontSize)
    const mathSpacing = getComputedStyle(block.querySelector('.katex')!).wordSpacing
    box.remove()
    return { gap, mathSpacing }
  }), ['```input\n$A$ $B$\n```', '```input\n$A \\quad B$\n```'].map(source => renderProblemMarkdown(source)))
  const [space, quad] = gaps
  // About an em space, which is a little narrower than \quad in KaTeX's larger font.
  expect(space!.gap).toBeGreaterThan(.9)
  expect(space!.gap).toBeLessThan(quad!.gap)
  expect(gaps.map(gap => gap.mathSpacing)).toEqual(['0px', '0px'])
})

test('explicit heading anchors preserve formatting and reject attribute injection', () => {
  expect(renderProblemMarkdown('## **判定**の例 {#testlib}\n\n本文'))
    .toBe('<h2 id="testlib"><strong>判定</strong>の例</h2>\n<p>本文</p>\n')
  for (const source of ['本文 {#testlib}', '```text\n## 見出し {#testlib}\n```', '## 見出し {#bad" onclick="alert(1)}']) {
    expect(renderProblemMarkdown(source)).not.toContain(' id=')
  }
})

test('HTML comments are hidden while code examples and escaped comments stay visible', () => {
  for (const label of ['問題文', '解説', '記事', 'コンテストの概要']) {
    expect(renderProblemMarkdown(`<!-- ここに${label}を記載 -->\n`)).toBe('')
  }
  expect(renderProblemMarkdown('## 問題文\n\n<!-- メモ\n\n## 非表示\n<script>alert(1)</script>\n-->\n\n本文'))
    .toBe('<h2>問題文</h2>\n<p>本文</p>\n')
  expect(renderProblemMarkdown('前<!-- メモ -->後')).toBe('<p>前後</p>\n')
  expect(renderProblemMarkdown('本文\n<!-- 未完了\n\nメモ')).toBe('<p>本文</p>\n')
  expect(renderProblemMarkdown('> <!-- メモ -->\n> 本文')).toBe('<blockquote>\n<p>本文</p>\n</blockquote>\n')
  for (const source of ['`<!-- メモ -->`', '```text\n<!-- メモ -->\n```', '    <!-- メモ -->', '\\<!-- メモ -->', '&lt;!-- メモ --&gt;']) {
    expect(renderProblemMarkdown(source)).toContain('&lt;!-- メモ --&gt;')
  }
})

test('colored text supports names and hex colors while preserving Markdown boundaries', () => {
  expect(renderProblemMarkdown('本文%赤文字ですよ%{red}と%白い文字%{#FFFFFF}です。'))
    .toContain('本文<span style="color: red">赤文字ですよ</span>と<span style="color: #FFFFFF">白い文字</span>です。')
  for (const color of ['blue', 'rebeccapurple', '#abc', '#abcd', '#123ABC', '#123ABC80']) {
    expect(renderProblemMarkdown(`| 説明 |\n| --- |\n| %文字%{${color}} |`))
      .toContain(`<td><span style="color: ${color}">文字</span></td>`)
  }
  expect(renderProblemMarkdown('%<img src=x onerror=alert(1)>&%{red}'))
    .toContain('<span style="color: red">&lt;img src=x onerror=alert(1)&gt;&amp;</span>')
  for (const source of ['`%文字%{red}`', '```text\n%文字%{red}\n```', '$%文字%{red}$', '\\%文字%{red}', '%文字%{red;display:none}', '%文字%{red" onclick="alert(1)}', '%文字%{#12345}', '%文字%{url(x)}', '%文字%{}', '%未完了', '50%です']) {
    expect(renderProblemMarkdown(source)).not.toContain('<span style="color:')
  }
})

test('attribute-free br tags break table cells without enabling HTML or changing code', () => {
  for (const tag of ['<br>', '<br/>', '<br />', '<BR>']) {
    const html = renderProblemMarkdown(`| 入力 | 説明 |\n| --- | --- |\n| \`3\`${tag}\`5\` | N と Q |`)
    expect(html).toContain('<td><code>3</code><br>\n<code>5</code></td>')
  }
  for (const source of ['`<br>`', '```text\n<br>\n```', '\\<br>', '&lt;br&gt;', '<br onclick="alert(1)">', '<br style="color:red">', '<bravo>', '</br>', '<img src=x onerror="alert(1)">']) {
    const html = renderProblemMarkdown(source)
    expect(html).not.toMatch(/<(br|bravo|img)\b/i)
    expect(html).toContain('&lt;')
  }
})

test('judge status shortcodes reuse badges and respect Markdown boundaries', () => {
  for (const verdict of ['AC', 'WA', 'TLE', 'MLE', 'OLE', 'RE', 'CE', 'JE', 'WJ']) {
    expect(renderProblemMarkdown(`結果は:${verdict}:です。`)).toContain(
      `<span class="verdict-badge"${verdict === 'WJ' ? '' : ` data-verdict="${verdict}"`}>${verdict}</span>`,
    )
  }
  expect(renderProblemMarkdown('**:AC:**:RE:')).toContain('</span></strong><span class="verdict-badge" data-verdict="RE">')
  for (const source of ['`:AC:`', '```text\n:RE:\n```', '```input\n:AC:\n```', '$:AC:$', '\\:AC:', ':UNKNOWN: :ac:', '[link](https://example.com/:AC:)']) {
    expect(renderProblemMarkdown(source)).not.toContain('verdict-badge')
  }
})

test('details render Markdown while preserving code fences and escaping titles', () => {
  expect(renderProblemMarkdown(':::details タイトル\n内容\n:::')).toBe('<details><summary>タイトル</summary>\n<p>内容</p>\n</details>\n')
  const html = renderProblemMarkdown([
    '::::details <img src=x onerror=alert(1)>', '**内容** $A$', '',
    '```text', ':::', '```', '::::', '', '後続の本文',
  ].join('\n'))
  expect(html).toContain('<details><summary>&lt;img src=x onerror=alert(1)&gt;</summary>')
  expect(html).toContain('<strong>内容</strong>')
  expect(html).toContain('class="katex"')
  expect(html).toContain('<code class="language-text">:::\n</code>')
  expect(html).toContain('</details>\n<p>後続の本文</p>')
  expect(html).not.toContain('<img')
  expect(html).not.toContain('<details open')
  expect(renderProblemMarkdown('```makefile\n:::details タイトル\n内容\n:::\n```')).not.toContain('<details>')
  expect(renderProblemMarkdown(':::details\n内容')).toContain('<summary>詳細</summary>\n<p>内容</p>\n</details>')
})

test('Markdown supports flexible structure and dedicated input / math fences', () => {
  const html = renderProblemMarkdown([
    '## 自由な見出し', '', '**太字**と $a_i$', '',
    '| A | B |', '| --- | --- |', '| 1 | 2 |', '',
    '```input', '$N$', '$A_1 \\quad A_N$', '```', '',
    '```math', '\\frac{n(n+1)}{2}', '```', '',
    '```text', '$literal$', '```', '', '`$code$`',
  ].join('\n'))
  expect(html).toContain('<h2>自由な見出し</h2>')
  expect(html).toContain('<strong>太字</strong>')
  expect(html).toContain('<table>')
  expect(html).toContain('class="input-format"')
  expect(html).toContain('class="katex-display"')
  expect(html).toContain('>$literal$\n</code>')
  expect(html).toContain('<code>$code$</code>')
  expect(html).not.toContain('$A_1')
})

test('code fences highlight every supported submission language', () => {
  for (const [language, source] of [
    ['cpp', '#include <iostream>\nint main() { return 0; }'],
    ['c', '#include <stdio.h>\nint main(void) { return 0; }'],
    ['python', 'def answer():\n    return 42'],
    ['rust', 'fn main() { let answer = 42; }'],
    ['csharp', 'using System;\nConsole.WriteLine(42);'],
    ['java', 'class Main { public static void main(String[] args) {} }'],
    ['nim', 'let answer = 42\necho answer'],
    ['go', 'package main\nfunc main() {}'],
    ['haskell', 'main :: IO ()\nmain = print 42'],
    ['javascript', 'const answer = 42;\nconsole.log(answer);'],
    ['typescript', 'const answer: number = 42;\nconsole.log(answer);'],
    ['ruby', 'def answer\n  42\nend'],
  ]) {
    const html = renderProblemMarkdown(`\`\`\`${language}\n${source}\n\`\`\``)
    const lineNumbers = source.split('\n').map((_, line) => line + 1).join('\n')
    expect(html).toContain(`<pre class="highlighted-code"><span class="code-line-numbers" aria-hidden="true">${lineNumbers}</span>`)
    expect(html).toContain(`class="language-${language}"`)
    expect(html).toContain('<span class="hljs-')
    expect(html).not.toContain('<iostream>')
  }
  expect(renderProblemMarkdown('```text\nint main() {}\n```')).not.toContain('class="hljs ')
  expect(renderProblemMarkdown('```text\na\nb\n```')).not.toContain('code-line-numbers')
})

test('code fence aliases use the canonical language and unknown languages stay plain', () => {
  for (const [canonical, aliases] of [
    ['python', ['py']], ['cpp', ['c++']], ['rust', ['rs']],
    ['csharp', ['cs', 'c#']], ['go', ['golang']], ['haskell', ['hs']],
    ['javascript', ['js']], ['typescript', ['ts']], ['ruby', ['rb']],
  ] as const) {
    for (const alias of aliases) {
      expect(renderProblemMarkdown('```' + alias.toUpperCase() + '\n42\n```'))
        .toBe(renderProblemMarkdown('```' + canonical + '\n42\n```'))
    }
  }
  for (const language of ['text', '', 'unknown']) {
    const html = renderProblemMarkdown('```' + language + '\n<script>alert(1)</script>\n```')
    expect(html).not.toContain('highlighted-code')
    expect(html).toContain('&lt;script&gt;')
  }
})

test('HTML and malicious protocols cannot become executable content', () => {
  const html = renderProblemMarkdown([
    '<script>alert(1)</script>',
    '<img src=x onerror="alert(1)">',
    '[click](javascript:alert%281%29)',
    '```input', '<img src=x onerror=alert(1)> $A$', '```',
    '', '$\\href{javascript:alert(1)}{unsafe}$',
  ].join('\n'))
  expect(html).not.toMatch(/<(script|img)\b/i)
  expect(html).not.toMatch(/href=["']javascript:/i)
  expect(html).toContain('&lt;script&gt;')
  expect(html).toContain('class="katex"')
})

test('unclosed fences and invalid math keep the preview usable; macros are isolated', () => {
  expect(() => renderProblemMarkdown('```math\n\\unknownCommand{x}')).not.toThrow()
  renderProblemMarkdown('$\\gdef\\openojmacro{123}\\openojmacro$')
  const next = renderProblemMarkdown('$\\openojmacro$')
  expect(next).not.toContain('>123<')
  expect(next).toContain('openojmacro')
})

test('draft errors name only a blank title and a blank statement', () => {
  expect(draftErrors({ title: '  ', markdown: '## 本文' })).toEqual({ title: '問題のタイトルを入力してください。', markdown: '' })
  expect(draftErrors({ title: 'A + B', markdown: ' \n' })).toEqual({ title: '', markdown: '問題の本文を入力してください。' })
})
