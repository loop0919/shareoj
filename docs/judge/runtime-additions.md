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

この5言語は実装済みだが、配布と実機smokeは未実施である。
既存のビルド手順でランタイムとworkerを再構築し、`rollout.py run`の実機検証に合格したIDだけを公開する。

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
TruffleRubyは固定gemディレクトリを直接参照して提出ごとのBundler起動を省き、JITを有効にした`latency`モードで実行する。
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
