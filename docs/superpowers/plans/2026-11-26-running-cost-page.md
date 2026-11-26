# 運用費ページ Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** AWSの請求額を月に一度自動で読み、GitHub Sponsorsでの支援を考えている人に向けて、月ごとの運用費と内訳をGitHub Pagesで公開する。

**Architecture:** 定期実行のワークフローが、費用の読み取りだけを許したロールを引き受けてCost Explorerを読む。Pythonのスクリプトが応答を8行の内訳にまとめ、`cost-data`ブランチの`costs.json`に積み、1枚の静的なHTMLを組み立てる。ページはActionsの成果物としてPagesに配り、`main`にはpushしない。

**Tech Stack:** Python 3（標準ライブラリだけ、`unittest`）、AWS CLI v2（`aws ce get-cost-and-usage`）、GitHub Actions、GitHub Pages、Terraform（`infra/deploy-access/`）。

**Spec:** [docs/superpowers/specs/2026-11-26-running-cost-page-design.md](../specs/2026-11-26-running-cost-page-design.md)。各タスクの前に、対応する節を読む。

## Global Constraints

- アカウントID、リソースのID、ロールのARNを、追跡対象のファイル、コミットメッセージ、`cost-data`ブランチ、ページ、ワークフローのログに出さない。例にはダミー（`123456789012`）を使う。
- Cost Explorerの生の応答（サービスごとの金額）を、リポジトリ、`cost-data`ブランチ、Actionsの成果物、ログに残さない。手元で扱うときは`.local/cost-report/`に置く。先に`git check-ignore .local`で無視されることを確かめる。
- `main`にpushするワークフローを作らない。月ごとのデータは`cost-data`ブランチの`costs.json`だけに置く。
- 金額は米ドル、消費税込み、`UnblendedCost`。タグやアカウントで絞り込まない。
- 始まりの月は`2026-09`。
- 金額の計算は`decimal.Decimal`で行う。`float`を使わない。丸めは`ROUND_HALF_UP`で小数2桁。
- 金額の表記は`$1,234.56`、負の額は`-$12.34`。
- Pythonは標準ライブラリだけを使う。`boto3`を足さない。
- 作業は`feat/cost-report`ブランチで行う。`main`へのpushは`Deploy dev`を走らせるので、取り込みと公開はTask 7で利用者の了承を得てから行う。
- コミットメッセージは`prefix: 対応内容`の形にする。
- 内訳の行のIDと表示名（この順に表示する）。

  | ID | 表示名 |
  | --- | --- |
  | `prepay` | サーバーの前払い |
  | `database` | データベース |
  | `judge` | ジャッジサーバー |
  | `site` | サイトとAPI |
  | `security` | 監視・セキュリティ |
  | `domain` | ドメイン |
  | `other` | その他 |
  | `tax` | 消費税 |

- ページの文言。
  - 題: 「ShareOJの運用費」
  - 説明: 「ShareOJを動かすために、AWSへ毎月支払っている金額です。AWSの請求額を、月に一度、自動でそのまま載せています。」
  - 合計の添え書き: 「AWSの請求額・税込」
  - 表の見出し: 「内訳」「金額」、最後の行「合計」
  - 過去の月の見出し: 「これまでの月」
  - 常に出す注記: 「メインのサーバー代は2026年10月に3年分を前払いしたため、以降の月の請求には含まれません。」
  - 2026年10月の注記: 「3年分のサーバー前払い {prepay} を含みます。」
  - 注記がない月の自動の注記: 「サーバーの前払い {prepay} を含みます。」
  - 月がまだないとき: 「まだ載せられる月がありません。」
  - 下部: 「最終更新日: 2026年12月8日」の形、「GitHub SponsorsでShareOJを支援する」

### 共通のデータの形

`cost-report/config.json`（`main`に置く）。

```json
{
  "start": "2026-09",
  "sponsors_url": null,
  "standing_note": "メインのサーバー代は…",
  "month_notes": { "2026-10": "3年分のサーバー前払い {prepay} を含みます。" },
  "groups": [
    { "id": "prepay", "label": "サーバーの前払い", "aws": "Savings Plans", "record_types": ["SavingsPlanUpfrontFee", "SavingsPlanRecurringFee"] },
    { "id": "database", "label": "データベース", "aws": "Amazon RDS", "services": ["Amazon Relational Database Service"] },
    { "id": "other", "label": "その他" },
    { "id": "tax", "label": "消費税", "record_types": ["Tax"] }
  ]
}
```

- `record_types`の一致は`services`の一致より先に見る。どちらにも当たらなければ`other`。
- `aws`は行に小さく添えるサービス名で、手で書く。

`costs.json`（`cost-data`ブランチに置く）。

```json
{
  "version": 1,
  "updated": "2026-12-08",
  "months": {
    "2026-10": { "total": "412.35", "rows": [ { "group": "prepay", "amount": "318.00" } ] }
  }
}
```

- 金額は小数2桁の文字列。`rows`は表示順で、0.00の行は入れない。
- `updated`は`months`が変わった日（日本時間）。変わらなければ動かさない。

## Review Focus

仕様が黙っているが、使っていて踏みそうなもの。各行のテストは、かっこ内のタスクに入れてある。

1. Cost Explorerの応答が複数ページに分かれ、同じ月が2ページにまたがる。金額は上書きではなく足し合わせる（Task 2、Task 4）。
2. 1月に実行する。前月は前の年の12月になる（Task 2）。
3. クレジットが料金を上回り、行や月の合計が負になる。`-$12.34`と出し、落ちない（Task 2、Task 3）。
4. 保存済みの古い月が、設定から消した行のIDを持っている。「その他」に寄せて出し、落ちない（Task 3）。
5. 丸めると0になるごく小さい額（`0.004`、`-0.004`）。行を出さず、`-$0.00`と出さない（Task 2）。

---

### Task 1: 実際の請求額との突き合わせ（ここで合わなければ止める）

利用者のAWSへのログインが要る。
始める前に、利用者に`aws sso login --profile loop0919`を頼む。

**Files:**
- Create: `cost-report/config.json`
- 追跡しない: `.local/cost-report/response.json`

**Interfaces:**
- Produces: `cost-report/config.json`（上の「共通のデータの形」）。`services`には実際のサービス名が入る。

- [ ] **Step 1: 生の応答を手元に取る**

```bash
git check-ignore .local   # 何も出なければ止めて、無視の設定を先に入れる
mkdir -p .local/cost-report
aws --profile loop0919 --region us-east-1 ce get-cost-and-usage \
  --time-period Start=2026-09-01,End=2026-12-01 --granularity MONTHLY \
  --metrics UnblendedCost \
  --group-by Type=DIMENSION,Key=SERVICE Type=DIMENSION,Key=RECORD_TYPE \
  --output json > .local/cost-report/response.json
```

応答に`NextPageToken`があれば、`--next-page-token`で続きも取る。

- [ ] **Step 2: 月ごとの合計と`Estimated`を出す**

9月と10月が`Estimated: false`、途中の11月が`Estimated: true`であることを確かめる。
そうでなければ、`Estimated`を確定の合図に使えない。止めて利用者に戻す。

- [ ] **Step 3: 請求額と比べる**

`aws invoicing list-invoice-summaries help`で引数を確かめ、2026年9月と10月に出た請求書を全部読んで、月ごとに米ドルの額を足す。
このコマンドで読めなければ、利用者に請求の画面の月の合計（米ドル）を2つ教えてもらう。

合格の条件: 9月と10月のどちらも、Cost Explorerの合計との差が0.05ドル以内。
外れたら実装を進めず、差の額と、どの行で差が出ているかを添えて利用者に戻す。

- [ ] **Step 4: 前払いの出方を確かめる**

10月に`RECORD_TYPE`が`SavingsPlanUpfrontFee`の行があり、その額が前払いの額であることを確かめる。
名前が違えば、実際の値を`config.json`の`prepay`の`record_types`に使う。
消費税の行の`RECORD_TYPE`が`Tax`であることも確かめる。

- [ ] **Step 5: `cost-report/config.json`を書く**

応答に出たサービス名を全部、上の表の行に割り振る。
割り振りは仕様の「内訳のまとめ方」の表に従う。
迷うものは`other`にせず、利用者に聞く。
`sponsors_url`は`null`、`start`は`"2026-09"`、文言はGlobal Constraintsのとおり。

- [ ] **Step 6: 追跡対象に実額と識別子が入っていないことを確かめて、コミットする**

```bash
git status --short            # .local/ が出ないこと
grep -nE '[0-9]{12}|arn:' cost-report/config.json   # 何も出ないこと
git add cost-report/config.json
git commit -m "feat: 運用費の内訳のまとめ方を設定に持つ"
```

### Task 2: 応答を内訳にまとめる

**Files:**
- Create: `cost-report/costs.py`
- Create: `cost-report/tests/test_costs.py`
- Modify: `.github/workflows/ci.yml`（ジョブを1つ足す）

**Interfaces:**
- Consumes: `config.json`の形。
- Produces:
  - `class Pending(Exception)`。属性`months: list[str]`。
  - `query_period(start: str, today: datetime.date) -> tuple[str, str] | None`
  - `summarize(results: list[dict], config: dict) -> dict[str, dict]`
  - `merge(stored: dict, fresh: dict[str, dict], today: datetime.date) -> tuple[dict, bool]`
  - `assert_public(text: str) -> None`

`results`は、全ページの`ResultsByTime`をつないだリストである。
テストは`sys.path.insert(0, str(Path(__file__).resolve().parents[1]))`のあと`import costs`する（`judge/tests`と同じ形）。
応答の代わりは、テストの中に辞書で書く。金額は作り物を使う。

- [ ] **Step 1: 失敗するテストを書く**

`test_costs.py`に次を入れる。

```python
CONFIG = {"start": "2026-09", "groups": [
    {"id": "prepay", "label": "サーバーの前払い", "record_types": ["SavingsPlanUpfrontFee", "RIUpfrontExample"]},
    {"id": "database", "label": "データベース", "services": ["Amazon Relational Database Service"]},
    {"id": "judge", "label": "ジャッジサーバー", "services": ["Amazon Elastic Compute Cloud - Compute", "EC2 - Other"]},
    {"id": "other", "label": "その他"},
    {"id": "tax", "label": "消費税", "record_types": ["Tax"]},
]}

def month(start, groups, estimated=False):
    end = ...  # 翌月の1日
    return {"TimePeriod": {"Start": start, "End": end}, "Estimated": estimated,
            "Groups": [{"Keys": [s, r], "Metrics": {"UnblendedCost": {"Amount": a, "Unit": "USD"}}} for s, r, a in groups]}
```

- `test_groups_by_service_and_record_type`:
  `("Amazon Relational Database Service","Usage","28.404")`、`("Amazon Elastic Compute Cloud - Compute","Usage","20.10")`、`("EC2 - Other","Usage","4.00")`、`("Savings Plans for AWS Compute usage","SavingsPlanUpfrontFee","318.00")`、`("Tax","Tax","37.05")`
  → `{"2026-10": {"total": "407.55", "rows": [{"group":"prepay","amount":"318.00"},{"group":"database","amount":"28.40"},{"group":"judge","amount":"24.10"},{"group":"tax","amount":"37.05"}]}}`
- `test_reserved_upfront_inside_rds_moves_to_prepay`: `("Amazon Relational Database Service","RIUpfrontExample","649.00")`と`("Amazon Relational Database Service","Usage","10.00")` → `prepay`が`649.00`、`database`が`10.00`。
- `test_unknown_service_goes_to_other`: `("Amazon Braket","Usage","1.50")` → `other`が`1.50`。例外を出さない。
- `test_rows_sum_to_total`: `database`、`judge`、`other`に入るサービスがどれも`0.335` → `total`は`"1.01"`（1.005の四捨五入）、行は順に`0.34`、`0.34`、`0.33`。
- `test_tiny_amounts_are_dropped`: `database`が`0.004`、`judge`が`-0.004`、`other`が`5.00` → `rows`は`other`だけ。どの額も`"-0.00"`にならない。
- `test_credit_makes_row_and_total_negative`: `("Amazon Relational Database Service","Credit","-30.00")`と`("Amazon Relational Database Service","Usage","10.00")` → `database`が`"-20.00"`、`total`が`"-20.00"`。
- `test_same_month_on_two_pages_is_added`: 同じ`2026-10`の要素を2つ渡す（片方に`database` 10.00、もう片方に`database` 5.00と`judge` 1.00） → `database`が`15.00`、`judge`が`1.00`。
- `test_month_without_groups`: `Groups`が空 → `{"total": "0.00", "rows": []}`。
- `test_estimated_month_raises_pending`: `2026-10`が確定、`2026-11`が`Estimated: True` → `Pending`を出し、`months == ["2026-11"]`。
- `test_query_period`:
  - `query_period("2026-09", date(2026,12,8))` → `("2026-09-01", "2026-12-01")`
  - `query_period("2026-09", date(2027,1,8))` → `("2026-09-01", "2027-01-01")`（前月は前の年の12月）
  - `query_period("2026-09", date(2028,3,8))` → `("2027-03-01", "2028-03-01")`（始まりを12か月前までに収める）
  - `query_period("2026-09", date(2026,9,20))` → `None`（確定した月がまだない）
- `test_merge_overwrites_and_keeps_old_months`: 保存済みが`2026-09`と`2026-10`、新しい分が`2026-10`（額が違う）と`2026-11` → 3つの月があり、`2026-09`は元のまま、`2026-10`は新しい額、変更ありが`True`、`updated`が`today`のISO形式。
- `test_merge_without_change_keeps_updated`: 新しい分が保存済みと同じ → 変更ありが`False`、`updated`は元のまま、返る辞書は保存済みと等しい。
- `test_merge_into_empty`: 保存済みが`{"version":1,"updated":None,"months":{}}` → 新しい月が入り、`True`。
- `test_assert_public`: `"arn:aws:iam::123456789012:role/x"`と`"id 123456789012"`は`ValueError`。`"$1,234,567.89"`と`"2026-12-08"`は通る。

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s cost-report/tests -v`
Expected: `ModuleNotFoundError: No module named 'costs'`

- [ ] **Step 3: `cost-report/costs.py`を実装する**

決めておくこと。

- 行への割り振りは、`record_types`に当たる行を先に探し、なければ`services`、なければ`other`。
- 月の合計は、丸める前の額を全部足してから丸める。
- 行の丸めは最大剰余法で行い、行の和を合計に一致させる。
  各行をセントの単位で切り捨て（`ROUND_FLOOR`）、合計との差のセント数だけ、端数の大きい行から1セントずつ足す。
  端数が同じときは設定の順で先の行を優先する。
- 丸めたあと`0.00`の行は落とす。
- `query_period`の終わりは`today`の月の1日。始まりは、`start`の月の1日と、`today`の月の12か月前の1日の、遅いほう。始まりが終わり以降なら`None`。
- `assert_public`は、前後が数字でない12桁の数字と、`arn:`を見つけたら`ValueError`。

- [ ] **Step 4: 通ることを確かめる**

Run: `python3 -m unittest discover -s cost-report/tests -v`
Expected: 全部`ok`

- [ ] **Step 5: CIにジョブを足す**

`.github/workflows/ci.yml`の`jobs`に、`api-infra`と同じ書き方で次を足す。

```yaml
  cost-report:
    name: Test cost report
    runs-on: ubuntu-24.04
    timeout-minutes: 5
    permissions:
      contents: read
    steps:
      - name: Check out repository
        uses: actions/checkout@v7.0.0
      - name: Test the cost report
        run: python3 -m unittest discover -s cost-report/tests -v
```

- [ ] **Step 6: コミットする**

```bash
git add cost-report/costs.py cost-report/tests/test_costs.py .github/workflows/ci.yml
git commit -m "feat: AWSの費用の応答を運用費の内訳にまとめる"
```

### Task 3: ページを組み立てる

**Files:**
- Create: `cost-report/page.py`
- Create: `cost-report/template.html`
- Create: `cost-report/tests/test_page.py`

**Interfaces:**
- Consumes: `costs.json`の形、`config.json`の形。
- Produces:
  - `money(amount: str) -> str`
  - `render(data: dict, config: dict) -> str`（HTMLの全文）

仕様の「ページ」の節を読む。
過去の月の開閉は`<details>`と`<summary>`で作る。JavaScriptは使わない。
外部のフォント、スクリプト、画像を読み込まない。

- [ ] **Step 1: 失敗するテストを書く**

`test_page.py`に、月が3つ（`2026-08`、`2026-09`、`2026-10`。10月に`prepay` `318.00`）のデータと、Task 1と同じ形の設定を置く。

- `test_money`: `"1234.5"`→`"$1,234.50"`、`"-12.34"`→`"-$12.34"`、`"0.00"`→`"$0.00"`、`"412.35"`→`"$412.35"`。
- `test_latest_month_is_on_top`: 出力で`2026年10月`が`これまでの月`より前にあり、10月の合計`$412.35`が`<details`より前に1回出る。
- `test_past_months_newest_first_in_details`: `<details`が2つ。`2026年9月`が`2026年8月`より前。10月は`<details>`の中にない。
- `test_rows_use_labels_in_config_order_with_total_last`: 10月の表で`サーバーの前払い`、`データベース`、`消費税`、`合計`がこの順に出る。
- `test_aws_names_are_shown_small`: 設定の`aws`の文字列（例: `Amazon RDS`）が出力にある。
- `test_month_note_fills_prepay`: `month_notes["2026-10"]`があるとき、`3年分のサーバー前払い $318.00 を含みます。`が出る。
- `test_auto_note_when_prepay_without_month_note`: `month_notes`を空にすると、`サーバーの前払い $318.00 を含みます。`が出る。`prepay`の行がない9月には注記が出ない。
- `test_standing_note`: `standing_note`が文字列なら出る。`None`なら出ない。
- `test_sponsors_link_only_when_set`: `sponsors_url`が`None`なら`GitHub Sponsors`が出力にない。`"https://example.com/s"`なら`href="https://example.com/s"`がある。
- `test_updated_date_in_japanese`: `updated`が`"2026-12-08"` → `最終更新日: 2026年12月8日`。
- `test_single_month`: 月が1つ → `これまでの月`と`<details`が出ない。
- `test_no_months`: `months`が空 → `まだ載せられる月がありません。`が出て、例外を出さない。
- `test_negative_amounts`: 行が`"-20.00"`、合計が`"-20.00"` → `-$20.00`が出る。
- `test_unknown_group_id_falls_into_other`: 保存済みの行に`{"group":"removed","amount":"3.00"}`と`{"group":"other","amount":"2.00"}` → `その他`が`$5.00`で1行。`removed`は出力にない。
- `test_text_is_escaped`: `standing_note`が`"a < b & c"` → `a &lt; b &amp; c`が出て、`a < b`は出ない。
- `test_page_basics`: `<html lang="ja">`、`<meta name="viewport"`、`<title>ShareOJの運用費</title>`があり、`noindex`、`<script`、`http://`がない。
- `test_output_is_public`: `costs.assert_public(render(...))`が通る。

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s cost-report/tests -v`
Expected: `ModuleNotFoundError: No module named 'page'`

- [ ] **Step 3: `template.html`と`page.py`を実装する**

- `template.html`はCSSを中に持つ1枚のHTMLで、`page.py`が`string.Template`で埋める。
- 設定とデータから来る文字列は、全部`html.escape`を通す。
- 色は`web/app/assets/css/main.css`の`:root`と同じ値を`light-dark()`で持つ（`--color-paper`、`--color-surface`、`--color-ink`、`--color-muted`、`--color-line`、`--color-accent`）。`color-scheme: light dark`を指定する。
- 書体は`"Noto Sans JP", "Hiragino Kaku Gothic ProN", "Yu Gothic", system-ui, sans-serif`。金額は`font-variant-numeric: tabular-nums`で右に寄せる。
- 本文の幅は最大40rem、左右の余白は16px。表は幅100%で、スマホの幅で横スクロールを出さない。
- 合計は大きく（2.5rem前後）出す。
- 下部に`https://www.share-oj.net`へのリンク「ShareOJ」を置く。

- [ ] **Step 4: 通ることを確かめる**

Run: `python3 -m unittest discover -s cost-report/tests -v`
Expected: 全部`ok`

- [ ] **Step 5: 見た目を確かめる**

テストのデータで`render`の結果を`.local/cost-report/preview.html`に書き出し、ブラウザで、幅375pxと広い幅、明るい配色と暗い配色を見る。
Tabキーで月に移り、Enterで開閉できることを確かめる。
スクリーンショットを利用者に見せる。

- [ ] **Step 6: コミットする**

```bash
git add cost-report/page.py cost-report/template.html cost-report/tests/test_page.py
git commit -m "feat: 運用費のページを組み立てる"
```

### Task 4: 取得と更新のコマンド

**Files:**
- Create: `cost-report/fetch.py`
- Create: `cost-report/update.py`
- Create: `cost-report/tests/test_fetch.py`
- Create: `cost-report/tests/test_update.py`

**Interfaces:**
- Consumes: Task 2の`query_period`、`summarize`、`merge`、`assert_public`、`Pending`。Task 3の`render`。
- Produces:
  - `fetch(start: str, end: str, run=subprocess.run) -> list[dict]`
  - `update.main(argv: list[str], fetcher=fetch.fetch) -> int`
  - コマンド: `python3 cost-report/update.py --config cost-report/config.json --data data/costs.json --site site [--today YYYY-MM-DD] [--no-fetch]`
  - `GITHUB_OUTPUT`があれば`status=updated|unchanged|pending`を1行足す。同じ行を標準出力にも出す。

- [ ] **Step 1: 失敗するテストを書く**

`test_fetch.py`（`run`に偽物を渡す。偽物は`argv`を記録し、`stdout`にJSONを返す）。

- `test_command`: `fetch("2026-09-01","2026-12-01", run)`の1回目の`argv`が、`aws`、`ce`、`get-cost-and-usage`、`--region us-east-1`、`--time-period Start=2026-09-01,End=2026-12-01`、`--granularity MONTHLY`、`--metrics UnblendedCost`、`--group-by Type=DIMENSION,Key=SERVICE Type=DIMENSION,Key=RECORD_TYPE`、`--output json`を含む。
- `test_follows_next_page_token`: 1回目が`NextPageToken: "t1"`、2回目がなし → 呼び出しは2回、2回目の`argv`に`--next-page-token t1`があり、返り値は2ページの`ResultsByTime`をつないだもの。
- `test_stops_after_five_pages`: 毎回`NextPageToken`を返す → 5回で`RuntimeError`。
- `test_cli_failure_raises_without_stdout`: 終了コード255、`stdout`に`"SECRET-BODY"`、`stderr`に`"denied"` → `RuntimeError`。メッセージに`denied`があり、`SECRET-BODY`がない。

`test_update.py`（一時ディレクトリを使い、`fetcher`に偽物を渡す。標準出力は`contextlib.redirect_stdout`で取る）。

- `test_updates_data_and_site`: 保存済みが空、偽物が確定した2か月を返す、`--today 2026-11-08` → 返り値0、`costs.json`に2つの月、`site/index.html`があり題を含む、出力に`status=updated`。
- `test_unchanged_writes_site_but_not_data`: 同じ応答で2回目 → `status=unchanged`、`costs.json`のバイト列が変わらず、`site/index.html`はある。
- `test_pending_writes_nothing`: 前月が`Estimated: True` → 返り値0、`status=pending`、`costs.json`が変わらず、`site`に何も作られない。
- `test_no_fetch_rebuilds_from_stored`: `--no-fetch` → 偽物が呼ばれず、`site/index.html`ができ、`status=unchanged`。
- `test_nothing_to_query`: `--today 2026-09-20`、保存済みが空 → 偽物が呼ばれず、`status=unchanged`、ページに`まだ載せられる月がありません。`。
- `test_fetch_error_is_nonzero_and_writes_nothing`: 偽物が`RuntimeError` → 返り値が0でなく、`costs.json`が変わらず、`site`に何も作られない。
- `test_leak_stops_publication`: 設定の`standing_note`に`123456789012`を入れる → 返り値が0でなく、`site/index.html`も`costs.json`も書かれない。
- `test_github_output`: 環境変数`GITHUB_OUTPUT`に一時ファイルを指す → そのファイルに`status=updated`の行がある。
- `test_stdout_has_no_amounts`: 更新したときの標準出力に、応答の金額の文字列が1つも出ない。

- [ ] **Step 2: 失敗を確かめる**

Run: `python3 -m unittest discover -s cost-report/tests -v`
Expected: `ModuleNotFoundError: No module named 'fetch'`

- [ ] **Step 3: `fetch.py`と`update.py`を実装する**

- `fetch`は`run(argv, capture_output=True, text=True)`で呼ぶ。応答を標準出力やログに出さない。
- `update.main`の順は、設定と保存済みを読む → `query_period` → `fetch` → `summarize` → `merge` → `render` → `assert_public`（ページと`costs.json`の両方の文字列） → 書き出し。
  書き出しは最後にまとめて行い、途中で失敗したら何も書かない。
- `costs.json`は`json.dumps(..., ensure_ascii=False, indent=2, sort_keys=True)`に改行を1つ付けて書く。変更がなければ書かない。
- `today`の既定は`datetime.datetime.now(datetime.timezone.utc).date()`。

- [ ] **Step 4: 通ることを確かめる**

Run: `python3 -m unittest discover -s cost-report/tests -v`
Expected: 全部`ok`

- [ ] **Step 5: 手元の本物の応答で通す**

Task 1の`.local/cost-report/response.json`から、始まりが2026年11月より前の月だけを返す偽物を作り、`update.main`を`--today 2026-11-26`で呼ぶ。
途中の11月は見積もり中なので、渡すと`pending`になる。
`.local/cost-report/site/index.html`を開き、9月と10月の合計が、Task 1で確かめた請求額と0.05ドル以内で合うことを確かめる。
この出力はコミットしない。

- [ ] **Step 6: コミットする**

```bash
git add cost-report/fetch.py cost-report/update.py cost-report/tests/test_fetch.py cost-report/tests/test_update.py
git commit -m "feat: 運用費を取得してデータとページを更新するコマンドを足す"
```

### Task 5: 費用を読むだけのロール

**Files:**
- Modify: `infra/deploy-access/main.tf`
- Modify: `infra/deploy-access/variables.tf`
- Modify: `infra/deploy-access/terraform.tfvars.example`
- Modify: `infra/deploy-access/tests/access.tftest.hcl`（`variables`に新しい変数を足す）
- Create: `infra/deploy-access/tests/cost_report.tftest.hcl`
- Modify: `infra/README.md`

**Interfaces:**
- Produces:
  - 変数`github_cost_report_subject`
  - リソース`aws_iam_role.github_cost_report`（名前`${local.name}-github-cost-report`）、`aws_iam_role_policy.cost_report`
  - 出力`github_cost_report_role_arn`

- [ ] **Step 1: 失敗するテストを書く**

`cost_report.tftest.hcl`は`access.tftest.hcl`と同じ`mock_provider`と`variables`を持ち、`github_cost_report_subject = "repo:owner@123/project@456:environment:cost-report"`を足す。

- `run "cost_report_reads_costs_only"`（`command = plan`）:
  - `jsondecode(aws_iam_role_policy.cost_report.policy).Statement`の長さが1。
  - その`Effect`が`"Allow"`、`Action`が`["ce:GetCostAndUsage"]`と等しい。
- `run "cost_report_trust_is_exact"`:
  - 信頼ポリシーの`Statement`の長さが1、`Action`が`"sts:AssumeRoleWithWebIdentity"`。
  - 条件が`StringEquals`だけで、`token.actions.githubusercontent.com:sub`が変数の値、`token.actions.githubusercontent.com:aud`が`"sts.amazonaws.com"`。
- `run "cost_report_subject_rejects_dev"`: `github_cost_report_subject = "repo:owner@123/project@456:environment:dev"`、`expect_failures = [var.github_cost_report_subject]`。
- `run "cost_report_subject_rejects_wildcard"`: `"repo:owner@123/*:environment:cost-report"`で同じ。
- `run "cost_report_subject_rejects_branch"`: `"repo:owner@123/project@456:ref:refs/heads/main"`で同じ。

- [ ] **Step 2: 失敗を確かめる**

Run: `terraform -chdir=infra/deploy-access init -backend=false -input=false -lockfile=readonly && terraform -chdir=infra/deploy-access test`
（`nix develop 'path:.'`の中で実行する）
Expected: `cost_report.tftest.hcl`が、変数とリソースがないことで失敗する。

- [ ] **Step 3: 変数、ロール、ポリシー、出力を足す**

- 変数の検査は`can(regex("^repo:[^*]+:environment:cost-report$", var.github_cost_report_subject))`。
- 信頼ポリシーは`github_deploy`と同じ形で、`sub`だけ新しい変数にする。
- ポリシーは`Sid = "ReadCosts"`、`Action = ["ce:GetCostAndUsage"]`、`Resource = "*"`の1文だけ。
- `terraform.tfvars.example`に`github_cost_report_subject = "repo:OWNER@OWNER_ID/REPO@REPO_ID:environment:cost-report"`を、既存の行と同じ注釈の付け方で足す。

- [ ] **Step 4: 通ることを確かめる**

Run: `terraform fmt -check -recursive infra && terraform -chdir=infra/deploy-access validate && terraform -chdir=infra/deploy-access test`
Expected: 全部成功。既存の`access.tftest.hcl`も通る。

- [ ] **Step 5: `infra/README.md`に書き足す**

「GitHubデプロイロール」の節のあとに、節「運用費を読むロール」を足す。
書くこと: ロールの名前、許可が`ce:GetCostAndUsage`だけであること、GitHubの環境`cost-report`に限ること、環境を`main`だけに制限するのはGitHub側の設定であること、ARNを環境のシークレット`AWS_COST_REPORT_ROLE_ARN`に入れること。
一番上の表の`deploy-access/`の行も合わせて直す。

- [ ] **Step 6: コミットする**

```bash
git add infra/deploy-access infra/README.md
git commit -m "feat: 運用費を読むだけのGitHub用ロールを足す"
```

### Task 6: ワークフローと運用の文書

**Files:**
- Create: `.github/workflows/cost-report.yml`
- Create: `docs/cost-report.md`
- Modify: `docs/README.md`（文書の一覧があれば1行足す）

**Interfaces:**
- Consumes: Task 4のコマンドと`status`、Task 5のシークレット名`AWS_COST_REPORT_ROLE_ARN`、環境`cost-report`、ブランチ`cost-data`の`costs.json`。

- [ ] **Step 1: 使うアクションの版を調べ、コミットのSHAで固定する**

`actions/checkout`と`aws-actions/configure-aws-credentials`は、`deploy-dev.yml`と同じ版にする。
`actions/configure-pages`、`actions/upload-pages-artifact`、`actions/deploy-pages`は、次で最新のリリースとSHAを調べる。

```bash
gh api repos/actions/deploy-pages/releases/latest -q .tag_name
gh api repos/actions/deploy-pages/commits/<tag> -q .sha
```

`uses: actions/deploy-pages@<SHA> # <tag>`の形で書く。
`aws-actions/configure-aws-credentials`のその版に`mask-aws-account-id`の入力があることを、その版の`action.yml`で確かめる。

- [ ] **Step 2: `.github/workflows/cost-report.yml`を書く**

```yaml
name: Cost report

on:
  schedule:
    - cron: '23 0 8-14 * *' # 09:23 JST、毎月8日から14日
  workflow_dispatch:
    inputs:
      fetch:
        description: AWSから読み直す。外すと、保存済みのデータでページだけ作り直す
        type: boolean
        default: true

permissions: {}

concurrency:
  group: cost-report
  cancel-in-progress: false
```

ジョブは2つ。

`build`（`environment: cost-report`、`permissions: contents: write, id-token: write`、`timeout-minutes: 10`、出力`publish`）。

1. `main`をチェックアウトする。
2. `cost-data`ブランチを`data`にチェックアウトする（`ref: cost-data`、`path: data`）。
3. `fetch`が真のときだけ、ロールを引き受ける。
   `role-to-assume: ${{ secrets.AWS_COST_REPORT_ROLE_ARN }}`、`aws-region: us-east-1`、`mask-aws-account-id: true`、`role-session-name: GitHubActions-${{ github.run_id }}`。
   `aws sts get-caller-identity`は呼ばない。
4. `python3 cost-report/update.py --config cost-report/config.json --data data/costs.json --site site`を、`id: update`で実行する。`fetch`が偽なら`--no-fetch`を付ける。
   定期実行では入力が空になる。空は真として扱う。
5. `status`が`pending`のとき: 手での実行か、UTCの日付が14日以降なら`::error::`を出して失敗させる。それ以外は`::notice::`を出し、以降のステップを飛ばす。
6. `status`が`updated`のとき: `data`で`github-actions[bot]`の名義でコミットし、`cost-data`へpushする。メッセージは`data: 運用費のデータを更新する`。
7. `status`が`updated`か、手での実行のとき: `site`をPagesの成果物として上げ、出力`publish`を`true`にする。

`deploy`（`needs: build`、`if: needs.build.outputs.publish == 'true'`、`environment: github-pages`、`permissions: pages: write, id-token: write`）。

1. `actions/deploy-pages`を実行する。

`${{ }}`の値を`run:`の中に直接書かない。`env:`を通す（`deploy-dev.yml`と同じ）。

- [ ] **Step 3: 書式を確かめる**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/cost-report.yml'))"`（`yaml`がなければ`nix develop`の中で実行する）
Expected: 何も出ない。

`actionlint`が開発シェルにあれば、それも通す。

- [ ] **Step 4: `docs/cost-report.md`を書く**

読む人はこのリポジトリの管理者である。短く、箇条書きと表で書く。
入れること。

- 何がいつ動くか（毎月8日から14日、確定するまで待つ）。
- 手での実行: `gh workflow run cost-report.yml`、ページだけ作り直すときは`-f fetch=false`。
- 内訳の直し方: `cost-report/config.json`の`groups`。新しいサービスは「その他」に入るので、月に一度見て割り振る。古い月は、Cost Explorerが読める12か月の間だけ新しいまとめ方に直る。
- 注記の書き方: `month_notes`と`{prepay}`、`standing_note`。
- 予約購入（リザーブドインスタンス）をしたとき: その月の応答で前払いの`RECORD_TYPE`を確かめ、`prepay`の`record_types`に足す。足すまで、その額は「データベース」などの行に入る。
- Sponsorsのリンクの出し方: `sponsors_url`に入れて、手で実行する。
- 失敗したとき: 14日になっても確定しない場合と、AWSの呼び出しが失敗した場合の見方。
- 生の応答を置いてよいのは`.local/cost-report/`だけであること。

- [ ] **Step 5: コミットする**

```bash
git add .github/workflows/cost-report.yml docs/cost-report.md docs/README.md
git commit -m "feat: 運用費のページを毎月更新するワークフローを足す"
```

### Task 7: 公開（利用者の手と了承が要る）

リポジトリの設定を変えるステップは、1つずつ利用者の了承を得てから行う。

**Files:**
- Modify: `docs/superpowers/specs/2026-11-26-running-cost-page-design.md`（状態）

- [ ] **Step 1: ロールを作る（利用者）**

`gh api repos/{owner}/{repo}/actions/oidc/customization/sub`で`sub_claim_prefix`を確かめ、末尾に`:environment:cost-report`を付けた値を、`infra/deploy-access/terraform.tfvars`の`github_cost_report_subject`に入れる。
利用者が`infra/README.md`の手順で`deploy-access`をplanし、足されるのがロール1つとポリシー1つだけであることを確かめてapplyする。

- [ ] **Step 2: GitHubの環境とシークレットを作る**

```bash
gh api -X PUT repos/{owner}/{repo}/environments/cost-report \
  -F 'deployment_branch_policy[protected_branches]=false' \
  -F 'deployment_branch_policy[custom_branch_policies]=true'
gh api -X POST repos/{owner}/{repo}/environments/cost-report/deployment-branch-policies \
  -f name=main -f type=branch
terraform -chdir=infra/deploy-access output -raw github_cost_report_role_arn \
  | gh secret set AWS_COST_REPORT_ROLE_ARN --env cost-report
terraform -chdir=infra/deploy-access output -raw github_cost_report_role_id \
  | gh secret set AWS_COST_REPORT_ROLE_ID --env cost-report
```

ARNとロールのIDを画面に出さない（パイプで渡す）。
環境とブランチの制限は、シークレットを入れる前、`main`に取り込む前に作る。
環境がないままワークフローが動くと、制限のない環境が自動で作られる。

確かめる。

Run: `gh api repos/{owner}/{repo}/environments/cost-report/deployment-branch-policies -q '[.branch_policies[].name]'`
Expected: `["main"]`

- [ ] **Step 3: `cost-data`ブランチを作る**

親のないブランチに`costs.json`（`{"version": 1, "updated": null, "months": {}}`をTask 4と同じ書式で）だけを置き、`data: 運用費のデータの置き場を作る`でコミットしてpushする。
作業ツリーを汚さないよう、`git worktree add --orphan`か一時のクローンで行う。

- [ ] **Step 4: Pagesを有効にする**

```bash
gh api -X POST repos/{owner}/{repo}/pages -f build_type=workflow
```

`github-pages`環境の配布できるブランチが`main`だけであることを、Step 2と同じ読み方で確かめる。

- [ ] **Step 5: `main`に取り込んでpushする（利用者の了承のあと）**

`feat/cost-report`を`main`に取り込み、pushする。
`Deploy dev`が走る。アプリの変更はないので、成功して差分が出ないことを見届ける。

- [ ] **Step 6: 手で実行する**

Run: `gh workflow run cost-report.yml && gh run watch`
Expected: `build`と`deploy`が成功。

確かめること。

- 公開されたページの9月と10月の合計が、Task 1の請求額と0.05ドル以内で合う。
- `cost-data`ブランチの`costs.json`に、行の金額だけが入っている。
- ジョブごとにログを取り、12桁の数字、`arn:`、ロールのID（`AROA`で始まる）が出ていないこと。

  Run: `gh run view <id> --job <job-id> --log | grep -cE '(^|[^0-9])[0-9]{12}([^0-9]|$)|arn:|AROA'`
  Expected: `0`

- `main`に新しいコミットができていない。`Deploy dev`が走っていない。
- スマホの幅と暗い配色で、公開されたページを見る。

- [ ] **Step 7: 設計書の状態を直してコミットする**

設計書の「状態」を「2026-MM-DDに公開」にする。

```bash
git add docs/superpowers/specs/2026-11-26-running-cost-page-design.md
git commit -m "docs: 運用費ページの設計の状態を、公開したことに合わせる"
```

- [ ] **Step 8: 残ることを利用者に伝える**

- 次の定期実行（12月8日）で11月分が載ることを、12月15日までに一度見る。
- Sponsorsを公開するときに、`sponsors_url`を入れ、Sponsorsの紹介文からこのページへリンクする。
