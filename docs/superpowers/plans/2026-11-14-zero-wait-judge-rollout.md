# 提出も採点も止めないジャッジ更新 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `judge/rollout.py rolling`を、送り出しを止めずに要求キューのレーンを切り替える手順へ変え、切り替えを戻す`rolling-revert`を足す。

**Architecture:** 要求キューを固定の2レーン（a、b）にする。新しい環境のワーカーを空きレーンで待機させ、bridgeの環境変数（送り先、digest、直前のdigest）を1回の更新で切り替える。旧ワーカーは旧レーンの残りを終えてから止める。

**Tech Stack:** Terraform（`infra/judge`）、Go（`api/cmd/judge-bridge`）、Python 3標準ライブラリ（`judge/rollout.py`、`judge/verify.py`、`judge/smoke-api.py`）、AWS CLI。

**Spec:** [docs/superpowers/specs/2026-11-14-zero-wait-judge-rollout-design.md](../specs/2026-11-14-zero-wait-judge-rollout-design.md)

## Global Constraints

- ワーカーのコード（`judge/runner/`）は変えない。ワーカーの配布物も変わらない。
- 既存の要求キュー`${local.name}-judge-requests`、結果キュー、DLQを作り直さない。採点ホストも作り直さない。
- レーンaは既存の`requests`、レーンbは`requests-b`。DLQと結果キューは共有する。
- 使用中のレーンの正は、稼働中のbridgeの`JUDGE_REQUEST_QUEUE_URL`である。レーンを変えるのは配布ツールだけ。
- `run`、`prepare`、`finish`の手順は変えない。レーンも切り替えない。
- `rolling`は`JUDGE_DISPATCH_PAUSED`を使わない。bridge側の実装は残す。
- 本番のAWSへの操作（`terraform apply`、`rollout.py`の実行）は、この計画では行わない。利用者が別に指示する。
- コミットメッセージは`prefix: 対応内容`の形式で日本語。タスクごとにコミットする。
- コードのコメントは英語、文書は日本語（既存にならう）。
- コードを変えたタスクの最後に`graphify update .`を実行する。
- テストの実行：
  - Go：`cd api && go test ./cmd/judge-bridge/`（DBを使うテストは`TEST_DATABASE_URL`が必要。`nix develop`の中でローカルDBのURLを指定する）
  - Python：`python3 -m unittest discover -s judge/tests -v`
  - Terraform：`terraform -chdir=infra/judge test`（`nix develop`の中で実行する。PATHの古いTerraformは使わない）

## Review Focus

1. 切り替えの後、`sync_settings`の前にローカルの実行が中断された。再開時の`terraform plan`が、古い`request_lane`のせいで失敗してはならない（Task 5の`sync_lane`）。
2. 空きレーンに前回の実行の残りがある。`rolling`は切り替え前に中断し、利用者に影響を出さない（Task 6）。
3. 旧レーンの提出がDLQに落ち、キューは空だが`previousInFlight`が残る。送り直してから旧ワーカーを止める（Task 6）。
4. `requeue-digest`に現在のdigest、空文字、不正な形式が渡された。1件も変えずに拒む（Task 3）。
5. `rolling-revert`が、切り替え前、または旧ワーカー停止後に呼ばれた。何も変えずに拒む（Task 7）。

---

### Task 1: Terraformでレーンbと`request_lane`を足す

**Files:**
- Modify: `infra/judge/storage.tf`（`aws_sqs_queue.queue`、`aws_sqs_queue_redrive_allow_policy.dead`）
- Modify: `infra/judge/bridge.tf`（IAM、`JUDGE_REQUEST_QUEUE_URL`、precondition）
- Modify: `infra/judge/worker.tf:6`、`infra/judge/ec2.tf:17`、`infra/judge/outputs.tf:20,26`
- Modify: `infra/judge/variables.tf`、`infra/judge/terraform.tfvars.example`
- Test: `infra/judge/tests/judge.tftest.hcl`

**Interfaces:**
- Produces: 変数`request_lane`（`"a"`または`"b"`、既定値なし）。出力`request_queue_url`（レーンa）、`request_queue_b_url`（レーンb）。`local.request_queue_urls = { a = ..., b = ... }`。

- [ ] **Step 1: 失敗するテストを書く**

`judge.tftest.hcl`の`variables`に`request_lane = "a"`を足し、次の`run`を加える。

- `request_lanes`：`aws_sqs_queue.queue["requests"].name == "judge-dev-judge-requests"`（既存の名前が変わらない。`local.name`はテストの既存の値に合わせる）、`aws_sqs_queue.queue["requests-b"].name`が`-judge-requests-b`で終わる、`requests-b`の`visibility_timeout_seconds == 2100`。
- `lane_b_selects_queue`：`variables { request_lane = "b" }`で、bridgeの`JUDGE_REQUEST_QUEUE_URL`が`aws_sqs_queue.queue["requests-b"].url`に等しい。モックのURLは全キュー同じなので、`override_resource`で`requests-b`のURLを別の値にする。
- `invalid_lane`：`request_lane = "c"`で`expect_failures = [var.request_lane]`。
- `stale_lane_fails`：`override_data`で稼働中のbridgeの`JUDGE_REQUEST_QUEUE_URL`をレーンbのURLにし、`request_lane = "a"`で`expect_failures = [aws_lambda_function.bridge]`。

- [ ] **Step 2: 失敗を確かめる**

Run: `terraform -chdir=infra/judge test`
Expected: FAIL（`request_lane`が未定義）

- [ ] **Step 3: 実装する**

- `aws_sqs_queue.queue`の`for_each`を`toset(["requests", "requests-b", "results"])`にする。`visibility_timeout_seconds`は`startswith(each.key, "requests") ? 2100 : 720`。DLQは`startswith(each.key, "requests") ? "requests" : "results"`を引く。
- `aws_sqs_queue_redrive_allow_policy.dead`の`requests`側の`sourceQueueArns`に両レーンを入れる。
- bridgeのIAMの`sqs:SendMessage`と、ワーカーのIAMの受信3操作を、両レーンのARNにする。
- `data "aws_lambda_function" "bridge_live"`で稼働中のbridgeを読む。`aws_lambda_function.bridge`の`lifecycle`に次のpreconditionを足す。

  ```hcl
  precondition {
    condition     = try(data.aws_lambda_function.bridge_live.environment[0].variables.JUDGE_REQUEST_QUEUE_URL, local.request_queue_urls[var.request_lane]) == local.request_queue_urls[var.request_lane]
    error_message = "request_lane differs from the live bridge. Only judge/rollout.py switches lanes; set request_lane to the live lane."
  }
  ```

  `try`の代替値は、bridgeがまだない最初の構築のためである。データソースが関数の不在でplan自体を失敗させる場合は、`count = var.enabled ? 1 : 0`のように既存の構築手順と両立する形にし、選んだ形をコメントに書く。
- `ec2.tf`と`outputs.tf`の`JUDGE_REQUEST_QUEUE_URL`を`local.request_queue_urls[var.request_lane]`にする。`ec2.tf:142`の`ignore_changes = [ami, user_data]`があるので、既存ホストは作り直されない。
- `terraform.tfvars.example`に`request_lane = "a"`を足す。

- [ ] **Step 4: 合格を確かめる**

Run: `terraform -chdir=infra/judge fmt -check && terraform -chdir=infra/judge test`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add infra/judge
git commit -m "feat: 要求キューを2レーンにし、使用中のレーンをrequest_laneで選ぶ"
```

---

### Task 2: bridgeの`deployment-status`に送り先と旧digestの残りを足す

**Files:**
- Modify: `api/cmd/judge-bridge/bridge.go`（`deploymentStatus`）
- Test: `api/cmd/judge-bridge/main_test.go`

**Interfaces:**
- Produces: `deployment-status`の応答に`requestQueueUrl`（文字列、`b.queueURL`）と`previousInFlight`（整数）。
  `previousInFlight`は、`dispatched_at IS NOT NULL AND status <> 'DONE'`かつdigestが`b.previous`である`submissions`（`job->>'image'`）と`code_runs`（`image`）の合計。`b.previous == ""`なら0。ランタイムの絞り込みは`pending`と同じ`submissions.IsolateRuntimeIDs()`。

- [ ] **Step 1: 失敗するテストを書く**

`TestRollingUpdateHoldsAndRebindsUndispatchedSubmissions`（`main_test.go:524`）と同じ準備で、`TestDeploymentStatusReportsLaneAndPreviousInFlight`を書く。

- `bridge{queueURL: "https://sqs.test/requests-b", runtime: newDigest, previous: oldDigest}`。
- 旧digestで送り出し済み未完了の提出1件とコード実行1件、旧digestで未送り出しの提出1件、旧digestで`DONE`の提出1件、新digestで送り出し済み未完了の提出1件を用意する。
- `got["requestQueueUrl"] == "https://sqs.test/requests-b"`、`got["previousInFlight"] == float64(2)`。
- `previous: ""`のbridgeでは`previousInFlight == float64(0)`。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd api && go test ./cmd/judge-bridge/ -run TestDeploymentStatusReportsLaneAndPreviousInFlight`
Expected: FAIL（項目がない）

- [ ] **Step 3: `deploymentStatus`に2項目を足す**

既存の1本のクエリにスカラー副問い合わせを足す。

- [ ] **Step 4: 合格を確かめる**

Run: `cd api && go test ./cmd/judge-bridge/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/cmd/judge-bridge
git commit -m "feat: bridgeの配布状態に送り先のキューと旧digestの残り件数を足す"
```

---

### Task 3: bridgeに`requeue-digest`を足す

**Files:**
- Create: `api/cmd/judge-bridge/requeue.go`
- Modify: `api/cmd/judge-bridge/bridge.go`（`handle`の`Operation`の分岐）
- Test: `api/cmd/judge-bridge/requeue_test.go`

**Interfaces:**
- Produces: Lambdaの呼び出し`{"operation":"requeue-digest","digest":"sha256:…"}`。応答は`{"kind":"judge-requeue","requeued":N}`。
  `func (b bridge) requeueDigest(ctx context.Context, digest string) (any, error)`。
- 対象：`dispatched_at IS NOT NULL AND status <> 'DONE'`で、digestが引数に等しい`submissions`と`code_runs`。
- 更新：`dispatched_at=NULL`、`judge_attempt=gen_random_uuid()`、`status='QUEUED'`、`progress=NULL`、`started_at=NULL`。`rejudged_at`と`created_at`は変えない（6時間の期限と送り出しの順序を保つ）。`code_runs`は持っている列だけ同じように戻す。
- 拒否：`digest`が`^sha256:[a-f0-9]{64}$`に合わない、または`digest != b.previous`ならエラーを返し、1件も変えない。エラー文は`"invalid operation"`。

- [ ] **Step 1: 失敗するテストを書く**

`requeue_test.go`に次を書く（DBの準備は`migratedDB`と既存のテストの補助関数を使う）。

- `TestRequeueDigestReturnsOnlyThePreviousDigestInFlight`：旧digestで送り出し済みの提出とコード実行は`dispatched_at IS NULL`になり、`judge_attempt`が変わる。応答は`requeued == 2`。新digestの送り出し済み、旧digestの`DONE`、旧digestの未送り出しは変わらない。
- `TestRequeuedSubmissionIsRebound`：戻した後に`b.dispatch(ctx)`を呼ぶと、送ったジョブの`runtimeDigest`が現在のdigestになる。
- `TestRequeueDigestDropsStaleResults`：戻す前の試行IDの結果を`b.results`に渡しても、提出は`DONE`にならない。
- `TestRequeueDigestRejectsOtherDigests`：現在のdigest、`""`、`"sha256:zz"`、`previous`が空のbridgeで、エラーになり行が変わらない。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd api && go test ./cmd/judge-bridge/ -run TestRequeue`
Expected: FAIL（`requeueDigest`が未定義）

- [ ] **Step 3: 実装する**

`handle`の`event`に`Digest string \`json:"digest"\``を足す。`Operation`の分岐を`deployment-status`と`requeue-digest`の2つにし、それ以外と`Records`付きは今までどおり`invalid operation`。2つの`UPDATE`は1つのトランザクションで行う。件数を`observe("requeued", …, "count", n)`で記録する。

- [ ] **Step 4: 合格を確かめる**

Run: `cd api && go test ./cmd/judge-bridge/`
Expected: PASS（`TestInvalidDeploymentOperationCannotDispatch`も通る）

- [ ] **Step 5: コミット**

```bash
git add api/cmd/judge-bridge
git commit -m "feat: 送り出し済みの提出を直前のdigestから送り直すbridge操作を足す"
```

---

### Task 4: `verify.py start`でワーカーのレーンを指定する

**Files:**
- Modify: `judge/verify.py`（`start_command`、`main`）
- Test: `judge/tests/test_verify.py`（なければ作る）

**Interfaces:**
- Produces: `start_command(run_id, digest, request_queue=None) -> str`。CLIは`verify.py start --run-dir DIR [--request-queue URL]`。
  `request_queue`があれば、`worker.env`の`JUDGE_REQUEST_QUEUE_URL=`の行を、digestの行と同じ書き込みで置き換える。その行がちょうど1行でなければ`SystemExit('expected exactly one request queue setting')`。
  URLは`^https://sqs\.[a-z0-9-]+\.amazonaws\.com/[0-9]{12}/[A-Za-z0-9_-]+$`に合わなければ、SSMへ送る前に`ValueError`。
  receiptに`requestQueue`を保存し、`collect`からの再開で同じ値を使う。

- [ ] **Step 1: 失敗するテストを書く**

- `test_start_command_rewrites_the_request_queue`：`start_command('r', digest, url)`の文字列に`JUDGE_REQUEST_QUEUE_URL=`の書き換えとURLが含まれる。`start_command('r', digest)`には含まれない（既存の呼び出しの出力が変わらない）。
- `test_start_command_rejects_other_urls`：`'https://example.com/x'`と、シェルの特殊文字を含む値で`ValueError`。

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s judge/tests -p 'test_verify.py' -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

- [ ] **Step 4: 合格を確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add judge/verify.py judge/tests/test_verify.py
git commit -m "feat: ワーカーの起動時に要求キューのレーンを指定できるようにする"
```

---

### Task 5: 配布ツールにレーンの設定と事前確認を足す

**Files:**
- Modify: `judge/rollout.py`（`preflight`、`wait_empty`、`contest_guard`、`sync_settings`）
- Modify: `judge/rollout.example.json`
- Test: `judge/tests/test_rollout.py`

**Interfaces:**
- Produces:
  - 設定`request_queue_b`（レーンbのURL、任意）。`queues[0]`はレーンa。
  - `lanes(config) -> dict`：稼働中のbridgeの`JUDGE_REQUEST_QUEUE_URL`から`dict(active='a'|'b', active_url=..., standby_url=...)`を返す。`request_queue_b`がない設定では`dict(active='a', active_url=queues[0], standby_url=None)`。bridgeのURLがどちらのレーンでもなければ`ValueError('bridge request queue is not a configured lane')`。
  - `all_queues(config) -> list`：`queues`に`request_queue_b`（あれば）を足した一覧。
  - `queue_counts(config, url) -> int`：可視、処理中、遅延の合計。
  - `sync_lane(config, lane)`：`infra/judge/zz-rollout.auto.tfvars.json`の`request_lane`だけを`lane`にする。ほかの値は保つ。
  - `sync_settings(..., lane=None)`：`lane`があれば`infra/judge`の値に`request_lane`を含める。

- [ ] **Step 1: 失敗するテストを書く**

- `test_lanes_follow_the_live_bridge`：bridgeのURLがレーンbなら`active == 'b'`、`standby_url == queues[0]`。レーンaなら逆。知らないURLで`ValueError`。
- `test_lanes_without_a_second_queue`：`request_queue_b`なしで`standby_url is None`。
- `test_wait_empty_watches_both_lanes`：レーンbに1件あるあいだ`wait_empty`が戻らない（既存の`wait_empty`のテスト`test_rollout.py:69`付近にならう）。
- `test_preflight_syncs_the_lane_before_terraform`：bridgeがレーンbを指し、tfvarsが`request_lane: "a"`のとき、`preflight`は`terraform plan`の前にtfvarsを`"b"`へ書き換える（Review Focus 1）。
- `test_contest_guard_requires_the_lane_fields`：`deployment-status`に`requestQueueUrl`か`previousInFlight`がなければ`ValueError`。

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s judge/tests -p 'test_rollout.py' -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

- `preflight`：`len(set(config['queues'])) != 4`の検査は残す。`request_queue_b`があれば、4つと重ならないこと、bridgeのURLが`lanes`で解決できること、レーンbの`RedrivePolicy`が`queues[2]`を指すことを確かめる。Terraformを呼ぶ前に`sync_lane(config, lanes(config)['active'])`を呼ぶ。`request_queue_b`がなければ`sync_lane`は呼ばない。
- 既存の`bridge[...]['JUDGE_REQUEST_QUEUE_URL'] != config['queues'][0]`の検査は、`lanes`の検査に置き換える。
- `wait_empty`は`all_queues(config)`を回す。
- `rollout.example.json`に`"request_queue_b": "https://sqs.ap-northeast-1.amazonaws.com/123456789012/judge-dev-judge-requests-b"`を足す。

- [ ] **Step 4: 合格を確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add judge/rollout.py judge/rollout.example.json judge/tests/test_rollout.py
git commit -m "feat: 配布ツールが要求キューの2レーンを確かめ、Terraformの入力をそろえる"
```

---

### Task 6: `rolling`の切り替えをレーンの切り替えにする

**Files:**
- Modify: `judge/rollout.py`（`switch`、`rolling`、`start_workers`、`wait_requests_idle`の置き換え）
- Test: `judge/tests/test_rollout.py`（`RollingTests`）

**Interfaces:**
- Consumes: `lanes`、`queue_counts`、`sync_settings(..., lane=)`（Task 5）。`verify.py start --request-queue`（Task 4）。`deployment-status`の`requestQueueUrl`、`previousInFlight`、操作`requeue-digest`（Task 2、3）。
- Produces:
  - `start_workers(verification, request_queue=None)`。
  - `requeue(config, digest) -> int`：bridgeの`requeue-digest`を呼び、件数を返す。`FunctionError`なら`RuntimeError`。
  - `drain_previous(config, directory, state, url, timeout=2700)`：旧レーンの排出。
  - `state.json`の項目：`previousLane`、`activeLane`、`previousQueue`、`activeQueue`（切り替え前に確定して保存）、`digestSwitched`、`apiSwitched`、`requeued`（件数）、`oldWorkersStopped`。
  - チェックポイントの名前（順に）：`hold-bursts`、`prepare-first`、`standby-first`、`switch-lane`、`switch-api`、`sync-settings`、`api-smoke-first`、`drain-previous`、`stop-old-workers`、`prepare-rest`、`start-rest`、`api-smoke`、`refresh-state`、`complete`。

`switch`の新しい順序（設計書の手順3〜6）：

1. `standby-first`：`request_queue_b`がなければ`ValueError('a rolling update needs request_queue_b')`。`queue_counts(standby_url) != 0`なら`ValueError('the standby lane is not empty')`。`start_workers(verification, request_queue=standby_url)`。`verify.py start`はワーカーの`failure`イベントがないことと`worker_started`を確かめる。その後35秒待ち（受信の待ち20秒を1回は超える）、`healthy_workers`と同じ形のSSMで`systemctl is-active`と、起動後のログに`queue_receive_failed`がないことを確かめる。
2. `switch-lane`：`contest_guard`。`update_env(bridge, dict(JUDGE_REQUEST_QUEUE_URL=standby_url, JUDGE_RUNTIME_DIGEST=digest, JUDGE_PREVIOUS_RUNTIME_DIGEST=previous))`を1回。`deployment-status`の`runtimeDigest`、`previousRuntimeDigest`、`requestQueueUrl`を確かめ、`digestSwitched=True`を保存する。
3. `switch-api`：`time.sleep(bridgeのTimeout + 5)`、`update_env(api, dict(JUDGE_CPP_IMAGE=digest))`、`tag_digest(config, [first], digest)`、`apiSwitched=True`。
4. `sync-settings`：`sync_settings(..., previous=previous, lane=新しいレーン)`。`api-smoke-first`：`api_smoke(config, directory, [first])`。
5. `drain-previous`：`drain_previous`。10秒おきに`queue_counts(旧レーン)`と`previousInFlight`を見る。両方0が3回連続で終わり。キューが3回連続で0なのに`previousInFlight > 0`なら、`requeue(config, previous)`を1回だけ呼び、件数を`state['requeued']`に足して、また両方0を待つ。`timeout`で`TimeoutError('the previous lane did not drain; old workers keep running')`。
6. `stop-old-workers`：`ssm_step(directory, 'switch-stop', config, rest, stop_command)`、`oldWorkersStopped=True`。

`rolling`の残りの台は`start_workers(verification, request_queue=state['activeQueue'])`で起動する。`set_dispatch`の呼び出しと`dispatchPaused`の確認は`rolling`から消す。関数`set_dispatch`は残す（運用者の手動操作用）。使われなくなった`wait_requests_idle`は消す。

- [ ] **Step 1: 失敗するテストを書く**

`RollingTests.run_rolling`のモックを新しい関数に合わせ、`test_rolling_prepares_a_burst_then_switches_with_dispatch_paused`を次のテストに置き換える。

- `test_rolling_switches_lanes_without_pausing_dispatch`：イベントの順が「`prepare first` → 待機の起動（空きレーンのURL付き）→ `contest` → bridgeの`env`（3つの変数を1回で）→ `sleep` → APIの`env` → `sync`（`lane`付き）→ `api`（firstのみ）→ `drain` → `switch-stop` → `prepare rest` → 残りの起動（新しい使用中のレーンのURL付き）」である。`('dispatch', True)`が1つもない。APIの`JUDGE_ENABLED_RUNTIMES`を`none`にする更新がない。
- `test_rolling_refuses_a_busy_standby_lane`：空きレーンに1件あると`ValueError`。bridgeとAPIの`env`の更新が1つもない（Review Focus 2）。
- `test_rolling_requires_the_second_lane`：`request_queue_b`なしで`ValueError`。`env`の更新がない。
- `test_drain_requeues_stranded_submissions`：キューが0で`previousInFlight`が2のまま3回続くと`requeue`を1回呼び、`state['requeued'] == 2`。その後0になれば`switch-stop`へ進む（Review Focus 3）。
- `test_drain_timeout_keeps_old_workers`：`previousInFlight`が減らないと`TimeoutError`。`switch-stop`がない。同じ`state`で再開すると、bridgeの`env`を更新し直さずに`drain`から続く。
- `test_resume_after_switch_skips_the_switch`：`digestSwitched=True, apiSwitched=False`の`state`から再開すると、bridgeの`env`は更新せず、APIの`env`から続く。

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s judge/tests -p 'test_rollout.py' -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

- [ ] **Step 4: 合格を確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add judge/rollout.py judge/tests/test_rollout.py
git commit -m "feat: rollingの切り替えを、送り出しを止めないレーンの切り替えにする"
```

---

### Task 7: `rolling-revert`を足す

**Files:**
- Modify: `judge/rollout.py`（`revert`、`main`の`choices`と分岐）
- Test: `judge/tests/test_rollout.py`

**Interfaces:**
- Consumes: Task 6の`state.json`の項目、`requeue`、`queue_counts`、`sync_settings`。
- Produces: `python3 judge/rollout.py rolling-revert --config … --run-dir …`。`revert(config, directory, state)`。終了時の`state`は`status='reverted'`、`step='reverted'`。
  `rolling`は`status == 'reverted'`の実行ディレクトリを`ValueError('this run was reverted; start a new run')`で拒む。

`revert`の順序（設計書「切り替えを戻す」）：

1. `state.get('digestSwitched')`が真でない、または`state.get('oldWorkersStopped')`が真なら`ValueError('revert is only possible between the lane switch and the old worker stop')`。何も変えない。
2. `update_env(bridge, dict(JUDGE_REQUEST_QUEUE_URL=state['previousQueue'], JUDGE_RUNTIME_DIGEST=previous, JUDGE_PREVIOUS_RUNTIME_DIGEST=digest))`を1回。`deployment-status`で確かめる。
3. `time.sleep(bridgeのTimeout + 5)`、`update_env(api, dict(JUDGE_CPP_IMAGE=previous))`。
4. `ssm_step(directory, 'revert-stop', config, [first], stop_command)`。firstの`JudgeInstalledDigest`タグを消す（bridgeが古い環境として起動しないようにする。タグがなければ`planCapacity`は起動しない）。
5. `aws sqs purge-queue`で`state['activeQueue']`（新レーン）を空にする。
6. `requeue(config, digest)`。件数を`state['requeued']`に足す。
7. `sync_settings(config, directory, previous, config['runtimes'], lane=state['previousLane'])`。`previous_runtime_digest`には新digestを書く。
8. 無効にしていたアラームを`state['alarm_actions']`のとおりに戻す。`checkpoint(..., 'reverted', status='reverted')`。

各段階は`state`に記録し、同じコマンドで途中から再開できるようにする。

- [ ] **Step 1: 失敗するテストを書く**

- `test_revert_restores_the_previous_lane_and_digest`：`digestSwitched=True`の`state`で、イベントの順が「bridgeの`env`（旧レーン、旧digest、直前=新digest）→ `sleep` → APIの`env`（旧digest）→ `revert-stop`（firstのみ）→ `purge-queue`（新レーン）→ `requeue`（新digest）→ `sync`（旧レーン）」。`state['status'] == 'reverted'`。
- `test_revert_refused_outside_its_window`：`state={}`と、`digestSwitched=True, oldWorkersStopped=True`で`ValueError`。AWSの呼び出しが1つもない（Review Focus 5）。
- `test_rolling_refuses_a_reverted_run`：`status='reverted'`で`rolling`が`ValueError`。

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s judge/tests -p 'test_rollout.py' -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

- [ ] **Step 4: 合格を確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add judge/rollout.py judge/tests/test_rollout.py
git commit -m "feat: レーンの切り替えを旧環境へ戻すrolling-revertを足す"
```

---

### Task 8: 切り替えの前後の採点待ちを計測する

**Files:**
- Modify: `judge/smoke-api.py`（`--watch`）
- Modify: `judge/rollout.py`（`switch`から計測を起動、終了、判定）
- Test: `judge/tests/test_smoke_api.py`（なければ作る）、`judge/tests/test_rollout.py`

**Interfaces:**
- Produces:
  - `smoke-api.py --watch --stop-file PATH --interval 5 --report PATH`（ほかの引数は既存と同じ）。`--stop-file`のファイルができるまで、`--interval`秒おきに`--runtime`で1件提出し、各提出の結果を待つ。終了時に後片付け（既存の`cleanup`）をして、レポートを書く。
  - レポート：`dict(status='passed'|'failed', submissions=[dict(id, submittedAt, startedAt, finishedAt, waitMs, verdict)], rejected=N, je=N, maxWaitMs=N, baselineWaitMs=N)`。`waitMs`は提出から採点開始まで。時刻はAPIの提出の`createdAt`と`startedAt`を使う。
  - `summarize(samples, switch_at) -> dict`：純粋な関数。`baselineWaitMs`は`switch_at`より前の提出の`waitMs`の中央値、`maxWaitMs`は`switch_at`以降の最大。
  - 合否：`rejected == 0 and je == 0 and maxWaitMs - baselineWaitMs <= 10000`で`passed`。
  - `rollout.py`：`standby-first`の後で計測を子プロセスとして起動し、6件（30秒）たまってから`switch-lane`へ進む。`switch-api`の後60秒で`--stop-file`を作り、終了を待つ。レポートは実行ディレクトリの`switch-watch.json`。`state['switchWatch']`にパスと`maxWaitMs`を残す。
  - 計測が`failed`でも、切り替えは済んでいるので実行は続け、`state['switchWatchFailed']=True`を残す。残りの台の更新、アラームの復帰、Terraformの同期、保持期限の短縮まで終えてから、`complete`へ進まずに`ValueError('switch latency check failed; see switch-watch.json')`で終わる（配布は済んでいるが、合格として報告しない）。再開しても同じ場所で同じエラーになる。

- [ ] **Step 1: 失敗するテストを書く**

- `test_summarize_compares_against_the_baseline`：切り替え前の`waitMs`が`[900, 1100, 1000]`、後が`[1200, 9000]`で`baselineWaitMs == 1000`、`maxWaitMs == 9000`、`passed`。後が`[12000]`なら`failed`。
- `test_summarize_fails_on_rejection_or_je`：`rejected=1`、または後の提出に`verdict='JE'`があれば`failed`。
- `test_summarize_needs_samples_on_both_sides`：切り替え前か後が0件なら`failed`。
- `test_rolling_watches_the_switch`（`test_rollout.py`）：計測の起動が`switch-lane`より前、停止が`switch-api`より後。計測が`failed`を返すと、残りの台の起動とアラームの復帰は行われるが、`rolling`は`complete`に達せず、`state['switchWatchFailed']`が真。

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

`smoke-api.py`は名前にハイフンがあるので、テストは`importlib`で読み込む（既存のテストに同じ読み込みがあればそれを使う）。

- [ ] **Step 4: 合格を確かめる**

Run: `python3 -m unittest discover -s judge/tests -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add judge/smoke-api.py judge/rollout.py judge/tests
git commit -m "feat: レーンの切り替えの前後で採点待ちを計測し、10秒を超えたら不合格にする"
```

---

### Task 9: ADRと手順書を更新する

**Files:**
- Create: `docs/adr/0039-switch-judge-environments-by-request-lanes.md`（0038は`feat/contest-webhooks`が使っている。作成時に`docs/adr/`と各ブランチを見て、空いている次の番号にする）
- Modify: `docs/adr/README.md`、`docs/adr/0014-roll-out-os-updates-without-stopping-judging.md`（冒頭に「一部変更：ADR 0039」を足すだけ。本文は書き換えない）
- Modify: `docs/judge/deployment-checks.md`、`docs/judge/ec2-pool.md`（配布の設定）、`AGENTS.md`（ジャッジの配布と検証）
- Modify: `docs/superpowers/specs/2026-11-14-zero-wait-judge-rollout-design.md`（状態を「実装済み、未配布」にする）

**Interfaces:**
- Consumes: Task 1〜8で決まった名前（`request_lane`、`request_queue_b`、`rolling-revert`、`switch-watch.json`、チェックポイントの名前）。

- [ ] **Step 1: ADR 0039を書く**

既存のADRの見出し（状態、決定日、一部変更、範囲、背景、決定、採用しなかった方法、影響）にならう。内容は設計書から取る。次を必ず含める。

- ADR 0014で不採用とした「digestごとにキューを分ける」を、固定の2レーンとして採用に変えた理由。
- `rolling`を使える配布の3条件と、DBのスキーマの扱い、`run`を使う場合。
- ワーカーの配布物を作るコミットと、新機能を有効にするコミットをpushする順序。運用者が守る決まりであること。
- 要求のDLQからレーンへ再送しないこと。
- 採用しなかった方法：切り替え前に試験用の提出を新レーンへ流す（bridgeに別の送り出し経路が要る）、切り替え後の自動ロールバック。

- [ ] **Step 2: 手順書を更新する**

- `deployment-checks.md`：`rolling`の説明を新しい順序にし、「切り替えを戻す」の節（使える段階、コマンド、戻した後の状態）を足す。失敗時の表を設計書のとおりに足す。導入時の1回だけの手順（`request_lane = "a"`でのapply、`rollout.json`への`request_queue_b`の追加）を書く。
- `AGENTS.md`：`rolling`の行を「送り出しも止めずに配布する」に直し、`rolling-revert`と、形式の変わる配布も3条件を満たせば`rolling`を使うことを足す。
- 文書は短く、箇条書きと表を優先する。

- [ ] **Step 3: リンクと番号を確かめる**

Run: `grep -rn "0039" docs AGENTS.md | head` と、足したリンク先のファイルがあることの確認。
Expected: ADRの番号が1つのファイルだけを指し、リンク切れがない。

- [ ] **Step 4: コミット**

```bash
git add docs AGENTS.md
git commit -m "docs: 要求キューのレーンで採点環境を切り替えるADRと手順を加える"
graphify update .
```

---

## 計画の外（利用者の指示で行う）

1. `infra/judge`を`request_lane = "a"`でapplyする。planで、既存のキューと採点ホストが作り直されないことを確かめる。
2. `judge/.build/rollout.json`に`request_queue_b`を足す。
3. 次のOS更新かワーカーの更新を新しい`rolling`で行い、`state.json`の`status: passed`、`step: complete`と、`switch-watch.json`の`maxWaitMs`を確かめる。
