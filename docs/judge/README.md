# ジャッジ設計

ジャッジは、ユーザーの提出を受け付けるバックエンドから分離し、Lightsailの2 GBインスタンス上でisolateを使ってプログラムをコンパイルして実行する設計である。
Lightsail用のTerraformとC++17ワーカーを追加しており、[構築手順](../../judge/README.md)に従って実機検証後に有効化する。
採点ホストは、EC2の常時稼働1台とコンテスト時に起動する増設の台へ移行する（[ADR 0012](../adr/0012-run-judge-on-ec2-pool.md)）。
ローカル開発用Dockerワーカーも引き続き使用できる。

現在の設計は次の文書に分けて管理する。

- [実行モデル](execution-model.md)：一つの提出が受理されてから採点結果が保存されるまでの流れ。
- [ランタイム方針](runtime-policy.md)：対応する提出形式と計算資源の契約。
- [スペシャルジャッジ](special-judge.md)：検証コードの登録、実行形式、判定と診断の扱い。
- [ローカルC++提出](local-cpp.md)：開発用Dockerワーカーの起動、テスト登録、提出と結果確認。
- [16 MiBのテストファイル](large-test-files.md)：S3への直接転送、検証、編集、採点時の取得方法。
- [EC2の採点台](ec2-pool.md)：常時稼働の台と増設の台の役割、台数の制御、配布の設定、Lightsailからの切り替え。

設計を選んだ理由は、[Architecture Decision Records](../adr/README.md)に記録する。
