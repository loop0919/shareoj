# B6 Webのその他（D17、D18、D20、D23、D26、D29、D31、D32、D33のWeb） 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Webの画面の説明・表示・戻り先・エラー文言の食い違い9件を、利用者が決めた変更内容どおりに直す。

**Architecture:** どれもNuxtの画面・共有関数・Playwrightのテストに閉じた変更で、API・BFFの許可リスト・DBは触らない。項目ごとに独立していて、同じファイルを触るのはD32とD33（`SubmissionForm.vue`、`submission.spec.ts`）、D18とD20（`featured.spec.ts`）だけ。

**Tech Stack:** Nuxt 4 / Vue 3、TypeScript、Playwright（ブラウザー用とアカウント用の2つの設定）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/docs/refactor/FIXES.md`の各項目の「変更内容」と「守ること」

## 判定（Ruling）

- D17: Ruling: 文言は利用者向けガイド`web/app/content/docs/authoring.md:142-144`の権限の書き方に合わせ、ガイドが書いていない「テスターリンクの発行」も足す — 実際の権限（`api/internal/problems/publication.go:58-59`、`store.go:498`、`testers.go:29`の`can_manage_problem_directly`）と一致し、「…ができます」と並べる文がすべての操作を挙げたものとして読めるようにする。ガイドと問題管理の説明の同じ抜けは範囲外として報告する。
- D18: Ruling: 順位表の列見出しと、正解の印の読み上げ名（`aria-label`）の両方に一覧と同じ式を使う。「順位の決まり方」の「Hardのみ正解」は枠の一般的な説明なので変えない — ADR 0011:25はUltimateを「Hard枠の特別な表示」としている。予定表は触らず、Lv.10の行がHardのままであることをテストに足す。
- D20: Ruling: 消したテストの代わりに、ホームにお知らせがないことと、320pxで横にはみ出さないことを確かめるテストを置く — 変更前（2026-10-31より前）のコードで失敗し、消したテストが見ていた狭い画面の確認も残る。
- D23: Ruling: `useSharePreview`で、画像のパスだけクエリを除いて作る。`og:url`はタグ付きのまま — 画像のルート（`server/routes/og/[...path].get.ts`）はパスだけで画像を決めるので、クエリ付きのパスに画像はない。呼ぶ側を変えずに1か所で済む。`og:image:alt`はほかのページと同じくページのタイトルのまま。
- D26: Ruling: 見出しの代わりに「絞り込みを解除」のリンクを待つ — `problems/index.vue:48`のh1は絞り込みの前後どちらも「問題」で、新しいページの目印にならない（違うのは`<title>`だけ）。このリンクはタグ付きの新しいページのTagFilterだけが描く。仕様の文言と違うので、理由をコミットの本文に書く。あわせて、タグ付きの一覧の応答を50ms遅らせるrouteをテストに残す。以前の調査では、同じ手順の再現用テストで応答を25msか50ms遅らせると、20回すべてが失敗した。待つ処理を消すとまた落ちるので、回帰の検出にもなる。
- D31: Ruling: 仕様のとおり、`//`は先頭だけでなくどこに含まれても`/my`にする（クエリ内の`https://`も含む）。「制御文字」はUnicodeの制御文字（Cc）全体とし、C0（U+0000〜U+001F）、U+007F、C1（U+0080〜U+009F）を拒む — 仕様の文言どおりで、`problem-draft.ts:22`や`shared/types/tag.ts:17`が使う既存の判定`/[\u0000-\u001f\u007f-\u009f]/`と同じになる。ログイン後とプロフィール登録後（`ProfileForm.vue`、`profile.global.ts:16`）は同じ関数を通るので、どちらも同じ規則になる。
- D32: Ruling: `invalid_submission`は、提出フォーム（`SubmissionForm.vue`）と入出力生成（`TestCaseGenerator.vue`）の両方に専用の文言を出す — どちらも`/api/my/submissions`へ送ってこのコードを受け取る画面。生成では、画面で事前に確かめていないテストケースの合計サイズが主な原因になるので、それを文言に入れる。`invalid_contest`はコンテスト編集画面の下書き保存・公開・公開済みの保存の3つで同じ文言にする。
- D33: Ruling: まとめるのは`SubmissionForm.vue`の数値の2か所（`:37`と`:99`）だけ。文言の「131,072バイト」（`:38`）と「128 KiBまで」（`:90`）はそのまま — 仕様が挙げたのは2か所で、文言はそのまま表示されるコピーなので。

## Global Constraints

- 各項目の変更内容だけを行う。ほかの公開API、DBスキーマ、依存ライブラリのバージョン、インフラ構成は変えない。npmパッケージを足さない。
- 認可、公開範囲、提出状態の遷移、重複・再実行時の挙動は保つ。BFFの`forwardedCodes`（`web/server/utils/private-api.ts`）は変えない。D21（`invalid_accounts`）は対象外。
- 1件1コミット（`prefix: 対応内容`、日本語）。変える挙動のテストを先に書き、変更前のコードで失敗することを確かめてから直す。どのコミットも、最後の`-m`に`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`を付ける。
- テストの削除・スキップ・検証の緩和はしない。例外は次の2つだけで、コミットメッセージの本文にも書く。D20では、消すお知らせのテスト（`featured.spec.ts`の`home announces 定期便 ...`）を消す。D18では、古い表示を固定している`featured-standings.spec.ts`の`'Hard'`の3か所を新しい表示に直す。
- 範囲外の問題を見つけたら直さずに報告する。「（日本時間）」の表記はB7（D19）で扱うので、この計画では触らない。
- 周りのコードに合わせる。コメントは英語で少なめにし、画面の文言は日本語にする。既存のヘルパーを使う。
- graphifyは実行しない（流れが終わった後にcontrollerが更新する）。`graphify-out/`はコミットしない。
- 行番号は`f66d6b7`の時点のもの。B5（`ContestEditor.vue`、`TestCaseGenerator.vue`）とB4（`problem-draft.ts`）の後に実装するとずれるので、実装の前に読み直し、引用した文字列で場所を探す。B7は後で`featured/standings.vue`と`problem-readiness.ts`を触るので、この計画の変更は最小にとどめる。
- Playwrightのポート（13000、13001、18080、18081など）は全laneで共有する。どのPlaywrightの実行も、下の`PW`・`PWA`の形（Nixのシェルの中で`flock`）でロックを取る。ロック待ちがあるので、Bashのtimeoutは長くする。ビルドはロックなしでよい。
- ブラウザーのPlaywrightは`web/.output`を使う。最初のタスクの前に一度`BUILD`する。アプリのコードを変えたら、通ることを確かめる前にもう一度ビルドする。失敗の確認は、そのタスクの変更前のビルドで行う。テストファイルだけの変更ならビルドは要らない。
- `--output`には必ずscratchpadを渡す（`web/test-results`はrootの持ち物）。
- Bashの規則: 変数から組み立てたコマンドは使わない。スクリプトファイルは実行しない。終了コードはBashの結果から読み、`; echo rc=$?`のような後置きは付けない。Nodeは`nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command`を付けて実行する。laneのworktreeで実装するときは、`cd`先の`/home/loop/judge/.claude/worktrees/refactor-maintainability/web`だけをそのworktreeの`web`に置き換える（`nix develop`の引数とロックのパスはそのまま）。

以下、`PW`・`PWA`・`BUILD`は次のコマンドの略。`<file>`と`<name>`はタスクごとに置き換え、`-g`なしのときは`-g "<name>"`ごと消す。

`PW`（ブラウザー用）:
`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command flock /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/playwright.lock env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test <file> -g "<name>" --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/b6/pw`

`PWA`（アカウント用）:
`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command flock /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/playwright.lock env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test --config playwright.account.config.ts <file> -g "<name>" --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/b6/pw-account`

`BUILD`: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`

## Review Focus

1. D31: ブラウザーが`/`とみなす`\`や、URLの解析で消えるタブ・改行。たとえば`/\attacker.example`や`/\t/attacker.example`は、外部へのリダイレクトになりうるので`/my`にする。Task 7の純粋関数のテストに行がある。
2. D31: 配列の`next`（`?next=a&next=b`）、空文字、相対パス、CRLFやC1の制御文字（U+0085）を含む値は`/my`にする。Googleのcallbackで`Location`ヘッダーに改行が入らないこと。Task 7のテストに行がある。
3. D26: 待つ目印が古いページにもあると、競合は残る。Task 1は遅延を入れた状態で20回流して確かめる。
4. D18: `difficulty`が`null`のHard枠（欠番）とLv.8以下はHardのまま。予定表（`FeaturedSchedule.vue`）は触らない。Task 3は、順位表のLv.7の回のテストと、予定表のLv.10の行（fixtureの2行目）の列見出し・`data-slot`がHardのままであることの確認を足す。
5. D23: `og:url`はタグ付きのままにする。`/`は今までどおり`/og/index.png`。Task 5のテストで`og:url`を確かめ、`/`は既存の`SSR exposes a working PNG preview: /`が守る。

---

### Task 1: D26 タグの絞り込みのテストで、新しいページを待ってから検索フォームをつかむ

**Files:**
- Modify: `web/tests/tags.spec.ts`（`the tag filter reuses the spelling in use`、今の114-132行）

- [ ] **Step 0: 変更前のコードでビルドする**（`BUILD`。この計画で1回目）

- [ ] **Step 1: 競合を再現するrouteを足す**

テストの`await showProblemTags(page)`の直後に足す。

```ts
  // The filtered list answers late, so the old page is still shown when the URL changes.
  await page.route(url => url.pathname === '/api/problems' && url.searchParams.get('tag') === 'DP', async route => {
    await new Promise(resolve => setTimeout(resolve, 50))
    await route.continue()
  })
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `PW`（`<file>`=`tests/tags.spec.ts`、`<name>`=`reuses the spelling`）に`--repeat-each=20`を付ける。
Expected: 大半の回がFAILし、`locator.screenshot: Element is not attached to the DOM`になる。1回も落ちなければ、遅延を25msにして試し、回数を報告する。

- [ ] **Step 3: 新しいページを待つ**

`await expect(page).toHaveURL('/problems?tag=DP')`の直後に足す。

```ts
  // The h1 reads 問題 on both pages; only the filtered page's form offers to clear the tag.
  await expect(page.getByRole('link', { name: '絞り込みを解除' })).toBeVisible()
```

- [ ] **Step 4: 通ることを確かめる**

Run: Step 2と同じ（`--repeat-each=20`）。Expected: 20 passed。続けて`PW`を`<file>`=`tests/tags.spec.ts`、`-g`なしで流す。Expected: すべてPASS。

- [ ] **Step 5: コミット**（本文に、見出しではなくリンクを待つ理由を書く）

```bash
git add web/tests/tags.spec.ts
git commit -m "test: タグの絞り込みのテストで、新しいページの描画を待ってから検索フォームをつかむ" -m "仕様は新しいページの見出しを待つとしているが、h1は絞り込みの前後どちらも「問題」で、新しいページの目印にならない。タグ付きのページのTagFilterだけが描く「絞り込みを解除」のリンクを待つ。タグ付きの一覧の応答を50ms遅らせるrouteを残し、競合を毎回起こす。" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 2: D17 テスター招待ページの説明を実際の権限に合わせる

**Files:**
- Modify: `web/app/pages/my/tester-invitations/[token].vue:42`
- Test: `web/tests/contest-setters.spec.ts`（`the tester invitation page explains why a registered participant cannot join`の後）

- [ ] **Step 1: 失敗するテストを書く**

```ts
test('the tester invitation page leaves publishing and 定期便 applications to the author', async ({ page }) => {
  await page.route(`**/api/my/tester-invitations/${token}`, route => route.fulfill({ json: { id: own, title: 'コンテストの問題', author: 'alice', joined: false } }))
  await page.goto(`/my/tester-invitations/${token}`)
  await expect(page.getByText(/^テスターとして参加すると/)).toHaveText('テスターとして参加すると、非公開の問題文・解説・テストケースの閲覧や提出に加え、問題の編集・削除・リジャッジ・非公開に戻す操作、テスターリンクの発行、自分のコンテストへの登録ができます。公開と公開予約、定期便への応募は作者だけができます。')
})
```

- [ ] **Step 2: 失敗することを確かめる** — `PW`（`tests/contest-setters.spec.ts`、`leaves publishing`）。Expected: FAIL（今の文は「編集・公開・削除を含む作者と同じ操作ができます」）。

- [ ] **Step 3: 42行目の`<p>`の文をテストの文に置き換える**（タグと属性はそのまま）。

- [ ] **Step 4: 通ることを確かめる** — `BUILD`の後、Step 2と同じ`PW`を流す。Expected: PASS。続けて`<file>`=`tests/contest-setters.spec.ts`を`-g`なしで流し、`tester invitation`の既存テストも含めてPASSすること。

- [ ] **Step 5: コミット**

```bash
git add 'web/app/pages/my/tester-invitations/[token].vue' web/tests/contest-setters.spec.ts
git commit -m "fix: テスター招待ページの説明から公開の権限を消し、実際の権限に合わせる" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 3: D18 定期便の順位表で、条件を満たすHard枠をUltimateと表示する

**Files:**
- Modify: `web/app/pages/featured/standings.vue`（12-13行の`columns`、38-41行の列見出し、47-49行の行のループと`aria-label`）
- Test: `web/tests/featured-standings.spec.ts`、`web/tests/featured.spec.ts`（`the schedule lists coming rounds by level without writers and fits narrow screens`）

- [ ] **Step 1: テストを直し、足す**

`edition standings show tied ranks ...`（fixtureのvol.12で、Hard枠はLv.9）の古い表示を固定している3か所を新しい表示に直す。
- 11行目: `['順位', 'ユーザー', '時間', 'Easy', 'Ultimate']`
- 17行目と21行目: `{ name: 'Ultimate 正解' }`

ファイルの末尾に、Lv.7の回（vol.11）がHardのままであることを確かめるテストを足す。これは変更前から通る。

```ts
test('an edition whose Hard problem is below Lv.9 keeps the Hard label', async ({ page }) => {
  await page.goto(`/featured/standings?at=${encodeURIComponent('2099-09-24T14:00:00Z')}`)
  const table = page.getByRole('table')
  await expect(table.locator('thead th').last()).toHaveText('Hard')
  await expect(table.locator('tbody tr').first().getByRole('img', { name: 'Hard 正解' })).toBeVisible()
})
```

`featured.spec.ts`の予定表のテストで、`await expect(rows.first().locator('td').nth(1)).toHaveText('Lv.7')`の直後に、予定表がHardのままであることを確かめる行を足す。fixtureの予定表の2行目は、Hard枠がLv.10の新作。これも変更前から通る。

```ts
  // The schedule's Hard column keeps its name even for a Lv.9+ problem: a column can hold both.
  await expect(page.locator('.featured-schedule thead th').last()).toHaveText('Hard')
  await expect(rows.nth(1).locator('td').nth(1)).toHaveText('Lv.10')
  await expect(rows.nth(1).locator('td').nth(1)).toHaveAttribute('data-slot', 'Hard')
```

- [ ] **Step 2: 失敗することを確かめる** — `PW`（`tests/featured-standings.spec.ts`、`-g`なし）。Expected: `edition standings show tied ranks`がFAIL（見出しが`Hard`）。新しいテストはPASS。`PW`（`tests/featured.spec.ts`、`the schedule lists coming rounds`）はPASS。

- [ ] **Step 3: `columns`に表示名を持たせる**

`columns`の各要素を`{ slot, problem, label }`にする。`label`は`featured/index.vue:23`と同じ式`slot === 'easy' ? 'Easy' : (problem?.difficulty ?? 0) >= 9 ? 'Ultimate' : 'Hard'`で作り、ADR 0011を指す短いコメントを付ける。列見出しの2か所は`column.label`を使う。行のセルは`v-for="column in columns"`にして、`row[column.slot]`と`` `${column.label} 正解` ``を使う。「順位の決まり方」は変えない。`FeaturedSchedule.vue`は触らない。

- [ ] **Step 4: 通ることを確かめる** — `BUILD`の後、Step 2と同じ`PW`を流す。Expected: すべてPASS。続けて`PW`（`tests/featured.spec.ts`、`-g`なし）も流してPASSすること（一覧のUltimate/Hardと、予定表のHardを守る）。

- [ ] **Step 5: コミット**（本文に、古い表示を固定していた3か所を新しい表示に直したことを書く）

```bash
git add web/app/pages/featured/standings.vue web/tests/featured-standings.spec.ts web/tests/featured.spec.ts
git commit -m "fix: 定期便の順位表で、Lv.9以上のHard枠をUltimateと表示する" -m "featured-standings.spec.tsで古い表示（Hard）を固定していた3か所を、新しい表示（Ultimate）に直した。予定表はHardのままであることをfeatured.spec.tsで確かめる。" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 4: D20 ホームの定期便のお知らせを消す

**Files:**
- Modify: `web/app/pages/index.vue`（24-26行の`featuredNewsUntil`と`showFeaturedNews`、63-67行の`<aside>`、113-120行の`.news-*`のスタイルと、その中の狭い画面についてのコメント）
- Test: `web/tests/featured.spec.ts`（328-338行の`home announces 定期便 with a link that fits narrow screens`を置き換える。Task 3で4行増えているので、テスト名で探す）

- [ ] **Step 1: お知らせのテストを、お知らせがないことを確かめるテストに置き換える**

```ts
test('home shows no 定期便 launch notice and fits narrow screens', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 800 })
  await page.goto('/')
  await expect(page.getByRole('region', { name: 'ShareOJ定期便' })).toBeVisible()
  await expect(page.getByRole('complementary', { name: 'お知らせ' })).toHaveCount(0)
  await expect(page.getByText('新機能「定期便」が追加されました！')).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
```

- [ ] **Step 2: 失敗することを確かめる** — `PW`（`tests/featured.spec.ts`、`no 定期便 launch notice`）。Expected: FAIL（2026-10-31より前なので、お知らせが出る）。

- [ ] **Step 3: 上のFilesに挙げた行を消す。** ほかのスタイルとアニメーションは残す。

- [ ] **Step 4: 通ることを確かめる** — `BUILD`の後、`PW`（`tests/featured.spec.ts`、`-g`なし）。Expected: すべてPASS。
`grep -rn "featuredNews\|news-banner\|news-label\|新機能「定期便」" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app /home/loop/judge/.claude/worktrees/refactor-maintainability/web/tests`が何も出さないこと。

- [ ] **Step 5: コミット**（本文に、消したお知らせのテストを一緒に消したことを書く）

```bash
git add web/app/pages/index.vue web/tests/featured.spec.ts
git commit -m "chore: ホームの定期便のお知らせを、そのテストと一緒に消す" -m "消すお知らせを確かめていたテスト（home announces 定期便 ...）を消し、お知らせがないことと狭い画面ではみ出さないことを確かめるテストに置き換えた。" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 5: D23 タグで絞り込んだ一覧のog:imageを、タグなしの一覧と同じ画像にする

**Files:**
- Modify: `web/app/composables/useSharePreview.ts:12`
- Test: `web/tests/share-preview.spec.ts`（`for (const path of paths)`のループの後）

- [ ] **Step 1: 失敗するテストを書く**

```ts
for (const [path, image] of [['/problems?tag=DP', '/og/problems.png'], ['/blog?tag=DP', '/og/blog.png']] as const) {
  test(`a tag-filtered list shares the unfiltered list's image: ${path}`, async ({ browser, request }) => {
    const context = await browser.newContext({ javaScriptEnabled: false })
    const page = await context.newPage()
    await page.goto(`http://127.0.0.1:13000${path}`)
    for (const selector of ['meta[property="og:image"]', 'meta[name="twitter:image"]']) {
      await expect(page.locator(selector)).toHaveAttribute('content', `https://judge.example${image}`)
    }
    // The shared page itself keeps its tag.
    await expect(page.locator('meta[property="og:url"]')).toHaveAttribute('content', `https://judge.example${path}`)
    expect((await request.get(image)).status()).toBe(200)
    await context.close()
  })
}
```

- [ ] **Step 2: 失敗することを確かめる** — `PW`（`tests/share-preview.spec.ts`、`tag-filtered list`）。Expected: 2件ともFAIL（`/og/problems?tag=DP.png`を指す）。`og:url`はページと`useSharePreview`の両方が設定するが、unheadが`meta:og:url`で1つにまとめるので、metaは1つだけになる。

- [ ] **Step 3: `image`のパスからクエリを除く**

`image`の中で`toValue(options.path)`の`?`より前だけを使う（`/`は今までどおり`/index`にする）。`ogUrl`は変えない。理由を書いた1行のコメントを付ける（画像はページだけで決まり、絞り込んだ一覧は絞り込む前の一覧の画像を使う）。

- [ ] **Step 4: 通ることを確かめる** — `BUILD`の後、`PW`（`tests/share-preview.spec.ts`、`-g`なし）。Expected: すべてPASS。

- [ ] **Step 5: コミット**

```bash
git add web/app/composables/useSharePreview.ts web/tests/share-preview.spec.ts
git commit -m "fix: タグで絞り込んだ一覧のog:imageを、タグなしの一覧と同じ画像にする" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 6: D29 公開予約中の問題にも提出先の制限を出し、状態コードの一覧を1つにする

**Files:**
- Modify: `web/app/utils/problem-readiness.ts:49-50`（`stateIssues`をexportする）
- Modify: `web/app/components/ProblemReadiness.vue`（2行目のimport、10行目の`stateCodes`を消す、16行目）
- Test: `web/tests/manage-problem.spec.ts`（`published problems explain where else they cannot go`の後）

**Interfaces:**
- Produces: `export const stateIssues: Set<string>`（`problem-readiness.ts`。中身は今のまま6件）

- [ ] **Step 1: 失敗するテストを書く**

```ts
test('scheduled problems explain where else they cannot go', async ({ page }) => {
  const id = '89898989-8989-4989-8989-898989898989'
  const draft = { title: '予約済み', markdown: '本文', editorial: '解説', difficulty: 3, timeLimitMs: '2000', memoryLimitMb: '512', testCases: [{ input: '1', output: '1' }] }
  const readiness = { publish: [], contest: ['scheduled'], featured: ['scheduled'] }
  await page.route(`**/api/my/problems/${id}`, route => route.fulfill({ json: { id, version: 2, publishedVersion: 0, scheduledAt: '2099-01-02T14:00:00Z', updatedAt: '2026-09-10T00:00:00Z', draft, readiness } }))
  await page.goto(`/problems/new?problem=${id}`)
  await page.getByRole('button', { name: '問題管理', exact: true }).click()
  const status = page.getByRole('region', { name: '状態' })
  await expect(status.locator('[data-kind="scheduled"]')).toContainText('公開予約中')
  await expect(status.getByRole('list', { name: '足りない項目' })).toHaveCount(0)
  await expect(status.getByRole('list', { name: '提出先の制限' }).getByRole('listitem')).toHaveText(['コンテスト：予約投稿を取り消してから登録してください', '定期便：予約投稿を取り消してから登録してください'])
})
```

- [ ] **Step 2: 失敗することを確かめる** — `PW`（`tests/manage-problem.spec.ts`、`scheduled problems explain`）。Expected: FAIL（`提出先の制限`の一覧が出ない）。

- [ ] **Step 3: 一覧を1つにする** — `stateIssues`の`const`を`export const`にし、コメントは今のままにする。`ProblemReadiness.vue`は`stateCodes`を消し、`stateIssues`をimportして`notes`の`filter`で使う。

- [ ] **Step 4: 通ることを確かめる** — `BUILD`の後、`PW`（`tests/manage-problem.spec.ts`、`-g`なし）。Expected: すべてPASS（`published problems explain ...`の2行もそのまま）。

- [ ] **Step 5: コミット**

```bash
git add web/app/utils/problem-readiness.ts web/app/components/ProblemReadiness.vue web/tests/manage-problem.spec.ts
git commit -m "fix: 公開予約中の問題にも提出先の制限を示し、状態コードの一覧を1つにまとめる" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 7: D31 ログイン後は、同じサイト内のパスなら元のページへ戻す

**Files:**
- Modify: `web/shared/utils/login-destination.ts`
- Test: `web/tests/session.spec.ts`（末尾）、`web/tests/post-problem.spec.ts`（`old posting links open the posting page, and guests can log in and return to it`）、`web/tests/account/google.spec.ts`（`Google login preserves a tester invitation and rejects external return paths`）

**Interfaces:**
- Produces: `loginDestination(value: unknown): string`（シグネチャは今のまま）

- [ ] **Step 1: 失敗するテストを書く**

`session.spec.ts`に`import { loginDestination } from '../shared/utils/login-destination'`を足し、末尾に置く。

```ts
test('login returns to any path of this site and sends anything else to my page', () => {
  for (const path of ['/', '/my', '/problems/post', '/featured/apply', `/problems/${'A'.repeat(22)}`, '/my/contests', '/my/contests/new',
    `/my/contests/${'A'.repeat(22)}`, '/my/submissions', '/my?tab=testing', `/my/tester-invitations/${'A'.repeat(32)}`, '/problems?tag=%E3%82%B0']) {
    expect(loginDestination(path), path).toBe(path)
  }
  for (const value of [undefined, null, 42, ['/problems/post'], '', 'problems', 'https://attacker.example/', '//attacker.example/', '/\\attacker.example/',
    '/problems//post', '/problems?next=https://attacker.example/', '/\t/attacker.example', '/my\n', '/my\r\nLocation: https://attacker.example/', '/my\u0000', '/my\u007f', '/my\u0085']) {
    expect(loginDestination(value), JSON.stringify(value)).toBe('/my')
  }
})
```

`post-problem.spec.ts`の該当テストでは、1行目の`/api/auth/me`のrouteを可変の`let user: { id: string } | null = null`を返すものに変え、hrefの確認の後に続ける。

```ts
  await page.route('**/api/auth/login', route => route.fulfill({ json: { user: { id: 'ui-test-user' } } }))
  await page.locator('.problem-post').getByRole('link', { name: 'ログイン' }).click()
  await expect(page).toHaveURL('/login?next=/problems/post')
  user = { id: 'ui-test-user' }
  await page.getByLabel('メールアドレス', { exact: true }).fill('ui_test@example.test')
  await page.getByLabel('パスワード', { exact: true }).fill('test-password')
  await page.getByRole('button', { name: 'ログイン', exact: true }).click()
  await expect(page).toHaveURL('/problems/post')
```

`google.spec.ts`の該当テストの配列に、`['/problems/post', '/problems/post']`、`` [`/my/contests/${'A'.repeat(22)}`, `/my/contests/${'A'.repeat(22)}`] ``、`['/\\attacker.example/', '/my']`を足す。

- [ ] **Step 2: 失敗することを確かめる**
- `PW`（`tests/session.spec.ts`、`any path of this site`）。Expected: FAIL（`/problems/post`が`/my`になる）。
- `PW`（`tests/post-problem.spec.ts`、`guests can log in and return`）。Expected: FAIL（`/my`へ移る）。
- `PWA`（`tests/account/google.spec.ts`、`external return paths`）。Expected: FAIL（`/problems/post`の行）。テスト用DB（127.0.0.1:25432）が動いていなければ報告する。

- [ ] **Step 3: 判定を置き換える**

`loginDestination`は次のときだけ`value`をそのまま返し、それ以外は`'/my'`を返す。`value`が文字列で、`/`で始まり、`//`と`\`を含まず、制御文字を含まないとき。制御文字の判定は、ほかのファイルと同じ`/[\u0000-\u001f\u007f-\u009f]/`を使う。正規表現の許可リストは消す。コメントは「同じサイト内のパスだけを戻り先にする。`//`と`\`はブラウザーが別のホストと読み、制御文字はURLの解析で消えるかヘッダーを壊すので拒む」という趣旨で1〜2行にする。呼ぶ側（`login.vue`、`GoogleLoginLink.vue`、`ProfileForm.vue`、`profile.global.ts`、Googleの2つのルート）は変えない。

- [ ] **Step 4: 通ることを確かめる** — `BUILD`の後、Step 2の3つを流す。Expected: すべてPASS。続けて`PW`（`tests/contest-teams.spec.ts`、`guests choose`）と、`PWA`の`tests/account/testers.spec.ts`・`tests/account/contest-setters.spec.ts`（招待へ戻る流れ）を`-g`なしで流してPASSすること。

- [ ] **Step 5: コミット**

```bash
git add web/shared/utils/login-destination.ts web/tests/session.spec.ts web/tests/post-problem.spec.ts web/tests/account/google.spec.ts
git commit -m "fix: ログイン後は、同じサイト内のパスなら元のページへ戻す" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 8: D32 5つのエラーコードに、その画面で専用の文言を出す

**Files:**
- Modify: `web/app/utils/profile.ts`（`invalid_avatar`）、`web/app/components/AccountSecurity.vue:27-30`（`password_unavailable`）、`web/app/components/SubmissionForm.vue:68-77`と`web/app/components/TestCaseGenerator.vue:127-133`（`invalid_submission`）、`web/app/utils/contest-team.ts`（`contest_participation_unavailable`。3行目のコメントに参加登録も含める）、`web/app/components/ContestEditor.vue`の`failure()`（353-357行）と`save()`の`catch`（493-502行）（`invalid_contest`）
- Test: `web/tests/settings.spec.ts`、`web/tests/submission.spec.ts`、`web/tests/generator.spec.ts`、`web/tests/contest-teams.spec.ts`、`web/tests/contest-editor-form.spec.ts`

文言（テストでもこのまま使う）:

| コード | 画面 | 文言 |
| --- | --- | --- |
| `invalid_avatar` | プロフィール（設定・登録） | アイコンの画像を保存できませんでした。別の画像を選ぶか、アイコンを削除してから保存してください。 |
| `password_unavailable` | 認証設定 | 外部アカウントでログインしているため、パスワードは変更できません。 |
| `invalid_submission` | 提出フォーム | 提出内容を受け付けられませんでした。言語を選び直し、ソースコードにNUL文字などの使えない文字がないか確認してください。 |
| `invalid_submission` | 入出力生成・入力検証 | 実行内容を受け付けられませんでした。言語とコード、件数と、テストケースの合計サイズを確認してください。 |
| `contest_participation_unavailable` | 参加登録（概要・チーム・オンサイト） | このコンテストには参加登録できません。スタッフ（作成者・作問者・テスター）と、スタッフを務めた人は参加できません。終了後は新しく参加登録できません。 |
| `invalid_contest` | コンテスト編集 | 保存できない値があります。「タイトル・説明」「問題・配点」「期間・ペナルティ」「参加形式」の内容を確認してください。 |

- [ ] **Step 1: 失敗するテストを書く**（どの画面も、変更前は汎用の文言になるので失敗する）

- `settings.spec.ts`の末尾に`profile and password refusals name their reason`を足す。
  - `**/api/my/profile`は、PUTに`failure(400, 'invalid_avatar')`を返し、それ以外には`{ profile }`を返す。`/my/settings`で「変更を保存」を押し、`page.getByRole('alert')`が表の文言になること。
  - 続けて、`**/api/my/account`は`provider: 'password'`を返し、`**/api/my/password`は`failure(409, 'password_unavailable')`を返す。`/my/settings?tab=security`で3つの欄に`Current-password-1`・`New-password-123!`・`New-password-123!`を入れて「パスワードを変更」を押し、`alert`が表の文言になること。
- `submission.spec.ts`の`unready judging settings ...`の後に`a refused submission names what to check`を足す。このファイルは`fixtures/account`ではなく`@playwright/test`を使うので、ほかのテストと同じく最初に`await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))`を置く（ないと未ログインになり、提出フォームが描かれない）。`**/api/my/submissions`は`{ status: 400, json: { data: { code: 'invalid_submission' } } }`を返す。`/problems/${problemId}`で`int main(){}`を提出し、`page.getByRole('region', { name: '提出', exact: true }).getByRole('alert')`が表の文言になること。
- `generator.spec.ts`の`generation frequency limit displays the wait time`の後に`a refused generation names what to check`を足す。同じ手順で400の`invalid_submission`を返し、`.generator [role=alert]`が表の文言になること。
- `contest-teams.spec.ts`の`joining without a passphrase ...`の後に`a refused participation says who cannot join`を足す。`const state = await routeContest(page, { ...base, status: 'running' as const })`と`routeJoin(page, state, [{ status: 409, code: 'contest_participation_unavailable' }])`を使い、`open(page)`の後に「参加する」を押す。`page.getByRole('alert')`が表の文言になること。
- `contest-editor-form.spec.ts`の`a published contest whose passphrase cannot be read ...`の後に`an invalid contest is named on the draft and the published save`を足す。
  - 下書き: `routeDraft(page)`の後、`**/api/my/contests/*/draft`をもう一度routeする。PUTには`{ status: 400, json: { statusCode: 400, data: { code: 'invalid_contest' } } }`を返し、ほかは`route.fallback()`に回す。`/my/contests/new`でタイトルを入れて「下書きを保存」を押し、`alert`が表の文言になること。
  - 公開済み: `routeContest(page, { json: { onsiteDivision: true, passphrase: 'Venue_2026' } })`の後、`` `**/api/my/contests/${contestId}` ``をもう一度routeする。PUTには可変の`reply`を返し、ほかは`fallback`に回す。`reply`を`invalid_contest`の400にして「変更を保存」を押し、`alert`が表の文言になること。次に`reply`をコードなしの400（`{ status: 400, json: {} }`）にして押し、`alert`が今までどおり`入力内容を確認してください。`になること（保つ挙動）。

- [ ] **Step 2: 失敗することを確かめる** — `PW`を5つのファイルについて、それぞれ足したテスト名で流す。Expected: 5件ともFAIL。表示は今の汎用の文言になる（`保存できませんでした。…`、`変更できませんでした。…`、`提出を確認できませんでした。…`、`実行できませんでした。…`、`参加登録できませんでした。…`、`入力内容を確認してください。`）。ログインの案内やタイムアウトで落ちたときは、テストの準備の誤りなので直してから確かめ直す。

- [ ] **Step 3: 文言を足す**
- `profileError`: `invalid_profile`の行の後に、`invalid_avatar`の行を足す。
- `AccountSecurity.vue`: 三項演算子の連なりで`invalid_password`の後に`password_unavailable`を足す。
- `SubmissionForm.vue`: `judging_unavailable`の後、最後の`else`の前に`invalid_submission`を足す。サンプル検証でも同じ文言にする。
- `TestCaseGenerator.vue`: `submission_rate_limited`の分岐の後に`detail?.code === 'invalid_submission'`の分岐を足す。
- `contest-team.ts`: `messages`に`contest_participation_unavailable`を足す。
- `ContestEditor.vue`: `noLongerSetter`の隣に`const invalidContest = '…'`を置く。`failure()`は、先頭の`creation_quota_exceeded`の後でコードが`invalid_contest`なら`invalidContest`を返す。`save()`の`catch`は、`contest_team_size_locked`の後に`if (code === 'invalid_contest') { message.value = invalidContest; return }`を足す。

- [ ] **Step 4: 通ることを確かめる** — `BUILD`の後、5つのファイルを`-g`なしで流す。Expected: すべてPASS（既存の`password users change their password ...`、`tests_not_ready`、`submission_rate_limited`、`incorrect_venue_passphrase`などの文言はそのまま）。

- [ ] **Step 5: コミット**

```bash
git add web/app/utils/profile.ts web/app/components/AccountSecurity.vue web/app/components/SubmissionForm.vue web/app/components/TestCaseGenerator.vue web/app/utils/contest-team.ts web/app/components/ContestEditor.vue web/tests/settings.spec.ts web/tests/submission.spec.ts web/tests/generator.spec.ts web/tests/contest-teams.spec.ts web/tests/contest-editor-form.spec.ts
git commit -m "fix: 画面が読んでいなかった5つのエラーコードに、その画面で専用の文言を出す" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 9: D33（Web） 提出コードの長さの上限を`SubmissionForm.vue`で1つにまとめる

**Files:**
- Modify: `web/app/components/SubmissionForm.vue`（37行目と99行目の`131072`）
- Test: `web/tests/submission.spec.ts`（`a refused submission names what to check`の後）

**Interfaces:**
- Produces: `SubmissionForm.vue`のscript内の`const sourceLimit = 131072`（exportしない）

- [ ] **Step 1: 保つ挙動のテストを書く**（変更前から通る）

```ts
test('source over 131,072 bytes is refused before sending, and the limit itself is sent', async ({ page }) => {
  await page.route('**/api/auth/me', route => route.fulfill({ json: { user: { id: 'alice' } } }))
  let posts = 0
  await page.route('**/api/my/submissions', route => { posts++; return route.fulfill({ status: 503, json: { data: { code: 'judging_unavailable' } } }) })
  await page.goto(`/problems/${problemId}`)
  const editor = page.getByLabel('ソースコード', { exact: true })
  const alert = page.getByRole('region', { name: '提出', exact: true }).getByRole('alert')
  // Bytes count, not characters: 43,691 × 3 bytes.
  await editor.fill('あ'.repeat(43691))
  await expect(page.getByText('131,073 / 131,072 bytes', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '提出する', exact: true }).click()
  await expect(alert).toHaveText('ソースコードを1〜131,072バイトで入力してください。')
  expect(posts).toBe(0)
  await editor.fill('a'.repeat(131072))
  await expect(page.getByText('131,072 / 131,072 bytes', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: '提出する', exact: true }).click()
  await expect(alert).toHaveText('ジャッジが設定されていません。')
  expect(posts).toBe(1)
})
```

- [ ] **Step 2: 変更前に通ることを確かめる** — `PW`（`tests/submission.spec.ts`、`over 131,072 bytes`）。Expected: PASS。

- [ ] **Step 3: 定数にまとめる** — scriptの`runtime`の宣言の近くに`const sourceLimit = 131072`と、「APIが受け付けるソースの上限（UTF-8のバイト数）」という趣旨の1行のコメントを置く。37行目の比較と99行目の`:limit`で使う。文言（38行目、90行目）は変えない。

- [ ] **Step 4: 通ることを確かめる** — `BUILD`の後、`PW`（`tests/submission.spec.ts`、`-g`なし）。Expected: すべてPASS（`12 / 131,072 bytes`の確認を含む）。
`grep -n "131072" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app/components/SubmissionForm.vue`で、出るのが定数の1行だけであること。

- [ ] **Step 5: コミット**

```bash
git add web/app/components/SubmissionForm.vue web/tests/submission.spec.ts
git commit -m "refactor: 提出コードの長さの上限を、提出フォームでは定数1つにまとめる" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## 検証コマンド（controllerが全体を流す）

どれも`/home/loop/judge/.claude/worktrees/refactor-maintainability/web`で実行する（laneでは、そのworktreeの`web`）。
- `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`
- `BUILD`
- ブラウザー: `PW`を`<file>`と`-g`なしで（`--output`は`.../scratchpad/b6/pw-all`）
- アカウント: `PWA`を`<file>`と`-g`なしで（`--output`は`.../scratchpad/b6/pw-account-all`）
- `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run package:lambda`
- `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run test:lambda`
- `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command node --test scripts/smoke-frontend.test.mjs`

## Out of scope（報告のみ）

- `web/README.md:130`は今も「テスターは編集、公開、削除…を作者と同じ権限で行える」と書いている（D17と同じ食い違い）。仕様が対象にしたのは招待ページだけ。
- ガイド`authoring.md:142-144`と問題管理の説明（`problems/new.vue:238`）は、テスターができる操作に「テスターリンクの発行」を挙げていない。`problems/new.vue:238`は「非公開に戻す操作」も挙げていない。誤りではないが、招待ページより狭く読める。
- `profile.global.ts:15`は、招待ページ以外の`/my`以下でプロフィールが未登録だと、`next`を付けずに`/onboarding`へ送る。D31はログイン後の戻り先が対象で、プロフィール登録後の戻り先は広げていない。
- Googleのcallback（`callback.get.ts:38`）は`next`を`sendRedirect`の`Location`にそのまま入れる。`/problems?tag=グラフ`のようにASCII外の文字を符号化せずに含む`next`では、Nodeのヘッダー検査で例外になり、`/login?socialError=failed`へ移る。今までの許可リストでも`/problems?…`で起きていた。画面が付ける`next`はどれもASCIIなので、今は表に出ない。
- `SubmissionForm.vue`の文言「131,072バイト」「128 KiBまで」と、ガイド`languages.md`の上限の記述は、定数とは別に数を書いている。
- `docs/refactor/FIXES.md`の「まとまりと順序」の状態とコミットの更新。controllerが行う。
