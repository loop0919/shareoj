# testlibと追加言語の実装計画

[ADR 0010](../adr/0010-extend-judge-languages-and-libraries.md)の実装候補と受入条件を記録する。
2026-09-14に実装・配布・実機検証を完了し、12言語を公開した。
採用値は以下に、全14ランタイムの実機smokeと本番APIの検証提出16件の結果は[適用記録](runtime-rollout.md)に記録する。
要求で指定された版と、互換性試験後に選ぶ版を区別する。

## コンパイラと依存の固定

| 対象 | 採用候補または要求 | 確定方法 |
| --- | --- | --- |
| testlib | `MikeMirzayanov/testlib` | upstreamのcommitと`testlib.h`のSHA-256を固定 |
| C# | C# 14 / .NET 10 LTS | SDK、Roslyn、参照アセンブリ、実行ランタイムを一組として固定 |
| MathNet.Numerics | 5.0.0を検証の出発点とする | managed providerで.NET 10との互換性を確認 |
| ac-library-csharp | `kzrnm/ac-library-csharp`の安定版 | NuGetのpackage ID、version、推移的依存を固定 |
| Java 25 | Eclipse Temurin 25 LTS | Linux x64の保守中パッチ版と配布物SHA-256を固定 |
| ac-library-java | `ocha98/ac-library-java` 2.0.0 | 既存の`ac_library23.jar`をJava 25で検証。必要なら変更理由を記録 |
| Nim | 2.2系の安定パッチ | 要求依存の互換性試験で選定。2.2.4を比較基準とする |
| Go | 公式gcの保守中安定系列 | 6モジュールの`go`要件と実機コンパイル上限を満たす版を固定 |

MathNet.Numericsの提供機能は[公式資料](https://numerics.mathdotnet.com/)を参照する。
ac-library-csharpの`-atcoder`版はSourceExpanderを省く目的の配布であり、通常安定版と同一版扱いにはしない。
まず通常安定版を検証し、ビルド時のSourceExpander等を実行環境に不要な依存として整理する。
特殊な版を採用する必要があれば、[upstreamの説明](https://github.com/kzrnm/ac-library-csharp)を踏まえて正確な版と理由を記録する。

## 採用する固定値

| 対象 | 採用値 | 記録先 |
| --- | --- | --- |
| testlib | `1e4e8a24c79c6bad3becbdb5a332ffc352b7d5dd` | `runtime-extension-sources.lock.json` |
| .NET | SDK 10.0.401、ランタイム10.0.12、C# 14 | 同上、参照DLLとRoslynも同じSDKから取得 |
| NuGet | MathNet.Numerics 5.0.0、ac-library-csharp 4.1.4 | `judge/runtime/deps/dotnet/packages.lock.json` |
| Java | Temurin 25.0.4.1+1 LTS、ac-library-java 2.0.0 | 新JDKは追加lock、JARは既存lock |
| Nim | 2.2.10、refc、GCC 16.2.0、x86-64汎用命令 | `runtimes.py`、固定nim.cfgと互換パッチ |
| Go | 1.27.1 | 追加lock、`judge/runtime/deps/go/go.mod`と`go.sum` |

Goの採用値はimmutable 0.4.3、gods 1.18.1、gostl 1.2.0、gonum 0.17.0である。
ac-library-goは`v0.0.0-20260106091915-2caa314afb5a`、x/expは`v0.0.0-20260908205506-85c1c2202aba`へ固定する。
標準パッケージを含め、コンパイルはネットワークなしで実行する。
GoのビルドキャッシュとNimの生成C++は提出ごとのboxに置き、次の提出へ持ち越さない。

## Nimの要求一覧

以下はユーザー指定値を維持する。
commitで指定されたものは、そのcommitを採用対象とし、安定リリースのタグが存在するとは仮定しない。

| 依存 | 要求版 | 取得元 |
| --- | --- | --- |
| AC Library | `v1.5.1` | `atcoder/ac-library` |
| Arraymancer | `84af537af1bc1f90229fff2b90abf5e5c1b02616` | `mratsim/Arraymancer` |
| Eigen | `3.4.0` | `libeigen/eigen` |
| Nim-ACL | `0.1.0` | `zer0-star/Nim-ACL` |
| bigints | `ca00f6da386af9ad7e3abf603c0201da6a014477` | `nim-lang/bigints` |
| bignum_chaemon | `1.0.6` | `chaemon/bignum` |
| boost | `1.88.0` | Boost公式配布 |
| fftw | `3.3.10` | FFTW公式配布 |
| gmp | `6.3.0` | GMP公式配布 |
| mpfr | `4.2.1` | MPFR公式配布 |
| neo | `0.3.5` | `andreaferretti/neo` |
| nimsimd | `1.3.2` | `guzba/nimsimd` |
| regex | `0.26.3` | `nitely/nim-regex` |
| sat | `faf1617f44d7632ee9601ebc13887644925dcc01` | `nim-lang/sat` |

取得元の対応は[AtCoderのNim構築定義](https://img.atcoder.jp/file/language-update/2025-10/072-2-2-0_nim.toml)を照合した。
各archiveは取得元とSHA-256を`judge/runtime/runtime-extension-sources.lock.json`へ固定する。
Nimbleの依存定義から解決したnimblas、nimlapack、Unicodeデータなどの推移的依存も同じlockへ記録する。
Nimble自体はビルド環境だけで使用し、提出のコンパイルには固定した`--path`と設定を渡す。

ネイティブ依存は専用prefixへ構築し、include、link、実行時の探索先を揃える。
既存C++のACL 1.6とBoost 1.92.0は維持する。
SIMDの対象CPU命令をmanifestへ記録し、別のビルドマシンの`-march=native`をそのまま持ち込まない。
`nim cpp -d:release --opt:speed --mm:refc`と既存GCC 16.2.0を使う。
neoとbignumを同時に使用できるよう、Nim-GMPとbignumのdestructor分岐に各1行の互換パッチを適用する。
パッチ原文はリポジトリと配布物のbuild-manifestへ保存する。
利用者向けに公開するimport例は、要求版で実際に動くものをfixtureから転記する。

## Goの要求一覧

| 要求名 | module path | 版の扱い |
| --- | --- | --- |
| ac-library-go | `github.com/monkukui/ac-library-go` | 未指定。タグまたはpseudo-versionへ固定 |
| gods | `github.com/emirpasic/gods` | 未指定。`/v2`へ暗黙に変更しない |
| golang_org_x_exp | `golang.org/x/exp` | 未指定。commitに対応するpseudo-versionへ固定 |
| gonum | `gonum.org/v1/gonum` | 未指定。互換性のある安定タグへ固定 |
| gostl | `github.com/liyue201/gostl` | 未指定。タグまたはpseudo-versionへ固定 |
| immutable | `github.com/benbjohnson/immutable` | 未指定。互換性のある安定タグへ固定 |

名称の対応は[AtCoderのGo構築定義](https://img.atcoder.jp/file/language-update/2025-10/051-1-23-2_gc.toml)と一致する。
同定義は版指定なしの`go get`を使うため、それを再実行しても当時と同じ版になるとは限らない。
今回のlock生成時に全6モジュールを同時解決し、必要なサブパッケージをimportする運用側の小さなソースを使ってvendorを生成する。
空のmainだけで`go mod tidy`やvendorを実行して依存が消えることを防ぐ。
保証するサブパッケージを一覧化し、vendoringで漏れたものがないことを確認する。

提出用の`main.go`と固定の`go.mod`、`go.sum`をboxへ置き、vendorは読み取り専用で参照する。
isolateは起動時にbox内のシンボリックリンクを削除するため、vendorはGoのコンパイル時だけ`/box/vendor`へ読み取り専用でbind mountする。
`GOWORK=off`、`GOTOOLCHAIN=local`、`GOPROXY=off`、`CGO_ENABLED=0`を固定する。
初期のビルド並列度は`-p=1`、実行時は`GOMAXPROCS=1`とする。
`GOCACHE`と一時ファイルはbox内へ置き、共有の書き込み可能なキャッシュを作らない。
Goの固定コンパイル工程だけはisolate 2.7の`--syscalls=65531`を指定し、モジュールとキャッシュのファイルロックを許可する。
CGOと`go generate`は使用しないため、この工程で提出コードは実行されない。
提出・checker・interactorの実行時には既定の全syscall制限を適用し、`flock`の拒否も実機で検証する。
C#のコンパイル工程では参照アセンブリのためにオープンファイル上限を256とし、実行時は64とする。
cold cacheで全6モジュールの代表操作がコンパイル上限に収まることを確認する。

## 変更箇所と完了条件

C++は既存の`cpp23-gcc`と`cpp23-clang`の構成を更新し、testlib対応用の新しい言語IDは追加しない。
公開するC++23はtestlib対応版だけとし、新しいdigestで既存の通常提出と両方の判定形式を検証する。
旧配布物は切り戻し用に退避するが、提出画面で選択できる別構成としては残さない。

| 工程 | 主な変更先 | 完了条件 |
| --- | --- | --- |
| 配布物の固定 | `judge/runtime/prepare-runtime-inputs.py`、`runtime-sources.lock.json`、言語別lock | 配布元、正確な版、SHA-256、推移的依存、ライセンスを記録 |
| ビルド | `judge/runtime/runtime.Dockerfile`、`build-runtimes.sh` | 別マシンで構築でき、提出側に取得処理が残らない |
| コンパイルと実行 | `judge/runner/runtimes.py`、`sandbox.py`、`interactive.py` | C#の単一アセンブリ収集、固定補助ファイル配置、用途別VM設定が動く |
| testlibの保存 | APIの問題保存、公開、提出スナップショットとdispatch | `protocol`が保持され、省略時はlegacy、未知値は拒否 |
| testlibの実行 | `judge/runner/host.py`、`sandbox.py`、`interactive.py` | 引数、ファイル、終了コード、対話終了の優先規則が一致 |
| 言語一覧 | `api/internal/submissions/runtimes.go`、`web/app/utils/runtime-label.ts`、画面と公開設定の許可値 | 提出、checker、interactorで同じ公開一覧を参照 |
| 公開前の検証 | `judge/runner/language-smoke.json`、`smoke.py`、`interactive_smoke.py`、関連tests | 全依存と三用途が同じdigestで合格 |
| 配布と公開 | assets、fingerprint、admission、CI設定、runtime方針と利用ガイド | 補助ファイルも検証対象であり、未合格言語を公開しない |
| メンテナンス表示 | `GET /runtimes`、共通レイアウト、提出と生成の画面、受付API | 同じ停止状態でバナーと操作制限を切り替え、APIへの直接要求も拒否 |

C#は`artifact='dotnet'`相当の明示的な種類を追加し、任意のコンパイル出力ディレクトリをrootで再帰コピーしない。
固定された出力ファイルだけを通常ファイルとして検証し、現行の32 MiB上限で回収する。
補助ファイルも運用側の許可リストから配置する。
Java 25では既存のclass収集を共用し、判定用途での`-ea`も維持する。
各言語固有の環境変数はランタイム定義に置き、他言語へ不要な探索パスを足さない。

実装前のsmokeには、Java 24だけを識別する分岐や、未分類言語をPythonとして扱う分岐があった。
C#、Java 25、Nim、Goの失敗例とflush付き対話例を明示的に追加し、fixtureがない言語を合格扱いにしない。
公開用レポートはライブラリ検証、checker検証、interactive検証のすべてが成功した後にだけその言語を`passedRuntimes`へ加える。
対話検証の失敗時に部分的な合格情報だけで公開できないこともテストする。

## メンテナンス表示の適用手順

バナーと停止理由のAPI応答はランタイム交換より先にデプロイし、今回の適用作業自体で使えるようにする。
`GET /runtimes`の既存`items`を維持して`maintenance: boolean`を追加し、レスポンスの`no-store`を維持する。
Nuxt側のプロキシとSSRにもキャッシュさせず、共通の状態を各画面で使う。
初回SSRで状態を取得し、ページ遷移、タブ復帰、表示中の30秒間隔の再取得で反映する。
再取得に失敗した場合は利用不可として操作を止め、取得失敗をメンテナンス開始や終了と解釈しない。
画面の状態にかかわらず、APIの受付チェックを最終的な制御とする。

1. 受付停止を実行し、公開APIが`maintenance=true`を返すこと、各ジャッジ受付が503になること、サイト上部の表示を確認する。
2. 停止前から処理中だった受付要求が完了するまで待ち、未dispatch、Outbox、可視、処理中、遅延中のジョブが残っていないことを確認してworkerを止める。
3. ランタイムを適用し、バナーと停止を維持した状態で実機smokeとworkerの復旧確認を行う。
4. 検証済みdigestと公開言語で受付を再開し、バナーの消去とAPI経由の実提出を確認する。失敗時は直ちに再停止する。

バナーは共通レイアウトのヘッダー非表示条件の外へ配置し、ページ上部の通常フロー内で表示する。
閉じるボタンは付けず、モバイルで折り返し、キーボード操作や本文へのスキップを妨げない構成にする。
状態変更は`role="status"`等で読み上げ可能にする。
メンテナンス解除時も入力内容を消したり自動提出したりしない。

## 受入試験

| 対象 | 確認内容 |
| --- | --- |
| C++のtestlib | GCCとClangで無修正の`registerTestlibCmd`例、空出力、形式違反、`_ok`、`_wa`、`_fail`、未対応部分点 |
| testlibの対話 | `registerInteraction`、`inf`、`ouf`、`ans`、`tout`、flush、EOF、早期終了、出力量超過、相手の強制停止 |
| 既存判定との共存 | protocol省略とlegacyで従来引数と非ゼロWAを維持。公開後の編集で受付済みprotocolが変わらない |
| C++環境の上書き | 既存IDを維持し、testlibをincludeしない提出、ACL、Boostの既存fixtureが新digestで合格。testlibなし版が公開一覧に増えない |
| C# | 標準入出力、MathNetの行列計算、ACLのDSUと区間木。Debug.Assertに依存せず明示的な失敗コードも検証 |
| Java 25 | 標準入出力、ACLの代表操作、JAR収集、checkerのassert、用途別メモリ設定 |
| Nim | ACL、Arraymancer、Eigen、Boost、neo、SIMD、regex、satの代表操作。各多倍長とFFTWを実際にリンクして実行 |
| Go | 全6モジュールの代表操作、goroutine、冷えたキャッシュからのビルド、外部依存不足時に通信せずCEとなること |
| 全言語 | AC/WA/CE/RE/TLE/MLE/OLE、秘密ファイル非公開、通信遮断、子孫回収、ケース間の清掃 |
| 対話と数値計算 | 提出512 MiBとinteractor256 MiBでJIT、GC、内部スレッド、同時負荷を含めて動作 |
| 切り替え | APIからの受付と三用途の往復、旧digestの拒否、公開設定とレポートの不一致を拒否 |
| メンテナンス | 未ログイン、編集画面、モバイル、SSR、開いたままの画面で表示。通常提出、コンテスト、試験実行、生成と検証をAPIでも拒否 |
| メンテナンス解除と障害 | 入力ソースを維持。閲覧と下書き保存は継続。取得失敗時の誤解除を防ぎ、失敗した適用ではバナーを維持 |

testlibの判定コードには正解ファイルを渡すが、提出には渡さないことを両側から検証する。
ソースと診断ログの上限、`test-output`、追加された依存配置のtmpfs使用量も計測する。
依存不備、動的リンク失敗、起動時メモリ不足を利用者のコードの失敗として公開しないため、正常例が失敗した言語は公開前に止める。

## 実装開始時に確定する値

- testlibのcommit、.NETとJavaのパッチ、Nimのパッチ、Goの系列とパッチ。
- 未指定NuGetパッケージとGoモジュールの版、Nimを含む全推移的依存。
- C#とNimとGoの公開ID。Javaは`java25`とし、既存`java24`を保存する。
- Javaと.NETの用途別メモリ配分、GoのGC目標、各コンパイラの正確な引数。
- 全依存が収まる配布容量、実機のディスク余裕、コンパイルCPU時間とピークメモリ。

これらは未確認値を最新版として推測して埋めず、lockと実測記録で確定する。
要求版で成立しない依存が見つかった場合は、失敗する組み合わせと代替版を記録し、要求からの変更として判断する。
