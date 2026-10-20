# C17 コンテスト編集画面のフォームの値を1つにまとめ、読み込みの変換を純粋関数にする 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `ContestEditor.vue`の12個のフォームのrefを`reactive`な`fields`1つにまとめる。未保存の判定（snapshot）はこの1つから作る。下書きと公開済みのコンテストからの読み込みは、`web/app/utils/contest-form.ts`の型付きの純粋関数に移す。

**Architecture:** 先に、送信本文の全体、読み込み直後に未保存にならないこと、スイッチだけの変更で未保存になること、合言葉の取得に失敗したときの表示、Ctrl+Sを、ブラウザーの特性テストで固定する（Task 1）。次に、フォームの値の型`ContestForm`と読み込みの関数を`utils/contest-form.ts`に置く。コンポーネントは`Object.assign(fields, …)`で読み込む（Task 2）。送信本文の組み立て、保存前の検証、エラーの処理は、コンポーネントに残す。

**Tech Stack:** Nuxt 4、Vue 3（`<script setup>`）、TypeScript、Playwright 1.63（DBなしのフィクスチャーと、Go APIとDBを使うaccount設定）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C17.json`（worthとsafetyの`revised_scope`、`must_preserve`、`test_plan`）

## 困りごと

- フォームの値が12個のref（`ContestEditor.vue:17-52`）に分かれている。設定を1つ足すたびに、宣言、snapshot（`:138`）、下書きの読み込み（`:204-206`）、公開済みの読み込み（`:216-221`）、`applyParticipation`（`:227-231`）、2つの送信本文（`:359-374`、`:514`）を一緒に直す必要がある。25c2a91、b0da3a2、682d592は、どれもこれらを同時に書き換えている（計画者が確認した）。
- 1db91bfは`freezeOn`を足したが、snapshotに入れ忘れた。bd03e84で「スイッチの状態も未保存の変更の判定に含める」と直している。snapshotを手で並べているので、同じ漏れがまた起きうる。スイッチだけを切り替えたときに未保存になることは、今どのテストも確かめていない。
- 読み込みは個別の代入なので、項目を1つ書き忘れても型検査は通る。
- 問題の編集画面には前例がある。`useProblemDraft`が`reactive(initialProblemDraft())`を持ち、`utils/problem-draft.ts`の関数をnodeのテスト（`tests/problem-markdown.spec.ts`）で確かめている。

## 判定の記録

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| reactiveなフォームの名前 | `form` | `form`以外 | `fields`にする。`form`は`<form>`のテンプレートref（`:89`、`:559`の`ref="form"`）で、`reportInvalid`（`:350`）とCtrl+S（`:163`）が使う |
| 公開済みの読み込み | `formFromContest(c, passphrase)`で全体を1回で代入する | 合言葉の取得の前と後の2段階に分ける | 2段階にする。両者とも「読み込みの順序は変えない」で一致している。1回で代入すると、`/onsite`の取得に失敗したときにタイトルと日程が空のまま残る。`formFromContest(c)`は参加形式以外を返し、`participationForm(...)`は参加形式を返す。どちらの戻り値も型で全項目を要求するので、書き忘れはtypecheckで止まる |
| 送信本文の関数化（draftBody、contestBody、participationBody、contestEnd） | 任意で、効果は小さい | 手順3で移す | 移さない。`draftBody`の戻り値の型を`Required<ContestDraft>`にするだけにする。公開済みの本文を関数にすると引数が6つになる。変換規則は、既存のPlaywrightとTask 1の本文全体の`toEqual`で固定される |
| 段階の分け方 | 2段階 | 3段階（テスト、コンポーネント内だけ、純粋関数） | 2タスクにする（特性テスト、フォームと読み込み）。コンポーネント内だけの段階を挟むと、同じ代入を2回書き換えることになる |
| 特性テスト | `contest-freeze.spec.ts`に1本、nodeテスト | (a)〜(j)、nodeテスト | 新しいspecに6本置く。(a)(b)(c)は1本にまとめる。worthの1本は3つのスイッチ全部に広げる。下書き側の(f)(g)で1本、公開済み側の(e)(f)で1本、(i)で1本、(j)で1本。(d)は`contest-teams.spec.ts:1131`が確かめている（個人戦にすると`maxTeamSize`がnullになり、チーム戦に戻すと上限の欄が残る）。(h)はnodeテストで確かめる |
| nodeテストを書く時期 | 先に | 移した後 | Task 2の最初に書く。モジュールがまだないので、失敗を確かめてから作る |
| localDateとタイムゾーン | 引数で渡すか、往復で比べる | TZを固定するか、往復で比べる | ブラウザーのテストは`test.use({ timezoneId: 'Asia/Tokyo' })`で固定する。nodeテストは往復で比べる。`localDate`は`utils/contest-form.ts`に移し、exportしない（Nuxtの自動importに出さない） |
| `toRefs`で今の名前を残す案 | — | — | 採らない。12個の名前の一覧が残り、なくしたい手作業の同期がそのまま残る |
| C18との順序 | — | 並行して進めない | C18は不採用になった。ツールバーや表示切替の部品化はしない |

## Global Constraints

- 公開HTTP API（ルート、JSONの形、状態コード、エラー文言）、DBスキーマ、依存のバージョン、CI・インフラを変えない。
- 既存のテストは変更・削除・スキップしない。Task 1で足すテストは、Task 2で書き換えない。
- 下書きの本文は`{ version, draft }`にする。`draft`のキーは、`title`、`description`、`startsAt`、`durationMinutes`、`penaltyMinutes`、`freezeMinutes`、`teamMode`、`maxTeamSize`、`onsiteDivision`、`onsitePassphrase`、`problems`の11個に限る。値の規則は次のとおりで、どれも変えない。
  - `startsAt`：日付として不正ならnull、それ以外は`toISOString()`
  - `durationMinutes`と`penaltyMinutes`：`integer()`を通す。数値型の整数で、絶対値が1e9以下のときだけその値を送り、それ以外はnullにする
  - `freezeMinutes`：`freezeOn`がオンなら`integer() ?? 0`、オフならnull
  - `maxTeamSize`：チーム戦で`teamSizeOn`がオンなら`integer() ?? 0`、チーム戦でなければnull
  - `onsitePassphrase`：オンサイトがオンなら入力のまま、オフならnull
  - `problems`：`fullScoreOf(id) ?? integer(points)`
- 公開済みの本文は次の12キーにする。`title`、`description`、`startsAt`、`endsAt`（算出プロパティ`endsAt`のISO）、`penaltyMinutes`、`freezeMinutes`、`teamMode`、`maxTeamSize`、`onsiteDivision`、`onsitePassphrase`、`version`、`problems`。値の規則は次のとおり。
  - `penaltyMinutes`は生の値を送る
  - `freezeMinutes`は、`freezeOn`がオンなら生の値、オフならnull
  - `maxTeamSize`は、`teamSizeOn`がオンなら`Number()`を通した値
  - `problems`は`fullScoreOf(id) ?? points`
- フォームだけの項目（`freezeOn`、`teamSizeOn`、`onsiteOn`、`passphrase`）は、どちらの本文にも入れない。`version`はフォームにもsnapshotにも入れない。
- 新規の既定値は今のrefの初期値と同じにする。`title` ''、`description` `'<!-- ここにコンテストの概要を記載 -->\n'`、`startsAt` ''、時間120、ペナルティ5、`freezeOn` false、`freezeMinutes` ''、`teamMode` 'individual'、`teamSizeOn` false、`maxTeamSize` ''、`onsiteOn` false、`passphrase` ''。
- 読み込みの規則は`:204-206`、`:216-218`、`:227-231`の式のまま変えない。参加形式の4項目は、チーム戦かどうか、オンサイトかどうかに関係なく読み込む。
- 読み込みの手順を変えない。
  - 下書き：`version`、`draftIssues`、`applySetters`の後でフォームに代入する。`selected`はその後にする。
  - 公開済み：`published=true`、ロックの判定、`applySetters`、参加形式以外の代入と`version`、合言葉の取得（`canEdit && onsiteDivision`のときだけ）、参加形式の代入、`contestPartial`、`selected`、`settersOnly`の順にする。
- watchは2つだけにする。監視するのは`() => fields.freezeOn`と`() => fields.teamSizeOn`のゲッターで、既定のflushのまま、`deep`は付けない。`teamMode`とフォーム全体は監視しない。中身は今と同じにする。
  - 凍結：オフなら''にする。オンで値が''なら、`Math.min(60, Number(durationMinutes) || 60)`を入れる。
  - 人数の上限：オフなら''にする。オンで値が''なら3を入れる。
- `setOnsite`の規則（オフで''、オンで空のときだけ8文字を作る）と、`InlineSwitch`の`:model-value`と`@update:model-value`の形を保つ。
- `const form = ref<HTMLFormElement>()`、テンプレートの`ref="form"`、`reportInvalid`、`saveWithShortcut`は変えない。Ctrl+Sの判定はD13として残す。
- snapshotは`fields`の全項目と`selected`から作る。`saveDraft`がPUTの前にsnapshotを取る点、`dirty`、`status`、離脱の確認、`beforeunload`は変えない。
- `save()`の検証の順序と文言、エラーコードごとのセクションの移動、`failure`、`publish`、`removeDraft`、setterの処理、`Date.now()`の判定の場所は変えない。
- テンプレートで変えるのは、フォームの値を読む23か所の束縛だけにする（Task 2のStep 4）。構造、ツールバー、表示切替には触れない（C18は不採用）。ThemeSelectは足さない（D15）。
- コードのコメントは英語で、周りと同じ密度にする。計画とレポートは日本語で書く。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターがタスクごとに行う。
- C04の変更（`useProblemDraft.ts`、`problem-draft.ts`、`editor.spec.ts`、`manual-problem-save.spec.ts`、`problem-save-behavior.spec.ts`）がまだ作業ツリーにあれば、触れない。
- `.output`とPlaywrightのポートは、ほかのサイクルと共有している。ビルドとPlaywrightを始める前に、毎回検証コマンド0を実行する。何か出たら、Monitorで終わるのを待つ。ほかのプロセスは止めない。10分を超えうるコマンドは、`run_in_background`で実行する。

## Review Focus

1. 名前の衝突。reactiveなフォームを`form`と名付けると、`ref="form"`が外れる。それでも型検査は通る。その結果、`reportInvalid`は常にtrueを返し、Ctrl+Sは何もしなくなる。→ T6と、既存の`contest-freeze.spec.ts:379`（`rangeOverflow`で送らない）が検出する。
2. 公開済みの読み込みを1回の代入にまとめる。そうすると、`/onsite`の取得に失敗したときにタイトルと時間が空のまま残る。→ T5と、Task 2のStep 6の変異で確かめる。
3. watchの書き換え。`watch(fields, …, { deep: true })`、`flush: 'sync'`、`teamMode`の監視にすると、読み込んだ0が既定値で上書きされたり、上限がリセットされたりする。→ T3（0のまま開き、未保存にならない）、`contest-teams.spec.ts:1131`、nodeテスト（`participationForm(false, 4, …)`）で確かめる。
4. 本文のキーの過不足。フォームだけの項目が混じると、Goの`DisallowUnknownFields`で400になる。キーが抜けると、APIは保存済みの値を黙って残す。→ T1とT4の`toEqual`、account設定の一式で確かめる。
5. 既定値の共有。定数のオブジェクトを`reactive()`に渡すと、プロキシが定数を書き換え、次に開いた新規作成の画面に前の値が残る。→ nodeテストの`not.toBe`で確かめる。

---

### Task 1: コンテスト編集画面のフォームの特性テストを追加する

**Files:**
- Create: `web/tests/contest-editor-form.spec.ts`

**Interfaces:**
- Consumes：`./fixtures/account`の`test`（ログイン済みのui_test）、`../shared/types/contest`の`Contest`と`ContestDraft`、`../shared/utils/short-id`の`shortId`
- Produces：T1〜T6。製品のコードは変えない。

計画者はC04の検証中のためPlaywrightを実行していない。T1〜T6は、変更前のコードで成功することをStep 3で必ず確かめる。失敗した場合は、コードではなくテストを実際の挙動に合わせて直し、レポートに記録する。

- [ ] **Step 1: 共通の準備を書く**

```ts
import type { Page } from '@playwright/test'
import { expect, test } from './fixtures/account'
import type { Contest, ContestDraft } from '../shared/types/contest'
import { shortId } from '../shared/utils/short-id'

// The date input reads local time, so a fixed zone keeps the instants in the bodies literal.
test.use({ timezoneId: 'Asia/Tokyo' })

const updatedAt = '2026-10-01T00:00:00Z'
const draftId = 'eeeeeeee-0000-4000-8000-000000000001'
const contestId = 'eeeeeeee-0000-4000-8000-000000000002'
const problem = { id: '11111111-aaaa-4aaa-8aaa-0000000000e1', title: 'フォームの問題', updatedAt, readiness: { publish: [], contest: [], featured: [] } }
const draftResult = (id: string, version: number, draft: ContestDraft) => ({ id, version, updatedAt, issues: ['start_missing'], authors: ['ui_test'], owner: true, problems: [], draft })
// Serves the setter's problem and one draft (404 while there is none), and records each saved body.
async function routeDraft(page: Page, draft?: ContestDraft) {
  const bodies: { version: number, draft: ContestDraft }[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: new URL(route.request().url()).searchParams.get('role') === 'tester' ? [] : [problem], nextCursor: '' } }))
  await page.route('**/api/my/contests/*/draft', (route) => {
    const request = route.request()
    if (request.method() !== 'PUT') return draft ? route.fulfill({ json: draftResult(draftId, 1, draft) }) : route.fulfill({ status: 404, json: {} })
    const body = request.postDataJSON()
    bodies.push(body)
    return route.fulfill({ json: draftResult(new URL(request.url()).pathname.split('/')[4]!, body.version + 1, body.draft) })
  })
  return bodies
}
const contest: Contest = {
  id: contestId, title: '公開済みのフォーム', author: 'ui_test', authors: ['ui_test'], problemAuthors: [], testers: [], description: '概要',
  startsAt: '2099-10-01T00:00:00Z', endsAt: '2099-10-01T02:00:00Z', penaltyMinutes: 5, version: 3, status: 'scheduled',
  canEdit: true, participating: false, official: false, staff: true, setter: true, owner: true, canViewSubmissions: true,
  freezeMinutes: 30, freezeAt: '2099-10-01T01:30:00Z', revealed: false, revealedAt: null, resultsFinal: false,
  teamMode: true, maxTeamSize: 4, onsiteDivision: true,
  problems: [{ id: problem.id, title: problem.title, points: 100 }],
}
// Serves the published contest with the given answer from the setters' venue route, and records each saved body.
async function routeContest(page: Page, venue: { status?: number, json: unknown }) {
  const bodies: Record<string, unknown>[] = []
  await page.route('**/api/my/problems**', route => route.fulfill({ json: { items: [], nextCursor: '' } }))
  await page.route(`**/api/my/contests/${contestId}/draft`, route => route.fulfill({ status: 404, json: {} }))
  await page.route(`**/api/my/contests/${contestId}/onsite`, route => route.fulfill(venue))
  await page.route(`**/api/my/contests/${contestId}`, (route) => {
    if (route.request().method() !== 'PUT') return route.fulfill({ json: contest })
    bodies.push(route.request().postDataJSON())
    return route.fulfill({ json: { ...contest, version: contest.version + 1 } })
  })
  return bodies
}
```

- [ ] **Step 2: 6本のテストを書く**

ラベルは既存のspecと同じにする（`コンテストタイトル`、`開始日時`、`コンテスト時間（分）`、`誤答ペナルティ（分）`、`凍結する時間（終了前の分）`、`1チームの人数の上限（人）`、`会場の合言葉`、スイッチ`順位表の凍結を行う`・`人数の上限を設ける`・`オンサイト区分を設ける`、radio`チーム戦`、配点は`#points-${problem.id}`）。セクションは`getByRole('button', { name, exact: true })`で開く。未保存かどうかは`page.locator('.draft-status')`で見る。保存が終わったことは、`.draft-status`に`公開まであと`が出ることで確かめる。

1. `a new draft sends all eleven fields, each a whole number or null`（(a)(b)(c)）
   - 新規作成の画面で、次のとおり入れて`下書きを保存`する。
     - タイトル：`フォームの往復`
     - 問題：`フォームの問題を追加`、配点`250`
     - 開始日時：`2099-10-01T09:00`、時間：`90`、ペナルティ：`0`
     - 凍結：オンにして`15`
     - 参加形式：チーム戦、上限をオンにして`4`、オンサイトをオンにして`Venue_2026`
   - `bodies[0]`を`{ version: 0, draft: first }`と`toEqual`で比べる。`first`は次のとおり。
     `{ title: 'フォームの往復', description: '<!-- ここにコンテストの概要を記載 -->\n', startsAt: '2099-10-01T00:00:00.000Z', durationMinutes: 90, penaltyMinutes: 0, freezeMinutes: 15, teamMode: true, maxTeamSize: 4, onsiteDivision: true, onsitePassphrase: 'Venue_2026', problems: [{ id: problem.id, points: 250 }] }`
   - 続けて次のとおり変えて保存する。開始日時`''`、時間`''`、ペナルティ`1.5`、配点`''`。
   - `bodies[1]`を`{ version: 1, draft: { ...first, startsAt: null, durationMinutes: null, penaltyMinutes: null, problems: [{ id: problem.id, points: null }] } }`と比べる。
2. `turning a switch off is an unsaved change even when its value was already empty`（bd03e84）
   - 3つのスイッチそれぞれについて、オンにし、値を`''`にして保存し、保存が終わったら未保存でないことを確かめる。そのあとスイッチだけをオフにし、`.draft-status`に`未保存の変更があります`が出ることを確かめる。
   - 順番は、凍結、人数の上限（先にチーム戦を選ぶ）、オンサイト（自動で入った合言葉を`''`にする）とする。
   - 最後に`bodies.map(b => [b.draft.freezeMinutes, b.draft.maxTeamSize, b.draft.onsitePassphrase])`を`[[0, null, null], [null, 0, null], [null, null, '']]`と比べる。
3. `a draft with every setting on, at 0 where it takes a number, opens unchanged`（(f)(g)）
   - 次の下書きを`routeDraft`に渡し、`/my/contests/${shortId(draftId)}`を開く。
     `{ title: '設定をすべて使う下書き', description: '', startsAt: '2099-10-01T00:00:00Z', durationMinutes: 90, penaltyMinutes: 0, freezeMinutes: 0, teamMode: true, maxTeamSize: 0, onsiteDivision: true, onsitePassphrase: 'Venue_2026', problems: [] }`
   - `下書きを保存`が有効になったら、`.draft-status`に`未保存の変更があります`が出ていないことを確かめる。
   - 欄の値を確かめる。開始日時`2099-10-01T09:00`、時間`90`、ペナルティ`0`、凍結はオンで`0`、チーム戦が選ばれていること、上限はオンで`0`、オンサイトはオンで`Venue_2026`。
   - `page.on('dialog', …)`でダイアログの文言を記録して閉じる。`自分のコンテスト`のリンクを押すと`/my?tab=contests`へ移り、ダイアログは出ないこと。
4. `a published contest opens unchanged and saves every setting as loaded`（(e)(f)）
   - `routeContest(page, { json: { onsiteDivision: true, passphrase: 'Venue_2026' } })`とする。
   - 開いて`変更を保存`が有効になったら、`自分のコンテスト`を押す。`/my?tab=contests`へ移り、ダイアログが出ないこと。
   - もう一度開いて、タイトルを`公開済みのフォーム（変更）`にし、`自分のコンテスト`を押す。ダイアログの文言が`['未保存の変更を破棄して移動しますか？']`で、URLが編集画面のままであること。
   - タイトルを`公開済みのフォーム`に戻して`変更を保存`する。`bodies[0]`を次と`toEqual`で比べる。
     `{ title: '公開済みのフォーム', description: '概要', startsAt: '2099-10-01T00:00:00.000Z', endsAt: '2099-10-01T02:00:00.000Z', penaltyMinutes: 5, freezeMinutes: 30, teamMode: true, maxTeamSize: 4, onsiteDivision: true, onsitePassphrase: 'Venue_2026', version: 3, problems: [{ id: problem.id, points: 100 }] }`
5. `a published contest whose passphrase cannot be read keeps its title and length beside the error`（(i)）
   - `routeContest(page, { status: 500, json: {} })`とする。
   - `getByRole('alert')`が`作成情報を読み込めませんでした。ログイン状態を確認して再読み込みしてください。`であること。
   - タイトル欄が`公開済みのフォーム`であること。`期間・ペナルティ`を開くと、時間が`120`であること。
6. `Ctrl+S saves the contest draft`（(j)）
   - 新規作成の画面で、タイトルに`ショートカットで保存`を入れ、`page.keyboard.press('Control+s')`を押す。
   - `bodies`が1件になり、保存が終わった後も1件のままで、`bodies[0].draft.title`が`ショートカットで保存`であること。

- [ ] **Step 3: 変更前のコードで成功することを確かめる**

実行：検証コマンド0、2、3。期待：`6 passed`。

コミットメッセージの案：`test: コンテスト編集画面の送信本文、読み込み、未保存の判定を特性テストで固定する`

### Task 2: フォームの値を1つにまとめ、読み込みをcontest-formの純粋関数にする

**Files:**
- Create: `web/app/utils/contest-form.ts`
- Create: `web/tests/contest-form.spec.ts`
- Modify: `web/app/components/ContestEditor.vue`（`:17-52`、`:110-120`、`:138`、`:168-171`、`:204-206`、`:216-221`、`:227-231`、`:359-374`、`:500-514`、テンプレートの23か所）

**Interfaces:**
- Consumes：Task 1のT1〜T6（安全網）
- Produces（`web/app/utils/contest-form.ts`。型は`import type { Contest, ContestDraft } from '~~/shared/types/contest'`）：
  - `export type ParticipationForm = { teamMode: 'individual' | 'team', teamSizeOn: boolean, maxTeamSize: number | string, onsiteOn: boolean, passphrase: string }`
  - `export type ContestForm = { title: string, description: string, startsAt: string, durationMinutes: number | string, penaltyMinutes: number | string, freezeOn: boolean, freezeMinutes: number | string } & ParticipationForm`
  - `export function initialContestForm(): ContestForm`：呼ぶたびに新しいオブジェクトを返す。キーの順番は上の型と同じにする。
  - `export function participationForm(team?: boolean, size?: number | null, onsite?: boolean, venue?: string | null): ParticipationForm`：`:227-231`の規則
  - `export function formFromDraft(d: ContestDraft): ContestForm`：`:204-205`の式と`participationForm(d.teamMode, d.maxTeamSize, d.onsiteDivision, d.onsitePassphrase)`
  - `export function formFromContest(c: Contest): Omit<ContestForm, keyof ParticipationForm>`：`:216-218`の式
  - `function localDate(value: string): string`：`:168-171`をそのまま移す。exportしない。

- [ ] **Step 1: nodeテスト`web/tests/contest-form.spec.ts`を書く**

`math-text.spec.ts`と同じく、`@playwright/test`の`test`を使い、ブラウザーは使わない。

```ts
import { expect, test } from '@playwright/test'
import type { Contest, ContestDraft } from '../shared/types/contest'
import { formFromContest, formFromDraft, initialContestForm, participationForm } from '../app/utils/contest-form'

const draft = (fields: Partial<ContestDraft>): ContestDraft => ({ title: '下書き', description: '', startsAt: null, durationMinutes: null, penaltyMinutes: null, problems: [], ...fields })
const noParticipation = { teamMode: 'individual', teamSizeOn: false, maxTeamSize: '', onsiteOn: false, passphrase: '' }

test('a new contest form starts from the editor defaults, a fresh object each time', () => {
  expect(initialContestForm()).toEqual({ title: '', description: '<!-- ここにコンテストの概要を記載 -->\n', startsAt: '', durationMinutes: 120, penaltyMinutes: 5, freezeOn: false, freezeMinutes: '', ...noParticipation })
  expect(initialContestForm()).not.toBe(initialContestForm())
})

test('a draft fills empty inputs for null and turns a switch on for any value, 0 included', () => {
  expect(formFromDraft(draft({}))).toEqual({ title: '下書き', description: '', startsAt: '', durationMinutes: '', penaltyMinutes: '', freezeOn: false, freezeMinutes: '', ...noParticipation })
  expect(formFromDraft(draft({ durationMinutes: 90, penaltyMinutes: 0, freezeMinutes: 0, teamMode: true, maxTeamSize: 0, onsiteDivision: true, onsitePassphrase: 'Venue_2026' })))
    .toEqual({ title: '下書き', description: '', startsAt: '', durationMinutes: 90, penaltyMinutes: 0, freezeOn: true, freezeMinutes: 0, teamMode: 'team', teamSizeOn: true, maxTeamSize: 0, onsiteOn: true, passphrase: 'Venue_2026' })
})

test('a draft start reads as local time for the date input', () => {
  const { startsAt } = formFromDraft(draft({ startsAt: '2099-10-01T03:04:00Z' }))
  expect(startsAt).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/)
  expect(new Date(startsAt).toISOString()).toBe('2099-10-01T03:04:00.000Z')
})

test('participation settings read apart from the team mode and the onsite division', () => {
  expect(participationForm(false, 4, false, 'keep_me')).toEqual({ teamMode: 'individual', teamSizeOn: true, maxTeamSize: 4, onsiteOn: false, passphrase: 'keep_me' })
  expect(participationForm(true, null, true, undefined)).toEqual({ teamMode: 'team', teamSizeOn: false, maxTeamSize: '', onsiteOn: true, passphrase: '' })
  expect(participationForm()).toEqual(noParticipation)
})

test('a published contest reads its length in whole minutes and leaves the participation to the venue read', () => {
  const contest = { title: '公開済み', description: '概要', startsAt: '2099-10-01T00:00:00Z', endsAt: '2099-10-01T01:30:29Z', penaltyMinutes: 0, freezeMinutes: null } as Contest
  const form = formFromContest(contest)
  expect(form).toEqual({ title: '公開済み', description: '概要', startsAt: expect.any(String), durationMinutes: 90, penaltyMinutes: 0, freezeOn: false, freezeMinutes: '' })
  expect(new Date(form.startsAt).toISOString()).toBe('2099-10-01T00:00:00.000Z')
  expect(formFromContest({ ...contest, endsAt: '2099-10-01T01:30:30Z', freezeMinutes: 30 })).toMatchObject({ durationMinutes: 91, freezeOn: true, freezeMinutes: 30 })
})
```

- [ ] **Step 2: 失敗を確かめる**

実行：検証コマンド0、4。期待：`../app/utils/contest-form`が見つからずに失敗する。

- [ ] **Step 3: `web/app/utils/contest-form.ts`を作る**

Interfacesのとおりに作る。本体は、元の式をそのまま写す。

元のコメントは、型の上に英語で1〜2行ずつ移す。対象は次の3つ。
- `:20`：Number inputs hold '' while empty; drafts save that as null.
- `:23`：凍結の時間は、スイッチがオンの間だけ使う。
- `:42`：合言葉は、下書きでは入力のまま返り、公開済みでは作成者用のvenueのルートから読む。

`formFromContest`には次の1行を付ける。
`// Everything but the participation, which waits for the venue route; assigned first, so a failed read leaves it shown.`

- [ ] **Step 4: `ContestEditor.vue`を書き換える**

1. `import { formFromContest, formFromDraft, initialContestForm, participationForm } from '~/utils/contest-form'`を足す。`ContestDraft`の型のimportは残す。
2. `:17-43`の12個のrefを`const fields = reactive(initialContestForm())`の1行にする。直前に、`form`は`<form>`要素のrefだという英語の1行コメントを付ける。
3. watchの2つは、`watch(() => fields.freezeOn, …)`と`watch(() => fields.teamSizeOn, …)`にする。中身の`.value`は`fields.x`に置き換え、元のコメントも残す。`teamModes`、`passphraseAlphabet`、`maxTeamMembers`は残す。
4. `setOnsite`は`fields.onsiteOn`と`fields.passphrase`を書き換える。コメントの「sets both refs directly」は「sets both fields directly」にする。
5. `endsAt`（`:110-113`）と`endLabel`（`:118-120`）は`fields.startsAt`と`fields.durationMinutes`を読む。
6. snapshot（`:138`）は`const snapshot = () => JSON.stringify([fields, selected.value])`にする。
7. `localDate`（`:168-171`）を消す。
8. 下書きの読み込み（`:204-206`）は`Object.assign(fields, formFromDraft(d))`の1行にする。前の`version`、`draftIssues`、`applySetters`と、後の`selected`の行はそのまま残す。
9. 公開済みの読み込みは次のようにする。
   - `:216-218`を`Object.assign(fields, formFromContest(c)); version.value = c.version`にする。
   - venueの取得の行はそのまま残す。
   - `:221`を`Object.assign(fields, participationForm(c.teamMode, c.maxTeamSize, c.onsiteDivision, venue?.passphrase))`にする。
10. `applyParticipation`（`:227-231`）を消す。
11. `draftBody`の戻り値の型を`Required<ContestDraft>`にする。`draftBody`、`participationBody`、`save()`（`:500`、`:504`、`:509`、`:514`）の`x.value`は、`fields.x`に置き換える。式の形と順番は変えない。
12. テンプレートの23か所を書き換える。`:562`、`:575`、`:633`（2か所）、`:636`、`:637`（4か所）、`:647`、`:649`、`:650`、`:652`（2か所）、`:655`、`:657`、`:658`（2か所）の束縛と、`:565`、`:581`、`:582`（2か所）、`:666`の式で、名前を`fields.x`にする。`p.title`やHTMLの`title="…"`属性は変えない。

- [ ] **Step 5: 検証する**

1. 検証コマンド0と1を実行する。typecheckのエラーがないこと。テンプレートに名前を置き換え忘れた箇所があれば、ここでエラーになる。
2. 検証コマンド2と4を実行する。`5 passed`になること。
3. `grep -n -E 'ref<number \| string>|applyParticipation|function localDate|toRefs' /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app/components/ContestEditor.vue`を実行する。出力がないこと。

- [ ] **Step 6: 読み込みの順序の変異が検出されることを確かめる**

1. Editで、公開済みの`Object.assign(fields, formFromContest(c))`を、venueの取得の行の後ろへ一時的に移す。
2. 検証コマンド0と2を実行し、`npx playwright test tests/contest-editor-form.spec.ts -g "cannot be read"`を、検証コマンド3と同じ形で実行する。FAILすること（タイトル欄が空）。
3. Editで元に戻し、もう一度検証コマンド2を実行する。

- [ ] **Step 7: 全体を検証し、差分を確かめ、グラフを更新する**

1. 検証コマンド0、2、5、6、7を実行する。
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git diff -- web/app/components/ContestEditor.vue`で、差分がStep 4の内容だけであることを確かめる。CSS、ほかの関数、テンプレートの構造には差分がないこと。
3. 検証コマンド8を実行する。

コミットメッセージの案：`refactor: コンテスト編集画面のフォームの値を1つにまとめ、読み込みの変換をcontest-formの純粋関数にする`

## 検証コマンド

`SP`は`/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad`を表す。実際のコマンドでは、展開した絶対パスを書く。

0. `pgrep -af '[n]uxt build|[p]laywright test|[.]output/server/index[.]mjs'` → 出力なし
1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck` → エラーなし
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build` → 終了コード0
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/contest-editor-form.spec.ts --output=SP/pw-c17` → すべてpassed
4. 3と同じ形で、`tests/contest-form.spec.ts`を実行する
5. 3と同じ形で、次のファイルをまとめて実行する。期待はすべてpassed。
   - `tests/contest-editor-form.spec.ts`、`tests/contest-form.spec.ts`
   - `tests/contest-freeze.spec.ts`、`tests/contest-teams.spec.ts`、`tests/partial-scores.spec.ts`、`tests/contest-setters.spec.ts`、`tests/contest-problem-picker.spec.ts`、`tests/creation-quota.spec.ts`
   - `tests/details.spec.ts`、`tests/scroll-sync.spec.ts`、`tests/markdown-editor.spec.ts`
6. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test --output=SP/pw-c17-all` → すべてpassed（ブラウザーの全件。C04では約5分）
7. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 2400 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test --config playwright.account.config.ts --output=SP/pw-c17-account` → すべてpassed。`contests.spec.ts`、`partial-scoring.spec.ts`、`images.spec.ts`が、実際のGo API（`DisallowUnknownFields`）を通して編集画面から保存する。15432番ポートは使わない。
8. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`

`npm run typecheck`は`web/tests`の型を見ない（C04のレポート）。テストの型の誤りは、Playwrightを実行したときに初めて分かる。

## 完了条件

- Task 1のT1〜T6が、変更前のコードで成功している。
- Task 2の後、検証コマンド1、2、5、6、7がすべて成功している。Step 6の変異は検出され、元に戻してある。
- 変わったファイルは次の4つだけ。
  - Task 1：`web/tests/contest-editor-form.spec.ts`
  - Task 2：`web/app/utils/contest-form.ts`、`web/tests/contest-form.spec.ts`、`web/app/components/ContestEditor.vue`
- `ContestEditor.vue`には、フォームの値の個別のref、`applyParticipation`、`localDate`がない。watchは2つで、ゲッターを監視している。

## 範囲外

- 送信本文の組み立てを`utils`へ移すこと（`draftBody`、公開済みの本文、`participationBody`、`contestEnd`）。2つの本文の送り方（`integer`、生の値、`Number()`）を揃えること。
- スイッチと値の組を一般化した部品。
- `selected`、`EditorProblem`、`fullScoreOf`、`contestPartial`、`shared`、`titles`、問題の選択。
- `save()`の検証、エラー文言、セクションの移動、`failure`、`publish`、`removeDraft`、setterの処理、`locked`と`settersOnly`。
- Markdownエディタの外枠、ツールバー、表示切替の部品化（C18は不採用）。
- Ctrl+Sの判定（D13）、ThemeSelect（D15）。

## レポートで触れる別件（直さない）

- D13とD15は、そのまま残した。
- `/onsite`の取得に失敗すると、参加形式の欄は既定値（個人戦、オフ）のまま、ほかの欄はコンテストの値になる。`ready`はfalseで、欄はすべて無効になっている。今の挙動のまま残した。
