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
| [0008](0008-support-special-judge.md) | Accepted | 作問者の検証コードによるスペシャルジャッジを提供する |
| [0009](0009-support-interactive-judge.md) | Accepted | 提出と同じ言語群で対話用ジャッジを実行する |
| [0010](0010-extend-judge-languages-and-libraries.md) | Accepted | testlib形式の判定とC#、Java 25、Nim、Goを追加する |
| [0011](0011-schedule-featured-problems.md) | Accepted（応募条件は0013で一部変更） | 月曜と木曜に難易度別の問題を出す |
| [0012](0012-run-judge-on-ec2-pool.md) | Accepted | EC2の常時稼働1台とコンテスト時の増設で採点する |
| [0013](0013-assess-problem-readiness.md) | Accepted | 問題の準備状況をサーバーで判定し、公開とコンテストにも難易度を求める |
