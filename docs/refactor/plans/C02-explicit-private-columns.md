# C02 テスター用の問題一覧のSQLを文字列置換で作るのをやめる 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `problems.Store.list`で、作者用一覧のSQLの末尾を`strings.Replace`で差し替えてテスター用一覧を作るのをやめる。2つのSQLを、共通の選択列から連結したパッケージ定数として書く。生成されるSQLはバイト単位で変えない。

**Architecture:** 先に、テスター用一覧のカーソルによるページ送りの特性テストを`TestTesterInvitationsPostgres`に足す（今のテストはこの経路を通らない）。次に`api/internal/problems/store.go:276-288`を書き換え、9列の共通部分`summaryColumns`、作者用`ownedSummaries`、テスター用`testedSummaries`の3定数にする。`list()`は`testing`で定数を選ぶだけにする。変更前後のSQLの一致は、`go test -overlay`でスクラッチの比較テストを足して確かめる。リポジトリには比較用のファイルを作らない。

**Tech Stack:** Go（`api`モジュール）、PostgreSQL（テストDB `127.0.0.1:25432/openoj_test`）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C02.json`（2つの反証の`revised_scope`、`must_preserve`、`test_plan`）

## 困りごと

- テスター用一覧のSQLは、作者用SQLの末尾の約70文字（`NULL::text,NULL::text,NULL::text FROM problem_drafts WHERE owner_id=$1`）を目印にした`strings.Replace`で作られている（`store.go:277`, `:281`）。テスター用のSQLの全体は、置換を頭の中で当てはめないと読めない。
- 目印が外れても`Replace`は元の文字列を返すだけで、コンパイルも型検査も通る。そうなると`?role=tester`が作者用一覧を返す。CIのDBテスト（`testers_test.go:142-147`）では落ちるが、カーソル付きの経路にはテストがない。
- 547e225と74b6ed0の2回とも、テスター用に列を足すときに、ベースの`NULL::text`の個数と`Replace`の検索文字列を同じコミットで直している（74b6ed0で確認済み）。列を足すたびに3か所をそろえる必要がある。変更後は2か所で済み、目印もなくなる。
- 価値は「低〜中」とする（2つの反証とも、候補の「high」を下げている）。小さく、生成されるSQLをバイト単位で比べられるので実施する。

## 判定の記録（反証の差と、計画で決めた点）

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| `submissions/store.go`の`Get`（`strings.Replace(columns, "result,", ...)`） | 外す | `columns`をresultの前後の2定数に分けて含める | **外す。** `columns`は5回の変更がすべて末尾への追加で、resultの位置は動いていない。`"result,"`は`columns`の中に1回しか現れない。2定数に分けると、`scan()`（`store.go:157`）の15引数と列の並びを見比べにくくなる。漏れは`submissions_test.go:652-660`と`:709-724`で検出できる。さらに、同じパッケージの`contest.go`と`problem.go`はC06が変更中である |
| 定数の置き場所 | 関数内でもパッケージでもよい | パッケージ定数3つ | パッケージの非公開定数にする。`publication.go:89-104`（`acceptedRun`など）の書き方に合わせる。overlayの比較テストから参照できる |
| 特性テスト：テスター一覧のカーソル | 不要（SQL比較で補う） | 追加する | **追加する。** 既存のテストが通らない経路である。計画者がoverlayで次の3点を確かめた。変更前のコードで3回とも成功する。変更後の試作でも成功する。テスター用だけカーソルを付けない変異では失敗する |
| 特性テスト：`Get`の管理者経路（テスターがcheckerLogを見られる） | — | 追加する | 入れない。submissions側を範囲外にしたので、今回の変更はこの経路に触れない |
| `role=tester&cursor=bad`の400 | — | 追加する | 入れない。`contentCursor`はroleの分岐より前で判定するので（`private_problems.go:198-201`）、今回の変更は触れない |
| 一時比較の方法 | 最終SQLを取り出して保存する | パッケージ内に一時の`_test.go`を置き、消してからコミットする | `go test -overlay`でスクラッチの比較テストを足す。リポジトリにファイルを作らないので、消し忘れが起きない |
| 比較の対象 | カーソルの有無×2モードの4通り | 作者、テスター、`columns`、`Get` | `ownedSummaries`と`testedSummaries`の2つにする。カーソル、ORDER BY、LIMITを付け足すコード（`:289-294`）は変えないので、2つが一致すれば4通りとも一致する |
| `make package package-judge` | — | 実行する | 実行しない。`Makefile`にこのターゲットはない（**誤り**：2つのターゲットは`api/Makefile`にあり、検証では実行した） |
| golangci-lint | — | 件数が増えないこと | 参考として実行する。CIでは実行されない。計画時点での件数は、`./internal/problems/ ./internal/httpapi/`で2件（どちらも`contest_teams_test.go`のstaticcheck） |

## Global Constraints

- 公開HTTP API（ルート、JSONの形、ステータスコード、エラー文言）、CLI、DBスキーマとマイグレーション、依存のバージョン、インフラとCIは変えない。
- テストは追加だけにする。既存のテストは変更・削除・Skipしない。DBテストはSkipを成功に数えない（`TEST_DATABASE_URL`がないとSkipになる）。
- `ownedSummaries`は、変更前のベースクエリ（`store.go:277`）とバイト単位で一致させる。`SELECT id, draft, updated_at, published_version,ever_published,`の空白の不ぞろい、`NULL::text`×3、`FROM problem_drafts WHERE owner_id=$1`もそのまま残す。
- `testedSummaries`は、変更前の`strings.Replace`の結果とバイト単位で一致させる。`via.via_author FROM problem_drafts`の後の改行、各行頭の空白（1個または2個）、`$1`だけを使うこと、`owner_id<>$1`、UNION ALLの候補、最終判定`AND can_manage_problem(id,$1)`も変えない。
- 選択する列は12列で、順番も変えない。`row.Scan`の12引数（`store.go:301`）は変えない。
- `list()`のうち、次の部分は1文字も変えない。
  - `args := []any{owner}`以降
  - カーソル条件` AND (updated_at, id) < ($2, $3::uuid)`を両モードの末尾に付けること
  - ` ORDER BY updated_at DESC, id DESC LIMIT 51`
  - 3列がすべて非NULLのときだけ`ViaContest`を付けること（`:304-307`）
- `List`と`ListTesting`のシグネチャとdocコメント、`list`のシグネチャ、HTTP層（`private_problems.go:197-229`）は変えない。
- `strings`のimportは、`store.go`でほかに使っていないので外す。ほかの整理には広げない。
- `api/internal/submissions/`には触れない。
- コードのコメントは英語にし、周りと同じ密度（1〜2行）にする。テスター用の分岐にある既存の2行のコメントは、`testedSummaries`の上へ文言を変えずに移す。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターが行う。
- Bashの`cd`には絶対パスを使う。Go、gofmt、golangci-lintは`nix develop`の中で実行する。
- npm、webのビルド、Playwrightは実行しない（ほかのサイクルの検証と共有している）。webのビルドとaccountのPlaywrightは、検証担当がワークフローの中で行う。

## Review Focus

1. 連結の境目で、バイト列がずれること。次の3か所が該当する。
   - `SELECT `の後の空白
   - `summaryColumns`とvia列の間のカンマ
   - `via.via_author FROM problem_drafts`の直後の改行と` LEFT JOIN`の先頭の空白

   DBテストではほとんど検出できない。検出できるのはStep 5のバイト比較だけである。
2. テスター用一覧にカーソルが付く経路。必要なことは2つある。
   - カーソル条件が`AND can_manage_problem(id,$1)`の後ろにANDで続くこと
   - `$2`と`$3`の番号がテスター用SQLの中で使われていないこと

   Step 1の特性テストで確かめる。
3. 作者用一覧が、変更前と同じ12列・`WHERE owner_id=$1`のままであること。`private_problems_test.go:283-290`（作者一覧のカーソル）と`testers_test.go:139-140`で確かめる。
4. `ViaContest`の付き方。コンテストのセッターだけが付き、招待されたテスターは付かない。作者のハンドルも含む。`contest_setters_test.go:239-252`と`contest_races_test.go:243-249`で確かめる。
5. `testing`がfalseのときに`testedSummaries`を選ぶ、あるいはその逆のような、定数の取り違え。`testers_test.go:142-147`（bobのテスター一覧にaがありbがない／作者aliceのテスター一覧にaがない）と、Step 1のテストで確かめる。

---

### Task 1: テスター一覧のカーソルを特性テストで固定し、SQLを定数の連結で書く

**Files:**
- Test: `api/internal/httpapi/testers_test.go`（`TestTesterInvitationsPostgres`の中、`"contest":[]`を確かめる`if`ブロック（現在の200-202行）の直後、`penalty := 5`の前）
- Modify: `api/internal/problems/store.go:3-16`（import）、`:276-288`（`list`の先頭と、その直前に定数を追加）

**Interfaces:**
- Consumes：既存のテスト補助`request(method, path, owner string, body any, want int) string`、`nextContentCursor(id string, date time.Time) string`、定数`a`と`c`
- Produces（`api/internal/problems/store.go`、パッケージ非公開の型なし文字列定数）
  - `summaryColumns`：変更前のベースクエリのうち`id, draft, ...`から`AND NOT c.released)`までの9列。`SELECT `と末尾のカンマは含めない。
  - `ownedSummaries = `SELECT ` + summaryColumns + `,NULL::text,NULL::text,NULL::text FROM problem_drafts WHERE owner_id=$1``
  - `testedSummaries = `SELECT ` + summaryColumns + `,via.via_id,via.via_title,via.via_author FROM problem_drafts` + 改行 + 変更前の置換文字列の2行目以降（` LEFT JOIN LATERAL ...`から` AND can_manage_problem(id,$1)`まで）をそのまま`

- [ ] **Step 0: 作業前の状態を記録する**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short`
出力を控えておく。完了条件で、増えたのが2ファイルだけであることを確かめるのに使う。

- [ ] **Step 1: 特性テストを追加する**

挿入位置は上の「Files」のとおり。次の内容にする（計画者がoverlayで実行し、確かめた形）。

```go
	// The tested list pages newest first with the same cursor as the owned list.
	testedPage := func(cursor string) []problems.Summary {
		t.Helper()
		var page struct {
			Items      []problems.Summary `json:"items"`
			NextCursor string             `json:"nextCursor"`
		}
		if err := json.Unmarshal([]byte(request("GET", "/my/problems?role=tester"+cursor, "bob", nil, 200)), &page); err != nil {
			t.Fatal(err)
		}
		if page.NextCursor != "" {
			t.Fatalf("short tested list has a next cursor: %+v", page)
		}
		return page.Items
	}
	newest := testedPage("")
	if len(newest) != 2 || newest[0].ID != c || newest[1].ID != a {
		t.Fatalf("tested page: %+v", newest)
	}
	if rest := testedPage("&cursor=" + nextContentCursor(c, newest[0].UpdatedAt)); len(rest) != 1 || rest[0].ID != a {
		t.Fatalf("tested page after c: %+v", rest)
	}
	if rest := testedPage("&cursor=" + nextContentCursor(a, newest[1].UpdatedAt)); len(rest) != 0 {
		t.Fatalf("tested page after a: %+v", rest)
	}
```

この時点で、bobがテストしているのはa（招待）とc（`problem_testers`）の2つである。cはaの最後の保存より後に作られる。時刻が等しくなっても`id DESC`でcが先になるので、並びは決まる。bはaliceの問題で、bobはテストしていない。そのため、カーソル付きでテスターの条件が外れると、bが2ページ目に出て失敗する。

- [ ] **Step 2: 変更前のコードで成功することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run 'TestTesterInvitationsPostgres' ./internal/httpapi/ 2>&1 | grep -E -- '^(--- |ok|FAIL)'`
Expected: `--- PASS: TestTesterInvitationsPostgres`と`ok  judge/api/internal/httpapi`。`--- SKIP`は出ない。

- [ ] **Step 3: `store.go`を書き換える**

1. `func (s *Store) list(`の直前（`ListTesting`の後）に、`Interfaces`の3定数を置く。
2. コメントは次のとおりにする。
   - `summaryColumns`の上に1行：`// Both lists select these columns, then the three columns of ViaContest, which the owned list leaves NULL.`
   - `ownedSummaries`にはコメントを付けない。
   - `testedSummaries`の上に、今テスター用の分岐にある2行（`// Setters of a published contest test its problems ...`と`// subquery only narrows the candidates; ...`）を、文言を変えずに移す。
3. `list()`の先頭（`query := ...`から`if testing { ... }`の閉じ括弧まで）を、次の形に置き換える。`args := []any{owner}`以降は変えない。

```go
	query := ownedSummaries
	if testing {
		query = testedSummaries
	}
```

4. importから`"strings"`を外す。

テスター用の2行目以降は、`git show HEAD:api/internal/problems/store.go`の282-287行（置換文字列の後半。最後の行の`, 1)`は除く）を文字どおり写す。手で打ち直さない。

- [ ] **Step 4: gofmtとvetを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l internal/problems internal/httpapi`
Expected: 出力なし（nixの`Git tree ... is dirty`の警告は除く）

- [ ] **Step 5: 変更前後のSQLがバイト単位で一致することを確かめる（一時、コミットしない）**

計画者が用意した比較テストは`/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c02/compare_test.go`にある。中身は`package problems`の`TestC02ListQueriesUnchanged`で、HEADの2つの文字列（ベースクエリと`strings.Replace`の結果）を、`ownedSummaries`および`testedSummaries`と`!=`で比べる。overlayの設定は`/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c02/overlay-compare.json`にある。これは、存在しない`api/internal/problems/c02_compare_test.go`に比較テストを足すだけのものである。

1. 比較テストの中の文字列が、今のHEADと一致することを確かめる。
   Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git show HEAD:api/internal/problems/store.go | sed -n '277p;281,287p' | sed 's/^\t*//; s/^query := //; s/^query = strings.Replace(query, /strings.Replace(old, /' > /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c02/old.txt && sed -n '10,17p' /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c02/compare_test.go | sed 's/^\t*//; s/^old := //; s/^oldTesting := //' > /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c02/mine.txt && diff /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c02/old.txt /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c02/mine.txt && echo SAME`
   Expected: `SAME`。差が出たら（HEADの`list`が計画後に変わったら）、止まって報告する。
   このファイルが読めないときは、自分のスクラッチに同じものを作る。HEADの277行と281-287行を写した比較テストと、それを指すoverlayである。
2. 比較テストを実行する。
   Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -overlay=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c02/overlay-compare.json -count=1 -v -run TestC02ListQueriesUnchanged ./internal/problems/ 2>&1 | grep -E -- '^(--- |ok|FAIL)|changed|missed'`
   Expected: `--- PASS: TestC02ListQueriesUnchanged`。`owned list changed`や`tested list changed`が出たら、Step 3の写し方を直す。
3. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short`
   Expected: Step 0の出力に比べて増えたのは、`api/internal/problems/store.go`と`api/internal/httpapi/testers_test.go`だけ。`c02_compare_test.go`は出ない。

- [ ] **Step 6: 関係するDBテストを実行する**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run 'TestTesterInvitationsPostgres|TestContestSettersPostgres|TestContestSetterRacesPostgres|TestMutualCoSetterDeletionPostgres|TestPrivateProblemsPostgres|TestProblemListPublicationPlansPostgres|TestProblemReadinessPostgres|TestFeatured|TestSubmissionsPostgres' ./internal/problems/ ./internal/httpapi/ 2>&1 | grep -E -- '^(--- |ok|FAIL)'`
Expected: すべて`--- PASS`で、2パッケージとも`ok`。`--- SKIP`と`--- FAIL`は出ない。

- [ ] **Step 7: 検証コマンドの1〜3を実行する**（後述）

- [ ] **Step 8: グラフを更新する**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`
（`graphify-out/`は`.gitignore`の対象で、コミットには含まれない）

コミットメッセージの案（オーケストレーター用）：`refactor: テスター用の問題一覧のSQLを文字列置換で作るのをやめ、作者用と共通の列から連結した定数にする`

## 検証コマンド

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 ./...` → すべて`ok`（または`no test files`）。`FAIL`は出ない。
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 終了コード0で、警告なし
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .` → 出力なし
4. 参考：`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 600 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command golangci-lint run ./internal/problems/ ./internal/httpapi/` → `2 issues`のまま（どちらも`contest_teams_test.go`）

## 完了条件

- Step 1の特性テストが、変更前のコードで成功している（Step 2）。
- Step 5の比較テストが成功している。その比較テストは、リポジトリに残っていない。
- Step 6と検証コマンドの1〜3がすべて成功している。DBテストに`--- SKIP`がない。
- `api/internal/problems/store.go`に`strings.Replace`がない。`list()`の`args := []any{owner}`以降の差分は0行である。
- 差分に出るのは、`api/internal/problems/store.go`と`api/internal/httpapi/testers_test.go`の2ファイルだけ。

## 範囲外

- `api/internal/submissions/store.go`の`Get`の`strings.Replace(columns, "result,", ...)`と、`columns`の分割（判定の記録を参照）
- `scan()`（`submissions/store.go:157`）と`list()`の`row.Scan`（`problems/store.go:301`）が、列の位置に依存していること
- `List`と`ListTesting`の統合、`testing bool`引数の廃止、クエリビルダーの導入
- `publication.go`などほかのSQL定数の整理。`SELECT id, draft, updated_at, ...`の空白の不ぞろいを直すこと（バイト列が変わる）
- `Get`の管理者経路や`role=tester&cursor=bad`の特性テスト

## レポートで触れる別件（直さない）

- 不具合は見つからなかった。
- safetyの反証の`test_plan`にある`make package package-judge`は、`Makefile`にない（**誤り**：2つのターゲットは`api/Makefile`にあり、検証では実行した）。
- `submissions/store.go:277`の置換は今は正しく動く（`"result,"`は`columns`に1回だけ現れ、それはresult列である）。ただし、目印との結合は残っている。将来`columns`のresultより前に`result,`を含む式を足すときは、ここも直す必要がある。今回は保留とする。
