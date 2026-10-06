# ShareOJ CLI

問題文、生成コード、テストケースをローカルで編集し、ShareOJ の下書きへ保存する Go 製 CLI です。
ビルド済みの実行ファイルをインストールでき、利用者の環境に Go は不要です。

## インストールと利用

Linux / WSL と macOS の x86_64 / arm64 に対応しています。
`curl` と `sha256sum` または `shasum` が必要です。

```sh
curl -fsSL https://www.share-oj.net/install.sh | bash
```

GitHub Releases からバイナリを取得し、SHA-256 を照合して `~/.local/bin/shareoj` に保存します。
sudo は不要です。
初回に PATH の案内が表示された場合は、表示された設定を `~/.bashrc` や `~/.zshrc` に追加し、端末を開き直してください。
同じコマンドを再実行すると最新版に更新します。
ダウンロードや検証が失敗した場合、既存のバイナリは保持します。

```sh
shareoj version
shareoj init a-plus-b
shareoj check a-plus-b
shareoj login --google
shareoj push a-plus-b
```

バージョンや保存先を指定する場合は次のように実行します。

```sh
curl -fsSL https://www.share-oj.net/install.sh | bash -s -- cli-v0.1.0
curl -fsSL https://www.share-oj.net/install.sh | SHAREOJ_INSTALL_DIR="$HOME/bin" bash
```

Windows で直接使う場合は [Releases](https://github.com/loop0919/shareoj/releases) から `shareoj-windows-amd64.exe` または `shareoj-windows-arm64.exe` を取得し、`shareoj.exe` に名前を変更してください。
削除する場合はインストール先の実行ファイルを削除します。
認証情報を削除するには、その前に `shareoj logout` を実行してください。

`login` は端末からパスワードを非表示で入力します。
メールアドレスとパスワードによるログイン、SMS / TOTP / メールの追加認証、初回パスワード変更に対応します。
初めて利用するアカウントは Web でプロフィールを登録してください。
Google アカウントを利用する場合は次のコマンドでブラウザーを開きます。

```sh
shareoj login --google
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
| `guide` | AIエージェント向けの作問手順を表示（通信と認証は不要） |
| `guide --install-skill <codex\|claude\|all>` | 作問スキルをユーザーのスキルディレクトリへ登録 |
| `init <directory>` | 新しいディレクトリに A + B の雛形を作成。既存ディレクトリは上書きしない |
| `check <directory>` | TOML、ファイル、文字コード、容量、テストの対応関係を検査 |
| `pull <problem-id> <directory>` | 自分または参加済みテスターの問題の下書きを新しいディレクトリへ取得 |
| `login [--username <email>]` | メールアドレスとパスワードでログイン |
| `login --google [--no-browser]` | ブラウザーで Google 認証を行いトークンを保存 |
| `push <directory>` | 指定されたファイルを読み、下書きを作成または更新 |
| `logout` | 接続先のローカル認証情報を削除 |
| `version` / `--version` | インストール済みのバージョンを表示 |

`check` はプログラムをコンパイルしたり実行したりしません。
言語 ID は形式のみを検査し、サーバーで利用可能かどうかや公開条件は判定しません。
生成器の実行と解答の正しさの検証は、ShareOJ の Web 画面または rime で行ってください。

## AIエージェントに作問を頼む

一度、利用するエージェントのスキルを登録してください。
CodexとClaude Codeの両方に登録する場合は次のコマンドを使います。

```sh
shareoj guide --install-skill all
```

Codexだけなら `codex`、Claude Codeだけなら `claude` を指定します。
登録先はそれぞれ `~/.agents/skills/shareoj-authoring/SKILL.md` と `~/.claude/skills/shareoj-authoring/SKILL.md` です。
同じ内容なら再実行でき、内容の異なる既存ファイルは上書きしません。

新しいエージェントセッションで「ShareOJで作問して」と頼むと、作問スキルが選択候補になり、選択された場合は最初に `shareoj guide` を読みます。
たとえば「ShareOJで二分探索の問題を作問して。ローカルで検証して、下書きまで保存して」と依頼できます。
スキルの自動選択はエージェント側の判断に依存します。
選択されない場合はCodexで `$shareoj-authoring`、Claude Codeで `/shareoj-authoring` を明示するか、「最初に `shareoj guide` を読んで」と指定してください。
ローカルのスキルが使えない環境でも、CLIが実行できればガイドを直接読めます。

ガイドとスキルはCLIバイナリに同梱されます。
スキルはガイドへの入口を持ち、CLIを更新するとガイドも更新されます。
`shareoj --help` にもガイドへの案内があります。
登録を解除するには、上記の該当スキルディレクトリを削除します。

スキル配置と自動選択の仕様: [OpenAI Docs](https://learn.chatgpt.com/docs/build-skills)、[Claude Code Docs](https://code.claude.com/docs/en/skills)。

## 既存の下書きを取得

Web の問題編集 URL に含まれる問題 ID（UUID）を指定します。
テスターの問題は、Web で招待を受け入れたアカウントでログインしてください。
作者・テスターとも同じコマンドを使います。

```sh
shareoj login
shareoj pull <problem-id> ./my-problem
shareoj check ./my-problem
# ファイルを編集後、取得元の下書きへ保存
shareoj push ./my-problem
```

問題文、解説、生成器、チェッカーまたはインタラクター、非公開を含む全テストケースを取得します。
コードは `input.txt`、`output.txt`、`validation.txt`、`checker.txt`、`interactor.txt` に保存し、実行言語は `problem.toml` に保持します。
テストファイルは `tests/case001.in` / `case001.out` のような連番で保存します。
元のケース名（空の名前を含む）は `[tests.names]` に記録し、ケースの順序とサンプル指定も保持します。
`samples` は元のケース名ではなく `case001` のようなファイル名の幹を指定します。

取得先がすでに存在する場合は上書きせずエラーにします。
取得中に失敗した場合は、作成途中のディレクトリを削除します。
外部保存されたテストファイルはサイズと SHA-256 を検証します。
取得した問題 ID と版番号を `.shareoj.json` に保存するため、後続の `push` は同じ問題を更新し、他の編集と競合した場合は停止します。
再取得するときは別の新しいディレクトリを指定してください。
接続先を変える場合は `shareoj pull --api <URL> <problem-id> <directory>` または `SHAREOJ_API_URL` を使います。

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
shareoj push --expect-version 4 a-plus-b
```

この指定でも、確認した番号からさらに変更されていれば保存を拒否します。
通信断で保存結果が不明な場合も `.shareoj.json` を残し、同じ ID で再実行してください。
保存済みなら更新番号の競合が表示されるので、Web で内容を確認してから同じ手順で再開できます。
ロックファイル `.shareoj.lock` が異常終了後に残った場合は、他の `push` が動いていないことを確認して削除してください。

## 認証と接続先

既定の接続先は `https://api.share-oj.net` です。
`login` / `pull` / `push` / `logout` の `--api`、または環境変数 `SHAREOJ_API_URL` で変更できます。
オプションはディレクトリ名より前に指定します。

```sh
shareoj login --api http://localhost:8080
shareoj push --api http://localhost:8080 a-plus-b
```

Google ログインで既定以外の API を使う場合は、対応する Web サイトも `--site` で指定します。

```sh
shareoj login --google --api http://localhost:8080 --site http://localhost:3000
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

ソースからビルドする場合は Go 1.26 以降を使用し、`cli/` で実行します。

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o shareoj .
# macOS / Apple Silicon 向けの例
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o shareoj-darwin-arm64 .
```

テストはローカルの模擬 HTTP API を使い、本番へ問題を作成しません。
ビルド時の依存は TOML パーサーと端末入力用ライブラリで、実行時に Go や Python のインストールは不要です。

### リリース手順（メンテナー向け）

インストーラーは `web/public/install.sh` で、通常の Web 配布に含まれます。
最初にこの変更を main へ push し、Web の配布を完了してください。
続いて、配布するコミットに `cli-vX.Y.Z` 形式のタグを付けて push します。

```sh
git tag cli-v0.1.0
git push origin cli-v0.1.0
```

`Release CLI` ワークフローがテスト・検査後、Linux / macOS / Windows の amd64 / arm64 向けにビルドします。
全バイナリと `SHA256SUMS` のアップロード後に Release を公開し、Latest に指定します。
初回 Release が公開されるまではインストールできません。
インストーラーのバージョン省略時は GitHub の Latest を参照するため、このリポジトリの Latest は CLI の安定版に指定してください。
アップロード失敗で draft が残った場合は、その draft を削除してからワークフローを再実行します。

Web の配布前に試す場合は GitHub 上のスクリプトも使えます（バイナリの Release は必要です）。

```sh
curl -fsSL https://raw.githubusercontent.com/loop0919/shareoj/main/web/public/install.sh | bash
```
