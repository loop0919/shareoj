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
| C03 | BFFのエラー変換を切り出し、画面へ渡すエラーコードを名前付きの一覧にする | `web/server/utils/private-api.ts` | 低・高・S | 修正・維持 | 計画済み（エラーコードの一覧を1行1コードの名前付き集合にし、変換を特性テストで固定） |
| C04 | useProblemDraftの既定値と自動保存の再予約をまとめる | `web/app/composables/useProblemDraft.ts` | 低・高・S | 維持・修正 | 完了（`dbc7784`） |
| C05 | ジャッジの失敗判定と上限検証をそれぞれ1か所にまとめる | `judge/sandbox.py`、`judge/host.py`、`judge/interactive.py` | 低・中・S | 修正・維持 | 計画済み（失敗判定だけをまとめる。上限検証の共通化は採らない） |
| C06 | コンテスト提出の閲覧条件をSQL断片1つにまとめ、2つの一覧で共有する | `api/internal/submissions/contest.go`、`problem.go` | 中・高・S | 維持・維持 | 完了（`da26be1`） |
| C07 | 提出とリジャッジで重複する「採点できる下書きか」の条件を1か所で生成する | `api/internal/submissions/store.go`、`rejudge.go` | 中・高・M | 修正・修正 | 計画済み |
| C08 | 問題とプロフィールのストアを具体型で持ち、実行時の型アサーションをやめる | `api/internal/httpapi/routes.go`ほか | 中・高・M | 修正・修正 | 完了（`57b902c`） |
| C09 | 「利用できる言語」の判定と既定の言語を1か所にまとめる | `web/app/composables/useJudgeCatalog.ts`ほか | 低・中・S | 修正・維持 | 計画済み |
| C10 | 自分の問題の詳細と一覧のスキーマを共通化 | `web/shared/types/account-problems.ts` | 低・中・S | 却下・却下 | 却下（変更漏れの根拠が履歴の読み違いで、共通化するとサーバー側の差が隠れる） |
| C11 | problems.scanが読む列のリストを共有する | `api/internal/problems/store.go`ほか | 低・中・S | 修正・維持 | 未着手 |
| C12 | 問題ページとコンテストの問題ページで、下書きから公開用の項目へ写す処理を共有する | `api/internal/problems/publication.go`、`api/internal/contests/store.go` | 低・中・S | 維持・維持 | 完了（`1bd2582`） |
| C13 | サイトマップと共有画像の静的ページ一覧を1つにする | `web/server/routes/sitemap.xml.get.ts`、`web/shared/social-pages.ts` | 低・中・S | 修正・修正 | 計画済み（固定ページ7件の写しを`socialPages`のキーにまとめる） |
| C14 | 送り出しの順序と待ち時間の起点を1つのSQL断片で定義する | `api/internal/submissions/store.go`、`api/cmd/judge-bridge/` | 低・中・S | 却下・維持 | 却下（変更漏れの実例がなく、既に振る舞いのテストで固定済み） |
| C15 | コンテストAPIの、パス末尾とメソッドの順序に依存する分岐をルートごとに分ける | `api/internal/httpapi/contests.go`、`routes.go` | 中・高・L | 修正・修正 | 保留（`contest()`を分けると、`%2F`を含むURLの応答（D01）が200から404に必ず変わる。外部動作の変更になるので人間の判断を待つ。挙動の変わらない部分だけでは効果が低い） |
| C16 | `host.judge()`をコンパイル・実行・集計の段階に分ける | `judge/host.py`、`judge/interactive.py` | 中・高・L | 却下・修正 | 却下（部分点の読み取りは既に1か所にあり、残る違いは2経路で本当に違う処理。規模とジャッジ中心部の変更に見合う効果がない） |
| C17 | ContestEditorのフォームとAPIの変換を純粋関数にする | `web/app/components/ContestEditor.vue` | 中・高・M | 修正・修正 | 計画済み（12個のrefを1つのフォームにし、読み込みを純粋関数へ。本文の組み立てはコンポーネントに残す） |
| C18 | 3つのMarkdown編集画面の表示切替・ツールバー・分割ハンドルを部品にする | `web/app/pages/problems/new.vue`ほか | 中・高・L | 却下・修正 | 却下（引用されたコミットの大半は部品化の対象に触れていない） |
| C19 | ScoringEditorの採点方式の切り替え（退避と復元）を純粋関数に分ける | `web/app/components/ScoringEditor.vue` | 中・中・M | 却下・修正 | 却下（変更履歴は機能を作った当日の作業で、保守の痛みの根拠にならない。提案の関数は純粋にならない） |
| C20 | マイグレーションの一覧を埋め込みFSとファイル名から組み立てる | `api/internal/database/database.go` | 中・中・M | 修正・修正 | 計画済み（範囲を一覧の関数化と、ファイル番号との整合テストに限定。embed.FSへの移行はしない） |

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
- 軽微（未対応）: 追加テストは`errors.As`で判定するため、デコードエラーを包み直す変更は検出しない。

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
- 変更: `handlerDependencies.Store`、`problemHandler.Store`、`submissions.Service.Problems`を`*problems.Store`に、プロフィールの3フィールドを`*profiles.Store`にした。16か所の実行時の型アサーション（それぞれに503の分岐）を、同じ位置の`nil`判定か、呼び出し側で判定済みの箇所では直接の呼び出しにした。使われなくなった`problems.Repository`、`problems.Publications`、`problemReadiness`、`profiles.Repository`を削除した。`git log -S "p.Store.("`ではこの形を写したコミットが14件あり、写し漏れはコンパイルを通る（常に503を返すか、readinessが応答から消える）。
- 守った点: ルートごとの入力検証と503の順序、エラーコード、release-on-readの省略、readinessの計算。テスト用の偽物が実在する`Files`のアサーションは残した。
- 特性テスト: `TestProblemRoutesWithoutDatabase`（ストアなしの35ルートのステータスとエラーコード）、`TestGenerationWithoutProblemStore`。変更前のコードで成功し、ストアをインターフェースに戻す変異（typed nil）を検出することを確かめた。
- simplify: テストのエラー確認を既存の`expect`にし、`releaseBefore`の引数名を`featured`から`problemStore`にした（単独公開の予約も解放するため）。
- レビュー: 4観点とも必須の指摘なし。
- 検証（最終tree）: API `go test -count=1 ./...`（DB連携）、`go vet`、`gofmt -l`、`make package package-judge`、webの`npm run build`、アカウント系Playwright（30件）がすべて成功。
- 軽微（未対応、変更前からの穴）: 認証付きの`release=true`ルート（`/my/problems/{id}/publication`など）で、期日を過ぎた予約と定期便が解放されることを確かめるテストがない。公開ルート側は既存のテストで検出できる。

## 保留事項

調査で見つけた別件の不具合と、仕様の判断が要る点。直さずに記録だけする。

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
| D13 | 保存ショートカットの判定がコンテスト編集画面だけ違う（IME変換中やShift付きでも保存） | `web/app/components/ContestEditor.vue:160-165` | 画面ごとに動きが違う |
| D14 | 入出力生成・入力検証の言語選択に「現在利用できません」の選択肢がない | `web/app/components/TestCaseGenerator.vue:153` | 保存済みの言語がカタログにないと選択欄が空に見える |
| D15 | Markdownエディターの画面ごとの差（プレビューの遅延、数式スニペット）とコンテスト編集画面だけテーマ切替がない点が意図的か不明 | `web/app/pages/problems/new.vue:86-97`、`ContestEditor.vue:531-545` | 画面によって入力の重さと挿入される記法が違う |
| D16 | `exportProblemMarkdown`と`draftErrors`の一部は本番コードから使われていない | `web/app/utils/problem-draft.ts:111-120` | 消すとテストも消すため、人間の判断が要る |
| D17 | テスター招待ページが、テスターも公開できると説明している | `web/app/pages/my/tester-invitations/[token].vue:42` | 実際にはない権限を伝えている |
| D18 | 定期便の順位表と予定表で、Hard枠にUltimateの表示名を使っていない | `web/app/pages/featured/index.vue:23` | ADR 0011と表示が食い違う |
| D19 | マイページと公開ページで、登録月のタイムゾーンが違う | `web/app/pages/my/index.vue:16` | 月末・月初の登録で表示がずれる |
| D20 | ホームの定期便のお知らせは2026-10-31を過ぎると表示されなくなる | `web/app/pages/index.vue:24-26` | 期限後にコードの整理が要る |
| D21 | BFFがAPIの`invalid_accounts`を画面へ渡さず、専用の文言が出ない | `web/server/utils/private-api.ts:32` | 汎用の「保存できませんでした」が出る |
| D22 | テストファイルのBFFルートが、ルートパラメーターを検証せずに上流のパスへ埋め込む | `web/server/api/my/problems/[id]/test-files/[file].get.ts:8`ほか | `%2e%2e`がURL解析で正規化される可能性（未検証。多層防御の穴） |
| D23 | タグで絞り込んだ一覧ページの`og:image`が404のURLを指す | `web/app/pages/problems/index.vue:34,44` | 共有時にプレビュー画像が出ない |
| D24 | 問題の編集画面で、ダイアログを閉じるときに保留中の自動保存タイマーを消さずに張り直す | `web/app/composables/useProblemDraft.ts`（離脱確認と削除確認を閉じる処理） | ダイアログの裏で草稿が非同期に変わると（保存して移動の途中のファイル準備、生成の完了）、タイマーが2本になりPUTが1回多く起きうる |

## 現在の作業と次の手順

- 計画済み（いずれも計画レビュー承認）: C03、C05、C07、C09、C13、C17、C20。次はC03の実装と、C11の計画（C08のコミット後）。
- C11はC02・C08と同じ`api/internal/problems/store.go`に触れるため、その後に計画する。
- C15〜C20は2観点の反証を行い、C15を保留、C16・C18・C19を却下、C17・C20を範囲を絞って採用した。
- 2026-10-20 03:40ごろ利用上限で一時停止し、08:30に再開した。停止時に失敗したエージェント（C04のsimplify、C07の計画レビュー、C20の反証）は、ワークフローの再開で実行し直した。完了済みの作業は重複して実行していない。
- 前の候補のレビュー中に、次の候補の計画を読み取りだけで進める（計画は`.local/`に書くので、固定したtreeに影響しない）。
- Ruling（C04）: 計画は「閉じるときに保留中のタイマーを消す」微差を受け入れていたが、ダイアログの裏で草稿が非同期に変わったときのPUTの回数が変わるため採らない。`scheduleAutosave()`は`clearTimeout`を含めず、呼び出し側の`clearTimeout`は今の位置のまま残す。誤っていた場合の損失: 二重のタイマー（計画のD2）が別件として残る。
