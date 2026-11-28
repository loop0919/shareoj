# 提出一覧の並び替えと絞り込み Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 問題ページの「すべての提出」「自分の提出」と、コンテストの「提出一覧」「自分の提出」で、並び順を選び、結果・言語・提出者（コンテストでは問題も）で絞り込めるようにする（Issue #36）。

**Architecture:** 並び替えと絞り込みは、今の「誰に何が見えるか」のSQLの条件に足す形でAPIが行う。実行時間と結果は採点結果のJSONの中にしかないので、取り出す式を1か所に決め、本番に近い大きさのデータで早い段階に速さを測って、要る索引を足す。言語と処理系の対応はAPIの処理系の一覧に持たせ、Webはそれを読む。画面は問題一覧（ADR 0045）のパネルの外枠を部品に取り出して共用し、条件はURLのクエリだけを元にする。

**Tech Stack:** Go（pgx、`api/internal/httpapi`、`api/internal/submissions`）、PostgreSQL 17、Nuxt 4（Vue 3、zod）、Playwright。ライブラリは足さない。

**Spec:** [docs/superpowers/specs/2026-11-28-submission-list-sort-filter-design.md](../specs/2026-11-28-submission-list-sort-filter-design.md)。各タスクの前に、対応する節を読む。

## Global Constraints

- クエリの名前と値（この計画で決めた。APIもWebも同じ）。初期値はURLに出さない。

  | クエリ | 値 | 初期 |
  | --- | --- | --- |
  | `sort` | `newest` `oldest` `shortest` `fastest` | `newest` |
  | `verdict` | `AC` `WA` `TLE` `MLE` `OLE` `RE` `CE` `JE` | なし |
  | `language` | 言語のキー（`c` `cpp` `python` `java` `javascript` `typescript` `ruby` など） | なし |
  | `runtime` | 処理系のID。接尾辞（`-isolate`、`-local`）は付けない。例 `cpp23-gcc` | なし |
  | `user` | ユーザーID | なし |
  | `problem` | 今のまま（Webのページは問題の記号、APIはUUID） | なし |

- `language`と`runtime`は、どちらか一方だけを指定する。両方あればAPIは400を返す。Webは両方あれば`runtime`を読む。
- APIは、知らない値や形の合わない値に`400 invalid_request`を返す（今の`mine`、`scope`、`problem`と同じ扱い）。
- Webは、URLの知らない値を初期値として読む。
- 並びの決まり。どの並び順も、最後は提出日時が新しい順、次にIDの降順で並べる（`oldest`だけは提出日時が古い順、IDの昇順）。
  - `shortest`: コードのバイト数（一覧の「コード長」の列の値）が小さい順。
  - `fastest`: 一覧の「実行時間」の列の値（ケースごとの`cpuTimeMs`の最大）が小さい順。値のない提出は最後。
- `verdict`は、採点が終わった提出（`status='DONE'`）の`result.verdict`で絞る。採点中の提出は、`verdict`を指定した一覧には出ない。部分点の問題でも、得点ではなく`result.verdict`で絞る（一覧の「結果」の列と同じ）。
- `language`は、その言語のどの処理系の提出も含む。今は提出に選べない処理系も含む。保存されている処理系のIDに接尾辞が付いていても、同じ処理系として扱う。
- `user`は、提出した本人のユーザーIDの完全一致。存在しないユーザーIDなら0件。Webは「自分の提出」「チームの提出」では`user`を送らない。
- 誰に何が見えるかの決まり（`problemSubmissionVisible`、`contestPublic`、`contestOwn`、`problemSubmissionsAccess`）は変えない。条件はその中だけで効く。コンテストの`total`は条件を当てたあとの数。
- 処理系が2つ以上ある言語は、C（`c`）、C++（`cpp`）、Python（`python`）、Java（`java`）、JavaScript（`javascript`）、TypeScript（`typescript`）、Ruby（`ruby`）。CとC++、JavaScriptとTypeScriptは別の言語。
- どの処理系がどの言語に入るかは、APIの処理系の一覧（`api/internal/submissions/runtimes.go`）だけに書く。Webはそれを`GET /runtimes`から読む。
- 画面の文言。
  - ボタン: 「絞り込み・並び替え」
  - 欄: 「並び順」「結果」「言語」「提出者」「問題」
  - 並び順: 「提出日時が新しい順」「提出日時が古い順」「コード長が短い順」「実行時間が短い順」
  - 結果: 「すべて」と、AC、WA、TLE、MLE、OLE、RE、CE、JE
  - 言語: 「すべて」、言語のまとめは「C++（すべて）」の形、処理系は一覧の「言語」の列と同じ表示
  - 提出者: 入力欄の名前は「提出者」、入れる例は「ユーザーID」
  - 札: 並び順はその文言、「結果: AC」「言語: C++」「言語: C++23 (GCC)」「提出者: alice」「問題: A」。×の名前は「（札の文言）を外す」
  - 「条件をすべて外す」「条件に合う提出はありません。」
- 条件は覚えない。Cookieは足さない。
- コンテストの「オンサイトのみ表示」のスイッチはパネルの外に残す。札にせず、数えず、「条件をすべて外す」でも変えない。
- 一覧の自動更新（15秒ごと、リジャッジ中は3秒ごと）は、今の条件のまま動かす。更新でパネルが閉じたり、選んだ行が外れたりしない。
- 配信モード（ADR 0042）で閉じている一覧は、今のまま閉じる。
- データベースは、索引と関数を足すことはあっても、表の形は変えない。表の形を変える必要が出たら（たとえば結果や実行時間を列に持つ）、実装を止めて利用者に相談する。
- 速さは、提出の`job`と`result`を本番と同じ数KBにしたデータで測る。小さい行で測らない。
- コメントは英語で、既存のファイルの書き方に合わせる。
- コミットは`prefix: 対応内容`の形式で、タスクごとに行う。末尾に、作業しているモデルのCo-Authored-Byの行を付ける。
- このリポジトリは公開されている。鍵やリソースのIDを書かない。直していない弱点を、追跡対象のファイルに書かない。
- ブランチ`feat/submission-list-conditions`で作業する。pushはしない（mainへのpushは自動で配布される）。
- マイグレーションの番号は、作業を始める時点のmainの次の番号にする。この計画では051と書く。

### テストの流し方

コマンドは`nix develop`の中から実行する。ローカルのDBは動いていればそのまま使う。

```bash
export TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable'
cd api && go test ./internal/httpapi/ -run TestSubmissionList -count=1
cd web && bun run typecheck && bun run build
cd web && bun run test tests/submission-list-conditions.spec.ts --output=<一時フォルダ>/pw --reporter=line
cd web && bun run test --config playwright.account.config.ts tests/account/submission-list.spec.ts --output=<一時フォルダ>/pw-account --reporter=line
```

- Playwrightは`.output`のビルドに対して走る。Webのアプリやサーバーのコードを変えたら、テストの前に`bun run build`をやり直す。
- `web/test-results`は書き込めない。必ず`--output`で別のフォルダを指定する。
- 条件を続けて変えるテストは、`--repeat-each=30 --workers=4`でも通ることを確かめる。

## Review Focus

仕様が暗に求めているが、ふつうに書くとテストから漏れやすいもの。それぞれ、持ち主のタスクにテストを入れてある。

1. 実行時間のない提出（採点中、CE、ケースごとの時間を持たない古い結果やローカル採点の結果）を「実行時間が短い順」で並べたとき。最後に回り、ページを送っても順が変わらない（Task 2）。
2. 保存されている処理系のIDに接尾辞が付いている提出と、今は選べない処理系の提出。言語のまとめでも処理系ごとでも、列で同じ表示になる提出が同じ選択肢で出る（Task 2）。
3. 見えない提出に条件を付けたとき。凍結中の提出、下書きの実行、定期便の公開前の提出は、その提出者や結果を指定しても出ず、コンテストの合計にも数えない（Task 2、Task 3）。
4. 一覧を開いたまま自動更新が走ったときと、条件を数ミリ秒のうちに2回変えたとき。パネル、条件、選んだ行が保たれ、2回目の変更が消えない（Task 7、Task 8）。
5. 「すべての提出」と「自分の提出」を切り替えたときと、コンテストの古いリンク（`?view=submissions&problem=A`）を開いたとき。提出者を除いて条件が保たれ、「自分の提出」ではURLの提出者が効かない（Task 7、Task 8）。

---

### Task 1: 処理系の一覧に言語を持たせる

**Files:**
- Modify: `api/internal/submissions/runtimes.go`
- Modify: `api/internal/httpapi/submissions.go`（`runtimes`の応答）、`api/internal/httpapi/docs.go`（`runtimesResponse`があれば）
- Modify: `api/README.md`
- Test: `api/internal/submissions/runtimes_test.go`（なければ新規）、`api/internal/httpapi/`の`GET /runtimes`を確かめている既存のテスト

**Interfaces:**
- Produces（Task 2、Task 5が使う）:

  ```go
  // api/internal/submissions/runtimes.go
  type Runtime struct {
      ID       string `json:"id"`
      Label    string `json:"label"`
      Language string `json:"language"` // the key of its Language
  }
  type Language struct {
      Key      string    `json:"key"`
      Label    string    `json:"label"`
      Runtimes []Runtime `json:"runtimes"` // every runtime of the language, retired ones included, in catalogue order
  }
  // Languages lists every language in catalogue order.
  func Languages() []Language
  // LanguageRuntimeIDs gives the catalogue IDs of a language's runtimes; ok is false for an unknown key.
  func LanguageRuntimeIDs(key string) (ids []string, ok bool)
  // KnownRuntime tells whether id is a catalogue ID (without a suffix).
  func KnownRuntime(id string) bool
  // StoredRuntimeIDs expands catalogue IDs to every form a stored submission may carry (bare, -isolate, -local).
  func StoredRuntimeIDs(ids []string) []string
  ```

  `GET /runtimes`の応答に`languages`（`[]Language`）を足す。今の`items`と`maintenance`は変えない。`languages`は、メンテナンス中でも、今は選べない処理系を含めて全部返す。

- [ ] **Step 1: 失敗するテストを書く**

  ```go
  func TestLanguagesGroupRuntimes(t *testing.T) {
      // every catalogue runtime belongs to exactly one language, and Languages() lists each runtime once
      // the languages with more than one runtime are exactly: c, cpp, python, java, javascript, typescript, ruby
      // LanguageRuntimeIDs("cpp") == [cpp17, cpp23-gcc, cpp23-clang] in catalogue order (check the real order and IDs)
      // LanguageRuntimeIDs("python") holds python314, pypy311, codon020
      // c and cpp are different languages; javascript and typescript are different languages
      // LanguageRuntimeIDs("cobol") -> ok false; KnownRuntime("cpp23-gcc") true; KnownRuntime("cpp23-gcc-isolate") false
      // StoredRuntimeIDs([]string{"cpp17"}) == ["cpp17", "cpp17-isolate", "cpp17-local"]
      // a language with one runtime has the runtime's language label, e.g. rust2024 -> key "rust", label "Rust"
  }
  ```

  HTTPのテスト: `GET /runtimes`の`languages`に`cpp`があり、その`runtimes`に今は選べない`cpp17`が入っている。メンテナンス中（`items`が空）でも`languages`は空でない。

- [ ] **Step 2: テストが失敗することを確かめる**

  Run: `cd api && go test ./internal/submissions/ -run TestLanguagesGroupRuntimes -count=1`
  Expected: FAIL（`Languages`がない）

- [ ] **Step 3: 実装する**

  言語のキーと表示名は`runtimes.go`の中の1つの表に書く。`Runtime`の各行に言語のキーを足す。言語の表示名は処理系の表示名の先頭に合わせる（`C++`、`Python`、`Common Lisp`など）。

- [ ] **Step 4: テストが通ることを確かめ、`api/README.md`の`GET /runtimes`の説明に`languages`を足す**

  Run: `cd api && go vet ./... && go test ./internal/submissions/ ./internal/httpapi/ -count=1`
  Expected: PASS

- [ ] **Step 5: コミット**

  `feat: 処理系の一覧に言語を持たせ、GET /runtimesで返す`

---

### Task 2: 問題の提出一覧の並び順と絞り込み

**Files:**
- Create: `api/internal/database/051_submission_list_conditions.sql`（関数。`database.go`の一覧にも足す）
- Create: `api/internal/submissions/conditions.go`
- Modify: `api/internal/submissions/problem.go`（`ProblemList`）
- Modify: `api/internal/httpapi/problem_submissions.go`、`api/internal/httpapi/docs.go`、`api/internal/httpapi/routes.go`（説明文）
- Test: `api/internal/httpapi/submission_list_test.go`（新規）

**Interfaces:**
- Consumes: Task 1の`LanguageRuntimeIDs`、`KnownRuntime`、`StoredRuntimeIDs`
- Produces（Task 3、Task 4が使う）:

  ```go
  // api/internal/submissions/conditions.go
  // ListConditions narrows and orders a submission list inside what the viewer may see.
  type ListConditions struct {
      Sort     string   // SortNewest, SortOldest, SortShortest or SortFastest; "" reads as SortNewest
      Verdict  string   // "" or one of Verdicts
      Runtimes []string // stored runtime IDs to keep; nil keeps every runtime
      User     string   // a handle; "" keeps every submitter
  }
  const (SortNewest = "newest"; SortOldest = "oldest"; SortShortest = "shortest"; SortFastest = "fastest")
  var Verdicts = []string{"AC", "WA", "TLE", "MLE", "OLE", "RE", "CE", "JE"}
  // where gives the SQL to AND onto a visibility filter and the arguments it adds, numbered from next.
  func (c ListConditions) where(next int) (sql string, args []any)
  // order gives the ORDER BY text; it comes from constants only.
  func (c ListConditions) order() string
  ```

  ```go
  // api/internal/submissions/problem.go — the new signature
  func (s *Store) ProblemList(ctx context.Context, problemID, contestID, viewer string, scope Scope, onsite bool, offset int, conditions ListConditions) (ContestSubmissionList, error)
  ```

  ```go
  // api/internal/httpapi/problem_submissions.go
  // listConditions reads sort, verdict, language, runtime and user; a bad value answers 400 invalid_request.
  func listConditions(w http.ResponseWriter, r *http.Request) (submissions.ListConditions, bool)
  ```

  ```sql
  -- 051: the time a list row shows, the largest cpuTimeMs of the cases; NULL when no case has one.
  CREATE FUNCTION submission_cpu_time_ms(result jsonb) RETURNS double precision LANGUAGE sql IMMUTABLE ...
  ```

- [ ] **Step 1: 失敗するテストを書く**

  `TestSubmissionListConditionsPostgres`。用意は`newTeamTest`（`contest_teams_test.go`）と`run`（`partial_scoring_test.go`）の形を使う。`run`は処理系とコードを決められないので、このテストの中に行を足す小さい関数を書く。
  公開の問題を1つ作り、次の提出を古い順に入れる（提出日時は1分ずつずらす）。

  | 提出 | 提出者 | 保存する処理系 | コード | 状態と結果 |
  | --- | --- | --- | --- | --- |
  | s1 | alice | `cpp17` | 40バイト | DONE、AC、ケースの`cpuTimeMs`は 12 と 30 |
  | s2 | bob | `cpp23-gcc-isolate` | 25バイト | DONE、AC、ケースの`cpuTimeMs`は 5 と 8 |
  | s3 | bob | `python314-isolate` | 25バイト | DONE、WA、ケースの`cpuTimeMs`は 100 |
  | s4 | carol | `pypy311-local` | 300バイト | DONE、TLE、ケースの`cpuTimeMs`は 2000 |
  | s5 | carol | `cpp23-clang-isolate` | 10バイト | DONE、CE、ケースなし |
  | s6 | alice | `python314-isolate` | 60バイト | DONE、AC、`cases`はあるが`cpuTimeMs`なし |
  | s7 | bob | `rust2024-isolate` | 70バイト | QUEUED、結果なし |
  | s8 | alice | `cpp23-gcc-isolate` | 5バイト | DONE、AC、`job`に`"privateDraft":true`（公開の一覧には出ない） |

  ```go
  listed := func(query string) []string // GET /problems/{id}/submissions?query as a guest, the ids in order
  for _, c := range []struct{ query string; want []string }{
      {"", []string{s7, s6, s5, s4, s3, s2, s1}},
      {"sort=oldest", []string{s1, s2, s3, s4, s5, s6, s7}},
      {"sort=shortest", []string{s5, s3, s2, s1, s6, s7, s4}},          // ties (s3, s2): newer first
      {"sort=fastest", []string{s2, s1, s3, s4, s7, s6, s5}},           // no time (s7, s6, s5) last, newer first
      {"verdict=AC", []string{s6, s2, s1}},                             // the judging s7 is not listed
      {"verdict=AC&sort=fastest", []string{s2, s1, s6}},
      {"verdict=AC&sort=shortest", []string{s2, s1, s6}},
      {"language=cpp", []string{s5, s2, s1}},                           // retired cpp17 and suffixed IDs included
      {"language=python", []string{s6, s4, s3}},                        // CPython and PyPy, -isolate and -local
      {"runtime=python314", []string{s6, s3}},
      {"runtime=cpp17", []string{s1}},
      {"user=bob", []string{s7, s3, s2}},
      {"user=bob&verdict=AC&language=cpp", []string{s2}},
      {"user=nobody", []string{}},
      {"user=alice&sort=shortest", []string{s1, s6}},                   // the private draft s8 stays hidden
      {"verdict=CE", []string{s5}},
  } { /* listed(c.query) equals c.want */ }
  for _, bad := range []string{"sort=longest", "verdict=OK", "verdict=ac", "language=cobol", "runtime=cpp23-gcc-isolate", "runtime=nope", "language=cpp&runtime=cpp17", "user=a b", "user=" + strings.Repeat("a", 100)} {
      // GET …?bad answers 400 invalid_request
  }
  // Review Focus 1: with sort=fastest, offsets 0 and 50 together list each of 60 extra no-time rows exactly once
  // (insert 60 more DONE CE rows first, then drop them again or use a second problem).
  // The author sees the draft run with the same conditions: as alice on /my/problems/{id}/submissions?user=alice&sort=shortest -> [s8, s1, s6]
  // mine keeps working with conditions: as bob, ?mine=1&verdict=AC -> [s2]
  // The value sorted by is the value the row shows: for every listed row of sort=fastest, result.cpuTimeMs is
  // non-decreasing until the first row without it, and for sort=shortest sourceBytes is non-decreasing.
  ```

  見え方が変わらないことの確認（Review Focus 3）。`TestContestFreezePostgres`と`TestSubmissionsPostgres`、`featured_test.go`の、見えない提出を確かめている所に、条件つきの呼び出しを1つずつ足す。見えない提出の提出者を`user`に指定しても、その結果を`verdict`に指定しても、出ないことを確かめる。

- [ ] **Step 2: テストが失敗することを確かめる**

  Run: `cd api && go test ./internal/httpapi/ -run TestSubmissionListConditionsPostgres -count=1`
  Expected: FAIL（`sort=oldest`の並びが違う）

- [ ] **Step 3: マイグレーション051と`conditions.go`を実装する**

  - `submission_cpu_time_ms`は、Goの`summarizeUsage`（`store.go`）と同じ値を返す。同じ値になることを、Step 1の「並べた値は行が出す値」の確認が守る。
  - `where`は、`Verdict`があれば`status='DONE'`と`result->>'verdict'`、`Runtimes`があれば`runtime = ANY(...)`、`User`があれば`user_profiles`の`handle`から引いた`owner_id`で絞る。値はすべてプレースホルダーで渡す。
  - `order`は並び順ごとの定数を返す。`shortest`は`octet_length(source)`、`fastest`は`submission_cpu_time_ms(result) ASC NULLS LAST`。

- [ ] **Step 4: `ProblemList`と`listConditions`を実装し、問題の一覧の入口から使う**

  - `listConditions`は、`language`を`LanguageRuntimeIDs`で、`runtime`を`KnownRuntime`で確かめ、`StoredRuntimeIDs`で広げて`Runtimes`に入れる。`user`は今のユーザーIDの決まり（`userHandle`）で確かめる。
  - `ProblemList`のほかの呼び出し元（コンテストの問題の一覧）には、このタスクでは空の`ListConditions{}`を渡す。
  - `docs.go`に5つのクエリの説明を足し、`routes.go`の問題の一覧の説明文「提出日時の降順」を「既定は提出日時の降順」に直す。

- [ ] **Step 5: テストが通ることを確かめる**

  Run: `cd api && go vet ./... && gofmt -l internal && go test ./internal/httpapi/ ./internal/submissions/ ./internal/database/ -count=1`
  Expected: PASS

- [ ] **Step 6: コミット**

  `feat: 問題の提出一覧を、並び替え、結果・言語・提出者で絞り込めるようにする`

---

### Task 3: コンテストの提出一覧の並び順と絞り込み

**Files:**
- Modify: `api/internal/submissions/contest.go`（`ContestList`）
- Modify: `api/internal/httpapi/contests.go`（`contestSubmissions`、`contestProblemSubmissions`）、`docs.go`、`routes.go`（説明文）
- Modify: `api/README.md`
- Test: `api/internal/httpapi/submission_list_test.go`

**Interfaces:**
- Consumes: Task 2の`ListConditions`、`listConditions`、`ProblemList`
- Produces（Task 4、Task 6が使う）:

  ```go
  func (s *Store) ContestList(ctx context.Context, contestID, viewer string, scope Scope, offset, limit int, onsite bool, problemID string, conditions ListConditions) (CountedSubmissionList, error)
  ```

  `GET /contests/{id}/submissions`、`GET /my/contests/{id}/submissions`、`GET /contests/{id}/problems/{problem}/submissions`、その`/my/`の入口が、`sort`、`verdict`、`language`、`runtime`、`user`を受ける。

- [ ] **Step 1: 失敗するテストを書く**

  `TestContestSubmissionListConditionsPostgres`。終わったコンテストに問題を2つ（A、B）入れ、Task 2の表と同じ形の提出を、AとBに分けてコンテストの提出として入れる。凍結のあるコンテストをもう1つ作り、凍結後の提出を1つ入れる。

  ```go
  // page(query, viewer) -> ids in order and total, from /contests/{id}/submissions
  // no condition: every visible submission, newest first, total == len
  // sort=fastest, sort=shortest, sort=oldest: same rules as Task 2 across both problems
  // verdict=AC: only DONE AC rows; total counts only them
  // language=python&problem=<B>: both conditions and the existing problem filter together; total follows
  // user=bob&limit=1&offset=1: the second of bob's rows; total is bob's count (the page size does not change the total)
  // mine=1&verdict=WA as bob (signed in): only bob's WA rows
  // mine=team in a team contest keeps working with sort=shortest (reuse a team fixture from contest_teams_test.go)
  // scope=onsite&verdict=AC: both narrow together
  // bad values answer 400 invalid_request, as in Task 2
  // Review Focus 3, frozen contest, as a guest:
  //   user=<the frozen submitter> -> the frozen row is absent and total does not count it
  //   verdict=<its verdict>&sort=fastest -> absent
  //   the submitter, signed in after the end, sees it with the same conditions (own frozen rows are visible to them)
  // contest-problem list: /contests/{id}/problems/{A}/submissions?verdict=AC&sort=shortest -> A's AC rows, shortest first
  ```

- [ ] **Step 2: テストが失敗することを確かめる**

  Run: `cd api && go test ./internal/httpapi/ -run TestContestSubmissionListConditionsPostgres -count=1`
  Expected: FAIL

- [ ] **Step 3: 実装する**

  - `ContestList`は、一覧の問い合わせと件数の問い合わせの両方に`conditions.where`を足す。プレースホルダーの番号が2つの問い合わせでずれないようにする（今の`$2`の扱いのコメントを読む）。
  - `contestSubmissions`と`contestProblemSubmissions`は`listConditions`を読み、それぞれ`ContestList`と`ProblemList`に渡す。
  - `docs.go`の2つの公開の入口にクエリの説明を足す。説明文の「提出日時の降順」を直す。
  - `api/README.md`に、3種類の一覧が受ける`sort`、`verdict`、`language`、`runtime`、`user`を書く。値、初期値、400、並びが同じときの順、`verdict`と採点中の関係を書く。

- [ ] **Step 4: テストが通ることを確かめる**

  Run: `cd api && go vet ./... && gofmt -l internal && go test -race ./... -count=1`
  Expected: PASS（コンテストの見え方の既存のテストが、変更なしで通る）

- [ ] **Step 5: コミット**

  `feat: コンテストの提出一覧を、並び替え、結果・言語・提出者で絞り込めるようにする`

---

### Task 4: 本番に近いデータで速さを測り、要る索引を足す

**Files:**
- Test: `api/internal/httpapi/submission_list_speed_test.go`（新規）
- Create（要るときだけ）: `api/internal/database/052_submission_list_indexes.sql`と、索引が使われていることを確かめるテスト

**Interfaces:**
- Consumes: Task 2、Task 3の`ProblemList`、`ContestList`

- [ ] **Step 1: 速さのテストを書く**

  `TestSubmissionListSpeedPostgres`。`OPENOJ_LIST_SPEED=1`のときだけ走る。作り方は`TestProblemListSpeedPostgres`（`problem_list_test.go`）にならう。
  - 提出の`job`は2KB以上、`result`は1.5KB以上になるようにし、TOASTされていることを確かめる（同じ確認をする）。`result`の各ケースに`cpuTimeMs`を入れる。
  - 1つの問題にコンテスト外の提出を5000件、1つのコンテスト（問題10個）にコンテストの提出を20000件。提出者200人、処理系8種類（接尾辞つき）、結果はACが半分で残りを分ける。採点中、時間のない結果、下書きの実行、`easyTest`を数%ずつ混ぜる。
  - 値がばらついていることを確かめる（結果の種類、処理系の種類、実行時間の種類の数）。
  - 測るもの: 問題の一覧（`ProblemList`、50件）と、コンテストの一覧（`ContestList`、50件と件数）。それぞれ、4つの並び順 ×（条件なし、`verdict=AC`、言語のまとめ、提出者1人、`verdict=AC`と言語のまとめ）。3回のうちいちばん速い回が200ミリ秒を超えたら失敗。組み合わせごとの時間を`t.Logf`で出す。
  - 条件なし・新しい順の値は「今のコードの速さ」でもある。ブランチの分かれ目のコードでも同じデータで測り、比べられるように報告に書く。

- [ ] **Step 2: 測る**

  Run: `cd api && OPENOJ_LIST_SPEED=1 go test ./internal/httpapi/ -run TestSubmissionListSpeedPostgres -count=1 -v`
  超えた組み合わせについて、`EXPLAIN (ANALYZE, BUFFERS)`を取り、どこに時間がかかっているかを報告に書く。

- [ ] **Step 3: 超えた組み合わせを、索引と問い合わせの直しで収める**

  - まず、いちばん効く索引から足す。候補は、`(problem_id, submission_cpu_time_ms(result))`、`(problem_id, (result->>'verdict'), created_at DESC)`、`(problem_id, runtime, created_at DESC)`、`(problem_id, owner_id, created_at DESC)`と、コンテスト側の同じ形。全部を足すのではなく、測って要るものだけにする。索引を足すたびに測り直す。
  - 索引の式や条件は、問い合わせの式と一字一句そろえる。そろえた所にコメントを書き、マイグレーション050と同じ注意（関数を置き換えるときは索引を作り直す）を書く。
  - 索引を足したら、`TestSolverQueriesUseTheirIndexesPostgres`（`api/internal/problems/publication_plan_test.go`）と同じ形の、いつも走るテストを足す。問い合わせはストアが実際に使う文字列から組み立てる。
  - 今のコードでもすでに200ミリ秒を超えている組み合わせ（条件なしのコンテストの件数など）が見つかったら、それも報告に書く。このタスクで直せるなら直す。
  - **索引では収まらない組み合わせが残ったら、止める。** 状態をBLOCKEDで報告し、測った数字、実行計画、試したこと、収めるのに要るいちばん小さい表の形の変更（たとえば結果と実行時間の生成列）を書く。表の形は変えない。

- [ ] **Step 4: 全体を通してコミット**

  Run: `cd api && go vet ./... && gofmt -l internal && go test -race ./... -count=1`と、Step 2のコマンド
  コミット: `test: 提出一覧の並び替えと絞り込みの速さを、本番に近いデータで測る`。索引を足したら`perf: 提出一覧の並び替えと絞り込みに索引を足す`を別に。

---

### Task 5: Webの条件の型と変換、言語の選択肢

**Files:**
- Create: `web/app/utils/submission-conditions.ts`
- Modify: `web/shared/types/`の処理系の一覧の型（`GET /api/runtimes`の応答に`languages`を足す）、`web/server/api/runtimes*.ts`（`languages`を通す）
- Modify: `web/tests/fixtures/public-api.mjs`（`/runtimes`が`languages`を返す）
- Test: `web/tests/submission-conditions.spec.ts`（新規、関数を直接呼ぶ）

**Interfaces:**
- Consumes: Task 1の`languages`
- Produces（Task 6〜9が使う）:

  ```ts
  export const submissionSorts: readonly { key: SubmissionSort, label: string }[]
  export type SubmissionSort = 'newest' | 'oldest' | 'shortest' | 'fastest'
  export const submissionVerdicts: readonly string[]   // AC, WA, TLE, MLE, OLE, RE, CE, JE
  export type RuntimeLanguage = { key: string, label: string, runtimes: { id: string, label: string }[] }
  export type SubmissionConditions = { sort: SubmissionSort, verdict: string, language: string, runtime: string, user: string, problem: string }
  export type SubmissionConditionKey = 'sort' | 'verdict' | 'language' | 'user' | 'problem'
  export const defaultSubmissionConditions: SubmissionConditions
  // Reads a route query. Unknown values read as the initial ones; with both language and runtime, runtime wins.
  // problem is kept as written (a contest's letter); the page resolves it.
  export function parseSubmissionConditions(query: unknown): SubmissionConditions
  // The query of a list with these conditions; initial values are left out.
  export function submissionConditionQuery(conditions: SubmissionConditions): Record<string, string>
  // The options of the 言語 select: a group per language, with 「（言語名）（すべて）」 first when it has two runtimes or more.
  export function languageOptions(languages: RuntimeLanguage[]): { label: string, options: { value: string, label: string }[] }[]
  // value is 'language:cpp' or 'runtime:cpp23-gcc'; '' is すべて.
  export function languageValue(conditions: SubmissionConditions): string
  export function languagePatch(value: string): Pick<SubmissionConditions, 'language' | 'runtime'>
  // The conditions in force, in the order sort, verdict, language, user, problem.
  export function submissionConditionChips(conditions: SubmissionConditions, languages: RuntimeLanguage[]): { key: SubmissionConditionKey, label: string }[]
  // The patch that puts one condition back to its initial value ('language' resets language and runtime).
  export function submissionConditionReset(key: SubmissionConditionKey): Partial<SubmissionConditions>
  ```

- [ ] **Step 1: 失敗するテストを書く**

  ```ts
  const languages = [
    { key: 'cpp', label: 'C++', runtimes: [{ id: 'cpp17', label: 'C++17 (GCC)' }, { id: 'cpp23-gcc', label: 'C++23 (GCC)' }] },
    { key: 'rust', label: 'Rust', runtimes: [{ id: 'rust2024', label: 'Rust (Edition 2024)' }] },
  ]
  test('conditions read from a query and write back without initial values', () => {
    expect(parseSubmissionConditions({})).toEqual(defaultSubmissionConditions)
    const all = parseSubmissionConditions({ sort: 'fastest', verdict: 'AC', language: 'cpp', user: 'alice', problem: 'A', view: 'submissions', page: '2' })
    expect(all).toEqual({ sort: 'fastest', verdict: 'AC', language: 'cpp', runtime: '', user: 'alice', problem: 'A' })
    expect(submissionConditionQuery(all)).toEqual({ sort: 'fastest', verdict: 'AC', language: 'cpp', user: 'alice', problem: 'A' })
    expect(submissionConditionQuery(defaultSubmissionConditions)).toEqual({})
    expect(parseSubmissionConditions({ language: 'cpp', runtime: 'cpp17' })).toMatchObject({ language: '', runtime: 'cpp17' })
  })
  test('unknown values read as the initial ones', () => {
    for (const bad of [{ sort: 'longest' }, { sort: ['fastest', 'newest'] }, { verdict: 'ac' }, { verdict: 'OK' }, { user: ['a', 'b'] }, { user: 'a b' }, { language: 'C++' }, { runtime: 'cpp23-gcc-isolate' }])
      expect(parseSubmissionConditions(bad)).toEqual(defaultSubmissionConditions)
    for (const broken of [null, undefined, 'sort=fastest', 42, []]) expect(parseSubmissionConditions(broken)).toEqual(defaultSubmissionConditions)
  })
  test('the language select groups runtimes and offers the whole language', () => {
    expect(languageOptions(languages)).toEqual([
      { label: 'C++', options: [{ value: 'language:cpp', label: 'C++（すべて）' }, { value: 'runtime:cpp17', label: 'C++17 (GCC)' }, { value: 'runtime:cpp23-gcc', label: 'C++23 (GCC)' }] },
      { label: 'Rust', options: [{ value: 'runtime:rust2024', label: 'Rust (Edition 2024)' }] },
    ])
    expect(languageValue(parseSubmissionConditions({ language: 'cpp' }))).toBe('language:cpp')
    expect(languageValue(parseSubmissionConditions({ runtime: 'cpp17' }))).toBe('runtime:cpp17')
    expect(languagePatch('language:cpp')).toEqual({ language: 'cpp', runtime: '' })
    expect(languagePatch('')).toEqual({ language: '', runtime: '' })
  })
  test('chips name the conditions in force', () => {
    const all = parseSubmissionConditions({ sort: 'shortest', verdict: 'AC', runtime: 'cpp23-gcc', user: 'alice', problem: 'A' })
    expect(submissionConditionChips(all, languages).map(chip => chip.label)).toEqual(['コード長が短い順', '結果: AC', '言語: C++23 (GCC)', '提出者: alice', '問題: A'])
    expect(submissionConditionChips(parseSubmissionConditions({ language: 'cpp' }), languages)[0].label).toBe('言語: C++')
    // A language the catalogue does not know yet (the list has not loaded) still shows a chip, by its key.
    expect(submissionConditionChips(parseSubmissionConditions({ language: 'cpp' }), [])[0].label).toBe('言語: cpp')
    expect(submissionConditionChips(defaultSubmissionConditions, languages)).toEqual([])
    expect(submissionConditionReset('language')).toEqual({ language: '', runtime: '' })
  })
  ```

  `parseSubmissionConditions`は言語の一覧を持たないので、`language`と`runtime`は形（英小文字・数字・ハイフン、接尾辞なし）だけを見る。中身が正しいかはAPIが見る。`user`はユーザーIDの形（今の決まりを`web/shared`から探して使う）を見る。

- [ ] **Step 2: テストが失敗することを確かめる** — Run: `cd web && bun run test tests/submission-conditions.spec.ts --output=<一時フォルダ>/pw --reporter=line`、Expected: FAIL（モジュールがない）

- [ ] **Step 3: 実装する。** Nuxtの自動インポートを使わない。`/api/runtimes`の型と経路に`languages`を通し、テスト用のAPIの`/runtimes`にも足す（`languageOptions`のテストの2言語に、Python（CPython、PyPy）を足した3言語でよい）。

- [ ] **Step 4: テストが通ることを確かめる** — Step 2のコマンドと`bun run typecheck`、`bun run build`、`bun run test tests/submission.spec.ts`（処理系の表示を確かめている既存のテスト）。Expected: PASS

- [ ] **Step 5: コミット** — `feat: 提出一覧の条件を、URLのクエリと相互に変換する`

---

### Task 6: Webサーバーの経路が条件をAPIへ渡す

**Files:**
- Modify: `web/server/utils/problem-submissions.ts`
- Modify: `web/server/api/contests/[...path].get.ts`
- Modify: `web/tests/contest-scope.spec.ts`（渡すクエリを確かめているテスト）
- Test: `web/tests/submission-list-conditions.spec.ts`（新規）

**Interfaces:**
- Consumes: Task 2、Task 3のAPI
- Produces（Task 8、Task 9が使う）: `/api/problems/{id}/submissions`と`/api/contests/{id}/submissions`（と問題別の一覧）が、`sort`、`verdict`、`language`、`runtime`、`user`をAPIへ渡す。

- [ ] **Step 1: 失敗するテストを書く**

  コンテストの経路は、`contest-scope.spec.ts`が使っている、届いたクエリをそのまま返すテスト用のコンテストで確かめる。

  ```ts
  test('the contest list route passes the conditions on', async ({ request }) => {
    // GET /api/contests/<echo contest>/submissions?sort=fastest&verdict=AC&language=cpp&user=alice&problem=<uuid>&limit=20
    // -> the echoed query holds exactly offset=0, mine=0, limit=20, problem, sort=fastest, verdict=AC, language=cpp, user=alice
    // runtime=cpp23-gcc is passed in place of language
    // values that cannot be valid are not passed on: sort=longest, verdict=<script>, user=a b, language=C++ -> absent from the echo
    // unknown keys are still dropped: evil=1 -> absent
  })
  test('the problem list route passes the conditions on', async ({ request }) => {
    // the fixture API answers the list of one dedicated fixture problem (a new UUID) with the query it received
    // -> offset, mine, sort, verdict, language/runtime, user arrive; nothing else (evil=1 is dropped)
    // with a session cookie the private path receives the same conditions
  })
  ```

  `contest-scope.spec.ts`の、渡すクエリの文字列を確かめている既存のテストは、条件がないときに今と同じ文字列になることを確かめ続ける。

- [ ] **Step 2: テストが失敗することを確かめる** — Run: `cd web && bun run build && bun run test tests/submission-list-conditions.spec.ts tests/contest-scope.spec.ts --output=<一時フォルダ>/pw --reporter=line`、Expected: 新しいテストがFAIL

- [ ] **Step 3: 実装する。** 2つの経路で、5つのクエリを、形が正しいときだけ渡す（`sort`と`verdict`は一覧から、`language`と`runtime`と`user`は形）。値の一覧と形の確認は`web/shared`に置き、Task 5の`parseSubmissionConditions`と同じものを使う。公開と非公開の呼び分けは変えない。配信モードの扱いも変えない。テスト用のAPIに、専用の問題IDの一覧だけ、届いたクエリを返す分岐を足す。

- [ ] **Step 4: テストが通ることを確かめる** — Step 2のコマンドに`tests/streaming-server.spec.ts tests/contest-submissions.spec.ts tests/rejudge.spec.ts`を足す。Expected: PASS

- [ ] **Step 5: コミット** — `feat: 提出一覧の経路で、並び順と絞り込みの条件をAPIへ渡す`

---

### Task 7: パネルの外枠を部品に取り出し、提出一覧のパネルを問題ページに入れる

**Files:**
- Create: `web/app/components/ConditionsPanel.vue`（外枠）
- Create: `web/app/components/SubmissionConditions.vue`
- Create: `web/app/components/UserFilter.vue`
- Create: `web/app/composables/useConditionChanges.ts`、`web/app/composables/useSubmissionConditions.ts`
- Modify: `web/app/components/ProblemConditions.vue`、`web/app/composables/useProblemConditions.ts`（外枠と、変更を続けて受ける仕組みを共用の部品に替える。動きは変えない）
- Modify: `web/app/components/ProblemSubmissionList.vue`、`web/app/pages/problems/[id]/index.vue`
- Test: `web/tests/submission-list-conditions.spec.ts`

**Interfaces:**
- Consumes: Task 5の関数、Task 6の経路
- Produces（Task 8が使う）:

  ```vue
  <!-- ConditionsPanel.vue: the shell both lists share -->
  <!-- props: chips: { key: string, label: string }[] -->
  <!-- emits: remove(key: string), clear() -->
  <!-- v-model:open: boolean; slots: default (the fields), bar (what sits at the end of the button row) -->
  <!-- exposes: focus() — moves focus to the toggle button -->
  ```

  ```vue
  <!-- SubmissionConditions.vue -->
  <!-- props: conditions: SubmissionConditions, languages: RuntimeLanguage[], withUser: boolean, problems?: { value: string, label: string }[] -->
  <!-- emits: change(patch: Partial<SubmissionConditions>), clear() -->
  <!-- v-model:open: boolean; slot bar; exposes focus() -->
  ```

  ```ts
  // useConditionChanges.ts — the part of useProblemConditions that is not about the problem list
  // Keeps the conditions a change is moving to until the route has them, so a second change builds on the first.
  export function useConditionChanges<T extends object>(options: {
    conditions: ComputedRef<T>
    equal: (a: T, b: T) => boolean
    navigate: (next: T) => Promise<boolean>   // true when the move happened
  }): { change: (patch: Partial<T>) => Promise<void>, pending: Ref<T | undefined> }

  // useSubmissionConditions.ts
  // keep names the query keys of the page that are not conditions (view, scope, …); they survive every change.
  export function useSubmissionConditions(options: { keep: string[], withUser: () => boolean }): {
    conditions: ComputedRef<SubmissionConditions>          // from the route's query; user is '' when withUser() is false
    change: (patch: Partial<SubmissionConditions>) => Promise<void>   // replaces the route's query, without a page number
    clear: () => Promise<void>
    query: ComputedRef<Record<string, string>>             // what to send to the list API, user left out when withUser() is false
  }
  ```

- [ ] **Step 1: 問題一覧の動きを守る準備をする**

  外枠と仕組みを取り出す前に、`tests/problem-list-conditions.spec.ts`と`tests/catalogue.spec.ts`と`tests/tags.spec.ts`が今のコードで通ることを`--repeat-each=10 --workers=4`で確かめ、通過数を控える。取り出したあとも、この3つのテストファイルを1行も変えずに同じ数だけ通すことが、この作業の合格条件である。

- [ ] **Step 2: 失敗するテストを書く（問題ページ）**

  ブラウザーからの`/api/problems/*/submissions`は`page.route`で受け、届いたクエリを記録して、クエリに合わせた行を返す（`rejudge.spec.ts`と同じやり方）。`/api/runtimes`はテスト用のAPIが3言語を返す。

  ```ts
  test('the problem page lists submissions by the chosen conditions', async ({ page }) => {
    await page.goto(`/problems/${id}?view=submissions`)
    const toggle = page.getByRole('button', { name: /^絞り込み・並び替え/ })
    await expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await toggle.click()
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: 'コード長が短い順' })
    await expect(page).toHaveURL(`/problems/${id}?view=submissions&sort=shortest`)
    await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'AC' })
    await page.getByRole('combobox', { name: '言語' }).selectOption({ label: 'C++（すべて）' })
    await page.getByRole('textbox', { name: '提出者' }).fill('alice')
    await page.getByRole('textbox', { name: '提出者' }).press('Enter')
    await expect(page).toHaveURL(`/problems/${id}?view=submissions&sort=shortest&verdict=AC&language=cpp&user=alice`)
    // the last list request carried mine=0, offset=0, sort=shortest, verdict=AC, language=cpp, user=alice
    await expect(toggle).toHaveAttribute('aria-expanded', 'true')
    // the 言語 select has an optgroup C++ holding C++（すべて）, C++17 (GCC), C++23 (GCC); Rust has one option and no （すべて）
    await page.getByRole('combobox', { name: '言語' }).selectOption({ label: 'C++23 (GCC)' })
    await expect(page).toHaveURL(/runtime=cpp23-gcc/)
    await expect(page).not.toHaveURL(/language=/)
  })
  test('chips, clearing and the empty result', async ({ page }) => {
    await page.goto(`/problems/${id}?view=submissions&sort=fastest&verdict=AC&language=cpp&user=alice`)
    const chips = page.getByRole('list', { name: '効いている条件' })
    await expect(chips.getByRole('listitem')).toHaveText([/実行時間が短い順/, /結果: AC/, /言語: C\+\+/, /提出者: alice/])
    await expect(page.getByRole('button', { name: /^絞り込み・並び替え/ })).toContainText('4')
    await chips.getByRole('button', { name: '結果: ACを外す' }).click()
    await expect(page).toHaveURL(`/problems/${id}?view=submissions&sort=fastest&language=cpp&user=alice`)
    // focus is on the toggle button after ×
    // with no row matching: 「条件に合う提出はありません。」 and a 「条件をすべて外す」 button that leads to ?view=submissions
    // with no condition and no row: 「提出はまだありません。」 as today
  })
  test('paging starts over on a change and keeps the conditions', async ({ page }) => {
    // 120 mocked rows: 次へ -> the request carries offset=50 and the conditions; changing the sort -> offset=0
  })
  test('the user field refuses what cannot be a user ID', async ({ page }) => {
    // typing `a b` + Enter keeps the URL, shows the reason next to the box (role alert), and sends no request
    // clearing the box + Enter removes the condition
  })
  test('switching between all and own submissions keeps the conditions but the user', async ({ page }) => {   // Review Focus 5
    await page.goto(`/problems/${id}?view=submissions&sort=shortest&verdict=AC&user=alice`)
    await page.getByRole('link', { name: '自分の提出' }).click()
    await expect(page).toHaveURL(`/problems/${id}?view=my-submissions&sort=shortest&verdict=AC`)
    // the panel has no 提出者 field; the list request carried mine=1, sort=shortest, verdict=AC and no user
    // opening ?view=my-submissions&user=alice directly: no 提出者 chip, and the request carries no user
    await page.getByRole('link', { name: 'すべての提出' }).click()
    await expect(page).toHaveURL(`/problems/${id}?view=submissions&sort=shortest&verdict=AC`)
  })
  test('the list refreshes with its conditions and keeps the panel and the selection', async ({ page }) => {   // Review Focus 4
    // as an author (rejudge allowed): open the panel, choose 結果 WA, tick two rows
    // page.clock.fastForward(15000) -> one more request with verdict=WA; the panel is still open; both rows are still ticked
    // rejudge the two rows: the next responses no longer hold them (they are judging); the 「リジャッジしました」 message shows;
    // requests keep coming every 3 s until the mock reports them DONE again, and stop within a minute if it never does
  })
  test('two changes made at once both take effect', async ({ page }) => {   // Review Focus 4
    // pick 並び順 and 結果 inside one page.evaluate (see quickly() in problem-list-conditions.spec.ts):
    // the URL, the selects, the chips and the last request carry both
    // two chip × in one task: both conditions are gone
  })
  test('the panel fits a phone', async ({ page }) => { /* 375 px, panel open with four conditions: no horizontal overflow */ })
  ```

- [ ] **Step 3: テストが失敗することを確かめる** — Run: `cd web && bun run build && bun run test tests/submission-list-conditions.spec.ts --output=<一時フォルダ>/pw --reporter=line`、Expected: FAIL（ボタンがない）

- [ ] **Step 4: 外枠と仕組みを取り出す**

  - `ConditionsPanel.vue`に、`ProblemConditions.vue`のボタン、数、札の一覧、パネルの枠、「条件をすべて外す」、フォーカスの移動、CSSを移す。`ProblemConditions.vue`は欄だけを持ち、外枠に`ConditionsPanel`を使う。
  - `useConditionChanges.ts`に、`useProblemConditions.ts`の`pending`と、変更を重ねる所と、移動が起きなかったときに戻す所を移す。`useProblemConditions`はそれを使う。Cookie、`restore`、難易度の下限と上限の合わせ方は`useProblemConditions`に残す。
  - Step 1の3つのテストファイルを変えずに、同じ数だけ通ることを確かめてから先へ進む。ここで1つコミットする: `refactor: 問題一覧のパネルの外枠と、条件の変更を続けて受ける仕組みを、共用の部品に取り出す`

- [ ] **Step 5: 提出一覧のパネルを実装し、問題ページに入れる**

  - `UserFilter.vue`: 入力欄と「絞り込む」のボタン。Enterかボタンで確定する。ユーザーIDの形でなければ、移動せずに理由を`role="alert"`で出す（`TagFilter.vue`のタグの理由の出し方に合わせる）。
  - `SubmissionConditions.vue`: 並び順と結果は`FilterSelect`、言語は`<optgroup>`を持つ選択欄、提出者は`UserFilter`（`withUser`のときだけ）、問題は`problems`があるときだけ。札は`submissionConditionChips`から作る。
  - `useSubmissionConditions`: 条件は`parseSubmissionConditions(route.query)`。`change`は`useConditionChanges`を使い、`router.replace`でクエリを置き換える（一覧の中の操作で履歴を増やさない。コンテストの「問題」の選択欄の今の動きと同じ）。`keep`のクエリは残し、`page`は外す。
  - `ProblemSubmissionList.vue`: 条件を`useSubmissionConditions({ keep: ['view'], withUser: () => !props.mine })`から読み、一覧の取得のクエリと、取得のキーと、監視に足す。条件が変わったら`offset`を0に戻す。パネルの開閉は部品の中の`ref`で持ち、更新で閉じない。条件が1つ以上あって0件なら「条件に合う提出はありません。」とボタンを出す。
  - リジャッジのあとの3秒ごとの更新: 今の「採点中の行が見えている間」に加えて、この一覧からリジャッジを頼んだあとは、頼んだ提出がどれも見えなくなっても、60秒たつまで続ける。条件がなければ今と同じ動きになる。
  - `pages/problems/[id]/index.vue`: 「すべての提出」「自分の提出」のリンクは、今の条件を保つ（「自分の提出」へは`user`を外す）。「問題文」「解説」のリンクは条件を持ち越さない。

- [ ] **Step 6: テストが通ることを確かめる**

  Run: `cd web && bun run typecheck && bun run build && bun run test tests/submission-list-conditions.spec.ts tests/rejudge.spec.ts tests/submission.spec.ts tests/partial-scores.spec.ts tests/problem.spec.ts tests/problem-list-conditions.spec.ts tests/catalogue.spec.ts tests/tags.spec.ts --output=<一時フォルダ>/pw --reporter=line`、続けて`tests/submission-list-conditions.spec.ts tests/problem-list-conditions.spec.ts`を`--repeat-each=30 --workers=4`で。
  Expected: PASS

- [ ] **Step 7: 目で確かめる** — PC幅とスマホ幅、ライトとダークで、閉じた状態（札が4つ）と開いた状態のスクリーンショットを撮り、札やボタンが見出しや更新ボタンと重ならないことを見る。

- [ ] **Step 8: コミット** — `feat: 問題ページの提出一覧に、並び替えと絞り込みのパネルを足す`

---

### Task 8: コンテストの提出一覧にパネルを入れる

**Files:**
- Modify: `web/app/pages/contests/[id]/index.vue`
- Modify: `web/tests/contest-submissions.spec.ts`、`web/tests/rejudge.spec.ts`、`web/tests/contest-teams.spec.ts`（「問題」の選択欄を使っているテストを、パネルの操作に直す。確かめている内容は減らさない）
- Test: `web/tests/submission-list-conditions.spec.ts`

**Interfaces:**
- Consumes: Task 7の`SubmissionConditions`、`useSubmissionConditions`

- [ ] **Step 1: 失敗するテストを書く**

  ```ts
  test('the contest list narrows and orders by the panel', async ({ page }) => {
    await page.goto(`/contests/${cid}?view=submissions`)
    await page.getByRole('button', { name: /^絞り込み・並び替え/ }).click()
    await page.getByRole('combobox', { name: '問題' }).selectOption({ label: /^A / })
    await page.getByRole('combobox', { name: '結果' }).selectOption({ label: 'WA' })
    await page.getByRole('combobox', { name: '並び順' }).selectOption({ label: '実行時間が短い順' })
    await expect(page).toHaveURL(`/contests/${cid}?view=submissions&problem=A&verdict=WA&sort=fastest`)   // the order of keys follows the implementation; assert the set
    // the last request carried offset=0, limit=50, problem=<A's uuid>, verdict=WA, sort=fastest
    // chips: 実行時間が短い順, 結果: WA, 問題: A — the problem chip shows the letter
    // the total and the page numbers follow the filtered total the mock returns; choosing a condition on page 3 goes back to page 1
  })
  test('an old link with a problem still opens narrowed', async ({ page }) => {   // Review Focus 5
    await page.goto(`/contests/${cid}?view=submissions&problem=A`)
    // the request carries problem=<A's uuid>; the chip 問題: A shows; the panel's 問題 select shows A
    // 「条件をすべて外す」 leads to ?view=submissions and a request without problem
  })
  test('the onsite switch stays outside the panel', async ({ page }) => {
    // a contest with an onsite division: the switch is visible with the panel closed, is not a chip, and is not counted
    // with the switch on and one condition, 「条件をすべて外す」 keeps scope=onsite in the URL and in the request
    // with only the switch on and no row: 「提出はまだありません。」; with a condition and no row: 「条件に合う提出はありません。」
  })
  test('own and team lists have no user field', async ({ page }) => {
    // ?view=my-submissions&user=alice&verdict=AC: no 提出者 field or chip; the request carries mine=1 (or mine=team), verdict=AC and no user
    // switching 提出一覧 <-> 自分の提出 through the contest menu keeps sort, verdict, language/runtime and problem, and drops user
  })
  test('refreshing keeps the panel, the page and the selection', async ({ page }) => {   // Review Focus 4
    // on page 2 with 結果 WA and a problem chosen, two rows ticked for a rejudge: after the 15 s refresh the panel is open,
    // the page is still 2, the rows are still ticked, and the request carried the same conditions
  })
  test('the conditions fit a phone', async ({ page }) => { /* 375 px, the switch, four chips: no horizontal overflow */ })
  ```

  既存のテストの直し: `contest-submissions.spec.ts`の「問題」の選択欄は、パネルを開いてから選ぶ。リジャッジは今と同じく、問題を1つ選んでいるときだけ出る。

- [ ] **Step 2: テストが失敗することを確かめる** — Run: `cd web && bun run build && bun run test tests/submission-list-conditions.spec.ts -g 'contest|old link|onsite|team|refreshing' --output=<一時フォルダ>/pw --reporter=line`、Expected: FAIL

- [ ] **Step 3: 実装する**

  - 「問題」の選択欄を`SubmissionConditions`の中へ移す。選択肢は今と同じ（「A - タイトル」）。URLの`problem`は今と同じ問題の記号のままにし、APIへはUUIDを渡す。今の`problemFilter`の書き込みは、パネルの`change({ problem })`に替える。
  - `useSubmissionConditions({ keep: ['view', 'scope', 'live'], withUser: () => submissionList.value === 'all' })`を使う。今このページが使っているクエリを確かめ、`keep`に漏れがないようにする。
  - 一覧の取得のクエリ、取得のキー、監視に条件を足す。条件が変わったら1ページ目に戻る。
  - 「オンサイトのみ表示」とリジャッジのボタンは、パネルの`bar`のスロットに置く。
  - 空のときの文言、リジャッジのあとの更新は、Task 7の問題ページと同じ決まりにする。
  - コンテストのメニューの「提出一覧」「自分の提出」のリンクは、今の条件を保つ（「自分の提出」へは`user`を外す）。順位表など、ほかの表示へのリンクは条件を持ち越さない。

- [ ] **Step 4: テストが通ることを確かめる**

  Run: `cd web && bun run typecheck && bun run build && bun run test tests/submission-list-conditions.spec.ts tests/contest-submissions.spec.ts tests/contest-own-submissions.spec.ts tests/contest-teams.spec.ts tests/contest-scope.spec.ts tests/rejudge.spec.ts tests/streaming-mode.spec.ts --output=<一時フォルダ>/pw --reporter=line`、続けて`tests/submission-list-conditions.spec.ts`を`--repeat-each=30 --workers=4`で。
  Expected: PASS

- [ ] **Step 5: 目で確かめる** — Task 7のStep 7と同じ4通りを、コンテストの一覧で。

- [ ] **Step 6: コミット** — `feat: コンテストの提出一覧に、並び替えと絞り込みのパネルを足す`

---

### Task 9: 実DBの通しテストと文書

**Files:**
- Modify: `api/internal/httpapi/browser_fixture_test.go`、`web/tests/account/schedule.ts`
- Create: `web/tests/account/submission-list.spec.ts`
- Create: `docs/adr/0046-sort-and-filter-submission-lists.md`（番号は、書く時点の次の番号）
- Modify: `docs/adr/README.md`、`web/README.md`、設計書の「状態」の行

**Interfaces:**
- Consumes: Task 1〜8の全部
- Produces: `POST /__test/problems/{id}/submissions`。本文`{ "owner": "bob", "runtime": "cpp23-gcc", "source": "…", "verdict": "AC", "cpuTimeMs": 12 }`で、採点の終わった公開の提出を1件入れる。`verdict`を省くと採点中の提出になる。今の`/__test/problems/{id}/accepted`と同じ書き方にする。

- [ ] **Step 1: 失敗するテストを書く**

  ```ts
  test('submissions on a real problem filter and sort', async ({ page }) => {
    // alice publishes a problem; the fixture records, oldest first:
    //   bob   cpp23-gcc   60 bytes  AC  30 ms
    //   bob   python314   20 bytes  WA  90 ms
    //   carol pypy311    200 bytes  AC   8 ms
    //   carol cpp17       35 bytes  AC  15 ms
    //   bob   rust2024    50 bytes  (judging)
    await page.goto(`/problems/${shortId(id)}?view=submissions`)
    // open the panel; 結果 AC + コード長が短い順 -> the rows are carol/cpp17 (35), bob/cpp23-gcc (60), carol/pypy311 (200), by their コード長 cells
    // 実行時間が短い順 with 結果 AC -> 8 ms, 15 ms, 30 ms
    // 言語 Python（すべて） with 結果 すべて -> bob's WA and carol's PyPy AC, newest first
    // 言語 C++（すべて） -> the cpp17 and cpp23-gcc rows
    // 提出者 bob with every other condition cleared -> three rows, the judging one first
    // a reload renders the same rows
  })
  ```

- [ ] **Step 2: テストが失敗することを確かめ、入口を足して通す**

  Run: `cd web && bun run build && bun run test --config playwright.account.config.ts tests/account/submission-list.spec.ts --output=<一時フォルダ>/pw-account --reporter=line`
  ほかのアカウント系のテストに響く副作用（初めてのACの通知など）がないかを確かめ、あれば、このテストの中で片付ける（`problem-list.spec.ts`のやり方）。

- [ ] **Step 3: 全体を通す**

  Run: `cd api && go test -race ./... -count=1`、`cd web && bun run test --output=<一時フォルダ>/pw --reporter=line`、`cd web && bun run test --config playwright.account.config.ts --output=<一時フォルダ>/pw-account --reporter=line`
  Expected: すべてPASS。

- [ ] **Step 4: 文書を書く**

  - ADR: 背景（#36）、決めたこと（2つの画面、条件とクエリ、言語のまとめと処理系、問題一覧のパネルとの共用、条件を覚えないこと、オンサイトのスイッチ、リジャッジとの関係）、退けた案（処理系ごとだけの選択肢、列見出しでの並び替え、メモリでの並び替え）、Task 4で測った数字と足した索引、受け入れた動き、確かめていないこと。`docs/adr/README.md`の一覧に足す。
  - `web/README.md`に、2つの画面のパネルと条件を書く。
  - 設計書の「状態」の行を、実装済みに直す。

- [ ] **Step 5: コミット** — `test: 提出一覧の並び替えと絞り込みを、実DBで通して確かめる`と、`docs: 提出一覧の並び替えと絞り込みをADRとREADMEに書く`の2つ。

---

## 公開したあと（この計画の外）

- mainへ入れてpushすると`Deploy dev`がAPI、マイグレーション、Webの順に配布する。索引を足した場合は、コンテスト中を避ける。
- 本番で、提出の多い問題とコンテストの一覧を、条件を変えて開き、応答時間を確かめる。
- Issue #36 に返信して閉じる。
