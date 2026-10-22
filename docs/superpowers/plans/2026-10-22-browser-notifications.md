# ブラウザ通知とベルの通知の追加 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 質問、回答、全体公開、結果の公開をベルに出し、開いているShareOJの画面がブラウザ通知（質問と回答、コンテストの開始と終了、定期便の出題と解説公開）を出せるようにする。

**Architecture:** サーバーは既存の`notifications`テーブルに4種類の通知を足し、参加中のコンテストの日時を返すAPIを1つ足すだけにする。ブラウザ通知はすべて画面側で出す。人の操作で起こる出来事はベルの30秒ごとの取得で見つけて通知し、時刻で起こる出来事は画面が取得した予定の時刻に1回だけのタイマーで通知する。

**Tech Stack:** Go（pgx、`api/internal/contests`、`api/internal/httpapi`）、PostgreSQL、Nuxt 4（Vue 3、`useCookie`）、Playwright（`page.clock`）

**Spec:** [docs/adr/0027-notify-open-pages-and-bell.md](../../adr/0027-notify-open-pages-and-bell.md)

## Global Constraints

- サーバーからブラウザへ送る仕組み（Web Push、WebSocket）、新しいLambda、新しいテーブルは追加しない。
- ブラウザ通知を出すのはパソコンのブラウザだけ。`typeof Notification === 'function'`かつユーザーエージェントが`/Android|iPhone|iPad|iPod/i`に当たらないときを対応とする。
- ベルの取得間隔は30秒のまま変えない。
- 通知に質問と回答の本文を入れない。本文にはコンテスト名（定期便は「定期便 vol.N」）だけを入れる。
- 受け取りの設定はCookie `openoj-browser-notifications`に保存する。初期値は受け取らない、種類は4つともオン。
- 種類の名前は`clarifications`（質問と回答）、`contests`（開始と終了）、`featured`（定期便の出題）、`editorials`（定期便の解説公開）。
- 時刻を過ぎて5分を超えたタイマーは通知しない。予定は10分ごとに取り直し、24時間先までの予定だけにタイマーを掛ける。
- 画面の文言は「このブラウザで通知を受け取る」「質問と回答（全体公開を含む）」「コンテストの開始と終了」「定期便の出題」「定期便の解説公開」をそのまま使う。
- コマンドはリポジトリのルートで`nix develop`の中から実行する。Goの統合テストは`make db-up`の後に`TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable'`を付ける。Playwrightは`npm run build`の後に`--output=<scratchpad>/pw`を付ける（`web/test-results`はroot所有）。
- コミットメッセージは`prefix: 対応内容`（日本語）。コードを変えたら`graphify update .`を実行する。

## Review Focus

- **パソコンのスリープで開始時刻を過ぎた**：復帰時に5分を超えて遅れたタイマーは何も出さない（Task 7のテスト「遅れて起きたタイマーは通知しない」）。
- **画面を開く前からある未読**：ベルの最初の取得にあった通知ではブラウザ通知を出さない（Task 6のテスト）。
- **開始時刻の変更**：取り直しで古い時刻のタイマーを消し、新しい時刻で出す（Task 7のテスト「開始時刻が変わったら新しい時刻で通知する」）。
- **許可の取り消し**：オンにした後でブラウザの許可が外れても例外を出さず、設定画面で許可の方法を案内する（Task 5のテスト）。
- **同じ出来事を複数のタブが通知する**：`tag`に出来事の鍵を入れ、ブラウザが1つにまとめられるようにする（Task 6とTask 7のテストで`tag`を確かめる）。

---

### Task 1: 質問、回答、全体公開をベルに出す

**Files:**
- Create: `api/internal/database/042_contest_notifications.sql`
- Modify: `api/internal/database/database.go`（embedと`migrations()`の末尾）
- Modify: `api/internal/contests/clarifications.go`（`AskClarification`、`UpdateClarification`）
- Test: `api/internal/httpapi/clarification_notifications_test.go`

**Interfaces:**
- Produces: 通知の種類`clarification_question`、`clarification_answer`、`clarification_published`、`contest_results`。どれも`contest_id`だけを持ち、`problem_id`は持たない。`GET /my/notifications`の`kind`、`contestId`、`actor`、`title`（コンテスト名）で返る。
- 既存の関数のシグネチャは変えない。

- [ ] **Step 1: テストを書く**

`TestClarificationNotificationsPostgres`。`TestClarificationsPostgres`と同じ準備（`newTeamTest`、`cid`にp1とp2、p1の作者`writer`とテスター`tester`、p2の作者`otherwriter`とテスター`othertester`、共同作成者`cos`、`bob`と`carol`のチーム、`f.schedule(cid, "running")`）をする。各利用者の`GET /my/notifications`を`"kind:actor"`の一覧にする関数を作り、次を確かめる。

```go
// bob がp1について質問する
want(t, "alice", "clarification_question:bob")
want(t, "writer", "clarification_question:bob")
want(t, "tester", "clarification_question:bob")
for _, u := range []string{"cos", "otherwriter", "othertester", "carol", "bob"} { want(t, u) } // 何もない
// writer が回答する
want(t, "bob", "clarification_answer:writer")
// alice が全体公開する
want(t, "bob", "clarification_answer:writer", "clarification_published:alice")
want(t, "carol", "clarification_published:alice")
want(t, "writer", "clarification_question:bob")
// 公開済みの質問をもう一度公開しても増えない
want(t, "carol", "clarification_published:alice")
// bob がp2について質問すると、p2の作者とテスターに届き、p1の作者には届かない
want(t, "otherwriter", "clarification_question:bob")
want(t, "writer", "clarification_question:bob") // 1件のまま
```

どの通知も`contestId == cid`であることも確かめる。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: `cd api && go test ./internal/httpapi -run TestClarificationNotificationsPostgres -count=1`
Expected: FAIL（通知が0件）

- [ ] **Step 3: マイグレーション`042_contest_notifications.sql`を追加する**

```sql
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('favorite','first_accept','tester','user_editorial','contest_setter','clarification_question','clarification_answer','clarification_published','contest_results'));
ALTER TABLE notifications DROP CONSTRAINT notifications_subject;
ALTER TABLE notifications ADD CONSTRAINT notifications_subject CHECK (
    CASE WHEN kind IN ('contest_setter','clarification_question','clarification_answer','clarification_published','contest_results')
    THEN problem_id IS NULL AND contest_id IS NOT NULL ELSE problem_id IS NOT NULL AND contest_id IS NULL END
);
```

`database.go`に`//go:embed 042_contest_notifications.sql`と`contestNotificationsSchema`を足し、`migrations()`の末尾に加える。

- [ ] **Step 4: `AskClarification`で質問の宛先に通知を作る**

トランザクションにし、質問を`INSERT ... RETURNING id`で作る（行がなければ今までどおり`ErrClarificationUnavailable`）。続けて、コンテストの`owner_id`、対象問題の`problem_drafts.owner_id`、`problem_testers.owner_id`の重複を除いた集合から質問者を除き、`kind='clarification_question'`、`actor_id=質問者`、`contest_id`で通知を作る。この集合は`contest_clarification_manager`が真になる人と同じである。

- [ ] **Step 5: `UpdateClarification`で回答と全体公開の通知を作る**

`FOR UPDATE`の読み取りで`owner_id`と`published_at IS NOT NULL`も取る。更新の後、`answer != nil`なら質問者に`clarification_answer`を作る。`publish`が真で、更新前に公開されていなかったときだけ、そのコンテストの`contest_participants`全員（操作した人を除く）に`clarification_published`を作る。`actor_id`は操作した人。

- [ ] **Step 6: テストを通す**

Run: `cd api && go test ./internal/httpapi -run 'TestClarification' -count=1`
Expected: PASS（既存の`TestClarificationsPostgres`も通る）

- [ ] **Step 7: コミットする**

```bash
git add api/internal/database/042_contest_notifications.sql api/internal/database/database.go api/internal/contests/clarifications.go api/internal/httpapi/clarification_notifications_test.go
git commit -m "feat: 質問と回答と全体公開をベルで知らせる"
```

### Task 2: 結果の公開をベルに出す

**Files:**
- Modify: `api/internal/contests/reveal.go`（`Reveal`）
- Test: `api/internal/httpapi/contest_freeze_test.go`（`ops`が結果を公開する箇所の直後）

**Interfaces:**
- Consumes: Task 1の`contest_results`の種類と制約。

- [ ] **Step 1: テストを書き足す**

`TestContestFreezePostgres`で`ops`が`/reveal`に成功した直後に、`bob`と`carol`の`GET /my/notifications`がちょうど1件の`kind == "contest_results"`、`actor == "ops"`、`contestId == cid`を含み、`cos`には`contest_results`がないことを確かめる。続く`cos`の2回目の公開の後も、`bob`の`contest_results`が1件のままであることを確かめる。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd api && go test ./internal/httpapi -run TestContestFreezePostgres -count=1`
Expected: FAIL（`contest_results`が0件）

- [ ] **Step 3: `Reveal`で参加者に通知を作る**

`UPDATE contests SET revealed_at=...`の後、同じトランザクションで`contest_participants`全員（操作した人を除く）に`kind='contest_results'`、`actor_id=操作した人`の通知を作る。操作した人に`user_profiles`の行がなければ作らない（`actor_id`の外部キーで公開そのものを失敗させないため）。公開済みで早く返る経路では作らない。

- [ ] **Step 4: テストを通す**

Run: `cd api && go test ./internal/httpapi -run 'TestContestFreeze|TestNotifications' -count=1`
Expected: PASS

- [ ] **Step 5: コミットする**

```bash
git add api/internal/contests/reveal.go api/internal/httpapi/contest_freeze_test.go
git commit -m "feat: 結果の公開を参加者のベルで知らせる"
```

### Task 3: 参加中のコンテストの日時を返すAPI

**Files:**
- Create: `api/internal/contests/schedule.go`
- Modify: `api/internal/httpapi/contests.go`（ハンドラ）、`api/internal/httpapi/routes.go`
- Modify: ルートの一覧を持つテスト（`contest_routes_test.go`、`head_test.go`など。質問機能のコミット`62332d1`がルートを足した箇所と同じ）
- Modify: `api/README.md`（非公開APIの一覧）
- Create: `web/server/api/my/contest-schedule.get.ts`
- Modify: `web/shared/types/contest.ts`
- Test: `api/internal/httpapi/contest_schedule_test.go`

**Interfaces:**
- Produces（Go）: `type ScheduledContest struct { ID string \`json:"id"\`; Title string \`json:"title"\`; StartsAt time.Time \`json:"startsAt"\`; EndsAt time.Time \`json:"endsAt"\` }`、`func (s *Store) ParticipationSchedule(ctx context.Context, owner string) ([]ScheduledContest, error)`
- Produces（HTTP）: `GET /my/contest-schedule` → `{"items":[ScheduledContest...]}`（空なら`[]`）
- Produces（TS）: `contestScheduleSchema`（`z.object({ items: z.array(z.object({ id: uuid, title: string, startsAt: datetime(offset), endsAt: datetime(offset) })) })`）、`type ScheduledContest`、Nuxtの`GET /api/my/contest-schedule`

- [ ] **Step 1: テストを書く**

`TestContestSchedulePostgres`（`newTeamTest`）。チーム戦のコンテストを予定の状態で作り、`bob`がチームを作って参加する。

- 未ログインは401。
- `bob`は1件で、`id`、`title`、`startsAt`、`endsAt`がコンテストと一致する。
- 参加していない`carol`は`items == []`。
- `f.schedule(cid, "running")`の後も`bob`は1件、`f.schedule(cid, "ended")`の後は0件。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd api && go test ./internal/httpapi -run TestContestSchedulePostgres -count=1`
Expected: FAIL（404）

- [ ] **Step 3: `ParticipationSchedule`とハンドラ`contestSchedule`を書き、`private("GET /my/contest-schedule", contests.contestSchedule, false)`で登録する**

条件は「`contest_participants`に行がある」かつ「`ends_at > clock_timestamp()`」。`starts_at, id`の順で最大50件。応答は`writeAuthJSON(w, 200, map[string]any{"items": items})`。ルート一覧のテストが落ちたら一覧に足す。

- [ ] **Step 4: Nuxtの`/api/my/contest-schedule`を書く**

`web/server/api/my/solved-problems.get.ts`と同じ形（`privateHeaders`、`privateAPI(event, '/my/contest-schedule')`、`contestScheduleSchema.safeParse`、失敗は502）。`api/README.md`に1行足す。

- [ ] **Step 5: テストを通す**

Run: `cd api && go test ./internal/httpapi -count=1`
Expected: PASS

- [ ] **Step 6: コミットする**

```bash
git add api/internal/contests/schedule.go api/internal/httpapi api/README.md web/server/api/my/contest-schedule.get.ts web/shared/types/contest.ts
git commit -m "feat: 参加中のコンテストの日時を返すAPIを追加"
```

### Task 4: ベルとイベント一覧で新しい通知を表示する

**Files:**
- Modify: `web/shared/types/notification.ts`
- Test: `web/tests/contest-notifications.spec.ts`

**Interfaces:**
- Consumes: Task 1とTask 2の種類。
- Produces: `contestNotification.kind`を`z.enum(['contest_setter','clarification_question','clarification_answer','clarification_published','contest_results'])`に広げる。`notificationMessages`に次を足す。

| kind | 文言 | `notificationLink` |
| --- | --- | --- |
| `clarification_question` | `が質問しました` | `/contests/${shortId(contestId)}?view=clarifications` |
| `clarification_answer` | `が質問に回答しました` | 同上 |
| `clarification_published` | `が質問と回答を全体公開しました` | 同上 |
| `contest_results` | `が結果を公開しました` | `/contests/${shortId(contestId)}?view=standings` |

`notificationTitle`の空のときの名前は、`contestId`を持つ通知なら「無題のコンテスト」にする。

- [ ] **Step 1: テストを書く**

`/api/my/notifications`と`/api/my/notifications/read`を4種類の通知（`title: '春のコンテスト'`、`actor: 'bob'`）で返すようにルートし、ベルを開いて次を確かめる。

- 各行が`春のコンテスト`と`bobが質問しました`、`bobが質問に回答しました`、`bobが質問と回答を全体公開しました`、`bobが結果を公開しました`を表示する。
- 前の3つのリンクが`?view=clarifications`、最後が`?view=standings`。
- `/my?tab=events`のイベント一覧でも同じ文言が出る（`/api/my/notifications?history=1`をルートする）。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd web && npm run build && npx playwright test tests/contest-notifications.spec.ts --output=<scratchpad>/pw`
Expected: FAIL（スキーマが新しい種類を拒んで502になり、ベルが空）

- [ ] **Step 3: `notification.ts`を上の表のとおりに変える**

- [ ] **Step 4: テストを通す**

Run: 同上
Expected: PASS

- [ ] **Step 5: コミットする**

```bash
git add web/shared/types/notification.ts web/tests/contest-notifications.spec.ts
git commit -m "feat: 質問と結果の公開の通知をベルに表示する"
```

### Task 5: 通知の設定と、ブラウザ通知を出す部品

**Files:**
- Create: `web/app/composables/useBrowserNotifications.ts`
- Create: `web/app/components/BrowserNotificationSettings.vue`
- Modify: `web/app/pages/my/settings.vue`（`tabs`の「表示」の次に`['notifications', '通知']`）
- Modify: `web/app/content/docs/contest-rules.md`（質問と回答の節）
- Create: `web/tests/fixtures/notifications.ts`
- Test: `web/tests/browser-notifications.spec.ts`

**Interfaces:**
- Produces:

```ts
export type BrowserNotificationKind = 'clarifications' | 'contests' | 'featured' | 'editorials'
export interface BrowserNotificationSettings { enabled: boolean, kinds: Record<BrowserNotificationKind, boolean> }
export interface BrowserNotice { key: string, title: string, body: string, url: string }
export function useBrowserNotifications(): {
  settings: Ref<BrowserNotificationSettings> // Cookie openoj-browser-notifications、maxAge 1年、sameSite lax、path /
  supported: Ref<boolean>                    // SSRとマウント前はfalse
  permission: Ref<NotificationPermission | 'unsupported'>
  enable(): Promise<void>                    // 許可を求め、grantedならenabledをtrueにする
  wants(kind: BrowserNotificationKind): boolean // supportedかつgrantedかつenabledかつkinds[kind]
  show(notice: BrowserNotice): void          // new Notification(title, { body, tag: key, icon: '/favicon.svg' })、onclickでwindow.focus()、navigateTo(url)、close()
}
```

- Produces（テスト用）: `fakeNotifications(page, { permission, answer })`（`addInitScript`で`window.Notification`を差し替え、`requestPermission`は`answer`を返し、生成した通知を`window.__shown`に`{ title, body, tag }`で積み、インスタンスを`window.__instances`に積む）と`shown(page)`。

- [ ] **Step 1: テストを書く**

- **許可するとすべての種類がオンになる**：`permission: 'default'`、`answer: 'granted'`で`/my/settings?tab=notifications`を開く。「このブラウザで通知を受け取る」のスイッチをオンにすると、4種類のスイッチがすべてオンで表示される。再読み込みしても残る。
- **拒否されたらオフのまま案内する**：`answer: 'denied'`。スイッチはオフのまま、「ブラウザで通知が拒否されています。」で始まる案内が出る。
- **許可が外れたら案内する**：Cookieをオンで用意し、`permission: 'denied'`で開く。種類のスイッチの代わりに同じ案内が出る。
- **スマホではベルを案内する**：`test.use({ userAgent: '... Android ...' })`。スイッチを出さず「このブラウザでは通知を出せません。ベルで確かめてください。」を出す。
- 375pxで横スクロールがない。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd web && npm run build && npx playwright test tests/browser-notifications.spec.ts --output=<scratchpad>/pw`
Expected: FAIL（「通知」タブがない）

- [ ] **Step 3: `useBrowserNotifications`を書く**

Cookieは`useProblemTags`と同じ書き方にし、値は`BrowserNotificationSettings`のJSON、初期値は`{ enabled: false, kinds: 4つともtrue }`。`supported`と`permission`は`onMounted`で決める。

- [ ] **Step 4: `BrowserNotificationSettings.vue`を書き、設定画面の「通知」タブに置く**

スイッチは`InlineSwitch`を使う（`.settings-section label { display: block }`のような親のCSSが`InlineSwitch`の`label`に当たらないよう注意する）。説明は次の2文だけにする。「ShareOJを開いているあいだ、このブラウザに通知を出します。閉じているときは届きません。」「質問と回答と結果の公開は、ベルにも残ります。」種類のスイッチは受け取りがオンで許可があるときだけ出す。拒否と取り消しの案内は「ブラウザで通知が拒否されています。ブラウザのサイトの設定で通知を許可してください。」。

- [ ] **Step 5: ガイドに1行足す**

`contest-rules.md`の質問と回答の節に「質問、回答、全体公開はベルに届きます。設定の「通知」タブでオンにすると、パソコンのブラウザにも通知を出せます。」を足す。

- [ ] **Step 6: テストを通す**

Run: 同上、加えて`npx playwright test tests/settings.spec.ts tests/guides.spec.ts`
Expected: PASS

- [ ] **Step 7: コミットする**

```bash
git add web/app/composables/useBrowserNotifications.ts web/app/components/BrowserNotificationSettings.vue web/app/pages/my/settings.vue web/app/content/docs/contest-rules.md web/tests/fixtures/notifications.ts web/tests/browser-notifications.spec.ts
git commit -m "feat: ブラウザ通知の設定を追加"
```

### Task 6: ベルで見つけた質問と回答をブラウザ通知にする

**Files:**
- Modify: `web/app/composables/usePolling.ts`
- Create: `web/app/utils/bell-alerts.ts`
- Modify: `web/app/components/NotificationBell.vue`
- Test: `web/tests/browser-notifications.spec.ts`（追記）

**Interfaces:**
- Consumes: Task 4の種類、Task 5の`useBrowserNotifications`と`fakeNotifications`。
- Produces: `usePolling(task, delay, enabled = () => true, whileHidden = () => false)`（`document.hidden`でも`whileHidden()`が真なら実行する）。`bellAlert(item: Notification): BrowserNotice | null`。

| kind | title | url |
| --- | --- | --- |
| `clarification_question` | `質問が届きました` | `notificationLink(item)` |
| `clarification_answer` | `質問に回答がありました` | 同上 |
| `clarification_published` | `質問と回答が全体公開されました` | 同上 |
| それ以外 | `null`（通知しない） | |

`key`は`notification:${item.id}`、`body`は`notificationTitle(item)`。

- [ ] **Step 1: テストを書き足す**

設定をオンにしたCookieと`permission: 'granted'`を用意し、`page.clock.install()`の後に開く。

- **新しい質問を1回だけ通知する**：最初の取得は`[]`、次から質問の通知を1件返す。`page.clock.runFor(30_000)`で`shown`が`[{ title: '質問が届きました', body: '春のコンテスト', tag: 'notification:<id>' }]`になり、さらに`runFor(30_000)`しても1件のまま。`window.__instances[0].onclick()`で`/contests/<short>?view=clarifications`へ移る。
- **前からある未読、結果の公開、オフの種類は通知しない**：最初の取得に質問の通知があっても出さない。後から来た`contest_results`も出さない。`kinds.clarifications`がfalseなら後から来た回答も出さない。
- **裏のタブでは、受け取る設定のときだけ取得を続ける**：`addInitScript`で`document.hidden`を`true`、`visibilityState`を`'hidden'`にする。`runFor(90_000)`のあいだの`/api/my/notifications`の取得が、設定オンなら2回以上、オフなら0回（マウント時の1回を除く）。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd web && npm run build && npx playwright test tests/browser-notifications.spec.ts --output=<scratchpad>/pw`
Expected: FAIL（`shown`が空）

- [ ] **Step 3: `usePolling`に`whileHidden`を足す**

- [ ] **Step 4: ベルで新しい通知を見分けて`show`する**

`NotificationBell.vue`に、ページで見た通知のidの集合を持たせる。最初に取得が成功したときは集合を作るだけにする。以後の取得で集合にないidを見つけたら集合に足し、`bellAlert`が値を返して`wants('clarifications')`が真なら`show`する。利用者が変わったら集合を捨てる。`usePolling`の第4引数は`() => wants('clarifications')`。

- [ ] **Step 5: テストを通す**

Run: 同上、加えて`npx playwright test tests/contest-notifications.spec.ts tests/contest-clarifications.spec.ts`
Expected: PASS

- [ ] **Step 6: コミットする**

```bash
git add web/app/composables/usePolling.ts web/app/utils/bell-alerts.ts web/app/components/NotificationBell.vue web/tests/browser-notifications.spec.ts
git commit -m "feat: 質問と回答をブラウザ通知で知らせる"
```

### Task 7: 開始、終了、定期便の時刻にブラウザ通知を出す

**Files:**
- Create: `web/app/utils/event-alarms.ts`
- Create: `web/app/composables/useEventAlarms.ts`
- Modify: `web/app/layouts/default.vue`（`useEventAlarms()`を1回呼ぶ）
- Modify: `web/app/composables/useContestJoin.ts`（参加登録に成功したら`refreshEventAlarms()`）
- Test: `web/tests/event-alarms.spec.ts`

**Interfaces:**
- Consumes: Task 3の`ScheduledContest`と`/api/my/contest-schedule`、既存の`FeaturedPage`と`/api/featured`、Task 5の`useBrowserNotifications`。
- Produces: `interface Alarm extends BrowserNotice { at: number, kind: BrowserNotificationKind }`、`eventAlarms(contests: ScheduledContest[], featured: FeaturedPage | null): Alarm[]`、`useEventAlarms(): void`、`refreshEventAlarms(): void`（予定をすぐ取り直す。`useEventAlarms`が動いていなければ何もしない）。

`FeaturedPage.current`は、出題済みで解説公開前の回である（`FeaturedList`は出題から23時間のあいだだけ`current`を返す）。

`eventAlarms`が作る通知は次のとおり。

| 出来事 | 条件 | key | at | kind | title | body | url |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 開始 | 各コンテスト | `contest-start:${id}:${startsAt}` | `startsAt` | `contests` | `コンテストが始まりました` | コンテスト名 | `/contests/${shortId(id)}?view=problems` |
| 終了 | 各コンテスト | `contest-end:${id}:${endsAt}` | `endsAt` | `contests` | `コンテストが終了しました` | コンテスト名 | `/contests/${shortId(id)}?view=standings` |
| 出題 | `nextSlots`に`missing`以外がある | `featured-release:${nextAt}` | `nextAt` | `featured` | `定期便を出題しました` | `定期便 vol.${nextNumber}` | `/featured` |
| 解説公開 | `current`に`kind === 'new'`かつ`editorialHidden`の枠がある | `featured-editorial:${current.scheduledAt}` | その枠の`revealAt`の最小 | `editorials` | `定期便の解説を公開しました` | `定期便 vol.${current.number}` | `/featured` |

- [ ] **Step 1: テストを書く**

設定オンのCookie、`permission: 'granted'`、`page.clock.install({ time: T })`。`/api/my/contest-schedule`と`/api/featured`をルートする（特に断らない限り、定期便は`nextAt`を1週間後、`nextSlots`を欠番だけ、`current: null`にする）。

- **開始と終了で通知する**：開始T+2分、終了T+62分。`runFor('02:00')`で`{ title: 'コンテストが始まりました', tag: 'contest-start:<id>:<startsAt>' }`、さらに`runFor('60:00')`で`コンテストが終了しました`。
- **定期便の出題と解説公開**：`nextAt`をT+1分にし`nextSlots`に`new`、`current`の新作の`revealAt`をT+3分にする。`定期便を出題しました`（body `定期便 vol.N`）と`定期便の解説を公開しました`が出る。`nextSlots`が欠番だけなら出題は出ない。
- **遅れて起きたタイマーは通知しない**：開始T+2分で`page.clock.fastForward('10:00')`（スリープからの復帰）。何も出ない。
- **開始時刻が変わったら新しい時刻で通知する**：最初は開始T+20分。ルートをT+40分に変えて`runFor('11:00')`で取り直させる。`runFor('10:00')`（T+21分）までに何も出ず、`runFor('20:00')`で1件出る。
- **開いた後に参加登録したコンテストも通知する**：予定は最初`[]`を返す。コンテストのページで参加登録する（`POST /api/my/contests/<id>/participation`をルートして成功させ、以後の予定は開始T+3分の1件を返す）。`runFor('03:00')`で`コンテストが始まりました`が出る。
- **オフの種類と未ログインではタイマーを掛けない**：`kinds.contests`がfalseなら開始で出ない。未ログインの画面では`/api/my/contest-schedule`を取得しない。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd web && npm run build && npx playwright test tests/event-alarms.spec.ts --output=<scratchpad>/pw`
Expected: FAIL（`shown`が空）

- [ ] **Step 3: `eventAlarms`を上の表のとおりに書く**

- [ ] **Step 4: `useEventAlarms`を書き、`default.vue`で呼ぶ**

クライアントだけで動かす（`onMounted`から始める）。ログイン中で、`contests`、`featured`、`editorials`のどれかを`wants`するときだけ、要る方のAPIを取得する（コンテストは`contests`のとき、定期便は`featured`か`editorials`のとき）。`eventAlarms`の結果のうち、`Date.now() < at <= Date.now() + 24時間`で`wants(kind)`のものに`setTimeout`を掛ける。タイマーは`key`ごとに持ち、取り直しで消えた`key`のタイマーは消し、同じ`key`は掛け直さない。発火時に`Date.now() - at > 5分`なら出さず、そうでなければ`show`して予定を取り直す。取り直しは10分ごとの`setTimeout`の連鎖で行い、`usePolling`は使わない（裏のタブでも止めないため）。利用者か設定が変わったらすべてのタイマーを消して掛け直す。`refreshEventAlarms`はモジュールの変数に置いた取り直しの関数を呼び、`useContestJoin`の`send`が成功したときに呼ぶ（チーム参加とオンサイト参加もこの`send`を通る）。

- [ ] **Step 5: テストを通す**

Run: 同上、加えて`npx playwright test`（全体）
Expected: PASS

- [ ] **Step 6: コミットする**

```bash
git add web/app/utils/event-alarms.ts web/app/composables/useEventAlarms.ts web/app/composables/useContestJoin.ts web/app/layouts/default.vue web/tests/event-alarms.spec.ts
git commit -m "feat: コンテストの開始と終了、定期便の時刻にブラウザ通知を出す"
```
