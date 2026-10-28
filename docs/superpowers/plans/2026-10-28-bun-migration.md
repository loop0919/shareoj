# フロントエンドのBun移行 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `web`の依存管理、開発、ビルド、テスト、CI/CD、本番SSR（Lambda）をNode.jsからBun 1.4.4へ移し、既存のテストとpush後のCI/CDを通す。

**Architecture:** 依存は`bun.lock`（`package-lock.json`から同じ版で移行）で管理し、スクリプト、Playwrightが起動するサーバー、`make dev`をBunで動かす。本番Lambdaは`provided.al2023`のカスタムランタイムにし、ZIPに同梱したARM64版Bunが`lambda/runtime.mjs`（Runtime APIの受け口）から既存の`lambda/index.handler`を呼ぶ。ZIPはNode.jsのLambdaでもそのまま動くので、ZIPの入れ替えとランタイムの切り替えを別のコミットにする。

**Tech Stack:** Bun 1.4.4、Nuxt 4.6.1（Nitro 2.13.4、Vite 8.3.4、vite-node 6）、Playwright 1.63.0、AWS Lambda `provided.al2023`（arm64）、Terraform（hashicorp/aws 6.63.0）、GitHub Actions、Nix

**Spec:** ユーザーの依頼（既存テストの全通過、デグレードの洗い出しと解消、push後のCI/CDの通過）と[ADR 0035](../../adr/0035-run-frontend-on-bun.md)

## Global Constraints

- Bunは1.4.4に揃える。版を書く場所は`flake.nix`、`web/package.json`の`packageManager`、`web/scripts/bun_release.py`（配布ZIPのSHA-256）の3か所で、`bun_release.py`が3か所の一致を確かめる。
- `web`とそのCI/CDから`node`、`npm`、`npx`を呼ばない。Bunは`bun run`以外（lifecycle scripts、ファイルの直接実行の子プロセス）ではPATHのNode.jsを使うので、`bun --bun`か`bun <file>`で明示する。
- `.env`を暗黙に読ませない。`web/bunfig.toml`の`env = false`、`make`とLambdaの`--no-env-file`で止める。
- 依存の版は変えない（`bun.lock`と`package-lock.json`の644件が一致）。
- ジャッジのNode.jsとBun（`judge/runtime`）は対象外。
- コマンドはworktreeで`nix develop /home/loop/judge/.claude/worktrees/bun-migration --command ...`から実行する。Playwrightは`--output=<scratchpad>/pw`、アカウント系は`TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable'`。
- コミットメッセージは`prefix: 対応内容`。コードを変えたら`graphify update .`を実行する。

## デグレード一覧

試作で確かめた事実と、各項目の解消・確認方法。

| ID | 起こりうるデグレード | 解消 | 確認 |
| --- | --- | --- | --- |
| D1 | `bun.lock`で依存の版が変わる | `bun pm migrate`で移行し、CIは`--frozen-lockfile` | 644件の版が一致（比較スクリプト） |
| D2 | 依存のinstall scriptがBunで止まる | 対象はesbuild（既定で信頼）とfsevents（macOS）だけ | `bun pm untrusted`が0件 |
| D3 | `postinstall`と、`--bun`のない`bun run`がPATHのNode.js（手元はv20、ランナーにも別版）で動く | lifecycle scriptは`bun --bun nuxt prepare`、ほかのスクリプトも`bun --bun`を明示 | `node`を失敗させる偽物をPATHに置いて全手順を流す |
| D4 | Bunが`.env`を自動で読み、`$VAR`を展開する（`abc$def`→`abc`）。ビルド済みサーバーとテストに`web/.env`が混ざる | `bunfig.toml`の`env = false`、`make`は`--no-env-file`、Lambdaも`--no-env-file` | `.env`を置いた状態で値が漏れないことを確認 |
| D5 | `nuxt dev`のSSRが全ページ500（Bunは名前空間に`__esModule`を見せ、vite-nodeのinteropがzodの`z`を落とす） | 開発時だけzodを`vite.ssr.noExternal`でVite変換に回す | Bunの`nuxt dev`で主要ページが200、Node.jsの`nuxt dev`と状態コードが一致 |
| D6 | JSCとV8の違い（Intl、Date）でSSRのHTMLや共有画像が変わり、hydrationがずれる | なし（差があれば個別に直す） | 同じ入力でNode.jsとBunのSSR（HTMLとPNG）を突き合わせる |
| D7 | Bunのnode:http互換で本文の読み方や応答ヘッダーが変わる | なし（差があれば個別に直す） | ブラウザーとアカウント系のPlaywright全件、`test:lambda` |
| D8 | Playwrightの起動するサーバーが黙ってNode.jsで動く（`bun --bun`の子プロセスの`node`はPATHのNode.js） | webServerのコマンドを`bun ...`に書き換える | D3の偽`node`で全件を流す |
| D9 | `node --import`の事前読み込み（Cognitoのトークン差し替え）が効かない | `bun --preload`に置き換える | アカウント系のGoogleログインのテスト |
| D10 | `node --test`がない | `bun test`（`node:test`互換） | 3件の成功と、失敗させた場合に落ちること |
| D11 | `npm audit`の代わりで、検出漏れや判定の違いが出る | `bun audit --omit=dev --json`の形式に合わせて判定する | 同じロックで勧告9件（high/critical 5件）がnpmと一致 |
| D12 | PlaywrightのコンテナにunzipがなくsetupのBunが入らない | `bun_release.py install`（python3で取得、SHA-256を検証、展開） | CIのweb-testsジョブ |
| D13 | Lambdaのカスタムランタイムの不備（実行権限、glibc、読み取り専用、auto-install、トランスパイラーのキャッシュ） | `bootstrap`で`--no-env-file --no-install`とキャッシュ無効、ZIPのモードを0755 | 公式`provided.al2023`イメージとRIEでZIPを起動（CIはarm64ランナーで本番と同じZIP） |
| D14 | Runtime APIの受け口の不備（エラー、初期化失敗、trace ID、接続断、5分を超える待機） | `lambda/runtime.mjs`で再試行と`timeout: false` | 偽Runtime APIの単体テスト、待機の実測 |
| D15 | ログからリクエストIDと重大度が消え、複数行が別イベントになる | Node.jsの管理ランタイムと同じ`時刻\tID\tLEVEL\t本文`で、改行は`\r` | 単体テストとRIEの出力 |
| D16 | ランタイム切り替えのデプロイで、新ランタイムと旧ZIPの組み合わせが数秒〜数十秒できる（providerは設定→コードの順で更新） | ZIPの入れ替え（段階1）とランタイムの切り替え（段階2）を別コミットにする | 段階1のZIPがNode.jsとBunの両方で動く（`test:lambda`とRIE） |
| D17 | ZIPが大きくなり（約12 MB→約50 MB）、展開後250 MiBの上限に近づく | 展開後のサイズを`package-lambda.py`で検査 | パッケージ時の出力、RIEの初期化時間の比較 |
| D18 | Bunの版がローカル、CI、Lambdaでずれる | 3か所の一致を`bun_release.py`が検査 | CIとパッケージ時 |

## Review Focus

- D6：本番のSSRがV8からJSCに変わる。テストが見ていない日付や数値の表示も、Node.jsとの突き合わせで確かめる。
- D16：段階1を先にデプロイしないと、切り替えの間にLambdaが起動できない時間ができる。
- D3／D8：Node.jsが入った環境（GitHubのランナー、Playwrightのイメージ、手元）では、取りこぼしたNode.jsの呼び出しが黙って成功する。
- D13：Lambdaの環境はローカルと違う（`/var/task`は読み取り専用、arm64）。CIで本番と同じZIPを公式イメージで起動する。
- D5：`nuxt dev`は既存のテストが通らない経路で、開発者が最初に踏む。

---

### Task 1: Bunの導入と依存管理

**Files:**
- Modify: `flake.nix`（`nodejs_22`を外し、Bun 1.4.4を入れる）
- Create: `web/bun.lock`、`web/bunfig.toml`、`web/scripts/bun_release.py`
- Delete: `web/package-lock.json`
- Modify: `web/package.json`（`packageManager: "bun@1.4.4"`、`engines.bun`、スクリプト）

**Interfaces:**
- Produces: `bun_release.py`の`VERSION`、`SHA256`（アセット名→16進）、`binary(asset: str, cache: Path) -> bytes`、`check_pins() -> None`、CLIの`install <dir>`

- [ ] `flake.nix`で`pkgs.bun`を1.4.4へ上書きする（4システムのSRIハッシュ。x86_64-linuxはnixpkgsと同じbaseline）
- [ ] `bun pm migrate`で`bun.lock`を作り、`package-lock.json`を消す
- [ ] `bunfig.toml`：`env = false`
- [ ] `package.json`のスクリプトをBunで明示する（`postinstall`は`bun --bun nuxt prepare`）
- [ ] 確認：`nix develop -c bun --version`が1.4.4、`bun install --frozen-lockfile`、`bun pm untrusted`が0件

### Task 2: 開発とテストのサーバーをBunで動かす

**Files:**
- Modify: `web/nuxt.config.ts`（D5）、`web/playwright.config.ts`、`web/playwright.account.config.ts`、`web/playwright.docs.config.ts`
- Modify: `scripts/dev.mjs`、`Makefile`

- [ ] webServerのコマンドを`bun ...`にし、`node --import`を`bun --preload`にする
- [ ] `dev.mjs`の指紋を`bun.lock`と`process.versions.bun`にし、`bun install --frozen-lockfile`を使う。Makefileは`bun --no-env-file`
- [ ] 確認：`bun run typecheck`、`bun run build`、ブラウザーとアカウント系のPlaywright全件、`bun test scripts/smoke-frontend.test.mjs`
- [ ] 確認（D6）：同じfixture APIでNode.jsとBunのSSRを並べ、主要URLのHTMLと共有画像を比べる
- [ ] 確認（D5）：Bunの`nuxt dev`で主要URLの状態コードがNode.jsの`nuxt dev`と一致する
- [ ] Commit

### Task 3: CIと脆弱性検査

**Files:**
- Modify: `.github/workflows/ci.yml`、`.github/workflows/vulnerabilities.yml`、`.github/workflows/deploy-dev.yml`、`scripts/audit-npm.mjs`

- [ ] runnerのジョブは`oven-sh/setup-bun@v2.2.0`（`bun-version-file: web/package.json`）、コンテナのジョブは`python3 scripts/bun_release.py install /usr/local/bin`
- [ ] Playwrightの版は`@playwright/test/package.json`から読む
- [ ] `audit-npm.mjs`を`bun audit`の形式（パッケージ名→勧告の配列）に合わせる
- [ ] 確認：`bun scripts/audit-npm.mjs web web/npm-audit-allowlist.json`が5件をすべて既知として通す
- [ ] Commit

### Task 4: LambdaのBunランタイム（段階1。ランタイムはnodejs22.xのまま）

**Files:**
- Create: `web/lambda/bootstrap`、`web/lambda/runtime.mjs`、`web/lambda/runtime.test.mjs`、`web/scripts/test-lambda-image.mjs`
- Modify: `web/scripts/package-lambda.py`、`.github/workflows/ci.yml`（arm64のweb-lambdaジョブ、ZIPのアップロード）

**Interfaces:**
- Consumes: `bun_release.binary('bun-linux-aarch64.zip', cache)`
- Produces: ZIPの`bootstrap`（0755）、`bin/bun`（0755）、`lambda/runtime.mjs`。`_HANDLER`（`lambda/index.handler`）の関数を`(event, context)`で呼ぶ

- [ ] 単体テスト：成功、例外、初期化失敗、trace ID、context、ログの形式、Runtime APIの切断からの再試行
- [ ] `runtime.mjs`と`bootstrap`を実装し、`package-lambda.py`で同梱する（展開後250 MiB未満を検査）
- [ ] `test-lambda-image.mjs`：ZIPを`unzip`で展開し、`public.ecr.aws/lambda/provided:al2023`へ読み取り専用で載せ、RIE経由でSSR、静的ファイル、APIの中継、共有画像、404を確かめる。ホストがarm64でなければ、同じ版のホストのBunに差し替える
- [ ] 確認：`bun run package:lambda`、`bun run test:lambda`、ローカルのRIE、Node.jsの管理ランタイムのイメージでも同じZIPが動く（D16）
- [ ] Commit

### Task 5: ランタイムの切り替え（段階2）と文書

**Files:**
- Modify: `infra/frontend/main.tf`（`runtime = "provided.al2023"`、handlerは据え置き）、`infra/frontend/tests/frontend.tftest.hcl`
- Create: `docs/adr/0035-run-frontend-on-bun.md`
- Modify: `docs/adr/README.md`、`README.md`、`web/README.md`、`infra/README.md`

- [ ] 確認：`terraform fmt -check`、`terraform validate`、`terraform test`（infra/frontend）
- [ ] Commit

### Task 6: 仕上げの確認

- [ ] Node.jsを消したPlaywrightのイメージで、CIと同じ手順（install、build、shard付きのPlaywright）を流す
- [ ] ブランチをpushし、CIとVulnerabilitiesを`workflow_dispatch`で実行して成功を確かめる
- [ ] デプロイの段階（段階1→段階2）を利用者に伝える
