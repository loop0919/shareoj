## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).

## git commit

指示を遂行してコードを変更したときは、毎度コミットをしてください。
コミットメッセージは `prefix: 対応内容` の形式にしてください。

## ジャッジの配布と検証

- `docs/judge/deployment-checks.md`の定型コマンドを使う。配布ごとに一時的な監視スクリプトを作り直さない。
- 既存2台への通常配布は`judge/rollout.py run`を使う。受付停止・DB排出・設定同期を個別の一時スクリプトで代替せず、`state.json`の`status: passed`と`step: complete`を完了条件にする。
- OSのセキュリティ更新は`judge/rollout.py rolling`で受付を止めずに配布する。送り出しの一時停止とdigestの切り替えを個別の一時スクリプトで代替せず、完了条件は`run`と同じにする。
- 長時間の実機検証は`judge/verify.py`でSSMに登録し、receiptとJSONレポートを保存する。待機は`collect --wait`に任せ、単発のSSM進捗確認を何十回も繰り返さない。
- 本番提出の検証は`judge/smoke-api.py`を使い、必要な場合だけ失敗ログを調査する。処理中や起動準備中を成功として報告しない。
