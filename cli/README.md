# ShareOJ CLI

問題文、生成コード、テストケースをローカルで編集し、ShareOJ の下書きへ保存する Go 製 CLI です。
Go 1.26 以降でビルドでき、利用者には単一の実行ファイルを配布できます。

## ビルドと利用

リポジトリのルートから実行します。

```sh
cd cli
go build -o shareoj .
./shareoj init a-plus-b
./shareoj check a-plus-b
./shareoj login --username your-email@example.com
./shareoj push a-plus-b
```

`login` は端末からパスワードを非表示で入力します。
メールアドレスとパスワードによるログイン、SMS / TOTP / メールの追加認証、初回パスワード変更に対応します。
初めて利用するアカウントは Web でプロフィールを登録してください。
Google アカウントを利用する場合は次のコマンドでブラウザーを開きます。

```sh
./shareoj login --google
```

ブラウザーで Google 認証を完了すると CLI に戻り、トークンを保存します。
ブラウザーを自動起動できない環境では、表示された URL を同じコンピューターのブラウザーで開いてください。
`--no-browser` を指定すると自動起動を省略できます。
待機は最大 10 分で、Ctrl+C で中止できます。
ブラウザーを開く前に Web 側の CLI 対応を確認し、未配布または未設定ならエラーを表示して終了します。
MFA の新規登録は Web で行ってください。

`push` は既存の `PUT /my/problems/{id}` を利用します。
単独の問題を公開する操作は含みません。
コンテストに登録済みの問題では、既存 API の仕様に従い、保存時にコンテスト側の問題も更新されます。

| コマンド | 動作 |
| --- | --- |
| `init <directory>` | 新しいディレクトリに A + B の雛形を作成。既存ディレクトリは上書きしない |
| `check <directory>` | TOML、ファイル、文字コード、容量、テストの対応関係を検査 |
| `login [--username <email>]` | メールアドレスとパスワードでログイン |
| `login --google [--no-browser]` | ブラウザーで Google 認証を行いトークンを保存 |
| `push <directory>` | 指定されたファイルを読み、下書きを作成または更新 |
| `logout` | 接続先のローカル認証情報を削除 |

`check` はプログラムをコンパイルしたり実行したりしません。
言語 ID は形式のみを検査し、サーバーで利用可能かどうかや公開条件は判定しません。
生成器の実行と解答の正しさの検証は、ShareOJ の Web 画面または rime で行ってください。

## 問題ディレクトリ

```text
a-plus-b/
  problem.toml
  statement.md
  input.cpp
  output.cpp
  validate.cpp
  tests/
    sample1.in
    sample1.out
```

`problem.toml` の例です。
ファイルのパスは問題ディレクトリからの相対パスで指定します。
未知の設定キーとディレクトリ外を参照するパスはエラーになります。

```toml
title = "A + B"
statement = "statement.md"
time_limit_ms = 2000
memory_limit_mb = 512
# editorial = "editorial.md"
# difficulty = 1

[generators.input]
source = "input.cpp"
runtime = "cpp17"

[generators.output]
source = "output.cpp"
runtime = "cpp17"

[generators.validation]
source = "validate.cpp"
runtime = "cpp17"

[tests]
directory = "tests"
output_suffix = ".out"
samples = ["sample1"]
```

入力生成器はケース番号を標準入力で受け取り、1 ケースの入力を標準出力へ書きます。
出力生成器は問題の入力を読み、期待出力を書きます。
入力検証器は入力を読み、妥当なら終了コード 0 を返します。
詳細は [生成器ガイド](../web/app/content/guides/generator-guide.md) を参照してください。

カスタム判定器は `[checker]`、対話プログラムは `[interactor]` に同じ `source` と `runtime` を指定します。
両方の同時指定はできません。
これらのテーブルだけ `protocol = "legacy"` または `protocol = "testlib"` を指定できます。
`testlib` は `cpp23-gcc` と `cpp23-clang` に対応し、生成器には `protocol` を指定しません。

## テストケースと rime

`tests.directory` 直下の `*.in` と同名の `*.out` を、ファイル名の順で取り込みます。
`samples` には拡張子を除いたサンプルの名前を指定します。
空ファイル、空白、CRLF、末尾の改行をそのまま保存します。
入力と出力の片方だけがある場合はエラーになります。

テストは最大 100 件、各入力と出力は 16 MiB、合計 512 MiB までです。
64 KiB を超える本文と、JSON 内の合計が 256 KiB を超える分は、署名付き URL でアップロードします。
ファイルの検証が完了するまで下書きを更新しません。
本文とコードを含む全ファイルは UTF-8 とし、NUL は受け付けません。

rime の問題ディレクトリに `problem.toml` と問題文を追加すれば、生成済みテストを利用できます。
例えば標準の出力先が `rime-out/tests` の場合は次のように設定します。

```toml
[tests]
directory = "rime-out/tests"
output_suffix = ".diff"
samples = ["00-sample1"]
```

先に rime で生成と検証を完了し、実際の出力先とサンプル名を設定してください。
CLI は `PROJECT` / `PROBLEM` / `TESTSET` を読み込まず、rime も実行しません。
生成コードを ShareOJ に登録する場合は、ShareOJ の入出力規約に合わせて別途指定します。

## 更新と競合

初回の `push` は問題 ID を発行し、接続先と更新番号を `.shareoj.json` に記録します。
再実行は同じ ID を使います。
このファイルは問題のソースと一緒に配布せず、手元に保持してください。
`init` が作る `.gitignore` では除外しています。

`push` はタイトル、問題文、時間制限、メモリ制限を毎回更新します。
設定した生成器、解説、難易度、判定器も更新します。
省略した任意項目はサーバー側の内容を保持します。
`[tests]` を指定すると全テストをローカルの一覧に置き換え、省略すると既存テストを保持します。
空のテストディレクトリを明示すると全テストを削除します。
判定器の種類を変更した場合は、以前の種類の設定を削除します。

Web 側で編集されて更新番号が変わっていれば、`push` は停止します。
Web の下書きとローカルの内容を比較して必要な変更を取り込んだ後、表示された更新番号を指定して再実行できます。

```sh
./shareoj push --expect-version 4 a-plus-b
```

この指定でも、確認した番号からさらに変更されていれば保存を拒否します。
通信断で保存結果が不明な場合も `.shareoj.json` を残し、同じ ID で再実行してください。
保存済みなら更新番号の競合が表示されるので、Web で内容を確認してから同じ手順で再開できます。
ロックファイル `.shareoj.lock` が異常終了後に残った場合は、他の `push` が動いていないことを確認して削除してください。

## 認証と接続先

既定の接続先は `https://api.share-oj.net` です。
`login` / `push` / `logout` の `--api`、または環境変数 `SHAREOJ_API_URL` で変更できます。
オプションはディレクトリ名より前に指定します。

```sh
./shareoj login --api http://localhost:8080
./shareoj push --api http://localhost:8080 a-plus-b
```

Google ログインで既定以外の API を使う場合は、対応する Web サイトも `--site` で指定します。

```sh
./shareoj login --google --api http://localhost:8080 --site http://localhost:3000
```

Google ログインには、この変更を含む Web サーバーの配布が必要です。
既存の Cognito のクライアントシークレットと登録済みの Web コールバックを再利用するため、CLI 用のクライアント作成やコールバック URL の追加は不要です。
CLI はクライアントシークレットを持たず、PKCE の検証情報を手元だけに保持します。
ブラウザーは Web の認証コールバックから `127.0.0.1` の一時ポートへ戻り、60 秒で期限が切れる署名付き認証コードを渡します。
CLI は Web の `/api/auth/cli/exchange` でトークンへ交換し、指定された API の `/auth/me` で利用できることを確認してから保存します。
アクセストークンと更新トークンはブラウザーの URL に含めません。
SSH 先で実行する場合はローカル受信ポートへの転送が必要になるため、通常はブラウザーと CLI を同じコンピューターで使ってください。
WSL や Docker を使う場合も、ホスト側のブラウザーから CLI の `127.0.0.1` の受信ポートへ到達できる構成が必要です。
Google 認証後に通常のマイページへ移る場合は CLI に通知されていません。
Web の配布と、使用している CLI バイナリが最新かどうかを確認してください。

認証情報は OS のユーザー設定ディレクトリ内の `shareoj/` に、接続先ごとに保存します。
Linux では通常 `~/.config/shareoj/` です。
`SHAREOJ_CONFIG_DIR` で保存先を変更できます。
POSIX 環境のファイル権限は `0600` で、トークンは有効期限に応じて更新します。
パスワードは保存しません。

自動化では `SHAREOJ_ACCESS_TOKEN` に Cognito のアクセストークンを設定できます。
ID トークンは使えません。
この環境変数は保存済みトークンより優先し、期限切れ時の更新は呼び出し側が行います。
`logout` は環境変数を変更せず、サーバー上のセッションも失効させません。
HTTPS を必須とし、ローカル開発用の `localhost` / `127.0.0.1` / `::1` のみ HTTP を許可します。

## 検証と配布

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o shareoj .
# macOS / Apple Silicon 向けの例
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o shareoj-darwin-arm64 .
```

テストはローカルの模擬 HTTP API を使い、本番へ問題を作成しません。
ビルド時の依存は TOML パーサーと端末入力用ライブラリで、実行時に Go や Python のインストールは不要です。
