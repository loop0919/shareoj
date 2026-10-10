# AWSインフラストラクチャ

Go APIとNuxtのSSRフロントエンドをAmazon API Gateway HTTP APIとAWS Lambdaで公開するTerraform構成である。

```text
Browser -> Frontend HTTP API -> Nuxt Lambda -> API HTTP API -> Go Lambda
```

| ディレクトリ | 管理するリソース | state |
| --- | --- | --- |
| `bootstrap/` | Terraform state用S3バケット | 初回はローカル、作成後にS3へ移行 |
| `api/` | パッケージ用S3、Lambda、HTTP API、Cognito、RDS、VPC、IAM、ログ | S3の`judge/dev/api.tfstate` |
| `judge/` | 採点用EC2（常時1台と増設の台）と専用VPC、移行中のLightsail 2 GB、S3、SQS、配送Lambda | S3の`judge/dev/judge.tfstate` |
| `frontend/` | Nuxt Lambda、HTTP API、パッケージ用S3、IAM、ログ | S3の`judge/dev/frontend.tfstate` |
| `domain/` | 購入済みRoute 53ゾーン、ACM証明書、独自ドメインとAPIマッピング | S3の`judge/dev/domain.tfstate` |
| `deploy-access/` | 既存GitHubデプロイロールの信頼関係・操作権限 | S3の`judge/dev/deploy-access.tfstate` |

採点用のEC2とLightsail、SQS、配送用S3、配送と結果反映用Lambdaは`judge/`で管理する。
EC2の台の構成と切り替えは[EC2の採点台の運用と切り替え](../docs/judge/ec2-pool.md)に記載する。
配送と結果反映を行うbridge Lambdaは予約同時実行5とし、コンテスト時の一斉提出でDB接続が急増しないようにする。
[ジャッジ構築手順](../judge/README.md)に従って手動で構築し、IPv6通信と2 GB実機でのisolateの制限と計測を確認してから提出受付を有効にする。
本番テストセットの取り込みとバンドル展開は未実装である。
請求アラート、リージョン制限、GuardDuty、CloudTrailのtrailはAWSアカウント全体の設定として、別フォルダ`~/aws-setting`へ分離している。
管理者が操作できるリージョンは東京と`us-east-1`に限られ、GitHubデプロイロールは`deploy-access/`で東京だけに限る。

## 開発環境

AWS CLI v2、Terraform 1.10以上（2.0未満）、Go、zipが必要である。
CIではTerraform 1.16.0を使い、AWS Providerは各ディレクトリの`.terraform.lock.hcl`で固定する。
リポジトリのルートにあるNix開発シェルには必要なツールが含まれる。
Terraformのライセンスを許可する設定は、このパッケージだけに限定している。

リポジトリのルートで開発シェルを起動する。

```console
nix develop 'path:.'
```

direnvを使う場合は、初回にルートで`direnv allow`を実行する。
`infra/bootstrap/`などのサブディレクトリでも同じ開発環境が適用される。

以下はリポジトリのルートで実行する。
AWSプロファイルは[ルートの`.env`](../README.md#awsプロファイル)で指定し、direnvから読み込める。
AWSの認証情報は環境変数またはAWS CLIのプロファイルで設定する。

## state保存先の初回作成

bootstrapは東京リージョンにstate用S3を作る。
バージョニング、暗号化、パブリックアクセス拒否、HTTPSの強制を設定し、古いstateを自動削除するルールは設けない。
バケットとバケットポリシーの`prevent_destroy`でTerraformによる削除を防ぐ。

```console
terraform -chdir=infra/bootstrap init
terraform -chdir=infra/bootstrap plan -out=bootstrap.tfplan
terraform -chdir=infra/bootstrap apply bootstrap.tfplan
JUDGE_STATE_BUCKET="$(terraform -chdir=infra/bootstrap output -raw state_bucket_name)"
```

作成後、bootstrap自身のstateもS3へ移す。
次の`backend.tf`はローカル設定としてGitの管理対象から除外している。

```console
cat > infra/bootstrap/backend.tf <<'HCL'
terraform {
  backend "s3" {
    encrypt      = true
    use_lockfile = true
  }
}
HCL
terraform -chdir=infra/bootstrap init -migrate-state \
  -backend-config="bucket=$JUDGE_STATE_BUCKET" \
  -backend-config="key=judge/bootstrap.tfstate" \
  -backend-config="region=ap-northeast-1"
```

移行確認に同意してstateをコピーし、`terraform -chdir=infra/bootstrap state list`で管理対象を確認する。
別の端末でbootstrapを管理するときも同じ`backend.tf`を作り、同じバケット、key、regionを指定して`init`する。
バケット名を控え、空のローカルstateからbootstrapを再作成しない。

S3 backendの`use_lockfile`で同時更新を排除する。
実行ロールには、バケットの`s3:ListBucket`、対象stateの`s3:GetObject`と`s3:PutObject`、同じkeyに`.tflock`を付けたオブジェクトの`s3:GetObject`、`s3:PutObject`、`s3:DeleteObject`が必要になる。
詳細は[HashiCorpのS3 backend仕様](https://developer.hashicorp.com/terraform/language/backend/s3)を参照する。
state、tfvars、backend設定、保存したplanはGitに含めない。

## APIのデプロイ

Lambdaパッケージをビルドしてからplanを作る。
TerraformがzipをS3へアップロードし、そのオブジェクトのバージョンIDとSHA-256をLambdaへ設定する。
パッケージ用バケットはバージョニング、暗号化、パブリックアクセス拒否、HTTPSの強制を有効にする。
未完了のマルチパートアップロードは7日後、非現行バージョンは30日後に削除する。

```console
make -C api package
terraform -chdir=infra/api init \
  -backend-config="bucket=$JUDGE_STATE_BUCKET" \
  -backend-config="key=judge/dev/api.tfstate" \
  -backend-config="region=ap-northeast-1"
terraform -chdir=infra/api plan -out=deploy.tfplan
terraform -chdir=infra/api apply deploy.tfplan
curl "$(terraform -chdir=infra/api output -raw health_url)"
```

正常時は`{"status":"ok"}`を返す。
planの確認後はzipを再ビルドせず、そのまま保存したplanをapplyする。
パッケージのパスを変える場合は`lambda_package_path`を指定する。

API用のbackend設定例は`infra/api/backend.tfbackend.example`にある。
ファイルで管理する場合は`backend.tfbackend`へコピーし、バケット名を設定して`terraform -chdir=infra/api init -backend-config=backend.tfbackend`で読み込む。
APIのkeyは`judge/dev/api.tfstate`とし、請求アラートの`judge/billing-alerts.tfstate`とは分ける。
ディレクトリが異なっていても、同じバケットとkeyを指定すると同じstateを参照する。

誤って別の構成のstateを参照した場合は、backend設定を修正し、`terraform -chdir=infra/api init -reconfigure -backend-config=backend.tfbackend`で接続先を切り替える。
この修正では`-migrate-state`を使わない。別の構成のstateまでコピーしてしまうためである。
修正前に保存したplanは破棄し、planを作り直して削除対象がないことを確認する。

既定値は`project_name=judge`、`environment=dev`、`aws_region=ap-northeast-1`である。
LambdaはARM64、`provided.al2023`、256 MiB、タイムアウト30秒、予約同時実行20で動作する。
ログ保持期間は14日、HTTP APIのスロットリングは毎秒50リクエスト、バースト100である。
変更する場合は`infra/api/variables.tf`の入力をtfvarsまたは`TF_VAR_*`で指定する。
環境を増やすときは`environment`だけでなくbackendのkeyも分け、別の作業ディレクトリで初期化する。

APIのパッケージ用バケットとポリシーにも`prevent_destroy`を設定しているため、APIルート全体の`terraform destroy`は停止する。
削除する場合は保持対象と削除対象を確認し、バケットをTerraformの管理から外すか、保護設定を明示的に変更する。
`prevent_destroy`は構成からリソース定義を消した場合の保護にはならない。

## PostgreSQLと外向き通信

`api/database.tf`が東京リージョンにPostgreSQL 17の`db.t4g.small`を作成する。
20GBの暗号化gp3ストレージ、7日間の自動バックアップ、削除保護を設定し、ストレージは最大100GBまで自動拡張する。
開発環境のためSingle-AZで、障害時の自動フェイルオーバーはない。
DBの接続はアプリのSecurity Groupからの5432番ポートに限り、インターネットには公開しない。

APIとマイグレーション用LambdaはIPv4/IPv6両対応の非公開サブネットに配置する。
DBへはVPC内のIPv4で接続し、CognitoとSecrets ManagerへのHTTPS通信にはIPv6を使う。
IPv6の外向き通信には[egress-only Internet Gateway](https://docs.aws.amazon.com/vpc/latest/userguide/egress-only-internet-gateway.html)を使う。
NAT GatewayとEIPは作成せず、IPv4のインターネット向け経路も設けない。
今後APIからIPv4専用の外部サービスを呼ぶ場合は、通信経路を追加検討する必要がある。

2026年9月18日のInfracost解析では、DB本体と20GBのストレージは月$39.26だった。
RDSが管理する認証情報1件のSecrets Manager保管料約$0.40を加えると、固定費の目安は月$39.66になる。
同日の直近7日間の実測はCPU最大7.7%、接続最大8、空きメモリ最小約156 MiBだった。
CPUではなくコンテスト時の接続とメモリの余裕を確保するため、`db.t4g.micro`から`db.t4g.small`へ変更した。
通信、ログ、APIリクエスト、ストレージ増加、無料枠を超えるバックアップ、CPUクレジットなどの料金は含めない。
これはアプリ全体の利用料金の上限ではない。
秘密値を除いたTerraform解析では、費用・タグポリシー違反と警告は0件だった。
一時解析ディレクトリにはリポジトリのガードレールが関連付かないため、月$250の増加制限は固定費の概算と手動で比較した。

DBパスワードはRDSとSecrets Managerに管理させ、Terraform入力やLambda環境変数へ保存しない。
Goは新しい物理接続ごとに現在の認証情報を読み、AWSの東京リージョン用CAでDBサーバー証明書とホスト名を検証する。
リージョンを変更する場合は`api/internal/database/rds-ca-bundle.pem`も更新する。
ローカル開発では従来どおり`DATABASE_URL`を使える。

`make -C api package`はAPIとマイグレーション用の2つのzipを作る。
手動でAPIをapplyした場合も、フロントエンドを公開する前に次の処理を実行する。

```console
JUDGE_MIGRATION_FUNCTION="$(terraform -chdir=infra/api output -raw migration_lambda_function_name)"
aws lambda wait function-updated-v2 --function-name "$JUDGE_MIGRATION_FUNCTION"
aws lambda invoke --function-name "$JUDGE_MIGRATION_FUNCTION" \
  --cli-read-timeout 150 --cli-binary-format raw-in-base64-out \
  --payload '{}' /tmp/judge-migration-result.json
cat /tmp/judge-migration-result.json
```

呼び出し結果に`FunctionError`がなく、レスポンスが`{"migrated":true}`であることを確認する。
失敗時はデプロイを進めず、専用LambdaのログとDB・Secrets Managerへの接続を確認する。

## フロントエンドのデプロイ

Node.js 22とPython 3を使い、NuxtのAWS Lambda用出力をzipにまとめる。
通常のローカル用ビルドは`.output/`、Lambda用は`.output-lambda/`へ出力する。

```console
npm --prefix web ci
npm --prefix web run package:lambda
npm --prefix web run test:lambda
terraform -chdir=infra/frontend init \
  -backend-config="bucket=$JUDGE_STATE_BUCKET" \
  -backend-config="key=judge/dev/frontend.tfstate" \
  -backend-config="region=ap-northeast-1"
export TF_VAR_api_endpoint="$(terraform -chdir=infra/api output -raw api_endpoint)"
terraform -chdir=infra/frontend plan -out=deploy.tfplan
terraform -chdir=infra/frontend apply deploy.tfplan
terraform -chdir=infra/frontend output -raw site_url
node web/scripts/smoke-frontend.mjs "$(terraform -chdir=infra/frontend output -raw site_url)"
```

`site_url`がブラウザーで開くHTTPS URLになる。
Nuxt LambdaはNode.js 22、ARM64、512 MiBで動作し、HTML、JavaScript、CSS、KaTeXフォントを配信する。
予約同時実行は200、HTTP APIのスロットリングは毎秒200リクエスト、バースト500である。
問題ページはAPIのデータを使ってSSRし、canonical URLも公開先に合わせる。
API接続先は入力変数で渡し、フロントエンドからAPIのstateを読み取らない。
編集画面の下書きは引き続きブラウザー内に保存される。

RDSは常時稼働し、API Gateway、Lambda、S3、ログなどの利用量に応じて課金される。
静的ファイルの取得もGatewayとLambdaのリクエストとして数える。
ログは14日、パッケージの非現行バージョンは30日保持する。
利用量未指定の費用表示は実運用の見積もりにならない。

## share-oj.netのDNSとHTTPS

`domain/`は購入時に作成されたRoute 53ホストゾーン、DNS検証用ACM証明書、API Gatewayの独自ドメインとAPIマッピングを管理する。
`www.share-oj.net`をフロントエンド、`api.share-oj.net`をGo APIへ接続する。
それぞれにAレコードとAAAAレコードを作成する。
ドメインの登録と自動更新はRoute 53コンソールで管理する。

このルートは`judge/dev/domain.tfstate`を使い、管理者が手動で適用する。
アプリのCIは構成を検証するが、ドメイン用stateやDNSを更新する権限は持たない。
初回は購入済みゾーンをimportし、既存のネームサーバーを維持する。
以下のimportは移管済みであり、同じstateで繰り返す必要はない。

```console
cp infra/domain/terraform.tfvars.example infra/domain/terraform.tfvars
terraform -chdir=infra/domain init \
  -backend-config="bucket=$JUDGE_STATE_BUCKET" \
  -backend-config="key=judge/dev/domain.tfstate" \
  -backend-config="region=ap-northeast-1"
terraform -chdir=infra/domain import aws_route53_zone.site Z06837452XP9XAWEH2PB9
terraform -chdir=infra/domain plan -out=domain.tfplan
terraform -chdir=infra/domain apply domain.tfplan
```

`frontend_api_id`と`backend_api_id`には、それぞれ既存フロントエンドとGo APIのHTTP API IDを指定する。
APIを置き換えた場合は、この入力とAPIマッピングも更新する。
証明書の自動更新に使うDNS検証用CNAMEは保持する。
ゾーンには`prevent_destroy`を設定している。

APIとフロントエンドの`public_site_url`は両方とも`https://www.share-oj.net`にする。
これによりCognitoのコールバック、フロントエンドの同一オリジン検証、canonical URLを揃える。
devのデプロイワークフローはこのURLをコード内で固定し、GitHub変数`PUBLIC_SITE_URL`は参照しない。
手動適用時も`TF_VAR_public_site_url=https://www.share-oj.net`を指定する。
APIの`public_api_url`は`https://api.share-oj.net`に設定し、フロントエンドの`api_endpoint`へ渡す。
APIの`api_endpoint`、`health_url`、`login_url`出力もこの独自ドメインを使う。
切り替え後は次のコマンドで公開ページ、静的ファイル、API接続を確認する。

```console
node web/scripts/smoke-frontend.mjs https://www.share-oj.net
```

2026年9月13日のInfracost解析では、ドメイン用構成の固定費は既存ホストゾーンの月$0.50で、費用とタグのポリシー違反は0件だった。
既存ゾーンを引き継ぐため、今回の変更による固定費の増加はない。
ドメイン更新料、DNS検証用レコードへのクエリ、アプリ本体のリクエスト料金などは別途かかる。

## Cognitoによるログイン

`api/auth.tf`がメールアドレスでサインインするUser Poolと、API専用のアプリクライアントを作成する。
メールログインは独自画面から`POST /auth/login`を呼び出す。
Googleログインを設定した場合はCognitoドメインとGoogle Identity Providerも作成する。
Lambdaには`COGNITO_USER_POOL_ID`、`COGNITO_CLIENT_ID`、`COGNITO_CLIENT_SECRET`が自動設定される。
クライアントシークレットはsensitive出力でフロントエンドへ引き渡す。
stateと保存済みplan、Lambdaの環境変数には含まれるため、これらの読み取り権限を制限する。

セルフサインアップを有効にし、`/signup`から登録したメールアドレスへ確認コードを送る。
既存のUser Poolでサインアップを利用するには、`allow_admin_create_user_only = false`への変更を適用する。
メール認証はリンクではなく確認コードを使う。
MFA登録画面が未実装のためMFAは無効とし、管理者が作成したユーザーには仮パスワードからの変更を求める。
パスワードは12文字以上で英大文字、英小文字、数字、記号が必要であり、仮パスワードの有効期間は7日である。
アクセストークンとIDトークンは60分、リフレッシュトークンは30日有効である。
APIのトークン更新処理は未実装のため、現時点では期限切れ後に再ログインする。

デプロイ後のUser Pool IDとクライアントIDは次のコマンドで確認できる。

```console
terraform -chdir=infra/api output -raw cognito_user_pool_id
terraform -chdir=infra/api output -raw cognito_client_id
terraform -chdir=infra/api output -raw login_url
```

テストユーザーは`/signup`から登録できる。
AWSコンソールで管理者がユーザーを作成する方法も利用できる。
メールアドレスを設定して仮パスワードを発行し、[ログインAPI](../api/README.md#ログインapi)でログインする。
`NEW_PASSWORD_REQUIRED`が返ったら`POST /auth/challenge`へ新しいパスワードを送る。
ユーザーのパスワードはTerraformで管理しない。

User PoolにはCognitoの削除保護とTerraformの`prevent_destroy`を設定している。
サインイン属性などの変更で置換が必要になった場合は、ユーザーの移行方法を決めてから保護設定を変更する。

料金プランはパスワード認証に対応するLiteを明示する。
InfracostではUser Poolの料金が未対応であり、利用量未設定のスキャン結果にある月額0ドルは実運用の見積もりではない。
2026年9月8日時点の[AWS料金表](https://aws.amazon.com/cognito/pricing/)では、Liteの直接ログインはアカウントまたはAWS組織ごとに月1万MAUの無料枠がある。
API Gateway、Lambda、ログ、メールなどの料金と、他のUser Poolによる無料枠の消費は別途確認する。
組織のコストガードレールは月額250ドルの増加をブロックするが、今回のスキャンには比較元とCognitoの利用料金がないため、この上限への適合は未判定である。

APIルートの共通タグには`Service=judge`を追加し、`Environment`は`dev`から`Dev`へ表記を揃える。
これは組織のタグポリシーに合わせた変更で、既存リソースにもタグ更新が発生する。
環境名を`stage`または`prod`にした場合のタグは`Stage`または`Prod`になる。
リソース名やstateのkeyに使う環境名は変わらない。

## CI/CD

`.github/workflows/ci.yml`はGoのテスト・静的解析、フロントエンドの型チェック・ブラウザーテスト、各Lambdaのビルド、Terraformの整形確認・`validate`・モックテストを実行する。
フロントエンドのLambdaテストは、ZIPをリポジトリ外へ展開してSSR・API接続・静的ファイル・404を確認する。
Terraformのテストは実際のAWSリソースを作成しない。
ローカルでも同じ検証を実行できる。

```console
make -C api package
terraform fmt -check -recursive infra
for root in bootstrap api; do
  terraform -chdir="infra/$root" init -backend=false -input=false -lockfile=readonly
  terraform -chdir="infra/$root" validate
  terraform -chdir="infra/$root" test
done
```

`.github/workflows/deploy-dev.yml`は`main`へのpushと手動実行を契機に、共通のCIワークフローを呼び出す。
CIが成功した場合だけ、APIのplan・apply、DBマイグレーション、フロントエンドのplan・applyの順に進む。
CIではPostgreSQL 17を起動し、GoのDB統合テストとアカウントの永続化ブラウザーテストも実行する。
公開確認は新規DBの空の一覧にも対応し、固定の問題データを必要としない。
APIのヘルスチェックと公開フロントエンドのSSR・静的ファイル確認を行い、ActionsのSummaryにURLを出力する。
bootstrapとdeploy-accessのapplyは管理者が手動で行う。
同時デプロイは一つに制限し、実行中のデプロイを後続のpushで中止しない。

GitHub ActionsはOIDCでAWSのデプロイロールを引き受ける。
初回にOIDCプロバイダー（`https://token.actions.githubusercontent.com`、Audienceは`sts.amazonaws.com`）とIAMロールを作成する。
Trust policyの`sub`条件を対象リポジトリのEnvironment `dev`に限定する。
詳細は[GitHubのAWS向けOIDC設定手順](https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-in-aws)を参照する。
ロールにはAPIとフロントエンドの更新に必要な操作権限と、実行ロールに限定した`iam:PassRole`、前述のstateとロックへの権限を設定する。
具体的な範囲は後述の`deploy-access/`で管理する。
Lambdaの実行ロールにはCognitoの管理権限を追加しない。

GitHubのEnvironment `dev`を作成し、次のVariablesを設定する。

| 変数 | 値 |
| --- | --- |
| `AWS_ACCOUNT_ID` | 12桁のAWSアカウントID |
| `AWS_DEPLOY_ROLE_ARN` | デプロイ用IAMロールのARN |
| `TF_STATE_BUCKET` | bootstrapが作成したstate用バケット名 |
| `GOOGLE_CLIENT_ID` | Google OAuthクライアントID。Googleを使わない場合は空 |
| `OPERATOR_SUBJECTS` | 運営ユーザーのCognito subをカンマ区切りで指定。省略可 |

ローカルとCIは同じアカウント、東京リージョンのstateバケット、`judge/dev/api.tfstate`を使う。
初回の自動デプロイ前にbootstrapとVariablesの設定を済ませる。
今回追加したRDS・IPv6ネットワーク・Google連携・テストデータ用S3の操作権限は、管理者が`deploy-access/`のplanを確認してapplyする。
GitHubのデプロイロールは自身の権限を更新できないため、この更新を済ませてからpushする。
ブランチ保護の必須チェックには`CI / Test and package API`と`CI / Test SSR frontend`を指定する。

Googleを使う場合は、Environment `dev`のSecretに`GOOGLE_CLIENT_SECRET`も設定する。
Google CloudでWebアプリ用OAuthクライアントを作り、承認済みリダイレクトURIを`https://<Cognitoドメイン>/oauth2/idpresponse`に設定する。
CognitoドメインはUser Pool IDの小文字化・アンダースコアのハイフン置換を接頭辞とする。
東京では`https://<接頭辞>.auth.ap-northeast-1.amazoncognito.com`となり、apply後は`cognito_domain`出力でも確認できる。
アプリ側のコールバックは`public_site_url`に`/auth/google/callback`を付けたURLとしてTerraformが設定する。
`environment = "dev"`でGoogleログインが有効な場合は、`http://localhost:3000/auth/google/callback`も許可するため、applyのたびに手動で追加する必要はない。
デフォルトのリダイレクト先は公開サイトのURLを維持する。
OAuth同意画面がテスト公開の場合は、Google側でテストユーザーの登録も必要になる。
Googleを使わない場合はIDとSecretを両方空にして、メール認証だけでデプロイできる。

手動でフロントエンドをデプロイするときは、APIの`cognito_domain`、`cognito_client_id`、`cognito_client_secret`出力を同名の`TF_VAR_*`へ渡す。
クライアントシークレットをログへ出力しない。
Actionsではこの引き渡しとログのマスキングを自動で行う。

## GitHubデプロイロール

`judge-dev-github-deploy`には`deploy-access/`で専用のインラインポリシーを設定する。
この構成はコンソールで作成済みの同名ロールをimportし、再作成せず更新する。
OIDCプロバイダーは事前に作成する。

GitHubのsubjectには所有者ID・リポジトリIDを含む場合があるため、名前だけで組み立てない。
次のAPIで`sub_claim_prefix`を確認し、その末尾に`:environment:dev`を付けた値を`github_subject`へ設定する。

```console
gh api repos/OWNER/REPO/actions/oidc/customization/sub
cp infra/deploy-access/terraform.tfvars.example infra/deploy-access/terraform.tfvars
```

例の各値を実環境に合わせて入力し、管理者のAWSプロファイルで実行する。
`gateway_ids`にはAPIとフロントエンド双方のHTTP API IDを指定する。

```console
terraform -chdir=infra/deploy-access init \
  -backend-config="bucket=$JUDGE_STATE_BUCKET" \
  -backend-config="key=judge/dev/deploy-access.tfstate" \
  -backend-config="region=ap-northeast-1"
terraform -chdir=infra/deploy-access plan -out=access.tfplan
terraform -chdir=infra/deploy-access apply access.tfplan
```

信頼関係はGitHubのAudienceとdev環境のsubjectに一致する場合だけを許可する。
操作権限はアプリのstate・ロック、dev用パッケージバケット、Lambda、ログ、指定済みのGateway・Cognito、アプリのタグを持つネットワーク、指定名のRDSに限定する。
IAM操作と`PassRole`の対象はAPI・フロントエンドの実行ロールだけにする。
全体サービスのIAMとSTSを除き、東京以外のリージョンへの操作は拒否する。
GitHubデプロイロール自身とそのstateの更新権限は付けない。
GatewayやUser Poolの新設・置換は管理者が実施し、IDをこの構成へ反映してからCIを再開する。

`Could not assume role with OIDC`は操作権限に到達する前の認証エラーなので、信頼関係を確認する。
認証後の`AccessDenied`は、失敗した操作と対象ARNに対応する許可ポリシーを確認する。
GitHub Environment `dev`のデプロイ対象ブランチも運用に合わせて制限する。

## dev環境の管理

既存のdev APIは、CloudFormationからリソースを保持してTerraformへ移管済みである。
API GatewayとLambdaをimportし、既存URLを維持している。
`infra/api/main.tf`のIAMロール名とLambda PermissionのStatement IDは、旧リソースの実際の値に合わせている。
別の環境を作る場合は、この2つの識別子も環境に合わせて設定する。
移管後はこのAPIをCloudFormationで再デプロイせず、API専用stateからTerraformで更新する。

## CloudFormationで作成済みの環境

この書き換えはリポジトリの構成を変更するものであり、既存のCloudFormationスタックを自動的にTerraformへ移管しない。
既存リソースがある場合は、CIの自動デプロイを停止し、以下の順序で移管する。
未デプロイの場合はこの作業は不要である。

1. 既存スタックのテンプレート、パラメータ、Outputs、物理リソースIDを保存する。
2. 移管対象に`DeletionPolicy: Retain`と`UpdateReplacePolicy: Retain`を設定してスタックを更新する。
3. 保持設定を確認してから対象をスタックから外す。全体を移管する場合はスタックを削除してもよいが、対象がすべて保持されることを事前に確認する。
4. state用bootstrapを作成し、APIのbackendを初期化する。
5. 実際の名前に合わせてTerraform構成を調整し、各リソースをimportする。自動生成されたS3名やIAMロール名は、`bucket_prefix`や`name_prefix`を実際の`bucket`や`name`に置き換えておく。
6. planを確認し、意図しない削除や再作成をなくしてからapplyとCIを再開する。

importは`terraform -chdir=infra/api import ADDRESS ID`の形式で実行する。
APIの主な対応は次のとおりである。

| 旧論理ID | Terraformアドレス | import ID |
| --- | --- | --- |
| `DeploymentArtifactBucket` | `aws_s3_bucket.artifacts` | バケット名 |
| バケットのバージョニング設定 | `aws_s3_bucket_versioning.artifacts` | バケット名 |
| バケットの暗号化設定 | `aws_s3_bucket_server_side_encryption_configuration.artifacts` | バケット名 |
| バケットの所有権設定 | `aws_s3_bucket_ownership_controls.artifacts` | バケット名 |
| バケットの公開拒否設定 | `aws_s3_bucket_public_access_block.artifacts` | バケット名 |
| バケットのライフサイクル設定 | `aws_s3_bucket_lifecycle_configuration.artifacts` | バケット名 |
| `DeploymentArtifactBucketPolicy` | `aws_s3_bucket_policy.artifacts` | バケット名 |
| `ApiLambdaLogGroup` | `aws_cloudwatch_log_group.lambda` | ロググループ名 |
| `ApiLambdaRole` | `aws_iam_role.api` | ロール名 |
| ロール内の`cloudwatch-logs`ポリシー | `aws_iam_role_policy.logs` | `ロール名:cloudwatch-logs` |
| `ApiLambda` | `aws_lambda_function.api` | 関数名 |
| `HttpApi` | `aws_apigatewayv2_api.api` | API ID |
| `ApiIntegration` | `aws_apigatewayv2_integration.api` | `API ID/Integration ID` |
| `DefaultRoute` | `aws_apigatewayv2_route.default` | `API ID/Route ID` |
| `ApiGatewayLogGroup` | `aws_cloudwatch_log_group.api_gateway` | ロググループ名 |
| `DefaultStage` | `aws_apigatewayv2_stage.default` | `API ID/$default` |
| `ApiGatewayInvokePermission` | `aws_lambda_permission.api_gateway` | `関数名/Statement ID` |

Lambda Permissionの`statement_id`も既存の値に合わせる。
`aws_s3_object.api_package`は新しい固定キーへアップロードするため、既存のハッシュ付きパッケージをimportする必要はない。

詳細は[HashiCorpのリソースimport手順](https://developer.hashicorp.com/terraform/cli/import/usage)を参照する。

## 公開範囲

現在の`$default`ルートは認証なしで公開される。
`GET /health`、`POST /auth/login`、`POST /auth/challenge`は認証前に呼び出すエンドポイントである。
業務APIを追加する際は、API GatewayのJWT AuthorizerやAPI内のトークン検証を追加する。

### 外部プロフィールのキャッシュ

フロントエンド Lambda は VPC 外で外部 API を取得する。
公開レーティングと公開ユーザー名は Nitro の `defineCachedFunction` で300秒キャッシュし、同じキーの取得中リクエストは結果を共有する。
キャッシュは実行環境内のメモリに最大1,000件保持する。
外部 API の失敗結果もキャッシュし、障害中の連打による再取得を抑える。
実行環境の新規起動・終了や件数上限による追い出しでは再取得が発生し、実行環境をまたぐ重複は許容する。
キャッシュ用の Valkey・Lambda・VPC は作成しない。
`npm run package:lambda && npm run test:lambda` で、同時取得の共有・キャッシュヒット・期限切れ後の再取得を検証できる。

以前の共有キャッシュ構成をデプロイ済みの場合は、`infra/frontend` の削除 plan を確認して適用した後に `infra/deploy-access` を適用する。
キャッシュの削除に必要なデプロイ権限を先に外さないこと。
削除時に `ec2:DeleteNetworkInterface` または `iam:ListInstanceProfilesForRole` が拒否された場合は、管理者がデプロイロールに一時ポリシーを追加してから、失敗したデプロイジョブを再実行する。
ENI の削除許可はエラーに出た ENI の ARN に、IAM の参照・削除許可は旧 `${project_name}-${environment}-web-cache` ロールの ARN に限定する。
Lambda が作成した ENI にアプリのタグが付いているとは限らないため、既存のタグ条件付きネットワーク権限への追加だけでは解消しない。
再実行は更新済みの Terraform state から残った削除を行うため、state からリソースを手動で除外しない。
デプロイ成功後に一時ポリシーを削除し、`infra/deploy-access` を適用して通常の実行ロールにも `iam:ListInstanceProfilesForRole` を反映する。
