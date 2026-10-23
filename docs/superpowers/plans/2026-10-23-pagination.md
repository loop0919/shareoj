# 一覧のページ番号と表示件数、順位表の自分の行 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 問題一覧、コンテストの順位表、コンテストの提出一覧を、AtCoder式のページ番号と表示件数の選択で送れるようにし、順位表では自分の行を強調してページの外でも表に出す。

**Architecture:** APIは問題一覧とコンテストの提出一覧に `limit` と `total`（問題一覧は `offset` も）を足すだけにする。順位表は今のまま全件を取り、画面で分ける。ページ番号はURLの `page`、表示件数は一覧ごとのCookieに置き、3つの一覧で同じ部品（`ListToolbar`、`PageNav`）を使う。

**Tech Stack:** Go（pgx、`api/internal/problems`、`api/internal/submissions`、`api/internal/httpapi`）、PostgreSQL、Nuxt 4（Vue 3、`useCookie`、`useFetch`）、Playwright

**Spec:** [docs/adr/0028-paginate-lists-by-page-number.md](../../adr/0028-paginate-lists-by-page-number.md)

## Global Constraints

- 表示件数は 20、50、100 の3つ。初期値は問題一覧50、提出一覧50、順位表20。
- 表示件数のCookieは `openoj-page-size-problems`、`openoj-page-size-standings`、`openoj-page-size-submissions`。ほかの表示設定（`useProblemTags`）と同じく `maxAge: 60 * 60 * 24 * 365, sameSite: 'lax', path: '/'`。値が 20、50、100 以外なら初期値として扱う。
- ページ番号の並びは、1、最後のページ、いまのページ p から ±1, ±2, ±3, ±7, ±15, ±31, …（3のあとは前の幅×2+1）のうち 1〜最後に入るもの。小さい順、重複なし。
- 「前へ／次へ」と「…」は置かない。最後のページが1ならページ番号を出さない。いまのページは `aria-current="page"` の `span`、ほかは `NuxtLink`。
- 文言：表の上の件数は問題一覧が「全 {N} 問中 {a}〜{b} 問目」、順位表と提出一覧が「全 {N} 件中 {a}〜{b} 件目」。数は `toLocaleString('en-US')`（例：1,000）。選択の見出しは「表示件数」。ページ番号の `nav` の `aria-label` は「ページ」。
- URLの `page`：1ページ目では付けない。整数でない値や1未満は1ページ目、最後より大きい値は最後のページ。
- APIの `limit`：1以上100以下の整数、省略時50、それ以外は `400 invalid_request`。`total` は問題一覧とコンテストの提出一覧の応答だけに足す。
- 自分の行の強調：背景 `var(--color-accent-soft)`、名前の欄の左端に `box-shadow: inset 4px 0 0 var(--color-accent)`。見えるラベルは付けず、名前の欄に `<span class="visually-hidden">（自分）</span>` を入れる。区切りの行の文字は「⋮」だけ。
- コマンドはリポジトリのルートで `nix develop` の中から実行する。Goの統合テストは `make db-up` の後に `TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable'` を付ける。Playwrightは `web` で `npm run build` の後に `npx playwright test <file> --output=<scratchpad>/pw`（`web/test-results` はroot所有）。
- コミットメッセージは `prefix: 対応内容`（日本語）。コードを変えたら `graphify update .` を実行する。

## Review Focus

- **URLの `page` が範囲外**：`page=999` は最後のページ、`page=abc` と `page=0` は1ページ目を表示する。問題一覧はサーバー側で最後のページのURLへ置き換える（Task 3 と Task 6 のテスト）。
- **同じ順位の行がページの境目にまたがる**：自分の行を上に出すか下に出すかは順位の数ではなく全体の並びでの位置で決める（Task 4 のテスト「境目の同順位」）。
- **3ページ目で絞り込みを切り替える**：「オンサイトのみ表示」「凍結なしで表示」を切り替えたら `page` がURLから外れる（Task 3 と Task 5 のテスト）。
- **公開APIの今までの使い方**：`GET /problems` を引数なしや `cursor` だけで呼ぶと、今と同じ `items` と `nextCursor` に `total` が増えるだけ。`cursor` と `offset` の同時指定、`limit=0`、`limit=101` は400（Task 2 のテスト）。
- **Cookieの値が壊れている**：`openoj-page-size-standings=7` でも順位表は20件ずつで表示する（Task 3 のテスト）。

---

### Task 1: コンテストの提出一覧に `limit` と `total` を足す

**Files:**
- Modify: `api/internal/submissions/contest.go`（`ContestList`、`submissionList`、新しい型）
- Modify: `api/internal/submissions/problem.go:62-69`（`submissionList` の呼び出し）
- Modify: `api/internal/httpapi/contests.go:242-261`（`contestSubmissions`）
- Create: `api/internal/httpapi/pagination.go`（`pageLimit`、`limitParameter`）
- Modify: `api/internal/httpapi/docs.go`（`onsiteSubmissionParameters` の使い分け、`publicContestSubmissionPage`）
- Modify: `api/internal/httpapi/routes.go:150`（`/contests/{id}/submissions` の Response と Parameters）
- Test: `api/internal/httpapi/contest_submission_pages_test.go`、`api/internal/httpapi/docs_test.go`

**Interfaces:**
- Produces: `type CountedSubmissionList struct { ContestSubmissionList; Total int \`json:"total"\` }`（JSONは `items`、`hasMore`、`total` が平らに並ぶ）。
- Produces: `func (s *Store) ContestList(ctx context.Context, contestID, viewer string, offset, limit int, onsite bool) (CountedSubmissionList, error)`。
- Produces: `func submissionList(rows pgx.Rows, limit int) (ContestSubmissionList, error)`（`limit+1` 行まで受け取り、`HasMore` を決めて `limit` 件に切る）。`ProblemList` は50を渡し、応答は変えない。
- Produces（httpapi）：`func pageLimit(w http.ResponseWriter, r *http.Request) (int, bool)`、`func limitParameter() *huma.Param`。

- [ ] **Step 1: テストを書く**

`TestContestSubmissionPagesPostgres`。`newTeamTest(t, "alice", "bob", "carol")`、`f.problem(pid)`、`f.create(cid, pid, nil)`、bobとcarolを `f.join(cid, owner, nil)` で参加させ、`f.schedule(cid, "running")` の後に `f.run(cid, owner, pid, "DONE", acceptedResult, at)` で5件（bob 3件、carol 2件、`at` は1分ずつずらす）入れ、`f.schedule(cid, "ended")` する。

```go
var page submissions.CountedSubmissionList
f.decode(f.request("GET", "/contests/"+cid+"/submissions?limit=2", "", nil, 200), &page)
// len(page.Items)==2, page.HasMore, page.Total==5, 先頭は一番新しい提出
f.decode(f.request("GET", "/contests/"+cid+"/submissions?offset=4&limit=2", "", nil, 200), &page)
// len==1, !HasMore, Total==5
f.decode(f.request("GET", "/contests/"+cid+"/submissions?offset=10&limit=2", "", nil, 200), &page)
// len==0, Total==5
f.decode(f.request("GET", "/contests/"+cid+"/submissions", "", nil, 200), &page)
// len==5, !HasMore, Total==5（limit省略で50件）
for _, q := range []string{"limit=0", "limit=101", "limit=abc"} { f.fail("GET", "/contests/"+cid+"/submissions?"+q, "", nil, 400, "invalid_request") }
// 問題ごとの提出一覧には total がない
body := f.request("GET", "/contests/"+cid+"/problems/"+pid+"/submissions", "", nil, 200)
// strings.Contains(body, `"total"`) が false
```

`docs_test.go` の `TestPublicDocumentation` に、`/contests/{id}/submissions` の GET が `limit`（query、必須でない）を持つことを足す。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd api && TEST_DATABASE_URL=... go test ./internal/httpapi -run 'TestContestSubmissionPagesPostgres|TestPublicDocumentation' -count=1`
Expected: FAIL（`Total` が0、`limit=0` が200）

- [ ] **Step 3: 実装する**

`ContestList` は今の問い合わせの `LIMIT 51` を `LIMIT $5`（`limit+1`）にし、同じ条件 `contestPublic` で `count(*)` を数える問い合わせを別に1回投げる。`contestPublic` は `$1`、`$3`、`$4` だけを使うので、数える問い合わせでも `$2` を参照しないとPostgreSQLが型を決められず失敗する。数える問い合わせでは `AND $2::integer>=0` を足して offset を渡し、理由をコメントに書く。`pageLimit` は `limit` が空なら50を返す。`contestSubmissions` で `contestOffset` の後に呼ぶ。`docs.go` に `publicContestSubmissionPage = submissions.CountedSubmissionList` と、`limitParameter()`（説明「1ページの件数。1〜100。省略すると50件。」、integer、min 1、max 100、default 50）を足し、`/contests/{id}/submissions` だけ Parameters に `limitParameter()` を加える。

- [ ] **Step 4: 通ることを確かめる**

Run: `cd api && TEST_DATABASE_URL=... go test ./internal/httpapi ./internal/submissions -count=1`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/submissions api/internal/httpapi
git commit -m "feat: コンテストの提出一覧で件数を指定でき、総数を返す"
```

---

### Task 2: 問題一覧に `offset`、`limit`、`total` を足す

**Files:**
- Modify: `api/internal/problems/publication.go:141-163`（`PublicList`、新しい `PublicCount`）
- Modify: `api/internal/httpapi/problems.go:56-104`（`publishedProblems`）
- Modify: `api/internal/httpapi/docs.go`（`countedCursorResponse`、`publicProblemPage`、問題一覧の Parameters）
- Modify: `api/internal/httpapi/routes.go:159`
- Test: `api/internal/httpapi/problem_pages_test.go`、`api/internal/httpapi/docs_test.go`

**Interfaces:**
- Consumes: Task 1 の `pageLimit`、`limitParameter`。既存の `contestOffset`（`offset` の読み取り）。
- Produces: `func (s *Store) PublicList(ctx context.Context, cursor *Cursor, offset, limit int, filter ListFilter) ([]PublicProblem, error)`（最大 `limit+1` 件）と `func (s *Store) PublicCount(ctx context.Context, filter ListFilter) (int, error)`。絞り込みのSQLは2つで共有する。
- Produces（httpapi）：`type countedCursorResponse[T any] struct { Items []T; NextCursor string; Total int \`json:"total"\` }`。`/posts` などほかの一覧の `cursorResponse` は変えない。

- [ ] **Step 1: テストを書く**

`TestPublicProblemPagesPostgres`。`newTeamTest(t, "alice", "bob")` で `f.problem(id)` を3つ作り、`f.publish("alice", id, true, 200)` を古い順に行う（公開日時の差を付けるため、必要なら `f.exec` で `published_at` を1分ずつずらす）。どの要求にも `author=alice` を付ける。

```go
type page struct { Items []problems.PublicProblem; NextCursor string; Total int }
// 引数なし：3件、NextCursor=="", Total==3（今までの使い方）
// limit=2：新しい2件、NextCursor!="", Total==3
// cursor=<前のNextCursor>&limit=2：一番古い1件、NextCursor=="", Total==3
// offset=1&limit=1：2番目に新しい1件、NextCursor!="", Total==3
// offset=3：0件、Total==3
// author=bob：0件、Total==0
// cursor=<有効なカーソル>&offset=1、limit=0、limit=101 → 400 invalid_request
```

`TestPublicDocumentation` に、`/problems` の GET が `offset` と `limit` を持つことを足す。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd api && TEST_DATABASE_URL=... go test ./internal/httpapi -run 'TestPublicProblemPagesPostgres|TestPublicDocumentation' -count=1`
Expected: FAIL

- [ ] **Step 3: 実装する**

`publishedProblems` は `cursor` と `offset` の両方が付いていたら400、`offset` があれば `OFFSET`、`cursor` があれば今の条件で `limit+1` 件を取る。`limit` 件より多ければ `limit` 件に切り、最後の問題から `nextContentCursor` を作る（offsetのときも同じ）。`PublicCount` で `total` を数えて `countedCursorResponse` で返す。問題一覧の Parameters は `catalogueParameters()`（`/posts` と共有）に、問題一覧用の `offset`（説明「先頭からスキップする件数。cursorと同時には指定できません。」）と `limitParameter()` を足したものにする。`cursor` の説明の「最大50件」は `/posts` と共有なのでそのままにし、`limit` の説明で件数を補う。

- [ ] **Step 4: 通ることを確かめる**

Run: `cd api && TEST_DATABASE_URL=... go test ./internal/httpapi ./internal/problems -count=1`
Expected: PASS（`TestPartialScoringSolvedPostgres` など `/problems` を読む既存のテストも通る）

- [ ] **Step 5: コミット**

```bash
git add api/internal/problems api/internal/httpapi
git commit -m "feat: 問題一覧でoffsetと件数を指定でき、総数を返す"
```

---

### Task 3: ページ送りの部品を作り、順位表をページに分ける

**Files:**
- Create: `web/app/utils/pagination.ts`
- Create: `web/app/composables/usePageQuery.ts`
- Create: `web/app/composables/usePageSize.ts`
- Create: `web/app/components/PageNav.vue`
- Create: `web/app/components/ListToolbar.vue`
- Modify: `web/app/composables/useOnsiteScope.ts`（切り替えで `page` を外す）
- Modify: `web/app/pages/contests/[id]/index.vue`（順位表の節、`liveChosen` の切り替えで `page` を外す）
- Test: `web/tests/contest-standings.spec.ts`

**Interfaces:**
- Produces（`pagination.ts`）：`pageNumbers(page: number, last: number): number[]`、`lastPage(total: number, size: number): number`（0件でも1）、`clampPage(page: number, last: number): number`、`pageAfterResize(page: number, from: number, to: number): number`（今のページの先頭の行 `(page-1)*from+1` が入るページ）、`pageRange(total: number, page: number, size: number): { first: number, last: number }`。
- Produces：`usePageQuery(): { requested: ComputedRef<number>, link: (page: number) => RouteLocationRaw, go: (page: number) => Promise<void> }`。`requested` は `route.query.page` の整数（1未満や整数でなければ1）。`link` と `go` は今のクエリを保ち、1ページ目では `page` を外す（`go` は `router.replace`）。
- Produces：`export const pageSizes = [20, 50, 100] as const`、`export type PageSize = typeof pageSizes[number]`、`usePageSize(list: 'problems' | 'standings' | 'submissions'): WritableComputedRef<PageSize>`。
- Produces：`<PageNav :page :last :link />`（`link: (page: number) => RouteLocationRaw`）。`<ListToolbar :total :page :size :unit :link @update:size />`（`unit: '問' | '件'`、`size: PageSize`）。件数の文、表示件数の `select`、`PageNav` を1行に並べ、狭い画面では折り返す。`total` が0のときは使う側で出さない。

- [ ] **Step 1: テストを書く**

`contest-standings.spec.ts` に足す。1000行（`rank: i + 1`、`handle: \`user${i}\``、問題aは偶数番目だけAC）を返すモックを使う。

```ts
test('standings are split into pages with AtCoder spacing', async ({ page }) => {
  // goto `/contests/${id}?view=standings&page=20`
  // 表の上：'全 1,000 件中 381〜400 件目'
  // nav「ページ」の先頭：links ['1','5','13','17','18','19','21','22','23','27','35','50']、aria-current="page" は '20'
  // tbody の行は20、先頭は user380
  // tfoot の正解者数は全1000行から数えた値（a は '500 / 1000'）
  // '35' を押す → URL に page=35、先頭は user680。再読み込みしても同じ
  // '1' を押す → URL に page がない。nav は ['2','3','4','8','16','32','50'] と現在の '1'
  // page=50 → ['1','19','35','43','47','48','49'] と現在の '50'
  // page=999 → '全 1,000 件中 981〜1,000 件目'、現在は '50'
  // page=abc と page=0 → '全 1,000 件中 1〜20 件目'
})
test('the page size is remembered per browser and keeps the first row', async ({ page, context }) => {
  // page=3 で「表示件数」を 50 にする → URL に page がない、'全 1,000 件中 1〜50 件目'
  // 再読み込み → 選択は 50、Cookie openoj-page-size-standings は '50'
  // Cookie を '7' にして開く → '全 1,000 件中 1〜20 件目'
})
test('switching the scope or the freeze returns to the first page', async ({ page }) => {
  // onsiteDivision: true、freezeAt を過去、canViewLiveStandings: true の開催中コンテスト
  // page=3 で「オンサイトのみ表示」を押す → URL は scope=onsite で page がない
  // page=3 で「凍結なしで表示」を押す → URL に page がない
})
```

既存の3行のテストには、`'全 3 件中 1〜3 件目'` が出てページ番号の `nav` がないことを足す。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd web && npm run build && npx playwright test tests/contest-standings.spec.ts --output=<scratchpad>/pw`
Expected: FAIL（件数の文とページ番号がない）

- [ ] **Step 3: `pageNumbers` を書く**

ページ番号を決めるのはこの関数だけで、テストはここを画面越しに確かめる。

```ts
export function pageNumbers(page: number, last: number) {
  const pages = new Set([1, last, page])
  for (let step = 1; step < last; step = step < 3 ? step + 1 : step * 2 + 1) {
    for (const p of [page - step, page + step]) if (p >= 1 && p <= last) pages.add(p)
  }
  return [...pages].sort((a, b) => a - b)
}
```

- [ ] **Step 4: 残りの関数、composable、部品を書き、順位表に組み込む**

順位表は `standings`（全行）から `clampPage(requested, lastPage(...))` のページの行を切り出して `tbody` に出す。`tfoot` の集計（`problemStats`）は今のまま全行から数える。表の上に `ListToolbar`、表の下に `PageNav` を置く。1行もないときは今の空の表示だけを出す。表示件数を変えたら `go(pageAfterResize(...))` で移る。`useOnsiteScope` の setter は `page: undefined` も入れる。`liveChosen` を切り替えたら `go(1)`。

- [ ] **Step 5: 通ることを確かめる**

Run: `cd web && npm run build && npx playwright test tests/contest-standings.spec.ts tests/contest-scope.spec.ts tests/contest-freeze.spec.ts tests/contest-teams.spec.ts --output=<scratchpad>/pw`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add web/app web/tests/contest-standings.spec.ts
git commit -m "feat: 順位表をページ番号と表示件数で分ける"
```

---

### Task 4: 順位表で自分の行を強調し、ページの外でも出す

**Files:**
- Create: `web/app/utils/standings-page.ts`
- Modify: `web/app/pages/contests/[id]/index.vue`（順位表の `tbody` と CSS）
- Test: `web/tests/contest-standings.spec.ts`

**Interfaces:**
- Consumes: Task 3 の切り出し（今のページの最初と最後の行の位置）。
- Produces: `isOwnStanding(row: Standing, handle: string | undefined): boolean`（`row.team` があれば `row.team.members` に含むか、なければ `row.handle === handle`。`handle` が空なら false）。
- Produces: 自分の行の `tr` に `class="standing-mine"`（ページ内の行と足した行の両方）。
- Produces: `pinnedStanding(rows: Standing[], start: number, end: number, handle: string | undefined): { row: Standing, place: 'before' | 'after' } | null`（`start` 以上 `end` 未満が今のページ。自分の行の位置が `start` より前なら `before`、`end` 以上なら `after`、ページ内や不在なら null）。

- [ ] **Step 1: テストを書く**

`./fixtures/account` の `test` を使う（ハンドルは `ui_test`）。100行を返すモックで、`ui_test` の位置を変える。

```ts
test('own row is highlighted and pinned above or below the page', async ({ page }) => {
  // ui_test を位置11（順位12）に置き page=3：tbody の1行目が ui_test（class standing-mine）、2行目が '⋮' だけの行、3行目が位置40の行。行は22
  // 強調の行の背景は 'rgb(237, 249, 241)'、名前の欄に visually-hidden の '（自分）'
  // ui_test を位置86に置く：最後の行が ui_test、その前が '⋮'、その後に tfoot
  // ui_test を位置45に置く：行は20で '⋮' はなく、ページ内の ui_test が強調される
})
test('a tie across the page boundary is placed by position', async ({ page }) => {
  // 位置59と位置60を同じ rank: 60 にし、位置60を ui_test にする。page=3（位置40〜59）→ ui_test は末尾に出る
})
test('own team row is highlighted in a team contest', async ({ page }) => {
  // teamMode: true、team: { members: ['bob', 'ui_test'] } の行が強調される
})
```

未ログインのテスト（`@playwright/test` の `test`）で、`.standing-mine` が0個であることを既存の1000行のテストに足す。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd web && npm run build && npx playwright test tests/contest-standings.spec.ts --output=<scratchpad>/pw`
Expected: FAIL

- [ ] **Step 3: 実装する**

`useAccount()` の `profile.value?.handle` を使う。足す行と「⋮」の行は今のページの行の前か後に置き、どちらも `tbody` の中で、`tfoot` より上にする。「⋮」の行は `aria-hidden="true"`、列をすべてまたぐ1つのセル。偶数行の背景（`tbody tr:nth-child(even)`）より強調の背景を優先させる。

- [ ] **Step 4: 通ることを確かめる**

Run: `cd web && npm run build && npx playwright test tests/contest-standings.spec.ts tests/contest-teams.spec.ts --output=<scratchpad>/pw`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add web/app web/tests/contest-standings.spec.ts
git commit -m "feat: 順位表で自分の行を強調し、ほかのページでも表示する"
```

---

### Task 5: コンテストの提出一覧をページ番号と表示件数で送る

**Files:**
- Modify: `web/server/api/contests/[...path].get.ts`（`limit` を渡す）
- Modify: `web/app/pages/contests/[id]/index.vue`（提出一覧の節と `loadSubmissions`）
- Test: `web/tests/contest-submissions.spec.ts`

**Interfaces:**
- Consumes: Task 1 の応答 `{ items, hasMore, total }`。Task 3 の `usePageQuery`、`usePageSize('submissions')`、`ListToolbar`（`unit: '件'`）、`PageNav`、`pageAfterResize`、`clampPage`。

- [ ] **Step 1: テストを書く**

```ts
test('the submission list pages by number and size', async ({ page }) => {
  // canViewSubmissions: true、onsiteDivision: true の終了したコンテスト。/submissions のモックは offset と limit で
  // 120件から切り出し total を返し、要求のURLを記録する
  // goto `?view=submissions&page=2` → 要求は offset=50&limit=50、'全 120 件中 51〜100 件目'、nav は '1' '3' と現在の '2'
  // 「表示件数」を20 → 要求は offset=40&limit=20、URL は page=3
  // page.clock で15秒進め、total を121にする → '全 121 件中 41〜60 件目'、URL は page=3 のまま
  // 「オンサイトのみ表示」を押す → URL に page がなく、要求は offset=0&limit=20&scope=onsite
  // page=99 → URL は最後のページ（20件ずつで page=7）に置き換わる
})
```

- [ ] **Step 2: 失敗を確かめる**

Run: `cd web && npm run build && npx playwright test tests/contest-submissions.spec.ts --output=<scratchpad>/pw`
Expected: FAIL

- [ ] **Step 3: 実装する**

`offset` の `ref` をやめ、`offset = (page - 1) * size` を要求に入れる。`watch` は `requested` と表示件数を見る。応答の `total` から最後のページを求め、範囲外なら `go(last)`。`ContentPagination` を `ListToolbar` と `PageNav` に置き換える。BFFは `query.limit` が文字列なら `search.set('limit', ...)`。`watch(filterSubmissions, ...)` の `offset.value = 0` は、`useOnsiteScope` が `page` を外すので消す。

- [ ] **Step 4: 通ることを確かめる**

Run: `cd web && npm run build && npx playwright test tests/contest-submissions.spec.ts tests/contest-scope.spec.ts tests/contest-teams.spec.ts --output=<scratchpad>/pw`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add web/server web/app web/tests/contest-submissions.spec.ts
git commit -m "feat: コンテストの提出一覧をページ番号と表示件数で送る"
```

---

### Task 6: 問題一覧をページ番号と表示件数で送る

**Files:**
- Modify: `web/server/utils/public-content.ts`（`publicCursor` に渡すキー）
- Modify: `web/server/api/problems/index.get.ts`
- Modify: `web/shared/types/problem.ts:8`（`publicProblemListSchema` に `total`）
- Modify: `web/app/pages/problems/index.vue`
- Modify: `web/tests/fixtures/public-api.mjs`（`/problems`）
- Test: `web/tests/catalogue.spec.ts`

**Interfaces:**
- Consumes: Task 2 の応答 `{ items, nextCursor, total }`。Task 3 の `usePageQuery`、`usePageSize('problems')`、`ListToolbar`（`unit: '問'`）、`PageNav`、`pageAfterResize`、`clampPage`、`lastPage`。
- Produces: `publicCursor(event: H3Event, keys: string[] = ['cursor', 'author', 'tag'])`。問題一覧は `['cursor', 'author', 'tag', 'offset', 'limit']` を渡す。`publicProblemListSchema` は `total: z.number().int().nonnegative()` を持つ。

- [ ] **Step 1: フィクスチャとテストを書く**

`public-api.mjs` の `/problems` は常に `total` を返す。`tag=pagination` のときは、`問題 001`〜`問題 120`（この順が新しい順）の120件から `offset` と `limit`（省略時50）で切り出す。ほかの一覧のテストのため、`cursor` を使う今の応答も残す。

`catalogue.spec.ts` の、問題とブログをまとめて回すテストからは問題を外し、ブログだけにする（ブログは今の「前へ／次へ」のまま）。問題には次を足す。

```ts
test('problem list pages by number and keeps the page in the URL', async ({ page, context }) => {
  // goto '/problems?tag=pagination&page=2'（サーバーで描画）→ '全 120 問中 51〜100 問目'、先頭の行は '問題 051'
  // canonical は 'https://judge.example/problems?tag=pagination&page=2'
  // nav は '1' '3' と現在の '2'。'3' を押す → URL に page=3、行は20で '問題 101'〜'問題 120'
  // 「表示件数」を20 → URL は page=6、先頭は '問題 101'。再読み込みしても20件ずつ
  // Cookie を消して page=99 を開く → URL は page=3、canonical も page=3
  // page=1 を開く → canonical は 'https://judge.example/problems?tag=pagination'
})
test('a failed page shows an alert and retries', async ({ page }) => {
  // page.route('**/api/problems*') で offset=100 の最初の1回だけ502、それ以外は route.continue()
  // page=2 から '3' を押す → role=alert に '一覧を取得できませんでした。もう一度お試しください。'、表はない
  // 「再試行」を押す → '問題 101' が出る
})
```

AC済みの行の強調（`tr.solved`、背景 `rgb(237, 249, 241)`、リンク名「（AC 済み）」）を確かめていた部分は、新しいテストに移す（`/api/my/solved-problems` のモックで `問題 101` のIDを返す）。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd web && npm run build && npx playwright test tests/catalogue.spec.ts --output=<scratchpad>/pw`
Expected: FAIL

- [ ] **Step 3: 実装する**

`useContentPages` をやめ、`useFetch('/api/problems', { query })` の `query` を `tag`、`offset`、`limit` の computed にする。サーバーで描画するときに `page` が最後より大きければ、`navigateTo(link(last), { replace: true })` で最後のページへ置き換える（ブラウザ上の移動でも同じ）。canonical と `useSharePreview` の `path` は表示しているページのURLにし、1ページ目では `page` を付けない。最初の描画での失敗は今のまま502のエラーページにする。ページを移ったあとの失敗では表を隠し、`role="alert"` の「一覧を取得できませんでした。もう一度お試しください。」と「再試行」ボタン（`refresh()`）を出す。タグを表示するスイッチ、タグ検索、AC済みの強調は今のまま残す。

- [ ] **Step 4: 通ることを確かめる**

Run: `cd web && npm run build && npx playwright test tests/catalogue.spec.ts tests/tags.spec.ts tests/share-preview.spec.ts tests/problem-navigation.spec.ts tests/account-menu.spec.ts --output=<scratchpad>/pw`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add web/server web/shared web/app web/tests
git commit -m "feat: 問題一覧をページ番号と表示件数で送る"
```

---

### Task 7: 全体を確かめる

**Files:**
- なし（失敗があれば、その原因のタスクのファイルを直す）

- [ ] **Step 1: 型と全テスト**

Run: `cd web && npm run typecheck && npm run build && npx playwright test --output=<scratchpad>/pw`
Expected: PASS

Run: `cd api && go vet ./... && TEST_DATABASE_URL=... go test ./... -count=1`
Expected: PASS

- [ ] **Step 2: 狭い画面**

順位表、提出一覧、問題一覧のテストに、幅320と375で `document.documentElement.scrollWidth <= innerWidth` を確かめる行があることを確認し、なければTask 3、5、6のテストに足して通す。

- [ ] **Step 3: グラフの更新とコミット**

Run: `graphify update .`

```bash
git add -A web api
git commit -m "test: 一覧のページ送りを狭い画面でも確かめる"
```

（Step 2 で何も足さなかったらコミットしない）
