# Lightsail上のisolateジャッジ

Ubuntu 24.04のLightsail（IPv6のみ、2 GB、2 vCPU）で、ホストごとに提出を1件ずつ採点する。
`infra/judge`の`worker_count`で台数を指定し、各ホストが同じSQS要求キューを受信する。
2台なら最大2提出を同時に採点できる。
C、C++、Python、RustとJavaを構築し、実機検証済みのランタイムを段階的に公開する。
再構築・SSM接続・段階公開の操作は[ランタイムの再構築と公開](../docs/judge/runtime-rollout.md)に従う。
日々の配布と実機検証は[定型コマンド](../docs/judge/deployment-checks.md)を使い、SSMコマンドIDと合否レポートを保存する。
APIとDBは既存のAWS環境を使い、専用の管理ワーカーがSQSとS3を介して処理する。
Terraformは[infra/judge](../infra/judge/)に置く。

採点ホストは、EC2の常時稼働1台とコンテスト時に起動する増設の台へ移行する（[ADR 0012](../docs/adr/0012-run-judge-on-ec2-pool.md)）。
EC2の台の構成、台数の制御、切り替え手順は[EC2の採点台の運用と切り替え](../docs/judge/ec2-pool.md)に従う。
この文書のLightsailに関する手順は、切り替えが終わるまで有効である。

## 隔離と制限

提出ごとのVMは作らず、isolateの名前空間、UID 60000、cgroup v2で隔離する。
提出と管理ワーカーはカーネルを共有するため、カーネル侵害時には認証情報や期待出力を含むホスト全体が影響を受け得る。
[ADR 0006](../docs/adr/0006-use-lightsail-and-isolate.md)でこの変更を記録する。
APIやDBをこのインスタンスに同居させず、VPCピアリングも設定しない。

| 対象 | 制限 |
| --- | --- |
| 同時採点 | 1ホストにつき1提出。手動テストもホスト内の同じロックを使用 |
| コンパイル | CPU 30秒、経過40秒、1 GiB |
| ケース実行 | CPU 100〜5000 ms、経過はCPU上限の3倍＋1秒、512 MiB |
| ワーカー全体 | systemdで1.5 GiB、128タスク、CPU 1個相当、swapなし |
| 作業領域全体 | tmpfs 512 MiB、16,384 inode。サービス停止時に破棄 |
| 提出プロセス | 64プロセス、64ファイル記述子 |
| 出力 | 標準出力16 MiB、標準エラー64 KiB |

コンパイル用とケースごとのisolate環境は毎回破棄する。
コンパイル成果物だけを管理側で保持し、各ケースへコピーする。
提出から見える`/etc`は合成したpasswdとgroupのみで、認証情報、管理ファイル、期待出力は公開しない。
ネットワーク名前空間は外部につながず、AWSへの通信は管理ワーカーだけが行う。
清掃に失敗した場合はプロセスを終了し、systemdによる子孫回収と一時領域破棄後に再起動する。

CPU時間と経過時間はms、最大メモリはcgroupのピーク値をbyteで返す。
子孫プロセスと課金されるキャッシュを含み、RSSとは異なる。
Lightsailはバースト可能なCPUなので、CPU残高や共有ホストの負荷で結果が変動し得る。
競技大会の厳密な順位付けに使う前に、繰り返し実行とCPU残高低下時の計測を検証する。

現在はソース64 KiB、100ケース以下、各入出力16 MiB、全入出力512 MiB以下が対象である。
通常の提出は、連続かどうかに関係なく計2ケースがTLEになった時点で採点を打ち切り、残りを未実行（SKIPPED）として記録する。
最終判定は従来どおり最初の不正解判定を保持し、正解数は実行済みケースだけを数える。
サンプル検証、入力検証、出力生成は打ち切らず全ケースを実行する。
64 KiBを超える入出力は、実行直前にS3から取得する。
入力ファイルは不変のS3 VersionIdとSHA-256で検証する。

## AWS環境の作成

操作用端末にはIPv6接続、SSH、AWS CLI、Terraform 1.10以上、Goを用意する。
LightsailプランとUbuntu blueprintは作成前に対象リージョンで確認する。

```sh
aws lightsail get-bundles --region ap-northeast-1 \
  --query "bundles[?bundleId=='small_ipv6_3_0'].{id:bundleId,price:price,ram:ramSizeInGb}"
aws lightsail get-blueprints --region ap-northeast-1 \
  --query "blueprints[?blueprintId=='ubuntu_24_04'].{id:blueprintId,active:isActive}"
make -C api package package-judge
# 先に「ランタイムの再構築と公開」に従って .build/runtime.tar.gz を構築する。
bash judge/build-assets.sh
cp infra/judge/terraform.tfvars.example infra/judge/terraform.tfvars
```

`terraform.tfvars`にSSH公開鍵、自分のIPv6アドレスの`/128`、既存APIの`judge_bridge_database`出力と`test_data_bucket`出力を設定する。
`runtime_digest`は空、`enabled=false`で最初の作成を行う。
S3 backendは既存bootstrapのバケットを使い、stateのキーをAPIと分ける。

```sh
terraform -chdir=infra/judge init \
  -backend-config='bucket=REPLACE' \
  -backend-config='key=judge/dev/terraform.tfstate' \
  -backend-config='region=ap-northeast-1'
terraform -chdir=infra/judge plan
terraform -chdir=infra/judge apply
terraform -chdir=infra/judge output worker_ipv6_addresses
```

SSHは指定したIPv6アドレスだけを許可し、HTTPポートは公開しない。
ホスト側もnftablesで着信を制限し、外向きはIPv6 HTTPSとDNS、時刻同期などに絞る。
NAT、ロードバランサー、パブリックIPv4は作成しない。
SSHのホスト鍵を確認した後、cloud-initの完了を待つ。

```sh
export JUDGE_IPV6='REPLACE'
ssh -6 "ubuntu@$JUDGE_IPV6" 'sudo cloud-init status --wait'
bash judge/deploy.sh
```

配置スクリプトはリリースのSHA-256を照合し、固定コミットのisolateをホストでビルドする。
Docker、Firecracker、カスタムカーネルは不要である。
インストール時にコード、isolate、システムパッケージ一覧、カーネルの指紋を記録し、ランタイムのdigestを表示する。
この値を`terraform.tfvars`の`runtime_digest`に設定する。

## 認証情報と有効化

LightsailはEC2インスタンスプロファイルを使わない。
Terraformが作る専用IAMユーザー名を`worker_iam_user`出力で確認し、そのユーザーのアクセスキーを運用者が作成する。
権限は要求キューの受信、結果キューへの送信、入力オブジェクトのVersion指定取得に限る。
キーはTerraformでは作らず、state、user-data、Git、ログに含めない。

SSHで`sudoedit /root/.aws/credentials`を実行し、次の形式で設定する。
保存先の所有者をroot、権限を0600にする。

```ini
[default]
aws_access_key_id = REPLACE
aws_secret_access_key = REPLACE
```

```sh
# digestを反映。まだenabled=falseのまま。
terraform -chdir=infra/judge apply
terraform -chdir=infra/judge output -raw worker_environment | \
  ssh -6 "ubuntu@$JUDGE_IPV6" 'sudo tee /opt/judge/worker.env >/dev/null; sudo chmod 600 /opt/judge/worker.env'
ssh -6 "ubuntu@$JUDGE_IPV6" 'sudo /opt/judge/smoke.sh'
```

smokeは本番と同じsystemdのメモリ上限とマウント設定で、AC、WA、CE、TLE、MLE、OLE、秘密ファイルへのアクセス拒否、ネットワーク遮断、ケース間のファイル破棄を確認する。
続いて[APIのマイグレーション手順](../infra/README.md)で`007_multilanguage_dispatch.sql`まで適用する。

```sh
ssh -6 "ubuntu@$JUDGE_IPV6" 'sudo systemctl enable --now judge-worker'
```

ワーカーのログを確認してから`infra/judge`の`enabled=true`を適用する。
API側の`judge_runtime_digest`にも同じdigestを設定し、APIをデプロイする。
GitHub ActionsでAPIをデプロイする場合はリポジトリ変数`JUDGE_RUNTIME_DIGEST`にも設定する。
APIは公開リストに含まれる言語だけを受け取り、保存時に`-isolate`識別子とdigestを固定する。
公開リストの既定値は`cpp17`だけである。
`GET /runtimes`と提出受付は同じ公開設定を参照する。
最後に実際のAPIから1提出してDBへの結果反映まで確認する。

## 更新と障害対応

複数台では全ホストのruntime digestをそろえ、それぞれの実機smoke合格後に起動する。
全台の更新中は受付を止め、要求キューだけでなくDBの未dispatch提出と処理中の提出もなくなるまで待つ。
`terraform output -json workers`で全ホストの名前とIPv6を確認できる。
既存の`worker_instance_name`と`worker_ipv6_addresses`出力は1台目を返す。
CloudWatch Agentには`cloudwatch_agent_configs`のホスト名に対応する設定を配置する。
各ホストのログストリームと`Worker`ディメンションを分け、片方だけの停止も検知する。

検証済みホストのスナップショットから増設する場合、複製先で元のSSM登録や採点サービスを起動させない。
複製先を別のSSM管理ノードとして登録し、Terraformの対応する`aws_lightsail_instance.worker[N]`へimportする。
`user_data`は初回作成専用で、変更しても登録済みホストを置換しないため、既存OSの変更は明示的な配置操作で行う。

IPv6アドレスはDHCPv6のリース更新を必要とする。
ホストのファイアウォールではDHCPv4（UDP 67→68）とDHCPv6（UDP 547→546）の受信を明示的に許可する。
マルチキャストやブロードキャスト宛ての要求への応答を、`ct state established,related`だけに依存させない。
SSHが時間経過後に切れる場合は、アドレスの有効期限と`journalctl -b -u systemd-networkd`を確認する。

接続できる検証用ホストでファイアウォールを変更するときは、採点ワーカーを停止した状態で行う。
適用予定のルールを`/tmp/judge-firewall.nft`に用意し、SSHの許可元が現在の操作端末と一致することを確認する。
ブラウザSSHの接続元は操作端末のIPv6とは異なるため、許可元を制限する前に端末からのSSH接続を確認する。
次の操作はホスト上で実行し、15分後に変更前のルールへ自動で戻す。

```sh
sudo nft -c -f /tmp/judge-firewall.nft
sudo sh -c 'umask 077; { echo "flush ruleset"; nft list ruleset; } > /root/judge-firewall-before.nft'
sudo systemd-run --unit=judge-firewall-rollback --on-active=15m /usr/sbin/nft -f /root/judge-firewall-before.nft
sudo nft -f /tmp/judge-firewall.nft
```

ルールはこの時点では永続化しない。
現在のリースの有効期間を超えてもIPv6アドレスが維持され、新しいSSH接続が成功することを確認する。
手動で戻す場合は`sudo systemctl start judge-firewall-rollback.service`を実行する。
確認中はロールバックのタイマーを解除しない。
接続不能な元ホストの設定もスナップショットには残るため、コピーだけで設定不良が解消するとは限らない。
user-dataの変更は既存OSへの修正適用ではないため、復旧中にTerraformを無条件でapplyしない。

更新時はAPIの受付とdispatchを止め、処理中の提出が完了してからワーカーを停止する。
TLEノックアウト対応を初めて反映する際は、SKIPPEDを受信できるjudge-bridgeを先にデプロイし、その後ワーカーを更新する。
OS更新後も指紋が変わるため、新しいdigestを登録するまでワーカーは要求を受信しない。
必要な再起動を済ませ、`fingerprint.py`、smoke、Terraformのdigest更新、環境ファイル更新を順に実施する。
旧digestの待機提出は再現できないためJEとなる。更新前にキューを空にする。
アクセスキーは定期的にローテーションし、古いキーを無効化して削除する。

SQSは要求の可視性期限2,100秒、3回失敗でDLQ、通常キュー1日、DLQ14日の保持とする。
ホストのOOMや強制停止時はメッセージが再配信される。
結果は提出IDと試行IDで照合し、確定済み結果を重複配信で上書きしない。
作成から6時間経過しても完了しない提出は、dispatch実行時にJEへ確定する。
CloudWatchへのログ集約と、原因別のDiscord通知は[監視と障害対応](../docs/judge/observability.md)に従って設定する。

## 費用と検証範囲

AWS公式料金のIPv6専用2 GB Linuxプランは月10 USDで、SSD 60 GBを含む。
2台構成のインスタンス料金は月20 USDとなる。
DLQアラーム2件の見積もりは月0.20 USDで、S3、SQS、Lambda、ログ、超過通信と既存API/DBの料金は別となる。
Infracostは同じプランを730時間で約11.77 USDと見積もっており、公式の月額と差がある。
見積もりだけで月額を確定せず、作成前の`get-bundles`とAWS料金を確認する。
停止状態でもLightsailのインスタンス料金は発生するため、停止による節約は前提としない。

Ubuntu 24.04の使い捨て環境を2 GBに制限し、本番と同じsystemd設定でsmokeが通ることを確認した。
Pythonの単体テスト、GoのDB結合テスト、Terraformのモックテストも実施した。
この検証はLightsail実機のCPU性能とIPv6通信を保証しない。
2 GB実機でのsmokeとAPI経由の往復確認を有効化の条件とする。

- [Lightsail料金](https://aws.amazon.com/lightsail/pricing/)
- [isolateマニュアル](https://github.com/ioi/isolate/blob/master/isolate.1.txt)
