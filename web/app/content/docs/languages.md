## 提出の基本 {#submission}

ソースコードは1ファイル、最大128 KiB（131,072バイト）です。 通常の問題では標準入力から入力を読み、標準出力へ解答を出力してください。 インタラクティブ問題では問題文の通信手順に従い、必要な箇所で出力をフラッシュしてください。

使える言語は、提出欄の「言語」に表示されているものです。 以下に各言語の実行環境の仕様をまとめます。 コンパイラや実行環境は、提出時に選んだものを使います。

「サンプル検証」ではサンプルケースだけを実行します。 すべての採点用ケースで判定するには「提出する」を押してください。

## 言語とバージョン {#languages}

言語IDは、[CLI](/docs/cli)の設定ファイルで`runtime`に書く値です。

| 言語 | バージョン | 言語ID |
| --- | --- | --- |
| C23 (GCC) | GCC 16.2.0 | `c23-gcc` |
| C23 (Clang) | Clang 23.1.1 | `c23-clang` |
| C++23 (GCC) | GCC 16.2.0 | `cpp23-gcc` |
| C++23 (Clang) | Clang 23.1.1 | `cpp23-clang` |
| Python (CPython 3.14) | CPython 3.14.7 | `python314` |
| Python (PyPy 3.11) | PyPy 7.3.23 / Python 3.11 | `pypy311` |
| Python (Codon 0.20) | Codon 0.20.0 | `codon020` |
| Rust (Edition 2024) | Rust 1.98.1 | `rust2024` |
| Java (Temurin 25) | Temurin 25.0.4.1+1 LTS | `java25` |
| C# 14 (.NET 10) | .NET 10.0.12 / SDK 10.0.401 | `csharp14` |
| Nim 2.2 | Nim 2.2.10 / GCC 16.2.0 | `nim22` |
| Go 1.27 | Go 1.27.1 | `go127` |
| Goose 2026.3 | Goose v2026.3 / GCC 16.2.0 | `goose20263` |
| Haskell (GHC 9.10 LTS) | GHC 9.10.3 / Stackage LTS 24系列 | `haskell-ghc910` |
| JavaScript (Node.js 24 LTS) | Node.js 24.21.0 LTS | `javascript-node24` |
| TypeScript (Node.js 24 LTS) | Node.js 24.21.0 LTS / TypeScript 7.0.2 | `typescript-node24` |
| JavaScript (Deno 2.9) | Deno 2.9.7 | `javascript-deno29` |
| TypeScript (Deno 2.9) | Deno 2.9.7 / TypeScript 6.0.3 | `typescript-deno29` |
| JavaScript (Bun 1.4) | Bun 1.4.2 | `javascript-bun14` |
| TypeScript (Bun 1.4) | Bun 1.4.2 / TypeScript 7.0.2 | `typescript-bun14` |
| Ruby (CRuby 4.0) | CRuby 4.0.7 | `ruby40` |
| Ruby (TruffleRuby 40) | TruffleRuby Community 40.0.0 | `ruby-truffle40` |
| cLay (20250308-1) | 20250308-1 / GCC 16.2.0 | `clay20250308` |
| dc (GNU dc 1.5.2) | GNU dc 1.5.2（bc 1.08.2同梱） | `dc152` |
| Nibbles (1.01+) | 1.01+ / GHC 9.10.3 | `nibbles101` |
| Brainfuck (Tritium) | Tritium（固定リビジョン `525d346c006e`） | `brainfuck-tritium` |
| Whitespace (whitespacers 1.3) | whitespacers 1.3.0 | `whitespace13` |
| Text (cat 9.4) | cat（GNU coreutils 9.4） | `text` |

CとC++は`main`関数、Rustは`fn main()`を含めてください。

Javaは`Main`クラスの`public static void main(String[] args)`から実行します。 C#は`static void Main()`またはトップレベルステートメント、Goは`package main`と`func main()`、Haskellは`main :: IO ()`を含めてください。 C#のusingはソースに記述してください。プロジェクトファイルやモジュール定義の提出は不要です。

Gooseは`fn main()`を含めてください。 標準入力は`import os;`で使える`read_stdin`や`read_line`で読み、インタラクティブ問題では`flush_stdout()`でフラッシュしてください。 整数の桁あふれは検査せずに折り返します。 使えるのは標準ライブラリだけで、gitから取得するパッケージには対応していません。

CPythonとPyPyは実行前に構文検査を行います。 Codonはネイティブコードへコンパイルする環境で、利用できるのはCodonが対応するPythonの機能です。 CPython連携やPyPIパッケージの追加には対応していません。

Nibblesはテキスト形式（`.nbl`）のソースを提出してください。
Whitespaceでは半角スペース、タブ、改行をそのまま保存します。
エディターのTabキーはタブを、Enterキーは自動字下げせず改行を入力します。
dcとBrainfuckには実行前の構文検査がなく、エラー時の判定は終了コードと出力に従います。
Brainfuckのセルは8ビットで、入出力はバイト単位、EOFは-1です。
Whitespaceの整数は多倍長です。
Nibbles、dc、Brainfuck、Whitespaceは、判定コードや生成コードに渡されるファイル引数を無視します。
これらの言語では標準入力を使ってください。

Textは書いた内容をそのまま標準出力に出し、標準入力は読みません。
output only問題の答えの提出に使います。
検証コード、対話のジャッジ、集計コード、入力・出力の生成と検証には使えません。

## コンパイルと実行 {#commands}

手元で試すときの目安です。 コンパイルが不要な言語は、実行コマンドだけを書いています。

- C23 (GCC)
  - コンパイル: `gcc -std=c23 -O2 -pipe main.c -lm -o main`
  - 実行: `./main`
- C23 (Clang)
  - コンパイル: `clang -std=c23 -O2 -pipe main.c -lm -o main`
  - 実行: `./main`
- C++23 (GCC)
  - コンパイル: `g++ -std=c++23 -O2 -pipe main.cpp -o main`
  - 実行: `./main`
- C++23 (Clang)
  - コンパイル: `clang++ -std=c++23 -O2 -pipe -stdlib=libstdc++ main.cpp -o main`
  - 実行: `./main`
- Python (CPython 3.14)
  - 構文検査: `python3.14 -I -B -c "import ast; ast.parse(open('main.py').read())"`
  - 実行: `python3.14 -I -B main.py`
- Python (PyPy 3.11)
  - 構文検査: `pypy3 -I -B -c "import ast; ast.parse(open('main.py').read())"`
  - 実行: `pypy3 -I -B main.py`
- Python (Codon 0.20)
  - コンパイル: `codon build -release main.py -o main`
  - 実行: `./main`
- Rust (Edition 2024)
  - コンパイル: `rustc --edition=2024 -C opt-level=2 main.rs -o main`
  - 実行: `./main`
- Java (Temurin 25)
  - コンパイル: `javac --release 25 -encoding UTF-8 -cp ac_library.jar -d classes Main.java`
  - 実行: `java -Xss1m -Xmx256m -XX:+UseSerialGC -cp classes:ac_library.jar Main`
- C# 14 (.NET 10)
  - コンパイル: `csc -langversion:14 -optimize+ -unsafe+ -target:exe -out:main.dll main.cs`
  - 実行: `dotnet main.dll`
- Nim 2.2
  - コンパイル: `nim cpp -d:release --opt:speed --mm:refc -o:main main.nim`
  - 実行: `./main`
- Go 1.27
  - コンパイル: `go build -mod=vendor -trimpath -o main main.go`
  - 実行: `./main`
- Goose 2026.3
  - コンパイル: `goose build --standalone -O2 main.goose -o main`
  - 実行: `./main`
- Haskell (GHC 9.10 LTS)
  - コンパイル: `ghc -O2 -threaded -rtsopts Main.hs -o main`
  - 実行: `./main +RTS -N1 -RTS`
- JavaScript (Node.js 24 LTS)
  - 構文検査: `node --check main.js`
  - 実行: `node main.js`
- TypeScript (Node.js 24 LTS)
  - コンパイル: `tsc main.ts --target ES2022 --module NodeNext --noEmitOnError --types node`
  - 実行: `node main.js`
- JavaScript (Deno 2.9)
  - 構文検査: `node --check main.js`
  - 実行: `deno run --cached-only --allow-read --allow-write --allow-env main.js`
- TypeScript (Deno 2.9)
  - 型検査: `deno check main.ts`
  - 実行: `deno run --no-check --cached-only --allow-read --allow-write --allow-env main.ts`
- JavaScript (Bun 1.4)
  - 構文検査: `node --check main.js`
  - 実行: `bun --no-install main.js`
- TypeScript (Bun 1.4)
  - コンパイル: `tsc main.ts --target ES2022 --module NodeNext --noEmitOnError --types bun`
  - 実行: `bun --no-install main.js`
- Ruby (CRuby 4.0)
  - 構文検査: `ruby -c main.rb`
  - 実行: `ruby --yjit main.rb`
- Ruby (TruffleRuby 40)
  - 構文検査: `truffleruby -c main.rb`
  - 実行: `truffleruby --engine.Mode=latency main.rb`
- cLay (20250308-1)
  - コンパイル: `clay < main.clay > main.cpp && g++ -std=c++23 -O2 -pipe main.cpp -o main`
  - 実行: `./main`
- dc (GNU dc 1.5.2)
  - 実行: `dc -f main.dc`
- Nibbles (1.01+)
  - コンパイル: `nibbles -hs main.nbl && ghc -O out.hs -o main`
  - 実行: `./main`
- Brainfuck (Tritium)
  - 実行: `tritium -b -B -e main.bf`
- Whitespace (whitespacers 1.3)
  - 構文検査: `wsc --dump main.dump main.ws`
  - 実行: `wsc main.ws`
- Text (cat 9.4)
  - 実行: `cat main.txt`

:::details ジャッジが実行する内部コマンド
ジャッジは隔離環境でこれらのコマンドを実行し、ソースを`/box/`に置きます。 環境変数は省略しています。

- C23 (GCC)
  - コンパイル: `/opt/judge-runtimes/gcc/bin/gcc -std=c23 -O2 -pipe /box/main.c -lm -Wl,-rpath,/opt/judge-runtimes/gcc/lib64 -o /box/main`
  - 実行: `/box/main`
- C23 (Clang)
  - コンパイル: `/opt/judge-runtimes/llvm/bin/clang -std=c23 -O2 -pipe /box/main.c --gcc-toolchain=/opt/judge-runtimes/gcc -lm -Wl,-rpath,/opt/judge-runtimes/gcc/lib64 -o /box/main`
  - 実行: `/box/main`
- C++23 (GCC)
  - コンパイル: `/opt/judge-runtimes/gcc/bin/g++ -std=c++23 -O2 -pipe /box/main.cpp -I/opt/judge-runtimes/include -Wl,-rpath,/opt/judge-runtimes/gcc/lib64 -o /box/main`
  - 実行: `/box/main`
- C++23 (Clang)
  - コンパイル: `/opt/judge-runtimes/llvm/bin/clang++ -std=c++23 -O2 -pipe /box/main.cpp --gcc-toolchain=/opt/judge-runtimes/gcc -stdlib=libstdc++ -I/opt/judge-runtimes/include -Wl,-rpath,/opt/judge-runtimes/gcc/lib64 -o /box/main`
  - 実行: `/box/main`
- Python (CPython 3.14)
  - 構文検査: `/opt/judge-runtimes/python/bin/python3.14 -I -B -c "import ast; ast.parse(open('/box/main.py').read())"`
  - 実行: `/opt/judge-runtimes/python/bin/python3.14 -I -B /box/main`
- Python (PyPy 3.11)
  - 構文検査: `/opt/judge-runtimes/pypy/bin/pypy3 -I -B -c "import ast; ast.parse(open('/box/main.py').read())"`
  - 実行: `/opt/judge-runtimes/pypy/bin/pypy3 -I -B /box/main`
- Python (Codon 0.20)
  - コンパイル: `/opt/judge-runtimes/codon/bin/codon build -release /box/main.py -o /box/main`
  - 実行: `/box/main`
- Rust (Edition 2024)
  - コンパイル: `/opt/judge-runtimes/rust/bin/rustc --edition=2024 -C opt-level=2 -C linker=/usr/bin/gcc @/opt/judge-runtimes/rust/extern.args /box/main.rs -o /box/main`
  - 実行: `/box/main`
- Java (Temurin 25)
  - コンパイル: `/opt/judge-runtimes/java25/bin/javac -J-Xmx512m -J-XX:ActiveProcessorCount=1 -J-XX:-UsePerfData --release 25 -encoding UTF-8 -cp /opt/judge-runtimes/java-libs/ac_library.jar -d /box/classes /box/Main.java`
  - 実行: `/opt/judge-runtimes/java25/bin/java -Xms16m -Xmx256m -Xss1m -XX:MaxMetaspaceSize=96m -XX:ReservedCodeCacheSize=32m -XX:MaxDirectMemorySize=32m -XX:+UseSerialGC -XX:ActiveProcessorCount=1 -XX:+ExitOnOutOfMemoryError -XX:-UsePerfData -cp /box/main:/opt/judge-runtimes/java-libs/ac_library.jar Main`
- C# 14 (.NET 10)
  - コンパイル: `/opt/judge-runtimes/dotnet/dotnet /opt/judge-runtimes/csharp-tools/csc.dll @/opt/judge-runtimes/csharp-tools/compile.args`
  - 実行: `/opt/judge-runtimes/dotnet/dotnet /box/main.dll`
- Nim 2.2
  - コンパイル: `/opt/judge-runtimes/nim/bin/nim cpp -d:release --opt:speed --mm:refc --parallelBuild:1 --hints:off --skipUserCfg:on --skipParentCfg:on --skipProjCfg:on --noNimblePath --nimcache:/box/nimcache -o:/box/main /box/main.nim`
  - 実行: `/box/main`
- Go 1.27
  - コンパイル: `/opt/judge-runtimes/go/bin/go -C /box build -buildmode=exe -mod=vendor -p=1 -trimpath -buildvcs=false -o /box/main /box/main.go`
  - 実行: `/box/main`
- Goose 2026.3
  - コンパイル: `/opt/judge-runtimes/goose/goose build --standalone --cc /opt/judge-runtimes/gcc/bin/gcc -O2 /box/main.goose -o /box/main`
  - 実行: `/box/main`
- Haskell (GHC 9.10 LTS)
  - コンパイル: `/opt/judge-runtimes/ghc/bin/ghc -O2 -threaded -rtsopts -j1 -L/opt/judge-runtimes/addition-native/lib @/opt/judge-runtimes/haskell-deps/compile.args /box/Main.hs -o /box/main`
  - 実行: `/box/main +RTS -N1 -RTS`
- JavaScript (Node.js 24 LTS)
  - 構文検査: `/opt/judge-runtimes/node/bin/node --check /box/main.js`
  - 実行: `/opt/judge-runtimes/node/bin/node /box/main.js`
- TypeScript (Node.js 24 LTS)
  - コンパイル: `/opt/judge-runtimes/node/bin/node /opt/judge-runtimes/js-deps/node_modules/typescript/bin/tsc /box/main.ts --target ES2022 --module NodeNext --moduleResolution NodeNext --skipLibCheck --noEmitOnError --pretty false --outDir /box/compiled --types node`
  - 実行: `/opt/judge-runtimes/node/bin/node /box/main.js`
- JavaScript (Deno 2.9)
  - 構文検査: `/opt/judge-runtimes/node/bin/node --check /box/main.js`
  - 実行: `/opt/judge-runtimes/deno/deno run --no-check --cached-only --no-prompt --allow-read=/box,/opt/judge-runtimes --allow-write=/box --allow-env --config=/opt/judge-runtimes/deno-deps/deno.json /box/main.js`
- TypeScript (Deno 2.9)
  - 型検査: `/opt/judge-runtimes/deno/deno check --cached-only --config=/opt/judge-runtimes/deno-deps/deno.json /box/main.ts`
  - 実行: `/opt/judge-runtimes/deno/deno run --no-check --cached-only --no-prompt --allow-read=/box,/opt/judge-runtimes --allow-write=/box --allow-env --config=/opt/judge-runtimes/deno-deps/deno.json /box/main.ts`
- JavaScript (Bun 1.4)
  - 構文検査: `/opt/judge-runtimes/node/bin/node --check /box/main.js`
  - 実行: `/opt/judge-runtimes/bun/bun --no-install /box/main.js`
- TypeScript (Bun 1.4)
  - コンパイル: `/opt/judge-runtimes/node/bin/node /opt/judge-runtimes/js-deps/node_modules/typescript/bin/tsc /box/main.ts --target ES2022 --module NodeNext --moduleResolution NodeNext --skipLibCheck --noEmitOnError --pretty false --outDir /box/compiled --types bun`
  - 実行: `/opt/judge-runtimes/bun/bun --no-install /box/main.js`
- Ruby (CRuby 4.0)
  - 構文検査: `/opt/judge-runtimes/ruby/bin/ruby -c /box/main.rb`
  - 実行: `/opt/judge-runtimes/ruby/bin/ruby --yjit /box/main.rb`
- Ruby (TruffleRuby 40)
  - 構文検査: `/opt/judge-runtimes/truffleruby/bin/ruby -c /box/main.rb`
  - 実行: `/opt/judge-runtimes/truffleruby/bin/ruby --engine.Mode=latency /box/main.rb`
- cLay (20250308-1)
  - コンパイル: `/usr/bin/sh /opt/judge-runtimes/golf/compile-clay.sh`
  - 実行: `/box/main`
- dc (GNU dc 1.5.2)
  - 実行: `/usr/bin/sh -c 'exec /opt/judge-runtimes/golf/dc -f /box/main.dc'`
- Nibbles (1.01+)
  - コンパイル: `/usr/bin/sh /opt/judge-runtimes/golf/compile-nibbles.sh`
  - 実行: `/usr/bin/sh -c 'exec /box/main'`
- Brainfuck (Tritium)
  - 実行: `/usr/bin/sh -c 'exec /opt/judge-runtimes/golf/tritium -b -B -e /box/main.bf'`
- Whitespace (whitespacers 1.3)
  - 構文検査: `/opt/judge-runtimes/golf/wsc --dump /box/main.dump /box/main.ws`
  - 実行: `/usr/bin/sh -c 'exec /opt/judge-runtimes/golf/wsc /box/main.ws'`
- Text (cat 9.4)
  - 実行: `/usr/bin/sh -c 'exec /usr/bin/cat /box/main.txt'`
:::

## 追加ライブラリ {#libraries}

各言語の標準ライブラリに加え、次のライブラリを用意しています。 提出時にパッケージをインストールすることはできません。

### C++23（GCC・Clang共通）とcLay

| ライブラリ | バージョン |
| --- | --- |
| AtCoder Library | 1.6 |
| Boost（ヘッダーのみ） | 1.92.0 |
| testlib | 固定リビジョン `1e4e8a24c79c` |

cLayもC++23と同じヘッダーを利用できます。
ACLは`#include <atcoder/all>`などで読み込めます。 別途リンクが必要なBoostライブラリは対象外です。

testlibを使う判定コードの書き方は、[判定方法の設定](/docs/checkers#testlib)を確認してください。

### CPython・PyPy

| ライブラリ | バージョン | 対応環境 |
| --- | --- | --- |
| more-itertools | 11.1.0 | CPython・PyPy |
| sortedcontainers | 2.4.0 | CPython・PyPy |
| ac-library-python | 固定リビジョン `27fdbb71cd0d` | CPython・PyPy |
| NumPy | 2.5.3 | CPythonのみ |
| SciPy | 1.18.1 | CPythonのみ |

ac-library-pythonは`atcoder`からインポートします。

### Rust

| ライブラリ | バージョン |
| --- | --- |
| ac-library-rs | 0.2.0 |
| fixedbitset | 0.5.7 |
| itertools | 0.15.0 |
| num | 0.4.3 |
| proconio | 0.6.0（derive有効） |
| rand | 0.10.2 |
| rustc-hash | 2.1.3 |

依存関係は設定済みで、`Cargo.toml`の提出は不要です。 たとえば`use ac_library::Dsu;`や`use proconio::input;`で読み込めます。

### C# 14

| ライブラリ | バージョン | 読み込み例 |
| --- | --- | --- |
| MathNet.Numerics | `5.0.0` | `using MathNet.Numerics.LinearAlgebra;` |
| ac-library-csharp | `4.1.4` | `using AtCoder;` |

ACLのUnion-Findは`new Dsu(n)`で作成します。MathNet.Numericsはmanaged providerを使います。

### Java 25

| ライブラリ | バージョン | 読み込み例 |
| --- | --- | --- |
| ac-library-java | `2.0.0` | `import ac_library.DSU;` |

### Nim 2.2

| ライブラリ | バージョン | 読み込み例 |
| --- | --- | --- |
| AC Library | `v1.5.1` | `<atcoder/dsu>` |
| Arraymancer | `84af537af1bc1f90229fff2b90abf5e5c1b02616` | `import arraymancer` |
| Eigen | `3.4.0` | `<Eigen/Dense>` |
| Nim-ACL | `0.1.0` | `import atcoder/dsu` |
| bigints | `ca00f6da386af9ad7e3abf603c0201da6a014477` | `import bigints` |
| bignum_chaemon | `1.0.6` | `import bignum` |
| boost | `1.88.0` | `<boost/multiprecision/cpp_int.hpp>` |
| fftw | `3.3.10` | `<fftw3.h>` |
| gmp | `6.3.0` | `import gmp` |
| mpfr | `4.2.1` | `<mpfr.h>` |
| neo | `0.3.5` | `import neo` |
| nimsimd | `1.3.2` | `import nimsimd/sse2` |
| regex | `0.26.3` | `import regex` |
| sat | `faf1617f44d7632ee9601ebc13887644925dcc01` | `import sat/sat` |

Nimのメモリ管理は`refc`です。bignumとそのGMPバインディングにはNim 2.2 / refc用の互換パッチを適用しています。 CPU向け機能を提供し、GPUは使用できません。SIMDは実行ホストの対応命令を確認してください。 山括弧の項目はC++ヘッダーで、NimのC++連携から使用します。GMPなどのネイティブライブラリは必要に応じて`{.passL: "-lgmp".}`のようにリンクしてください。

### Go 1.27

| ライブラリ | バージョン | 読み込み例 |
| --- | --- | --- |
| ac-library-go | `v0.0.0-20260106091915-2caa314afb5a` | `github.com/monkukui/ac-library-go/dsu` |
| gods | `v1.18.1` | `github.com/emirpasic/gods/sets/treeset` |
| golang_org_x_exp | `v0.0.0-20260908205506-85c1c2202aba` | `golang.org/x/exp/slices` |
| gonum | `v0.17.0` | `gonum.org/v1/gonum/mat` |
| gostl | `v1.2.0` | `github.com/liyue201/gostl/ds/queue` |
| immutable | `v0.4.3` | `github.com/benbjohnson/immutable` |

Goは上記のパッケージを`import`して使用します。モジュール定義は設定済みで、提出時のダウンロードとCGOは無効です。

### Haskell

| ライブラリ | バージョン |
| --- | --- |
| ac-library-hs | 1.5.3.2 |
| adjunctions | 4.4.4 |
| attoparsec | 0.14.4 |
| bifunctors | 5.6.3 |
| bitvec | 1.1.6.0 |
| comonad | 5.0.10 |
| contravariant | 1.5.6 |
| distributive | 0.6.3 |
| extra | 1.8.1 |
| fgl | 5.8.3.1 |
| flow | 2.0.0.11 |
| free | 5.2 |
| hashable | 1.5.1.0 |
| heaps | 0.4.1 |
| hmatrix | 0.20.2 |
| hmatrix-glpk | 0.19.0.0 |
| hmatrix-gsl | 0.19.0.1 |
| hmatrix-special | 0.19.0.0 |
| ilist | 0.3.1.0 |
| indexed-traversable | 0.1.5 |
| indexed-traversable-instances | 0.1.2.1 |
| integer-logarithms | 1.0.5 |
| kan-extensions | 5.2.8 |
| lens | 5.3.6 |
| linear-base | 0.8.1 |
| list-t | 1.0.5.7 |
| massiv | 1.0.5.0 |
| megaparsec | 9.8.2 |
| monad-memo | 0.5.4 |
| mono-traversable | 1.0.21.0 |
| mutable-containers | 0.3.4.1 |
| mwc-random | 0.15.3.0 |
| parallel | 3.3.0.0 |
| parser-combinators | 1.3.1 |
| primitive | 0.9.1.0 |
| profunctors | 5.6.3 |
| psqueues | 0.2.8.3 |
| QuickCheck | 2.19.0.0 |
| random | 1.3.1 |
| reflection | 2.1.9 |
| regex-tdfa | 1.3.2.6 |
| safe-exceptions | 0.1.7.4 |
| scientific | 0.3.8.1 |
| semialign | 1.4 |
| semigroupoids | 6.0.2 |
| split | 0.2.5.1 |
| strict | 0.5.1 |
| strict-lens | 0.4.1 |
| tagged | 0.8.10 |
| tasty | 1.5.4 |
| tf-random | 0.5 |
| these | 1.2.1 |
| these-lens | 1.0.2 |
| trifecta | 2.1.4 |
| unboxing-vector | 0.2.0.0 |
| unordered-containers | 0.2.21 |
| utility-ht | 0.0.17.2 |
| vector | 0.13.2.0 |
| vector-algorithms | 0.9.1.0 |
| vector-split | 1.0.0.4 |
| vector-stream | 0.1.0.1 |
| vector-th-unbox | 0.2.2 |
| wide-word | 0.1.9.0 |
| witherable | 0.5 |

GHC付属のパッケージ（`containers`、`bytestring`、`text`など）はGHC 9.10.3の版を使えます。 依存関係は設定済みで、`.cabal`ファイルの提出は不要です。 たとえば`import AtCoder.Dsu`や`import qualified Data.Vector.Unboxed as VU`で読み込めます。

### JavaScriptとTypeScript

Node.jsとBunでは`node:fs`などのNode互換API、Denoでは`Deno.stdin`などのAPIを利用できます。
Node.jsのJavaScriptは`require`と`import`の両方に対応します。
TypeScriptは型エラーをCEとして扱います。
Node.jsとBun向けのTypeScriptは、`NodeNext`形式でJavaScriptにコンパイルします。

| パッケージ | バージョン |
| --- | --- |
| ac-library-js | 0.1.1 |
| data-structure-typed | 2.6.4 |
| immutable | 5.1.9 |
| lodash | 4.18.1 |
| mathjs | 15.2.0 |
| tstl | 3.0.0 |

たとえば`import { DSU } from "ac-library-js"`でACLを利用できます。
Denoでは一覧にある`@std/*`も固定しており、`import { assertEquals } from "@std/assert"`のように読み込みます。
`@std/encoding/base64`などの公開サブパスもキャッシュしています。
提出時の依存ダウンロードや自動インストールはできません。
型定義は`@types/node`、`@types/lodash`を用意し、Bun向けには`@types/bun`も使用します。

### Ruby

| ライブラリ | バージョン | 対応環境 |
| --- | --- | --- |
| ac-library-rb | 1.2.0 | CRuby・TruffleRuby |
| bit_utils | 0.1.2 | CRuby・TruffleRuby |
| bitarray | 1.3.2 | CRuby・TruffleRuby |
| faster_prime | 1.0.2 | CRuby・TruffleRuby |
| ffi-geos | 2.5.0 | CRuby・TruffleRuby |
| immutable-ruby | 0.2.0 | CRuby・TruffleRuby |
| rbtree | 0.4.7 | CRuby・TruffleRuby |
| rgl | 0.6.7 | CRuby・TruffleRuby |
| sorted_containers | 1.1.0 | CRuby・TruffleRuby |
| sorted_set | 1.1.0 | CRuby・TruffleRuby |
| z3 | 0.0.20260727 | CRuby・TruffleRuby |
| fast_trie | 0.5.1 | CRubyのみ |
| numo-linalg | 0.1.7 | CRubyのみ |
| numo-narray | 0.9.2.1 | CRubyのみ |
| numo-openblas | 0.5.5 | CRubyのみ |
| or-tools | 0.18.0 | CRubyのみ |
| polars-df | 0.27.1 | CRubyのみ |

ACLのUnion-Findは`require "ac-library-rb/dsu"`で読み込み、`AcLibraryRb::DSU.new(n)`で作成します。

インタラクティブ問題では、`$stdout.sync = true`または`STDOUT.flush`で出力をフラッシュしてください。 判定コードには入力ファイルなどの引数が渡されるため、引数なしの`gets`は標準入力ではなくそのファイルを読みます。 判定コードで標準入力を読むときは`STDIN.gets`を使ってください。

## 実行制限 {#limits}

各テストケースの実行時間制限は問題ページで確認してください。 採点ではプログラムと子プロセスの合計CPU時間を計測し、経過時間にも別の上限を設けています。

メモリ上限は512 MiBで、実行環境やライブラリが使うメモリも含みます。 コンパイル処理は別枠で、CPU時間30秒、経過時間40秒、メモリ1 GiBが上限です。

NumPyやSciPyが使う数値計算ライブラリの内部スレッド数は1に設定しています。 実行中の外部ネットワーク接続は利用できません。

コンテストでの得点や誤答ペナルティについては、[コンテストのルール](/docs/contest-rules)を確認してください。
