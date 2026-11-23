# 配信モード Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** スイッチを入れたブラウザでは、スタッフ・作者・テスターとして見える内容と、マイページや作問画面を出さない「配信モード」を足す。

**Architecture:** 状態はCookie `openoj-streaming` に持つ。Webサーバーは、セッション付きでAPIを呼ぶ唯一の入口`privateAPI`で、配信モードの要求を既定で断り、呼び出し側が明示した呼び出しだけを通す。画面は、Cookieを読むcomposableを見て、閉じたページをエラー画面に差し替え、ヘッダーに印を出す。

**Tech Stack:** Nuxt 4（Vue 3、Nitro）、TypeScript、Playwright。APIとデータベースは変えない。

**Spec:** [docs/superpowers/specs/2026-11-24-streaming-mode-design.md](../specs/2026-11-24-streaming-mode-design.md)。各タスクの前に、対応する節を読む。

## Global Constraints

- 変えるのは`web/`と`docs/`だけ。`api/`は読むだけにする。
- Cookieの名前は`openoj-streaming`、オンの値は`1`、オフはCookieを消す。期限は1年、`SameSite=Lax`、`path=/`、`httpOnly`にしない。
- 断るときの応答は、403と`data: { code: 'streaming_mode' }`。画面は`error.data?.data?.code`または`error.data?.code`で見分ける。
- 既定は「閉じる」。通す呼び出しは、設計書の表にあるものだけにする。
- 画面の文言は設計書のとおりにする。
  - 印：「配信モード」
  - スイッチ：「このブラウザを配信モードにする」
  - 閉じた画面：「配信モード中は表示できません」「このページは、配信に映らないように閉じています。」「ホームへ戻る」「配信モードを切る」
  - 確認：「配信モードを切りますか？」「マイページやスタッフ向けの画面が、このブラウザで見えるようになります。」「切る」「キャンセル」
  - 部品の代わり：「配信モード中は参加登録できません。」「配信モード中は表示できません。」
  - 覆い：「読み込み直す」
- コメントは英語で、既存のファイルの書き方に合わせる。
- コミットは`prefix: 対応内容`の形式で、タスクごとに行う。末尾に`Co-Authored-By: Claude Fable 5.2 <noreply@anthropic.com>`を付ける。
- このリポジトリは公開されている。鍵やリソースのIDを書かない。
- ブランチ`feat/streaming-mode`で作業する。pushはしない（mainへのpushは自動で配布される）。

### テストの流し方

Playwrightはビルド済みの`.output`に対して走る。コードを変えたら、流す前に必ずビルドする。
`web/test-results`は書き込めないので、出力先を別に指定する。

```bash
cd web
nix develop .. --command bash -c 'bun run typecheck && bun run build'
nix develop .. --command bun run test tests/streaming-server.spec.ts --output="$TMPDIR/pw" --reporter=line
```

`$TMPDIR`が空なら、書き込める一時ディレクトリを指定する。
`bun test`はPlaywrightではない。必ず`bun run test`を使う。

テストでのログインは、Cookie `openoj_access=valid-access`で表す。フィクスチャのAPI（`web/tests/fixtures/public-api.mjs`）はこのトークンだけを通す。

## Review Focus

仕様が含意しているが、放っておくとテストされない条件。それぞれ、持ち主のタスクにテストを足してある。

1. アクセス用のCookieが切れ、更新用のCookieだけが残っている配信モードの要求でも、`GET /api/auth/me`と提出が通る（Task 1）。
2. Cookieの値が`1`以外（`0`、`true`、空）のときは配信モードにしない（Task 1）。
3. スタッフかどうかの読み取りが401（セッション切れ）のときは、失敗にせず公開の経路で答える（Task 2）。
4. サンプル検証の結果に付く判定コードの診断は、配信モードでも残る（Task 2）。
5. 閉じたページへは、URLの直接入力だけでなく、サイト内のリンクとブラウザの「戻る」で来ても閉じた画面になる（Task 4）。

---

### Task 1: Webサーバーで既定を「閉じる」にする

**Files:**
- Create: `web/shared/utils/streaming-mode.ts`
- Create: `web/server/utils/streaming-mode.ts`
- Modify: `web/server/utils/private-api.ts`（`privateAPI`の引数と冒頭）
- Modify: `web/server/api/auth/[action].ts`、`web/server/api/my/profile.ts`、`web/server/api/my/submissions/index.ts`、`web/server/api/my/submissions/[id].get.ts`、`web/server/api/my/code-runs/index.post.ts`、`web/server/api/my/code-runs/latest.get.ts`、`web/server/api/my/solved-problems.get.ts`、`web/server/api/my/favorites/[id].ts`、`web/server/api/my/difficulty-votes/[id].ts`、`web/server/utils/problem-submissions.ts`、`web/server/api/images/[id].get.ts`
- Test: `web/tests/streaming-server.spec.ts`（新規）

**Interfaces:**
- Produces（`web/shared/utils/streaming-mode.ts`）:
  - `export const streamingCookie = 'openoj-streaming'`
  - `export const streamingCode = 'streaming_mode'`
- Produces（`web/server/utils/streaming-mode.ts`）:
  - `export function streamingMode(event: H3Event): boolean`：Cookieの値がちょうど`'1'`のとき真。
  - `export function refuseStreaming(): never`：`createError({ statusCode: 403, statusMessage: 'Streaming mode', data: { code: streamingCode } })`を投げる。
- Produces（`privateAPI`の`options`）: `streaming?: true`。「配信モードでも通す」の明示。

- [ ] **Step 1: 失敗するテストを書く**

`web/tests/streaming-server.spec.ts`に、`request`でWebサーバーの経路を直接たたくテストを書く。
Cookieは要求ヘッダーで渡す。更新を伴う要求には`origin: 'https://judge.example'`を付ける（`requireSameOrigin`が見る）。
`problemId`と`submissionId`は、`web/tests/submission.spec.ts`の冒頭と同じ値を使う。

```ts
const on = 'openoj_access=valid-access; openoj-streaming=1'
const off = 'openoj_access=valid-access'

test('a streaming request is refused on every route not marked', async ({ request }) => {
  for (const path of ['/api/my/contests/drafts', '/api/my/notifications', '/api/my/problems', '/api/my/contest-schedule', '/api/my/account']) {
    const response = await request.get(path, { headers: { cookie: on } })
    expect(response.status(), path).toBe(403)
    expect((await response.json()).data).toEqual({ code: 'streaming_mode' })
    expect((await request.get(path, { headers: { cookie: off } })).status(), path).not.toBe(403)
  }
  const update = await request.put('/api/my/profile', { headers: { cookie: on, origin: 'https://judge.example' }, data: { handle: 'alice' } })
  expect(update.status()).toBe(403)
})

test('a streaming request keeps the marked routes', async ({ request }) => {
  for (const path of ['/api/auth/me', '/api/my/profile', '/api/my/solved-problems', '/api/my/code-runs/latest', `/api/my/favorites/${problemId}`, `/api/my/difficulty-votes/${problemId}`, `/api/my/submissions/${submissionId}`]) {
    expect((await request.get(path, { headers: { cookie: on } })).status(), path).not.toBe(403)
  }
  const submitted = await request.post('/api/my/submissions', { headers: { cookie: on, origin: 'https://judge.example' }, data: { problemId, runtime: 'cpp23-gcc', source: 'int main(){}' } })
  expect(submitted.status()).toBe(202)
})
```

ほかに次の名前のテストを同じファイルに書く。期待は名前のとおり。

- `only the value 1 turns streaming mode on`：Cookieが`openoj-streaming=0`、`=true`、`=`（空）のとき、`/api/my/notifications`が403にならない。
- `a session with only the refresh cookie still answers in streaming mode`：Cookieが`openoj_refresh=valid-refresh; openoj-streaming=1`のとき、`/api/auth/me`が`{ user: { id: 'session-user' } }`を返す。
- `login works in streaming mode`：`POST /api/auth/login`にCookie `openoj-streaming=1`を付けても、ログインに成功する。既存のログインのテスト（`web/tests`で`/api/auth/login`を呼んでいるもの）の要求の形に合わせる。
- `the own list of a problem stays private, the whole list and a detail turn public`：`/api/problems/{featuredProblem.id}/submissions?mine=1`はフィクスチャの非公開の応答（`author: 'carol'`の行）を返し、`mine=0`と詳細はそれを返さない。フィクスチャの`featuredProblem`のIDは`public-api.mjs`から読む。
- `an image is read from the public route in streaming mode`：フィクスチャに画像を1枚足す（`/images/{id}`と`/my/images/{id}`で中身が違うもの）。配信モードの応答が公開側の中身と一致する。画像の経路が受け付ける形式は`web/server/api/images/[id].get.ts`を読んで合わせる。

- [ ] **Step 2: ビルドして流し、失敗を確かめる**

Run: 上の「テストの流し方」のコマンド
Expected: FAIL（403を期待した要求が200を返す）

- [ ] **Step 3: 定数とサーバーの判定を作る**

`web/shared/utils/streaming-mode.ts`と`web/server/utils/streaming-mode.ts`を、Interfacesのとおりに作る。

- [ ] **Step 4: `privateAPI`で既定を閉じる**

`privateAPI`の冒頭、トークンを読む前に置く。

```ts
// Streaming mode refuses every call its route has not marked, so a new private route starts closed. A call with its
// own token is a login in progress, not the browser's session.
if (options.token === undefined && !options.streaming && streamingMode(event)) refuseStreaming()
```

- [ ] **Step 5: 通す呼び出しに`streaming: true`を付ける**

設計書の表のとおり。条件が付くものは次のとおり。

- `auth/[action].ts`：`token`を渡していない3つの呼び出し（`/auth/me`、登録系、ログイン系）に付ける。
- `my/profile.ts`：`event.method === 'GET'`のときだけ付ける。
- `utils/problem-submissions.ts`：配信モードでは、`query.mine === '1'`かつ`submission`がないときだけ`/my`の経路を呼び、そこに付ける。それ以外は初めから`publicContent`で答える。
- `images/[id].get.ts`：配信モードでは`/my/images/{id}`を呼ばず、公開の経路だけを読む。このファイルは403で公開に切り替えないので、`privateAPI`に断らせない。

- [ ] **Step 6: ビルドして流し、通ることを確かめる**

Run: 同じコマンド
Expected: PASS

- [ ] **Step 7: 既存のテストを流す**

Run: `nix develop .. --command bun run test --output="$TMPDIR/pw" --reporter=dot`
Expected: 全件PASS（配信モードのCookieがない要求の動きは変わらない）

- [ ] **Step 8: コミット**

```bash
git add web/shared/utils/streaming-mode.ts web/server web/tests
git commit -m "feat: 配信モードの要求では、明示した呼び出しだけをAPIへ通す"
```

---

### Task 2: コンテストのスタッフと、自分の提出の詳細

**Files:**
- Modify: `web/server/utils/streaming-mode.ts`
- Modify: `web/server/api/contests/[...path].get.ts`
- Modify: `web/server/api/my/submissions/[id].get.ts`
- Modify: `web/tests/fixtures/public-api.mjs`
- Test: `web/tests/streaming-server.spec.ts`

**Interfaces:**
- Consumes: `streamingMode`、`refuseStreaming`、`privateAPI(…, { streaming: true })`（Task 1）
- Produces: `export async function streamingContestView(event: H3Event, id: string): Promise<'participant' | 'public'>`
  - セッションがなければ`'public'`。
  - `/my/contests/{id}`を`streaming: true`で読む。応答の`staff`、`setter`、`owner`、`canEdit`、`canViewLiveStandings`、`canRevealResults`のどれかが真なら`'public'`、どれも真でなければ`'participant'`。
  - 読み取りが401か403なら`'public'`。それ以外の失敗は、そのまま投げる。

- [ ] **Step 1: フィクスチャにコンテストを2つ足す**

`public-api.mjs`に足す。IDは既存のものと重ならない形式の正しいUUIDにする。

- スタッフのコンテスト：`/my/contests/{id}`が`{ id, staff: true, canViewSubmissions: true, … }`を返す。その下の経路（`/my/contests/{id}/…`）は、既存の`6666…`と同じく`{ route: 'private', path, query }`を返す。公開側（`/contests/{id}`と`/contests/{id}/…`）は`{ route: 'public', path, query }`を返す。
- 読めないコンテスト：`/my/contests/{id}`が500を返す。公開側は`{ route: 'public', … }`を返す。

既存の`6666…`は、`/my/contests/6666…`（末尾の`/`なし）が既定の`{ items: [], nextCursor: '' }`を返すので、スタッフでない人のコンテストとして使える。

- [ ] **Step 2: 失敗するテストを書く**

`streaming-server.spec.ts`に足す。`on`と`off`はTask 1のもの。

```ts
test('staff of a contest get the public answers in streaming mode', async ({ request }) => {
  for (const path of ['', '/submissions', `/submissions/${submissionId}`, `/problems/${problemId}`, `/problems/${problemId}/submissions`, '/standings?live=1']) {
    const streaming = await (await request.get(`/api/contests/${staffContest}${path}`, { headers: { cookie: on } })).json()
    expect(streaming.route, path).toBe('public')
    const usual = await (await request.get(`/api/contests/${staffContest}${path}`, { headers: { cookie: off } })).json()
    expect(usual.route ?? 'private', path).toBe('private')
  }
  for (const mine of ['1', 'team']) {
    const own = await request.get(`/api/contests/${staffContest}/submissions?mine=${mine}`, { headers: { cookie: on } })
    expect(own.status()).toBe(403)
    expect((await own.json()).data).toEqual({ code: 'streaming_mode' })
  }
})

test('a participant keeps the private answers in streaming mode, but never the live standings', async ({ request }) => {
  const contest = '66666666-6666-4666-8666-666666666666'
  for (const path of ['/submissions', '/submissions?mine=1', '/submissions?mine=team', `/problems/${problemId}`]) {
    expect((await (await request.get(`/api/contests/${contest}${path}`, { headers: { cookie: on } })).json()).route, path).toBe('private')
  }
  expect((await (await request.get(`/api/contests/${contest}/standings?live=1`, { headers: { cookie: on } })).json()).route).toBe('public')
})

test('a failed staff check uses neither route', async ({ request }) => {
  const response = await request.get(`/api/contests/${brokenContest}/submissions`, { headers: { cookie: on } })
  expect(response.status()).toBe(502)
})
```

スタッフのコンテストの1件目（`path`が空）は、スタッフの応答が`route`を持たない。通常時の期待はその形に合わせて書く。

ほかに次の名前のテストを書く。

- `an expired session reads the public route`：Cookieが`openoj_access=expired; openoj-streaming=1`のとき、`/api/contests/{staffContest}/submissions`が`route: 'public'`を返す（フィクスチャは未知のトークンに401を返す）。
- `the own submission detail drops the checker diagnostics in streaming mode`：フィクスチャに`/my/submissions/{id}`を2件足す。1件は`easyTest`なしで`result.checkerLog: 'secret-checker-log'`、もう1件は`easyTest: true`で`result.checkerLog: 'sample-diagnostics'`。配信モードでは前者の応答に`checkerLog`がなく、後者には残る。配信モードでなければ前者にも残る。

- [ ] **Step 3: ビルドして流し、失敗を確かめる**

Expected: FAIL（スタッフのコンテストが`route: 'private'`を返す）

- [ ] **Step 4: `streamingContestView`を実装する**

Interfacesのとおり。

- [ ] **Step 5: `contests/[...path].get.ts`を、設計書「コンテストのページ」の5つの手順にする**

配信モードでないときの分岐は変えない。配信モードのときは次の順。

1. `live=1`の分岐は通らず、公開の経路へ進む。
2. `streamingContestView(event, id)`を呼ぶ。`id`は`path`の先頭のUUID。
3. `'public'`なら、`mine`が`1`か`team`のとき`refuseStreaming()`、それ以外は`publicContent`で答える。
4. `'participant'`なら、いまの`/my/contests/…`の呼び出しに`streaming: true`を付けて進む。

- [ ] **Step 6: 自分の提出の詳細から診断を落とす**

`my/submissions/[id].get.ts`：配信モードで、応答が`easyTest`でなければ、`result.checkerLog`を消してから返す。
`api/internal/submissions/store.go`の`Get`を読み、作者とテスターにだけ付く項目がほかにないことを確かめる（2026-11-24時点では、結果の`checkerLog`だけである）。

- [ ] **Step 7: ビルドして流し、通ることを確かめる**

Expected: PASS。続けて全件を流し、既存のテストが通ることも確かめる。

- [ ] **Step 8: コミット**

```bash
git add web/server web/tests
git commit -m "feat: 配信モードでは、コンテストのスタッフに公開の内容を返す"
```

---

### Task 3: スイッチ、ヘッダーの印、オフの確認

**Files:**
- Create: `web/app/composables/useStreamingMode.ts`
- Create: `web/app/components/StreamingSettings.vue`、`web/app/components/StreamingBadge.vue`、`web/app/components/StreamingOffDialog.vue`
- Modify: `web/app/pages/my/settings.vue`、`web/app/layouts/default.vue`、`web/app/components/AccountMenu.vue`
- Modify: `web/app/composables/useNotificationBell.ts`、`web/app/composables/useEventAlarms.ts`、`web/app/composables/useBrowserNotifications.ts`
- Test: `web/tests/streaming-mode.spec.ts`（新規）

**Interfaces:**
- Consumes: `streamingCookie`（Task 1）
- Produces（`useStreamingMode()`の戻り値）:
  - `streaming: ComputedRef<boolean>`：Cookieが`'1'`のとき真。`useCookie`で読むので、サーバーでの描画でも同じ値になる。
  - `turnOn(): void`：Cookieを書き、ページを読み込み直す。
  - `askOff(): void`：オフの確認を開く。
  - `turnOff(): void`：Cookieを消し、ページを読み込み直す。確認の「切る」だけが呼ぶ。
  - `offOpen: Ref<boolean>`：確認が開いているか。`useState`で持ち、`StreamingOffDialog`が見る。
- Produces: `StreamingOffDialog`は`default.vue`に1つだけ置く。印、閉じた画面、設定のスイッチは、どれも`askOff()`を呼ぶ。

- [ ] **Step 1: 失敗するテストを書く**

`web/tests/streaming-mode.spec.ts`に書く。ファイルの冒頭に、次の2つの補助を置く。

- `signIn(page)`：既存のテストと同じく、`page.route`で`**/api/auth/me`に`{ user: { id: 'alice' } }`、`**/api/my/profile`にaliceのプロフィールを返す。
- `streamingOn(context)`：`context.addCookies([{ name: 'openoj-streaming', value: '1', url: 'http://127.0.0.1:13000' }])`。

```ts
test('the settings switch turns streaming mode on, and off only after a confirmation', async ({ page, context }) => {
  await signIn(page)
  await page.goto('/my/settings?tab=streaming')
  const badge = page.getByRole('button', { name: '配信モード', exact: true })
  await expect(badge).toHaveCount(0)
  await page.getByRole('switch', { name: 'このブラウザを配信モードにする' }).check()
  await expect(badge).toBeVisible()
  expect((await context.cookies()).find(cookie => cookie.name === 'openoj-streaming')?.value).toBe('1')
  await page.reload()
  await expect(badge).toBeVisible()
  await page.getByRole('switch', { name: 'このブラウザを配信モードにする' }).uncheck()
  const dialog = page.getByRole('dialog', { name: '配信モードを切りますか？' })
  await expect(dialog).toContainText('マイページやスタッフ向けの画面が、このブラウザで見えるようになります。')
  await dialog.getByRole('button', { name: 'キャンセル', exact: true }).click()
  await expect(badge).toBeVisible()
  await expect(page.getByRole('switch', { name: 'このブラウザを配信モードにする' })).toBeChecked()
  await badge.click()
  await dialog.getByRole('button', { name: '切る', exact: true }).click()
  await expect(badge).toHaveCount(0)
  expect((await context.cookies()).some(cookie => cookie.name === 'openoj-streaming')).toBe(false)
})
```

ほかに次の名前のテストを書く。

- `streaming mode hides 作成, the bell and マイページ, and reads no notifications`：配信モードでホームを開く。「作成」、通知のベル、アカウントメニューの「マイページ」がなく、「設定」と「ログアウト」はある。`/api/my/notifications`と`/api/my/contest-schedule`への要求が、開いてから数秒たっても1件も出ない。
- `the badge shows on a phone and on a contest page`：幅375pxでホームとコンテストのページ（`/contests/88888888`の短いID）を開き、印が見えて、画面の幅からはみ出さない。
- `a signed-out visitor turns streaming mode off from the badge`：未ログインで配信モードのCookieを入れ、印から切れる。
- `streaming mode survives a logout`：ログアウトしたあとも印が出ている。
- `the settings page shows only 配信モード in streaming mode`：`/my/settings?tab=identity`を開いても、項目は「配信モード」だけで、プロフィールの入力欄がない。

- [ ] **Step 2: ビルドして流し、失敗を確かめる**

Expected: FAIL（スイッチが見つからない）

- [ ] **Step 3: `useStreamingMode`を実装する**

Interfacesのとおり。Cookieの属性はGlobal Constraintsのとおり。読み込み直しは`location.reload()`を使う（Nuxtの状態を全部捨てるため）。

- [ ] **Step 4: 設定の項目を足す**

- `settings.vue`の`tabs`に`['streaming', '配信モード']`を、`notifications`の次、`deletion`の前に足す。
- 配信モード中は、`tabs`を`streaming`だけにし、`tab`は常に`streaming`を返す。
- `StreamingSettings.vue`：`InlineSwitch`でスイッチを置く。オンにする操作は`turnOn()`、オフにする操作は`askOff()`を呼び、スイッチの見た目は確認が済むまでオンのままにする（`BrowserNotificationSettings.vue`の`switchKey`のやり方を使う）。
- 説明は設計書「設定」の3点を、箇条書きで短く書く。

- [ ] **Step 5: 印と確認を作り、ヘッダーに置く**

- `StreamingBadge.vue`：`<button type="button">`で「配信モード」。押すと`askOff()`。赤系の色は、既存のエラー表示（`.notice-error`）の色に合わせる。
- `StreamingOffDialog.vue`：`SavedProblems.vue`の取り下げの確認と同じ作り（`<dialog>`、`v-dialog-dismiss`、`leave-dialog`）。文言はGlobal Constraintsのとおり。「キャンセル」に`autofocus`を付ける。
- `default.vue`：`.header-tools`の先頭、`ThemeSelect`の左に印を置く。配信モード中は「作成」メニューと`NotificationBell`を描かない。`StreamingOffDialog`を1つ置く。
- スマホ幅では、印が折り返しや横スクロールで隠れないようにする。
- `AccountMenu.vue`：配信モード中は「マイページ」のリンクを描かない。

- [ ] **Step 6: 通知を止める**

`useNotificationPolling`、`useEventAlarms`、ブラウザの通知を出す処理は、配信モード中は通信も表示もしない。
`usePolling`の`enabled`に条件を足すなど、各composableの入口で止める。

- [ ] **Step 7: ビルドして流し、通ることを確かめる**

Expected: PASS。続けて全件を流す。

- [ ] **Step 8: コミット**

```bash
git add web/app web/tests
git commit -m "feat: 設定で配信モードを切り替え、ヘッダーに印を出す"
```

---

### Task 4: 閉じたページ

**Files:**
- Modify: `web/shared/utils/streaming-mode.ts`
- Create: `web/app/middleware/streaming.global.ts`
- Modify: `web/app/error.vue`、`web/app/layouts/default.vue`
- Test: `web/tests/streaming-mode.spec.ts`

**Interfaces:**
- Consumes: `useStreamingMode()`（Task 3）、`streamingCode`（Task 1）
- Produces: `export function closedInStreaming(path: string): boolean`（`web/shared/utils/streaming-mode.ts`）
  - 真になるパス：`/my`と`/my/`で始まるもの（ただし`/my/settings`と`/my/submissions/{1区切り}`は偽）、`/problems/new`、`/problems/post`、`/featured/apply`、`/blog/new`、`/onboarding`、`/contests/{1区切り}/team`。
  - 末尾の`/`の有無で結果を変えない。

- [ ] **Step 1: 失敗するテストを書く**

```ts
const closedPages = ['/my', '/my/contests', '/my/contests/new', `/my/contests/${shortId(contestId)}`, '/my/contest-invitations/token', '/my/tester-invitations/token', '/problems/new', '/problems/post', '/featured/apply', '/blog/new', '/onboarding', `/contests/${shortId(contestId)}/team`]

for (const path of closedPages) {
  test(`streaming mode closes ${path}`, async ({ page, context }) => {
    await signIn(page)
    await streamingOn(context)
    const privateReads: string[] = []
    page.on('request', request => { if (new URL(request.url()).pathname.startsWith('/api/my/') && !request.url().endsWith('/api/my/profile')) privateReads.push(request.url()) })
    await page.goto(path)
    await expect(page.getByRole('heading', { name: '配信モード中は表示できません', exact: true })).toBeVisible()
    await expect(page.getByText('このページは、配信に映らないように閉じています。')).toBeVisible()
    await expect(page.getByRole('button', { name: '配信モード', exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: 'ホームへ戻る' })).toHaveAttribute('href', '/')
    expect(privateReads).toEqual([])
  })
}
```

`/onboarding`は、プロフィールのあるアカウントだと配信モードでなくても別のページへ送られる。配信モードで閉じた画面になることだけを確かめれば足りる。

ほかに次の名前のテストを書く。

- `closedInStreaming keeps the own submission and the settings open`：`closedInStreaming`を直接呼ぶ。`/my/settings`、`/my/submissions/abc`、`/`、`/problems`、`/contests/abc`、`/contests/abc/A`は偽。`/my/`、`/my/new-page`、`/my/submissions`、`/contests/abc/team/`は真。
- `a link and the back button also land on the closed screen`：配信モードでホームを開き、画面内の遷移で`/problems/new`へ移る。`page.evaluate(() => document.querySelector('#__nuxt').__vue_app__.config.globalProperties.$router.push('/problems/new'))`を使う。閉じた画面になる。「ホームへ戻る」でホームへ移り、`page.goBack()`でも閉じた画面になる。
- `配信モードを切る on the closed screen returns the page`：`/my`の閉じた画面で「配信モードを切る」を押し、確認で「切る」を押すと、同じURLでマイページの中身が出る。
- `pages outside the list stay open in streaming mode`：`/my/settings`、`/`、`/problems`が閉じた画面にならない。

- [ ] **Step 2: ビルドして流し、失敗を確かめる**

Expected: FAIL（マイページの中身が出る）

- [ ] **Step 3: `closedInStreaming`を実装する**

Interfacesのとおり。

- [ ] **Step 4: ミドルウェアを作る**

`streaming.global.ts`：配信モードで`closedInStreaming(to.path)`なら、`createError({ statusCode: 403, statusMessage: 'Streaming mode', data: { code: streamingCode }, fatal: true })`でナビゲーションを止める。
サーバーでもクライアントでも同じに動かす。

`profile.global.ts`は、配信モードで`closedInStreaming(to.path)`なら、冒頭で何もせずに戻る。
閉じた画面を出したあとに、ログインやプロフィール登録への案内で別のページへ送らないためである。

- [ ] **Step 5: エラー画面とレイアウトを直す**

- `error.vue`：`error.statusCode === 403`で`data`の`code`が`streamingCode`のとき、設計書「閉じたページ」の文言を出す。「配信モードを切る」は`<button>`で`askOff()`を呼ぶ。「ホームへ戻る」は`/`へのリンクで、`clearError({ redirect: '/' })`でエラーを消して移る。タイトルは「配信モード中は表示できません | ShareOJ」。
- `default.vue`：エラーを表示しているあいだは、`route.meta.editorLayout`が真でもヘッダーを出す（`pageError`は既にある）。
- 確認で「切る」を押したあとは読み込み直すので、同じURLの中身が出る。

- [ ] **Step 6: ビルドして流し、通ることを確かめる**

Expected: PASS。続けて全件を流す。

- [ ] **Step 7: コミット**

```bash
git add web/app web/shared web/tests
git commit -m "feat: 配信モードでは、マイページと作問画面を閉じた画面にする"
```

---

### Task 5: 公開ページの部品と、閉じた通信の表示

**Files:**
- Modify: `web/app/components/ContestList.vue`、`FeaturedDelivery.vue`、`ProblemRejudge.vue`（と`web/app/composables/useRejudge.ts`）、`ContestParticipation.vue`、`ContestTeamPanel.vue`、`ContestMenu.vue`、`ContestClarifications.vue`、`EventHistory.vue`
- Modify: `web/app/pages/contests/[id]/index.vue`（「自分の提出」と「質問」の表示）
- Test: `web/tests/streaming-mode.spec.ts`

**Interfaces:**
- Consumes: `useStreamingMode().streaming`（Task 3）、`streamingCode`（Task 1）

- [ ] **Step 1: `/api/my/`を呼ぶ箇所を洗う**

Run: `grep -rnoE "/api/my/[A-Za-z0-9_\$\{\}\./\-]*" web/app --include=*.vue --include=*.ts | sort -u`

Task 1で通した経路と、Task 4で閉じたページの中だけで使う経路を除き、残ったものが上のFilesに入っているか確かめる。漏れがあれば、このタスクで同じ扱いにする。

- [ ] **Step 2: 失敗するテストを書く**

- `public pages show no error in streaming mode`：ログインして配信モードで、`/`、`/problems`、`/contests`、`/featured`、問題ページ、開催中のコンテストのページを順に開く。`role="alert"`の要素が1つもなく、ブラウザのコンソールにエラーが出ず、403の応答が1件もない（`page.on('response', …)`で`/api/`への403を集める）。このテストは`page.route`でログインをまねず、Cookie `openoj_access=valid-access`で本物の経路を通す。フィクスチャは`/my/contests/8888…`にコンテストとして読める応答を返さないので、同じ内容を公開側と同じ形で返すように足す（`staff`なし、`participating: false`）。
- `the join panel gives way to a note in streaming mode`：配信モードで、参加していないコンテスト（`99999999…`、開催予定）を開く。「参加する」がなく、「配信モード中は参加登録できません。」が出る。未ログインのときは、いまと同じログインへの案内が出る。
- `the contest menu drops 質問 in streaming mode`：コンテストのメニューに「質問」がない。`?view=clarifications`を直接開くと「配信モード中は表示できません。」が出る。
- `the own list of a staff contest says it is closed`：`**/api/contests/*/submissions?*mine=1*`が403と`{ data: { code: 'streaming_mode' } }`を返すようにして「自分の提出」を開くと、「配信モード中は表示できません。」が出る。
- `the public answer of a contest gives no staff controls`：`page.route`で、コンテストに公開の応答（`staff`なし、`canViewSubmissions: false`、`canViewLiveStandings`なし、`participating: false`）、順位表に`submissionId`付きの行を返す。ログイン中で配信モードのとき、順位表の得点がリンクでなく、「凍結なし」への切り替えがなく、問題ページに提出欄（「提出する」のボタン）がない。スタッフに公開の応答が返ること自体はTask 2が確かめている。
- `a problem page still submits and runs code in streaming mode`：配信モードで問題ページから提出でき、提出結果の画面へ移る。`/run`でコード実行のボタンが押せる。

- [ ] **Step 3: ビルドして流し、失敗を確かめる**

Expected: FAIL

- [ ] **Step 4: 部品を直す**

配信モード中の扱いは次のとおり。どれも`streaming`を見て分ける。

- `ContestList`の自分の下書きと自分のコンテスト、`FeaturedDelivery`、`ProblemRejudge`、`ContestTeamPanel`、`EventHistory`：描かず、通信も出さない。
- `ContestParticipation`：ログイン中で、`settled`でないとき、参加の操作の代わりに「配信モード中は参加登録できません。」を出す。未ログインの案内は変えない。
- `ContestMenu`：`clarifications`の項目を出さない。
- `contests/[id]/index.vue`：`view=clarifications`では`ContestClarifications`を描かず、「配信モード中は表示できません。」を出す。提出一覧の読み取りが`streamingCode`で断られたときも、同じ文を出す。

- [ ] **Step 5: ビルドして流し、通ることを確かめる**

Expected: PASS。続けて全件を流す。

- [ ] **Step 6: コミット**

```bash
git add web/app web/tests
git commit -m "feat: 配信モードでは、公開ページのスタッフ向けの部品を出さない"
```

---

### Task 6: ほかのタブを覆う

**Files:**
- Modify: `web/app/composables/useStreamingMode.ts`
- Create: `web/app/components/StreamingCover.vue`
- Modify: `web/app/layouts/default.vue`
- Test: `web/tests/streaming-mode.spec.ts`

**Interfaces:**
- Consumes: `useStreamingMode()`（Task 3）
- Produces: `useStreamingMode()`に`stale: Ref<boolean>`を足す。いまのCookieが、このタブを読み込んだ時点の値と違うとき真。

- [ ] **Step 1: 失敗するテストを書く**

同じ`context`で2ページ開く。

```ts
test('another tab is covered and reloads when streaming mode turns on', async ({ context }) => {
  const staff = await context.newPage()
  await signIn(staff)
  await staff.goto('/my')
  await expect(staff.getByRole('heading', { name: 'マイページ' })).toBeVisible()
  const settings = await context.newPage()
  await signIn(settings)
  await settings.goto('/my/settings?tab=streaming')
  await settings.getByRole('switch', { name: 'このブラウザを配信モードにする' }).check()
  await expect(staff.getByRole('heading', { name: '配信モード中は表示できません', exact: true })).toBeVisible()
})
```

マイページの見出しの実際の文言は`web/app/pages/my/index.vue`で確かめて合わせる。

ほかに次の名前のテストを書く。

- `a tab that refuses the reload stays covered, and uncovers when the mode returns`：1つ目のタブで`beforeunload`を止める（`page.evaluate`で`addEventListener('beforeunload', e => e.preventDefault())`を足し、`page.on('dialog', d => d.dismiss())`で読み込み直しを断る。Chromiumはページを操作したあとでないと確認を出さないので、先にページ内を1回クリックする）。別のタブでオンにすると、1つ目のタブの中身（マイページの見出し）が見えなくなり、「読み込み直す」のボタンが出る。別のタブでオフに戻すと、覆いが外れて中身が見える。
- `a tab notices the change when it becomes visible`：`BroadcastChannel`を使えなくした状態（`page.addInitScript`で`window.BroadcastChannel = undefined`）でも、`context.addCookies`でCookieを入れてから`visibilitychange`を起こすと覆われる。

- [ ] **Step 2: ビルドして流し、失敗を確かめる**

Expected: FAIL（1つ目のタブがマイページのまま）

- [ ] **Step 3: 知らせと検知を実装する**

- `turnOn()`と`turnOff()`は、Cookieを変えたあと、`BroadcastChannel('openoj-streaming')`に知らせを送ってから読み込み直す。`BroadcastChannel`がない環境では送らない。
- 各タブは、読み込んだ時点のCookieの値を覚える。知らせを受けたとき、`visibilitychange`で表示に戻ったとき、`focus`を得たとき、画面を移るときに、`document.cookie`からいまの値を読み直して`stale`を更新する（`useCookie`の値は別のタブの変更を拾わない）。
- `stale`が真になったら`location.reload()`を呼ぶ。偽に戻ったら何もしない。

- [ ] **Step 4: 覆いを作る**

`StreamingCover.vue`：`stale`が真のあいだ、画面全体を不透明に覆う。中に「読み込み直す」のボタンを置き、押すと`location.reload()`を呼ぶ。
`default.vue`に1つ置く。エディタ用の表示（ヘッダーなし）でも覆えるように、`.site-shell`の直下に置き、`position: fixed`で全面を覆う。
覆っているあいだ、下の内容は`inert`にする。

- [ ] **Step 5: ビルドして流し、通ることを確かめる**

Expected: PASS。続けて全件を流す。

- [ ] **Step 6: コミット**

```bash
git add web/app web/tests
git commit -m "feat: 配信モードを切り替えたら、ほかのタブを覆って読み込み直す"
```

---

### Task 7: ADRとガイド

**Files:**
- Create: `docs/adr/0042-close-staff-views-in-streaming-mode.md`
- Modify: `docs/adr/README.md`
- Modify: `web/app/content/docs/hosting-contests.md`
- Modify: `web/docs-screenshots/guides.spec.ts`
- Create: `web/public/images/docs/streaming-mode.png`（撮影で作る）

- [ ] **Step 1: ADRを書く**

既存のADR（たとえば`docs/adr/0040-favorite-contest-problems-during-contest.md`）の章立てに合わせる。
書くことは、決めたこと（ブラウザごとのCookie、既定で閉じること、スタッフには公開の内容を返すこと）、退けた案（アカウント単位の設定、ログアウトと同じ見え方、APIで権限を落とす案）とその理由、影響（Webだけの配布、認可ではないこと）である。
`docs/adr/README.md`の表に1行足す。状態は`Accepted`。

- [ ] **Step 2: ガイドに節を足す**

`hosting-contests.md`（主催者向け）に節「配信するとき」を足す。見出しのIDは`{#streaming}`。
内容は、入れ方、配信モード中に見えなくなるもの、切り方の3点。箇条書きか表で短く書く。
スタッフは配信モード中、自分のコンテストで提出できないことを1行入れる。
書いた文面は、リポジトリのyomiyasuスキル（`.agents/skills/yomiyasu`）で推敲する。

- [ ] **Step 3: 画像を撮る**

`guides.spec.ts`にテスト`streaming mode`を足す。設定の「配信モード」の項目を開き、スイッチと説明に番号を付けて`streaming-mode`の名前で撮る。既存の`submission form`のテストの書き方に合わせる。

Run: `cd web && nix develop .. --command bash -c 'bun run build && bun run docs:screenshots -g "streaming mode" --output="$TMPDIR/pw-docs" --reporter=line'`
Expected: 1 passed。`web/public/images/docs/streaming-mode.png`ができる。画像を開いて、番号と文言が読めることを確かめる。

ガイドから`![…](/images/docs/streaming-mode.png)`で参照する。

- [ ] **Step 4: 全件を流す**

画像を足したあとはビルドし直す（`tests/guides.spec.ts`が画像の有無を見る）。

Run: `cd web && nix develop .. --command bash -c 'bun run typecheck && bun run build && bun run test --output="$TMPDIR/pw" --reporter=dot'`
Expected: 全件PASS

- [ ] **Step 5: グラフを更新してコミット**

```bash
graphify update .
git add docs web
git commit -m "docs: 配信モードのADRとガイドを書く"
```
