# C11 Problemを読む列のリストを定数にし、Get・Save・Publish・Scheduleで共有する 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `problems.scan`が読む8列のリストを、パッケージ定数`problemColumns`にまとめる。今は6つのクエリに手で書き写されているので、それをやめて定数を使う。返す`Problem`と、クライアントから見えるJSONは変えない。

**Architecture:** 最初に特性テストを1つ足す。書き込み5経路（新規作成、更新、予約、公開、取り下げ）の応答が、直後のGETと同じ値を返すことを固定するテストである。次に`store.go`の`scan`の直前に定数を置く。Get、SaveのINSERTとUPDATE、Publishの2クエリ、Scheduleの列リストを、``SELECT `+problemColumns+` FROM …``と``RETURNING `+problemColumns``の形に置き換える。`posts/store.go:65-70,95,102`の`columns`と同じ書き方である。

**Tech Stack:** Go（`api`モジュール）、PostgreSQL（テストDB `127.0.0.1:25432/openoj_test`）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C11.json`（worthの`revised_scope`が拘束力を持つ。両反証の`must_preserve`と`test_plan`も参照）

## 困りごと

- 次の8列のリストが、`scan`（`store.go:216-228`）とは別に6か所へ手で書かれている。
  - 8列：id, version, updated_at, draft, published_version, 作者ハンドルのCOALESCE, contest_idのCOALESCE, scheduled_at
  - 6か所：`store.go:231`（Get）、`:373`（INSERT … RETURNING）、`:382`（UPDATE … RETURNING）、`publication.go:69`と`:71`（Publish）、`schedule.go:41-42`（Schedule）
- 0acd889（author）、c118c35（contestId）、190fe16（scheduledAt）の3回とも、`scan`とこれらのSQLを同じコミットで直している。1か所でも漏れるとコンパイルは通り、その経路だけが実行時にScanエラー（500）になる。漏れた式が別の値を返す場合は、500にもならない。計画者がoverlayで確かめたところ、公開側のPublishの`RETURNING`で作者式だけを壊しても、`problems`、`httpapi`、`contests`の既存テストはすべて通った。Step 1のテストだけがこれを検出する。
- 同じ層に前例がある。`posts`の`columns`（SELECTとRETURNINGで共有）、`submissions`の`columns`、`contests`の`fields`である。
- 価値は「低〜中」とする（worthの判定に従う）。変更は小さく慣習どおりで、6経路はどれもDBテストで実行される。

## 判定の記録

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| Scheduleを対象に含めるか | 含める（`revise`。`revised_scope`は拘束力を持つ） | 今のSQLのまま残し、コメントを置く | **含める。** contest_idの`''`は定数のサブクエリに置き換わるが、結果は必ず`''`になる。根拠は下の「Scheduleの等価性」を参照。等価性に疑いが残った場合だけ、Scheduleを除いた5か所にする。そのときは`schedule.go`のRETURNINGに「scanの列と一致させる」と1行のコメントを置き、理由をレポートに書く |
| 定数のコメント | 「scanの並びと一致させる」 | 別名を付けない前提を明記する | 両方を英語2行で書く（Step 3） |
| 定数の文字列 | — | 今の式と完全に一致させる | Get（`store.go:231`）の`SELECT `と` FROM`の間をバイトどおり写す。Get、INSERT、UPDATEとはバイト単位で一致する。Publishの2つとの違いは、先頭5列のカンマの後の空白だけ。Scheduleとの違いは、改行と行頭の空白、そして`''`である（Step 5で機械的に確かめる） |
| 特性テスト | 3件（Schedule、Publish、新規作成） | RETURNING経路の応答（author、id、versionなど） | 1つのテストで5経路を確かめる。各応答のscan由来の値が直後のGETと一致すること、そして具体的な値（Author=`alice`、ContestID=`""`、version 1〜5、PublishedVersion、ScheduledAt）を確かめる |
| 公開済みコンテストの問題をPublishしたときのcontestId | — | 準備が重ければ省いてよい | 省く。Getと同じ定数の式を通るので、`contests_test.go:263-266`で確かめられる |

### Scheduleの等価性（レポートとコミットメッセージに残す）

1. Scheduleは、`schedule.go:22`で`problem_drafts`の行を`FOR UPDATE`でロックする。同じトランザクションの`:33`で`NOT EXISTS(contest_problems WHERE problem_id=$1)`を確かめ、行があれば`ErrSchedule`で終わる。
2. 本番で`contest_problems`へ行を入れるのは`contests/store.go:403`だけである（migrationsにもINSERTはない）。その前の`:334-338`で、対象の問題を`FOR UPDATE OF d`でロックし、`d.scheduled_at IS NULL`を条件にしている。
3. このため、Scheduleが行ロックを持っている間は、その問題の`contest_problems`の行は増えない。Schedule側が先にロックを取っていれば、コンテスト保存はコミットまで待つ。READ COMMITTEDで再評価すると`scheduled_at`が非NULLなので、その行は外れて`ErrConflict`になる。コンテスト保存が先なら、`:33`の新しいスナップショットに行が見えるので、`ErrSchedule`になる。
4. `contest_problems`は`UNIQUE(problem_id)`なので、サブクエリは0行か1行を返す。UPDATEのRETURNINGの時点では0行で、COALESCEにより`''`になる。応答の`contestId`は今と同じく省略される（`omitempty`）。

## Global Constraints

- 公開HTTP API（ルート、JSONの形、ステータスコード、エラー文言）、CLI、DBスキーマとマイグレーション、依存のバージョン、インフラとCIは変えない。
- 認可と可視範囲を変えない。各WHERE句、SET句、プレースホルダの番号、引数の並びは1文字も変えない。
  - Getの`WHERE can_manage_problem(id,$1) AND id=$2`、引数`(owner, id)`
  - INSERTの`VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING`、引数`(id, owner, data)`
  - UPDATEのSET句と`WHERE can_manage_problem(id,$1) AND id=$2 AND version=$3`、引数`(owner, id, version, data)`
  - Publishの2つのSET句と`WHERE can_manage_problem(id,$1) AND id=$2 AND version=$3`、引数`(owner, id, version)`
  - Scheduleの`SET scheduled_at=$2,…`と`WHERE id=$1`、引数`(id, at)`
- `problemColumns`の8列は、`scan`の`row.Scan`の8引数（`store.go:219`）と同じ順にする。各式の文字列は、今のGetのものと一致させる（`COALESCE(…,'')`、`contest_id::text`、`problem_drafts.owner_id`と`problem_drafts.id`の修飾）。`$n`を含めない。
- 定数を使うクエリでは、`problem_drafts`に別名を付けない（6か所とも今は付けていない）。
- Go側の処理は変えない。
  - `scan`の`ErrNoRows`から`ErrNotFound`への変換
  - SaveでErrNotFoundのときにGetで確かめてから`ErrConflict`を返すこと（`store.go:406-413`）
  - Saveの`p.ContestID = ""`と、`UPDATE contest_problems … RETURNING`による上書き（`:395-402`）
  - Publishの`ErrNotFound`から`ErrConflict`への変換（`publication.go:74-76`）、テスターの権限判定（`:59`）、`ErrContestLocked`（`:66-68`）
  - Getが後から`Testers`と`ContestEditors`を埋めること（`store.go:235-239`）
- `list`（`summaryColumns`など）、`State`、`contestEditors`、`PublicGet`、`PublicList`のSQLには触れない。
- テストは追加だけにする。既存のテストは変更・削除・Skipしない。DBテストはSkipを成功に数えない（`TEST_DATABASE_URL`がないとSkipになる）。`--- SKIP`が出てよいのは`TestBrowserFixture`（`browser fixture disabled`）だけ。
- コードのコメントは英語で書き、周りと同じ密度（1〜2行）にする。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターが行う。
- npm、webのビルド、Playwrightは実行しない（C03の担当者が同じワークツリーで実行している）。
- Bashの`cd`には絶対パスを使う。Goとgofmtは`nix develop`の中で実行する。シェル変数からコマンドを組み立てない。

## Review Focus

1. **定数の列の順序が`scan`とずれること。** 8列すべての経路が同時に壊れる。型が合わなければ500になり、型が合えば値が入れ替わる。既存のGETのテストで検出できる（`testers_test.go:153`のauthor、`contests_test.go:263-266`のcontestId、`scheduled_problems_test.go:29`と`:63`のscheduledAtとpublishedVersion）。Step 1のテストでも検出できる。
2. **連結の境目と引数。** 確かめるのは次の2点である。DBテストでは、構文エラー（500）としてしか見えない。
   - `SELECT `と` FROM problem_drafts`、`RETURNING `の前後の空白
   - 各クエリの引数の並びが変わっていないこと（特に、INSERTは`(id, owner, data)`で、UPDATEの`(owner, id, version, data)`と違う）
3. **Scheduleの`contestId`が空のままであること。** Step 1のテストで`got.ContestID == ""`を確かめる。等価性の根拠は「Scheduleの等価性」のとおり。
4. **RETURNINGの経路だけが別の値を返すこと**（今回なくしたい漏れそのもの）。Step 1のテストで、5経路の応答をGETと比べる。計画者がoverlayで確かめた結果、公開側のPublishの作者式だけを壊すと、このテストだけが失敗した（`problems`、`httpapi`、`contests`の既存テストは通った）。
5. **SaveとPublishの競合経路。** 次の挙動を変えないこと。Go側に手を入れないので変わらないはずだが、既存のテストで確かめる。
   - 古いversionでの更新が409になること
   - 他人のversion 0の作成が404になること
   - 同じversionで並行に書くと、片方だけが成功すること

   確かめるテストは`private_problems_test.go:206-251`と`scheduled_problems_test.go:81`。

---

### Task 1: 書き込み応答の特性テストを足し、Problemの列を定数にする

**Files:**
- Test: `api/internal/httpapi/scheduled_problems_test.go`（`TestScheduledProblemValidationAndCancellationPostgres`の直後、今の108行目の後。importに`"reflect"`を足す）
- Modify: `api/internal/problems/store.go:214-231`（定数の追加とGet）、`:373`、`:382`
- Modify: `api/internal/problems/publication.go:69`、`:71`
- Modify: `api/internal/problems/schedule.go:40-42`

**Interfaces:**
- Consumes：既存のテスト補助`newFeaturedTest(t)`、`f.request(method, path, owner string, body any, want int) string`、`scheduledProblem(t, raw string) problems.Problem`、`newSubmissionID() string`
- Produces：`api/internal/problems/store.go`の非公開の型なし文字列定数`problemColumns`。パッケージ内に同じ名前の識別子はない。

- [ ] **Step 0: 作業前の状態を記録する**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short -- api`
Expected: 出力なし。C03が`web/`を変更中なので、確かめる範囲は`api`に限る。

- [ ] **Step 1: 特性テストを追加する**

挿入位置は「Files」のとおり。次の内容にする。計画者がoverlayで実行し、変更前と変更後の両方のコードで成功することを確かめた形である。

```go
// Every write answers with the problem as the next read shows it.
func TestProblemWriteResponsesMatchReadPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	level := 3
	d := problems.Draft{Title: "Featured problem", Markdown: "Statement", Editorial: "SECRET EDITORIAL", TimeLimitMS: "1000", MemoryLimitMB: "256", Difficulty: &level, TestCases: []problems.TestCase{{Input: "1", Output: "2"}}}
	path := "/my/problems/" + newSubmissionID()
	read := func(step, answer string) problems.Problem {
		t.Helper()
		got := scheduledProblem(t, answer)
		want := scheduledProblem(t, f.request("GET", path, "alice", nil, 200))
		if got.ID != want.ID || got.Version != want.Version || !got.UpdatedAt.Equal(want.UpdatedAt) || !reflect.DeepEqual(got.Draft, want.Draft) ||
			got.PublishedVersion != want.PublishedVersion || got.Author != "alice" || want.Author != "alice" || got.ContestID != "" || want.ContestID != "" ||
			(got.ScheduledAt == nil) != (want.ScheduledAt == nil) || (got.ScheduledAt != nil && !got.ScheduledAt.Equal(*want.ScheduledAt)) {
			t.Fatalf("%s answered %+v, read %+v", step, got, want)
		}
		return got
	}
	p := read("create", f.request("PUT", path, "alice", map[string]any{"version": 0, "draft": d}, 200))
	if p.Version != 1 || p.PublishedVersion != 0 || p.ScheduledAt != nil {
		t.Fatal(p)
	}
	d.Markdown = "Saved statement"
	p = read("save", f.request("PUT", path, "alice", map[string]any{"version": p.Version, "draft": d}, 200))
	if p.Version != 2 || p.Draft.Markdown != "Saved statement" {
		t.Fatal(p)
	}
	at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	p = read("schedule", f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 200))
	if p.Version != 3 || p.PublishedVersion != 0 || p.ScheduledAt == nil || !p.ScheduledAt.Equal(at) {
		t.Fatal(p)
	}
	p = read("publish", f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true}, 200))
	if p.Version != 4 || p.PublishedVersion != 4 || p.ScheduledAt != nil {
		t.Fatal(p)
	}
	p = read("unpublish", f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": false}, 200))
	if p.Version != 5 || p.PublishedVersion != 0 || p.ScheduledAt != nil {
		t.Fatal(p)
	}
}
```

比べるのは`scan`が埋める値だけである。`Testers`、`ContestEditors`、`Readiness`、`EverPublished`は比べない。PUTの応答には`Testers`と`ContestEditors`が入らず、GETとは形が違う。これは今の仕様である。

- [ ] **Step 2: 変更前のコードで成功することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run 'TestProblemWriteResponsesMatchReadPostgres' ./internal/httpapi/ 2>&1 | grep -E -- '^(--- |ok|FAIL)'`
Expected: `--- PASS: TestProblemWriteResponsesMatchReadPostgres`と`ok  judge/api/internal/httpapi`。`--- SKIP`は出ない。失敗したらコードは変えずに止まり、出力を報告する。

- [ ] **Step 3: 定数を置き、6か所を書き換える**

1. `store.go`の`func (s *Store) Migrate(…)`の行と`func scan(`の間に、次のコメントと定数を置く。定数の値は、今の`store.go:231`の`SELECT `の直後から` FROM problem_drafts`の直前までを写す。手で打ち直さない。
   ```go
   // problemColumns are what scan reads, in its order. Their subqueries refer to problem_drafts by name, so a query that
   // reads them leaves the table without an alias.
   const problemColumns = `id, version, updated_at, draft, published_version,COALESCE(…),COALESCE(…),scheduled_at`
   ```
2. 置き換えは次の6か所。どれも、列リストの部分だけを定数に変える。その前後の文字は変えない。
   - Get（`store.go:231`）：`` `SELECT `+problemColumns+` FROM problem_drafts WHERE can_manage_problem(id,$1) AND id=$2` ``
   - INSERT（`store.go:373`）：`` `… ON CONFLICT (id) DO NOTHING RETURNING `+problemColumns, id, owner, data)) ``
   - UPDATE（`store.go:382`）：`` `… AND version=$3 RETURNING `+problemColumns, owner, id, version, data)) ``
   - Publishの取り下げ（`publication.go:69`）：`` query := `… AND version=$3 RETURNING ` + problemColumns ``
   - Publishの公開（`publication.go:71`）：`` query = `… AND version=$3 RETURNING ` + problemColumns ``
   - Schedule（`schedule.go:40-42`）：1行目（`UPDATE … version=version+1`）はそのまま残す。2〜3行目を`` WHERE id=$1 RETURNING `+problemColumns, id, at)) ``の1行にする。

   `+`の前後の空白は書き手が決めなくてよい。gofmtの結果に従う（計画者の試作では、関数の引数の中は空白なし、代入の右辺は空白ありになった）。

- [ ] **Step 4: gofmtとvetを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l internal/problems internal/httpapi`
Expected: 出力なし（nixの`Git tree ... is dirty`の警告は除く）

- [ ] **Step 5: 定数と変更前のSQLの一致を確かめる（一時ファイルはスクラッチに置き、コミットしない）**

1. 定数の値を取り出す。
   Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && sed -n 's/^const problemColumns = `\(.*\)`$/\1/p' api/internal/problems/store.go > /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c11/cols.txt && wc -l /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c11/cols.txt`
   Expected: `1`。このパスに書けないときは、自分のスクラッチディレクトリに置き換える。
2. 変更前のGet、INSERT、UPDATEとバイト単位で一致することを確かめる。
   Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git show HEAD:api/internal/problems/store.go | grep -cF -f /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c11/cols.txt`
   Expected: `3`
3. 変更前のPublishの2クエリとの違いが、`, `の空白だけであることを確かめる。
   Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && sed 's/, /,/g' /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c11/cols.txt > /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c11/cols-nospace.txt && git show HEAD:api/internal/problems/publication.go | grep -cF -f /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c11/cols-nospace.txt`
   Expected: `2`
4. 手書きの列リストが残っていないことを確かめる。
   Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && grep -rnF "owner_id=problem_drafts.owner_id),''" api/internal/problems/ && grep -c problemColumns api/internal/problems/store.go api/internal/problems/publication.go api/internal/problems/schedule.go`
   Expected: 1つ目のgrepは`store.go`の`const problemColumns`の1行だけ。2つ目は`store.go:5`（コメント、宣言、3か所の利用）、`publication.go:2`、`schedule.go:1`。
5. 差分を目で見て、前後の文字が変わっていないことを確かめる。
   Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git diff --word-diff=plain -- api/internal/problems/`
   Expected: 消えた語は列リストだけで、増えた語は`` `+problemColumns ``とその連結、定数の宣言とコメントだけ。WHERE句、SET句、引数に`[-…-]`や`{+…+}`が出ない。

- [ ] **Step 6: 関係するパッケージのDBテストを実行する**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v ./internal/problems/ ./internal/httpapi/ ./internal/contests/ 2>&1 | grep -E -- '--- FAIL|--- SKIP|^(ok|FAIL)'`
Expected: 3パッケージとも`ok`。`--- FAIL`は出ない。`--- SKIP`は`TestBrowserFixture`の1件だけ。計画者がoverlayで試作を実行したときは、この結果で、PASSは290件だった。

- [ ] **Step 7: 検証コマンドの1〜3を実行する**（後述）

- [ ] **Step 8: グラフを更新する**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`
`graphify-out/`は`.gitignore`の対象なので、コミットには含まれない。

コミットメッセージの案（オーケストレーター用）：

```
refactor: 問題を読む列のリストを定数にし、Get・保存・公開・予約のクエリで共有する

scanが読む8列を6つのクエリに書き写していたため、列を足すたびに
すべてを同時に直す必要があり、漏れはコンパイルを通った。列を
problemColumnsにまとめ、SELECTと各RETURNINGで使う。Scheduleの
contestIdは''からサブクエリに変わるが、行ロックの下で
コンテストに入っていないことを確かめており、コンテスト保存は
同じ行をロックしてscheduled_at IS NULLを条件にするため、常に''になる。
書き込み5経路の応答が直後のGETと一致することを特性テストで固定した。
```

## 検証コマンド

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 ./...` → すべて`ok`（または`no test files`）。`FAIL`は出ない。
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 終了コード0で、警告なし
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .` → 出力なし

## 完了条件

- Step 1のテストが、変更前のコードで成功している（Step 2）。
- Step 5の1〜5がすべて期待どおりである。
- Step 6と検証コマンドの1〜3がすべて成功している。DBテストの`--- SKIP`は`TestBrowserFixture`だけである。
- `git status --short -- api`に出るのは、次の4ファイルだけである。
  - `api/internal/problems/store.go`
  - `api/internal/problems/publication.go`
  - `api/internal/problems/schedule.go`
  - `api/internal/httpapi/scheduled_problems_test.go`
- レポートに「Scheduleの等価性」の根拠を書いている。Scheduleを外した場合は、その理由を書いている。

## 範囲外

- `list`（`summaryColumns`、`ownedSummaries`、`testedSummaries`）、`State`、`contestEditors`、`PublicGet`、`PublicList`、`featured*.go`のSQL
- 他のSQLの空白の不ぞろいをそろえること
- `scan`の形の変更（`submissions`のような`extra ...any`の導入など）や、`Problem`の構造体の変更
- SaveのRETURNINGのcontest_idが、直後の`UPDATE contest_problems`で常に上書きされて実質使われないこと。今回は削らない。
- PUTの応答に`Testers`と`ContestEditors`が入らず、GETと形が違うこと。今の仕様と考える。
- 公開済みコンテストに入った問題のPublishが返す`contestId`の特性テスト（判定の記録を参照）
- `posts`、`submissions`、`contests`の列定数

## レポートで触れる別件（直さない）

- 不具合は見つからなかった。
- SaveのRETURNINGのcontest_idの式は、`store.go:395-399`で常に上書きされる。読み出しとしては無駄だが、結果は正しい。
- PUTと公開・予約の応答には`testers`と`contestEditors`が含まれない。GETにだけ含まれる。既存の仕様と思われる。
