# API

Goで実装するAPIと非同期Lambdaのためのモジュールである。

## Interactive API documentation

Swagger UI is served at `/docs` (also `/docs/`), with OpenAPI 3.1 JSON at
`/openapi.json`. After deployment, these are available at
<https://api.share-oj.net/docs> and <https://api.share-oj.net/openapi.json>.
Locally, use <http://localhost:8080/docs>.

The public read APIs are documented; account, authentication, and write APIs are
outside this document's scope. **Try it out** sends GET requests to the same API
origin, so the same page works locally and in production.

The specification is generated at startup from public route registrations in
`internal/httpapi/routes.go` and their actual Go response types, using Huma.
There is no handwritten OpenAPI JSON/YAML or separate generation command.
When adding a public route, supply its summary, response type, and query parameters
in `publicOperation`. JSON field names, types, nested models, and optional fields
are derived automatically. Descriptions and publication rules remain explicit
route metadata; generation does not infer business rules from handler code.
Use the same typed response envelope in the handler and route metadata.
The `hidden` schema tags on submission diagnostics reflect fields stripped from
public responses; they do not change JSON serialization or access control.

Swagger UI 5.33.0 assets are loaded from UNPKG with pinned versions and integrity
hashes. The spec and page are served by the Go binary, including Lambda builds.
The UI needs CDN access; `/openapi.json` remains available without it. External
Swagger validation is disabled. Run `go test ./...` to check generated schemas,
HTTP routes, and API Gateway response handling.

## 開発環境

APIとinfraは、リポジトリのルートにあるNix開発シェルを共用する。
初回はルートでdirenvを許可する。
以下は、この`api/`ディレクトリから実行する。

```console
cd ..
direnv allow
cd api
```

direnvを使わない場合は、ルートで開発シェルを起動してからAPIへ移動する。

```console
cd ..
nix develop .
cd api
```

開発シェルにはGo、gopls、gofumpt、golangci-lint、AWS CLI、Terraform、zipが含まれる。
GoはCIと同じく`go.mod`の`toolchain`行のバージョンを使う。
開発シェルのGoが古ければ、初回のコマンドで公式のtoolchainを取得し、チェックサムデータベースで検証する。
以降のコマンドは`api/`ディレクトリで実行する。

## ローカル起動

API とフロントエンドをまとめて起動する場合は、リポジトリのルートで `make dev` を実行する。
準備と停止方法は[ルートの README](../README.md#開発環境)を参照する。

開発シェルでAPIを起動する。

```console
go run ./cmd/api
```

既定では`http://localhost:8080`で待ち受ける。
ポートを変更する場合は`PORT`を指定する。

```console
PORT=3000 go run ./cmd/api
```

別のターミナルからヘルスチェックを呼び出し、起動を確認できる。

```console
curl -i http://localhost:8080/health
```

正常時は`200 OK`と次のJSONを返す。

```json
{"status":"ok"}
```

## 公開問題API

`GET /problems` は公開済み問題の一覧、`GET /problems/{id}` は公開済み本文と制限値を返します。
認証は不要です。存在しない問題と非公開の問題は `404` を返します。
未公開の下書きは、保存した後に公開します。
公開中の問題は、保存すると公開内容も更新されます。
C++の提出と採点は[ローカル開発用](../docs/judge/local-cpp.md)として利用できます。

```console
curl -i http://localhost:8080/problems
```

Nuxtからの取得とSSRの確認方法は[フロントエンドの手順](../web/README.md)を参照する。

### Public sample cases

`GET /problems/{id}/samples` returns the cases marked as samples in the published
snapshot, in their saved order. No authentication is required. The website exposes
the same response at `GET /api/problems/{id}/samples`:

```sh
curl --fail "https://www.share-oj.net/api/problems/$PROBLEM_ID/samples"
```

```json
{"items":[{"name":"sample_1","input":"3 5\n","output":"8\n"}]}
```

Input and output retain whitespace and trailing newlines. An empty sample set is
`{"items":[]}`. Missing or unpublished problems return `404`, even for their author.
Unpublished edits and cases not marked as samples are never returned. Markdown-only
examples are not extracted; authors must register them as sample cases.
Contest problems become available here after they are released as public problems.

For externally stored input or output, `inputFile` or `outputFile` replaces the
corresponding text as the source of file contents:

```json
{"items":[{"name":"large","input":"","output":"ok\n","inputFile":{"url":"https://...","size":100000,"sha256":"<64 lowercase hex characters>"}}]}
```

Download that URL as bytes and verify its size and SHA-256. URLs expire after ten
minutes; fetch the sample list again to renew them. Already issued URLs remain valid
until expiration if a problem is unpublished. API responses use `Cache-Control: no-store`.
Storage failures return `503` from Go (`502` through the website), without partial results.

## ログインAPI

独自のログイン画面からCognito User Poolを使って認証する。
APIはパスワードを保存せず、Cognitoの`InitiateAuth`へ渡す。
接続には`AWS_REGION`と`COGNITO_CLIENT_ID`を指定する。
シークレット付きのアプリクライアントでは`COGNITO_CLIENT_SECRET`も指定する。
シークレットはサーバーだけに設定し、ブラウザーへ渡さない。

```console
AWS_REGION=ap-northeast-1 COGNITO_CLIENT_ID=your-client-id go run ./cmd/api
```

Cognito側には以下の設定が必要である。
`infra/api/auth.tf`がUser Poolとシークレット付きアプリクライアントを作成し、Lambdaの環境変数へ接続情報を渡す。
Terraformを適用した環境では、これらを手動で設定する必要はない。
既存のUser Poolへローカルから接続する場合は、以下の設定を確認する。

- メールアドレスでログインする場合は、User Poolのサインイン属性にメールアドレスを指定する。
- アプリクライアントで`ALLOW_USER_PASSWORD_AUTH`を有効にする。
- `PreventUserExistenceErrors`を`ENABLED`にする。
- 動作確認用のユーザーを作成する。仮パスワードの場合は追加認証で本パスワードを設定する。
- Lambdaにも`COGNITO_CLIENT_ID`と、必要なら`COGNITO_CLIENT_SECRET`を設定する。`AWS_REGION`はLambdaが設定する。

`InitiateAuth`と`RespondToAuthChallenge`はIAM認証を使わないため、この処理用のIAM権限やローカルのAWS認証情報は不要である。
詳細は[AWSのInitiateAuth仕様](https://docs.aws.amazon.com/cognito-user-identity-pools/latest/APIReference/API_InitiateAuth.html)を参照する。

### メールアドレスとパスワードによるログイン

`POST /auth/login`へJSONを送る。
`username`にはUser Poolで許可したサインイン属性（メールアドレスなど）を指定する。

```http
POST /auth/login
Content-Type: application/json

{"username":"user@example.com","password":"your-password"}
```

認証が完了すると`200 OK`でトークンを返す。
`expires_in`はアクセストークンの有効期間を秒で表す。

```json
{
  "access_token": "...",
  "id_token": "...",
  "refresh_token": "...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

初回パスワード変更やMFAが必要な場合も`200 OK`だが、トークンの代わりに`challenge_name`と`session`を返す。
画面側は`challenge_name`があれば追加認証画面を表示し、ログイン完了として扱わない。

```json
{
  "challenge_name": "NEW_PASSWORD_REQUIRED",
  "challenge_parameters": {},
  "session": "..."
}
```

### 追加認証

`POST /auth/challenge`へ直前の応答の`session`と入力値を送る。
`challenge_parameters.USER_ID_FOR_SRP`が返された場合は、その値を`username`に使う。
それ以外はログイン時の`username`を使う。

```http
POST /auth/challenge
Content-Type: application/json

{
  "username": "user@example.com",
  "challenge_name": "NEW_PASSWORD_REQUIRED",
  "session": "...",
  "responses": {"NEW_PASSWORD": "your-new-password"}
}
```

| challenge_name | responsesに指定するキー |
| --- | --- |
| `NEW_PASSWORD_REQUIRED` | `NEW_PASSWORD`。必須属性があれば`userAttributes.email`なども指定する |
| `SMS_MFA` | `SMS_MFA_CODE` |
| `SOFTWARE_TOKEN_MFA` | `SOFTWARE_TOKEN_MFA_CODE` |
| `EMAIL_OTP` | `EMAIL_OTP_CODE` |

追加認証の応答形式はログインと同じである。
別の追加認証が返った場合は、新しい`session`で続ける。
上記以外のチャレンジには未対応であり、MFAの新規登録や認証方式の選択が必要なUser Poolでは、その処理を別途実装する必要がある。

### エラーと対象範囲

エラーは`{"error":"invalid_credentials"}`の形式で返す。
AWSの内部エラーメッセージやパスワードは応答やログへ出力しない。
認証応答には`Cache-Control: no-store`を付ける。

| HTTPステータス | 意味 |
| --- | --- |
| `400` | 入力不正、パスワードポリシー違反、未対応のチャレンジ |
| `401` | 認証失敗、確認コード不正または期限切れ |
| `413` | リクエスト本文が16 KiBを超えている |
| `415` | Content-Typeがapplication/jsonではない |
| `429` | Cognitoの試行回数制限。時間を空けて再試行する |
| `502` | Cognitoとの通信失敗や想定外の応答 |
| `503` | Cognitoの接続設定がない |

ユーザー未登録、パスワード不一致、登録未確認、パスワードリセット必須は同じ`invalid_credentials`を返す。
`POST /auth/refresh`は`refresh_token`を受け取り、Cognitoの`GetTokensFromRefreshToken`でトークンを更新する。
Go APIにはパスワード再設定は含まれない。
ブラウザー向けのCookie管理とログアウトはNuxtの`/api/auth/*`が担当する。
本番のログイン通信はHTTPSを使う。
`/auth/me`と`/my/problems`はアクセストークンの署名、有効期限、発行元、`client_id`と`token_use=access`を検証する。
Cognitoの公開鍵は設定したUser PoolのJWKSから取得し、所有者には検証済みの`sub`を使う。
既存の`GET /health`は公開エンドポイントのままである。

## サインアップAPI

Cognitoのセルフサインアップを使い、登録したメールアドレスへ確認コードを送る。
`infra/api/auth.tf`で`allow_admin_create_user_only = false`とメールのコード認証を設定している。
既存のUser PoolにもこのTerraform変更を適用する必要がある。
登録画面は`/signup`、メール確認後のログイン画面は`/login`である。

| API | JSON本文 | 成功応答 |
| --- | --- | --- |
| `POST /auth/signup` | `email`、`password` | `{"confirmed":false}`（メール確認が必要） |
| `POST /auth/confirm-signup` | `email`、`code` | `{"confirmed":true}` |
| `POST /auth/resend-confirmation` | `email` | `{"sent":true}` |

ログインと同じCognitoクライアント設定を使う。
シークレット付きクライアントには、正規化したメールアドレスから計算した`SecretHash`を送る。
AWSの管理者APIは使わず、追加のIAM権限を必要としない。
パスワードをDBへ保存せず、登録とメール確認だけではログインCookieを発行しない。
Cognitoのトリガーなどで自動確認された場合、登録応答の`confirmed`は`true`になる。

パスワードは12文字以上で英大文字、英小文字、数字、記号を含み、空白を含まないものを受け付ける。
メールアドレスは128バイト、パスワードは256バイト、確認コードは2048バイト、JSON本文は16 KiBまでとする。
CognitoでもUser Poolのパスワードポリシーを検証する。
登録時に受け付けるユーザー属性はメールアドレスだけであり、確認済みフラグや所有者IDは指定できない。

メールアドレスが登録済みの場合も、通常の未確認登録と同じ応答を返す。
再送では、存在しないユーザーや確認済みのユーザーに対しても同じ応答を返し、登録状況を公開しない。
したがって`sent: true`はメール配送の成功を保証する値ではない。
既存アカウントからメールアドレスを移す操作は行わない。

誤った確認コードと期限切れには400、試行回数制限には429を返す。

確認メールを送る登録と再送は、利用者のネットワークとメールアドレスごとに回数を制限する。
ネットワークは連続20回まで、3分ごとに1回分回復する。同じNATの教室からでも登録できる値にしている。
メールアドレスは連続3回まで、20分ごとに1回分回復する。
どちらかが上限なら、どちらも消費せずに`Retry-After`付きの429を返す。
確認コードの照合はメールを送らないため数えない。
DBにはIPとメールアドレスのSHA-256だけを保存し、回復しきった行は次の要求で削除する。

ネットワークはIPv4ならアドレス、IPv6なら`/64`である。
直接の呼び出しではAPI Gatewayの送信元IPを使う。
Nuxt経由では送信元がNuxtのLambdaになるため、Nuxtが`X-ShareOJ-Client-IP`に利用者のIPを入れる。
あわせて、Cognitoのクライアントシークレットで`shareoj-client-ip:<IP>`を署名したHMAC-SHA256を`X-ShareOJ-Client-IP-Signature`に入れる。
署名が合わない場合は送信元IPで数える。
Nitroは要求ハンドラーにLambdaのイベントを渡さないため、`web/lambda/index.mjs`がAPI Gatewayの`sourceIp`をヘッダーへ写す。
同名のヘッダーを利用者が送っても上書きする。
メール送信など上流サービスの失敗は502、セルフサインアップが無効の場合は503を返す。
確認コードが届かない場合や登録要求の応答を受け取れなかった場合も、画面からメールアドレスを入力して確認と再送を再開できる。

## PostgreSQLへの問題保存

`DATABASE_URL`を設定するとPostgreSQLへ非公開の問題を保存できる。
認証にはログイン用の設定に加えて`COGNITO_USER_POOL_ID`が必要になる。
接続プールはプロセスあたり最大4接続とし、API起動時にはスキーマを変更しない。
本番の接続先、TLS、ネットワークと資格情報はデプロイ環境で設定する。
今回の変更にはAWS上のDB作成やデプロイは含まれない。

`make dev`は接続先が未設定の場合に開発用DBを作成し、マイグレーションを適用する。
外部DBを指定した場合は、対象の`DATABASE_URL`を環境変数へ読み込んだうえで、`api/`から次を明示的に実行する。

```console
go run ./cmd/migrate
```

マイグレーションはトランザクション内で実行し、適用済みのバージョンを記録する。
同時実行はPostgreSQLのアドバイザリロックで直列化する。
DB接続先のURLや資格情報はエラー応答へ含めない。

| API | 動作 |
| --- | --- |
| `GET /auth/me` | 検証済みユーザーのIDを取得 |
| `GET /my/problems` | 自分の問題を更新日時の降順で50件取得。部分点の問題には`partial`と`fullScore`（満点）を付ける |
| `GET /my/problems?role=tester` | テスト中の問題を更新日時の降順で50件取得。コンテストのセッターとしてだけ扱える問題には`viaContest`（`id`と`title`）を付ける |
| `GET /my/problems/{id}` | 作者またはテスターとして問題の本文と更新番号を取得。コンテストのセッターとして扱える人は`contestEditors`に返す |
| `PUT /my/problems/{id}` | UUID v4のIDを指定して作成または更新 |
| `DELETE /my/problems/{id}?version=1` | 指定した更新番号の問題を削除 |
| `POST /my/problems/{id}/tester-invitation` | 作者または招待されたテスターが招待トークンを発行。同じ問題では同じトークンを返す。コンテストのセッターとしてだけ扱える人には404 |
| `GET /my/tester-invitations/{token}` | 問題名、作者名、参加済みかどうかを取得 |
| `POST /my/tester-invitations/{token}` | ログイン中のユーザーをテスターに登録。問題が入っているコンテストに参加登録していれば、結果確定まで409 `contest_participant_locked` |

すべての要求に`Authorization: Bearer <access_token>`を付ける。
ユーザーIDはリクエスト本文から受け取らず、検証済みトークンから取得する。
問題の取得、編集、公開、削除、非公開問題への提出、テストケースの操作は、作者と登録済みテスターに許可する。
公開済みのコンテストに入っている問題は、そのコンテストのセッターも、結果確定までテスターと同じく扱える（[ADR 0022](../docs/adr/0022-share-contests-among-setters.md)）。
アクセス権のない問題と存在しない問題は同じ404を返す。
招待の閲覧だけでは権限を付与せず、POSTでの同意時に登録する。
参加済みユーザーの再送は成功として扱い、作者はテスター一覧に追加しない。
招待リンクは複数人で利用でき、問題を削除すると登録とリンクも削除される。
リンクの失効期限、再発行による無効化、テスターの解除は未対応。
公開問題APIから下書きを取得することはできない。

PUTの本文は次の形式とする。
新規作成には`version: 0`を、更新には直前の取得または保存応答の`version`を指定する。
空のタイトルと本文も下書きとして保存できる。

```json
{
  "version": 0,
  "draft": {
    "title": "A + B",
    "markdown": "## 問題文\nA + B を求めてください。",
    "timeLimitMs": "2000",
    "memoryLimitMb": "512"
  }
}
```

保存と詳細取得は`id`、`version`、`updatedAt`、`draft`を返す。
古い更新番号による保存と削除は409を返し、既存の変更を上書きしない。
タイトルは120文字、本文は100,000文字、JSON本文は3 MiBまでとする。
時間制限は100〜5,000 msの100 ms刻み、メモリ制限は64〜512 MiBの整数を受け付ける。

問題、コンテスト、記事の新規作成には、アカウントごとに種類別の独立した作成枠を設ける。
各枠は初回20件分で、2時間に1件分回復し、最大20件分まで貯まる。
新規作成がDBに保存されたときだけ1件分消費し、既存の編集や公開、保存失敗では消費しない。
削除しても作成枠は返さない。
枠が足りない場合は429と`{"error":"creation_quota_exceeded"}`を返し、`Retry-After`ヘッダーに次の1件分が回復するまでの秒数を設定する。
作成枠とコンテンツを同じPostgreSQLトランザクションで更新し、複数APIインスタンスからの同時作成にも適用する。
導入前のコンテンツは消費に算入しない。

一覧は`items`（`id`、`title`、`updatedAt`）と`nextCursor`を返す。
`nextCursor`が空でなければ、次の要求の`cursor`パラメーターへ渡す。
ページを取得する間にも更新は行われるため、一覧全体のスナップショットは保証しない。

未認証または無効なトークンには401、認証やDBが未設定の場合とDB処理に失敗した場合には503を返す。
非公開APIの応答には`Cache-Control: no-store`を付ける。

`POST /my/submissions`は、認証済みアカウントごとに、通常提出・コンテスト提出を合算して直近60秒で2回まで受理する。
サンプル実行は独立した枠で直近60秒で3回まで受理し、通常ジャッジの枠を消費しない。
入力生成・出力生成・入力検証も独立した枠で、合算して直近60秒で3回まで受理する（ADR 0032）。
いずれも受理後のジャッジ結果にかかわらずカウントする。
超過時は429と`{"error":"submission_rate_limited"}`を返し、`Retry-After`ヘッダーに再試行までの秒数を設定する。
提出前の入力エラーや保存失敗は枠を消費しない。
PostgreSQLのアカウント単位のトランザクションロックにより、複数APIインスタンスからの同時提出にも適用する。

`POST /my/code-runs`は問題なしでソースを標準入力で実行し（ADR 0031）、`GET /my/code-runs/latest`は最後の1件を返す。
受付は提出・サンプル・作問の枠とは別に、アカウントごとに直近60秒で3回まで。入力エラーは数えず、超過時は429と`{"error":"code_run_rate_limited"}`、`Retry-After`を返す。

## 確認

```console
go test ./...
golangci-lint run
```

実DBのテストは、専用の接続先を`TEST_DATABASE_URL`へ設定して`go test -race ./...`を実行する。
テストは一意なスキーマを作成して終了時に削除し、永続化、所有者の分離、競合と一覧のページングを検証する。
`TEST_DATABASE_URL`が未設定なら実DBのテストはスキップする。
CIは時間のかかる`internal/httpapi`のテストを2台のランナーに分け、それぞれ3プロセスで、ほかのパッケージと並べて実行する。
1台で4プロセスより増やしても、同じランナーのDBサーバーとCPUを取り合い、速くならなかったためである。
手元では`scripts/parallel-test.sh ./internal/httpapi 4`で、1台に分けて実行できる。

Lambda用バイナリでは、ビルド時に`CGO_ENABLED=0`と対象アーキテクチャを指定する。

## ユーザープロフィール

`GET /my/profile` はログインユーザーの `{ "profile": null }` またはプロフィールを返す。
`PUT /my/profile` は `{ "handle": "coder_01", "avatar": "", "version": 0 }` を受け取り、ユーザーID・アイコンを登録する。
更新時は取得した `version` を指定する。
`handle` は小文字に正規化し、英字で始まる3〜20文字の英小文字・数字・アンダースコアに制限する。
重複は `409 handle_taken`、古い更新番号は `409 profile_conflict` となる。
`avatar` は空文字かPNGのdata URLを受け取り、最大256×256px・180,000文字に制限して、サーバーで再エンコードする。
元画像や任意URL・SVGは保存しない。
リクエスト全体は200KiBまでで、所有者はアクセストークンの `sub` から決定する。
ユーザーIDを変更してもCognito内部IDと問題の所有権は変わらない。
プロフィール未登録時、`/my/problems` は `403 profile_required` を返す。

`deleted_` と16進数11桁からなるユーザーIDは退会済みアカウント用に予約し、`400 invalid_profile` とする。

### ログイン方法・パスワード・アカウント削除

- `GET /my/account`：Cognitoの`AdminGetUser`で`{"email":"...","provider":"password"}`を返す。Googleでログインしたユーザーは`provider`が`google`になる。
- `POST /my/password`：`{"current":"...","proposed":"..."}`を受け取り、Cognitoの`ChangePassword`でパスワードを変更して204を返す。現在のパスワードの誤りは`400 incorrect_password`、パスワードの条件違反は`400 invalid_password`、Googleのユーザーは`409 password_unavailable`、試行回数の超過は429とする。
- `DELETE /my/account`：確認として`{"handle":"現在のユーザーID"}`を受け取り、[ADR 0016](../docs/adr/0016-anonymize-deleted-accounts.md)に従ってアカウントを削除し、204を返す。ユーザーIDが違えば`400 confirmation_mismatch`、共同作成者のいない結果確定していない自分のコンテスト（凍結のあるコンテストでは結果の公開まで）があるか、終了していないコンテストに自分の未公開の問題が入っていれば`409 account_has_active_contest`とする。
- [ADR 0022](../docs/adr/0022-share-contests-among-setters.md)に従い、共同作成者の削除は共同作成者から抜けたものとして扱う。
  オーナーの結果確定していないコンテストと下書きのうち、共同作成者のいるものは、最も早く加わった共同作成者にオーナーを移す。
- 削除では、公開していない問題・記事・コンテストの下書き、公開済みの問題・記事の未公開の編集内容、お気に入り、自分宛ての通知、残るコンテンツから参照されない画像を消す。公開済みの問題・記事、終了したコンテスト、提出、開始したコンテストの参加登録とチームの所属、難易度の投票、テスターの担当は残す。開始前のコンテストの参加登録とチームの所属は外す（[ADR 0024](../docs/adr/0024-team-and-onsite-participation.md)）。
- プロフィールの行は残し、ユーザーIDを`deleted_`で始まる値に置き換え、アイコンと外部アカウントを消して`deleted_at`を記録する。Cognitoのユーザーは`AdminDeleteUser`で削除し、失敗した場合はDBの変更も取り消す。
- Cognitoのアクセストークンは期限まで有効なため、削除済みアカウントのトークンはすべての認証付きAPIで`401 account_deleted`とする。
- `AdminGetUser`と`AdminDeleteUser`は、APIのLambdaロールの権限（`infra/api/main.tf`の`cognito_users`）で呼ぶ。Googleでログインしたトークンには、トークンで呼ぶ操作に必要なスコープがないためである。
- 配布時は、APIを切り替える前に`terraform apply`でこの権限を追加し、マイグレーション`026_account_deletion.sql`を適用する。

マイグレーションは `internal/database` にまとめ、バージョン2で `user_profiles` を追加した。
既存DBは `go run ./cmd/migrate` で更新する。
`make dev` で自動起動するローカルDBは起動時にマイグレーションを適用する。

## 公開問題とブログ

DB マイグレーション 003 で問題の公開スナップショットと `blog_posts` を追加します。
既存の問題は非公開のままです。`make dev` は起動時にマイグレーションを適用します。
個別に適用する場合は `DATABASE_URL=... go run ./cmd/migrate` を実行してください。

- `GET /problems`、`GET /problems/{id}`: 公開された問題の一覧と詳細（認証不要）。
- `GET /posts`、`GET /posts/{id}`: 公開された記事の一覧と詳細（認証不要）。
- `GET /my/posts`、`GET|PUT|DELETE /my/posts/{id}`: 自分の記事の保存・取得・削除。
- `PUT /my/problems/{id}/publication`、`PUT /my/posts/{id}/publication`: `{ "version": 1, "publish": true }` で現在の保存内容を公開。`false` で非公開に戻す。

公開中の問題を保存すると、問題文、解説、採点設定を公開内容とともに更新します。
公開中・公開予約中の問題と結果確定前のコンテストに入っている問題では、公開条件を満たさない入力を 400 `incomplete_problem` で拒否し、保存済みの内容を保ちます。
ジャッジのメンテナンス中は使える言語がないため、この保存では判定用コードと集計コードの言語が使えるかを確かめません。
公開条件は公開・公開予約・コンテスト・定期便で共通で、タイトル、本文、難易度、テストケース1件以上、解説と、判定用コードを使う問題ではジャッジで使える言語の判定用コード、部分点の問題では下の「部分点」の条件です（[ADR 0021](../docs/adr/0021-unify-problem-requirements.md)）。
未公開の問題と記事は、保存しても公開されません。
予約中の問題は、保存すると予約内容も更新され、公開日時は維持されます。
公開条件を満たさない保存は拒否し、直前の予約内容を保持します。
保存、公開、非公開への変更ではバージョンを検証し、他画面の更新と競合した場合は 409 を返します。
削除は公開内容も削除します。一覧は 50 件ずつ `nextCursor` を次回の `cursor` に指定して取得できます。
記事には登録済みユーザー ID を表示します。運営の記事には API 環境変数 `OPERATOR_SUBJECTS` にカンマ区切りで指定した Cognito `sub` の投稿だけ「運営」を表示します。
一般ユーザーは運営フラグを設定できません。運営も通常のログイン・記事作成画面を使います。

記事は、解説する問題を最大30問まで持てます（[ADR 0017](../docs/adr/0017-link-posts-as-user-editorials.md)）。
`PUT /my/posts/{id}`の`problemIds`で指定し、省略すると現在の一覧を保ちます。
公式解説を誰でも読める問題以外を含むと`400 editorial_problem_unavailable`を返します。
一覧は記事の公開時に公開版へ写り、公開APIでは次の形で返します。

- `GET /posts/{id}`の`problems`: 記事が解説する問題の`id`と`title`。
- `GET /problems/{id}/editorials`: 問題を解説する公開記事。公開日時の昇順。
- `GET /contests/{id}/editorials`: 終了したコンテストの各問題を解説する公開記事。`problemId`で問題を示す。

問題と記事は、同じタグを最大5個まで持てます（[ADR 0018](../docs/adr/0018-tag-problems-and-posts.md)）。
問題は下書きの`draft.tags`、記事は`PUT /my/posts/{id}`の`tags`で指定します。記事の`tags`を省略すると現在の一覧を保ちます。
各タグはNFKCで正規化し、前後の空白を除いて連続する空白を1つにした30文字以内の文字列です。
正規化されていない値、カンマや制御文字、大文字と小文字だけが違う重複を含むと、問題は`400 invalid_draft`、記事は`400 invalid_post`を返します。
タグは公開時に公開版へ写ります。

- `GET /tags`: 公開中の問題と記事のタグを、使われている数の多い順に最大1000件返す。`problemCount`と`postCount`を含む。
- `GET /problems?tag=`、`GET /posts?tag=`: タグの完全一致で絞り込む。`author`と併用できる。不正なタグは`400 invalid_tag`。
- `GET /problems/{id}`、`GET /problems`、`GET /posts/{id}`、`GET /posts`の`tags`: 公開版のタグ。

定期便の回に含まれる問題は、新作・復刻を問わず、その回の解説公開までタグを返しません。
タグでの絞り込みと`GET /tags`の件数からも除きます。作問者とテスターには返します。
コンテストの問題APIはタグを返しません。

## 問題のテストケース

下書きの`draft.testCases`へ`[{"input":"3 5\n","output":"8\n"}]`の形式で保存する。
省略時は空のケース一覧として扱う。
最大100件、各入力と期待出力は16 MiB、入出力の合計は512 MiBまでとし、NUL文字は拒否する。
64 KiB以下の本文は下書きJSONへ保存し、それを超える本文とJSON内の本文合計が256 KiBを超える分は、署名付きURLで専用S3バケットへ直接アップロードする。
保存するファイル参照は、APIがサイズ、SHA-256、UTF-8、NUL不在を検証したものに限る。
更新番号による競合検出と所有者の検証は問題本文と共通である。
公開操作は本文とケースを一緒に固定し、提出は公開版のケースだけを使う。
公開問題詳細APIと提出結果APIはテストケース本体を返さない。サンプル専用APIは公開版のサンプルケースだけを返す。
ローカルジャッジの起動に必要な設定は`JUDGE_CPP_IMAGE`であり、`JUDGE_TEST_DIR`は使用しない。
ローカルではC++23とCPython 3.14を使い、イメージの取得方法は[開発用ジャッジ](../docs/judge/local-cpp.md)に従う。

### 部分点

[ADR 0025](../docs/adr/0025-score-partially-with-test-groups.md)に従い、問題ごとに一括方式、グループ方式、集計コード方式のどれかで採点する。

- 下書きの`scoring`は`{"mode":"groups","groups":[{"name":"小課題1","points":30}]}`の形で、集計コード方式は`"mode":"aggregate"`と`"aggregator":{"runtime":"cpp23-gcc","source":"..."}`にする。ケースは`testCases[].groups`にグループ名を並べる。`scoring`がないかnullなら一括方式。
- 保存済みの下書きに`scoring`があるとき、`scoring`の項目を挙げない保存は409 `version_conflict`で断る。部分点を知らない古いクライアントが、採点の設定とケースの所属を黙って消さないためである。一括方式に戻すには`"scoring": null`を送る。
- 保存のたびに構造を確かめ、満たさなければ400 `invalid_draft`とする。グループは100個まで、名前はケース名の規則に加えて空にできず、前後の空白を除いて重ならない。配点は0〜1,000,000の整数で、グループがあれば合計（満点）を1〜1,000,000にする。ケースのグループは存在する名前を重ねずに挙げる。集計コードは集計コード方式だけに置け、判定用コードと同じ言語と大きさの規則で、`protocol`を持たない。
- 内容の条件のコードは`scoring_groups_missing`（グループがない）、`scoring_group_empty`（ケースのないグループがある）、`test_case_ungrouped`（サンプル以外にどのグループにも属さないケースがある）、`aggregator_source_missing`、`aggregator_runtime_unavailable`。
- 開始したコンテストに入っている問題は、方式、グループの名前、配点、順番を変える保存（部分点の有無の切り替えを含む）を409 `problem_scoring_locked`で拒む。ケースの所属、テストケース、集計コードは直せ、既存の提出にはリジャッジで反映する。
- 部分点の問題のコンテストの配点は満点とし、開始前は問題を保存するたびに`contest_problems.points`をそろえる。問題の保存は、コンテストの行をロックしてから開始したかを一度だけ判定し、固定とそろえに同じ判定を使う。コンテストの保存では部分点の問題の`points`を読まない（0か省略でよい）。一括方式の問題の`points`が0なら400 `invalid_contest`とし、下書きの`points_invalid`も一括方式の問題だけで判定する。下書きの応答の`problems[]`は、部分点の問題に`partial`と`fullScore`を付ける。
- 「解いた」は満点の提出で決める（SQL関数`submission_full_score(result)`。`scoring`のない結果はACを満点とする）。解いた人数、`GET /my/solved-problems`、初AC通知、定期便の順位が対象で、定期便では満点に届かない判定済みの提出を誤答に数え、得点は出さない。提出をまたいでグループの得点を合計しないので、コンテストで小課題ごとに別の提出で満点に届いても、これらには数えない。
- 問題詳細とコンテストの問題は、部分点の問題に`scoring`（`fullScore`と`groups`の`name`、`points`）を返す。方式と集計コードは返さない。
- コンテスト詳細は`partialScoring`（部分点の問題があるか。開始前も返す）と`problems[].partial`を返す。`problems[].solved`は、グループごとの最高点の合計が満点に達した問題に付く（チーム戦ではチームの提出から求める）。
- 配布時は`040_submission_full_score.sql`だけのコミットを先に配布して適用し、その後にAPIを配布する。部分点の保存を有効にするAPIは、[配布と検証の手順](../docs/judge/deployment-checks.md)の「部分点を有効にする前の配布」が終わってから配布する。

順位表は、問題ごとにグループごとの全提出の最高点をとって合計する（JOIとIOIの規則）。

- 行とセルは`score`（得点の100倍の整数）、`scoreText`（表示用の文字列。端数がなければ整数、あれば小数第2位までの`37.50`の形）、`points`（得点の整数部分。古いクライアント向け）を返す。順位は`score`で比べる。
- セルの`scoredAt`は最後に得点を伸ばした提出の時刻、`acceptedAt`は満点になった提出の時刻とする。
- 得点を伸ばさなかった判定済みの提出（CEとJEを除く）を誤答とし、ペナルティには最後に得点を伸ばした提出より前の誤答を数える。セルの`wrong`は、得点0の問題ではすべての誤答、それ以外ではペナルティに数えた誤答とする。`pending`は満点の提出より前の判定待ちを数える。
- 一括方式の問題では、最後に得点を伸ばした提出が初ACなので、これまでと同じ値になる。

### 難易度投票

公開問題では、プロフィール登録済みのユーザーが1〜10の難易度を1人1票で投票できます。
`GET /my/difficulty-votes/{id}`で自分の票と集計を取得し、`PUT`に`{"difficulty": 7}`を送ると投票・変更、`DELETE`で取り消します。
レスポンスは`difficulty`（自分の票、未投票はnull）、`difficultyAverage`（平均、0票はnull）、`difficultyVoteCount`、`difficultyDistribution`（Lv.1〜10の票数を順に並べた10要素の配列）です。
公開問題詳細APIも平均・票数・分布を返し、未ログインでも分布を閲覧できます。作成者設定の`difficulty`は変更しません。
配布時はAPIを切り替える前に通常のDBマイグレーションを実行し、`018_difficulty_votes.sql`を適用してください。


### コンテストの下書きと公開

[ADR 0015](../docs/adr/0015-save-contests-as-drafts.md)に従い、新しいコンテストは下書きとして保存し、公開時に検証する。

- `PUT /my/contests/{id}/draft`：`{"version":0,"draft":{...}}`で下書きを作成し、以降は返された`version`で更新します。`draft`は`title`、`description`、`startsAt`（null可）、`durationMinutes`（null可）、`penaltyMinutes`（null可）、`freezeMinutes`（null で凍結なし、省くと保存済みの値を保つ）、`teamMode`、`maxTeamSize`、`onsiteDivision`、`onsitePassphrase`（下の「チーム戦とオンサイト」、省くと保存済みの値を保つ）、`problems`（`id`と、null可の`points`）です。
- 下書きは未入力や範囲外の値を含んでいても保存できます。拒否するのは、タイトル120文字・説明100,000文字・問題100件の上限超過、不正または重複した問題ID、絶対値が10億を超える数値だけです（400 `invalid_contest`）。
- 応答は`id`、`version`、`updatedAt`、`draft`と、公開できない理由のコード一覧`issues`を返します。コードは`title_missing`、`start_missing`、`start_past`、`duration_invalid`、`penalty_invalid`、`freeze_invalid`（1分からコンテスト時間までの範囲外）、`problems_missing`、`points_invalid`（部分点の問題を除く）、`problem_unavailable`（公開済み・削除済み・他のコンテストに登録済み）、`problem_incomplete`（コンテストの内容の条件を満たさない）です。
- `GET`で取得、`DELETE ?version=N`で削除し、`GET /my/contests/drafts?offset=0`で自分の下書きを更新日時の降順に50件ずつ返します。一覧の各要素の`author`はオーナーのユーザーIDです。
- `PUT /my/contests/{id}/publication`に`{"version":N}`を送ると、保存済みの下書きを同じ`id`のコンテストとして公開し、コンテスト詳細を返します。終了日時は`startsAt`に`durationMinutes`を足した時刻です。`issues`が残っていれば409 `contest_not_ready`、古い`version`や問題の競合は409 `contest_conflict`です。
- 下書きは`contests`とは別の`contest_drafts`に置くため、公開APIの一覧・詳細・順位表・参加登録からは見えません。他人の下書きは404です。
- 新規作成枠は下書きの作成時に1件分消費し、公開では消費しません。公開済みで開始前のコンテストは、これまでどおり`PUT /my/contests/{id}`で検証してから保存します。
- 共同作成者も下書きの取得、保存、公開ができ、下書きの一覧にも出ます。削除はオーナーだけができ、共同作成者には404を返します。誰が公開しても、オーナーは下書きのオーナーのままです。
- 応答の`authors`はオーナーを先頭とするセッター、`owner`は閲覧者がオーナーかどうかです。`problems`は、今のセッターの誰かが作者か招待されたテスターである問題を下書きの順に並べ、`id`、`title`、`author`、コンテストに入れられない理由`issues`（問題の`readiness.contest`と同じコード）を返します。それ以外の問題は`problem_unavailable`になります。
- 下書きに新しく加える問題のうち、存在する問題は、保存する人が作者か招待されたテスターでなければ409 `contest_conflict`です。存在しない問題IDはこれまでどおり保存し、`problem_unavailable`として示します。
- 配布時はAPIを切り替える前に通常のDBマイグレーションを実行し、`025_contest_drafts.sql`を適用してください。

### コンテストへの参加

- `POST /my/contests/{id}/participation`：認証・プロフィール登録が必要。開始前または開催中に参加登録し、更新後のコンテスト詳細を返します。同じユーザーの再登録は重複しません。
  本文が空なら、チーム戦でも個人のオープン参加です。チーム戦とオンサイト区分の本文は下の「チーム戦とオンサイト」にあります。
- 詳細の `participating` は閲覧ユーザーの登録状態です。`official` は公式順位の対象になれるかどうかで、スタッフとスタッフだった人は対象外です。`staff` は閲覧ユーザーがスタッフかどうかです。
- 登録者は未提出でも0点で順位表に表示されます。開催中の提出には登録が必要です（スタッフを除く）。終了後の練習提出には登録不要です。
- 終了後の新規登録・対象外ユーザーの登録は409 `contest_participation_unavailable`、未登録での提出は409 `contest_participation_required` です。
- 移行020で既存の公式提出者を登録済みにします。撤回済みの移行019は番号を予約し、既存環境で番号を再利用しません。

### コンテストの役割

[ADR 0022](../docs/adr/0022-share-contests-among-setters.md)に従い、コンテストに関わる人を次のように判定する。
判定は`031_contest_roles.sql`のSQL関数にまとめ、各クエリはそれを呼ぶ。

- セッター（`contest_setter`）：コンテストか下書きのオーナーと、抜けていない共同作成者。
- スタッフ（`contest_staff`）：セッター、コンテストの問題の作者、コンテストの問題のテスター。
  開催中もすべての提出を閲覧でき、参加登録できず、順位表に載らない。
- 開始前に問題を見られる人（`contest_problem_access`）：セッターと、その問題の作者とテスター。
  開始前のコンテストでの提出も、この人に限る。
- 結果確定（`contest_results_final`）：凍結のないコンテストでは終了、凍結のあるコンテストでは結果の公開（下の「順位表の凍結と結果の公開」）。
  コンテストの提出の公開（提出一覧と提出詳細、公開問題の提出一覧、解いた人数）は、提出ごとの`contest_submission_public`を使う。
  問題と解説は、これまでどおり終了時に公開する。

参加者とスタッフは兼ねられない。

- 参加登録した人が作者かテスターである問題を加える保存と公開は、409 `contest_conflict`で拒否する。
  下書きの`issues`にも同じ判定を`problem_participant`として入れているが、公開前の下書きには参加者がいないため、今は出ない。
- 参加登録した人は、結果確定まで、コンテストに入っている問題のテスター招待を受けられない（409 `contest_participant_locked`）。
- 開始前に問題をコンテストから外すと、その問題の作者とテスターを`contest_exclusions`に記録する。
  記録された人は参加登録できず、登録済みでも開催中に提出できず、順位表に載らない。
- 結果確定の後にテスターになった参加者は、順位表に残る。
- 問題の作者もスタッフになるため、作者が参加していた終了済みのコンテストの順位表からは、その行が消える。
- 配布時は`031_contest_roles.sql`だけのコミットを先に配布して適用し、その後にAPIを配布する。

### 共同作成者

[ADR 0022](../docs/adr/0022-share-contests-among-setters.md)に従い、セッターは招待リンクで共同作成者を加えられる。
共同作成者と招待リンクは下書きと公開済みのコンテストに共通のIDで`032_contest_setters.sql`の表に記録し、公開しても引き継ぐ。

| API | 動作 |
| --- | --- |
| `POST /my/contests/{id}/setter-invitation` | セッターが招待リンクのトークンを`{"token":"..."}`で取得する。初回に発行し、以降は同じトークンを返す |
| `PUT /my/contests/{id}/setter-invitation` | トークンを作り直して返す。古いリンクは404になる |
| `GET /my/setter-invitations/{token}` | `id`、`title`、`author`（オーナー）、`published`（公開済みか）、`joined`（閲覧者がセッターか）を返す |
| `POST /my/setter-invitations/{token}` | 共同作成者に加わり、同じ形で返す。セッターの再送は何もしない |
| `DELETE /my/contests/{id}/setters/{handle}` | オーナーが共同作成者を外すか、共同作成者が自分で抜ける。204を返す |

- 招待を受けられるのは結果確定までで、その後は409 `contest_invitation_closed`とする。
  参加登録した人は409 `contest_participant_locked`とする。
- 招待を受けると、オーナーに`kind: "contest_setter"`の通知を送る。
  この通知は`problemId`を持たず、`contestId`と、コンテストか下書きの`title`を返す。
- 外す相手が共同作成者でない場合や、共同作成者がほかの人を外そうとした場合は404 `setter_not_found`とする。
  オーナーは抜けられない。
- 抜けた人と外された人は`contest_exclusions`に記録し、そのコンテストに参加できない。
  オーナーが外したときは、招待リンクを作り直す。
- 招待リンクのトークンは、公開の`Contest`を含む公開APIの型には載せない。
- コンテストの詳細と一覧は`authors`（オーナーを先頭とするセッター）、`setter`（閲覧者がセッターか）、`owner`（閲覧者がオーナーか）を返す。
  `author`はこれまでどおりオーナーである。
  開始前のセッターには、今のセッターの誰も作者でもテスターでもない問題に`unmanaged: true`を付ける。
  その問題を外すまで保存は409 `contest_conflict`になる。
- コンテストの問題は`authors`と、閲覧者が今解説を受け取れるかどうかの`canReadEditorial`を返す。
- コンテストに入れられるのは、今のセッターの誰かが作者か招待されたテスターである問題で、新しく加えられるのは、保存する人が作者か招待されたテスターである問題に限る。
  コンテストのセッターとして扱えることは、この判定に数えない。
- 公開済みのコンテストのセッターは、結果確定まで、コンテストの問題を編集、リジャッジ、テストファイルの取得などでテスターと同じく扱える（`can_manage_problem`）。
  問題に行は作らず、問題のテスター欄にも出さない。
  問題のテスター招待リンクの発行と、コンテストに入れてよいかの判定は、作者と招待されたテスターだけを見る`can_manage_problem_directly`を使う。
- 説明の画像は、セッターの誰がアップロードしたものでも使え、下書きの説明の画像は今のセッターが見られる。
  抜けた人がアップロードした画像も、そのコンテストではそのまま使える。
- 配布時は`032_contest_setters.sql`だけのコミットを先に配布して適用し、その後にAPIを配布する。

### 順位表の凍結と結果の公開

[ADR 0023](../docs/adr/0023-freeze-contest-standings.md)に従い、終了前の一定時間の提出の結果を、結果の公開まで伏せられる。

- `PUT /my/contests/{id}`と下書きの`freezeMinutes`：null で凍結なし、1分からコンテスト時間までの分数。開始後は変えられない。範囲外は400 `invalid_contest`。
  省くと保存済みの値を保つ（凍結を知らない古いクライアントの保存で凍結が消えないように）。保つ凍結が新しいコンテスト時間を超える保存は409 `contest_conflict`。
- コンテストの詳細と一覧は`freezeMinutes`、`freezeAt`（凍結を始める時刻、凍結なしはnull）、`revealed`、`revealedAt`、`resultsFinal`を返す。
  閲覧者向けに`canViewLiveStandings`（凍結のあるコンテストのスタッフ）と`canRevealResults`（終了して結果を公開していない凍結コンテストのセッターと運営者）を返す。
- 凍結後の提出（`freezeAt`から終了までに作られた提出）は、結果確定まで、本人とスタッフ以外には一覧にも詳細にも出さない。凍結前の提出と終了後の練習の提出は終了時に公開する。解いた人数も同じ条件で数え、`GET /my/solved-problems`は凍結に関係なく本人の結果を返す。
- `GET /contests/{id}/standings`は、結果確定まで凍結前の提出だけで計算し、各セルの`frozen`に凍結後の提出の件数（CE、JE、判定待ちを含み、凍結前に正解した問題では0）を返す。
- `GET /my/contests/{id}/standings`は同じ表を返し、スタッフが`?live=1`を付けたときだけ凍結なしの表を返す。
- `POST /my/contests/{id}/reveal`：セッターか運営者（`OPERATOR_SUBJECTS`）が結果を公開し、コンテスト詳細を返す。取り消せず、公開済みなら何もせず200。
  ほかの人は404、終了前か凍結のないコンテストは409 `contest_conflict`、参加者のコンテスト中の提出に判定待ち（リジャッジ中を含む）が残っていれば409 `contest_results_pending`。スタッフの提出、サンプル検証、終了後の練習は待たない。
- 配布時は`034_contest_freeze.sql`だけのコミットを先に配布して適用し、その後にAPIを配布する。

### チーム戦とオンサイト

[ADR 0024](../docs/adr/0024-team-and-onsite-participation.md)に従い、コンテストを個人戦かチーム戦にし、オンサイト区分を設けられる。

- `PUT /my/contests/{id}`と下書きの設定は次のとおり。省くと保存済みの値を保ち、開始後は変えられない。
  - `teamMode`：チーム戦かどうか。
  - `maxTeamSize`：チーム戦の最大人数で、2〜50かnull（無制限。内部の上限は50人）。個人戦では保存しない。
  - `onsiteDivision`：オンサイト区分の有無。
  - `onsitePassphrase`：会場の合言葉。`onsiteDivision`をtrueで送るときは必須で、前後の空白を除きNFKCで正規化したあとの、英字、数字、`_`だけの4〜64文字にする（大文字と小文字は区別する）。それ以外のときは読まない。
- 範囲外は400 `invalid_contest`で、下書きでは`issues`に`team_size_invalid`か`venue_passphrase_invalid`を入れる。
- 参加登録があると、個人戦とチーム戦は切り替えられず（409 `contest_team_mode_locked`）、最大人数は最も大きいチームの人数より下げられない（409 `contest_team_size_locked`）。
- 開始前に区分をなしにすると、記録したオンサイトを消し、合言葉も消す。
- コンテストの詳細と一覧は`teamMode`、`maxTeamSize`、`onsiteDivision`を返す。
  閲覧者向けに、`team`（自分のチームの`id`、`name`、`members`、`onsite`。個人で参加した人はnull）と`onsite`（自分かチームがオンサイトか）を返す。
- 会場の合言葉と参加コードは、公開の`Contest`を含む公開APIの型に載せない。

参加の本文は`{"team":{"action":"create","name":"..."}}`か`{"team":{"action":"join","code":"..."}}`と、任意の`onsitePassphrase`である。

- `team`を省くと、チーム戦でも個人で参加する。個人戦では`team`を指定できない（409 `contest_team_unavailable`）。
- 個人で参加した人は、開始前ならチームを作るか入ってメンバーになる。開始後は409 `contest_individual_locked`とし、試行回数には数えない。
  オンサイトの個人がチームを作るか入ると、チームもオンサイトになる。
- チーム名はNFKCで正規化して空白をまとめ、1〜40文字にする。制御文字と書式文字、異体字セレクタやハングルの埋め字などの表示されない文字を含む名前と、見える文字のない名前は400 `invalid_team_name`、大文字と小文字だけが違う名前を含めて同じ名前は409 `team_name_taken`。
- 1人が入れるチームは1つで、2つ目は409 `team_already_joined`とする。自分のチームのコードを再び送っても何も変えない。満員のチームは409 `team_full`。
- 参加コードはサーバーが作る10文字で、0、O、1、I、L、Uを使わない。大文字と小文字、全角、空白とハイフンの違いは無視し、誤りは400 `incorrect_team_code`。
- 会場の合言葉が正しければ本人（チームのメンバーならチーム）をオンサイトにし、誤りは400 `incorrect_venue_passphrase`として何も登録しない。使えない文字を含む合言葉も誤りとして数える。区分のないコンテストでは409 `contest_onsite_unavailable`。
- 登録済みの人は、終了まで`onsitePassphrase`だけを送ってオンサイトになれる。チームの誰か1人が入れればチーム全体がオンサイトになる。
  空白だけの合言葉は、未登録の人ならオープン参加として登録し、登録済みの人からは400 `incorrect_venue_passphrase`とする（試行回数には数えない）。
- 参加コードと合言葉の誤りはアカウントとコンテストの組ごとに数え、10分に10回に達すると、以後の試行を429 `contest_attempts_limited`と`Retry-After`で断る。

| API | 動作 |
| --- | --- |
| `GET /my/contest-schedule` | 参加登録した終了前のコンテストの`id`、`title`、`startsAt`、`endsAt`を開始の昇順で最大50件、`{"items":[...]}`で返す |
| `GET /my/contests/{id}/team` | メンバーに自分のチームの`id`、`name`、`members`、`onsite`と参加コード`code`を返す |
| `PUT /my/contests/{id}/team/code` | 参加コードを作り直してチームを返す。終了まで |
| `PUT /my/contests/{id}/team/name` | `{"name":"..."}`でチーム名を変えてチームを返す。終了まで |
| `DELETE /my/contests/{id}/team/members/{handle}` | 開始前に、ほかのメンバーを外すか、自分のハンドルで抜ける。204を返す |
| `GET /my/contests/{id}/onsite` | セッターに`{"onsiteDivision":true,"passphrase":"..."}`を返す |
| `PUT /my/contests/{id}/onsite` | セッターが`{"passphrase":"..."}`で合言葉を差し替える。終了まで |
| `GET /my/contests/{id}/participants` | セッターにチームと個人で参加した人を`{"items":[...]}`で返す |
| `PUT /my/contests/{id}/participants/{handle}/onsite` | セッターが個人で参加した人を`{"onsite":true}`でオンサイトにする。結果確定まで |
| `PUT /my/contests/{id}/teams/{team}/onsite` | セッターがチームを`{"onsite":true}`でオンサイトにする。結果確定まで |
| `PUT /my/contests/{id}/teams/{team}/name` | セッターがチーム名を直す。結果確定まで |

- メンバーの操作は、チームがなければ404 `team_not_found`、チームにいないハンドルは404 `team_member_not_found`、期間外は409 `team_locked`とする。
- メンバーを外すか自分で抜けると、その人の参加登録を消し、参加コードを作り直す。最後の1人が抜けるとチームを消す。
- セッターの操作は、セッター以外に404 `contest_not_found`、期間外に409 `contest_conflict`、該当する参加者やチームがなければ404 `participant_not_found`か`team_not_found`を返す。チームのメンバーのハンドルは`participant_not_found`とする。
- オープンに戻す`{"onsite":false}`は400 `onsite_irreversible`とする。オンサイトからオープンに戻す経路はなく、区分をなしにしたときだけ全員がオープンになる。
- 参加者一覧の各要素は、チームなら`team`、個人で参加した人なら`handle`を持ち、`onsite`、`onsiteAt`、`onsiteVia`（`passphrase`か`setter`、オープンなら空）、`createdAt`を返す。
- 合言葉の差し替えはコンテストの`version`を進めるので、古い合言葉を持つ編集画面の保存は409 `contest_conflict`になる。
- 開始前の退会では、そのコンテストの参加登録とチームの所属を外し、空になったチームを消し、残ったチームの参加コードを作り直す。
- 配布時は`035_contest_teams.sql`だけのコミットを先に配布して適用し、その後にAPIを配布する。`037_contest_onsite_kept_open.sql`と`038_contest_team_individuals.sql`も同じく先に適用する。
  `039_contest_onsite_drop_kept_open.sql`は`kept_open`の列を消すので、列を読まないAPIを配布した後に適用する。

チームの提出の共有、順位表、オンサイトの絞り込みは次のとおり。

- チームのメンバーは、開始後いつでも、今のメンバーがこのコンテストに出した提出を、凍結後の提出も含めて見られる（`contest_teammates`）。
  コンテストの提出一覧と詳細、`GET /my/submissions/{id}/public`、公開問題の提出一覧と詳細で同じ条件を使う。
  開催中のコンテストの提出一覧はチームの提出だけを返し、コンテスト詳細の`canViewSubmissions`は開催中のメンバーにもtrueになる。
- 提出詳細は、チームメイト（自分を含む）にだけコンパイルエラー（`result.compileLog`）を返す。
  チェッカーの診断は返さず、サンプル検証は共有しない。
- コンテストの問題の提出一覧は、`mine=team`で自分とチームメイトのこのコンテストへの提出を返す。
  認証が必要で、個人戦では`mine=1`と同じになり、ほかの経路では400 `invalid_request`とする。
- [ADR 0030](../docs/adr/0030-list-own-contest-submissions.md)に従い、コンテストの提出一覧（`GET /my/contests/{id}/submissions`）も`mine=1`と`mine=team`を受け付ける。
  問題ごとの同じ一覧を、コンテストの全問題分まとめて返す。開始前から終了後までいつでも返し、`total`も同じ条件で数え、`scope=onsite`では絞り込まない。
  公開APIの`GET /contests/{id}/submissions`では、`mine`を付けると401 `authentication_required`とする。
- コンテスト詳細の`problems[].solved`は、チーム戦ではメンバーの誰かが正解した問題に付く。
- チーム戦の順位表は、チームごとの行と、個人で参加した人ごとの行を同じ表に並べる。
  チームの行は`handle`を空文字にして`team`（`id`、`name`、`members`、`onsite`）を返し、個人の行は`handle`を返して`team`をnullにする。
  得点、ペナルティ、凍結は、チームならメンバー全員の提出をまとめ、個人ならその人の提出で、同じ規則で計算する。
  個人で参加した人の提出の公開範囲、解いた印、オンサイト区分は、個人戦と同じく本人だけのものとする。
- 順位表の各行は、`onsite`（区分があり、本人かチームがオンサイト）と`overallRank`（全体の順位）を返す。
- 順位表（公開と`/my`）、コンテストの提出一覧、問題ごとの提出一覧は`scope=onsite`を受け付け、ほかの値は400 `invalid_request`とする。
  順位表はオンサイトの行だけを返し、`rank`をその中で振り直す。
  提出一覧は、表示した時点でオンサイトの参加者（`contest_onsite`）がこのコンテストに出した提出だけを返し、練習の提出も残す。
  スタッフの提出と、参加登録していない人の練習の提出は返さず、`mine`を付けた一覧では絞り込まない。
- 配布時は`036_contest_team_visibility.sql`だけのコミットを先に配布して適用し、その後にAPIを配布する。

## コンテストの質問と回答

マイグレーション044まで適用してからAPIとWebを更新する。
質問と回答は既存のPostgreSQLに保存する。

| メソッドとパス | 内容 |
| --- | --- |
| `GET /my/contests/{id}/clarifications?offset=0` | 閲覧できる質問を作成日時の降順で50件返す。レスポンスは`items`と`hasMore`。 |
| `POST /my/contests/{id}/clarifications` | `problemId`と`question`で質問を送信する。スタッフは`answer`で質問者なしの回答を全体公開する。成功は204。 |
| `PUT /my/contests/{id}/clarifications/{clarification}` | `version`と`answer`で回答を更新する。`publish: true`で全体公開する。成功は204。 |

全経路で認証を必須とし、レスポンスは`Cache-Control: no-store`とする。
公開APIには質問と回答の経路を設けない。
全体公開済みの質問は、プロフィールの登録やコンテストへの参加に関係なく、ログインした利用者が閲覧できる。
非公開の質問は質問者本人、コンテストのセッター（オーナーと抜けていない共同作成者）、対象問題の直接の作問者とテスターだけに返す。
ほかの問題の作問者とテスターであることや、チームの所属だけでは閲覧権限を与えない。
担当者の権限は取得時と更新時の担当情報から判定する。

質問は開催中の公式参加者から受け付け、質問の変更と削除は提供しない。
質問と回答はそれぞれ空白のみを除く4,000文字以内のテキストとする。
回答は質問ごとに1件で、セッターと対象問題の直接の作問者、テスターが更新できる。
スタッフは開催前・開催中・終了後に、`question`を省略し`answer`だけを送って回答を全体公開できる。
`problemId`を省略または空文字にするとコンテスト全体、指定するとそのコンテストに含まれる問題が対象になる。
この投稿の質問者はDB上でNULLとし、一覧の`author`と`question`は空文字、コンテスト全体の場合の`problemId`も空文字で返す。
質問者なしの投稿は現在のスタッフ全員が編集できる。通常の非公開質問の権限は変わらない。
`version`が古い更新と公開は409 `clarification_conflict`とし、質問受付期間外や未参加の送信は409 `clarification_unavailable`とする。
全体公開は取り消せず、公開後の回答更新も全体公開される。
アカウント削除後も質問と回答は残り、投稿者名は既存の匿名化済みプロフィールに従う。

## コンテストとリジャッジの通知

[ADR 0027](../docs/adr/0027-notify-open-pages-and-bell.md)に従い、次の出来事でベルの通知（`GET /my/notifications`）を作る。
通知は出来事と同じトランザクションで作り、操作した本人には作らない。

| `kind` | 出来事 | 宛先 |
| --- | --- | --- |
| `clarification_question` | 質問の送信 | コンテストのセッター、対象問題の直接の作問者とテスター |
| `clarification_answer` | 回答の保存（編集を含む） | 質問者 |
| `clarification_published` | 質問・回答の全体公開（質問者なしの投稿を含む） | 参加登録している人（チームのメンバーを含む） |
| `contest_results` | 結果の公開 | 参加登録している人 |
| `rejudge` | リジャッジ | 待ち状態に戻った提出の提出者（リジャッジした人を含む） |

- 質問の宛先は、非公開の質問を閲覧して回答できる人と同じにする。抜けた共同作成者には送らない。
- 全体公開の通知は、初めて全体公開したときだけ作る。公開後に回答を更新しても、質問者への回答の通知だけを作る。
- 質問者なしの投稿は全体公開の通知だけを作り、編集時には通知を作らない。
- 結果の公開の通知は、プロフィールのない運営者が公開したときは作らない。
- リジャッジの通知は、リジャッジを始めたときに提出者ごとに1件作る。リジャッジした人か提出者にプロフィールがなければ作らない。
- 質問、回答、全体公開、結果の公開の通知は`problemId`を持たず、`contestId`とコンテストの`title`を返す。
- リジャッジの通知は`problemId`と問題の`title`を返す。
  問題がコンテストに入っていて、コンテストが開始しているか、宛先の人がコンテストの問題を閲覧できる（セッターか、問題の作問者かテスター）ときだけ、`contestId`と`problemIndex`（0から始まる出題順）も返し、`title`をコンテストの問題の題にする。
  それ以外の`title`は公開版の題で、公開版がなければ問題の作問者とテスターにだけ下書きの題を返し、ほかの人には空文字を返す。
- 配布時は`042_contest_notifications.sql`と`043_rejudge_notifications.sql`だけのコミットを先に配布して適用し、その後にAPIを配布する。
- 古いWebは新しい`kind`を含む通知の一覧を受け付けず、ベルに通知を表示できなくなる。そのため、WebはAPIと同時に配布する。

## 定期便

[ADR 0011](../docs/adr/0011-schedule-featured-problems.md)に従い、日本時間の月曜と木曜22時にEasyとHardを各一問出題する。
`GET /featured?offset=0`は最新と過去の出題を20回ずつ返す。
応募は`PUT /my/problems/{id}/featured`へ`{"version":1,"preference":"soon"}`を送り、`later`なら後日の公開を希望し、空文字なら取り下げる。
応募一覧は`GET /my/featured`で取得する。
`GET /my/featured/next`は、次回予告のうち自分が作問者かテスターの枠について、`slot`・`problemId`・`title`を`items`で返す。
復刻の`title`は公開版の題とする。

[ADR 0020](../docs/adr/0020-post-and-apply-on-dedicated-pages.md)に従い、`GET /featured/schedule`は次回から8回分の予定を返す。
各回の`slots`はEasyとHardの難易度・`kind`（`new`、`revival`、`missing`）で、問題名・問題ID・作者名は含めない。
`confirmed: true`の次回は次回予告と同じ選出で、2回目以降は今の応募が残ると仮定した見込みである。
`GET /my/featured/schedule`は同じ形で、自分の問題に`mine`と`title`を付ける。
`?problem=<id>&preference=soon|later`を付けると、その問題を応募したと仮定した予定を返し、入る枠に`trial: true`を付ける。
応募できない問題は仮定に含めず、他人の問題は404 `problem_not_found`、不正な希望は400 `invalid_request`を返す。

新作の公開問題レスポンスは、翌22時まで`editorialHidden: true`と`editorialRevealAt`を返し、`editorial`を返さない。
提出一覧と詳細にも同じ解禁条件を適用する。
その間も自分の提出は認証済みAPIから確認できる。
作者とテスターは解禁前も、`GET /my/problems/{id}/publication`で公開版の解説を、`GET /my/problems/{id}/submissions`ですべての提出を取得できる。
`GET /my/problems/{id}/publication`は`GET /problems/{id}`と同じ形で返し、`editorialHidden`は一般公開の解禁状態を表す。

反映時はマイグレーション021を先に適用し、API、Web、judge-bridgeの成果物を更新する。
既存の毎分ディスパッチが定期出題も処理するため、常駐プロセスや新しいスケジューラーは不要である。
judge-bridgeの`JUDGE_ENABLED_RUNTIMES`はAPIと同じ値にする。
通常配布の`judge/rollout.py run`はこの値とランタイムダイジェストを同期する。
未設定時は既存の既定値であるC++17だけを有効として再確認する。
ディスパッチ停止中も関連APIへのアクセスで出題を回復するが、アクセスがなければ出題は遅れる。
APIの`JUDGE_ENABLED_RUNTIMES`が`none`の保守中も、予告・予定・出題では全言語を使えるものとして選出し、判定用コードを持つ問題を外さない。新しい応募は保守中は受け付けない。
定時処理の精度と遅延時の扱いはADRの「実装時の決定」を参照する。
