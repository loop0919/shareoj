# C08 問題とプロフィールのストアを具体型で持ち、ハンドラーごとの型アサーションをやめる 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `handlerDependencies.Store`、`problemHandler.Store`、`submissions.Service.Problems`を`*problems.Store`にする。17か所の実行時の型アサーションを、`p.Store == nil`の判定か直接の呼び出しに置き換える。使われなくなる`problems.Repository`、`problems.Publications`、`problemReadiness`を削除する。続けて、プロフィールのストアにも同じ変更をする（Task 3）。

**Architecture:** 先に、DBなしで各ルートが返すステータスとエラーコードを表テストで固定する。生成の提出がtyped nilでpanicしないことも、別のテストで固定する（Task 1）。次に問題ストアの型を変える（Task 2）。最後にプロフィールストアの型を変える（Task 3）。Task 3は単独で不採用にできる。同じパッケージの`imageHandler`、`notificationHandler`、`tagHandler`は、すでに具体型と`p.Store == nil`の判定を使っている（`notifications.go:6`）。この変更で、問題とプロフィールもその形にそろう。

**Tech Stack:** Go（`api`モジュール）、PostgreSQL（テストDB `127.0.0.1:25432/openoj_test`）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C08.json`（worthとsafetyの`revised_scope`、`must_preserve`、`test_plan`）

## 困りごと

- `problemHandler.Store`は`problems.Repository`（Get/List/Save/Deleteだけ）として宣言されている。しかし実際に渡されるのは、本番（`config.go:104-105`）でもテストでも、`problems.New`か`problems.Open`で作った`*problems.Store`だけである。偽物の実装はない。そのため各ハンドラーは、使うメソッドごとに型アサーションと503の分岐を書いている。`git log -S "p.Store.("`で数えると、新しいエンドポイントを足すたびにこの手順が写されてきたことが分かる（14コミット）。
- アサーションの先は、`*problems.Store`、匿名インターフェース、`problems.Publications`、`problemReadiness`である。`*problems.Store`のメソッドのシグネチャを変えて、写しの側を直し忘れても、コンパイルは通る。その場合、本番ではそのルートが常に503になる。`problems.Publications`は、d831f72と7a038aaでメソッドと一緒に書き換えられている。気づけるのはDBありのテストだけである。
- `withReadiness`（`private_problems.go:47`）と一覧のreadiness（`:231`）では、アサーションが失敗しても503にならない。readinessが黙って応答から消えるだけである。
- `problemReadiness`のコメント「test fakes may omit it」は事実と異なる。そのような偽物は、導入時（8e5c36a）からない。
- 価値はmedium（worthの見直しに従う）。CIはDBありでテストするので、得られるのは主に、コンパイル時に気づけるようになることと、写す手順がなくなることである。

## 判定の記録

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| (b) プロフィール | 任意（効果が小さい） | 別コミットで実施 | **Task 3として別に実施する。** Task 2の後、`handlerDependencies`の中でインターフェースのまま残るのは`Files`（偽物が実在する）と`Profiles`だけになる。`profile.go:135`も同じ匿名インターフェースの形である。差分は小さい。typed nilは既存の`TestUnrelatedRoutesDoNotReleaseContests`（`/auth/me`→200、`/my/profile`→503）で検出できる。レビューでTask 3だけを不採用にしてもよい |
| `submissions.Service.Problems` | `*problems.Store`を推奨 | `*problems.Store` | **`*problems.Store`にする。** 条件付きの代入は採らない。これで`problems.Repository`の利用者がいなくなるので、型ごと削除する |
| Storeがnilでないと保証済みの箇所 | アサーションを削除するだけ | 同じ | 同じ。`withReadiness`、`list`のtester分岐、`list`のreadiness、`publishProblem`のSchedule。新しいnil判定は置かない |
| 特性テストの置き場所 | — | httpapiパッケージ | `routes_test.go`に2つの関数を足す。DBなしのルートのテストと`staticVerifier`がこのファイルにある |
| 特性テストのContests | nilにする | nil | nil。`&contests.Store{}`を使うと、release=trueのルートが`releaseBefore`でpanicする |
| エラーコードの照合 | — | — | JSONを読んで`error`を完全一致で比べる。部分一致では`not_found`と`problem_not_found`を区別できない |
| importが1つだけになるファイル | — | — | `favorites.go`、`difficulty_votes.go`、`solved_problems.go`は`import "net/http"`にする。`notifications.go:3`と同じ書き方 |
| `make package package-judge` | — | — | 実行する。`api/Makefile:3,12`にある（C02の計画はルートの`Makefile`を見て「ない」としているが、誤り）。CI（`ci.yml:151`）も実行する |

計画者が確かめたこと（いずれも`go test -overlay`でスクラッチのファイルを差し込んだ。リポジトリのファイルは変えていない）:
- HEADの`*problems.Store`と`*profiles.Store`が、すべてのアサーション先（6つの匿名インターフェース、`Publications`、`problemReadiness`、両方の`Repository`）を満たすことを、コンパイル時の代入で確かめた。ストアがあればアサーションは必ず成功する。したがって、直接の呼び出しに変えても挙動は同じである。
- Task 1の2つのテストは、HEADで成功する（35件の表とgenerationのテスト）。
- Task 2とTask 3を当てた試作で、`go build ./...`と`go vet`が通った。DBなしのテストと、関係するDBテスト33件も成功した（`TestBrowserFixture`だけは常にSKIP）。
- 変異を入れると失敗することも確かめた。
  - `Service.Problems`を`problems.Repository`に戻すと、`TestGenerationWithoutProblemStore`がnilの`(*Store).Get`でpanicする。
  - `authentication.Profiles`をインターフェースに戻すと、`TestUnrelatedRoutesDoNotReleaseContests`がpanicする。
  - favoritesのnil判定をIDの検証より前に動かすと、表の`GET /my/favorites/bad`で失敗する。

## Global Constraints

- 公開HTTP API（ルート、JSONの形、ステータスコード、エラー文言、`Cache-Control`/`X-Content-Type-Options`）、CLI、DBスキーマとマイグレーション、依存のバージョン、インフラとCIは変えない。
- テストは追加だけにする。既存のテストは変更・削除・Skipしない。DBテストのSkipは成功に数えない。
- 呼び出すストアのメソッド、引数、エラー変換（`problemError`）は変えない。認可、公開範囲、状態遷移も変えない。
- 置き換えるnil判定は、元のアサーションと同じ位置に置く。
  - 入力検証が先、503が後: publishedProblems（catalogueFilterの400 → IDの404 `not_found` → 503 → cursorの400）、publishProblem（IDの404 `problem_not_found` → 503 → 本文の415/400）、favorite（404 `not_found` → 503 → 本文）、difficultyVote（404 `problem_not_found` → 503 → 本文）、publicSamples（404 `not_found` → 503）、myFeaturedSchedule（problemの404 → preferenceの400 → 503）、featuredStandings（atの400 → offsetの400 → 503）、publicProfile（handleの404 → 503）。
  - 503が先: featured（offsetより前）、featuredApplication（GETでもPUTでも、IDと本文より前）、writeFeaturedPlan、testerInvitation（tokenとIDより前）、solvedProblems、problem()（既存の`:66`。ID、cursor、roleより前）。
- Storeがnilでないことを呼び出し側が保証している4か所（`withReadiness`、`list`のtester分岐、`list`のreadiness、`publishProblem`のSchedule）には、nil判定を置かない。readinessは、Storeがあれば常に計算する。テスターの一覧にもreadinessを付ける。テスター本人が問題を開いたときは、`State`が`ErrNotFound`を返すのでreadinessを付けない。
- `releaseBefore`には`d.Store`をそのまま渡す。nilなら定期便と予約投稿の解放を飛ばし、nilでなければ`ReleaseScheduled`と`ReleaseFeatured`を呼ぶ。
- `submissions.Service.Create`は、generationで`Problems`がnilなら`ErrProblemStorage`を返す（503 `database_unavailable`）。`service.go:71`のnil判定は残す。
- `authentication.require`は、`Profiles`がnilなら削除済みの判定もプロフィール必須の判定もしない（`/auth/me`は200、`/my/profile`は503）。認証の401は常に503より先に来る。
- 消費側のフィールドの型は、同じタスクの中ですべて一緒に変える。Task 2では`handlerDependencies.Store`、`problemHandler.Store`、`Service.Problems`。Task 3では`handlerDependencies.Profiles`、`profileHandler.Profiles`、`authentication.Profiles`。インターフェースのまま残すと、typed nilになってもコンパイルは黙って通る。
- `Files`（`testFileStore`）と、`public_samples.go:52`の`PublicSampleDownload`へのアサーションは変えない。`fakeTestFiles`と`sampleTestFiles`という偽物が実在する。
- `registerRoutes`では、ローカル変数`problems`、`profiles`、`submissions`がパッケージ名を隠している（`routes.go:111-114`）。`d.Store`はフィールドの参照なので影響しない。新しい行を、隠された後の位置に足さない。
- コードのコメントは英語にし、周りと同じ密度にする。既存のコメントは、削除する`problemReadiness`の1行を除いて残す（`private_problems.go:45`、`:230`など）。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターが行う。
- 開始の条件: C02（`api/internal/problems/store.go`と`api/internal/httpapi/testers_test.go`）がコミット済みであること。`git -C /home/loop/judge/.claude/worktrees/refactor-maintainability status --short`が空でなければ、作業を始めずに報告する。Task 2で`store.go`の`Repository`を消す。C02の変更と同じファイルなので、行番号ではなく内容で探す。
- Go、gofmt、makeは`nix develop`の中で実行する。npm、webのビルド、Playwrightは実行しない（検証担当が行う）。

## Review Focus

1. typed nil: 消費側のフィールドが1つでもインターフェースのまま残る。DBなしでは、nilのポインターを持つ非nilのインターフェースになり、nilの判定をすり抜けてpanicする。→ Task 1の`TestGenerationWithoutProblemStore`と、既存の`TestUnrelatedRoutesDoNotReleaseContests`で検出する。
2. 503と入力検証の順序: nil判定を関数の先頭へ「整理」して動かすと、404/400が503に変わる（逆もある）。→ Task 1の表で、ルートごとの境界を固定する。
3. readinessの消失や二重判定: `if ok`を外すときに、`list`のreadinessの計算を落とす。あるいはnil判定で包んでしまう。→ 既存の`TestProblemReadinessPostgres`（作者の一覧、保存の応答、テスターには付けない、テスターの一覧には付ける）と、`TestTesterInvitationsPostgres`の「tested list lacks contest readiness」で検出する。
4. `releaseBefore`の解放: `featuredStore`を消すときに渡す値を誤ると、予約投稿の解放が止まる。→ Task 1の表（Contestsなし、Storeなしのrelease=trueのルートがpanicしない）と、既存の`TestScheduledProblemImagesAndReleaseOnReadPostgres`、`TestFeaturedReleaseAndEmbargoPostgres`で検出する。
5. `Files`のアサーションを巻き込んで消すと、偽物のテストが壊れる。`test_file_storage_unavailable`の経路も変わる。→ 既存の`TestProfilesPostgres`（`testPublicSamples`を呼ぶ）と`TestPrivateTestFileRoutes`で検出する。Task 2のStep 5のgrepでも、`Files\.(`が1件残ることを確かめる。

---

### Task 1: DBなしのルートの応答と、生成の提出の特性テストを追加する

**Files:**
- Modify: `api/internal/httpapi/routes_test.go`（importに`"encoding/json"`と`"judge/api/internal/submissions"`を足す。2つの関数を、`TestUnrelatedRoutesDoNotReleaseContests`の直後（現在の49行目の後）に置く）

**Interfaces:**
- Consumes: `newHandler`、`handlerDependencies`、`staticVerifier`（同じファイル）
- Produces: `TestProblemRoutesWithoutDatabase`、`TestGenerationWithoutProblemStore`。本番コードは変えない。

- [ ] **Step 1: テストを書く**

```go
// TestProblemRoutesWithoutDatabase pins which input checks come before database_unavailable on the problem and
// profile routes when no store is configured.
func TestProblemRoutesWithoutDatabase(t *testing.T) {
	// Without a contest store, release-dependent routes skip the release.
	h := newHandler(AuthConfig{}, handlerDependencies{Verifier: staticVerifier{}})
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		method, path string
		status       int
		code         string
	}{
		{"GET", "/featured", 503, "database_unavailable"},
		{"GET", "/featured?offset=-1", 503, "database_unavailable"},
		{"GET", "/featured/schedule", 503, "database_unavailable"},
		{"GET", "/featured/standings?at=bad", 400, "invalid_request"},
		{"GET", "/featured/standings?at=2026-10-19T14:00:00Z&offset=-1", 400, "invalid_request"},
		{"GET", "/featured/standings?at=2026-10-19T14:00:00Z", 503, "database_unavailable"},
		{"GET", "/problems?author=BAD", 400, "invalid_author"},
		{"GET", "/problems/bad?author=BAD", 400, "invalid_author"},
		{"GET", "/problems/bad", 404, "not_found"},
		{"GET", "/problems?cursor=bad", 503, "database_unavailable"},
		{"GET", "/problems/" + id, 503, "database_unavailable"},
		{"GET", "/problems/bad/samples", 404, "not_found"},
		{"GET", "/problems/" + id + "/samples", 503, "database_unavailable"},
		{"GET", "/users/BAD", 404, "not_found"},
		{"GET", "/users/alice", 503, "database_unavailable"},
		{"GET", "/my/solved-problems", 503, "database_unavailable"},
		{"GET", "/my/favorites/bad", 404, "not_found"},
		{"PUT", "/my/favorites/" + id, 503, "database_unavailable"},
		{"PUT", "/my/difficulty-votes/bad", 404, "problem_not_found"},
		{"PUT", "/my/difficulty-votes/" + id, 503, "database_unavailable"},
		{"GET", "/my/featured", 503, "database_unavailable"},
		{"PUT", "/my/problems/bad/featured", 503, "database_unavailable"},
		{"GET", "/my/featured/schedule?problem=bad", 404, "problem_not_found"},
		{"GET", "/my/featured/schedule?preference=x", 400, "invalid_request"},
		{"GET", "/my/featured/schedule", 503, "database_unavailable"},
		{"GET", "/my/tester-invitations/bad", 503, "database_unavailable"},
		{"POST", "/my/problems/bad/tester-invitation", 503, "database_unavailable"},
		{"GET", "/my/problems?role=tester", 503, "database_unavailable"},
		{"GET", "/my/problems?role=x", 503, "database_unavailable"},
		{"GET", "/my/problems?cursor=bad", 503, "database_unavailable"},
		{"GET", "/my/problems/bad", 503, "database_unavailable"},
		{"GET", "/my/problems/bad/publication", 404, "not_found"},
		{"GET", "/my/problems/" + id + "/publication", 503, "database_unavailable"},
		{"PUT", "/my/problems/bad/publication", 404, "problem_not_found"},
		{"PUT", "/my/problems/" + id + "/publication", 503, "database_unavailable"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			// No body is sent: where a route reads one, the store check comes first.
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer token")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			var body struct{ Error string }
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != tc.status || body.Error != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

// TestGenerationWithoutProblemStore pins that a generation is refused, not run against a nil problem store.
func TestGenerationWithoutProblemStore(t *testing.T) {
	// The submission store is never used: the generation stops before it.
	h := newHandler(AuthConfig{}, handlerDependencies{Verifier: staticVerifier{}, Submissions: &submissions.Store{}, JudgeImage: "sha256:" + strings.Repeat("a", 64)})
	r := httptest.NewRequest("POST", "/my/submissions", strings.NewReader(`{"problemId":"11111111-1111-4111-8111-111111111111","runtime":"cpp17-local","source":"x","generation":{"mode":"input","start":1,"count":1}}`))
	r.Header.Set("Authorization", "Bearer token")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var body struct{ Error string }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 503 || body.Error != "database_unavailable" {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: 変更前のコードで成功することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 -v -run 'TestProblemRoutesWithoutDatabase|TestGenerationWithoutProblemStore|TestUnrelatedRoutesDoNotReleaseContests' ./internal/httpapi/`
Expected: `ok`。`--- PASS: TestProblemRoutesWithoutDatabase`の下に、35件の`--- PASS`がある。`--- FAIL`はない。

- [ ] **Step 3: gofmtを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l internal/httpapi/routes_test.go`
Expected: 出力なし

コミットメッセージの案: `test: DBなしでの問題とプロフィールのルートの応答と、ストアなしの生成の提出を固定する`

---

### Task 2: 問題ストアを`*problems.Store`で持つ

**Files:**
- Modify: `api/internal/httpapi/routes.go`、`problems.go`、`private_problems.go`、`featured.go`、`favorites.go`、`difficulty_votes.go`、`solved_problems.go`、`public_samples.go`、`testers.go`
- Modify: `api/internal/submissions/service.go:24`
- Modify: `api/internal/problems/store.go`（`type Repository interface { ... }`の6行。HEADでは196-201行）、`api/internal/problems/publication.go:38-42`

**Interfaces:**
- Consumes: Task 1のテスト
- Produces: `handlerDependencies.Store *problems.Store`、`problemHandler.Store *problems.Store`、`submissions.Service.Problems *problems.Store`。`problems.Repository`、`problems.Publications`、`problemReadiness`は存在しなくなる。

入口に置くnil判定は、次の4行に統一する（`notifications.go:6-9`と同じ）。

```go
	if p.Store == nil {
		authError(w, 503, "database_unavailable")
		return
	}
```

- [ ] **Step 1: 型を変え、未使用になる型を削除する**
  - `routes.go:43`と`:50`の`problems.Repository`を`*problems.Store`にする。
  - `routes.go:110`の`featuredStore, _ := d.Store.(*problems.Store)`を削除し、`:126`と`:135`の`featuredStore`を`d.Store`にする。`:114`の`Problems: d.Store`はそのまま残す。
  - `service.go:24`を`Problems      *problems.Store`にする。
  - `problems/store.go`の`Repository`と、`publication.go:38-42`の`Publications`を削除する。

- [ ] **Step 2: 入口のアサーションを、同じ位置のnil判定に置き換える**

各箇所の`store, ok := p.Store.(...)`と`if !ok {...}`を、上のnil判定にする。続く`store.X(`は`p.Store.X(`にする。

| 箇所（HEAD） | 関数 |
| --- | --- |
| `problems.go:82-86` | publishedProblems（`defer cancel()`の後、`if id != ""`の前） |
| `problems.go:138-142` | publishProblem（IDの検証の後、`publicationRequest`の前） |
| `featured.go:18`、`:41`、`:104`、`:138` | featured、featuredApplication、writeFeaturedPlan、featuredStandings |
| `favorites.go:16-22` | favorite |
| `difficulty_votes.go:16-22` | difficultyVote |
| `solved_problems.go:9-15` | solvedProblems |
| `public_samples.go:28-34` | publicSamples（`:52`の`p.Files`のアサーションは残す） |
| `testers.go:13-17` | testerInvitation |

使われなくなるimportは削除する。`favorites.go`、`difficulty_votes.go`、`solved_problems.go`は`import "net/http"`だけになる。`testers.go`からは`problems`がなくなる。

- [ ] **Step 3: 判定済みの箇所のアサーションを削除する**
  - `private_problems.go:22-26`: `problemReadiness`とそのコメントを削除する。
  - `private_problems.go:47-50`（withReadiness）: アサーションと`if !ok { return nil }`を削除し、`p.Store.State(...)`を呼ぶ。
  - `private_problems.go:210-214`（listのtester）: 削除し、`items, err = p.Store.ListTesting(r.Context(), owner, cursor)`にする。
  - `private_problems.go:231-242`（listのreadiness）: `if store, ok := ...; ok {`の囲みを外し、中身を1段浅くして常に実行する。`p.Store.ApplicationCount`を呼ぶ。`:230`のコメントは残す。
  - `problems.go:163-170`（Schedule）: 匿名インターフェースへのアサーションを削除し、`result, err = p.Store.Schedule(r.Context(), owner, id, in.Version, *in.ScheduledAt)`にする。

- [ ] **Step 4: ビルド、vet、gofmtを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go build ./...`
Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...`
Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .`
Expected: どれもエラーなし。gofmtは出力なし。

- [ ] **Step 5: 残りがないことをgrepで確かめる**

Run: `grep -rn 'Store\.(\|problemReadiness\|problems\.Publications\|problems\.Repository\|type Publications \|type Repository ' /home/loop/judge/.claude/worktrees/refactor-maintainability/api --include=*.go`
Expected: `api/internal/profiles/store.go:40:type Repository interface {`の1件だけ（Task 3で消える）。HEADでは、`Store\.(`だけで16件（アサーションの全箇所）が出る

Run: `grep -rn 'Files\.(' /home/loop/judge/.claude/worktrees/refactor-maintainability/api/internal/httpapi --include=*.go`
Expected: `public_samples.go`の1件だけ

- [ ] **Step 6: DBなしのテストと、関係するDBテストを実行する**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run 'TestProblemRoutesWithoutDatabase|TestGenerationWithoutProblemStore|TestUnrelatedRoutesDoNotReleaseContests|TestPublicProblemErrors|TestPublicDocumentation|Featured|Scheduled|Tester|TestPrivate|TestProfilesPostgres|TestPublishedProblemSave|TestSubmissionsPostgres|ProblemReadiness|ProblemListPublicationPlans' ./internal/httpapi/ 2>&1 | grep -E '^(--- FAIL|--- SKIP|FAIL|ok|panic)'`
Expected: `ok  	judge/api/internal/httpapi`の1行だけ。SKIPとFAILはない（この選択には`TestBrowserFixture`は入らない）。

コミットメッセージの案: `refactor: 問題ストアを具体型で持ち、ハンドラーごとの型アサーションをやめる`

---

### Task 3: プロフィールストアを`*profiles.Store`で持つ

**Files:**
- Modify: `api/internal/httpapi/routes.go:44`、`:59`
- Modify: `api/internal/httpapi/authentication.go:53`
- Modify: `api/internal/httpapi/profile.go:135-144`
- Modify: `api/internal/profiles/store.go:40-43`

**Interfaces:**
- Consumes: Task 2の`routes.go`（同じファイルの別の行）
- Produces: `handlerDependencies.Profiles`、`profileHandler.Profiles`、`authentication.Profiles`がすべて`*profiles.Store`になる。`profiles.Repository`は存在しなくなる。

- [ ] **Step 1: 3つのフィールドの型を、同じ変更の中で`*profiles.Store`にする**（`routes.go:44`、`routes.go:59`の`profileHandler    struct{ Profiles *profiles.Store }`、`authentication.go:53`）。`profiles/store.go:40-43`の`Repository`を削除する。

- [ ] **Step 2: `profile.go:135-141`のアサーションを置き換える**。handleの検証の後、`context.WithTimeout`の前の同じ位置に`if p.Profiles == nil { authError(w, 503, "database_unavailable"); return }`を置く（4行の形はTask 2と同じ）。`:144`は`p.Profiles.GetByHandle(ctx, handle)`にする。`context`のimportは`WithTimeout`で使うので残る。

- [ ] **Step 3: ビルド、vet、gofmt、grep**

Run: Task 2のStep 4の3つのコマンド → 同じ期待値
Run: `grep -rn 'Profiles\.(\|profiles\.Repository\|type Repository ' /home/loop/judge/.claude/worktrees/refactor-maintainability/api --include=*.go`
Expected: 出力なし

- [ ] **Step 4: テストを実行する**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run 'TestProblemRoutesWithoutDatabase|TestUnrelatedRoutesDoNotReleaseContests|TestAuthenticationPreservesHandlerDeadline|TestProfilesPostgres|TestAccount|TestTagsPostgres' ./internal/httpapi/ 2>&1 | grep -E '^(--- FAIL|--- SKIP|FAIL|ok|panic)'`
Expected: `ok  	judge/api/internal/httpapi`の1行だけ

コミットメッセージの案: `refactor: プロフィールストアを具体型で持ち、公開プロフィールの型アサーションをやめる`

---

## 検証コマンド（全タスクの後）

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v ./... 2>&1 | grep -E '^(--- FAIL|FAIL|ok|panic)|TEST_DATABASE_URL required'` → `ok`の行だけが出る。`FAIL`、`panic`、`TEST_DATABASE_URL required`は出ない（SKIPしてよいのは`TestBrowserFixture`と、`TEST_JUDGE_CPP_IMAGE`が要る`submissions/runner_test.go`のDockerテストだけ）
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 警告なし
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .` → 出力なし
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command make package package-judge` → 終了コード0（`api/.build/`はgitignoreの対象）
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`（AGENTS.mdの規則。`graphify-out/`はコミットしない）
6. webのビルドとaccountのPlaywright（`TestBrowserFixture`経由でこのhandlerを使う）は、検証担当が行う。

## 完了条件

- Task 1の2つのテストが、変更前のコードで成功している（Task 1のStep 2）。
- 検証コマンドの1〜4がすべて成功している。
- Task 2のStep 5とTask 3のStep 3のgrepが、期待どおりの結果になっている。
- 差分に出るファイルは、Task 1が`routes_test.go`、Task 2がFilesの12ファイル、Task 3がFilesの4ファイル（`routes.go`はTask 2と重なる）だけである。`routes_test.go`以外のテストファイルは変わっていない。

## 範囲外

- `Files`（`testFileStore`）と`PublicSampleDownload`のアサーション。偽物が実在する正当な抽象である。
- ほかのハンドラーのnil判定やエラーコードの統一。たとえば、favoritesの`not_found`とdifficulty-votesの`problem_not_found`、GETとPUTの`/my/problems/{id}/publication`の違い。
- `problemHandler`のメソッドの分割や、ストアのインターフェースを新しく切り出すこと。
- `config.go`の組み立てと、テストの`handlerDependencies{...}`の書き方。

## レポートで触れる別件（直さない）

- 不具合は見つからなかった。HEADでは、すべてのアサーションが`*problems.Store`と`*profiles.Store`で成功する（計画者がコンパイル時に確かめた）。
- 仕様の確認事項: 不正なIDに対するエラーコードが、ルートによって`not_found`と`problem_not_found`に分かれている（favoritesと公開設定のGETは`not_found`、difficulty-votesと公開設定のPUTは`problem_not_found`）。今回は変えずに、表テストで固定する。
- C02の計画は「`make package package-judge`は`Makefile`にない」としている。実際には`api/Makefile:3,12`にあり、CI（`.github/workflows/ci.yml:151`）も実行している。C02の検証では、これを実行していない可能性がある。
