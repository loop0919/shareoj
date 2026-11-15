# 追加ランタイムの構築

既存14ランタイムの実体を保持して9ランタイムを追加し、2026年9月19日に既存の公開12言語と合わせて21言語を公開した。
実機smokeに合格したIDだけを公開する。
配布とDB変更の完了状況は[適用記録](runtime-rollout.md)を参照する。

## 採用バージョン

| 公開ID | 実体 |
| --- | --- |
| `haskell-ghc910` | GHC 9.10.3、Stackage LTS 24.59対応コンパイラ |
| `javascript-node24` / `typescript-node24` | Node.js 24.21.0 LTS、TypeScript 7.0.2 |
| `javascript-deno29` / `typescript-deno29` | Deno 2.9.7、同梱TypeScript 6.0.3 |
| `javascript-bun14` / `typescript-bun14` | Bun 1.4.2、TypeScript 7.0.2 |
| `ruby40` | CRuby 4.0.7 |
| `ruby-truffle40` | TruffleRuby Community 40.0.0 |

Denoはユーザーが承認したGitHub公式安定版を使用する。
LTS専用配布物としては扱わない。
BunとRubyはLTSの指定がないため安定版を固定する。
HaskellはコンパイラをLTS対応版とし、ライブラリ全体をStackage snapshotそのものに固定する方式ではない。

## cLay、dc、Nibbles、Brainfuck、Whitespaceの追加

この5言語はCRubyのYJIT有効化と合わせてローリング配布し、公開済みである。
EC2の3台すべてで全28ランタイムの実機smokeと起動確認に合格してから公開し、本番APIの検証も通過した。
配布物と検証結果は[適用記録](runtime-rollout.md)に記録する。

| 公開ID | 固定した処理系 |
| --- | --- |
| `clay20250308` | [cLay](https://github.com/laycrs/cLay)、20250308-1、`5798ba2e652c5438269199bde5388769a49ee8db` |
| `dc152` | GNU bc 1.08.2同梱のdc 1.5.2 |
| `nibbles101` | [Nibbles](https://github.com/darrenks/nibbles)、1.01+、`5bbfddb9892baa5c6e3fd52e71a0db29ee994f06` |
| `brainfuck-tritium` | [Tritium](https://github.com/rdebath/Brainfuck)、`525d346c006ea30dfc847ae3b32ed44d44fa9925` |
| `whitespace13` | [whitespacers](https://github.com/CensoredUsername/whitespace-rs)、1.3.0、`5e496ec7c2a56569f9169471519b5071b4e6444b` |

cLayはC++へ変換し、既存のGCCでC++23としてコンパイルする。
Boost 1.92.0のヘッダーとACL 1.6を利用できる。
Nibblesはテキスト形式をHaskellへ変換してからGHCでコンパイルし、両方をコンパイル枠で処理する。
処理系の依存としてmemoize 1.1.2、murmur-hash 0.1.0.11、dlist 1.0と既存のsplitを固定する。
memoizeにはGHC 9.8以降のTemplate Haskell互換修正を適用し、Nibblesが生成するmainでは対話に備えて標準出力のバッファリングを無効にする。
WhitespaceのRust依存は`golf/Cargo.lock`で固定する。
ビルド用Cargoはベース配布物と同じRust配布物から導入し、各処理系のライセンスを成果物に同梱する。

Brainfuckは8ビットセル、バイト入出力とEOF=-1を指定し、入力前に標準出力をフラッシュする修正を適用する。
dcとBrainfuckは実行前の構文検査を行わず、Whitespaceは`wsc --dump`で検査する。
Nibbles、dc、Brainfuck、Whitespaceはcheckerなどの追加ファイル引数を処理系に渡さず、標準入力を使用する。
Whitespaceの空白だけのソースはAPI、worker、編集画面で保持し、エディターのTabとEnterにも空白を変換しない設定を適用する。

2026年10月21日のローカル検証では、固定した既存ランタイム上で5処理系のビルドと代表プログラムの実行を確認した。
cLayのBoostとACL、各言語の逐次対話、時間制限と出力制限に使うfixtureも確認した。
Python 155テスト、Goの関連テスト、Webの型検査、ビルドとブラウザ61テスト、Terraform 1.14のvalidateが成功した。
これらは実機のisolate内での判定検証や公開判定を代替しない。

## Textの追加

`text`は、ソースをそのまま標準出力に出す言語である。
output only問題の答えを提出するために加えた。

イメージにあるcoreutilsの`cat` 9.4を使い、新しいバイナリは加えない。
標準入力は読まず、`sh -c 'exec /usr/bin/cat /box/main.txt'`で実行する。
checker、interactor、集計コードと、入力・出力の生成と検証には使えない。
Textのcheckerはすべての出力を受理し、Textの生成コードは同じ文字列を繰り返すだけだからである。
API、worker、Webの選択肢のいずれでも除外する。

smokeのfixtureはACとWAだけを確かめる。
Textは時間超過も実行時エラーも起こさないためである。
checker、生成、対話のsmokeは行わない。

## Gooseの追加

`goose20263`は[Goose](https://github.com/aardappel/goose)のv2026.3である。
公式のLinux x64配布物をSHA-256で固定し、`/opt/judge-runtimes/goose`へ展開する。
配布物には標準ライブラリ、TinyCCの実行時ファイル、組み込みパッケージのgfxとphysicsが含まれる。
ドキュメント、サンプル、VS Code拡張は展開後に削除する。

コンパイルは`goose build --standalone --cc /opt/judge-runtimes/gcc/bin/gcc -O2`で行う。
GooseがCへ変換し、既存のGCC 16.2.0が`-O2 -lm -pthread`でネイティブコードにする。
`--standalone`で実行時ライブラリを各プログラムへ組み込むため、ボックスの外にキャッシュを残さない。
中間ファイルは`GOOSE_HOME=/box/goose-home`に置く。
gitから取得するパッケージは、ボックスにgitもネットワークもないためCEになる。

Gooseは値を置くデータスタックごとに2 GiBのアドレス空間を予約し、使った分だけをコミットする。
メモリ上限はcgroupの使用量で判定するため、予約だけでは上限に達しない。
整数の桁あふれは`--debug`なしでは検査せずに折り返し、ゼロ除算と範囲外アクセスは常に停止する。

2026年10月29日のローカル検証では、Ubuntu 24.04のコンテナに固定GCCとGooseを置き、提出と同じコマンドで確認した。
smokeのfixtureは時間・出力・メモリ超過を除いて合格し、ソースは1件0.2〜0.3秒でコンパイルできた。
時間超過、出力超過と、512 MiBのcgroupでのメモリ超過も別に確認した。
Hello Worldは64 MiBの上限でも実行できた。
これらは実機のisolate内での判定検証や公開判定を代替しない。

配布済みの`runtime-current.tar.gz`へGooseだけを加える手順は次のとおりである。
既存の処理系はビルドし直さない。

```sh
python3 judge/runtime/prepare-runtime-inputs.py --additions
runtime_stage=$(mktemp -d)
tar -xzf judge/.build/runtime-current.tar.gz -C "$runtime_stage"
test ! -e "$runtime_stage/judge-runtimes/goose"
docker run --rm -e DEBIAN_FRONTEND=noninteractive -e OWNER="$(id -u):$(id -g)" -v "$runtime_stage:/opt" \
  -v "$PWD/judge/.build/addition-inputs:/inputs/additions:ro" \
  -v "$PWD/judge/runtime/deps/additions:/build/deps:ro" \
  -v "$PWD/judge/runtime/build-runtime-additions.py:/build/build-runtime-additions.py:ro" \
  ubuntu:24.04@sha256:224a1869083a311ef3f13648a154ba79832fbef6364d31493642ca03082da254 sh -c \
  'apt-get update && apt-get install -y --no-install-recommends python3 libc6-dev libmpc3 binutils &&
   python3 /build/build-runtime-additions.py goose && chown -R "$OWNER" /opt/judge-runtimes/goose'
tar -C "$runtime_stage" -I 'gzip -1' -cf judge/.build/runtime-goose.tar.gz judge-runtimes
rm -rf "$runtime_stage"
sha256sum judge/.build/runtime-goose.tar.gz
RUNTIME_ARCHIVE=.build/runtime-goose.tar.gz bash judge/build-assets.sh
```

2026年10月29日にこの手順で作った`runtime-goose.tar.gz`のSHA-256は`3ba427f14ae6b6bcbe2c89ce4bb6c3497045e0e91190df59c85fcdfed4691d3f`である。
配布は[統合コマンドの手順](deployment-checks.md#通常は統合コマンドを使う)で行い、`rollout.json`の`runtimes`に`goose20263`を加える。
全言語の実機smokeに合格してから公開する。

2026年10月29日に`rolling`（`judge/.build/rolling-20261029T122648Z`）で全3台へ配布し、公開した。
先行台と残りの2台の両方で、Gooseを含む28言語の実機smokeに合格した。
runtime digestは`sha256:5910a1dfcf9f4ad2318da12612ba779a92844f35dd4c20251a0352d428c5edbe`、workerの配布物のSHA-256は`eef1163212f5906e7a1d8db391080735e008fef2c182f11567025188f179eba5`である。
同じ配布で、CRubyの実行をBundlerなしに変えた（issue #15）。
本番のHello Worldは、368 msから80 ms以下になった。

## 関数型言語4つの追加

Common Lisp、F#、Gleam、Fixの4言語である。
判断の経緯は[ADR 0040](../adr/0040-add-functional-languages.md)に記録している。
2026年11月15日の時点で、未配布、実機smoke未実施である。

| 公開ID | 処理系 | 入手元 | `/opt/judge-runtimes`の下に置くもの |
| --- | --- | --- | --- |
| `commonlisp-sbcl26` | SBCL 2.6.10 | SourceForgeの公式バイナリ | `sbcl/`（162 MB） |
| `fsharp10` | .NET SDK 10.0.401のF# 10（fsc 15.2.401） | C#と同じ`dotnet.tar.gz` | `fsharp-tools/`（70 MB）、`fsharp-libs/`（20 MB） |
| `gleam119` | Gleam 1.19.2、Erlang/OTP 29.1.2 | GleamはGitHubの公式バイナリ（musl）、OTPはhex.pmのUbuntu 24.04向けビルド | `gleam/`（29 MB）、`erlang/`（176 MB）、`gleam-project/`（7.4 MB） |
| `fix16` | Fix 1.6.1 | GitHubの公式バイナリ | `fix/`（85 MB）、`fix-project/`（39 MB） |

- 処理系のURLとSHA-256は`runtime-addition-sources.lock.json`に固定する。
  F#の`dotnet.tar.gz`だけは`runtime-extension-sources.lock.json`の固定を使い、`prepare-runtime-inputs.py --additions`が`addition-inputs`へ置く。
- 各ステップは入力のSHA-256を自分で確かめるので、Gooseと同じく配布済みのツリーへ単独で足せる。
- 外したライブラリは、言語ごとの`judge/runtime/deps/additions/<言語>/excluded.json`に理由とともに書く。
- 大きさは、ローカルで構築したディレクトリの値である。

### Common Lisp

- コンパイルは`sbcl/bin/sbcl-judge --script sbcl/compile.lisp /box/main.lisp /box/main`で、faslを`/box/main`に作る。
- 実行は`sbcl/bin/sbcl-judge --script /box/main`である。
  `--script`は初期化ファイルを読まず、デバッガーにも入らない。
- `sbcl-judge`は、contribとライブラリを読み込んでから保存したイメージである（100 MB）。
  動的領域1 GiBと制御スタック512 MBを、イメージに保存してある。
- 動的領域は提出のメモリ上限（最大512 MiB）より大きいので、メモリを使い切るとヒープの枯渇ではなくcgroupのOOM（MLE）になる。
- `compile-file`がfaslを作らないか、`sb-c:compiler-error`が出たときは、コンパイルを失敗（CE）にする。
  マクロの展開のエラーもCEになり、警告はCEにしない。
  ビルドの確認で、この判定が働くことを確かめる。
- ビルドはQuicklispのクライアントを使わない。
  `quicklisp.lock.json`の32リリースをURLとSHA-256で取得し、ASDFで読み込む。
  取得した配布物は、ライセンスとソースとして`sbcl/sources/`に残す。
- 読み込んだシステムは変更不可として登録するので、提出の`require`は何もせず、ディスクを探さない。
  `ql:quickload`はなく、書くとCEになる。
- GCの間隔は、既定ではヒープが51 MiB増えるごとである。
  64 MiBの上限では、短命なデータを多く作るプログラムがMLEになる（2 GBを作っては捨てるプログラムで確認した）。
  そこで`sandbox.py`が`SBCL_BYTES_CONSED_BETWEEN_GCS`を渡し、イメージの初期化フックがそれを読む。
  値はコンパイルで51 MiB、対話用ジャッジで25 MiB、提出ではメモリ上限に比例する（512 MiBで51 MiB、64 MiBで6 MiB）。
  64 MiBで同じプログラムを動かすと、ピークは26 MiBだった。
- `LD_LIBRARY_PATH`に`addition-native/lib`を入れる。
  イメージは起動のたびに、libgmp、GSL、libffiを名前で開く。

入れたライブラリは次のとおりである。

- SBCL 2.6.10付属のcontrib: sb-gmp、sb-queue、sb-rotate-byte、sb-simd。
- Quicklisp 2026-01-01: cl-ppcre、gsll、iterate、pileup、queues（simple-queue、priority-queue、simple-cqueue、priority-cqueueを含む）、str、trees、trivia。
- gsllは、設計では条件1に当たる見込みだったが入れた。
  実行時に開くlibgsl、libgslcblas、libffiが、`addition-native/lib`に既にあるからである。
  ビルドには、gcc、libc6-dev、libgsl-dev、libffi-dev、pkg-configが要る。
- gsllの依存のuiop 3.3.7が、SBCL 2.6.10付属のUIOP 3.3.1をイメージの中で置き換える。
  Quicklispで読み込んだときと同じである。
- Quicklispの配布の日付を変えるときは、`libraries.json`の`quicklisp`と`quicklisp.lock.json`を一緒に変える。

外したライブラリは次の2つである。

| ライブラリ | 理由 |
| --- | --- |
| fast-generic-functions | SBCL 2.6.10で消えた`sb-pcl::check-applicable-keywords`を参照し、読み込みがSB-PCLのパッケージロックに違反する。Quicklisp 2026-01-01の版は上流の最新（2025-03-13）と同じである。原因はSBCLとの非互換で、`excluded.json`では最も近い条件4に分けた |
| sb-mpfr | libmpfrが`addition-native/lib`にない（条件1） |

### F#

- コンパイルは`dotnet fsharp-tools/fsc.dll @fsharp-tools/compile.args`で行い、MSBuildもrestoreも使わない。
- `compile.args`はビルドが作る。
  `--optimize+`、`--nocopyfsharpcore`などの指定と、参照するDLLを並べる。
  参照するのは、配布済みの.NET 10.0.12の参照アセンブリ167個と、`fsharp-libs`のDLLである。
- 実行はC#と同じ`dotnet /box/main.dll`である。
  `fsharp-libs`の`main.runtimeconfig.json`と8個のDLLを、ケースごとにボックスへ複製する。
  `main.deps.json`は置かない。ないと、ホストはアプリのディレクトリのDLLをすべて読み込む。
- 複製は1ケースあたり20.7 MB（FSharpPlus.dllだけで11 MB）で、ローカルでは約10 msかかる（C#は1.7 MBで約1 ms）。
  計測時間の外だが、ケースごとの経過時間とディスクへの書き込みが増える。
  実機の配布で測る。
- FSharp.CoreはSDKに入っているReadyToRun版（10.1.401）を使う。
  NuGetの版と同じ番号で、Hello Worldの起動が約8 ms速い。
- 環境変数はC#と同じで、`DOTNET_EnableWriteXorExecute=0`を含む（[C#の変更](#cの変更)）。
- GCのヒープの上限もC#と同じく、メモリ上限の3/8（512 MiBで192 MiB、64 MiBで24 MiB）である。
  マネージドヒープを使い切るとOutOfMemoryExceptionで止まり、MLEではなくREになる。
  Javaのヒープ上限と同じ扱いである。
  `memory`のfixtureはアンマネージドのメモリでMLEを確かめ、F#だけの`heap`のfixtureがREを確かめる。
- コンパイルでは、開けるファイル数をC#と同じ256にする。
  64ではfscが「Out of memory.」で止まる。
- メインスレッドのスタックには上限がない。
  isolateは`--stack`を渡さないとスタックを無制限にし、`run_args`は渡さない。
- ワーカーにlibicu74が要る。
  `judge/deploy/install.sh`で入る。
- `fsharp-tools`は、SDKの`FSharp/`をそのまま置いたものである（fsiやMSBuildのtargetsも含む70 MB）。
  削ることはできる。
- F# 11（FSharp.Core 11.0.101）は.NET 11を必要とするので使わない。

ライブラリは`main.fsproj`と`packages.lock.json`で固定し、nuget.orgから`--locked-mode`で取得する。
推移的な依存はない。

| ライブラリ | 版 |
| --- | --- |
| FSharp.Core | 10.1.401（SDKの版） |
| FSharpPlus | 1.9.1 |
| FSharpx.Collections | 3.1.0 |
| FSharpx.Extras | 3.3.0 |
| FsToolkit.ErrorHandling | 5.2.0（6.0.0はベータ） |
| MathNet.Numerics | 5.0.0（C#と同じ） |
| MathNet.Numerics.FSharp | 5.0.0 |
| ac-library-csharp | 4.1.4（C#と同じ） |

- MathNet.Numericsとac-library-csharpのDLLが、C#の`dotnet-libs`とバイト単位で同じことを、ビルドが確かめる。
- `runtimes.py`の`files`は、固定したパッケージ1つにつきDLLを1つ並べる。
  推移的な依存が増えるとビルドが止まるので、`files`にも足す。
  `files`とロックが合うことは、テストが確かめる。
- 外したライブラリはない（`excluded.json`は`{}`）。

### Gleam

コンパイルは`sh gleam/compile.sh`で、次の順に行う。

1. ひな形`gleam-project`の`gleam.toml`、`manifest.toml`、`src`、`build`を`/box/project`へ複製し、`/box/main.gleam`を`src/main.gleam`に置く。
2. `gleam build --no-print-progress`を実行する。
   依存はコンパイル済みで、提出のモジュールだけをコンパイルする。
3. `beam_lib`で`main/0`があるかを確かめ、なければ「main.gleam must define pub fn main()」でCEにする（約135 ms）。
4. `main.beam`を`/box/main.beam`へ置く。

- 複製は`cp -R`なので、更新時刻を引き継がない。
  ローカルでは、更新時刻が新しくなっても、`gleam build`は依存をコンパイルし直さず、提出のパッケージ（`judge`）だけをコンパイルした（2026年11月15日、tmpfsのボックスで確認）。
  ビルドは、複製した`build/`のキャッシュをgleamがそのまま使うことに頼っている。
  実機でもコンパイル時間で確かめる。
- `compile.sh`は`HOME`を設定せず、サンドボックスの`HOME=/box`に頼る。
- gleamは、生成したErlangをescriptでコンパイルする。
  そのため`ERL_FLAGS`で、実行と同じスケジューラーの指定と`+JMsingle true`を渡す。
- 成果物の`main.beam`は、生成したJavaScriptと同じ`generated-source`の仕組み（`compile_output`と`program`）で取り出す。
  シンボリックリンクとディレクトリは拒否し、`sandbox.py`は変えていない。

実行は`erl … -pa /box -pa gleam-project/ebin -eval 'judge@@main:run(main)' -extra`である。

- `judge@@main`は、gleamがパッケージに生成する入口のモジュールである。
  標準入力をバイナリにし、panicで終了コード1にする。
- 判定用コードのファイルは`-extra`の後に付くので、`init:get_plain_arguments()`にはそれだけが入る。
- `gleam-project/ebin`は、全依存のBEAMファイル（74個）を1つにまとめたディレクトリである。
  依存ごとに`-pa`を17個並べると、erlはモジュールを読み込むたびに全部を探すので、起動が約50 ms遅くなる。

実行の指定と理由は次のとおりである（ローカル、`--cpus=1`）。

| 指定 | 理由 |
| --- | --- |
| `+S 1:1 +SDcpu 1:1 +SDio 1 +A 1` | 既定ではCPUの数だけスレッドを作り、64プロセスの上限で起動に失敗する（「Failed to create aux thread」） |
| `+sbwt none +sbwtdcpu none +sbwtdio none` | busy waitをやめる。起動と`io:get_line`が速くなり、対話の待ちでCPU時間を使わない。1行ずつの出力は約20%遅くなる |
| `-mode minimal -boot no_dot_erlang` | 分散などを起動しない（`erlc`と同じ指定）。何もしないプログラムの起動が229〜235 msから196〜201 msになった |
| `+JMsingle true` | 必須である。JITの既定の二重写像はファイルサイズの上限より大きいmemfdを作り、起動時に`SIGXFSZ`（終了コード153）で止まる |
| `+P 262144` | 既定の$2^{20}$のプロセス表は常駐する。Hello Worldの使用量が48.4 MBから36.0 MBになる。同時に動かせるプロセスは262,144までで、超えると`system_limit`でREになる |
| `+MMmcs 1` | 既定では解放したセグメントを10個まで保持し、伸び続けるヒープのピークが2〜3倍になる。1にすると、`dict.insert`を$10^6$回するプログラムのピークが264 MiBから123 MiBになった。作っては捨てる処理は最大で約10%遅い |
| `ERL_CRASH_DUMP_SECONDS=0` | クラッシュダンプを`/box`に書かない。大きなダンプはファイルサイズの上限に届き得る |

- `-noinput`は使わない。
  `io:get_line`が`{error,terminated}`になる。
- 最終の指定で、何もしないプログラムの起動はCPU 147 msである。
- $10^6$段の非末尾再帰は約85 MiBを使う。
  スタックはプロセスのヒープの上に伸びるからである。
  64 MiBではMLEになり、$4 \times 10^5$段までは動く。
- `LD_LIBRARY_PATH`に`addition-native/lib`を入れる。
  `beam.smp`はlibtinfo、libz、libstdc++、libgcc_sを使い、新しく要るのはlibtinfo.so.6だけである。
  `finish`がこれを`addition-native/lib`へ複製する。
  ハッシュ（Ubuntuの`libtinfo6 6.4+20240113-1ubuntu2.2`）は`native-library-sha256.json`に先に書いてあり、`finish`を実行するまで照合されない。
- `finish`がlddで調べるのは`beam.smp`だけである。
  OTPのNIF（`crypto.so`など）が要るライブラリは調べていない。
- ビルドの確認は、ネットワークを使わないことを、届かないHTTPプロキシで確かめる。
  既定のコンテナでは`unshare -n`が使えないからである。

ライブラリは`gleam.toml`と`manifest.toml`で固定する。
ビルドは、Hexの各tarballのSHA-256も`manifest.toml`のouter checksumと照らし合わせる。

| ライブラリ | 版 |
| --- | --- |
| bidict | 1.0.0 |
| bigdecimal | 1.4.0（依存のbigi 4.1.1も入る） |
| bitsandbobs | 1.1.0 |
| dijkstra | 1.0.0 |
| gleam_community_maths | 2.0.1（最新の2.0.2は`gleam_stdlib` 1.0以上が要る） |
| gleam_deque | 1.0.0 |
| gleam_regexp | 1.1.1 |
| gleam_stdlib | 0.71.0（最新は1.0.5） |
| gleam_yielder | 1.1.0 |
| gleamy_structures | 1.3.0 |
| glearray | 2.1.2 |
| ieee_float | 1.6.0（2.0系は`gleam_stdlib` 1.0以上が要る） |
| iv | 1.4.4 |
| stdin | 2.0.2 |
| trie_again | 1.1.4 |

- `gleam_stdlib`が0.71.0なのは、最新の`stdin` 2.0.2が1.0未満にしか対応しないからである。
  `stdin`は必須なので、`gleam_stdlib`の側を下げた。
  1.x向けの例の一部（`list.range`、`io.debug`）は動かない。
- 外したライブラリはない（`excluded.json`は`{}`）。
  必須の`gleam_stdlib`、`stdin`、`gleam_yielder`が入っているので、公開できる。
- GleamとOTPの配布物にはライセンスのファイルがない。
  dijkstra、gleam_deque、gleam_regexp、gleam_yielder、stdinにもなく、それぞれの`gleam.toml`のライセンスの欄だけがある。

$2 \times 10^5$行の入出力は、ローカルで次のとおりだった（CPU時間、起動の約0.15秒を含む）。

| 処理 | CPU時間 |
| --- | --- |
| `stdin.read_lines()`と`gleam_yielder`で整数を読んで足す（AtCoderの方法） | 1.88〜1.92秒 |
| `io:get_chars`で64 KiBずつまとめて読み、`string.split_once`で分ける | 0.46秒 |
| `io.println`を1行ごとに呼ぶ | 10.0〜10.4秒 |
| `string.join`でつなぎ、`io.println`を1回呼ぶ | 0.22〜0.23秒 |

`io:get_line`と`io:put_chars`は、1回ごとに標準入出力の中継のプロセスを通る。
速い読み方と書き方は、「使える言語」のページに書いた。

### Fix

コンパイルは`sh fix/compile.sh`で、次の順に行う。

1. `/box/project`を作り、ひな形`fix-project`を`cp -R --preserve=timestamps`で複製する。
   `/box/project`が既にあれば失敗する。
2. `/box/main.fix`を`main.fix`として置く。
3. `fix build -O max -o /box/main`を実行する。

- 提出から入るのは`main.fix`だけで、`fixproj.toml`、ロック、依存は運用側のひな形のものである。
- 複製は36 MiB、1,720ファイルで、ローカルでは50〜130 msかかる。
  fixはキャッシュをプロジェクトの中に書き、シンボリックリンクの`.fixlang`を拒むので、複製が要る。
  `/opt`と`/box`は別のマウントなので、ハードリンクも使えない。
- 更新時刻を引き継ぐのは、4つのライブラリ（cp-library、minilib-common、minilib-text、time）の`preliminary_commands`のためである。
  fixはビルドのたびに`make -s lib.o`を実行し、これは`lib.o`が`lib.c`より新しい間だけ何もしない。
  1回のコンパイルで`make`を8回呼ぶ。
- サンドボックスの中で`/usr/bin/make`が要る。
  ワーカーには`judge/deploy/install.sh`の`build-essential`で入る。
  実機で確かめる。
- `PATH`の先頭に既存のGCC 16（`/opt/judge-runtimes/gcc/bin`）を置く。
  fixは、実行時ライブラリのコンパイルとリンクに`PATH`のgccを使う。
- gmp-fixは`-lgmp`でリンクする。
  ワーカーにはリンク用の名前の`libgmp.so`がないので、ひな形の`library_paths`に`addition-native/lib`を書き、実行時もそこから読む。
- ひな形には`preliminary_commands`を書かない。
  `fix build`はプロジェクトのコマンドを実行するからである。
  テストが、ひな形にないことと、`files`が`main.fix`だけであることを確かめる。
- ビルドは全ライブラリの全モジュール（131個）をimportするプログラムを1回ビルドし、型検査のキャッシュ（25 MB）を残す。
  キャッシュがないと、Hello Worldのコンパイルに6.8秒かかる。
- fixの既定どおり、ホストのCPUの機能に合わせたコードを作る。
  コンパイルと実行は同じワーカーで行い、判定用コードのキャッシュもワーカーごとなので、いまは問題ない。
  成果物を別のCPUのホストで使うようになったら見直す。
- コード生成は、128関数ごとの単位に1スレッドを同時に起動する。
  ローカルで最大のfixtureは約11単位で、2 CPUでは最大7タスクだった。
  約7,000関数の巨大なプログラムは`--processes=64`に届き、CEになり得る。
  ひな形の`cu_size = "inf"`で抑えられるが、既定のままにした。
- 実行時の検査（配列の範囲、`undefined`、`as_`）は有効のままで、失敗はREになる。
  `--no-runtime-check`にするかは、利用者が決める。
- 実行は`/box/main`である。
  `LD_LIBRARY_PATH`に`addition-native/lib`を入れ、libgmpをそこから読む。
- `fix`の実行ファイルはlibzstd.so.1を使う。
  `finish`がこれを`addition-native/lib`へ複製する（Ubuntuの`libzstd1 1.5.5+dfsg2-2build1.1`、ハッシュは先に書いてある）。
  そのため、`LD_LIBRARY_PATH`に`addition-native/lib`を入れる言語（Haskell、Nibbles、CRuby、TruffleRuby、Brainfuck、Whitespace、SBCL、Gleam、Fix）で、libzstd.so.1を使うものはホストではなくこの複製を読み込む。
  libtinfo.so.6も同じである。
  OSの更新でホストのパッケージが新しくなっても、この複製は変わらない。

ライブラリは24個で、版はすべて構築した日に使える最新である。

- binary-heap 0.2.1、bool-array 0.2.0、character 1.0.1、cp-library 0.16.0、gmp-fix 0.12.0、hash 1.1.3、hashmap 2.2.0、hashset 2.1.0、math 2.0.2、random 1.1.3、ring-buffer 0.1.2、time 2.0.1。
- minilib-app 0.9.0、minilib-binary 0.8.0、minilib-collection 0.10.0、minilib-common 0.15.2、minilib-comonad 0.8.0、minilib-crypto 0.8.0、minilib-json 0.8.0、minilib-math 0.10.0、minilib-monad 0.13.0、minilib-random 0.9.0、minilib-text 0.11.1、minilib-xml 0.8.0。
- `fixdeps.lock`でgitのコミットに固定し、`fixdeps.sha256.json`で`.git`を除いた木のSHA-256にも固定する。
  このSHA-256はパスと内容だけを見て、権限は見ない。
  ビルドは、各`.git/HEAD`がロックのコミットであることも確かめる。
- `.git`は残す。
  `fix build`が各HEADを確かめるからである。
- ビルドは、GitHubの24リポジトリを履歴ごとcloneする（`.git`は合わせて9 MB）。
  リポジトリが消えると、構築し直せない。
- 各ライブラリのLICENSEは`fix-project/.fixlang/deps/`の下にある。
  `fix`の配布物は実行ファイルだけでライセンスのファイルがなく、ロックにMITと書いてある。

使うとコンパイルが長くなるライブラリがある（ローカル、`-O max`、CPU時間）。

| 使うライブラリ | コンパイル |
| --- | --- |
| json、xml、app以外の1つ（20個で測った） | 1.17〜1.81秒 |
| json、xml、app以外の21個すべて | 4.1秒 |
| minilib-xml | 5.6秒 |
| minilib-app | 7.8秒 |
| minilib-json | 8.1秒 |
| regexp（外した） | 29.5秒、690 MiB |
| 24個すべて | 28.0秒 |

- regexpは、上限の30秒に届くので外した（`excluded.json`）。
  `-O basic`では12.4秒になるが、実行が約4倍遅くなる。
- 1つのfixtureで24個すべてを使うと間に合わないので、`libraries`を4つに分けた。
  `libraries`はjson、xml、app以外の21個を使い、`libraries-json`、`libraries-xml`、`libraries-app`はそれぞれ1つを使う。
  最も長い`libraries-app`は8.8秒である。
- 実機との差はまだ分からない。
  実機が2倍遅ければ、8.8秒は約18秒になる。
  実機でfixtureが上限を超えたら、同じ扱いで外す。

### C#の変更

2026年11月15日に、利用者の依頼でC#の環境変数に`DOTNET_EnableWriteXorExecute=0`を加えた（コミット`96dba09`）。
既存の言語の`compile`、`run`、`env`を変えたのは、これだけである。

- W^Xが有効だと、.NETはJITのコードを二重に写像するmemfdを使い、その大きさはファイルサイズの上限で決まる。
  isolateの`--fsize`（コンパイル時32 MiB、実行時16 MiB）が、JITのコードの上限になる。
- cscはHello Worldで17〜20 MiBを使い、余裕が少ない。
  F#のfscは21〜28 MiBを使い、超えるとコンパイラが落ちてCEに見える。
- コンパイルでも実行でも無効にする。
  ローカルでは、F#のコンパイルと実行の時間は変わらず、C#の6つのfixtureがコンテナで合格した。
- F#の環境変数はC#の写しである。
  テストが、C#とF#の`run_args`が同じであることを確かめる。
- C#の環境変数が変わるので、次の配布でruntime digestが変わる。
  4言語と合わせて、C#の実機smokeの合格も確かめる。

### ローカル検証

2026年11月15日に、言語ごとにステップを共有のステージ（`runtime-goose.tar.gz`を展開したもの）で実行し、Ubuntu 24.04のコンテナで提出と同じコマンドを確かめた。

- コンテナは`--network=none --cpus=1 --pids-limit=64`で、開けるファイル数とファイルサイズの上限は`run_args`の値に合わせた。
  メモリはコンパイルで1 GiB、実行で512 MiBである。
- 全fixtureが期待どおりになった。
  共通の13個に加え、F#の`heap`とFixのライブラリの3つも含む。
  各fixtureを新しいボックスで2回ずつ実行した。
- パイプ越しの提出、4つのファイル引数を受け取るinteractor、checker、判定用コードの引数の読み方も確かめた。
- F#、Gleam、Fixでは、コンパイルと実行が`/box`の外に何も書かないことも確かめた。

| 言語 | Hello Worldのコンパイル | Hello Worldの実行 | 64 MiBの上限 | $10^6$段の再帰 |
| --- | --- | --- | --- | --- |
| Common Lisp | 14〜17 ms | 9〜11 ms | 動く（ピーク5 MiB） | 動く（スタック約30 MB） |
| F# | 2.40〜2.62秒 | 42〜54 ms | 動く（ピーク6〜7 MiB） | 動く（ピーク36〜37 MiB） |
| Gleam | CPU 604〜667 ms | CPU 141〜166 ms | 動く（ピーク24 MiB） | ピーク85 MiB。64 MiBではMLE |
| Fix | CPU 1.29〜1.40秒 | CPU 0〜1 ms | 動く（ピーク4〜5 MiB） | 動く（ピーク32 MiB） |

- Common LispとF#の時間は経過時間、GleamとFixはcgroupのCPU時間である。
- F#の64 MiBの値は、isolateと同じく、ボックスのファイルをcgroupの外から置いたときのものである。
- Fixの$2 \times 10^5$個の整数の読み込みは、`CPLib.IO::read_is`でCPU 7 msだった。
  Common LispとF#の$2 \times 10^5$行の読み込みは、まだ測っていない。
- 4言語を足した配布物での検証（Task 7）と、実機の値はまだない。
- これらは実機のisolate内での判定検証や公開判定を代替しない。

### 配布済みのランタイムへ4言語を加える手順

基にするのは、配布中の`runtime-goose.tar.gz`（SHA-256 `3ba427f14ae6b6bcbe2c89ce4bb6c3497045e0e91190df59c85fcdfed4691d3f`）である。
`runtime-current.tar.gz`はGooseより前の版なので、基にするとGooseが消える。
既存の処理系はビルドし直さない。

```sh
python3 judge/runtime/prepare-runtime-inputs.py --additions
runtime_stage=$(mktemp -d)
tar -xzf judge/.build/runtime-goose.tar.gz -C "$runtime_stage"
for name in sbcl fsharp-tools fsharp-libs erlang gleam gleam-project fix fix-project; do
  test ! -e "$runtime_stage/judge-runtimes/$name" || echo "already exists: $name"
done
build_step() {
  step=$1
  shift
  docker run --rm -e DEBIAN_FRONTEND=noninteractive -e OWNER="$(id -u):$(id -g)" -v "$runtime_stage:/opt" \
    -v "$PWD/judge/.build/addition-inputs:/inputs/additions:ro" \
    -v "$PWD/judge/runtime/deps/additions:/build/deps:ro" \
    -v "$PWD/judge/runtime/build-runtime-additions.py:/build/build-runtime-additions.py:ro" \
    ubuntu:24.04@sha256:224a1869083a311ef3f13648a154ba79832fbef6364d31493642ca03082da254 sh -c \
    "apt-get update && apt-get install -y --no-install-recommends python3 $* &&
     python3 /build/build-runtime-additions.py $step; s=\$?; chown -R \"\$OWNER\" /opt/judge-runtimes; exit \$s"
}
build_step sbcl bzip2 ca-certificates gcc libc6-dev libgsl-dev libffi-dev pkg-config &&
build_step fsharp ca-certificates libicu74 &&
build_step gleam ca-certificates &&
build_step fix ca-certificates libc6-dev libmpc3 binutils make &&
build_step finish build-essential ca-certificates curl xz-utils bzip2 unzip git pkg-config cmake libssl-dev \
  libyaml-dev libreadline-dev zlib1g-dev libffi-dev libgmp-dev libnuma-dev libncurses-dev libopenblas-dev \
  liblapack-dev libgsl-dev libglpk-dev libgeos-dev libz3-dev libicu74
tar -C "$runtime_stage" -I 'gzip -1' -cf judge/.build/runtime-functional.tar.gz judge-runtimes
rm -rf "$runtime_stage"
sha256sum judge/.build/runtime-functional.tar.gz
```

- ステップの順は`sbcl`、`fsharp`、`gleam`、`fix`、`finish`である。
- `sbcl`から`fix`までの4行は、2026年11月15日に共有のステージで実行したものと同じパッケージである。
  各ステップは30秒から1分ほどで終わった。
- `finish`の行はまだ実行していない（Task 7）。
  パッケージは`runtime-additions.Dockerfile`のtoolchainsと同じにした。
  `finish`は`addition-native/lib`のライブラリを`native-library-sha256.json`と照らし合わせるので、Ubuntuのパッケージが更新されていると止まる。
  差をレビューしてから、ロックを更新する。
- `finish`は4言語のディレクトリを`content-sha256.json`に加え、`beam.smp`と`fix`のlddからlibtinfo.so.6とlibzstd.so.1を`addition-native/lib`へ複製する。
  ruby-depsとtruffleruby-depsの共有ライブラリにも`strip`をかけ直す。
  4言語のディレクトリ、`build-manifest`、`addition-native/lib`に加わる2つのファイルのほかに差がないことを、`diff -r`で確かめる。
- `prepare-runtime-inputs.py --additions`は、240 MBの`dotnet.tar.gz`も取得する。
  `judge/.build/extension-inputs/dotnet.tar.gz`を先に`judge/.build/addition-inputs/`へ複製すれば、取得を省ける。
  どちらでもSHA-256を確かめる。
- 各ステップは、自分のディレクトリが既にあると失敗する（`mkdir`や`copytree`が拒む）。
  途中で失敗したら、そのステップのディレクトリだけを消してやり直す。

| ステップ | ディレクトリ |
| --- | --- |
| `sbcl` | `sbcl/` |
| `fsharp` | `fsharp-tools/`、`fsharp-libs/` |
| `gleam` | `erlang/`、`gleam/`、`gleam-project/` |
| `fix` | `fix/`、`fix-project/` |

- ステップの中でも取得がある。
  Quicklispの配布物、NuGet、Hex、GitHubのcloneである。
  Quicklispの取得（`fetch()`）は再試行しない。
  取得に失敗したら、そのステップのディレクトリを消してやり直す。
- 全体のビルド（`runtime-additions.Dockerfile`）にも4つのステップを加えた。
  ただし全体のビルドでは`sbcl`が`finish`より前に動き、`addition-native/lib`にまだGSLなどがない。
  この順序は確かめていない。

### 依存を更新するとき

`docker run`に`-e RESOLVE=1`を足すと、ステップがロックを解決し直す。
`deps/`は読み取り専用でマウントするので、解決したロックはツリーの各言語のディレクトリに書かれる。
手で`judge/runtime/deps/additions/`へ戻し、レビューしてから、新しいステージで`RESOLVE`なしにビルドして確かめる。

| ステップ | 書かれるファイル | 戻す先 |
| --- | --- | --- |
| `sbcl` | `sbcl/quicklisp.lock.json` | `sbcl/quicklisp.lock.json` |
| `fsharp` | `fsharp-tools/packages.lock.json` | `fsharp/packages.lock.json` |
| `gleam` | `gleam-project/manifest.toml` | `gleam/manifest.toml` |
| `fix` | `fix-project/fixdeps.lock`、`fix/fixdeps.sha256.json` | `fix/fixdeps.lock`、`fix/fixdeps.sha256.json` |

- 解決にはネットワークが要る。
  Fixは`fix deps update`でレジストリを読む。
- 入れるライブラリは、Common Lispは`libraries.json`、F#は`main.fsproj`、Gleamは`gleam.toml`、Fixは`fixproj.toml`で変える。
- Fixのライブラリは`fix deps add name@=version`で足す。
  `fixproj.toml`に`preliminary_commands`を書かない。

### 公開までに残っていること

1. 上の手順で`runtime-functional.tar.gz`を作り、4言語を足した配布物で全fixtureと測定をやり直す（Task 7）。
   $2 \times 10^5$行の整数の読み込みも測る。
   `finish`の後に、`addition-native/lib`を使う既存の言語のfixtureも通す。
   SHA-256と値をこの節に書く。
2. `RUNTIME_ARCHIVE=.build/runtime-functional.tar.gz bash judge/build-assets.sh`で、workerの配布物を作る。
3. 利用者の了承を得てから、`rollout.json`の`runtimes`に4つの公開IDを加え、`judge/rollout.py rolling`で配布する。
   手順と完了の条件は[配布と検証の定型コマンド](deployment-checks.md#os更新はローリング更新で配布する)に従う。
   C#を含む全言語の実機smokeに合格することを確かめる。
4. 実機で、次を測る。
   - 4言語のHello Worldのコンパイル時間と実行時間。コンパイルが10秒を超えた言語は公開しない。
   - Gleamの起動のCPU時間。「使える言語」のページの約0.15秒（ローカルの値）を置き換える。
   - Gleamのコンパイル時間（`cp -R`の複製で依存がコンパイルし直されないこと）。
   - F#のケースごとのDLLの複製の時間。
   - Fixの`libraries-json`、`libraries-xml`、`libraries-app`のコンパイル時間と、サンドボックスの中の`make`。
5. 全台のsmokeに合格した言語だけを`judge_enabled_runtimes`に加えて公開し、`judge/smoke-api.py`で本番の提出を確かめる。
   一部の言語だけが落ちたときは、合格した言語だけを公開する。

## 依存の固定

対象一覧は[AtCoder 2025年10月一覧](https://img.atcoder.jp/file/language-update/2025-10/language-list.html)から抽出し、`judge/runtime/deps/additions/catalog.json`に保存する。
追加指示によりTorch、LightGBM、Rumaleと専用の推移的依存を除外する。
LibTorchも配布しない。

公式配布物のURLとSHA-256は`judge/runtime/runtime-addition-sources.lock.json`に固定する。
npmは`package-lock.json`、JSRは`deno.lock`、Rubyは処理系別のGemfile.lock、Haskellは`cabal.project.freeze`とビルドplanを使用する。
GHC組み込みパッケージはGHC付属版を維持する。
HaskellのHackage index-stateは2026年9月18日00:00 UTCに固定し、ソースアーカイブのSHA-256も保存する。
Denoは一覧のパッケージの全公開サブパスを事前キャッシュする。
通常ビルドで最新版を再解決しない。

Ruby 4ではfiddleが標準添付から外れているため、numo-linalgの構築依存として固定導入する。
numo-openblasは自前のOpenBLASヘッダーを優先し、ビルドホスト固有のAVX命令へ固定されないようCORE2向けに構築する。
OR-Toolsのネイティブ配布物はgem側で固定されたURLとSHA-256を使用する。
TruffleRuby 40ではOR-Toolsが必要とする`rb_frame_method_id_and_class`が未実装である。
Rice 4.12と4.11.5ではGC root登録時の異常終了、4.10では当該シンボル欠落を確認した。
ユーザー承認によりTruffleRubyだけOR-Toolsを除外し、CRubyでは0.18.0を提供する。

## コンパイルと隔離

内部IDは公開IDに`-isolate`を付ける。
API形式とDBスキーマは変更しない。
JSとRubyは構文検査し、TSは型検査して型エラーをCEにする。
CRubyはベース配布物のRustでYJITを組み込み、実行時に`--yjit`で有効にする。
ビルド時にYJITの有効化を確認し、実機smokeではコードがJITコンパイルされることも検証する。
適用にはランタイムとworkerの再ビルド・配布が必要になる。
Haskellは固定パッケージDBでコンパイルし、提出時にCabalを実行しない。

スクリプトの拡張子を成果物にも保持する。
Node.jsとBun向けのTSでは、コンパイル結果のJSを成果物として保存する。
コンパイラ出力の親ディレクトリとファイルのシンボリックリンクを拒否する。
`node_modules`は読み取り専用でマウントする。
Denoのキャッシュは提出ごとのboxへ複製し、共有領域への書き込みを許可しない。
Bunの自動インストールは無効にする。

CPU、メモリ、ファイル数、ネットワークと秘密ファイルの隔離は現行条件を維持する。
VMのヒープ設定は提出用とinteractor用で分ける。
未知の言語をPythonとして扱うsmokeのフォールバックは廃止する。

実機には開発用パッケージを入れないため、Haskellがリンクに使うGSL、GLPK、BLAS、LAPACK、GMPの非バージョン付きライブラリ名も配布し、検索先を明示する。
ネイティブライブラリの実体は`native-library-sha256.json`でも照合し、通常ビルドで変更を検出したら停止する。
GEOSは`GEOS_LIBRARY_PATH`で固定配布先を指定する。
CRubyとTruffleRubyは固定gemディレクトリを`GEM_HOME`と`GEM_PATH`で直接参照し、提出ごとのBundler起動を省く。
TruffleRubyはJITを有効にした`latency`モードで実行する。
Node.js／Bun向けTypeScriptの型検査時だけファイル数上限を256とし、実行時の64は維持する。

出力上限は16 MiBのままとする。
処理系が`SIGXFSZ`や`EFBIG`を捕捉する場合にも超過を判定できるよう、OSのファイルサイズ制限には出力上限より1 KiBだけ余裕を設け、判定側は上限を超える1バイトまで読み取る。
上限ちょうどの出力を許容し、例外を捕捉しても超過が隠れないことを回帰テストで確認する。

## ビルド

```sh
python3 judge/runtime/prepare-runtime-inputs.py --additions
docker build -f judge/runtime/runtime-additions.Dockerfile -t openoj-runtime:additions judge
docker create --name openoj-additions-export openoj-runtime:additions
docker cp openoj-additions-export:/runtime.tar.gz judge/.build/runtime-additions.tar.gz
docker rm openoj-additions-export
bash judge/build-assets.sh
```

`RESOLVE=1`は依存更新時だけ使用する。
更新後のlockをレビューして保存し、通常ビルドで検証する。
ベースアーカイブはADR 0010適用版で、SHA-256は`4e28c974185c64330731c15d9a0db1504c91b6c322dec92b28ecaa4bc69adfdc`とする。

## 2026年9月18日の配置前検証

以下は最初の配布前に取得した記録である。
修正後の配布物と実環境の適用結果は[適用記録](runtime-rollout.md)に記載した。

実装コミットは`f6bb6d0`。
既存84,718ファイルの内容とシンボリックリンクがベース配布物から変わっていないことを確認した。
Python 56テスト、Goの関連テスト、Webの型検査・ビルド、Terraformの検証と8テストが成功した。
新処理系の構文・型検査と全対象ライブラリの代表操作をローカルで確認した。
Denoはisolateと同じファイルロック拒否条件でも成功し、Ruby・JS系はinteractor用のヒープ設定とファイル数上限でも検証した。
これらは実機smokeの代わりにはならず、公開判定はまだ行っていない。

| 配布物 | SHA-256 |
| --- | --- |
| `runtime-additions.tar.gz` | `67948063ce59af2da3b40b2ef3145ab2d6bf92e7d6c4ea30085ef18b9cec1b11` |
| `worker.tar.gz` | `1d68168d5b710e22a8c1772fcea18db18d03d94ac511192d9c5c0d4cb6c72e99` |

workerのスナップショット`judge-dev-before-language-additions-20260919`は、9月18日に作成して`available`を確認した。
現行Lambdaのコードと設定も非公開のローカル領域へ退避した。
worker上の古いADR 0007退避ツリーを整理し、現行ツリーを保持したまま約31 GiBの空きを確保した。

この時点のDBは`db.t4g.micro`で、適用待ちは`db.t4g.small`への変更のみだった。
DBだけを対象にしたTerraform planは追加・削除・置換がなく、PostgreSQL 17.9と20 GiB/gp3を維持する。
この時点では受付停止、DB変更、実機配置、公開は未実施だった。
配布物のS3転送は、自動承認レビューによる宛先確認を経てユーザーが明示的に許可した。
