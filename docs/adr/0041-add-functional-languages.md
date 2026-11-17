# ADR 0041: Common Lisp、F#、Gleam、Fixを加える

- 状態：Accepted（未配布、実機smoke未実施）
- 決定日：2026-11-15
- 範囲：4言語の処理系と版、コンパイルと実行の方法、ライブラリの選び方、C#とF#の.NETのW^Xの無効化。設計は[関数型言語4つの追加の設計](../superpowers/specs/2026-11-15-functional-languages-design.md)、構成は[追加環境の構成](../judge/runtime-additions.md#関数型言語4つの追加)にある

## 背景

ShareOJで使える関数型言語は、Haskellだけである。
利用者の要望で、Common Lisp、F#、Gleam、Fixを加える。
特定のコンテストに合わせた期限はない。

AtCoderの2025年10月の言語一覧には4言語とも入っていて、言語ごとにライブラリがある。
ShareOJの提出はisolateの中で動き、プロセス数は64（対話では32）、開けるファイル数は64、出力のファイルサイズは16 MiBまでである。
コンパイルはCPU 30秒、経過40秒、1 GiBまでで、ネットワークは使えない。
4言語の処理系とライブラリを、この制限の中で動かす必要がある。

## 決定

### 利用者と決めたこと

- 加える言語は、Common Lisp、F#、Gleam、Fix（[fixlang](https://github.com/tttmmmyyyy/fixlang)）の4つとする。
- GleamはErlang（BEAM）の上で動かす。
- Gleamの標準入力は、AtCoderと同じ`stdin`と`gleam_yielder`で読めるようにする。
  `gleam_stdlib`、`stdin`、`gleam_yielder`が入らなければ、Gleamは公開しない。
- ライブラリは、AtCoderの2025年10月の一覧と同等のものを、ジャッジの運用に影響しない範囲で入れる。
- C#にも、F#と同じくW^Xを止める指定を加える（2026-11-15の依頼）。

### 処理系と版

構築した日（2026-11-15）の最新の安定版を使う。

| 公開ID | 表示名 | 処理系 |
| --- | --- | --- |
| `commonlisp-sbcl26` | Common Lisp (SBCL 2.6) | SBCL 2.6.10 |
| `fsharp10` | F# 10 (.NET 10) | .NET SDK 10.0.401のF# 10（fsc 15.2.401）。C#と同じ.NET 10.0.12で動かす |
| `gleam119` | Gleam 1.19 (Erlang/OTP 29) | Gleam 1.19.2、Erlang/OTP 29.1.2 |
| `fix16` | Fix 1.6 | Fix 1.6.1 |

- F#だけは最新のF# 11を使わない。
  F# 11のコンパイラは.NET 11を必要とし、C#が使う.NETの版は変えないからである。
- 4言語は配布済みのランタイム（Gooseを含む`runtime-goose.tar.gz`）へ足し、既存の処理系はビルドし直さない。
- 処理系の配布物はURLとSHA-256で、ライブラリはロックファイルで固定する。

### コンパイルと実行

| 言語 | コンパイル | 実行 | 成果物 |
| --- | --- | --- | --- |
| Common Lisp | ライブラリを読み込んで保存したSBCLのイメージで`compile-file`する | 同じイメージでfaslを読み込む | fasl 1個 |
| F# | `fsc.dll`を`dotnet`で直接起動する。MSBuildは使わない | C#と同じ`dotnet main.dll` | `main.dll`。ライブラリのDLLは運用側の固定ファイルから置く |
| Gleam | `gleam compile-package`で、提出のモジュールだけをコンパイルする。依存はコンパイル済みのひな形をその場で読む | `erl`を直接起動する | BEAMファイル1個 |
| Fix | 依存を取得済みのひな形をボックスへ複製し、`fix build -O max`を実行する。リンクは既存のGCC 16で行う | ネイティブの実行ファイル | 実行ファイル1個 |

- 成果物の取り出しは既存の種類（`native`、`dotnet`、`generated-source`）を使う。
- コンパイルの上限は、どの言語でも変えない。

### Gleamは`gleam build`を使わずにコンパイルする

`gleam build`は、ビルドのディレクトリを`flock`でロックする。
isolateは、サンドボックスの間の抜け道になり得るファイルロックを拒み、許すのはGoのコンパイルだけである（[ADR 0010](0010-extend-judge-languages-and-libraries.md)）。
最初の実機smoke（2026-11-18）では、`gleam build`がこのロックで失敗し、GleamだけがCEになった。
そこで、ロックを取らない`gleam compile-package`に替えた。
Gleamのためにファイルロックの制限を外すことはしない。

### Gleamは`erl`を直接起動する

`gleam run`は、実行のたびにビルドを確かめ直す。
そこで、コンパイル済みのBEAMファイルとひな形の`ebin`を`erl`に渡し、gleamが生成する入口のモジュールから始める。

Erlangは既定の指定のままでは、isolateの制限の中で起動しない。
次のように指定する。
値と測定は[追加環境の構成](../judge/runtime-additions.md#gleam)にある。

- スケジューラーと非同期スレッドを1本ずつにする（`+S 1:1 +SDcpu 1:1 +SDio 1 +A 1`）。
  既定ではCPUの数だけスレッドを作り、64プロセスの上限で起動に失敗する。
- JITのコードを二重に写像しない（`+JMsingle true`）。
  既定ではファイルサイズの上限より大きいmemfdを作り、起動時に`SIGXFSZ`で止まる。
- busy waitをやめ、分散などを起動しない`-mode minimal`で起動し、依存のBEAMファイルを1つのディレクトリにまとめる。
  起動が速くなる。
- プロセス表を$2^{18}$にする（`+P 262144`）。
  既定の$2^{20}$の表は常駐し、Hello Worldの使用量を増やす。
- 解放したメモリのセグメントの保持を、10個から1個にする（`+MMmcs 1`）。
  伸び続けるヒープの使用量のピークが約半分になる。
  作っては捨てる処理は最大で約10%遅くなるが、メモリ上限が64〜512 MiBなので使用量を優先した。

何もしないプログラムの起動のCPU時間は、スケジューラーの指定と`+JMsingle true`だけの268 msから、147 msになった（ローカルの値）。

### ライブラリは4つの条件で外す

AtCoderの一覧を出発点にし、次のどれかに当たるライブラリを外す。

1. イメージにないネイティブの共有ライブラリを新しく必要とする。
2. 固定したURLとSHA-256、またはロックファイルで再現できる形で取得できない。
3. 入れると、Hello Worldのコンパイルが実機で10秒を超える。または、Hello Worldが64 MiBのメモリ上限で動かなくなる。
4. ほかのライブラリと版が解決できない。

- 条件3は配布の前に実機で測れない。
  ローカルのコンテナをCPU 1個に制限して測り、半分の5秒を基準にした。
- Fixのregexpは、Hello Worldには影響しないが外した。
  regexpを使うプログラムのコンパイルが、ローカルでCPU 29.5秒かかり、上限の30秒に届くからである。
  条件3を、Hello World以外のプログラムへ広げて当てはめた。
- 版は、互いに解決できる最新とする。
  Gleamの`gleam_stdlib`は1.xではなく0.71.0にした。
  必須の`stdin` 2.0.2が、1.0未満の`gleam_stdlib`にしか対応しないからである。

| 言語 | 外したライブラリ | 理由 |
| --- | --- | --- |
| Common Lisp | fast-generic-functions | SBCL 2.6.10で消えた内部関数を参照し、読み込めない。原因はSBCLとの非互換で、`excluded.json`では最も近い条件4に分けた |
| Common Lisp | sb-mpfr | libmpfrがイメージにない（条件1） |
| Fix | regexp | 使うプログラムのコンパイルが上限に届く（条件3に準じる） |

F#とGleamで外したライブラリはない。

### .NETのW^Xを止める

C#とF#は、`DOTNET_EnableWriteXorExecute=0`を指定する。
W^Xが有効だと、.NETがJITのコードに使える大きさは、isolateのファイルサイズの上限（コンパイル時32 MiB、実行時16 MiB）までになる。
F#のコンパイラは、Hello Worldで21〜22 MiB、100行のプログラムで27〜28 MiBを使い、超えると落ちてCEに見える。
C#のコンパイラも、Hello Worldで17〜20 MiBを使う。

W^Xは、.NETのプロセスを外からの攻撃から守る仕組みである。
isolateの中で動くのは提出のプログラムそのもので、隔離はisolateが受け持つ。
AtCoderも同じ指定をしている。
既存の言語の`compile`、`run`、`env`を変えたのは、このC#の1つだけである。

## 検討した選択肢

### GleamをJavaScriptへ変換して動かす

整数が$2^{53}$を超えると精度が落ちるので、採らない。

### Gleamを`gleam run`で実行する

実行のたびにビルドを確かめ直すので、採らない。

### AtCoderと同じ版にそろえる

AtCoderの一覧は2025年10月の版で、構築した時点ですでに新しい版が出ている。
設計の段階で、処理系もライブラリも構築した日の最新の安定版を使うと決めた。
ただし、F#は.NETの版に縛られる。
Gleamの3つのライブラリ（`gleam_stdlib`、`ieee_float`、`gleam_community_maths`）も、`stdin`に合わせて最新より前の版になる。

### F#を`dotnet publish`でコンパイルする

AtCoderの方法で、設計では`fsc.dll`の直接起動が動かないときの代わりにしていた。
直接起動が動いたので、C#と同じく、提出ごとのrestoreとMSBuildの評価を避けた。

### Fixの最適化を`-O basic`に下げる

regexpを使うプログラムは12.4秒でコンパイルできるようになるが、実行が約4倍遅くなる。
設計では、Hello Worldのコンパイルが基準を超えるときだけ最適化を下げる。
Hello Worldのコンパイルは、ライブラリを含めてローカルで約1.3秒である。

### Fixのregexpを入れる

Hello Worldでは条件3に当たらない。
しかし、使うプログラムは実質CEになり、そのたびにワーカーのコンパイルの枠を30秒使い切る。

### C#のファイルサイズの上限を上げる

isolateの`--fsize`は、提出が書くファイルの上限も兼ねる。
F#と同じW^Xの指定で足りるので、上限は変えない。

## 影響

### 利用者

- 実機smokeに合格した言語から、提出、判定用コード、対話用ジャッジに使える。
- Gleamは、実行のたびにErlangの起動で約0.15秒のCPU時間を使う（ローカルの値）。
  1行ずつの入出力も遅いので、速い書き方を「使える言語」のページに書いた。
- F#は、Hello Worldでもコンパイルに約2.5秒かかる（ローカルの値）。
  C#と同じくGCのヒープの上限があり、超えるとMLEではなくREになる。
- Fixは、minilib-json、minilib-xml、minilib-appを使うとコンパイルが5.6〜8.8秒になる（ローカルの値）。
- C#のコンパイルと実行は、JITのコードがファイルサイズの上限に縛られなくなる。
  ローカルのコンテナでは、C#の6つのfixtureが合格した。

### 運用

- ランタイムの展開後の大きさが、約590 MB増える。
- `finish`が、libtinfo.so.6（Erlang）とlibzstd.so.1（Fix）を`addition-native/lib`へ加える。
  `LD_LIBRARY_PATH`でこのディレクトリを使う既存の言語も、これらを使うときはホストの代わりにこの複製を読み込む。
- C#の環境変数が変わるので、次の配布でruntime digestが変わる。
  4言語だけでなく、C#の実機smokeの合格も確かめる。
- ジョブの形式もDBも変わらないので、配布は`judge/rollout.py rolling`で行う。
  実機への配布と公開は、利用者の了承を得てから行う。
- 依存を更新したときは、解決したロックを手で`deps/additions`へ戻す。

### 未決事項

- 実機の値。
  Hello Worldのコンパイルが実機で10秒を超えた言語は公開しない。
- Fixの実行時の検査（配列の範囲など）を`--no-runtime-check`で外すか。
  いまは有効で、失敗はREになる。
- Fixのコード生成のスレッド数。
  巨大なプログラムではプロセス数の上限に届き、CEになり得る。
- F#が、ケースごとにライブラリのDLL（20.7 MB）をボックスへ複製すること。
- 問題文のコードブロックの色分け。
  highlight.jsにGleamとFixの文法がないので、4言語とも加えていない。

## 関連資料

- [関数型言語4つの追加の設計](../superpowers/specs/2026-11-15-functional-languages-design.md)
- [追加環境の構成](../judge/runtime-additions.md#関数型言語4つの追加)
- [ジャッジのランタイム方針](../judge/runtime-policy.md)
- [ADR 0007: 競技プログラミング向けのランタイム構成を限定する](0007-limit-judge-runtime-libraries.md)
- [ADR 0010: testlibとC#、Java 25、Nim、Goを採点環境へ追加する](0010-extend-judge-languages-and-libraries.md)
- [AtCoderの2025年10月の言語一覧](https://img.atcoder.jp/file/language-update/2025-10/language-list.html)
