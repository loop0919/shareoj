# ShareOJ 作問ガイド

このガイドはインストール済みCLIに同梱されており、通信やログインなしで読めます。
ShareOJへの保存まで依頼されている場合は、以下の手順で下書きを保存します。
ローカルでの作成や検討だけが依頼されている場合は、その範囲で進めます。

## 利用できる操作

| コマンド | 用途 |
| --- | --- |
| `shareoj init <directory>` | 問題の雛形を新しいディレクトリに作る |
| `shareoj pull <problem-id> <directory>` | 作者または参加済みテスターとして、全テストを含む下書きを新しいディレクトリへ取得する |
| `shareoj check <directory>` | 設定、ファイル、文字コード、容量、入出力の対応を検査する |
| `shareoj push <directory>` | 下書きを作成または更新する |
| `shareoj login --google` | ユーザーがブラウザーでGoogleログインする |
| `shareoj login` | ユーザーが端末でメールアドレスとパスワードを入力する |
| `shareoj version` | インストール済みのバージョンを確認する |

オプションは問題IDやディレクトリより前に指定します。
既定のAPI接続先は `https://api.share-oj.net` です。
別の環境に接続する場合は `--api <URL>` または `SHAREOJ_API_URL` を使い、取得と保存で同じ接続先を指定してください。
公開、提出、サーバーでの生成や採点を実行するCLIコマンドはありません。
これらの操作にはWeb画面を使います。

## 新規作問と既存問題の編集

新しく作問する場合は `shareoj init ./my-problem` で雛形を作り、問題文、解法、生成器、テストケースを作成する問題に合わせて書き換えます。
既存の問題を編集する場合は、Web画面の問題編集URLのUUIDか、問題ページのURL（`/problems/<ID>`）にある22文字のIDを確認し、`shareoj pull <problem-id> ./my-problem` で下書きを取得します。
テスターとして問題を取得する場合は、招待を受け入れたアカウントでログインしてください。
`init` と `pull` は既存ディレクトリを上書きしません。

編集前に `problem.toml` を読み、そこに指定されたファイルを使います。
`pull` で取得したコードファイルの拡張子は `.txt` ですが、実行言語は設定の `runtime` で決まります。
解法の正しさ、制約に対する計算量、問題文とサンプルの一致を確認し、境界値を確認するテストや、想定する誤解法を区別するテストを用意します。

```sh
shareoj check ./my-problem
# コードをコンパイル、実行してテストを検証する
# ShareOJへの保存が依頼されている場合
shareoj push ./my-problem
```

## problem.toml とファイル

パスは問題ディレクトリからの相対パスです。
ディレクトリ外の参照と未知の設定キーはエラーになります。
ファイルはUTF-8で保存し、NUL文字は使いません。

```toml
title = "問題タイトル"
statement = "statement.md"
editorial = "editorial.md"
time_limit_ms = 2000
memory_limit_mb = 512
difficulty = 3

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

`statement.md` に問題文、制約、入出力形式、サンプルを記述します。
`editorial.md` には解法、正しさの根拠、計算量を記述します。
問題文中のサンプルは `tests/sample1.in` と `tests/sample1.out` にも用意し、内容を一致させます。
タイトルは120文字まで、問題文と解説はそれぞれ100,000文字までです。
時間制限は100〜5,000 msの100 ms刻み、メモリ制限は64〜512 MiBです。
難易度を指定する場合は1〜10です。
生成器、解説、難易度、テスト設定は省略できます。
利用できる実行言語はWebの言語ガイドで確認します。

## 生成器と判定器

| 設定 | 入力と結果 |
| --- | --- |
| `generators.input` | 標準入力からケース番号を1つ読み、1ケースの問題入力を標準出力へ書く。ケース番号はコマンドライン引数ではない |
| `generators.output` | 問題入力を標準入力から読み、期待出力だけを標準出力へ書く |
| `generators.validation` | 問題入力を標準入力から読み、妥当なら終了コード0、不正なら非0で終える |

生成器のソースはそれぞれ65,536バイトまでです。
ケース番号を乱数のシードにすると、同じケース番号から同じ入力を再現できます。
出力生成器の答えは、手計算できる例や独立した愚直解と照合します。

スペシャルジャッジを使う場合は `[checker]` に、対話形式を使う場合は `[interactor]` に `source` と `runtime` を指定します。
`[checker]` と `[interactor]` は同時に設定できません。
`protocol = "legacy"` または `protocol = "testlib"` を指定できるのは、この2種類だけです。
`testlib` は `cpp23-gcc` と `cpp23-clang` に対応します。
生成器には `protocol` を指定しません。
判定器を実装する前に、Webの判定方法ガイドで入出力や終了コードの仕様を確認してください。

## テストの配置と検証

`tests.directory` 直下に、拡張子を除いた名前が同じ `.in` と `.out` をペアで置きます。
`samples` には拡張子を除いたファイル名を指定します。
CLIはファイル名順に取り込み、空白と空ファイルを保持します。
改行のCRLFとCRはLFに変換し、空でないファイルの末尾に改行がなければ足します。
最大100ケース、入力と出力は各16 MiB、合計512 MiBまでです。
`push` は大きいファイルを外部保存に切り替えます。

`pull` はテストの順序を保持するため、`case001.in` のような連番のファイル名で保存します。
元のケース名は `[tests.names]` の対応表に保持します。
取得後にケースを削除するときは、対応する `names`、`samples`、`[[tests.groups]]` の `cases` の項目も更新してください。

`check` はコンパイルも実行もしません。
想定解と入力検証器を手元またはrimeで実行し、必要ならWebの「生成と検証」も使います。
CLIはrimeを実行せず、`PROJECT`、`PROBLEM`、`TESTSET` も読みません。
rimeの生成済みファイルを使う場合は、実際の出力先を `tests.directory` に指定し、`output_suffix = ".diff"` にします。
実行できていない検証は未検証として報告します。

## 部分点

部分点を付ける問題では、`[scoring]` に採点方式を、`[[tests.groups]]` にグループを指定します。
指定しない問題は、すべてのケースでACのときだけ満点にします。

```toml
[scoring]
mode = "groups"

[[tests.groups]]
name = "小課題1"
points = 30
cases = ["01", "02"]

[[tests.groups]]
name = "小課題2"
points = 70
cases = ["01", "02", "03", "04"]
```

`mode = "groups"` では、属するケースがすべてACのグループに配点を与えます。
`mode = "aggregate"` では、`[scoring.aggregator]` に `source` と `runtime` を指定した集計コードが、グループごとの得点を決めます。
集計コードには `protocol` を指定しません。
集計コードの入出力と、判定器がケースの点を返す方法は、Webの判定方法ガイドで確認します。
`cases` には `samples` と同じく拡張子を除いたファイル名を指定し、1つのケースを複数のグループに入れられます。
グループは100個まで、名前は64文字までで、前後の空白を除いて重複できません。
配点は0以上の整数で、満点（配点の合計）は1〜1,000,000点です。
`[scoring]` を使う場合は `[tests]` も指定します。
ケースのないグループと、どのグループにも属さないサンプル以外のケースは、`check` が警告します。
この状態でも下書きには保存できますが、公開の条件を満たしません。

## 保存の範囲と競合

`push` はタイトル、問題文、時間制限、メモリ制限を更新し、設定された任意項目も更新します。
省略した任意項目については、サーバー側の内容を保持します。
`[tests]` があれば全テストをローカルの一覧に置き換え、空のテストディレクトリなら全テストを削除します。
判定器の種類を切り替えた場合は、以前の判定器の設定を削除します。
`[scoring]` があれば採点方式とグループをローカルの内容に置き換え、集計コード方式で `[scoring.aggregator]` を省略した場合はサーバー側の集計コードを保持します。
部分点の問題で `[scoring]` を省略して `[tests]` を指定すると、ケースの所属が消えるため、`push` は保存せずに止まります。

単独問題の公開は `push` に含まれません。
コンテスト登録済みの問題では保存時にコンテスト側の問題も更新されるため、依頼された対象と変更内容を確認して保存します。
公開中の問題と、結果確定前のコンテストに入っている問題は、公開の条件を満たさない内容では保存できず、`push` は `incomplete_problem` で失敗します。
コンテストの開始後に採点方式とグループの名前、配点、順番を変える保存は、`problem_scoring_locked` で拒否されます。
`push` の成功を、採点成功や公開完了として報告しないでください。

`.shareoj.json` は取得元または作成先の問題ID、API、版番号を保持します。
再度 `push` するときもこのファイルを残し、他の問題用にコピーしたり、手作業で版番号を変更したりしません。
通信断でもこのファイルを残し、同じIDで再試行します。
競合したら別ディレクトリへ `pull` して比較し、必要な変更を取り込んでから、確認した版番号を `shareoj push --expect-version <version> <directory>` に指定します。
変更内容を確認せずに版番号だけを合わせて再試行すると、他の人の変更が失われるため、この操作は自動では行いません。

認証が必要な場合はユーザーにCLIでログインしてもらい、パスワードやトークンを会話や問題ファイルへ書き出さないでください。
最後に問題ディレクトリ、検証結果、保存の有無を示し、保存済みなら問題IDと版番号を添えます。

## AIエージェントへの登録

一度 `shareoj guide --install-skill codex` または `shareoj guide --install-skill claude` を実行し、新しいエージェントセッションを開始します。
両方に登録する場合は `shareoj guide --install-skill all` を使います。
このスキルは「ShareOJで作問して」などの依頼で使うもので、AIエージェントに最初にこのガイドを読むよう案内します。
自動選択されない場合はCodexで `$shareoj-authoring`、Claude Codeで `/shareoj-authoring` を明示できます。

詳しいCLI仕様: https://github.com/loop0919/shareoj/tree/main/cli
生成器ガイド: https://www.share-oj.net/docs/generator
言語ガイド: https://www.share-oj.net/docs/languages
判定方法ガイド: https://www.share-oj.net/docs/checkers
