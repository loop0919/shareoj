# 問題一覧の並び替えと絞り込み Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 問題一覧（`/problems`）で、並び順を選び、解答状況・コンテスト・難易度・タグで絞り込めるようにし、選んだ条件を次に開いたときも残す（Issue #35）。

**Architecture:** 並び替えと絞り込みは、ページ送りと合うようにAPIのSQLで行う。解答状況だけは誰の解答かが要るので、ログイン中の利用者として呼ぶ入口を1つ足し、WebサーバーがCookieのセッションを見て呼び分ける。画面の条件はURLのクエリだけを元にし、パネルと札はそれを映す。覚えた条件はCookieに置き、クエリのない`/problems`を開いたときにURLへ置き換える。

**Tech Stack:** Go（pgx、`api/internal/httpapi`、`api/internal/problems`）、PostgreSQL、Nuxt 4（Vue 3、zod）、Playwright。ライブラリは足さない。

**Spec:** [docs/superpowers/specs/2026-11-28-problem-list-sort-filter-design.md](../specs/2026-11-28-problem-list-sort-filter-design.md)。各タスクの前に、対応する節を読む。

## Global Constraints

- クエリの名前と値（この計画で決めた。APIもWebも同じ）。初期値はURLに出さない。

  | クエリ | 値 | 初期 |
  | --- | --- | --- |
  | `sort` | `newest` `oldest` `solvers-desc` `solvers-asc` `favorites-desc` `difficulty-desc` `difficulty-asc` | `newest` |
  | `solved` | `unsolved` `solved` | なし |
  | `contest` | `only` `exclude`（今のまま） | なし |
  | `difficulty` | `下限-上限`。どちらも1〜10で、下限≦上限。例 `3-6` | なし |
  | `tag` | 今のまま | なし |

- APIは、知らない値に400を返す。エラーコードは`invalid_sort`、`invalid_difficulty`、`invalid_solved`。
- Webは、URLの知らない値を初期値として読む（今の`?contest=yes`と同じ）。`difficulty=1-10`も初期値として読み、APIには送らない。
- 難易度は「作成者設定」（`published_draft->>'difficulty'`）を使う。`difficulty`を指定したら、未設定の問題は外れる。難易度で並べるとき、未設定はどちら向きでも最後。
- 並びが同じ値の問題は、公開日が新しい順、次にIDの降順で並べる。
- 「AC済み」の判定は`GET /my/solved-problems`（`ownAcceptedSubmission`）と同じにする。
- `cursor`は`sort`が初期のときだけ使える。ほかの`sort`と併用されたら`400 invalid_request`。
- 公開の`GET /problems`は`solved`を読まない。
- 覚えるCookieは`openoj-problem-conditions`。`usePageSize`と同じ設定（1年、`sameSite: 'lax'`、`path: '/'`）。中身は`sort`、`solved`、`contest`、`difficulty`のクエリの形（初期値は入れない）。タグとページ番号は入れない。
- 画面の文言。
  - ボタン: 「絞り込み・並び替え」
  - 欄: 「並び順」「解答状況」「コンテスト」「難易度」「タグ」
  - 並び順: 「公開日が新しい順」「公開日が古い順」「正解者数が多い順」「正解者数が少ない順」「お気に入りが多い順」「難易度が高い順」「難易度が低い順」
  - 解答状況: 「すべて」「未ACのみ」「AC済みのみ」
  - コンテスト: 「すべての問題」「コンテストの問題のみ」「コンテストの問題を除く」
  - 難易度の選択欄の名前: 「難易度の下限」「難易度の上限」。選択肢は「Lv.1」〜「Lv.10」
  - 札: 並び順はその文言、解答状況とコンテストはその文言、難易度は「難易度 Lv.3〜6」、タグは「タグ: DP」。×の名前は「（札の文言）を外す」
  - 「条件をすべて外す」「条件に合う問題はありません。」「ログインすると使えます」
- 検索エンジン向けの正規URL（canonical）に入れるのは、今と同じ`tag`、`contest`、`page`だけ。
- 一覧はサーバーが最初から描く。条件を当てた一覧も、JavaScriptなしで読める。
- データベースは、索引を足すことはあっても、表の形は変えない。表の形を変える必要が出たら、実装を止めて利用者に相談する。
- コメントは英語で、既存のファイルの書き方に合わせる。
- コミットは`prefix: 対応内容`の形式で、タスクごとに行う。末尾に`Co-Authored-By: Claude Fable 5.2 <noreply@anthropic.com>`を付ける。コードを変えたら`graphify update .`を実行する。
- このリポジトリは公開されている。鍵やリソースのIDを書かない。
- ブランチ`feat/problem-list-conditions`で作業する。pushはしない（mainへのpushは自動で配布される）。

### テストの流し方

コマンドは`nix develop`の中から実行する。ローカルのDBは`make db-up`で起こす（すでに動いていれば失敗するが、そのまま使える）。

```bash
export TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable'
cd api && go test ./internal/httpapi/ -run TestProblemList -count=1
cd web && bun run typecheck && bun run build
cd web && bun run test tests/problem-list-conditions.spec.ts --output=<一時フォルダ>/pw --reporter=line
cd web && bun run test --config playwright.account.config.ts tests/account/problem-list.spec.ts --output=<一時フォルダ>/pw-account --reporter=line
```

- Playwrightは`.output`のビルドに対して走る。Webを変えたら、テストの前に`bun run build`をやり直す。
- `web/test-results`は書き込めない。必ず`--output`で別のフォルダを指定する。
- pushの前に、`cd api && go test -race ./... -count=1`と、`bun run test`（全体）と、アカウント系の全体を通す。

## Review Focus

仕様が暗に求めているが、ふつうに書くとテストから漏れやすいもの。それぞれ、持ち主のタスクにテストを入れてある。

1. 覚えたCookieの中身が壊れている、または古い形のとき。初期の条件で開き、エラーにも置き換えの繰り返しにもならない（Task 4、Task 7）。
2. `solved`がURLにあり、セッションのCookieはあるがAPIに断られたとき。全問の一覧に落とし、札を出さず、エラーページにしない（Task 5）。
3. 並びの値が全部同じ問題をページ送りしたとき。1ページ目と2ページ目に同じ問題が出ない（Task 1）。
4. ブラウザーの「戻る」で前の条件に戻ったとき。パネルの欄、札、一覧がURLと合う（Task 6）。
5. `/problems?page=3`のようにページ番号だけのURLを、覚えた条件がある人が開いたとき。置き換えずに、初期の条件のそのページを出す（Task 7）。最後のページを超えた番号が今の仕組みで番号なしのURLに直されたときは、クエリがなくなるので覚えた条件が当たる。これは受け入れる。

---

### Task 1: APIの並び順と難易度の範囲

**Files:**
- Modify: `api/internal/problems/publication.go`（`ListFilter`、`publicFilter`、`PublicList`）
- Modify: `api/internal/httpapi/problems.go`（`publishedProblems`の一覧の部分）
- Modify: `api/internal/httpapi/docs.go`（`problemParameters`）
- Test: `api/internal/httpapi/problem_list_test.go`（新規）

**Interfaces:**
- Produces（Task 2が使う）:

  ```go
  // api/internal/problems/publication.go
  type ListFilter struct {
      Author, Tag, Contest string
      Sort                 string // one of the Sort* constants; "" reads as SortNewest
      DifficultyMin        int    // 0 with DifficultyMax 0 means no range
      DifficultyMax        int
      Solved               string // SolvedOnly or UnsolvedOnly, with Viewer
      Viewer               string
  }
  const (
      SortNewest = "newest"; SortOldest = "oldest"
      SortSolversDesc = "solvers-desc"; SortSolversAsc = "solvers-asc"
      SortFavoritesDesc = "favorites-desc"
      SortDifficultyDesc = "difficulty-desc"; SortDifficultyAsc = "difficulty-asc"
      SolvedOnly = "solved"; UnsolvedOnly = "unsolved"
  )
  func ValidSort(sort string) bool
  ```

  ```go
  // api/internal/httpapi/problems.go
  // listFilter reads sort and difficulty (and, with a viewer, solved) on top of catalogueFilter and contest.
  func listFilter(w http.ResponseWriter, r *http.Request, viewer string) (problems.ListFilter, bool)
  ```

- [ ] **Step 1: 失敗するテストを書く**

  `TestProblemListConditionsPostgres`を足す。DBの用意は`tags_test.go`の`TestTagsPostgres`（先頭からルーターを作るところまで）と同じ形にする。
  公開の問題を5問作る（`UPDATE problem_drafts SET published_draft=...`で公開する書き方も同じ）。

  | 問題 | 公開日 | 難易度 | 正解者数 | お気に入り |
  | --- | --- | --- | --- | --- |
  | a | 5日前 | 3 | 2 | 0 |
  | b | 4日前 | 未設定 | 0 | 2 |
  | c | 3日前 | 7 | 2 | 1 |
  | d | 2日前 | 3 | 1 | 0 |
  | e | 1日前 | 10 | 0 | 0 |

  正解者は、bobがaとc、carolがaとcとd。`submissions`に`'{"privateDraft":false}','DONE','{"verdict":"AC"}'`の行で入れる（`profile_test.go`の`/my/solved-problems`のテストと同じ書き方）。
  お気に入りは、bobとcarolがb、bobがc。

  ```go
  listed := func(path string) []string // GET path, 200, the ids of items in order
  for _, c := range []struct{ query string; want []string }{
      {"", []string{e, d, c, b, a}},
      {"sort=newest", []string{e, d, c, b, a}},
      {"sort=oldest", []string{a, b, c, d, e}},
      {"sort=solvers-desc", []string{c, a, d, e, b}},   // ties: newer first
      {"sort=solvers-asc", []string{e, b, d, c, a}},
      {"sort=favorites-desc", []string{b, c, e, d, a}},
      {"sort=difficulty-desc", []string{e, c, d, a, b}}, // unset last
      {"sort=difficulty-asc", []string{d, a, c, e, b}},  // unset last
      {"difficulty=3-7", []string{d, c, a}},
      {"difficulty=1-10", []string{e, d, c, a}},         // a given range drops the unset
      {"difficulty=10-10&sort=oldest", []string{e}},
      {"difficulty=3-7&contest=exclude&sort=solvers-desc", []string{c, a, d}},
  } { /* listed("/problems?"+c.query) equals c.want, and total == len(c.want) */ }
  for _, bad := range []string{"sort=popular", "difficulty=0-3", "difficulty=6-3", "difficulty=3", "difficulty=3-11", "difficulty=a-b"} {
      // GET /problems?bad answers 400 with invalid_sort or invalid_difficulty
  }
  // Review Focus 3: with limit=2, sort=solvers-desc, offsets 0, 2 and 4 together list each problem exactly once.
  // A cursor continues only the initial order.
  // GET /problems?limit=2 -> nextCursor; GET /problems?limit=2&cursor=<it> -> [c, b]
  // GET /problems?sort=oldest&cursor=<it> -> 400 invalid_request
  // The public route does not read solved.
  // GET /problems?solved=unsolved -> all five
  ```

- [ ] **Step 2: テストが失敗することを確かめる**

  Run: `cd api && go test ./internal/httpapi/ -run TestProblemListConditionsPostgres -count=1`
  Expected: FAIL（`sort=oldest`の並びが違う）

- [ ] **Step 3: `ListFilter`、`ValidSort`、並び順、難易度の範囲を実装する**

  - `publicFilter`に難易度の範囲を足す。`PublicCount`と共用なので、合計も合う。
  - `PublicList`の`ORDER BY`を`Sort`で切り替える。正解者数とお気に入り数は、`SELECT`にすでにある式を並びに使う。どの並びも最後に`d.published_at DESC,d.id DESC`を足す（`oldest`だけは`d.published_at ASC,d.id ASC`）。
  - 並び順の文字列はGoの定数から選び、利用者の入力をSQLに連結しない。

- [ ] **Step 4: `listFilter`を実装し、`publishedProblems`から使う**

  `sort`が省略なら`SortNewest`。`cursor`があり`Sort`が`SortNewest`でなければ`400 invalid_request`。
  `docs.go`の`problemParameters`に`sort`と`difficulty`を足す（説明は日本語、値の一覧と例を書く）。`GET /problems`の説明文の「公開日時の降順。」を「既定は公開日時の降順。」に直す。

- [ ] **Step 5: テストが通ることを確かめる**

  Run: `cd api && go test ./internal/httpapi/ ./internal/problems/ -count=1`
  Expected: PASS

- [ ] **Step 6: コミット**

  `feat: 公開問題の一覧を並び替え、難易度の範囲で絞り込めるようにする`

---

### Task 2: 解答状況で絞り込む入口

**Files:**
- Modify: `api/internal/problems/publication.go`（`publicFilter`）
- Modify: `api/internal/httpapi/problems.go`（`listFilter`）
- Modify: `api/internal/httpapi/routes.go`
- Modify: `api/internal/httpapi/routes_test.go`（DBなしの表に1行）
- Modify: `api/README.md`
- Test: `api/internal/httpapi/problem_list_test.go`

**Interfaces:**
- Consumes: Task 1の`ListFilter`、`listFilter`
- Produces（Task 5が使う）: `GET /my/public-problems`。`GET /problems`と同じクエリに`solved`を足したものを受け、同じ形（`items`、`nextCursor`、`total`）を返す。認証が要る。

- [ ] **Step 1: 失敗するテストを書く**

  Task 1のテストに続けて書く。bobはaとcをACしている（Task 1のデータ）。aliceは何もACしていない。

  ```go
  // as(path, actor) lists the ids like listed, signed in as actor
  // as("/my/public-problems", "bob")                       -> [e, d, c, b, a]
  // as("/my/public-problems?solved=unsolved", "bob")       -> [e, d, b], total 3
  // as("/my/public-problems?solved=solved", "bob")         -> [c, a], total 2
  // as("/my/public-problems?solved=solved&sort=oldest&difficulty=3-7", "bob") -> [a, c]
  // as("/my/public-problems?solved=solved", "alice")       -> [], total 0
  // The solved filter agrees with the rows the list paints green.
  // the ids of ?solved=solved for bob, sorted, equal the items of GET /my/solved-problems for bob, sorted
  // A run that /my/solved-problems does not count is not solved here either:
  // insert for alice on problem e a DONE AC run with job {"privateDraft":true}; ?solved=solved for alice stays []
  // GET /my/public-problems?solved=maybe as bob -> 400 invalid_solved
  // GET /my/public-problems without a session -> 401
  ```

  `routes_test.go`のDBなしの表に`{"GET", "/my/public-problems", 503, "database_unavailable"}`を足す（`/my/solved-problems`の行の隣）。

- [ ] **Step 2: テストが失敗することを確かめる**

  Run: `cd api && go test ./internal/httpapi/ -run 'TestProblemListConditionsPostgres|TestRoutes' -count=1`
  Expected: FAIL（`/my/public-problems`が404）

- [ ] **Step 3: 実装する**

  - `routes.go`に`private("GET /my/public-problems", problems.publishedProblems, true)`を足す（`GET /my/solved-problems`の近く）。`publishedProblems`は、IDのないときに一覧を返す。
  - `listFilter`は、`viewer`があるときだけ`solved`を読み、`Solved`と`Viewer`を入れる。
  - `publicFilter`は、`Solved`があるとき、`submissions`に`ownAcceptedSubmission`を満たす`Viewer`の行があるかどうかで絞る。`SolvedProblems`と同じ式を使い、条件を書き写さない。

- [ ] **Step 4: テストが通ることを確かめる**

  Run: `cd api && go test ./internal/httpapi/ ./internal/problems/ -count=1`
  Expected: PASS

- [ ] **Step 5: `api/README.md`を直す**

  `GET /problems?contest=only|exclude`の行の下に、`sort`、`difficulty`、`GET /my/public-problems`の`solved`を1行ずつ足す。値、初期値、400のコード、`cursor`との併用を書く。

- [ ] **Step 6: コミット**

  `feat: ログイン中の利用者が、公開問題の一覧を解答状況で絞り込めるようにする`

---

### Task 3: 一覧の速さを測る

**Files:**
- Test: `api/internal/httpapi/problem_list_test.go`

**Interfaces:**
- Consumes: Task 1、Task 2の`Store.PublicList`、`Store.PublicCount`

- [ ] **Step 1: 速さのテストを書く**

  `TestProblemListSpeedPostgres`。`OPENOJ_LIST_SPEED=1`のときだけ走り、ふだんの`go test`とCIでは飛ばす。
  `generate_series`で、公開の問題1000問（難易度は1〜10と未設定を混ぜる）、利用者200人、ACの提出30000件、お気に入り5000件を入れる。
  7つの`Sort`それぞれについて、絞り込みなしと`Solved: UnsolvedOnly`（利用者1人）の2通りで、`PublicList(ctx, nil, 0, 50, filter)`と`PublicCount(ctx, filter)`を3回ずつ呼ぶ。
  いちばん速かった回の合計が200ミリ秒を超えたら失敗にし、どの並びが何ミリ秒だったかを`t.Logf`で出す。

- [ ] **Step 2: 測る**

  Run: `cd api && OPENOJ_LIST_SPEED=1 go test ./internal/httpapi/ -run TestProblemListSpeedPostgres -count=1 -v`
  Expected: PASS。出た数字を控える（Task 8のADRに書く）。

- [ ] **Step 3: 超えたときだけ、索引を足す**

  `EXPLAIN (ANALYZE)`で遅い所を見て、索引で直るなら`api/internal/database/`に次の番号のマイグレーションを足し、Step 2をやり直す。
  索引で直らず、数えた値を列に持つような直し方が要るなら、ここで止めて利用者に相談する。

- [ ] **Step 4: コミット**

  `test: 問題一覧の並び替えと絞り込みの速さを測るテストを足す`

---

### Task 4: Webの条件の型と変換

**Files:**
- Create: `web/app/utils/problem-conditions.ts`
- Test: `web/tests/problem-conditions.spec.ts`（新規。`web/tests/`のほかの純粋な関数のテストと同じく、ページを開かずに関数を直接呼ぶ）

**Interfaces:**
- Produces（Task 5〜7が使う）:

  ```ts
  export const problemSorts: readonly { key: ProblemSort, label: string }[]   // the seven, in the order of Global Constraints
  export type ProblemSort = 'newest' | 'oldest' | 'solvers-desc' | 'solvers-asc' | 'favorites-desc' | 'difficulty-desc' | 'difficulty-asc'
  export type ProblemSolved = '' | 'unsolved' | 'solved'
  export type ProblemContest = '' | 'only' | 'exclude'
  export type ProblemConditions = { sort: ProblemSort, solved: ProblemSolved, contest: ProblemContest, min: number, max: number, tag: string }
  export type ConditionKey = 'sort' | 'solved' | 'contest' | 'difficulty' | 'tag'
  export const defaultConditions: ProblemConditions   // newest, '', '', 1, 10, ''
  // Reads a route query or a remembered cookie. Anything unknown reads as the initial value.
  export function parseConditions(query: unknown): ProblemConditions
  // The query of a list with these conditions; initial values are left out.
  export function conditionQuery(conditions: ProblemConditions): Record<string, string>
  // conditionQuery without the tag: what the cookie keeps.
  export function rememberedQuery(conditions: ProblemConditions): Record<string, string>
  // The conditions in force, in the order sort, solved, contest, difficulty, tag. solved is listed only when applied.
  export function conditionChips(conditions: ProblemConditions, solvedApplied: boolean): { key: ConditionKey, label: string }[]
  // The conditions with one of them back at its initial value.
  export function withoutCondition(conditions: ProblemConditions, key: ConditionKey): ProblemConditions
  ```

- [ ] **Step 1: 失敗するテストを書く**

  ```ts
  test('conditions read from a query and write back without initial values', () => {
    expect(parseConditions({})).toEqual(defaultConditions)
    const all = parseConditions({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', difficulty: '3-6', tag: 'DP', page: '2' })
    expect(all).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', min: 3, max: 6, tag: 'DP' })
    expect(conditionQuery(all)).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', difficulty: '3-6', tag: 'DP' })
    expect(rememberedQuery(all)).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', difficulty: '3-6' })
    expect(conditionQuery(defaultConditions)).toEqual({})
  })
  test('unknown values read as the initial ones', () => {   // Review Focus 1
    for (const bad of [{ sort: 'popular' }, { sort: ['oldest', 'newest'] }, { solved: 'yes' }, { contest: 'yes' }, { difficulty: '6-3' }, { difficulty: '0-3' }, { difficulty: '3-11' }, { difficulty: '3' }, { difficulty: '1-10' }, { tag: ['a', 'b'] }])
      expect(parseConditions(bad)).toEqual(defaultConditions)
    for (const broken of [null, undefined, 'sort=oldest', 42, [], { sort: { key: 'oldest' } }])
      expect(parseConditions(broken)).toEqual(defaultConditions)
  })
  test('chips name the conditions in force', () => {
    const all = parseConditions({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', difficulty: '3-6', tag: 'DP' })
    expect(conditionChips(all, true).map(chip => chip.label)).toEqual(['正解者数が多い順', '未ACのみ', 'コンテストの問題のみ', '難易度 Lv.3〜6', 'タグ: DP'])
    expect(conditionChips(all, false).map(chip => chip.key)).toEqual(['sort', 'contest', 'difficulty', 'tag'])
    expect(conditionChips(defaultConditions, true)).toEqual([])
    expect(conditionChips(parseConditions({ difficulty: '4-4' }), true)[0].label).toBe('難易度 Lv.4〜4')
    expect(conditionQuery(withoutCondition(all, 'difficulty'))).toEqual({ sort: 'solvers-desc', solved: 'unsolved', contest: 'only', tag: 'DP' })
  })
  ```

- [ ] **Step 2: テストが失敗することを確かめる**

  Run: `cd web && bun run test tests/problem-conditions.spec.ts --output=<一時フォルダ>/pw --reporter=line`
  Expected: FAIL（モジュールがない）

- [ ] **Step 3: `problem-conditions.ts`を実装する**

  Nuxtの自動インポートを使わない（テストが直接読むため）。タグの値は検査せず、文字列ならそのまま持つ（正しさはAPIが見る）。

- [ ] **Step 4: テストが通ることを確かめる**

  Run: Step 2と同じ。Expected: PASS

- [ ] **Step 5: コミット**

  `feat: 問題一覧の条件を、URLのクエリと相互に変換する`

---

### Task 5: Webサーバーの経路と、テスト用のAPI

**Files:**
- Modify: `web/server/api/problems/index.get.ts`
- Modify: `web/tests/fixtures/public-api.mjs`
- Test: `web/tests/problem-list-conditions.spec.ts`（新規）

**Interfaces:**
- Consumes: Task 2の`GET /my/public-problems`
- Produces（Task 6が使う）: `GET /api/problems`は、今の応答に`solved: '' | 'unsolved' | 'solved'`（実際に当てた解答状況）を足して返す。`sort`と`difficulty`をAPIへ渡す。

- [ ] **Step 1: テスト用のAPI（`public-api.mjs`）を合わせる**

  - `/problems`と`/my/public-problems`で、本物と同じく`sort`、`difficulty`、（後者だけ）`solved`の知らない値に400を返す。
  - `tag=pagination`の120問: `sort=oldest`は逆順にする。難易度はすべて10なので、`difficulty`の範囲に10が入らなければ0件。`/my/public-problems`では、番号が奇数の60問をAC済みとして`solved`を当てる。
  - タグなしの1問（A + B、難易度は未設定）: `difficulty`があれば0件。`/my/public-problems`では未ACとして扱う。
  - `/my/public-problems`の認証は、今の`/my/`の扱い（`Bearer valid-access`以外は401）のままでよい。

- [ ] **Step 2: 失敗するテストを書く**

  ```ts
  const session = { cookie: 'openoj_access=valid-access' }
  test('the list route passes the order and the difficulty range on', async ({ request }) => {
    const oldest = await (await request.get('/api/problems?tag=pagination&sort=oldest&limit=20')).json()
    expect(oldest.items[0].id).toBe('00000000-0000-4000-8000-000000000120')
    expect(oldest.solved).toBe('')
    expect((await (await request.get('/api/problems?tag=pagination&difficulty=1-9')).json()).total).toBe(0)
    expect((await request.get('/api/problems?sort=popular')).status()).toBe(400)
  })
  test('the solved filter needs a session and says whether it was applied', async ({ request }) => {
    const mine = await (await request.get('/api/problems?tag=pagination&solved=unsolved', { headers: session })).json()
    expect([mine.total, mine.solved]).toEqual([60, 'unsolved'])
    const guest = await (await request.get('/api/problems?tag=pagination&solved=unsolved')).json()
    expect([guest.total, guest.solved]).toEqual([120, ''])
    // Review Focus 2: a session the API refuses falls back to the whole list.
    const expired = await (await request.get('/api/problems?tag=pagination&solved=unsolved', { headers: { cookie: 'openoj_access=expired' } })).json()
    expect([expired.total, expired.solved]).toEqual([120, ''])
    // An unknown value is not sent on.
    const unknown = await (await request.get('/api/problems?tag=pagination&solved=maybe', { headers: session })).json()
    expect([unknown.total, unknown.solved]).toEqual([120, ''])
  })
  ```

- [ ] **Step 3: テストが失敗することを確かめる**

  Run: `cd web && bun run build && bun run test tests/problem-list-conditions.spec.ts --output=<一時フォルダ>/pw --reporter=line`
  Expected: FAIL（`sort=oldest`が効かない）

- [ ] **Step 4: `index.get.ts`を実装する**

  - 公開の呼び出しに渡すクエリに`sort`と`difficulty`を足す。
  - `solved`が`unsolved`か`solved`で、`hasSession(event)`なら、`privateAPI`で`/my/public-problems`を呼ぶ。応答のヘッダーは`privateHeaders(event)`にする。
    配信モードでも通す（`{ streaming: true }`）。行を緑にする`/api/my/solved-problems`と同じ情報だからである。
  - その呼び出しが`sessionRefused`で失敗したら、公開の呼び出しに落とす。ほかの失敗はそのまま返す。
  - `web/tests/streaming-mode.spec.ts`に、配信モードで通す経路を数えるテストがあれば合わせる。

- [ ] **Step 5: テストが通ることを確かめる**

  Run: Step 3と同じ。Expected: PASS。`bun run test tests/catalogue.spec.ts tests/streaming-mode.spec.ts`も通す。

- [ ] **Step 6: コミット**

  `feat: 問題一覧の経路で、並び順と難易度と解答状況をAPIへ渡す`

---

### Task 6: パネル、札、URLへの反映

**Files:**
- Create: `web/app/components/ProblemConditions.vue`
- Create: `web/app/composables/useProblemConditions.ts`
- Modify: `web/app/pages/problems/index.vue`
- Modify: `web/app/components/TagLinks.vue`（`keep`を足す）
- Modify: `web/app/components/StatusFilter.vue`（`disabled`を足す）
- Modify: `web/tests/catalogue.spec.ts`（コンテストの選択欄のテストを、パネルの操作に直す）
- Test: `web/tests/problem-list-conditions.spec.ts`

**Interfaces:**
- Consumes: Task 4の関数、Task 5の`solved`
- Produces（Task 7が使う）:

  ```ts
  // web/app/composables/useProblemConditions.ts
  export function useProblemConditions(): {
    conditions: ComputedRef<ProblemConditions>               // read from the route's query
    change: (patch: Partial<ProblemConditions>) => Promise<void>  // opens /problems with the new conditions, on page 1
    clear: () => Promise<void>                                // opens /problems with no query
  }
  ```

  ```vue
  <!-- web/app/components/ProblemConditions.vue -->
  <!-- props: conditions: ProblemConditions, solvedApplied: boolean, signedIn: boolean, showTags: boolean -->
  <!-- emits: change(patch: Partial<ProblemConditions>), clear() -->
  <!-- v-model:open: boolean -->
  ```

  ```ts
  // web/app/components/TagLinks.vue: keep carries the list's other conditions across a tag link
  defineProps<{ tags: string[], to: '/problems' | '/blog', keep?: Record<string, string> }>()
  // web/app/components/StatusFilter.vue: disabled turns every button off
  // props gain: disabled?: boolean
  ```

- [ ] **Step 1: 失敗するテストを書く**

  `./fixtures/account`の`test`を使う（ブラウザーの中ではログイン中になる）。
  一覧はサーバーもブラウザーもテスト用のAPI（Task 5）が答える。ログイン中のテストは、サーバーにもセッションが見えるように`signIn`でCookieを入れる。
  ブラウザーが送った`/api/problems`のクエリは`page.on('request')`で記録する。

  ```ts
  const signIn = (page: Page) => page.context().addCookies([{ name: 'openoj_access', value: 'valid-access', url: 'http://127.0.0.1:13000' }])
  test('the panel changes the conditions, the URL and the list', async ({ page }) => {
    await signIn(page)
    // requests: the query of each /api/problems call from the browser
    await page.goto('/problems')
    const toggle = page.getByRole('button', { name: /^絞り込み・並び替え/ })
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await expect(page.getByRole('combobox', { name: '並び順' })).toHaveCount(0)
    await toggle.click()
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '正解者数が多い順' })
    await expect(page).toHaveURL('/problems?sort=solvers-desc')
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')          // the panel stays open
    await page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: '未ACのみ' }).click()
    await page.getByRole('combobox', { name: 'コンテスト' }).selectOption({ label: 'コンテストの問題を除く' })
    await page.getByRole('combobox', { name: '難易度の下限' }).selectOption({ label: 'Lv.3' })
    await page.getByRole('combobox', { name: '難易度の上限' }).selectOption({ label: 'Lv.6' })
    await expect(page).toHaveURL('/problems?sort=solvers-desc&solved=unsolved&contest=exclude&difficulty=3-6')
    // the last request carried sort=solvers-desc, solved=unsolved, contest=exclude, difficulty=3-6, offset=0
    // A lower bound above the upper one moves the upper one with it.
    await page.getByRole('combobox', { name: '難易度の下限' }).selectOption({ label: 'Lv.8' })
    await expect(page).toHaveURL(/difficulty=8-8/)
  })
  test('the closed panel shows the conditions as chips', async ({ page }) => {
    await signIn(page)
    // 60 of the fixture's 120 problems are unsolved, so page 2 exists at 50 rows a page.
    await page.goto('/problems?sort=solvers-desc&solved=unsolved&difficulty=3-10&tag=pagination&page=2')
    const toggle = page.getByRole('button', { name: /^絞り込み・並び替え/ })
    await expect(toggle).toContainText('4')
    const chips = page.getByRole('list', { name: '効いている条件' })
    await expect(chips.getByRole('listitem')).toHaveText([/正解者数が多い順/, /未ACのみ/, /難易度 Lv\.3〜10/, /タグ: pagination/])
    await chips.getByRole('button', { name: '難易度 Lv.3〜10を外す' }).click()
    await expect(page).toHaveURL('/problems?sort=solvers-desc&solved=unsolved&tag=pagination')   // back on page 1
    await toggle.click()
    await expect(chips).toHaveCount(0)                                                             // the panel says the same
    await page.getByRole('button', { name: '条件をすべて外す' }).click()
    await expect(page).toHaveURL('/problems')
    await expect(toggle).not.toContainText(/\d/)
  })
  test('the back button returns to the earlier conditions', async ({ page }) => {   // Review Focus 4
    await page.goto('/problems')
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '公開日が古い順' })
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '難易度が高い順' })
    await page.goBack()
    await expect(page).toHaveURL('/problems?sort=oldest')
    await expect(page.getByRole('combobox', { name: '並び順' })).toHaveValue('oldest')
    // the last request carried sort=oldest
  })
  test('a signed-out viewer cannot filter by solved', async ({ page }) => {
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
    await page.goto('/problems?solved=unsolved&contest=exclude')                                   // no session: the route answers solved: ''
    await expect(page.getByRole('list', { name: '効いている条件' }).getByRole('listitem')).toHaveText([/コンテストの問題を除く/])
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    await expect(page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: '未ACのみ' })).toBeDisabled()
    await expect(page.getByRole('link', { name: 'ログイン' }).last()).toHaveAttribute('href', /^\/login\?next=/)
    await expect(page.getByText('ログインすると使えます')).toBeVisible()
  })
  test('an empty result offers to clear the conditions, and rows keep them on a tag', async ({ page }) => {
    await page.goto('/problems?difficulty=1-9')                 // the fixture problem has no difficulty
    await expect(page.getByText('条件に合う問題はありません。')).toBeVisible()
    await page.getByRole('button', { name: '条件をすべて外す' }).click()
    await expect(page).toHaveURL('/problems')
    await page.goto('/problems?sort=oldest')
    // turn タグを表示 on, the way catalogue.spec.ts does
    await page.getByRole('table').getByRole('link', { name: 'DP', exact: true }).click()
    await expect(page).toHaveURL('/problems?sort=oldest&tag=DP')
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://judge.example/problems?tag=DP')
  })
  test('the conditions fit a phone and work without JavaScript', async ({ page, browser }) => {
    await page.setViewportSize({ width: 375, height: 812 })
    await page.goto('/problems?sort=solvers-desc&contest=exclude&difficulty=3-6')
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    const context = await browser.newContext({ javaScriptEnabled: false })
    const plain = await context.newPage()
    await plain.goto('/problems?tag=pagination&sort=oldest')
    await expect(plain.getByRole('table').locator('tbody tr').first()).toContainText('問題 120')
    await expect(plain.getByRole('list', { name: '効いている条件' })).toContainText('公開日が古い順')
    await context.close()
  })
  ```

  `catalogue.spec.ts`の「the problem list narrows to contest problems or to the rest」は、選択欄の名前を「コンテスト」に、開く操作と、空のときの文言（「条件に合う問題はありません。」）を新しい画面に合わせて直す。確かめている内容（URL、再読み込み、タグとの併用、`contest=yes`）は減らさない。

- [ ] **Step 2: テストが失敗することを確かめる**

  Run: `cd web && bun run build && bun run test tests/problem-list-conditions.spec.ts --output=<一時フォルダ>/pw --reporter=line`
  Expected: FAIL（「絞り込み・並び替え」のボタンがない）

- [ ] **Step 3: `useProblemConditions`を実装する**

  `conditions`は`parseConditions(route.query)`。`change`は今の条件に`patch`を重ね、`navigateTo({ path: '/problems', query: conditionQuery(next) })`で移る（履歴に積む。`page`は付けない）。
  難易度は、下限が上限を超えたら上限を下限に、上限が下限を下回ったら下限を上限に合わせる。

- [ ] **Step 4: `ProblemConditions.vue`を実装する**

  - ボタンは`aria-expanded`と`aria-controls`を持ち、札の数（`conditionChips`の長さ）を出す。0のときは数を出さない。
  - 閉じているときだけ、札を`<ul aria-label="効いている条件">`で並べる。
  - 並び順とコンテストは`FilterSelect`、解答状況は`StatusFilter`（`label="解答状況"`、選択肢は「未ACのみ」「AC済みのみ」）を使う。見た目は今の部品のままでよい。
  - `signedIn`でなければ、解答状況の`StatusFilter`を`disabled`にし、「ログインすると使えます」と`/login?next=<今のURL>`へのリンクを添える。
  - タグの欄は今の`TagFilter`をパネルの中に置く。出す条件は今と同じ（`showTags || tag`）。`keep`には、タグを除いた`conditionQuery`を渡す。
  - 色と余白は`main.css`の変数を使い、スマホ幅（39.999rem以下）では欄を1列にする。

- [ ] **Step 5: `index.vue`を組み替える**

  - `definePageMeta`の`key`を外す。クエリが変わってもページを作り直さず、パネルの開閉（`ref(false)`）を保つ。
  - `tag`と`contest`を一度だけ読んでいる所を、`conditions`から計算する形にする。`useFetch`のキーは固定にし、クエリは`conditionQuery(conditions)`から`solved`を含めて作る。見出し、説明、正規URL、空のときの文言も計算にする。
  - 今の「コンテストの問題で絞り込む」の選択欄と、パネルの外の`TagFilter`を外し、`ProblemConditions`を置く。「タグを表示」のスイッチと注意書きは今の位置に残す。
  - 札と欄に出す解答状況は、応答の`solved`（実際に当たったもの）で決める。
  - 空のとき: 札が1つでもあれば「条件に合う問題はありません。」と「条件をすべて外す」のボタン。条件がタグだけのときと、条件がないときは、今の文言のまま。
  - 行のタグ（`TagLinks`）に`keep`を渡し、今の条件を保つ。問題ページの`TagLinks`は変えない。
  - ページ送り、表示件数、AC済みの行の色、失敗時の再試行は、今の動きを変えない。

- [ ] **Step 6: テストが通ることを確かめる**

  Run: `cd web && bun run typecheck && bun run build && bun run test tests/problem-list-conditions.spec.ts tests/catalogue.spec.ts tests/list-cards.spec.ts tests/problem.spec.ts --output=<一時フォルダ>/pw --reporter=line`
  Expected: PASS

- [ ] **Step 7: 目で確かめる**

  PC幅とスマホ幅で、閉じた状態（札が3つ）と開いた状態のスクリーンショットを撮り、札が折り返してもボタンやスイッチと重ならないことを見る。ダークテーマでも見る。

- [ ] **Step 8: コミット**

  `feat: 問題一覧に、並び替えと絞り込みのパネルを足す`

---

### Task 7: 前回の条件を覚える

**Files:**
- Modify: `web/app/composables/useProblemConditions.ts`
- Modify: `web/app/pages/problems/index.vue`
- Test: `web/tests/problem-list-conditions.spec.ts`

**Interfaces:**
- Consumes: Task 6の`useProblemConditions`
- Produces: `useProblemConditions()`に足す。

  ```ts
  // On a /problems without any query, replaces the URL with the remembered conditions. Resolves to nothing when there is none.
  restore: () => Promise<unknown> | undefined
  ```

- [ ] **Step 1: 失敗するテストを書く**

  ```ts
  const remembered = async (page: Page) => JSON.parse(decodeURIComponent((await page.context().cookies()).find(c => c.name === 'openoj-problem-conditions')?.value ?? 'null'))
  test('the list remembers the conditions the viewer chose', async ({ page }) => {
    await signIn(page)
    await page.goto('/problems')
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '正解者数が多い順' })
    await page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: '未ACのみ' }).click()
    expect(await remembered(page)).toEqual({ sort: 'solvers-desc', solved: 'unsolved' })
    // The server applies them to a bare /problems, without adding to the history.
    const response = await page.request.get('/problems', { maxRedirects: 0 })
    expect([response.status(), response.headers().location]).toEqual([302, '/problems?sort=solvers-desc&solved=unsolved'])
    // So does a move inside the app.
    await page.goto('/contests')
    await page.getByRole('navigation').first().getByRole('link', { name: '問題', exact: true }).click()
    await expect(page).toHaveURL('/problems?sort=solvers-desc&solved=unsolved')
    await page.goBack()
    await expect(page).toHaveURL('/contests')
  })
  test('a URL with a query opens as it is', async ({ page }) => {
    await page.context().addCookies([{ name: 'openoj-problem-conditions', value: encodeURIComponent(JSON.stringify({ sort: 'oldest', contest: 'only' })), url: 'http://127.0.0.1:13000' }])
    // Review Focus 5: a page number alone is a query too. The fixture's list has one page, and a page past the end is
    // replaced by the last one, so the test asks for page 1.
    for (const path of ['/problems?tag=DP', '/problems?contest=exclude', '/problems?page=1']) {
      await page.goto(path)
      await expect(page).toHaveURL(path)
    }
    // Opening them did not rewrite what is remembered.
    expect(await remembered(page)).toEqual({ sort: 'oldest', contest: 'only' })
    // Taking the tag away leaves no query, so the remembered conditions come back.
    await page.goto('/problems?tag=DP')
    await page.getByRole('list', { name: '効いている条件' }).getByRole('button', { name: 'タグ: DPを外す' }).click()
    await expect(page).toHaveURL('/problems?sort=oldest&contest=only')
    // A condition chosen on a tag's list replaces what was remembered.
    await page.goto('/problems?tag=DP')
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '難易度が低い順' })
    expect(await remembered(page)).toEqual({ sort: 'difficulty-asc' })
  })
  test('clearing forgets, and a broken cookie reads as nothing', async ({ page }) => {   // Review Focus 1
    await page.context().addCookies([{ name: 'openoj-problem-conditions', value: encodeURIComponent(JSON.stringify({ sort: 'oldest' })), url: 'http://127.0.0.1:13000' }])
    await page.goto('/problems')
    await expect(page).toHaveURL('/problems?sort=oldest')
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    await page.getByRole('button', { name: '条件をすべて外す' }).click()
    await expect(page).toHaveURL('/problems')
    expect(await remembered(page)).toBeNull()
    for (const value of ['%7Bbroken', encodeURIComponent('"oldest"'), encodeURIComponent(JSON.stringify({ sort: 'popular', difficulty: '9-2' }))]) {
      await page.context().addCookies([{ name: 'openoj-problem-conditions', value, url: 'http://127.0.0.1:13000' }])
      const response = await page.request.get('/problems', { maxRedirects: 0 })
      expect(response.status()).toBe(200)
    }
  })
  test('a signed-out viewer keeps a remembered solved filter without it taking effect', async ({ page }) => {
    await page.route('**/api/auth/me', route => route.fulfill({ json: { user: null } }))
    await page.context().addCookies([{ name: 'openoj-problem-conditions', value: encodeURIComponent(JSON.stringify({ solved: 'unsolved', sort: 'oldest' })), url: 'http://127.0.0.1:13000' }])
    await page.goto('/problems')
    await expect(page.getByRole('list', { name: '効いている条件' }).getByRole('listitem')).toHaveText([/公開日が古い順/])
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    await page.getByRole('combobox', { name: 'コンテスト' }).selectOption({ label: 'コンテストの問題のみ' })
    expect(await remembered(page)).toEqual({ sort: 'oldest', solved: 'unsolved', contest: 'only' })
  })
  ```

- [ ] **Step 2: テストが失敗することを確かめる**

  Run: `cd web && bun run build && bun run test tests/problem-list-conditions.spec.ts -g 'remember|as it is|forgets|signed-out viewer keeps' --output=<一時フォルダ>/pw --reporter=line`
  Expected: FAIL（Cookieが書かれない）

- [ ] **Step 3: 実装する**

  - Cookieは`useCookie`で持ち、読むときは必ず`parseConditions`を通す。書くのは`rememberedQuery`の形で、空なら`null`（Cookieを消す）。
  - `change`は、`patch`に`sort`、`solved`、`contest`、`min`、`max`のどれかがあるときだけCookieを書く。`tag`だけの`patch`では書かない。
  - 未ログインでは、URLの`solved`は効かないが条件としては持ち続ける。URLにも残り、Cookieを書くときも消えない。
  - `clear`はCookieを消してから`/problems`へ移る。
  - `restore`は、`route.query`が空で、覚えた条件があるときだけ、`navigateTo({ path: '/problems', query }, { replace: true, redirectCode: 302 })`を返す。
  - `index.vue`は、セットアップで`await restore()`を呼び、ページを作り直さない移動でも当たるように、ルートの変化でも呼ぶ。`TagFilter`の「絞り込みを解除」でクエリが空になったときも同じ道を通る。

- [ ] **Step 4: テストが通ることを確かめる**

  Run: `cd web && bun run build && bun run test tests/problem-list-conditions.spec.ts tests/catalogue.spec.ts tests/session.spec.ts --output=<一時フォルダ>/pw --reporter=line`
  Expected: PASS

- [ ] **Step 5: コミット**

  `feat: 問題一覧で選んだ条件を覚え、次に開いたときも当てる`

---

### Task 8: 実DBの通しテストと文書

**Files:**
- Modify: `api/internal/httpapi/browser_fixture_test.go`（テスト専用の入口を1つ足す）
- Create: `web/tests/account/problem-list.spec.ts`
- Create: `docs/adr/0045-sort-and-filter-problem-list.md`（番号は、書く時点の次の番号にする）
- Modify: `docs/adr/README.md`、`web/README.md`
- Modify: `docs/superpowers/specs/2026-11-28-problem-list-sort-filter-design.md`（状態の行）

**Interfaces:**
- Consumes: Task 1〜7の全部
- Produces: `POST /__test/problems/{id}/accepted`。本文`{ "owner": "bob" }`（アカウント系のテストの利用者名）で、その利用者の公開のACの提出を1件入れる。既存の`POST /__test/contests/{id}/schedule`と同じ書き方にし、`OPENOJ_BROWSER_TEST=1`のときだけ生える。

- [ ] **Step 1: 失敗するテストを書く**

  ```ts
  test('a solved problem leaves the unsolved list and shows in the solved one', async ({ page }) => {
    // alice publishes two problems, 未ACの問題 and AC済みの問題, through the editor or the API as the other account specs do
    // bob signs in; the fixture records bob's accepted run on AC済みの問題 (POST /__test/problems/{id}/accepted)
    await page.goto('/problems')
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    await page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: '未ACのみ' }).click()
    await expect(page).toHaveURL('/problems?solved=unsolved')
    const table = page.getByRole('table')
    await expect(table.getByRole('link', { name: '未ACの問題', exact: true })).toBeVisible()
    await expect(table.getByRole('link', { name: 'AC済みの問題', exact: true })).toHaveCount(0)
    await page.getByRole('group', { name: '解答状況' }).getByRole('button', { name: 'AC済みのみ' }).click()
    await expect(table.getByRole('link', { name: /^AC済みの問題/ })).toBeVisible()
    await expect(table.locator('tr.solved')).toHaveCount(await table.locator('tbody tr').count())   // every listed row is green
    // A reload renders the same list on the server.
    await page.reload()
    await expect(table.getByRole('link', { name: '未ACの問題', exact: true })).toHaveCount(0)
  })
  ```

- [ ] **Step 2: テストが失敗することを確かめ、入口を足して通す**

  Run: `cd web && bun run build && bun run test --config playwright.account.config.ts tests/account/problem-list.spec.ts --output=<一時フォルダ>/pw-account --reporter=line`
  Expected: 入口を足す前はFAIL、足したあとはPASS

- [ ] **Step 3: 全体を通す**

  Run: `cd api && go test -race ./... -count=1`、`cd web && bun run test --output=<一時フォルダ>/pw --reporter=line`、`cd web && bun run test --config playwright.account.config.ts --output=<一時フォルダ>/pw-account --reporter=line`
  Expected: すべてPASS。問題一覧の移動先やクエリを前提にしたテストがほかにないか、`web/tests`を`/problems?`と`コンテストの問題で絞り込む`で検索して確かめる。

- [ ] **Step 4: 文書を書く**

  - ADR: 背景（#35）、決めたこと（1つのパネルに畳む、条件はURL、Cookieで覚える、クエリのない`/problems`だけ置き換える、解答状況はログイン中の入口）、退けた案（外に出す形、列見出しで並び替える形、アカウントに保存する形）、Task 3で測った数字。`docs/adr/README.md`の一覧に足す。
  - `web/README.md`の「問題と記事の公開」の節に、並び替えと絞り込み、覚え方を足す。
  - 設計書の「状態」の行を、実装済みに直す。

- [ ] **Step 5: コミット**

  `test: 問題一覧の解答状況の絞り込みを、実DBで通して確かめる`と、`docs: 問題一覧の並び替えと絞り込みをADRとREADMEに書く`の2つに分ける。

---

## 公開したあと（この計画の外）

- mainへ入れてpushすると`Deploy dev`がAPI、Webの順に配布する。
- 本番で、並び順と絞り込みを切り替えて一覧の応答時間を確かめる。
- Issue #35 に返信して閉じる。
