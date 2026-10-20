# C13 サイトマップの入口を、固定の共有画像を持つページの一覧から作る 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `web/server/routes/sitemap.xml.get.ts:6`の直書きの配列をやめ、`Object.keys(socialPages)`から入口を作る。あわせて、`web/README.md:308`の誤った記述を直す。

**Architecture:** 先に、sitemapのすべての入口について「ページが告げるog:imageが200のPNGを返す」ことを確かめる特性テストを`web/tests/seo.spec.ts`に足す。次にsitemapの一覧を`socialPages`のキーから作る。`socialPages`の形・型・値・順序と、ogルートは変えない。

**Tech Stack:** Nuxt 4 / Nitro、TypeScript、Playwright（`.output/server`を起動して検証）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C13.json`（worthとsafetyの`revised_scope`、`must_preserve`、`test_plan`。両者が食い違う点は下の判定に従う）

## 困りごと

- 固定の公開ページ7件（`'/'`、`'/problems'`、`'/contests'`、`'/blog'`、`'/docs'`、`'/terms'`、`'/privacy'`）が、2か所に同じ順序で書かれている。1つは`sitemap.xml.get.ts:6`の配列、もう1つは`shared/social-pages.ts:3-12`のキーである（計画者が実コードで確認した）。
- ガイドは4da2115以降、どちらも`shared/docs.ts:52`の`docPages`から作っている。残る重複は固定の7件だけで、同時に直した例はbc6e72c（terms/privacyの追加）の1回である。効果は「低〜中」とする（safetyの訂正を採る）。
- 片方を直し忘れても、どのテストも落ちない。sitemapへの追加を忘れた場合、`seo.spec.ts`の期待値も直さなければ見つからない。`socialPages`への追加を忘れた場合はページのog:imageが404になるが、`share-preview.spec.ts:9-11`は`/terms`、`/privacy`、ガイド8本を対象にしていない。
- `web/README.md:308`の「Guide titles live in `shared/social-pages.ts`」は誤りである。ガイドのタイトルは`shared/docs.ts`の`docSections`にある。

## 判定の記録

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| sitemapの一覧の作り方 | `Object.keys(socialPages)`にする。新しい配列や型は作らない | `{ path, title, kind }[]`を新設し、そこから`socialPages`（`Object.fromEntries`）とsitemapを作る。`Object.keys`の列挙順に頼らない | **worthの案を採る。** `socialPages`はすでに挿入順を保つRecordで、キーはすべて`'/'`で始まる。整数とみなされる文字列でないので、`Object.keys`もオブジェクトリテラルのスプレッドも挿入順になると仕様で決まっている。配列を新設すると、型と構造が1段増える。しかも`socialPages`の値を組み立て直すので、title/kindの取り違えという新しいリスクを負う。このリスクは既存のテストでは検出できない（画像の中身を見ていない）。順序に依存することは`social-pages.ts`のコメントに書く。順序が崩れれば`seo.spec.ts:34-39`の完全一致で検出できる |
| 2つの集合を結びつけること（worthの理由3） | 受け入れてよい。(a)だけに縮めてもよい | — | **受け入れる。** 今の2つの集合は一致している（/featured系はどちらにもない）。sitemapに入る固定ページは、すべて`useSharePreview`で固定の画像を告げている。将来分ける必要が出たら、そのときに分ける |
| 特性テスト | 不要 | `seo.spec.ts`のループで、各入口のog PNGが200になることを確かめる | **足す。ただし既存のテストには書き足さず、`seo.spec.ts`に新しいテストとして足す。** 変更後は「sitemapの入口＝固定の共有画像を持つページ」が構造上の約束になる。それを実際のHTTPの挙動で固定する。画像のURLはページのSSR HTMLのog:imageから読み、パスと画像の対応規則をテストに再実装しない |
| title/kindを変更前後で比べる（sha256など） | — | コミットしない比較で確かめる | **行わない。** `socialPages`の値に触れないため。`social-pages.ts`の差分がコメント1行だけであることを、差分の確認で確かめる |
| README | 直す | 直す | 直す。この節は英語なので英語で書く |
| コメントの言語 | — | — | 英語。周りのコードに合わせる。計画とレポートは日本語 |

## Global Constraints

- 公開HTTP API、ルート、DBスキーマ、依存のバージョン、CI・インフラを変えない。テストは追加だけにする。既存のテストは変更・削除・スキップしない。
- `/sitemap.xml`のバイト列を変えない。`<loc>`は19件で、順序は`'/'`、`'/problems'`、`'/contests'`、`'/blog'`、`'/docs'`、`docPages`の順のガイド12件（submissions〜api）、`'/terms'`、`'/privacy'`。`new URL(path, siteUrl).href`、`&`・`<`・`>`のエスケープ式、Content-Type `application/xml; charset=utf-8`、XML宣言とurlsetのxmlnsも変えない。`sitemap.xml.get.ts`で変えてよいのは、1行目のimport、5行目のコメント、6行目の`paths`の式だけ。
- `web/shared/social-pages.ts`は、export名`socialPages`、型`Record<string, { title: string, kind: string }>`、キーの集合と順序、各値（`'/'`→Share Online Judge/ShareOJ、`'/problems'`→問題/PROBLEMS、`'/contests'`→コンテスト/CONTESTS、`'/blog'`→記事/ARTICLES、`'/docs'`→ガイド/GUIDE、各ガイド→docPagesのtitle/GUIDE、`'/terms'`→利用規約/TERMS、`'/privacy'`→プライバシーポリシー/PRIVACY）、import（`./docs`だけ）を変えない。足してよいのはコメント1行だけ。
- `web/server/routes/og/[...path].get.ts`、`web/server/routes/robots.txt.get.ts`、`web/shared/docs.ts`、`web/app/pages/terms.vue`、`web/app/pages/privacy.vue`、`web/app/composables/useSharePreview.ts`、`web/app/pages/problems/index.vue`、`web/app/pages/blog/index.vue`は変更しない。共有画像のルートの挙動と、D23（タグで絞り込んだ一覧のog:image）はそのまま残す。
- `/featured`系のページを、sitemapにも`socialPages`にも足さない。
- `git commit`、`git stash`、`git reset`、他ブランチの`git checkout`は実行しない。コミットはオーケストレーターが行う。C04の作業中のファイル（`web/app/composables/useProblemDraft.ts`、`web/app/utils/problem-draft.ts`、`web/tests/editor.spec.ts`、`web/tests/manual-problem-save.spec.ts`、`web/tests/problem-save-behavior.spec.ts`）には触れない。
- ビルドとPlaywrightは同じworktreeの`.output`とポート13000/13001を使う。実行する前に、他のサイクルのビルドやPlaywrightが動いていないことを確かめる（検証コマンド0）。

## Review Focus

1. 入口の順序。`Object.keys(socialPages)`が今の配列と同じ順序を返すこと。キーに整数とみなされる文字列が入ると先頭に移るが、キーはすべて`'/'`で始まる。→ `seo.spec.ts`の既存テストが順序込みの完全一致で確かめる。
2. 共有画像のタイトルと種別。`social-pages.ts`の値は変えない。画像の中身を見るテストはないので、→ 差分の確認（Step 5）で、コメント1行以外の変更がないことを確かめる。
3. XMLの組み立て。URLの作り方、エスケープ、Content-Type、XML宣言が変わらないこと。→ 差分の確認で`sitemap.xml.get.ts`の7〜9行目に変更がないことを確かめる。既存テストはContent-Typeとxmlnsを確かめている。
4. 新しいテストが空回りしないこと。og:imageのmetaが見つからないときや、属性の順序が違うときにも正しく判定できること。→ metaタグ全体を拾ってから`content`を読む。値が`undefined`なら`toMatch`が失敗する。sitemapが空なら`not.toHaveLength(0)`で失敗する。
5. クライアント側からの読み込み。`terms.vue`と`privacy.vue`は`social-pages.ts`をクライアントのバンドルに入れる。`social-pages.ts`にサーバー専用のimport（`useRuntimeConfig`、`server/`）を足さないこと。→ `social-pages.ts`のimportは変えない。`policy.spec.ts`のh1の確認と、検証コマンド2のビルドで確かめる。

---

### Task 1: sitemapの入口をsocialPagesのキーから作る

**Files:**
- Test: `web/tests/seo.spec.ts`（`'robots and sitemap expose canonical public entry points'`の直後、48行目と50行目の間に新しいテストを足す）
- Modify: `web/server/routes/sitemap.xml.get.ts:1,5,6`
- Modify: `web/shared/social-pages.ts`（3行目の直前にコメント1行）
- Modify: `web/README.md:308`

**Interfaces:**
- Consumes: `socialPages`（`~~/shared/social-pages`、既存のまま）
- Produces: 新しいexportはない。

- [ ] **Step 1: 特性テストを足す**

次のテストを足す。既存のテストは変えない。

```ts
// The sitemap's entry points are the pages with a fixed share image; each preview must render.
test('every sitemap entry point advertises a working share image', async ({ request }) => {
  const xml = await (await request.get('/sitemap.xml')).text()
  const locations = [...xml.matchAll(/<loc>(.*?)<\/loc>/g)].map(match => match[1]!)
  expect(locations).not.toHaveLength(0)
  for (const location of locations) {
    const entry = await request.get(new URL(location).pathname)
    expect(entry.status()).toBe(200)
    const meta = (await entry.text()).match(/<meta[^>]*property="og:image"[^>]*>/)?.[0]
    const image = meta?.match(/content="([^"]+)"/)?.[1]
    expect(image, location).toMatch(/^https:\/\/judge\.example\/og\/.+\.png$/)
    const response = await request.get(new URL(image!).pathname)
    expect(response.status(), image).toBe(200)
    expect(response.headers()['content-type']).toBe('image/png')
    const png = await response.body()
    expect(png.subarray(0, 8).toString('hex')).toBe('89504e470d0a1a0a')
    expect(png.readUInt32BE(16)).toBe(1200)
    expect(png.readUInt32BE(20)).toBe(630)
  }
})
```

- [ ] **Step 2: 変更前のコードで成功することを確かめる**

検証コマンド0、1、2を実行してから、`seo.spec.ts`だけを流す。

`cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/seo.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c13`

Expected: `4 passed`。新しいテストも成功すること。30秒の既定のタイムアウトで失敗した場合に限り、テストの先頭に`test.slow()`を足して再実行し、レポートに記録する。それ以外の理由で失敗したら、本番コードに手を入れずに止まり、レポートに書く。

- [ ] **Step 3: sitemapの一覧を`socialPages`のキーから作る**

`sitemap.xml.get.ts`を次のようにする。

- 1行目: `import { socialPages } from '~~/shared/social-pages'`にする。`docPages`と`docPath`のimportは使わなくなるので外す。
- 5行目のコメント: 趣旨を保ったまま、一覧の出どころを書く。例: `// Stable entry points: the pages with a fixed share image, in their listed order. Public content is linked from the catalogue pages.`
- 6行目: `const paths = Object.keys(socialPages)`にする。
- 7〜9行目は変えない。

`social-pages.ts`の`export const socialPages`の直前に、英語のコメントを1行足す。キーの順序がsitemapの順序になることを書く。例: `// Public pages with a fixed share image. The sitemap lists these paths, in this order, as its entry points.`

- [ ] **Step 4: READMEを直す**

`web/README.md:308`の`Guide titles live in \`shared/social-pages.ts\`.`を、次の2点がわかる英語の文に置き換える。前後の文と、80〜90字前後で折り返す書き方は保つ。

- 固定ページの画像のタイトルは`shared/social-pages.ts`にあり、そのパスがsitemapの入口にもなること
- ガイドのタイトルは`shared/docs.ts`にあること

例: ``The fixed pages' image titles live in `shared/social-pages.ts`, whose paths are also the sitemap's entry points; guide titles come from `shared/docs.ts`.``

- [ ] **Step 5: 検証コマンド0〜5を実行し、差分を確かめる**

1. 検証コマンド0〜5がすべて成功すること。
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git diff -- web/server/routes/sitemap.xml.get.ts web/shared/social-pages.ts web/README.md web/tests/seo.spec.ts`を実行し、次を確かめる。
   - `sitemap.xml.get.ts`の差分が、1・5・6行目だけであること
   - `social-pages.ts`の差分が、コメント1行の追加だけであること
   - `seo.spec.ts`の差分が、新しいテストの追加だけであること
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short`を実行し、C13で変わったファイルが上の4つだけであることを確かめる。C04のファイルは数えない。
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .`を実行する（AGENTS.mdの規則）。

コミットメッセージの案（オーケストレーター用）。2つに分けるなら、`test: サイトマップの全入口で共有画像が表示できることを特性テストで固定する`と`refactor: サイトマップの入口を固定の共有画像を持つページの一覧から作る`。1つにまとめるなら後者だけを使う。

## 検証コマンド

0. `pgrep -af '[n]uxt|[p]laywright|[o]utput/server/index.mjs'` → 何も出ないこと（角かっこは、このコマンド自身に一致させないため）。出た場合は、他のサイクルのビルドやテストが終わるまで待つ。
1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck` → エラーなし
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build` → 終了コード0
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/seo.spec.ts tests/share-preview.spec.ts tests/policy.spec.ts tests/guides.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-c13` → すべて`passed`、`skipped`と`failed`は0
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run package:lambda` → 終了コード0（出力先は`.output-lambda`なので、3の`.output`は上書きされない）
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run test:lambda` → 終了コード0（`/og/docs/markdown.png`の確認を含む）

## 完了条件

- Step 1のテストが、変更前のコードで成功している（Step 2）。
- Step 3〜4の後、最後のコード変更より後に実行した検証コマンド1〜5がすべて成功している。
- Step 5の差分の確認を満たしている。

## 範囲外

- safetyの案の`{ path, title, kind }[]`の新設と、`socialPages`の組み立て直し（判定で採らない）
- ogルート、`useSharePreview`、`terms.vue`/`privacy.vue`、`shared/docs.ts`（1〜2行目のコメントは変更後も正しい）
- ページ側のog:titleと`socialPages`のタイトルの重複（worthの理由4。og:titleの組み立てが変わる）
- `share-preview.spec.ts`への対象の追加（新しいテストがsitemapの全入口を対象にする）
- `docs/refactor/PROGRESS.md`の更新（オーケストレーターが行う）

## レポートで触れる別件（直さない）

- D23（タグで絞り込んだ問題・記事一覧のog:imageが404のURLを指す）は保留のまま。この変更では触れない。
- `/featured`、`/featured/schedule`、`/featured/standings`は公開ページだが、sitemapにも共有画像にも入っていない。入れるかどうかは製品判断。変更後に入れる場合、`socialPages`に足せばsitemapにも入る。
