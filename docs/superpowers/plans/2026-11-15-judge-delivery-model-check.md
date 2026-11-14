# ジャッジの配送と切り替えのモデル検査 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 採点の配送と2レーン切り替えをTLA+のモデルに書き、提出が失われない・止まらないという性質をTLCで検査して、結果を利用者に報告する。

**Architecture:** 仕様は`judge/formal/JudgeDelivery.tla`の1モジュールで、機能の有無（切り替え、戻し、6時間切れ、わざと崩す箇所）を定数で切り替える。検査ごとに`.cfg`を1つ置き、`checks.json`に「通るはず」か「この性質が破れるはず」かを書く。Pythonの`run.py`が全検査を回し、期待と違えば失敗で終わる。

**Tech Stack:** TLA+とTLC（nixpkgsの`tlaplus` 1.7.4）、Python 3標準ライブラリ、GNU Make。

**Spec:** [docs/superpowers/specs/2026-11-15-judge-delivery-model-check-design.md](../specs/2026-11-15-judge-delivery-model-check-design.md)

モデルに写す切り替え手順は[2レーン切り替えの設計書](../specs/2026-11-14-zero-wait-judge-rollout-design.md)にある。実装者は両方を読む。

## Global Constraints

- 製品コードを変えない。`api/`、`judge/runner/`、`judge/rollout.py`、`judge/verify.py`、`infra/`には触れない。変えてよいのは`judge/formal/`、`judge/tests/test_formal_run.py`、`flake.nix`、`flake.lock`、`Makefile`、`.gitignore`、`docs/judge/`だけである。
- モデルは、設計書といまの実装に書いてあるとおりに書く。承認されていない直し方を先にモデルへ入れない。
- TLCが見つけた破れは、モデルの誤り、2レーン切り替えの設計の穴、いまの実装の不具合のどれかに分ける。あとの2つなら、そこで作業を止めて利用者に報告する（反例を日本語の手順に直し、該当する設計書の節かコードの場所を添える）。
- 2レーン切り替えの実装計画（`2026-11-14-zero-wait-judge-rollout.md`）は、この計画のTask 6が終わるまで実行しない。
- CIのワークフローにTLCの実行を足さない。
- `make judge-model`は10分以内に終わる。超えたら、検査を分けるか規模を下げる。
- 検査の規模の初期値：提出2件、リジャッジは1件につき1回、故障は合計2回、DLQの受信上限2、ワーカーは旧1台と新1台。
- 空きがあるときだけ送るものの上限は2（`spareInFlight`）。
- TLA+のコメントは英語、文書は日本語。
- コミットメッセージは`prefix: 対応内容`の形式で日本語。タスクごとにコミットする。
- タスクの最後に`graphify update .`を実行する。
- テストの実行：
  - ランナー：`python3 -m unittest discover -s judge/tests -p 'test_formal_run.py' -v`
  - モデル：`nix develop . --command python3 judge/formal/run.py --only <名前>`

## Review Focus

1. 故障の動作が一度も起きないモデル。検査は通るが何も確かめていない。到達できるはずの状態（両方のDLQ、付け替え、送り直し、再開、戻し）に本当に到達することを検査する（Task 2とTask 4の`Reach*.cfg`）。
2. 公平性の書き方のせいで、いつか成り立つ性質が空で通る。ワーカーが1台も動かない設定で`EventuallyJudged`が破れることを確かめる（Task 2の`BrokenNoWorker.cfg`）。
3. `.cfg`の書き間違いや構文エラーでTLCが止まり、「破れるはず」の検査が成功と数えられる。ランナーは、名前の付いた性質の破れ以外をすべて失敗にする（Task 1）。
4. 状態数が増えてTLCが終わらない。ランナーは検査ごとに600秒で打ち切り、失敗にする（Task 1）。
5. 切り替えツールが、手順の効果を出したあと記録する前に止まる。再開で同じ手順をもう一度実行しても性質が保たれることを検査する（Task 4）。

---

### Task 1: TLCを回すランナーと骨組み

**Files:**
- Create: `judge/formal/run.py`, `judge/formal/checks.json`, `judge/formal/JudgeDelivery.tla`, `judge/formal/Smoke.cfg`, `judge/formal/SmokeBroken.cfg`
- Create: `judge/tests/test_formal_run.py`, `judge/tests/fixtures/tlc/`（TLCの実際の出力）
- Create: `docs/judge/formal-model.md`
- Modify: `flake.nix`（開発シェルの`packages`に`tlaplus`）, `Makefile`, `.gitignore`, `docs/judge/README.md`

**Interfaces:**
- Produces:
  - `checks.json`：`[{"name": str, "config": str, "violates": [str], "continue": bool, "large": bool, "note": str}]`。`violates`が空なら「通るはず」。`continue`と`large`と`note`は省略できる。
  - `run.classify(output: str, returncode: int) -> tuple[str, frozenset[str]]`：`("pass", frozenset())`、`("violates", 破れた性質の名前)`、`("error", frozenset())`のどれか。
  - `run.run_check(check: dict, command=tlc_command, timeout=600) -> tuple[bool, str]`：期待どおりなら`True`。2つ目は1行の説明。
  - `run.select(checks: list[dict], only: list[str], large: bool) -> list[dict]`：回す検査を選ぶ。`only`が空なら全部。`large`の検査は`large`が真のときだけ残す。
  - `run.main(argv) -> int`：`--only NAME`（複数可）と`--large`。`large`の検査は`--large`のときだけ回す。全検査が期待どおりなら0。
  - `make judge-model`：`nix develop . --command python3 judge/formal/run.py`

- [ ] **Step 1: `flake.nix`に`tlaplus`を足し、コマンドを確かめる**

Run: `nix develop . --command sh -c 'ls $(dirname $(command -v tlc))'`
Expected: `tlc`がある。名前が違えば、`run.py`の`tlc_command`をそれに合わせる。

- [ ] **Step 2: 骨組みの仕様と、TLCの実際の出力を取る**

`JudgeDelivery.tla`に、変数1つ（0から2まで増えるカウンタ）と不変条件`SmokeBound`（カウンタ ≤ 2）、`SmokeBroken`（カウンタ ≤ 1）、`SmokeBroken2`（カウンタ ≤ 0）、時相の性質`SmokeLive`（いつか3になる。破れる）を書く。
次の5通りでTLCを回し、標準出力を`judge/tests/fixtures/tlc/`へ`pass.txt`、`invariant.txt`、`temporal.txt`、`parse-error.txt`（`.cfg`に存在しない不変条件の名前を書く）、`two-invariants.txt`（`-continue`を付け、破れる不変条件を2つ指定する）として保存する。

TLCの呼び出しは`tlc -workers auto -deadlock -noGenerateSpecTE -metadir <一時ディレクトリ> -config <cfg> JudgeDelivery.tla`を出発点にし、`judge/formal/`に作業ファイルが残らないことを`git status`で確かめる。残るものは`.gitignore`に足す。
時相の性質が破れたときの出力に性質の名前が出ない場合は、時相の性質を1つの`.cfg`に1つだけ書く決まりにし、`classify`はその`.cfg`の`PROPERTY`の名前を返す（`run_check`から渡す）。この決まりは`formal-model.md`に書く。

- [ ] **Step 3: 失敗するテストを書く**

`judge/tests/test_formal_run.py`（読み込み方は`test_rollout.py`にならい、`judge/formal`を`sys.path`に足す）。

```python
def test_classify_reads_real_tlc_output(self):
    self.assertEqual(run.classify(fixture('pass.txt'), 0), ('pass', frozenset()))
    self.assertEqual(run.classify(fixture('invariant.txt'), 12), ('violates', frozenset({'SmokeBroken'})))
    self.assertEqual(run.classify(fixture('two-invariants.txt'), 12)[1], frozenset({'SmokeBroken', 'SmokeBroken2'}))
    self.assertEqual(run.classify(fixture('parse-error.txt'), 1)[0], 'error')
    self.assertEqual(run.classify('', 0)[0], 'error')

def test_expected_violation_must_match_exactly(self):
    fake = lambda check, metadir: ['cat', str(FIXTURES / 'invariant.txt')]
    self.assertTrue(run.run_check(dict(name='x', config='x.cfg', violates=['SmokeBroken']), command=fake)[0])
    self.assertFalse(run.run_check(dict(name='x', config='x.cfg', violates=['Other']), command=fake)[0])
    self.assertFalse(run.run_check(dict(name='x', config='x.cfg', violates=[]), command=fake)[0])

def test_tlc_error_never_counts_as_expected_violation(self):
    fake = lambda check, metadir: ['cat', str(FIXTURES / 'parse-error.txt')]
    self.assertFalse(run.run_check(dict(name='x', config='x.cfg', violates=['SmokeBroken']), command=fake)[0])

def test_timeout_fails(self):
    fake = lambda check, metadir: ['sleep', '5']
    ok, message = run.run_check(dict(name='x', config='x.cfg', violates=[]), command=fake, timeout=0.2)
    self.assertFalse(ok)
    self.assertIn('timeout', message)

def test_large_checks_run_only_when_asked(self):
    checks = [dict(name='small', config='s.cfg', violates=[]), dict(name='big', config='b.cfg', violates=[], large=True)]
    self.assertEqual([c['name'] for c in run.select(checks, only=[], large=False)], ['small'])
    self.assertEqual([c['name'] for c in run.select(checks, only=[], large=True)], ['small', 'big'])
    self.assertEqual([c['name'] for c in run.select(checks, only=['big'], large=True)], ['big'])
```

終了コードの数値は、Step 2で実際に見た値に合わせる。`cat`の終了コードは0なので、`classify`は終了コードだけで「通った」と判断しない。

- [ ] **Step 4: テストが失敗することを確かめる**

Run: `python3 -m unittest discover -s judge/tests -p 'test_formal_run.py' -v`
Expected: FAIL（`run`が読み込めない）

- [ ] **Step 5: `run.py`を実装する**

`classify`は「エラーなしで完了した」という行があり、破れの行がないときだけ`pass`にする。破れの行から性質の名前を集める。どちらでもなければ`error`。
`run_check`は一時ディレクトリを`metadir`に渡し、`continue`が真なら`-continue`を足す。1検査につき、名前、結果、秒数を1行で出す。

- [ ] **Step 6: テストが通ることを確かめる**

Run: `python3 -m unittest discover -s judge/tests -p 'test_formal_run.py' -v`
Expected: PASS

- [ ] **Step 7: 骨組みを`make`で回す**

`checks.json`に`smoke`（`Smoke.cfg`、`violates: []`）と`smoke-broken`（`SmokeBroken.cfg`、`violates: ["SmokeBroken"]`）を書く。

Run: `make judge-model`
Expected: 2行とも期待どおりで、終了コード0。`git status`に作業ファイルが出ない。

- [ ] **Step 8: `docs/judge/formal-model.md`を作り、`docs/judge/README.md`から張る**

この時点では「何のためのものか」「回し方」「時相の性質は1つの`.cfg`に1つ」の3節だけを書く。あとのタスクが節を足す。

- [ ] **Step 9: コミット**

```bash
git add judge/formal judge/tests/test_formal_run.py judge/tests/fixtures/tlc flake.nix flake.lock Makefile .gitignore docs/judge
git commit -m "feat: ジャッジのモデル検査を回すランナーを加える"
```

---

### Task 2: 配送のモデル

骨組みの中身を、切り替えのない配送のモデルに置き換える。レーンは1本、Bridgeの設定は変わらない。

**Files:**
- Modify: `judge/formal/JudgeDelivery.tla`, `judge/formal/checks.json`, `docs/judge/formal-model.md`
- Create: `judge/formal/Delivery.cfg`, `DeliveryLive.cfg`, `ReachDelivery.cfg`, `BrokenAttempt.cfg`, `BrokenNoWorker.cfg`
- Delete: `Smoke.cfg`, `SmokeBroken.cfg`

**Interfaces:**
- Produces（Task 3以降が使う名前）：
  - 定数：`Subs`（提出の集合）、`SpareSubs`（そのうち空きがあるときだけ送るもの。このタスクでは空集合）、`Workers`、`Runs`（同時に動くBridgeの実行。2つ）、`MaxAttempt`、`MaxFaults`、`MaxReceive`、`SpareLimit`、`Expiry`（6時間切れを起こすか）、`CheckAttempt`（結果の反映で試行IDを照合するか）
  - 判定の値：`"judged"`（本来の判定）、`"je_mismatch"`（Bridgeの`runtime_mismatch`）、`"je_expired"`（6時間切れ）
  - 動作：`Submit`、`Rejudge`、`BridgeStart`、`BridgePick`、`BridgeSend`、`BridgeMark`、`BridgeEnd`、`BridgeFail`、`Receive`、`SendResult`、`DeleteRequest`、`WorkerCrash`、`WorkerTerm`、`VisibilityTimeout`、`DuplicateRequest`、`ApplyResult`、`ApplyFail`、`Expire`
  - 不変条件：`TypeOK`、`ResultMatchesAttempt`（性質2）、`NothingLost`（性質3）、`JudgesOwnDigest`（性質5）
  - 動作の性質：`ResultNotOverwritten`（性質1）
  - 時相の性質：`EventuallyJudged`（性質9）
  - 到達の確認用（破れるはずの不変条件）：`NeverRequestDLQ`、`NeverResultDLQ`、`NeverDuplicateResult`、`NeverStaleResult`（古い試行の結果が結果キューにある）、`NeverResend`（送ったあと送り出し済みにする前に失敗し、同じ試行をもう一度送った）

- [ ] **Step 1: 検査を先に`checks.json`へ書く**

| name | config | violates | 設定の要点 |
|---|---|---|---|
| `delivery` | `Delivery.cfg` | `[]` | `Expiry = TRUE`。不変条件4つと`ResultNotOverwritten` |
| `delivery-live` | `DeliveryLive.cfg` | `[]` | `Expiry = FALSE`。`EventuallyJudged`だけ |
| `reach-delivery` | `ReachDelivery.cfg` | 到達の確認用5つすべて | `continue: true` |
| `broken-attempt` | `BrokenAttempt.cfg` | `["ResultMatchesAttempt"]` | `CheckAttempt = FALSE` |
| `broken-no-worker` | `BrokenNoWorker.cfg` | `["EventuallyJudged"]` | `Workers = {}` |

どの設定も、Global Constraintsの規模の初期値を使う。

- [ ] **Step 2: 失敗することを確かめる**

Run: `nix develop . --command python3 judge/formal/run.py`
Expected: 5つとも`error`（定数も性質もまだない）

- [ ] **Step 3: モデルを書く**

設計書の「モデルにするもの」の状態、動作、故障を写す。次は設計書に書いていない、この計画の決定である。

- 提出の状態は実装の列に合わせる：`done`、`attempt`、`dispatched`、`digest`、`result`（判定と、確定させた試行ID）。
- Bridgeの実行は`BridgeStart`で設定を写し取り、`BridgeEnd`まで写し取った値で動く。`BridgePick`は行をつかみ（ほかの実行は同じ行を選べない。`FOR UPDATE SKIP LOCKED`）、`BridgeFail`はつかんだ行を放す。`BridgeSend`のあとの`BridgeFail`ではメッセージが残り、提出は未送出のままになる。
- `BridgePick`のdigestの扱いは`sendSubmission`と同じにする：提出のdigestが実行のdigestと違うとき、実行の「直前のdigest」と同じなら実行のdigestへ付け替え、そうでなければ`"je_mismatch"`で確定する。
- `Rejudge`は`done`の提出だけに使え、`attempt`を1増やし、`dispatched`と`result`を消し、digestをAPIの設定の値にする。
- ワーカーは`SendResult`と`DeleteRequest`の間で止まれる。`WorkerTerm`は、結果を送る前ならメッセージをすぐ見える状態に戻し、送ったあとなら戻さない（`worker.py`の`settled`）。
- 受信回数は`Receive`で増える。`VisibilityTimeout`で戻るとき、受信回数が`MaxReceive`に達していればDLQへ移る。結果キューも同じで、`ApplyFail`が戻し、上限でDLQへ移る。
- `ApplyResult`は、提出が`done`でなく、`CheckAttempt`が偽か試行IDが一致するときだけ確定する。それ以外は結果を捨てる。
- 故障（`WorkerCrash`、`WorkerTerm`、`BridgeFail`、`DuplicateRequest`、`ApplyFail`）は、合計`MaxFaults`回まで。
- `Expire`は`Expiry`が真のときだけ、`done`でない提出をいつでも`"je_expired"`で確定できる。
- 公平性は、故障と`Submit`と`Rejudge`と`Expire`以外の動作に弱い公平性を置く。置いた仮定は`formal-model.md`に書く。
- `EventuallyJudged`：どの提出も、`done`でなければ、いつか「`"judged"`で`done`」になるか、「いまの試行のメッセージと結果が、要求のDLQか結果のDLQにしか残っていない」状態になる。
- `NothingLost`：`done`でない提出は、未送出か、いまの試行のメッセージか結果が、レーン、ワーカーの処理中、結果キュー、要求のDLQ、結果のDLQのどこかにある。

- [ ] **Step 4: 5つの検査が期待どおりになることを確かめる**

Run: `nix develop . --command python3 judge/formal/run.py`
Expected: 5行とも期待どおりで、終了コード0。

期待と違う結果は、Global Constraintsの3種類に分ける。モデルの誤りなら直してこのステップを繰り返す。設計か実装の問題なら、ここで止めて利用者に報告する。

- [ ] **Step 5: `formal-model.md`に対応表と結果を足す**

- モデルの動作と実装の場所の対応表（`dispatch.go`、`results.go`、`finish.go`、`rejudge.go`、`worker.py`、`infra/judge/storage.tf`）。
- 確かめている性質と、確かめていないこと（設計書の「やらないこと」）。
- 公平性の仮定。
- 検査した日、規模、検査ごとの状態数と秒数。

- [ ] **Step 6: コミット**

```bash
git add judge/formal docs/judge/formal-model.md
git commit -m "feat: 採点の配送をモデル検査する"
```

---

### Task 3: 空きの制限

設計書は、DLQに落ちた提出が2件あると、空きがあるときだけ送るものが送られなくなると予想している。モデルで反例を出して確かめる。

**Files:**
- Modify: `judge/formal/JudgeDelivery.tla`, `judge/formal/checks.json`, `docs/judge/formal-model.md`
- Create: `judge/formal/SpareGate.cfg`

**Interfaces:**
- Consumes: Task 2の定数`SpareSubs`、`SpareLimit`、動作`BridgePick`、性質`EventuallyJudged`。
- Produces: `BridgePick`が、`SpareSubs`の提出を「送り出し済みで`done`でない提出が`SpareLimit`件未満」のときだけ選ぶ。新しい提出が残っている間は`SpareSubs`の提出を選ばない。

- [ ] **Step 1: 検査を`checks.json`へ書く**

`spare-gate`：`SpareGate.cfg`、`violates: ["EventuallyJudged"]`、`note: "利用者の判断待ち"`。
設定は、提出3件（うち1件が`SpareSubs`）、`SpareLimit = 2`、`MaxReceive = 1`、`MaxFaults = 2`、`Expiry = FALSE`、リジャッジなし（`MaxAttempt = 1`）。

- [ ] **Step 2: 制限がない状態で、検査が期待と違うことを確かめる**

Run: `nix develop . --command python3 judge/formal/run.py --only spare-gate`
Expected: 失敗（`EventuallyJudged`は破れず、通ってしまう）

- [ ] **Step 3: `BridgePick`に制限を足す**

- [ ] **Step 4: 全検査を回す**

Run: `nix develop . --command python3 judge/formal/run.py`
Expected: `spare-gate`が`EventuallyJudged`の破れで期待どおり。Task 2の5つは変わらず期待どおり（`SpareSubs`が空なので影響しない）。

`spare-gate`が通ってしまう場合は、設計書の予想が外れている。モデルが`dispatchOne`の件数の数え方（DLQに落ちた提出も数える）と合っているかを確かめ、合っていれば「予想は外れた」と記録して`violates`を空にする。

- [ ] **Step 5: 反例を日本語の手順に直し、`formal-model.md`の「見つかったこと」に書く**

`tlc`を`-noGenerateSpecTE`なしで`SpareGate.cfg`に対して1回だけ回し、反例の状態の列を読む。
書く内容：起きる順番、該当するコード（`api/cmd/judge-bridge/dispatch.go`の`spareInFlight`の判定）、利用者から見える影響（コード実行、作問の実行、リジャッジが6時間切れまで送られない）。
結果のDLQについても同じ節に書く：結果の反映に`MaxReceive`回失敗した提出は、6時間切れまで採点待ちで残る（`reach-delivery`の`NeverResultDLQ`の反例から手順を起こす）。

- [ ] **Step 6: コミットし、利用者に報告して止まる**

```bash
git add judge/formal docs/judge/formal-model.md
git commit -m "feat: 空きがあるときだけ送る制限をモデル検査する"
```

報告する内容：2つの経路の手順、該当するコードの場所、直すかどうかの判断の依頼。
利用者が「直さない」と決めたら`note`を「既知。直さない」に変える。「直す」と決めたら、その修正は別の作業として扱い、この計画はTask 4へ進む。どちらでも、Task 4以降はこの判断に依存しない。

---

### Task 4: 2レーン切り替えのモデル

**Files:**
- Modify: `judge/formal/JudgeDelivery.tla`, `judge/formal/checks.json`, `docs/judge/formal-model.md`
- Create: `judge/formal/Switch.cfg`, `SwitchLiveJudged.cfg`, `SwitchLiveDrain.cfg`, `ReachSwitch.cfg`, `BrokenBridgeWait.cfg`, `BrokenSingleLane.cfg`

**Interfaces:**
- Consumes: Task 2のすべて。
- Produces:
  - 定数：`Switch`（切り替えを起こすか。Task 2と3の設定では`FALSE`）、`TwoLanes`、`WaitForBridge`
  - 値：レーン`"a"`と`"b"`、digest`"d0"`（旧）と`"d1"`（新）、ワーカー`w_old`と`w_new`
  - 変数：`phase`（切り替えの位置：`"idle"`、`"standby"`、`"switched"`、`"api"`、`"drained"`、`"done"`）、Bridgeの設定（`lane`、`digest`、`previous`）、APIのdigest、ワーカーごとの`lane`と`digest`と状態
  - 動作：`StartStandby`（手順3）、`SwitchBridge`（手順4）、`SwitchAPI`（手順5）、`RequeuePrevious`（手順6の`requeue-digest`）、`StopOld`（手順6）、`StartRest`（手順7）、`Checkpoint`（`phase`を進める）
  - 不変条件：`OneDigestPerLane`（性質4）、`GuardNeverFires`（性質6）、`NoMismatchJE`（性質7）、`DestinationHasWorker`（性質8）
  - 時相の性質：`OldLaneDrains`（性質10）
  - 到達の確認用：`NeverRebound`（付け替えが起きた）、`NeverRequeued`（`requeue-digest`が提出を戻した）、`NeverStepRepeated`（手順の効果を2回出した）、`NeverSwitchDone`（`phase = "done"`）、`NeverJudgedOnOldAfterSwitch`（切り替え後に旧ワーカーが採点を終えた）

- [ ] **Step 1: 検査を`checks.json`へ書く**

| name | config | violates | 設定の要点 |
|---|---|---|---|
| `switch` | `Switch.cfg` | `[]` | `Switch = TRUE`、`TwoLanes = TRUE`、`WaitForBridge = TRUE`、`Expiry = TRUE`。Task 2の不変条件と、このタスクの不変条件4つ、`ResultNotOverwritten` |
| `switch-live-judged` | `SwitchLiveJudged.cfg` | `[]` | `Expiry = FALSE`。`EventuallyJudged` |
| `switch-live-drain` | `SwitchLiveDrain.cfg` | `[]` | `Expiry = FALSE`。`OldLaneDrains` |
| `reach-switch` | `ReachSwitch.cfg` | 到達の確認用5つすべて | `continue: true` |
| `broken-bridge-wait` | `BrokenBridgeWait.cfg` | `["NoMismatchJE"]` | `WaitForBridge = FALSE` |
| `broken-single-lane` | `BrokenSingleLane.cfg` | `["GuardNeverFires"]` | `TwoLanes = FALSE`。旧ワーカーを止めずに切り替える |

- [ ] **Step 2: 失敗することを確かめる**

Run: `nix develop . --command python3 judge/formal/run.py --only switch --only reach-switch`
Expected: どちらも`error`

- [ ] **Step 3: 切り替えを書く**

2レーン切り替えの設計書の「新しい`rolling`の手順」3〜7を写す。次はこの計画の決定である。

- 初期状態：Bridgeは`lane = "a"`、`digest = "d0"`、`previous`なし。APIは`"d0"`。`w_old`は`"a"`で動作中、`w_new`は停止中。`phase = "idle"`。
- 各手順は「効果」と「記録」（`Checkpoint`）の2つの動作に分ける。効果は、`phase`がその手順の手前にある間、何度でも起こせる。これで、効果のあと記録の前にツールが止まり、再開で同じ手順をもう一度実行する場合を表す。
- `StartStandby`：`w_new`を`digest = "d1"`、`lane = "b"`で起動する。前提は、レーンbが空であること（手順1の事前確認）。
- `SwitchBridge`：`lane`、`digest`、`previous`を1つの動作で`"b"`、`"d1"`、`"d0"`にする。すでに動いているBridgeの実行は、写し取った古い値のまま続く。
- `SwitchAPI`：APIのdigestを`"d1"`にする。`WaitForBridge`が真なら、古い設定を写し取ったBridgeの実行が1つも残っていないことを前提にする（Bridgeのタイムアウト＋5秒の待ち）。
- `RequeuePrevious`：前提は、旧レーンにメッセージも処理中もないこと。digestが`previous`で、送り出し済みで`done`でない提出を、`attempt`を1増やして未送出に戻す。
- `StopOld`：前提は、旧レーンが空で、digestが`previous`の送り出し済みで`done`でない提出が0件であること。`w_old`を止める。
- `StartRest`：`w_old`を`digest = "d1"`、`lane`をBridgeの`lane`にして起動する。
- ワーカーがdigestの違うジョブを受け取ったら、メッセージを戻して停止状態になる（`worker.py`の`runtime_mismatch`）。`GuardNeverFires`は、この停止が起きないことである。
- `TwoLanes`が偽のときは、`w_new`もレーンaで起動し、`SwitchBridge`は`lane`を変えない。
- `DestinationHasWorker`：Bridgeの実行がメッセージを送るレーンには、そのメッセージのdigestと同じdigestのワーカーが割り当てられていて、手順によって止められていない。故障（`WorkerCrash`）で止まっている場合は破れとしない。
- `OldLaneDrains`：`phase = "done"`になったあと、いつか、使っていないレーンにメッセージがなく、digestが`"d0"`の送り出し済みで`done`でない提出もない状態になり、その状態が続く。
- 公平性は、切り替えの動作にも弱い公平性を置く。

- [ ] **Step 4: 全検査を回す**

Run: `nix develop . --command python3 judge/formal/run.py`
Expected: 全行が期待どおりで、終了コード0。合計が10分を超える場合は、Global Constraintsに従って分けるか規模を下げ、下げた値を`formal-model.md`に書く。

期待と違う結果は3種類に分ける。2レーン切り替えの設計の穴なら、反例を日本語の手順に直し、設計書のどの節を直せばよいかの案を付けて、ここで止めて利用者に報告する。設計書と実装計画を直すのは、利用者が決めたあとである。

- [ ] **Step 5: `formal-model.md`に対応表（切り替えの手順と設計書の節）と結果を足す**

- [ ] **Step 6: コミット**

```bash
git add judge/formal docs/judge/formal-model.md
git commit -m "feat: 2レーン切り替えをモデル検査する"
```

---

### Task 5: 切り替えを戻す手順

**Files:**
- Modify: `judge/formal/JudgeDelivery.tla`, `judge/formal/checks.json`, `docs/judge/formal-model.md`
- Create: `judge/formal/Revert.cfg`, `RevertLiveJudged.cfg`, `RevertLiveDrain.cfg`, `ReachRevert.cfg`

**Interfaces:**
- Consumes: Task 4のすべて。
- Produces:
  - 定数：`Revert`（戻しを起こすか。これまでの設定では`FALSE`）
  - `phase`の値：`"rv-bridge"`、`"rv-api"`、`"rv-stopped"`、`"reverted"`
  - 動作：`RevertBridge`、`RevertAPI`、`StopNew`（新ワーカーを止め、新レーンを空にする）、`RequeueNew`（digestが`"d1"`の提出を戻す）
  - 到達の確認用：`NeverReverted`（`phase = "reverted"`）、`NeverRequeuedNew`、`NeverPurged`（`StopNew`がメッセージを消した）

- [ ] **Step 1: 検査を`checks.json`へ書く**

| name | config | violates |
|---|---|---|
| `revert` | `Revert.cfg` | `[]`（`switch`と同じ不変条件と`ResultNotOverwritten`） |
| `revert-live-judged` | `RevertLiveJudged.cfg` | `[]`（`EventuallyJudged`） |
| `revert-live-drain` | `RevertLiveDrain.cfg` | `[]`（`OldLaneDrains`） |
| `reach-revert` | `ReachRevert.cfg` | 到達の確認用3つすべて（`continue: true`） |

- [ ] **Step 2: 失敗することを確かめる**

Run: `nix develop . --command python3 judge/formal/run.py --only revert --only reach-revert`
Expected: どちらも`error`

- [ ] **Step 3: 戻しを書く**

2レーン切り替えの設計書の「切り替えを戻す」の5手順を写す。

- 始められるのは、`phase`が`"switched"`か`"api"`の間（`SwitchBridge`の後、`StopOld`の前）だけである。始めたら、切り替えの動作（`SwitchAPI`以降）は起こせない。
- `RevertBridge`：`lane`、`digest`、`previous`を`"a"`、`"d0"`、`"d1"`にする。
- `RevertAPI`：`SwitchAPI`と同じ前提（古い設定のBridgeの実行が残っていない）で、APIのdigestを`"d0"`にする。
- `StopNew`：`w_new`を止め、レーンbのメッセージをすべて消す。
- `RequeueNew`：digestが`"d1"`で、送り出し済みで`done`でない提出を、`attempt`を1増やして未送出に戻す。
- `OldLaneDrains`の「使っていないレーン」と「直前のdigest」は、`phase = "reverted"`ではレーンbと`"d1"`を指す。Task 4の定義を、Bridgeの`lane`と`previous`から決める形に直す。
- 各手順はTask 4と同じく、効果と記録に分ける。

- [ ] **Step 4: 全検査を回す**

Run: `nix develop . --command python3 judge/formal/run.py`
Expected: 全行が期待どおりで、終了コード0、合計10分以内。

期待と違う結果の扱いは、Task 4のStep 4と同じである。

- [ ] **Step 5: `formal-model.md`に結果を足し、コミット**

```bash
git add judge/formal docs/judge/formal-model.md
git commit -m "feat: 切り替えを戻す手順をモデル検査する"
```

---

### Task 6: 規模を上げた検査と報告

**Files:**
- Modify: `judge/formal/checks.json`, `docs/judge/formal-model.md`
- Create: `judge/formal/DeliveryLarge.cfg`, `SwitchLarge.cfg`, `RevertLarge.cfg`

**Interfaces:**
- Consumes: Task 1の`large`と`--large`、Task 2〜5の性質。

- [ ] **Step 1: 規模を上げた設定を足す**

提出3件、`MaxReceive = 3`、ほかは初期値。不変条件と`ResultNotOverwritten`だけを検査する（時相の性質は入れない）。`checks.json`では`large: true`、`violates: []`。

- [ ] **Step 2: 一度だけ回す**

Run: `nix develop . --command python3 judge/formal/run.py --large --only delivery-large --only switch-large --only revert-large`
Expected: 3つとも通る。1検査の打ち切りは、`--large`のときだけ4時間に延ばす（`run.main`で`timeout`を渡す。Task 1のテストに1件足す）。4時間で終わらない設定は、提出3件のまま`MaxFaults = 1`に下げて回し直し、下げたことを記録する。

- [ ] **Step 3: `make judge-model`が10分以内で通ることを確かめる**

Run: `time make judge-model`
Expected: 全行が期待どおりで、終了コード0、10分以内。`large`の検査は回らない。

- [ ] **Step 4: `formal-model.md`を仕上げる**

次の節がそろっていることを確かめ、足りないものを書く。

- 何を確かめていて、何を確かめていないか。
- モデルの動作と実装の場所の対応表。
- 回し方と、回し直すとき（配送、試行ID、キュー、切り替え手順のどれかを変えるとき）。
- 検査の一覧（`checks.json`の各行が何を確かめるか）。
- 検査した日、規模、結果。規模を上げた検査の結果を含む。
- 見つかったことと、その扱い。

- [ ] **Step 5: コミット**

```bash
git add judge/formal judge/tests/test_formal_run.py docs/judge/formal-model.md
git commit -m "docs: モデル検査の結果を記録する"
```

- [ ] **Step 6: 利用者に報告する**

報告する内容：通った性質、見つかったこととその扱い、2レーン切り替えの設計書を変える必要があるかどうか、2レーン切り替えの実装計画に進んでよいか。
