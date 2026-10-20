# C20 マイグレーションの一覧を関数に出し、ファイル番号との対応をテストで固定する 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `Migrate`の中にある40要素の1行リストを非公開関数`func migrations() []string`に移す。あわせて、`NNN_*.sql`が001から欠番なく並び、`migrations()`の各要素と順に一致することを、DB不要のテストで確かめる。

**Architecture:** embed変数40個と`Migrate`の処理はそのまま残す。変えるのは、ループの`range`の式だけ。リストは1行1要素で関数に移し、運用規則を短いコメントで書く。新しいテストはパッケージ内（`package database`）に置き、ディスク上のSQLファイルを正として`migrations()`と比べる。

**Tech Stack:** Go（`api`モジュール、go 1.26）、標準ライブラリの`os`、`regexp`、`strconv`、`strings`、`testing`

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C20.json`（worthとsafetyの`must_preserve`、`test_plan`、`revised_scope`）。controllerの裁定：範囲は(1)だけ。

## 困りごと

- `api/internal/database/database.go:161`に、40要素の1行リストがある。`:162`の`version := index + 1`で、versionはリスト上の位置で決まる（計画者が実コードで確認した）。
- 「version＝ファイル番号＝リストの位置」は運用の前提になっている。たとえば`019_reserved.sql`は、取り消したversion 19を予約している。`profile_test.go:430`の`version=11`も番号に依存している。ところが、この前提を機械で確かめる仕組みがない。
- 途中への挿入や、ファイル番号の飛ばしには気づけない。新規スキーマへ全件を適用する既存テストでは、どちらも通ってしまう。
- マイグレーションを足すたびに、1行全体の置き換えが差分に出る。そのため、レビューで順序を追えない（`database.go`を変えたコミットは38本ある）。

## 判定の記録

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| embed.FSへの置き換え（brief の(2)） | 採らない | 別コミットで後から判断 | 採らない（controllerの裁定）。embed変数40個は残す |
| DB付きの特性テスト（`schema_migrations`が{1..N}） | 挙げていない | ステップ1のc)で足す | 足さない。controllerの裁定は、DB不要のテストだけを挙げている。`version := index + 1`とループ本体は変えない。番号と内容の対応は、既存のテストが挙動で確かめている（`TestProfilesPostgres`はversion 11を、`TestContestsPostgres`は13、20、35、37、39を再適用する）。これらを検証コマンド2で、SKIPされずに通ることまで確かめる |
| リストの書き方 | 元の`:161`と文字単位で同じ | 要素も順序も変えない | 1行1要素にする。差分レビューで追えないことが困りごとなので、次の追加が1行の追加で済む形にする。要素と順序が同じことは、識別子の列を比べて確かめる（Step 4） |
| テストを変更前のコードで流せるか | 変更前でも成り立つ形 | 切り出しの前でもPASS | 流せない。`migrations()`がないので、変更前はコンパイルが通らない。これをTDDの「失敗するテスト」とする。切り出しが同値であることは、識別子列の比較と、既存DBテストの変更前後の実行で示す |
| コメントの言語 | — | — | 英語。周りのGoのコメントに合わせる。計画とレポートは日本語で書く |

## Global Constraints

- DBスキーマ、各SQLファイルの内容とファイル名、適用順、versionの番号（＝ファイル番号＝index+1）を変えない。`019_reserved.sql`も残す。`git diff --stat`に`*.sql`が出ないこと。
- `Migrate(ctx context.Context, pool *pgxpool.Pool) error`のシグネチャを変えない。ループ本体も、`range`の式以外は1文字も変えない。対象は次のとおり。
  - 1回の`pool.Begin`と`defer` Rollback
  - `SELECT pg_advisory_xact_lock(709091001)`
  - `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint PRIMARY KEY)`
  - EXISTSで確かめて、適用済みなら飛ばすこと
  - 引数なしの`tx.Exec(ctx, schema)`
  - `INSERT INTO schema_migrations VALUES ($1)`
  - 最後の`tx.Commit`
  - エラーは包まずにそのまま返すこと
- embed変数40個（`//go:embed NNN_*.sql`の個別指定）は、名前も順番もそのまま残す。`//go:embed *.sql`のglobは使わない。
- `init`、パッケージ変数の初期化、`panic`による検証は入れない。`database`パッケージはAPI Lambda、judge-bridge、judge-workerからもimportされている。
- `cmd/migrate-lambda/main.go`と`cmd/migrate/main.go`、`Open`、`OpenConfigured`、`config.go`、`go.mod`、`go.sum`、CI、インフラには触れない。
- テストは追加だけにする。既存のテストは変更・削除しない。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターが行う。
- worktreeには、別サイクル（C04）でstage済みの`web/`の変更がある。`web/`には触れない。npm、Playwright、webのビルドは実行しない。

## Review Focus

1. 移すときの要素の欠落、重複、並べ替え（とくに`reservedSchema`の位置）。→ Step 4で識別子列を比べる。Step 5の変異（19と20の入れ替え、末尾の削除）で、テストが失敗することを確かめる。
2. 末尾の書き忘れ。ファイルは足したのに、リストに足していない場合。→ テストの件数の比較で検出する。Step 5で、未登録の`041_*.sql`を一時的に置いて確かめる。
3. ファイル番号の飛ばしや重複。たとえば予約ファイルを置かずに、次の番号を使う場合。→ テストの番号の確認で検出する。Step 5で、040を041に改名して確かめる。
4. ディレクトリにある`.sql`以外のファイル（`rds-ca-bundle.pem`、`*.go`）を拾わないこと。→ 拡張子`.sql`だけで絞る。これはStep 3が現状のツリーでPASSすることで確かめられる。
5. `Migrate`の挙動が変わらないこと（新規スキーマへの全件適用、2回目の冪等性、途中のversionの再適用）。→ 検証コマンド2で、既存のDBテストが`--- PASS`になり、SKIPされないことを確かめる。

---

### Task 1: リストを`migrations()`に移し、ファイル番号との対応をテストする

**Files:**
- Create: `api/internal/database/database_test.go`（`package database`、DB不要）
- Modify: `api/internal/database/database.go:149-161`（`Migrate`の直前に`migrations()`を追加し、`:161`の`range`の式を置き換える）

**Interfaces:**
- Produces: `func migrations() []string`（非公開。`Migrate`とテストだけが使う）

- [ ] **Step 1: 変更前の既存テストを流しておく**

検証コマンド2を実行する。→ 対象の6テストがすべて`--- PASS`になり、`--- SKIP`は出ない。

- [ ] **Step 2: テストを書く**

`api/internal/database/database_test.go`を次の内容で作る。計画者は、HEADのコピーで切り出しを行った後にPASSすることを確かめた。Step 5の4つの変異で失敗することも確かめた。

```go
package database

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestMigrationsFollowFileNumbers pins that version N applies NNN_*.sql. Deployed databases record versions by
// number, so the files must be numbered from 001 without gaps and migrations() must list each one at its number.
func TestMigrationsFollowFileNumbers(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	numbered := regexp.MustCompile(`^[0-9]{3}_[a-z0-9_]+\.sql$`)
	for i, file := range files {
		if !numbered.MatchString(file) {
			t.Errorf("%s is not named NNN_name.sql", file)
		} else if n, _ := strconv.Atoi(file[:3]); n != i+1 {
			t.Errorf("%s is file %d; number the files from 001 without gaps or duplicates", file, i+1)
		}
	}
	list := migrations()
	for i := 0; i < len(list) && i < len(files); i++ {
		data, err := os.ReadFile(files[i])
		if err != nil {
			t.Fatal(err)
		}
		if list[i] != string(data) {
			t.Errorf("migrations()[%d] (version %d) is not %s", i, i+1, files[i])
		}
	}
	if len(list) != len(files) {
		t.Errorf("migrations() lists %d schemas for %d .sql files", len(list), len(files))
	}
}
```

`os.ReadDir`は名前順に返す。`go test`はパッケージのディレクトリで実行されるので、`"."`でSQLファイルを読める。

- [ ] **Step 3: 失敗を確かめ、`migrations()`を実装して成功させる**

1. 検証コマンド1を実行する。→ `undefined: migrations`でビルドが失敗する。
2. `database.go`の`func Migrate`の直前に、次を追加する。
   - `func migrations() []string { return []string{...} }`
   - 中身は、`:161`のリストと同じ40個の識別子。順序も同じにし、1行1要素で末尾にカンマを付ける。
   - 直前のコメント（英語、3行程度）には、次の3点を書く。
     - versionはリスト上の位置で決まり、N番目の要素は`NNN_*.sql`であること
     - 新しいマイグレーションは、次の番号で末尾に足すこと
     - 適用済みのversionを記録したDBがあるので、要素を消したり並べ替えたりしないこと。取り消したマイグレーションは、`019_reserved.sql`のように番号を予約して残すこと

   例：

   ```go
   // migrations lists the schemas in version order: version N is the Nth entry, from NNN_*.sql.
   // Add a migration at the end under the next number. Deployed databases record the versions they applied, so never
   // remove or reorder an entry; a reverted migration keeps its number reserved, like 019_reserved.sql.
   ```

   コメントには`Schema`という語を入れない。Step 4の識別子の抽出に混ざるためである。
3. `Migrate`の`for index, schema := range []string{...} {`を、`for index, schema := range migrations() {`に置き換える。ループ本体には触れない。
4. 検証コマンド1を実行する。→ `--- PASS: TestMigrationsFollowFileNumbers`

- [ ] **Step 4: 移したリストがHEADと同じことを確かめる**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git show HEAD:api/internal/database/database.go | grep 'range \[\]string{' | grep -o '[A-Za-z]*Schema' > .local/refactor/C20/list-before.txt
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && sed -n '/^func migrations/,/^}/p' api/internal/database/database.go | grep -o '[A-Za-z]*Schema' > .local/refactor/C20/list-after.txt
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && diff .local/refactor/C20/list-before.txt .local/refactor/C20/list-after.txt && wc -l < .local/refactor/C20/list-after.txt
```

Expected: `diff`は何も出さず、`40`が出る（`.local/`はgitignore対象）。

続いて`git diff -- api/internal/database/database.go`を見る。差分が次の3つだけであることを確かめる。
- コメント付きの`migrations()`の追加
- `-`の1行（元の`range []string{...}`）
- `+`の1行（`range migrations()`）

- [ ] **Step 5: テストが誤りを捕まえることを確かめる（変更はコミットせず、必ず戻す）**

次の変異を1つずつ入れる。それぞれについて検証コマンド1を実行して`FAIL`を確かめ、すぐに元に戻す。

| 変異 | 期待するメッセージ |
| --- | --- |
| `migrations()`の`reservedSchema`と`contestParticipantsSchema`を入れ替える | `migrations()[18] (version 19) is not 019_reserved.sql` |
| `migrations()`の末尾の`submissionFullScoreSchema,`を消す | `migrations() lists 39 schemas for 40 .sql files` |
| `SELECT 1;`だけを書いた`api/internal/database/041_forgotten.sql`を置く | `migrations() lists 40 schemas for 41 .sql files` |
| `040_submission_full_score.sql`を`041_…`に`mv`し、その`//go:embed`行も合わせて直す | `041_submission_full_score.sql is file 40; …` |

戻した後に、次の2つを確かめる。
- `git status --short api/internal/database`に出るのが、`database.go`（M）と`database_test.go`（??）だけであること
- 検証コマンド1がPASSすること

- [ ] **Step 6: 検証コマンド2〜6を実行し、すべて成功することを確かめる**

- [ ] **Step 7: グラフを更新する**

`cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`を実行する（AGENTS.mdの規則。`graphify-out/`はgitignore対象）。

コミットメッセージの案（オーケストレーター用）：`refactor: マイグレーションの一覧をmigrations()に出し、ファイル番号との対応をテストで固定する`

## 検証コマンド

TEST_DATABASE_URLは`postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable`を使う。15432は使わない。

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 600 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 -v -run TestMigrationsFollowFileNumbers ./internal/database/` → `--- PASS: TestMigrationsFollowFileNumbers`
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run '^(TestRegistrationQuota|TestProfilesPostgres|TestContestsPostgres|TestPrivateProblemsPostgres|TestCapacityRunsOnlyOnScheduleAndReadsDemand|TestExactLimitUploadCompletionAndDownload)$' ./internal/database/ ./internal/httpapi/ ./cmd/judge-bridge/ ./internal/testfiles/ 2>&1 | grep -E '^(--- |ok|FAIL)'` → 6つの`--- PASS`と4行の`ok`が出る。`--- SKIP`は出ない
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 ./...` → すべて`ok`（`migratedDB`を使うjudge-bridgeのテストも含む）
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 出力なし
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .` → 出力なし
6. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command make package package-judge` → エラーなしで終わり、`api/.build/`に`api.zip`、`migrate.zip`、`judge-bridge.zip`、`judge-notify.zip`ができる（`.build/`はgitignore対象）。別サイクルのwebビルドやPlaywrightと重ならない時間に実行する

## 完了条件

- Step 3の1でビルドが失敗し、Step 3の4以降でテストが成功している。
- Step 4で、識別子列の差分がなく、件数が40になっている。
- Step 5の4つの変異をすべて検出し、元に戻してある。
- 検証コマンド1〜6がすべて成功している。
- このタスクで変わったファイルは、`api/internal/database/database.go`と`api/internal/database/database_test.go`の2つだけ（`.local/`の作業ファイルを除く）。

## 範囲外

- `//go:embed *.sql`とembed.FSへの置き換え、`Migrate`での名前の検証（briefの(2)）
- DB付きの新しい特性テスト（判定の記録を参照）
- `httpapi/profile_test.go:414`のDROP列。マイグレーションのたびに直す箇所だが、既存テストの失敗で気づける
- `infra/README.md`の手動`make -C api package`の手順
- embed変数の名前や並び、`Open`と`config.go`の整理

## レポートで触れる別件（直さない）

- 取り消したマイグレーションの番号を、別のファイルが再利用した場合は検出できない（例：`019_contest_participants.sql`）。ファイル番号とリストの位置が一致するので、このテストは通ってしまう。そのため、規則を`migrations()`のコメントに書くだけにとどめる。
