# B8 BFFのパス（D22） 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** テストファイルの3つのBFFルートで、ルートパラメーターの形式を検証してから上流のパスに埋め込む（D22）。`%2e%2e`が別の上流パスに届くかの検証結果を、証拠として残す。

**Architecture:** 3つのルートに、同じ`/my/problems/[id]/`の下のルートと同じ`/^[a-f0-9-]{36}$/`の検査を足す。合わなければ404にする。テストは`node:http`で生のパスを送る。フィクスチャAPIに、受け取ったパスを返すエコーを足す。変更前は、点の区切りが解かれて別のパスに届く。テストの失敗の出力に、その届いた先が出る。

**Tech Stack:** Nuxt 4 / Nitro 2.13（h3 1.15.11、ofetch 1.5.1）、Node 22、Playwright 1.63

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/docs/refactor/FIXES.md`のD22の「変更内容」と「守ること」。根拠は`.local/fixes/briefs/B8.md`と`.local/fixes/briefs/extra.md`の「## B8」。行番号は`f66d6b7`のもの。

## 検証の結果（計画時に実測）

方法: scratchpadで`node --input-type=module -e`を使い、h3（`createRouter({ preemptive: true })`、`getRouterParam`）とofetchの`createFetch({ fetch: globalThis.fetch })`で、3つのルートと同じ組み立てを再現した。Nitroの`$fetch`も、絶対URLなら`globalThis.fetch`を呼ぶ（`.output/server/chunks/nitro/nitro.mjs`の`localFetch`）。上流には、受け取った`req.url`を記録するスタブを置いた。BFFへは`node:http`で、パスをそのまま送った。

| BFFに送ったもの | 上流が受け取ったもの |
| --- | --- |
| `GET /api/my/problems/<p>/test-files/%2e%2e` | `GET /my/problems/<p>/` |
| `GET /api/my/problems/<p>/test-files/.%2E` | `GET /my/problems/<p>/` |
| `GET /api/my/problems/%2e%2e/test-files/<f>` | `GET /my/test-files/<f>` |
| `POST /api/my/problems/<p>/test-files/%2e%2e/complete` | `POST /my/problems/<p>/complete` |
| `POST /api/my/problems/%2e%2e/test-files/<f>/complete` | `POST /my/test-files/<f>/complete` |
| `POST /api/my/problems/%2e%2e/test-files` | `POST /my/test-files` |
| `POST /api/my/problems/%2e/test-files` | `POST /my/problems/test-files` |
| `GET /api/my/problems/<p>/test-files/<f>`（正しいid） | `GET /my/problems/<p>/test-files/<f>`（そのまま） |

- 結論: 届く。h3は`%2e%2e`を1つのセグメントとしてルートに当て、`getRouterParam`はデコードしないまま返す。fetchのWHATWG URL解析は、`%2e%2e`・`.%2e`・`%2e.`・`%2e`を大文字小文字を問わず点の区切りとして解く。`%2F`はデコードしないので、パラメーターがセグメントを増やすことはない。
- 今のAPIへの影響（`api/internal/httpapi/routes.go`と各ハンドラーを読んで確かめた）: 届く先は、ルートがない（404）、メソッドが合わない（405、BFFでは502）、ハンドラーがidを`problemID`か`problems.ValidID`で検証して404にする、のどれか。受け付けるルートは見つからなかった。つまり今は多層防御の穴で、実害はない。
- 他のBFFのルート（`web/server`の`getRouterParam`をすべて確認）: 次のとおり、どれも埋め込む前に検証している。検証がないのはテストファイルの3つだけ。
  - UUIDの式で検証: `images/[id]`、`my/images/[id]`、`posts/[id]`、`problems/[id]`、`problems/[id]/samples`、`problems/[id]/editorials`、`submissions/[id]`、`my/submissions/[id]`、`problem-submissions.ts`、`my/problems/[id]/{featured,rejudge,tester-invitation}`、`my/contests/[id]/*`（`contestAction`、`contestParam`、`participation`、`reveal`、`setter-invitation`、`setters/[handle]`）、`private-content.ts`、`private-problems-route.ts`。`my/favorites/[id]`と`my/difficulty-votes/[id]`はzodの`uuid()`で検証している。
  - 他の形式で検証: `users/[handle]`はハンドルの式、`my/{tester,setter}-invitations/[token]`は`^[A-Z2-7]{32}$`、`auth/[action]`は許可リスト、`contests/[...path]`と`routes/og/[...path]`はパス全体の式。
  - `ratings/[service]`は、値を上流のパスに入れない。

## 判定（Ruling）

- Ruling（形式）: `id`と`file`は、どちらも`/^[a-f0-9-]{36}$/`で検証し、合わなければ`createError({ statusCode: 404 })`にする。理由: 仕様は「他の`/my/*/[id]`系と同じく」。同じ`/my/problems/[id]/`の下の`featured.put.ts`・`rejudge.get.ts`・`rejudge.post.ts`・`tester-invitation.post.ts`が、この式で404にしている。テストファイルのidは、APIの`newSubmissionID`が作る小文字のUUIDなので通る。
- Ruling（順序）: 401（セッション）、403（同一オリジン）、404（形式）、本文の読み取り、上流の順にする。兄弟のルートと同じ。`test-files.post.ts`では、形式の検査を`limitedJSON`より前に置く（`featured.put.ts`と同じ）。そのため、idが不正で本文も不正な要求は、413・415・400ではなく404になる。
- Ruling（共通化しない）: 検査は3つのルートに1行ずつ書く。`contestParam`と`uuidPattern`はコンテスト用のヘルパーで、問題側の兄弟のルートは式をそのまま書いている。新しいヘルパーは作らない。
- Ruling（テストの送り方）: `node:http`の`request`に、`path`を含むオプションを渡して送る。PlaywrightのAPIRequestContextも、URL文字列を渡す`http.request`も、送る前にWHATWG URLで`%2e%2e`を解いてしまうため。
- Ruling（エコーの場所）: フィクスチャAPIで、専用の問題id `12121212-1212-4212-8212-121212121212`の下と、`/my/test-files`の下をエコーにする。既存のエコー用のid `66666666-…`は、問題側には広げない。問題の編集画面は`/api/my/problems/<id>/rejudge`などを読むので、`manage-problem.spec.ts`など、そのidを使う画面のテストの挙動が変わる恐れがあるため。`/my/test-files`は、APIにもほかのテストにもない。点の区切りで`/my/problems/`から1段上がった先だけを受ける。
- Ruling（記録の場所）: 検証の結果は、この計画の「検証の結果」とコミットの本文に書く。`FIXES.md`の状態は、controllerが更新する。

## Global Constraints

- D22の変更内容だけを行う。ほかの公開API、DBスキーマ、依存ライブラリのバージョン、インフラ構成は変えない。APIの側（Go）は変えない。
- 認可と公開範囲は変えない。正しいidの要求は、今と同じパス、メソッド、本文、期限（`complete`の`timeout: 22000`）で上流に届く。
- 1件1コミット（`prefix: 対応内容`、末尾に空行と`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`）。新しいテストを先に書き、変更前のコードで失敗することを確かめてから直す。
- テストの削除・スキップ・検証の緩和はしない。
- 範囲外の問題を見つけたら、直さずに報告する。
- 周りのコードに合わせる。コメントは英語で少なめにし、既存の書き方（`setters/[handle].delete.ts`の1行の検査）に揃える。
- `graphify update .`はcontrollerが行う（流れの規則）。`graphify-out/`はコミットしない。
- Bashの規則: 変数から組み立てたコマンドは使わない。スクリプトファイルは実行しない。Nodeは`nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command`を付けて実行する。Playwrightは、どの流れから実行するときもロックを取る（下の`PW`）。`--output`には必ずscratchpadを渡す。

`BUILD`は次のコマンドの略。

`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`

`PW <files>`は次のコマンドの略。

`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command flock /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/playwright.lock env CI=1 npx playwright test <files> --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b8`

流れのworktree（`.local/lanes/…`）で実行するときは、先頭の`cd`のパスだけをそのworktreeに置き換える。

## Review Focus

1. 正しいidの要求は、今と同じパス・メソッド・本文で上流に届く。3つのルートすべてについて、テストのエコーの行で確かめる。
2. 大文字小文字の混ざった点の区切り（`.%2E`、`%2E%2e`）と、点ではない不正な値（`not-a-test-file`）も404になり、上流に届かない。テストの`rejected`の行で確かめる。
3. セッションなし（401）と別オリジン（403）の判定は、形式の検査より先に行う。テストの最後の行で確かめる。
4. `test-files.post.ts`の本文の上限（2 KiB）は、正しいidのとき今のとおり413を返す。テストの行で確かめる。
5. フィクスチャの変更で、ほかのspecの挙動が変わらない。専用のidと`/my/test-files`は、ほかでは使っていない。Task 1のStep 6で、フィクスチャを使う関連のspecを流して確かめる。

---

### Task 1: D22 テストファイルのBFFルートで、ルートパラメーターの形式を検証してから埋め込む

**Files:**
- Modify: `web/server/api/my/problems/[id]/test-files/[file].get.ts:8`
- Modify: `web/server/api/my/problems/[id]/test-files/[file]/complete.post.ts:9`
- Modify: `web/server/api/my/problems/[id]/test-files.post.ts:9-11`
- Modify: `web/tests/fixtures/public-api.mjs`（107行の`if (path.startsWith('/my/problems/')) return res.writeHead(404).end('{}')`の直前）
- Create: `web/tests/test-file-routes.spec.ts`

**Interfaces:**
- Consumes: なし
- Produces: なし（ルートの公開の形は変えない）

- [ ] **Step 1: フィクスチャにエコーを足す**

`web/tests/fixtures/public-api.mjs`の107行の直前に、次の分岐を足す。本文の読み方は、既存のコンテストのエコー（110-111行）と同じにする。

```js
    // This problem, and the path a dot segment in its ID leads to, echo what the web server forwarded.
    if (path.startsWith('/my/problems/12121212-1212-4212-8212-121212121212/') || path.startsWith('/my/test-files')) {
      let body = ''
      for await (const chunk of req) body += chunk
      return res.end(JSON.stringify({ path, method: req.method, body }))
    }
```

- [ ] **Step 2: 失敗するテストを書く**

`web/tests/test-file-routes.spec.ts`を作る。

```ts
import { request as httpRequest } from 'node:http'
import { expect, test } from '@playwright/test'

// The fixture API echoes the requests for this problem.
const problem = '12121212-1212-4212-8212-121212121212'
const file = '11111111-1111-4111-8111-111111111111'
const session = { cookie: 'openoj_access=valid-access', origin: 'https://judge.example' }
const begin = { size: 1, sha256: 'a'.repeat(64) }

// Playwright's request resolves dot segments such as %2e%2e before sending, so this sends the path as written.
function send(method: 'GET' | 'POST', path: string, headers: Record<string, string> = session, body?: unknown) {
  return new Promise<{ status: number, body: string }>((resolve, reject) => {
    const request = httpRequest({ host: '127.0.0.1', port: 13000, method, path, headers: body === undefined ? headers : { ...headers, 'content-type': 'application/json' } }, response => {
      let text = ''
      response.setEncoding('utf8')
      response.on('data', chunk => { text += chunk })
      response.on('end', () => resolve({ status: response.statusCode ?? 0, body: text }))
    })
    request.on('error', reject)
    request.end(body === undefined ? undefined : JSON.stringify(body))
  })
}

test('test file routes check their IDs before building the API path', async () => {
  const echo = async (method: 'GET' | 'POST', path: string, body?: unknown) => {
    const response = await send(method, path, session, body)
    expect(response.status, `${method} ${path}`).toBe(200)
    return JSON.parse(response.body) as { path: string, method: string, body: string }
  }
  expect(await echo('GET', `/api/my/problems/${problem}/test-files/${file}`)).toMatchObject({ path: `/my/problems/${problem}/test-files/${file}`, method: 'GET' })
  expect(await echo('POST', `/api/my/problems/${problem}/test-files/${file}/complete`)).toMatchObject({ path: `/my/problems/${problem}/test-files/${file}/complete`, method: 'POST' })
  const begun = await echo('POST', `/api/my/problems/${problem}/test-files`, begin)
  expect(begun).toMatchObject({ path: `/my/problems/${problem}/test-files`, method: 'POST' })
  expect(JSON.parse(begun.body)).toEqual(begin)
  expect((await send('POST', `/api/my/problems/${problem}/test-files`, session, { ...begin, sha256: 'a'.repeat(3000) })).status).toBe(413)

  // Each answers 404 without reaching the API. Any other answer comes with its body, which names the path the API received.
  const rejected: ['GET' | 'POST', string, unknown?][] = [
    ['GET', `/api/my/problems/${problem}/test-files/%2e%2e`],
    ['GET', `/api/my/problems/${problem}/test-files/.%2E`],
    ['GET', `/api/my/problems/${problem}/test-files/not-a-test-file`],
    ['GET', `/api/my/problems/%2e%2e/test-files/${file}`],
    ['POST', `/api/my/problems/${problem}/test-files/%2E%2e/complete`],
    ['POST', `/api/my/problems/%2e%2e/test-files/${file}/complete`],
    ['POST', '/api/my/problems/%2e%2e/test-files', begin],
  ]
  const answers: Record<string, number | string> = {}
  for (const [method, path, body] of rejected) {
    const response = await send(method, path, session, body)
    answers[`${method} ${path}`] = response.status === 404 ? 404 : `${response.status} ${response.body}`
  }
  expect(answers).toEqual(Object.fromEntries(rejected.map(([method, path]) => [`${method} ${path}`, 404])))

  // The session and origin checks still come first.
  expect((await send('GET', `/api/my/problems/%2e%2e/test-files/${file}`, {})).status).toBe(401)
  expect((await send('POST', '/api/my/problems/%2e%2e/test-files', { ...session, origin: 'https://attacker.example' }, begin)).status).toBe(403)
})
```

- [ ] **Step 3: 変更前のコードで失敗することを確かめ、結果を控える**

Run: `BUILD`（`.output`を、変更前のルートで作り直す）
Run: `PW tests/test-file-routes.spec.ts`
Expected: FAIL。`expect(answers).toEqual(...)`の1か所で落ちる。差分には、`rejected`の7行がすべて`200 {"path":…}`として出る。期待する届き先は、上から順に次のとおり。
- `/my/problems/<p>/`（2行）
- `/my/problems/<p>/test-files/not-a-test-file`
- `/my/test-files/<f>`
- `/my/problems/<p>/complete`
- `/my/test-files/<f>/complete`
- `/my/test-files`（本文あり）

エコーの3行、413の行、401の行、403の行は、この時点で通る（変えない挙動）。差分に出た届き先を、コミットの本文に使う。計画の表と違う結果が出たら、実装に進まずに報告する。

- [ ] **Step 4: 3つのルートに検査を足す**

`setters/[handle].delete.ts:9-11`と同じ形にする。
- `[file].get.ts`: `hasSession`の検査の後で、`const id = getRouterParam(event, 'id') ?? ''`と`const file = getRouterParam(event, 'file') ?? ''`を取る。`if (!/^[a-f0-9-]{36}$/.test(id) || !/^[a-f0-9-]{36}$/.test(file)) throw createError({ statusCode: 404 })`で検査してから、`` privateAPI(event, `/my/problems/${id}/test-files/${file}`) ``を返す。
- `[file]/complete.post.ts`: `requireSameOrigin`の後で、同じ2つの値を取って同じ検査をする。その後、`` privateAPI(event, `/my/problems/${id}/test-files/${file}/complete`, { method: 'POST', timeout: 22000 }) ``を返す。
- `test-files.post.ts`: `const id = getRouterParam(event, 'id') ?? ''`にする。`if (!/^[a-f0-9-]{36}$/.test(id)) throw createError({ statusCode: 404 })`を、`limitedJSON`の行より前に置く。本文の上限`2 << 10`と、上流の呼び出しは変えない。

- [ ] **Step 5: 通ることを確かめる**

Run: `BUILD`
Run: `PW tests/test-file-routes.spec.ts`
Expected: PASS

- [ ] **Step 6: 関連のspecと型を確かめる**

Run: `PW tests/test-case-editor.spec.ts tests/generator.spec.ts tests/contest-teams.spec.ts tests/contest-scope.spec.ts tests/manage-problem.spec.ts tests/private-problem.spec.ts`
Expected: PASS（`test-case-editor`と`generator`は、テストファイルのBFFの呼び出しを`page.route`で置き換えている。残りは、フィクスチャのエコーや`/my/problems/`の404を使っている）
Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`
Expected: エラーなし

- [ ] **Step 7: コミット**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git add web/server/api/my/problems web/tests/fixtures/public-api.mjs web/tests/test-file-routes.spec.ts
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git commit -m "fix: テストファイルのBFFルートで、ルートパラメーターの形式を検証してから上流のパスに埋め込む" -m "検証: 変更前は、%2e%2e や .%2E のセグメントがfetchのURL解析で .. として解かれ、別の上流パスに届いていた（例: GET /api/my/problems/<p>/test-files/%2e%2e は GET /my/problems/<p>/ に、POST /api/my/problems/%2e%2e/test-files は POST /my/test-files に届いた）。今のAPIでは、届く先はどれもルートがないか、idの検証で404になり、受け付けるものはなかった。ほかのBFFのルートは、ルートパラメーターを埋め込む前にすでに検証している。" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

`git status --short`で、ステージしたのが上の5ファイルだけであることを確かめてからコミットする。本文の例は、Step 3の差分に出た届き先に合わせる。

## 検証のコマンド

どれも`/home/loop/judge/.claude/worktrees/refactor-maintainability/web`で、`nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command`を付けて実行する（Playwrightは`flock …/playwright.lock`で囲む）。全体のスイートはcontrollerが流す。
- `npm run typecheck`
- `npm run build`
- ブラウザー: `flock /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/playwright.lock env CI=1 npx playwright test --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b8-all`
- アカウント: `flock /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/playwright.lock env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test --config playwright.account.config.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b8-account`
- `npm run package:lambda`、`npm run test:lambda`

## Out of scope

- Lambdaの前段（CloudFrontや関数URL）が`%2e%2e`を解くかどうかは調べていない。BFFで検証するので、どちらでも結果は同じ。
- UUIDの式が、ルートによって`[a-f0-9-]{36}`と厳密なv4の式（`private-problems-route.ts`、`submissions/[id]`）に分かれている。揃えない。
- APIの側（Go）の`testFile`は、今も`problemID`で両方のidを検証している。変えない。
- `FIXES.md`の「まとまりと順序」の状態とコミットの欄は、controllerが更新する。
