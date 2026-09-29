# EC2の採点台の運用と切り替え

採点ホストは、Lightsailの2台から、EC2の常時稼働1台とコンテスト時に起動する増設の台へ移行する。
判断の経緯は[ADR 0012](../adr/0012-run-judge-on-ec2-pool.md)に記録している。
この文書では、台の役割、台数を制御する仕組み、配布の設定、初回構築から切り替えまでの手順を説明する。
切り替えが終わるまでは、Lightsailの台が本番の採点を続ける。

## 台の役割とタグ

EC2の台は`infra/judge/ec2.tf`で作る。
どの台もt3a.small（unlimitedモード）で、1台が同時に採点するのは1件である。

| 名前 | 役割 | 平常時 |
| --- | --- | --- |
| `judge-dev-judge-primary` | **primary**：常時稼働する | 起動 |
| `judge-dev-judge-burst-1`、`-2` | **burst**：コンテストと採点待ちの滞留時に起動する | 停止 |

台の数は`burst_worker_count`で変える。
インスタンスIDは`terraform -chdir=infra/judge output -json pool_hosts`で確認できる。
ワーカーは`/opt/judge/worker.env`の`JUDGE_POOL_ROLE`で自分の役割を知る。
この値はuser-dataが作成時に書き込む。

台数の制御と配布は、次のインスタンスタグでやり取りする。

| タグ | 書き込むもの | 意味 |
| --- | --- | --- |
| `JudgePool` | Terraform | bridgeと配布ツールが対象にする台の目印 |
| `JudgeRole` | Terraform | `primary`または`burst` |
| `JudgeHoldUntil` | bridge | コンテストや滞留のために台を動かしておく期限 |
| `JudgeMaintenanceHoldUntil` | `judge/rollout.py` | 配布中に台を動かしておく期限 |
| `JudgeInstalledDigest` | `judge/rollout.py` | 検証を通って導入されたruntime digest |

Terraformは最後の3つを`ignore_tags`で無視し、applyで消さない。
採点台と専用VPCには`Component=judge-worker`タグも付き、GitHubの配布ロールはこれらを参照しかできない。

## 台数制御の動き

bridgeは毎分のEventBridge実行のたびに、あるべき稼働状態を計算する。
提出ごとの即時dispatchと`deployment-status`の呼び出しでは、EC2を操作しない。
burstを起動するのは次の場合である。

- **コンテスト**：参加登録が`burst_min_participants`人（既定10人）以上のコンテストについて、開始30分前から`min(終了, 開始+3時間)+15分`まで、全burstを動かす。対象期間の合計は24時間で8時間までとし、超えた分は`capacity_capped`を記録して起動しない。
- **primaryの停止**：primaryが止まっていて採点待ちがあれば、burstを1台、30分の保持期限で起動する。
- **採点待ちの滞留**：SQSへ送った提出が2分以上受け取られていなければ、burstをもう1台、30分の保持期限で起動する。

bridgeは保持期限を延ばすだけで、短くすることはない。
起動するのは`JudgeInstalledDigest`がbridgeの`JUDGE_RUNTIME_DIGEST`と一致する台に限る。
一致しない台を起動すると受け取った提出をすべてJEにしてしまうので、`capacity_stale_instance`を記録して起動しない。
停止処理中の台は、次の実行で起動し直す。

burstは自分で電源を切る。
10分以上提出を受け取らず、`JudgeHoldUntil`と`JudgeMaintenanceHoldUntil`の遅いほうを60秒以上過ぎたら、採点と採点の間で`idle_stop`を記録して停止する。
タグをIMDSから読めない場合や値が不正な場合は停止せず、1時間に1回`pool_hold_unavailable`を記録する。
primaryは自分では停止しない。

台数制御は`capacity_enabled`で有効にする。
既定は無効で、bridgeはEC2を操作しない。

## 配布の設定

EC2の台へ配布するときは、`judge/rollout.example.json`を元に次の値を設定する。

- `pool`：`JudgePool`タグの値（`judge-dev`）。
- `nodes`：`pool_hosts`の全インスタンスID。停止中の台も含める。
- `worker_alarms`：`judge-dev-judge-primary`などの台ごとのアラーム名。
- `retire_nodes`：切り替え時だけ、退役させるLightsailのSSM ID（`mi-`形式）を指定する。

`pool`を指定すると、`rollout.py`は`prepare`、`run`、`finish`の最初に次を行う。

1. `JudgePool`タグの付いた全インスタンスと`nodes`が一致することを確かめる。一致しなければ中断する。
2. 全台の`JudgeMaintenanceHoldUntil`を現在+4時間にし、停止中の台を起動して、`running`とSSMのOnlineを待つ。

`finish`では健全性を確かめた後に`JudgeInstalledDigest`を付け、完了時に`JudgeMaintenanceHoldUntil`を現在+15分に縮める。
縮められなかった場合は`state.json`の`maintenanceHoldReleased`が`false`になり、burstは4時間の期限まで動き続ける。

`infra/judge`は公開言語を`enabled_runtimes`として必須の変数に持つ。
applyでbridgeの`JUDGE_ENABLED_RUNTIMES`を消さないためである。
初回だけは`terraform.tfvars`へ手で設定し、以後は`rollout.py`が`zz-rollout.auto.tfvars.json`へ書き込む。
未設定のままでは、`rollout.py`の事前確認で実行する`terraform plan`も失敗する。

## 初回構築と切り替え

以下はAWSの認証情報を更新し、リポジトリルートで実行する。
各段階は本番の費用や受付に影響するため、段階ごとに結果を確認してから次へ進む。

### 1. 台を作る

t3a.smallを使えるアベイラビリティゾーンを確認し、`worker_availability_zones`に指定する。
2026年9月時点では`ap-northeast-1a`と`ap-northeast-1d`で使え、`ap-northeast-1c`では使えない。

```sh
aws ec2 describe-instance-type-offerings --region ap-northeast-1 \
  --location-type availability-zone --filters Name=instance-type,Values=t3a.small
```

`infra/judge/terraform.tfvars`に`enabled_runtimes`を設定し、`capacity_enabled = false`と`pool_alerts_enabled = false`のままplanを確認する。
primaryのプロセス監視アラームは、ワーカーが動くまでALARMになる。
`pool_alerts_enabled = false`の間は、EC2の台のアラームだけ通知しない。
planでLightsailの台、キュー、bridgeが置換されないことと、作成されるのがEC2の台とその周辺、変更されるのがbridgeの権限と環境変数とアラームであることを確かめる。
`infra/deploy-access`のDenyも、管理者の権限で適用する。

```sh
terraform -chdir=infra/judge plan -out="$PWD/judge/.build/pool.tfplan"
terraform -chdir=infra/judge apply "$PWD/judge/.build/pool.tfplan"
terraform -chdir=infra/judge output -json pool_hosts
```

作成直後は3台とも起動している。
ワーカーはまだ導入されておらず、提出を受け取らない。

### 2. IPv6専用の構成で動くことを確かめる

primaryで、SSMの接続、IPv6のIMDSとインスタンスロール、時刻同期、APTを確かめる。
SSMがOnlineになれば、エージェントはIMDSからインスタンスロールの認証情報を取得できている。
`JUDGE_PRIMARY`には`pool_hosts`の`judge-dev-judge-primary`のIDを入れる。

```sh
aws ssm describe-instance-information --filters Key=tag:JudgePool,Values=judge-dev \
  --query 'InstanceInformationList[].[InstanceId,PingStatus]'
cat > judge/.build/pool-check.json <<'JSON'
{"commands": [
  "token=$(curl -sf -X PUT 'http://[fd00:ec2::254]/latest/api/token' -H 'X-aws-ec2-metadata-token-ttl-seconds: 60')",
  "curl -sf -H \"X-aws-ec2-metadata-token: $token\" 'http://[fd00:ec2::254]/latest/meta-data/iam/security-credentials/'",
  "chronyc tracking || timedatectl timesync-status",
  "apt-get update >/dev/null && echo apt ok"
]}
JSON
aws ssm send-command --instance-ids "$JUDGE_PRIMARY" --document-name AWS-RunShellScript \
  --parameters file://judge/.build/pool-check.json
```

出力にインスタンスロール名（`judge-dev-judge-pool`）が含まれていれば、IPv6のIMDSに届いている。

SSMがOnlineにならない場合や認証情報を取得できない場合は、デュアルスタックへの切り替えを検討する。
サブネットにIPv4のCIDRとインターネットゲートウェイを追加し、台にパブリックIPv4を付ける変更で、起動中の台1台あたり月約3.7 USD増える。
SSMエージェントのログにec2messagesへの接続失敗がないことも確かめる。
ec2messagesのデュアルスタックの接続先は、2026年9月時点でIPv6のアドレスを公開していない。

### 3. 監視と配布物を導入し、実機で検証する

各台にCloudWatchエージェントを`ec2`モードで導入する。
設定は`terraform -chdir=infra/judge output -json cloudwatch_agent_configs`の台の名前に対応する値を使う。

```sh
sudo bash install-observability.sh amazon-cloudwatch-agent.deb 記録したSHA256 agent.json ec2
```

続いて、[定型コマンド](deployment-checks.md)の「配布の開始と回収」と「全ホストの実機検証」に従い、3台へ配布してsmokeを通す。
この段階では`verify.py start`を実行しない。
EC2の台はカーネルがLightsailと異なるため、新しいruntime digestになる。
検証後、各台で`df -h /`と`du -sh /opt/judge-runtimes`を記録する。
2026年9月時点のランタイムは展開後に約20 GBある。
更新時は、配布の前に退避ツリーを削除しても、現行ツリー、アーカイブ、展開中のツリーが同時に置かれて約50 GBになる。
`worker_volume_size`の既定値60 GBは、この値を元にしている。

代表的な提出をLightsailとEC2で繰り返し実行し、CPU時間の中央値とばらつきを比べる。
この比較のための専用ツールはまだない。
差が大きい場合は、既存の問題の制限時間を見直してから切り替える。

### 4. 受付を止めて切り替える

Lightsailのプロセス監視アラーム（`judge-dev-judge-worker`、`judge-dev-judge-worker-2`）は、退役後に鳴り続ける。
切り替えの前にアクションを止めておく。

```sh
aws cloudwatch disable-alarm-actions --alarm-names judge-dev-judge-worker judge-dev-judge-worker-2
```

`rollout.json`の`nodes`をEC2の3台、`retire_nodes`をLightsailの2台にして、段階を分けて実行する。
`prepare`は受付を止めて排出し、EC2とLightsailの両方のworkerを停止する。
手順3で`verify.py submit`に使った実行ディレクトリを`JUDGE_VERIFY_RUN`とし、`verify.py start`を実行してから`finish`で公開する。
`worker_alarms`にはEC2の3台のアラームだけを指定する。

```sh
python3 judge/rollout.py prepare --config judge/.build/rollout.json --run-dir "$JUDGE_RELEASE_RUN"
python3 judge/verify.py start --run-dir "$JUDGE_VERIFY_RUN"
python3 judge/verify.py collect --run-dir "$JUDGE_VERIFY_RUN" --wait
python3 judge/rollout.py finish --config judge/.build/rollout.json --run-dir "$JUDGE_RELEASE_RUN" \
  --report "$JUDGE_VERIFY_RUN/report.json"
```

`finish`はLightsailのworkerが停止して無効になっていることを確かめてから、配信を再開する。
完了条件は`state.json`が`status: passed`かつ`step: complete`になることである。

### 5. 台数制御を有効にする

`capacity_enabled = true`と`pool_alerts_enabled = true`にしてapplyする。
試験のため、一時的に`burst_min_participants = 1`にし、運営者のアカウントで参加登録した試験コンテストを35分後に開始する設定で作る。
開始30分前にbridgeのログに`capacity_start`が出てburstが起動すること、対象期間の後に10分で`idle_stop`が出て停止することを確かめる。
確認後に`burst_min_participants`を10へ戻す。

### 6. 安定期間の後に片付ける

切り替え後にコンテストを1回以上運用して問題がなければ、次を行う。

- `worker_count = 0`をapplyし、Lightsailの台とそのアラームを削除する。
- LightsailのworkerのIAMユーザーのアクセスキーを無効化してから削除し、`mi-`形式のSSM管理ノードの登録を解除する。
- EC2 Instance Savings Plans（t3a、東京、1年、前払いなし、1時間あたり0.0154 USD）を購入する。unlimitedモードの追加料金は割引の対象外で、`CPUSurplusCreditsCharged`で確認できる。

安定期間中にLightsailへ戻す場合は、台数制御を無効にし、`nodes`と`retire_nodes`を入れ替えて配布する。
旧digestの配布物は、切り替え前にローカルへ保存しておく。
配布が成功すると、S3上の古い配布物は削除されるためである。

## 監視

プロセス監視アラームは台ごとに作られ、名前は台の名前と同じである。
primaryは欠測を異常として扱う。
burstは停止中に何も送らないのが正常なので、欠測を正常として扱う。
起動中にワーカーが止まった場合は、エージェントがプロセス数0を送るため検知できる。

台数制御とburstの停止に関する障害は`category: platform`として記録され、`platform`アラームで通知される。

| `reason` | 意味 | 最初に確認する対象 |
| --- | --- | --- |
| `capacity_failed` | bridgeがEC2の操作またはDBの読み取りに失敗した | bridgeのログ、EC2 APIのエラー、容量不足 |
| `capacity_stale_instance` | 起動したい台の導入済みdigestが古い | その台を含めた配布のやり直し |
| `capacity_start_timeout` | 起動を指示した台が5分たっても`running`にならない | EC2コンソールの状態遷移理由 |
| `pool_hold_unavailable` | burstが保持期限のタグを読めない | IMDSの設定、nftables、タグの値 |
| `request_release_failed` | 停止時に処理中の提出をキューへ戻せなかった | SQSへの接続。提出は35分後に再配信される |

## 費用

月額はAWSの公開料金（東京、2026年9月）で約30 USDと見込む。
primaryのSavings Plans約11.2 USD、ディスク（gp3で60 GB × 3台）約17.3 USD、burstの稼働（毎週4時間）約1.9 USDの合計である。
停止中の台もディスク代はかかる。
ディスクを増やす前に、手順3で記録したピーク時の使用量を確認する。
