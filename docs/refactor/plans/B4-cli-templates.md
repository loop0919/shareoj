# B4 CLIとひな形（D09、D10） 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `shareoj login`・`logout`に余分な引数を渡したときにそのコマンドに合った文言を出す（D09）。新規作成時の問題文と解説のひな形を1つのJSONファイルにまとめ、CLIは埋め込み、Webはビルド時に読み込む（D10）。

**Architecture:** D09は`cli/main.go`の`run`で、位置引数の数の検査を「init/check/push」と「login/logout」に分ける。D10はひな形を`cli/problem-template.json`に置く。`go:embed`は`cli/`の外を読めないため、ファイルは`cli/`に置き、Webの`web/app/utils/problem-draft.ts`が相対パスでimportする。

**Tech Stack:** Go（`cli`モジュール、`embed`）、Nuxt 4 / Vite 8（JSONのimport）、TypeScript 5.9、Playwright 1.63

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/docs/refactor/FIXES.md`のD09とD10の「変更内容」、同じファイルの「守ること」

## 判定（Ruling）

- Ruling（D09の文言）: `login takes no positional arguments`と`logout takes no positional arguments`にする。`fmt.Errorf("%s takes no positional arguments", command)`で作る。理由: 同じCLIの`guide`が同じ誤りに`guide takes no positional arguments`（`cli/guide.go:34`）を返している。いちばん近い前例に合わせる。
- Ruling（D10のファイルの形式）: 1つのJSONファイル`cli/problem-template.json`に、キー`statement`と`editorial`の2つを置く。理由: 仕様は「1つのファイル」。JSONなら、Goは既存の`decodeJSON`（`cli/problem.go:403`）で読め、Viteは標準でimportできる。依存も独自の区切り形式も増えない。
- Ruling（D10のimportの書き方）: `import markdownTemplate from '../../../cli/problem-template.json' with { type: 'json' }`とし、import属性を付ける。理由: `web/tests/problem-markdown.spec.ts`が`problem-draft.ts`をPlaywright経由でNodeに読み込む。計画者がscratchpadで試したところ、属性がないと`needs an import attribute of "type: json"`で読み込みに失敗した。属性があれば、Playwright・`tsc`（`module: preserve`、`resolveJsonModule`）・`vite build`のいずれでも通り、ビルドの出力にはJSONの中身が埋め込まれる。
- D10で文言は変わらない。計画者が確かめたところ、今の`cli/main.go:244-245`の文字列と`web/app/utils/problem-draft.ts:122-135`の文字列はバイト単位で同じだった。そのため、D10の新しいテストが変更前のコードで失敗する理由は「共有ファイルがまだない」ことになる。Webが写しではなく共有ファイルを読んでいることは、Task 2のgrepの確認（写しが残っていないこと）で確かめる。
- ビルドと配布で共有ファイルが見えること（計画者が確かめた）:
  - CIの`ci.yml`はリポジトリ全体をcheckoutし、パスによる絞り込みもない。`deploy-dev.yml`はCIが作った`web.zip`を使う。
  - `release-cli.yml`は`cli/`でビルドする。埋め込むファイルは`cli/`の中にある。
  - `package:lambda`はビルドの出力を固めるだけで、実行時にこのファイルを読まない。
  - `nuxt dev`の`server.fs.allow`には`workspaceDir`（gitのルート）が入る。

## Global Constraints

- 各項目の変更内容だけを行う。ほかの公開API、DBスキーマ、依存ライブラリのバージョン、インフラ構成は変えない。npmやGoのパッケージを足さない。
- 1件1コミット（`prefix: 対応内容`）。変える挙動のテストを先に書き、変更前のコードで失敗することを確かめてから直す。
- テストの削除・スキップ・検証の緩和はしない。
- 範囲外の問題を見つけたら直さずに報告する。
- 周りのコードに合わせる（英語のコメント、コメントの量、既存のヘルパーを使う）。
- コードを変えたら`graphify update .`を実行する（AGENTS.md）。`graphify-out/`は`.gitignore`の対象なので、コミットには含まない。
- Bashの規則: 変数から組み立てたコマンドは使わない。スクリプトファイルは実行しない。Go・Nodeは`nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command`を付けて実行する。

## Review Focus

1. `problem-markdown.spec.ts`は`problem-draft.ts`をNodeで読む。import属性を落とすと、このファイルのテストが全部読み込みで落ちる。Task 2のStep 6で、このspecを流して確かめる。
2. ひな形の文字列は1バイトも変えない（`\\mathrm`の逆斜線、末尾の改行、全角の文字）。Task 2のStep 3で、変更前の`main.go`と変更前のWebのビルドに対して新しいテストを流し、通ることを確かめる。`TestPushLifecycle`（`cli/main_test.go:134`）も解説の文字列を完全一致で見ている。
3. `shareoj logout foo`で、保存済みの認証情報を消さない。Task 1のテストで、認証情報のファイルが残ることを確かめる。
4. 位置引数の後に置いたオプション（`shareoj login foo --google`）も位置引数として数え、ブラウザーのログインを始めずにloginの文言を返す。Task 1のテストに行がある。
5. `init`/`check`/`push`/`pull`の既存の文言と、検査の順序（位置引数の検査が`--expect-version`や`--google`の組み合わせの検査より先）は変えない。Task 1のテストに既存の文言の行を入れる。

---

### Task 1: D09 `login`・`logout`の余分な引数に、そのコマンドの文言を出す

**Files:**
- Modify: `cli/main.go`（`run`の`needsDirectory`の後の検査、今の355-357行）
- Test: `cli/main_test.go`（末尾に追加）

**Interfaces:**
- Consumes: なし
- Produces: なし（`run`のシグネチャは変えない）

- [ ] **Step 1: 失敗するテストを書く**

`cli/main_test.go`の末尾に`TestPositionalArguments`を足す。

```go
func TestPositionalArguments(t *testing.T) {
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	t.Setenv("SHAREOJ_API_URL", "")
	t.Setenv("SHAREOJ_ACCESS_TOKEN", "")
	client, err := newClient(defaultAPI)
	if err != nil {
		t.Fatal(err)
	}
	write(t, client.credential, "{}")
	directory := "expected one directory for init/check/push; place options before the directory"
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"login", "foo"}, "login takes no positional arguments"},
		{[]string{"login", "foo", "--google"}, "login takes no positional arguments"},
		{[]string{"logout", "foo"}, "logout takes no positional arguments"},
		{[]string{"init"}, directory},
		{[]string{"check", "a", "b"}, directory},
		{[]string{"push"}, directory},
		{[]string{"pull", "id"}, "expected pull <problem-id> <directory>; place options before the problem ID"},
	} {
		if err := run(context.Background(), tc.args, io.Discard); err == nil || err.Error() != tc.want {
			t.Errorf("%v: got %v, want %q", tc.args, err, tc.want)
		}
	}
	// A rejected logout keeps the saved credentials.
	if _, err := os.Stat(client.credential); err != nil {
		t.Fatalf("credentials removed: %v", err)
	}
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -run TestPositionalArguments ./...`
Expected: FAIL。`login`/`logout`の3行が`got expected one directory for init/check/push; ...`になる。残りの4行と認証情報の確認は通る。

- [ ] **Step 3: `run`の検査を分ける**

`cli/main.go`の今の`if (needsDirectory && flags.NArg() != 1) || (!needsDirectory && command != "pull" && flags.NArg() != 0) {`を次の2つに分ける。順序は`pull`の検査の後、`--expect-version`の検査の前のままにする。
- `needsDirectory && flags.NArg() != 1`なら、今の`expected one directory for init/check/push; place options before the directory`を返す。
- `(command == "login" || command == "logout") && flags.NArg() != 0`なら、`fmt.Errorf("%s takes no positional arguments", command)`を返す。

- [ ] **Step 4: 通ることを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test ./...`
Expected: PASS（`TestPositionalArguments`、`TestPullLifecycle`の不正な引数の行、`TestAuthoringGuide`を含む）

- [ ] **Step 5: コミット**

```bash
git add cli/main.go cli/main_test.go
git commit -m "fix: loginとlogoutに余分な引数を渡したときに、そのコマンドに合った文言を出す"
```

### Task 2: D10 問題文と解説のひな形を1つのファイルにまとめ、CLIとWebで使う

**Files:**
- Create: `cli/problem-template.json`
- Modify: `cli/main.go`（`initProblem`の`statement.md`・`editorial.md`の文字列、今の244-245行。import）
- Modify: `web/app/utils/problem-draft.ts`（import、今の122-135行の`initialProblemMarkdown`・`initialEditorialMarkdown`、139行の`initialProblemDraft`）
- Test: `cli/main_test.go`（末尾に追加）、`web/tests/editor.spec.ts`（`creation page has noindex ...`の後に追加）

**Interfaces:**
- Consumes: Task 1とは独立
- Produces: `cli/problem-template.json` = `{"statement": string, "editorial": string}`。Goは`var markdownTemplate []byte`（`//go:embed problem-template.json`）、Webは`markdownTemplate`という名前でdefault importする

- [ ] **Step 1: 失敗するテストを2つ書く**

`cli/main_test.go`の末尾に足す。`decodeJSON`は`cli/problem.go:403`の既存のヘルパー。

```go
func TestInitUsesProblemTemplate(t *testing.T) {
	// The website's problem editor starts from the same file (web/app/utils/problem-draft.ts).
	data, err := os.ReadFile("problem-template.json")
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Statement string `json:"statement"`
		Editorial string `json:"editorial"`
	}
	if err := decodeJSON(data, &want); err != nil || want.Statement == "" || want.Editorial == "" {
		t.Fatalf("template: %+v %v", want, err)
	}
	dir := example(t)
	for name, text := range map[string]string{"statement.md": want.Statement, "editorial.md": want.Editorial} {
		if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != text {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
}
```

`web/tests/editor.spec.ts`の1行目の後に`import { readFileSync } from 'node:fs'`を足し、最初のテストの後に足す。

```ts
test('a new problem starts from the statement and editorial shared with shareoj init', async ({ page }) => {
  const template = JSON.parse(readFileSync(new URL('../../cli/problem-template.json', import.meta.url), 'utf8'))
  await page.goto('/problems/new?fresh=1')
  await page.getByLabel('問題のタイトル', { exact: true }).fill('ひな形')
  const saved = page.waitForRequest(request => request.method() === 'PUT' && /^\/api\/my\/problems\/[^/]+$/.test(new URL(request.url()).pathname))
  await page.getByRole('button', { name: '保存', exact: true }).click()
  const { draft } = (await saved).postDataJSON()
  expect(draft.markdown).toBe(template.statement)
  expect(draft.editorial).toBe(template.editorial)
})
```

- [ ] **Step 2: 両方が失敗することを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -run TestInitUsesProblemTemplate ./...`
Expected: FAIL（`open problem-template.json: no such file or directory`）

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`
Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npx playwright test tests/editor.spec.ts -g "shared with shareoj init" --output /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b4`
Expected: FAIL（`ENOENT ... cli/problem-template.json`）

- [ ] **Step 3: 共有ファイルを作り、変更前のCLIとWebに一致することを確かめる**

Writeで`cli/problem-template.json`を作る。中身は次のとおり（UTF-8、2スペースの字下げ、末尾に改行1つ）。JSONの文字列リテラルは、今の`cli/main.go:244-245`のGoの文字列リテラルと同じ表記になる。

```json
{
  "statement": "## 問題文\n\n<!-- ここに問題文を記載 -->\n\n$T$ 個のテストケースが与えられるので、それぞれについて答えてください。\n\n## 制約\n\n- 入力される値はすべて整数\n\n## 入力\n\n入力は以下の形式で標準入力から与えられる。\n```input\n$T$\n$\\mathrm{case}_1$\n$\\mathrm{case}_2$\n$\\vdots$\n$\\mathrm{case}_T$\n```\n\n各テストケース $\\mathrm{case}_t ~ (1 \\leq t \\leq T)$ は以下の形式で与えられる。\n```input\n```\n\n## 出力\n\n$T$ 行出力せよ。 $t$ 行目には $t$ 番目のテストケースについての答えを出力せよ。\n\n<!-- テストケース画面の「サンプルを問題文に追加」ボタンから、サンプルを追加できます。 -->\n",
  "editorial": "## 解説\n\n<!-- ここに解説を記載 -->\n"
}
```

Step 2の2つのコマンドのうち、`go test`と`npx playwright test`をもう一度流す（コードはまだ変えていない。Webのビルドもし直さない）。
Expected: 両方PASS。これで、共有ファイルの中身が変更前のCLIの文字列とWebの文字列の両方と同じだとわかる。どちらかが落ちたら、コードではなくJSONを直す。

- [ ] **Step 4: CLIを共有ファイルに切り替える**

`cli/main.go`:
- importに`_ "embed"`を足す（`cli/guide.go`と同じ書き方）。
- `const template`の後、`initProblem`の前に置く。

```go
// The statement and editorial a new problem starts with. The website's problem editor imports the same file
// (web/app/utils/problem-draft.ts), so both start from the same text.
//go:embed problem-template.json
var markdownTemplate []byte
```

- `initProblem`の先頭、`os.Mkdir`より前で、`decodeJSON(markdownTemplate, &text)`を呼ぶ。`text`は`Statement string \`json:"statement"\``と`Editorial string \`json:"editorial"\``を持つ無名の構造体にする。失敗したらそのエラーを返す。`Mkdir`より前に置くので、読めなかったときに空のディレクトリを残さない。
- `files`の`"statement.md"`と`"editorial.md"`の値を`text.Statement`と`text.Editorial`にし、文字列リテラルを消す。ほかの項目と、既存のディレクトリを上書きしない動作は変えない。

- [ ] **Step 5: Webを共有ファイルに切り替える**

`web/app/utils/problem-draft.ts`:
- 1行目のimportの後に、次の2行を置く。

```ts
// The statement and editorial a new problem starts with, shared with `shareoj init`. The import attribute lets the
// Playwright tests, which load this module in Node, read the JSON too.
import markdownTemplate from '../../../cli/problem-template.json' with { type: 'json' }
```

- `initialProblemMarkdown`（122-133行）と`initialEditorialMarkdown`（135行）を消す。
- `initialProblemDraft()`の`markdown: initialProblemMarkdown, editorial: initialEditorialMarkdown`を`markdown: markdownTemplate.statement, editorial: markdownTemplate.editorial`にする。ほかのキー、順序、型の注記は変えない。

- [ ] **Step 6: 通ることと、写しが残っていないことを確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test ./...`
Expected: PASS（`TestInitUsesProblemTemplate`、`TestPushLifecycle`、`TestProblemValidationAndRime`を含む）

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`
Expected: エラーなしで終わる

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build`
Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npx playwright test tests/editor.spec.ts tests/problem-markdown.spec.ts --output /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b4`
Expected: PASS（新しいテストと、読み込み失敗で初期値に戻すテスト`editor.spec.ts:130`を含む）

Run: `grep -rn "ここに問題文を記載\|ここに解説を記載" /home/loop/judge/.claude/worktrees/refactor-maintainability/cli /home/loop/judge/.claude/worktrees/refactor-maintainability/web/app`
Expected: 出るのは`cli/problem-template.json`と`cli/main_test.go`（`TestPushLifecycle`の解説の確認）だけ。`cli/main.go`と`web/app`に写しが残っていない。

- [ ] **Step 7: コミット**

```bash
git add cli/problem-template.json cli/main.go cli/main_test.go web/app/utils/problem-draft.ts web/tests/editor.spec.ts
git commit -m "refactor: 新規作成時の問題文と解説のひな形を1つのファイルにまとめ、CLIとWebの両方から使う"
```

## 検証コマンド（controllerが全体を流す）

- CLI: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test ./...`
- CLI: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...`
- CLI: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .`（何も出ない）
- CLI: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/cli && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go build -trimpath -o /tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/shareoj .`
- Web: `npm run typecheck`、`npm run build`、ブラウザーのPlaywright（`npx playwright test --output ...`）、アカウントのPlaywright（`npx playwright test --config playwright.account.config.ts --output ...`、テスト用DBは127.0.0.1:25432）、`npm run package:lambda`、`npm run test:lambda`、`node --test scripts/smoke-frontend.test.mjs`。いずれも`/home/loop/judge/.claude/worktrees/refactor-maintainability/web`で、`nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command`を付けて実行する。

## Out of scope

- `problem.toml`のひな形（`const template`）、生成器のコード、テスト例。Webに同じものはなく、仕様の対象は問題文と解説だけ。
- サーバーのzodの既定値（`problemDraftSchema`の`editorial`は`''`）。C04の計画にあるとおり、意図して違う値になっている。
- 同じ`problem-draft.ts`の`exportProblemMarkdown`と`draftErrors`の未使用の項目。B5のD16で扱う。
- `init`/`check`/`push`/`pull`の文言の見直し。
- `docs/refactor/FIXES.md`の「まとまりと順序」の状態とコミットの更新。controllerが行う。
