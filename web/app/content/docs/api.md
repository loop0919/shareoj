## 概要 {#overview}

ShareOJの公開APIにはログインなしで使えるものと、ログインして使うものがあります。

| 種類 | できること | メソッド |
| --- | --- | --- |
| ログインなし | 公開されている問題、サンプルケース、コンテスト、提出、記事、プロフィールの取得 | GETだけ |
| [ログインあり](#login) | 提出と、自分の提出の取得 | GETとPOST |

- ベースURL：`https://api.share-oj.net`
- 仕様書（Swagger UI）：[https://api.share-oj.net/docs](https://api.share-oj.net/docs)
- OpenAPIの定義：[https://api.share-oj.net/openapi.json](https://api.share-oj.net/openapi.json)

仕様書に載っているのは、ログインなしで使えるAPIだけです。

URLには `/api` を付けません。
たとえば、公開問題の一覧は次のように取得します。

```sh
curl https://api.share-oj.net/problems
```

## ログインなしで使えるエンドポイント {#endpoints}

| パス | 取得できるもの |
| --- | --- |
| `/problems` | 公開問題の一覧 |
| `/problems/{id}` | 公開問題の詳細 |
| `/problems/{id}/samples` | 公開問題のサンプルケース |
| `/problems/{id}/submissions` | 公開問題への提出の一覧 |
| `/contests` | コンテストの一覧 |
| `/contests/upcoming` | 7日以内に開始するコンテスト（開始が近い順に最大5件） |
| `/contests/running` | 開催中のコンテスト（終了が近い順に最大5件） |
| `/contests/{id}/standings` | コンテストの順位表 |
| `/featured` | 定期便の出題 |
| `/posts` | 公開記事の一覧 |
| `/users/{handle}` | 公開プロフィール |
| `/users?handles=a,b` | 複数ユーザーのアイコン（最大20件） |
| `/runtimes` | 提出に使える言語 |

チーム戦の順位表には、チームの行と個人参加の行が並びます。
チームの行は `handle` が空文字なので、`team.id` で区別してください。
個人参加の行は、`team` が `null` です。
順位表と、コンテストの提出の一覧（`/contests/{id}/submissions` など）は、`scope=onsite` を付けると、オンサイト参加の行と提出だけを返します。
部分点のあるコンテストでは、順位表の `score` が100倍した得点、`scoreText` が表示用の得点（`37.50` など）で、`points` は得点の整数部分です。
得点のある問題には `submissionId` が付きます。その得点を決めた提出、つまり最後に得点を上げた提出のIDです。

すべてのエンドポイントと応答の形式は、仕様書で確認してください。

## IDとページ送り {#pagination}

パスの `{id}` には、小文字のUUIDを指定します。
サイトのURLに含まれる22文字のIDは、APIでは使えません。

一覧の続きの取り方は、エンドポイントによって違います。

- `/problems` と `/posts` は、応答の `nextCursor` を次のリクエストの `cursor` に渡します。`nextCursor` が空なら最後のページです。
- `/problems` は `offset` と `limit`（1〜100）でも位置を指定できます。`/problems` とコンテストの提出一覧は、応答の `total` に総件数を返します。
- 提出の一覧、`/contests`、`/featured` などは `offset` で位置を指定し、応答の `hasMore` で続きがあるかを判断します。

1ページの件数はエンドポイントごとに異なります。

## ログインとトークン {#login}

ログインして使うAPIを呼ぶときは、アクセストークンを `Authorization: Bearer <アクセストークン>` のヘッダーで付けます。
アクセストークンを取得するには、メールアドレスとパスワードを `POST /auth/login` に送ります。

```sh
curl -X POST https://api.share-oj.net/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"you@example.com","password":"パスワード"}'
```

| 応答の項目 | 内容 |
| --- | --- |
| `access_token` | アクセストークン。60分で期限が切れます |
| `refresh_token` | アクセストークンを取り直すためのトークン。30日間使えます |
| `expires_in` | アクセストークンの期限までの秒数 |

- アクセストークンの期限が切れたら、`{"refresh_token":"..."}` を `POST /auth/refresh` に送ります。応答で新しい `access_token` が返ります。
- 応答には `id_token` も入りますが、APIには使えません。
- Googleでログインしているアカウントは、パスワードがないのでこの方法を使えません。
- ログアウトのAPIはありません。使い終わったトークンは保存した場所から消してください。
- POSTの本文はJSONで送り、`Content-Type: application/json` を付けます。

## 提出する {#submit}

`POST /my/submissions` に、次の項目を送ります。

| 項目 | 内容 |
| --- | --- |
| `problemId` | 問題のID。[小文字のUUID](#pagination)で指定します |
| `runtime` | 言語のID。`/runtimes` の `items` にある `id` を指定します |
| `source` | ソースコード。128 KiBまでです |
| `contestId` | コンテストのID。開催中のコンテストの問題に提出するときだけ付けます |

```sh
curl -X POST https://api.share-oj.net/my/submissions \
  -H 'Authorization: Bearer <アクセストークン>' \
  -H 'Content-Type: application/json' \
  -d '{"problemId":"<問題のID>","runtime":"python314","source":"print(1)"}'
```

受け付けると202を返します。応答の `id` が提出のIDです。
採点はこのあとに進むので、結果は[自分の提出を取得する](#my-submissions)の方法で確かめます。

- 提出は1分に2回までです。超えると429（`submission_rate_limited`）を返し、`Retry-After` ヘッダーに待つ秒数が入ります。
- 提出には、プロフィールの登録が必要です。
- 開催中のコンテストに提出するには、参加登録が必要です。

## 自分の提出を取得する {#my-submissions}

| パス | 取得できるもの |
| --- | --- |
| `/my/submissions` | 自分の提出の一覧（新しい順に最大50件）。ソースコードは含みません |
| `/my/submissions/{id}` | 提出1件。ソースコードを含みます |

採点の進み具合は `status` で分かります。

| `status` | 意味 |
| --- | --- |
| `QUEUED` | 採点の順番を待っています |
| `RUNNING` | 採点しています |
| `DONE` | 採点が終わりました。結果は `result.verdict` に入ります |

- `DONE` になるまで、数秒おきに取得してください。
- `result.verdict` は `AC` や `WA` などの文字列です。意味は[判定結果の種類](/docs/submissions#verdicts)を見てください。

## 使うときの注意 {#notes}

エラーのときは、`{"error":"コード"}` の形式で本文を返します。
CORSには対応していないため、ほかのサイトのページからブラウザーで直接呼び出すことはできません。
短時間に大量のリクエストを送ると、エラーになることがあります。
利用にあたっては[利用規約](/terms)を守ってください。
