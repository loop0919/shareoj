# 誤差ジャッジ Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 検証コードを書かずに、提出の出力と期待出力を絶対誤差・相対誤差で比べる判定方法「誤差ジャッジ」を、ワーカー、API、bridge、Web、CLIに加える。

**Architecture:** 比較は新しいモジュール`judge/runner/tolerance.py`に置き、ワーカーの通常判定（`host.case_result`）が、ジョブに`errorJudge`があるときだけ呼ぶ。下書きとジョブには`checker`、`interactor`、`runTwice`と排他の`errorJudge`（設定値だけでコードなし）を足し、API、bridge、Web、CLIは2ステップ形式（`runTwice`）で足した箇所に並べて足す。古いワーカーが完全一致で採点した結果は、2ステップ形式と同じ「結果の印」でJEにする。

**Tech Stack:** Python（`judge/runner`、unittest、標準ライブラリだけ）、Go（`api/internal/problems`、`api/internal/submissions`、`api/cmd/judge-bridge`、`cli`）、Nuxt 4（Vue 3、zod）、Playwright

**Spec:** [docs/superpowers/specs/2026-11-12-error-judge-design.md](../specs/2026-11-12-error-judge-design.md)（土台は[2ステップ形式の仕様](../../judge/run-twice.md)と[スペシャルジャッジの仕様](../../judge/special-judge.md)）

## Global Constraints

- 下書きとジョブの項目は`errorJudge`、形は`{"mode": "absolute" | "relative" | "either", "epsilon": "1e-1" … "1e-12", "marginPercent"?: "<10進>"}`。
- `epsilon`は`"1e-1"`、`"1e-2"`、…、`"1e-12"`の12個の文字列だけ。
- `marginPercent`は省略と`null`で0.1。指定するなら正規表現`^[0-9]{1,3}(\.[0-9]{1,6})?$`に合い、値が0以上100以下。
- `checker`、`interactor`、`runTwice`、`errorJudge`は同時に2つ以上を指定できない。生成用と入力検証用のジョブには付けない。
- しきい値は $T = \varepsilon \times (1 + m/100)$。絶対誤差は $\lvert a-b \rvert \le T$、相対誤差は $\lvert a-b \rvert \le T \times \lvert b \rvert$、`either`はどちらか（$a$は提出、$b$は期待出力）。
- 数の形は`-?([0-9]+|[0-9]+\.[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?`の全体一致。400バイトを超えるトークンと、指数の絶対値が400を超えるトークンは数としない。
- トークンの分け方は通常判定と同じ（`[^ \t\n\r\v\f]+`）。トークン数が違えば、数の比較をせずにWA。
- 1組の判定は、バイト列の一致、両方が数なら条件、それ以外は不一致、の順。
- 比較の結果は、有理数（`fractions.Fraction`）だけで計算した場合と必ず同じにする。
- 公開APIは`errorJudge`の`mode`と`epsilon`だけを返す。`marginPercent`は返さない。
- 結果の印は`errorJudge: true`。印のない非JEの結果は、bridgeがJEとして保存し、`platform`の`unmarked_error_judge_result`を記録する。印は保存せず、応答に出さない。
- 画面の文言：選択肢「誤差ジャッジ」。種類の表示は「絶対誤差」「相対誤差」「絶対誤差または相対誤差」。入力欄の名前は「種類」「許容誤差」「判定の余裕」。問題ページは「誤差ジャッジ：<種類>が10<sup>-k</sup>以下なら正解です。」。
- 初期値：種類`either`、許容誤差`1e-6`、判定の余裕`0.1`。
- CLI：`problem.toml`の表は`[error_judge]`、キーは`mode`、`epsilon`、`margin_percent`。4つの重複の誤りは`choose one of checker, interactor, run_twice or error_judge`。
- 採点ホストに追加のパッケージやコンパイル済みの補助を入れない。
- ブランチ`feat/error-judge`（`feat/run-twice`から分岐）で作業し、`main`へpushしない。
- コマンドはworktreeのルートで、単独のコマンドとして実行する（`bash -c`、`cd`、gitとほかのコマンドの連結は保護に止められる）。Pythonは`python3 -m unittest discover -s judge/tests -v`（1ファイルは`-p <file>`）。Goは`env TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable' nix develop -c go -C api test ./...`と`nix develop -c go -C cli test ./...`。Webは`nix develop -c bun install --cwd web`、`nix develop -c bun run --cwd web build`、`nix develop -c bun run --cwd web test <file> --output=<scratchpad>/pw`、`nix develop -c bun run --cwd web typecheck`。
- コミットメッセージは`prefix: 対応内容`（日本語）。コードを変えたら`graphify update .`を実行する。

## Review Focus

- **提出が指数表記や末尾の小数点で出力する**：`1E-7`、`2.`、`.5`、`-0.0`を数として読み、値で比べる。`+1.0`は数としないので、期待出力が`1.0`ならWA（Task 1「数の形」）。
- **期待出力が0で、相対誤差**：提出が`0`、`0.0`、`-0`なら一致し、`1e-300`は不一致。`either`では絶対誤差で一致する（Task 1「期待出力が0」）。
- **文字列と数が混ざる出力**：`Yes 3.14`に対して`Yes 3.1400001`は一致、`yes 3.14`は不一致、`3.14 Yes`は不一致（Task 1「文字列の混在」）。
- **トークン数だけ多い巨大な出力**：期待出力が1個のとき、提出が数百万個の数を出しても、数の比較をせずにWAになり、時間もメモリも使わない（Task 1「トークン数の違い」）。
- **余裕の入力が空や`0.1.2`や`101`**：Webは保存せず誤りを出し、APIは拒否する。既存の下書き（`errorJudge`なし）は何も変わらない（Task 3「形の検査」、Task 5「余裕の誤り」）。

---

### Task 1: 比較のモジュール

**Files:**
- Create: `judge/runner/tolerance.py`
- Modify: `judge/runner/fingerprint.py`、`judge/build-assets.sh`、`judge/deploy/install.sh`（配布するファイルの一覧に`tolerance.py`を足す）
- Test: `judge/tests/test_tolerance.py`（新規）、`judge/tests/test_release.py`

**Interfaces:**
- Produces:
  - `tolerance.validate(config) -> None`。Global Constraintsの形に合わなければ`ValueError`。
  - `tolerance.matches(actual: bytes, expected: bytes, config: dict) -> bool`。`config`は`validate`を通ったもの。
  - `tolerance.EPSILONS`（12個の文字列のタプル）、`tolerance.DEFAULT_MARGIN = '0.1'`。

- [ ] **Step 1: 失敗するテストを書く**

`judge/tests/test_tolerance.py`。`test_runner.py`と同じ方法で`judge/runner`をimportできるようにする。

```python
def test_boundary_is_exact_in_each_mode(self):
    # mode=absolute, epsilon=1e-6, marginPercent='0': 期待 1 に対し 1.000001 は一致、1.0000010000000001 は不一致
    # mode=relative, epsilon=1e-6, marginPercent='0': 期待 1000 に対し 1000.001 は一致、1000.0010000001 は不一致
    # mode=either,   epsilon=1e-6, marginPercent='0': 期待 0.5 に対し 0.500001 は一致（絶対）、
    #   期待 1000 に対し 1000.001 は一致（相対）、期待 1000 に対し 1000.0011 は不一致

def test_margin_widens_the_threshold(self):
    # 省略（0.1%）: 期待 0、absolute、1e-6 に対し 0.000001001 は一致、0.0000010010000001 は不一致
    # marginPercent='100': 0.000002 は一致、0.0000020000001 は不一致
    # marginPercent='0': 0.000001001 は不一致

def test_decimal_values_are_not_rounded_through_floats(self):
    # absolute, 1e-1, margin 0: 期待 0.3 に対し 0.2 は一致（差はちょうど 0.1）、0.19999999999999999 は不一致
    # 浮動小数点では 0.3-0.2 > 0.1 になる組である

def test_number_forms(self):
    # 数として読む: 3, -0.5, 2., .25, 1e-3, 6.02E+23, -0, 0.0
    #   期待 2 に対し 2. と 2.0 と 2e0 は一致。期待 1000 に対し 1e3 は一致。期待 0 に対し -0 と -0.0 は一致
    # 数としない: +1.0, 0x10, 1_000, nan, NaN, inf, -inf, infinity, 1e, e5, ., -, 1.2.3
    #   同じ文字列どうしは一致（nan 対 nan）。期待 1.0 に対し +1.0 は不一致。期待 inf に対し Infinity は不一致

def test_oversized_tokens_are_compared_as_text(self):
    # 401バイトの数字列どうしが同じなら一致、最後の1桁だけ違えば不一致（条件内でも）
    # 1e401 と 1e401 は一致、1e401 と 1.0e401 は不一致。1e400 と 1.0e400 は一致
    # 1e-401 と 0 は不一致、1e-400 と 0（absolute, 1e-6）は一致

def test_expected_zero_in_relative_mode(self):
    # relative: 期待 0 に対し 0, 0.0, -0 は一致、1e-300 は不一致
    # either: 期待 0 に対し 1e-300 は一致

def test_text_tokens_need_exact_match(self):
    # 期待 b'Yes 3.14\n' に対し b'Yes 3.1400001'（absolute 1e-6）は一致、b'yes 3.14' と b'3.14 Yes' は不一致
    # 空白と改行の違い（b'1 2\n' 対 b'1\n2'、CRLF、末尾の空白）は一致

def test_token_count_mismatch_is_rejected_before_comparing_numbers(self):
    # 期待 b'1' に対し b'1 1' と b'' は不一致
    # 期待 b'1' に対し b'1.0 ' * 2_000_000 は不一致で、1秒以内に返る

def test_validate_rejects_malformed_config(self):
    # 通る: {'mode': 'either', 'epsilon': '1e-6'}、marginPercent '0', '0.5', '100', '12.345678'
    # ValueError: None, {}, mode 'both', epsilon '1e-13' / '1e-0' / '0.000001' / 1e-6（数値）,
    #   marginPercent '100.000001', '101', '-1', '1e1', '.5', '0.1234567', 0.1（数値）, 余分なキー

def test_fast_path_agrees_with_fractions(self):
    # 乱数（seed固定）で作った20,000組を、3つの種類とε 1e-3, 1e-6, 1e-12、余裕 0, 0.1 で、
    # Fraction だけで書いた参照実装（テストの中に書く）と比べ、すべて同じ結果になる
    # 組は、しきい値ちょうど、その前後1桁、桁数の多い値、指数表記を混ぜる

def test_million_boundary_pairs_finish_in_time(self):
    # 期待 b'0\n' * 1_000_000、提出 b'0.000001001\n' * 1_000_000（absolute 1e-6、余裕省略）→ 一致
    # 経過時間を測り、30秒以内。実測値を print ではなく self.assertLess のメッセージに入れる
```

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s judge/tests -p test_tolerance.py -v`
Expected: `ModuleNotFoundError: No module named 'tolerance'`

- [ ] **Step 3: `judge/runner/tolerance.py`を書く**

トークン数は、数の比較の前に数える。出力をトークンのリストに丸ごと展開せず、`re.finditer`などで順に取り出す。速い経路を作るかどうかは任せるが、作るなら境界に近い組は必ず正確な計算へ回す。`int()`の桁数制限（4,300桁）に掛からないよう、数として読むのは400バイト以下のトークンだけにする。

- [ ] **Step 4: 配布の一覧に`tolerance.py`を足す**

`fingerprint.py`の計算対象、`build-assets.sh`の`cp`、`deploy/install.sh`の`install`の3か所。`host.py`と同じ並びに足し、`test_release.py`が一覧を検査していればそのテストも直す。

- [ ] **Step 5: 通ることを確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: 全件PASS。`test_million_boundary_pairs_finish_in_time`の実測値を報告に書く。

- [ ] **Step 6: コミット**

```bash
git add judge/runner/tolerance.py judge/runner/fingerprint.py judge/build-assets.sh judge/deploy/install.sh judge/tests
git commit -m "feat: 出力を誤差の条件で比べるモジュールを加える"
```

### Task 2: ワーカーのジョブ処理とsmoke

**Files:**
- Modify: `judge/runner/host.py`（`validate_job`、`case_result`、`judge`、`validate_code_run`）
- Modify: `judge/local.py`、`judge/runner/smoke.py`
- Test: `judge/tests/test_tolerance.py`、`judge/tests/test_smoke.py`、`judge/tests/test_runner.py`

**Interfaces:**
- Consumes: `tolerance.validate`、`tolerance.matches`（Task 1）
- Produces: ジョブの`errorJudge`を受け付ける`host.judge`。`errorJudge`のあるジョブの結果（JE以外。CEを含む）に`errorJudge: True`が付く。

- [ ] **Step 1: 失敗するテストを書く**

2ステップ形式のテスト（`judge/tests/test_run_twice.py`の`twice_job`とhostのテスト）を手本にする。

```python
def test_jobs_reject_malformed_or_conflicting_error_judge(self):
    # errorJudge と checker / interactor / runTwice のどれかが同時 → ValueError
    # errorJudge と generate / validate → ValueError
    # errorJudge の形が誤り（epsilon '1e-13'）→ ValueError
    # errorJudge だけ → 通る

def test_host_compares_with_tolerance(self):
    # sandbox.execute を差し替え、期待出力 '0.5\n' のケースに提出が '0.5000001\n' を出す
    # errorJudge {'mode': 'absolute', 'epsilon': '1e-6'} → AC、errorJudge なし → WA
    # 提出が '0.50001\n' → WA

def test_results_of_error_judge_jobs_carry_the_mark(self):
    # AC、WA、CE の結果に errorJudge == True。errorJudge のないジョブの結果にキーがない
    # runTwice のジョブの結果に errorJudge キーがない

def test_sample_check_keeps_sample_details(self):
    # easyTest=True の errorJudge ジョブで、ケースに sampleDetails（input、expectedOutput、actualOutput）が付く

def test_group_scoring_uses_tolerance_verdicts(self):
    # グループ方式で、誤差内の出力のケースがACとして数えられ、グループの配点が付く

def test_code_run_rejects_error_judge(self):
    # validate_code_run が errorJudge を持つジョブを拒否する（test_runner.py の既存のテストを広げる）

def test_error_judge_smoke_covers_three_modes(self):
    # test_smoke.py。judge を差し替え、3つの種類それぞれで
    # 「しきい値ちょうどを出す提出 → AC」「しきい値を1桁超える提出 → WA」のジョブが送られ、
    # 結果が違えば smoke が失敗する
```

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: 新しいテストがFAIL

- [ ] **Step 3: `host.py`を直す**

- `validate_job`：4つのうち2つ以上は`ValueError('conflicting judge modes')`。`errorJudge`は`tolerance.validate`で検査し、`generate`、`validate`と一緒なら拒否する。
- `case_result`：キーワード引数`error_judge=None`を足す。`None`でなければ、完全一致の比較の代わりに`tolerance.matches(output, case['output'].encode(), error_judge)`を使う。
- `judge`：`case_result`に`job.get('errorJudge')`を渡す。結果の印は、2ステップ形式の`marker`と同じ場所で`errorJudge=True`を足す。
- `validate_code_run`：禁止するキーに`'errorJudge'`を足す。
- `judge/local.py`：`errorJudge`のジョブを「誤差ジャッジはisolateワーカーで実行してください。」で拒否する。

- [ ] **Step 4: `smoke.py`に誤差ジャッジの検証を足す**

通常判定のsmokeと同じ作り方で、提出は入力を読まず固定の値を出す`python314`のプログラムにする。3つの種類で、しきい値ちょうどとその1桁上を確かめる。あわせて、`tolerance.matches`で「期待`0`、提出`0.000001001`を100万組」の経過時間を測り、smokeのレポート（既存の出力の形に合わせる）にミリ秒で記録する。この実測は合否に使わない。

- [ ] **Step 5: 通ることを確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: 全件PASS

- [ ] **Step 6: コミット**

```bash
git add judge/runner/host.py judge/runner/smoke.py judge/local.py judge/tests
git commit -m "feat: ワーカーが誤差ジャッジのジョブを採点する"
```

### Task 3: 下書きと公開（API）

**Files:**
- Modify: `api/internal/problems/store.go`（`Draft`）、`api/internal/problems/validation.go`、`api/internal/problems/publication.go`
- Test: `api/internal/problems/error_judge_test.go`（新規）

**Interfaces:**
- Produces:
  - `type ErrorJudge struct { Mode string; Epsilon string; MarginPercent *string }`（JSONは`mode`、`epsilon`、`marginPercent,omitempty`）
  - `Draft.ErrorJudge *ErrorJudge`（`json:"errorJudge,omitempty"`）
  - `func (e *ErrorJudge) Valid() bool`
  - 公開問題の`ErrorJudge *PublicErrorJudge`（`json:"errorJudge,omitempty"`、`PublicErrorJudge`は`Mode`と`Epsilon`だけ）

- [ ] **Step 1: 失敗するテストを書く**

`api/internal/problems/run_twice_test.go`を手本にする。

```go
// 形の検査: mode absolute/relative/either、epsilon 1e-1〜1e-12、marginPercent 省略・"0"・"0.5"・"100"・"12.345678" は有効。
// mode "both"、epsilon "1e-13"・"1e-0"・"0.000001"、marginPercent "100.000001"・"101"・"-1"・"1e1"・".5"・"0.1234567"・"" は無効。
// 排他: ErrorJudge と Checker / Interactor / RunTwice のどれかが同時なら無効。ErrorJudge だけは有効。
// 公開準備: ErrorJudge だけの下書きに、判定コードの指摘（*_source_missing、*_runtime_unavailable）が出ない。
// 公開: ErrorJudge のある下書きから作った公開問題のJSONに "errorJudge":{"mode":"either","epsilon":"1e-6"} があり、
//   marginPercent を含まない。ErrorJudge のない下書きでは errorJudge キーがない。
```

- [ ] **Step 2: 失敗を確かめる**

Run: `nix develop -c go -C api test ./internal/problems/...`
Expected: コンパイルエラー（`ErrorJudge`が未定義）

- [ ] **Step 3: 3つのファイルを直す**

- [ ] **Step 4: 通ることを確かめる**

Run: `nix develop -c go -C api test ./internal/problems/...`、`nix develop -c go -C api build ./...`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/problems
git commit -m "feat: 下書きと公開問題に誤差ジャッジの設定を加える"
```

### Task 4: 提出とbridge（API）

**Files:**
- Modify: `api/internal/submissions/store.go`（`Job`、`Result`、`judgeReady`、`CreateRun`のジョブ組み立て）、`finish.go`、`rejudge.go`
- Modify: `api/cmd/judge-bridge/dispatch.go`、`results.go`
- Modify: `docs/judge/observability.md`（`platform`の行に`unmarked_error_judge_result`）
- Test: `api/internal/httpapi/submissions_test.go`、`api/cmd/judge-bridge/*_test.go`

**Interfaces:**
- Consumes: `problems.ErrorJudge`（Task 3）
- Produces: ジョブの`ErrorJudge *problems.ErrorJudge`（`json:"errorJudge,omitempty"`）、`submissions.ErrUnmarkedErrorJudge`、bridgeがワーカーへ送る要求の`"errorJudge"`。

- [ ] **Step 1: 失敗するテストを書く**

2ステップ形式のテスト（`submissions_test.go`の run twice の節、bridgeのテスト）を手本にする。

```go
// 受付: errorJudge の下書きへの提出は、job に errorJudge と cases が固定され、その後の下書きの編集で変わらない。
// 受付: 下書きに errorJudge と checker（または interactor、runTwice）の両方があるとき受け付けない。
// 受付: 提出の runtime が cpp17-local、cpp23-gcc-local、python314-local のとき受け付けない。
// 結果: errorJudge のジョブの結果は Interactive == false で、easyTest では sampleDetails を残す。
// リジャッジ: 作り直した job に errorJudge が入る。
// 印: errorJudge のジョブに、印のない AC / WA / CE の結果 → ErrUnmarkedErrorJudge。印のある結果はそのまま保存され、
//   保存した結果と応答に errorJudge の印がない。errorJudge のないジョブに印のある結果 → ErrInvalid。
// bridge: 要求に "errorJudge" が入る。4つのうち2つ以上を持つ job は conflicting_judge_modes でJE。
// bridge: 印のない結果は JE として保存され、platform の unmarked_error_judge_result が記録される。
//   部分点の job で印も scoring もない結果は、誤差ジャッジの理由が1回だけ記録される。
```

- [ ] **Step 2: 失敗を確かめる**

Run: `env TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable' nix develop -c go -C api test ./...`
Expected: 新しいテストがFAIL

- [ ] **Step 3: 直す**

`judgeReady`には、`errorJudge`があるときの節（ほかの3つが`null`、提出のruntimeがローカルでない）を足し、既存の`checker`、`interactor`、`runTwice`の節には「`errorJudge`が`null`」を足す。印の検査は`finish.go`の`runTwiceMark`の隣に置き、2つを1つの関数にまとめるかは任せる。

- [ ] **Step 4: 通ることを確かめる**

Run: Step 2と同じコマンド、`nix develop -c go -C api vet ./...`、`nix develop -c gofmt -l api`
Expected: PASS、`gofmt`の出力なし

- [ ] **Step 5: コミット**

```bash
git add api docs/judge/observability.md
git commit -m "feat: 誤差ジャッジの提出を受け付けて採点台へ送る"
```

### Task 5: Web

**Files:**
- Create: `web/app/utils/error-judge.ts`
- Modify: `web/shared/types/problem-draft.ts`、`web/shared/types/problem.ts`、`web/app/utils/problem-draft.ts`
- Modify: `web/app/components/ProblemChecker.vue`、`web/app/pages/problems/new.vue`
- Modify: `web/app/pages/problems/[id]/index.vue`、`web/app/pages/contests/[id]/[label].vue`、`web/server/api/problems/[id].get.ts`
- Modify: `web/tests/fixtures/public-api.mjs`（誤差ジャッジの問題を1つ足す）
- Test: `web/tests/checker.spec.ts`、`web/tests/error-judge.spec.ts`（新規、`error-judge.ts`の単体）

**Interfaces:**
- Consumes: APIの`errorJudge`（Task 3、4）
- Produces:
  - 下書きの`errorJudge`：`z.object({ mode: z.enum(['absolute', 'relative', 'either']), epsilon: z.enum([...12個]), marginPercent: z.string().regex(/^[0-9]{1,3}(\.[0-9]{1,6})?$/).refine(100以下).optional() }).nullable().default(null)`
  - 公開問題の`errorJudge`：`z.object({ mode, epsilon }).nullable().default(null)`
  - `web/app/utils/error-judge.ts`：
    - `errorJudgeModeLabel(mode): string`（「絶対誤差」「相対誤差」「絶対誤差または相対誤差」）
    - `validMargin(text: string): boolean`
    - `thresholdFactor(marginPercent: string): string`（`'0.1'`→`'1.001'`、`'0'`→`'1'`、`'100'`→`'2'`、`'12.345678'`→`'1.12345678'`。文字列の10進演算で正確に求め、末尾の0を落とす）
    - `epsilonExponent(epsilon: string): number`（`'1e-6'`→`-6`）
  - `ProblemChecker`の`v-model:error-judge`

- [ ] **Step 1: 失敗するテストを書く**

```ts
// web/tests/error-judge.spec.ts
test('しきい値の倍率を正確に求める', ...)
// thresholdFactor: '0.1'→'1.001', '0'→'1', '100'→'2', '12.345678'→'1.12345678', '0.000001'→'1.00000001', '50'→'1.5'
test('余裕の形を検査する', ...)
// validMargin: '0', '0.5', '100', '12.345678' は true。'', '101', '100.000001', '-1', '1e1', '.5', '0.1.2', '0.1234567' は false

// web/tests/checker.spec.ts
test('誤差ジャッジを選ぶと下書きに errorJudge が入る', ...)
// 「誤差ジャッジ」を選ぶ → 保存した下書きが errorJudge: { mode: 'either', epsilon: '1e-6', marginPercent: '0.1' }、
// checker・interactor・runTwice は null。検証コードの欄が出ない

test('しきい値の説明が入力に合わせて変わる', ...)
// 初期状態: 「期待出力との誤差が 1.001 × 10-6 以下なら正解になります。」と
//   「想定解の出力も真の値からわずかにずれるので、その分だけ広げて比べます。」が見える（指数は <sup>）
// 許容誤差を 10^-9、余裕を 0.5 にする → 「1.005 × 10-9 以下」
// 種類を相対誤差にする → 「期待出力との相対誤差が…」
// 余裕を 0 にする → 「期待出力との誤差が 10-9 を少しでも超えると不正解になります。」だけになる

test('余裕の誤りは保存しない', ...)
// 余裕を空、'101'、'0.1.2' にする → 誤りの文言が出て、下書きの保存要求が飛ばない（余裕の誤り）

test('誤差ジャッジからほかの判定方法へ切り替えると errorJudge が null になる', ...)
// 誤差ジャッジ → スペシャルジャッジ → errorJudge は null で checker にコード欄が出る。逆も確かめる

test('問題ページに誤差ジャッジの条件を表示する', ...)
// errorJudge: { mode: 'either', epsilon: '1e-6' } の問題ページとコンテストの問題ページに
// 「誤差ジャッジ：絶対誤差または相対誤差が10-6以下なら正解です。」がある（指数は <sup>）。通常の問題にはない
```

- [ ] **Step 2: 失敗を確かめる**

Run: `nix develop -c bun run --cwd web build`、続けて`nix develop -c bun run --cwd web test checker.spec.ts error-judge.spec.ts --output=<scratchpad>/pw`
Expected: 新しいテストがFAIL

- [ ] **Step 3: 型、`error-judge.ts`、下書きの初期値を直す**

- [ ] **Step 4: `ProblemChecker.vue`を直す**

判定方法の値に`'errorJudge'`を足す。誤差ジャッジを選んだときは、言語・形式・コードの欄を出さず、次を出す。

```text
種類        [絶対誤差または相対誤差 ▾]
許容誤差    [10^-6 ▾]
判定の余裕  [0.1] %

期待出力との誤差が 1.001 × 10^-6 以下なら正解になります。
想定解の出力も真の値からわずかにずれるので、その分だけ広げて比べます。
数として読めるトークンだけを誤差で比べ、それ以外は完全一致で比べます。
```

- しきい値は`thresholdFactor`と`epsilonExponent`から組み立て、指数は`<sup>`で出す。倍率が`1`のときは「× 」を省く。
- 種類が相対誤差なら「期待出力との相対誤差が」、`either`なら「期待出力との絶対誤差または相対誤差が」と書く。絶対誤差は「期待出力との誤差が」。
- 余裕が0のときは、最初の2行を「期待出力との<誤差の言い方> 10<sup>-k</sup> を少しでも超えると不正解になります。」の1行に替える。
- 余裕が`validMargin`に合わないときは、しきい値の行の代わりに「判定の余裕は0〜100の数で、小数は6桁までです。」を出し、`errorJudge`を書き換えない（直前の正しい値を保つ）。保存は既存の検証の仕組みに合わせて止める。
- 最後に[判定コードのドキュメント](../../../web/app/content/docs/checkers.md)の`#error-judge`へのリンクを置く。
- ほかの判定方法へ切り替えたら`errorJudge`を`null`にする。誤差ジャッジへ切り替えたら、ほかの3つを`null`にし、初期値を入れる。ほかの3つのあいだの切り替えは変えない。

- [ ] **Step 5: 問題ページ、コンテストの問題ページ、`[id].get.ts`、fixtureを直す**

表示は対話形式の注記と同じ位置と見た目にする。`[id].get.ts`は下書きの`errorJudge`から`mode`と`epsilon`だけを返す。

- [ ] **Step 6: 通ることを確かめる**

Run: Step 2と同じコマンド、`nix develop -c bun run --cwd web typecheck`、最後に全件`nix develop -c bun run --cwd web test --output=<scratchpad>/pw`
Expected: 全件PASS、型エラーなし

- [ ] **Step 7: コミット**

```bash
git add web
git commit -m "feat: 問題編集と問題ページに誤差ジャッジを加える"
```

### Task 6: CLI

**Files:**
- Modify: `cli/problem.go`、`cli/pull.go`、`cli/main.go`
- Modify: `cli/README.md`、`cli/guide.md`、`web/app/content/docs/cli.md`
- Test: `cli/main_test.go`、`cli/pull_test.go`

**Interfaces:**
- Produces: `config.ErrorJudge`（`toml:"error_judge"`、キーは`mode`、`epsilon`、`margin_percent`）。下書きのキーは`errorJudge`、`margin_percent`は`marginPercent`になる。

- [ ] **Step 1: 失敗するテストを書く**

```go
// push: [error_judge] mode="either" epsilon="1e-6" は下書きの errorJudge {mode, epsilon} になる（marginPercent なし）。
// push: margin_percent = "0.5" は marginPercent "0.5" になる。
// push: mode "both"、epsilon "1e-13"、margin_percent "101" は、APIへ送る前に誤りになる。
// push: 4つの表のうち2つ以上は "choose one of checker, interactor, run_twice or error_judge" で失敗する（6通りの組）。
// push: [error_judge] を指定すると、リモートの checker・interactor・runTwice が消える。
//   [checker] などを指定すると、リモートの errorJudge が消える。
// push: どれも指定しなければ、リモートの errorJudge は残る。
// pull: errorJudge のある下書きから [error_judge] ができ、コードのファイルは作られない。marginPercent があれば margin_percent も出る。
```

- [ ] **Step 2: 失敗を確かめる**

Run: `nix develop -c go -C cli test ./...`
Expected: 新しいテストがFAIL

- [ ] **Step 3: 3つのGoファイルを直し、README、ガイド、WebのCLIページに`[error_judge]`を足す**

既存の重複の誤りの文言`choose one of checker, interactor or run_twice`を新しい文言に置き換え、それを検査しているテストを直す。

- [ ] **Step 4: 通ることを確かめる**

Run: `nix develop -c go -C cli test ./...`、`nix develop -c go -C cli vet ./...`、`nix develop -c gofmt -l cli`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add cli web/app/content/docs/cli.md
git commit -m "feat: CLIで誤差ジャッジの設定をpushとpullできるようにする"
```

### Task 7: ドキュメント

**Files:**
- Create: `docs/adr/0037-support-error-judge.md`、`docs/judge/error-judge.md`
- Modify: `docs/adr/README.md`、`docs/judge/README.md`、`docs/judge/deployment-checks.md`（ワーカーを戻すと誤差ジャッジの提出がJEになる注記を、2ステップ形式の注記の隣に足す）
- Modify: `web/app/content/docs/checkers.md`（「誤差ジャッジ {#error-judge}」の節）、`web/shared/docs.ts`（ページの説明）
- Test: `web/tests/guides.spec.ts`

- [ ] **Step 1: ADR 0037を書く**

[ADR 0036](../../adr/0036-support-run-twice-judge.md)の章立てに合わせ、設計書の「利用者と決めたこと」「やらないこと」、余裕を上乗せする理由、有理数で比べる理由、トークン数を先に数える理由を書く。

- [ ] **Step 2: `docs/judge/error-judge.md`を書く**

比べ方、数の形、条件、速さ（Task 1の実測値と、採点ホストでの実測は配置時に取ること）、保存とAPI、結果の印、配置を、実装した値で書く。

- [ ] **Step 3: `checkers.md`に節を足す**

短く、表と箇条書きで書く。載せるもの：

- 3つの種類と条件の表。
- 数として読む形と、読まない形（`+1.0`、`nan`、`inf`、400バイト超）の例の表。
- 余裕の意味と、次の目安の表。

  | 余裕 | 吸収できる期待出力の誤差 |
  | --- | --- |
  | 0.1%（初期値） | εの1,000分の1まで |
  | 1% | εの100分の1まで |
  | 0% | なし（期待出力が正確な場合だけ） |

- 注意の箇条書き：期待出力は十分な桁数で出す（εが$10^{-6}$なら小数第10位以上）。文字列は完全一致で比べる。空白と改行は区別しない。期待出力の数が数百万個になると比較に時間がかかる。

- [ ] **Step 4: 確かめてコミット**

Run: `nix develop -c bun run --cwd web build`、`nix develop -c bun run --cwd web test guides.spec.ts --output=<scratchpad>/pw`
Expected: PASS

```bash
git add docs web/app/content/docs/checkers.md web/shared/docs.ts web/tests
git commit -m "docs: 誤差ジャッジのADRと仕様と作問ガイドを加える"
```

### Task 8: 配置と実機検証

本番への配布を含む。**利用者の確認を得てから始める。** 2ステップ形式の配置（その計画のTask 9）がまだなら、同じ配布にまとめる。

- [ ] **Step 1: ワーカーを配布する**

[定型コマンド](../../judge/deployment-checks.md)に従い、`judge/rollout.py run`で配布する。完了条件は`state.json`の`status: passed`と`step: complete`。smokeのレポートから100万組の実測値を`hosts.<インスタンスID>.toleranceMillionPairsMs`で読み、`docs/judge/error-judge.md`と作問ガイド（`web/app/content/docs/checkers.md`）の両方に書く。5秒を超えていたら、その値を目安として書く。

- [ ] **Step 2: `main`へ入れてpushする**

API、Webの順に自動で配置される。

- [ ] **Step 3: 本番で確かめる**

未公開の問題（期待出力`0.333333333333`、種類`either`、ε $10^{-6}$）を作り、次を提出する。確認は`judge/smoke-api.py`を使い、処理中を成功として扱わない。

| 提出の出力 | 期待する結果 |
| --- | --- |
| `0.3333333` | AC |
| `0.333334` | AC（誤差 約6.7×10⁻⁷） |
| `0.33333` | WA |
| `3.33333333e-1` | AC |
| `+0.3333333` | WA |
| `0.3333333 0.3333333` | WA |

- [ ] **Step 4: 配置記録を書いてコミットする**

```bash
git add docs/judge
git commit -m "docs: 誤差ジャッジの配置記録を加える"
```
