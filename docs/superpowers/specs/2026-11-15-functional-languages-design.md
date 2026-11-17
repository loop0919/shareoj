# 関数型言語4つ（Common Lisp、F#、Gleam、Fix）の追加の設計

- 作成日: 2026-11-15
- 状態: 承認待ち
- 手本: Goose 2026.3の追加（コミット`825289f`、[追加環境の構成](../../judge/runtime-additions.md#gooseの追加)）

## 目的

ShareOJで使える関数型言語を増やす。
いまの関数型言語はHaskellだけなので、Common Lisp、F#、Gleam、Fixを加える。
特定のコンテストや要望に合わせた期限はない。

## 利用者と決めたこと

- 加える言語は、Common Lisp、F#、Gleam、Fix（[fixlang](https://github.com/tttmmmyyyy/fixlang)）の4つとする。
- GleamはErlang（BEAM）の上で動かす。
  JavaScriptへの変換は使わない。整数が$2^{53}$を超えると精度が落ちるためである。
- Gleamの標準入力は、AtCoderと同じ`stdin`と`gleam_yielder`で読めるようにする。
- ライブラリは、AtCoderの2025年10月の一覧と同等のものを、ジャッジの運用に影響しない範囲で入れる。

## やらないこと

- GleamのJavaScript版。
- 複数ファイルの提出。4言語とも、ほかの言語と同じく1ファイルを提出する。
- 言語ごとの時間制限の延長や係数。
- AtCoderと同じ版にそろえること。処理系もライブラリも、構築時点の最新の安定版を使う。
- C#が使う.NET SDKの版の変更。
- 開発用Dockerワーカーでの対応。

## 避けたい結果

- 既存の言語のコンパイル方法や実行方法が変わる。
- コンパイルや実行がネットワークを必要とする。
- ボックスの外にキャッシュや中間ファイルが残る。
- 実機smokeに合格していない言語が公開される。

## 処理系と公開ID

以下は設計者の判断である。版は2026年11月15日に確かめた最新の安定版である。

| 言語 | 公開ID | 表示名 | 処理系 | 入手元 |
| --- | --- | --- | --- | --- |
| Common Lisp | `commonlisp-sbcl26` | Common Lisp (SBCL 2.6) | SBCL 2.6.10 | SourceForgeの公式バイナリ |
| F# | `fsharp10` | F# 10 (.NET 10) | 取得済みの.NET SDK 10.0.401に含まれるF# 10 | 既存の`dotnet.tar.gz` |
| Gleam | `gleam119` | Gleam 1.19 (Erlang/OTP 29) | Gleam 1.19.2、Erlang/OTP 29.1.2 | GleamはGitHubの公式バイナリ、OTPはhex.pmのUbuntu 24.04向けビルド |
| Fix | `fix16` | Fix 1.6 | Fix 1.6.1 | GitHubの公式バイナリ |

- 配布物はすべてURLとSHA-256を`runtime-addition-sources.lock.json`に固定する。
- ソースのファイル名の拡張子は`.lisp`、`.fs`、`.gleam`、`.fix`とする。
- OTPのビルド済み配布物を`/opt/judge-runtimes`の下へ置き直して動かせないときは、公式のソースからビルドする。
- 構築する日に新しい安定版が出ていれば、その版を使い、公開IDと表示名を合わせる。

## コンパイルと実行

以下は設計者の判断である。コマンドの細部は構築する人が決める。

| 言語 | コンパイル | 実行 |
| --- | --- | --- |
| Common Lisp | ライブラリを読み込み済みのSBCLで`compile-file`し、faslを作る | 同じSBCLでfaslを読み込んで実行する |
| F# | `fsc.dll`を`dotnet`で直接起動し、`main.dll`を作る（C#と同じくMSBuildを通さない） | `dotnet main.dll` |
| Gleam | 依存をコンパイル済みのひな形プロジェクトをボックスへ用意し、提出だけをコンパイルする | `erl`を直接起動する（`gleam run`は使わない） |
| Fix | 依存を取得済みのひな形プロジェクトで`fix build -O max`を実行し、既存のGCCでリンクする | できたネイティブの実行ファイル |

- Erlangは、スケジューラーを1本に制限して起動する（`+S 1:1 +SDcpu 1:1 +SDio 1 +A 1`に相当する指定）。
  制限しないと、コア数ぶんのスレッドがCPU時間を使う。
  手元のコンテナでは、何もせず終了するまでのCPU時間が、制限なしで約1.4秒、制限ありで約0.2秒だった。
- Gleamを`erl`で直接起動するのは、`gleam run`が実行のたびにビルドを確かめ直すためである。
- AtCoderはSBCLの制御スタックを512 MBにしている。同じ指定にする。
- FixはGCCを`PATH`から探すので、実行環境の`PATH`に既存のGCCを入れる。
- F#のコンパイラは、いまビルド時に消しているSDKの中にある。
  既存の`dotnet`の実体は作り直さず、F#のコンパイラと`FSharp.Core.dll`だけを取り出して加える。

うまくいかないときの代わりの方法を、先に決めておく。

| うまくいかないこと | 代わりの方法 |
| --- | --- |
| `fsc.dll`の直接起動でコンパイルできない | 復元済みのプロジェクトで`dotnet publish`を使う（AtCoderの方法） |
| `gleam build`がネットワークを必要とする | 依存をコンパイル済みにしておき、提出だけを`gleam compile-package`でコンパイルする |
| Fixが取得済みの依存でもネットワークを必要とする | 標準ライブラリ（Std）だけで公開し、ライブラリは次の機会に回す |

## ライブラリ

出発点は[AtCoderの2025年10月の一覧](https://img.atcoder.jp/file/language-update/2025-10/language-list.html)にある各言語のライブラリである。

| 言語 | AtCoderが入れているライブラリ |
| --- | --- |
| Common Lisp | cl-ppcre、fast-generic-functions、gsll、iterate、pileup、queues、str、trees、trivia。SBCLの同梱モジュールはsb-gmp、sb-mpfr、sb-queue、sb-rotate-byte、sb-simd |
| F# | FSharpPlus、FSharpx.Collections、FSharpx.Extras、FsToolkit.ErrorHandling、MathNet.Numerics、MathNet.Numerics.FSharp、ac-library-csharp |
| Gleam | bidict、bigdecimal、bitsandbobs、dijkstra、gleam_community_maths、gleam_deque、gleam_regexp、gleam_stdlib、gleam_yielder、gleamy_structures、glearray、ieee_float、iv、stdin、trie_again |
| Fix | character、hash、hashmap、hashset、math、random、regexp、time、binary-heap、bool-array、cp-library、ring-buffer、gmp-fix、minilib-*一式 |

- 版は、構築する日に互いに解決できる最新の安定版を使い、ロックファイルで固定する。
  MathNet.Numericsとac-library-csharpは、C#用に入れてある版をそのまま使う。
- Gleamの`gleam_stdlib`、`stdin`、`gleam_yielder`は必ず入れる。これが入らなければGleamを公開しない。

「運用に影響しない範囲」は、設計者が次のように解釈した。どれかに当たるライブラリは外す。

1. イメージにないネイティブの共有ライブラリを新しく必要とする。
   gsll（libgslとlibffi）はこれに当たる見込みである。
   sb-gmp、sb-mpfr、gmp-fixは、必要なライブラリがイメージに既にあれば入れる。
2. 固定したURLとSHA-256、またはロックファイルで再現できる形で取得できない。
3. 入れると、Hello Worldのコンパイルが実機で10秒を超える。または、Hello Worldが64 MiBのメモリ上限で動かなくなる。
4. ほかのライブラリと版が解決できない。

条件3の実機の値は、配布の前には測れない。次のように扱う（設計者の判断）。

- ライブラリの採否は、ローカルのコンテナをCPU 1個に制限して測った値で決める。
  実機との差を見込み、ローカルでは半分の5秒を基準にする。
- 実機の値は、配布のときの実機smokeで測る。
  10秒を超えた言語は公開せず、ライブラリを減らして次の配布で入れ直す。
  公開していない言語がイメージにあっても、利用者には影響しない。

処理系そのものが遅いとき（ライブラリを全部外してもHello Worldのコンパイルが基準を超えるとき）は、次の順に対応する（設計者の判断）。

1. 最適化の段階を下げる（Fixなら`-O max`から`-O basic`へ）。下げたことを「使える言語」のページに書く。
2. それでも実機で20秒を超える言語は公開しない。

コンパイルの上限（CPU 30秒、経過40秒、1 GiB）は、どの言語でも変えない。

外したライブラリと理由は、[追加環境の構成](../../judge/runtime-additions.md)と、サイトの「使える言語」のページに書く。

## 3つの用途

ほかの言語と同じく、4言語とも提出、判定用コード、対話用ジャッジの3つに使えるようにする。

- 判定用コードは、ファイルのパスをコマンドライン引数で受け取る。4言語とも引数を読めることを確かめる。
- 対話では、標準出力のフラッシュの方法を言語ごとに確かめ、「使える言語」のページに書く。
- [判定用コードのコンパイル結果の使い回し](../../judge/runtime-additions.md)が、4言語の成果物（fasl、`main.dll`と付随ファイル、BEAMファイルのディレクトリ、ネイティブ）で働くことを確かめる。

## 変更する場所

Gooseの追加と同じ場所を変える。

- ランタイムのビルド: `judge/runtime/build-runtime-additions.py`、`runtime-additions.Dockerfile`、`runtime-addition-sources.lock.json`、`judge/runtime/deps/additions/`（ひな形プロジェクトとロックファイル）。
- ワーカー: `judge/runner/runtimes.py`、必要なら`judge/runner/sandbox.py`（プロセス数や開けるファイル数の上限）。
- smoke: `judge/runner/language-smoke.json`、`judge/runner/interactive_smoke.py`、`judge/tests/test_runtime_additions.py`。
- API: `api/internal/submissions/runtimes.go`、`infra/api/variables.tf`。
- 配布の設定: `judge/rollout.example.json`。
- Web: `web/app/utils/runtime-label.ts`、`web/app/utils/code-language.ts`、`web/app/content/docs/languages.md`。
- CLI: 言語IDや拡張子の対応があれば加える。
- 文書: `docs/judge/runtime-policy.md`、`docs/judge/runtime-additions.md`、新しいADR 0041。

エディターの色分けは設計者の判断で次のとおりとする。

- Common LispとF#は、CodeMirrorの既存のモード（`commonlisp`、`mllike`の`fSharp`）を使う。
- GleamとFixは既存のモードがないので、`goose-mode.ts`と同じ程度の小さなモード（キーワード、コメント、文字列、数値）を書く。

## 確かめること

ローカル（Ubuntu 24.04のコンテナに、固定した既存ランタイムと4言語を置く）で次を確かめる。

- 提出と同じコマンドで、ネットワークなしにコンパイルと実行ができる。
- smokeのfixture（AC、WA、RE、CE、時間超過、出力超過、メモリ超過、対話）が期待どおりになる。
- Hello Worldのコンパイル時間と実行時間、64 MiBの上限で動くか。
- 入れたライブラリを1つずつ読み込むプログラムがコンパイルでき、動く。
- Erlangが、isolateのプロセス数の上限（64、対話では32）と開けるファイル数の上限（64）の中で起動する。

実機では次を確かめる。

- 全台で、4言語を含む全言語の実機smokeに合格する。
- 4言語のHello Worldの実行時間を記録する。Gleamは、起動だけでかかる時間を「使える言語」のページに書く。

## 配布

- 配布済みの`runtime-current.tar.gz`へ4言語だけを加える。既存の処理系はビルドし直さない。
- ジョブの形式もDBも変わらないので、`judge/rollout.py rolling`で配布する。
  `rollout.json`の`runtimes`に4つの公開IDを加える。
- 4言語の公開（`judge_enabled_runtimes`への追加）は、`rolling`が`status: passed`と`step: complete`になり、全台のsmokeに合格してから行う。
- 一部の言語だけが実機smokeに落ちたときは、合格した言語だけを公開し、落ちた言語は公開しない。
- 実機への配布と公開は、実装とローカル検証が終わってから、利用者の了承を得て行う。

## 構築する人に任せること

- コンパイルと実行の正確なコマンドと環境変数。
- ひな形プロジェクトをボックスへ用意する方法（コピーか、読み取り専用のマウントか）。
- 各ライブラリの版と、上の4つの条件に照らした採否。
- メモリ超過をMLEとして報告させるための、各処理系のヒープやスタックの指定。
- 4言語を1つのコミットにするか、言語ごとに分けるか。
