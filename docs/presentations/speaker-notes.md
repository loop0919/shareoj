# ShareOJ 発表者ノート

想定時間は約15分（付録を除く）。

## 01 ShareOJ

約15分でShareOJの利用体験と採点基盤を紹介する。2026年9月22日のリポジトリを根拠とし、本番環境をこの資料作成時に再検証したものではない。

- [README.md](../../README.md)
- [web/app/pages/index.vue](../../web/app/pages/index.vue)

## 02 解く経験を、次の問題と知識へ

ShareOJはプログラミング問題を解き、作り、共有できるオンラインジャッジ。利用者は解答者だけでなく作問者やコンテスト主催者にもなれる。4つの機能を学習と創作のつながりとして紹介する。

- [README.md](../../README.md)

## 03 問題を読んで、提出し、結果を確かめる

画面そのもののスクリーンショットではなく、利用の流れを示す模式図。サンプル検証と本提出は別操作。進捗は実際の採点情報から表示し、結果詳細にはケースの判定や資源使用量がある。ACなどの表示は説明用の例で、実測や本番提出の結果ではない。

- [web/app/components/SubmissionForm.vue](../../web/app/components/SubmissionForm.vue)
- [web/app/components/SubmissionStatus.vue](../../web/app/components/SubmissionStatus.vue)
- [web/app/content/guides/language-guide.md](../../web/app/content/guides/language-guide.md)

## 04 作問を支える編集と検証

問題編集画面には問題文、解説、テストケース、生成、判定コード、管理の入口がある。Markdownと数式、生成コードと入力検証、未公開問題への提出、テスター招待を組み合わせて公開前に確認できる。古いweb/READMEの未実装という記述より現行コンポーネントを優先した。

- [web/app/pages/problems/new.vue](../../web/app/pages/problems/new.vue)
- [web/app/content/guides/generator-guide.md](../../web/app/content/guides/generator-guide.md)
- [api/README.md](../../api/README.md)

## 05 コンテストと記事で、学びを共有する

コンテストは主催者が開催期間、問題、配点、誤答ペナルティを設定する。公式順位は期間内にコンテストの問題ページから受け付けた提出が対象。記事は問題の解き方や学びを共有する場である。利用者数、継続率、開催実績などの定量効果は確認していないため記載しない。

- [web/app/pages/blog/contest-rules.vue](../../web/app/pages/blog/contest-rules.vue)
- [web/app/pages/blog/new.vue](../../web/app/pages/blog/new.vue)
- [README.md](../../README.md)

## 06 Webと採点基盤を分けた全体構成

Nuxt SSRとGo APIはそれぞれAPI Gateway HTTP APIとLambdaで公開。図ではHTTP APIをノード内の説明へまとめた。Go APIとbridgeはPostgreSQLに接続し、workerはSQSとS3を介して動く。線は主な要求またはデータ移動の向きを示す。結果はbridgeからDBへ反映され、画面側がAPI経由で取得する。

- [infra/README.md](../../infra/README.md)
- [docs/judge/execution-model.md](../../docs/judge/execution-model.md)
- [judge/README.md](../../judge/README.md)

## 07 1件の提出が結果になるまで

提出行が永続Outboxを兼ねる。bridgeは未配送の提出を読み、入力をS3に保存して要求をSQSへ送る。workerは入力とruntime digestを検証し、コンパイルが必要な言語では一度コンパイルした成果物をケース間で使う。進捗と最終結果を結果キューで返し、bridgeがDBへ反映する。UIのポーリングは現行SubmissionFormで1.5秒間隔。

- [api/internal/database/005_judge_outbox.sql](../../api/internal/database/005_judge_outbox.sql)
- [api/cmd/judge-bridge/dispatch.go](../../api/cmd/judge-bridge/dispatch.go)
- [api/cmd/judge-bridge/results.go](../../api/cmd/judge-bridge/results.go)
- [web/app/components/SubmissionForm.vue](../../web/app/components/SubmissionForm.vue)

## 08 再配信があっても、確定結果を守る

SQS Standardの再配信を前提にしている。exactly-once実行を保証するという意味ではない。配送時にはFOR UPDATE SKIP LOCKEDを使用。結果は提出IDと試行IDを照合し、確定済み提出は上書きしない。進捗の逆行もSQL条件で抑える。worker停止時は可視性期限後の再配信を使い、期限を過ぎた未完了提出はdispatch時にJEへ確定する。

- [api/cmd/judge-bridge/dispatch.go](../../api/cmd/judge-bridge/dispatch.go)
- [api/cmd/judge-bridge/results.go](../../api/cmd/judge-bridge/results.go)
- [api/internal/submissions/finish.go](../../api/internal/submissions/finish.go)
- [docs/judge/execution-model.md](../../docs/judge/execution-model.md)

## 09 提出コードをケースごとに隔離する

通常判定の模式図。isolateは名前空間、UID、cgroup v2で提出コードを隔離する。各ケースの作業領域と子孫プロセスを回収し、コンパイル成果物だけを次ケースへ渡す。期待出力や認証情報は解答プログラム側へ渡さない。スペシャルジャッジと対話用ジャッジには、それぞれ別の隔離環境へ必要なテストデータを渡す。共有カーネルなのでVM相当の隔離とは主張しない。

- [judge/README.md](../../judge/README.md)
- [docs/judge/execution-model.md](../../docs/judge/execution-model.md)
- [docs/judge/interactive-judge.md](../../docs/judge/interactive-judge.md)

## 10 資源を制限し、判定の根拠を記録する

通常提出の問題設定はCPU時間100〜5000ms、メモリ64〜512MiB。メモリは子孫と課金対象のキャッシュを含むcgroupピークで、RSSとは区別する。通常判定の経過時間上限はCPU上限の3倍+1秒。対話形式には別の経過時間計算がある。既存2台構成は1台1提出で最大2提出。同一ホスト内でコンパイルと通常提出のケース実行を重ねない。

- [docs/judge/runtime-policy.md](../../docs/judge/runtime-policy.md)
- [judge/README.md](../../judge/README.md)
- [docs/judge/interactive-judge.md](../../docs/judge/interactive-judge.md)

## 11 問題に合わせて選ぶ3つの判定方法

通常判定は期待出力との比較、スペシャルジャッジは作問者の検証コード、対話形式は提出と対話用ジャッジの双方向通信。チェッカーとインタラクターは別の隔離環境で実行し、資源を制限する。対話では両側の正常終了を待ってACにする。スコアファイルの値の集計や部分点を提供するという意味ではない。

- [docs/judge/special-judge.md](../../docs/judge/special-judge.md)
- [docs/judge/interactive-judge.md](../../docs/judge/interactive-judge.md)

## 12 言語環境を固定し、検証して公開する

言語一覧はリポジトリの言語ガイドに基づく例。受付可能なランタイムはGET /runtimesと提出欄が正とし、ここでは本番の公開状態を再確認していない。各言語の配布元、版、ライブラリをlockファイルで固定し、runtime名とdigestで採点条件を識別する。旧環境を保存して過去提出を再実行する仕組みは未実装。

- [web/app/content/guides/language-guide.md](../../web/app/content/guides/language-guide.md)
- [docs/judge/runtime-policy.md](../../docs/judge/runtime-policy.md)
- [judge/runtimes.py](../../judge/runtimes.py)

## 13 SSRと認証をサーバー側でつなぐ

Nuxtは問題文や数式を含むHTMLをSSRする。トークンはHttpOnly Cookieに保存し、NuxtサーバーからGo APIへBearerとして渡す。Cookieを使う書き込みにはOrigin検証を適用。Markdownの生HTMLや危険なURLを抑え、KaTeXはtrust:falseで描画する。認証済みであることに加え、Go APIは所有者や公開範囲などの認可を行う。

- [web/README.md](../../web/README.md)
- [api/README.md](../../api/README.md)
- [web/app/utils/problem-markdown.ts](../../web/app/utils/problem-markdown.ts)

## 14 配布の完了を、提出の成功まで確認する

WebとAPIはGitHub Actionsで型、ビルド、ブラウザー、実DBなどを検証する。ジャッジ更新はrollout.py runに統合され、受付停止、DBとキューの排出、配布、全言語検証、起動と設定同期、受付再開、本番提出の検証まで行う。SSMへ登録しただけでは完了としない。今回の作業では配布や本番提出は実施していない。

- [.github/workflows/ci.yml](../../.github/workflows/ci.yml)
- [docs/judge/deployment-checks.md](../../docs/judge/deployment-checks.md)

## 15 設計上の選択と、現在の制約

左の設計選択に対して、右に運用上の制約を対応させる。2台は既存配布対象としてドキュメントに記録されている構成であり、今回稼働状態を検査したものではない。RDS Single-AZはリポジトリの環境設定に基づく。性能やセキュリティの保証、無停止配布、無制限のスケーラビリティは主張しない。

- [judge/README.md](../../judge/README.md)
- [infra/README.md](../../infra/README.md)
- [docs/judge/runtime-policy.md](../../docs/judge/runtime-policy.md)
- [docs/judge/deployment-checks.md](../../docs/judge/deployment-checks.md)

## 16 考える楽しさを、次の一問へ。

締めは、使う側と技術側の説明を接続する。ShareOJは問題を解くだけでなく、作問、記事、コンテストまでをつなぐ。その下で、Web/API、永続化、非同期配送、隔離実行が役割を分担している。公開サイトとリポジトリへ案内する。

- [README.md](../../README.md)

## 17 参照資料と、このスライドの前提

各スライドの発表者ノートに参照ファイルを記載した。基準コミットはc59a2e1。旧ADRや初期実装の説明と現行コードに差がある箇所は、新しい運用資料と言語ガイド、実装を優先した。数値は設定値であり、今回の負荷試験の実測結果ではない。図は説明用であり、実画面のスクリーンショットではない。

- [README.md](../../README.md)
- [docs/judge/runtime-policy.md](../../docs/judge/runtime-policy.md)
- [docs/judge/deployment-checks.md](../../docs/judge/deployment-checks.md)
