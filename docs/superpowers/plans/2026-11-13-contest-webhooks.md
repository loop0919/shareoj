# コンテストのWebhook Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** コンテストの提出の判定が確定するたびに、主催者が登録した送り先（1コンテスト3つまで、JSON形式かDiscord形式）へ、判定と順位表の行をHTTPで送る。

**Architecture:** `submissions`の表に置くトリガーが、判定の確定と同じトランザクションで出来事を1行記録する。VPCの中の`judge-bridge`がそれを読み、順位表を集計して送り先ごとの本文を作り、SQSのFIFOキューへ入れる。VPCの外に新しく置く`webhook-sender`（Go）がHTTPで送り、結果を別のキューで`judge-bridge`へ返して送信の記録に書く。設定は、コンテスト本体の保存とは別のAPIと、編集画面の新しい区分「外部連携」で読み書きする。

**Tech Stack:** Go（pgx、`aws-lambda-go`、`aws-sdk-go-v2`のSQS）、PostgreSQL（トリガー）、Terraform（`infra/judge`）、Nuxt 4（Vue 3、zod）、Playwright

**Spec:** [docs/superpowers/specs/2026-11-13-contest-webhooks-design.md](../specs/2026-11-13-contest-webhooks-design.md)

## Global Constraints

- 送り先は1コンテストにつき3つまで。形式は`json`か`discord`。絞り込みは判定が`all`か`ac`、区分が`all`か`onsite`。
- 送り先のURL：`https`、ポート443だけ、ユーザー名とパスワードなし、2,000文字まで。`discord`は、ホストが`discord.com`でパスが`/api/webhooks/`で始まるものだけ。公開のインターネットでないアドレスは、保存時と送信時の両方で拒む。リダイレクトに従わない。
- 送信：1回5秒で打ち切り、2xxが成功、合計3回まで、出来事の記録から60秒であきらめる。同じ送り先へは1通ずつ順に送る。
- 自動停止：20件続けて失敗した送り先を止める。429であきらめた出来事は数えない。
- 最近の送信は送り先ごとに直近20件。相手の応答の本文は残さない。
- テスト送信は同じ送り先へ10秒に1回まで。
- JSON形式のヘッダー：`Content-Type: application/json`、`User-Agent: ShareOJ-Webhook/1`、`X-ShareOJ-Event-Id`、`X-ShareOJ-Timestamp`（UNIX秒）、`X-ShareOJ-Signature: sha256=<hex>`（`<タイムスタンプ>.<本文>`を署名の鍵でHMAC-SHA256）。
- JSON形式の本文の`version`は`1`、`event`は`submission.judged`か`ping`。
- 送り先のURLと署名の鍵は、ログ、エラーメッセージ、監視の通知に出さない。設定のAPIは、セッター（オーナーと共同作成者）以外には`404 contest_not_found`を返す。
- 判定の確定と順位表の表示を遅らせない。`judge-bridge`がWebhookに使う時間は、1回の起動につき5秒まで。
- 採点ワーカー（`judge/runner`、`api/cmd/judge-worker`）は変えない。
- コマンドはリポジトリのルートで`nix develop`の中から実行する。Goの統合テストは`make db-up`の後に`TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable'`を付ける。Playwrightは`web`で`bun run build`の後に`bun --bun playwright test <file> --output=<scratchpad>/pw`（`web/test-results`はroot所有）。
- 作業は新しいブランチ`feat/contest-webhooks`で行う。配布（データベースの移行、Lambda、Terraformの適用）はこの計画に含めず、利用者と別に行う。
- コミットメッセージは`prefix: 対応内容`（日本語）。コードを変えたら`graphify update .`を実行する。
- 文書は`.claude/skills/japanese-tech-writing`の規範に従う。

## 設計書から変えた点

- **出来事の記録はトリガーで行う**：設計書は「判定を確定する4つの処理のすべてで記録する」とした。4か所にコードを足す代わりに、`submissions`の`status`が`DONE`になった行を拾うトリガーを1つ置く。同じトランザクションで記録する、という要件は変わらない。
- **自動停止のベルは、コンテストのオーナーを通知の主にする**：設計書は「操作した人のいない通知を作れるようにする」とした。通知の一覧は通知の主のプロフィールを必ず結合するので、`actor_id`にコンテストのオーナーを入れ、画面はこの種類だけ名前を出さない。Task 12で設計書のその一文を直す。

## Review Focus

- **ハンドルやチーム名にDiscordの書式や長い文字列が入る**：`*`、`_`、`` ` ``、`~`、`|`、`>`、`@everyone`を含む名前は、書式として効かず、メンションも飛ばない。1行が長くても、1通は2,000文字を超えない（Task 4とTask 8のテスト）。
- **内部を指すアドレスの変種**：`::ffff:10.0.0.1`、`169.254.169.254`、`fd00::1`、`[::1]`、公開と非公開のアドレスを両方返すホスト名、`https://example.com:8443/`は拒む（Task 2とTask 8のテスト）。
- **相手が応答を返さない、または本文を少しずつ返し続ける**：接続から本文の読み終わりまでを5秒で打ち切り、同じまとまりの中のほかの送り先への送信は続く（Task 8のテスト）。
- **一括リジャッジで出来事が1,000件たまる**：`judge-bridge`は1回の起動で5秒分だけ処理し、60秒を過ぎた出来事は送らずに捨てる。採点結果の反映は止まらない（Task 7とTask 9のテスト）。
- **判定の確定から本文を作るまでのあいだに状況が変わる**：提出者がスタッフになった、送り先が消された、止められた、コンテストの下書きが消された。どの場合も出来事を捨てるだけで、エラーにしない。消された送り先への結果の報告は何もしない（Task 7のテスト）。

---

### Task 1: 表とトリガーを足す

**Files:**
- Create: `api/internal/database/048_contest_webhooks.sql`
- Modify: `api/internal/contests/drafts.go`（`forgetContest`に`DELETE FROM contest_webhooks WHERE contest_id=$1`を足す）
- Test: `api/internal/webhooks/trigger_test.go`（新しいパッケージ`webhooks`。`api/internal/httpapi/contest_teams_test.go`と同じやり方で`TEST_DATABASE_URL`のデータベースを使う）

**Interfaces:**
- Produces: 下の3つの表、通知の種類`contest_webhook_stopped`。

```sql
CREATE TABLE contest_webhooks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    contest_id uuid NOT NULL,            -- 下書きと公開後で同じID。contestsへの外部キーは張らない（contest_setter_invitationsと同じ）
    url text NOT NULL,
    format text NOT NULL CHECK (format IN ('json','discord')),
    verdicts text NOT NULL DEFAULT 'all' CHECK (verdicts IN ('all','ac')),
    division text NOT NULL DEFAULT 'all' CHECK (division IN ('all','onsite')),
    enabled boolean NOT NULL DEFAULT true,
    secret text NOT NULL,
    failures integer NOT NULL DEFAULT 0,
    stopped_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX contest_webhooks_contest ON contest_webhooks(contest_id, created_at, id);

CREATE TABLE contest_webhook_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id uuid NOT NULL REFERENCES contest_webhooks(id) ON DELETE CASCADE,
    ping boolean NOT NULL DEFAULT false,
    summary text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','delivered','failed')),
    detail text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX contest_webhook_deliveries_recent ON contest_webhook_deliveries(webhook_id, created_at DESC, id);

-- 送るべき出来事。判定の確定ならsubmission_id、テスト送信ならdelivery_idを持つ。
CREATE TABLE contest_webhook_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    contest_id uuid NOT NULL,
    submission_id uuid REFERENCES submissions(id) ON DELETE CASCADE,
    delivery_id uuid REFERENCES contest_webhook_deliveries(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((submission_id IS NULL) <> (delivery_id IS NULL))
);
```

- [ ] **Step 1: 失敗するテストを書く**

`TestWebhookTrigger`に次の場合を置く。どれも、提出の`status`を`DONE`に`UPDATE`した後の`contest_webhook_events`の行数を見る。

| 場合 | 行数 |
| --- | --- |
| 開催中の参加者の提出、有効な送り先あり | 1 |
| 同じ提出を`RUNNING`に戻してもう一度`DONE`にする（リジャッジ） | 2 |
| `DONE`の行の`result`だけを`UPDATE`する | 増えない |
| 送り先がない、または`enabled=false`の送り先だけ | 0 |
| 終了後に作られた提出（練習） | 0 |
| セッターの提出、テスターの提出（`contest_competitor`が偽） | 0 |
| `job`の`easyTest`、`generate`、`validate`のどれかが真 | 0 |
| `contest_id`のない提出 | 0 |
| 終了前に作られ、終了後に`DONE`になった提出 | 1 |
| 複数の行を1つの`UPDATE`で`DONE`にする（期限切れのJEと同じ形） | 対象の行ごとに1 |

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/webhooks/ -run TestWebhookTrigger`
Expected: FAIL（表がない）

- [ ] **Step 3: 移行を書く**

上の3つの表に続けて、トリガーの関数`contest_webhook_enqueue()`と、`AFTER UPDATE ON submissions FOR EACH ROW WHEN (NEW.status='DONE' AND OLD.status IS DISTINCT FROM 'DONE' AND NEW.contest_id IS NOT NULL)`のトリガーを置く。
関数は、Step 1の表の条件をすべて満たすときだけ`contest_webhook_events(contest_id, submission_id)`へ1行入れる。
開催中の判定は`NEW.created_at>=c.starts_at AND NEW.created_at<c.ends_at`、参加者の判定は`contest_competitor(c, NEW.owner_id)`を使う。

通知の2つの`CHECK`（`notifications_kind_check`と`notifications_subject`）を、`043_rejudge_notifications.sql`の今の定義に`contest_webhook_stopped`を足した形で作り直す。
`contest_webhook_stopped`は`contest_results`と同じく、`contest_id`を持ち`problem_id`を持たない側に入れる。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd api && go test ./internal/webhooks/ ./internal/contests/ ./internal/database/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/database/048_contest_webhooks.sql api/internal/contests/drafts.go api/internal/webhooks/trigger_test.go
git commit -m "feat: コンテストのWebhookの表と、判定の確定を記録するトリガーを加える"
```

---

### Task 2: 送り先のURLとアドレスの検査

**Files:**
- Create: `api/internal/webhooks/address.go`
- Test: `api/internal/webhooks/address_test.go`

**Interfaces:**
- Produces:

```go
var (
    ErrInvalidURL  = errors.New("invalid webhook url")       // 形が条件を満たさない
    ErrNotPublic   = errors.New("webhook url is not public") // 公開のインターネットでないアドレス
    ErrUnresolved  = errors.New("webhook url does not resolve")
)

// ValidURL checks the form only: scheme, port, userinfo, length and, for discord, host and path.
func ValidURL(raw, format string) (*url.URL, error)

// PublicAddr reports whether a is an address on the public internet.
func PublicAddr(a netip.Addr) bool

// Resolver is net.Resolver's LookupNetIP, replaceable in tests.
type Resolver interface {
    LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// CheckHost resolves the host and fails with ErrNotPublic if any address is not public, or ErrUnresolved if none.
// A literal IP address is checked without a lookup.
func CheckHost(ctx context.Context, r Resolver, host string) error
```

- [ ] **Step 1: 失敗するテストを書く**

`TestValidURL`：

| 入力 | format | 結果 |
| --- | --- | --- |
| `https://example.com/hook` | json | 通る |
| `https://example.com:443/hook?x=1` | json | 通る |
| `http://example.com/hook` | json | `ErrInvalidURL` |
| `https://example.com:8443/hook` | json | `ErrInvalidURL` |
| `https://user:pass@example.com/` | json | `ErrInvalidURL` |
| `https://example.com/#frag` | json | `ErrInvalidURL` |
| `https://` + 2,000文字を超える長さ | json | `ErrInvalidURL` |
| `https://discord.com/api/webhooks/1/abc` | discord | 通る |
| `https://discord.com.evil.example/api/webhooks/1/abc` | discord | `ErrInvalidURL` |
| `https://discord.com/api/other` | discord | `ErrInvalidURL` |
| `https://example.com/hook` | discord | `ErrInvalidURL` |

`TestPublicAddr`：偽になるのは`127.0.0.1`、`10.0.0.1`、`172.16.0.1`、`192.168.1.1`、`169.254.169.254`、`100.64.0.1`、`0.0.0.0`、`224.0.0.1`、`::1`、`::`、`fe80::1`、`fd00::1`、`ff02::1`、`::ffff:10.0.0.1`、`64:ff9b::a00:1`。
真になるのは`93.184.216.34`、`2606:4700::1111`。

`TestCheckHost`：偽の`Resolver`で、公開だけを返すホストは通る、公開と`10.0.0.1`を両方返すホストは`ErrNotPublic`、何も返さないホストは`ErrUnresolved`、`[::1]`と`127.0.0.1`は引かずに`ErrNotPublic`。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/webhooks/ -run 'TestValidURL|TestPublicAddr|TestCheckHost'`
Expected: FAIL（未定義）

- [ ] **Step 3: `address.go`を実装する**

`PublicAddr`は、`a.Unmap()`した上で`IsGlobalUnicast()`が真、かつ`IsPrivate()`、`IsLoopback()`、`IsLinkLocalUnicast()`が偽であることを見る。
これらで落ちない`100.64.0.0/10`、`192.0.0.0/24`、`198.18.0.0/15`、`240.0.0.0/4`、`64:ff9b::/96`、`2001:db8::/32`は、`netip.Prefix`の表で拒む。

- [ ] **Step 4: テストが通ることを確かめる**

Run: 同じコマンド
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/webhooks/address.go api/internal/webhooks/address_test.go
git commit -m "feat: Webhookの送り先のURLとアドレスを検査する"
```

---

### Task 3: 出来事に載せる順位表の行を集計する

**Files:**
- Modify: `api/internal/contests/standings.go`（`Standings`の読み出し部分を切り出して共有する。`Attempt`に`ID string`を足す）
- Create: `api/internal/contests/standings_event.go`
- Test: `api/internal/contests/standings_event_test.go`

**Interfaces:**
- Produces:

```go
// EventRow is the row of a submission's team or participant as a webhook carries it.
type EventRow struct {
    Rank       int     // 全体の順位
    OnsiteRank *int    // オンサイト区分の中の順位。オンサイトでない行と、区分のないコンテストではnil
    Onsite     bool
    Team       *TeamSummary
    Score      int64   // 合計。100分の1点
    TimeMS     int64
    Problem    Score   // 提出の問題のセル
}

// EventRows is what one judged submission adds to a webhook event.
type EventRows struct {
    Live         EventRow // 凍結しない順位表での行
    Public       EventRow // 参加者に見えている順位表での行。凍結中でなければLiveと同じ
    PreviousRank int      // この提出を除いて集計したLiveの全体の順位
    Frozen       bool     // 凍結後の提出で、結果確定していない
}

// EventContest is the contest's part of an event.
type EventContest struct {
    ID, Title      string
    OnsiteDivision bool
}

// EventStandings ranks the contest once per call and returns the rows for each of the submissions, keyed by
// submission id. A submission whose submitter has no row (no longer a competitor, or not in the contest) is left out.
func (s *Store) EventStandings(ctx context.Context, contestID string, submissionIDs []string) (EventContest, map[string]EventRows, error)
```

- [ ] **Step 1: 失敗するテストを書く**

`RankRows`を直接使う単体テスト（データベースなし）と、`EventStandings`の統合テストを置く。

- `TestEventRowsPreviousRank`：3人が各1問AC済みで、4人目がACして1位になる。その提出の`PreviousRank`は4、`Live.Rank`は1。
- `TestEventRowsWrongAnswer`：WAの提出では`PreviousRank == Live.Rank`、`Live.Problem.Wrong`が1増える。
- `TestEventRowsFrozen`：凍結を行うコンテストで、凍結後のAC。`Frozen`は真、`Live.Rank`は1、`Public.Rank`は凍結前の順位のまま、`Public.Problem.Frozen`は1。
- `TestEventRowsPublicDuringFreeze`：凍結前に出した提出（`Frozen`は偽）でも、ほかの人の凍結後のACがあるあいだは`Live.Rank`と`Public.Rank`が違う。
- `TestEventRowsAfterFinal`：結果確定後は`Frozen`が偽で、`Public`が`Live`と等しい。
- `TestEventRowsOnsite`：オンサイト区分のあるコンテストで、オンサイトの行は`OnsiteRank`が区分の中の順位、そうでない行は`nil`。区分のないコンテストでは全員`nil`。
- `TestEventRowsTeam`：チームのメンバーの提出は、`Team`に名前とメンバーを持つチームの行になる。
- `TestEventRowsMissing`：スタッフになった人の提出IDは、返る`map`に入らない。
- `TestEventRowsPartial`：部分点の問題で、`Problem.Score`が群ごとの最高の合計になる。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/contests/ -run TestEventRows`
Expected: FAIL（未定義）

- [ ] **Step 3: 実装する**

`Standings`から「参加者の行と提出を読む」部分を、閲覧者を取らない関数へ切り出す。
`EventStandings`はそれを1回呼び、提出ごとに次を求める。データベースは提出の数だけ読まない。

- `Live`：`RankRows(entries, attempts, start, penalty, time.Time{})`の中の、その提出者の行。
- `Public`：凍結中（`FreezeAt`があり結果確定していない）なら`RankRows(..., freezeAt)`の中の行、そうでなければ`Live`。
- `PreviousRank`：その提出を除いた`attempts`での`RankRows(..., time.Time{})`の行の順位。
- `OnsiteRank`：`OnsiteRows`を通した表での`Rank`。

`Standings`の今の結果は変えない。既存の`standings_test.go`と`scoring_test.go`がそのまま通ることを確かめる。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd api && go test ./internal/contests/`
Expected: PASS

- [ ] **Step 5: 集計時間を測る**

`BenchmarkEventStandings`を足す：参加者300人、提出3,000件、出来事1件。
Run: `cd api && go test ./internal/contests/ -bench EventStandings -run '^$'`
1件あたりの時間を控える（Task 12で設計書に書く）。50ミリ秒を超えたら、`PreviousRank`のための並べ直しを見直してから先へ進む。

- [ ] **Step 6: コミット**

```bash
git add api/internal/contests/standings.go api/internal/contests/standings_event.go api/internal/contests/standings_event_test.go
git commit -m "feat: Webhookの出来事に載せる順位表の行を集計する"
```

---

### Task 4: JSON形式とDiscord形式の本文を作る

**Files:**
- Create: `api/internal/webhooks/payload.go`
- Test: `api/internal/webhooks/payload_test.go`

**Interfaces:**
- Consumes: `contests.EventRows`、`contests.EventRow`、`contests.EventContest`、`contests.ScoreText`（Task 3）
- Produces:

```go
// Judged is one judged submission, ready to be written in either format.
type Judged struct {
    EventID     string    // "evt_" + 出来事ごとに一意の値
    SiteURL     string    // 公開サイトの起点。末尾に/を付けない
    Contest     contests.EventContest
    SubmissionID string
    Handle      string
    ProblemLabel, ProblemTitle string // ラベルはコンテスト内の記号（A、B、…）
    Verdict     string
    Score, FullScore int64 // 100分の1点
    SubmittedAt, JudgedAt time.Time
    Rejudge     bool
    Rows        contests.EventRows
}

func ShortID(uuid string) string                    // web/shared/utils/short-id.tsのshortIdと同じ結果
func JSONBody(j Judged, sentAt time.Time) []byte    // 設計書「JSON形式の中身」のとおり
func PingBody(c contests.EventContest, siteURL, eventID string, sentAt time.Time) []byte
func DiscordLine(j Judged) string                   // 設計書「Discord形式の中身」のとおり。改行を含みうる
func DiscordPing() string                           // "ShareOJからのテスト送信です"
func Summary(j Judged) string                       // 最近の送信に出す要約。例："A問題 · loop · AC"
func Sign(secret string, timestamp int64, body []byte) string // "sha256=" + hex
```

- [ ] **Step 1: 失敗するテストを書く**

- `TestShortID`：`web/shared/utils/short-id.ts`の`shortId`に実際のUUIDを3つ通した結果（`bun -e`で求めて定数にする）と一致する。UUIDでない文字列はそのまま返る。
- `TestJSONBody`：個人でACの出来事を`json.Unmarshal`し、設計書の例の全項目を見る。`version`は1、`event`は`submission.judged`、`submission.score`と`fullScore`は`"100"`、`standing.rank`、`standing.previousRank`、`standing.onsiteRank`、`standing.team`は`null`、`standing.problem.wrong`、`publicStanding.rank`、`contest.url`は`<SiteURL>/contests/<ShortID>`。`submission.url`は、`web/app/pages/contests/[id]/submissions`の実在するページのURLにする。
- `TestJSONBodyTeam`：`standing.team`が`{"name":…,"members":[…]}`、`by.handle`は提出した本人。
- `TestJSONBodyPartial`：得点37.5点が`"37.50"`。
- `TestJSONBodyFrozen`：`frozen`が`true`で、`submission.verdict`と`standing.rank`は本当の値、`publicStanding.rank`は凍結した順位表の値。
- `TestPingBody`：`event`が`ping`で、`contest`を持ち、`submission`と`standing`を持たない。
- `TestSign`：鍵`whsec_test`、タイムスタンプ`1700000000`、本文`{"a":1}`の署名が、`openssl dgst -sha256 -hmac`で求めた値と一致する。
- `TestDiscordLine`：
  - 個人のAC、順位3位から1位、オンサイト1位：`✅ loop が A問題 を AC（100点）\n順位 3位 → 1位 ／ オンサイト 1位`
  - チームのWA、順位は変わらず5位、誤答2回：`❌ チーム たこやき（taro）が B問題 で WA\n順位 5位 ／ B問題の誤答 2回`
  - `Rows.Frozen`が真：`🧊 凍結中　hana が C問題 に提出`。判定も順位も含まない。
  - 順位は`Rows.Public`から書く：`Live.Rank`が1、`Public.Rank`が4の出来事の行は「4位」を含み「1位」を含まない。
- `TestDiscordEscape`：チーム名`**bold** _x_ ~~s~~ ||sp|| > q @everyone`を含む行に、エスケープされていない`*`、`_`、`~`、`|`、`` ` ``、行頭の`>`がなく、`@everyone`は`@`の後にゼロ幅スペースが入る。300文字のチーム名は100文字で切って`…`を付ける。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/webhooks/ -run 'TestShortID|TestJSONBody|TestPingBody|TestSign|TestDiscord'`
Expected: FAIL（未定義）

- [ ] **Step 3: `payload.go`を実装する**

JSONは項目の順が設計書の例と同じになるよう、構造体で組み立てる。
時刻はUTCのRFC 3339（秒まで）で書く。
Discordの絵文字は、ACが✅、それ以外が❌、凍結後が🧊。

- [ ] **Step 4: テストが通ることを確かめる**

Run: 同じコマンド
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/webhooks/payload.go api/internal/webhooks/payload_test.go
git commit -m "feat: WebhookのJSON形式とDiscord形式の本文を作る"
```

---

### Task 5: 送り先の設定を保存する

**Files:**
- Create: `api/internal/webhooks/store.go`
- Test: `api/internal/webhooks/store_test.go`

**Interfaces:**
- Consumes: `ValidURL`、`CheckHost`、`Resolver`（Task 2）、表（Task 1）
- Produces:

```go
type Store struct {
    Pool     *pgxpool.Pool
    Resolver Resolver // nilならnet.DefaultResolver
    // AllowPrivate skips the address check. Only the development server sets it.
    AllowPrivate bool
}

type Delivery struct {
    ID        string    `json:"id"`
    Ping      bool      `json:"ping"`
    Summary   string    `json:"summary"`
    Status    string    `json:"status"` // pending, delivered, failed
    Detail    string    `json:"detail"` // "204"、"timeout"、"connection"、"not_public"、"rate_limited"、"expired"など
    CreatedAt time.Time `json:"createdAt"`
}

type Webhook struct {
    ID         string     `json:"id"`
    URL        string     `json:"url"`
    Format     string     `json:"format"`
    Verdicts   string     `json:"verdicts"`
    Division   string     `json:"division"`
    Enabled    bool       `json:"enabled"`
    Secret     string     `json:"secret"`
    StoppedAt  *time.Time `json:"stoppedAt,omitempty"` // 自動で止めた時刻
    Deliveries []Delivery `json:"deliveries"`          // 新しい順に20件まで
}

type Settings struct {
    Webhooks       []Webhook `json:"webhooks"`
    OnsiteDivision bool      `json:"onsiteDivision"` // 「オンサイトだけ」を選べるか
    Freeze         bool      `json:"freeze"`         // 凍結を行うコンテストか（凍結の注意を出すため）
}

type Input struct {
    URL, Format, Verdicts, Division string
    Enabled bool // Updateだけが使う
}

var (
    ErrLimit               = errors.New("webhook limit reached")          // 3つある
    ErrInvalid             = errors.New("invalid webhook")                // format、verdicts、divisionの値
    ErrDivisionUnavailable = errors.New("contest has no onsite division")
    ErrPingTooSoon         = errors.New("webhook ping too soon")
)

func (s *Store) Settings(ctx context.Context, actor, contestID string) (Settings, error)
func (s *Store) Create(ctx context.Context, actor, contestID string, in Input) (Webhook, error)
func (s *Store) Update(ctx context.Context, actor, contestID, id string, in Input) (Webhook, error)
func (s *Store) Delete(ctx context.Context, actor, contestID, id string) error
func (s *Store) RotateSecret(ctx context.Context, actor, contestID, id string) (Webhook, error)
// Ping records a pending test delivery and its event, and returns the delivery's id.
func (s *Store) Ping(ctx context.Context, actor, contestID, id string) (string, error)
```

セッターでない`actor`、存在しないコンテスト、そのコンテストにない送り先には、どの関数も`pgx.ErrNoRows`を返す。

- [ ] **Step 1: 失敗するテストを書く**

- `TestSettingsSettersOnly`：オーナーと共同作成者は読める。テスター、作問者、参加者、他人は`pgx.ErrNoRows`。
- `TestCreate`：`secret`は`whsec_`で始まり、2つ作ると違う値になる。`enabled`は真。
- `TestCreateLimit`：4つ目は`ErrLimit`。2つの`Create`を同時に走らせても3つを超えない。
- `TestCreateInvalid`：`format`が`xml`は`ErrInvalid`。`http://`のURLは`ErrInvalidURL`。偽の`Resolver`が`10.0.0.1`を返すホストは`ErrNotPublic`。`AllowPrivate`が真なら通る。
- `TestCreateOnsiteDivision`：オンサイト区分のないコンテストで`division: onsite`は`ErrDivisionUnavailable`。区分のあるコンテストと、区分を設けた下書きでは通る。
- `TestCreateOnDraft`：下書きのコンテストにも作れ、公開した後も同じ送り先が読める。下書きを削除すると送り先も消える。
- `TestUpdateAfterStart`：開始後、終了後、結果確定後のどれでも`Update`が通る。
- `TestUpdateEnableResets`：`failures=20`、`enabled=false`、`stopped_at`ありの送り先を`Enabled: true`で`Update`すると、`failures`が0、`stopped_at`が`NULL`になる。
- `TestRotateSecret`：値が変わる。
- `TestDelete`：送信の記録と、まだ送っていないテスト送信の出来事も消える。
- `TestPing`：`pending`で`ping`が真の記録が1件、`contest_webhook_events`に`delivery_id`を持つ行が1件できる。10秒以内の2回目は`ErrPingTooSoon`。`enabled=false`の送り先にも送れる。
- `TestSettingsDeliveries`：25件の記録があっても、新しい順に20件だけ返る。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/webhooks/ -run 'TestSettings|TestCreate|TestUpdate|TestRotate|TestDelete|TestPing'`
Expected: FAIL（未定義）

- [ ] **Step 3: `store.go`を実装する**

セッターかどうかは`contest_setter($1,$2)`で見る（下書きでも働く）。
オンサイト区分と凍結の有無は、公開後は`contests`から、下書きのあいだは`contest_drafts.draft`から読む。
3つの上限は、`pg_advisory_xact_lock`でコンテストごとに順番を付けてから数える。
`secret`は`"whsec_"`に、160ビットの乱数を`base32`（パディングなし）で書いたものを続ける。
`AllowPrivate`が真のときは、アドレスの検査を外すのに加えて、`http://localhost`と`http://127.0.0.1`（ポートは任意）のURLも通す。
開発用の受け手へ送るためで、それ以外のURLの条件は変えない。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd api && go test ./internal/webhooks/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/webhooks/store.go api/internal/webhooks/store_test.go
git commit -m "feat: Webhookの送り先の設定を保存する"
```

---

### Task 6: 設定のAPIを足す

**Files:**
- Create: `api/internal/httpapi/contest_webhooks.go`
- Modify: `api/internal/httpapi/routes.go`（`/my/contests/{id}/onsite`の近くに経路を足す）
- Modify: `api/internal/httpapi/config.go`（`webhooks.Store`を作って渡す。`WEBHOOKS_ALLOW_PRIVATE=true`のとき`AllowPrivate`を立てる）
- Modify: `api/internal/httpapi/handler.go`か`contests.go`（`contestHandler`に`Webhooks *webhooks.Store`を足す）
- Test: `api/internal/httpapi/contest_webhooks_test.go`

**Interfaces:**
- Consumes: `webhooks.Store`とその誤り（Task 5）、`DispatchJudge func(context.Context) error`（今ある、`judge-bridge`を起こす関数）
- Produces（どれも`private(...)`で登録し、セッター以外には`404 contest_not_found`）:

| 経路 | 本文 | 成功 |
| --- | --- | --- |
| `GET /my/contests/{id}/webhooks` | なし | `200` `Settings` |
| `POST /my/contests/{id}/webhooks` | `{url,format,verdicts,division}` | `201` `Webhook` |
| `PUT /my/contests/{id}/webhooks/{webhook}` | `{url,format,verdicts,division,enabled}` | `200` `Webhook` |
| `DELETE /my/contests/{id}/webhooks/{webhook}` | なし | `204` |
| `PUT /my/contests/{id}/webhooks/{webhook}/secret` | なし | `200` `Webhook` |
| `POST /my/contests/{id}/webhooks/{webhook}/ping` | なし | `202` `{"deliveryId":"…"}` |

誤りの対応：

| 誤り | 応答 |
| --- | --- |
| `webhooks.ErrInvalidURL` | `400 invalid_webhook_url` |
| `webhooks.ErrNotPublic` | `400 webhook_url_not_public` |
| `webhooks.ErrUnresolved` | `400 webhook_url_unresolved` |
| `webhooks.ErrInvalid` | `400 invalid_webhook` |
| `webhooks.ErrDivisionUnavailable` | `400 webhook_division_unavailable` |
| `webhooks.ErrLimit` | `409 webhook_limit` |
| `webhooks.ErrPingTooSoon` | `429 webhook_ping_too_soon` |
| `pgx.ErrNoRows`、UUIDでない`{id}`や`{webhook}` | `404 contest_not_found` |

- [ ] **Step 1: 失敗するテストを書く**

`contest_teams_test.go`の組み立て方にならい、`TestContestWebhooksPostgres`に次を置く。

- オーナーが`POST`して`201`、`GET`で1件返り、`url`と`secret`が入っている。
- 共同作成者も`GET`と`POST`ができる。
- テスター、参加者、ログインしていない人の`GET`は`404`か`401`で、応答の本文に送り先のURLも`whsec_`も含まれない。
- `GET /my/contests/{id}`（今あるコンテストの詳細）の応答に、セッターが見ても送り先のURLと`whsec_`が含まれない（設定は専用の経路だけが返す）。
- 上の誤りの表の各行。
- `ping`の後に`DispatchJudge`が1回呼ばれる。`DispatchJudge`が誤りを返しても`202`を返す。
- 開始後のコンテストでも`PUT`が`200`。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/httpapi/ -run TestContestWebhooksPostgres`
Expected: FAIL（404）

- [ ] **Step 3: ハンドラと経路を実装する**

`venue`ハンドラ（`contests.go`）と同じ形で書く。
本文は`contentJSON`で読み、4 KiBまでとする。
`ping`は`Store.Ping`の後、2秒の期限で`DispatchJudge`を呼ぶ（提出のときの起こし方と同じ）。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd api && go test ./internal/httpapi/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/httpapi/
git commit -m "feat: Webhookの送り先を設定するAPIを加える"
```

---

### Task 7: 出来事から送信を作り、結果を記録する

**Files:**
- Create: `api/internal/webhooks/outbox.go`
- Test: `api/internal/webhooks/outbox_test.go`

**Interfaces:**
- Consumes: `contests.Store.EventStandings`（Task 3）、`JSONBody`、`PingBody`、`DiscordLine`、`DiscordPing`、`Summary`（Task 4）、表（Task 1）
- Produces:

```go
// Message is one delivery as the sender receives it. It carries the URL and the secret, so it must not be logged.
type Message struct {
    DeliveryID string    `json:"deliveryId"`
    WebhookID  string    `json:"webhookId"`
    URL        string    `json:"url"`
    Format     string    `json:"format"`
    Secret     string    `json:"secret,omitempty"`  // jsonのとき
    EventID    string    `json:"eventId"`
    Body       string    `json:"body,omitempty"`    // jsonのとき、送る本文そのもの
    Text       string    `json:"text,omitempty"`    // discordのとき、1件分の文章
    Deadline   time.Time `json:"deadline"`          // 出来事の記録から60秒後
}

// Result is what the sender reports back for one delivery.
type Result struct {
    DeliveryID  string `json:"deliveryId"`
    WebhookID   string `json:"webhookId"`
    Delivered   bool   `json:"delivered"`
    Detail      string `json:"detail"`      // Delivery.Detailに入る値
    RateLimited bool   `json:"rateLimited"` // 429であきらめた。自動停止の数に入れない
}

const (
    EventLifetime   = 60 * time.Second
    StopAfter       = 20
    KeptDeliveries  = 20
)

// Drain turns recorded events into messages until none is left or the context ends. It hands each message to send,
// and removes an event once all its messages are sent. Events older than EventLifetime are removed unsent.
// siteURL is the public site's origin.
func (s *Store) Drain(ctx context.Context, standings *contests.Store, siteURL string, send func(context.Context, Message) error) error

// Record stores a delivery's result, counts consecutive failures and stops the webhook after StopAfter of them,
// notifying the contest's setters. A result for a deleted webhook or delivery does nothing.
func (s *Store) Record(ctx context.Context, r Result) error
```

- [ ] **Step 1: 失敗するテストを書く**

`Drain`（`send`は受け取った`Message`をためる関数）：

- `TestDrainJudged`：JSON形式とDiscord形式の送り先を1つずつ持つコンテストでACの出来事1件。`Message`が2通、`pending`の記録が2件でき、出来事の行が消える。JSONの`Body`の`submission.verdict`は`AC`、Discordの`Text`は`✅`で始まる。`Deadline`は出来事の`created_at`の60秒後。
- `TestDrainFilters`：`verdicts: ac`の送り先にWAは届かず、記録も作らない。`division: onsite`の送り先にオンサイトでない行の出来事は届かない。
- `TestDrainDiscordRejudge`：リジャッジ済みの提出（`rejudged_at`あり）の出来事は、JSONには`rejudge: true`で届き、Discordには届かない。
- `TestDrainDiscordFrozenAcOnly`：凍結後の提出は、Discordの`verdicts: all`には🧊の文で届き、Discordの`verdicts: ac`には届かない。JSONの`verdicts: ac`には、ACなら`frozen: true`で届く。
- `TestDrainDisabled`：`enabled=false`の送り先には届かない。
- `TestDrainPing`：テスト送信の出来事は、`Task 5`の`Ping`が作った記録のIDを`DeliveryID`に持つ`Message`1通になる。JSONは`event: ping`、Discordは`ShareOJからのテスト送信です`。止めてある送り先にも届く。
- `TestDrainStale`：61秒前の出来事は`send`を呼ばずに消える。テスト送信なら、その記録は`failed`、`detail`は`expired`になる。
- `TestDrainMissingRow`：提出者がスタッフになった出来事、送り先がすべて消された出来事は、`send`を呼ばずに消え、誤りを返さない。
- `TestDrainBatch`：同じコンテストの出来事100件で、`EventStandings`の読み出しがコンテストごとに1回になる（1回の読み出しで扱う出来事は100件まで）。
- `TestDrainSendFails`：`send`が誤りを返した出来事は残り、次の`Drain`でもう一度送られる。そのとき記録は重複しない。
- `TestDrainDeadline`：期限の切れた`ctx`では、何もせずに`nil`を返す。
- `TestDrainConcurrent`：2つの`Drain`を同時に走らせても、同じ出来事の`Message`は1組だけ（`FOR UPDATE SKIP LOCKED`）。

`Record`：

- `TestRecordDelivered`：記録が`delivered`、`detail`が`204`になり、`failures`が0に戻る。
- `TestRecordFailed`：`failures`が1増える。
- `TestRecordRateLimited`：記録は`failed`、`detail`は`rate_limited`で、`failures`は増えない。
- `TestRecordStops`：20件目の失敗で`enabled`が偽、`stopped_at`が入り、オーナーと共同作成者のそれぞれに`contest_webhook_stopped`の通知が1件できる。通知の`actor_id`はコンテストのオーナー。21件目の失敗では通知が増えない。
- `TestRecordPingNotCounted`：テスト送信の失敗は`failures`を増やさない。
- `TestRecordPrunes`：21件目の記録が結果を得ると、その送り先の記録は新しい20件になる。
- `TestRecordGone`：消された送り先や記録への結果は`nil`を返す。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/webhooks/ -run 'TestDrain|TestRecord'`
Expected: FAIL（未定義）

- [ ] **Step 3: `outbox.go`を実装する**

`Drain`は、1回のトランザクションで出来事を古い順に100件まで`FOR UPDATE SKIP LOCKED`で取り、コンテストごとにまとめて`EventStandings`を1回呼ぶ。
提出の判定、得点、時刻、問題の記号と題は`submissions`と`contest_problems`から読む。
`EventID`は`"evt_"`に続けて、出来事のIDと送り先によらない値にする（同じ出来事はどの送り先でも同じ`id`）。
記録の`INSERT`と出来事の`DELETE`を同じトランザクションで行い、`send`がすべて通ってから確定する。

`Record`は、送り先の行を`FOR UPDATE`で取ってから数える。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd api && go test ./internal/webhooks/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/webhooks/outbox.go api/internal/webhooks/outbox_test.go
git commit -m "feat: Webhookの出来事から送信を作り、結果を記録する"
```

---

### Task 8: 送信用のLambdaを作る

**Files:**
- Create: `api/internal/webhooks/sender.go`
- Create: `api/cmd/webhook-sender/main.go`
- Modify: `api/Makefile`（`package-judge`に`webhook-sender.zip`を足す。`judge-bridge`と同じビルド）
- Test: `api/internal/webhooks/sender_test.go`、`api/cmd/webhook-sender/main_test.go`

**Interfaces:**
- Consumes: `Message`、`Result`（Task 7）、`PublicAddr`（Task 2）、`Sign`（Task 4）
- Produces:

```go
// Sender posts messages. Its zero value is not usable; use NewSender.
type Sender struct {
    // AllowPrivate lets the development server reach localhost over http. Never set in a deployed sender.
    AllowPrivate bool
    Now   func() time.Time             // テスト用
    Sleep func(context.Context, time.Duration) error // テスト用
}

func NewSender() *Sender

// Send delivers the messages of one webhook in order and returns a result for each. Discord messages waiting
// together are merged into posts of at most 2,000 characters. It never returns the URL or the secret in a result.
func (s *Sender) Send(ctx context.Context, messages []Message) []Result
```

`webhook-sender`の環境変数：`WEBHOOK_RESULT_QUEUE_URL`。
SQSの出来事（FIFO、`MessageGroupId`は送り先のID）を受け、送り先ごとにまとめて`Send`を呼び、`Result`を1件ずつ結果のキューへ`SendMessage`する。

- [ ] **Step 1: 失敗するテストを書く**

`httptest.NewTLSServer`を相手にする。`Sender`の中のHTTPクライアントを、テストのサーバーの証明書を信じ、アドレスの検査を外した形に差し替えられるようにしておく。

- `TestSendJSON`：`POST`が1回届き、`Content-Type`、`User-Agent`、`X-ShareOJ-Event-Id`、`X-ShareOJ-Timestamp`が付き、`X-ShareOJ-Signature`が`Sign(secret, timestamp, body)`と一致する。結果は`Delivered: true`、`Detail: "204"`。
- `TestSendDiscord`：本文が`{"content":"…","allowed_mentions":{"parse":[]}}`で、署名のヘッダーがない。
- `TestSendDiscordMerges`：同じ送り先の3通は1回の`POST`になり、`content`は3件を改行でつないだもの。3通とも`Delivered: true`。
- `TestSendDiscordSplits`：合計が2,000文字を超える分は次の`POST`に回る。1件だけで2,000文字を超える文は2,000文字で切る。
- `TestSendRetries`：500を2回返してから204を返す相手に、3回目で届く。待ちは3秒、12秒（`Sleep`の引数で確かめる）。
- `TestSendGivesUp`：500を返し続ける相手には3回で終わり、`Delivered: false`、`Detail: "500"`。
- `TestSendDeadline`：`Deadline`を過ぎた`Message`は送らず、`Detail: "expired"`。次の待ちが`Deadline`を越えるなら、待たずにあきらめる。
- `TestSendRateLimited`：429と`Retry-After: 2`には2秒待って送り直す。Discordの`{"retry_after":1.5}`にも従う。待ちが`Deadline`を越えるなら`RateLimited: true`、`Detail: "rate_limited"`。
- `TestSendTimeout`：ヘッダーを返さない相手、本文を1バイトずつ返し続ける相手のどちらも、5秒で打ち切って`Detail: "timeout"`。
- `TestSendNoRedirect`：302を返す相手には従わず、`Detail: "302"`で失敗。
- `TestSendNotPublic`：差し替えていない`Sender`で`https://127.0.0.1/`、`https://[::ffff:10.0.0.1]/`、`https://169.254.169.254/`は、接続せずに`Detail: "not_public"`。名前が`127.0.0.1`を引くホストも同じ（接続の直前の検査で拒む）。
- `TestSendPort`：`https://example.com:8443/`は送らず`Detail: "invalid_url"`。
- `TestSendErrorsHideURL`：存在しないホストへの失敗で、どの`Result`の`Detail`にもURLと鍵が含まれない。`Detail`は`connection`。
- `TestHandlerGroups`（`main_test.go`）：2つの送り先の`Message`が混ざったSQSの出来事で、送り先ごとに`Send`が呼ばれ、`Result`が`Message`の数だけ結果のキューへ送られる。ログに出した文字列に`https://`も`whsec_`も含まれない。壊れた本文の記録は捨てて、ほかを続ける。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/webhooks/ -run TestSend && go test ./cmd/webhook-sender/`
Expected: FAIL（未定義）

- [ ] **Step 3: 実装する**

アドレスの検査は`net.Dialer`の`Control`で行う。
名前を引いた後の、実際につなぐアドレスを見ることになるので、引き先が後から変わっても内部へ届かない。
`Control`は、アドレスが`PublicAddr`でないかポートが443でなければ誤りを返す。

HTTPクライアントは、`CheckRedirect`で`http.ErrUseLastResponse`を返し、`Timeout`を5秒にする。
応答の本文は4 KiBまで読んで捨てる（429のときだけ`retry_after`を読む）。
プロキシの環境変数は使わない。

`main.go`は、送り先ごとの`Send`を並行に動かす。
1つの送り先が送り直しで60秒近くかかっても、同じまとまりのほかの送り先を待たせず、Lambdaのタイムアウト（90秒）に収めるためである。
`main.go`は、失敗した記録をSQSへ返さない（送り直しは`Send`の中で済んでいる）。
結果のキューへの送信が失敗したときは、記録のIDだけをログに出す。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd api && go test ./internal/webhooks/ ./cmd/webhook-sender/ && make -C api package-judge && ls api/.build/webhook-sender.zip`
Expected: PASS、zipができる

- [ ] **Step 5: コミット**

```bash
git add api/internal/webhooks/sender.go api/internal/webhooks/sender_test.go api/cmd/webhook-sender api/Makefile
git commit -m "feat: Webhookを送るLambdaを加える"
```

---

### Task 9: `judge-bridge`と開発用サーバーにつなぐ

**Files:**
- Create: `api/cmd/judge-bridge/webhooks.go`
- Modify: `api/cmd/judge-bridge/main.go`（環境変数を読む）、`bridge.go`（`handle`で結果のキューの記録を振り分け、`results`と`dispatch`の後に`webhooks`を呼ぶ）
- Modify: `api/cmd/api/main.go`（開発用の送信）
- Test: `api/cmd/judge-bridge/webhooks_test.go`

**Interfaces:**
- Consumes: `webhooks.Store.Drain`、`Record`、`Message`、`Result`（Task 7）、`webhooks.Sender`（Task 8）
- Produces:

`judge-bridge`の環境変数：

| 名前 | 内容 |
| --- | --- |
| `WEBHOOK_SEND_QUEUE_URL` | 送信のFIFOキュー。空ならWebhookの処理をしない |
| `WEBHOOK_RESULT_QUEUE_ARN` | 結果のキュー。`handle`が`eventSourceARN`で見分ける |
| `PUBLIC_SITE_URL` | 公開サイトの起点（例：`https://shareoj.example`） |

```go
// webhooks drains the webhook events for at most five seconds. It never fails the invocation.
func (b bridge) webhooks(ctx context.Context)

// webhookResults records the sender's results and reports the records that failed, for SQS to redeliver.
func (b bridge) webhookResults(ctx context.Context, event events.SQSEvent) events.SQSEventResponse
```

FIFOキューへ送るときは、`MessageGroupId`に送り先のID、`MessageDeduplicationId`に送信の記録のIDを使う。

開発用サーバー（`api/cmd/api`）は、`WEBHOOKS_LOCAL=true`のとき、1秒ごとに`Drain`を呼び、`send`の中で`Sender{AllowPrivate: true}.Send`と`Record`を続けて行う。
`AllowPrivate`が真の`Sender`は、`http://localhost`と`http://127.0.0.1`へも送る。

- [ ] **Step 1: 失敗するテストを書く**

今ある`dispatch_test.go`の偽のSQSにならう。

- `TestWebhooksSendsToFIFO`：ACの出来事1件とJSONの送り先1つで、`SendMessage`が1回呼ばれ、`MessageGroupId`が送り先のID、`MessageDeduplicationId`が記録のID、本文が`webhooks.Message`として読める。
- `TestWebhooksAfterResult`：採点結果の記録を`handle`に渡すと、提出が`DONE`になり、同じ起動の中でWebhookの`SendMessage`まで行われる。
- `TestWebhooksUnconfigured`：`WEBHOOK_SEND_QUEUE_URL`が空なら`SendMessage`を呼ばず、採点結果の反映は今までどおり通る。
- `TestWebhooksNeverFailJudging`：`SendMessage`が誤りを返しても、`results`の応答に失敗の記録が入らず、提出は`DONE`のまま。
- `TestWebhooksBudget`：`SendMessage`が1回1秒かかるとき、出来事が100件あっても`webhooks`は5秒ほどで戻り、残りは次の起動に回る。
- `TestHandleRoutesWebhookResults`：`eventSourceARN`が`WEBHOOK_RESULT_QUEUE_ARN`の記録は`webhookResults`へ行き、送信の記録が`delivered`になる。採点結果のキューの記録は今までどおり`results`へ行く。
- `TestWebhookResultsBadRecord`：壊れた本文の記録は失敗として報告せずに捨てる。データベースの誤りは失敗として報告する。
- `TestWebhooksLogsHideSecrets`：上のテストで出たログに`https://`も`whsec_`も含まれない。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./cmd/judge-bridge/ -run 'TestWebhook|TestHandleRoutes'`
Expected: FAIL（未定義）

- [ ] **Step 3: 実装する**

`webhooks`は`context.WithTimeout(ctx, 5*time.Second)`で`Drain`を呼び、誤りは`observe("failure", "platform", "webhook_drain_failed", "", "")`で記録するだけにする。
`handle`は、`results`の後と`dispatch`の後の両方で`webhooks`を呼ぶ。
結果のキューと採点結果のキューの記録が1回の起動に混ざることはない（別の対応付けなので）。先頭の記録の`EventSourceARN`で振り分ける。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd api && go test ./cmd/judge-bridge/ ./cmd/api/ && go vet ./...`
Expected: PASS

- [ ] **Step 5: 開発用サーバーで通しで確かめる**

`make dev`を`WEBHOOKS_LOCAL=true WEBHOOKS_ALLOW_PRIVATE=true`で立ち上げ、スクラッチパッドに置いた小さな受け手（`POST`の本文とヘッダーを表示するだけのもの）を`http://localhost:<port>/`で動かす。
`scripts/dev.mjs`が環境変数をAPIへ渡す形を確かめ、必要ならルートの`.env`の例に2つの変数を足す。
`curl`で送り先を作り、テスト送信が受け手に届くこと、署名がTask 12のサンプルコードの手順で確かめられることを見る。
Expected: 受け手に`event: ping`が1通届き、`GET …/webhooks`の記録が`delivered`になる。

- [ ] **Step 6: コミット**

```bash
git add api/cmd/judge-bridge api/cmd/api
git commit -m "feat: 判定の確定からWebhookの送信までをつなぐ"
```

---

### Task 10: キューとLambdaをTerraformに足す

**Files:**
- Create: `infra/judge/webhooks.tf`
- Modify: `infra/judge/bridge.tf`（権限と環境変数）、`infra/judge/variables.tf`、`infra/judge/outputs.tf`、`infra/judge/terraform.tfvars.example`
- Modify: `infra/judge/tests/judge.tftest.hcl`
- Modify: `.github/workflows/ci.yml:483`（`touch`するzipに`api/.build/webhook-sender.zip`を足す）

**Interfaces:**
- Consumes: `api/.build/webhook-sender.zip`（Task 8）、`judge-bridge`の環境変数（Task 9）
- Produces:

| 資源 | 設定 |
| --- | --- |
| `aws_sqs_queue.webhook_sends` | `${local.name}-webhook-sends.fifo`、FIFO、`sqs_managed_sse_enabled`、保持300秒、可視性タイムアウト180秒、2回受け取られたら`…-webhook-sends-dead.fifo`へ |
| `aws_sqs_queue.webhook_results` | `${local.name}-webhook-results`、保持1日、可視性タイムアウト720秒、3回で`…-webhook-results-dead`へ |
| `aws_lambda_function.webhook_sender` | `${local.name}-webhook-sender`、`provided.al2023`、`arm64`、メモリ128、タイムアウト90秒、同時実行の予約5、**`vpc_config`なし** |
| `aws_lambda_event_source_mapping`（送信） | `webhook_sends`から`webhook_sender`へ、`batch_size = 10` |
| `aws_lambda_event_source_mapping`（結果） | `webhook_results`から`bridge`へ、`batch_size = 10`、`ReportBatchItemFailures` |

変数：`sender_package_path`（既定値`../../api/.build/webhook-sender.zip`、`bridge_package_path`の既定値の形に合わせる）、`public_site_url`。

権限：

- `webhook_sender`の役割：自分のロググループへの書き込み、`webhook_sends`の`ReceiveMessage`、`DeleteMessage`、`GetQueueAttributes`、`webhook_results`の`SendMessage`。それ以外は付けない。
- `bridge`の役割に足す：`webhook_sends`の`SendMessage`、`webhook_results`の`ReceiveMessage`、`DeleteMessage`、`GetQueueAttributes`。

- [ ] **Step 1: 失敗するテストを書く**

`judge.tftest.hcl`に`run`を足す。

- `webhook_sender`に`vpc_config`がない。
- `webhook_sends`の名前が`.fifo`で終わり、`fifo_queue`が真。
- `bridge`の環境変数に`WEBHOOK_SEND_QUEUE_URL`、`WEBHOOK_RESULT_QUEUE_ARN`、`PUBLIC_SITE_URL`がある。
- `webhook_sender`の環境変数は`WEBHOOK_RESULT_QUEUE_URL`だけで、データベースの値を持たない。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `mkdir -p api/.build && touch api/.build/webhook-sender.zip && terraform -chdir=infra/judge init -backend=false -input=false -lockfile=readonly && terraform -chdir=infra/judge test`
Expected: FAIL（資源がない）

- [ ] **Step 3: Terraformを書く**

`observability.tf`の`notify`（VPCの外のLambda）と、`storage.tf`のキューの書き方に合わせる。
ロググループの保持日数は`notify`と同じにする。

- [ ] **Step 4: テストと整形を確かめる**

Run: `terraform fmt -check -recursive infra && terraform -chdir=infra/judge validate && terraform -chdir=infra/judge test`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add infra/judge .github/workflows/ci.yml
git commit -m "feat: Webhookの送信用のキューとLambdaを加える"
```

---

### Task 11: 編集画面に「外部連携」を足す

**Files:**
- Create: `web/shared/types/contest-webhook.ts`
- Create: `web/server/api/my/contests/[id]/webhooks/index.ts`（`GET`、`POST`）、`[webhook]/index.ts`（`PUT`、`DELETE`）、`[webhook]/secret.put.ts`、`[webhook]/ping.post.ts`
- Create: `web/app/components/ContestWebhooks.vue`
- Modify: `web/app/components/ContestEditor.vue`（`Section`に`'integrations'`を足す。並びは「参加形式」と「コンテスト管理」のあいだ。開始後も開ける）
- Modify: `web/shared/types/notification.ts`（種類`contest_webhook_stopped`）と、ベルの表示
- Test: `web/tests/contest-webhooks.spec.ts`

**Interfaces:**
- Consumes: Task 6の経路と誤りの符号
- Produces: `ContestWebhooks.vue`は`contestId: string`を受け取り、自分で`/api/my/contests/{id}/webhooks`を読み書きする。`ContestEditor.vue`の保存（下書きの保存、公開）とは独立に、操作のたびに保存する。

画面の文言：

| 場所 | 文言 |
| --- | --- |
| メニューと見出し | 外部連携 |
| 見出しの下 | 提出の判定が確定するたびに、指定したURLへ送ります。送り先は3つまで登録できます。 |
| 文書へのリンク | Webhookの仕様（`/docs/webhooks`、新しいタブ） |
| 追加のボタン | 送り先を追加 |
| 上限のとき | 送り先は3つまでです。 |
| スイッチ | この送り先へ送る |
| 入力欄 | 送り先のURL、形式（JSON、Discord）、判定（すべての判定、ACだけ）、区分（全員、オンサイトだけ）、署名の鍵 |
| 形式の説明 | JSON：すべての情報を載せて送ります。自作のプログラムで受け取るときに選びます。／Discord：DiscordのWebhook URLにそのまま流せる文章にして送ります。 |
| ボタン | 保存、表示、隠す、作り直す、テスト送信、削除 |
| 自動で止まったとき | 失敗が20件続いたため、この送り先を止めました。送り先を確かめてから、スイッチを入れ直してください。 |
| 凍結の注意 | このコンテストは順位表を凍結します。`frozen`が真の出来事には本当の判定と順位が入っています。参加者に見える場所へ出さないでください。 |
| 最近の送信 | 見出し「最近の送信」。状態は「送信中」「届いた（204）」「失敗（404）」「失敗（時間切れ）」など。記録がないときは「まだ送信していません。」 |

誤りの文言：

| 符号 | 文言 |
| --- | --- |
| `invalid_webhook_url` | URLは`https://`で始まる2,000文字以内のものを入力してください。Discord形式では、DiscordのWebhook URLを入力してください。 |
| `webhook_url_not_public` | このURLには送れません。インターネットから届くURLを入力してください。 |
| `webhook_url_unresolved` | このURLのホストが見つかりません。 |
| `webhook_limit` | 送り先は3つまでです。 |
| `webhook_ping_too_soon` | テスト送信は10秒に1回までです。 |
| `webhook_division_unavailable` | オンサイト区分のないコンテストでは「オンサイトだけ」を選べません。 |

`detail`の表示：数字は「（数字）」、`timeout`は「時間切れ」、`connection`は「つながらない」、`not_public`と`invalid_url`は「送れないURL」、`rate_limited`は「相手の制限」、`expired`は「期限切れ」。

- [ ] **Step 1: 失敗するテストを書く**

`contest-setters.spec.ts`の組み立て方（下書きを作り、編集画面を開く）にならう。

- 「外部連携」を開き、URLを入れて追加すると一覧に1件出て、再読み込みしても残る。
- URLと署名の鍵は伏せて出て、「表示」で見える。
- 3つ追加すると「送り先を追加」が押せなくなり、「送り先は3つまでです。」が出る。
- `http://example.com`を入れると`invalid_webhook_url`の文言が出て、追加されない。
- Discord形式を選ぶと署名の鍵の欄が出ない。
- オンサイト区分のない下書きでは「オンサイトだけ」を選べない。
- 「作り直す」で署名の鍵が変わる。
- 「削除」で一覧から消える。
- 「テスト送信」を押すと「最近の送信」に「送信中」が出る。APIの応答を`page.route`で差し替え、`delivered`にすると「届いた（204）」に変わる。
- 開始後のコンテスト（`canEdit`が偽）でも「外部連携」を開けて、URLを変えて保存できる。ほかの区分（問題・配点など）は今までどおり出ない。
- 凍結を行うコンテストでJSON形式の送り先があるとき、凍結の注意が出る。Discord形式だけなら出ない。
- `stoppedAt`のある送り先に、自動で止まった旨が出る。
- ベルに`contest_webhook_stopped`の通知が「『（コンテスト名）』のWebhookの送り先を、失敗が続いたため止めました」と出て、名前（通知の主）は出ない。クリックで、そのコンテストの編集画面の「外部連携」が開く。
- 幅390pxで、入力欄とボタンが横にはみ出さない。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd web && bun run build && bun --bun playwright test tests/contest-webhooks.spec.ts --output=<scratchpad>/pw`
Expected: FAIL（「外部連携」がない）

- [ ] **Step 3: 型、中継、部品を実装する**

- 中継は`onsite.ts`と同じく`contestAction`を使う。`{webhook}`は`contestParam(event, 'webhook', uuidPattern)`で確かめる。本文の上限は4 KiB。
- 部品は今ある`InlineSwitch`と`settings-section`のスタイルを使う。新しい色や影を足さない。
- テスト送信の後は、その記録が`pending`でなくなるまで、1秒おきに最大15回`GET`する。
- `ContestEditor.vue`は、`route.query.section === 'integrations'`でこの区分を最初に開く。開始後（`settersOnly`）のメニューは「外部連携」と「コンテスト管理」の2つを出す。
- 下書きをまだ1度も保存していない（IDが確定していない）あいだは、「下書きを保存すると設定できます。」とだけ出す。

- [ ] **Step 4: テストと型検査が通ることを確かめる**

Run: `cd web && bun run typecheck && bun run build && bun --bun playwright test tests/contest-webhooks.spec.ts tests/contest-setters.spec.ts tests/contest-editor-form.spec.ts tests/contest-notifications.spec.ts --output=<scratchpad>/pw`
Expected: PASS

- [ ] **Step 5: 実際の画面を見る**

`make dev`で立ち上げ、パソコンの幅とスマホの幅で「外部連携」を開く。
Task 9の受け手へテスト送信し、「届いた」になるまでを画面で確かめる。
ライトとダークの両方で、伏せた欄と状態の色が読めることを見る。

- [ ] **Step 6: コミット**

```bash
git add web/shared/types web/server/api/my/contests web/app/components web/tests/contest-webhooks.spec.ts
git commit -m "feat: コンテスト編集に外部連携の区分を加え、Webhookの送り先を設定できるようにする"
```

---

### Task 12: 仕様の文書、規約、ADRを書く

**Files:**
- Create: `web/app/content/docs/webhooks.md`（ほかの文書と同じ方法で一覧に登録する。`hosting-contests.md`の登録のされ方を見る）
- Modify: `web/app/content/docs/hosting-contests.md`（Webhookの節と、`/docs/webhooks`へのリンク）
- Modify: `web/app/content/docs/contest-rules.md`、`web/app/pages/privacy.vue`
- Create: `docs/adr/0038-send-contest-webhooks.md`、Modify: `docs/adr/README.md`
- Modify: `docs/superpowers/specs/2026-11-13-contest-webhooks-design.md`
- Test: `web/tests/guides.spec.ts`（今ある文書のテストに`/docs/webhooks`を足す）、`api/internal/webhooks/docs_test.go`

**Interfaces:**
- Consumes: `Sign`、`JSONBody`（Task 4）

- [ ] **Step 1: 失敗するテストを書く**

- `docs_test.go`の`TestDocsSignatureExample`：`webhooks.md`に載せる検証のサンプルコード（Node.jsとPython）を、文書のコードブロックから取り出して実行し、`Sign`で作った署名を正しいと判定し、本文を1バイト変えた署名を誤りと判定することを確かめる。
- `docs_test.go`の`TestDocsFieldsMatch`：`webhooks.md`の項目の表にある名前の集合が、`JSONBody`の出力のキー（入れ子は`standing.rank`の形）の集合と一致する。
- `guides.spec.ts`：`/docs/webhooks`が開き、見出し「Webhook」と凍結の注意が見える。`hosting-contests`からのリンクで着く。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/webhooks/ -run TestDocs`
Expected: FAIL（文書がない）

- [ ] **Step 3: `webhooks.md`を書く**

主催者が読んで受け手を作るための文書である。
記憶にある利用者の好み（短く、箇条書きと表を使う）に合わせる。
構成は次のとおりとする。

1. 何が届くか（2、3行）
2. 凍結の注意（ページの上のほうに、目立つ形で）：「`frozen`が真の出来事には本当の判定と順位が入っている。参加者に見える場所へ出すと、順位表の凍結が破れる。参加者に見せる順位は`publicStanding`を使う」
3. 設定の手順
4. JSON形式：例と、全項目の表
5. 署名の確かめ方：手順と、Node.jsとPythonのサンプルコード。タイムスタンプが5分より古い通知を捨てることを勧める。
6. Discord形式：流れる文章の例、リジャッジは流れないこと、凍結後は判定を書かないこと、「ACだけ」では凍結後の提出が流れないこと
7. 届け方の約束：5秒、3回まで、60秒であきらめる、順序は保証しない（`submission.judgedAt`で並べる）、重複しうる（`id`で捨てる）、取り直しはできない、20件続けて失敗すると止まる
8. 送り先のURLの条件
9. 項目は後から増えることがあるので、知らない項目は無視すること

- [ ] **Step 4: 規約とルールに書き足す**

- `privacy.vue`：コンテストの主催者がWebhookを設定したとき、そのコンテストへの提出の提出者のユーザーID、チーム名、問題、判定、得点、順位が、主催者の指定した外部の送り先へ送られることを、第三者への提供にあたる箇所へ足す。ソースコードは送らないことも書く。
- `contest-rules.md`：凍結の節に、主催者はWebhookで凍結後の判定を受け取れること（主催者はもともとスタッフ向けの順位表で見られる）を1文で足す。

この2つは利用者に見せる約束なので、文面をコミットの前に利用者へ見せて確かめる。

- [ ] **Step 5: ADRを書き、設計書を直す**

- `docs/adr/0038-send-contest-webhooks.md`：ほかのADRの形（状態、決定日、範囲、背景、決定、代替案、影響）で書く。代替案には、つなぎっぱなしの方式（SSE）、VPCの中から送る方式、NATゲートウェイを置く方式、判定を確定する処理ごとに記録する方式を入れる。`docs/adr/README.md`の一覧に足す。
- 設計書：状態を「承認済み（2026-11-13）」にする。「再試行と自動停止」の、操作した人のいない通知を作るという段落を、コンテストのオーナーを通知の主にする形へ書き直す。「判定の確定から送信まで」の手順1を、トリガーで記録する形へ書き直す。「負荷」に、Task 3で測った集計時間を書く。

- [ ] **Step 6: テストが通ることを確かめる**

Run: `cd api && go test ./internal/webhooks/ && cd ../web && bun run build && bun --bun playwright test tests/guides.spec.ts tests/content-links.spec.ts --output=<scratchpad>/pw`
Expected: PASS

- [ ] **Step 7: コミット**

```bash
git add web/app/content/docs web/app/pages/privacy.vue web/tests/guides.spec.ts api/internal/webhooks/docs_test.go docs/adr docs/superpowers/specs
git commit -m "docs: Webhookの仕様の文書とADR 0038を加え、プライバシーポリシーに送信先を書く"
```

---

### Task 13: 全体を確かめる

**Files:** なし（確認だけ）

- [ ] **Step 1: すべてのテストを通す**

Run:

```bash
cd api && go vet ./... && go test ./...
cd ../web && bun run typecheck && bun run build && bun --bun playwright test --output=<scratchpad>/pw
terraform fmt -check -recursive infra && terraform -chdir=infra/judge test
```

Expected: PASS。
今ある失敗（記憶にある、あてにならないテスト）は、このブランチの前の`main`でも落ちることを確かめてから報告に書く。

- [ ] **Step 2: 開発用サーバーで、提出からDiscord形式の文章までを通す**

開発用サーバーでコンテストを作り、JSON形式の送り先（Task 9の受け手）を登録する。
参加者として提出し、ACとWAのそれぞれで受け手に1通ずつ届くこと、`standing.rank`と`previousRank`が順位表の画面と合うことを見る。
凍結を行うコンテストで、凍結後の提出が`frozen: true`で届き、`publicStanding`が画面の順位表と合うことを見る。
Discord形式は、受け手を`discord.com`以外に向けられないので、`DiscordLine`の出力を`Drain`のテストで見るにとどめ、実物のDiscordでの確認は配布後に行うと報告に書く。

- [ ] **Step 3: 秘密がログに出ていないことを確かめる**

Step 2のあいだのAPIのログに、送り先のURLと`whsec_`が出ていないことを`grep`で確かめる。

- [ ] **Step 4: グラフを更新し、配布の手順を報告にまとめる**

Run: `graphify update .`

配布はこの計画の外である。報告には、設計書の順（データベースの移行、API、`judge-bridge`と`webhook-sender`とキュー、Web）と、`infra/judge`に足す変数（`sender_package_path`、`public_site_url`）、配布後に確かめること（実物のDiscordへのテスト送信、Discordの受け付ける数の制限の実測、CloudWatchでの`webhook-sender`の誤り）を書く。
