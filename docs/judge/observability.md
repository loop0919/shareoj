# ジャッジの監視とDiscord通知

ワーカーのJSONログをCloudWatch Logsへ転送し、CloudWatch AlarmからSNSと専用Lambdaを経由してDiscordへ通知する。
通知処理はLightsailに依存しない。
ワーカーはjournalと`/var/log/judge/worker.jsonl`へ出力し、ファイルは5 MiBを3世代、CloudWatch Logsは14日間保持する。

## JEの分類

利用者向けのJEと、運用ログの`category`を分ける。
結果のJSON形式とDBスキーマは変更しない。

| category | 発生箇所 | 通知 |
| --- | --- | --- |
| `judge_code` | チェッカーやインタラクタのコンパイル失敗、実行制限超過、testlibの失敗終了 | 作問コードの確認が必要 |
| `platform` | isolateの起動と清掃、ランタイム照合、S3/SQS/DBの処理 | ジャッジ基盤の異常 |
| `unknown` | 原因未分類の例外やJE | ジャッジ基盤の異常（ログで未分類と確認） |

`judge_code`は作問者の過失を断定する分類ではない。
チェッカーの実行中でも、isolateや外部サービスの失敗は`platform`とする。
清掃失敗は先行するチェッカーの失敗より優先し、ワーカーを停止する。
legacy checkerの非ゼロ終了など、既存のWA判定規則は維持する。
WA、CE、TLE、MLE、OLE、REという提出結果だけでは障害通知しない。

`judge_code`アラームの解除は「直近の検出なし」と通知する。
これは修正や再検証の成功を意味しない。
集約した5分間の検出件数で状態を判定するため、同じ状態が続く間は個々のJEごとの通知は送らない。

## ログの調べ方

ワーカーは`/judge/judge-dev/worker`、bridgeは`/aws/lambda/judge-dev-judge-bridge`へ出力する。
Logs Insightsの保存済みクエリ`judge-dev/judge-failures`で障害の分類と提出IDを確認する。
提出IDを得たら、両方のロググループを選び、次のクエリで配送から結果処理まで追う。

```text
fields @timestamp, service, event, category, reason, submissionId, attemptId, phase, verdict, durationMs
| filter submissionId = "確認する提出ID"
| sort @timestamp asc
| limit 200
```

`request_sent`、`job_received`、`judge_started`、`judge_finished`、`result_sent`、`result_processed`を順に確認する。
`result_processed`は結果の重複配信を正常に無視した場合も記録するため、実際の確定状態は提出画面またはDBと照合する。
`durationMs`はワーカーの受信からの時間、bridgeでは各処理の経過時間である。
ソース、テスト入出力、コンパイラ診断、資格情報、Webhook URLはログへ出さない。
チェッカーの詳しい診断は既存の権限付き提出結果で確認する。

## アラートの確認先

| 名前の末尾 | 条件 | 最初に確認する対象 |
| --- | --- | --- |
| 台の名前（`judge-dev-judge-primary`など） | プロセス数0が3期間。常時稼働の台では欠測も含む | SSM接続、`systemctl status judge-worker amazon-cloudwatch-agent`、journal |
| `pending` | 未完了提出の最長経過時間が300秒超 | 未送信提出、要求キュー、実行中提出、結果キュー |
| `dispatch-missing` | DB観測の欠測が3期間 | 定期EventBridge、bridgeのErrors/Throttles、DB接続 |
| `platform` | 基盤または未分類のエラーが5分間に1件以上 | `category`と`reason`、該当提出のログ |
| `judge-code` | 作問コードのエラーが5分間に1件以上 | 該当提出の検証コードと診断 |
| `bridge-failure` | Lambda ErrorsとThrottlesの合計が1件以上 | bridgeのログ、Lambda同時実行上限 |
| `requests-dead` / `results-dead` | DLQに可視メッセージが1件以上 | 対応する配送失敗。原因修正前に再投入しない |
| `memory` | ホストメモリ90%以上が5期間 | ワーカーとAgentのメモリ、OOMのjournal |
| `disk` | `/`の使用率85%以上が5期間 | 配布物、退避ランタイム、ログ。必要な復旧物を確認して整理 |

収集間隔は60秒で、実際の通知にはCloudWatchの取り込みと評価の遅延もある。
提出の最大実行時間は約30分なので、長い正常採点でも5分の滞留警告が出る。
メモリはホスト全体を監視し、ケース単体のMLEとは区別する。

通知が届かない場合は`/aws/lambda/judge-dev-judge-notify`、SNS subscription、`judge-dev-judge-notification-dead`を確認する。
Discordの一時的な429は短い待機後に再試行し、それ以外の配信失敗はLambdaの非同期再試行後にDLQへ入る。
SNSからLambdaへの配信失敗も同じDLQへ入る。
Discord自体の障害は同じDiscord経路では通知できないため、DLQとLambda ErrorsをAWSコンソールで確認する。
配信はat-least-onceで、再試行時に同じ通知が重複する場合がある。

## 初回設定と更新

`make -C api package-judge`でbridgeと通知Lambdaを構築する。
`infra/judge`で以下の変数を設定し、planで既存インスタンスやキューが置換されないことを確認する。

```hcl
discord_webhook_secret_arn = "既存SecretのARN"
discord_webhook_secret_key = "ALART_DISCORD_WEBHOOK"
alerts_enabled            = false
```

SecretがプレーンURLの場合は`discord_webhook_secret_key = ""`とする。
JSONのキー名は大文字小文字も含めて一致させる。
Secretの値はTerraformで管理せず、AWSコンソールで保存する。
通知Lambdaは指定Secretの読取権限だけを持ち、ワーカーにはこの権限を渡さない。
共有Secretの場合、IAMはJSON内のキー単位には権限を分離できないため、通知LambdaはそのSecret全体を読める。

[ランタイムの更新手順](runtime-rollout.md)に従い、受付停止、DBとキューのdrain、dispatch停止、ワーカー停止、退避を実施する。
監視導入時はAWS公式のUbuntu amd64 Agentパッケージを操作端末で取得し、SHA-256を記録して専用S3経由で配布する。
`terraform -chdir=infra/judge output -raw cloudwatch_agent_config`をJSONファイルとして配置する。

```sh
sudo bash install-observability.sh amazon-cloudwatch-agent.deb 記録したSHA256 agent.json
```

LightsailのAgentはon-premiseモードで既存worker用credentialsを使い、CloudWatchのdual-stackエンドポイントへ接続する。
EC2の台では4番目の引数に`ec2`を渡し、インスタンスロールの認証情報を使う。
EC2の台の増設が停止中に鳴らない理由と、台数制御の障害の分類は[EC2の採点台の運用と切り替え](ec2-pool.md)に記載する。
インストーラーは設定原本を`/etc/judge/cloudwatch-agent.json`へ保存し、初期化時のリージョンも明示する。
メモリのメトリクスは元からディメンションがないため、`drop_original_metrics`を設定しない。
メトリクス送信権限は専用namespace、ログ送信権限は専用ロググループに制限する。
プロセス監視は通常メトリクスとディメンションなしの集約メトリクスを出し、停止検知には集約側を使う。
投稿数でディメンション数が増えることはない。

Agentインストールと制御コード配置の後、fingerprintを再生成して実機smokeを実行する。
API、bridge、worker、Terraform、GitHubのdev変数を同じ検証済みdigestに揃える。
失敗時は旧制御コード、旧manifest、旧worker環境を復元し、Agentの追加で変わったOS指紋も整合させるか、更新前スナップショットから復旧する。
検証できていないdigestでは受付を再開しない。

CloudWatchでworkerのプロセス数1、メモリとディスクのデータ、bridgeの`OldestPendingSeconds`、ログ到達を確認する。
通知Lambdaで試験通知を確認した後、`alerts_enabled=true`を適用する。
CloudWatchの試験的な状態変更で、SNS経由の異常通知と解除通知も確認する。
最後にAPIから提出して結果確定まで確認し、保守終了を記録する。

## 保守中の通知

保守前に`alerts_enabled=false`を適用して全アラームのアクションを停止する。
監視データの収集と状態評価は続く。
保守後はアラームが正常であることを確認してから`alerts_enabled=true`に戻す。
アクション再開だけでは既存のALARM状態が再通知されるとは限らないため、異常が残る場合はコンソールで原因を確認する。

## 費用の目安

カスタムメトリクスはAgentの4系列、ログ由来の3系列を想定する。
アラームは既存DLQの2個を含めて10個で、bridgeの数式アラームは2メトリクス分を評価する。
標準メトリクス月0.30 USD、標準アラームの評価メトリクス月0.10 USDで計算すると、メトリクス約2.10 USD、アラーム約1.10 USDになる。
既存DLQ分を除く追加分は約3 USDで、ログ取り込み、保存、API、SNS、Lambda、Secrets Managerの利用料金が加わる。
月0.1 GB程度のログと少量の通知を想定し、追加月5 USD程度を目安とする。
これは予算上限を強制する仕組みではない。
実際の東京リージョンの請求、ログ量、Insights検索量で見直す。
有料ダッシュボードは作成しない。

- [CloudWatch料金](https://aws.amazon.com/cloudwatch/pricing/)
- [CloudWatch Agentの設定](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/CloudWatch-Agent-Configuration-File-Details.html)
- [SNSとLambdaの連携](https://docs.aws.amazon.com/lambda/latest/dg/with-sns.html)

## dev導入記録（2026-09-16）

既存Lightsailを置換せず、受付と配送を一時停止して導入した。
CloudWatch Agentは`1.300072.0b1766-1`、配布debのSHA-256は`05baeadca96c4bb8e43906ed09cf0bebd0f321ff6d41987bdc46ce681de0978d`である。
14ランタイムの実機smokeが成功し、既存の公開12ランタイムを再開した。
API、bridge、workerと運用設定のdigestは`sha256:a6bf7d578411472140e22f69c7ca369967a781a9a23b087d187f6cc7407ecaf2`に揃えた。

Pythonの52テスト、Goの全テスト、Terraformの7モックテストが成功した。
実APIからの提出で、正常コードのACとチェッカーのコンパイル失敗によるJEを確認した。
JEのログには`judge_code / checker_compile_failed`と提出・試行IDが記録され、bridgeの`result_processed`とAPIの確定結果まで照合した。
検証用の非公開問題とCognitoユーザーは削除した。

基盤障害はCloudWatchの試験的な状態変更、作問コードの通知は実際のJEで検証した。
SNSへのアクション成功と通知Lambdaの正常終了によりDiscordへの送信成功を確認した。
基盤障害の試験解除と、作問コードアラームの自然解除（20:54 JST）でも通知が成功し、全10アラームがOK・通知有効の状態になった。
ワーカーは再起動0回、要求・結果・各DLQは空で、ホストメモリ約19%、ルートディスク約71%だった。
一時DB確認Lambdaと導入時に作成した一時スナップショットは削除し、旧制御コードと対応するmanifest・環境設定はホストの`/opt/judge-backup-observability/control.tar.gz`に残した。

Infracostによる外部解析はIaC・環境情報の送信が自動承認レビューで拒否されたため未実施である。
上記費用は公開料金に基づく概算であり、Infracostの実測結果ではない。
