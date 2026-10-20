# B3 ジャッジの保留事項（D34、D33、D12、D11）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task（実装者は1人。各タスクの最後に自分でコミットする）. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `docs/refactor/FIXES.md`のD34、D33（ジャッジ側）、D12、D11の「変更内容」を、1件1コミットで実装する。

**Architecture:** 先に既存の検証をテストで固定し（D34）、提出コードの長さの上限を`sandbox.SOURCE_LIMIT`にまとめる（D33）。次に、提出のヒープ上限を`sandbox.scaled_heap`でメモリ制限に比例させ、対話ジャッジ側は`interactor`引数で判別する（D12）。最後に、rolloutの`finish()`から既存の`api_smoke()`を呼ぶ（D11）。

**Tech Stack:** Python 3（標準ライブラリのunittest、DBなし、isolateなし）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/docs/refactor/FIXES.md`（「守ること」と、D11・D12・D33・D34の「変更内容」）。根拠：`.local/fixes/briefs/B3.md`。行番号は、すべて変更前（`b7bd62a`）のもの。

## Global Constraints

- 各項目の「変更内容」だけを変える。公開API、DBスキーマ、依存のバージョン、インフラは変えない。
- 判定結果・隔離・時間計測の意味は、D12で明記した提出側のヒープ上限を除いて保つ。cgroupのメモリ上限（`--cg-mem`）、CPU・経過時間、プロセス数、ファイル数は変えない。
- 1件1コミット。メッセージは`prefix: 対応内容`（日本語）。末尾に`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`を付ける。
- テストを先に書く。新しい挙動を固定するテストは、変更前のコードで失敗することを確かめる。保つ挙動のテスト（D34、コンパイル時の値）は、最初から成功してよい。
- テストの削除・スキップ・検証の緩和はしない。この計画で、古い挙動を固定していて書き換えが要る既存テストはない（計画時に確認した）。
- 範囲外の問題は直さずに報告する。
- コードのコメントは英語で、周りと同じく1行程度にする。文書と報告は日本語で書く。
- push、マージ、配布、AWSの操作はしない。`git stash`、`git reset`、`git rebase`、`git checkout`、`git switch`は使わない。isolateの実機テスト（`judge/smoke.py`、`judge/interactive_smoke.py`）は実行しない。

## 判定の記録（Ruling）

- **D12の丸め**：Ruling: `scaled_heap(at_512, memory) = at_512 * memory // 512`（MiB単位で切り捨て）にする。各フラグは整数のMiBしか取らない。512では今の値と完全に一致する。
- **D12の明示の引数**：Ruling: `run_args`と`java_heap`に`interactor=False`を足す。既存の`interactive`は対話の両側で真になるので、対話ジャッジ側の判別には使えない。対話ジャッジ側の値は今のまま固定する（C# 128 MiB、Go 160MiB、TruffleRuby 96m、Node・Deno 128、Java 128m）。
- **D12のコンパイル時**：Ruling: コンパイル時の値は変えない（C# 512 MiB、Go 384MiB、TruffleRuby・Node・Bun・Deno 640）。Goのコンパイル時は、今は`memory == 256`が偽なので`384MiB`になっている。比例の式に通すと768MiBになるので、`compile_phase`で明示的に384を返す。
- **D12のJava**：Ruling: `runtimes.py`の`-Xmx256m`（512 MiBのときの値）は残し、実行の直前に`sandbox.java_heap`で置き換える。メモリ制限を知っているのは、コマンドを組み立てる呼び出し元（`sandbox.execute`と`interactive.execute`）だけのため。判定コード（チェッカー・集計コード）は常に512 MiBで動くので、`-Xmx256m`のまま変わらない。
- **D12の文書**：Ruling: `docs/judge/runtime-policy.md:73`の「Javaはヒープ256 MiB」は、512 MiB以外の提出では誤りになる。この1行だけを新しい挙動に合わせる。他の文書は変えない（`runtime-additions.md:58`と`interactive-judge.md:138`は、変更後も正しい）。
- **D33の定数の置き場所**：Ruling: `sandbox.SOURCE_LIMIT = 128 * 1024`とし、`host`は`sandbox.SOURCE_LIMIT`を参照する。`host`はすでに`sandbox`をimportしている。逆向きにすると循環importになる。
- **D11の直し方**：Ruling: `finish()`の本番APIテストの部分（`rollout.py:411-420`）を、`api_smoke(config, directory, config['nodes'])`の呼び出しに置き換える。この部分は`api_smoke()`（`:626-638`）の本体と、`--instance`の条件以外は同じ写しである。D11は、6b938aeが片方の写しだけを直したことで起きた。`rolling()`（`:721`）はすでに同じ呼び出しを使っている。2台以上のときの引数、カタログの確認、レポートのパスは変わらない。
- **D34の範囲**：Ruling: 変更内容が`judge/host.py`と指定しているので、`host.validate_job`だけを確かめる。`sandbox.execute`の`invalid limits`は対象外にする。

## Review Focus

1. メモリ制限が512 MiBのときは、全処理系で今と同じフラグになる。判定コード（常に512 MiB）も同じ。→ Task 3の表の512の行と、既存の`test_checker.CheckerTests.test_sandbox_uses_fixed_file_arguments_for_every_runtime`（Javaのコマンド全体を固定）。
2. コンパイル時（memory=1024）に値が変わらない。特にGoは`768MiB`になりやすい。→ Task 3の`test_compile_heap_caps_stay_fixed`。
3. 対話問題で提出の`memoryLimitMb`が256のとき、提出側は比例した値になり、対話ジャッジ側は固定値のまま。→ Task 3の`test_interactor_heap_caps_stay_fixed_while_the_submission_scales`の256の行。
4. 512で割り切れないメモリ（315）でも、フラグは整数になる（`157.5m`などにならない）。→ Task 3の315の行（Java `-Xmx157m`、C# `0x7600000`）。
5. 上限ちょうどのソースを、インタープリター系（`artifact == 'source'`）の成果物として切り詰めずに読み出す。→ Task 2の`limit=200000`の行。

---

### Task 1: D34 不正なtimeLimitMsを拒否するテストを足す

**Files:**
- Test: `judge/tests/test_runner.py`（`RunnerTests.test_memory_limits_reach_isolate_and_reject_invalid_jobs`の直後、現在の49行の後）

- [ ] **Step 1: テストを書く**

```python
def test_invalid_time_limits_are_rejected(self):
    job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
               memoryLimitMb=512, cases=[dict(input='', output='')])
    for ms in (None, True, '1000', 1000.0, 0, 99, 5001, 5100, 150, 1050):
        with self.subTest(invalid=ms):
            job['timeLimitMs'] = ms
            with self.assertRaisesRegex(ValueError, 'time limit'):
                host.validate_job(job, 'sha256:test')
    for ms in (100, 1000, 5000):
        with self.subTest(valid=ms):
            job['timeLimitMs'] = ms
            host.validate_job(job, 'sha256:test')
```

各値が捕まえる退行：`1000.0`は型の検査だけが拒否する。`0`と`5100`は範囲の検査だけ、`150`と`1050`は100ms単位の検査だけが拒否する。`None`と`'1000'`は、型の検査がなければ`TypeError`になり、`ValueError`の期待に合わない。

- [ ] **Step 2: 実行して成功を確かめる**（既存の挙動なので、変更前から成功する）

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/judge/tests && python3 -B -m unittest -v test_runner.RunnerTests.test_invalid_time_limits_are_rejected`
Expected: `Ran 1 test`、`OK`（計画時に全値を確認済み）

- [ ] **Step 3: コミットする**

`git add judge/tests/test_runner.py`、メッセージ：`test: ジャッジが不正なtimeLimitMsを拒否することをテストで固定する`

### Task 2: D33 提出コードの長さの上限をsandbox.SOURCE_LIMITにまとめる

**Files:**
- Modify: `judge/sandbox.py:20-23`（定数の追加）、`:80`、`:257`
- Modify: `judge/host.py:77`
- Test: `judge/tests/test_runner.py`（`RunnerTests.test_source_size_limit`の直後、現在の448行の後）

**Interfaces:**
- Produces: `sandbox.SOURCE_LIMIT = 128 * 1024`（int、131072）

- [ ] **Step 1: 失敗するテストを書く**

```python
def test_one_source_limit_covers_validation_and_reading_back_the_source(self):
    import tempfile
    self.assertEqual(sandbox.SOURCE_LIMIT, 131072)
    runtime = RUNTIMES['python314-isolate']
    for limit in (8, 200000):
        with self.subTest(limit=limit), patch.object(sandbox, 'SOURCE_LIMIT', limit), \
                tempfile.TemporaryDirectory() as tmp:
            job = dict(runtime='python314-isolate', runtimeDigest='sha256:test', source='#' * limit,
                       memoryLimitMb=512, timeLimitMs=1000, cases=[dict(input='', output='')])
            host.validate_job(job, 'sha256:test')
            (Path(tmp) / 'main.py').write_text(job['source'])
            self.assertEqual(sandbox.collect_artifact(Path(tmp), runtime), job['source'].encode())
            job['source'] += '#'
            with self.assertRaisesRegex(ValueError, 'source'):
                host.validate_job(job, 'sha256:test')
            with patch.object(sandbox, 'invoke') as invoke, self.assertRaisesRegex(ValueError, 'invalid source'):
                sandbox.execute(job, compile_phase=True)
            invoke.assert_not_called()
```

変更前に失敗する理由：`sandbox.SOURCE_LIMIT`がないので、最初の`assertEqual`が`AttributeError`になる。仮に定数だけがあっても、3か所がリテラルのままなら次の検査で失敗する。`limit=8`では、9バイトのソースをhostとsandboxが受け付ける。`limit=200000`では、`collect_artifact`が131073バイトで切り詰める。

- [ ] **Step 2: 実行して失敗を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/judge/tests && python3 -B -m unittest -v test_runner.RunnerTests.test_one_source_limit_covers_validation_and_reading_back_the_source`
Expected: ERROR（`AttributeError: ... 'SOURCE_LIMIT'`）

- [ ] **Step 3: 定数にまとめる**

- `sandbox.py`：`OUTPUT_LIMIT`の近くに`SOURCE_LIMIT = 128 * 1024`を置く。英語1行のコメントで、検証（`host.validate_job`、コンパイル段階）と、インタープリター系の成果物の読み出しが同じ値を使うことを書く。
- `sandbox.py:80`の`<= 131072`を`<= SOURCE_LIMIT`に、`:257`の`regular_read(box / runtime['source'], 131072)`を`regular_read(box / runtime['source'], SOURCE_LIMIT)`にする。
- `host.py:77`の`<= 131072`を`<= sandbox.SOURCE_LIMIT`にする。条件式の他の部分（型、`0 <`、NULの検査、エラーメッセージ）は変えない。

保つ挙動：131072バイトを受け付け、131073バイト（`'あ' * 43691`）をhostとsandboxの両方で拒否する。→ 既存の`test_source_size_limit`。

- [ ] **Step 4: 実行して成功を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/judge/tests && python3 -B -m unittest -v test_runner.RunnerTests.test_one_source_limit_covers_validation_and_reading_back_the_source test_runner.RunnerTests.test_source_size_limit`
Expected: `Ran 2 tests`、`OK`

- [ ] **Step 5: コミットする**

`git add judge/sandbox.py judge/host.py judge/tests/test_runner.py`、メッセージ：`refactor: ジャッジの提出コードの長さの上限をsandbox.SOURCE_LIMITの1か所にまとめる`

### Task 3: D12 提出のヒープ上限をメモリ制限に比例させ、対話ジャッジ側を明示の引数で判別する

**Files:**
- Modify: `judge/sandbox.py:151-163`（`run_args`）、`run_args`の直後（`scaled_heap`と`java_heap`の追加）、`:112`の直後（`execute`の実行段階）
- Modify: `judge/interactive.py:106-109`
- Modify: `docs/judge/runtime-policy.md:73`
- Test: `judge/tests/test_runner.py`（Task 1のテストの直後）
- Test: `judge/tests/test_interactive.py`（`InteractiveTests.test_submission_memory_limit_is_separate_from_interactor_limit`の直後、現在の39行の後）

**Interfaces:**
- Produces: `sandbox.scaled_heap(at_512: int, memory: int) -> int`。値は`at_512 * memory // 512`。
- Produces: `sandbox.run_args(runtime, cpu, wall, memory, *, compile_phase=False, meta=None, interactive=False, interactor=False)`
- Produces: `sandbox.java_heap(command: list[str], memory: int, *, interactor: bool = False) -> list[str]`。`'-Xmx256m'`の要素を、対話ジャッジ側なら`'-Xmx128m'`に、それ以外なら`f'-Xmx{scaled_heap(256, memory)}m'`に置き換えたリストを返す。

値の規則（MiB。提出側の列は512 MiBのときの値で、これを`scaled_heap`で比例させる）：

| フラグ | コンパイル時 | 対話ジャッジ側 | 提出側（512 MiBのとき） |
| --- | --- | --- | --- |
| C# `DOTNET_GCHeapHardLimit`（バイト数の16進） | 512 | 128 | 192 |
| Go `GOMEMLIMIT=<n>MiB` | 384 | 160 | 384 |
| TruffleRuby `RUBYOPT=--vm.Xmx<n>m` | 640 | 96 | 256 |
| Node `NODE_OPTIONS`（Bunのコンパイル時も） | 640 | 128 | 320 |
| Deno `DENO_V8_FLAGS` | 640 | 128 | 320 |
| Java `-Xmx<n>m`（実行コマンド） | 対象外（javacの`-J-Xmx512m`のまま） | 128 | 256 |

- [ ] **Step 1: 失敗するテストを書く**

1. `test_runner.py`に`test_submission_heap_caps_scale_with_the_memory_limit`を足す。`flags(args, prefix)`は、`prefix`で始まる要素のリストを返すローカル関数にする。

   | memory | C# | Go | TruffleRuby | Node・Deno | Java |
   | --- | --- | --- | --- | --- | --- |
   | 64 | `0x1800000` | `48MiB` | `--vm.Xmx32m` | `40` | `-Xmx32m` |
   | 256 | `0x6000000` | `192MiB` | `--vm.Xmx128m` | `160` | `-Xmx128m` |
   | 315 | `0x7600000` | `236MiB` | `--vm.Xmx157m` | `196` | `-Xmx157m` |
   | 512 | `0xc000000` | `384MiB` | `--vm.Xmx256m` | `320` | `-Xmx256m` |

   各`memory`の`subTest`で、次を確かめる。`args(name)`は`sandbox.run_args(name, 1, 4, memory)`の意味。
   - `flags(args('csharp14-isolate'), '--env=DOTNET_GCHeapHardLimit=') == ['--env=DOTNET_GCHeapHardLimit=' + C#]`
   - `flags(args('go127-isolate'), '--env=GOMEMLIMIT=') == ['--env=GOMEMLIMIT=' + Go]`
   - `flags(args('ruby-truffle40-isolate'), '--env=RUBYOPT=') == ['--env=RUBYOPT=' + TruffleRuby]`
   - `javascript-node24-isolate`と`typescript-node24-isolate`：`flags(..., '--env=NODE_OPTIONS=') == [f'--env=NODE_OPTIONS=--max-old-space-size={v8}']`
   - `javascript-deno29-isolate`と`typescript-deno29-isolate`：`flags(..., '--env=DENO_V8_FLAGS=') == [f'--env=DENO_V8_FLAGS=--max-old-space-size={v8}']`
   - `flags(args('typescript-bun14-isolate'), '--env=NODE_OPTIONS=') == []`（実行時のBunには付けない。今と同じ）
   - Java：`java24-isolate`と`java25-isolate`それぞれで、`sandbox.execute(dict(runtime=name, input='', timeLimitMs=1000, memoryLimitMb=memory), artifact=artifact)`を実行する。スタブは`test_memory_limits_reach_isolate_and_reject_invalid_jobs`と同じ形にする（`invoke`は`--run`のときにmeta・stdout・stderrを書き、`args[args.index('--') + 1:]`を記録する。`META`を差し替える）。`prepare_program`が`box/main`を読み取り専用にするので、runtimeとmemoryの組ごとに新しい`TemporaryDirectory`を使う。`flags(command, '-Xm') == ['-Xms16m', Java]`を確かめる。

   変更前に失敗する理由：64では512の値（`0xc000000`など）、256では対話ジャッジ側の値（`0x8000000`、`160MiB`など）が出る。Javaは常に`-Xmx256m`になる。
2. `test_runner.py`に`test_compile_heap_caps_stay_fixed`を足す。`sandbox.run_args(name, 30, 40, 1024, compile_phase=True)`が次を含むこと（`assertIn`）を確かめる。この時点で成功する。
   - `csharp14-isolate`：`--env=DOTNET_GCHeapHardLimit=0x20000000`
   - `go127-isolate`：`--env=GOMEMLIMIT=384MiB`
   - `ruby-truffle40-isolate`：`--env=RUBYOPT=--vm.Xmx640m`
   - `javascript-node24-isolate`、`typescript-node24-isolate`、`typescript-bun14-isolate`：`--env=NODE_OPTIONS=--max-old-space-size=640`
   - `javascript-deno29-isolate`、`typescript-deno29-isolate`：`--env=DENO_V8_FLAGS=--max-old-space-size=640`
3. `test_interactive.py`に`test_interactor_heap_caps_stay_fixed_while_the_submission_scales`を足す。
   - 仕組みは直前の`test_submission_memory_limit_is_separate_from_interactor_limit`と同じにする（`invoke`、`META`、`Popen`、`relay`の差し替え）。違うのは2点だけ。`prepare_program`を`side_effect=lambda box, runtime, *args, **kwargs: list(runtime['run'])`で差し替える。`request = job()`から`checker`を外し、`runtime=name`、`memoryLimitMb=memory`、`interactor=dict(runtime=name.removesuffix('-isolate'), source='x')`にする。
   - 処理系とmemory（64、256、512）の組ごとに`subTest`を作る。`commands`の各要素から`prefix`で始まるものを取り出し、`[[提出側], [対話ジャッジ側]]`と比べる。

     | name | prefix | 64 | 256 | 512 | 対話ジャッジ側 |
     | --- | --- | --- | --- | --- | --- |
     | `java24-isolate` | `-Xmx` | `32m` | `128m` | `256m` | `128m` |
     | `csharp14-isolate` | `--env=DOTNET_GCHeapHardLimit=` | `0x1800000` | `0x6000000` | `0xc000000` | `0x8000000` |
     | `go127-isolate` | `--env=GOMEMLIMIT=` | `48MiB` | `192MiB` | `384MiB` | `160MiB` |
     | `ruby-truffle40-isolate` | `--env=RUBYOPT=` | `--vm.Xmx32m` | `--vm.Xmx128m` | `--vm.Xmx256m` | `--vm.Xmx96m` |
     | `javascript-node24-isolate` | `--env=NODE_OPTIONS=` | `--max-old-space-size=40` | `…=160` | `…=320` | `…=128` |
     | `typescript-deno29-isolate` | `--env=DENO_V8_FLAGS=` | `--max-old-space-size=40` | `…=160` | `…=320` | `…=128` |

     （期待する要素は、prefixに表の値をつないだ文字列。`…`は`--max-old-space-size`の略）
   - 変更前に失敗する理由：64と256の提出側が誤った値になる。対話ジャッジ側と512の列は、変更前から成り立つ（計画時に、この差し替えの仕組みで確認済み）。

- [ ] **Step 2: 実行して失敗を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/judge/tests && python3 -B -m unittest -v test_runner.RunnerTests.test_submission_heap_caps_scale_with_the_memory_limit test_runner.RunnerTests.test_compile_heap_caps_stay_fixed test_interactive.InteractiveTests.test_interactor_heap_caps_stay_fixed_while_the_submission_scales`
Expected: `test_compile_heap_caps_stay_fixed`だけが成功し、他の2つはFAIL（64・256・315のsubTest）

- [ ] **Step 3: 実装する**

- `sandbox.py`：`run_args`の直後に`scaled_heap`と`java_heap`を置く（Interfacesのとおり。docstringは英語1行で、512 MiBのときの値を基準に比例させることを書く）。
- `run_args`に`interactor=False`を足す。5つの分岐で、`memory == 256`を`interactor`に、512のときの値を`scaled_heap(<表の値>, memory)`に置き換える。三項演算子の形（`<compile> if compile_phase else <interactor> if interactor else <submission>`）はそろえる。Goは`'GOMEMLIMIT=' + str(384 if compile_phase else 160 if interactor else scaled_heap(384, memory)) + 'MiB'`にする。
- `sandbox.execute`の実行段階で、`command = prepare_program(...)`（`:112`）の直後に`if runtime['artifact'] == 'java': command = java_heap(command, memory)`を足す。コンパイル段階には足さない。
- `interactive.py:106-107`を`if RUNTIMES[name]['artifact'] == 'java': command = sandbox.java_heap(command, job['memoryLimitMb'], interactor=i == 1)`にする。`:108-109`の`run_args`に`interactor=i == 1`を足す。対話ジャッジ側の`--cg-mem`に渡す256は変えない。
- `docs/judge/runtime-policy.md:73`を次の1行にする：`Javaはヒープをメモリ上限の半分（512 MiBで256 MiB）、メタスペース96 MiB、コードキャッシュ32 MiB、直接メモリ32 MiBに制限し、Serial GCを使う。`（対話ジャッジ側の128 MiBも、256 MiBの半分なので、この記述に合う）

保つ挙動と、それを守るテスト：
- 判定コードを512 MiBで動かすときのJavaのコマンド全体 → `test_checker.CheckerTests.test_sandbox_uses_fixed_file_arguments_for_every_runtime`
- 対話の両側の`--cg-mem` → `test_interactive.InteractiveTests.test_submission_memory_limit_is_separate_from_interactor_limit`
- 512での`run_args`のその他の引数 → `test_runtime_additions`の2テスト
- コンパイル時の値 → 新しい`test_compile_heap_caps_stay_fixed`

- [ ] **Step 4: 実行して成功を確かめる**

Run: Step 2と同じコマンド
Expected: `Ran 3 tests`、`OK`

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && grep -n "memory == 256\|Xmx128m\|Xmx256m" judge/sandbox.py judge/interactive.py judge/runtimes.py`
Expected: `memory == 256`は出ない。`Xmx`は`runtimes.py:32`と、`sandbox.py`の`java_heap`の中だけに出る

- [ ] **Step 5: コミットする**

`git add judge/sandbox.py judge/interactive.py docs/judge/runtime-policy.md judge/tests/test_runner.py judge/tests/test_interactive.py`、メッセージ：`fix: 提出のヒープ上限をメモリ制限に比例させ、対話ジャッジ側は明示の引数で判別する`

### Task 4: D11 rolloutのfinishは2台以上のときだけ--instanceを渡す

**Files:**
- Modify: `judge/rollout.py:411-420`（`finish()`の本番APIテスト）
- Test: `judge/tests/test_rollout.py`（`run_finish`ヘルパー、現在の175行と181行。新しいテストは`test_finish_tags_the_pool_after_health_and_releases_the_hold`の直後、現在の227行の後）

**Interfaces:**
- Consumes: `rollout.api_smoke(config, directory, nodes) -> Path`（既存。`:626`）

- [ ] **Step 1: 失敗するテストを書く**

- `run_finish(self, **extra)`を`run_finish(self, count=3, **extra)`にし、`nodes = ['i-0123456789abcdef' + str(i) for i in range(count)]`にする（既定の3台は今の3つの値と同じ）。
- 新しいテストを書く。

```python
def test_finish_passes_instances_only_with_several_hosts(self):
    for count in (1, 2):
        with self.subTest(count=count):
            state, calls, _, nodes, _ = self.run_finish(count=count)
            smoke = next(args for args in calls if 'judge/smoke-api.py' in args)
            self.assertEqual([smoke[i + 1] for i, arg in enumerate(smoke) if arg == '--instance'],
                             nodes if count > 1 else [])
            self.assertEqual(state['apiReport'], smoke[smoke.index('--report') + 1])
            self.assertEqual((state['status'], state['step']), ('passed', 'complete'))
```

変更前に失敗する理由：1台のとき、`--instance i-0123456789abcdef0`が渡る。

- [ ] **Step 2: 実行して失敗を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/judge/tests && python3 -B -m unittest -v test_rollout.RolloutTests.test_finish_passes_instances_only_with_several_hosts`
Expected: FAIL（`count=1`のsubTestで`['i-0123456789abcdef0'] != []`）

- [ ] **Step 3: 実装する**

`finish()`の、`checkpoint(directory, state, 'api-smoke')`の次の行から`raise ValueError('API smoke did not pass')`までを、`api_report = api_smoke(config, directory, config['nodes'])`の1行にする。前後の`checkpoint`（`'api-smoke'`、`'refresh-state'`）と、`state['apiReport']`に`api_report`を入れる処理は変えない。

保つ挙動：3台のときは全台を`--instance`で渡し、`complete`まで進む → `test_finish_reaches_complete_only_after_api_smoke_and_state_refresh`。`api_smoke`の条件 → `test_api_smoke_requires_concurrency_only_with_several_hosts`。失敗時に受付を止め直す → `test_failure_repauses_admission_and_records_recovery_state`。

- [ ] **Step 4: 実行して成功を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/judge/tests && python3 -B -m unittest -v test_rollout`
Expected: `OK`

- [ ] **Step 5: コミットする**

`git add judge/rollout.py judge/tests/test_rollout.py`、メッセージ：`fix: rolloutのfinishで、1台構成のときは本番APIテストに--instanceを渡さない`

---

## 全体の検証（最後のコミットの後）

1. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && python3 -B -m unittest discover -s judge/tests -v`
   Expected: `Ran 151 tests`、`OK`（開始時の145件に、新しいテスト6件を足した数）
2. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command make package-judge`
   Expected: 終了コード0
3. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && grep -rn "131072" judge --include=*.py`
   Expected: `judge/tests/test_runner.py`の行だけ
4. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`
   Expected: 終了コード0（`graphify-out/`は`.gitignore`の対象）
5. Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short && git log --oneline -4`
   Expected: 作業ツリーはきれい。4件のコミットが、D34、D33、D12、D11の順に並ぶ

## 範囲外（報告事項）

- isolateの実機での確認（`judge/smoke.py`、`judge/interactive_smoke.py`）は行わない。比例させた小さいヒープ（64 MiBで、Java 32m、TruffleRuby 32m、C# 24 MiB）で各処理系が起動できるかは未確認である。smokeがメモリの境界を確かめるのはcpp17だけ（`judge/smoke.py:86-94`）。
- JavaのVMのその他の固定値（`-Xms16m`、メタスペース96m、コードキャッシュ32m、直接メモリ32m）は、変更内容にないので比例させない。
- D33のWeb側（`SubmissionForm.vue`の2か所）はB6で扱う。
- `sandbox.execute`が自分で行う`timeLimitMs`の検証（`invalid limits`）のテストは、D34の対象外とした。
- `host.py:61`の使われない`source = code.get('source')`（C05で報告済み）。
