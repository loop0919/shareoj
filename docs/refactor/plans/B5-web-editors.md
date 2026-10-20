# B5 Webのエディター（D16、D14、D24、D13、D15） 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task（実装者は1人。各タスクの最後に自分でコミットする）. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `docs/refactor/FIXES.md`のD16、D14、D24、D13、D15の「変更内容」を、1件1コミットで実装する。

**Architecture:** どれもWebの画面（`web/app`）だけで閉じる。使われないコードを消し（D16）、生成の言語選択に「現在利用できません」を足し（D14）、自動保存のタイマーの張り直しを`scheduleAutosave`に任せて1本にする（D24）。最後に、コンテスト編集画面の保存ショートカット（D13）と、記事・コンテストのMarkdownエディターのプレビュー・数式スニペット・配色の切替（D15）を問題の編集画面に揃える。

**Tech Stack:** Nuxt 4 / Vue 3.5、TypeScript 5.9、Playwright（`page.clock`、`page.route`）

**Spec:** `docs/refactor/FIXES.md`（「守ること」と、D13・D14・D15・D16・D24の「変更内容」）。根拠：`/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/fixes/briefs/B5.md`。

**作業場所:** laneのworktree `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors`（ブランチ`fix/web-editors`）。B4（D09・D10）はこのブランチにだけある。以下のパスはこのworktreeからの相対パス、行番号は計画の時点のHEAD（`541fa0e`、B4を含む）のもの。`web/app`と`web/tests`でB4が変えたのは`problem-draft.ts`と`editor.spec.ts`だけ。

## 判定（Ruling）

- Ruling（D13・D15の共通化）: 共通の関数や部品は作らない。コンテスト編集画面に、問題・記事の画面と同じ判定の式と、問題の編集画面と同じプレビューのタイマーを書く。理由: 問題・記事の画面もそれぞれ自前で同じ式を持っている（`useProblemDraft.ts:177-182`、`blog/new.vue:125-129`）。3つのMarkdown編集画面の部品化はC18で却下済み。変更を「揃える」範囲に限る。
- Ruling（D14の条件）: `ProblemChecker.vue:65`・`ScoringEditor.vue:215`と同じく、「今の値がカタログにない」ときに選択肢を出す。保存済みかどうかは区別しない。理由: 仕様は「他の言語選択と同じ」。そのため、新しい問題の生成コードの既定`cpp23-gcc`をジャッジが出していないときも`cpp23-gcc（現在利用できません）`と出る。今は空欄に見えている。`judge-catalog.spec.ts`の既存の2か所は、空欄だった前提を固定しているので新しい挙動に直す（Task 2）。
- Ruling（D15の読み込み時）: 記事・コンテストでも、読み込みが終わった時点（`ready`が真になったとき）には待たずに描画する。入力による更新だけを150ms遅らせる。理由: 問題の編集画面がそうしている（`problems/new.vue:92`）。開始後のコンテスト（`locked`）は`ready`が偽のままだが、説明の`watch`が150ms後に描画するので、プレビューは出る。
- Ruling（D15の配色の切替の位置）: `ContestEditor.vue`の`editor-topbar`で、`<h1 class="visually-hidden">`の直後、`editor-view-switch`の前に`<ThemeSelect />`を置く。理由: 見える並びが問題・記事と同じ（ロゴ、配色、表示の切り替え）になる。見出しはロゴの直後に残る。
- Ruling（D16のテスト）: `problem-markdown.spec.ts`の`'export keeps the exact body and safely quotes title metadata'`は、消すコード（`exportProblemMarkdown`と3項目）のテストなので消す。ただし、その中の`title`の確認は残る項目のテストなので、`draftErrors`の残る2項目を完全一致で確かめるテストに置き換える。
- Ruling（D24）: `scheduleAutosave()`の先頭で、`manualSaveOnly`の判定より前に`clearTimeout(saveTimer)`を呼ぶ。`watch(draft)`の`clearTimeout`は、同じことを`scheduleAutosave()`がするので消す。これは`C04-rulings.md`の裁定（`scheduleAutosave()`は`clearTimeout`を呼ばない）を、利用者の判断（D24）で置き換えるものである。ほかの`clearTimeout(saveTimer)`（`saveDraft`、`unpublishProblem`、`onBeforeRouteLeave`、`onBeforeUnmount`、`openDeleteConfirmation`、`removeProblem`）は変えない。
- D24の再現: C04の計画は「テストでは再現できない」としていた。この計画では、離脱確認ダイアログの裏で入出力生成が完了する手順で再現する（Task 3）。生成の`POST /api/my/submissions`を`page.route`で止め、生成中にタイトルを編集し、ダイアログを開いてから応答を返す。変更前のコードでは、生成の完了で張られたタイマーが閉じた後も残り、次の編集の600msの待ちより先にPUTが起きる。
- 計画者の確認: Task 2〜5の新しいテストを、scratchpadの別の設定（`web/tests`には置かない）で変更前のビルド（B4を含まない`fix/deferred-items`。B4はこれらのテストが触れるコードを変えない）に対して流した。D24は閉じてから899ms（編集から599ms）の時点でPUTが1回済み、状態は`saved`だった。D13は`prevented`が`[true, true, true, true]`で、保存は2回。D15はプレビューの記事・コンテストと、数式のコンテストだけが落ち、問題の編集画面の2件と記事の数式は通った。D14は再読み込み後の選択肢が`['C++23']`、値が空だった。開始後のコンテストで説明がプレビューに出る確認と、使えない言語で「生成する」を押したときの文言は、変更前から通った。`KeyboardEvent`の`isComposing`と`repeat`は、初期化の値どおりに付く。

## Global Constraints

- 各項目の変更内容だけを行う。ほかの公開API、DBスキーマ、依存ライブラリのバージョン、インフラ構成は変えない。npmのパッケージを足さない。
- 認可、公開範囲、重複・再実行時の挙動は変えない。保存の条件（`ready`・`locked`・`busy`・`resolveLeave`・`confirmingDelete`の確認）、手動保存だけの問題（`manualSaveOnly`）でタイマーを張らないこと、生成が使えない言語を拒否すること（`TestCaseGenerator.vue:58-61`）は保つ。
- 1件1コミット（`prefix: 対応内容`）。変える挙動のテストを先に書き、変更前のコードで失敗することを確かめてから直す。コミットメッセージは、空行のあとに`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`の行で終える（各タスクのコミットのコマンドに含めてある）。`git commit --amend`はしない。
- テストの削除・スキップ・検証の緩和はしない。例外は2つ。D16で消すコードのテスト（Task 1）と、D14が意図して変える挙動を固定している`judge-catalog.spec.ts`の2か所（Task 2。新しい挙動に直し、コミットメッセージに書く）。
- 範囲外の問題を見つけたら直さずに報告する。
- 周りのコードに合わせる（英語のコメント、コメントの量、テスト名は英語）。共通化や整理はしない（Ruling参照）。
- laneのworktreeの外のファイルには触れない（scratchの`/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/editors/`を除く）。graphifyは実行しない（controllerが行う）。
- Bashの規則: 変数から組み立てたコマンドは使わない。スクリプトファイルは実行しない。`; echo rc=$?`のような後置きは付けない（終了コードはBashの結果で見る）。Nodeは`nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command`を付けて、laneの`web`で実行する。
- Playwrightは`.output`のビルドを使うので、変更前・変更後の確認の前に、それぞれ下の「ビルド」をし直す。Playwrightは固定のポートをほかのlaneと共有するので、1つのspecだけでも必ず下の「Playwright」の形（nixのshellの中で`flock`を取る）で流す。lockの待ちは長くなりうるので、Bashのtimeoutは600000にする。`--output`は上のscratchの下にする（`web/test-results`はrootの所有）。

### コマンドの形

- ビルド: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`
- Playwright（`<specs>`は各Stepで指定するspecと`-g`）: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command flock /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/playwright.lock env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test <specs> --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/editors/pw-b5`

各Stepの「Run」は、この形の`<specs>`を置き換えた1行のコマンドを指す。

## Review Focus

1. D24: `scheduleAutosave()`の`clearTimeout`を`manualSaveOnly`の判定の中に入れると、手動保存だけの問題で`watch(draft)`が保留中のタイマーを消さなくなる。既存の`manual-problem-save.spec.ts`（C04のT4）をTask 3で流す。
2. D13: Shift・Alt付きとIME変換中は、問題・記事の画面と同じく`preventDefault`しない（ブラウザーの動作に任せる）。キーリピートは`preventDefault`するが保存しない。Task 4のテストが4つを別々に固定する。
3. D15: 開始後のコンテストは`ready`が偽のままなので、`ready`の`watch`だけで描画すると説明のプレビューが出なくなる。Task 5で`contest-setters.spec.ts`の開始後のテストに、説明がプレビューに出ることの確認を足す。
4. D15: 問題の編集画面（問題文と解説の切り替えで待たずに描画する動作）は変えない。Task 5のテストは3画面を同じ手順で流し、問題の編集画面は変更前から通る。150msの遅延で、編集しながら折りたたみを確かめる既存の`details.spec.ts`（記事とコンテストを含む）も、Task 5で流す。
5. D14: 使える言語を選ぶと「現在利用できません」の選択肢は消え、使えない言語のまま「生成する」を押すと今までどおり`利用できる言語を選択してください。`で止まる。Task 2のテストに両方の行がある。

---

### Task 1: D16 使われていない`exportProblemMarkdown`と`draftErrors`の項目を消す

**Files:**
- Modify: `web/app/utils/problem-draft.ts`（`draftErrors`、101-112行。`exportProblemMarkdown`、114-123行）
- Test: `web/tests/problem-markdown.spec.ts`（3行目のimport、209-219行のテスト）

**Interfaces:**
- Produces: `export function draftErrors(draft: Pick<ProblemDraft, 'title' | 'markdown'>): { title: string, markdown: string }`（戻り値の型は推論に任せる）。呼び出し側は`useProblemDraft.ts:67`だけで、変えない。

- [ ] **Step 1: テストを置き換える**

3行目を`import { draftErrors } from '../app/utils/problem-draft'`にする。209-219行のテストを消し、同じ位置に次を置く。

```ts
test('draft errors name only a blank title and a blank statement', () => {
  expect(draftErrors({ title: '  ', markdown: '## 本文' })).toEqual({ title: '問題のタイトルを入力してください。', markdown: '' })
  expect(draftErrors({ title: 'A + B', markdown: ' \n' })).toEqual({ title: '', markdown: '問題の本文を入力してください。' })
})
```

- [ ] **Step 2: 失敗することを確かめる**

Run: ビルド
Run: Playwright（`<specs>` = `tests/problem-markdown.spec.ts -g "draft errors"`）
Expected: FAIL。実際の値に`checker: ''`と、`timeLimitMs`・`memoryLimitMb`の文言（`100〜5,000 ms の範囲で…`、`64〜512 MiB の整数を…`）が余分に入る。

- [ ] **Step 3: コードを消す**

- `exportProblemMarkdown`を関数ごと消す。
- `draftErrors`の引数の型を`Pick<ProblemDraft, 'title' | 'markdown'>`にし、戻り値を`title`と`markdown`の2項目だけにする。文言は今のまま。使われなくなる`time`・`memory`・`code`のローカル変数も消す。
- `ProblemChecker.vue`の検証コードの検査は別物なので触らない。

- [ ] **Step 4: 通ることと、残りがないことを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`
Expected: エラーなし（`useProblemDraft.ts:67`の`draftErrors(draft)`と`new.vue`の`errors.title`・`errors.markdown`の型が通る）

Run: ビルド。続けてPlaywright（`<specs>` = `tests/problem-markdown.spec.ts`）
Expected: PASS（`problem-markdown.spec.ts`の全件）

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && grep -rn "exportProblemMarkdown\|errors\.checker\|errors\.timeLimitMs\|errors\.memoryLimitMb" app tests`
Expected: 出力なし（終了コード1）。変更前は`app/utils/problem-draft.ts:114`と`tests/problem-markdown.spec.ts:3,211,218`が出る。`time_limit_ms`はパターンに入れない（`app/content/docs/cli.md:71`のCLIの設定例に一致するが、これは残す）。

- [ ] **Step 5: コミット**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors && git add web/app/utils/problem-draft.ts web/tests/problem-markdown.spec.ts && git commit -m "refactor: 本番コードから使われていないexportProblemMarkdownとdraftErrorsの項目を、そのテストと一緒に消す" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 2: D14 生成の言語選択に「（現在利用できません）」の選択肢を付ける

**Files:**
- Modify: `web/app/components/TestCaseGenerator.vue:154`
- Test: `web/tests/judge-catalog.spec.ts`（34行、50-64行の既存テストを直し、末尾にテストを足す）

**Interfaces:**
- Consumes: なし
- Produces: なし（テンプレートだけ）

- [ ] **Step 1: 新しいテストを書き、古い前提の2か所を直す**

`judge-catalog.spec.ts`の末尾に足す。

```ts
// The saved runtime stays selected and is named as unavailable, as in the checker and aggregator selects.
test('the generator keeps a saved runtime the judge no longer offers and names it as unavailable', async ({ page }) => {
  const offered = [{ id: 'cpp23-gcc', label: 'C++23' }, { id: 'python314', label: 'Python' }]
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: offered } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
  await page.locator('#problem-title').fill('生成の言語')
  await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
  const language = page.locator('.generator-options select')
  await expect(language.locator('option')).toHaveText(['C++23', 'Python'])
  await language.selectOption('python314')
  await page.getByLabel('入力生成のコード', { exact: true }).fill('print(1)')
  await page.getByRole('button', { name: '保存', exact: true }).click()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'saved')
  await page.route('**/api/runtimes', route => route.fulfill({ json: { items: offered.slice(0, 1) } }))
  await Promise.all([page.waitForResponse('**/api/runtimes'), page.reload()])
  await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
  // The server-rendered catalog still offers Python; the options show that the narrowed one arrived.
  await expect(language.locator('option')).toHaveText(['python314（現在利用できません）', 'C++23'])
  await expect(language).toHaveValue('python314')
  await page.getByRole('button', { name: '生成する', exact: true }).click()
  await expect(page.locator('.generator [role=alert]')).toHaveText('利用できる言語を選択してください。')
  // Each mode keeps its own runtime; validation was left in C++23, which is still offered.
  await page.getByRole('button', { name: '入力検証', exact: true }).click()
  await expect(language.locator('option')).toHaveText(['C++23'])
  await page.getByRole('button', { name: '入力生成', exact: true }).click()
  await language.selectOption('cpp23-gcc')
  await expect(language.locator('option')).toHaveText(['C++23'])
})
```

既存の2か所を新しい挙動に直す（新しい問題の生成コードの既定は`cpp23-gcc`。Ruling参照）。
- 34行: `toHaveCount(0)`を`toHaveText(['cpp23-gcc（現在利用できません）'])`にする。
- 50-64行のループ: 各要素に`generator`を足し、60行の`toHaveText(labels)`を`toHaveText(generator)`にする。1つ目は`generator: ['Python', 'C++23']`、2つ目は`generator: ['cpp23-gcc（現在利用できません）', 'C++17', 'Python']`。2つ目の直前に`// New generator code is in C++23 (GCC), which this judge does not offer.`を置く。

- [ ] **Step 2: 失敗することを確かめる**

Run: ビルド
Run: Playwright（`<specs>` = `tests/judge-catalog.spec.ts`）
Expected: FAIL 4件。新しいテストは選択肢が`['C++23']`だけになる。直した34行の2件は選択肢が0件、60行の`cpp17`の1件は`['C++17', 'Python']`になる。60行の`cpp23-gcc`の1件と、残りは通る。

- [ ] **Step 3: 選択肢を足す**

`TestCaseGenerator.vue:154`の`<select>`の中で、`v-for`の`<option>`の前に、`ProblemChecker.vue:65`と同じ形の選択肢を置く。条件は`!available.some(item => item.id === program.runtime)`、値は`program.runtime`、表示は`{{ program.runtime }}（現在利用できません）`。`generate()`の言語の検査（58-61行）は変えない。

- [ ] **Step 4: 通ることを確かめる**

Run: ビルド。続けてPlaywright（`<specs>` = `tests/judge-catalog.spec.ts tests/generator.spec.ts`）
Expected: PASS（全件）

- [ ] **Step 5: コミット**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors && git add web/app/components/TestCaseGenerator.vue web/tests/judge-catalog.spec.ts && git commit -m "fix: 入出力生成・入力検証の言語選択に、カタログにない言語を「現在利用できません」として出す" -m "judge-catalog.spec.tsで生成の言語の選択肢が空になる前提だった2か所を、新しい挙動に合わせた。" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 3: D24 保留中の自動保存タイマーを常に1本にする

**Files:**
- Modify: `web/app/composables/useProblemDraft.ts:234-244`（`scheduleAutosave`とそのコメント、`watch(draft)`）
- Test: `web/tests/problem-save-behavior.spec.ts`（69-93行の`saveWithFrozenClock`に引数を足し、末尾にテストを足す）

**Interfaces:**
- Consumes: なし
- Produces: `saveWithFrozenClock(page: Page, prepare?: () => Promise<void>)`（テスト内の補助関数。戻り値は今のまま）

- [ ] **Step 1: 失敗するテストを書く**

`saveWithFrozenClock`に省略できる`prepare`を足す。タイトルを入力した後、「保存」を押す前に`await prepare?.()`を呼ぶ。69行のコメントを「Saves a new problem, after prepare edits it further if given, then freezes the clock …」の内容にする。既存の2つの呼び出しは変えない。

末尾に足す。

```ts
test('a generation that finishes behind the leave dialog leaves a single autosave timer', async ({ page }) => {
  const { expectOneAutosave } = await saveWithFrozenClock(page, async () => {
    await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
    await page.getByLabel('入力生成のコード', { exact: true }).fill('print(1)')
  })
  let finish!: () => void
  const finished = new Promise<void>(resolve => { finish = resolve })
  await page.route('**/api/my/submissions', async (route) => {
    await finished
    await route.fulfill({ json: { id: 'generation-job', status: 'DONE', result: { verdict: 'AC', cases: [{ output: '1\n' }] } } })
  })
  const generation = page.waitForRequest('**/api/my/submissions')
  await page.getByRole('button', { name: '生成する', exact: true }).click()
  await generation
  // An edit during the generation makes leaving ask first, so the generation finishes behind the dialog.
  await page.getByRole('button', { name: '問題文', exact: true }).click()
  await page.locator('#problem-title').fill('生成中の編集')
  await page.getByRole('link', { name: 'ShareOJ マイページ', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeVisible()
  finish()
  await expect(page.getByText('1件の入力を追加しました。', { exact: true })).toBeAttached()
  await page.getByRole('button', { name: '編集を続ける', exact: true }).click()
  await expect(page.getByRole('dialog')).toBeHidden()
  // A timer left from the generation would write 600 ms after closing, before this edit's debounce ends.
  await page.clock.runFor(300)
  await page.locator('#problem-title').fill('閉じた後の編集')
  await expectOneAutosave('閉じた後の編集')
})
```

- [ ] **Step 2: 失敗することを確かめる**

Run: ビルド
Run: Playwright（`<specs>` = `tests/problem-save-behavior.spec.ts`）
Expected: 新しいテストだけFAIL。`expectOneAutosave`の`runFor(599)`の後の`data-save-state`が`dirty`ではなく`saving`/`saved`になる（閉じてから600msで、残ったタイマーがPUTする）。既存の5件（conflictのループの2件、in-flightの1件、離脱確認の1件、削除確認の1件）は通る。

- [ ] **Step 3: `scheduleAutosave`で前のタイマーを消す**

- `scheduleAutosave()`の先頭に`clearTimeout(saveTimer)`を置く。`manualSaveOnly`の判定より前にする（Review Focus 1）。
- `watch(draft)`の`clearTimeout(saveTimer)`（242行）を消し、`scheduleAutosave()`だけを呼ぶ。
- 234行のコメントを、「保留中のタイマーを先に消すので、張られるのは常に1本。ダイアログの裏で草稿が変わったときも同じ」という内容に書き換える。235行の`manualSaveOnly`の説明は残す。
- `finishLeave`（266行）と`closeDeleteConfirmation`（288行）の呼び出しと条件は変えない。

- [ ] **Step 4: 通ることを確かめる**

Run: ビルド。続けてPlaywright（`<specs>` = `tests/problem-save-behavior.spec.ts tests/manual-problem-save.spec.ts tests/leave-editor.spec.ts tests/save-shortcut.spec.ts tests/generator.spec.ts`）
Expected: PASS（離脱確認と削除確認を閉じた後に600msで1回だけ保存するC04のT2・T3、手動保存だけの問題でタイマーを張らないT4を含む）

- [ ] **Step 5: コミット**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors && git add web/app/composables/useProblemDraft.ts web/tests/problem-save-behavior.spec.ts && git commit -m "fix: 問題の編集画面で、保留中の自動保存タイマーを常に1本にする" -m "ダイアログを閉じるときも、前のタイマーを消してから張り直す。ダイアログの裏で生成が完了したときに、次の編集の待ちより先にPUTが起きなくなる。" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 4: D13 コンテスト編集画面の保存ショートカットを問題・記事と同じ判定にする

**Files:**
- Modify: `web/app/components/ContestEditor.vue:148-153`（`saveWithShortcut`）
- Test: `web/tests/contest-editor-form.spec.ts`（176-184行の`'Ctrl+S saves the contest draft'`の後に足す）

**Interfaces:**
- Consumes: 同じファイルの`routeDraft(page)`（保存された本文の配列を返す）
- Produces: なし

- [ ] **Step 1: 失敗するテストを書く**

```ts
test('Ctrl+S does nothing while composing, with Shift or Alt, or on key repeat', async ({ page }) => {
  const bodies = await routeDraft(page)
  await page.goto('/my/contests/new')
  await page.getByLabel('コンテストタイトル', { exact: true }).fill('保存しない操作')
  const prevented = await page.evaluate(() => [{ isComposing: true }, { shiftKey: true }, { altKey: true }, { repeat: true }].map((init) => {
    const event = new KeyboardEvent('keydown', { key: 's', ctrlKey: true, cancelable: true, bubbles: true, ...init })
    window.dispatchEvent(event)
    return event.defaultPrevented
  }))
  // As in the problem and post editors, only a repeat still keeps the browser from saving the page.
  expect(prevented).toEqual([false, false, false, true])
  await page.getByLabel('コンテストタイトル', { exact: true }).fill('ショートカットで保存')
  await page.keyboard.press('Control+s')
  await expect(page.locator('.draft-status')).toContainText('公開まであと')
  expect(bodies.map(body => body.draft.title)).toEqual(['ショートカットで保存'])
})
```

- [ ] **Step 2: 失敗することを確かめる**

Run: ビルド
Run: Playwright（`<specs>` = `tests/contest-editor-form.spec.ts`）
Expected: 新しいテストだけFAIL。`prevented`が`[true, true, true, true]`になる（最初のイベントで保存も走る）。

- [ ] **Step 3: 判定を揃える**

`saveWithShortcut`の判定を`blog/new.vue:126-128`と同じにする。`event.isComposing || event.altKey || event.shiftKey || !(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 's'`なら何もせずに戻る。それ以外は`preventDefault()`し、`!event.repeat`のときだけ、今の条件（`ready.value && !locked.value && !busy.value`）で`form.value?.requestSubmit()`を呼ぶ。公開済みのコンテストでも同じ関数を通る（送信と画面移動は今のまま）。

- [ ] **Step 4: 通ることを確かめる**

Run: ビルド。続けてPlaywright（`<specs>` = `tests/contest-editor-form.spec.ts`）
Expected: PASS（`'Ctrl+S saves the contest draft'`を含む全件）

- [ ] **Step 5: コミット**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors && git add web/app/components/ContestEditor.vue web/tests/contest-editor-form.spec.ts && git commit -m "fix: コンテスト編集画面の保存ショートカットを、問題・記事の編集画面と同じ判定にする" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 5: D15 3つのMarkdownエディターのプレビュー・数式スニペット・配色の切替を揃える

**Files:**
- Modify: `web/app/pages/blog/new.vue`（script、217行のプレビュー）
- Modify: `web/app/components/ContestEditor.vue`（147行の`mathSnippet`、script、507-509行の`editor-topbar`、558行のプレビュー）
- Test: `web/tests/markdown-editor.spec.ts`（60行のループの後に足す）、`web/tests/theme.spec.ts:37`、`web/tests/contest-setters.spec.ts:177`の後
- 変えない: `web/app/pages/problems/new.vue`（揃える先）

**Interfaces:**
- Consumes: なし
- Produces: なし（各画面の中だけ）

- [ ] **Step 1: テストを書く**

`markdown-editor.spec.ts`の最初のループ（3-60行）の後に足す。

```ts
for (const [path, selector] of [
  ['/problems/new', '#problem-source'],
  ['/blog/new', '#post-body'],
  ['/my/contests/new', '#contest-description'],
]) {
  test(`the preview in ${path} follows 150 ms after the last edit`, async ({ page }) => {
    await page.goto(path!)
    const editor = page.locator(selector!)
    await expect(editor).toBeEditable()
    const preview = page.locator('#preview-pane')
    await page.clock.install()
    await page.clock.pauseAt(new Date(Date.now() + 1000))
    await editor.fill('最初の段落')
    await page.clock.runFor(100)
    await expect(preview).not.toContainText('最初の段落')
    await editor.fill('次の段落')
    await page.clock.runFor(149)
    await expect(preview).not.toContainText('次の段落')
    await page.clock.runFor(1)
    await expect(preview).toContainText('次の段落')
  })

  test(`the math button in ${path} inserts a math block`, async ({ page }) => {
    await page.goto(path!)
    const editor = page.locator(selector!)
    await expect(editor).toBeEditable()
    await editor.fill('数式の前')
    await editor.press('ControlOrMeta+End')
    await page.locator('.editor-toolbar').getByRole('button', { name: '数式', exact: true }).click()
    await expect.poll(() => editor.locator('.cm-line').allTextContents()).toEqual(['数式の前', '```math', '\\sum_{i=1}^{N} A_i', '```', ''])
  })
}
```

`theme.spec.ts:37`の`['/', '/problems/new', '/blog/new']`に`'/my/contests/new'`を足す（320pxで配色の切替があり、横にはみ出さないことを確かめる）。

`contest-setters.spec.ts`の`'after the start a setter still manages co-setters while the settings stay locked'`の末尾（177行の後）に足す。

```ts
  // The settings are locked, yet the description still shows in the preview.
  await page.getByRole('button', { name: 'タイトル・説明', exact: true }).click()
  await expect(page.locator('.contest-preview').getByText('説明', { exact: true })).toBeVisible()
```

- [ ] **Step 2: 失敗することを確かめる**

Run: ビルド
Run: Playwright（`<specs>` = `tests/markdown-editor.spec.ts tests/theme.spec.ts tests/contest-setters.spec.ts`）
Expected: FAIL 4件。プレビューの`/blog/new`と`/my/contests/new`（入力のたびに描画するので、100ms後に`最初の段落`が出ている）、数式の`/my/contests/new`（`['数式の前', '$$', 'a^2 + b^2 = c^2', '$$', '']`）、配色の`/my/contests/new`（切替のボタンが見つからない）。`/problems/new`の2件、`/blog/new`の数式、`contest-setters.spec.ts`の追加の確認は通る。

- [ ] **Step 3: 記事の編集画面のプレビューを遅らせる**

`blog/new.vue`に、`problems/new.vue:86-99`と同じ仕組みを置く。
- `renderedSource = ref(markdown.value)`と`previewTimer`。
- `watch(markdown, …)`で前のタイマーを消し、150ms後に`renderedSource.value = markdown.value`とする。
- `watch(ready, …)`で待たずに`renderedSource.value = markdown.value`とする（Ruling）。
- `onBeforeUnmount`でタイマーを消す。
- 217-218行の`ProblemMarkdown`の`v-if`と`:source`を`renderedSource`にする。`preview-title`のタイトルは今のまま（問題の編集画面と同じく待たない）。

- [ ] **Step 4: コンテスト編集画面を揃える**

`ContestEditor.vue`:
- `mathSnippet`を`'\n```math\n\\sum_{i=1}^{N} A_i\n```\n'`にする（`problems/new.vue:109`、`blog/new.vue:124`と同じ文字列）。
- Step 3と同じプレビューの仕組みを、`fields.description`に対して置く（`watch(() => fields.description, …)`と`watch(ready, …)`）。558-559行の`ProblemMarkdown`の`v-if`と`:source`を`renderedSource`にする。
- `editor-topbar`の`<h1 class="visually-hidden">`の直後に`<ThemeSelect />`を置く（Ruling）。

- [ ] **Step 5: 通ることを確かめる**

Run: ビルド。続けてPlaywright（`<specs>` = `tests/markdown-editor.spec.ts tests/theme.spec.ts tests/contest-setters.spec.ts tests/details.spec.ts tests/blog-editor.spec.ts tests/blog.spec.ts tests/editor.spec.ts tests/scroll-sync.spec.ts tests/contest-editor-form.spec.ts tests/contest-problem-picker.spec.ts`）
Expected: PASS（全件。`details.spec.ts`の`/blog/new`・`/my/contests/new`で編集しながら折りたたみを確かめるテストと、`contest-problem-picker.spec.ts:74`の320pxで横にはみ出さない確認を含む）

- [ ] **Step 6: コミット**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors && git add web/app/pages/blog/new.vue web/app/components/ContestEditor.vue web/tests/markdown-editor.spec.ts web/tests/theme.spec.ts web/tests/contest-setters.spec.ts && git commit -m "fix: 記事とコンテストのMarkdownエディターのプレビューの遅延・数式スニペット・配色の切替を、問題の編集画面に揃える" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

## 検証コマンド（controllerが全体を流す）

どれも1行で、laneの`web`で実行する。Playwrightの2つは時間がかかるので、`run_in_background`で流す。

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck` → エラーなし
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build` → エラーなし
3. ブラウザーのPlaywright: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command flock /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/playwright.lock env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/editors/pw-b5` → すべてpassed
4. アカウントのPlaywright: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command flock /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/playwright.lock env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test --config playwright.account.config.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/editors/pw-b5-account` → すべてpassed（`account/contests.spec.ts`がコンテスト編集画面の各幅を、`account/publication.spec.ts`が記事の読み込み後のプレビューを確かめる）
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run package:lambda`、同じ形で`npm run test:lambda`と`node --test scripts/smoke-frontend.test.mjs` → どれも終了コード0
6. Task 1のStep 4の`grep` → 出力なし
7. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/lanes/editors && git status --short && git log --oneline 541fa0e..HEAD` → 作業ツリーはきれい。Task 1〜5のコミットが順に並ぶ（そのあとに簡潔化や修正のコミットが続いてもよい）

## Out of scope

- 新しい問題の生成コードの既定の言語は`cpp23-gcc`の固定で（`shared/types/problem-draft.ts:17`の`emptyGenerators`）、検証コード・集計コードのように`preferredRuntime`を使わない。D14の後は、ジャッジが`cpp23-gcc`を出していないと「現在利用できません」と出る。既定の選び方を変えるのは仕様の外なので、報告に留める。
- ダイアログを開いている間に`watch(draft)`が張るタイマーで自動保存が走ること。D24の仕様は「常に1本」と「閉じるときの張り直し」だけで、開いている間の保存は変えない（C04の計画のとおり）。
- 保存ショートカットとプレビューのタイマーの共通化、3画面の部品化（C18で却下）。
- `problems/new.vue`の挙動。
- `app/content/docs/cli.md:71`の`time_limit_ms = 2000`。CLIの`problem.toml`の設定例で、D16の対象（Webの`exportProblemMarkdown`）とは別物なので残す。
- コンテスト編集画面の日時の表示と入力（D19、B7で扱う）。
- `docs/refactor/FIXES.md`の「まとまりと順序」の状態とコミットの更新、graphifyの更新。controllerが行う。
