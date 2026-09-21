# 配布と検証の定型コマンド

APIとfrontendは既存のGitHub Actionsで配布する。
ジャッジの長時間検証はSSM上で動かし、AIに待機や繰り返しのログ確認をさせない。
運用者は配布を開始した後、保存されたJSONと終了コードで合否を確認する。
新しいランタイム、OS更新、ホスト増設でも実機検証を省略しない。

## 通常は統合コマンドを使う

既存の2台へ更新版を配布する場合は`judge/rollout.py run`を使う。
受付停止、DBとキューの排出、配布、全言語検証、起動、設定同期、受付再開、本番APIテストを順に実行する。
合格時だけ`state.json`が`status: passed`、`step: complete`になる。
SSMへの登録成功だけでは配布完了にしない。

実行前に更新したランタイムの配布物をビルドし、AWS CLI、GitHub CLI、Terraformを使える状態にする。
AWSのprofileは明示し、GitHub CLIは対象リポジトリのEnvironment変数を書き込める権限を使う。
Terraformは`infra/api`と`infra/judge`のbackendと入力変数を設定済みであることが前提である。
bridgeは旧workerとも互換な版を用意する。

```sh
export AWS_PROFILE=loop0919
export AWS_DEFAULT_REGION=ap-northeast-1
mkdir -p judge/.build
# ランタイム本体のビルド方法は、変更した言語のビルド手順に従う。
make -C api package-judge
bash judge/build-assets.sh
cp judge/rollout.example.json judge/.build/rollout.json
```

`rollout.json`の配布物パス、対象2台、公開言語一覧を確認する。
言語のIDを更新した場合は`runtimes`も更新し、API確認用の`smoke_runtime`へ公開するCPythonまたはPyPyのIDを指定する。
例のリソース名とSSM IDは2026-09-19時点の本番値であり、ホスト交換後は置き換える。
要求、結果、要求DLQ、結果DLQの順で`queues`へ指定する。
スクリプトはAWSアカウントと、bridge、キュー、dispatch、結果受信の対応も確認する。

```sh
export JUDGE_RELEASE_RUN="judge/.build/rollout-$(date -u +%Y%m%dT%H%M%SZ)"
nohup python3 judge/rollout.py run --config judge/.build/rollout.json \
  --run-dir "$JUDGE_RELEASE_RUN" > "$JUDGE_RELEASE_RUN.log" 2>&1 < /dev/null &
```

実行中はAIによるポーリングを行わず、後で`$JUDGE_RELEASE_RUN/state.json`を確認する。
配布物のSHA-256、停止段階、公開digest、本番APIテストのレポートパスが残る。
詳細は`commands.log`と各段階の`receipt.json`、`status.json`、`report.json`で確認する。
ローカルの待機プロセスが中断されても、同じコマンドと同じ実行ディレクトリで、登録済みSSMコマンドの回収から再開できる。
配布物の内容や設定を変更した場合は、同じ実行として再開することを拒否する。
同じ端末から同じAPIへ同時に2つの更新を実行することも拒否する。
別端末との排他は行わないため、更新作業の運用者は1人に限定する。

### 受付停止と排出の判定

bridgeを先に更新し、IAM認証付きのLambda呼び出し`{"operation":"deployment-status"}`でDBを確認する。
この呼び出しは公開HTTP APIには追加していない。
`pending`には処理中と未dispatchの両方を含み、`undispatched`は未dispatchだけを数える。
廃止した言語IDの古い提出も、`-isolate`の提出として数える。
古いbridgeが返す`null`は空件数として扱わない。

APIの受付停止後は、変更前の設定で開始していたAPI呼び出しが終了するまで待つ。
DBの両件数と、4キューの可視、処理中、遅延件数が3回連続で0になってからdispatchと結果受信を止める。
実行中のdispatchが終了するまで待ち、再度排出を確認した後にworkerを停止する。
1時間で空にならない場合は終了し、受付停止を維持する。
DLQのメッセージを削除して見かけ上空にする処理は行わない。

### 設定の同期と受付再開

合格レポートが対象の全ホストを含むことを確認し、実際に稼働中のプロセスのdigestと起動完了も再確認する。
bridgeのdigest、GitHub Environmentの`JUDGE_RUNTIME_DIGEST`と`JUDGE_ENABLED_RUNTIMES`を更新し、読み戻して一致を確認する。
Terraformの入力は、各rootの`zz-rollout.auto.tfvars.json`へ出力する。
これはGit管理対象外で、既存の`terraform.tfvars`や`runtime.auto.tfvars`より後に読み込まれる。
配布後に手動でdigestを変更する場合も、このファイルの優先順位を考慮する。

dispatchと結果受信を再開し、停止前に有効だった監視通知を戻してからAPIの受付を再開する。
公開言語一覧の一致と、本番APIの4提出、2台の同時採点、後片付けが通過して初めて成功とする。
最後にTerraformを`refresh-only`で同期する。
通常の`terraform apply`で予定外のインフラ変更を適用することはない。

### バックアップと失敗時の操作

実行ディレクトリにAPIとbridgeの変更前コードと設定を保存する。
設定には秘密情報が含まれ得るため、ディレクトリは0700、ファイルは0600とし、Gitや公開成果物へアップロードしない。
worker停止時には`/var/lib/judge-backups/<停止コマンドのrunId>/control.tar.gz`へ制御コード、環境設定、isolateを保存する。
既存インストーラーは、ランタイムアーカイブが変わった場合に旧ツリーを退避する。

この退避はOS全体のバックアップではない。
OSパッケージやカーネルの変更を含む配布では、開始前にLightsailのスナップショットを作成し、`available`を確認する。
通常のインストーラーでもAPTを実行するため、更新内容によってはOSパッケージが変わる。
復旧用スナップショットの作成と保持、ホスト作成やSSM登録、DBスキーマの破壊的変更は、統合コマンドでは実行しない。

失敗時は受付を再度停止し、`state.json`へ`status: failed`と失敗段階を残す。
`admissionPaused: false`の場合は停止操作自体が失敗しているため、AWS接続を復旧して受付状態を確認する。
ローカルの強制終了や端末断では終了処理を保証できないため、その場合も状態を確認する。
旧版への自動ロールバックは行わない。
新digestの提出を受け付けた後に旧workerへ戻すと、その提出を正しく採点できないためである。

- ローカルの待機中断：同じ`run`コマンドで再開する。登録済みSSM処理は再送しない。
- SSM処理そのものの失敗：コマンドIDから原因を確認する。実行中の処理が残っていないことを確認し、失敗した段階のディレクトリを退避してから再実行する。実行中のreceiptは削除しない。
- workerの再停止が必要：同じディレクトリで`prepare`を明示的に実行して排出と停止をやり直す。その後、完了済みの`verify`ディレクトリを退避して、新しい実機検証と起動確認を行う。起動済みホストに対してsmokeを再実行しない。
- 本番APIテストの後片付け失敗：保存されたAPIレポートと同じ引数で`smoke-api.py --cleanup`を実行する。`finish`の再試行で別のテストアカウントを作る前に片付ける。
- 旧版へ戻す：受付停止を維持し、現在のdigestの提出を排出した後、退避したOS、制御コード、ランタイムを復元する。全台の実機検証と起動確認をやり直してから旧digestを公開する。

配布と検証を別々に行う場合は、前半と後半だけを呼び出せる。

```sh
python3 judge/rollout.py prepare --config judge/.build/rollout.json --run-dir "$JUDGE_RELEASE_RUN"
# 配布と verify.py の submit / collect / start を実行する。
python3 judge/rollout.py finish --config judge/.build/rollout.json --run-dir "$JUDGE_RELEASE_RUN" \
  --report "$JUDGE_RELEASE_RUN/verify/report.json"
```

以下の個別コマンドは、この統合処理を分けて実行したい場合や、障害調査時に使用する。

## 配布前の条件

[ランタイムの再構築と公開](runtime-rollout.md)に従い、APIの受付を止めてから、DBの未dispatch提出と処理中の提出、SQSの要求と結果が空になるまで待つ。
空になった後にdispatchと結果受信を止め、全workerを停止する。
SQSが空というだけでは、DBの未dispatch提出がないことを保証できない。
`rollout.py prepare`は受付停止、排出、処理停止、制御コードのバックアップ、bridgeの先行配布を実行する。
SSM登録とインフラの作成、OS全体のバックアップは別途行う。
失敗時に受付を自動再開する処理も置かない。

以下はリポジトリルートで実行する。
AWS CLIのprofileとregionを設定し、SSMノードはTerraformのホスト一覧と照合する。
新しい配布ごとに異なる実行ディレクトリを使う。
既存のディレクトリは上書きしない。

```sh
export AWS_PROFILE=loop0919
export AWS_DEFAULT_REGION=ap-northeast-1
export JUDGE_RELEASE_RUN="judge/.build/rollout-$(date -u +%Y%m%dT%H%M%SZ)"
export JUDGE_NODE_1=mi-08a9ccbdc9116b369
export JUDGE_NODE_2=mi-07cb78f3b979951e7
export JUDGE_RELEASE_BUCKET=judge-dev-judge-5983370c65ca9cd6b9cd685c8d
mkdir -p "$JUDGE_RELEASE_RUN"
```

## 配布の開始と回収

既存の`deploy-ssm.py`で、SHA-256を固定した配布物を送る。
`--no-wait`ならSSMコマンドの登録後に戻る。
停止済みの各ホストに対して実行する。
例では配布物は事前に`judge/.build/worker.tar.gz`へ構築済みとする。

```sh
python3 judge/deploy-ssm.py --instance "$JUDGE_NODE_1" --bucket "$JUDGE_RELEASE_BUCKET" \
  --run-dir "$JUDGE_RELEASE_RUN/install-1" --no-wait
python3 judge/deploy-ssm.py --instance "$JUDGE_NODE_2" --bucket "$JUDGE_RELEASE_BUCKET" \
  --run-dir "$JUDGE_RELEASE_RUN/install-2" --no-wait
```

次の`collect --wait`は通常のPythonプロセスで待機する。
AIのツール呼び出しで待ち続ける必要はない。
端末を閉じる場合は`nohup`やtmuxを使う。
途中でローカルの待機が止まってもSSMの処理は続き、同じディレクトリで結果を再取得できる。

```sh
python3 judge/verify.py collect --run-dir "$JUDGE_RELEASE_RUN/install-1" --wait
python3 judge/verify.py collect --run-dir "$JUDGE_RELEASE_RUN/install-2" --wait
```

## 全ホストの実機検証

両ホストの配布が成功してから実行する。
全ランタイムのsmokeを両ホストで同時に開始する。
通常問題・対話問題の64・315・512 MiB制限について、上限内のACと上限超過のMLEも検証する。
開始だけなら待機は発生しない。

```sh
python3 judge/verify.py submit --run-dir "$JUDGE_RELEASE_RUN/verify" \
  --instance "$JUDGE_NODE_1" --instance "$JUDGE_NODE_2"
nohup python3 judge/verify.py collect --run-dir "$JUDGE_RELEASE_RUN/verify" --wait \
  > "$JUDGE_RELEASE_RUN/verify-wait.log" 2>&1 < /dev/null &
```

初回ディスク読み込みを含め、smokeサービスには90分の待機上限を設ける。
検証は既存の`smoke.sh`を使い、言語の一部だけを選択する環境変数は解除する。
失敗時の詳細ログは、対象ホストの`/var/lib/judge-verification/<runId>/smoke.log`に残る。
`runId`とSSMコマンドIDはローカルの`receipt.json`に記録される。

## 起動と公開

全ホストで同じdigestと同じランタイム一覧が合格した場合だけ起動できる。
起動後は、そのsystemd起動回に属する`worker_started`を待つ。
単にプロセスが存在するだけでは合格にしない。
dispatchと結果受信は、この確認が終わるまで停止したままにする。

```sh
python3 judge/verify.py start --run-dir "$JUDGE_RELEASE_RUN/verify"
nohup python3 judge/verify.py collect --run-dir "$JUDGE_RELEASE_RUN/verify" --wait \
  > "$JUDGE_RELEASE_RUN/start-wait.log" 2>&1 < /dev/null &
```

`collect`が成功すると、`report.json`の`ready`が`true`になる。
そのレポートを既存の`admission.py`へ渡す。
複数ホストを含むレポートは、全ホストの起動完了を確認できなければ公開を拒否する。
公開対象の言語一覧は承認済みの一覧を指定し、smokeに通った全言語を自動公開しない。

公開前にbridgeのdigestをレポートと一致させ、dispatchと結果受信を再開する。
公開後はGitHub Environmentの`JUDGE_RUNTIME_DIGEST`、`JUDGE_ENABLED_RUNTIMES`とTerraform設定を同期する。
これらの設定変更は`rollout.py finish`で実行できる。

```sh
python3 judge/admission.py --function judge-dev-api \
  --smoke-report "$JUDGE_RELEASE_RUN/verify/report.json" \
  --runtimes "$JUDGE_APPROVED_RUNTIMES"
```

## 本番APIでの確認

Python 3.14の公開後、次のコマンドで一時的な非公開問題を作成して確認する。
PythonのIDが変わった場合は`--runtime`で指定する。
Cognitoの招待メールは送信しない。
4件の提出で、非連続の累計2回TLE、TLEが1回の場合、サンプル検証を確認する。
一時問題のメモリ制限は315 MiBとし、512 MiB以外でもAPIから採点完了まで通ることを確認する。
2台のIDを渡した場合は、両ホストのログに採点区間の重なりがあることも必須とする。
キューの分配によって同時採点を観測できなかった場合も失敗として返すため、結果を確認して新しいレポート名で再実行する。

```sh
python3 judge/smoke-api.py --function judge-dev-api --api-url https://api.share-oj.net \
  --instance "$JUDGE_NODE_1" --instance "$JUDGE_NODE_2" \
  --report "$JUDGE_RELEASE_RUN/api-report.json"
```

成功時は一時問題と一時アカウントを削除してから`status: passed`を保存する。
プロフィールと提出の監査記録は残る。
後片付けに失敗した場合やプロセスが中断された場合は、同じ引数に`--cleanup`を追加する。
対象を`api-report.cleanup.json`に保存するため、AIがユーザー名をログから探し直す必要はない。
このファイルにパスワードやトークンは保存しない。

## 終了コードと記録

| コマンド | 0 | 1以上 |
| --- | --- | --- |
| `deploy-ssm.py --no-wait` / `verify.py submit` / `verify.py start` | SSM登録済み（処理完了ではない） | 登録失敗 |
| `verify.py collect` | 対象ホストすべて成功 | 3は処理中、1は失敗 |
| `smoke-api.py` | 判定確認と後片付けが成功 | 検証または後片付けに失敗 |

`status.json`は直近の回収結果であり、バックグラウンドの回収プロセスを起動していなければ自動更新されない。
SSMがまだ処理中なら、同じ`collect`で再開する。
SSM自体が失敗した場合は原因を解消して新しい実行ディレクトリで検証し直す。
SSM登録直後からIDの保存までの間にプロセスが中断された場合、`collect`は不足IDを検出して停止する。
その場合はSSMのコメントにあるrunIdから登録済みコマンドを確認し、二重に配布を開始しない。

配布記録にはコミット、配布物SHA-256、`receipt.json`、`report.json`、API検証結果を残す。
通常は結果JSONだけを確認し、失敗時に限ってAIへ該当ログを渡す。

## 自動テスト

```sh
python3 -m unittest discover -s judge/tests -v
```

既存CIがこのコマンドを実行するため、追加のCI設定は不要である。
AWS呼び出しを模擬し、ホスト欠落、digest不一致、古いrunId、実機テスト失敗、起動前の公開拒否、APIテストと後片付けを検証する。
実機の23ランタイムテストと本番APIテストは運用時に実行し、単体テストで代替しない。
