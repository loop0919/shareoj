# Terraform に基づくインフラ構成

[構成図を開く](current.html)（ブラウザーで開く自己完結 HTML）。
図の編集元は [Archify JSON](current.architecture.json) である。
図中の説明は日本語、Viewer の固定 UI と HTML の言語指定は英語となる。

2026年9月17日に、リビジョン `82689b1e8ae232122560280ea117ac7c4d0f7f13` の Terraform と作業環境の設定値を確認した。
対象は既定の `judge/dev`、東京リージョン `ap-northeast-1` である。
AWS API や Terraform state との照合は実施していないため、実際のデプロイ状況やドリフトを示す図ではない。
ローカル tfvars の識別子や秘密値は転記していない。

## 公開経路と認証

利用者は `www.share-oj.net` の Frontend HTTP API を経由し、Nuxt SSR Lambda に接続する。
図の Frontend は、この HTTP API と Lambda を一つにまとめている。
フロントエンドは `api_endpoint` で指定された Backend HTTP API を呼び、`AWS_PROXY` 統合が Go API Lambda を起動する。
API 側にも `api.share-oj.net` の公開入口がある。
Route 53 の A/AAAA alias、ACM 証明書、REGIONAL カスタムドメインを [domain/main.tf](../../infra/domain/main.tf) が管理する。

Nuxt Lambda は VPC 外で動作し、`provided.al2023`（zip に同梱した Bun 1.4.4）、arm64、512 MiB、タイムアウト25秒である。
Go API Lambda は VPC に接続し、`provided.al2023`、arm64、タイムアウト30秒である。
API Gateway の `$default` ルートは `authorization_type = "NONE"` であり、Terraform に Gateway JWT Authorizer は定義されていない。
認証基盤は Cognito User Pool と機密クライアントで、Google IdP と Cognito ドメインは `google_client_id` が空でない場合に作成される。
Google 連携の有効状態は、この図では確定していない。

根拠: [frontend/main.tf](../../infra/frontend/main.tf)、[api/main.tf](../../infra/api/main.tf)、[auth.tf](../../infra/api/auth.tf)、[google.tf](../../infra/api/google.tf)。

## VPC とデータベース

VPC は `10.42.0.0/16` と自動割当の IPv6 CIDR を持ち、2 AZ に非公開の dual-stack サブネットを配置する。
API、migration、bridge の各 Lambda がこのサブネットと application Security Group を利用する。
図では配置をノード内に記し、VPC の枠線は省略している。

RDS PostgreSQL 17 は `db.t4g.micro`、Single-AZ、暗号化 gp3 20 GB（最大100 GB）、バックアップ保持7日である。
サブネットが2 AZ に存在しても、DB が Multi-AZ になるわけではない。
DB は非公開で、application Security Group からの TCP 5432 のみを受け付ける。
RDS 管理パスワードは Secrets Manager に保存され、Lambda に取得権限を付与する。

外向きの HTTPS は IPv6 の `::/0` から egress-only Internet Gateway を通る。
Terraform に NAT Gateway、IPv4 のインターネット向けデフォルトルート、VPC endpoint は定義されていない。
根拠: [database.tf](../../infra/api/database.tf)、[migration.tf](../../infra/api/migration.tf)、[bridge.tf](../../infra/judge/bridge.tf)、[outputs.tf](../../infra/api/outputs.tf)。

## 採点の要求と結果

API には bridge Lambda の名前と `lambda:InvokeFunction` 権限が設定される。
EventBridge も `rate(1 minute)` で同じ bridge を起動する。
bridge は DB に接続し、S3 の `jobs/*` にジョブを保存して SQS requests に送信する権限を持つ。

図の SQS ノードは requests と results の2キューをまとめたもので、矢印は要求側の流れを表す。
結果は逆方向に返る。

1. bridge → requests → worker: worker が要求を取得する。
2. worker → S3: バージョンを指定してジョブとテストデータを取得する。
3. worker → results → bridge: worker が結果を送信し、SQS event source mapping が bridge を起動する。
4. bridge → RDS: DB へ接続して結果を反映する。

通常キューの保持は1日、long polling は20秒、可視性タイムアウトは requests が2100秒、results が720秒である。
各キューに保持14日の DLQ があり、`maxReceiveCount = 3` を設定する。
結果取込の batch size は1、`ReportBatchItemFailures` を有効にする。
根拠: [bridge.tf](../../infra/judge/bridge.tf)、[storage.tf](../../infra/judge/storage.tf)。

worker は `ap-northeast-1a` の Lightsail、Ubuntu 24.04、`small_ipv6_3_0`、IPv6 のみである。
アプリ VPC との peering はなく、DB 認証情報も渡さない。
SSM hybrid managed node 用の IAM ロールを作成するが、登録や制限付きアクセスキーの設置は Terraform 外の手順となる。
ローカル設定では `enabled = true`、`ssh_enabled = false`、`test_data_bucket` 設定済みである。
根拠: [worker.tf](../../infra/judge/worker.tf)、[variables.tf](../../infra/judge/variables.tf)。

## 図で省略したストレージ接続と運用基盤

S3 jobs と test-data は別バケットである。
図では worker の取得先としてまとめており、bridge → jobs の書込み、API → test-data の読書き、worker → test-data の生成ファイル書込みは省略した。
worker の書込み権限は `test-files/*/*/generated/*` に限定される。
jobs の `jobs/` は現行版と非現行版を7日で期限切れにする。

| Terraform ルート | 管理対象と図での扱い |
| --- | --- |
| [bootstrap](../../infra/bootstrap/main.tf) | 暗号化、バージョニング、公開拒否を設定した state 用 S3。図では省略 |
| [frontend](../../infra/frontend/storage.tf) | Frontend のパッケージ用 S3。配信 CDN ではなく Lambda デプロイ元。図では省略 |
| [api](../../infra/api/storage.tf) | API と migration のパッケージ用 S3、および test-data バケット |
| [api/migration.tf](../../infra/api/migration.tf) | API と同じ DB に接続する migration Lambda。公開 Gateway 経路なし。図では省略 |
| [deploy-access](../../infra/deploy-access/main.tf) | 既存 GitHub OIDC プロバイダーを信頼するデプロイロール。主に API と Frontend の state とリソースを操作。図では省略 |
| [judge/observability.tf](../../infra/judge/observability.tf) | CloudWatch のログ、メトリクス、アラーム、SNS、通知 Lambda、通知用 DLQ。図ではカードに要約 |

通知経路は CloudWatch alarm → SNS → Python 通知 Lambda → Discord である。
Webhook は Secrets Manager から取得し、SNS 配信失敗と Lambda 実行失敗には通知用 SQS DLQ を設定する。
ローカル設定では `alerts_enabled = true` である。

## 検証記録

[delivery.json](delivery.json) に仕様と HTML の SHA-256、バイト数、参照リビジョンを記録した。
Archify の showcase 検証は9項目を通過し、エラー0件、警告0件である。
ブラウザーによる計測結果は [current.visual-check.json](current.visual-check.json) に記録する。
これらは図の検証であり、Terraform plan やデプロイの検証ではない。
