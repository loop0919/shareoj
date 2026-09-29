# ADR 0006: Lightsailの2 GBインスタンスでisolateを使う

- 状態：Superseded by [ADR 0012](0012-run-judge-on-ec2-pool.md)（採点ホスト基盤。isolateによる隔離と計測の決定は引き継ぐ）
- 決定日：2026-09-11
- 置き換え対象：[ADR 0005](0005-use-firecracker-and-isolate-on-ec2.md)

## 背景

個人プロジェクトとしての固定費を抑える。
EC2のc7i.largeとFirecrackerによる構成は常時稼働で約85 USD/月の見積もりとなった。
利用者は、提出ごとのVM隔離を省く違いを確認したうえで、Lightsailの2 GBプランとisolateを選択した。

## 決定

IPv6専用のLightsail Linux 2 GBプランを使い、Ubuntu 24.04上でisolateを直接動かす。
同時採点数は1とし、コンパイル用の上限を1 GiB、ケース実行用を512 MiBにする。
コンパイルと各ケースに新しいisolate環境とcgroupを作成し、CPU時間、経過時間、最大メモリを計測する。
Firecracker、jailer、提出専用VM、ゲストイメージのビルドは使用しない。

管理ワーカーだけがSQSとS3へアクセスし、APIとDBは別環境に置く。
LightsailとAPIのVPCをピアリングしない。
専用IAMユーザーの権限を要求受信、結果送信、Version指定の入力取得に限定し、アクセスキーはTerraformのstateやuser-dataに保存しない。
入力と成果物だけをサンドボックスへ渡し、期待出力、認証情報、管理ファイルは渡さない。

サンドボックスは外部と通信できないネットワーク名前空間を使う。
ホストのSSHは運用者のIPv6アドレスに限定する。
ケース終了時は子孫プロセスと作業領域を回収し、回収できなければワーカーを終了する。
systemdでもメモリとタスク数を制限し、作業領域は容量とinode数に上限のあるtmpfsとする。

## セキュリティと計測への影響

提出と管理処理は同じLinuxカーネルを共有する。
カーネルやisolateの脆弱性から脱出された場合、他の提出、期待出力、ホスト上の限定されたAWS認証情報、計測結果が侵害され得る。
ADR 0005の提出ごとのカーネル分離と同等の安全性は主張しない。
ホストをジャッジ専用にし、OSとisolateの更新、認証情報のローテーション、再構築を運用側で担う。

LightsailのCPUはバースト可能であり、CPU残高や共有ホストの競合が経過時間や計測のばらつきに影響する。
同時実行を1件にしても、この影響を排除できない。
公開競技で厳密な実行時間比較が必要になった時点で、CPU資源とVM隔離を再検討する。

## 引き継ぐ設計

非同期受付、Outbox、SQSによる再試行、試行IDによる結果の冪等性は維持する。
S3のVersioningとチェックサム照合、管理側での期待出力比較も維持する。
ADR 0003の大規模なテストセット取り込みとバンドル形式は変更せず、今回の実装は既存DBの小さなテストセットを扱う。

## 検証と費用

2 GB実機での制限超過、ネットワーク遮断、秘密ファイルの非公開、ケース間の清掃、APIからの往復を確認してから受付を有効化する。
実装と手順は[ジャッジのREADME](../../judge/README.md)、処理と計測の定義は[実行モデル](../judge/execution-model.md)に記録する。

AWS公式料金のIPv6専用2 GBプランは月10 USDである。
Infracostの見積もりとは差があるため、料金と対象リージョンのプランを作成前に照合する。

- [Lightsail料金](https://aws.amazon.com/lightsail/pricing/)
- [isolateマニュアル](https://github.com/ioi/isolate/blob/master/isolate.1.txt)
