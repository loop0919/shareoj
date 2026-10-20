# B7 日本時間（D19） 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 日時の表示と入力を、閲覧者の環境のタイムゾーンによらず日本時間にする。表示の精度は今のままにする。「日本時間」と書いている箇所は「21:00 (JST)」の形にする（APIの説明文も含む）。

**Architecture:** 2つに分けてコミットする。(A) タイムゾーンを指定していない表示に`timeZone: 'Asia/Tokyo'`を足し、コンテスト編集画面の開始日時の入力と読み込みを、`web/app/utils/contest-form.ts`のJSTの変換に寄せる。(B) 残りの「日本時間」の文言を「(JST)」の表記に置き換える。対象はWebの画面、Webのガイド、APIの`routes.go`の説明文。テストではPlaywrightの`timezoneId: 'UTC'`で日本以外の閲覧者をまね、変更前のコードで落ちることを示す。

**Tech Stack:** Nuxt 4 / Vue 3、TypeScript、Playwright（`timezoneId`）、Go（`api/internal/httpapi`、huma v2のOpenAPI）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/docs/refactor/FIXES.md`のD19の「変更内容」、同じファイルの「守ること」。調査の記録は`.local/fixes/briefs/B7.md`と`.local/fixes/briefs/extra.md`の「## B7」。

## 判定（Ruling）

- Ruling（表記の形）: 「（日本時間）」は、半角スペース1つと半角の「(JST)」に置き換える（例：`提出日時 (JST)`、`2099/09/29 22:00 (JST)に公開します`）。後ろに助詞が続くときも、`(JST)`の後に空白は入れない。理由: 仕様の例「21:00 (JST)」に合わせた。同じ画面の`本文 <span>(Markdown)</span>`（`problems/new.vue`）とも同じ書き方になる。
- Ruling（「23時（日本時間）」の時刻）: 「N時（日本時間）」は「N:00 (JST)」にする（`23時（日本時間）`→`23:00 (JST)`、`22時（日本時間）`→`22:00 (JST)`、`'翌22時' }}（日本時間）`→`'翌22:00' }} (JST)`）。理由: 仕様の例が「21:00 (JST)」で、値（時ちょうど）は変わらない。「日本時間」と書いていない時刻（`翌22時までに`など）は対象外なので、そのまま残す。
- Ruling（文中の「日本時間で」）: 「日本時間で」は「JSTで」にする（`日時はJSTで表示しています。`、`未来の公開日時をJSTで指定してください。`）。理由: 時刻の後ろではないので「(JST)」の形にならない。語だけを置き換えるのがいちばん近い。
- Ruling（コンテスト編集画面の注記）: `日時は {{ timezone || '端末のタイムゾーン' }} で入力します。公開ページでは日本時間で表示します。`を`日時はJSTで入力します。公開ページでもJSTで表示します。`にする。ガイド`hosting-contests.md`の同じ説明は`開始日時はJSTで入力し、公開されるページでもJSTで表示します。`にする。理由: 入力もJSTになるので、端末のタイムゾーンの名前は出さない。ラベル「開始日時」は変えない（多くのテストが`getByLabel('開始日時', { exact: true })`でつかんでいる。仕様が求めているのは「日本時間」と書いた箇所だけ）。
- Ruling（日本以外のタイムゾーン）: テストでは`timezoneId: 'UTC'`を使う。理由: 夏時間がなく、日本より9時間遅い。`2026-09-30T20:00:00Z`のような瞬間が、UTCでは9月30日、日本では10月1日になる。Lambdaの実行環境もUTCなので、SSRの環境とも同じ条件になる。この機械のタイムゾーンは`Asia/Tokyo`なので、ゾーンを指定しないと、変更前のコードでもテストが通ってしまう。
- Ruling（`problems/post.vue`の変換）: 公開予約の入力は今もJSTで変換している（`japanTime`と`` `${value}:00+09:00` ``）。そのため、文言のほかは変えない。コンテストの変換と1か所にまとめることはしない。理由: 挙動の変更とリファクタリングを混ぜない方針（FIXES.md）。
- 表示の精度: 今と同じ書式のオプションに`timeZone: 'Asia/Tokyo'`を足すだけにする。`toLocaleString('ja-JP')`は秒まで（`2026/10/1 5:00:00`）、2桁指定のものは分まで（`2026/10/01 05:00`）、登録月は年と月のまま。計画者がNodeのICUで確かめた出力：`2026-09-30T20:00:00Z`は、UTCで`2026/9/30 20:00:00`・`2026年9月`・`2026/09/30 20:00`、日本時間で`2026/10/1 5:00:00`・`2026年10月`・`2026/10/01 05:00`。`dateStyle: 'medium', timeStyle: 'short'`の日本時間は`2026/10/01 5:00`の形になる。

## Global Constraints

- D19の変更内容だけを行う。ほかの公開API、DBスキーマ、依存ライブラリのバージョン、インフラ構成は変えない。npmやGoのパッケージを足さない。CSSは変えない。
- 認可、公開範囲、提出状態の遷移は変えない。送る瞬間（`startsAt`、`endsAt`、`scheduledAt`のISO文字列）の意味も変えない。変わるのは、入力欄の値をどのタイムゾーンで読むかと、表示のタイムゾーンだけ。
- 2コミット（A：表示と入力、B：表記）。どちらも、変える挙動のテストを先に書き、変更前のコードで失敗することを確かめてから直す。
- テストの削除・スキップ・検証の緩和はしない。古い挙動（端末のタイムゾーンで読む入力、「（日本時間）」の文言）を固定しているテストは、新しい挙動に書き換え、コミットの本文にそう書く。
- 範囲外の問題を見つけたら、直さずに報告する。
- 周りのコードに合わせる。コメントは英語にし、量も周りに合わせる。テスト名は英語にする。
- ほかのまとまり（B1、B5、B6）が先に`ContestEditor.vue`、`routes.go`、`docs_test.go`、`contest-editor-form.spec.ts`、`featured/standings.vue`、`problems/new.vue`を変えている。行番号ではなく、引用した文字列で場所を探す。
- コードを変えたら`graphify update .`を実行する（AGENTS.md）。`graphify-out/`はコミットしない。
- Bashの規則: 変数から組み立てたコマンドは使わない。スクリプトファイルは実行しない。Go・Nodeは`nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command`を付けて実行する。Playwrightの`--output`は`/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7`にする。

## Review Focus

1. 日本以外のタイムゾーンの閲覧者には、すべての日時の表示と、コンテストの開始日時の入力がJSTになる。新しい`web/tests/japan-time.spec.ts`と、`timezoneId: 'UTC'`にした`contest-editor-form.spec.ts`で確かめる。
2. 公開済みのコンテストを開き、何も変えずに保存する。このとき開始と終了の瞬間は変わらず、「未保存の変更」の確認も出ない。UTCで流す`a published contest opens unchanged and saves every setting as loaded`で確かめる。読み込みだけ、または保存だけをJSTにすると、ここで9時間ずれる。
3. 開始日時が空の下書きは、今と同じく`startsAt: null`で保存する（`fromJapanInput('')`はInvalid Date）。`a new draft sends all eleven fields`の2回目の保存と、`contest-form.spec.ts`の新しいテストで確かめる。
4. 日付や月をまたぐ瞬間（UTCでは前日、日本では翌日）でも、入力欄の日付と登録月が日本の日付になる。`contest-form.spec.ts`の`2099-09-30T15:30:00Z`→`2099-10-01T00:30`と、マイページの`2026年10月に登録`で確かめる。
5. 精度と表記が揺れない。秒まで出していた表示は秒まで、分までの表示は分までにする。「(JST)」の前の空白は半角1つにし、全角の括弧は使わない。「日本時間」も残さない。テストは正規表現を使わず、要素の全体を`toHaveText`で完全一致させるか、文を丸ごと`toContainText`で比べる。置き換える文言は、どれも1つ以上のテストで新しい形を固定する。例外は`problems/[id]/index.vue`の`'翌22:00'`の分岐（`editorialRevealAt`がないとき）だけで、今もテストがない（フィクスチャの`featuredProblem`は`editorialRevealAt`を持つ）。この分岐のためにフィクスチャを増やすことはせず、この分岐の表記と、残りがないことは、Task 1のStep 10のgrepで確かめる。

---

### Task 1: D19 日時の表示と入力を日本時間にし、「日本時間」を「(JST)」の表記にする

2コミットに分ける。A（Step 1〜6）が表示と入力、B（Step 7〜11）が表記。

**Files:**
- Modify（A）: `web/app/utils/contest-form.ts`（`localDate`）、`web/app/components/ContestEditor.vue`（`timezone`、`endsAt`、`endLabel`、`onMounted`、`draftBody`、`save`、開催時間の注記）、`web/app/components/NotificationBell.vue`、`web/app/components/EventHistory.vue`、`web/app/components/SubmissionDetail.vue`、`web/app/components/SubmissionHistory.vue`、`web/app/components/SavedPosts.vue`、`web/app/components/SavedProblems.vue`（`updatedLabel`）、`web/app/pages/my/index.vue`、`web/app/content/docs/hosting-contests.md`
- Create（A）: `web/tests/japan-time.spec.ts`
- Test（A）: `web/tests/contest-editor-form.spec.ts`、`web/tests/contest-form.spec.ts`、`web/tests/guides.spec.ts`、`web/tests/account/contests.spec.ts`、`web/tests/account/contest-setters.spec.ts`
- Modify（B）: Step 9の表のファイルと`api/internal/httpapi/routes.go`
- Test（B）: `api/internal/httpapi/docs_test.go`、`web/tests/contest-setters.spec.ts`、`web/tests/contest-teams.spec.ts`、`web/tests/featured.spec.ts`、`web/tests/featured-standings.spec.ts`、`web/tests/partial-scores.spec.ts`、`web/tests/post-problem.spec.ts`、`web/tests/manage-problem.spec.ts`、`web/tests/guides.spec.ts`、`web/tests/navigation.spec.ts`、`web/tests/rejudge.spec.ts`

**Interfaces:**
- Consumes: なし
- Produces: `web/app/utils/contest-form.ts`に`export function fromJapanInput(value: string): Date`を置く。`localDate`は、同じファイル内だけで使う`japanInput(at: string): string`に置き換える。

#### A. 表示と入力

- [ ] **Step 1: 失敗するテストを書く（表示）**

`web/tests/japan-time.spec.ts`を作る。

```ts
import { expect, test } from './fixtures/account'
import { shortId } from '../shared/utils/short-id'

// The browser runs in UTC, nine hours behind Japan, so each instant below falls on another day or month there.
test.use({ timezoneId: 'UTC' })
const at = '2026-09-30T20:00:00Z' // 2026/10/1 5:00 in Japan
const problemId = 'dddddddd-0000-4000-8000-000000000001'
const submissionId = 'dddddddd-0000-4000-8000-000000000002'

test('the my page shows its dates in Japan time in any time zone', async ({ page }) => {
  await page.route('**/api/my/profile', route => route.fulfill({ json: { profile: { handle: 'ui_test', avatar: '', version: 1, createdAt: at } } }))
  await page.route(url => url.pathname === '/api/my/problems', route => route.fulfill({ json: { items: [{ id: problemId, title: '時刻の問題', updatedAt: at }], nextCursor: '' } }))
  await page.route('**/api/my/posts', route => route.fulfill({ json: { items: [{ id: 'dddddddd-0000-4000-8000-000000000003', title: '時刻の記事', publishedVersion: 0, publishedAt: null, updatedAt: at, isOperator: false }], nextCursor: '' } }))
  await page.route('**/api/my/notifications**', route => route.fulfill({ json: { notifications: [{ id: 'dddddddd-0000-4000-8000-000000000004', problemId, title: '時刻の問題', actor: 'bob', kind: 'favorite', createdAt: at }] } }))
  await page.goto('/my')
  await expect(page.getByText('2026年10月に登録', { exact: true })).toBeVisible()
  await expect(page.getByRole('table', { name: '自分の問題' }).locator('tbody time')).toHaveText('2026/10/01 05:00')
  await page.getByLabel('通知（未読1件）').click()
  await expect(page.locator('.notification-panel time')).toHaveText('2026/10/1 5:00:00')
  const tabs = page.getByRole('navigation', { name: 'マイページのコンテンツ' })
  await tabs.getByRole('button', { name: '記事', exact: true }).click()
  await expect(page.getByRole('table', { name: '作成した記事' }).locator('tbody time')).toHaveText('2026/10/01 05:00')
  await tabs.getByRole('button', { name: 'イベント', exact: true }).click()
  await expect(page.getByRole('region', { name: 'イベント', exact: true }).locator('time')).toHaveText('2026/10/1 5:00:00')
})

test('submission times read in Japan time in any time zone', async ({ page }) => {
  const submission = { id: submissionId, problemId, problemVersion: 1, problemTitle: '時刻の問題', runtime: 'cpp17', sourceBytes: 12, status: 'DONE', result: { verdict: 'AC', passed: 1, total: 1 }, createdAt: at }
  await page.route('**/api/my/submissions', route => route.fulfill({ json: { items: [submission] } }))
  await page.route(`**/api/my/submissions/${submissionId}`, route => route.fulfill({ json: { ...submission, source: 'int main(){}', rejudgedAt: '2026-10-31T16:00:00Z' } }))
  await page.goto('/my?tab=submissions')
  await expect(page.getByRole('table', { name: '提出履歴' }).locator('tbody time')).toHaveText('2026/10/1 5:00:00')
  // Rejudged at 16:00 on October 31 in UTC, already November in Japan.
  await page.goto(`/my/submissions/${shortId(submissionId)}`)
  await expect(page.getByRole('table', { name: '提出情報' }).locator('time')).toHaveText(['2026/10/1 5:00:00', '2026/11/1 1:00:00'])
})
```

- [ ] **Step 2: 失敗するテストを書く（入力）**

`web/tests/contest-editor-form.spec.ts`:
- 先頭の`// The date input reads local time, so a fixed zone keeps the instants in the bodies literal.`と`test.use({ timezoneId: 'Asia/Tokyo' })`を、次の2行にする。

```ts
// The date input reads Japan time whatever the browser's zone; a browser in UTC proves it, and the instants in the bodies stay literal.
test.use({ timezoneId: 'UTC' })
```

- `a draft with every setting on, at 0 where it takes a number, opens unchanged`の`await expect(page.getByLabel('開始日時', { exact: true })).toHaveValue('2099-10-01T09:00')`の直後に、次の2行を足す。

```ts
  await expect(page.getByText('日時はJSTで入力します。公開ページでもJSTで表示します。', { exact: true })).toBeVisible()
  await expect(page.locator('.end-time')).toHaveText('終了日時 2099/10/01 10:30（1時間30分）')
```

`web/tests/contest-form.spec.ts`:
- importに`fromJapanInput`を足す（`import { formFromContest, formFromDraft, fromJapanInput, initialContestForm, participationForm } from '../app/utils/contest-form'`）。
- `a draft start reads as local time for the date input`は、古い挙動（端末のタイムゾーンで読む）を固定している。次の2つのテストに置き換える。

```ts
test('a draft start reads as Japan time for the date input', () => {
  expect(formFromDraft(draft({ startsAt: '2099-10-01T03:04:00Z' })).startsAt).toBe('2099-10-01T12:04')
  // 15:30 in UTC is already the next day in Japan.
  expect(formFromDraft(draft({ startsAt: '2099-09-30T15:30:00Z' })).startsAt).toBe('2099-10-01T00:30')
})

test('a typed start reads as Japan time, and an empty input as no time', () => {
  expect(fromJapanInput('2099-10-01T09:00').toISOString()).toBe('2099-10-01T00:00:00.000Z')
  expect(fromJapanInput('').getTime()).toBeNaN()
})
```

- `a published contest reads its length in whole minutes ...`の`toEqual`の`startsAt: expect.any(String)`を`startsAt: '2099-10-01T09:00'`にする。古い挙動を固定している`expect(new Date(form.startsAt).toISOString()).toBe('2099-10-01T00:00:00.000Z')`の行は消す（入力の値を端末のタイムゾーンで読み直していた）。

`web/tests/guides.spec.ts`の末尾に、次のテストを足す（ガイドの説明を固定する。変更前は「使っている端末のタイムゾーンで入力し」なので失敗する）。

```ts
test('the contest guide says start times are typed in JST', async ({ page }) => {
  await page.goto('/docs/hosting-contests')
  await expect(page.locator('article .markdown-body')).toContainText('開始日時はJSTで入力し、公開されるページでもJSTで表示します。')
})
```

`web/tests/account/contests.spec.ts`と`web/tests/account/contest-setters.spec.ts`:
- `localDate`は、端末のタイムゾーンの値を入力欄に入れている。これを次の`japanDate`に置き換え、ファイル内の呼び出しをすべて`japanDate(...)`にする。これは入力の準備で、検証は変わらない。

```ts
// The date input reads Japan time.
function japanDate(value: number) {
  return new Date(value + 9 * 60 * 60 * 1000).toISOString().slice(0, 16)
}
```

- [ ] **Step 3: 変更前のコードで失敗することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`
Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/japan-time.spec.ts tests/contest-editor-form.spec.ts tests/guides.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7`
Expected: FAIL。
- `japan-time.spec.ts`は2件とも失敗する。UTCの`2026年9月に登録`、`2026/9/30 20:00:00`、`2026/09/30 20:00`が出るため。
- `guides.spec.ts`は、足した`the contest guide says start times are typed in JST`だけが失敗する（ガイドが「使っている端末のタイムゾーンで入力し」のまま）。ほかのガイドのテストは通る。
- `contest-editor-form.spec.ts`では次の3件が失敗する。
  - `a new draft sends all eleven fields`：`startsAt`が`2099-10-01T09:00:00.000Z`になる。
  - `a draft with every setting on`：入力欄が`2099-10-01T00:00`になる。
  - `a published contest whose passphrase cannot be read`：入力欄が`2099-10-01T00:00`になる。
- `a published contest opens unchanged and saves every setting as loaded`は、読み込みと保存が同じゾーンなので通ってよい。

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 TZ=UTC npx playwright test tests/contest-form.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7`
Expected: FAIL。このファイルはNodeで関数を直接呼ぶので、`TZ=UTC`で日本以外のゾーンにする。`fromJapanInput`がまだないため、読み込み、または`fromJapanInput is not a function`で失敗する。読み込みが通った場合も、`a draft start reads as Japan time`はUTCの値`2099-10-01T03:04`で失敗し、`a published contest reads its length`は`2099-10-01T00:00`で失敗する。この機械は`Asia/Tokyo`なので、`TZ=UTC`がないと、この2件は変更前でも通ってしまう。Step 5では、`TZ`ありとなしの両方で流して、通ることを確かめる。

- [ ] **Step 4: 表示と入力を直す**

`web/app/utils/contest-form.ts`:
- `localDate`を`japanInput(at: string): string`に置き換える（exportしない）。中身は`problems/post.vue`の`japanTime`と同じ式`new Date(new Date(at).getTime() + 9 * 60 * 60 * 1000).toISOString().slice(0, 16)`にする。コメントには「入力欄は閲覧者のタイムゾーンによらず日本時間の値を持つ。日本には夏時間がないので、ずれは固定」を英語で書く。
- `formFromDraft`と`formFromContest`の`localDate(...)`を`japanInput(...)`にする。
- `export function fromJapanInput(value: string): Date`を足す。入力欄の値を日本時間として読む。中身は`post.vue`と同じ`` new Date(`${value}:00+09:00`) ``にする。空の値ではInvalid Dateになる、とコメントに書く。

`web/app/components/ContestEditor.vue`:
- importに`fromJapanInput`を足す。
- `endsAt`、`draftBody()`、`save()`の`new Date(fields.startsAt)`（3か所）を`fromJapanInput(fields.startsAt)`にする。
- `endLabel`の`new Intl.DateTimeFormat('ja-JP', { dateStyle: 'medium', timeStyle: 'short' })`に`timeZone: 'Asia/Tokyo'`を足す。
- `const timezone = ref('')`を消す。`onMounted(() => { timezone.value = Intl.DateTimeFormat().resolvedOptions().timeZone; void load() })`は`onMounted(() => { void load() })`にする。
- 開催時間の注記`日時は {{ timezone || '端末のタイムゾーン' }} で入力します。公開ページでは日本時間で表示します。`を`日時はJSTで入力します。公開ページでもJSTで表示します。`にする。

表示（書式のオプションは変えず、`timeZone: 'Asia/Tokyo'`だけを足す）:
- `NotificationBell.vue`、`EventHistory.vue`、`SubmissionHistory.vue`の`toLocaleString('ja-JP')`、`SubmissionDetail.vue`の`toLocaleString('ja-JP')`（提出日時とリジャッジ日時の2か所）→`toLocaleString('ja-JP', { timeZone: 'Asia/Tokyo' })`
- `SavedPosts.vue`の更新日時、`SavedProblems.vue`の`updatedLabel`、`pages/my/index.vue`の`joined`→今のオプションの末尾に`timeZone: 'Asia/Tokyo'`を足す（`users/[handle].vue`の`joined`と同じ形）

`web/app/content/docs/hosting-contests.md`: `開始日時は使っている端末のタイムゾーンで入力し、公開されるページでは日本時間で表示します。`を`開始日時はJSTで入力し、公開されるページでもJSTで表示します。`にする。

- [ ] **Step 5: 通ることを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`
Expected: エラーなし

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`
Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/japan-time.spec.ts tests/contest-editor-form.spec.ts tests/contest-form.spec.ts tests/guides.spec.ts tests/post-problem.spec.ts tests/rejudge.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7`
Expected: PASS（`contest-editor-form.spec.ts`の全件を含む。ほかのまとまりがこのファイルに足したテストもUTCで通ること）

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 TZ=UTC npx playwright test tests/contest-form.spec.ts tests/japan-time.spec.ts tests/contest-editor-form.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7`
Expected: PASS（Nodeの関数のテストも、UTCで動くSSRのサーバーでも通る）

Run: `grep -rnE "toLocale(Date|Time)?String\('ja-JP'|DateTimeFormat\(" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app /home/loop/judge/.claude/worktrees/refactor-maintainability/web/shared | grep -v "timeZone: 'Asia/Tokyo'"`
Expected: 出力なし（日時の書式はすべて日本時間を指定している）

Run: `grep -rn "getTimezoneOffset\|resolvedOptions\|new Date(fields.startsAt)" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app /home/loop/judge/.claude/worktrees/refactor-maintainability/web/shared`
Expected: 出力なし

- [ ] **Step 6: グラフを更新してコミットする（A）**

`cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`。変更したファイルをすべて`git add`する。

メッセージ：`fix: 日時の表示とコンテストの開始日時の入力を、閲覧者の環境によらず日本時間にする`

本文には次のことを書く。
- マイページの登録月、通知、イベント、提出履歴と提出結果、保存した問題・記事の更新日時、コンテスト編集画面の終了日時に、日本時間を指定した。表示の精度は変えていない。
- コンテストの開始日時の入力と読み込みを、日本時間に固定した。
- 端末のタイムゾーンで読む入力を固定していた`contest-form.spec.ts`の2件と、アカウント系の`localDate`を、日本時間に書き換えた。
- `contest-editor-form.spec.ts`のタイムゾーンを、`Asia/Tokyo`からUTCに変えた。

#### B. 「日本時間」を「(JST)」の表記にする

- [ ] **Step 7: 失敗するテストを書く（表記）**

`api/internal/httpapi/docs_test.go`の`TestPublicDocumentation`:
- `Paths`の値の構造体（`OperationID`と`Parameters`を持つもの）に`Description string \`json:"description"\``を足す。
- `seen[op.OperationID] = true`の直後に、次を足す。

```go
			// Times in the descriptions read like 21:00 (JST).
			if strings.Contains(op.Description, "日本時間") {
				t.Fatal("time zone written out", path)
			}
```

- `doc.Paths`のループの後に、次を足す。

```go
		if d := doc.Paths["/featured"]["get"].Description; !strings.HasPrefix(d, "毎週月・木23:00 (JST)のEasy/Hard各一問。") {
			t.Fatal("featured description", d)
		}
```

Webのテスト。いずれも、古い文言を固定していた行を新しい文言に書き換えるか、新しい行を足す。
- `contest-setters.spec.ts`の`my contests list every setter ...`:
  - 下書きの見出しを`['下書き', '開始予定 (JST)', '更新 (JST)', 'オーナー', '操作']`にする。
  - `const published = ...`の直後に`await expect(published.getByRole('columnheader')).toHaveText(['コンテスト', '状態', '開始 (JST)', '終了 (JST)', '作成者', '操作'])`を足す。
- `contest-teams.spec.ts`：`['チーム・個人', '区分', 'オンサイトになった時刻 (JST)', '登録日時 (JST)']`
- `partial-scores.spec.ts`：最後の見出しを`'提出日時 (JST)'`にする。
- `featured.spec.ts`:
  - `hidden editorial ...`の文言を`'定期便の新作です。解説と他者の提出は2099/09/29 22:00 (JST)に公開します。'`にする。
  - `featured editions show two slots ...`の`await expect(notes).toContainText('次回予定を含む')`の直後に、次の3行を足す。

```ts
  await expect(notes).toContainText('新作の解説と他者の提出は、出題翌日の22:00 (JST)に公開します。')
  await expect(page.locator('.delivery-kicker')).toHaveText('毎週 月・木 23:00 (JST)')
  await expect(page.locator('meta[name="description"]')).toHaveAttribute('content', '毎週月曜・木曜23:00 (JST)にEasyとHardを一問ずつ出題します。新作の解説は翌22時に公開。')
```

  - `the schedule lists coming rounds ...`の見出しの確認の直後に、`await expect(page.locator('.schedule-note')).toContainText('毎週月曜・木曜の23:00 (JST)に出題します。')`を足す。
- `featured-standings.spec.ts`の`edition standings show tied ranks ...`の見出しの確認の直後に、次を足す。フィクスチャ（`web/tests/fixtures/public-api.mjs`の`featuredStandings`）の`closesAt`は固定の`2099-09-29T13:00:00Z`で、`contestDate`では`2099/09/29 22:00`になる（計画者がNodeで確かめた）。

```ts
  await expect(page.locator('p', { hasText: '集計締め切り：' })).toHaveText('集計締め切り：2099/09/29 22:00 (JST)')
```
- `post-problem.spec.ts`:
  - `getByLabel('公開日時（日本時間）')`（2か所）を`getByLabel('公開日時 (JST)')`にする。
  - `toContainText('未来の公開日時')`を`toContainText('未来の公開日時をJSTで指定してください。')`にする。
  - `schedule in Japan time ...`の`await expect(row).toContainText('23:00')`の直後に、次の2行を足す。

```ts
  await expect(row.locator('.problem-state time')).toHaveText(`${year}/1/2 23:00:00 (JST)`)
  await expect(row).toContainText(`公開予定：${year}/1/2 23:00:00 (JST)`)
```

- `manage-problem.spec.ts`：ファイルの末尾に、次のテストを足す（エディターの「公開設定」の予約中の表示を固定する）。B6のD29が足す`scheduled problems explain where else they cannot go`とは別のテストにし、そちらは変えない。`testing`は作者が自分でないときだけ真になる。`author`を返さないので偽になり、予約を変えるリンクまで同じ段落に出る。`公開予約中です。`を含む段落は`web/app`でここだけ。

```ts
test('a scheduled problem says when it goes public, in JST', async ({ page }) => {
  const id = '44444444-4444-4444-8444-444444444444'
  const draft = { title: '予約済み', markdown: '本文', editorial: '解説', difficulty: 3, timeLimitMs: '2000', memoryLimitMb: '512', testCases: [{ input: '1', output: '1' }] }
  const readiness = { publish: [], contest: ['scheduled'], featured: ['scheduled'] }
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: { id, version: 1, publishedVersion: 0, scheduledAt: '2099-10-01T14:00:00Z', updatedAt: '2026-09-10T00:00:00Z', draft, readiness } }))
  await page.goto(`/problems/new?problem=${id}`)
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  await expect(page.locator('p', { hasText: '公開予約中です。' })).toHaveText('2099/10/1 23:00:00 (JST)に公開予約中です。編集を公開内容に反映するには、保存後に予約を更新してください。予約を変更・取り消す')
})
```

- `guides.spec.ts`の末尾（Step 2で足したテストの後）に、次のテストを足す。

```ts
test('the featured guide gives the delivery time in JST', async ({ page }) => {
  await page.goto('/docs/featured')
  await expect(page.locator('article .markdown-body')).toContainText('定期便は、毎週月曜と木曜の23:00 (JST)に、Easy（Lv.1〜4）とHard（Lv.5〜10）を1問ずつ出題します。')
})
```

- `navigation.spec.ts`：`await expect(page.getByText('開始日時', { exact: true })).toBeVisible()`の直後に、`await expect(page.getByText('日時はJSTで表示しています。', { exact: true })).toBeVisible()`を足す。
- `rejudge.spec.ts`の最初のテスト：`await expect(all).toBeVisible()`の直後に、`await expect(list.locator('thead th').nth(1)).toHaveText('提出日時 (JST)')`を足す。

- [ ] **Step 8: 変更前の文言で失敗することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 -run TestPublicDocumentation ./internal/httpapi/`
Expected: FAIL（`time zone written out /featured`）

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/contest-setters.spec.ts tests/contest-teams.spec.ts tests/partial-scores.spec.ts tests/featured.spec.ts tests/featured-standings.spec.ts tests/post-problem.spec.ts tests/manage-problem.spec.ts tests/guides.spec.ts tests/navigation.spec.ts tests/rejudge.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7`
Expected: FAIL。Step 7で書き換えたり足したりした行のあるテストが、すべて失敗する（ビルドはStep 5のもの）。失敗の仕方は2通りある。
- `post-problem.spec.ts`の`posting keeps the timing beside a long list ...`と`schedule in Japan time ...`は、文言を比べる前に、`getByLabel('公開日時 (JST)')`が要素を見つけられずタイムアウトする（今のラベルは`公開日時（日本時間）`）。
- ほかのテストは、「（日本時間）」「23時」「日本時間で」の残った文言との不一致で失敗する（`manage-problem.spec.ts`と`guides.spec.ts`は、足したテストだけが失敗する）。
- Step 7で何も変えていないテストは通る。

- [ ] **Step 9: 文言を置き換える**

Webの画面（`web/app`）は、次の表のとおりに置き換える。表の外の文字は変えない。

| ファイル | 今 | 後 |
| --- | --- | --- |
| `components/FeaturedDelivery.vue` | `毎週 月・木 23:00 <span>日本時間</span>` | `毎週 月・木 23:00 <span>(JST)</span>` |
| `components/FeaturedDelivery.vue` | `出題翌日の22時（日本時間）に公開します。` | `出題翌日の22:00 (JST)に公開します。` |
| `pages/featured/index.vue` | `毎週月曜・木曜23時（日本時間）に` | `毎週月曜・木曜23:00 (JST)に` |
| `pages/featured/schedule.vue` | `毎週月曜・木曜の23時（日本時間）に出題します。` | `毎週月曜・木曜の23:00 (JST)に出題します。` |
| `pages/featured/standings.vue` | `</time>（日本時間）</p>` | `</time> (JST)</p>` |
| `pages/problems/[id]/index.vue` | `: '翌22時' }}（日本時間）に公開します。` | `: '翌22:00' }} (JST)に公開します。` |
| `pages/problems/new.vue` | `}}（日本時間）に公開予約中です。` | `}} (JST)に公開予約中です。` |
| `pages/problems/post.vue` | `公開日時（日本時間）` | `公開日時 (JST)` |
| `pages/problems/post.vue` | `未来の公開日時を日本時間で指定してください。` | `未来の公開日時をJSTで指定してください。` |
| `components/SavedProblems.vue` | `}}（日本時間）</time>` | `}} (JST)</time>` |
| `utils/problem-readiness.ts` | `` })}（日本時間）` `` | `` })} (JST)` `` |
| `components/ProblemSubmissionList.vue` | `提出日時（日本時間）` | `提出日時 (JST)` |
| `pages/contests/[id]/index.vue` | `日時は日本時間で表示しています。` | `日時はJSTで表示しています。` |
| `pages/contests/[id]/index.vue` | `提出日時（日本時間）` | `提出日時 (JST)` |
| `components/ContestList.vue` | `開始予定（日本時間）`、`更新（日本時間）`、`開始（日本時間）`、`終了（日本時間）` | `開始予定 (JST)`、`更新 (JST)`、`開始 (JST)`、`終了 (JST)` |
| `components/ContestSetterTools.vue` | `オンサイトになった時刻（日本時間）`、`登録日時（日本時間）` | `オンサイトになった時刻 (JST)`、`登録日時 (JST)` |
| `content/docs/featured.md` | `毎週月曜と木曜の23時（日本時間）に` | `毎週月曜と木曜の23:00 (JST)に` |

`api/internal/httpapi/routes.go`の`GET /featured`の説明: `毎週月・木23時（日本時間）のEasy/Hard各一問。`を`毎週月・木23:00 (JST)のEasy/Hard各一問。`にする。説明の残り（`翌22時に公開。`など）は変えない。

- [ ] **Step 10: 通ることと、残りがないことを確かめる**

Run: `grep -rn --exclude='*_test.go' "日本時間" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app /home/loop/judge/.claude/worktrees/refactor-maintainability/web/shared /home/loop/judge/.claude/worktrees/refactor-maintainability/web/server /home/loop/judge/.claude/worktrees/refactor-maintainability/api/internal /home/loop/judge/.claude/worktrees/refactor-maintainability/cli`
Expected: 出力なし。Goのテストファイルは対象から外す。Step 7で`docs_test.go`に足した確認が、文字列`"日本時間"`をそのまま含むため（Webのテストは`web/tests`にあり、もともと対象外）。

Run: `grep -rnE "（JST）|[^ ]\(JST\)" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app /home/loop/judge/.claude/worktrees/refactor-maintainability/api/internal/httpapi/routes.go | grep -v "23:00 <span>(JST)</span>"`
Expected: 出力なし（全角の括弧も、前に空白のない「(JST)」もない。`FeaturedDelivery.vue`の`<span>`は、前の空白が`<span>`の外にあるので除く）

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 -run TestPublicDocumentation ./internal/httpapi/`
Expected: `ok`

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .`
Expected: 出力なし

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`
Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/contest-setters.spec.ts tests/contest-teams.spec.ts tests/partial-scores.spec.ts tests/featured.spec.ts tests/featured-standings.spec.ts tests/post-problem.spec.ts tests/manage-problem.spec.ts tests/guides.spec.ts tests/navigation.spec.ts tests/rejudge.spec.ts tests/japan-time.spec.ts tests/contest-editor-form.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7`
Expected: PASS。あわせて、`contest-teams.spec.ts`と`contest-setters.spec.ts`にある狭い画面の横スクロールの確認も通る。

- [ ] **Step 11: グラフを更新してコミットする（B）**

`cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`。変更したファイルをすべて`git add`する。

メッセージ：`fix: 「日本時間」の表記を「21:00 (JST)」の形にそろえる（APIの説明文を含む）`

本文には次のことを書く。
- 「（日本時間）」は「 (JST)」にした。「N時（日本時間）」は「N:00 (JST)」に、文中の「日本時間で」は「JSTで」にした。
- 古い文言を固定していたWebのテストを、新しい文言に書き換えた。

## 検証コマンド（controllerが全体を流す）

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`→エラーなし
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`→エラーなし
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7`→すべてpassed
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 TZ=UTC npx playwright test tests/contest-form.spec.ts tests/japan-time.spec.ts tests/contest-editor-form.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7-utc`→すべてpassed（サーバーとNodeがUTCでも、日本時間になる）
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 2400 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test --config playwright.account.config.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b7-account`→すべてpassed（`japanDate`で入れた開始日時で公開できる）
6. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run package:lambda`、同じ形で`npm run test:lambda`と`node --test scripts/smoke-frontend.test.mjs`→どれも終了コード0
7. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 ./...`→すべて`ok`
8. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...`→警告なし。同じ形で`gofmt -l .`→出力なし（`routes.go`の説明文を変えたため）
9. Task 1のStep 5とStep 10の`grep`（Step 10の1つ目は`--exclude='*_test.go'`付き）→どれも出力なし
10. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short && git log --oneline -3`→作業ツリーはきれい（計画ファイルを除く）。Aのコミット、Bのコミットの順に並ぶ

## Out of scope（報告のみ）

- 開発者向けの文書と記録の「日本時間」（`README.md:157`、`api/README.md:692`、`docs/adr/0011-schedule-featured-problems.md`、`docs/judge/ec2-pool.md`）。利用者に見せる表示ではない。ADRと記録は書き換えない方針。
- 「日本時間」と書いていない時刻の表記（`翌22時までに`、`翌22時に公開`など。`featured/standings.vue`、`featured/index.vue`、`routes.go`の`/featured`と`/featured/standings`の説明）。仕様の対象外なので、「22時」のまま残す。同じ文の中で「23:00 (JST)」と「翌22時」が並ぶ箇所がある。
- 「日本時間」と書いていない日時の見出し（提出結果の「提出日時」、コンテスト編集画面の「開始日時」など）には、「(JST)」を足さない。
- `problems/post.vue`の公開予約の変換（`japanTime`、`` `${value}:00+09:00` ``）。すでに日本時間なので変えない。コンテストの変換と同じ式が2か所にあるが、まとめるのはリファクタリングにあたるので、この作業ではしない。
- `web/public/images/docs`のガイドのスクリーンショットに、古い文言（「日本時間」や端末のタイムゾーンの注記）が写っている可能性がある。撮り直し（`npm run docs:screenshots`）は、この作業ではしない。
- CLIは日時を表示も入力もしない。変更はない。
- `FIXES.md`の「まとまりと順序」の状態とコミットの欄は、controllerが更新する。
