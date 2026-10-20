# C03 画面へ渡すエラーコードを名前付きの一覧にする 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `web/server/utils/private-api.ts`で1行の配列リテラルに埋め込まれている2つのコード一覧（画面へ渡す50個、429でRetry-Afterと一緒に渡す3個）を、モジュールレベルの名前付き`Set`に移し、1行に1コードで並べる。

**Architecture:** 先に、BFFが上流のコードを落とす・渡す境目を固定する特性テスト5件を、fixtureと新しいspecに足す。そのあと2つの`Set`を`privateAPI`の前に置き、26行と32行の判定を`.has(upstreamCode ?? '')`に置き換える。制御フロー、ステータスの扱い、Retry-After、リフレッシュには触れない。形は`web/app/utils/runtime-label.ts`の`runtimeLabels`（1行のコメント、`new Map([`、1行1要素、末尾カンマ、`])`）に合わせる。

**Tech Stack:** Nuxt 4（Nitro）、TypeScript、Playwright

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/.local/refactor/candidates/C03.json`（worthの`revised_scope`が拘束。両方の`must_preserve`）

## 困りごと

- 画面へ渡す50コードが`private-api.ts:32`の1行にある（計画者が実コードで数えて50、重複なし）。`git log -L32,32`でこの1行を変えたコミットは21件ある。
- 1行全体が置き換わるので、diffでもblameでも、どのコードをいつ・なぜ足したり消したりしたかが読めない。例えばe5237acは`onsite_kept_open`を消しているが、1行の置き換えに埋もれている。
- 429用の3コード（26行）も同じ形で、3回変わっている。50個とは別の一覧であることがコードから読み取れない。

## 判定の記録

| 論点 | worth | safety | 判定 |
| --- | --- | --- | --- |
| `upstreamError`への切り出し | しない | 条件付きで可 | しない。`revised_scope`が拘束する。呼び出し元は1つで、webにはPlaywright以外のテスト基盤がない |
| 429用の3コード | 任意で名前付きにする | 別の一覧のまま保つ | `retryAfterCodes`にする。2つの一覧が別物であることを名前とコメントで示す。50個とはまとめない |
| 通すステータスの一覧（33行） | インラインのまま | 変えない | インラインのままにする |
| 機能別の分類コメント | 付けない | — | 付けない |
| 並び順 | 今の順かアルファベット順 | — | 今の順。変更前後の一致を、順序まで含めてdiffで確かめられる |
| 特性テスト | 一覧にないコードを1件 | (a)〜(f) | (a)、(d)、(e)と、429で一覧のコードが来る場合、500で一覧のコードが来る場合の5件にする。(c)のRetry-Afterの解釈と(f)のrefreshSessionは、変更しない行の挙動なので足さない |
| safetyの(b)の期待値 | — | 500→502、`data`はundefined | 誤り。32行は`status`に関係なく`code`を決め、33行は502でも`data: { code }`を付ける。Nitroの本番用ハンドラーも、`unhandled`/`fatal`でないエラーの`data`をそのまま返す（`node_modules/nitropack/dist/runtime/internal/error/prod.mjs:60`）。テストは実際の挙動（502で`{ code: 'team_full' }`）で固定する |
| テストの置き場所 | 既存のspecに1件 | 新しいspec | 新しい`web/tests/upstream-errors.spec.ts`にする。題材はBFFの変換で、凍結のspecに置くと探しにくい |
| コードコメントの言語 | — | — | 英語（周りに合わせる）。計画とレポートは日本語 |

## Global Constraints

- 公開HTTP API（ルート、JSONの形、ステータスコード、エラー文言）、DBスキーマ、依存のバージョン、CI・インフラを変えない。テストは追加だけにする。
- `forwardedCodes`の中身は、`7e19fc1`の32行にある50コードと、順序まで同じにする。`retryAfterCodes`の中身は、26行の3コード（`submission_rate_limited`、`creation_quota_exceeded`、`contest_attempts_limited`）と同じにする。`invalid_accounts`は足さない。
- 2つの一覧は別のまま保つ。3コードは`forwardedCodes`に入れない。
- 判定は`Set.has(upstreamCode ?? '')`で行う。オブジェクトのキーや`in`では引かない。
- `7e19fc1`から消える行は26行と32行の2行だけにする。新しい26行は`if (status === 429 && retryAfterCodes.has(upstreamCode ?? '')) {`、新しい32行は`const code = forwardedCodes.has(upstreamCode ?? '') ? upstreamCode : undefined`。次のものは変えない。
  - 通すステータス`[400, 401, 403, 404, 409, 413, 415, 429, 503]`と、それ以外を502にする扱い
  - `'Request failed'`と、`data`の3つの形（`{ code }`、`{ code, retryAfter }`、undefined）
  - Retry-Afterの解釈（27-29行）
  - トークンの選択、リフレッシュ、1回だけの再試行（4-23行）
- 2つの定数はexportしない。Nitroが`server/utils`のexportを自動インポートし、名前の衝突が実際に起きたことがある（4c7a319）。`^export`は`privateAPI`の1つだけのままにする。
- 定数は`import`と`privateAPI`の間に、`forwardedCodes`、`retryAfterCodes`の順に置く。2スペースで字下げし、1行に1コードを末尾カンマ付きで書き、`])`で閉じる。
- 定数の直前のコメントは英語にし、`forwardedCodes`には2行まで、`retryAfterCodes`には1行を書く。内容は次のとおり。
  - `forwardedCodes`：上流の診断を画面へ出さないための許可リストであること。画面に文言があるコードはここにも足すこと。
  - `retryAfterCodes`：429のときだけRetry-Afterと一緒に渡すこと。`forwardedCodes`には入っていないので、ほかのステータスでは落ちること。
- fixtureは追加だけにする。既存の経路（`88888888…/reveal`など）は変えない。
- 変えるファイルは`web/server/utils/private-api.ts`、`web/tests/fixtures/public-api.mjs`、`web/tests/upstream-errors.spec.ts`（新規）の3つだけにする。C12の作業中ファイル（`api/`）には触れない。
- `git commit`、`git stash`、`git reset`は実行しない。コミットはオーケストレーターが行う。

## Review Focus

1. 50コードを書き写すときの脱字・欠落・重複。特性テストは5件の抜き取りなので、これを検出できない。検出できるのは、Step 4-1の順序込みのdiff、件数50、1行1コードの数え上げだけである。
2. 2つの一覧の取り違え（3コードを50個に足す、1つのSetにまとめる）。→ f002（400で`creation_quota_exceeded`が来ると`data`はundefined）と、Step 4-2で検出する。
3. 429の分岐の条件や順序を変えてしまうこと。→ f003とf004（429でも3コード以外ならRetry-Afterを付けない）、既存の`creation-quota.spec.ts`と`submission.spec.ts`（3コードならヘッダーと`retryAfter`を付ける）で検出する。
4. 判定をオブジェクトのキーや`in`で行う、`?? ''`を落とす。→ Step 4-3（消えた行が2行だけであること）の差分を読んで確かめる。プロトタイプのキー（`constructor`など）のテストは足さない。
5. 定数をexportすること、ステータス一覧・502・`'Request failed'`への波及。→ Step 4-3、Step 4-4と、f005（500→502でも`{ code: 'team_full' }`）で検出する。

---

### Task 1: 2つのコード一覧を名前付きのSetにし、BFFの変換を特性テストで固定する

**Files:**
- Modify: `web/tests/fixtures/public-api.mjs`（`submission`定数の後に表を置き、`/my/`分岐の111行の直後で応答する）
- Create: `web/tests/upstream-errors.spec.ts`
- Modify: `web/server/utils/private-api.ts`（3行目の空行と4行目の`export`の間に2つの定数を挿入し、26行と32行を置き換える）

**Interfaces:**
- Produces: なし（モジュール内の`const forwardedCodes`、`const retryAfterCodes`。exportしない）

- [ ] **Step 1: fixtureとspecに特性テストを足す**

fixtureには、`POST /my/contests/<id>/reveal`が次の表どおりに断る5つのコンテストを足す。本文は`{ error }`だけで、`retryAfter`があるときは`Retry-After`ヘッダーを付ける。表の直前に、BFFのエラー変換用であることを1行のコメントで書く。

| id | 上流のステータス | `error` | `Retry-After` |
| --- | --- | --- | --- |
| `f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f001` | 400 | `private-upstream-diagnostic` | なし |
| `f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f002` | 400 | `creation_quota_exceeded` | なし |
| `f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f003` | 429 | `team_full` | `30` |
| `f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f004` | 429 | `private-upstream-diagnostic` | `30` |
| `f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f005` | 500 | `team_full` | なし |

`web/tests/upstream-errors.spec.ts`：

```ts
import { test, expect } from '@playwright/test'

// Each contest's reveal answers with one refusal from tests/fixtures/public-api.mjs.
const cases = [
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f001', name: 'drops a code the screen does not know', status: 400, data: undefined },
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f002', name: 'drops a rate-limit code that came without 429', status: 400, data: undefined },
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f003', name: 'passes a listed code on a 429 without the wait', status: 429, data: { code: 'team_full' } },
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f004', name: 'drops an unknown code and the wait on a 429', status: 429, data: undefined },
  { id: 'f0f0f0f0-f0f0-4f0f-8f0f-f0f0f0f0f005', name: 'answers 502 for an unexpected status and keeps a listed code', status: 502, data: { code: 'team_full' } },
]
for (const { id, name, status, data } of cases) {
  test(`the web server ${name}`, async ({ request }) => {
    const response = await request.post(`/api/my/contests/${id}/reveal`, { headers: { Cookie: 'openoj_access=valid-access', Origin: 'https://judge.example' } })
    expect(response.status()).toBe(status)
    expect(response.headers()['retry-after']).toBeUndefined()
    const text = await response.text()
    expect(text).not.toContain('private-upstream-diagnostic')
    expect(JSON.parse(text).data).toEqual(data)
  })
}
```

- [ ] **Step 2: 変更前にビルドし、特性テストと関連specが成功することを確かめる**

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git diff --quiet 7e19fc1 -- web/server/utils/private-api.ts && echo same` → `same`
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run build` → 成功
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 1200 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test tests/upstream-errors.spec.ts tests/contest-freeze.spec.ts tests/creation-quota.spec.ts tests/submission.spec.ts tests/session.spec.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c03-pw-before` → `failed`なし。新しい5件が`passed`に含まれる

失敗した場合の扱い：
- 本文に`private-upstream-diagnostic`が出た場合、または表にないコードが`data`に入った場合は、別件の不具合である。コードは直さずに報告して止まる。
- それ以外の不一致は、計画の読み違いとみなす。期待値を観測した値に直し、レポートに書く。

- [ ] **Step 3: `forwardedCodes`と`retryAfterCodes`を置き、26行と32行を置き換える**

Global Constraintsの配置・書式・コメント・新しい2行に従う。それ以外の行は1文字も変えない。

- [ ] **Step 4: 一覧が変わっていないことを機械的に確かめる**

以下の`<S>`は`/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad`である。コマンドでは展開して書く。作業ディレクトリは`/home/loop/judge/.claude/worktrees/refactor-maintainability`。

1. 50コードの比較
   - `git show 7e19fc1:web/server/utils/private-api.ts | sed -n 32p | grep -oE "'[a-z_]+'" > <S>/c03-forwarded-before.txt`
   - `sed -n '/^const forwardedCodes = /,/^])$/p' web/server/utils/private-api.ts | grep -oE "'[a-z_]+'" > <S>/c03-forwarded-after.txt`
   - `diff <S>/c03-forwarded-before.txt <S>/c03-forwarded-after.txt && wc -l < <S>/c03-forwarded-after.txt` → 差分なしで`50`
   - `sed -n '/^const forwardedCodes = /,/^])$/p' web/server/utils/private-api.ts | grep -cE "^  '[a-z_]+',$"` → `50`
2. 3コードの比較
   - `git show 7e19fc1:web/server/utils/private-api.ts | sed -n 26p | grep -oE "'[a-z_]+'" > <S>/c03-retry-before.txt`
   - `sed -n '/^const retryAfterCodes = /,/^])$/p' web/server/utils/private-api.ts | grep -oE "'[a-z_]+'" > <S>/c03-retry-after.txt`
   - `diff <S>/c03-retry-before.txt <S>/c03-retry-after.txt && wc -l < <S>/c03-retry-after.txt` → 差分なしで`3`
   - `sed -n '/^const retryAfterCodes = /,/^])$/p' web/server/utils/private-api.ts | grep -cE "^  '[a-z_]+',$"` → `3`
3. `git diff -U0 7e19fc1 -- web/server/utils/private-api.ts | grep -cE '^-[^-]'` → `2`。`git diff 7e19fc1 -- web/server/utils/private-api.ts`を読み、消えたのが26行と32行で、置き換えがGlobal Constraintsの2行どおりであることを確かめる
4. `grep -c '^export' web/server/utils/private-api.ts` → `1`。`grep -c invalid_accounts web/server/utils/private-api.ts` → `0`

- [ ] **Step 5: webの検証一式を実行する**

作業ディレクトリは`/home/loop/judge/.claude/worktrees/refactor-maintainability/web`。各コマンドの前に`nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command`を付ける。Playwrightは長いので、背景実行してよい。

1. `npm run typecheck` → エラーなし
2. `npm run build` → 成功
3. `npm run package:lambda` → 成功
4. `npm run test:lambda` → 成功
5. `npm run build` → 成功（3で`.output`がLambda用になるため、ビルドし直す）
6. `timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c03-pw` → `failed`も`flaky`もない。新しい5件を含む
7. `timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test --config playwright.account.config.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/c03-pw-account` → 30件すべて`passed`。`contest-teams.spec.ts`で、実際のGo APIが返すコードが画面へ渡ることを確かめる
   - このsuiteは`../api`をビルドする。`git status --short api`に他の候補の未コミットの変更があるときは実行せず、「検証者が実行」とレポートに書く
   - ポート15432は使わない

- [ ] **Step 6: 仕上げ**

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short` → C03で変わったのは、上の3ファイルだけであること
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .` → 成功（`graphify-out/`はgit管理外）

コミットメッセージの案（オーケストレーター用）：`refactor: 画面へ渡すエラーコードを名前付きの一覧にして1行に1コードで並べ、BFFの変換を特性テストで固定する`

## 完了条件

- 新しい5件の特性テストが、変更前（Step 2）と変更後（Step 5-6）の両方で成功している。
- Step 4の確認がすべて期待どおりである。2つの一覧は順序まで変更前と同じで、消えた行は2行、exportは1つ、`invalid_accounts`はない。
- typecheck、build、package:lambda、test:lambdaが成功し、Playwright（ブラウザー）に失敗がない。アカウント系は成功しているか、未実行の理由が記録されている。
- 変わったファイルは3つだけである。

## 範囲外と保留事項

- `upstreamError`への切り出しと、`privateAPI`の制御フローの変更。
- 通すステータスの定数化、機能別の分類コメント、並べ替え。
- `private-session.ts`の`refreshSession`にあるステータス一覧`[401, 429, 503]`。
- Retry-Afterの解釈とrefreshSessionの経路のテスト（候補の(c)と(f)）。この変更では触れない。
- 保留（別件の不具合）：`invalid_accounts`がBFFで落ちる。APIは400で返し（`api/internal/httpapi/profile.go:107`）、画面にも文言がある（`web/app/utils/profile.ts:4`）。一覧にないため、汎用の文言しか出ない。`dropped.json`に記録済み。
- 保留（テスト方針）：画面テストは`**/api/...`を直接モックしてBFFを通らない。そのため、BFFの転送漏れはテストで検出されない。
- 保留（仕様の確認）：上流が一覧にないステータスを返したとき、BFFは502にするが、一覧のコードは`data`に入れて渡す。f005はこの挙動をそのまま固定する。これが望ましいかは別に判断する。
