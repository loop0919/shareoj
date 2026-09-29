import { createServer } from 'node:http'

createServer((req, res) => {
  res.setHeader('Content-Type', 'application/json')
  if (['/health','/problems','/posts'].includes(req.url)) {
    res.end('{"items":[],"nextCursor":""}')
  } else if (req.url === '/problems/22222222-2222-4222-8222-222222222222') {
    res.end(JSON.stringify({
      id: '22222222-2222-4222-8222-222222222222', title: 'Math fixture', description: 'Math rendering test',
      markdown: [
        String.raw`合計は $\sum_{i=1}^{n} a_i$ です。`,
        String.raw`$$\frac{n(n+1)}{2} = \sum_{i=1}^{n} i$$`,
        '<img src=x onerror="alert(1)">',
        String.raw`$\unknownCommand{x}$`,
        String.raw`$\href{javascript:alert(1)}{unsafe}$`,
        '$$' + Array.from({ length: 30 }, (_, i) => `x_{${i}}`).join('+') + '$$',
        "```\n$literal input$\n```",
      ].join("\n\n"),
      author: "alice", publishedAt: "2026-09-10T00:00:00Z",
      timeLimitMs: 2000, memoryLimitMb: 256, isSample: true,
    }))
  } else if (req.url === '/problems/44444444-4444-4444-8444-444444444444') {
    res.end(JSON.stringify({ id: '44444444-4444-4444-8444-444444444444', title: 'Incomplete upstream data' }))
  } else {
    res.writeHead(503)
    res.end(JSON.stringify({ error: 'private-upstream-diagnostic' }))
  }
}).listen(18081, '127.0.0.1', () => console.log('failing API ready'))
