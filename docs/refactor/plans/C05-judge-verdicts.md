# C05 ジャッジの失敗判定（OLE/MLE/TLE/RE）をsandbox.failure_verdictにまとめる 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** isolateのメタデータから提出の失敗（OLE→MLE→TLE→RE）を決める規則を、`judge/runner/sandbox.py`の`failure_verdict(metrics)`の1か所に置く。通常実行（`host.case_result`）、対話（`interactive.relay`）、集計コード（`host.judge`）の3経路は、この関数を呼ぶ。

**Architecture:** `interactive.failure`の本体を、そのまま`sandbox.judge_verdict`の直前に移す。名前は`failure_verdict`にする。`host.py`の2か所にある写し（if/elifの連鎖と三項演算子の連鎖）を、この関数の呼び出しに置き換え、`interactive.failure`は削除する。先に、3経路の入口で優先順位を固定する特性テストを足す。

**Tech Stack:** Python 3（標準ライブラリのunittest、DBなし）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C05.json`（worthの`revised_scope`が拘束力を持つ。safetyの`must_preserve`と`test_plan`も使う）

## 困りごと

- 同じ規則が3か所に書かれている。
  - `judge/runner/host.py:278-285`：`case_result`のif/elifの連鎖
  - `judge/runner/interactive.py:16-27`：`failure(metrics)`。呼び出し元は`:41`の`relay`だけ
  - `judge/runner/host.py:520-521`：集計コードの判定。8d6ea6bで`interactive.failure`を三項演算子の連鎖に書き写したもの
- 実行経路を足すたびに写しが増えている。253ffe9（対話）で2つ目、8d6ea6b（集計コード）で3つ目が入った。優先順位や条件を変えると、1つの経路だけ古い規則が残るおそれがある。
- 集計コードの経路で`interactive.failure`を呼ぶと、依存の向きが不自然になる（hostがinteractiveのヘルパーを借りる）。そのため、規則はメタデータを解釈するsandboxに置く。

## 判定の記録（反証の差と、計画で決めた点）

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| (b) 提出コード長・メモリ・時間の上限検証の共通化 | revise：本番コードは変えない | keep：`SOURCE_LIMIT`などにまとめる | **採らない**。worthの`revised_scope`に従う。sandboxの範囲検証は、チェッカー・集計コードを5000ms・512MiBで動かすときにもかかる。提出の範囲と同じ関数にすると、提出の上限だけを下げたときに全問題がJEになる結合ができる。safetyが挙げた`collect_artifact`の`131072`（`sandbox.py:242`）の結合は、範囲外の報告事項に残す |
| timeLimitMsの不正値を拒否するテスト | 任意、別コミット | 追加する | C05では足さない。このリファクタリングが触る挙動ではないため。テストの欠落として報告する |
| 特性テストを置く場所 | 3経路 | 移動する関数ではなく、前後で変わらない入口 | 入口に置く。`host.case_result`、`Judge`ハーネス経由の`host.judge`、`interactive.relay`（`run_pair`）である。`sandbox.failure_verdict`を直接呼ぶテストは書かない |
| 集計コードの特性テストの形 | 新しいテスト | 既存の表に行を足す | `test_failures_are_je_with_a_kept_diagnostic`の表に行を足す。理由とメッセージの検査がすでにそろっているため。既存の行は変えない |
| 残った参照の確認に使うgrep | `def failure` | — | `def failure(metrics`にする。`def failure`だと`telemetry.failure`に当たる |
| `host.py:61`の使われない代入 | 外す | 1行だけなら可 | 外さない。C05の目的と関係がないので範囲外とする（「範囲外（報告事項を含む）」の節を参照） |
| `graphify update .` | — | — | 実行する。worktreeに`graphify-out/graph.json`があり、`.gitignore`の対象なのでtreeには入らない（C06と同じ扱い） |
| タスクの分割 | — | — | 1つにする。特性テストだけを採っても価値は小さい |

## Global Constraints

- この計画の行番号は、すべて変更前（d3adb76の作業ツリー）のものである。
- 公開HTTP API、CLI、DBスキーマ、依存のバージョン、インフラ・CIを変えない。判定結果、時間・メモリの計測、sandboxの隔離も変えない。
- テストは追加だけにする。既存のテストとアサーションは削除も変更もスキップもしない。
- `failure_verdict(metrics)`の本体は、`interactive.py:17-27`と同じにする。
  - 判定の順序：`metrics is None`ならNone、`overflow`ならOLE、`oom`ならMLE、`status == 'TO'`ならTLE、`status`・`exitCode`・`signal`のどれかが真ならRE、それ以外はNone。
  - 参照は`metrics['key']`のまま（`.get()`にしない）。短絡評価の順序も同じにする。
  - 引数やフラグを足さない。
- `interactive.relay`の`:41`は`'OLE' if totals[i] > sandbox.OUTPUT_LIMIT else sandbox.failure_verdict(m)`にする。合計バイト数の判定が先に来る順序を保つ。`:42-57`（pointsでの`judge_verdict`による上書き、MLE・OLE・wallTimeoutなしのTLEをJEにする処理、`errors[0]`を先に返す順序、経過時間超過の診断文）は1文字も変えない。
- `host.case_result`：
  - `:264-277`の検証（index、statusの許可リスト、フラグのbool型、exitCode・signalの0〜255、`number()`、base64、出力上限）は変えない。判定より前に置く。
  - 空白区切りのトークン比較は、`failure_verdict`がNoneのときだけ評価する。条件は今と同じ`not generate and not validate`である。
  - 失敗でなく、トークン比較が一致しなければWA、それ以外はACにする。
  - `:290-300`（itemのキーと既定名「ケース{n}」、generateでACのときのUTF-8・NULによるREの再判定）は変えない。
- 集計コード（`host.py:519-531`）：
  - ローカル変数`failure`に`sandbox.failure_verdict(reply)`を入れる。
  - 診断文`f"集計コードの実行失敗: {failure}\n" + stderr + '\n'`、`keep=True`、`checker_error('aggregator_execution_failed')`、失敗でなければ`aggregator_scores`へ進む流れは変えない。
- `sandbox.judge_verdict`（判定用コードの資源超過はJE）とは統合しない。名前と意味も変えない。`sandbox.execute`、`metadata`、`run_args`、`host.validate_job`、上限のリテラルにも触れない。
- `interactive.failure`は削除する。互換用の別名は残さない。新しいimportは足さない（hostもinteractiveもすでに`import sandbox`している）。sandboxからhost・interactiveをimportしない。
- コードのコメントは英語にする。周りと同じく、1行程度にする。計画とレポートは日本語で書く。
- `git commit`、`git stash`、`git reset`は実行しない。コミットはオーケストレーターが行う。
- ステージ済みの`api/`の3ファイル（`api/internal/contests/store.go`、`api/internal/httpapi/contest_problem_statement_test.go`、`api/internal/problems/publication.go`）には触れない。

## Review Focus

1. `case_result`でWAの判定が失敗の判定より先に来る、またはトークン比較を先に計算する形にする。そうすると、失敗したうえに出力も違うケースがWAになる。→ 新しい表の行は、どれも期待出力と異なる出力を返す。そのため、失敗の行がWAになれば検出できる。
2. generateで失敗したケース（例：TLE、出力がNUL）が、UTF-8・NULの再判定でREに変わる、または`output`キーが付く。→ 新しいテストのgenerateの行で検出する。
3. relayで、実行中の相手（metricsがNone）を失敗と扱う。または、合計バイト数によるOLEの判定より後に`failure_verdict`を置く。→ `run_pair`の新しい行では、相手側が実行中のままである。既存の`test_each_direction_and_stderr_resource_failure`も使う。
4. 複数のフラグが同時に立つときの優先順位（overflow+oom+TO→OLE、oom+TO→MLE、TO+exitCode→TLE）。status=''でexitCodeだけ、またはsignalだけが立つときはREになる。→ 3経路それぞれの新しい行で検出する。
5. `interactive.failure`への参照が残る。または、テストが`sandbox`の属性を差し替えたせいで、`failure_verdict`が使えなくなる。→ relayのテスト群（参照が残れば`NameError`になる）、grep、全件の実行で検出する。

---

### Task 1: 失敗判定をsandbox.failure_verdictにまとめ、3経路の優先順位を特性テストで固定する

**Files:**
- Test: `judge/tests/test_runner.py`（`RunnerTests.test_signal_137_is_not_automatically_mle`の直後、現在の197行の後）
- Test: `judge/tests/test_scoring.py`（`AggregatorTests.test_failures_are_je_with_a_kept_diagnostic`の表。`'memory'`の行（現在の377-378行）の直後）
- Test: `judge/tests/test_interactive.py`（`InteractiveTests.test_each_direction_and_stderr_resource_failure`の直後、現在の123行の後）
- Modify: `judge/runner/sandbox.py`（`judge_verdict`の直前、現在の227行の前に追加）
- Modify: `judge/runner/interactive.py:16-27`（削除）、`:41`
- Modify: `judge/runner/host.py:278-289`、`:520-521`

**Interfaces:**
- Produces: `sandbox.failure_verdict(metrics: dict | None) -> str | None`。戻り値は`'OLE'`、`'MLE'`、`'TLE'`、`'RE'`、`None`のどれか。

- [ ] **Step 1: 特性テストを追加する（期待値はリテラルで書く。すでに実在のコードで成り立つことを確認済み）**

1. `test_runner.py`に`test_case_failure_flags_follow_ole_mle_tle_re_priority`を足す。
   - `reply = dict(index=0, status=..., oom=..., overflow=..., exitCode=..., signal=..., cpuTimeMs=1, wallTimeMs=2, memoryBytes=4096, output='')`、`case = {'output': 'expected'}`とし、`host.case_result(reply, 0, case)['verdict']`を`subTest`で表と比べる。

     | (overflow, oom, status, exitCode, signal) | 期待値 |
     | --- | --- |
     | (True, True, 'TO', 1, 9) | OLE |
     | (True, False, '', 0, 0) | OLE |
     | (False, True, 'TO', 1, 9) | MLE |
     | (False, True, '', 0, 0) | MLE |
     | (False, False, 'TO', 1, 9) | TLE |
     | (False, False, 'TO', 0, 0) | TLE |
     | (False, False, 'SG', 0, 9) | RE |
     | (False, False, 'RE', 1, 0) | RE |
     | (False, False, '', 1, 0) | RE |
     | (False, False, '', 0, 9) | RE |
     | (False, False, '', 0, 0) | WA |

   - 失敗のない行で`case = {'output': ' \n'}`にすると、`'AC'`になる。
   - generateの行：`status='TO'`、`output=base64.b64encode(b'\0').decode()`、`case={'output': ''}`、`generate=True`とする。結果のitemは`{'name': 'ケース1', 'verdict': 'TLE', 'cpuTimeMs': 1, 'wallTimeMs': 2, 'memoryBytes': 4096}`と等しい（REにならず、`output`キーもない）。
2. `test_scoring.py`の表に4行を足す。既存の`checker(b'')`と`finished(...)`を使う。ループの検査はそのまま使われる（JE、0/4、キー集合、理由、メッセージ、16384バイト以下）。
   - `('overflow', … aggregator=lambda _: finished(overflow=True, oom=True, status='TO', checkerLog='big'))` → `'aggregator_execution_failed'`、`'集計コードの実行失敗: OLE\nbig'`
   - `('memory-timeout', … finished(oom=True, status='TO', checkerLog=''))` → 同じ理由、`'集計コードの実行失敗: MLE'`
   - `('timeout-exit', … finished(status='TO', exitCode=1, checkerLog='late'))` → 同じ理由、`'集計コードの実行失敗: TLE\nlate'`
   - `('exit', … finished(exitCode=1, checkerLog='exit'))` → 同じ理由、`'集計コードの実行失敗: RE\nexit'`
3. `test_interactive.py`に`test_resource_flags_of_each_side_follow_relay_rules`を足す。
   - 各行で、`codes = ['import time; time.sleep(10)'] * 2; codes[i] = 'pass'`とし、`result, log = self.run_pair(*codes, statuses={i: flags})`を`subTest`で比べる。

     | i | flags | 期待値 |
     | --- | --- | --- |
     | 0 | `{'oom': True}` | MLE |
     | 0 | `{'status': 'TO'}` | TLE |
     | 0 | `{'status': 'TO', 'oom': True}` | MLE |
     | 0 | `{'overflow': True, 'oom': True}` | OLE |
     | 0 | `{'exitCode': 1}` | RE |
     | 1 | `{'oom': True}` | JE |
     | 1 | `{'status': 'TO'}` | JE |
     | 1 | `{'status': 'TO', 'wallTimeout': True}` | TLE |

   - 最後の行では、さらに`self.assertIn('経過時間超過', log)`も確かめる。

- [ ] **Step 2: 変更前に新しいテストを実行し、成功を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/judge/tests && python3 -B -m unittest -v test_runner.RunnerTests.test_case_failure_flags_follow_ole_mle_tle_re_priority test_scoring.AggregatorTests.test_failures_are_je_with_a_kept_diagnostic test_interactive.InteractiveTests.test_resource_flags_of_each_side_follow_relay_rules`
Expected: `Ran 3 tests`、`OK`。失敗したら、コードは変えずに期待値と実際の値を報告して止まる（計画時に、表の全行が今のコードで成り立つことを確認済み）。

- [ ] **Step 3: `failure_verdict`を追加し、3か所の呼び出しを置き換える**

- `sandbox.py`：`judge_verdict`の直前に`def failure_verdict(metrics):`を置く。本体は`interactive.py:17-27`をそのまま移す。docstringは英語の1行にする。内容は「提出のメタデータからOLE・MLE・TLE・REを返す。正常終了とメタデータがまだないときはNone」とし、判定用コードは`judge_verdict`が扱うことが分かるようにする。
- `interactive.py`：`failure`を削除する（`BUFFER_LIMIT`と`relay`の間の空行は2行に保つ）。`:41`をGlobal Constraintsの形にする。
- `host.py:278-289`：`verdict = sandbox.failure_verdict(reply)`とし、Noneのときだけ従来の条件でWA・ACを決める。たとえば`sandbox.failure_verdict(reply) or ('WA' if <今の:286の条件> else 'AC')`の形でよい。`:286`の条件式は変えずに残す。
- `host.py:520-521`：`failure = sandbox.failure_verdict(reply)`の1行にする。

- [ ] **Step 4: 全体を検証する**

1. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && python3 -B -m unittest discover -s judge/tests -v`
   Expected: `Ran 144 tests`、`OK`（開始時点の142件に、新しいテストメソッド2件を足した数）
2. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && grep -rn "interactive\.failure\|def failure(metrics" judge/`
   Expected: 出力なし（終了コード1）
3. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && grep -rn "failure_verdict" judge/ --include=*.py`
   Expected: 4行。`sandbox.py`の定義、`interactive.py`のrelay、`host.py`の`case_result`、`host.py`の集計コード
4. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && grep -n "'OLE'" judge/runner/host.py judge/runner/interactive.py judge/runner/sandbox.py`
   Expected: `host.py`は生成の合計サイズ超過（`item['verdict'] = 'OLE'`）の1行だけ。`interactive.py`はrelayの`:41`相当と`('MLE', 'OLE')`の行だけ。`sandbox.py`は`failure_verdict`の`return 'OLE'`だけ
5. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short`
   Expected: 新しく変わったのは`judge/runner/sandbox.py`、`judge/runner/host.py`、`judge/runner/interactive.py`、`judge/tests/test_runner.py`、`judge/tests/test_scoring.py`、`judge/tests/test_interactive.py`の6ファイルだけ。ステージ済みの`api/`の3ファイルは開始時のまま
6. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`
   Expected: 終了コード0

コミットメッセージの案（オーケストレーター用）：`refactor: ジャッジの失敗判定（OLE/MLE/TLE/RE）をsandbox.failure_verdictの1か所にまとめ、3経路の優先順位を特性テストで固定する`

isolateの実機での確認（`judge/runner/smoke.py`、`judge/runner/interactive_smoke.py`）は、この委任範囲では行わない。未実行としてレポートに記録する。

## 完了条件

- 新しい特性テスト（2メソッドと、表に足した4行）が、変更前（Step 2）と変更後（Step 4）の両方で成功している。
- `python3 -B -m unittest discover -s judge/tests -v`が144件すべて成功する。
- 失敗の規則がコードにあるのは`sandbox.failure_verdict`の1か所だけである。`interactive.failure`と、`host.py`にあった2つの写しは残っていない。
- `relay`の`:42-57`、`case_result`の`:264-277`と`:290-300`、集計コードの診断文と理由コード、`sandbox.judge_verdict`に差分がない。

## 範囲外（報告事項を含む）

- (b) 上限検証の共通化（`SOURCE_LIMIT`、`valid_source`などの関数）。判定の記録を参照。
- `sandbox.py:242`の`regular_read(..., 131072)`：Python・Rubyの実行物の読み取り上限で、`host.py:77`・`sandbox.py:80`と同時に変える必要がある。片方だけ上限を上げると、ソースが黙って切り詰められ、JEではなく誤ったRE・WAになる。これを捕まえるテストもない。別の候補として報告する。
- テストの欠落：timeLimitMsの不正値（99、5001、150、True、1000.0、None）を、hostとsandboxで拒否することのテスト。sandboxのコンパイル段階で131072バイトを受け付けることのテスト。
- `host.py:61`の使われない`source = code.get('source')`と、`validate_job`の判定用コードのruntime検査の重複（`host.py:56-60`と`validate_judge_code`）。
- overflowの算出（stderrが65536を超える、または`signal == 25`）の重複（`sandbox.py:124`と`interactive.py:142`）。対話側はストリームを途中で観測するため、条件が同じではない。
- `sandbox.py:97`の`write_text(source)`がencodingを指定していない。長さの検証はUTF-8のバイト数で行っている（PEP 538/540に頼っている）。
- `sandbox.judge_verdict`、`relay`のJE・TLEへの振り分け、コンパイル時のsuccess判定（`sandbox.py:126`）。
