# Architecture Decision Records

Architecture Decision Record（ADR）は、採用した設計だけでなく、その設計が必要になった状況と代替案を記録する。

実装時に変更される設定値は仕様書へ置き、ADRには長く残る判断を記載する。

採用済みの判断を変更するときは既存のADRを書き換えず、新しいADRから置き換え対象を明示する。

## 記録一覧

| ID | 状態 | 決定 |
| --- | --- | --- |
| [0001](0001-use-aws-fargate-for-judge-execution.md) | Superseded by 0005 | 採点実行基盤にAWS Fargateを採用する |
| [0002](0002-standardize-judge-task-resources.md) | Superseded by 0005 | ジャッジタスクのリソースを統一する |
| [0003](0003-store-test-sets-in-amazon-s3.md) | Accepted（採点時の取得は0005で変更） | テストセットをAmazon S3へ保存する |
| [0004](0004-use-postgresql-as-primary-database.md) | Accepted | 主データベースにPostgreSQLを採用する |
| [0005](0005-use-firecracker-and-isolate-on-ec2.md) | Superseded by 0006 | EC2上のFirecrackerとisolateで採点する |
| [0006](0006-use-lightsail-and-isolate.md) | Superseded by 0012（isolateは継続） | Lightsailの2 GBインスタンスでisolateを使う |
| [0007](0007-limit-judge-runtime-libraries.md) | Accepted | 競技プログラミング向けのランタイム構成を限定する |
| [0008](0008-support-special-judge.md) | Accepted（スコアファイルの扱いは0025で一部変更） | 作問者の検証コードによるスペシャルジャッジを提供する |
| [0009](0009-support-interactive-judge.md) | Accepted | 提出と同じ言語群で対話用ジャッジを実行する |
| [0010](0010-extend-judge-languages-and-libraries.md) | Accepted（testlibの部分点の扱いは0025で一部変更） | testlib形式の判定とC#、Java 25、Nim、Goを追加する |
| [0011](0011-schedule-featured-problems.md) | Accepted（応募条件は0013、応募一覧は0020、順位表の正解は0025で一部変更） | 月曜と木曜に難易度別の問題を出す |
| [0012](0012-run-judge-on-ec2-pool.md) | Accepted（配布は0014、採点時間の見積もりは0025で一部変更） | EC2の常時稼働1台とコンテスト時の増設で採点する |
| [0013](0013-assess-problem-readiness.md) | Accepted（内容の条件は0021で統一） | 問題の準備状況をサーバーで判定し、公開とコンテストにも難易度を求める |
| [0014](0014-roll-out-os-updates-without-stopping-judging.md) | Accepted（未実装） | 採点を止めずにOS更新を配布する |
| [0015](0015-save-contests-as-drafts.md) | Accepted（下書きを作った人だけが扱う決定は0022で一部変更） | コンテストを下書きとして保存し、公開時に検証する |
| [0016](0016-anonymize-deleted-accounts.md) | Accepted（コンテストを持つ利用者の削除は0022と0023、参加登録の扱いは0024で一部変更） | アカウント削除では公開物を残し、作者を匿名にする |
| [0017](0017-link-posts-as-user-editorials.md) | Accepted | 記事をユーザー解説として問題に紐付ける |
| [0018](0018-tag-problems-and-posts.md) | Accepted（問題のタグの表示を切り替える場所は0026と0029で一部変更） | 問題と記事にタグを付け、使われている表記を候補に出す |
| [0019](0019-rejudge-submissions.md) | Accepted（コンテストの問題をリジャッジできる人は0022、作問の処理の送り出しは0032で一部変更） | 提出を今の問題の内容でリジャッジし、新しい提出の採点を優先する |
| [0020](0020-post-and-apply-on-dedicated-pages.md) | Accepted（予定表の作者名は2026-10-18に非表示へ変更） | 問題の投稿と定期便の応募を専用ページで行い、定期便の予定を公開する |
| [0021](0021-unify-problem-requirements.md) | Accepted（内容の条件と保存時に確かめる範囲は0025で一部変更） | 公開・コンテスト・定期便に、同じ内容の条件を求める |
| [0022](0022-share-contests-among-setters.md) | Accepted | コンテストを複数のセッターで作り、スタッフと参加者を兼ねさせない |
| [0023](0023-freeze-contest-standings.md) | Accepted | 順位表を凍結し、結果を公開するまで凍結後の提出を伏せる |
| [0024](0024-team-and-onsite-participation.md) | Accepted（個人参加、合言葉の文字、オンサイトの切り替えは2026-10-19に変更。開催中のチームの提出の置き場所は0030で一部変更） | チーム戦を参加コードで組み、オンサイト参加を会場の合言葉で区別する |
| [0025](0025-score-partially-with-test-groups.md) | Accepted | テストケースのグループごとに部分点を付ける |
| [0026](0026-switch-problem-tags-on-problem-list.md) | Accepted（表示の保存と問題ページの切り替えは0029で変更） | 問題のタグを問題一覧で表示に切り替えられるようにする |
| [0027](0027-notify-open-pages-and-bell.md) | Accepted（質問の宛先は2026-10-24に共同作成者へ広げた） | コンテストと定期便の出来事を、開いている画面のブラウザ通知とベルで知らせる |
| [0028](0028-paginate-lists-by-page-number.md) | Accepted | 一覧をページ番号で送り、表示件数を選べるようにし、順位表で自分の行を残す |
| [0029](0029-switch-problem-tags-per-page.md) | Accepted | 問題のタグの表示をページごとに切り替え、設定の「表示」タブをなくす |
| [0030](0030-list-own-contest-submissions.md) | Accepted | コンテストの自分の提出を一覧にし、見られない「すべての提出」をメニューに出さない |
| [0031](0031-run-code-without-problem.md) | Proposed | 問題に結び付けずにコードを実行する「コード実行」を加える |
| [0032](0032-count-authoring-runs-separately.md) | Accepted | 作問の生成と検証を提出と別の枠で数え、空きがあるときに送る |
