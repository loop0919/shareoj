# フロントエンド

Nuxt 4 と TypeScript で公開問題を表示する。
問題詳細は Go API から取得し、Nuxt サーバーで HTML を生成する（SSR）。

## ローカル起動

API とフロントエンドをまとめて起動する場合は、リポジトリのルートで `make dev` を実行する。
準備と停止方法は[ルートの README](../README.md#開発環境)を参照する。
以下は個別に起動する手順である。

Bun 1.4.4 と Go が必要になる。
Node.js は使わない。
リポジトリのルートで `nix develop .` を実行すると、共通の開発環境に入れる。
Bun の版は `package.json` の `packageManager`、ルートの `flake.nix`、`scripts/bun_release.py` で揃え、`scripts/bun_release.py` が一致を確かめる。

まず、ターミナルで API を起動する。

```console
cd api
go run ./cmd/api
```

別のターミナルで同じ開発環境に入り、フロントエンドを起動する。

```console
cd web
bun install --frozen-lockfile
cp -n .env.example .env
bun run dev
```

`.env` は Nuxt が `nuxt dev` と `nuxt build` で読み込む。
Bun 自身の `.env` の自動読み込みは `bunfig.toml` の `env = false` で止めている（Bun は値の `$` を展開するため）。

`http://localhost:3000/problems` で DB の公開問題一覧を表示できます。
公開問題の閲覧に認証や AWS の認証情報は不要である。

| 環境変数 | 用途 | 既定値 |
| --- | --- | --- |
| `NUXT_API_BASE_URL` | Nuxt サーバーから接続する Go API | `http://127.0.0.1:8080` |
| `NUXT_PUBLIC_SITE_URL` | canonical と OGP に使う公開オリジン | `http://localhost:3000` |

API の接続先はサーバー専用の設定であり、ブラウザーには渡さない。
公開 URL は信頼できる設定値から生成し、リクエストの Host ヘッダーを使わない。

## ページの URL

ページの URL では、UUID を 22 文字の base62 で表す（`shared/utils/short-id.ts`）。
API との通信は UUID のまま行い、ページが受け取った値は `fullId` で UUID に戻す。
コンテストの問題は出題順の記号で `/contests/<ID>/A` と表す。
問題記号は、コンテストの API が問題 ID を返す閲覧者だけが解決できる。

UUID を含む旧 URL は 301 で短い形へ転送する。
旧形式のコンテスト問題 `/contests/<ID>/problems/<問題ID>` は、出題順を引いて問題記号へ転送する。
開始前は出題順が変わりうるため、この転送だけ 302 にする。

## ガイド

使い方の説明は `/docs` にまとめる。
ページの一覧と並び順は `shared/docs.ts` に書き、本文は `app/content/docs/<slug>.md` に置く。
サイドバー、ガイドのトップ、フッター、サイトマップ、共有画像は `shared/docs.ts` から作る。
ページを足すときは、一覧に1件加えて本文の Markdown を置けばよい。

本文は問題文と同じレンダラーで表示する。
`## 見出し {#id}` の形の見出しが、ページ内の目次になる。
画面の幅が狭くても目次が1行に収まるように、見出しは短く書く。
4つ以上のバッククォートで囲んだ `example` ブロックは、書き方と表示例を並べて表示する。
ガイド間のリンクは `/docs/<slug>` と書く。

画面の操作は、赤枠と番号を付けたスクリーンショットで示し、本文では①②の番号で指す。
画像は `public/images/docs/<name>.png` に置き、`![説明](/images/docs/<name>.png)` で載せる。
画像は `docs-screenshots/guides.spec.ts` が、テスト用の架空のデータと本番の言語一覧で撮影する。
画面を変えたときは、ビルドしてから撮り直す。

```console
bun run build
bun run docs:screenshots
```

以前の `/blog/*-guide` と `/blog/contest-rules` は、対応するガイドへ 301 で転送する（`legacyDocPaths`）。

## SSR とエラー応答

`/problems/:id` は `useFetch` で Nuxt の `/api/problems/:id` を呼び出す。
Nuxt のサーバールートは Go の `/problems/:id` を取得し、応答の形式を検証する。
初回 HTML に問題文、制約、入出力例、ページ固有のタイトルと説明を含める。
取得済みデータは Nuxt のペイロードを通してブラウザーへ引き継ぐ。

存在しない問題には 404、API の接続失敗や不正な応答には 502 を返す。
エラーページには `noindex, nofollow` を付け、上流サービスの診断情報を表示しない。
本文の通常の文字列は Vue のテキスト展開で描画し、数式部分だけを KaTeX が生成した HTML と MathML で表示する。
API から受け取った HTML を直接挿入しない。

## 数式の記法

問題文、制約、入力形式、出力の説明、入出力例の解説では、`$...$` で文中数式、`$$...$$` で独立した数式を記述できる。

```text
整数 $A$ と $B$ の和を求めてください。
$0 \le A \le 10^9$
$$\sum_{i=1}^{n} i = \frac{n(n+1)}{2}$$
```

ドル記号をそのまま表示する場合は `\$` と書く。
JSON の文字列内ではバックスラッシュを `\\` とエスケープする。
入力形式は `$A \quad B$` のように記述し、複数行の形式では行ごとに数式を記述する。
数式の外側の改行はそのまま表示する。
入出力例のコード部分は数式に変換せず、元の文字列を表示する。
タイトルと SEO の説明文も通常のテキストとして扱う。

KaTeX の `renderToString` を SSR とブラウザーの両方で使用する。
CSS とフォントはアプリに同梱し、外部 CDN を必要としない。
未対応の命令、閉じ忘れた区切り、不正な数式は元の文字列を表示する。
`trust: false` で数式からのリンクや画像の挿入を無効にし、マクロの展開回数と数式の大きさに上限を設ける。

## 問題作成ページ

`/problems/new` で Markdown の編集とプレビューを行える。
エディターは画面の高さに収まり、ページ外側のスクロールを発生させない。
PC では編集・分割・プレビューを切り替え、本文の各ペイン内でスクロールできる。
表示切り替えは操作名付きのアイコンで行う。
行番号は本文の改行を数え、折り返しとスクロールに追従する。
左側のサイドバーに問題文・テストケース・生成・検証・判定・採点・解説・問題管理を並べる。
サイドバーはラベル付き表示とアイコンのみの表示を切り替えられる。
すべての画面幅でアイコンのみの折りたたみ状態から開始する。
狭い画面では、展開時に本文の上に重ねて表示する。
テストケースと解説は準備中として無効化している。
歯車の「問題管理」で本文エリア全体を管理画面に切り替える。「問題文」で編集を再開できる。
「採点」では採点方式（一括採点・部分点採点）、部分点採点の「集計コードで採点する」のスイッチ、グループの名前と配点、集計コードを編集し、ケースの所属は「テストケース」で選ぶ（ADR 0025）。
一括採点への切り替えと「集計コードで採点する」のオフは、グループや所属、集計コードが下書きから外れるなら確認してから行い、外した内容はページを開いている間、戻すと元に戻る。
管理画面の「テスターリンク」で招待リンクを発行し、コピーできる。
招待先の `/my/tester-invitations/<token>` では、ログイン済みユーザーが権限の説明を読んで「許可する」を押すとテスターになる。
未ログインの場合はログイン後に招待画面へ戻り、プロフィール未登録の場合も登録後に招待を続けられる。
テスターは非公開の問題文・解説・テストケースの閲覧と提出、問題の編集・削除・リジャッジ・非公開に戻す操作、テスターリンクの発行、自分のコンテストへの登録ができる。
公開と公開予約、定期便への応募は作者だけができる。
コンテスト編集画面の「コンテスト管理」では、共同作成者の招待リンクを発行・作り直しでき、オーナーは共同作成者を外し、共同作成者は自分で抜けられる。
招待先の `/my/contest-invitations/<token>` も、ログインとプロフィール登録のあとに招待画面へ戻る。
コンテスト編集画面の「参加形式」では、個人戦とチーム戦、チームの人数の上限、オンサイト区分と会場の合言葉を設定する。
チーム戦の概要ページでは「個人参加」と「チーム参加」を選び、チームのページ（`/contests/<id>/team`）でチームを作るか参加コードでチームに入る。
個人参加の人は開始前ならチームのページでチームに切り替えられ、会場の合言葉（英数字と `_` の4〜64文字）を入れるとオンサイト参加になる。
セッターは概要ページの「参加者の管理」で、会場の合言葉の差し替え、オープンの参加者とチームのオンサイトへの切り替え、チーム名の変更を行う。
リジャッジは「問題管理」、問題ページの提出一覧、提出結果の画面から依頼でき、問題削除は確認モーダルから実行できる。
本文横のガイドは別タブで開く。
スマートフォンでは編集とプレビューを切り替える。
書き方の説明と表示例は `/docs/markdown` にまとめ、エディターから移動できる。
見出し、箇条書き、表、リンク、画像、コードブロックを使い、問題文の構成を自由に決められる。
文中の `$...$` と独立した `$$...$$` に加え、次のコードブロックを利用できる。

````markdown
## 入力

```input
$N$
$A_1 \quad A_2 \quad \cdots \quad A_N$
```

## 合計

```math
\sum_{i=1}^{N} A_i
```
````

`input` は改行と数式以外の文字を保持し、`$...$` を数式に変換する。
`math` はブロック全体を LaTeX の数式として扱う。
通常のコードブロックとインラインコードでは数式を変換しない。
Markdown 内の生の HTML は文字列として表示し、実行可能な URL スキームを無効にする。

問題の作成・一覧表示・編集にはログインが必要。
未公開・コンテスト未登録の問題は、編集から約600 ms後にPostgreSQLへ自動保存し、「保存」ボタンでも保存できる。
公開済み・コンテスト登録済みの問題は自動保存せず、「保存」ボタンまたは保存ショートカットで保存する。
コンテストに登録した問題は、保存時に開始後も問題文・採点設定が反映され、受付済みの提出はリジャッジしたときだけ採点し直す。
ただし開始後は、採点方式、集計コードで採点するかどうか、グループの名前・配点・順番を変える保存をAPIが拒む。
マイページは「自分の問題」と「テスト中の問題」を分け、テスター参加後は `/my?tab=testing` に移動する。
自分の問題は `/my?tab=problems`、記事は `/my?tab=posts` で一覧表示する。
新規作成は `/problems/new?fresh=1`、再編集は `/problems/new?problem=<id>` を使う。
ブラウザー単独の下書き保存・一覧・移行機能は提供しない。
旧localStorageデータは参照せず、自動削除もしない。
DBから取得・保存した内容だけを、ユーザーID・問題IDごとのsessionStorageキャッシュに保持する。
再編集では必ずDBの内容とアクセス権を確認し、キャッシュだけでは編集を許可しない。
キャッシュを削除してもDBの問題は失われず、キャッシュが使えなくても保存できる。
DB保存の競合・通信失敗では自動保存を止め、入力内容を画面に保持する。
未保存の変更がある状態でページを移動すると、編集を続ける・保存せずに移動・保存して移動を選ぶ。
保存失敗時は移動せず、再読み込みやタブを閉じる操作にはブラウザー標準の確認を使う。
問題の公開と採点用テストケースの登録は未対応。

## ログインとセッション

`/login`はCognitoのメールアドレスとパスワードによるログインを行う。
初回パスワード変更、SMS、認証アプリとメールの確認コードに対応する。
`/signup`でメールアドレスとパスワードを登録し、届いた確認コードでメールアドレスを確認してからログインできる。
確認コードの再送と、ページを閉じた後の確認再開にも対応する。
パスワードと確認コードをURL、localStorage、sessionStorageへ保存しない。
パスワード再設定の画面は未実装である。

NuxtはアクセストークンとリフレッシュトークンをHttpOnly、SameSite=LaxのCookieに保存し、ブラウザーへトークンをJSONで返さない。
公開URLがHTTPSならSecure属性も付ける。
`NUXT_PUBLIC_SITE_URL`は実際にブラウザーがアクセスするオリジンと一致させる。
Cookieを使う書き込み要求はOriginを検証し、NuxtからGo APIへBearerトークンを渡す。
アクセストークンのCookieが期限切れ、または認証付きAPIが401を返した場合、サーバーがトークンを更新して要求を最大1回再試行する。
リフレッシュトークンのCookieはCognitoの設定に合わせて30日間保持し、実際の有効期限はCognitoが検証する。
更新用トークンが無効ならセッションを削除し、通信障害時はCookieを保持する。
ログアウトは両方のCookieを削除する。トークンの失効APIは呼び出さない。
Googleログインが有効な場合は、続けてCognitoの`/logout`を経由して`/login`へ戻る。
アカウント削除も同じくCognitoの`/logout`を経由し、トップページへ戻る。
Cognitoは自分のドメインにもログイン状態のCookieを持ち、残っていると次のGoogleログインがGoogleを経由せず同じアカウントで完了するためである。
戻り先の`<公開URL>/login`と`<公開URL>/`は、Cognitoのアプリクライアントの`logout_urls`に登録する。
変更前にログイン済みの場合は、再ログインすると自動更新が有効になる。
編集中に期限が切れた場合は、別のタブで再ログインし、元の画面で保存を再試行できる。

## ビルドとテスト

以下は `web/` で実行する。

```console
bun run typecheck
bun run build
bun --bun playwright install chromium
bun run test
```

Linux でブラウザーの共有ライブラリが不足する場合は `bun --bun playwright install --with-deps chromium` を使う。
Playwright のコマンドには `--bun` を付ける。
付けないと、PATH に Node.js があればそちらで動く。
ブラウザーのテストは `bun run test` で実行する。
`bun test` は Bun のテストランナーで、`lambda/` と `scripts/` の Bun のテストだけを実行する。
テストは Go API とビルド済み Nuxt を自動起動し、HTML 本文、SEO メタデータ、404 と 502、JavaScript 無効時の閲覧、画面遷移、画面幅 320 / 375 / 414 / 768 / 1280 px での表示を検証する。
テスト用にポート13000、13001、18080、18081を使用する。

実DBを使うブラウザーテストは`TEST_DATABASE_URL`を設定して実行する。

```console
bun run test --config playwright.account.config.ts
```

このテストはポート13002と18082を使う。
Cognitoの代わりにテスト用の署名鍵でトークンを発行し、実際のJWT検証、Go API、Nuxt、PostgreSQLを通して保存と再編集を確認する。
テスト用の認証処理はGoの`_test.go`だけに含まれ、本番バイナリには入らない。

CIではSSRを一度ビルドし、画面テストと実DBテストをそれぞれ複数のジョブに分けて実行する。
ジョブの数は`.github/workflows/ci.yml`で決める。
分け方は`scripts/shard-tests.mjs`が`tests/durations.json`の所要時間から決め、どのジョブもほぼ同じ時間で終わるようにする。
所要時間の記録がないテストは中央値として数えるため、追加したテストもどれかのジョブで必ず実行される。
実DBテストはジョブごとに独立したPostgreSQLを使い、同じDB内では1ワーカーで実行する。
同じファイルのテストが別のジョブに分かれることがあるため、実DBテストは1件だけでも成功するように書く。
Go APIのテスト用サーバーは一度だけビルドし、各ジョブで使い回す。
CIのwebのジョブは、失敗する`node`（`scripts/no-node/node`）をPATHの先頭に置き、Node.jsへの取りこぼしを検出する。
すべてのジョブが成功した場合だけ`Test SSR frontend`が成功し、デプロイへ進む。
自動リトライは行わない。

テストが増えてジョブの時間が偏ってきたら、手元で全体を実行して所要時間を記録し直す。

```console
PLAYWRIGHT_JSON_OUTPUT_FILE=.build/browser.json bun run test --reporter=json
PLAYWRIGHT_JSON_OUTPUT_FILE=.build/account.json bun run test --config playwright.account.config.ts --reporter=json
bun scripts/shard-tests.mjs record .build/browser.json .build/account.json
```

CIの1ジョブを手元で再実行する場合は、CIと同じ数で分け方を作り、対象の番号のリストを渡す。

```console
bun scripts/shard-tests.mjs split playwright.config.ts 16 .build/shards/browser
bun run test --test-list .build/shards/browser/1.txt
```

失敗したジョブは、`playwright-browser-1`などの名前でHTMLレポートとトレースを7日間保存する。
アーティファクトを展開し、`bun --bun playwright show-report <展開先>/playwright-report`で開く。
トレースは`bun --bun playwright show-trace <trace.zipのパス>`で開く。

手動では、開発サーバー起動後に HTML を確認できる。

```console
curl -i http://localhost:3000/problems
curl -i http://localhost:3000/problems/missing
```

## 本番起動

SSR には Nuxt サーバーを実行する環境が必要になる。
`bun run build` 後、接続先と公開 URL を指定して起動する。

```console
NUXT_API_BASE_URL=https://api.example.com \
NUXT_PUBLIC_SITE_URL=https://judge.example.com \
bun run start
```

ビルド済みサーバーは `.env` を読み込まないため、実行環境で変数を設定する。
`web/` の外から `bun .output/server/index.mjs` のように起動する場合は、Bun が `.env` を読まないよう `--no-env-file` を付ける。
本番の Lambda は `provided.al2023` で動き、ZIP に同梱した Bun が `lambda/runtime.mjs` からハンドラーを呼ぶ（[ADR 0035](../docs/adr/0035-run-frontend-on-bun.md)、[デプロイ手順](../infra/README.md#フロントエンドのデプロイ)）。
AWSへのデプロイとは別に、[ローカルC++提出](../docs/judge/local-cpp.md)を利用できる。
Go API のカタログには、ユーザーが公開した問題を表示します。

## 参照

- [Nuxt のデータ取得](https://nuxt.com/docs/4.x/getting-started/data-fetching)
- [Nuxt の SEO とメタデータ](https://nuxt.com/docs/4.x/getting-started/seo-meta)

Googleログインの外部サービス設定と環境変数は [設定手順](../docs/google-login.md) を参照してください。

作問 CLI の `shareoj login --google` も同じ設定を使います。
Web の配布後、登録済みコールバックから CLI のループバック受信先へ署名付き認証コードを渡し、`POST /api/auth/cli/exchange` で PKCE を確認してトークンへ交換します。
クライアントシークレットは Web サーバーに保持し、CLI には渡しません。
操作方法は [CLI の README](../cli/README.md) を参照してください。

## マイページと初回プロフィール登録

`/my` にユーザーID・アイコン・登録月・作成した問題を表示する。
`/my/settings` でユーザーIDとアイコンを変更できる。
Googleログインとメール・パスワードログインのどちらも、プロフィールがなければ `/onboarding` でユーザーIDを登録する。
既存ユーザーも同じ導線を通り、Cognitoの内部IDに紐づいた問題を引き続き利用できる。
ユーザーIDは英字で始まる3〜20文字の英小文字・数字・アンダースコアで、DBで重複を防ぐ。
アイコンは任意で、未設定時はユーザーIDの頭文字を表示する。
PNG・JPEG・WebP（5MB以下）をブラウザーで中央から正方形に切り抜き、128pxのPNGに変換して送信する。
プロフィールが未登録の状態では、API側でも問題の作成・取得・更新・削除を拒否する。
プロフィールは本人のマイページ向けで、公開プロフィールURLやランキングはまだ提供しない。

## 問題と記事の公開

`/problems` は DB に公開した問題を表示します。初期状態では空です。
問題作成画面の上部の「公開・応募」で投稿か定期便の応募かを選び、投稿ページ（`/problems/post`）か応募ページ（`/featured/apply`）へ移る。
問題一覧の「問題投稿」も投稿ページを開き、旧URLの`/problems?post=1`は`/problems/post`へ301で転送する。
公開中の問題は「保存」で公開内容に反映し、「問題管理」の「非公開に戻す」で一覧と詳細ページから取り下げられる。

「ブログ」では公開記事をログインせずに閲覧できます。
ログインとユーザー ID 登録を終えると「記事を書く」から Markdown・数式で記事を作成し、プレビュー・DB 保存・公開できます。
マイページの「自分の記事」から再編集できます。記事も保存と公開を分けています。
記事エディターの「解説する問題」に問題のURLかIDを追加すると、記事の公開後に問題の解説タブへユーザー解説として載ります。
問題の解説タブの「ユーザー解説を書く」からは、その問題を指定した状態で記事を書き始められます。

問題エディターの「問題設定」と記事エディターに、タグの入力欄があります。
入力すると、公開中の問題と記事で使われているタグを候補に出します。
大文字と小文字、全角と半角、カタカナとひらがなの違いを無視して照合し、同じタグがあればEnterでその表記を選びます。
カンマか「、」で区切ると、まとめて追加できます。
問題ページ・記事ページのタグと、一覧の「タグで絞り込む」から、`/problems?tag=`と`/blog?tag=`の一覧を開けます。
絞り込み中は検索欄にタグが入り、横の×で解除します。問題一覧の検索欄は、タグを表示しているときだけ出します。
問題のタグは解法の手がかりになるため、初期状態では表示しません。問題一覧と問題ページの「タグを表示」で、そのページだけ表示に切り替えます。選んだ表示は保存せず、ページを開くたびにオフから始まります。未ログインでも使えます。
エディターのタグ欄は、広い画面ではタイトルの右に置きます（問題エディターは1200px以上、記事エディターは960px以上）。
定期便の開催中の問題とコンテストの問題には、スイッチに関係なくタグを表示しません。
運営表示の設定は API の `OPERATOR_SUBJECTS` を参照してください。

### Link previews

Public pages expose Open Graph and Twitter Card metadata during SSR. `/og/<page-path>.png`
returns a 1200×630 PNG with the ShareOJ mark and the page title (`/og/index.png` for home).
The renderer uses `@resvg/resvg-wasm` and the bundled, unmodified IPAex Gothic font;
the font license is in `server/assets/fonts/LICENSE.txt`. Nuxt stages the WASM under
`.build/og-renderer` and Nitro bundles both assets for the server and Lambda.

Dynamic images fetch only the public API, never session cookies or private fallbacks.
Private and pre-start contest problems have no image preview. Responses use `no-store`
to recheck publication on each fetch; X and Discord may independently cache previews.
The fixed pages' image titles live in `shared/social-pages.ts`, whose paths are also the
sitemap's entry points; guide titles come from `shared/docs.ts`. After deployment, verify
a public URL with the [Discord Embed Debugger](https://discord.com/developers/embeds).

## 作問とデータ取得の責務

- `app/composables/useProblemDraft.ts` が作問の復元・保存・競合回復・キャッシュ・公開・削除・離脱確認を管理する。ページは編集欄とプレビューを担当する。
- API の共有スキーマは `shared/types/` に置き、画面のエラー表示や編集補助は `app/utils/` に置く。`server/` と `shared/` から `app/` を参照しない。
- 一覧取得は `useLatestRequest` で同じキーの同時実行をまとめ、条件変更や破棄後の古い応答を無視する。取得条件を変えたらキーを変え、取得をやめる場合は `invalidate()` を呼ぶ。
- `usePolling` は取得完了後に次のタイマーを設定し、非表示タブでは取得を控え、スコープ破棄時に終了する。取得エラーの表示は呼び出し側が担当する。
- サーバーのセッション処理は `private-session.ts`、Origin・ボディ検証と非公開レスポンスヘッダーは `private-request.ts`、上流 API 通信は `private-api.ts` が担当する。

保存の機能テストは `data-save-state`、API への書き込み、再読み込み後の値を検証する。
表示文言は `tests/problem-editor-copy.spec.ts` で検証し、保存の成否判定と分ける。

## 定期便

`/featured`に次回予告または公開中の問題、Easy／Hard別の待ち件数、過去の出題を表示する。
次回予告のwriterとtesterには担当する枠に目のアイコンを出し、押したときだけ`GET /api/my/featured/next`から問題名を取得して、問題ページへのリンクで表示する。
表示は覚えず、ページを開くたびに隠した状態から始める。
応募状態と取り下げはマイページの「自分の問題」にまとめる。
問題の状態（×準備中、✅準備中、応募中（コンテスト）、応募中（定期便）、公開中）と足りない項目は、APIが返す`readiness`をそのまま表示する。
公開、コンテスト、定期便の条件は画面側に持たず、`app/utils/problem-readiness.ts`は理由のコードを文言と編集画面のセクションに対応づけるだけにする。
各回の「順位表」から`/featured/standings?at=<scheduledAt>`へ移動できる。
公開から翌22時までの提出を集計し、Hardの点数、同点ならEasyの点数で順位を決め、同じ点数は同順位にする（[ADR 0033](../docs/adr/0033-rank-featured-by-scores.md)）。
部分点の問題は点数を示し、満点にはチェックマークを並べる。部分点のない問題は正解をチェックマークだけで示す。
経過時間と不正解数を併記し、誤答ペナルティはなく、同順位内はその点数に到達した提出が早い順に表示する。
「新作を応募」は`/featured/apply`を開き、保存済みの未公開問題と公開希望を選ぶ。
選んだ問題と希望を`GET /api/my/featured/schedule`へ渡し、その問題が入る見込みの回を予定表に示す。
`/featured/schedule`は次回から8回分の予定を作者名つきで表示し、2回目以降は見込みと明記する（[ADR 0020](../docs/adr/0020-post-and-apply-on-dedicated-pages.md)）。
応募の取り下げはマイページの「自分の問題」で行う。
応募条件と選出方法は[ADR 0011](../docs/adr/0011-schedule-featured-problems.md)に記載する。
新作の問題画面はAPIの`editorialHidden`に従い、解禁日時を表示する。
解禁前も「自分の提出」と通常の提出フォームは利用できる。
