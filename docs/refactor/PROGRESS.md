# 保守性リファクタリングの進行記録

外部動作を保ったまま保守性を上げるリファクタリングの記録。
各サイクルの完了時と中断前に更新する。

## 開始時の状態

| 項目 | 値 |
| --- | --- |
| 開始日 | 2026-10-19 |
| 開始コミット | `f8b4507`（`main`。origin/mainより1コミット先行。未コミット変更なし） |
| 作業ブランチ | `refactor/maintainability` |
| worktree | `/home/loop/judge/.claude/worktrees/refactor-maintainability` |
| worktreeの除外 | `.claude/worktrees/`を`.git/info/exclude`へ追加（ローカルのみ。`main`は変更していない） |
| テスト用DB | worktreeの`.local/pg`に専用PostgreSQL 17（127.0.0.1:25432、DB `openoj_test`と`openoj_web`）。開発用DB（15432）は使わない |

push、PR作成、マージ、デプロイ、本番AWS・本番DBの操作はしない。

## 維持条件

- 公開HTTP API（ルート、JSONの形、ステータスコード、クライアントに見えるエラー文言）、CLIのコマンド・フラグ・出力、DBスキーマ、依存ライブラリのバージョン、インフラ構成を変えない。
- 認可、公開範囲、提出状態の遷移、重複・再実行時の挙動、ジャッジ結果・隔離・時間計測の意味を変えない。
- テストの削除・スキップや、検証ルールの緩和をしない。
- 別件の不具合や仕様判断が要る問題は「保留事項」に記録し、直さない。

## 検証コマンド

すべて`nix develop`のシェルから実行する（素のNode 20ではNuxt 4が動かない）。

| 対象 | コマンド | 作業ディレクトリ |
| --- | --- | --- |
| API | `TEST_DATABASE_URL=<api用DB> go test ./...`、`go vet ./...`、`make package package-judge` | `api` |
| CLI | `go test ./...`、`go vet ./...`、`CGO_ENABLED=0 go build -trimpath -o <出力先> .` | `cli` |
| judge | `python3 -m unittest discover -s judge/tests` | ルート |
| web | `npm ci`、`npm run typecheck`、`npm run build`、`npm run package:lambda`、`npm run test:lambda`、`node --test scripts/smoke-frontend.test.mjs` | `web` |
| webブラウザー | `npm run build`の後に`CI=1 TEST_DATABASE_URL=<web用DB> npx playwright test --output=<書き込める場所>` と `--config playwright.account.config.ts` | `web` |
| Terraform | `terraform fmt -check -recursive infra`（validate・testはプロバイダー取得が要るため、インフラを変えない今回は対象外） | ルート |
| 参考（CI外） | `golangci-lint run ./...`、`gofumpt -l .` | `api`、`cli` |

## ベースライン（`f8b4507`）

| 検証 | 結果 |
| --- | --- |
| API `go test ./...`（DB連携あり） | 成功 |
| API `go vet ./...` | 成功 |
| API `make package package-judge` | 成功 |
| CLI test・vet・build | 成功 |
| judge unittest | 成功（142件） |
| web typecheck・build・package:lambda・test:lambda・smoke | 成功 |
| webブラウザー（Playwright 2系統、並行実行） | 成功（ブラウザー450件、アカウント30件） |
| `gofmt -l`（api、cli） | 差分なし |
| 参考: golangci-lint | 開始前から指摘あり（api 4件、cli 28件）。CIでは実行していない |
| 参考: gofumpt | 開始前から未整形あり（api 20ファイル、cli 6ファイル）。CIでは実行していない。触れたファイルも一括整形はしない |

## 候補と状態

9領域を読み取り専用で調べ（49件）、重複を統合して20件に順位を付けた。
上位14件は「価値」と「安全性」の2つの観点から、それぞれ独立に反証を試みた。
各候補の詳細（根拠の行、変更案、反証の結果）はworktreeの`.local/refactor/candidates/`に置いた（git管理外）。
計画は`docs/refactor/plans/`に置く。

状態: 未着手 / 計画中 / 実装中 / 完了 / 保留 / 却下

| ID | 内容 | 主なファイル | リスク・効果・規模 | 反証（価値・安全） | 状態 |
| --- | --- | --- | --- | --- | --- |
| C01 | CLI pushの下書き合成を関数に切り出し、規則を特性テストで固定 | `cli/main.go` | 低・高・S | 修正・修正 | 完了（`0c17328`） |
| C02 | SQLの文字列置換をやめ、テスター用一覧と提出詳細の非公開列を明示的に組み立てる | `api/internal/problems/store.go`、`api/internal/submissions/store.go` | 低・高・S | 修正・維持 | 完了（`f3f44a4`。範囲を`problems/store.go`の`list()`に限定し、submissions側は除外） |
| C03 | BFFのエラー変換を切り出し、画面へ渡すエラーコードを名前付きの一覧にする | `web/server/utils/private-api.ts` | 低・高・S | 修正・維持 | 完了（`e5661c0`） |
| C04 | useProblemDraftの既定値と自動保存の再予約をまとめる | `web/app/composables/useProblemDraft.ts` | 低・高・S | 維持・修正 | 完了（`dbc7784`） |
| C05 | ジャッジの失敗判定と上限検証をそれぞれ1か所にまとめる | `judge/sandbox.py`、`judge/host.py`、`judge/interactive.py` | 低・中・S | 修正・維持 | 完了（`47afc9a`。失敗判定だけをまとめ、上限検証の共通化は採らない） |
| C06 | コンテスト提出の閲覧条件をSQL断片1つにまとめ、2つの一覧で共有する | `api/internal/submissions/contest.go`、`problem.go` | 中・高・S | 維持・維持 | 完了（`da26be1`） |
| C07 | 提出とリジャッジで重複する「採点できる下書きか」の条件を1か所で生成する | `api/internal/submissions/store.go`、`rejudge.go` | 中・高・M | 修正・修正 | 完了（`67fe6b4`） |
| C08 | 問題とプロフィールのストアを具体型で持ち、実行時の型アサーションをやめる | `api/internal/httpapi/routes.go`ほか | 中・高・M | 修正・修正 | 完了（`57b902c`） |
| C09 | 「利用できる言語」の判定と既定の言語を1か所にまとめる | `web/app/composables/useJudgeCatalog.ts`ほか | 低・中・S | 修正・維持 | 完了（`2d1807a`） |
| C10 | 自分の問題の詳細と一覧のスキーマを共通化 | `web/shared/types/account-problems.ts` | 低・中・S | 却下・却下 | 却下（変更漏れの根拠が履歴の読み違いで、共通化するとサーバー側の差が隠れる） |
| C11 | problems.scanが読む列のリストを共有する | `api/internal/problems/store.go`ほか | 低・中・S | 修正・維持 | 完了（`18b879d`） |
| C12 | 問題ページとコンテストの問題ページで、下書きから公開用の項目へ写す処理を共有する | `api/internal/problems/publication.go`、`api/internal/contests/store.go` | 低・中・S | 維持・維持 | 完了（`1bd2582`） |
| C13 | サイトマップと共有画像の静的ページ一覧を1つにする | `web/server/routes/sitemap.xml.get.ts`、`web/shared/social-pages.ts` | 低・中・S | 修正・修正 | 完了（`b5ad5e5`。固定ページ7件の写しを`socialPages`のキーにまとめる） |
| C14 | 送り出しの順序と待ち時間の起点を1つのSQL断片で定義する | `api/internal/submissions/store.go`、`api/cmd/judge-bridge/` | 低・中・S | 却下・維持 | 却下（変更漏れの実例がなく、既に振る舞いのテストで固定済み） |
| C15 | コンテストAPIの、パス末尾とメソッドの順序に依存する分岐をルートごとに分ける | `api/internal/httpapi/contests.go`、`routes.go` | 中・高・L | 修正・修正 | 保留（`contest()`を分けると、`%2F`を含むURLの応答（D01）が200から404に必ず変わる。外部動作の変更になるので人間の判断を待つ。挙動の変わらない部分だけでは効果が低い） |
| C16 | `host.judge()`をコンパイル・実行・集計の段階に分ける | `judge/host.py`、`judge/interactive.py` | 中・高・L | 却下・修正 | 却下（部分点の読み取りは既に1か所にあり、残る違いは2経路で本当に違う処理。規模とジャッジ中心部の変更に見合う効果がない） |
| C17 | ContestEditorのフォームとAPIの変換を純粋関数にする | `web/app/components/ContestEditor.vue` | 中・高・M | 修正・修正 | 完了（`7669f38`。12個のrefを1つのフォームにし、読み込みを純粋関数へ。本文の組み立てはコンポーネントに残す） |
| C18 | 3つのMarkdown編集画面の表示切替・ツールバー・分割ハンドルを部品にする | `web/app/pages/problems/new.vue`ほか | 中・高・L | 却下・修正 | 却下（引用されたコミットの大半は部品化の対象に触れていない） |
| C19 | ScoringEditorの採点方式の切り替え（退避と復元）を純粋関数に分ける | `web/app/components/ScoringEditor.vue` | 中・中・M | 却下・修正 | 却下（変更履歴は機能を作った当日の作業で、保守の痛みの根拠にならない。提案の関数は純粋にならない） |
| C20 | マイグレーションの一覧を埋め込みFSとファイル名から組み立てる | `api/internal/database/database.go` | 中・中・M | 修正・修正 | 完了（`46b6720`。範囲を一覧の関数化と、ファイル番号との整合テストに限定。embed.FSへの移行はしない） |

上位20件の枠外とした候補（offsetの検証、CLIのコマンド表など）と除外理由は`.local/refactor/dropped.json`にある。
`judge/rollout.py`の整理は、本番AWSなしでは重要な挙動を確かめられないため除外した。

## サイクルの進め方

1件ごとに次の順で進める。作業ツリーを編集する担当は常に1人にする。

1. 計画: 計画担当が候補の資料と実コードから短い計画を書き、別の担当が読み取り専用で計画をレビューする（人間の確認の代わり）。
2. 実装: 実装担当が特性テストを先に足し、変更前のコードで成功することを確かめてから変更する。コミットはしない。
3. simplify: メインセッションから`simplify`を実行する（再利用・単純化・効率・深さの4観点を並列でレビューし、妥当なものだけ適用）。
4. 差分の固定: `git add -A`と`git write-tree`でtreeを記録し、以後はそのtreeを検証・レビューする。
5. 検証とレビュー: 検証担当が検証コマンドを実行する。挙動・安全・設計・`code-review`の4観点で独立にレビューし、判定担当が指摘を実コードで確かめて「必須」「軽微」「却下」に分ける。必須の指摘は新しい修正担当が直し、範囲を絞った再レビューで確かめる（最大3回。同じ指摘が2回続けて残れば中断）。最後のコード変更の後に検証をやり直す。
6. コミット: 記録したtreeと一致することを確かめてからコミットし、この記録を更新する。

実行に使うワークフローのスクリプトと各サイクルの記録（計画、実装レポート、simplifyの結果、レビュー結果、検証ログ）は、worktreeの`.local/refactor/`に置く（git管理外）。採用した計画は`docs/refactor/plans/`に写す。

## 完了したサイクル

### C01 CLI pushの下書き合成（`0c17328`）

- 計画: [plans/C01-cli-merge-draft.md](plans/C01-cli-merge-draft.md)。計画レビューは承認（軽微な指摘4件）。
- 変更: `push`の合成部分（27行）を`mergeDraft(local, remote draft) error`へ処理順も中身も変えずに移した。docコメントに規則の要約と、順序の理由（合成はcopyで上書きされるremoteの値を読む、testCasesの参照に置き換わる前に呼ぶ）を書いた。
- 特性テスト: `TestPushMergeRules`（generatorsの役割ごとの更新、全部省いたときの保持、nullの扱い、判定器の置き換え、省いた項目の保持）、`TestScoringPushMerge`（停止時点と文言、版の競合の優先、不正なremoteのscoringとgenerators）。変更前のコードで成功し、変異（直接書き込み、null判定や排他削除の削除、呼び出し位置の移動など）をすべて検出することを確かめた。
- simplify: docコメントの理由の修正、`strings.Replace`が一致しないときの検出、テストの繰り返し整理を適用。共通ヘルパー化などは範囲外として見送り。
- レビュー: 4観点で挙動の変化はなし。必須の指摘3件（generatorsを全部省いたpushのテストがない、remoteのgeneratorsのデコード失敗を通るテストがない、docコメントに規則の要約がない）を1回の修正で解消。
- 検証（最終tree）: `cli`で`go test -count=1 ./...`、`go vet ./...`、`CGO_ENABLED=0 go build -trimpath`、`gofmt -l .`がすべて成功。
- 軽微（未対応）: 追加テストは`errors.As`で判定するため、デコードエラーを包み直す変更は検出しない。既存の`cli/scoring_test.go:236`のコメント「push stops before any request」は不正確（実際にはGETが先に行われる。変更前からの記述）。

### C06 コンテスト提出の閲覧条件（`da26be1`）

- 計画: [plans/C06-contest-submission-visibility.md](plans/C06-contest-submission-visibility.md)。計画レビューは承認（軽微な指摘4件。実装時に反映）。
- 変更: `contestPublic`と`problemSubmissionVisible`に写しで書かれていた4条件（公開、スタッフ、終了後の本人、開始後のチームメイト）を`contestSubmissionVisible(contest string) string`にまとめた。規則を変えた直近4コミットはどれも両ファイルを同時に直していた。
- 守った点: `contestPublic`の生成SQLはバイト単位で同じ。問題側の差は`(SELECT contest_staff(c.id,$3))`と`IN (SELECT unnest(...))`の2か所の書き換えだけ（WHERE内のORで意味は同じ）。変更前後のSQL文字列とEXPLAINを一時テストで比べた（一時ファイルは削除済み）。
- 特性テスト: 問題ページ側（コンテスト指定あり・なし）で、終了後のスタッフ・本人・チームメイトの見え方を5件固定。引数の取り違え（`c.id`の代わりに`NULLIF($2,'')::uuid`）を2件のテストが検出することを確かめた。
- simplify: 引数の約束（1つのコンテストに固定する呼び出しはパラメーター、複数にまたがる呼び出しは`c.id`）を関数のdocコメントに移した。問題側の副問い合わせが行ごとに評価される点は、関数の仕事が同じで差が小さいため見送り。
- レビュー: 必須1件。断片内の`created_at`の表名修飾が外れても、テストのフィクスチャ（作成後に開始時刻を過去へ動かす）では検出できなかった。2つのテストでコンテストの作成時刻を開始前に置き、変異を検出するようにした。
- 検証（最終tree）: API `go test -count=1 ./...`（DB連携）、`go vet`、`gofmt -l`、`make package package-judge`、webの`npm run build`、アカウント系Playwright（30件）がすべて成功。
- 軽微（未対応）: コンテスト外の枝の`(SELECT contest_staff($1::uuid,$3))`を副問い合わせにする理由のコメントが、simplifyで消えた。本人とチームメイトの枝の「開始後」の条件を丸ごと消しても検出するテストがない（変更前からの穴）。ほかのテストのフィクスチャも、作成後に開始時刻を過去へ動かすだけである。

### C12 下書きから問題ページへ写す項目（`1bd2582`）

- 計画: [plans/C12-publication-copy.md](plans/C12-publication-copy.md)。計画レビューは承認（軽微な指摘3件。実装時に反映）。
- 変更: `problems.Store.PublicGet`と`contests.Store.Problem`が下書きから写していた9項目を`func (p *PublicProblem) SetStatement(d Draft)`にまとめた。解説とタグは2つのページで隠し方が違うので呼び出し側に残し、その理由をdocコメントに書いた。項目の追加（740404e、ddbc289）は両方を同時に直していた。
- 特性テスト: `TestContestProblemStatementPostgres`（新規）。コンテストの問題ページ（匿名で開催中・終了後、オーナー）と公開後の問題ページで同じ項目が出ること、omitemptyの扱い、コンテスト側でタグを出さないこと、解説の出し分けを固定した。`SetStatement`にタグを入れる変異を3か所で検出する。Scoringは既存の`TestPartialScoringProblemsPostgres`が両方の経路で確かめる。
- simplify: テストの省略キーを`want`から導くようにし、パスとidの二重指定をなくした。docコメントを「両方のページが必ず見せる項目を置く」という規則にした。
- レビュー: 必須1件。docコメントがコンテスト側の解説の条件を「日程で決まる」とだけ書いていたので、実際の条件（終了後、またはsetter・作者・テスター。定期便の解禁前は伏せる）に直した。
- 検証（最終tree）: API `go test -count=1 ./...`（DB連携）、`go vet`、`gofmt -l`、`make package package-judge`、webの`npm run build`、アカウント系Playwright（30件）がすべて成功。
- 別件（記録のみ）: 項目を足すときは、Goの型と`SetStatement`に加えて、`web/shared/types/problem.ts`と`web/server/api/problems/[id].get.ts`（非公開の問題の表示で下書きから同じ項目を写す）も変える必要がある。

### C04 新しい問題の草稿の既定値と自動保存の予約（`dbc7784`）

- 計画: [plans/C04-problem-draft-defaults.md](plans/C04-problem-draft-defaults.md)、裁定: [plans/C04-rulings.md](plans/C04-rulings.md)。計画レビューは承認（軽微な指摘2件）。
- 変更: `useProblemDraft`が初期化と読み込み失敗時のリセットに写しで持っていた既定値を`initialProblemDraft()`に、3か所の600msの予約を`scheduleAutosave()`と`autosaveDelayMs`にまとめた。両方を同時に直したコミットは10件以上あった。
- Ruling: 計画が受け入れていた「閉じるときに保留中のタイマーを消す」微差は採らず、`clearTimeout`の位置は呼び出し側のまま残した（D24に記録）。自動保存の時刻とPUTの回数はどの場合も変わらない。
- 特性テスト: 読み込み失敗で既定値に戻ること（T1）、離脱ダイアログと削除確認を閉じた後に600msで1回だけ保存すること（T2、T3。`page.clock`で599/600msの境界を固定）、手動保存だけの問題で編集やダイアログの後もタイマーを張らないこと（T4）。2つの変異（既定値の`tags`を抜く、`manualSaveOnly`の判定を外す）を検出することを確かめた。後者は既存のテストでは検出できなかった。
- simplify: `scheduleAutosave`のコメントを守るべき制約が分かる内容にし、T2・T3の同じ確認と`manual-problem-save.spec.ts`の写した準備をまとめ、使われなくなった2つの定数の`export`を外した。
- レビュー: 必須1件。通常の編集（watch）経路で手動保存の問題にタイマーを張らないことを確かめるテストがなかったので、T4に足した（変異で検出を確認）。
- 検証（最終tree）: webの`npm run typecheck`、`npm run package:lambda`、`npm run test:lambda`、`npm run build`、ブラウザーPlaywright（455件）、アカウント系Playwright（30件）、`node --test scripts/smoke-frontend.test.mjs`がすべて成功。
- 軽微（未対応）: T2・T3は`runFor(2000)`の直後に同期で記録を読むため、遅れて届くPUTを見逃しうる。編集とダイアログの開閉が同じ時刻なので、境界の起点（閉じた時点か編集した時点か）は区別できない。どちらも今回変えていない箇所の退行に限られる。

### C02 テスター用の問題一覧のSQL（`f3f44a4`）

- 計画: [plans/C02-explicit-private-columns.md](plans/C02-explicit-private-columns.md)。計画レビューは承認（軽微な指摘4件）。範囲は`problems.Store.list`だけに絞った（submissions側の置換は、位置が5回の変更で一度も動いておらず、2定数に分けるとScanとの対応が読みにくくなるため除外）。
- 変更: 作者用SQLの末尾約70文字を目印に`strings.Replace`でテスター用SQLを作っていたのをやめ、`summaryColumns`、`ownedSummaries`、`testedSummaries`の3定数の連結で書く。547e225と74b6ed0は、目印とNULLの個数を同時に直していた。
- 守った点: 生成されるSQLは両方ともバイト単位で同じ（変更前のソースから組み立てた文字列との比較を、一時テストで実施。リポジトリには残していない）。
- 特性テスト: テスター一覧のカーソルによるページ送り（`TestTesterInvitationsPostgres`に追加）。テスター用だけカーソルを外す変異を検出する。
- simplify: テストの匿名構造体を既存の`cursorResponse[problems.Summary]`にした。`list`の`testing bool`をやめる案は計画どおり見送り。
- レビュー: 4観点とも指摘なし。
- 検証（最終tree）: API `go test -count=1 ./...`（DB連携）、`go vet`、`gofmt -l`、`make package package-judge`、webの`npm run build`、アカウント系Playwright（30件）がすべて成功。
- 備考: レビューの`code-review`観点は、サブエージェントからスキルを呼んでも結果が返らないことがあり、その場合はレビュー担当が同じ手順で行っている。最後の累積レビューでは、メインセッションから`code-review`を実行する。

### C08 問題とプロフィールのストアを具体型で持つ（`57b902c`）

- 計画: [plans/C08-concrete-store-types.md](plans/C08-concrete-store-types.md)。計画レビューは承認（軽微な指摘3件）。
- 変更: `handlerDependencies.Store`、`problemHandler.Store`、`submissions.Service.Problems`を`*problems.Store`に、プロフィールの3フィールドを`*profiles.Store`にした。16のハンドラーと`registerRoutes`の計17か所の実行時の型アサーション（ハンドラーの14か所は503の分岐、`private_problems.go`の2か所は黙ってreadinessを省く形）を、同じ位置の`nil`判定か、呼び出し側で判定済みの箇所では直接の呼び出しにした。使われなくなった`problems.Repository`、`problems.Publications`、`problemReadiness`、`profiles.Repository`を削除した。`git log -S "p.Store.("`ではこの形を写したコミットが14件あり、写し漏れはコンパイルを通る（常に503を返すか、readinessが応答から消える）。
- 守った点: ルートごとの入力検証と503の順序、エラーコード、release-on-readの省略、readinessの計算。テスト用の偽物が実在する`Files`のアサーションは残した。
- 特性テスト: `TestProblemRoutesWithoutDatabase`（ストアなしの35ルートのステータスとエラーコード）、`TestGenerationWithoutProblemStore`。変更前のコードで成功し、ストアをインターフェースに戻す変異（typed nil）を検出することを確かめた。
- simplify: テストのエラー確認を既存の`expect`にし、`releaseBefore`の引数名を`featured`から`problemStore`にした（単独公開の予約も解放するため）。
- レビュー: 4観点とも必須の指摘なし。
- 検証（最終tree）: API `go test -count=1 ./...`（DB連携）、`go vet`、`gofmt -l`、`make package package-judge`、webの`npm run build`、アカウント系Playwright（30件）がすべて成功。
- 軽微（未対応、変更前からの穴）: 認証付きの`release=true`ルート（`/my/problems/{id}/publication`など）で、期日を過ぎた予約と定期便が解放されることを確かめるテストがない。公開ルート側は既存のテストで検出できる。

### C03 画面へ渡すエラーコードの一覧（`e5661c0`）

- 計画: [plans/C03-bff-error-codes.md](plans/C03-bff-error-codes.md)。計画レビューは承認（軽微な指摘4件。実装時に反映）。
- 変更: `privateAPI`の中の1行の配列（50コード。`git log -L`で21件のコミットが行全体を置き換えていた）と429用の3コードを、モジュールレベルの`forwardedCodes`と`retryAfterCodes`（1行1コードの`Set`）にした。判定は`.has()`にした。50件と3件はHEADから機械的に写し、順序込みの差分がないことを確かめた。
- 特性テスト: 上流の拒否5通り（400で一覧にないコード、400で来た429用の3コードの1つ＝捨てられる、429で3コードの1つ、429で一覧にないコード、通さないステータス500で一覧のコード）をBFFに通し、ステータス、Retry-Afterが付かないこと、`data`、診断文字列が漏れないことを確かめる（`web/tests/upstream-errors.spec.ts`と`fixtures/public-api.mjs`）。Retry-Afterを渡す経路は既存の`creation-quota.spec.ts`と`submission.spec.ts`が固定している。
- simplify: `forwardedCodes`のコメントに、`retryAfterCodes`という例外を名指しで書いた（そのままだと429のコードを誤って一覧に足しうる）。
- レビュー: 4観点とも必須の指摘なし。
- 検証: 最初の実行でブラウザーテスト`tags.spec.ts:114`が1件失敗した。修正担当と再レビュー担当が、変更前のコードでも再現する既存の競合（D26）で今回の変更の経路を通らないことを確かめ、コードは変えずに再実行した。最終tree（変更前と同じtree）で、`npm run typecheck`、`npm run package:lambda`、`npm run test:lambda`、`npm run build`、ブラウザーPlaywright（460件）、アカウント系Playwright（30件）、smokeがすべて成功。
- 軽微（未対応）: `retryAfterCodes`のうち`contest_attempts_limited`だけはBFFを通るテストがない（変更前からの穴）。D25を記録した。

### C05 ジャッジの失敗判定（`47afc9a`）

- 計画: [plans/C05-judge-verdicts.md](plans/C05-judge-verdicts.md)。計画レビューは承認（軽微な指摘3件）。上限検証の共通化（briefの(b)）は価値の反証で外した。
- 変更: 提出の失敗判定（OLE→MLE→TLE→RE）が`interactive.failure`、`host.case_result`、集計コードの3か所に写されていたので、`sandbox.failure_verdict(metrics)`にまとめた。本体は`interactive.failure`と同じ。relayの式は参照先が変わっただけで、合計バイト数のOLEを先に判定する順序はそのまま。
- 特性テスト: `case_result`の13通り、集計コードの4行、relayの8行で優先順位を固定した。変更前のコードへの変異12個をすべて検出し、そのうち9個は既存のテストでは検出できなかった。
- simplify: docstringを実際の役割（実行の資源・終了の失敗の分類。判定用コードにとっての意味は呼び出し側が決める）に直した。
- レビュー: 必須1件。relayで合計バイト数のOLEを`failure_verdict`より先に判定する順序がテストで固定されていなかった（計画と実装は「決定的には固定できない」としていたが誤り）。状態を持つ`observe`で順序を固定するテストを足し、順序を入れ替える変異で3件とも失敗することを確かめた。
- 検証（最終tree）: `python3 -B -m unittest discover -s judge/tests`（145件）、`make package-judge`が成功。isolateの実機での確認（`judge/smoke.py`など）は本番環境が要るため実行していない。
- 軽微（未対応）: REの条件のうち、statusだけが真の行（exitCode・signalが0）がテストの表にない。isolateのメタデータではREとSGに必ずexitcodeかexitsigが付くので、実害はほぼない。

### C07 提出とリジャッジの受け付け条件（`67fe6b4`）

- 計画: [plans/C07-gradable-draft-condition.md](plans/C07-gradable-draft-condition.md)。計画レビューは承認（軽微な指摘3件。実装時に反映）。
- 変更: `CreateRun`と`Rejudge`に14行ずつ写されていた「採点できる下書きか」の条件（メモリ、判定用コード、対話ジャッジ、部分点）を`judgeReady(draft, runtime, checkerRuntimes, sampleCheck string) string`にまとめた。部分点の追加（d4909a6）とgroupsの条件（2c9923a）は2か所へ手で同じ差分を入れていた。ケース数の条件は呼び出し側に残した。
- 守った点: `CreateRun`の生成SQLはバイト単位で同じ。`Rejudge`は`false OR `が1か所増えるだけで、三値論理でも結果は同じ。パラメータ番号、引数の順番、`ErrNotReady`と0件の区別、ジョブJSON。
- 特性テスト: `TestSubmitAndRejudgeAgreeOnReadiness`（28通りの下書き×提出・サンプル・ローカル・リジャッジ）。変更前のコードで成功し、引数の取り違えの変異を検出することを確かめた。
- simplify: `judgeReady`のdocコメントに`runtime`引数の役割（ローカルのDocker採点の制限）を移し、テストのIDを固定値にした。ケース数の条件も含める案は、`CreateRun`のバイト一致を崩すので見送り。
- レビュー: 4観点とも必須の指摘なし。
- 検証（最終tree）: API `go test -count=1 ./...`（DB連携）、`go vet`、`gofmt -l`、`make package package-judge`、webの`npm run build`、アカウント系Playwright（30件）がすべて成功。
- 軽微（未対応）: `judgeReady`のdocコメントの1文目は、受け付け条件がすべてここにあるように読めるが、ケース数の条件（1〜100件）は呼び出し側に残っていて、書き方も2通りある。ケース数の上限を片方だけ変えても特性テストは検出しない。

### C20 マイグレーションの一覧（`46b6720`）

- 計画: [plans/C20-migration-list.md](plans/C20-migration-list.md)。計画レビューは承認（軽微な指摘4件）。
- 変更: `Migrate`の中の1行に並んでいた40個の一覧を、1行1要素の非公開関数`migrations()`に移し、規則（version Nは N番目、末尾に追加、消さない・並べ替えない、取り消したものは番号を予約）をコメントに書いた。`database.go`は38回変更されており、1行の一覧では差分で順序を追えなかった。
- 守った点: 識別子の列はHEADと同じ40件。`Migrate`の処理（トランザクション、advisory lock、適用済みの判定）、embed変数、SQLファイルは変えていない。
- テスト: `TestMigrationsFollowFileNumbers`（DB不要）。`.sql`ファイルが001から欠番も重複もなく並び、`migrations()[i]`がi+1番のファイルと同じ内容で、件数が一致することを確かめる。新しい関数を呼ぶので変更前のコードではビルドできないため、移し替えの同値性はHEADとの識別子の比較と、変更前後のDBテストで確かめた。
- simplify: テストを`filepath.Glob`と番号の接頭辞の比較1つに書き直した（計画にない小文字の名前規則を課していた正規表現をやめた）。書き直し後も、入れ替え・末尾の削除・未登録のファイル・番号の飛び・重複の変異をすべて検出する（レビュー担当が独立に確認）。
- レビュー: 4観点とも必須の指摘なし。
- 検証（最終tree）: API `go test -count=1 ./...`（DB連携。各テストが`Migrate`でスキーマを作る）、`go vet`、`gofmt -l`、`make package package-judge`（migrate Lambdaを含む）がすべて成功。
- 軽微（既知の限界）: ファイルと一覧を一緒に付け替える誤り（例: 019を消して後ろを詰める）は検出できない。本番の`schema_migrations`は番号で記録しているので適用されなくなる。規則はコメントに書いた。直すならリリース済みの版を固定する別の仕組みが要る。

### C11 問題の読み書きが返す列（`18b879d`）

- 計画: [plans/C11-problem-scan-columns.md](plans/C11-problem-scan-columns.md)。計画レビューは承認（軽微な指摘3件。実装時に反映）。
- 変更: `scan`が読む8列が6つのクエリ（Get、SaveのINSERTとUPDATE、Publishの2つ、Schedule）に手で写されていたので、定数`problemColumns`にまとめた。0acd889、c118c35、190fe16は全部を同時に直していた。計画担当の確認では、公開のクエリの作者の式だけを壊しても既存のテストはすべて通っていた。
- 守った点: Get・Save・Publishの列は変更前とバイト単位で同じ（Publishは空白だけの違い）。WHERE、SET、プレースホルダ、引数、エラーの変換は変えていない。Scheduleのcontest列は定数`''`からサブクエリになるが、予約中の問題は行ロックと事前の確認で同じトランザクション中にコンテストへ入れず、コンテストの保存も`scheduled_at IS NULL`で行ロックを取るので、結果は常に`''`（論証による確認。競合の並びの実行での再現はしていない）。
- 特性テスト: `TestProblemWriteResponsesMatchReadPostgres`（作成・更新・予約・公開・取り下げの応答と直後のGET、作者・版・公開版・予約の具体値）、`TestProblemReadAnswersContestPostgres`（コンテストに入った問題のGETのトップレベルの`contestId`）。
- simplify: 任意の指摘2件（既存の補助関数を使う、Publishの2クエリを1つにする）は、テストを弱めるか範囲を超えるため見送り。
- レビュー: 必須1件。共有した定数のcontest列が壊れても、どのテストも失敗しなかった（既存のテストは部分文字列の比較で、`contestEditors`の欄に一致していた）。トップレベルの`contestId`を直接確かめるテストを足し、変異で検出できることを確かめた。
- 検証（最終tree）: API `go test -count=1 ./...`（DB連携）、`go vet`、`gofmt -l`、`make package package-judge`、webの`npm run build`、アカウント系Playwright（30件）がすべて成功。

### C09 利用できる言語の判定と既定の言語（`2d1807a`）

- 計画: [plans/C09-judge-catalog-availability.md](plans/C09-judge-catalog-availability.md)。計画レビューは承認（軽微な指摘4件。実装時に反映）。
- 変更: メンテナンス中や取得失敗時に言語の一覧を空にする規則（4つのコンポーネントに写されていた。5943ab4で4つ目を追加）を`usableRuntimes(catalog, error)`に、新しい判定用コードの既定の言語の式（2か所）を`preferredRuntime(items)`にまとめた（`web/app/utils/judge-catalog.ts`）。式は元のまま移した。
- 守った点: 各画面の選択肢、「（現在利用できません）」の選択肢、既定の言語、`useJudgeCatalog()`、D14。レビュー担当が元の式と新しい関数を1000通り以上の入力で比べ、差がないことを確かめた。
- 特性テスト: `web/tests/judge-catalog.spec.ts`（E2E 6件。メンテナンス中でも言語を並べる応答で、4画面がフラグに従うことと、既定の言語の選び方を確かめる。nodeテスト2件）。既存の`maintenance.spec.ts`は空の一覧を返すため、画面がフラグを見なくなっても検出できなかった。変異（1画面だけフラグを見ない）を検出することを確かめた。取得失敗（502）では、Nuxtがdataを消すため`error`の配線の誤りをE2Eで見分けられず、nodeテストだけが固定している。
- simplify: 2つのE2Eで写していた10手順を`expectNewCode`にまとめ、コメントを短くした。
- レビュー: 4観点とも必須の指摘なし。
- 検証（最終tree）: webの`npm run typecheck`、`npm run package:lambda`、`npm run test:lambda`、`npm run build`、ブラウザーPlaywright（468件）、アカウント系Playwright（30件）、smokeがすべて成功。
- 軽微（未対応）: nodeテストの`preferredRuntime([{ id: '' }, ...])`はAPIが返さない入力で`??`を固定している。`usableRuntimes`の`error`の型が`unknown`なので、呼び出し側が`.value`を付け忘れても型で防げない（今の4か所は正しい）。

### C13 サイトマップの入口と共有画像の固定ページ（`b5ad5e5`）

- 計画: [plans/C13-static-page-list.md](plans/C13-static-page-list.md)。計画レビューは承認（軽微な指摘4件。実装時に反映）。
- 変更: `sitemap.xml.get.ts`の直書きの配列（固定ページ7件とガイド。`socialPages`のキーの写し）を`Object.keys(socialPages)`にした。`social-pages.ts`に順序の約束をコメントで書き、`web/README.md`の誤った記述（ガイドのタイトルの置き場所）を直した。
- 守った点: 生成されるサイトマップのXMLはバイト単位で同じ（レビュー担当がsiteUrlを4通りに変えて比べた）。キーはすべて`/`で始まるので列挙は挿入順になり、既存のテストが順序込みの完全一致で固定している。
- 特性テスト: `every sitemap entry point advertises a working share image`（全19入口のog:imageが200で1200×630のPNGを返す）。一覧から1件外す変異を検出することを確かめた。
- simplify: サイトマップ側のコメントから順序の記述を外した（順序の約束は`social-pages.ts`にだけ置く）。既存テストに統合する案は見送り。
- レビュー: 4観点とも必須の指摘なし。
- 検証（最終tree）: webの`npm run typecheck`、`npm run package:lambda`、`npm run test:lambda`、`npm run build`、ブラウザーPlaywright（469件）、アカウント系Playwright（30件）、smokeがすべて成功。
- 軽微（未対応、変更前からの穴）: 新しいテストは画像がそのページ自身のものかまでは確かめない（例: `/terms`が`/privacy`の画像を告げても通る）。足すなら各入口の`og:url`が入口と一致することを確かめる。

### C17 コンテスト編集画面のフォーム（`7669f38`）

- 計画: [plans/C17-contest-form-conversion.md](plans/C17-contest-form-conversion.md)。計画レビューは承認（軽微な指摘4件。実装時に反映）。
- 変更: `ContestEditor.vue`の12個のフォームのrefを`reactive(initialContestForm())`の1つにまとめ、未保存の判定を`JSON.stringify([fields, selected])`から自動で作るようにした。下書きと公開済みの読み込みの変換を`web/app/utils/contest-form.ts`の純粋関数（`formFromDraft`、`formFromContest`、`participationForm`）にした。直近の4コミットはどれも6〜7か所を同時に直しており、1db91bfは未保存の判定に`freezeOn`を入れ忘れた（bd03e84で修正）。
- 守った点: 送る本文の式とキー、公開済みの読み込み順（参加形式以外 → 会場の取得 → 参加形式）、watchの既定のflush、`<form>`のref、Ctrl+S（D13）、テーマ選択なし（D15）。テンプレートは`fields.`を除くと元とバイト単位で同じ。レビュー担当が実際のVueで旧実装と新実装を8641通りの読み込みで比べ、差がないことを確かめた。
- 特性テスト: `web/tests/contest-editor-form.spec.ts`（本文の全キー、スイッチだけの変更が未保存になること、読み込み直後に未保存にならないこと、合言葉の取得失敗時もタイトルと日程が残ること、Ctrl+S）と`web/tests/contest-form.spec.ts`（純粋関数）。bd03e84の退行の再現と、読み込み順の変異を検出することを確かめた。
- simplify: `participationForm`を設定のオブジェクトと合言葉の2引数にし、新規の既定値の二重定義をなくし、読み込み順の理由をコンポーネント側に書いた。
- レビュー: 4観点とも必須の指摘なし。
- 検証（最終tree）: webの`npm run typecheck`、`npm run package:lambda`、`npm run test:lambda`、`npm run build`、ブラウザーPlaywright（480件）、アカウント系Playwright（30件）、smokeがすべて成功。
- 軽微: `formFromContest`のコメント「会場のルートから参加形式を読む」は不正確だった（会場から取るのは合言葉だけ。simplifyで入れた文面）。最終レビューの修正（`f10cc24`）で直した。未対応: `setOnsite`の「オンにしたとき既存の合言葉を残す」分岐はテストがない（変更前から）。

## 全体の再調査（1回目）

- 5領域を、これまでの候補・却下・保留・枠外の一覧と保留事項を除いて、独立した担当が読み取り専用で調べ直した（C17の実装と並行）。
- 4領域（httpapi、APIのドメインとcmd、CLIとjudge、webのserver・shared・scripts）は「1サイクルに見合う候補なし」。web/appの1件（N01: 問題とコンテストの提出一覧の4列を部品にまとめる）は、価値の観点で反証が成り立ち却下した（変更漏れの実例がない、同じ写しの3か所のうち2か所しか覆えない、ずれても見た目だけで既存テストが固定している）。
- 新しく見つかった別件を保留事項D27〜D32に記録した。

## 保留事項

調査で見つけた別件の不具合と、仕様の判断が要る点。直さずに記録だけする。
根拠の行番号は記録した時点のもの（D01〜D23は開始コミット`f8b4507`、D24以降と見直したD13〜D15・D29は最終tree）。

| ID | 内容 | 根拠 | 影響 |
| --- | --- | --- | --- |
| D01 | 提出idに`%2F`を含めると、コンテスト提出詳細のルートが順位表を返す | `api/internal/httpapi/contests.go:88` | 異常なURLだけ。順位表の権限はストアで確認するので漏れはない |
| D02 | 運営者の参加登録の応答だけ`canRevealResults`が付かない | `api/internal/httpapi/contests.go:60-62` | 画面で公開ボタンが一時的に出ない程度 |
| D03 | JSON本文のContent-Type判定がエンドポイントごとに3通りある | `api/internal/httpapi/problems.go:17`、`auth.go`、`profile.go:73` | 一部のエンドポイントでだけ415になるヘッダーがある |
| D04 | GETルートに一致するHEADが、メソッドで分岐するハンドラーの書き込み側に入る | `api/internal/httpapi/`のメソッド分岐 | 認可は通るので権限昇格はないが、HEADで書き込みが起きうる |
| D05 | プロフィール保存のJSON読み取りが共通処理の写しで、上限超過に413でなく400を返す | `api/internal/httpapi/profile.go:84-99` | エンドポイントでステータスが違う |
| D06 | アカウント削除で、残った下書きと同じidの公開済みコンテストの関連行まで消える可能性 | `api/internal/accounts/store.go:41-44` | lockID導入前の競合でしか生じないはず |
| D07 | 送り出しの先頭の提出が恒久的に失敗すると、期限切れ（6時間）まで後続が止まる | `api/cmd/judge-bridge/dispatch.go:65-80` | 起きる可能性は低いが、起きると全提出の採点が止まる |
| D08 | deployment-statusの期間のコメントが実装と一致しない | `api/cmd/judge-bridge/bridge.go:48-56` | 挙動は安全側。コメントを信じた修正が危険 |
| D09 | `login`・`logout`に余分な引数を渡すと、init/check/push向けの文言が出る | `cli/main.go`の`run`（`needsDirectory`の判定） | 文言が実際の誤りと合わない |
| D10 | 新規作成時の問題文と解説のひな形をCLIとWebが別々に持つ | `cli/main.go`の`initProblem`、`web/app/utils/problem-draft.ts:124,135` | 片方だけ変えるとずれる（494674fで手作業で同期した前例あり） |
| D11 | rolloutの`finish()`が1台構成でも`--instance`を全台分渡し、api-smokeで失敗する | `judge/rollout.py:416-418` | 1台構成の通常配布を支援するかの判断が要る |
| D12 | 処理系別のヒープ上限が「memory==256なら対話ジャッジ側」という前提のまま | `judge/sandbox.py:154-163` | 256 MiBの提出が小さいヒープで動き、MLEがREになりうる |
| D13 | 保存ショートカットの判定がコンテスト編集画面だけ違う（IME変換中やShift付きでも保存） | `web/app/components/ContestEditor.vue`の`saveWithShortcut`（149行） | 画面ごとに動きが違う |
| D14 | 入出力生成・入力検証の言語選択に「現在利用できません」の選択肢がない | `web/app/components/TestCaseGenerator.vue:154` | 保存済みの言語がカタログにないと選択欄が空に見える |
| D15 | Markdownエディターの画面ごとの差（プレビューの遅延、数式スニペット）とコンテスト編集画面だけテーマ切替がない点が意図的か不明 | `web/app/pages/problems/new.vue:86-97`、`ContestEditor.vue`の`editor-topbar`（507行） | 画面によって入力の重さと挿入される記法が違う |
| D16 | `exportProblemMarkdown`と`draftErrors`の一部は本番コードから使われていない | `web/app/utils/problem-draft.ts:111-120` | 消すとテストも消すため、人間の判断が要る |
| D17 | テスター招待ページが、テスターも公開できると説明している | `web/app/pages/my/tester-invitations/[token].vue:42` | 実際にはない権限を伝えている |
| D18 | 定期便の順位表と予定表で、Hard枠にUltimateの表示名を使っていない | `web/app/pages/featured/index.vue:23` | ADR 0011と表示が食い違う |
| D19 | マイページと公開ページで、登録月のタイムゾーンが違う | `web/app/pages/my/index.vue:16` | 月末・月初の登録で表示がずれる |
| D20 | ホームの定期便のお知らせは2026-10-31を過ぎると表示されなくなる | `web/app/pages/index.vue:24-26` | 期限後にコードの整理が要る |
| D21 | BFFがAPIの`invalid_accounts`を画面へ渡さず、専用の文言が出ない | `web/server/utils/private-api.ts`の`forwardedCodes` | 汎用の「保存できませんでした」が出る |
| D22 | テストファイルのBFFルートが、ルートパラメーターを検証せずに上流のパスへ埋め込む | `web/server/api/my/problems/[id]/test-files/[file].get.ts:8`ほか | `%2e%2e`がURL解析で正規化される可能性（未検証。多層防御の穴） |
| D23 | タグで絞り込んだ一覧ページの`og:image`が404のURLを指す | `web/app/pages/problems/index.vue:34,44` | 共有時にプレビュー画像が出ない |
| D25 | 上流が通さないステータス（500など）を返すとBFFは502にするが、一覧にあるコードは`data`に残して画面へ渡す | `web/server/utils/private-api.ts`（`upstream-errors.spec.ts`のf005が現状を固定） | 是非は未判断。変えると画面に出る文言が変わる |
| D26 | ブラウザーテスト`tags.spec.ts:114`が不安定（全体実行で時々失敗） | `web/tests/tags.spec.ts:124`の`getByRole('search')` | ページの作り直し中に古いフォームをつかんで撮影する競合。変更前のコードでも応答を25〜50ms遅らせると毎回再現した。テストで新しいページの見出しを待てば直る |
| D24 | 問題の編集画面で、ダイアログを閉じるときに保留中の自動保存タイマーを消さずに張り直す | `web/app/composables/useProblemDraft.ts`（離脱確認と削除確認を閉じる処理） | ダイアログの裏で草稿が非同期に変わると（保存して移動の途中のファイル準備、生成の完了）、タイマーが2本になりPUTが1回多く起きうる |
| D27 | 公開ルートのうち、コンテスト・公開提出・定期便のハンドラーだけ10秒の期限を付けていない | `api/internal/httpapi/contests.go`の公開側（`routes.go`の7ルート、順位表を含む）など。期限ありは`problems.go:79`、`posts.go:28`ほか | DBが遅いとき、これらだけLambdaの30秒まで実行が続く（API Gatewayは20秒で打ち切る）。揃えると応答が変わるため判断が要る |
| D28 | deployment-statusの「未完了の提出」だけがランタイムを接尾辞（`LIKE '%-isolate'`）で数え、送り出し・期限切れ（`runtime=ANY(...)`）と対象がずれる | `api/cmd/judge-bridge/bridge.go:56`、`dispatch.go:47-48,103-107` | ランタイムを一覧から外したとき、残った提出がpendingに残り続け、rolloutの完了待ちが終わらない可能性（推定） |
| D29 | 編集画面の「提出先の制限」で、公開予約中の問題だけ制限の注記が出ない | `web/app/components/ProblemReadiness.vue:10`の`stateCodes`に`scheduled`がない（`utils/problem-readiness.ts:50`の`stateIssues`にはある。190fe16で片方だけ追加） | 予約中の問題は、コンテストと定期便に出せないことが注記に出ない |
| D30 | チーム戦のサンプル検証の「結果の詳細」リンクだけ`?from=team`が付かない | `web/app/components/SubmissionForm.vue:120`（同じファイルの:64と`ProblemSubmissionList.vue:31`は付ける。a33d0acは:64だけ直した） | パンくずリストが「この問題の自分の提出」になる。意図的な可能性もある |
| D31 | ログイン後の戻り先の許可リストが、画面が付ける`next`の多くを受け付けず`/my`へ戻す | `web/shared/utils/login-destination.ts:3`（`/problems/post`、`/featured/apply`、`/problems/{短縮ID}`、`/my/contests`以下、`/my/submissions`を通さない）。呼ぶ側は`web/app/pages/problems/post.vue:88`ほか | 問題の投稿、定期便の応募、お気に入りなどからログインすると元のページに戻らない。許す範囲はオープンリダイレクト対策の判断が要る |
| D32 | BFFが画面へ通すエラーコードのうち5つ（`invalid_avatar`、`invalid_submission`、`contest_participation_unavailable`、`invalid_contest`、`password_unavailable`）を、どの画面も読んでいない | `web/server/utils/private-api.ts`の`forwardedCodes` | 実害はない。専用の文言を足すか一覧から外すかは仕様の判断 |
| D33 | 提出コードの長さの上限131072バイトが、検証（`judge/host.py:77`、`judge/sandbox.py:80`）と成果物の読み出し（`judge/sandbox.py:257`の`regular_read(..., 131072)`）に別々に書かれている | 左記の3か所（C05の計画で範囲外の報告事項とした） | 検証の上限だけを上げると、ソースが黙って切り詰められて誤ったRE・WAになりうる |
| D34 | 提出の`timeLimitMs`の不正値を拒否する挙動のテストがない | `judge/host.py`の上限検証（C05の計画で報告事項とした） | 検証の退行に気づけない |

## 最終レビュー（開始コミットからの累積差分）

- 範囲: `f8b4507..a536ad5`（29コミット、77ファイル）。Superpowersの`code-reviewer`の型で、独立したレビュー担当4人（API、CLIとjudge、web、サイクル間の相互作用と記録）が読み取り専用で確かめ、判定担当が指摘を実コードで確認した。あわせてメインセッションから`code-review`（high）を実行した。
- 結果: 維持条件を破る変更はなし（3人が「そのままマージ可」、1人が「文書の修正後に可」）。`code-review`も確かな正しさの退行はなし。
- 必須の指摘3件はすべてコメントと文書: `formFromContest`のコメントとテスト名の誤り（G1）、型の先頭へ移った「空ならnull」のコメントの誤り（G2）、裁定で覆した設計をそのまま書いたC04の計画（G3）。
- 修正（1回、`f10cc24`）: G1〜G3に加え、軽微のうち特性テストの穴（C11の応答のidとupdated_at、C05のstatusだけの行）、新しいテストで増えたlintの1件、`judgeReady`のdocコメントのケース数の条件、C02・C05の計画の誤りを直した。範囲を絞った再レビューで9件とも解消を確認し、新たな問題はなし。記録の誤り（C08の箇所数、C03のテストの説明、保留事項の行番号と並び、古くなった次の手順、計画にだけあった別件のD33・D34への移し替え）は、この記録で直した。
- `code-review`の指摘の扱い: 保留中のタイマー（D24。C04の裁定で外部動作を保つため意図して残した）は却下。ケース数の条件の写しはdocコメントで明記した。`contestSubmissionVisible`の閲覧者のパラメーター化、nil判定のラッパー、`list`の防御的な判定、サイトマップと共有画像の結び付き（C13で受け入れた設計）、Scheduleの副問い合わせ（C11で受け入れた差）、CLIの別名の変数は、新しい抽象化・まだない呼び出し側への備え・受け入れ済みのトレードオフ・好みの範囲として採らない。
- 残した軽微: `judgeReady`のdocコメントで、足した文の直後の「They」が指す先がやや曖昧。C02の計画の表の1セルで、判定と誤りの注記が並ぶ。どちらもコメントと文書だけで、2回目の修正はしない（Superpowersの手順）。

## 最終の検証（tree `24e4b6a`、コミット`f10cc24`）

すべて成功した。

| 対象 | 結果 |
| --- | --- |
| API `go test -count=1 ./...`（DB連携、ポート25432） | 10パッケージok、FAIL 0 |
| API `go vet ./...`、`gofmt -l .`、`make package package-judge` | 成功 |
| CLI `go test -count=1 ./...`、`go vet ./...`、`gofmt -l .`、`CGO_ENABLED=0 go build -trimpath` | 成功 |
| judge `python3 -B -m unittest discover -s judge/tests` | 145件 OK（開始時142件） |
| `terraform fmt -check -recursive infra` | 成功 |
| web `npm run typecheck`、`npm run package:lambda`、`npm run test:lambda`、`npm run build` | 成功 |
| web ブラウザーPlaywright | 480件成功（開始時450件。`tags.spec.ts:114`のD26は今回失敗しなかった） |
| web アカウント系Playwright（実際のGo APIとDB） | 30件成功 |
| web `node --test scripts/smoke-frontend.test.mjs` | 3件成功 |

実行していない検証: Terraformの`validate`と`test`（インフラを変えておらず、プロバイダーの取得が要る）、isolateの実機での確認（`judge/smoke.py`、`judge/interactive_smoke.py`。本番環境が要る）、本番への配布と`judge/smoke-api.py`（委任の範囲外）。参考のgolangci-lintは、api・cliとも開始時と同じ件数（新しく足したテストで増えた1件は直した）。

## 状態と次の手順

- 候補の処理は収束した。完了14件（C01〜C09、C11〜C13、C17、C20）、保留1件（C15）、却下6件（C10、C14、C16、C18、C19、再調査のN01）。全体の再調査（1回目）で新しい候補はなかった。
- 作業ブランチ`refactor/maintainability`にローカルでコミットした（push、PR、マージ、デプロイはしていない）。未完了の変更は残っていない。
- 人間の判断が要るもの: C15（`%2F`を含むURLの応答を変えてよいか）と、保留事項D01〜D34。
- 経緯: 2026-10-20 03:40ごろ利用上限で一時停止し、08:30に再開した。停止時に失敗したエージェント（C04のsimplify、C07の計画レビュー、C20の反証）は、ワークフローの再開で実行し直した。完了済みの作業は重複して実行していない。
- 進め方の補足: 前の候補のレビュー中に、次の候補の計画を読み取りだけで進めた（計画は`.local/`に書くので、固定したtreeに影響しない）。
- Ruling（C04）: 計画は「閉じるときに保留中のタイマーを消す」微差を受け入れていたが、ダイアログの裏で草稿が非同期に変わったときのPUTの回数が変わるため採らない。`scheduleAutosave()`は`clearTimeout`を含めず、呼び出し側の`clearTimeout`は今の位置のまま残す。誤っていた場合の損失: 二重のタイマー（D24）が別件として残る。
