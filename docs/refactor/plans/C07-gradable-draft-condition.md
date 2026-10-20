# C07 提出とリジャッジの「採点できる下書きか」のSQL条件をjudgeReadyにまとめる 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `CreateRun`と`Rejudge`に同じ内容で2回書かれている受け付け条件（メモリ、checker、interactor、部分点）を、特性テストで固定する。そのうえで、SQL断片を返す非公開関数`judgeReady`の1か所にまとめる。

**Architecture:** 先に、Store層の表駆動テストを1つ追加する（Task 1）。下書きをDBへ直接書き込み、提出4通りとリジャッジ2通りの結果を確かめる。次に、`store.go:251-264`と`rejudge.go:83-96`を`judgeReady(...)`の呼び出しに置き換える（Task 2）。組み立ては、既存の`onsiteParticipants`や`contestSubmissionVisible`（`contest.go`）と同じ文字列連結にする。

**Tech Stack:** Go、PostgreSQL（`TEST_DATABASE_URL`）、pgx

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C07.json`（worthとsafetyの`revised_scope`、`must_preserve`、`test_plan`）

## 困りごと

- 受け付け条件の14行が、`api/internal/submissions/store.go:251-264`（CreateRun）と`api/internal/submissions/rejudge.go:83-96`（Rejudge）にある。違うのは次の4点だけ。
  - 下書きの式：`selected_draft`と`t.draft`
  - 提出言語の式：`$6`と`s.runtime`
  - 判定コードの言語配列：`$7::text[]`と`$5::text[]`
  - CreateRunだけ、部分点の条件の前に`$8::boolean OR `がある
- 計画者が置換してdiffを取り、上の4点以外は1文字も違わないことを確かめた。
- eabbf4fでCreateRunの条件をRejudgeへ写した。その後のd4909a6（部分点）と2c9923a（groupsが1件以上）では、両方へ手で同じ差分を入れている。
- 片方だけを直すと、どちらかの不具合になる。
  - リジャッジだけが緩い：新規の提出なら断る下書きを再キューし、ワーカーでJEになる。
  - 提出だけが緩い：採点できる問題のリジャッジが409になる。
- 2つの文の間に相互参照がない。規則の説明はCreateRunのdocコメント（`store.go:178-181`）にしかなく、rejudge.goからは辿れない。

候補の記述は2点訂正する。

| 候補の記述 | 訂正 |
| --- | --- |
| 部分点の拒否（グループなし、集計コードなし、集計コードの言語）にテストがない | `api/cmd/judge-bridge/scoring_test.go`の`TestPartialScoringJobsAndResults`が、提出とリジャッジの両方で確かめている。実際にないのは、リジャッジ側のメモリ、checker、interactor、cpp17-localの条件と、NULLになる値 |
| `submissions_test.go:625`（Pythonのcheckerで409）が、cpp17-localのcheckerをcpp17に限る条件を確かめている | ローカル構成では言語配列が`["cpp17"]`だけなので、配列の条件で拒否されている。cpp17の限定そのものは確かめていない |

## 判定の記録（2つの反証が分かれた点）

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| ヘルパーの形 | 1関数。サンプル検証の式を引数で受け取る | 断片を2つ返し、SQLをバイト単位で一致させる | worthの1関数にする：`judgeReady(draft, runtime, checkerRuntimes, sampleCheck string) string`。Rejudgeは`"false"`を渡すので、SQLに`false OR `が1か所増える。理由は2つ。呼び出し側で組み立てる部分が残らないので、条件群を足すときに直すのはヘルパーだけで済む。`false OR x`はNULLを含めてxと同じ値になる。計画者がスクラッチのコピーで確かめた結果、CreateRunのSQLはバイト単位で一致し、Rejudgeの違いは`false OR `だけだった |
| ケース総数の条件 | 含めてもよい | 含めない | 含めない。書き方が違い（`>0 AND <=100`と`BETWEEN 1 AND 100`）、含めるとCreateRunのバイト一致が崩れる。`selected_cases`と同じく「どのケースを採点するか」の条件で、判定コードの条件ではない |
| 言語配列の引数名 | `runtimes` | — | `checkerRuntimes`。`RunInput.CheckerRuntimes`、`RejudgeInput.CheckerRuntimes`と同じ名前にし、提出言語の`RejudgeInput.Runtimes`（`$2`）と取り違えないようにする |
| 組み立て方 | `onsiteParticipants`にそろえる | 文字列連結。`fmt.Sprintf`は使わない | 両方を採用する |
| `problems.ContentIssues`への言及 | 範囲外とだけ記録 | docコメントに「別の規則」と書く | 1文だけ書く |
| 特性テストの層と置き場所 | HTTPの409か、StoreのErrNotReady | judge-bridgeのStore層 | `api/cmd/judge-bridge/readiness_test.go`に置き、Store層で確かめる。`migratedDB`があり、`TestPartialScoringJobsAndResults`が同じ層で同じ種類の確認をしている |
| タスクの分割 | — | — | 2つに分ける。特性テストだけを採用し、置き換えを却下することもできるようにする |

## Global Constraints

- 次のものを変えない。
  - 公開HTTP API（ルート、JSON、ステータス、エラー文言）
  - DBスキーマ、migration、依存のバージョン、CI、インフラ
- 既存のテストは変更も削除もしない。テストは追加だけにする。
- `ErrNotReady`になる条件を、提出とリジャッジのどちらでも論理式として変えない。確かめる手段は2つある。
  - Task 1の表の28行が、変更の前後で同じ結果になる。
  - Task 2 Step 4のバイト比較で、CreateRunの断片は一致する。Rejudgeの断片は`false OR `の挿入だけが違う。
- 次の条件と処理は、それぞれの呼び出し側に残し、変えない。
  - `jsonb_array_length(selected_cases) > 0`、ケース総数の条件
  - `t.draft IS NOT NULL`、`s.id=t.id AND s.status='DONE'`
  - `rejudgeTarget`
  - ジョブJSONの組み立て（`privateDraft`、`scoring`と`acceptPoints`のCASE）
  - `judge_attempt`、`rejudged_at`、`problem_version`、`problem_title`の更新
  - コンテストと参加者の行ロック、参加資格の判定（`ErrParticipationRequired`は`ErrNotReady`より先に返る）
  - 問題のロックと権限の再判定
  - Rejudgeが0件のときの区別（`rejudge.go:101-111`）：対象があればErrNotReady、なければ0件で成功
- パラメータ番号、型変換、Goから渡す引数とその順番を変えない。
  - CreateRunの引数：`id, owner, problemID, source, image, runtime, checkerRuntimes, easyTest, contestID`
  - Rejudgeの引数：`in.ProblemID, in.Runtimes, in.SubmissionIDs, in.Image, in.CheckerRuntimes`
- `judgeReady`に渡すのは、コード中の固定のSQL式だけにする。利用者の入力は連結しない。
- コード中のコメントは英語で書き、周りと同じ密度にする。計画とレポートは日本語で書く。
- `git commit`、`git stash`、`git reset`は実行しない。コミットはオーケストレーターが行う。
- web/は別の実装者（C04）が編集中なので、npm、ビルド、Playwrightは実行しない。

## Review Focus

1. パラメータの取り違え：Rejudgeの`$5`を`$2`にする、CreateRunの`$6`を別の番号にする、など。→ Task 1の「python checker」「interactor」「groups」の行で、cpp17-localの列が変わる。計画者がスクラッチのコピーで変異を入れて確かめた。Rejudgeの`$5`を`$2`にするとSQLのパラメータ数エラーでFatalになる。CreateRunの`$6`を`$5`にすると、python checker、interactor、groupsなどの行が失敗する。サンプル検証の式の変異（CreateRunに`"false"`、Rejudgeに`"true"`）も、部分点の行で失敗する。
2. サンプル検証の式は部分点の条件にだけ効く：メモリ、checker、interactorの拒否は、サンプル検証でも残らなければならない。→ Task 1でメモリ、checker、interactorの行の`sample`と`localSample`の列が`false`であること。
3. NULLになる値：`false OR `が増えても、NULLは同じに扱われる必要がある。→ Task 1の次の行で確かめる。
   - 「memory null」
   - 「checker without source」
   - 「mode missing」
   - 「aggregate without aggregator」
4. Goの生文字列の中のエスケープ：`E' \t\r\n'`は、バックスラッシュの並びのまま残す必要がある。→ Task 2 Step 4のバイト比較と、Task 1の「blank」の4行。
5. Rejudgeの0件とErrNotReadyの区別と、繰り返しで重ならないこと。→ 既存の`TestRejudgePostgres`で確かめる（2回目の`{"queued":0}`と409）。あわせて、Task 1のrejudgedで`(1, nil)`か`(0, ErrNotReady)`以外ならFatalにする。

---

### Task 1: 提出とリジャッジの受け付け条件の特性テストを追加する

**Files:**
- Create: `api/cmd/judge-bridge/readiness_test.go`（`package main`）

**Interfaces:**
- Consumes：次のものを使う。
  - `migratedDB(t)`（`main_test.go:261`）
  - `problems.New(db).Save` と `Publish`
  - `submissions.Store.CreateRun`、`submissions.Store.Rejudge`
  - `submissions.ErrNotReady`
- Produces：`TestSubmitAndRejudgeAgreeOnReadiness`。本番コードは変えない。

- [ ] **Step 1: 準備を書く**
  - `migratedDB(t)`を使い、`user_profiles`に`alice`、`bob`、`carol`を入れる。
  - 問題`11111111-1111-4111-8111-111111111111`を、aliceが`Save`して`Publish(…, true)`する。
    - 下書き：`Title "t"`、`Markdown "m"`、`Difficulty 1`、`TimeLimitMS "1000"`、`MemoryLimitMB "256"`
    - TestCases：`{IsSample: true, Input: "1", Output: "1"}`と`{Input: "2", Output: "2"}`
  - `SELECT published_draft::text`の結果を`base`として保存する。
  - carolの`DONE`の提出をSQLのINSERTで2件作る。
    - runtimeは`cpp17-isolate`と`cpp17-local`。
    - `job`は`'{}'::jsonb`。
    - 列は`id,owner_id,problem_id,problem_version,problem_title,runtime,source,status,job`。
  - `checkers := []string{"cpp17", "python314"}`。CreateRunとRejudgeの両方に渡す。英語のコメントを1行付ける。内容は「ローカル構成はcpp17しか公開しないが、python314も並べて、cpp17に限る規則だけを確かめる」。
- [ ] **Step 2: 判定の補助を書く**
  - `ready(runtime string, easyTest bool) bool`
    - bobで`CreateRun`を呼ぶ。IDは呼ぶたびに新しくする（例：`fmt.Sprintf("%08d-3333-4333-8333-333333333333", n)`）。`Image`は`"sha256:"+64桁`、`CheckerRuntimes: checkers`とする。
    - `nil`ならtrue、`ErrNotReady`ならfalseを返す。それ以外のエラーはFatalにする。
    - 呼ぶたびに`DELETE FROM submissions WHERE owner_id='bob'`を実行する。提出の制限（90秒に2件）に掛からないようにするため。
  - `rejudged(runtime string) bool`
    - aliceで`Rejudge`を呼ぶ。引数は`SubmissionIDs: [その提出]`、`Runtimes: []string{runtime}`、`CheckerRuntimes: checkers`。
    - `(1, nil)`ならtrue、`(0, ErrNotReady)`ならfalseを返す。それ以外はFatalにする。
    - 呼ぶたびに`UPDATE submissions SET status='DONE' WHERE owner_id='carol'`を実行する。
- [ ] **Step 3: 表を書く**
  - 各行で`UPDATE problem_drafts SET published_draft=$2::jsonb || $3::jsonb WHERE id=$1`（`base`、`patch`）を実行する。
  - 次の6つを`[6]bool`にまとめる。
    1. `ready("cpp17-isolate", false)`
    2. `ready("cpp17-isolate", true)`
    3. `ready("cpp17-local", false)`
    4. `ready("cpp17-local", true)`
    5. `rejudged("cpp17-isolate")`
    6. `rejudged("cpp17-local")`
  - 期待値は`{isolate, sample, local, localSample, isolate, local}`。リジャッジは、同じ言語の通常の提出と同じ結果になる。
  - 違ったら、行の名前を付けて`t.Errorf`する。
  - 期待値は次の表のリテラルのとおりにする（列は`name, patch, isolate, sample, local, localSample`）。

```go
group := `[{"name":"g","points":100}]`
{"plain", `{}`, true, true, true, true},
{"memory 63", `{"memoryLimitMb":"63"}`, false, false, false, false},
{"memory 64", `{"memoryLimitMb":"64"}`, true, true, true, true},
{"memory 512", `{"memoryLimitMb":"512"}`, true, true, true, true},
{"memory 513", `{"memoryLimitMb":"513"}`, false, false, false, false},
{"memory null", `{"memoryLimitMb":null}`, false, false, false, false},
{"checker null", `{"checker":null}`, true, true, true, true},
{"cpp17 checker", `{"checker":{"runtime":"cpp17","source":"x"}}`, true, true, true, true},
{"python checker", `{"checker":{"runtime":"python314","source":"x"}}`, true, true, false, false},
{"checker outside the languages", `{"checker":{"runtime":"java24","source":"x"}}`, false, false, false, false},
{"blank checker", `{"checker":{"runtime":"cpp17","source":" \t\r\n"}}`, false, false, false, false},
{"checker without source", `{"checker":{"runtime":"cpp17"}}`, false, false, false, false},
{"interactor", `{"interactor":{"runtime":"python314","source":"x"}}`, true, true, false, false},
{"interactor outside the languages", `{"interactor":{"runtime":"java24","source":"x"}}`, false, false, false, false},
{"blank interactor", `{"interactor":{"runtime":"python314","source":" \n"}}`, false, false, false, false},
{"checker and interactor", `{"checker":{"runtime":"cpp17","source":"x"},"interactor":{"runtime":"cpp17","source":"x"}}`, false, false, false, false},
{"scoring null", `{"scoring":null}`, true, true, true, true},
{"groups", `{"scoring":{"mode":"groups","groups":` + group + `}}`, true, true, false, true},
{"no groups", `{"scoring":{"mode":"groups","groups":[]}}`, false, true, false, true},
{"groups missing", `{"scoring":{"mode":"groups"}}`, false, true, false, true},
{"groups object", `{"scoring":{"mode":"groups","groups":{"g":100}}}`, false, true, false, true},
{"groups string", `{"scoring":{"mode":"groups","groups":"g"}}`, false, true, false, true},
{"mode missing", `{"scoring":{"groups":` + group + `}}`, false, true, false, true},
{"aggregator outside groups mode", `{"scoring":{"mode":"groups","groups":` + group + `,"aggregator":{"runtime":"java24","source":""}}}`, true, true, false, true},
{"aggregate", `{"scoring":{"mode":"aggregate","groups":` + group + `,"aggregator":{"runtime":"cpp17","source":"x"}}}`, true, true, false, true},
{"aggregate without aggregator", `{"scoring":{"mode":"aggregate","groups":` + group + `}}`, false, true, false, true},
{"aggregator outside the languages", `{"scoring":{"mode":"aggregate","groups":` + group + `,"aggregator":{"runtime":"java24","source":"x"}}}`, false, true, false, true},
{"blank aggregator", `{"scoring":{"mode":"aggregate","groups":` + group + `,"aggregator":{"runtime":"cpp17","source":"\t"}}}`, false, true, false, true},
```

テスト関数の上に、英語のコメントを2〜3行付ける。書く内容は3点。

- 提出とリジャッジは同じ下書きを受け付ける。そのため、リジャッジがJEになる実行を積むことはない。
- サンプル検証で外れるのは、部分点の条件だけである。
- 下書きをDBへ直接書くのは、保存時の検証が断る値も確かめるためである。

- [ ] **Step 4: 変更前のコードで成功することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run TestSubmitAndRejudgeAgreeOnReadiness ./cmd/judge-bridge/`

Expected: `--- PASS: TestSubmitAndRejudgeAgreeOnReadiness`。SKIPが出ないこと。計画者がスクラッチのコピーで同じ表を実行し、変更前のコードで成功することを確かめた（約0.7秒）。失敗した行があれば、期待値を書き換えない。その行と実際の値をレポートに書き、止まる。

- [ ] **Step 5: `gofmt -l cmd/judge-bridge/readiness_test.go`の出力が空であることを確かめる**（`nix develop … --command`の中で実行する）

コミットメッセージの案：`test: 提出とリジャッジが同じ下書きを受け付けることを特性テストで固定する`

### Task 2: 受け付け条件をjudgeReadyにまとめる

**Files:**
- Modify: `api/internal/submissions/store.go`
  - `RunInput`の後、`CreateRun`の直前に`judgeReady`を追加する。
  - `CreateRun`のdocコメント（178-181行）を直す。
  - 251-264行を置き換える。
- Modify: `api/internal/submissions/rejudge.go:83-96`

**Interfaces:**
- Consumes：Task 1のテスト（安全網）
- Produces：`func judgeReady(draft, runtime, checkerRuntimes, sampleCheck string) string`（パッケージ内だけで使う）

- [ ] **Step 1: 始める前に確かめる**

`cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git diff 092e9de -- api/internal/submissions/store.go api/internal/submissions/rejudge.go`

出力が空であること。空でなければ、行番号とバイト比較の基準がずれているので、止まって報告する。

- [ ] **Step 2: `judgeReady`を追加する**
  - 戻り値は、今の`store.go:251-264`の文字列から作る。先頭の`  AND `を除き、`(selected_draft->>'memoryLimitMb')`の`(`で始めて、`...E' \t\r\n'))>0))))`で終える。改行と各行の先頭の`  AND `や空白は、そのまま残す。
  - そのうえで、次のように置き換え、生文字列の連結（`` `…` + draft + `…` ``）で組み立てる。

    | 今の文字列 | 置き換え先 |
    | --- | --- |
    | `selected_draft` | `draft` |
    | `$6` | `runtime` |
    | `$7::text[]` | `checkerRuntimes` |
    | `$8::boolean` | `sampleCheck` |

  - `E' \t\r\n'`は、生文字列の中のバックスラッシュの並びのまま残す。解釈される文字列リテラル（`"…"`）に書き換えない。
  - docコメントは英語で6行以内にし、次の点を含める。
    1. CreateRunとRejudgeが共有する条件である。そのため、新規の提出が断る下書きを、リジャッジが積むことはない。
    2. 返すのはWHERE句用にANDでつないだ条件で、呼び出し側は`AND `の後に置く。
    3. `draft`、`runtime`、`checkerRuntimes`はSQL式で、それぞれ下書き、提出の言語、判定コードの言語の`text[]`を表す。
    4. `sampleCheck`は真偽値のSQL式で、真なら部分点の条件を問わない。サンプル検証は採点しないため。
    5. 今の`store.go:180-181`の文：groupsのない部分点や、集計コードのないaggregateは、判定コードの欠けと同じく待たせる。ジャッジがすべての実行をJEにするため。
    6. 公開の基準は`problems.ContentIssues`で、これとは別の規則である。
- [ ] **Step 3: 呼び出し側を置き換える**
  - `store.go`
    - 251-264行を、``  AND `+judgeReady("selected_draft", "$6", "$7::text[]", "$8::boolean")+` ``の1行にする。
    - 次の行の``  RETURNING `+columns, id, owner, …``は変えない。
    - CreateRunのdocコメントからは、Step 2の5番の文（180-181行の後半）を除く。最初の2文はそのまま残す。
  - `rejudge.go`
    - 83-96行を、``  AND `+judgeReady("t.draft", "s.runtime", "$5::text[]", "false"),``の1行にする。
    - 次の行の`in.ProblemID, in.Runtimes, in.SubmissionIDs, in.Image, in.CheckerRuntimes)`は変えない。
- [ ] **Step 4: 生成されるSQLをバイト単位で比べる（一時的な確認。コミットしない）**
  1. `api/internal/submissions/zz_c07_bytes_test.go`を一時的に作る。`TestTmpJudgeReadyBytes`は、次の2つを`os.WriteFile`で書き出す。
     - `"  AND "+judgeReady("selected_draft", "$6", "$7::text[]", "$8::boolean")+"\n"`を、`/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/C07/bytes/create.sql`へ。
     - `"  AND "+judgeReady("t.draft", "s.runtime", "$5::text[]", "false")+"\n"`を、同じディレクトリの`rejudge.sql`へ。
     - `.local/`はgitignoreの対象。
  2. `mkdir -p /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/C07/bytes`を実行してから、`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 -run TestTmpJudgeReadyBytes ./internal/submissions/`を実行する。
  3. 変更前のファイルを書き出す。2つのコマンドは別々に実行する。
     - `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git show 092e9de:api/internal/submissions/store.go > /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/C07/bytes/old_store.go`
     - `rejudge.go`も同じようにして、`old_rejudge.go`へ書き出す。
  4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/C07/bytes && sed -n '251,264p' old_store.go | diff - create.sql`を実行する。→ 出力が空であること。
  5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/C07/bytes && sed -n '83,96p' old_rejudge.go | diff - rejudge.sql`を実行する。→ 違いは次の2つだけであること。計画者のコピーでも同じ結果だった。
     - `10c10`：`AND (COALESCE(t.draft->'scoring'`が`AND (false OR COALESCE(t.draft->'scoring'`になる。
     - `14c14`：旧行の末尾の`` `, ``（Goの文字列の終わり）がない。
  6. `zz_c07_bytes_test.go`と`bytes/`ディレクトリを削除する。
- [ ] **Step 5: 検証コマンドの1〜4を実行し、すべて成功することを確かめる**
- [ ] **Step 6: 差分を確かめ、グラフを更新する**
  1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short api/`を実行する。出るのが`store.go`、`rejudge.go`、Task 1のテストだけで、`zz_c07_bytes_test.go`がないこと。
  2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git diff -- api/internal/submissions/store.go api/internal/submissions/rejudge.go`を実行する。差分がStep 2-3の内容だけであること。
  3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`を実行する。AGENTS.mdの規則による。`graphify-out/`はgitignoreの対象。

コミットメッセージの案：`refactor: 提出とリジャッジの判定コードと部分点の受け付け条件をjudgeReadyにまとめる`

## 検証コマンド

TEST_DATABASE_URLは`postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable`。15432は使わない。

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run 'TestSubmitAndRejudgeAgreeOnReadiness|TestPartialScoringJobsAndResults|TestRejudge|TestSubmissionsPostgres' ./cmd/judge-bridge/ ./internal/httpapi/`
   - 期待：`--- PASS`だけが出て、SKIPが出ない。
   - 対象：`TestSubmitAndRejudgeAgreeOnReadiness`、`TestPartialScoringJobsAndResults`、`TestRejudgesWaitBehindNewSubmissions`、`TestRejudgePostgres`、`TestSubmissionsPostgres`（メモリ63/513の409、checkerとinteractorの固定）
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 ./...`
   - 期待：すべて`ok`か`[no test files]`。計画者のコピーで変更後のコードを試したときは、httpapiが約37秒だった。
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...`
   - 期待：出力がなく、終了コードが0。
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .`
   - 期待：出力が空。

## 完了条件

- Task 1のテストが、変更前のコードで成功している。
- Task 2の後、検証コマンドの1〜4がすべて成功している。
- Step 4のバイト比較で、CreateRunの差分が空になる。Rejudgeの差分は`false OR `の挿入と、Goの文字列の終わりだけになる。
- 一時的なテストと`bytes/`は削除してある。
- `git status --short api/`に出るのは、`api/cmd/judge-bridge/readiness_test.go`（新規）、`api/internal/submissions/store.go`、`api/internal/submissions/rejudge.go`の3つだけ。

## 範囲外

- ケース総数、`selected_cases`、`t.draft IS NOT NULL`、`status='DONE'`の条件をヘルパーへ移すこと
- ジョブJSONの組み立ての共通化
- `problems/readiness.go`（`ContentIssues`）や`submissions/runner.go:113-122`（ローカルのアダプタ）にある規則の写しとの統合
- 関数を2つ（判定コードと部分点）に分ける形
- DB関数への移動（migrationが要る）
- 既存のテスト（`scoring_test.go`、`submissions_test.go`、`rejudge_test.go`）の変更

## レポートで触れる別件（直さない）

- `memoryLimitMb`が数値でない文字列のときは、`::int`の変換でSQLエラーになり、ErrNotReadyにはならない。保存時の検証（`problems/validation.go:83`）で防がれているので、DBを直接書き換えた場合にしか起きない。今回の変更でもこの動作は変わらない。
- リジャッジの対象に、採点できる提出とできない提出が混ざることがある。例は、部分点の問題に対する`cpp17-local`の提出。このとき、できない方は黙って除かれ、件数には入らない。ErrNotReadyになるのは、1件も積めなかったときだけである。今の仕様のまま変えない。
