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
| C01 | CLI pushの下書き合成を関数に切り出し、規則を特性テストで固定 | `cli/main.go` | 低・高・S | 修正・修正 | 未着手 |
| C02 | SQLの文字列置換をやめ、テスター用一覧と提出詳細の非公開列を明示的に組み立てる | `api/internal/problems/store.go`、`api/internal/submissions/store.go` | 低・高・S | 修正・維持 | 未着手 |
| C03 | BFFのエラー変換を切り出し、画面へ渡すエラーコードを名前付きの一覧にする | `web/server/utils/private-api.ts` | 低・高・S | 修正・維持 | 未着手 |
| C04 | useProblemDraftの既定値と自動保存の再予約をまとめる | `web/app/composables/useProblemDraft.ts` | 低・高・S | 維持・修正 | 未着手 |
| C05 | ジャッジの失敗判定と上限検証をそれぞれ1か所にまとめる | `judge/sandbox.py`、`judge/host.py`、`judge/interactive.py` | 低・中・S | 修正・維持 | 未着手 |
| C06 | コンテスト提出の閲覧条件をSQL断片1つにまとめ、2つの一覧で共有する | `api/internal/submissions/contest.go`、`problem.go` | 中・高・S | 維持・維持 | 未着手 |
| C07 | 提出とリジャッジで重複する「採点できる下書きか」の条件を1か所で生成する | `api/internal/submissions/store.go`、`rejudge.go` | 中・高・M | 修正・修正 | 未着手 |
| C08 | 問題とプロフィールのストアを具体型で持ち、実行時の型アサーションをやめる | `api/internal/httpapi/routes.go`ほか | 中・高・M | 修正・修正 | 未着手 |
| C09 | 「利用できる言語」の判定と既定の言語を1か所にまとめる | `web/app/composables/useJudgeCatalog.ts`ほか | 低・中・S | 修正・維持 | 未着手 |
| C10 | 自分の問題の詳細と一覧のスキーマを共通化 | `web/shared/types/account-problems.ts` | 低・中・S | 却下・却下 | 却下（変更漏れの根拠が履歴の読み違いで、共通化するとサーバー側の差が隠れる） |
| C11 | problems.scanが読む列のリストを共有する | `api/internal/problems/store.go`ほか | 低・中・S | 修正・維持 | 未着手 |
| C12 | 問題ページとコンテストの問題ページで、下書きから公開用の項目へ写す処理を共有する | `api/internal/problems/publication.go`、`api/internal/contests/store.go` | 低・中・S | 維持・維持 | 未着手 |
| C13 | サイトマップと共有画像の静的ページ一覧を1つにする | `web/server/routes/sitemap.xml.get.ts`、`web/shared/social-pages.ts` | 低・中・S | 修正・修正 | 未着手 |
| C14 | 送り出しの順序と待ち時間の起点を1つのSQL断片で定義する | `api/internal/submissions/store.go`、`api/cmd/judge-bridge/` | 低・中・S | 却下・維持 | 却下（変更漏れの実例がなく、既に振る舞いのテストで固定済み） |
| C15 | コンテストAPIの、パス末尾とメソッドの順序に依存する分岐をルートごとに分ける | `api/internal/httpapi/contests.go`、`routes.go` | 中・高・L | 未検証 | 未着手 |
| C16 | `host.judge()`をコンパイル・実行・集計の段階に分ける | `judge/host.py`、`judge/interactive.py` | 中・高・L | 未検証 | 未着手 |
| C17 | ContestEditorのフォームとAPIの変換を純粋関数にする | `web/app/components/ContestEditor.vue` | 中・高・M | 未検証 | 未着手 |
| C18 | 3つのMarkdown編集画面の表示切替・ツールバー・分割ハンドルを部品にする | `web/app/pages/problems/new.vue`ほか | 中・高・L | 未検証 | 未着手 |
| C19 | ScoringEditorの採点方式の切り替え（退避と復元）を純粋関数に分ける | `web/app/components/ScoringEditor.vue` | 中・中・M | 未検証 | 未着手 |
| C20 | マイグレーションの一覧を埋め込みFSとファイル名から組み立てる | `api/internal/database/database.go` | 中・中・M | 未検証 | 未着手 |

上位20件の枠外とした候補（offsetの検証、CLIのコマンド表など）と除外理由は`.local/refactor/dropped.json`にある。
`judge/rollout.py`の整理は、本番AWSなしでは重要な挙動を確かめられないため除外した。

## 完了したサイクル

なし。

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
| D09 | `login`・`logout`に余分な引数を渡すと、init/check/push向けの文言が出る | `cli/main.go:339-345` | 文言が実際の誤りと合わない |
| D10 | 新規作成時の問題文と解説のひな形をCLIとWebが別々に持つ | `cli/main.go:232-233`、`web/app/utils/problem-draft.ts:124,135` | 片方だけ変えるとずれる（494674fで手作業で同期した前例あり） |
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

## 現在の作業と次の手順

- 実行順の目安は、C01、C06、C12、C04、C02、C03、C05、C11、C09、C07、C08、C13。C15〜C20は着手前に反証を行う。
- 次はC01。
