# C04 作問画面の草稿の既定値をinitialProblemDraftに、自動保存の再予約をscheduleAutosaveにまとめる 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `useProblemDraft.ts`で2回書かれている新しい草稿の既定値を、`utils/problem-draft.ts`の`initialProblemDraft()`の1か所にまとめる（Task 1）。3か所にある600msの自動保存の再予約を、`scheduleAutosave()`の1か所にまとめる（Task 2）。

**Architecture:** どちらも、先にPlaywrightで外から見える挙動を固定する。Task 1では、読み込みに失敗したときに、キャッシュの値を既定値で上書きすることを固定する。Task 2では、ダイアログを閉じたあとの自動保存の回数と時刻を、`page.clock`で固定する。変更は`web/app/composables/useProblemDraft.ts`と`web/app/utils/problem-draft.ts`の中だけで閉じる。

**Tech Stack:** Nuxt 4 / Vue 3（`web`）、TypeScript、Playwright 1.55（UIテストは`tests/fixtures/account.ts`の偽のAPIを使う）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C04.json`（2つの反証の`revised_scope`、`must_preserve`、`test_plan`）

## 困りごと

- 新しい草稿の既定値12項目が、`useProblemDraft.ts:56`（`reactive`の初期化）と`:218`（読み込みに失敗したときの`Object.assign`）に、同じ内容の長い1行で書かれている。
  - 項目を足すたびに両方を直してきた。`git log -G "timeLimitMs: '2000'" -- web/app`で、両方を直したコミットが10件以上ある（1eea4f1、253ffe9、ee7bafb、d646140、d831f72、5943ab4など）。5943ab4（scoring）は昨日のコミットである。
  - `:204-205`で、取得の前にキャッシュの草稿を`draft`へ入れている。そのため、`:218`に項目を足し忘れると、読み込みに失敗したときにキャッシュの値が画面に残る。この経路を確かめるテストは今はない（`load-failed`を使うテストが1件もない）。
- 600msの自動保存の予約が`:237`、`:260`、`:282`の3か所にあり、どれにも`!manualSaveOnly.value`が書かれている。c118c35では、手動保存だけの問題を入れたときに3か所すべてに条件を足した。1か所でも漏れると、`:106`のガードが「変更を反映するには保存ボタンを押してください。」を勝手に出す。

## 判定の記録（反証の差と、計画で決めた点）

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| ファクトリの置き場所 | utilsでもcomposable内でもよい | utils | `utils/problem-draft.ts`の`initialEditorialMarkdown`の直後。初期本文のテンプレートと同じ場所にする |
| 遅延の定数 | 名前付きの定数にする | `autosaveDelayMs` | `useProblemDraft.ts`のモジュールの先頭に`const autosaveDelayMs = 600`を置く。名前の付け方は周り（`inlineTestDataLimit`など）に合わせる |
| 再予約のテストの時間 | 実時間で回数だけを確かめる | `page.clock`で599/600ms | `page.clock`を使う。偽の時計なら境界の判定が決まった結果になり、実時間で待つ必要もない。編集してから離脱リンクを押すまでに600msを超えるという競合（D1）も起きない |
| 「PUTの回数は変わらない」 | そのまま | 「手動の操作の範囲では」と限定する | safetyの書き方にする。微差はTask 2のコミットメッセージに書く |
| T1の準備 | UIで値を変えて保存する | 同じ | 偽のAPIへ`fetch`でPUTしてから開く（`editor.spec.ts`の「legacy memory limits」と同じ方法）。成功した読み込みがキャッシュを書くので、通る経路は同じである。UIの操作による不安定さを避けられる |
| テストの置き場所 | — | 新しいspecか`editor.spec.ts` | T1は`editor.spec.ts`に置く。T2とT3は`problem-save-behavior.spec.ts`、T4は`manual-problem-save.spec.ts`に置く。どれも主題が同じ既存ファイルである |
| タスクの分割 | 2コミット | 2コミット | 2タスクにする。(b)は根拠が(a)より弱いので、レビューで(b)だけを不採用にできるようにする |
| 変異で確かめること | — | — | 各タスクで1回ずつ確かめる。Task 1では`:218`から`tags`を抜く。Task 2では`manualSaveOnly`の判定を外す。後者は既存の`manual-problem-save.spec.ts`では見つからない見込みである（書き込み回数しか見ておらず、`:106`のガードがPUTを止めるため） |

## Global Constraints

- 公開HTTP API、CLI、DBスキーマ、依存のバージョン（`web/package.json`、`package-lock.json`）、CI・インフラを変えない。
- 変更してよいファイルは次の5つだけ。
  - `web/app/composables/useProblemDraft.ts`
  - `web/app/utils/problem-draft.ts`
  - `web/tests/editor.spec.ts`
  - `web/tests/problem-save-behavior.spec.ts`
  - `web/tests/manual-problem-save.spec.ts`
- ステージ済みの`api/`の4ファイル（別のサイクルの変更）には触れない。
- テストは追加だけにする。既存のテストとアサーションは変えず、削除もスキップもしない。
- 新規作成時の初期値を変えない。
  - `checker`、`interactor`、`scoring`、`difficulty`は`null`
  - `title`は`''`
  - `markdown`は`initialProblemMarkdown`、`editorial`は`initialEditorialMarkdown`
  - `tags`と`testCases`は`[]`
  - `generators`は`emptyGenerators()`
  - `timeLimitMs`は`'2000'`、`memoryLimitMb`は`'512'`（どちらも文字列）
- `initialProblemDraft()`は、呼ぶたびに新しいオブジェクトを返す。`tags`、`testCases`、`emptyGenerators()`の結果も、呼ぶたびに新しく作る。モジュールのレベルの定数や配列を返したり、中に含めたりしない。キーの順序と型の注記（`null as ProblemDraft['checker']`、`null as number | null`、`[] as string[]`、`[] as TestCase[]`など）は、今の`:56`と同じにする。
- `draft`の推論型を変えない。`npm run typecheck`が通れば、`draftErrors`、`persistedDraft`、`scoringError`、`new.vue`のv-modelの型も保たれている。
- 読み込みに失敗したとき（`:216-221`）の順序を守る。
  1. `removeProblemCache`
  2. 既定値で上書きする
  3. `saveState = 'load-failed'`
  4. `storageError = accountError(error)`
  5. `return`する。`ready`は`false`のままで、`saved`の更新とリスナーの登録には進まない。
- 次の箇所には触れない。
  - キャッシュを先に代入する`:204-205`
  - `:211`の512 MiBへの丸め
  - `:202-203`の、到達しない再確認
- `scheduleAutosave()`がすることは2つだけにする。
  1. `clearTimeout(saveTimer)`
  2. `manualSaveOnly.value`が偽なら、`saveTimer = setTimeout(() => saveDraft(), autosaveDelayMs)`とする。`saveDraft()`は引数なしで呼ぶ。引数なしの呼び出しが自動保存として扱われる。
- `scheduleAutosave()`の中に、`fingerprint() !== saved`などのほかの条件を入れない。`saveDraft:106`のガードとも一本化しない。
- 呼び出し側に固有の条件と順序は、呼び出し側に残す。
  - `watch(draft)`：`ready`でなければ何もしない。そのあと、`saveState = 'dirty'`にしてから`scheduleAutosave()`を呼ぶ。
  - `finishLeave`：`resolve?.(choice !== 'stay')`のあとで、`choice === 'stay' && fingerprint() !== saved`のときに呼ぶ。
  - `closeDeleteConfirmation`：`close()`と`confirmingDelete.value = false`のあとで、`!deleted && fingerprint() !== saved`のときに呼ぶ。
- 単に`clearTimeout(saveTimer)`しているだけの箇所（`:107`、`:164`、`:244`、`:267`、`:273`、`:285`）は変えない。ダイアログを開いている間に`watch`がタイマーを張り直す今の挙動に、新しいガードを足さない。
- `saveDraft`のガード、`inFlight`の直列化、失われた応答の照合（`:95-158`）には触れない。
- コードのコメントは英語にし、周りと同じ密度（1行程度）にする。計画とレポートは日本語で書く。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターがタスクごとに行う。
- `.output`とPlaywrightのポート（13000、13001、18080、18081）は、ほかのサイクルと共有している。ビルドとPlaywrightを始める前に、毎回次を実行する。
  `pgrep -af '[n]uxt build|[p]laywright test|[.]output/server/index[.]mjs'`
  角括弧は、Bashの包みのシェル自身に一致しないようにするためのものである。何か出たら、ほかの検証が動いている。Monitorでその終了を待ってから始める。ほかのプロセスは止めない。
- 10分を超えうるコマンド（ビルドとPlaywright）は、Bashの`run_in_background`で実行して、終了の通知を待つ。

## Review Focus

1. 既定値の参照の共有。オブジェクトの定数を`reactive()`に渡すと、プロキシが元のオブジェクトを書き換える。その場合、`Object.assign(draft, 同じ定数)`は何もしないので、キャッシュの値が残る。T1はこれを検出する。一方、`tags`だけを共有の空配列にするような部分的な共有は、T1では検出できない。差分を読んで確かめる。
2. 読み込みに失敗したときの順序と、`ready`が`false`のままであること。これはT1が検出する。具体的には、キャッシュのキーが消えること、`load-failed`になること、警告の文言が出ること、タイトルと保存ボタンが無効になることを確かめる。
3. `scheduleAutosave`に`fingerprint() !== saved`を入れる「簡略化」は、元に戻した編集で「保存済み」に戻らなくする（aa60308の不具合が再発する）。これは既存の`problem-scoring.spec.ts`の「the switch starts off…」が検出する。`manualSaveOnly`の判定を消して`:106`に任せる案は、T4の「文言が出ない」で検出する。
4. 呼び出し側の条件と順序を取り違えること。`finishLeave`で`resolve`より前に予約しても、外からは見えない。`closeDeleteConfirmation`の`fingerprint() !== saved`を落としても、編集がなければ`saveDraft:100`がPUTせずに`saved`へ戻すので、T3の前半では検出できない。どちらも差分を読んで確かめる。
5. 閉じるときに`clearTimeout`が先に入る点は、意図した微差である。手動の操作では、閉じる時点に保留中のタイマーはない（開くときに消えている）。そのため、T2とT3の599ms/600msの境界は、変更の前後で同じになる。保留中のタイマーが残るのは、ダイアログを開いたあとに草稿が非同期で変わる場合だけである。例は2つある。
   - 「保存して移動」の途中で、`prepareTestFiles`がケースに`File`を書き込んだ場合
   - ダイアログの裏で生成が完了した場合

   この場合、今のコードは前のタイマーの参照を捨てる（D2）。新しいコードはそのタイマーを消し、1本だけ張り直す。テストでは再現できないので、差分を読んで確かめる。

---

### Task 1: 新しい草稿の既定値を`initialProblemDraft()`にまとめる

**Files:**
- Test: `web/tests/editor.spec.ts`（`'cache failures do not prevent DB saving'`のテストの直後、現在の94行の後に足す）
- Modify: `web/app/utils/problem-draft.ts`（末尾の`initialEditorialMarkdown`の後）
- Modify: `web/app/composables/useProblemDraft.ts:3-4, :56, :218`

**Interfaces:**
- Produces: `export function initialProblemDraft()`（`web/app/utils/problem-draft.ts`）。戻り値の型は推論に任せ、今の`:56`のオブジェクトリテラルと同じ型にする。

- [ ] **Step 1: 特性テストT1を追加する**

```ts
test('a failed reload replaces the cached draft with the new-problem defaults', async ({ page }) => {
  const id = '22222222-2222-4222-8222-222222222222'
  await page.goto('/problems/new')
  await page.evaluate(async (id) => {
    await fetch(`/api/my/problems/${id}`, {
      method: 'PUT', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ version: 0, draft: { title: 'キャッシュの題名', markdown: '## キャッシュの本文', difficulty: 3, tags: ['キャッシュ'], timeLimitMs: '3000', memoryLimitMb: '256', testCases: [] } }),
    })
  }, id)
  // A successful read caches the draft.
  await page.goto(`/problems/new?problem=${id}`)
  await expect(page.locator('#problem-title')).toHaveValue('キャッシュの題名')
  const cached = () => page.evaluate(id => Object.keys(sessionStorage).some(key => key.endsWith(`.${id}`)), id)
  expect(await cached()).toBe(true)
  let release!: () => void
  const hold = new Promise<void>(resolve => { release = resolve })
  await page.route(`**/api/my/problems/${id}`, async route => {
    if (route.request().method() !== 'GET') return route.fallback()
    await hold
    return route.fulfill({ status: 500, json: {} })
  })
  await page.reload()
  // The cached draft shows while the read is pending.
  await expect(page.locator('#problem-title')).toHaveValue('キャッシュの題名')
  await expect(page.locator('#time-limit')).toHaveAttribute('aria-valuenow', '3000')
  release()
  await expect(page.locator('[data-save-state]')).toHaveAttribute('data-save-state', 'load-failed')
  await expect(page.getByRole('alert')).toHaveText('サーバーに接続できないか、処理に失敗しました。入力内容を保持したまま、もう一度お試しください。')
  await expect(page.locator('#problem-title')).toHaveValue('')
  await expect(page.locator('#problem-title')).toBeDisabled()
  await expect(page.getByRole('button', { name: '保存', exact: true })).toBeDisabled()
  await expect(page.locator('#problem-difficulty')).toHaveText(/未設定/)
  await expect(page.locator('#time-limit')).toHaveAttribute('aria-valuenow', '2000')
  await expect(page.locator('#memory-limit')).toHaveAttribute('aria-valuenow', '512')
  await expect(page.getByRole('list', { name: 'タグ' })).toHaveCount(0)
  await expect(page.locator('#problem-source')).toContainText('<!-- ここに問題文を記載 -->')
  await expect(page.locator('#problem-source')).not.toContainText('キャッシュの本文')
  expect(await cached()).toBe(false)
})
```

外から見えるのは、タイトル、本文、難易度、時間、メモリ、タグの6項目である。残りの6項目（`checker`、`interactor`、`scoring`、`editorial`、`generators`、`testCases`）は、`new.vue:163-167`でほかのセクションが無効になるため観測できない。この6項目は、ファクトリの共通化と`typecheck`で担保する。

- [ ] **Step 2: 変更前のアプリをビルドし、T1が成功することを確かめる**

1. `pgrep -af '[n]uxt build|[p]laywright test|[.]output/server/index[.]mjs'`を実行する。出力がないこと（Global Constraintsを参照）。
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`を実行する。終了コードが0になること。
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/editor.spec.ts -g "failed reload" --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c04`を実行する。`1 passed`になること。

失敗した場合は、まずロケーターと文言を、`new.vue`と`accountError`の実際の表示に合わせて直す。挙動そのものが計画と違う場合は、コードを直さずに止まって報告する。

- [ ] **Step 3: T1がリセット漏れを検出することを確かめる（一時的な変異）**

1. Editで`useProblemDraft.ts:218`から`tags: [] as string[], `を一時的に消す。
2. Step 2の1〜3（ビルドとT1）を実行する。T1が失敗し、`getByRole('list', { name: 'タグ' })`の件数が1になること。
3. Editで元に戻す。`git diff -- web/app/composables/useProblemDraft.ts`の出力が空であること。

- [ ] **Step 4: `initialProblemDraft()`を追加し、2か所を置き換える**

- `web/app/utils/problem-draft.ts`
  - 1行目のimportに`emptyGenerators`を足す。
  - 末尾の`initialEditorialMarkdown`の後に、`export function initialProblemDraft()`を置く。中身は今の`:56`のオブジェクトリテラルを`return`するだけにする（キーの順序と型の注記も同じにする）。
  - 英語で1行のコメントを付ける。例：`// A new problem's draft. Each call builds new arrays and generators, so the editor's draft and its reset share nothing.`
- `web/app/composables/useProblemDraft.ts`
  - `:56`を`const draft = reactive(initialProblemDraft())`にする。
  - `:218`を`Object.assign(draft, initialProblemDraft())`にする。
  - importから、使わなくなった`emptyGenerators`と`type ProblemDraft`（3行目）、`initialEditorialMarkdown`、`initialProblemMarkdown`、`type TestCase`（4行目）を外し、`initialProblemDraft`を足す。外す前に、grepで、ほかに使っている箇所がないことを確かめる。
- `app/utils`はNuxtのauto-importの対象である。`initialProblemDraft`と同じ名前のexportがほかにないことを確かめる。
  `grep -rn "initialProblemDraft" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app /home/loop/judge/.claude/worktrees/refactor-maintainability/web/shared /home/loop/judge/.claude/worktrees/refactor-maintainability/web/server`
  ヒットするのが、定義と`useProblemDraft.ts`の3か所（import、`:56`、`:218`）だけであること。

- [ ] **Step 5: 検証する**

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`を実行する。エラーがないこと。
2. 先に`pgrep`で、ほかの検証が動いていないことを確かめる。そのあと、Step 2-2のビルドを実行する。
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/editor.spec.ts tests/draft-library.spec.ts tests/manage-problem.spec.ts tests/problem-navigation.spec.ts tests/problem-save-behavior.spec.ts tests/problem-editor-copy.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c04`を実行する。すべて`passed`になること。
4. `grep -c "timeLimitMs: '2000'" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app/composables/useProblemDraft.ts /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app/utils/problem-draft.ts`を実行する。結果が`0`と`1`になること。
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`を実行する。

コミットメッセージの案（オーケストレーター用）：`refactor: 作問画面の新しい草稿の既定値をinitialProblemDraftにまとめ、読み込みに失敗したときの既定値への上書きを特性テストで固定する`

### Task 2: 自動保存の再予約を`scheduleAutosave()`にまとめる

**Files:**
- Test: `web/tests/problem-save-behavior.spec.ts`（末尾）
- Test: `web/tests/manual-problem-save.spec.ts`（既存の`for (const mode of ['published', 'contest'])`のループの直後、現在の42行の後に足す）
- Modify: `web/app/composables/useProblemDraft.ts`（モジュールの先頭、`:233-238`、`:260`、`:282`）

**Interfaces:**
- Consumes: Task 1の後の`useProblemDraft.ts`
- Produces: `const autosaveDelayMs = 600`（モジュールのレベル、`saveMessages`の直後）と、`function scheduleAutosave(): void`（`useProblemDraft()`の中、`watch(draft, …)`の直前）。どちらもexportしない。

- [ ] **Step 1: 特性テストT2とT3を`problem-save-behavior.spec.ts`に追加する**

T2とT3の準備は同じである。

1. `/problems/new?fresh=1`でタイトルを入力し、`保存`を押す。`toHaveURL(/problem=/)`になり、`[data-save-state]`が`saved`になるまで待つ。URLにはすでに問題のIDがあるので、以後の保存でURLを書き換える処理（`router.replace`）は走らず、時計にも依存しない。
2. `page.clock.install()`と`page.clock.pauseAt(new Date(Date.now() + 1000))`を呼ぶ（`save-shortcut.spec.ts`と同じ方法）。
3. 次のようにPUTのタイトルを記録する。
   `const titles: string[] = []; page.on('request', request => { if (request.method() === 'PUT' && request.url().includes('/api/my/problems/')) titles.push(request.postDataJSON().draft.title) })`
4. `const status = page.locator('[data-save-state]')`とする。

T2では、次の手順で確かめる。テスト名は`'keeping on editing from the leave dialog autosaves the edit once after the debounce'`とする。

1. `#problem-title`に`'続きの編集'`を入力する。
2. `getByRole('link', { name: 'ShareOJ マイページ' })`を押す。ダイアログが表示されること。
3. `編集を続ける`を押す。ダイアログが閉じること。
4. `page.clock.runFor(599)`を呼ぶ。`status`が`dirty`のままで、`titles`が`[]`であること。
5. `page.clock.runFor(1)`を呼ぶ。`expect.poll(() => titles).toEqual(['続きの編集'])`となり、`status`が`saved`になること。
6. `page.clock.runFor(2000)`を呼ぶ。`titles`が`['続きの編集']`のままであること。

T3では、次の手順で確かめる。テスト名は`'cancelling the delete confirmation autosaves only an unsaved edit, once after the debounce'`とする。テストの中に、削除の確認を開いてキャンセルする手順をまとめた関数を置く。関数の手順は次のとおり。

- `問題管理`を押す。
- `問題を削除`を押す。ダイアログが表示されること。
- `キャンセル`を押す。ダイアログが閉じること。

1. 編集する前に、この関数を呼ぶ。そのあと`runFor(2000)`を呼ぶ。`titles`が`[]`で、`status`が`saved`であること。
2. `問題文`を押し、`#problem-title`に`'削除せずに編集'`を入力して、もう一度この関数を呼ぶ。
3. `runFor(599)`を呼ぶ。`dirty`のままで、`[]`であること。
4. `runFor(1)`を呼ぶ。`['削除せずに編集']`となり、`saved`になること。
5. `runFor(2000)`を呼ぶ。1件のままであること。

- [ ] **Step 2: 特性テストT4を`manual-problem-save.spec.ts`に追加する**

テスト名は`` `${mode} problems stay unsaved after the leave and delete dialogs close` ``とし、`for (const mode of ['published', 'contest'])`のループで回す。

- 準備：既存のテストの5-24行と同じ手順を、新しいテストの中に書く。
  1. 保存する。
  2. `stored`を取得し、`publishedVersion`か`contestId`を差し替える。
  3. `page.route`でPUTを`writes`として数える。
  4. 再読み込みする。
  5. `#problem-title`が有効になるのを待つ。

  既存のテストは変えない。
- そのあと、Step 1の2（時計）を同じように行う。
- 文言は`const reminder = page.getByText('変更を反映するには保存ボタンを押してください。')`で取る。

次の手順で確かめる。

1. `#problem-title`に`'ダイアログ後も未保存'`を入力する。
2. 離脱リンクを押す。
3. `編集を続ける`を押し、ダイアログが閉じるのを待つ。
4. `runFor(2000)`を呼ぶ。次の3点を確かめる。
   - `writes`が`0`である。
   - `[data-save-state]`が`dirty`である。
   - `reminder`の件数が`0`である。
5. `問題管理`、`問題を削除`、`キャンセル`の順に押し、ダイアログが閉じるのを待つ。
6. `runFor(2000)`を呼ぶ。4と同じ3点を確かめる。

- [ ] **Step 3: 変更前にT2、T3、T4が成功することを確かめる**

`.output`は、Task 1のStep 5のビルドのままでよい（Task 2のアプリの変更はまだ入っていない）。ほかの検証がその後にビルドし直したかもしれない場合は、`pgrep`で確かめてから、もう一度ビルドする。

`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/problem-save-behavior.spec.ts tests/manual-problem-save.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c04`

すべて`passed`になること（追加した4件と既存の6件、計10件）。

失敗した場合の扱いは、Task 1のStep 2と同じにする。

- [ ] **Step 4: `scheduleAutosave()`を追加し、3か所から呼ぶ**

- `const autosaveDelayMs = 600`を、`saveMessages`の直後に置く。
- `function scheduleAutosave()`を、`watch(draft, …)`の直前に置く。中身はGlobal Constraintsに書いた2つの処理だけにする。英語で1行のコメントを付ける。例：`// Restarts the autosave debounce; problems that save only on an explicit action never write on their own.`
- 呼び出し側を置き換える。
  - `:236-237`は`scheduleAutosave()`の1行にする。直前の`saveState.value = 'dirty'`は残す。
  - `:260`は`if (choice === 'stay' && fingerprint() !== saved) scheduleAutosave()`にする。
  - `:282`は`if (!deleted && fingerprint() !== saved) scheduleAutosave()`にする。
- `grep -n "setTimeout(() => saveDraft()" /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app/composables/useProblemDraft.ts`を実行する。`scheduleAutosave`の中の1行だけがヒットすること。

- [ ] **Step 5: T4が手動保存のガードの漏れを検出することを確かめる（一時的な変異）**

1. Editで`scheduleAutosave`の中の`if (!manualSaveOnly.value) `を一時的に消し、常に予約するようにする。
2. 先に`pgrep`で、ほかの検証が動いていないことを確かめる。そのあとビルドして、Step 3のコマンドを実行する。次の2点を確かめる。
   - T4の2件が、`reminder`の件数が1になって失敗すること。
   - 既存の`published/contest problems save only on an explicit action`が成功すること。これは、既存のテストではこの変異を検出できないことを示す。結果をレポートに書く。
3. Editで元に戻す。`scheduleAutosave`の中身がStep 4の形に戻っていることを確かめる。

- [ ] **Step 6: 検証する**

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`を実行する。エラーがないこと。
2. 先に`pgrep`で確かめてから、`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`を実行する。終了コードが0になること。
3. 関係するspecを実行する。
   `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/problem-save-behavior.spec.ts tests/manual-problem-save.spec.ts tests/editor.spec.ts tests/leave-editor.spec.ts tests/manage-problem.spec.ts tests/problem-navigation.spec.ts tests/save-shortcut.spec.ts tests/problem-scoring.spec.ts tests/draft-library.spec.ts tests/problem-editor-copy.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c04`
   すべて`passed`になること。`problem-scoring.spec.ts`には、aa60308で入れた、元に戻した編集で「保存済み」に戻るテストが含まれる。
4. ブラウザーのテストを全件実行する。
   `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c04-all`
   `failed`が0件になること。作問画面と関係のないspecが失敗した場合は、そのspecだけを2回再実行する。成功すれば、不安定なテストとしてレポートに記録する。`leave-editor.spec.ts`の1つ目のテストは、実時間の600msに依存している（D1）。これが失敗した場合も、同じように扱う。
5. DBを使うPlaywrightを実行する（できる場合だけ）。
   `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' npx playwright test --config playwright.account.config.ts tests/account/persistence.spec.ts tests/account/publication.spec.ts tests/account/test-cases.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c04-account`
   すべて`passed`になること。次の場合は、実行しなかったことと、その理由をレポートに書く。
   - `api/`のステージ済みの変更（別のサイクル）が原因で、Goのフィクスチャが起動しない場合
   - ポート18082または13002が使用中の場合
6. lintは`web/package.json`にスクリプトがないので、「対象なし」と記録する。
7. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short`を実行する。新しく変わっているのが、Global Constraintsに書いた5ファイルだけであること。ステージ済みの`api/`の4ファイルが開始時のままであること。
8. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`を実行する。

コミットメッセージの案（オーケストレーター用）：

```
refactor: 作問画面の自動保存の再予約をscheduleAutosaveにまとめる

ダイアログを閉じるときも、保留中のタイマーを先に消すようにした。
手動の操作の範囲では、PUTの回数と最終的な保存状態は変わらない。

意図した微差：ダイアログを開いたあとに草稿が非同期で変わった場合は、タイマーが1本にまとまる。
そのため、2本のタイマーの間にさらに編集すると、PUTが1回減ることがある。
```

## 完了条件

- T1〜T4（T2とT3は1件ずつ、T4は2件）が、変更前（Task 1のStep 2、Task 2のStep 3）と変更後（Task 2のStep 6）の両方で成功している。
- 2つの変異（Task 1のStep 3、Task 2のStep 5）で、計画どおりにテストが失敗した。どちらの変異も元に戻した。
- `npm run typecheck`と`npm run build`が成功し、関係するspecとブラウザーの全件が成功している（不安定なテストを記録した場合は、再実行で成功したことも記録する）。
- 既定値のオブジェクトリテラルは`initialProblemDraft()`の1か所だけにある。`setTimeout(() => saveDraft(), …)`は`scheduleAutosave()`の1か所だけにある。
- 変わったファイルは5つだけで、`api/`のステージ済みの変更はそのままである。
- `graphify update .`を実行した。

## 範囲外

- `useProblemDraft.ts:202-203`の到達しない再確認（D3）。
- `:204-205`でキャッシュを先に入れることと、`:211`の丸め。
- サーバーのzodの既定値（例：`problemDraftSchema`の`editorial`は`''`）や、テストのフィクスチャを`initialProblemDraft()`とまとめること。サーバー側の既定値は、意図して違う値になっている。
- `saveDraft`のガード、`inFlight`、失われた応答の照合。`:106`のガードの文言と、そこに至る経路（`createTesterLink`、`TestCaseGenerator`の`save()`）。
- 単に`clearTimeout`しているだけの6か所。ダイアログを開いている間に`watch`がタイマーを張り直す挙動。
- 別件として記録だけするもの。
  - D1：`leave-editor.spec.ts:3-21`は、`:13`から`:16`までが600msを超えると失敗しうる（テストが不安定になるだけ）。
  - D2：今の`:260`と`:282`は、`clearTimeout`せずに`saveTimer`を上書きする。Task 2で結果として解消する。
  - D4：`web`にlintのスクリプトがない。
