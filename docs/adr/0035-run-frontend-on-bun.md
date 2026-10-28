# ADR 0035: フロントエンドをBunで動かし、LambdaのSSRもBunにする

- 状態：Accepted
- 決定日：2026-10-29
- 範囲：webの依存管理、開発、ビルド、テスト、CI/CD、フロントエンドのLambdaのランタイム

## 背景

webはnpmとNode.js 22で開発し、本番のSSRはLambdaの`nodejs22.x`で動いていた。
2026-09-16にBunへ移した変更（8e4e826）は、同じ日に取り消した（d9bb638）。
今回は、移行で変わりうる挙動を洗い出し、それぞれを確かめてから移す。

LambdaにはBunの管理ランタイムがない。
APIの関数は、すでにカスタムランタイムの`provided.al2023`で動いている。

## 決定

### 開発、テスト、CIをBunで動かす

- Bunは1.4.4に固定する。版は`flake.nix`、`web/package.json`の`packageManager`、`web/scripts/bun_release.py`に書き、`bun_release.py`が一致を確かめる。
- `package-lock.json`を同じ版のまま`bun.lock`へ移す。CIは`--frozen-lockfile`でインストールする。
- スクリプト、Playwrightとその起動するサーバー、`make dev`をBunで動かす。開発シェルからNode.jsを外す。
- 依存の脆弱性は`bun audit`で調べる。判定と許可リストは変えない。

### LambdaのSSRをBunで動かす

- ランタイムを`provided.al2023`にする。zipにARM64版Bun、`bootstrap`、Runtime APIの受け口`lambda/runtime.mjs`を入れる。
- ハンドラーの設定（`lambda/index.handler`）は変えない。zipは`nodejs22.x`でも動く。
- ログとエラーの扱いは、`nodejs22.x`のランタイムのクライアント（イメージの`/var/runtime`）にそろえる。
- ログには時刻、リクエストID、レベルを付け、1回の出力を1つのイベントにする。
- ハンドラーの外で起きた例外とPromiseの拒否は、実行中の呼び出しか初期化のエラーとして報告し、Node.jsと同じく129と128で終了する。次の呼び出しを受け取れないときも終了し、Lambdaに新しいランタイムを起動させる。
- CIはarm64のランナーで、デプロイするzipを公式の`provided.al2023`イメージとRuntime Interface Emulatorで起動して確かめる。

### 移行で見つかった違いと対処

| 違い | 対処 |
| --- | --- |
| `postinstall`と、`--bun`のない`bun run`は、PATHのNode.jsで動く | Bunを明示する。CIは失敗する`node`をPATHの先頭に置いて取りこぼしを検出する |
| Bunは`.env`を自動で読み、値の`$`を展開する | `bunfig.toml`の`env = false`、`make`とLambdaの`--no-env-file` |
| `nuxt dev`のSSRが全ページ500になる（Bunは名前空間に`__esModule`を見せ、vite-nodeがzodの`z`を落とす） | 開発時だけzodをViteで変換する |
| PlaywrightをBunで動かすと、Nuxtの`~`と`~~`を解決できない | `tsconfig.json`に`paths`を置く |
| BunはテンプレートのASCII以外の文字を`\u`で持ち、`String.raw`に表れる | `String.raw`はASCIIだけにする。今の本番のバンドルに該当はない |
| `Response.blob()`が、まれに`Content-Type`を落とす | アップロード画像の種類を、データの先頭のシグネチャで判定する |
| `bun test`は`package.json`の`test`ではなくBunのテストランナーで、Playwrightのテストを読み込む | ブラウザーのテストは`bun run test`。`bun test`はBunのテストだけを対象にする |
| Playwrightのイメージにunzipがなく、`oven-sh/setup-bun`が動かない | `bun_release.py`がBunを取得し、SHA-256を検証して入れる。今はx86-64のマシンで起動テストのイメージにBunを入れるときに使う |
| 自作のRuntime APIの受け口は、Node.jsのランタイムがするエラーの報告と終了、ログの見出し、`context`の項目を持たない | Node.jsのランタイムの実装に合わせる。ハンドラーがグローバルの`fetch`を置き換えても、Runtime APIとの通信は変わらない |

### 確かめたこと

- 既存のテスト：ブラウザー604件、実DB 32件（ローカルのジャッジが要る1件はCIと同じく省略）、`test:lambda`、Terraformのテストが通る。
- 依存の版：644件が`package-lock.json`と一致する。勧告は`npm audit`と同じ9件（highとcritical 5件）。
- SSR：同じビルドをNode.js 22とBunで動かし、69のURLのHTMLと共有画像がバイト単位で一致する（違いは比較用のポート番号だけ）。
- 開発サーバー：同じURLの状態コードが、Node.jsの`nuxt dev`と一致する。
- Lambda：Runtime Interface Emulatorでの初期化はBunが118〜126 ms、同じzipの`nodejs22.x`が199〜206 msだった（ローカル、zipのダウンロードは含まない）。
- Lambdaの512 MBと同じ約0.29 vCPUとメモリ512 MBに絞ると、初期化はBunが中央値637 ms、`nodejs22.x`が1266 ms、初期化を含む最初のSSRは2767 msと3375 msだった（各5回）。
- エラーの扱い：AWSの`nodejs:22`イメージと`provided.al2023`イメージで同じハンドラーを動かした。ハンドラーの例外、読み込み時の例外、ハンドラーの未定義、Promiseの拒否、捕まえていない例外、ログの6つで、応答とログの行が一致した（違うのはスタックの中身だけ）。
- `TZ`：LambdaがすべてのランタイムでTZに設定する`:UTC`を、BunもNode.jsと同じくUTCとして読む。起動テストもこの値と関数のメモリで動かす。
- Runtime APIの次の呼び出しは長く待つ。Bunの`fetch`は既定のまま330秒の応答を待ち切ったので、`timeout: false`のような指定は付けていない。

## 代替案

- **Lambdaは`nodejs22.x`のまま、開発とCIだけBunにする**：テストと本番で処理系が分かれ、テストが本番の処理系を確かめない。
- **BunをLambdaレイヤーに置く**：zipは小さくなるが、デプロイ用のロールにレイヤーの権限が要り、Bunの版の置き場所も増える。
- **ローカルのサーバーをNitroの`bun`プリセットにする**：要求の本文の読み方などが、ブラウザーのテストを書いた`node-server`と変わる。本番は`aws-lambda`プリセットなので、ローカルのプリセットは変えずにBunで動かす。
- **PlaywrightだけNode.jsで動かす**：8e4e826の形で、開発環境にNode.jsが残る。

## 影響

- zipは約12 MBから約64 MB（展開後132 MB）になる。
- Bunは管理ランタイムと違いAWSが更新しない。更新は3か所の版とSHA-256を揃えて行う。
- AWSプロバイダーは関数の設定をコードより先に更新する。ランタイムの切り替えとzipの入れ替えを同じデプロイで行うと、新しいランタイムで古いzipを起動する時間ができる。ランタイムを切り替えるコミット（「フロントエンドのLambdaをBunのカスタムランタイムに切り替える」）の1つ前までを先にpushしてデプロイし、残りは次のpushにする。
- 戻すときは`infra/frontend/main.tf`の`runtime`を`nodejs22.x`にする。zipはそのまま動き、CIは両方のランタイムでzipを起動して確かめる。
- 急いで戻すときは、先に`aws lambda update-function-configuration --function-name judge-dev-web --runtime nodejs22.x`で切り替え（数秒で終わる）、そのあと`main.tf`を戻してpushする。
- zipは大きくなったが、エミュレーターでは初期化が速くなった。本番の値は切り替えの後でないと測れないので、CloudWatchの`Init Duration`と`Max Memory Used`を切り替え前と比べる。
