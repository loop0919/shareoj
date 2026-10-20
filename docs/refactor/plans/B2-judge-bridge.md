# B2 judge-bridge（D28、D08、D07）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** deployment-statusの数え方とコメントを送り出し側に合わせ、送り出しの先頭で恒久的に失敗する提出をJEにして後ろの提出を止めないようにする。

**Architecture:** 変えるのは`api/cmd/judge-bridge/bridge.go`の`deploymentStatus`と`dispatch.go`の`dispatchOne`だけ。JEにする処理は、今の`runtime_mismatch`のUPDATEを関数1つに出し、3か所から呼ぶ。テストは既存の`migratedDB`とhttptestのS3/SQSを使う。

**Tech Stack:** Go（`api`モジュール）、pgx v5、aws-sdk-go-v2（s3 v1.113.0、sqs v1.52.0）、PostgreSQL（テスト用DB）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/docs/refactor/FIXES.md`の「守ること」と、D07、D08、D28の「変更内容」。根拠は`/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/fixes/briefs/B2.md`。

## Global Constraints

- 「変更内容」に書かれた挙動だけを変える。公開API、DBスキーマ、依存のバージョン、インフラは変えない。
- 提出状態の遷移、重複・再実行時の挙動、ジャッジ結果は、D07の「恒久的な失敗をJEにする」以外は保つ。JEの内容は今の`runtime_mismatch`と同じ（`status='DONE'`、`finished_at=clock_timestamp()`、`result='{"verdict":"JE","passed":0,"total":0}'`）。
- 1件1コミット。変える挙動のテストを先に書き、変更前のコードで失敗することを確かめる。保つ挙動のテストは最初から通ってよい。
- テストの削除・スキップ・検証の緩和はしない。D28で古い挙動を固定していた1行だけは、新しい挙動へ書き換える（コミット本文にその旨を書く）。
- 範囲外の問題は直さずに報告する。コードのコメントは周りに合わせて英語、計画・ドキュメント・コミットは日本語。
- TEST_DATABASE_URLは`postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable`。15432は使わない。

## 裁定

- Ruling（D07の対象）: JEにするのは、(a) `job`列の`json.Unmarshal`の失敗と、(b) `ResolveTestFiles`が`problems.ErrTestFile`を返したとき（テストファイルの行がない・readyでない・サイズやsha256が一致しない）の2つだけ。どちらも行の内容で決まり、再試行しても結果は変わらない。それ以外（SELECT、`test_files`の読み取り、`runtime_rebind`、S3、SQS、`dispatched_at`の更新のエラー、`versioned job bucket required`）は基盤側の失敗なので、今のとおりエラーを返して再試行に任せ、`stage+"_failed"`の名前も変えない。`json.Marshal(request)`はデコードできたジョブでは失敗しないので触らない。
- Ruling（D07のログ）: reasonは`job_invalid`と`test_files_invalid`、categoryは`platform`。`runtime_mismatch`と同じ「対象_状態」の形にし、再試行する失敗の`outbox_read_failed`、`test_files_resolve_failed`と見分けられるようにする。`platform`なので、今の`platform`アラームで通知される。
- Ruling（D07のドキュメント）: `docs/judge/observability.md`の「JEの分類」の`platform`の行に2つのreasonを足す。この表はbridgeが出すJEをreason付きで載せている（`unscored_result`）ため。
- Ruling（D28のドキュメント）: `docs/judge/deployment-checks.md:59`の「廃止した言語IDの古い提出も、`-isolate`の提出として数える。」は変更後に誤りになるので書き換える。
- Ruling（D08のテスト）: コメントだけの変更なので失敗するテストはない。「実装を正とする」を守るため、3時間の上限がないことと終了15分後までを、保つ挙動のテストとして足す（最初から通る）。

## Review Focus

1. `test_files`の読み取りがDBエラーで失敗した（`ErrTestFile`ではない）→ JEにせず、提出は未送信のまま再試行。→ Task 3の`TestDispatchLeavesInfrastructureFailuresForRetry/test_file_lookup`。
2. S3（またはSQS）の失敗 → JEにせず再試行、reasonは今の`job_upload_failed`。→ Task 3の`TestDispatchLeavesInfrastructureFailuresForRetry/job_upload`。
3. JEにした回のログは`platform`の失敗1件だけ（`outbox_read_failed`などのstage名や`dispatch_failed`を重ねない）。同じ回のうちに後ろの提出が送られる。→ Task 3の`TestUnsendableSubmissionBecomesJEAndDispatchContinues`。
4. 一覧から外したランタイムの提出は`pending`と`undispatched`に数えず、一覧にあるランタイムの提出（送信済み・未送信）は今までどおり数える。→ Task 1で書き換える`checkDeployment`と、その前後の既存の`checkDeployment(1, 1)`、`(1, 0)`、`(0, 0)`、`TestRollingUpdateHoldsAndRebindsUndispatchedSubmissions`の`undispatched == 4`。
5. 開始から3時間を過ぎ、終了から15分以内のコンテストが`nextContestWindowAt`に入る。→ Task 2で足す確認。

---

### Task 1: D28 deployment-statusの未完了の提出を`runtime=ANY(...)`で数える

**Files:**
- Modify: `api/cmd/judge-bridge/bridge.go:52-56`
- Modify: `api/cmd/judge-bridge/main_test.go:371-374`（`checkOutboxAndResultIdempotency`）
- Modify: `docs/judge/deployment-checks.md:59`

- [ ] **Step 1: 古い挙動を固定していた確認を書き換える**

`main_test.go:374`の`checkDeployment(1, 1)`（`runtime='retired-language-isolate'`に変えた直後）を`checkDeployment(0, 0)`にし、`:371`の`UPDATE`の直前に次のコメントを置く。

```go
	// A runtime removed from the list is neither dispatched nor expired, so it must not hold a rollout either.
```

前後の`checkDeployment(1, 1)`（:370）、`(1, 0)`（:399）、`(0, 0)`（:475）はそのまま残す。これらが「一覧にあるランタイムは今までどおり数える」ことを守る。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run '^TestOutboxAndResultIdempotency$' ./cmd/judge-bridge/ 2>&1 | grep -E -- '--- |invalid or mutating|^(ok|FAIL)'`
Expected: 6つのサブテストすべてが`--- FAIL`。`invalid or mutating deployment status: {... "pending":1, ... "undispatched":1}`（`LIKE '%-isolate'`が`retired-language-isolate`を数えるため）。

- [ ] **Step 3: SQLを変える**

`bridge.go:56`の`WHERE runtime LIKE '%-isolate' AND status <> 'DONE'`を`WHERE runtime=ANY($1::text[]) AND status <> 'DONE'`にし、`QueryRow`に引数`submissions.IsolateRuntimeIDs()`を渡す（`dispatch.go:48`と同じ書き方）。コンテストの期間の副問い合わせは変えない。

`deployment-checks.md:59`を次の1行にする。

```
送り出しと期限切れと同じく、ランタイムの一覧（`submissions.Runtimes`）にある言語の提出だけを数える。
```

- [ ] **Step 4: 通ることを確かめる**

Step 2のコマンドで、6つのサブテストと`TestOutboxAndResultIdempotency`が`--- PASS`、最後に`ok`。あわせて`-run '^TestRollingUpdateHoldsAndRebindsUndispatchedSubmissions$'`も`--- PASS`。

- [ ] **Step 5: グラフを更新してコミットする**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .
git -C /home/loop/judge/.claude/worktrees/refactor-maintainability add api/cmd/judge-bridge/bridge.go api/cmd/judge-bridge/main_test.go docs/judge/deployment-checks.md
git -C /home/loop/judge/.claude/worktrees/refactor-maintainability commit -m "fix: deployment-statusの未完了の提出を送り出しと同じランタイムの一覧で数える" -m "一覧から外したランタイムの提出を数えていたテストの確認は、新しい挙動（数えない）に書き換えた。" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: D08 deployment-statusの期間のコメントを実装に合わせる

**Files:**
- Modify: `api/cmd/judge-bridge/bridge.go:48-49`
- Modify: `api/cmd/judge-bridge/main_test.go`（`TestRollingUpdateHoldsAndRebindsUndispatchedSubmissions`の末尾、:612の閉じ括弧の前）

- [ ] **Step 1: 期間を固定する確認を足す（保つ挙動。最初から通る）**

テスト関数の最後（`for name, want := range ...`のループの後）に足す。`contest`、`status`、`window`、`err`は同じ関数内で定義済み。

```go
	// Unlike the burst hold, the window has no 3-hour cap and lasts until 15 minutes after the end.
	contest("44444444-4444-4444-8444-444444444444", -4*time.Hour, -10*time.Minute, true)
	window, err = time.Parse(time.RFC3339Nano, status()["nextContestWindowAt"].(string))
	if err != nil || time.Since(window) < 4*time.Hour || time.Since(window) > 5*time.Hour {
		t.Fatalf("contest window without a cap: %v %v", window, err)
	}
```

期待値は開始の30分前（約4時間30分前）。burstの式（`LEAST(ends_at, starts_at+3時間)+15分`）なら45分前に閉じているので含まれず、2時間30分後の別のコンテストが返る。PostgreSQLが`-10m0s`を`-00:10:00`と読むことは確認済み。

- [ ] **Step 2: コメントを書き換える**

`bridge.go:48-49`の2行を次にする。

```go
		// A rolling update must not change the environment within a contest (ADR 0014).
		// The window runs from 30 minutes before the start to 15 minutes after the end of each contest
		// with a participant. Unlike the burst hold, it has no 3-hour cap and needs only one participant.
```

SQLは変えない。

- [ ] **Step 3: 通ることを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run '^TestRollingUpdateHoldsAndRebindsUndispatchedSubmissions$' ./cmd/judge-bridge/ 2>&1 | grep -E -- '^(--- |ok|FAIL)'`
Expected: `--- PASS`、`ok`。

- [ ] **Step 4: グラフを更新してコミットする**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .
git -C /home/loop/judge/.claude/worktrees/refactor-maintainability add api/cmd/judge-bridge/bridge.go api/cmd/judge-bridge/main_test.go
git -C /home/loop/judge/.claude/worktrees/refactor-maintainability commit -m "docs: deployment-statusの期間のコメントを実装に合わせ、期間をテストで固定する" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: D07 送り出せない提出をJEにして、次の提出へ進む

**Files:**
- Create: `api/cmd/judge-bridge/dispatch_test.go`
- Modify: `api/cmd/judge-bridge/dispatch.go:111-133`
- Modify: `docs/judge/observability.md:15`

**Interfaces:**
- Produces: `func finishAsJE(ctx context.Context, tx pgx.Tx, reason, id, attempt string) error`（`dispatch.go`。`observe("failure", "platform", reason, id, attempt)`の後に、今の`runtime_mismatch`のUPDATEを`tx`で実行し、そのエラーを返す）

- [ ] **Step 1: テストを書く**

`dispatch_test.go`（`package main`）に2つのテストを置く。準備は`rejudge_test.go`に合わせる：`migratedDB(t)`、`user_profiles`に`alice`、`runtime`は`python314-isolate`、`id`は`fmt.Sprintf("%08d-1111-4111-8111-111111111111", i)`、`created_at`は`clock_timestamp()+make_interval(secs=>i)`、`judge_attempt`は`id`と同じ。httptestのサーバーはPUTに`x-amz-version-id`を返し、SQSの本文から`submissionId`を`sent`に集める。ログは`slog`のJSONハンドラーで集める。

共通の小さな補助（テスト内のクロージャでよい）：
- `state(id)`：`SELECT status||'/'||COALESCE(result->>'verdict','')||'/'||(finished_at IS NOT NULL)||'/'||(dispatched_at IS NOT NULL) FROM submissions WHERE id=$1`
- `failures()`：ログのうち`event == "failure"`の行の`reason+"/"+submissionId`（`submissionId`がなければ空文字）を順に並べた`[]string`

`valid`は`submissions.Job{Image: digest, TimeLimitMS: 1000, MemoryLimitMB: 256, Cases: []submissions.Case{{Input: "1", Output: "1"}}}`。`missing`は同じジョブで、ケースを`{Input: "1", OutputFile: &problems.TestFile{ID: "33333333-3333-4333-8333-333333333333", Size: 12, SHA256: strings.Repeat("b", 64)}}`にしたもの。

(1) `TestUnsendableSubmissionBecomesJEAndDispatchContinues`。サブテストごとに新しいDBを使い、先頭（i=0）に壊れた提出、次（i=1）に`valid`を入れる。

| サブテスト | 先頭の`job` | 期待するreason |
| --- | --- | --- |
| `broken job` | `{"cases":"broken"}` | `job_invalid` |
| `missing test file` | `missing`（`test_files`に行を入れない） | `test_files_invalid` |

```go
if err := b.dispatch(ctx); err != nil {
	t.Fatal(err)
}
if got := state(head); got != "DONE/JE/true/false" {
	t.Fatalf("unsendable submission: %s", got)
}
if got := state(next); got != "QUEUED//false/true" {
	t.Fatalf("submission behind it: %s", got)
}
if fmt.Sprint(sent) != fmt.Sprint([]string{next}) {
	t.Fatalf("sent: %v", sent)
}
if got := failures(); fmt.Sprint(got) != fmt.Sprint([]string{tc.reason + "/" + head}) {
	t.Fatalf("failure logs: %v", got)
}
```

変更前に失敗する理由：`dispatchOne`が`json.Unmarshal`のエラー（`broken job`）や`problems.ErrTestFile`（`missing test file`）をそのまま返し、`send`と`dispatch`がエラーで終わるので、最初の`t.Fatal`で止まる。先頭はQUEUEDのまま、後ろの提出は送られない。

(2) `TestDispatchLeavesInfrastructureFailuresForRetry`（保つ挙動。最初から通る）。サブテストごとに新しいDBを使い、提出を1件だけ入れる。

| サブテスト | 提出の`job` | 失敗のさせ方 | 期待するreason |
| --- | --- | --- | --- |
| `job upload` | `valid` | サーバーがPUTに500を返す。s3クライアントは`o.RetryMaxAttempts = 1`（`capacity_test.go:235`と同じ） | `job_upload_failed` |
| `test file lookup` | `missing` | dispatchの前に`ALTER TABLE test_files RENAME TO test_files_unavailable` | `test_files_resolve_failed` |

```go
if err := b.dispatch(ctx); err == nil {
	t.Fatal("an infrastructure failure must fail the run")
}
if got := state(id); got != "QUEUED//false/false" {
	t.Fatalf("an infrastructure failure must leave the submission for retry: %s", got)
}
if len(sent) != 0 {
	t.Fatalf("sent: %v", sent)
}
if got := failures(); fmt.Sprint(got) != fmt.Sprint([]string{tc.reason + "/" + id, "dispatch_failed/"}) {
	t.Fatalf("failure logs: %v", got)
}
```

`test file lookup`で別の段階が先に失敗した場合は、reasonの確認で分かる。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run '^(TestUnsendableSubmissionBecomesJEAndDispatchContinues|TestDispatchLeavesInfrastructureFailuresForRetry)$' ./cmd/judge-bridge/ 2>&1 | grep -E -- '--- |^(ok|FAIL)|cannot unmarshal|invalid test file'`
Expected: `TestUnsendableSubmissionBecomesJEAndDispatchContinues`の2つのサブテストが`--- FAIL`（`json: cannot unmarshal string ...`と`invalid test file`）。`TestDispatchLeavesInfrastructureFailuresForRetry`の2つのサブテストは`--- PASS`。

- [ ] **Step 3: `finishAsJE`を足し、3か所から呼ぶ**

`dispatch.go`に`finishAsJE`を足す。コメントは1行で「再試行しても送れない提出を確定させ、後ろの提出を止めない」趣旨（英語）。`dispatchOne`を次のように変える。
- `json.Unmarshal(raw, &job)`の失敗 → `return finishAsJE(ctx, tx, "job_invalid", id, attempt)`
- `ResolveTestFiles`の結果を`err`で受け、`errors.Is(err, problems.ErrTestFile)`なら`return finishAsJE(ctx, tx, "test_files_invalid", id, attempt)`、それ以外の`err != nil`は今のとおり`return err`。ここに「`test_files`との不一致だけが恒久的で、読み取りの失敗は次の回に再試行する」趣旨の1行コメント（英語）を置く。
- `runtime_mismatch`の`observe`とUPDATE（:130-132）→ `return finishAsJE(ctx, tx, "runtime_mismatch", id, attempt)`

`stage`の代入、deferの`observe`、`send`のループ（JEにした回は`nil`が返ってコミットされ、同じ回のうちに次の行へ進む）は変えない。`runtime_mismatch`の挙動は`TestRollingUpdateHoldsAndRebindsUndispatchedSubmissions`（`unpublished`、`unknown`が`DONE`）が守る。

`observability.md:15`の`platform`の行の「発生箇所」を次にする。

```
isolateの起動と清掃、ランタイム照合、S3/SQS/DBの処理、bridgeが送り出せない提出をJEにした場合（壊れたジョブの`job_invalid`、テストファイルの不一致の`test_files_invalid`）、bridgeが部分点のジョブの得点のない結果をJEにした場合（`unscored_result`）
```

- [ ] **Step 4: 通ることを確かめる**

Step 2のコマンドで、4つのサブテストと2つのテストが`--- PASS`、`ok`。続けて`-run '^(TestOutboxAndResultIdempotency|TestRollingUpdateHoldsAndRebindsUndispatchedSubmissions|TestRejudgesWaitBehindNewSubmissions|TestPartialScoringJobsAndResults)$'`も`--- PASS`。

- [ ] **Step 5: グラフを更新してコミットする**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .
git -C /home/loop/judge/.claude/worktrees/refactor-maintainability add api/cmd/judge-bridge/dispatch.go api/cmd/judge-bridge/dispatch_test.go docs/judge/observability.md
git -C /home/loop/judge/.claude/worktrees/refactor-maintainability commit -m "fix: 送り出せない提出をJEにして、後ろの提出の送り出しを止めない" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## 検証コマンド

作業ディレクトリは`/home/loop/judge/.claude/worktrees/refactor-maintainability/api`。1〜4はcontrollerが全体で実行する。

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 ./...` → すべて`ok`（`./cmd/judge-bridge`を含む）
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 出力なし
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .` → 出力なし
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command make package package-judge` → エラーなしで終わる
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v ./cmd/judge-bridge/ 2>&1 | grep -E -- '^(--- |ok|FAIL)'` → `--- SKIP`と`--- FAIL`がない

## Out of scope（報告のみ）

- 一覧から外したランタイムの未完了の提出は、送り出しも期限切れの処理もされないので、`DONE`にならずに残る。D28で数えなくなるだけで、片付ける処理は足さない。
- `judge/rollout.py`の完了条件（`pending == undispatched == 0`）とローリング更新の`busy`の判定は変えない。数え方の変更は両方の件数に同じだけ効く。
- ローカルのjudge-worker（`api/cmd/judge-worker/main.go:49`）は、`ResolveTestFiles`の失敗をすでにJEにしている（DBエラーも含む）。B2の対象外なので変えない。
