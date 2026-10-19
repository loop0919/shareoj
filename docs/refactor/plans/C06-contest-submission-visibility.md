# C06 コンテスト提出の閲覧条件をcontestSubmissionVisibleにまとめる 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** コンテストの提出を見られる4条件（公開済み、スタッフ、終了後の本人、開始後のチームメイト）を、`contest.go`の`contestSubmissionVisible(contest string) string`に1か所でまとめる。`contestPublic`と`problemSubmissionVisible`はこの関数を呼ぶ。

**Architecture:** `onsiteParticipants(contest)`と同じ形で、SQL断片を返す関数を置く。`contestPublic`は`"$1::uuid"`を渡し、生成されるSQLは空白を除いて変更前と同じ文字列になる。`problemSubmissionVisible`は`"c.id"`を渡し、SQLの差は2か所の等価な書き換えだけになる。先に、問題側の経路を固定する特性テストを足す。

**Tech Stack:** Go（`api`モジュール）、PostgreSQL 17（worktree専用、127.0.0.1:25432）、pgx v5

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C06.json`（2つの反証の`revised_scope`、`must_preserve`、`test_plan`）

## 困りごと

- 同じ4条件が`api/internal/submissions/contest.go:64-66`（`contestPublic`）と`api/internal/submissions/problem.go:35-38`（`problemSubmissionVisible`）に別々に書かれている。違うのはスタッフとチームメイトの書き方だけで、意味は同じ。
- 規則を変えた直近のコミット（ee09fed、35a8d31、22ba8d3、4fb79e9。10/18〜10/19）は、どれも両ファイルの同じ条件を同時に直している。
- 片方だけ直すと、同じ提出の見え方が`/contests/{id}/submissions`系（`ContestGet`/`ContestList`。`/submissions/{id}`も`VisibleGet`経由で通る）と`/problems/{id}/submissions`系（`ProblemList`/`ProblemGet`）で変わる。伏せたはずの凍結後の提出が、もう片方の経路から見えてしまう。
- 規則の説明コメントも、`contest.go:55-57`と`problem.go:19-20`に重複している。

## 判定の記録（反証の差と、計画で決めた点）

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| 特性テストの追加 | 必須ではない | 5件を追加 | 追加する。問題側でコンテストを指定する経路（`$2`あり）では、終了後の凍結後の提出をスタッフ・本人・チームメイトが見られることを確かめるテストがない。2つの写しがずれるのはこの経路なので、コンテスト側と同じ期待値で固定する |
| EXPLAINでの確認 | 両経路 | 両経路 | `contestPublic`は文字列の一致で代える（SQLが同じなら計画も同じ）。問題側だけ、移行済みの空のスキーマでEXPLAINの前後を比べる |
| 断片でのスタッフ・チームメイトの書き方 | 副問い合わせの形 | 同じ | `(SELECT contest_staff(X,$3))`と`IN (SELECT unnest(contest_teammates(X,$3)))`に統一する。`contestPublic`の文字列を変えないためである |
| コードコメントの言語 | — | — | 英語。周りのコメントに合わせる。計画とレポートは日本語 |
| タスクの分割 | — | — | 1つにする。特性テストだけを採っても価値は小さい |
| 価値とリスク | 中〜高 | リスク低〜中 | 記録だけする。実際の取りこぼしはまだ起きていない |

## Global Constraints

- 公開HTTP API（ルート、JSONの形、ステータスコード、エラー文言）、DBスキーマとマイグレーション、依存のバージョン、CI・インフラを変えない。
- テストは追加だけにする。既存のアサーションは変えず、削除もスキップもしない。
- 断片が返すのは、括弧で包んだ4条件の論理和だけにする。順序は`contest_submission_public(c,submissions.created_at)`、スタッフ、終了後の本人、開始後のチームメイトで、今の`contest.go:64-66`と同じにする。
- 断片の中では、submissionsの列に必ず`submissions.`を付ける（`owner_id`、`created_at`）。cの列には`c.`を付ける。`contests`にも`owner_id`と`created_at`があり、修飾しないとcの列として解決されてしまう（22ba8d3と同じ不具合）。
- 断片の前提は、外側の表が別名なしの`submissions`、コンテスト行の別名が`c`、閲覧者が`$3`であること。この前提をdocコメントに書く。4つの呼び出し（`ContestGet`、`ContestList`、`ProblemList`、`ProblemGet`）は、どれも`$3`にviewerを渡している。
- `contestPublic`には`"$1::uuid"`を渡す。スタッフの判定とチームメイトの一覧は、非相関の副問い合わせとして一度だけ評価されるようにする。`problemSubmissionVisible`には`"c.id"`を渡す（`NULLIF($2,'')::uuid`ではない。`$2=''`のときは、行ごとにその提出のコンテストで判定する）。
- 呼び出し側に残すもの：
  - 結合条件：`contest_id=c.id AND`（contest.go）、`c.id=contest_id AND`（problem.go）
  - コンテスト外の提出に対するスタッフの枝（`contest.go:67-68`）と、`problem_drafts`の枝（`problem.go:32-34`）
  - Scopeの分岐（`problem.go:29-31`）
  - オンサイトの絞り込み（`$4`/`$6`）
  - `featured_reveal_at`の条件。問題側だけが`can_manage_problem`を含む（ADR 0011）
  - easyTest、generate、validateの除外
- 変えないもの：
  - 一覧の入口の判定（`ContestList`の`allowed`、`problemSubmissionsAccess`）
  - `teammate`定数（`contest.go:46`。`=ANY`のまま）と`teamSubmission`
  - `publicSubmission`と`submissionList`
  - `ORDER BY created_at DESC,id DESC`、`LIMIT 51`、`OFFSET`
  - 引数の番号と順序
- `git commit`、`git stash`、`git reset`は実行しない。コミットはオーケストレーターが行う。worktreeには`graphify-out/`がないので、`graphify update .`は実行しない。
- ステージ済みの`cli/`の変更（C01）には触れない。

## Review Focus

1. 断片の中で`owner_id`や`created_at`を修飾し忘れると、cの列として黙って解決される。その結果、終了後の本人に自分の凍結後の提出が見えなくなる。→ 既存の`TestContestFreezePostgres`の`submitter list`と`submitter problem list`、および新しいcarolの問題側アサーションで検出する。
2. 断片を括弧で包まないと、`contest_id=c.id AND A OR B ...`となる。スタッフ・本人・チームメイトの枝が結合条件から外れ、ほかのコンテストの提出まで見えてしまう。→ 文字列の一致の確認と、既存の`TestContestTeamVisibilityPostgres`の`expect(list, "alice", ...)`で検出する（tcのスタッフであるaliceの一覧に、ic・ncの提出が混ざる）。
3. 渡す値を取り違える場合。`contestPublic`に`c.id`を渡すと行ごとの評価になる。問題側に`NULLIF($2,'')::uuid`を渡すと、`$2=''`の経路でスタッフ・本人・チームメイトの判定が失われる。→ 文字列の一致の確認と、新しい`/my/problems/p1/submissions`（tess）のアサーションで検出する。
4. 隣の条件まで巻き込んで統一してしまう場合。対象は`featured_reveal_at`、コンテスト外の枝、`teammate`列、easyTest等の除外。→ 問題側のSQLの差が2か所の書き換えだけであることを、文字列の比較で確かめる。
5. 問題側の`=ANY(contest_teammates(c.id,$3))`を`IN (SELECT unnest(...))`に変えても結果は同じであること。`contest_teammates`は`ARRAY(...)`を返すのでNULLにならず、`owner_id`もNOT NULLである。→ 新しいチーム戦の問題側アサーションと、既存の`/my/problems/pb/submissions`のアサーションで検出する。

---

### Task 1: 閲覧条件の断片を共有し、問題側の経路を特性テストで固定する

**Files:**
- Test: `api/internal/httpapi/contest_freeze_test.go`（`staff list`の確認の直後、現在の327行の後）
- Test: `api/internal/httpapi/contest_team_visibility_test.go`（`missing("/my/problems/"+pb+"/submissions/"+carolFrozen, "sam")`の直後、現在の290行の後）
- Modify: `api/internal/submissions/contest.go:48-70`
- Modify: `api/internal/submissions/problem.go:18-41`
- 一時ファイル（コミットしない。完了前に削除する）：`api/internal/submissions/zz_c06_sql_test.go`

**Interfaces:**
- Produces: `func contestSubmissionVisible(contest string) string`（package `submissions`、`contest.go`の`onsiteParticipants`の直後）。`contest`はcのidを表すSQL式で、`"$1::uuid"`か`"c.id"`のどちらかを渡す。

- [ ] **Step 1: 特性テストを追加する（既存の補助関数だけを使う）**

`contest_freeze_test.go`には、終了後・結果の公開前の区間に3件を足す。既存の`ids`、`sorted`、`slices.Equal`、`t.Fatalf`の書き方に合わせる。

| 経路 | 閲覧者 | 期待するID集合 | 固定するもの |
| --- | --- | --- | --- |
| `"/my/contests/"+cid+"/problems/"+p1+"/submissions"` | `tess` | `sorted(bobWrong, bobAC, bobLate, carolAC, carolQueue, samEnd)` | 問題側（`$2`あり）で、スタッフに凍結後の他人の提出が見える |
| `"/my/contests/"+cid+"/problems/"+p1+"/submissions"` | `carol` | `sorted(bobWrong, bobAC, samEnd, carolAC, carolQueue)` | 問題側（`$2`あり）で、終了後の本人に自分の凍結後の提出が見え、bobLateは見えない |
| `"/my/problems/"+p1+"/submissions"` | `tess` | tessの1行目と同じ集合 | `$2=''`の経路で、行のc.idによりスタッフと判定される |

`contest_team_visibility_test.go`には、既存の`expect`と`problem(contest, p, query)`を使って2件を足す。

```go
expect(problem(tc, pb, ""), "bob", carolCE, danAC, carolFrozen)
expect(problem(tc, pb, ""), "sam", carolCE, danAC)
```

問題側（`$2`あり）でも、終了後のチームメイトには凍結後の提出が見えることを固定する。期待値は、直前にある`/my/problems/pb/submissions`の期待値と同じである。

- [ ] **Step 2: 変更前に特性テストを実行し、成功を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -run 'TestContestFreezePostgres|TestContestTeamVisibilityPostgres' ./internal/httpapi/`
Expected: `ok  judge/api/internal/httpapi`（skipではないこと。`-v`で`--- PASS`を確かめてもよい）

失敗した場合は、まず期待値をコンテスト側の同じ閲覧者の結果と照らし合わせる。コンテスト側と一致していれば、期待値の誤りとして直す。一致していなければ別件の不具合なので、コードは直さずに報告して止まる。

- [ ] **Step 3: 変更前のSQLとEXPLAINを一時テストで書き出す**

`api/internal/submissions/zz_c06_sql_test.go`に、`func TestC06DumpSQL(t *testing.T)`を一時的に置く。環境変数`C06_OUT`（出力ファイル名の接頭辞）がなければskipする。動作は次のとおり。

- `C06_OUT+".contest.sql"`に`contestPublic`を、`C06_OUT+".problem.sql"`に`problemSubmissionVisible`を書く。
- `TEST_DATABASE_URL`があれば、EXPLAINの結果も書き出す。
  - 専用のスキーマ`c06_explain_<UnixNano>`を作り、`search_path`を付けたURLで`database.Open`し、`database.Migrate`する。作り方は`contest_freeze_test.go:43-62`と同じで、最後に`DROP SCHEMA ... CASCADE`する。
  - 次の文を実行する。
    `EXPLAIN (COSTS OFF) SELECT `+columns+` FROM submissions WHERE `+problemSubmissionVisible+` ORDER BY created_at DESC,id DESC LIMIT 51 OFFSET $5`
  - 引数は`(problemID, contestID, "viewer", "", 0, false)`。`problemID`には固定のUUIDを使う。`contestID`は`""`と固定のUUIDの2通り試し、それぞれの各行を`C06_OUT+".explain-all.txt"`と`C06_OUT+".explain-contest.txt"`に書く。
  - 引数の型を推論できずに失敗した場合は、最初の引数に`pgx.QueryExecModeSimpleProtocol`を渡す。

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 600 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env C06_OUT=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c06-before TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -run TestC06DumpSQL -v ./internal/submissions/`
Expected: `--- PASS: TestC06DumpSQL`。scratchpadに`c06-before.contest.sql`、`.problem.sql`、`.explain-all.txt`、`.explain-contest.txt`の4ファイルができる。

- [ ] **Step 4: `contestSubmissionVisible`を追加し、2つの呼び出し側を置き換える**

- `contest.go`：`onsiteParticipants`の直後に`func contestSubmissionVisible(contest string) string`を置く。
  - 返す式は、今の`contest.go:64-66`で`contest_id=c.id AND `の直後から始まる括弧付きの4条件の論理和そのものにする。その中の`$1::uuid`（2か所）を`contest`に置き換える。
  - `contestPublic`の64-66行は、`(contest_id=c.id AND ` + `contestSubmissionVisible("$1::uuid")` + `)`にする。
- `problem.go`：35-38行を`OR EXISTS(SELECT 1 FROM contests c WHERE c.id=contest_id AND ` + `contestSubmissionVisible("c.id")` + `)`にする。
- docコメント（英語、周りと同じ密度）：
  - 新しい関数には次の3点を書く。
    - 規則：今の`contest.go:55-57`と`problem.go:20`の内容。スタッフ（作者・テスターを含む）、終了後の本人の凍結後の提出、結果の公開まで他人には伏せること、チームメイト（ADR 0024）。
    - 前提：cがコンテスト行、`submissions`は別名なし、`$3`が閲覧者。cにも`owner_id`と`created_at`があるので、submissionsの列は表名で修飾すること。
    - `contest`には、cのidを表す式を渡すこと。
  - `contestPublic`のコメント：
    - 55-57行は新しい関数に移す。
    - 58行は「`$1`を渡すので、閲覧者の役割とチームは行ごとではなく一度だけ判定される」という趣旨に書き換える。
    - 59行（`$4`）は残す。
    - コンテスト外の提出もスタッフは見られる、という1行を残してよい（55行が含んでいた内容）。
  - `problemSubmissionVisible`のコメント：
    - 18行、21-23行は残す。
    - 19行は下書きの実行を伏せる部分だけを残し、コンテストの提出は`contestSubmissionVisible`に従う、と書く。
    - 20行は削除する（新しい関数に移したため）。

- [ ] **Step 5: 変更後のSQLとEXPLAINを書き出し、変更前と比べる**

1. Run: Step 3と同じコマンドを、`C06_OUT=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c06-after`にして実行する。
   Expected: `--- PASS: TestC06DumpSQL`
2. Run:
   ```
   python3 -I -c 'd="/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c06-"
   r=lambda n: "".join(open(d+n).read().split())
   assert r("after.contest.sql")==r("before.contest.sql"), "contestPublic changed"
   want=r("before.problem.sql").replace("contest_staff(c.id,$3)","(SELECTcontest_staff(c.id,$3))",1).replace("=ANY(contest_teammates(c.id,$3))","IN(SELECTunnest(contest_teammates(c.id,$3)))",1)
   assert r("after.problem.sql")==want, "problemSubmissionVisible differs beyond the two rewrites"
   print("ok")'
   ```
   Expected: `ok`
3. Run: `diff /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c06-before.explain-all.txt /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c06-after.explain-all.txt` と、同じ形で`explain-contest.txt`
   Expected: 差は、コンテスト行の条件の中で`contest_staff`（または、インライン展開された`contest_setter ... OR contest_problem_staff ...`）が`(SubPlan n)`になることと、`= ANY (contest_teammates(...))`がSubPlanを使う形になることだけ。ほかに、SubPlanの番号がずれる。`submissions`と`contests`に対するScanノードの種類と順序、`Limit`/`Sort`は変わらない。それ以外の差が出たら止まって報告する。
4. 一時ファイルを消す。Run: `rm /home/loop/judge/.claude/worktrees/refactor-maintainability/api/internal/submissions/zz_c06_sql_test.go`

- [ ] **Step 6: 全体を検証する**

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -run 'TestContestFreezePostgres|TestContestTeamVisibilityPostgres|TestContestTeamIndividualsPostgres|TestContestRolesPostgres|TestContestsPostgres|TestSharedSubmissionVisibilityPostgres|TestSubmissionsPostgres|TestTesterInvitationsPostgres|TestFeatured' ./internal/httpapi/` → `ok`
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 ./...` → すべて`ok`（または`no test files`）。`FAIL`がないこと
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 出力なし、終了コード0
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .` → 出力なし
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short` → 新たに変わったのは`api/internal/submissions/contest.go`、`api/internal/submissions/problem.go`、`api/internal/httpapi/contest_freeze_test.go`、`api/internal/httpapi/contest_team_visibility_test.go`だけ。`zz_c06_sql_test.go`が残っていないこと。ステージ済みの`cli/`3ファイルは開始時のまま

コミットメッセージの案（オーケストレーター用）：`refactor: コンテスト提出の閲覧条件をcontestSubmissionVisibleにまとめ、コンテストと問題の提出一覧で共有する`

アカウント系のPlaywright（`web`、`playwright.account.config.ts`）は、検証者が後で実行する。

## 完了条件

- 追加した5件の特性テストが、変更前（Step 2）と変更後（Step 6）の両方で成功している。
- Step 5の文字列比較が`ok`を出す。`contestPublic`は空白を除いて変更前と同じで、`problemSubmissionVisible`の差は2か所の書き換えだけである。
- EXPLAINの差が、Step 5-3で認めた範囲に収まっている。
- `go test ./...`（DB連携あり）、`go vet ./...`が成功し、`gofmt -l .`が何も出さない。
- 4条件がコードに書かれているのは`contestSubmissionVisible`の1か所だけで、規則の説明コメントもそこにまとまっている。
- 一時テストファイルが削除されている。

## 範囲外

- SQL関数（例：`contest_submission_visible`）への移行。マイグレーション、つまりDBスキーマの変更が要る。
- `teammate`定数（`contest.go:46`の`=ANY`）、`teamSubmission`、compileLogの扱い。
- `featured_reveal_at`の条件の差。問題側だけが`can_manage_problem`を含む。これはADR 0011に基づく仕様であり、不具合だとしても別件の仕様判断になる。
- 一覧の入口の判定：`ContestList`の`allowed`、`problemSubmissionsAccess`、`contests.Store`の`CanViewSubmissions`。規則が違う。
- 次のものも対象にしない。
  - easyTest・generate・validateの除外3行の重複
  - オンサイトの絞り込み
  - Scopeの分岐
  - `problems/publication.go:97`と`contests/store.go:239`の別の用途の条件
- ステージ済みの`cli/`の変更（C01）。
