# B9 最後の修正 Implementation Plan

> 最終レビュー（`77d6c8f..3751baa`）とB1〜B8のレビューで残った点を直す。実装者は1人で、各タスクの最後に自分でコミットする。

**Goal:** 最終レビューの必須の指摘（F1）と、記録しておいた小さい修正4件を片付ける。

**Spec:** `docs/refactor/FIXES.md`（各項目の変更内容と判断の記録）

## Global Constraints

- FIXES.mdの「守ること」に従う。変える挙動のテストを先に書き、変更前のコードで失敗することを確かめる。
- 1タスク1コミット。メッセージは`prefix: 対応内容`の形で、末尾に`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`を付ける。
- 下に書いたこと以外は変えない。

## Review Focus

1. D07: JEにする更新が失敗したときも、成功したときも、記録される理由と段階名が実際に起きたことと合う。
2. D31: 非ASCIIの戻り先を符号化しても、外部のサイトへは飛ばない（`//`、`\`、制御文字は今までどおり`/my`）。
3. D12: Javaの対話ジャッジのヒープの値は今と同じで、判別だけが明示の引数になる。

---

### Task 1: F1（D03）CLIとBFFが送るContent-Typeをテストで固定する

**Files:** `cli/main_test.go`（`TestPushLifecycle`の偽API）、`web/tests/test-file-routes.spec.ts`、必要なら`web/tests/fixtures/public-api.mjs`のエコー

- CLI: 偽APIで、本文のある要求（`POST …/test-files`と`PUT /my/problems/{id}`など）について、`mime.ParseMediaType(r.Header.Get("Content-Type"))`が`application/json`であることを確かめる。今のCLIは付けているので最初から通る（守る挙動のテスト）。付けない形に一時的に変えると落ちることを確かめ、元に戻してから記録する。
- BFF: テストファイルの開始の要求が上流に`application/json`で届くことを、エコーが返す型で確かめる（エコーが型を返さないなら足す）。
- コミット例: `test: CLIとBFFがテストファイルの開始にapplication/jsonを付けて送ることをテストで固定する`

### Task 2: D12 Javaの対話ジャッジのヒープを明示の引数で判別する

**Files:** `judge/runner/sandbox.py`、`judge/runner/interactive.py`、`judge/tests/`の該当テスト

- B3の簡潔化（`015ea60`）の後、Javaの対話ジャッジ側の`-Xmx`だけが`memory==256`の値から決まっている。他の処理系と同じく`interactor`の明示の引数で決める。値（対話側は今と同じ、提出側は比例）は変えない。
- 対話側でメモリの値を変えても対話側のJavaの値が変わらないことを確かめるテストを先に足す（変更前は落ちる）。

### Task 3: D07 JEにしたときの記録を、実際に起きたことと合わせる

**Files:** `api/cmd/judge-bridge/dispatch.go`、`api/cmd/judge-bridge/dispatch_test.go`

- 今は`finishAsJE`がUPDATEの前に理由（`job_invalid`、`test_files_invalid`）を記録し、段階名は`outbox_read`や`test_files_resolve`のまま。UPDATEが失敗すると、JEにできていないのに理由が残り、段階名も紛らわしい。
- UPDATEが成功してから理由を記録し、UPDATEの失敗は専用の段階名（例: `finish_je`）で記録する。他の失敗の段階名は変えない。
- UPDATEの失敗を起こすテスト（例: テーブルの権限やトリガーで更新を拒む）で、理由が記録されず、`finish_je_failed`だけが出ることを確かめる。

### Task 4: D31 Googleログインの戻り先の非ASCIIを符号化する

**Files:** `web/server/routes/auth/google/callback.get.ts`、`web/tests/account/google.spec.ts`

- `loginDestination`を通った`next`に、U+00FFより大きい文字が生のまま入っていると、`sendRedirect`がLocationヘッダーで例外を投げ、ログイン失敗の画面になる。`sendRedirect`に渡す前に`new URL(next, 'http://x')`の`pathname + search + hash`にして符号化する（既存の`%xx`は二重にしない）。
- テスト: `next=/グ`のような戻り先でGoogleログインを終えると、符号化した同じパスへ戻る（変更前は`/login?socialError=failed`）。

### Task 5: D17 web/README.mdのテスターの権限の説明を実装に合わせる

**Files:** `web/README.md`（130行目付近）

- 「テスターは編集、公開、削除…を作者と同じ権限で行える」を、招待ページ（D17で直した文）と`web/app/content/docs/authoring.md`の説明に合わせる。公開・公開予約・定期便の応募は作者だけ。
- 文書だけの変更なのでテストは不要。コミット例: `docs: web/READMEのテスターの権限の説明を実装に合わせる`

## 検証

コントローラーが全体（API、CLI、judge、web）の検証を流す。実装者は触った領域のテストを流す（Playwrightはロックを取って、変えたspecだけ）。

## Out of scope

- 「23:00 (JST)」と「翌22時」のように表記が混ざる文、開発者向けの文書の「日本時間」（利用者の判断を待つ）。
- 64 MiBなどの小さいヒープで各処理系が実機で起動できるかの確認（isolateの実機が要る）。
- プロフィール未作成の利用者で、戻り先がログイン方法によって違う件（変更前から）。
