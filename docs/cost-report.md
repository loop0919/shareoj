# 運用費のページの運用

AWSの請求額を月に一度読み、GitHub Pagesの「ShareOJの運用費」に載せる仕組みの手引きである。
読む人は、このリポジトリの管理者である。
設計は[運用費ページの設計](superpowers/specs/2026-11-26-running-cost-page-design.md)にある。

## 何がどこにあるか

| もの | 場所 |
| --- | --- |
| ワークフロー | `.github/workflows/cost-report.yml` |
| 取得とページのスクリプト | `cost-report/` |
| 内訳のまとめ方、注記、リンク | `cost-report/config.json` |
| 月ごとの金額 | `cost-data`ブランチの`costs.json` |
| AWSのロール | `infra/deploy-access/`（[説明](../infra/README.md#運用費を読むロール)） |

`main`には金額のデータを置かない。
`main`へのpushは`Deploy dev`を走らせるためである。

## いつ動くか

- 毎月8日から14日の09:23（日本時間）に動く。
- AWSが前月の額を確定していれば、その日に載る。
  確定していなければ何も変えずに終わり、翌日にまた試す。
- 14日になっても確定していなければ、その実行は失敗し、GitHubの通知が届く。
- 一度載せたあとの実行は、金額が同じなら同じページを出し直すだけである。
  `costs.json`は書き換えない。
- AWSの記録があとから直されたときは、次の実行で過去の月も直る。
  直るのは、実行日から12か月前までの月である。それより古い月は保存した値のまま残る。

## 手で動かす

```console
gh workflow run cost-report.yml
gh workflow run cost-report.yml -f fetch=false
```

- 1つめは、AWSから読み直して載せる。
  前月がまだ確定していないと失敗する。
- 2つめは、AWSを読まずに、保存済みの金額でページだけ作り直す。
  ページの見た目や`config.json`を変えたあとに使う。

## 内訳を直す

`config.json`の`groups`が、ページの行である。上から順に表示する。

- `services`にAWSのサービス名を書くと、そのサービスの料金がその行に入る。
- `record_types`に料金の種類を書いた行は、サービスより先に当てはめる。
  前払いと消費税がこれに当たる。
- どの行にも当たらないサービスは「その他」に入る。
  「その他」が増えたら、サービス名を確かめて行に割り振る。
- `aws`は、行の下に小さく出すサービス名である。金額には関わらない。
- 行の`id`は`costs.json`に保存される。
  `id`を消すと、その行の古い月の金額は「その他」に寄せて表示される。

サービス名を確かめるときは、手元でCost Explorerを読む。
生の応答にはサービスごとの金額が入っている。
置いてよいのは`.local/cost-report/`だけで、リポジトリ、`cost-data`ブランチ、Actionsのログや成果物には残さない。

## 注記を書く

| 設定 | 出る場所 |
| --- | --- |
| `standing_note` | 一番上の合計の下に、常に出る。`null`で消える |
| `month_notes`の`"2026-10"`など | その月の内訳の上に出る |

- `month_notes`の文の中の`{prepay}`は、その月の「サーバーの前払い」の額に置き換わる。
- 注記を書いていない月でも、前払いの行があれば「サーバーの前払い $◯◯ を含みます。」と自動で出る。

## 予約購入をしたとき

RDSなどを予約購入（リザーブドインスタンス）で前払いすると、その額はサービスの料金の中に出る。
そのままでは「データベース」などの行に入る。

1. 買った月の応答を手元で読み、前払いの行の`RECORD_TYPE`を確かめる。
2. その値を`prepay`の`record_types`に足す。
3. 必要なら`month_notes`にその月の注記を書く。
4. 手で動かして、ページを確かめる。

Savings Planの前払いは、設定済みである。

## Sponsorsへのリンクを出す

`config.json`の`sponsors_url`にSponsorsのページのURLを入れ、`fetch=false`で手で動かす。
`null`の間、リンクは出ない。

## 失敗したとき

| ログの内容 | 見るところ |
| --- | --- |
| `AWS has not finalized last month's costs` | AWSの請求の画面で、前月の請求書が出ているかを確かめる。出たあとに手で動かす |
| `Could not assume role with OIDC` | Environment `cost-report`のシークレットと、ロールの信頼関係のsubject |
| `aws ce get-cost-and-usage failed` | ロールの許可（`ce:GetCostAndUsage`）と、Cost Explorerが有効か |
| `the output contains an account ID or an ARN` | `config.json`に12桁の数字や`arn:`を書いていないか。公開を止めるための検査である |
| `Publish the page`だけが失敗 | 金額は保存済みである。`fetch=false`で手で動かせば載る |

失敗した実行は、ページも`costs.json`も変えない。
