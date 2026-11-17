# 関数型言語4つ（Common Lisp、F#、Gleam、Fix）の追加 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ジャッジにCommon Lisp（SBCL）、F#、Gleam（Erlang）、Fixを加え、ローカルのコンテナで全fixtureに合格するランタイムの配布物を作る。

**Architecture:** Goose 2026.3の追加（コミット`825289f`）と同じく、配布済みの`runtime-current.tar.gz`へ処理系を足す。言語ごとに`build-runtime-additions.py`のステップ、`runtimes.py`の定義、smokeのfixtureを1組で加える。成果物は既存どおり1ファイルで扱い、ひな形プロジェクトが必要なGleamとFixは、`golf/compile-clay.sh`と同じくコンパイル用のシェルスクリプトで包む。

**Tech Stack:** Python 3（ビルドとワーカー）、Docker（Ubuntu 24.04）、isolate、Go（API）、Terraform、Nuxt + CodeMirror（Web）。

**Spec:** `docs/superpowers/specs/2026-11-15-functional-languages-design.md`

## Global Constraints

- 公開IDと表示名: `commonlisp-sbcl26` / Common Lisp (SBCL 2.6)、`fsharp10` / F# 10 (.NET 10)、`gleam119` / Gleam 1.19 (Erlang/OTP 29)、`fix16` / Fix 1.6。ワーカーのランタイム名は公開IDに`-isolate`を付ける。
- 版: SBCL 2.6.10、.NET SDK 10.0.401のF# 10、Gleam 1.19.2、Erlang/OTP 29.1.2、Fix 1.6.1。構築する日に新しい安定版があればその版を使い、公開IDと表示名を合わせる。
- ソースのファイル名: `main.lisp`、`main.fs`、`main.gleam`、`main.fix`。
- 配布物はURLとSHA-256を`judge/runtime/runtime-addition-sources.lock.json`に固定する。ライブラリはロックファイルで固定する。
- コンパイルと実行はネットワークを使わない。中間ファイルとキャッシュは`/box`の中だけに置く。
- 既存の言語の`compile`、`run`、`env`を変えない。既存の`dotnet`の実体を作り直さない。
- コンパイルの上限（CPU 30秒、経過40秒、1 GiB）を変えない。
- ライブラリを外す条件は設計書の4つ。コンパイル時間は、コンテナを`--cpus=1`にしたローカルのHello Worldで5秒を基準にする。
- Gleamは`gleam_stdlib`、`stdin`、`gleam_yielder`が入らなければ公開しない。
- 実機への配布と公開は、この計画に含めない。Task 7の後に利用者の了承を得て行う。
- コミットメッセージは`prefix: 対応内容`の形式にし、コードを変えたら`graphify update .`を実行する。
- テスト: `python3 -m unittest discover -s judge/tests -v`

## Review Focus

- 対話用ジャッジとして使ったとき、出力がフラッシュされず相手が待ち続ける（各言語の`response`のfixtureで確かめる。Task 1〜4）。
- メモリを使い切ったときに、MLEではなくREやJEになる（各言語の`memory`のfixture。Task 1〜4）。
- 判定用コードとして使ったとき、コマンドライン引数のファイルを読めない（各言語の`args`のfixtureを足す。Task 1〜4）。
- 深い再帰でスタックがあふれる。関数型言語では再帰が普通なので、$10^6$段の再帰がACになることを確かめる（各言語の`recursion`のfixture。Task 1〜4）。
- 大きな入力（$2 \times 10^5$行の整数）の読み込みが時間制限1秒に収まらない（Task 7で測り、収まらない言語は「使える言語」のページに読み方の注意を書く）。

---

## ファイルの構成

| ファイル | 役割 |
| --- | --- |
| `judge/runtime/build-runtime-additions.py` | `sbcl`、`fsharp`、`gleam`、`fix`の4ステップを足す。各ステップは入力のSHA-256を自分で確かめ、配布済みのツリーへ単独で足せる（`goose()`と同じ） |
| `judge/runtime/deps/additions/sbcl/`、`fsharp/`、`gleam/`、`fix/` | ひな形プロジェクト、ロックファイル、コンパイル用スクリプト |
| `judge/runner/runtimes.py` | 4つのランタイムの定義 |
| `judge/runner/sandbox.py` | 成果物の取り出しと、上限の調整（必要な場合だけ） |
| `judge/runner/language-smoke.json`、`interactive_smoke.py` | 4言語のfixture |
| `judge/tests/test_runtime_additions.py` | 4言語の定義のテスト |

各言語のステップが`/opt/judge-runtimes`の下に作るもの（後のタスクと`runtimes.py`が使う名前）:

| 言語 | ディレクトリ |
| --- | --- |
| Common Lisp | `sbcl/`（処理系と、ライブラリを読み込み済みの実行ファイル`sbcl/bin/sbcl-judge`） |
| F# | `fsharp-tools/`（`fsc.dll`と付随ファイル、`compile.args`、`ref/`）、`fsharp-libs/`（実行時にボックスへ置くDLLと`main.runtimeconfig.json`） |
| Gleam | `gleam/`（`gleam`と`compile.sh`）、`erlang/`、`gleam-project/`（依存をコンパイル済みのひな形） |
| Fix | `fix/`（`fix`と`compile.sh`）、`fix-project/`（依存を取得済みのひな形） |

4言語に共通するfixture（`language-smoke.json`、名前はGooseに合わせる）:

| 名前 | 判定 | 内容 |
| --- | --- | --- |
| `stdin` | AC | 入力`1 2\n`を読み、和の`3`を出す |
| `wrong` | WA | `4`を出す |
| `syntax` | CE | 構文エラー |
| `exception` | RE | 例外や異常終了 |
| `timeout` | TLE | 無限ループ |
| `output` | OLE | 出力し続ける |
| `memory` | MLE | メモリを確保し続ける |
| `libraries` | AC | 入れたライブラリを全部読み込み、`3`を出す |
| `cleanup` | AC | `marker`がないことを確かめてから作り、`3`を出す |
| `interactive`、`response` | AC | Gooseのfixtureと同じ対話 |
| `args` | AC | 引数がないことを確かめて`3`を出す（引数の読み方が動くことの確認） |
| `recursion` | AC | $10^6$段の再帰で和を求め、`3`を出す |

---

### Task 1: Common Lisp（SBCL 2.6.10）

**Files:**
- Modify: `judge/runtime/build-runtime-additions.py`、`judge/runtime/runtime-additions.Dockerfile`、`judge/runtime/runtime-addition-sources.lock.json`
- Create: `judge/runtime/deps/additions/sbcl/`（ライブラリの一覧とロック）
- Modify: `judge/runner/runtimes.py`、`judge/runner/language-smoke.json`、`judge/runner/interactive_smoke.py`
- Test: `judge/tests/test_runtime_additions.py`

**Interfaces:**
- Produces: `RUNTIMES['commonlisp-sbcl26-isolate']`（`source='main.lisp'`）、ビルドのステップ名`sbcl`、`/opt/judge-runtimes/sbcl/bin/sbcl-judge`。

- [ ] **Step 1: 失敗するテストを書く**

```python
def test_sbcl_compiles_and_runs_with_the_preloaded_image(self):
    runtime = RUNTIMES['commonlisp-sbcl26-isolate']
    self.assertEqual(runtime['source'], 'main.lisp')
    self.assertEqual(runtime['compile'][0], '/opt/judge-runtimes/sbcl/bin/sbcl-judge')
    self.assertEqual(runtime['run'][0], '/opt/judge-runtimes/sbcl/bin/sbcl-judge')
    # No init files: a submission must not depend on the operator's home.
    self.assertFalse(any('sbclrc' in arg for arg in runtime['compile'] + runtime['run']))
```

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `python3 -m unittest judge.tests.test_runtime_additions -v`（またはdiscover）。Expected: `KeyError: 'commonlisp-sbcl26-isolate'`

- [ ] **Step 3: ビルドのステップ`sbcl()`を書く**

- 公式バイナリ（`sbcl-2.6.10-x86-64-linux-binary.tar.bz2`）を`sbcl/`へ入れる。
- ライブラリ（cl-ppcre、fast-generic-functions、iterate、pileup、queues、str、trees、trivia、gsll）は、Quicklispの配布物のURLとSHA-256を1つずつロックに固定して取得する。Quicklispのクライアントをビルド時にネットワークへつながせない。
- 同梱モジュール（sb-gmp、sb-mpfr、sb-queue、sb-rotate-byte、sb-simd）とライブラリを読み込み、`save-lisp-and-die`（`:executable t :save-runtime-options t`、制御スタック512 MB）で`sbcl/bin/sbcl-judge`を作る。
- 外す条件に当たるライブラリは外し、理由をロックの横の`excluded.json`（`{"名前": "理由"}`）に書く。gsllは条件1に当たる見込み。sb-gmpとsb-mpfrは、libgmpとlibmpfrが`addition-native/lib`にあるときだけ入れる。
- 最後に、提出と同じコマンドでHello Worldをコンパイルして実行し、出力を確かめる（`goose()`のprobeと同じ）。
- `finish()`の`pending`と`chmod`と`content-sha256`の対象に`sbcl`を足す。

- [ ] **Step 4: `runtimes.py`に定義を足す**

`compile`は`compile-file`でfaslを`/box/main`に作る（警告だけなら成功、エラーなら終了コードを0以外にする）。`run`はそのfaslを読み込んで実行し、終了時に標準出力をフラッシュする。未捕捉のエラーはデバッガーに入らず、0以外で終了する。成果物の種類は既存の`native`を使う（1ファイル）。

- [ ] **Step 5: fixtureを足す**

共通のfixture13個を`language-smoke.json`に、`failure_program`と対話の応答を`interactive_smoke.py`に足す。`memory`がMLEになるよう、動的領域の上限をcgroupの上限より大きくする（必要なら`sandbox.run_args`でメモリ上限に応じた値を渡す）。

- [ ] **Step 6: テストが通ることを確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`。Expected: 全件PASS（`test_all_runtime_commands_are_operator_owned_and_have_smoke_fixtures`を含む）

- [ ] **Step 7: コンテナで確かめる**

`docs/judge/runtime-additions.md`のGooseの手順と同じ形で、`runtime-current.tar.gz`を展開したツリーに`sbcl`のステップだけを実行する。そのツリーをマウントしたUbuntu 24.04のコンテナ（`--network=none --cpus=1`）で、fixtureを1つずつ`runtimes.py`のコマンドで実行し、期待どおりの出力と終了コードになることを確かめる（時間・出力・メモリ超過は、`timeout`と`--memory=512m`で別に確かめる）。Hello Worldのコンパイル時間と実行時間、`--memory=64m`で動くかを記録する。

- [ ] **Step 8: コミット**

`feat: Common Lisp (SBCL 2.6)を加える`

### Task 2: F# 10

**Files:**
- Modify: `judge/runtime/build-runtime-additions.py`、`judge/runtime/runtime-additions.Dockerfile`、`judge/runtime/prepare-runtime-inputs.py`（`--additions`で既存の`dotnet.tar.gz`も入力に置く）
- Create: `judge/runtime/deps/additions/fsharp/`（`main.fsproj`と`packages.lock.json`）
- Modify: `judge/runner/runtimes.py`、`judge/runner/sandbox.py`、`judge/runner/language-smoke.json`、`judge/runner/interactive_smoke.py`
- Test: `judge/tests/test_runtime_additions.py`

**Interfaces:**
- Consumes: 既存の`/opt/judge-runtimes/dotnet/dotnet`、`dotnet-libs/MathNet.Numerics.dll`、`dotnet-libs/ac-library-csharp.dll`。
- Produces: `RUNTIMES['fsharp10-isolate']`（`source='main.fs'`、`artifact='dotnet'`）、ステップ名`fsharp`、`fsharp-tools/`、`fsharp-libs/`。

- [ ] **Step 1: 失敗するテストを書く**

```python
def test_fsharp_compiles_without_msbuild_and_ships_its_core_library(self):
    runtime = RUNTIMES['fsharp10-isolate']
    self.assertEqual(runtime['artifact'], 'dotnet')
    self.assertEqual(runtime['compile'][:2], ['/opt/judge-runtimes/dotnet/dotnet', '/opt/judge-runtimes/fsharp-tools/fsc.dll'])
    self.assertEqual(runtime['run'], RUNTIMES['csharp14-isolate']['run'])
    self.assertIn('FSharp.Core.dll', runtime['files'])
    self.assertEqual(RUNTIMES['csharp14-isolate']['files'],
                     ['main.runtimeconfig.json', 'main.deps.json', 'MathNet.Numerics.dll', 'ac-library-csharp.dll'])
```

- [ ] **Step 2: テストが失敗することを確かめる**（`KeyError: 'fsharp10-isolate'`）

- [ ] **Step 3: ビルドのステップ`fsharp()`を書く**

- `dotnet.tar.gz`（SDK 10.0.401、SHA-256は`runtime-extension-sources.lock.json`の値）から`sdk/10.0.401/FSharp/`と参照アセンブリ（`packs/Microsoft.NETCore.App.Ref/10.0.12/ref/net10.0/`）だけを取り出し、`fsharp-tools/`に置く。既存の`dotnet/`には何も書かない。
- NuGetのライブラリ（FSharpPlus、FSharpx.Collections、FSharpx.Extras、FsToolkit.ErrorHandling、MathNet.Numerics.FSharp）は、作業用に展開したSDKで`dotnet restore --locked-mode`して取得し、DLLを`fsharp-libs/`に置く。MathNet.Numericsとac-library-csharpは既存のDLLと同じ版に固定する。
- `fsharp-tools/compile.args`に、`--targetprofile:netcore --noframework --simpleresolution --target:exe --optimize+ --debug- --nologo`、全参照の`-r:`、`--out:/box/main.dll`、`/box/main.fs`を書く（`csharp-tools/compile.args`と同じ役割）。
- `fsharp-libs/`に、実行に要る`main.runtimeconfig.json`と各DLLを置く。
- probe: Hello Worldを`compile.args`でコンパイルし、`dotnet main.dll`で実行して出力を確かめる。
- `fsc.dll`の直接起動でprobeが通らないときは、設計書の代わりの方法（復元済みプロジェクトで`dotnet publish`）に切り替え、Step 1のテストの`compile`の期待値をそれに合わせる。

- [ ] **Step 4: `runtimes.py`と`sandbox.py`を変える**

- `runtimes.py`: `files`は`fsharp-libs/`の中身。`env`はC#と同じ4つ。
- `sandbox.py`の`prepare_program`は`files`を`dotnet-libs`から複写している。ランタイムごとの複写元（`files_root`、既定は今の場所）を読むようにする。
- `run_args`: `fsharp10-isolate`にもC#と同じ`DOTNET_GCHeapHardLimit`と、コンパイル時の`--open-files=256`を適用する。

- [ ] **Step 5: fixtureを足す**（共通の13個。`libraries`は7つのライブラリを全部使う。時間はC#に合わせ`timeLimitMs: 5000`を付けてよい）

- [ ] **Step 6: テストが通ることを確かめる**（discoverで全件PASS）

- [ ] **Step 7: コンテナで確かめる**（Task 1のStep 7と同じ手順。コンパイル時間が5秒を超えたら、ライブラリの参照を減らす前に、`fsc`の起動時間そのものかどうかを切り分けて記録する）

- [ ] **Step 8: コミット** `feat: F# 10を加える`

### Task 3: Gleam 1.19.2（Erlang/OTP 29.1.2）

**Files:**
- Modify: `judge/runtime/build-runtime-additions.py`、`judge/runtime/runtime-additions.Dockerfile`、`judge/runtime/runtime-addition-sources.lock.json`
- Create: `judge/runtime/deps/additions/gleam/`（`gleam.toml`、`manifest.toml`、`compile.sh`）
- Modify: `judge/runner/runtimes.py`、`judge/runner/sandbox.py`（成果物の取り出し）、`judge/runner/language-smoke.json`、`judge/runner/interactive_smoke.py`
- Test: `judge/tests/test_runtime_additions.py`、`judge/tests/test_runner.py`

**Interfaces:**
- Produces: `RUNTIMES['gleam119-isolate']`（`source='main.gleam'`、`program='main.beam'`）、ステップ名`gleam`、`gleam/compile.sh`、`erlang/bin/erl`、`gleam-project/`。

- [ ] **Step 1: 失敗するテストを書く**

```python
def test_gleam_runs_on_one_scheduler_without_the_gleam_cli(self):
    runtime = RUNTIMES['gleam119-isolate']
    self.assertEqual(runtime['compile'], ['/usr/bin/sh', '/opt/judge-runtimes/gleam/compile.sh'])
    run = runtime['run']
    self.assertEqual(run[0], '/opt/judge-runtimes/erlang/bin/erl')
    for flag, value in (('+S', '1:1'), ('+SDcpu', '1:1'), ('+SDio', '1'), ('+A', '1')):
        self.assertEqual(run[run.index(flag) + 1], value)
    self.assertIn('-noshell', run)
    self.assertFalse(any(arg.endswith('/gleam') for arg in run))
```

`test_runner.py`には、`gleam119-isolate`の成果物の取り出しが`/box`の`main.beam`を1ファイルで返し、リンクやディレクトリに置き換えられていたら拒むテストを足す（Javaの`test_java_artifact_rejects_symlink_directory`と同じ形）。

- [ ] **Step 2: テストが失敗することを確かめる**

- [ ] **Step 3: ビルドのステップ`gleam()`を書く**

- Gleamの公式バイナリ（musl版）を`gleam/`へ、OTP（`https://builds.hex.pm/builds/otp/amd64/ubuntu-24.04/OTP-29.1.2.tar.gz`）を`erlang/`へ入れ、`./Install -minimal`で置き場所を直す。`erl`が`/opt/judge-runtimes/erlang`で動かなければ、公式ソースから`--without-termcap`でビルドする。
- ひな形プロジェクト（パッケージ名`judge`）: `gleam.toml`に、AtCoderの15個（bidict、bigdecimal、bitsandbobs、dijkstra、gleam_community_maths、gleam_deque、gleam_regexp、gleam_stdlib、gleam_yielder、gleamy_structures、glearray、ieee_float、iv、stdin、trie_again）を書く。`manifest.toml`をコミットして版を固定する。Hexの各パッケージのチェックサムは`manifest.toml`が持つ。
- ビルド時に依存を取得して`gleam build`し、`build/packages`と`build/dev/erlang/<依存>`を`gleam-project/`に残す。
- probe: ネットワークのない状態（`unshare -n`またはコンテナの`--network=none`）で`compile.sh`を実行し、`gleam build`が通ることを確かめる。通らないときは`compile.sh`の中を`gleam compile-package`に替える。
- 依存が実行時に要るネイティブライブラリ（`ldd erlang/erts-*/bin/beam.smp`）を`finish()`の`pending`に足す。

- [ ] **Step 4: `compile.sh`を書く**

`/box/main.gleam`を、`/box/project`へ複写したひな形の`src/main.gleam`に置き、ビルドし、`main.beam`を`/box/main.beam`へ出す。提出が`main`以外のモジュールを作れないこと（1ファイル）を前提にする。ひな形の複写がコンパイル時間を押し上げるなら、依存のコンパイル済みディレクトリは`gleam-project`を直接参照する。

- [ ] **Step 5: `runtimes.py`と`sandbox.py`を変える**

- `run`: `erl`に上の4つの指定、`-noshell`、`-pa /box`、依存の`ebin`の`-pa`を付け、`main:main()`を呼んで`erlang:halt()`で終わる。未捕捉の例外は0以外で終了する。Gleamが生成する`judge@@main:run/1`を使えるなら、ひな形のコンパイル済みのものを使う。
- `collect_artifact`と`prepare_program`: 成果物は`main.beam`の1ファイル。既存の`compile_output`と`program`の仕組みで足りなければ、成果物の種類`beam`を足す。
- `memory`のfixtureがMLEになることを確かめ、ならなければ`+MMscs`などの指定ではなくcgroupのOOMに任せる形に直す。
- Erlangのスレッド数が`--processes=32`（対話）に収まることを確かめる。収まらなければ`+SDio`などをさらに絞る。上限は上げない。

- [ ] **Step 6: fixtureを足す**（共通の13個。`stdin`は`stdin.read_lines()`と`gleam_yielder`で読む。`libraries`は15個を全部使う）

- [ ] **Step 7: テストが通ることを確かめる**

- [ ] **Step 8: コンテナで確かめる**

Task 1のStep 7と同じ手順に加え、`--ulimit nofile=64`で起動時間を測る（isolateの`--open-files=64`に合わせる）。何もせず終了するプログラムのCPU時間が0.5秒を超えたら、起動の指定（`-mode minimal`、`-boot no_dot_erlang`、`-noinput`）を足して測り直し、値を記録する。

- [ ] **Step 9: コミット** `feat: Gleam 1.19 (Erlang/OTP 29)を加える`

### Task 4: Fix 1.6.1

**Files:**
- Modify: `judge/runtime/build-runtime-additions.py`、`judge/runtime/runtime-additions.Dockerfile`、`judge/runtime/runtime-addition-sources.lock.json`
- Create: `judge/runtime/deps/additions/fix/`（`fixproj.toml`、`fixdeps.lock`、`compile.sh`）
- Modify: `judge/runner/runtimes.py`、`judge/runner/language-smoke.json`、`judge/runner/interactive_smoke.py`
- Test: `judge/tests/test_runtime_additions.py`

**Interfaces:**
- Produces: `RUNTIMES['fix16-isolate']`（`source='main.fix'`、`artifact='native'`）、ステップ名`fix`、`fix/compile.sh`、`fix-project/`。

- [ ] **Step 1: 失敗するテストを書く**

```python
def test_fix_links_with_the_judge_gcc_and_builds_inside_the_box(self):
    runtime = RUNTIMES['fix16-isolate']
    self.assertEqual(runtime['compile'], ['/usr/bin/sh', '/opt/judge-runtimes/fix/compile.sh'])
    self.assertEqual(runtime['run'], ['/box/main'])
    path = next(env for env in runtime['env'] if env.startswith('PATH='))
    self.assertEqual(path.split('=', 1)[1].split(':')[0], '/opt/judge-runtimes/gcc/bin')
```

- [ ] **Step 2: テストが失敗することを確かめる**

- [ ] **Step 3: ビルドのステップ`fix()`を書く**

- 公式バイナリ（`fix-v1.6.1-x86_64-unknown-linux-gnu`）を`fix/fix`に置く。`ldd`で要る共有ライブラリを確かめ、`finish()`の`pending`に足す。
- ひな形プロジェクト: `fix init`と`fix deps add 名前@=版`で、AtCoderの一覧（character、hash、hashmap、hashset、math、random、regexp、time、binary-heap、bool-array、cp-library、ring-buffer、gmp-fix、minilib-*）を足す。`fixdeps.lock`をコミットする。レジストリがgitの参照で版を指すので、取得した各依存のコミットIDを`fixdeps.lock`で固定できることを確かめ、できない依存は条件2で外す。
- gmp-fixは、libgmpが`addition-native/lib`にあるときだけ入れる。
- AtCoderと同じく、全ライブラリを読み込むプログラムを`-O none`で一度ビルドして型検査のキャッシュを作り、中間ファイルを消してから`fix-project/`に残す。
- ひな形の`fixproj.toml`に`preliminary_commands`を書かない。提出は`main.fix`だけなので、利用者は`fixproj.toml`を差し替えられない。
- probe: ネットワークのない状態で`compile.sh`を実行して確かめる。取得済みの依存でもネットワークを必要とするなら、依存を全部外してStdだけにする（設計書の代わりの方法）。

- [ ] **Step 4: `compile.sh`と`runtimes.py`を書く**

`compile.sh`はひな形を`/box/project`へ複写し、`/box/main.fix`を置き、`fix build -O max`を実行して、できた実行ファイルを`/box/main`へ移す。`-O max`のHello Worldがローカルで5秒を超えたら、設計書の順に従い、まずライブラリを減らし、それでも超えるなら`-O basic`に下げる。

- [ ] **Step 5: fixtureを足す**（共通の13個）

- [ ] **Step 6: テストが通ることを確かめる**

- [ ] **Step 7: コンテナで確かめる**（Task 1のStep 7と同じ手順）

- [ ] **Step 8: コミット** `feat: Fix 1.6を加える`

### Task 5: API、配布の設定、Web

**Files:**
- Modify: `api/internal/submissions/runtimes.go`、`infra/api/variables.tf`、`judge/rollout.example.json`
- Modify: `web/app/utils/runtime-label.ts`、`web/app/utils/code-language.ts`、`web/app/utils/problem-markdown.ts`（問題文のコードブロックの色分けに4言語を足せる場合）
- Create: `web/app/utils/gleam-mode.ts`、`web/app/utils/fix-mode.ts`
- Modify: `web/app/content/docs/languages.md`
- Test: `web/tests/editor-settings.spec.ts`、`judge/tests/test_language_docs.py`（既存のテストが新しい4言語も検査する）

**Interfaces:**
- Consumes: Task 1〜4の公開IDと、決まった`compile`と`run`。
- Produces: `codeLanguage(runtime)`が`commonlisp-sbcl26`→`commonlisp`、`fsharp10`→`fsharp`、`gleam119`→`gleam`、`fix16`→`fix`を返す。

- [ ] **Step 1: 失敗するテストを書く**

`editor-settings.spec.ts`の言語ごとの表（186行目付近）に、`['commonlisp-sbcl26', '; hello']`、`['fsharp10', '// hello']`、`['gleam119', '// hello']`、`['fix16', '// hello']`を足す。`test_language_docs.py`は、`languages.md`に4言語がないことで失敗する。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `python3 -m unittest judge.tests.test_language_docs -v` と、`nix develop`の中でのPlaywright（`--output`を付ける）。

- [ ] **Step 3: APIと設定に4つの公開IDを足す**

`runtimes.go`の`Runtimes`（表示名はGlobal Constraintsのとおり）、`variables.tf`の検証の一覧、`rollout.example.json`の`runtimes`。`judge_enabled_runtimes`の既定値や`terraform.tfvars`は変えない（公開は配布の後）。

- [ ] **Step 4: Webを変える**

- `runtime-label.ts`に4行足す。
- `code-language.ts`: `commonlisp`は`@codemirror/legacy-modes/mode/commonlisp`、`fsharp`は`mode/mllike`の`fSharp`を使う。正規表現に4つを足す（`go`より前に`goose`があるのと同じく、前方一致の順に気をつける）。
- `gleam-mode.ts`と`fix-mode.ts`は`goose-mode.ts`と同じ形で、キーワード、行コメント（どちらも`//`）、文字列、数値、型名（大文字始まり）を色分けする。

- [ ] **Step 5: 「使える言語」のページを書く**

`languages.md`に4言語を、既存の言語と同じ3か所（言語とバージョンの表、言語ごとの注意、コンパイルと実行）に足す。注意に書くこと:

- Common Lisp: 使えるライブラリ、未捕捉のエラーはRE。
- F#: 使えるライブラリ。
- Gleam: モジュール名は`main`で`pub fn main()`を書く。入力は`stdin.read_lines()`で読む。起動だけでかかる時間（Task 7で測った値）。
- Fix: `module Main;`と`main: IO ()`を書く。最適化の段階。
- 各言語の対話でのフラッシュの方法。外したライブラリ。

サイトの文章なので、書いた後に`/yomiyasu`で推敲する。

- [ ] **Step 6: テストが通ることを確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`、`cd api && go test ./...`、`terraform -chdir=infra/api validate`、Webの型検査とPlaywright。

- [ ] **Step 7: コミット** `feat: 関数型言語4つをAPIと画面の言語一覧に加える`

### Task 6: 文書とADR

**Files:**
- Create: `docs/adr/0041-add-functional-languages.md`
- Modify: `docs/adr/README.md`、`docs/judge/runtime-policy.md`、`docs/judge/runtime-additions.md`

- [ ] **Step 1: ADR 0041を書く**

既存のADRの形（状態、決定日、背景、検討した選択肢、決定、影響）で、次を記録する: 4言語を加える理由、GleamをErlangで動かす判断（JavaScriptは整数の精度で不採用）、`gleam run`ではなく`erl`を直接起動する判断、ライブラリをAtCoderの一覧から選び4つの条件で外す判断、版を最新の安定版にする判断。

- [ ] **Step 2: `runtime-policy.md`の表に4行足す**（状態は「実装済み、未配布、実機smoke未実施」）

- [ ] **Step 3: `runtime-additions.md`に節を足す**

Gooseの節と同じ内容を4言語について書く: 固定した処理系と入手元、コンパイルと実行の方法、入れたライブラリと版、外したライブラリと理由（各`excluded.json`と一致させる）、ローカル検証の結果（Task 7の値）、配布済みの`runtime-current.tar.gz`へ4言語だけを加える手順。

- [ ] **Step 4: コミット** `docs: 関数型言語4つの追加の決定と構成を記録する`

### Task 7: 4言語を足した配布物を作り、まとめて確かめる

**Files:**
- Modify: `docs/judge/runtime-additions.md`（測った値とSHA-256）、`web/app/content/docs/languages.md`（Gleamの起動時間、入力の注意）

- [ ] **Step 1: 配布物を作る**

`runtime-additions.md`に書いた手順で、`runtime-current.tar.gz`へ`sbcl`、`fsharp`、`gleam`、`fix`、`finish`の順に実行し、`judge/.build/runtime-functional.tar.gz`を作る。`test ! -e`で4つのディレクトリが元のツリーにないことを先に確かめる。SHA-256を記録する。

- [ ] **Step 2: 既存の言語が変わっていないことを確かめる**

元のツリーと新しいツリーで、4言語のディレクトリと`build-manifest`以外に差がないことを確かめる。

Run: `diff -r`（4言語のディレクトリと`build-manifest`を除く）。Expected: 差なし

- [ ] **Step 3: 4言語の全fixtureをコンテナで通す**

`--network=none --cpus=1`のコンテナで、4言語の全fixtureを実行する。Expected: 全fixtureが期待どおり。

- [ ] **Step 4: 測る**

言語ごとに次を測り、`runtime-additions.md`に表で書く。

| 測るもの | 基準 |
| --- | --- |
| Hello Worldのコンパイル時間 | 5秒以内（超えたら設計書の順に対応する） |
| Hello Worldの実行時間（CPU時間） | 記録する。Gleamは`languages.md`にも書く |
| `--memory=64m`でHello Worldが動くか | 動く（動かなければ、ライブラリを減らすか、最小のメモリ上限を`languages.md`に書く） |
| $2 \times 10^5$行の整数を読んで和を出す時間 | 1秒以内。超えた言語は、速い読み方を`languages.md`に書く |

- [ ] **Step 5: 全テストを通す**

Run: `python3 -m unittest discover -s judge/tests -v`、`cd api && go test ./...`。Expected: 全件PASS

- [ ] **Step 6: コミットし、`graphify update .`を実行する** `docs: 関数型言語4つのローカル検証の結果を記録する`

---

## この計画の後（利用者の了承を得てから）

1. `RUNTIME_ARCHIVE=.build/runtime-functional.tar.gz bash judge/build-assets.sh`で配布物を作る。
2. `judge/.build/rollout.json`の`runtimes`に4つの公開IDを足し、`judge/rollout.py rolling`で配布する。完了条件は`state.json`の`status: passed`と`step: complete`。
3. 全台のsmokeに合格した言語だけを`judge_enabled_runtimes`に足して公開し、`judge/smoke-api.py`で本番の提出を確かめる。
4. 実機で測ったコンパイル時間と実行時間を`runtime-additions.md`と`languages.md`に書く。10秒を超えた言語は公開しない。
