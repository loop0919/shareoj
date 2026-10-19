## 概要 {#overview}

ShareOJの公開APIでは、公開されている問題、サンプルケース、コンテスト、提出、記事、プロフィールを取得できます。
認証は不要で、使えるのはGETだけです。
公開されていないデータは取得できません。

- ベースURL：`https://api.share-oj.net`
- 仕様書（Swagger UI）：[https://api.share-oj.net/docs](https://api.share-oj.net/docs)
- OpenAPIの定義：[https://api.share-oj.net/openapi.json](https://api.share-oj.net/openapi.json)

URLには `/api` を付けません。
たとえば、公開問題の一覧は次のように取得します。

```sh
curl https://api.share-oj.net/problems
```

## 主なエンドポイント {#endpoints}

| パス | 取得できるもの |
| --- | --- |
| `/problems` | 公開問題の一覧 |
| `/problems/{id}` | 公開問題の詳細 |
| `/problems/{id}/samples` | 公開問題のサンプルケース |
| `/problems/{id}/submissions` | 公開問題への提出の一覧 |
| `/contests` | コンテストの一覧 |
| `/contests/{id}/standings` | コンテストの順位表 |
| `/featured` | 定期便の出題 |
| `/posts` | 公開記事の一覧 |
| `/users/{handle}` | 公開プロフィール |
| `/runtimes` | 提出に使える言語 |

チーム戦の順位表には、チームの行と個人参加の行が並びます。
チームの行は `handle` が空文字なので、`team.id` で区別してください。
個人参加の行は、`team` が `null` です。
順位表と、コンテストの提出の一覧（`/contests/{id}/submissions` など）は、`scope=onsite` を付けると、オンサイト参加の行と提出だけを返します。

すべてのエンドポイントと応答の形式は、仕様書で確認してください。

## IDとページ送り {#pagination}

パスの `{id}` には、小文字のUUIDを指定します。
サイトのURLに含まれる22文字のIDは、APIでは使えません。

一覧の取得方法は2種類あります。

- `/problems` と `/posts` は、応答の `nextCursor` を次のリクエストの `cursor` に渡します。`nextCursor` が空なら最後のページです。
- 提出の一覧、`/contests`、`/featured` などは `offset` で位置を指定し、応答の `hasMore` で続きがあるかを判断します。

1ページの件数はエンドポイントごとに異なります。

## 使うときの注意 {#notes}

エラーのときは、`{"error":"コード"}` の形式で本文を返します。
CORSには対応していないため、ほかのサイトのページからブラウザーで直接呼び出すことはできません。
短時間に大量のリクエストを送ると、エラーになることがあります。
利用にあたっては[利用規約](/terms)を守ってください。
