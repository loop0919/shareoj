# ガイドを読者別に分け直す Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** サイトのガイドを「参加者向け」「作問者向け」「開発者向け」に分け直し、誰向けかを画面に出し、文面を短く書き直す。

**Architecture:** ページの一覧と大分類は`web/shared/docs.ts`の`docSections`が唯一の出どころで、サイドバー、`/docs`の一覧、パンくず、サイトマップがここから作られる。大分類の`title`を読者の名前に変え、ページの題名の上の飾りを同じ名前に置き換える。本文は`web/app/content/docs/<slug>.md`で、2ページを新設し、残りを設計書の文面の決まりで書き直す。

**Tech Stack:** Nuxt 4（Bun）、Markdown、Playwright。

**Spec:** `docs/superpowers/specs/2026-11-13-guides-by-reader-design.md`
本文を書くタスクは、着手前に設計書の「ページの構成」「文面の決まり」「書き直しの見本」を読むこと。

## Global Constraints

- ブランチ`feat/guides-by-reader`で作業し、全タスクが終わるまでmainに入れない。
- 既存のslugは消さない。`legacyDocPaths`は変えない。
- 「コンテストのルール」はslug`contest-rules`と題名を変えない。提出がいつ誰に公開されるかの記述を残す。
- 機能の挙動は変えない。スクリーンショットは撮り直さず、載せるページだけ移す。
- 1ページの読者は1人。手順は番号付きの箇条書きで1番号1操作。条件、上限、対応表は表か箇条書き。敬体で書く。
- 「ただし」の例外は、読者の多くが当たるものだけを独立した箇条書きの1項目として残す。
- 画面のボタン名や項目名を読めば分かる説明は書かない。スクリーンショットの番号（①②）は本文の手順と対応させて残す。
- 書き直すページは記述を実装と突き合わせ、合わないものは実装に合わせる。
- 見出しには`{#id}`を付ける。ガイド間のリンクは`/docs/<slug>#id`と書く。
- コミットメッセージは`prefix: 対応内容`の形式で、末尾に`Co-Authored-By: Claude Fable 5.2 <noreply@anthropic.com>`を付ける。
- コードを変更したコミットのあとに`graphify update .`を実行する。

### テストの実行

`web/test-results/`は書き込めないので、出力先を必ず指定する。
`nix develop`の中で実行する。

```bash
cd web && bun run build && bun run test tests/guides.spec.ts tests/seo.spec.ts --output="$PW_OUT"
```

`$PW_OUT`は、リポジトリの外の書き込めるディレクトリにする。
本文や画像を変えたあとは、テストの前に必ず`bun run build`をやり直す。

## Review Focus

- 利用規約とプライバシーポリシーから`/docs/contest-rules`へ飛んだ人が、提出の公開時期を見つけられること（タスク2のテストで固定）。
- 移した節を指す外部リンク（`/docs/contest-rules#participation`、`/docs/featured#apply`）で来た人が、ページの冒頭から移動先へ行けること（タスク2と4のテストで固定）。
- 読者のラベルを足しても、幅320pxで横スクロールが出ないこと（既存の「narrow screens」のテストが全ページを見ている。タスク1で通す）。
- 文面だけ整えるページで、表とコード例の数が変わらないこと（既存のテストが`generator`3個/1個、`checkers`7個/5個、`languages`0個/11個を数えている。タスク7で通す）。
- サイトマップの並びが`docSections`の順と一致すること（タスク1と4で`seo.spec.ts`の期待値を直す）。

---

### Task 1: 大分類を読者の名前にし、ページにラベルを出す

**Files:**
- Modify: `web/shared/docs.ts`
- Modify: `web/app/pages/docs/[slug].vue`（`<p class="eyebrow">SHAREOJ GUIDE</p>`の行）
- Modify: `web/app/pages/docs/index.vue`（導入文と`description`）
- Test: `web/tests/guides.spec.ts`、`web/tests/seo.spec.ts`

**Interfaces:**
- Produces: `docSections`の`title`は`'参加者向け' | '作問者向け' | '開発者向け'`の3つ。`docSection(slug).title`がそのページの読者のラベルになる。ページの題名の上には`<p class="eyebrow">`でこのラベルが出る。

この段階の並びは次のとおり（`featured-apply`と`contest-participation`は後のタスクで足す）。

| 大分類 | slug の順 |
| --- | --- |
| 参加者向け | `submissions`, `languages`, `difficulty`, `contest-rules`, `featured` |
| 作問者向け | `authoring`, `markdown`, `generator`, `checkers`, `cli`, `hosting-contests` |
| 開発者向け | `api` |

- [ ] **Step 1: ブランチを切る**

```bash
git switch -c feat/guides-by-reader
```

- [ ] **Step 2: 失敗するテストを書く**

`web/tests/guides.spec.ts`に加える。

```ts
test('each guide names its reader above the title and in the lists', async ({ page }) => {
  for (const [slug, reader] of [['submissions', '参加者向け'], ['contest-rules', '参加者向け'], ['hosting-contests', '作問者向け'], ['cli', '作問者向け'], ['api', '開発者向け']] as const) {
    await page.goto(`/docs/${slug}`)
    await expect(page.locator('article .eyebrow')).toHaveText(reader)
    await expect(page.getByRole('navigation', { name: 'パンくずリスト' })).toContainText(reader)
  }
  await page.goto('/docs')
  await expect(page.locator('.docs-index').getByRole('heading', { level: 2 })).toHaveText(['参加者向け', '作問者向け', '開発者向け'])
})
```

`web/tests/seo.spec.ts`のサイトマップの期待値を、上の表の順に並べ替える。

- [ ] **Step 3: テストが失敗することを確かめる**

Run: 「テストの実行」のコマンド
Expected: 加えたテストが、`SHAREOJ GUIDE`と`参加者向け`の不一致でFAIL。サイトマップのテストもFAIL。

- [ ] **Step 4: 実装する**

- `docSections`を上の表のとおりに組み替える。各ページの`title`、`label`、`description`、`lead`はこのタスクでは変えない。
- `[slug].vue`の飾りを`{{ docSection(page.slug).title }}`にする。
- `index.vue`の導入文を「ShareOJ の使い方を、読む人ごとにまとめています。」にし、`description`の末尾「目的別にまとめています」を「読む人ごとにまとめています」にする。

- [ ] **Step 5: テストが通ることを確かめる**

Run: 「テストの実行」のコマンド
Expected: `guides.spec.ts`と`seo.spec.ts`がすべてPASS。

- [ ] **Step 6: コミットする**

```bash
git add web/shared/docs.ts web/app/pages/docs web/tests/guides.spec.ts web/tests/seo.spec.ts
git commit -m "feat: ガイドの大分類を読者の名前にし、題名の上に読者を出す"
```

---

### Task 2: 「コンテストに参加する」を新設し、「コンテストのルール」を狭める

**Files:**
- Create: `web/app/content/docs/contest-participation.md`
- Modify: `web/app/content/docs/contest-rules.md`
- Modify: `web/app/content/docs/submissions.md`、`web/app/content/docs/hosting-contests.md`（`contest-rules`の節を指すリンクだけ）
- Modify: `web/shared/docs.ts`
- Test: `web/tests/guides.spec.ts`、`web/tests/seo.spec.ts`、`web/tests/account/contests.spec.ts`（`/docs/contest-rules`への遷移の期待は変えない）

**Interfaces:**
- Produces: `contest-participation`の節のid：`#join`、`#team`、`#onsite`、`#clarifications`、`#my-submissions`。`contest-rules`の節のid：`#official`、`#scoring`、`#penalty`、`#freeze`、`#visibility`。後のタスクはこのidにリンクする。

`docs.ts`に加える項目（参加者向けの`difficulty`の次、`contest-rules`の前）：

```ts
{ slug: 'contest-participation', title: 'コンテストに参加する', label: 'コンテストに参加する', description: 'ShareOJのコンテストの参加登録、チームでの参加、会場の合言葉、質問の出し方を説明します。', lead: '参加登録から、開催中に質問を出すところまでの手順です。' },
```

`contest-rules`の`description`は「ShareOJのコンテストの得点、順位、誤答ペナルティ、順位表の凍結、問題と提出の公開範囲を説明します。」、`lead`は「順位の決まり方と、問題や提出が公開される時期をまとめています。」にする。

- [ ] **Step 1: 失敗するテストを書く**

`web/tests/guides.spec.ts`で、既存の「the guide index lists every guide…」の期待値`参加と開催時間`/`#participation`（2か所）を、`contest-rules`の最初の節`公式順位に入る提出`/`#official`に直す。
次のテストを加える。

```ts
test('the contest guides split joining from ranking', async ({ page }) => {
  await page.goto('/docs/contest-participation')
  const joining = page.locator('article .markdown-body')
  for (const id of ['join', 'team', 'onsite', 'clarifications', 'my-submissions']) await expect(joining.locator(`#${id}`)).toHaveCount(1)
  await expect(joining.locator('img[src="/images/docs/contest-join.png"]')).toHaveCount(1)
  await expect(joining).not.toContainText('誤答ペナルティは')

  await page.goto('/docs/contest-rules')
  const rules = page.locator('article .markdown-body')
  await expect(rules.locator('h2')).toHaveCount(5)
  for (const id of ['official', 'scoring', 'penalty', 'freeze', 'visibility']) await expect(rules.locator(`#${id}`)).toHaveCount(1)
  // The terms and the privacy policy defer to this page for when contest submissions become public.
  await expect(rules.locator('#visibility ~ *').filter({ hasText: '終了後' }).first()).toBeVisible()
  // Readers arriving at a section that moved find the way from the top of the page.
  await expect(rules.locator('p').first().getByRole('link', { name: 'コンテストに参加する' })).toHaveAttribute('href', '/docs/contest-participation')
  await expect(rules).not.toContainText('回答')
  await expect(rules.locator('img')).toHaveCount(0)
})
```

`web/tests/seo.spec.ts`のサイトマップの期待値に`/docs/contest-participation`を`/docs/difficulty`の次へ加える。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: 「テストの実行」のコマンド
Expected: 加えたテストが`/docs/contest-participation`の404でFAIL。

- [ ] **Step 3: `contest-participation.md`を書く**

読者は、これからコンテストに出る人。
節は次の5つで、設計書の「書き直しの見本」を`#join`と`#team`の文面としてそのまま使う。

| 節 | id | 書くこと |
| --- | --- | --- |
| 参加登録する | `#join` | 見本のとおり |
| チームで参加する | `#team` | 見本のとおり。画像`contest-join.png`を載せる |
| 会場で参加する | `#onsite` | 会場で配られた合言葉を①に入れるとオンサイト参加、空ならオープン参加 |
| 質問する | `#clarifications` | コンテストメニューの「質問」から問題を選んで出す手順。全体公開されることがあるので個人情報を書かないこと。回答が届く場所（ベル） |
| 自分の提出を見る | `#my-submissions` | 「自分の提出」（チーム戦では「チームの提出」）。ほかの人の提出は終了後に見られること。順位の決まり方は`/docs/contest-rules`へリンク |

回答できる人、全体公開の操作、4,000文字の上限、15秒ごとの更新、合言葉の文字種は書かない。

- [ ] **Step 4: `contest-rules.md`を書き直す**

読者は、順位の決まり方を確かめたい参加者。
冒頭の1段落に、参加の手順は[コンテストに参加する](/docs/contest-participation)にある旨を1行で書く。
節は次の5つだけにする。

| 節 | id | 残すこと |
| --- | --- | --- |
| 公式順位に入る提出 | `#official` | コンテスト内の問題ページから開催時間中に受け付けた提出。判定が終了後になっても入る。サンプル検証と通常の問題ページからの提出は入らない |
| 得点と順位 | `#scoring` | 配点、部分点の数え方、チームは全員の提出をまとめること、同点のときの比べ方 |
| 誤答ペナルティ | `#penalty` | 加算対象と対象外の表、数え方、計算例 |
| 順位表の凍結 | `#freeze` | 凍結後は件数だけ出て、結果の公開まで伏せること |
| 問題と提出の公開範囲 | `#visibility` | 「いつ」「誰に」「何が」見えるかの表（開始前、開催中、凍結中、終了後）。開始前のテスト提出とサンプル検証は公開しないこと |

スタッフが参加できないこと、スタッフに見える提出、メニューに「すべての提出」が出る条件は書かない（スタッフの話はタスク3で`hosting-contests`に入れる）。
得点と順位の記述は、`api`の順位表の実装と突き合わせて確かめる。

- [ ] **Step 5: ほかのページからのリンクを直す**

- `submissions.md`の`/docs/contest-rules#visibility`はそのまま有効であることを確かめる。
- `hosting-contests.md`の`/docs/contest-rules#clarifications`を`/docs/contest-participation#clarifications`に直す（このリンク自体はタスク3で書き直す）。

- [ ] **Step 6: テストが通ることを確かめる**

Run: 「テストの実行」のコマンド
Expected: `guides.spec.ts`と`seo.spec.ts`がすべてPASS。「links between guides point at existing pages and headings」が通ること。

- [ ] **Step 7: コミットする**

```bash
git add web/app/content/docs web/shared/docs.ts web/tests
git commit -m "feat: コンテストのガイドを参加の手順と順位の決まりに分ける"
```

---

### Task 3: 「コンテストを開く」を書き直す

**Files:**
- Modify: `web/app/content/docs/hosting-contests.md`
- Test: `web/tests/guides.spec.ts`

**Interfaces:**
- Consumes: `contest-participation#clarifications`、`contest-rules#freeze`、`authoring#readiness`、`authoring#rejudge`、`authoring#testers`、`authoring#scoring`
- Produces: 節のid`#problems`、`#create`、`#publish`、`#changes`、`#setters`、`#staff`、`#clarifications`、`#after`。旧`#testers`は`#staff`に改める（外から`#testers`を指すリンクはリポジトリ内にない。着手時に`git grep 'hosting-contests#'`で確かめる）。

- [ ] **Step 1: 失敗するテストを書く**

既存の「the contest guide says start times are typed in JST」は、期待する文を「開始日時はJSTで入力します。」に直す。
次を加える。

```ts
test('the hosting guide carries what staff need to know', async ({ page }) => {
  await page.goto('/docs/hosting-contests')
  const body = page.locator('article .markdown-body')
  for (const id of ['problems', 'create', 'publish', 'changes', 'setters', 'staff', 'clarifications', 'after']) await expect(body.locator(`#${id}`)).toHaveCount(1)
  await expect(body).toContainText('スタッフは参加登録できません')
  await expect(body.locator('ol').first()).toBeVisible()
})
```

- [ ] **Step 2: テストが失敗することを確かめる**

Run: 「テストの実行」のコマンド
Expected: 加えたテストが`#staff`がないことでFAIL。

- [ ] **Step 3: 書き直す**

読者は、コンテストを開く作問者。

| 節 | id | 書くこと |
| --- | --- | --- |
| 出題する問題を用意する | `#problems` | 出題できる条件の箇条書き |
| コンテストを作る | `#create` | 作成の手順を番号付きで。設定項目と範囲は表（問題数、配点、ペナルティ、凍結、参加形式、人数の上限、オンサイト区分）。「開始日時はJSTで入力します。」 |
| 公開する | `#publish` | 公開の条件の箇条書き。公開後に参加登録が始まり、問題は開始時刻に公開されること |
| 開始前と開始後の変更 | `#changes` | 「開始前」「開始後」に変えられるものの表。公開したコンテストは削除も中止もできないこと |
| 共同作成者 | `#setters` | 招待の手順を番号付きで。共同作成者にできること |
| スタッフ | `#staff` | スタッフの定義。「スタッフは参加登録できません」。開催中もすべての提出を見られること。テスターの招き方へのリンク |
| 質問に回答する | `#clarifications` | 回答できる人、全体公開すると戻せないこと。旧`contest-rules`の「質問と回答」のうち担当者向けの部分 |
| 終了したあと | `#after` | 問題と解説が自動で公開されること。凍結したときは「結果を公開」で確定すること |

合言葉の文字種と桁数、一度スタッフになった人の扱い、参加登録した人がテスターになれない期間は書かない。

- [ ] **Step 4: テストが通ることを確かめる**

Run: 「テストの実行」のコマンド
Expected: すべてPASS。

- [ ] **Step 5: コミットする**

```bash
git add web/app/content/docs/hosting-contests.md web/tests/guides.spec.ts
git commit -m "feat: 「コンテストを開く」を作問者向けの手順に書き直す"
```

- [ ] **Step 6: 利用者に文体を確かめてもらう**

ここで止まり、`contest-participation`、`contest-rules`、`hosting-contests`の3ページを利用者に読んでもらう。
直しがあれば反映し、文面の決まりに足すべきことがあれば設計書に書き足してから、タスク4へ進む。

---

### Task 4: 定期便を「解く人向け」と「応募する人向け」に分ける

**Files:**
- Create: `web/app/content/docs/featured-apply.md`
- Modify: `web/app/content/docs/featured.md`
- Modify: `web/app/content/docs/authoring.md`、`web/app/content/docs/submissions.md`（定期便へのリンクだけ）
- Modify: `web/app/pages/featured/apply.vue`（「出題のルール」のリンク先）
- Modify: `web/shared/docs.ts`
- Test: `web/tests/guides.spec.ts`、`web/tests/seo.spec.ts`

**Interfaces:**
- Produces: `featured`の節のid`#schedule`、`#solve`、`#standings`。`featured-apply`の節のid`#apply`、`#conditions`、`#selection`、`#withdraw`。

`docs.ts`に加える項目（作問者向けの`cli`の次、`hosting-contests`の前）：

```ts
{ slug: 'featured-apply', title: '定期便に応募する', label: '定期便に応募する', description: 'ShareOJの定期便に新作を応募する手順、応募できる条件、選ばれ方を説明します。', lead: '未公開の問題を、月曜と木曜の定期便に出せます。' },
```

`featured`の`description`は「毎週月曜・木曜に出題する定期便の出題の日時、解説と提出の公開時期、順位表の決まり方を説明します。」にする。

- [ ] **Step 1: 失敗するテストを書く**

既存の「the featured guide gives the delivery time in JST」は、期待する文字列を`22:00 (JST)`だけにする。
次を加える。

```ts
test('the featured guides split solving from applying', async ({ page }) => {
  await page.goto('/docs/featured')
  const solving = page.locator('article .markdown-body')
  for (const id of ['schedule', 'solve', 'standings']) await expect(solving.locator(`#${id}`)).toHaveCount(1)
  await expect(solving.locator('#apply')).toHaveCount(0)
  await expect(solving.getByRole('link', { name: '定期便に応募する' })).toHaveAttribute('href', '/docs/featured-apply')
  await expect(solving).toContainText('Hardの点数')

  await page.goto('/docs/featured-apply')
  await expect(page.locator('article .eyebrow')).toHaveText('作問者向け')
  const applying = page.locator('article .markdown-body')
  for (const id of ['apply', 'conditions', 'selection', 'withdraw']) await expect(applying.locator(`#${id}`)).toHaveCount(1)
  await expect(applying.locator('img[src="/images/docs/featured-apply.png"]')).toHaveCount(1)
})
```

`web/tests/seo.spec.ts`のサイトマップの期待値に`/docs/featured-apply`を`/docs/cli`の次へ加える。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: 「テストの実行」のコマンド
Expected: 加えたテストが`#standings`がないことでFAIL。

- [ ] **Step 3: `featured.md`を書き直す**

読者は、定期便を解く人。

| 節 | id | 書くこと |
| --- | --- | --- |
| 出題の日時 | `#schedule` | 毎週月曜と木曜の22:00 (JST)にEasyとHardを1問ずつ。新作と復刻の違い。次回予告に出るもの |
| 解く | `#solve` | 通常の問題と同じように提出すること。新作の解説とほかの人の提出は翌日22時に公開されること |
| 順位表 | `#standings` | 対象になる提出。順位の決まり方 |

順位の決まり方は、いまのガイドの記述（正解した問題の組み合わせで決める）が古い。
`web/app/pages/featured/standings.vue`、`docs/adr/0033-rank-featured-by-scores.md`、APIの実装を読み、今の決まり（Hardの点数、同点ならEasyの点数）に合わせて書く。
作問者とテスターがタイトルを表示する操作、復刻の選び方、欠番は書かない。
末尾に[定期便に応募する](/docs/featured-apply)へのリンクを1行置く。

- [ ] **Step 4: `featured-apply.md`を書く**

読者は、新作を応募する作問者。

| 節 | id | 書くこと |
| --- | --- | --- |
| 応募する | `#apply` | 手順を番号付きで。画像`featured-apply.png`。応募は作者ごとに3件まで |
| 応募できる問題 | `#conditions` | 条件の箇条書き。足りない項目は「問題管理」で確かめること |
| 選ばれ方 | `#selection` | 希望（早めに出したい、ゆっくりで良い）と応募順。出題時刻の保存内容で公開されること。予定は`/featured/schedule`で見られること |
| 取り下げる | `#withdraw` | マイページから取り下げること。公開すると取り下げになること |

- [ ] **Step 5: リンクを直す**

- `authoring.md`の`/docs/featured#apply`を`/docs/featured-apply#apply`にする。
- `web/app/pages/featured/apply.vue`の「出題のルール」を、`/docs/featured-apply`を指す「応募のガイド」にする。
- `git grep -n 'docs/featured' -- web`で残りがないか確かめる。

- [ ] **Step 6: テストが通ることを確かめる**

Run: 「テストの実行」のコマンド
Expected: すべてPASS。

- [ ] **Step 7: コミットする**

```bash
git add web/app/content/docs web/app/pages/featured/apply.vue web/shared/docs.ts web/tests
git commit -m "feat: 定期便のガイドを解く人向けと応募する人向けに分ける"
```

---

### Task 5: 「提出と判定結果」を書き直す

**Files:**
- Modify: `web/app/content/docs/submissions.md`
- Test: `web/tests/guides.spec.ts`

**Interfaces:**
- Produces: 節のid`#submit`、`#sample-check`、`#code-run`、`#verdicts`、`#judging`、`#comparison`、`#visibility`、`#rejudge`。`#usage`はなくす（着手時に`git grep 'submissions#usage'`で参照がないことを確かめる）。

- [ ] **Step 1: 失敗するテストを書く**

```ts
test('the submission guide leads with the steps and keeps the verdict table', async ({ page }) => {
  await page.goto('/docs/submissions')
  const body = page.locator('article .markdown-body')
  await expect(body.locator('#submit ~ ol').first().locator('li')).toHaveCount(3)
  await expect(body.locator('table').first().locator('tbody tr')).toHaveCount(9)
  await expect(body.locator('#usage')).toHaveCount(0)
  await expect(body).not.toContainText('1,048,576')
})
```

- [ ] **Step 2: テストが失敗することを確かめる**

Run: 「テストの実行」のコマンド
Expected: 加えたテストが、`#submit`の下に`ol`がないことでFAIL。

- [ ] **Step 3: 書き直す**

読者は、問題を解いて提出する人。

- `#submit`：言語を選ぶ、コードを入れる、「提出する」を押す、の3手順。回数制限は表（提出60秒に2回、サンプル検証60秒に3回、コード実行60秒に3回）。
- `#sample-check`と`#code-run`：何ができるかと、結果が順位に入らないこと。コード実行の上限は表。
- `#verdicts`：判定結果の表は9行のまま残す。OLEの閾値と経過時間の上限の式は削る。
- `#judging`：提出全体の結果は最初に正解にならなかったケースの結果であること。TLE2件で打ち切ること。
- `#comparison`：空白の違いは無視され、大文字小文字は区別されること。
- `#visibility`：ほかの人に公開されない提出の箇条書き。
- `#rejudge`：問題が直されるとリジャッジで結果が変わりうること。
- 旧`#usage`のうち「実行時間とメモリは各ケースの最大値」だけを`#judging`に1行で残す。

- [ ] **Step 4: テストが通ることを確かめる**

Run: 「テストの実行」のコマンド
Expected: すべてPASS。

- [ ] **Step 5: コミットする**

```bash
git add web/app/content/docs/submissions.md web/tests/guides.spec.ts
git commit -m "feat: 「提出と判定結果」を手順と表を中心に書き直す"
```

---

### Task 6: 「問題の作成と公開」を書き直す

**Files:**
- Modify: `web/app/content/docs/authoring.md`
- Test: `web/tests/guides.spec.ts`

**Interfaces:**
- Produces: 節のidは今のまま保つ（`#start`、`#statement`、`#tests`、`#scoring`、`#save`、`#readiness`、`#publish`、`#after-publish`、`#rejudge`、`#testers`、`#editorials`）。アプリ内の`ScoringEditor.vue`（`#scoring`）と`problems/post.vue`（`#readiness`）、ほかのガイドがこれらを指している。

- [ ] **Step 1: 失敗するテストを書く**

```ts
test('the authoring guide keeps its anchors and drops the sample marker details', async ({ page }) => {
  await page.goto('/docs/authoring')
  const body = page.locator('article .markdown-body')
  for (const id of ['start', 'statement', 'tests', 'scoring', 'save', 'readiness', 'publish', 'after-publish', 'rejudge', 'testers', 'editorials']) await expect(body.locator(`#${id}`)).toHaveCount(1)
  await expect(body.locator('img')).toHaveCount(11)
  await expect(body).not.toContainText('sample-begin')
  await expect(body.locator('#publish ~ ol').first()).toBeVisible()
})
```

着手時に今の画像の数を`grep -c '^!\[' web/app/content/docs/authoring.md`で数え、11と違えばテストの数を合わせる。

- [ ] **Step 2: テストが失敗することを確かめる**

Run: 「テストの実行」のコマンド
Expected: 加えたテストが`sample-begin`を含むことでFAIL。

- [ ] **Step 3: 書き直す**

読者は、問題を作って公開する作問者。
作ってから公開するまでを一本道で読めるようにする。

- 画像と①②の番号はすべて残し、操作は番号付きの手順にする。
- `#statement`の範囲の表と`#readiness`の必要なものの箇条書きは残す。
- `#save`は「公開前は自動で保存、公開中やコンテスト登録後は『保存』を押す」の2行の表にする。作成枠（最大20件、2時間ごとに1件回復）は残す。
- `#tests`から、改行の変換、サンプルのマーカーの書式、キーボードでの並べ替えを削る。
- `#publish`から、予約中の保存の細かい挙動を削り、「今すぐ」と「日時を指定」の違いだけを書く。
- `#testers`は、テスターにできることと作者だけができることを表にする。

- [ ] **Step 4: テストが通ることを確かめる**

Run: 「テストの実行」のコマンド
Expected: すべてPASS。

- [ ] **Step 5: コミットする**

```bash
git add web/app/content/docs/authoring.md web/tests/guides.spec.ts
git commit -m "feat: 「問題の作成と公開」を公開までの手順として書き直す"
```

---

### Task 7: 参照用のページの文面を整える

**Files:**
- Modify: `web/app/content/docs/languages.md`、`difficulty.md`、`markdown.md`、`generator.md`、`checkers.md`、`cli.md`、`api.md`
- Test: `web/tests/guides.spec.ts`（既存のテストだけ）

**Interfaces:**
- Produces: 見出しのidは1つも変えない。表、コード例、数値は削らない。

- [ ] **Step 1: 変える前の形を記録する**

```bash
cd web/app/content/docs && for f in languages difficulty markdown generator checkers cli api; do echo "$f $(grep -c '^|' $f.md) $(grep -c '^```' $f.md) $(grep -o '{#[a-z0-9-]*}' $f.md | sort | md5sum | cut -c1-8)"; done
```

出力を控える。

- [ ] **Step 2: 地の文を整える**

1ページずつ、設計書の「文面の決まり」に合わせて地の文だけを直す。

- 「〜できますが、〜はできません」の連結は2文に分けるか、箇条書きにする。
- 画面を見れば分かる説明と、同じことの言い直しを削る。
- 文中に並んだ条件は箇条書きにする。
- `cli.md`の内容は`cli/guide.md`と重なるところがある。`cli/guide.md`は変えない。
- 気づいた事実の誤りだけを、実装に合わせて直す。

1ページ直すごとにコミットする。

```bash
git commit -m "fix: 「<ページの題名>」のガイドの言い回しを整える" web/app/content/docs/<slug>.md
```

- [ ] **Step 3: 形が変わっていないことを確かめる**

Step 1のコマンドをもう一度実行する。
Expected: 表の行数、コードフェンスの数、idのハッシュが、どのページもStep 1と同じ。箇条書きにした結果、表の行が増減していないこと。

- [ ] **Step 4: テストが通ることを確かめる**

Run: 「テストの実行」のコマンド
Expected: すべてPASS。特に`generator`、`checkers`、`languages`の表とコード例の数のテスト。

---

### Task 8: READMEを合わせ、全体を確かめる

**Files:**
- Modify: `web/README.md`（「## ガイド」の節）
- Test: `web/tests/`の全体

- [ ] **Step 1: READMEを直す**

「## ガイド」の節に、次の3点を書き足す。

- 大分類は読者（参加者向け、作問者向け、開発者向け）で、`shared/docs.ts`の`docSections`の`title`がそのままページの題名の上に出ること。
- 1ページの読者は1人に決め、別の読者に必要な話はその読者のページへ移すこと。
- 文面の決まりは設計書（`docs/superpowers/specs/2026-11-13-guides-by-reader-design.md`）にあること。

- [ ] **Step 2: テスト全体を通す**

```bash
cd web && bun run typecheck && bun run build && bun run test --output="$PW_OUT"
```

Expected: すべてPASS。`tests/share-preview.spec.ts`と`tests/tweet.spec.ts`は`/docs/contest-rules`を使っているが、slugを保っているので変更なしで通る。
`catalogue.spec`は以前から不安定なので、落ちたらそのテストだけ再実行して切り分ける。

アカウントのテストも、`make db-up`のあとで実行する。

```bash
TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable' bun run test --config playwright.account.config.ts tests/account/contests.spec.ts --output="$PW_OUT"
```

Expected: PASS。

- [ ] **Step 3: 読者になって読む**

`bun run dev`でサイトを開き、14ページを頭から読む。

- 題名の上とサイドバーに読者の名前が出ている。
- どのページにも、別の読者に向けた節が残っていない。
- `/terms`と`/privacy`の「コンテストのルール」のリンクから飛び、提出の公開時期が書かれている。
- コンテストのページの「ルール」と、応募ページの「応募のガイド」のリンクが正しいページを開く。

- [ ] **Step 4: コミットする**

```bash
git add web/README.md
git commit -m "docs: ガイドの読者別の構成をREADMEに書く"
graphify update .
```

mainへの取り込みは利用者が決める。
未マージの`feat/contest-webhooks`との衝突の解き方は、設計書の「未マージのWebhookブランチとの関係」に従う。
