# C12 問題ページとコンテストの問題ページで下書きから写す処理を共有する 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `problems.Draft`から`problems.PublicProblem`へ9項目を写す処理を`func (p *PublicProblem) SetStatement(d Draft)`にまとめ、`problems.Store.PublicGet`と`contests.Store.Problem`の両方から呼ぶ。

**Architecture:** 先に、コンテストの問題ページ（開催中・終了後）と、終了後に公開された問題ページが同じ下書きから同じ9項目を出すこと、コンテスト側はタグを出さないことを、Postgresの特性テストで固定する（Task 1）。次に、写す処理を`problems`パッケージの公開メソッドに移す（Task 2）。EditorialとTagsは呼び出し側に残す。

**Tech Stack:** Go（`api`モジュール）、PostgreSQL（worktree専用、127.0.0.1:25432）、pgx v5

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C12.json`（worthとsafetyの`must_preserve`、`test_plan`、safetyの`revised_scope`）

## 困りごと

- 同じ9項目（Title、Markdown、Difficulty、TimeLimitMS、MemoryLimitMB、SpecialJudge=`d.Checker != nil`、Interactive=`d.Interactor != nil`、HasSamples=`d.HasSamples()`、Scoring=`d.PublicScoring()`）を写す処理が、`api/internal/problems/publication.go:122-132`と`api/internal/contests/store.go:476-484`にある（計画者が実コードで確認した。リポジトリ内でこの写しはこの2か所だけ）。
- 問題ページに項目を足した740404e（hasSamples）とddbc289（scoring）は、どちらも両方のファイルに同じ1行を足している。
- 片方を忘れても見つからない。`GET /contests/{id}/problems/{pid}`のテストが見ているのはscoringとeditorialだけで、title、markdown、difficulty、制限、specialJudge、interactive、hasSamples、タグを出さないことは確かめていない。`GET /problems/{id}`のhasSamplesにもHTTPのテストがない。

## 判定の記録

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| 特性テストの置き場所 | `contests_test.go`か近くの`contest_*_test.go` | 新しいファイルで`newTeamTest`を使う | 新しいファイル`api/internal/httpapi/contest_problem_statement_test.go`にする。`contests_test.go`は独自の準備を持つ1つの大きな関数で、無関係な状態と絡む。候補の`files`にある`contests_test.go`は変更しない |
| checkerとinteractorの付け方 | — | `f.exec`で`contest_problems.draft`をjsonbで書き換える | 3つの問題を最初から`f.store.Save`で保存する。`cpp17`のchecker/interactorはコンテストの検証を通る。SQLで検証を迂回しない。計画者がHEADのコピーで成功を確認した |
| `GET /problems/{id}`のspecialJudge、interactive | なければ1件足す | 既存のテストを流す | 別のテストは足さない。`submissions_test.go:639,696`が確かめている。新しいテストでも終了後の公開問題で確かめる |
| タスクの分割 | — | 特性テストを先に別コミット | 2つに分ける。Task 1だけを採ることもできる |
| メソッド名と置き場所 | `SetStatement` | `SetStatement` | `func (p *PublicProblem) SetStatement(d Draft)`を`publication.go`の`PublicGet`の直後に置く |
| docコメントの言語 | — | — | 英語。周りのコメントに合わせる。計画とレポートは日本語 |

## Global Constraints

- 公開HTTP API（ルート、JSONの形、状態コード、エラー文言）、DBスキーマ、依存のバージョン、CI・インフラを変えない。
- テストは追加だけにする。既存のテストは変更・削除しない。
- `PublicProblem`と`ContestProblem`のフィールド、JSONタグ、フィールドの順番を変えない（OpenAPIは`docs.go:22,30`がこの型から作る）。
- `SetStatement`が触るのは9項目だけ。ID、Author、PublishedAt、EditorialHidden、EditorialRevealAt、件数、難易度の分布、Testers、Tags、Editorialには触れない。
- `PublicGet`では、`p.Editorial = d.Editorial`と`p.Tags = d.Tags`を呼び出し側に残す。値はSQLのCASE（`can_manage_problem`、`featured_reveal_at`、`problem_tags_hidden`）で伏せた後のものをそのまま使う。Testersは`s.Testers(ctx, id)`のまま。
- `contests.Store.Problem`ではTagsを設定しない（ADR 0018:73）。Editorialは`if editorial && !p.EditorialHidden`の中だけで設定し、`CanReadEditorial`の式も変えない。Testersは`problems.New(s.Pool).Testers(ctx, pid)`のまま。
- どちらも`json.Unmarshal`の失敗は写す前にすぐ返す。`SetStatement`はその確認の後で呼ぶ。
- `PublicList`（`publication.go:138-157`）はSQLで項目を選ぶ別経路なので、`SetStatement`を使わない。一覧はScoringを返さないままにする。
- `contests`から`problems`への既存の依存以外に、パッケージ間の依存を増やさない。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターが行う。
- 別の実装者が`api/internal/submissions/`、`api/internal/httpapi/contest_freeze_test.go`、`contest_team_visibility_test.go`、`zz_c06_sql_test.go`を編集中である。これらのファイルには触れない。

## Review Focus

1. 将来タグやEditorialを`SetStatement`に入れると、コンテスト中に漏れる。→ Task 1でコンテストの問題ページ（匿名で開催中・終了後、オーナーで開催中）に`tags`キーがないことを確かめる。Task 2のStep 3で、`SetStatement`にTagsを足すと失敗することを確かめる。
2. Editorialの扱いを共有側へ動かすと、開催中の匿名の閲覧者に解説が出る。→ Task 1で開催中の匿名に`editorial`がなく、オーナーと終了後には出ることを確かめる。
3. omitemptyの項目（`specialJudge`、`interactive`、`scoring`）は、設定しない問題ではキーごと出ない。`hasSamples`は`false`でも必ず出る。→ Task 1の`omitted`と`want`。
4. 値レシーバーで書くと何も写らない。→ シグネチャをポインタレシーバーに固定する。Task 1の`want`がすべての項目を確かめる。
5. 問題ページとコンテストのページが、同じ下書きから同じ値を出すこと。→ Task 1で両方の経路を同じ`want`で比べる。

---

### Task 1: コンテストの問題ページの特性テストを追加する

**Files:**
- Create: `api/internal/httpapi/contest_problem_statement_test.go`

**Interfaces:**
- Consumes：`contest_teams_test.go`の`newTeamTest`、`f.store.Save`、`f.create`、`f.schedule`、`f.request`、`f.decode`
- Produces：`TestContestProblemStatementPostgres`。本番コードは変えない。

- [ ] **Step 1: テストを書く**

次の内容で作る。計画者がHEADのコピーで成功を確かめ、変異（コンテスト側でTagsを写す、Interactiveを写さない）で失敗することも確かめた。

```go
package httpapi

import (
	"reflect"
	"testing"

	"judge/api/internal/problems"
)

// TestContestProblemStatementPostgres pins what a contest problem page and the problem page published after the
// contest show from the same draft. The contest pages never show the tags (ADR 0018).
func TestContestProblemStatementPostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	const (
		plain       = "d5000000-0000-4000-8000-000000000001"
		checker     = "d5000000-0000-4000-8000-000000000002"
		interactive = "d5000000-0000-4000-8000-000000000003"
		cid         = "d6000000-0000-4000-8000-000000000001"
	)
	four, two := 4, 2
	judge := &problems.Generator{Runtime: "cpp17", Source: "int main(){}"}
	for id, d := range map[string]problems.Draft{
		plain: {Difficulty: &four, Title: "Plain", Markdown: "plain statement", Editorial: "plain editorial", Tags: []string{"DP"}, TimeLimitMS: "1500", MemoryLimitMB: "512",
			TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}, {Input: "3", Output: "4"}}},
		checker: {Difficulty: &two, Title: "Checker", Markdown: "checker statement", Editorial: "checker editorial", TimeLimitMS: "2000", MemoryLimitMB: "256", Checker: judge,
			TestCases: []problems.TestCase{{Input: "1", Output: "2"}}},
		interactive: {Difficulty: &two, Title: "Interactive", Markdown: "interactive statement", Editorial: "interactive editorial", TimeLimitMS: "1000", MemoryLimitMB: "128", Interactor: judge,
			TestCases: []problems.TestCase{{Input: "1", Output: ""}}},
	} {
		if _, err := f.store.Save(f.ctx, "alice", id, 0, d); err != nil {
			t.Fatal(err)
		}
	}
	// Both pages show these fields of the draft, and leave out the omitted keys.
	want := map[string]map[string]any{
		plain:       {"title": "Plain", "markdown": "plain statement", "difficulty": 4.0, "timeLimitMs": "1500", "memoryLimitMb": "512", "hasSamples": true},
		checker:     {"title": "Checker", "markdown": "checker statement", "difficulty": 2.0, "timeLimitMs": "2000", "memoryLimitMb": "256", "hasSamples": false, "specialJudge": true},
		interactive: {"title": "Interactive", "markdown": "interactive statement", "difficulty": 2.0, "timeLimitMs": "1000", "memoryLimitMb": "128", "hasSamples": false, "interactive": true},
	}
	omitted := map[string][]string{
		plain:       {"scoring", "specialJudge", "interactive"},
		checker:     {"scoring", "interactive"},
		interactive: {"scoring", "specialJudge"},
	}
	check := func(path, viewer, id string, extra map[string]any, absent ...string) {
		t.Helper()
		var got map[string]any
		f.decode(f.request("GET", path, viewer, nil, 200), &got)
		for _, fields := range []map[string]any{want[id], extra} {
			for k, v := range fields {
				if !reflect.DeepEqual(got[k], v) {
					t.Errorf("%s %s: got %#v want %#v", path, k, got[k], v)
				}
			}
		}
		for _, keys := range [][]string{omitted[id], absent} {
			for _, k := range keys {
				if v, ok := got[k]; ok {
					t.Errorf("%s: unexpected %s %#v", path, k, v)
				}
			}
		}
	}
	f.create(cid, plain, map[string]any{"problems": []map[string]any{{"id": plain, "points": 100}, {"id": checker, "points": 100}, {"id": interactive, "points": 100}}})
	f.schedule(cid, "running")
	for id := range want {
		check("/contests/"+cid+"/problems/"+id, "", id, nil, "tags", "editorial")
	}
	// The contest owner reads the editorial during the contest, but not the tags.
	check("/my/contests/"+cid+"/problems/"+plain, "alice", plain, map[string]any{"editorial": "plain editorial"}, "tags")

	f.schedule(cid, "ended")
	check("/contests/"+cid+"/problems/"+plain, "", plain, map[string]any{"editorial": "plain editorial"}, "tags")
	// The problem published at the end shows the same statement, now with its tags.
	check("/problems/"+plain, "", plain, map[string]any{"editorial": "plain editorial", "tags": []any{"DP"}})
	check("/problems/"+checker, "", checker, nil)
	check("/problems/"+interactive, "", interactive, nil)
}
```

`GET /problems/{id}`はリクエストの前に`contests.Store.Release`を走らせる（`routes.go:126`）ので、明示的なReleaseは要らない。

- [ ] **Step 2: 変更前のコードで成功することを確かめる**

Run: 検証コマンド1
Expected: `--- PASS: TestContestProblemStatementPostgres`。`SKIP`や`TEST_DATABASE_URL required`が出ないこと。

- [ ] **Step 3: 検証コマンド5（gofmt）を実行する** → 出力なし

コミットメッセージの案（オーケストレーター用）：`test: コンテストの問題ページと公開後の問題ページが下書きから出す項目を特性テストで固定する`

### Task 2: 写す処理をSetStatementにまとめる

**Files:**
- Modify: `api/internal/problems/publication.go:122-132`（`PublicGet`の中）。`PublicGet`の直後に`SetStatement`を追加する。
- Modify: `api/internal/contests/store.go:476-484`（`Store.Problem`の中）

**Interfaces:**
- Consumes：Task 1のテスト（安全網）
- Produces：`func (p *PublicProblem) SetStatement(d Draft)`（`problems`パッケージ、公開）

- [ ] **Step 1: `SetStatement`を追加し、`PublicGet`から呼ぶ**

本体は`PublicGet`の9行をそのまま移す。順番はTitle、Difficulty、Markdown、TimeLimitMS、MemoryLimitMB、SpecialJudge、Interactive、HasSamples、Scoring。`PublicGet`の`p.Title = d.Title`から`p.Scoring = d.PublicScoring()`までは、次のようにする。

```go
	p.SetStatement(d)
	p.Editorial = d.Editorial
	p.Tags = d.Tags
```

docコメントは英語で2〜3行にする。次の2点を含める。

- 問題ページが下書きから写す項目を、そのまま写すこと
- EditorialとTagsは写さず、呼び出し側に任せること。理由は、問題ページは閲覧者と定期便の解禁で伏せ、コンテストは日程で解説を出してタグを出さないこと（ADR 0018）

例：`// SetStatement copies what a problem page shows from the draft as it is. The editorial and the tags are left to the caller: the problem page hides them by viewer and by the 定期便 embargo, while a contest shows the editorial by its schedule and never the tags (ADR 0018).`

- [ ] **Step 2: `contests.Store.Problem`から呼ぶ**

`store.go:476-484`の9行を`p.SetStatement(d)`の1行に置き換える。その直後の`if editorial && !p.EditorialHidden { p.Editorial = d.Editorial }`とTestersの取得は変えない。

- [ ] **Step 3: タグを共有側に入れると検出されることを確かめる**

1. Editで`SetStatement`の末尾に`p.Tags = d.Tags`を一時的に足す。
2. 検証コマンド1を実行する。→ FAILし、`/my/contests/.../problems/d5000000-...-000000000001: unexpected tags`が出る。
3. Editで元に戻す。

- [ ] **Step 4: 検証コマンド1〜5を実行し、すべて成功することを確かめる**

- [ ] **Step 5: 差分を確かめ、グラフを更新する**

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git diff -- api/internal/problems/publication.go api/internal/contests/store.go`で、差分がStep 1-2の内容だけであることを確かめる。
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`（AGENTS.mdの規則。`graphify-out/`はgitignore対象）。

コミットメッセージの案：`refactor: 問題ページとコンテストの問題ページで下書きから写す項目をSetStatementにまとめる`

## 検証コマンド

TEST_DATABASE_URLは`postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable`。15432は使わない。

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run 'TestContestProblemStatementPostgres' ./internal/httpapi/` → `--- PASS`、SKIPなし
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -run 'ContestProblemStatement|Contest|PartialScoring|Tags|Featured|Submission|Docs|Samples|Testers|Scoring' ./internal/httpapi/ ./internal/problems/ ./internal/contests/` → 3行とも`ok`（計画者のコピーでは約25秒）
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 ./...` → すべて`ok`
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 出力なし
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l internal/problems/publication.go internal/contests/store.go internal/httpapi/contest_problem_statement_test.go` → 出力なし。あわせて`gofmt -l .`も実行する

別の実装者の編集中のファイルが原因で、コンパイルや2〜5が失敗することがある（失敗が`api/internal/submissions/`、`contest_freeze_test.go`、`contest_team_visibility_test.go`、`zz_c06_sql_test.go`に出る場合）。そのときはそれらを直さず、少し待って再実行する。それでも残るなら、レポートに記録する。

## 完了条件

- Task 1のテストが、変更前のコードで成功している。
- Task 2の後、検証コマンド1〜5がすべて成功している。Step 3の変異は検出され、元に戻してある。
- このタスクで変わったファイルは、`api/internal/problems/publication.go`、`api/internal/contests/store.go`、`api/internal/httpapi/contest_problem_statement_test.go`の3つだけ。

## 範囲外

- `PublicList`（SQLの経路）、`web/server/api/problems/[id].get.ts`のTS側の写し、`web/shared/types/problem.ts`
- `contests_test.go`ほか既存テストの書き換え
- difficultyがnullの公開問題のテスト（JSONタグで決まり、この変更では動かない）
- PublicGetやStore.Problemの他の部分（SQL、認可、エラーの対応づけ）の整理

## レポートで触れる別件（直さない）

- 項目を足すときは、Goの型に加えて`web/shared/types/problem.ts`と`web/server/api/problems/[id].get.ts`（非公開の問題を表示するために下書きから同じ項目を写す）も変える必要がある。不具合ではない。
