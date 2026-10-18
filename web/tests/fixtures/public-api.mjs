import { createServer } from 'node:http'
const problem = {
  id: '11111111-1111-4111-8111-111111111111', title: 'A + B', author: 'alice', testers: ['bob', 'carol'], publishedAt: '2026-09-10T00:00:00Z',
  timeLimitMs: 2000, memoryLimitMb: 256, hasSamples: true,
  markdown: '2 つの整数 $A$ と $B$ の和を求めてください。\n\n## 制約\n\n$1 \\le A,B \\le 10^9$\n\n## 入出力例\n\n```\n3 5\n```\n\n```\n8\n```\n\n最大の答えは 2000000000 です。',
  editorial: '## 解説\n\n$A+B$ を計算します。',
  tags: ['DP', 'グラフ'],
}
// The API leaves out the tags of a problem in a 定期便 round.
const featuredProblem = { ...problem, id: 'eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee', title: '定期便の新作', difficulty: 9, editorial: '', tags: [], editorialHidden: true, editorialRevealAt: '2099-09-29T13:00:00Z' }
const tags = [
  { name: 'DP', problemCount: 3, postCount: 1 }, { name: 'セグメント木', problemCount: 2, postCount: 0 },
  { name: 'Rust', problemCount: 0, postCount: 2 }, { name: 'グラフ', problemCount: 1, postCount: 0 },
]
const publicPost = {
  id: '77777777-7777-4777-8777-777777777777', title: '公開記事', author: 'alice', publishedVersion: 1,
  updatedAt: '2026-09-10T00:00:00Z', publishedAt: '2026-09-10T00:00:00Z', isOperator: false, tags: ['DP', 'Rust'],
}
const featured = {
  nextNumber: 13, nextAt: '2099-10-01T14:00:00Z', hasMore: false,
  current: null, waiting: { easy: 3, hard: 2 },
  nextSlots: [
    { slot: 'easy', kind: 'new', difficulty: 4, writer: 'alice', testers: ['bob'] },
    { slot: 'hard', kind: 'new', difficulty: 7, writer: 'carol', testers: ['alice', 'bob'] },
  ],
  items: [
    { number: 12, scheduledAt: '2099-09-28T14:00:00Z', slots: [
      { writer: 'alice', testers: ['bob'], slot: 'easy', kind: 'revival', problemId: problem.id, title: problem.title, difficulty: 2, revealAt: '2099-09-29T13:00:00Z', editorialHidden: false },
      { writer: 'bob', testers: ['carol'], slot: 'hard', kind: 'new', problemId: featuredProblem.id, title: featuredProblem.title, difficulty: 9, revealAt: featuredProblem.editorialRevealAt, editorialHidden: true },
    ] },
    { number: 11, scheduledAt: '2099-09-24T14:00:00Z', slots: [
      { writer: '', testers: [], slot: 'easy', kind: 'missing', problemId: '', title: '', difficulty: null, revealAt: '2099-09-25T13:00:00Z', editorialHidden: false },
      { writer: 'alice', testers: ['bob'], slot: 'hard', kind: 'revival', problemId: problem.id, title: problem.title, difficulty: 7, revealAt: '2099-09-25T13:00:00Z', editorialHidden: false },
    ] },
  ],
}
// Eight rounds: the announced one, then estimates with an open Easy slot and Hard revivals.
const featuredSchedule = {
  waiting: featured.waiting,
  rounds: ['2099-10-01T14:00:00Z', '2099-10-05T14:00:00Z', '2099-10-08T14:00:00Z', '2099-10-12T14:00:00Z', '2099-10-15T14:00:00Z', '2099-10-19T14:00:00Z', '2099-10-22T14:00:00Z', '2099-10-26T14:00:00Z'].map((scheduledAt, index) => ({
    scheduledAt, confirmed: index === 0,
    slots: [
      index < 3 ? { slot: 'easy', kind: 'new', difficulty: [4, 2, 3][index] } : { slot: 'easy', kind: 'missing', difficulty: null },
      index < 2 ? { slot: 'hard', kind: 'new', difficulty: [7, 10][index] } : { slot: 'hard', kind: 'revival', difficulty: null },
    ],
  })),
}
const featuredResult = (timeMs = null, wrong = 0) => ({ accepted: timeMs !== null, timeMs, wrong })
const featuredStandings = {
  round: featured.items[0], closesAt: '2099-09-29T13:00:00Z', closed: false, hasMore: false,
  items: [
    { rank: 1, handle: 'carol', timeMs: 1200000, easy: featuredResult(300000, 2), hard: featuredResult(1200000, 1) },
    { rank: 1, handle: 'alice', timeMs: 1800000, easy: featuredResult(600000), hard: featuredResult(1800000) },
    { rank: 3, handle: 'bob', timeMs: 900000, easy: featuredResult(null, 3), hard: featuredResult(900000, 1) },
    { rank: 4, handle: 'yuki', timeMs: 120000, easy: featuredResult(120000, 1), hard: featuredResult() },
    { rank: 5, handle: 'henry', timeMs: null, easy: featuredResult(null, 2), hard: featuredResult() },
    { rank: 5, handle: 'mika', timeMs: null, easy: featuredResult(), hard: featuredResult(null, 1) },
  ],
}
const submission = {
  id: '22222222-2222-4222-8222-222222222222', problemId: problem.id, problemTitle: problem.title,
  problemVersion: 1, author: 'alice', runtime: 'cpp17', source: 'int main(){}', status: 'DONE',
  createdAt: '2026-09-10T00:00:00Z',
  result: { verdict: 'AC', passed: 1, total: 1, cpuTimeMs: 2, memoryBytes: 1048576, cases: [{ name: 'sample_1', verdict: 'AC', cpuTimeMs: 2, wallTimeMs: 4, memoryBytes: 1048576 }] },
}
createServer(async (req, res) => {
  res.setHeader('Content-Type', 'application/json')
  const path = new URL(req.url, 'http://localhost').pathname
  if (path === '/auth/login' || path === '/auth/refresh') {
    let raw = ''
    for await (const chunk of req) raw += chunk
    const body = JSON.parse(raw)
    if (path === '/auth/refresh' && body.refresh_token !== 'valid-refresh') {
      res.writeHead(body.refresh_token === 'unavailable' ? 503 : 401).end('{}')
    } else res.end(JSON.stringify({ access_token: 'valid-access', expires_in: 3600, ...(path === '/auth/login' ? { refresh_token: 'valid-refresh' } : {}) }))
  }
  else if (path === '/auth/me' || path.startsWith('/my/')) {
    if (req.headers.authorization !== 'Bearer valid-access') return res.writeHead(401).end('{}')
    if (path === '/auth/me') return res.end(JSON.stringify({ id: 'session-user' }))
    if (path === '/my/profile') return res.end(JSON.stringify({ profile: { handle: 'alice', avatar: '', version: 1, createdAt: '2026-09-10T00:00:00Z' } }))
    if (req.method === 'PUT' && /^\/my\/(problems|contests|posts)\/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa$/.test(path)) {
      res.setHeader('Retry-After', '7200')
      return res.writeHead(429).end(JSON.stringify({ error: 'creation_quota_exceeded' }))
    }
    if (path === '/my/problems/55555555-5555-4555-8555-555555555555') return res.end(JSON.stringify({
      id: '55555555-5555-4555-8555-555555555555', version: 1, publishedVersion: 0, updatedAt: '2026-09-10T00:00:00Z',
      draft: { title: '非公開の練習問題', markdown: '保存済みの問題文', editorial: '非公開の解説', timeLimitMs: '2000', memoryLimitMb: '512', testCases: [{ input: 'secret-input', output: 'secret-output' }] },
    }))
    // The fixture session is alice, the author of the featured problem whose editorial is still hidden.
    if (path === `/my/problems/${featuredProblem.id}/publication`) return res.end(JSON.stringify({ ...featuredProblem, editorial: '定期便の解説本文' }))
    if (path === `/my/problems/${featuredProblem.id}/submissions`) return res.end(JSON.stringify({ items: [{ ...submission, id: '33333333-3333-4333-8333-333333333333', problemId: featuredProblem.id, problemTitle: featuredProblem.title, author: 'carol' }], hasMore: false }))
    if (path.startsWith('/my/problems/')) return res.writeHead(404).end('{}')
    // This contest echoes what the web server forwarded, so tests can check the query it built (ADR 0024).
    if (path.startsWith('/my/contests/66666666-6666-4666-8666-666666666666/')) {
      let body = ''
      for await (const chunk of req) body += chunk
      return res.end(JSON.stringify({ route: 'private', path, query: new URL(req.url, 'http://localhost').search, method: req.method, type: req.headers['content-type'] ?? '', body }))
    }
    // The private standings name the table the web server asked for; this contest still has runs to judge (ADR 0023).
    if (path === '/my/contests/88888888-8888-4888-8888-888888888888/standings') {
      const live = new URL(req.url, 'http://localhost').searchParams.get('live') === '1'
      return res.end(JSON.stringify([{ rank: 1, handle: live ? 'live' : 'frozen', points: 0, timeMs: 0, problems: {} }]))
    }
    if (req.method === 'POST' && path === '/my/contests/88888888-8888-4888-8888-888888888888/reveal') return res.writeHead(409).end(JSON.stringify({ error: 'contest_results_pending' }))
    if (req.method === 'POST') {
      let raw = ''
      for await (const chunk of req) raw += chunk
      // Keep successful submissions available to session-refresh tests.
      if (path === '/my/submissions' && JSON.parse(raw).source === '// rate-limit-fixture') {
        res.setHeader('Retry-After', '42')
        return res.writeHead(429).end(JSON.stringify({ error: 'submission_rate_limited' }))
      }
      return res.end(raw)
    }
    res.end(JSON.stringify({ items: [], nextCursor: '' }))
  }
  else if (['/users/alice', '/users/bob', '/users/carol', '/users/yuki'].includes(path)) res.end(JSON.stringify({ handle: path.split('/').at(-1), accounts: path === '/users/yuki' ? { yukicoder: '123' } : {}, avatar: '', createdAt: '2026-09-10T00:00:00Z' }))
  else if (path === '/health') res.end('{}')
  else if (path === '/runtimes') res.end(JSON.stringify({ items: [{ id: 'cpp23-gcc', label: 'C++23 (GCC)' }, { id: 'cpp17', label: 'C++17 (GCC)' }, { id: 'python314', label: 'Python 3.14' }] }))
  else if (path === '/featured/standings') {
    const at = new URL(req.url, 'http://localhost').searchParams.get('at')
    const round = featured.items.find(round => round.scheduledAt === at)
    if (!round) return res.writeHead(404).end('{}')
    res.end(JSON.stringify({ ...featuredStandings, round }))
  }
  else if (path === '/featured') res.end(JSON.stringify(featured))
  else if (path === '/featured/schedule') res.end(JSON.stringify(featuredSchedule))
  else if (path === `/problems/${featuredProblem.id}`) res.end(JSON.stringify(featuredProblem))
  else if (path === '/tags') res.end(JSON.stringify({ items: tags }))
  else if (path === '/problems') {
    const query = new URL(req.url, 'http://localhost').searchParams
    if (query.get('tag')?.includes(',')) return res.writeHead(400).end(JSON.stringify({ error: 'invalid_tag' }))
    const matches = (!query.get('author') || query.get('author') === 'alice') && (!query.get('tag') || problem.tags.includes(query.get('tag')))
    res.end(JSON.stringify({ items: matches ? [problem] : [], nextCursor: '' }))
  }
  else if (path === `/problems/${problem.id}`) res.end(JSON.stringify(problem))
  else if (path === `/problems/${problem.id}/editorials`) res.end(JSON.stringify({ items: [{ problemId: problem.id, id: '77777777-7777-4777-8777-777777777777', title: '日本語 & #記事 + 共有', author: 'alice', publishedAt: '2026-09-10T00:00:00Z', isOperator: false }] }))
  else if (path === `/problems/${problem.id}/samples`) res.end(JSON.stringify({ items: [
    { name: 'sample_1', input: ' 3 5\n\n', output: '8\n' },
    { name: 'empty', input: '', output: '' },
    { name: 'large', input: '', output: 'ok\n', inputFile: { url: 'https://download.example/input', size: 100000, sha256: 'a'.repeat(64) } },
  ] }))
  else if (path === `/problems/${problem.id}/submissions`) res.end(JSON.stringify({ items: [submission], hasMore: false }))
  else if (path === `/problems/${problem.id}/submissions/${submission.id}`) res.end(JSON.stringify(submission))
  else if (path === `/contests/88888888-8888-4888-8888-888888888888/submissions/${submission.id}`) res.end(JSON.stringify({ ...submission, contestId: '88888888-8888-4888-8888-888888888888', problemIndex: 0, rejudgedAt: '2026-09-10T03:00:00Z' }))
  else if (path === '/posts') {
    const query = new URL(req.url, 'http://localhost').searchParams
    const matches = query.get('author') === 'alice' || (query.get('tag') && publicPost.tags.includes(query.get('tag')))
    res.end(JSON.stringify({ items: matches ? [publicPost] : [], nextCursor: '' }))
  }
  else if (path === '/posts/77777777-7777-4777-8777-777777777777') res.end(JSON.stringify({ id: '77777777-7777-4777-8777-777777777777', title: '日本語 & #記事 + 共有', author: 'alice', markdown: '共有する記事です。', publishedVersion: 1, updatedAt: '2026-09-10T00:00:00Z', isOperator: false, publishedAt: '2026-09-10T00:00:00Z', problems: [{ id: problem.id, title: problem.title }], tags: publicPost.tags }))
  else if (/^\/contests\/(88888888-8888-4888-8888-888888888888|99999999-9999-4999-8999-999999999999)$/.test(path)) res.end(JSON.stringify({
    id: path.split('/').pop(), title: '共有コンテスト', author: 'alice', description: '共有するコンテストです。',
    startsAt: '2026-09-10T00:00:00Z', endsAt: '2026-09-10T02:00:00Z', penaltyMinutes: 5,
    status: path.endsWith('99999999-9999-4999-8999-999999999999') ? 'scheduled' : 'running', canEdit: false, official: true, participating: false, canViewSubmissions: false, problems: [{ id: problem.id, title: problem.title, points: 100 }],
  }))
  else if (/^\/contests\/(88888888-8888-4888-8888-888888888888|99999999-9999-4999-8999-999999999999)\/standings$/.test(path)) res.end('[]')
  else if (path.startsWith('/contests/66666666-6666-4666-8666-666666666666/')) res.end(JSON.stringify({ route: 'public', path, query: new URL(req.url, 'http://localhost').search }))
  else if (path === `/contests/88888888-8888-4888-8888-888888888888/problems/${problem.id}` || path === `/contests/99999999-9999-4999-8999-999999999999/problems/${problem.id}`) res.end(JSON.stringify(problem))
  else res.writeHead(404).end('{}')
}).listen(18080, '127.0.0.1', () => console.log('fixture API ready'))
