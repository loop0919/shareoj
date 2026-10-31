## できること {#overview}

作問CLI（`shareoj`）を使うと、問題文、生成コード、テストケースを手元のファイルで編集し、ShareOJの下書きに保存できます。
保存した問題の公開は、Webのエディタの「公開・応募」か、問題一覧の「問題投稿」から行います。
CLIは問題を公開したり、解答を提出したりしません。

すべての設定とオプションは、GitHubの[CLIのREADME](https://github.com/loop0919/shareoj/blob/main/cli/README.md)に書いています。

## インストールと更新 {#install}

LinuxとmacOSでは、次のコマンドでインストールします。
WSLも同じ手順です。
`curl` と、`sha256sum` または `shasum` が必要です。

```sh
curl -fsSL https://www.share-oj.net/install.sh | bash
```

`~/.local/bin/shareoj` にインストールし、sudoは不要です。
PATHの設定を案内された場合は、表示された設定をシェルの設定ファイルに追加してください。
Windowsでは、[GitHub Releases](https://github.com/loop0919/shareoj/releases) から `shareoj-windows-amd64.exe` または `shareoj-windows-arm64.exe` を取得し、`shareoj.exe` に名前を変えて使います。

更新するときは、同じインストールコマンドをもう一度実行します。
新しい版が出ていると、`login`、`pull`、`push` のあとに案内を表示します。

## シェルの補完 {#completions}

`shareoj completions` が出力するスクリプトで、コマンドやオプションをTabで補完できます。

```sh
# bash: ~/.bashrc に追加
eval "$(shareoj completions bash)"

# zsh: 保存し、~/.zshrc の compinit より前に fpath=(~/.zfunc $fpath) を追加
mkdir -p ~/.zfunc
shareoj completions zsh > ~/.zfunc/_shareoj

# fish
mkdir -p ~/.config/fish/completions
shareoj completions fish > ~/.config/fish/completions/shareoj.fish
```

設定したら、端末を開き直してください。

## ログイン {#login}

```sh
shareoj login
shareoj login --google
```

`shareoj login` はメールアドレスとパスワードでログインします。
Googleアカウントでは `--google` を付けると、ブラウザーで認証します。
初めて使うアカウントは、先にWebでプロフィールを登録してください。
ログイン情報を消すには `shareoj logout` を実行します。

## 問題を作って保存する {#workflow}

```sh
shareoj init a-plus-b
shareoj check a-plus-b
shareoj push a-plus-b
```

`init` は、問題文と解説の雛形、A + Bの生成コード、テストケースの例を入れたディレクトリを作ります。
`check` は設定ファイル、文字コード、容量、テストケースの組み合わせを検査します。
コードのコンパイルや実行はしません。
`push` は内容をShareOJの下書きに保存します。

Webで作った問題やテスターとして参加している問題は、`pull` で手元に取得できます。
問題IDには、問題ページのURL（`/problems/<ID>`）にある22文字のIDも使えます。

```sh
shareoj pull <問題ID> ./my-problem
```

取得したディレクトリで `push` すると、取得元の問題を更新します。
Webなどで先に更新されていた場合、`push` は保存せずに止まります。
内容を確かめてから、表示された更新番号を `--expect-version` に指定して再実行してください。

## problem.toml {#problem-toml}

問題の設定は `problem.toml` に書きます。
ファイルのパスは、問題ディレクトリからの相対パスです。

```toml
title = "A + B"
statement = "statement.md"
editorial = "editorial.md"
time_limit_ms = 2000
memory_limit_mb = 512
# difficulty = 1

[generators.input]
source = "input.cpp"
runtime = "cpp23-gcc"

[generators.output]
source = "output.cpp"
runtime = "cpp23-gcc"

[generators.validation]
source = "validate.cpp"
runtime = "cpp23-gcc"

[tests]
directory = "tests"
output_suffix = ".out"
samples = ["sample1"]
```

スペシャルジャッジは `[checker]`、インタラクティブは `[interactor]` に `source` と `runtime` を書きます。
2つを同時には指定できません。
testlib形式の判定コードでは、`protocol = "testlib"` も指定します。
判定コードの書き方は[判定方法の設定](/docs/checkers)を見てください。

## AIエージェントに作問を頼む {#agents}

CodexやClaude Codeに、作問の手順をまとめたスキルを登録できます。

```sh
shareoj guide --install-skill all
```

Codexだけなら `codex`、Claude Codeだけなら `claude` を指定します。
登録したあと、新しいセッションで「ShareOJで作問して」と頼んでください。
`shareoj guide` を実行すると、エージェント向けの作問手順を直接読めます。
