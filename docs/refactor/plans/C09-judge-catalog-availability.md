# C09 利用できる言語の判定と既定の言語をutils/judge-catalog.tsにまとめる 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 4つのコンポーネントに写されている「ジャッジを使えないときは言語を選べなくする」式を`usableRuntimes()`に、2つに写されている既定の言語の式を`preferredRuntime()`にまとめる。2つの関数は新しい`web/app/utils/judge-catalog.ts`に置く。

**Architecture:** 先にPlaywrightで、4つの画面の挙動を固定する（Task 1）。対象は、メンテナンス中と取得失敗のときに言語が並ばないことと、判定コード・集計コードの既定の言語。次に純粋関数を作ってnodeテストで固定し、4つのコンポーネントの式を置き換える（Task 2）。`useJudgeCatalog()`とその呼び出しは変えない。

**Tech Stack:** Nuxt 4.6.1 / Vue 3.5（`web`）、TypeScript、Playwright 1.55（UIテストは`tests/fixtures/account.ts`の偽のAPIを使う）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C09.json`（worthの`revised_scope`に従う。両方の`must_preserve`と`test_plan`も参照する）

## 困りごと

- 次の同じ式が4か所にある（計画者が実コードで確認した）。
  - 式：`catalogError.value || catalog.value?.maintenance ? [] : catalog.value?.items ?? []`
  - 場所：`SubmissionForm.vue:12`、`TestCaseGenerator.vue:20`、`ProblemChecker.vue:11`、`ScoringEditor.vue:13`
  - この規則を作ったのは468cbeeで、3か所に同時に書いた。直し漏れの履歴ではない。5943ab4で`ScoringEditor`を足したときに、4か所目として写された。ジャッジの言語を使う画面が増えるたびに、名前のない規則がまた写される。
- 既定の言語の式`available.value.find(item => item.id === 'cpp23-gcc')?.id ?? available.value[0]?.id ?? 'cpp23-gcc'`は2か所にある（`ProblemChecker.vue:16`はbc1329cで作られ、`ScoringEditor.vue:25`は5943ab4で写された）。既定の言語の文字列はほかにも`SubmissionForm.vue:10`、`shared/types/problem-draft.ts:10,15,17`にある。この変更で1か所になるのは、2か所にある同じ式だけである。
- 今のところ、漏れの影響はUXだけにとどまる。
  - サーバーはメンテナンス中に空の`items`を返し、提出を503で断る（`api/internal/submissions/config.go:13-16`）。
  - Nuxt 4.6.1は、取得に失敗すると`data`を既定値（undefined）に戻す（`node_modules/nuxt/dist/app/composables/asyncData.js:418-419`）。
  - そのため、クライアントのこの規則は二重の防御である。価値は低〜中で、規則に名前を付けて1か所にし、テストで固定することが目的になる。
- 判定コード、集計コード、生成の画面には、メンテナンス中の挙動を確かめるテストがない。集計コードの既定の言語にもテストがない。「cpp23-gccが先頭でないときも、cpp23-gccを選ぶ」ことを確かめるテストもない。

## 判定の記録

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| ファイル名 | `utils/judge-catalog.ts` | `utils/runtime-catalog.ts`（例） | `web/app/utils/judge-catalog.ts`。worthの`revised_scope`に従い、`useJudgeCatalog`やキー`'judge-catalog'`と名前を揃える |
| 変更前の特性テスト | nodeテストに加え、E2Eは最低1本 | E2Eの(a)〜(d)が条件 | Task 1でE2Eを6件足す。nodeテストは、Task 2で関数より先に書く |
| メンテナンスのモック | — | `{ items: [], maintenance: true }` | `items`に言語を並べたまま`maintenance: true`を返す。本物のサーバーはメンテナンス中に`[]`を返すので、`[]`では、フラグを見ない画面を見分けられない。サーバー側の規則はGoのテストの範囲である |
| 502のときの古いdata | 古いdataが残るときに差が出る | — | Nuxt 4.6.1では、取得に失敗するとdataが消える（上記）。502のテストは、配線の誤りを見つけるための画面単位の固定とする。`error`の分岐そのものはnodeテストで固定する |
| 提出画面のE2E | — | 既存の`maintenance.spec.ts`で足りる | 2件足す。既存のテストはメンテナンス中に`items: []`を返すので、提出画面がフラグを見なくなっても成功してしまう |
| 依存の追跡の差（safetyの3） | — | 結果は同じ | 受け入れる。今は`catalogError`があるとcatalogを読まないが、関数にすると引数として両方を読む。ただし、エラーの間のdataはundefinedのままなので、再計算は増えない |
| 型 | — | — | `usableRuntimes<T>`をジェネリックにする。言語の項目の形（`server/api/runtimes.get.ts`のzod）を2か所目に書かずに済む |
| タスクの分割 | — | 特性テストを先に | 2つに分ける。worthが優先度を低〜中としているので、Task 1のテストだけを採ることもできるようにする |

## Global Constraints

- 公開HTTP API、CLI、DBスキーマ、依存のバージョン（`web/package.json`、`package-lock.json`）、CI・インフラを変えない。
- 変更してよいファイルは次の6つだけ。
  - 新規：`web/app/utils/judge-catalog.ts`、`web/tests/judge-catalog.spec.ts`
  - 変更：`web/app/components/SubmissionForm.vue`、`TestCaseGenerator.vue`、`ProblemChecker.vue`、`ScoringEditor.vue`
- ステージ済みの`api/`の3ファイル（別のサイクルの変更）には触れない。
- テストは追加だけにする。既存のテストは変更・削除・スキップしない。
- `usableRuntimes(catalog, error)`は、次の値を返す。
  - `error`が真か`catalog?.maintenance`が真なら、`[]`
  - それ以外は`catalog?.items ?? []`
  - 真偽値として判定する。`=== true`や`!== false`にはしない。`maintenance`がないとき（ルートのモックが省く）は`items`を返す。
  - `items`はコピーもfilterもせず、同じ配列を返す。
- `preferredRuntime(items)`は、`items`にcpp23-gccがあれば位置に関係なく`'cpp23-gcc'`を返す。なければ`items[0]?.id`、空なら`'cpp23-gcc'`を返す。フォールバックは`??`のままにする（`||`にしない）。
- `judge-catalog.ts`は何もimportせず、Nuxtの自動インポート（`useFetch`など）も使わない。こうすると、nodeテストから`'../app/utils/judge-catalog'`で直接importできる。
- 4つのコンポーネントは、`import { ... } from '~/utils/judge-catalog'`で明示的にimportする（`runtime-label.ts`などの使い方に合わせる）。
- `ScoringEditor.vue`は`const defaultRuntime = () => preferredRuntime(available.value)`とし、呼ぶたびに評価する。呼び出し側（`:86`、`:148`、`:152`）は変えない。
- `ProblemChecker.vue`は、`method`のsetterの中で`preferredRuntime(available.value)`を評価する。`:17`のprotocolの式（`runtime === 'cpp23-gcc' ? 'testlib' : 'legacy'`）は変えない。
- 次は変えない。
  - `useJudgeCatalog.ts`
  - 各コンポーネントの`useJudgeCatalog()`の分割代入の行（`SubmissionForm.vue:11`の`await`を含む）
  - `refreshCatalog()`の呼び出し
  - 「（現在利用できません）」の選択肢（`ProblemChecker.vue:64`、`ScoringEditor.vue:214`）
  - `SubmissionForm.vue`の`:10`、`:13-26`、`:97`
  - `layouts/default.vue`
  - `shared/types/problem-draft.ts`
  - `ProblemChecker.vue:24`の`supportsTestlib`
- D14（生成の言語に「（現在利用できません）」の選択肢がない）はこのままにする。テストでも、生成のselectの値は確かめない。
- コードのコメントは英語で、周りと同じ密度にする（関数ごとに1行程度）。計画とレポートは日本語で書く。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターがタスクごとに行う。
- `.output`とPlaywrightのポート（13000、13001、18080、18081）は、ほかのサイクルと共有している。ビルドとPlaywrightを始める前に、毎回次を実行する。
  `pgrep -af '[n]uxt build|[p]laywright test|[.]output/server/index[.]mjs'`
  何か出たら、ほかの検証が動いている。Monitorで終了を待ってから始め、ほかのプロセスは止めない。
- ビルドとPlaywrightは、Bashの`run_in_background`で実行して、終了の通知を待つ。

## Review Focus

1. カタログに言語が並んでいても、`maintenance: true`なら4つの画面のどれにも言語が並ばない。→ Task 1の`editors offer no runtime`と`submission form offers no runtime`（メンテナンス）、Task 2のnodeテスト。
2. 取得に失敗した（502）ときも、4つの画面のどれにも言語が並ばない。→ Task 1の同じテスト（502）、Task 2のnodeテスト（`error`あり）。
3. cpp23-gccが先頭以外にあるとき、検証コードと集計コードの既定はcpp23-gccで、判定コードの形式はtestlibになる。→ Task 1の`start in cpp23-gcc`、Task 2のnodeテスト。
4. cpp23-gccがないとき、既定は先頭（cpp17）で、形式はlegacyになる。`maintenance`を省いた応答でも言語が並ぶ。→ Task 1の`start in cpp17`、Task 2のnodeテスト。
5. 言語が1つもないときも、新しいコードの言語は`'cpp23-gcc'`になる。このとき「cpp23-gcc（現在利用できません）」だけが並び、形式はtestlibになる。→ Task 1の`editors offer no runtime`、Task 2のnodeテスト（`[]`）。

---

### Task 1: 4つの画面の言語の選択肢と既定の言語を特性テストで固定する

**Files:**
- Create: `web/tests/judge-catalog.spec.ts`

**Interfaces:**
- Consumes：`tests/fixtures/account.ts`の`test`と`expect`
- Produces：E2Eのテスト6件。本番コードは変えない。

- [ ] **Step 1: テストを書く**

計画者は、ほかのサイクルの検証中のため、このテストを実行していない。セレクタは既存のspecと同じものを使っている（`checker.spec.ts:4-7,65`、`problem-scoring.spec.ts:25-27,111`、`maintenance.spec.ts:58`、`generator.spec.ts:39,52`）。

```ts
import { expect, test } from './fixtures/account'

// Every screen that picks a judge runtime offers none while the judge cannot take work, and new judge code starts in
// C++23 (GCC) when it is offered. The catalog arrives after hydration, so each test waits until it shows.
const unavailable = [
  // The server lists no runtime in maintenance; listing some shows that the screens follow the flag themselves.
  { state: 'is in maintenance', status: 200, json: { items: [{ id: 'cpp23-gcc', label: 'C++23' }, { id: 'python314', label: 'Python' }], maintenance: true },
    banner: 'ジャッジ機能のメンテナンスを行っています。この期間中は提出等ができません。' },
  { state: 'cannot be reached', status: 502, json: {}, banner: 'ジャッジ機能の状態を確認できません。現在、提出等を利用できません。' },
]

for (const { state, status, json, banner } of unavailable) {
  test(`while the judge ${state}, the editors offer no runtime and new code names C++23 (GCC) as unavailable`, async ({ page }) => {
    await page.route('**/api/runtimes', route => route.fulfill({ status, json }))
    await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
    await expect(page.locator('.judge-banner')).toHaveText(banner)
    await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
    await expect(page.locator('.generator-options select option')).toHaveCount(0)
    await expect(page.getByRole('button', { name: '生成する', exact: true })).toBeDisabled()
    await page.getByRole('group', { name: '生成・検証・判定の種類' }).getByRole('button', { name: '判定方法', exact: true }).click()
    await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('special')
    await expect(page.getByLabel('検証コードの言語')).toHaveValue('cpp23-gcc')
    await expect(page.getByLabel('検証コードの言語').locator('option')).toHaveText(['cpp23-gcc（現在利用できません）'])
    await expect(page.getByLabel('判定コードの形式')).toHaveValue('testlib')
    await page.getByRole('button', { name: '採点', exact: true }).click()
    await page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' }).check()
    await page.getByRole('switch', { name: '集計コードで採点する' }).check()
    await expect(page.getByLabel('集計コードの言語')).toHaveValue('cpp23-gcc')
    await expect(page.getByLabel('集計コードの言語').locator('option')).toHaveText(['cpp23-gcc（現在利用できません）'])
  })

  test(`while the judge ${state}, the submission form offers no runtime`, async ({ page }) => {
    await page.route('**/api/runtimes', route => route.fulfill({ status, json }))
    await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/11111111-1111-4111-8111-111111111111')])
    await expect(page.locator('.judge-banner')).toHaveText(banner)
    await page.getByLabel('ソースコード', { exact: true }).fill('int main(){}')
    await expect(page.getByRole('combobox', { name: '言語' }).locator('option')).toHaveText(['-- 未選択 --'])
    await expect(page.getByText('現在、提出受付を停止しています。', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: '提出する', exact: true })).toBeDisabled()
  })
}

for (const { items, preferred, protocol } of [
  { items: [{ id: 'python314', label: 'Python' }, { id: 'cpp23-gcc', label: 'C++23' }], preferred: 'cpp23-gcc', protocol: 'testlib' },
  { items: [{ id: 'cpp17', label: 'C++17' }, { id: 'python314', label: 'Python' }], preferred: 'cpp17', protocol: 'legacy' },
]) {
  test(`new checker and aggregator code start in ${preferred} when the judge offers ${items.map(item => item.id).join(', ')}`, async ({ page }) => {
    const labels = items.map(item => item.label)
    await page.route('**/api/runtimes', route => route.fulfill({ json: { items } }))
    await Promise.all([page.waitForResponse('**/api/runtimes'), page.goto('/problems/new?fresh=1')])
    await page.getByRole('button', { name: '生成・検証・判定', exact: true }).click()
    // These labels differ from the server-rendered catalog, so they show that the offered runtimes arrived.
    await expect(page.locator('.generator-options select option')).toHaveText(labels)
    await expect(page.getByRole('button', { name: '生成する', exact: true })).toBeEnabled()
    await page.getByRole('group', { name: '生成・検証・判定の種類' }).getByRole('button', { name: '判定方法', exact: true }).click()
    await page.getByRole('combobox', { name: '判定方法', exact: true }).selectOption('special')
    await expect(page.getByLabel('検証コードの言語')).toHaveValue(preferred)
    await expect(page.getByLabel('検証コードの言語').locator('option')).toHaveText(labels)
    await expect(page.getByLabel('判定コードの形式')).toHaveValue(protocol)
    await page.getByRole('button', { name: '採点', exact: true }).click()
    await page.getByRole('group', { name: '採点方式' }).getByRole('radio', { name: '部分点採点' }).check()
    await page.getByRole('switch', { name: '集計コードで採点する' }).check()
    await expect(page.getByLabel('集計コードの言語')).toHaveValue(preferred)
    await expect(page.getByLabel('集計コードの言語').locator('option')).toHaveText(labels)
  })
}
```

- [ ] **Step 2: 変更前のコードで6件が成功することを確かめる**

1. `pgrep`を確かめる。
2. 検証コマンド2（build）を実行する。
3. 次を実行する。
   `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/judge-catalog.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c09-before`

Expected：`6 passed`。

失敗した場合は、まず原因を見分ける。
- 画面への行き方やセレクタ、待ち方が原因なら、テストの側だけを直す。
- 期待値が今の挙動と違う場合は、期待値も本番コードも変えない。その事実をレポートに書いて止まる。

コミットメッセージの案（オーケストレーター用）：`test: ジャッジを使えないときに4つの画面で言語を選べないことと、判定・集計コードの既定の言語を特性テストで固定する`

### Task 2: usableRuntimesとpreferredRuntimeを作り、4つのコンポーネントで使う

**Files:**
- Create: `web/app/utils/judge-catalog.ts`
- Modify: `web/tests/judge-catalog.spec.ts`（nodeテストを足す）
- Modify: `web/app/components/SubmissionForm.vue:12`、`TestCaseGenerator.vue:20`、`ProblemChecker.vue:11,16`、`ScoringEditor.vue:13,25`（それぞれのimportの並びに1行足す）

**Interfaces:**
- Consumes：Task 1のテスト（安全網）
- Produces（`web/app/utils/judge-catalog.ts`）：
  - `export function usableRuntimes<T>(catalog: { items?: T[], maintenance?: boolean } | null | undefined, error: unknown): T[]`
  - `export function preferredRuntime(items: { id: string }[]): string`

- [ ] **Step 1: nodeテストを足す**

`tests/judge-catalog.spec.ts`の先頭に`import { preferredRuntime, usableRuntimes } from '../app/utils/judge-catalog'`を足し、末尾に次の2件を足す（`problem-markdown.spec.ts`と同じ形）。

```ts
test('no runtime is usable while the judge is in maintenance or its catalog could not be fetched', () => {
  const items = [{ id: 'cpp17', label: 'C++17' }, { id: 'python314', label: 'Python' }]
  expect(usableRuntimes({ items, maintenance: false }, undefined)).toBe(items)
  expect(usableRuntimes({ items }, null)).toBe(items)
  expect(usableRuntimes({ items, maintenance: true }, undefined)).toEqual([])
  expect(usableRuntimes({ items, maintenance: false }, new Error('502'))).toEqual([])
  for (const catalog of [undefined, null, {}]) expect(usableRuntimes(catalog, undefined)).toEqual([])
})

test('new judge code prefers C++23 (GCC), then the first runtime, and names C++23 (GCC) when none is offered', () => {
  expect(preferredRuntime([{ id: 'python314' }, { id: 'cpp23-gcc' }])).toBe('cpp23-gcc')
  expect(preferredRuntime([{ id: 'cpp17' }, { id: 'python314' }])).toBe('cpp17')
  expect(preferredRuntime([])).toBe('cpp23-gcc')
  expect(preferredRuntime([{ id: '' }, { id: 'cpp17' }])).toBe('')
})
```

- [ ] **Step 2: 失敗することを確かめる**

1. `pgrep`を確かめる。
2. 次を実行する。
   `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/judge-catalog.spec.ts -g "usable|prefers" --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c09-unit`

Expected：`judge-catalog`のモジュールが見つからず、失敗する。

- [ ] **Step 3: `web/app/utils/judge-catalog.ts`に2つの関数を書く**

- シグネチャはInterfacesのとおりにする。本体は今の2つの式をそのまま移す（Global Constraintsの意味を守る）。
- 関数ごとに1行の英語のコメントを付ける。
  - `usableRuntimes`：メンテナンス中と状態を取得できないときは、どの言語も選ばせないこと
  - `preferredRuntime`：新しいコードはC++23 (GCC)を優先し、なければ先頭、何もなければC++23 (GCC)の名前のまま（画面では「現在利用できません」として出る）になること

- [ ] **Step 4: Step 2のコマンドを再実行する** → `2 passed`

- [ ] **Step 5: 4つのコンポーネントで使う**

各ファイルの既存のimportの並びの最後に、`~/utils/judge-catalog`からの明示的なimportを1行足す。
- `SubmissionForm.vue`、`TestCaseGenerator.vue`：`usableRuntimes`
- `ProblemChecker.vue`、`ScoringEditor.vue`：`preferredRuntime, usableRuntimes`

そのうえで、次の4種類を置き換える。
- 4ファイルの`available`：`const available = computed(() => usableRuntimes(catalog.value, catalogError.value))`
- `ProblemChecker.vue:16`：`const runtime = preferredRuntime(available.value)`。setterの中に置いたままにする。
- `ScoringEditor.vue:25`：`const defaultRuntime = () => preferredRuntime(available.value)`。直前のコメント（`:23-24`）は、`method`の切り替えについてのものなので、そのまま残す。

- [ ] **Step 6: 画面の置き換え漏れが見つかることを確かめる**

1. Editで、`ScoringEditor.vue`の`available`を一時的に`computed(() => catalog.value?.items ?? [])`に戻す。
2. `pgrep`を確かめてから、検証コマンド2（build）を実行する。
3. 次を実行する。
   `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/judge-catalog.spec.ts -g "editors offer no runtime" --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c09-mutant`
   - Expected：`is in maintenance`の1件が、`集計コードの言語`の選択肢（`['C++23', 'Python']`が並ぶ）で失敗する。
   - `cannot be reached`の1件は成功する見込みである。取得に失敗するとdataが消えるためで、そうなった場合もそのことをレポートに書く。
4. Editで元に戻す。`usableRuntimes(catalog.value, catalogError.value)`になっていることを確かめる。

- [ ] **Step 7: 検証コマンド1〜6を順に実行し、すべて期待どおりになることを確かめる**

コミットメッセージの案：`refactor: 利用できる言語の判定と既定の言語を、utils/judge-catalogのusableRuntimesとpreferredRuntimeにまとめる`

## 検証コマンド

ビルドとPlaywrightの前には、毎回`pgrep -af '[n]uxt build|[p]laywright test|[.]output/server/index[.]mjs'`を実行する。何か出たら、その終了を待つ。

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck` → エラーなし（`tests/*.spec.ts`は対象外。nodeテストの型は見ない）
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build` → 終了コード0。最後のコード変更のあとに必ずビルドし直す（Playwrightは`.output`を使う）
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/judge-catalog.spec.ts tests/maintenance.spec.ts tests/checker.spec.ts tests/generator.spec.ts tests/problem-scoring.spec.ts tests/submission.spec.ts tests/editor-settings.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c09` → `failed`なし。`judge-catalog.spec.ts`の8件が含まれる
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c09-all` → `failed`なし（CIの`npm test`と同じ全件）。言語の選択と関係のないspecが失敗した場合は、そのspecだけを2回再実行する。成功すれば、不安定なテストとしてレポートに書く
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short` → 新しく変わったのは、Global Constraintsの6ファイルだけである。ステージ済みの`api/`の3ファイルは開始時のままである
6. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`（AGENTS.mdの規則。`graphify-out/`はgitignore対象）

## 完了条件

- Task 1の6件が、変更前のコード（Task 1のStep 2）と変更後のコード（検証コマンド3）の両方で成功している。
- Task 2のnodeテスト2件が、関数がないときに失敗し、関数を書いたあとに成功している。
- Step 6の変異で、メンテナンスの1件が失敗した。変異は元に戻した。
- 検証コマンド1〜6が期待どおりになった。
- `catalog.value?.maintenance`と`find(item => item.id === 'cpp23-gcc')`が、`web/app/components/`に残っていない。

## 範囲外

- `useJudgeCatalog.ts`の戻り値の形（`available`を返すようにすることも含む）、`layouts/default.vue`のバナーの条件と文言
- `SubmissionForm.vue`の言語の初期選択（`ref('cpp23-gcc')`、localStorage、先頭へ戻すwatch）と`:97`の`catalogError ||`
- `shared/types/problem-draft.ts`の既定値、`emptyGenerators`、testlibに対応する言語の一覧
- `ProblemChecker.vue:24`の`supportsTestlib`
- D14（生成の言語に「（現在利用できません）」の選択肢がない）
- サーバー側（`api/`、`web/server/api/runtimes.get.ts`）

## レポートで触れる別件（直さない）

- testlibに対応する言語の一覧`['cpp23-gcc', 'cpp23-clang']`が、`ProblemChecker.vue:24`（UI）と`shared/types/problem-draft.ts:12`（保存時の検証）に重複している。片方だけを直すと、UIで選べる形式が保存で拒否される。別の候補にする価値がある。
- cpp23-clangはtestlibに対応しているが、判定コードの形式の初期値をtestlibにするのはcpp23-gccのときだけである（`ProblemChecker.vue:17`）。意図した仕様かどうかの判断が要る。
- `SubmissionForm.vue:97`の`catalogError ||`は冗長である（エラーのときは`available`が空になる）。
- Nuxt 4.6.1では、取得に失敗するとdataが消えるので、クライアント側の`error`の判定は、今は結果に表れない。
