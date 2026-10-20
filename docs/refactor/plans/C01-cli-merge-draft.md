# C01 CLI pushの下書き合成をmergeDraftに切り出す 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `push`に直接書かれた下書きの合成規則を、特性テストで固定する。そのうえで、合成部分を`mergeDraft(local, remote draft) error`として同じファイル内に切り出す。

**Architecture:** 先に、`fakeDrafts`を使ってpushを通す特性テストを2つ追加する（Task 1）。次に、`cli/main.go:103-130`の28行を処理順も中身も変えずに`mergeDraft`へ移し、`push`からは同じ位置で呼ぶ（Task 2）。DBもネットワークも使わない。

**Tech Stack:** Go（`cli`モジュール）、`net/http/httptest`

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C01.json`（2つの反証の`revised_scope`、`must_preserve`、`test_plan`）

## 困りごと

- `cli/README.md`の「更新と競合」（259-268行）に書かれた規則が、`push`の途中に28行のまま埋まっている。`push`は1関数でロック、状態ファイル、版の検査、GET、合成、アップロード、PUT、状態の保存まで行う。
- 規則は順序に依存するが、その理由はどこにも書かれていない。`mergeScoring`とgeneratorsの合成は、結果を`local`に書いてから上書きのループで`d`へ反映する。`d`へ直接書くように直すと、上書きで消える。
- 守られていない規則がある。計画者がスクラッチのコピーで、次の4つの変異を同時に入れて確かめた。既存のテストはすべて成功した。
  - generatorsの合成結果を`remote`へ直接書く
  - remoteのgeneratorsのnull判定を消す
  - interactorの排他削除を消す
  - remoteのscoringのnull判定を消す
- 部分点対応（7ddb2e9）では、この途中に`mergeScoring`の呼び出しを差し込んだ。次に任意項目が増えたときも、同じ場所に足すことになる。

候補の説明は2点を訂正する（safetyの指摘による）。

| 候補の記述 | 訂正 |
| --- | --- |
| 「remoteにだけあるキーの保持にはテストがない」 | テストがないのは`difficulty`と`tags`だけ。`editorial`、`testCases`、`scoring`は既存テストで確認済み |
| 「mergeDraftはremoteを書き換える」 | `remote`と`local`の両方を書き換える。`local`の`scoring`と`generators`も変わる |

## 判定の記録（2つの反証が分かれた点）

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| 「存在してnullでない」のヘルパー | 除外 | 任意 | 入れない。呼び出しは2か所だけで、行数の圧縮に近い。null分岐そのものに触れる |
| mergeDraftの表駆動の単体テスト | 除外 | 任意 | 入れない。push経由のテストと重複する |
| 残すキーのテスト対象 | difficulty | difficulty、tags | 両方 |
| null、止まる時点、不正なscoring、競合の順序 | なし | (d)(e)は必須、(f)は追加、(g)は任意 | すべて入れる。(g)は1ステップで済む |
| アップロードの数え方 | — | fakeDraftsへのカウンタ追加、またはRoundTripper | 既存の`updateTransport`（`cli/update_test.go:14`）で`client.http`を包む。`fakeDrafts`の引数と既存の呼び出しは変えない |
| docコメントの言語 | — | — | 英語。`main.go`の既存コメントに合わせる。計画とレポートは日本語 |
| タスクの分割 | 主な価値は特性テスト | — | 2つに分ける。抽出だけを却下できるようにする |

## Global Constraints

- 公開HTTP API、CLIのコマンド・フラグ・出力、DBスキーマ、依存のバージョン、CI・インフラを変えない。`cli/README.md`も変えない。
- テストは追加だけにする。既存のテストと`fakeDrafts`は変更・削除しない。
- PUTで送る下書きのJSONは、同じ入力ならバイト列まで変えない（`draft`はmapなので、`json.Marshal`でキーが整列される）。
- `mergeScoring`の本体とエラー文言は変えない。エラーはどれも包まずにそのまま返す。CLIはこれを`shareoj: <err>`と出力する。
- `mergeDraft`は、版の確認（`main.go:100-102`）の直後、testCasesのアップロード（`:131`）の前で呼ぶ。そのため次の順序が保たれる。
  - 版の競合は、合成エラーより先に返る。
  - 合成エラーは、`test-files`へのPOSTとPUTより前に返る。
- `remote.Draft == nil`の確認（`:97`）と`d := remote.Draft`は、`push`に残す。404で版0のときは、空の`draft{}`から合成する（`:92-93`）。
- 合成の中では、次の点を変えない。
  - 処理順：`mergeScoring`、generatorsの役割ごとの合成（結果は`local.set("generators", ...)`）、`local`のキーで`remote`を上書き、判定器の排他削除。
  - remote側の判定：キーがない場合とJSONの`null`を同じに扱う式のまま。
  - local側の判定：`!= nil`のまま。
  - デコードには`decodeJSON`を使う。`json.Unmarshal`に置き換えない。
- testCasesのアップロードと`d.set("testCases", ...)`は`push`に残す。合成は`local["testCases"]`を変えない。
- 次のものは変えない：ロックファイル、`.shareoj.json`を書く時点（初回のID発行直後とPUTの成功後だけ）、PUTの`Version`、`"unexpected save response; ..."`の文言。
- `git commit`、`git stash`、`git reset`は実行しない。コミットはオーケストレーターが行う。worktreeには`graphify-out/`がないので、`graphify update .`も実行しない。

## Review Focus

1. 合成のエラー（部分点の停止、不正なremoteのscoring）が、アップロードの前に返ること。既存のテストはPUTの数しか見ていない。→ Task 1の`TestScoringPushMerge`でアップロード数を数える。
2. generatorsの合成結果を`local`に書いていること。`remote`へ直接書くと、上書きでlocal側の役割だけが残る。→ `TestPushMergeRules`の役割ごとの更新。
3. remoteの`generators`や`scoring`がJSONの`null`の場合。前者はnil mapへの代入でpanicし、後者は部分点と誤って判定する。→ 両テストのnullのステップ。
4. 文言が完全に一致すること（包んだり変えたりしない）。→ `TestScoringPushMerge`で`err.Error()`を完全一致で比べる。
5. 版の競合と合成エラーが同時に起きたとき、版の競合が返ること。→ `TestScoringPushMerge`の競合のステップ。

---

### Task 1: 下書き合成の特性テストを追加する

**Files:**
- Test: `cli/main_test.go`（`TestPushLifecycle`の後に`TestPushMergeRules`を追加）
- Test: `cli/scoring_test.go`（`TestScoringPushAndPull`の後に`TestScoringPushMerge`を追加）

**Interfaces:**
- Consumes：既存のテスト補助。
  - `example(t)`、`scoringExample(t)`、`write(t, path, content)`
  - `fakeDrafts(t, *problem, *int) *httptest.Server`
  - `updateTransport`（`func(*http.Request) (*http.Response, error)`型）
  - 定数`template`、`noAggregator`
- Produces：テスト関数2つ。本番コードは変えない。

共通の準備は`TestScoringPushAndPull`（`scoring_test.go:189-204`）と同じにする。

- `t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())`
- `t.Setenv("SHAREOJ_ACCESS_TOKEN", "author")`
- `fakeDrafts(t, &remote, &puts)`
- `newClient(server.URL)`

最初のpushで問題を作る。その後は`remote.Draft`を直接書き換えて、サーバー側の状態を作る。コメントは規則ごとに英語で1行書き、既存のテストと同じ書き方にする。

- [ ] **Step 1: `TestPushMergeRules`を書く**（`client.http = server.Client()`）

| 手順 | 操作 | 期待 |
| --- | --- | --- |
| 0 | `example(t)`をそのままpushする | エラーなし。この時点のgeneratorsを`before`として保存する |
| 1 | problem.tomlを、`template`から`[generators.output]`と`[generators.validation]`の節（各3行と空行）を除いたものにする。`input.cpp`は`"// new input\n"`にする。pushする | generatorsは3役割。`input.Source == "// new input\n"`。`output`と`validation`は`before`と等しい |
| 2 | `remote.Draft["generators"] = json.RawMessage("null")`にして、同じ内容でpushする | エラーなし。generatorsは`map[string]program{"input": {Runtime: "cpp23-gcc", Source: "// new input\n"}}`と`reflect.DeepEqual`で一致する |
| 3 | `remote.Draft.set("interactor", program{Runtime: "python3", Source: "print(1)\n", Protocol: "legacy"})`を入れる。`checker.cpp`を`"// checker\n"`にする。problem.tomlを`template + "\n[checker]\nsource = \"checker.cpp\"\nruntime = \"cpp23-gcc\"\nprotocol = \"testlib\"\n"`にしてpushする | `remote.Draft`に`interactor`キーがない。`checker`は`` `{"runtime":"cpp23-gcc","source":"// checker\n","protocol":"testlib"}` `` |
| 4 | `interactor.py`を`"print(2)\n"`にする。problem.tomlを`template + "\n[interactor]\nsource = \"interactor.py\"\nruntime = \"python3\"\nprotocol = \"legacy\"\n"`にしてpushする | `checker`キーがない。`interactor`は`` `{"runtime":"python3","source":"print(2)\n","protocol":"legacy"}` `` |
| 5 | `remote.Draft.set("difficulty", 3)`と`remote.Draft.set("tags", []string{"graph"})`を入れる。problem.tomlを`template`にしてpushする | `difficulty`は`3`、`tags`は`["graph"]`。`interactor`は手順4の値のまま |

- [ ] **Step 2: `TestScoringPushMerge`を書く**

準備では`scoringExample(t)`を使い、`tests/03.in`を`strings.Repeat("3\n", 40<<10)`（64KiBを超える）にする。`client.http`は次のように包み、アップロードの数を数える。

```go
next := server.Client().Transport
client.http = &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/test-files") {
		uploads++
	}
	return next.RoundTrip(r)
})}
```

| 手順 | 操作 | 期待 |
| --- | --- | --- |
| 0 | pushする（部分点で作成） | エラーなし。`uploads == 1` |
| 1 | problem.tomlを`template`（`[tests]`あり、`[scoring]`なし）にしてpushする | `err.Error()`が`main.go:180`の文言と完全に一致する。`puts`と`uploads`は増えない |
| 2 | `remote.Version++`の後にpushし、終わったら`remote.Version--`で戻す | `err.Error() == "version conflict: local 1, remote 2; review the web draft before using --expect-version 2"` |
| 3 | ``remote.Draft["scoring"] = json.RawMessage(`"x"`)``にする。problem.tomlを`template + noAggregator`にしてpushする | `err.Error() == "invalid scoring in the remote draft"`。`puts`と`uploads`は手順1の前の値のまま |
| 4 | `remote.Draft["scoring"] = json.RawMessage("null")`にする。problem.tomlを`template`にしてpushする | エラーなし（部分点として扱われない）。`remote.Draft["scoring"]`は`null`のまま。`puts`は1増える |

- [ ] **Step 3: 変更前のコードで成功することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 -run 'TestPushMergeRules|TestScoringPushMerge' -v .`
Expected: 2つとも`--- PASS`（計画者がスクラッチのコピーで同じ内容を実行し、成功を確かめた）

- [ ] **Step 4: CLI全体を確かめる**（後述「検証コマンド」の1〜4）

コミットメッセージの案（オーケストレーター用）：`test: CLI pushの下書き合成規則を特性テストで固定する`

### Task 2: 合成をmergeDraftに切り出す

**Files:**
- Modify: `cli/main.go:103-130`（`push`の中）。`push`と`mergeScoring`の間に`mergeDraft`を追加する。

**Interfaces:**
- Consumes：Task 1の2つのテスト（安全網）
- Produces：`func mergeDraft(local, remote draft) error`

- [ ] **Step 1: `mergeDraft`を追加する**

本体は`main.go:104-130`をそのまま移し、次の3点だけを置き換える。

- `d`を`remote`にする。
- `return s, err`を`return err`にする。
- 末尾に`return nil`を足す。

null判定の式、`decodeJSON`、`local.set("generators", generators)`、処理順は1文字も変えない。

docコメントは英語で6行以内にし、`mergeScoring`のコメントと同じ調子で書く。次の3点を含める。

1. `cli/README.md`の「更新と競合」の規則を適用すること。設定した項目は置き換え、省いた任意項目はremoteの値を残す。generatorsは役割ごとに置き換え、checkerとinteractorは互いを置き換える。
2. scoringとgeneratorsは、`local`を`remote`へ写す前に`local`へ合成する。`remote`へ直接書くと、写すときに消える。
3. そのため、`local`も書き換わる。

`mergeScoring`の規則はそのdocコメントに任せ、繰り返さない。

- [ ] **Step 2: `push`から呼ぶ**

`d := remote.Draft`の直後の28行を、次の3行に置き換える。続く`if local["testCases"] != nil {`以降は変えない。

```go
if err := mergeDraft(local, d); err != nil {
	return s, err
}
```

- [ ] **Step 3: テストを実行する**（「検証コマンド」の1〜4）。すべて成功し、`gofmt -l`の出力が空であること。

- [ ] **Step 4: 呼び出し位置を変えると検出されることを確かめる**

1. `mergeDraft`の呼び出し（3行）を、Editで`var result problem`の直前へ一時的に移す。
2. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 -run TestScoringPushMerge -v .`
   Expected: FAILし、手順1で`uploads=1`が報告される。
3. Editで元の位置へ戻す。
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git diff -- cli/main.go`で、差分がStep 1-2の内容だけであることを確かめる。

- [ ] **Step 5: 検証コマンドの1〜4をもう一度実行し、すべて成功することを確かめる**

コミットメッセージの案：`refactor: CLI pushの下書き合成をmergeDraftに切り出し、規則の順序をdocコメントに書く`

## 検証コマンド

作業ディレクトリは`/home/loop/judge/.claude/worktrees/refactor-maintainability/cli`。

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 ./...` → `ok  judge/cli`
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 出力なし、終了コード0
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CGO_ENABLED=0 go build -trimpath -o /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/shareoj .` → 終了コード0
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l main.go main_test.go scoring_test.go` → 出力なし

`TestInstaller`は`../web/public/install.sh`を読む。必ずworktreeの`cli`で実行すること（コピー先では失敗する）。

## 完了条件

- Task 1の2つのテストが、変更前のコードで成功している。
- Task 2の後、検証コマンドの1〜4がすべて成功し、追加したテストと既存のテスト（`TestPushLifecycle`、`TestScoringPushAndPull`、`TestScoringValidation`、`TestPullLifecycle`）も成功している。
- 呼び出し位置を移す変異は、`TestScoringPushMerge`で検出され、元に戻してある。
- `git diff --stat`に出るのは、`cli/main.go`、`cli/main_test.go`、`cli/scoring_test.go`の3ファイルだけ。

## 範囲外

- 「存在してnullでない」のヘルパー化、local側の`!= nil`判定の書き換え
- `push`を`push.go`へ移すこと。testCasesのアップロード処理の整理や移動
- `mergeScoring`の変更。`mergeDraft`の表駆動の単体テスト
- `fakeDrafts`の変更、既存テストの書き換え
- `cli/README.md`の変更

## レポートで触れる別件（直さない）

- `cli/scoring_test.go:235`のコメント「push stops before any request」は不正確。実際には、GETが先に行われる。
