# B1 API（HTTP）の保留事項（D04、D05、D03、D02、D01、D27）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task（実装者は1人。各タスクの最後に自分でコミットする）. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `docs/refactor/FIXES.md`のD01、D02、D03、D04、D05、D27の「変更内容」を、1件1コミットで実装する。

**Architecture:** 変更は`api/internal/httpapi`だけ。先にHEADをGETとして扱う入口を作り（D04）、プロフィールのJSON読み取りを共通の`readJSONBody`に寄せる（D05）。続けてContent-Typeの判定を`readJSONBody`の中の1か所にまとめる（D03）。D02で参加登録の応答に`p.get`を使い、D01でコンテスト系の4つのハンドラーをルートごとの関数に分ける。最後に、公開ルートの共通処理（`registerRoutes`の`public`）で、ハンドラーに10秒の期限を付ける（D27）。

**Tech Stack:** Go 1.26（`api`モジュール、`net/http`のServeMux）、pgx v5.11.0、PostgreSQL（テスト用DB）

**Spec:** `/home/loop/judge/.claude/worktrees/refactor-maintainability/docs/refactor/FIXES.md`（「守ること」と、6項目の「変更内容」）。根拠は`.local/fixes/briefs/B1.md`と、D01については`.local/refactor/candidates/C15.json`（proposal、preserve、両方のverdictの`must_preserve`）。行番号はすべて変更前（`b7bd62a`）のもの。

## Global Constraints

- 各項目の「変更内容」だけを変える。公開API（ルートの集合、OpenAPIの出力）、DBスキーマ、依存のバージョン、インフラは変えない。
- 認可、公開範囲、提出状態の遷移、重複・再実行の挙動は保つ。ルートごとの`release`、`auth.require`の`profileRequired=true`と10秒（テストファイルは25秒）も変えない。
- 1件1コミット。メッセージは`prefix: 対応内容`（日本語）で、末尾に`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`を付ける。
- テストを先に書く。新しい挙動のテストは、変更前のコードで失敗することを確かめてから直す。保つ挙動のテストは最初から通ってよい。保つ挙動の行が変更前のコードで失敗したら、行の期待値の誤りである。実測値に直し、報告に書く。
- テストの削除・スキップ・検証の緩和はしない。構造の都合で書き換える既存テストは`docs_test.go:18`の1行だけ（Task 1。検証の内容は同じ）。
- 範囲外の問題は直さずに報告する。コードのコメントは周りに合わせて英語で1行程度、文書とコミットは日本語。
- push、マージ、配布、AWSの操作はしない。`git stash`、`git reset`、`git rebase`、`git checkout`、`git switch`は使わない。
- TEST_DATABASE_URLは`postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable`。15432は使わない。

## 判定の記録（Ruling）

- **D04の直し方**：Ruling: ハンドラーごとのメソッド判定は直さず、`newHandler`の入口でHEADをGETに書き換えてからServeMuxに渡し、本文を捨てるResponseWriterで応答する。ServeMuxではGETのパターンだけがHEADに一致するので、ルートの一致は変わらない。HEADがGET以外の分岐に入るハンドラーは`submissions`、`rejudge`、`images`、`profile`、`featuredApplication`、`problem`、`privatePost`、`entrants`、`testFile`の9つあり、1か所で直せば後から足すハンドラーにも効く。本文は、Lambdaのアダプター（`lambdaapi/adapter.go:39`は`httptest.NewRecorder`）が書かれた本文をそのまま返すため、ここで捨てる。
- **D04とdocs_test**：Ruling: 今の`newHandler`の本体を`newMux`に移し、`newHandler`は`headAsGet(newMux(...))`を返す。`docs_test.go:18`の`newHandler(...).(*http.ServeMux)`は`newMux(...)`に変える。同じmuxに同じ検証をするので、緩和ではない。
- **D05とD03の順序**：Ruling: D05を先にする。D05でプロフィールの手書きのデコードを`readJSONBody`に置き換えれば、D03でContent-Typeの判定を`readJSONBody`の中に入れるだけで、すべてのJSON本文に同じ判定が当たる。
- **D03の関数**：Ruling: 判定は`readJSONBody`（`handler.go:42`）の先頭に置く。今の`readAuthJSON`と同じく`mime.ParseMediaType`のエラーか`application/json`以外なら415 `json_required`。`contentJSON`、`readAuthJSON`は上限だけを決める薄い関数として残し、`submissions.go:53-57`、`rejudge.go:32-36`、`private_problems.go:79-83`、`profile.go:73-76`の個別の判定は消す。消す判定はどれも`readJSONBody`の直前にあるので、検証の順序は変わらない。
- **D03の対象**：Ruling: 今まで判定していなかった画像のアップロード（`POST /my/images`、`images.go:122`）とテストファイルの開始（`POST /my/problems/{id}/test-files`、`private_problems.go:158`）にも当てる。どちらも本文はJSON（`{"data":"<base64>"}`と`{"size","sha256"}`）で、`readJSONBody`で読んでいる。「本文がJSONでないもの（画像など）」に当たるエンドポイントは、APIには今ない（テストファイルの中身は署名付きURLでS3へ直接送る）。BFFはofetch 1.5.1（`web/node_modules/ofetch`）で、オブジェクトの本文に`content-type: application/json`を付ける（`limitedJSON`が先に`application/json`を要求する）。CLIは`cli/api.go:119`で、すべての呼び出しに`Content-Type: application/json`を付ける。下の「確認したい点」に挙げた。
- **D03の副作用**：`application/json;garbage`のように引数が壊れたヘッダーは、`contentJSON`の系統でも415になる（今は通る）。`readAuthJSON`と同じ扱いで、BFFとCLIは送らない。
- **D02の範囲**：Ruling: 変えるのは参加登録（`contests.go:60-62`）だけ。保存（PUT）と公開（publication）の応答は開始前のコンテストを返すので、`Revealable()`（終了後だけ真）にならず、`p.get`に変えても値は同じ。変更内容にもないので触らない。
- **D01の範囲**：Ruling: 変更内容が4つのハンドラーを名指しし「C15の提案どおり」としているので、C15のproposalの(2)(3)をすべて行う。`JudgePolicy`の組み立て（`contests.go:77,161`）も`p.judgePolicy()`1か所にする。worthのverdictがentrantsを外した理由（`HEAD /participants`が415から200に変わる）は、Task 1のD04で先に解消される。エラー処理を共通化する新しい抽象は作らない（C15のとおり、各関数の最後は`contestError`と`writeAuthJSON`）。
- **D01の前処理**：Ruling: `available`（no-store、Contestsがnilなら503）と、それを最初に呼ぶ`contestID(w, r, notFound)`の2つにする。teamのルートだけ不正なidが`team_not_found`なので、404のコードは呼び出し側が渡す。teamとentrantsは今`Cache-Control`を自分で付けていないが、`auth.require`（`authentication.go:59`）が必ず付けるので応答は変わらない。
- **D27の置き場所**：Ruling: `registerRoutes`の`public`（`routes.go:119-130`）で、`handler`を「10秒の期限を付けて呼ぶ関数」で包み、`release`の有無どちらの登録にも使う。期限は`releaseBefore`の後、ハンドラーの直前から数える。今の公開ハンドラーの期限（解放処理の後のハンドラー内で始まる）と同じ意味にするため。
- **D27の重複**：Ruling: 共通処理で付けるので、公開ハンドラーに個別に書いた10秒（`problems.go:79-80`、`posts.go:28-29`と`:99-100`、`tags.go:19-20`、`public_samples.go:32-33`、`profile.go:139-140`、`images.go:49-51`）は消し、`r.Context()`を使う。消しても期限の終わりはほぼ同じである（外側の期限が数マイクロ秒早く始まるだけ）。`publishedProblems`は`GET /my/problems/{id}/publication`でも使うが、そこでは`auth.require`の10秒が先に付いているので、今も個別の期限は効いていない。

## 確認したい点

- D03の「本文がJSONでないもの（画像など）は対象外」が、画像のアップロード（本文はbase64を入れたJSON）を指しているかどうか。計画は「JSONなので対象」として進める（上の裁定）。対象外とする場合は、Task 3から画像の行を外し、`images.go:122`を`readJSONBody`から判定のない読み取りに分ける必要がある。

## Review Focus

1. メソッドで分岐するハンドラーへのHEADが、Lambdaのアダプターと同じ`httptest`の記録器を通っても、GETと同じステータスとヘッダーを空の本文で返し、何も書き込まない。POSTだけのルートへのHEADは今のとおり405。→ Task 1の`TestHeadServesReadPostgres`、`TestHealthHead`、`TestPrivateTestFileRoutes`のHEADの行。
2. 実際のクライアントのヘッダー。BFFは本文があるときだけ`content-type: application/json`を付け、本文のないPOST（reveal、complete、tester-invitation、notifications/read、本文なしの参加登録）にはContent-Typeを付けない。これらが415にならない。→ 判定を`readJSONBody`の中だけに置く（本文を読まないルートは呼ばない）。Task 5の参加登録の「本文なし、Content-Typeなし→200」の行、既存の`TestPrivateTestFileRoutes`のcomplete（Content-Typeなし）、Playwrightのaccountスイート（実際のBFFとGoのAPI）。
3. 分けた各ルートで、ワイルドカード（id、problem、submission、handle、team）に`%2F`を入れると、そのワイルドカードの検証で404になる。例外は、Submissionsがnilのときの提出系3ルートの503（今の順序のまま）。→ Task 5の表の「新」の行と`%2F`の行。
4. 分けた後も、各ルートの検証の順序とコードが変わらない（Contestsがnilの503が最初、teamのルートだけ`team_not_found`、offsetを見るのは一覧だけ、問題別の提出一覧はoffset→problem→mine→scope、提出系はjudging_unavailableがsid・offset・problemより先）。→ Task 5の`TestContestRoutesWithoutDatabase`と`TestContestRoutesPostgres`。
5. 公開ルートの期限はハンドラーだけに効き、すべての公開ルートのDBの問い合わせに期限が付く。認証付きのルートの期限は変わらない。→ Task 6の`TestPublicRoutesHaveDeadlinePostgres`（期限がもともとあるルートも対照として入れる）と、既存の`TestAuthenticationPreservesHandlerDeadline`。

---

### Task 1: D04 GETのルートに一致したHEADを、GETの読み取りとして本文なしで返す

**Files:**
- Modify: `api/internal/httpapi/handler.go:20-32`
- Modify: `api/internal/httpapi/docs_test.go:18`
- Modify: `api/internal/httpapi/handler_test.go`（`TestHealthHead`を足す）
- Modify: `api/internal/httpapi/private_problems_test.go:120-146`（`TestPrivateTestFileRoutes`）
- Create: `api/internal/httpapi/head_test.go`

**Interfaces:**
- Produces: `func newMux(config AuthConfig, private handlerDependencies) *http.ServeMux`（今の`newHandler`の本体）、`func newHandler(config AuthConfig, private handlerDependencies) http.Handler`（`headAsGet(newMux(config, private))`を返す）、`func headAsGet(next http.Handler) http.Handler`、`type headResponse struct{ http.ResponseWriter }`

- [ ] **Step 1: 失敗するテストを書く**

`handler_test.go`（パッケージ`httpapi_test`）に`TestHealthHead`を足す。`httpapi.NewHandler()`に`HEAD /health`を送り、ステータス200、`Content-Type`が`application/json`、`recorder.Body.Len() == 0`を確かめる。続けて`HEAD /auth/login`が405であることも確かめる（POSTだけのルート。保つ挙動）。

`TestPrivateTestFileRoutes`のdownloadの行の後に、`request("HEAD", "/my/problems/"+problem+"/test-files/"+file, "")`がステータス200、本文が空、`files.file == file`であることを足す。

`head_test.go`に`TestHeadServesReadPostgres`を作る。`f := newTeamTest(t, "alice")`、`f.problem(p1)`、`f.create(cid, p1, nil)`（`p1 = "11111111-1111-4111-8111-111111111111"`、`cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"`）。次の各行で、同じパスへ本文なしのGETを送り、続けてJSONの本文と`Content-Type: application/json`を付けたHEADを送る（`/my/`はalice、`/contests/`は匿名）。HEADの`Code`がGETと同じ、`Body.Len() == 0`、`Content-Type`ヘッダーがGETと同じことを確かめる。

| パス | HEADの本文 |
| --- | --- |
| `/my/profile` | `{"handle":"zed","avatar":"","version":1}` |
| `/my/images` | `{"data":"`＋`base64.StdEncoding.EncodeToString(imagePNG(t, 4))`＋`"}` |
| `/my/submissions` | `{"problemId":"<p1>","runtime":"cpp17","source":"x"}` |
| `/my/problems/<p1>/rejudge` | `{"submissionIds":[]}` |
| `/my/contests/<cid>/participants` | `{"onsite":true}` |
| `/my/problems/<p1>` | なし |
| `/my/featured` | なし |
| `/contests/<cid>` | なし |

最後に、`f.count`で`SELECT count(*) FROM submissions`、`SELECT count(*) FROM content_images`、`SELECT count(*) FROM user_profiles WHERE handle='zed'`がすべて0であることを確かめる。

変更前のコードで失敗する理由（計画時に`go test -overlay`で実測）：HEADはGETのパターンに一致したうえで書き込み側に入る。プロフィールはzedに変わり、画像は201で保存され、提出は202で作られ、再ジャッジは202、participantsは409、`/my/problems/<p1>`は503（GETは200）。`/my/featured`は応募の保存の分岐に入り、空のidで404 `problem_not_found`になる（コードから。GETは200）。どの行も本文が記録される。`HEAD /health`は本文が記録され、テストファイルのHEADは405。

- [ ] **Step 2: 失敗を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 900 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v -run 'TestHealthHead|TestPrivateTestFileRoutes|TestHeadServesReadPostgres' ./internal/httpapi/ 2>&1 | grep -E -- '^(--- |FAIL|ok|panic)'`
Expected: 3つとも`--- FAIL`

- [ ] **Step 3: 実装する**

`handler.go`：今の`newHandler`の本体を`newMux`に移し、`newHandler`は`headAsGet(newMux(config, private))`を返す。`headAsGet`は、HEAD以外はそのまま`next`に渡す。HEADなら`r.Clone(r.Context())`の`Method`を`http.MethodGet`にし、`headResponse{w}`で`next`を呼ぶ。`headResponse.Write`は書かずに`len(p), nil`を返す（ステータスとヘッダーは埋め込んだResponseWriterのまま）。コメントには、メソッドで分岐するハンドラーがHEADを書き込みとして扱わないこと、Lambdaのアダプターは書かれた本文を返すのでここで捨てることを1〜2行で書く。

`docs_test.go:18`を`mux := newMux(AuthConfig{}, handlerDependencies{Contests: &contests.Store{}})`にする。

- [ ] **Step 4: 通ることを確かめる**

Step 2のコマンドで3つとも`--- PASS`。あわせて`-run 'TestPublicDocumentation|TestHealth|TestPublicProblemErrors|TestUnrelatedRoutesDoNotReleaseContests|TestProblemRoutesWithoutDatabase'`が`ok`。

- [ ] **Step 5: グラフを更新してコミットする**

```bash
cd /home/loop/judge/.claude/worktrees/refactor-maintainability && graphify update .
git -C /home/loop/judge/.claude/worktrees/refactor-maintainability add api/internal/httpapi/handler.go api/internal/httpapi/docs_test.go api/internal/httpapi/handler_test.go api/internal/httpapi/private_problems_test.go api/internal/httpapi/head_test.go
git -C /home/loop/judge/.claude/worktrees/refactor-maintainability commit -m "fix: GETのルートに一致したHEADをGETの読み取りとして扱い、本文を返さない" -m "docs_test.goはServeMuxを直接受け取るためnewMuxを呼ぶように変えた（検証の内容は同じ）。" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: D05 プロフィール保存のJSON読み取りを共通の処理に寄せ、上限超過に413を返す

**Files:**
- Modify: `api/internal/httpapi/profile.go:77-100`
- Modify: `api/internal/httpapi/profile_test.go`（`TestProfileRequestBody`を足す）

**Interfaces:**
- Consumes: `readJSONBody(w, r, target any, limit int64) bool`（`handler.go:42`）
- Produces: `TestProfileRequestBody`の表（Task 3が行を足す）

- [ ] **Step 1: 失敗するテストを書く**

`profile_test.go`に`TestProfileRequestBody`を作る。DBは使わず、`profileHandler{Profiles: profiles.New(nil)}.profile(w, r, "alice")`を直接呼ぶ（本文の読み取りと検証はストアより前で終わる。`newHandler`を通すと`auth.require`がnilのプールでプロフィールを読む）。`PUT /my/profile`の表（Content-Type、本文、ステータス、`expect`で照合するコード）：

| Content-Type | 本文 | 期待 |
| --- | --- | --- |
| `application/json` | `{"handle":"ab","version":0}`＋空白`200<<10`個 | 413 `request_too_large`（新。今は400 `invalid_request`） |
| `application/json` | `{"handle":"ab","version":0} {}` | 400 `invalid_request` |
| `application/json` | `{"handle":"ab","version":0,"owner":"x"}` | 400 `invalid_request` |
| `application/json` | `{"handle":"ab","version":0}` | 400 `invalid_profile` |
| `text/plain` | `{}` | 415 `json_required` |
| なし | `{}` | 415 `json_required` |

- [ ] **Step 2: 失敗を確かめる**

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go test -count=1 -v -run 'TestProfileRequestBody' ./internal/httpapi/ 2>&1 | grep -E -- '^(--- |FAIL|ok|panic)|got '`
Expected: `--- FAIL`。最初の行が`got 400 {"error":"invalid_request"}`（計画時に実測）。

- [ ] **Step 3: 実装する**

`profile.go:77-100`（`MaxBytesReader`から手書きの2回の`Decode`まで）を、`input`の宣言の後の`if !readJSONBody(w, r, &input, 200<<10) { return }`に置き換える。上限は今の200 KiBのまま。Content-Typeの判定（`:73-76`）はこのタスクでは残す（Task 3で消す）。使われなくなるimport（`encoding/json`、`io`）を消す。

- [ ] **Step 4: 通ることを確かめる**

Step 2のコマンドで`--- PASS`。あわせてDBありの`-run 'TestProfilesPostgres|TestAvatarValidation'`（Task 1のStep 2と同じ形のコマンド）が`ok`。

- [ ] **Step 5: グラフを更新してコミットする**

`graphify update .`の後、`git add api/internal/httpapi/profile.go api/internal/httpapi/profile_test.go`。メッセージ：`fix: プロフィール保存のJSON読み取りを共通の処理に寄せ、上限を超える本文に413を返す`

---

### Task 3: D03 JSON本文のContent-Type判定を1つの関数にまとめる

**Files:**
- Modify: `api/internal/httpapi/handler.go:41-60`
- Modify: `api/internal/httpapi/problems.go:16-22`、`auth.go:168-175`、`submissions.go:53-57`、`rejudge.go:32-36`、`private_problems.go:79-83`、`profile.go:73-76`
- Modify: `api/internal/httpapi/json_test.go`、`profile_test.go`（`TestProfileRequestBody`と`TestProfilesPostgres`）、`private_problems_test.go`（`TestPrivateTestFileRoutes`）

**Interfaces:**
- Consumes: Task 2の`TestProfileRequestBody`
- Produces: `readJSONBody`がContent-Typeも判定する（415 `json_required`）。`contentJSON`（700 KiB）と`readAuthJSON`（16 KiB）は`readJSONBody`を呼ぶだけになる。

- [ ] **Step 1: 失敗するテストを書く**

`json_test.go`の`TestJSONRequestLimits`：
- readerの表に`{"shared", 2 << 10, func(w http.ResponseWriter, r *http.Request, target any) bool { return readJSONBody(w, r, target, 2<<10) }}`を足す。
- 行の表に次を足す（全readerに当たる）。`{"mixed case", `{"Value":"ok"}`, "Application/JSON", 200}`、`{"space before parameter", `{"Value":"ok"}`, "application/json ; charset=utf-8", 200}`、`{"missing media type", `{}`, "", 415}`、`{"malformed parameter", `{}`, "application/json;garbage", 415}`、`{"other subtype", `{}`, "application/jsonx", 415}`。`"valid"`の行の`application/json; charset=utf-8`は、BFFとCLIが送る`application/json`に引数が付いた形として残す。

`TestProfileRequestBody`に`{"Application/JSON; charset=UTF-8", `{"handle":"ab","version":0}`, 400, "invalid_profile"}`を足す。

`TestPrivateTestFileRoutes`の`created`の前に、Content-Typeを付けずに同じ本文で`POST /my/problems/<problem>/test-files`を送る行を足す（`request`はContent-Typeを付けるので、この行だけ`httptest.NewRequest`で組む）。`expect(t, w, 415, "json_required")`と`files.file == ""`（`Begin`を呼んでいない）を確かめる。続く`created`の行（`application/json`。CLIの`cli/api.go:119`とBFFのofetchが送る値）は200のまま。

`TestProfilesPostgres`の`testContentImages(t, request, store)`の直前に、Content-Typeを付けない`POST /my/images`（本文`{"data":""}`、aliceのトークン）を`handler`へ送り、`expect(t, w, 415, "json_required")`を確かめる行を足す。

変更前のコードで失敗する理由：`contentJSON`は大文字と空白を415にし、壊れた引数を通す。`readJSONBody`は判定しない。プロフィールは`Application/JSON`を415にする。テストファイルの開始は200、画像は400 `invalid_image`を返す（計画時に実測）。

- [ ] **Step 2: 失敗を確かめる**

Run: Task 1のStep 2と同じ形で`-run 'TestJSONRequestLimits|TestProfileRequestBody|TestPrivateTestFileRoutes|TestProfilesPostgres'`
Expected: 4つとも`--- FAIL`

- [ ] **Step 3: 実装する**

`readJSONBody`の先頭で、`mime.ParseMediaType(r.Header.Get("Content-Type"))`がエラーか`application/json`以外なら`authError(w, 415, "json_required")`で`false`を返す（今の`readAuthJSON`の判定をそのまま移す）。コメントを「application/jsonとして送られた1つのJSON値だけを受け付ける」旨に直す。`contentJSON`は`return readJSONBody(w, r, target, 700<<10)`、`readAuthJSON`は`return readJSONBody(w, r, target, 16<<10)`にする。`submissions.go`、`rejudge.go`、`private_problems.go`、`profile.go`の個別の判定を消す。使われなくなるimport（`mime`、`strings`など）はコンパイラーに従って消す。`images.go`と`testFile`のコードは変えない（`readJSONBody`で判定が当たる）。

保つ挙動：判定の位置は、消した判定と同じく`readJSONBody`の直前である。提出の作成は`p.Ready()`の後、問題の保存はid・ストアの確認の後、テストファイルの開始はid・`Files`の確認の後に415を返す。参加登録の`optionalJSON`は、空の本文ならContent-Typeを見ない。→ 既存の`TestGenerationWithoutProblemStore`（503が先）、`TestContestTeamsPostgres`ほかの参加登録、`TestPrivateTestFileRoutes`のcomplete。

- [ ] **Step 4: 通ることを確かめる**

Step 2のコマンドで4つとも`--- PASS`。あわせて`-run 'TestSubmissionsPostgres|TestRejudgePostgres|TestPrivateProblemsPostgres|TestAccountPostgres|TestRegistration|TestRefreshSession|TestLogin|TestChallenge|TestAuth|TestGenerationWithoutProblemStore|TestContestTeamsPostgres|TestContestSettersPostgres'`が`ok`。

- [ ] **Step 5: グラフを更新してコミットする**

`graphify update .`の後、変更したファイルをすべて`git add`。メッセージ：`fix: JSON本文のContent-Type判定をreadJSONBodyにまとめ、mime.ParseMediaTypeで判定する`。本文に「画像のアップロードとテストファイルの開始にも判定を当てた。BFFとCLIはapplication/jsonを送る」と書く。

---

### Task 4: D02 運営者の参加登録の応答にもcanRevealResultsを付ける

**Files:**
- Modify: `api/internal/httpapi/contests.go:60-63`
- Modify: `api/internal/httpapi/contest_freeze_test.go`（`TestOperatorParticipationCanRevealPostgres`を足す）

- [ ] **Step 1: 失敗するテストを書く**

`TestOperatorParticipationCanRevealPostgres`：`f := newTeamTest(t, "alice", "ops", "bob")`。`f.handler`を、`newTeamTest`の`handlerDependencies`（`contest_teams_test.go:77`。`JudgeImage`などジャッジの設定も含む）をそのまま写して`Operators: map[string]bool{"ops": true}`を足した`newHandler`で置き換える（`Verifier`は`newCognitoVerifier(f.sign.server.URL, "client")`）。`f.problem(p1)`、`f.create(cid, p1, map[string]any{"freezeMinutes": 30})`。

- 開始前の`f.join(cid, "ops", nil)`の応答は`CanRevealResults == false`（終了前は公開できない。保つ挙動）。`f.join(cid, "bob", nil)`。
- `f.schedule(cid, "ended")`の後、本文なしで再び`f.join(cid, "ops", nil)`。応答の`CanRevealResults == true`、かつ`f.contest(cid, "ops").CanRevealResults == true`（新。今は参加登録だけfalse。計画時に実測）。
- `f.join(cid, "bob", nil)`の応答は`CanRevealResults == false`（運営者でない参加者。保つ挙動）。

- [ ] **Step 2: 失敗を確かめる**

Run: Task 1のStep 2と同じ形で`-run 'TestOperatorParticipationCanRevealPostgres'`
Expected: `--- FAIL`（終了後の運営者の参加登録の応答がfalse）

- [ ] **Step 3: 実装する**

`contests.go:62`の`p.Contests.Get(r.Context(), id, owner)`を`p.get(r, id, owner)`にする。

- [ ] **Step 4: 通ることを確かめる**

Step 2のコマンドで`--- PASS`。あわせて`-run 'TestContestFreezePostgres|TestContestTeamsPostgres|TestContestsPostgres'`が`ok`。

- [ ] **Step 5: グラフを更新してコミットする**

`git add api/internal/httpapi/contests.go api/internal/httpapi/contest_freeze_test.go`。メッセージ：`fix: 運営者の参加登録の応答にも、結果を公開できるかを付ける`

---

### Task 5: D01 コンテスト系のハンドラーを登録したルートごとの処理に分ける

**Files:**
- Modify: `api/internal/httpapi/contests.go:18-230`（`publicContest`、`contest`、`contestDraft`）、`:308-348`（`team`）、`:399-449`（`entrants`）
- Modify: `api/internal/httpapi/routes.go:139-144,146,172-180,186-201`（登録先だけ）
- Create: `api/internal/httpapi/contest_routes_test.go`

**Interfaces:**
- Consumes: Task 4の`joinContest`は`p.get`で応答する。Task 1により、ハンドラーにHEADは届かない（GETとして届く）。
- Produces（`contests.go`）：
  - `func anonymous(next actorHandler) http.HandlerFunc` — `next(w, r, "")`を呼ぶ。`publicContest`の代わり。
  - `func (p contestHandler) available(w http.ResponseWriter) bool` — `Cache-Control: no-store`、Contestsがnilなら503 `database_unavailable`。
  - `func (p contestHandler) contestID(w http.ResponseWriter, r *http.Request, notFound string) (string, bool)` — `available`の後、`r.PathValue("id")`が`problemID`に合わなければ404 `notFound`。
  - `func (p contestHandler) judging(w http.ResponseWriter) bool` — Submissionsがnilなら503 `judging_unavailable`。
  - `func (p contestHandler) judgePolicy() contests.JudgePolicy` — `KnownRuntimes: submissions.RuntimeIDs(), EnabledRuntimes: p.Judging.RuntimeIDs()`。
  - `func (p contestHandler) setOnsite(w http.ResponseWriter, r *http.Request, id, actor, handle, team string)` — `contentJSON`→`onsite`がなければ400 `invalid_request`→`SetOnsite`。
  - ルートの関数はすべて`func (p contestHandler) NAME(w http.ResponseWriter, r *http.Request, owner string)`（teamとentrantsは今と同じく引数名`actor`）。

| ルート | 関数 | 中身の順序（今の分岐から写す） |
| --- | --- | --- |
| `GET /contests`、`GET /my/contests` | `contestList` | `available`→`contestOffset`→`List`→51件目で`HasMore` |
| `GET (/my)/contests/{id}` | `contestDetail` | `contestID("contest_not_found")`→`p.get` |
| `PUT /my/contests/{id}` | `saveContest` | `contestID`→`contentJSON`→`PenaltyMinutes`がnilなら5→`ValidInput`（400 `invalid_contest`）→`Save(..., p.judgePolicy())`→`p.Contests.Get` |
| `POST /my/contests/{id}/participation` | `joinContest` | `contestID`→`optionalJSON`→`ValidJoin`/`joinInputError`→`Join`→`p.get` |
| `POST /my/contests/{id}/reveal` | `revealContest` | `contestID`→`Reveal(..., p.Operators[owner])`→`p.get` |
| `GET (/my)/contests/{id}/problems/{problem}` | `contestProblem` | `contestID`→problem（404 `problem_not_found`）→`Problem` |
| `GET (/my)/contests/{id}/problems/{problem}/submissions` | `contestProblemSubmissions` | `contestID`→`judging`→`contestOffset`→problem（404）→`submissionScope(w, r, owner, true)`→`onsiteScope`→`ProblemList(..., onsite && scope == submissions.ScopeAll, offset)` |
| `GET (/my)/contests/{id}/standings` | `contestStandings` | `contestID`→`onsiteScope`→`Standings(..., owner != "" && live=="1", onsite)`（ADR 0023のコメントを残す） |
| `GET (/my)/contests/{id}/submissions` | `contestSubmissions` | `contestID`→`judging`→`contestOffset`→`onsiteScope`→`ContestList` |
| `GET (/my)/contests/{id}/submissions/{submission}` | `contestSubmission` | `contestID`→`judging`→submission（404 `submission_not_found`）→`ContestGet` |
| `GET /my/contests/drafts` | `contestDrafts` | `available`→`contestOffset`→`Drafts`→51件目で`HasMore` |
| `GET /my/contests/{id}/draft` | `contestDraft` | `contestID`→`Draft(..., p.judgePolicy())` |
| `PUT /my/contests/{id}/draft` | `saveContestDraft` | `contestID`→`contentJSON`→`ValidDraft`（400 `invalid_contest`）→`SaveDraft` |
| `DELETE /my/contests/{id}/draft` | `deleteContestDraft` | `contestID`→version（400 `invalid_version`）→`DeleteDraft`→204 |
| `PUT /my/contests/{id}/publication` | `publishContestDraft` | `contestID`→`contentJSON`→version（`<=0`か`>9007199254740990`で400 `invalid_version`）→`Publish`→`p.Contests.Get` |
| `GET /my/contests/{id}/team` | `myTeam` | `contestID("team_not_found")`→`MyTeam` |
| `PUT /my/contests/{id}/team/code` | `renewTeamCode` | `contestID("team_not_found")`→`RenewTeamCode` |
| `PUT /my/contests/{id}/team/name` | `renameTeam` | `contestID("team_not_found")`→`teamName`→`RenameTeam` |
| `DELETE /my/contests/{id}/team/members/{handle}` | `removeTeamMember` | `contestID("team_not_found")`→handle（404 `team_member_not_found`）→`RemoveTeamMember`→204 |
| `GET /my/contests/{id}/participants` | `entrants` | `contestID("contest_not_found")`→`Entrants`→`itemsResponse` |
| `PUT /my/contests/{id}/participants/{handle}/onsite` | `setParticipantOnsite` | `contestID`→handle（404 `participant_not_found`）→`setOnsite(..., handle, "")` |
| `PUT /my/contests/{id}/teams/{team}/onsite` | `setTeamOnsite` | `contestID`→team（404 `team_not_found`）→`setOnsite(..., "", team)` |
| `PUT /my/contests/{id}/teams/{team}/name` | `renameEntrantTeam` | `contestID`→team（404 `team_not_found`）→`teamName`→`RenameEntrantTeam` |

各関数は最後に、エラーなら`contestError`、成功なら`writeAuthJSON(w, 200, result)`（削除の2つは204）を書く。`get`、`optionalJSON`、`joinInputError`、`teamName`、`contestOffset`、`contestError`、`submissionScope`、`onsiteScope`は中身を変えない。`setterInvitation`、`setter`、`venue`は触らない。

- [ ] **Step 1: テストを書く（大半は保つ挙動、「新」の行だけ失敗する）**

`contest_routes_test.go`に次の3つを作る。行は`{method, path, owner, body, contentType string; status int; code string}`。`contentType`が空なら付けず、`owner`が空なら匿名。`code`が空の行はステータスだけ、ほかは`expect`で照合する。定数は`p1 = "11111111-1111-4111-8111-111111111111"`、`p2 = "22222222-2222-4222-8222-222222222222"`、`cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"`、`cid2 = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"`、`missing = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"`、`team = "ffffffff-ffff-4fff-8fff-ffffffffffff"`。`J`は`Content-Type: application/json`付き、`N`はContent-Typeなし。

(a) `TestContestRoutesWithoutDatabase`：`newHandler(AuthConfig{}, handlerDependencies{Verifier: staticVerifier{}})`。上の表の30パターンを正しいメソッドで、ワイルドカードをすべて`bad`にして本文なしで送る。すべて503 `database_unavailable`で、`Cache-Control`が`no-store`。

(b) `TestContestRoutesPostgres`：`f := newTeamTest(t, "alice", "bob")`、`f.problem(p1)`、`f.problem(p2)`、`f.create(cid, p1, nil)`。`f.handler`へ次の行を順に送る。

```
GET  /contests/bad                                          ""    -> 404 contest_not_found
GET  /contests/x%2Fy                                        ""    -> 404 contest_not_found
GET  /my/contests/bad/standings?scope=bad                   alice -> 404 contest_not_found
GET  /contests?offset=abc | ?offset=1000001                 ""    -> 400 invalid_request
GET  /contests?offset=1000000                               ""    -> 200
GET  /my/contests?offset=-1                                 alice -> 400 invalid_request
GET  /contests/<cid>?offset=abc&scope=bad                   ""    -> 200
GET  /contests/<cid>/problems/bad                           ""    -> 404 problem_not_found
GET  /contests/<cid>/problems/x%2Fstandings                 ""    -> 404 problem_not_found
GET  /my/contests/<cid>/problems/<p1>?offset=abc&scope=bad  alice -> 200
GET  /contests/<cid>/standings?offset=abc                   ""    -> 200
GET  /contests/<cid>/standings?scope=bad                    ""    -> 400 invalid_request
GET  /contests/<cid>/submissions?offset=abc | ?scope=bad    ""    -> 400 invalid_request
GET  /contests/<cid>/submissions/bad                        ""    -> 404 submission_not_found
GET  /my/contests/<cid>/submissions/<missing>?offset=abc    alice -> 404 contest_not_found
GET  /contests/<cid>/problems/bad/submissions?offset=abc    ""    -> 400 invalid_request
GET  /contests/<cid>/problems/bad/submissions?mine=bad      ""    -> 404 problem_not_found
GET  /contests/<cid>/problems/<p1>/submissions?mine=1&scope=bad ""-> 401 authentication_required
GET  /contests/<cid>/problems/<p1>/submissions?mine=team    ""    -> 401 authentication_required
PUT  /my/contests/<cid>  N {}                               alice -> 415 json_required
PUT  /my/contests/<cid>  J {                                alice -> 400 invalid_request
PUT  /my/contests/<cid>  J {}                               alice -> 400 invalid_contest
POST /my/contests/<cid>/participation J {"team":{"action":"create","name":""}} bob -> 400 invalid_team_name
POST /my/contests/<cid>/participation J {"team":{"action":"x"}}  bob -> 400 invalid_request
POST /my/contests/<cid>/participation J {"x":"<a×9216>"}    bob   -> 413 request_too_large
POST /my/contests/<cid>/participation N {"x":1}             bob   -> 415 json_required
POST /my/contests/<cid>/participation （本文なし、Content-Typeなし） bob -> 200
POST /my/contests/<cid>/reveal                              alice -> 409 contest_conflict
POST /my/contests/<cid>/reveal                              bob   -> 404 contest_not_found
GET  /my/contests/drafts?offset=abc                         alice -> 400 invalid_request
GET  /my/contests/bad/draft                                 alice -> 404 contest_not_found
PUT  /my/contests/<cid>/draft N {}                          alice -> 415 json_required
PUT  /my/contests/<cid>/publication J {"version":0} | {"version":9007199254740991} alice -> 400 invalid_version
PUT  /my/contests/<cid>/publication N {"version":1}         alice -> 415 json_required
DELETE /my/contests/<cid>/draft?version=abc | ?version=0    alice -> 400 invalid_version
GET  /my/contests/bad/team                                  alice -> 404 team_not_found
DELETE /my/contests/bad/team/members/Bad-Handle             alice -> 404 team_not_found
DELETE /my/contests/<cid>/team/members/Bad-Handle | x%2Fy   alice -> 404 team_member_not_found
PUT  /my/contests/<cid>/team/name N {"name":"x"}            alice -> 415 json_required
PUT  /my/contests/<cid>/team/name J {"name":" "}            alice -> 400 invalid_team_name
PUT  /my/contests/bad/participants/Bad-Handle/onsite J {"onsite":true} alice -> 404 contest_not_found
PUT  /my/contests/<cid>/participants/Bad-Handle/onsite | x%2Fy  J {"onsite":true} alice -> 404 participant_not_found
PUT  /my/contests/<cid>/participants/bob/onsite J {}        alice -> 400 invalid_request
PUT  /my/contests/<cid>/teams/bad/onsite J {"onsite":true}  alice -> 404 team_not_found
PUT  /my/contests/<cid>/teams/bad/name J {"name":"x"}       alice -> 404 team_not_found
PUT  /my/contests/<cid>/teams/<team>/name J {"name":" "}    alice -> 400 invalid_team_name
新 GET /contests/<cid>/submissions/x%2Fstandings            ""    -> 404 submission_not_found（今は200で順位表）
新 GET /my/contests/<cid>/submissions/x%2Fstandings?live=1  alice -> 404 submission_not_found（今は200）
新 GET /contests/<cid>/problems/x%2Fsubmissions?offset=abc  ""    -> 400ではなく404 problem_not_found
```

`|`で並べた値は、それぞれ別の行にする。続けて、`penaltyMinutes`を省いた本文（`f.create`の本文から`penaltyMinutes`を除き、問題は`p2`）で`PUT /my/contests/<cid2>`を送り、200で応答の`PenaltyMinutes == 5`を確かめる。

(c) 同じテストの中で、`Submissions`だけを外した`newHandler`（`Store`、`Contests`、`Profiles`、`Verifier`）を作り、次の行を送る。

```
GET /contests/bad/submissions                               "" -> 404 contest_not_found
GET /contests/<cid>/submissions/bad                         "" -> 503 judging_unavailable
GET /contests/<cid>/submissions?offset=abc                  "" -> 503 judging_unavailable
GET /contests/<cid>/problems/bad/submissions?offset=abc     "" -> 503 judging_unavailable
GET /contests/<cid>/standings                               "" -> 200
GET /my/contests/<cid>/problems/<p1>                     alice -> 200
新 GET /contests/<cid>/submissions/x%2Fstandings            "" -> 503 judging_unavailable（今は200）
新 GET /contests/<cid>/problems/x%2Fsubmissions             "" -> 404 problem_not_found（今は503）
```

51件目の`hasMore`（`contests_test.go:704`）、公開ルートの`live=1`（`contest_freeze_test.go:273-278`）、revealの権限と冪等性（同`:409-423`）は既存のテストが守るので、ここには書かない。

- [ ] **Step 2: 変更前のコードで確かめる**

Run: Task 1のStep 2と同じ形で`-run 'TestContestRoutes'`
Expected: `TestContestRoutesWithoutDatabase`は`--- PASS`。`TestContestRoutesPostgres`は`--- FAIL`で、失敗するのは「新」の5行だけ（`t.Errorf`で全行を回すか、サブテストにして確かめる）。「新」以外の行が失敗したら、行の期待値を実測値に直して報告する（コードはまだ変えない）。

- [ ] **Step 3: 実装する**

上の表のとおり`contests.go`を分け、`routes.go`の登録先を差し替える。公開ルートは`anonymous(contests.X)`、`/my/`のルートは`contests.X`。パターン、`release`の値、`publicOperation`は変えない。`publicContest`、`contest`、switchの`contestDraft`、`team`、`entrants`は消す。`strings`のimportが不要になれば消す。

- [ ] **Step 4: 通ることを確かめる**

Run: Task 1のStep 2と同じ形で`-run 'TestContestRoutes|TestContest|TestRevealWaits|TestJoinWaits|TestTesterAcceptanceWaitsForJoin|TestSetterAcceptanceWaitsForJoin|TestMutualCoSetterDeletion|TestPartialScoring|TestCreationQuotas|TestPublicDocumentation|TestDocumentationUsesResponseTypes|TestUnrelatedRoutesDoNotReleaseContests|TestOperatorParticipationCanReveal|TestHeadServesRead'`
Expected: すべて`--- PASS`、最後に`ok`

Run: `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && grep -n "r.URL.Path\|r.Method" api/internal/httpapi/contests.go`
Expected: `setterInvitation`の2行（`r.Method == http.MethodPost`、`r.Method == http.MethodPut`）と`venue`の1行（`r.Method == http.MethodPut`）だけ

- [ ] **Step 5: グラフを更新してコミットする**

`git add api/internal/httpapi/contests.go api/internal/httpapi/routes.go api/internal/httpapi/contest_routes_test.go`。メッセージ：`fix: コンテスト系のハンドラーを登録したルートごとの処理に分ける`。本文に「ワイルドカードの値に%2Fを含むURLは、その値の検証で404になる（今は提出詳細のルートが順位表を返していた）」と書く。

---

### Task 6: D27 公開ルートの共通処理でハンドラーに10秒の期限を付ける

**Files:**
- Modify: `api/internal/httpapi/routes.go:119-130`
- Modify: `api/internal/httpapi/problems.go:79-80`、`posts.go:28-29,99-100`、`tags.go:19-20`、`public_samples.go:32-33`、`profile.go:139-140`、`images.go:48-52`
- Create: `api/internal/httpapi/public_deadline_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`public_deadline_test.go`に、pgxの`QueryTracer`を実装する`deadlineTracer`を作る。`TraceQueryStart`は、mutexの下で問い合わせの数を数え、`ctx.Deadline()`がないか`time.Until(deadline) > 10*time.Second`ならSQLを`missing`に足す。`TraceQueryEnd`は何もしない。

`TestPublicRoutesHaveDeadlinePostgres`：TEST_DATABASE_URLがなければSkip。他のテストと同じくスキーマを作り、`pgxpool.ParseConfig`の`ConnConfig.Tracer`に`deadlineTracer`を入れて`pgxpool.NewWithConfig`でプールを開く。`problems.New(pool).Migrate`の後、`user_profiles`に`alice`を入れる。`newHandler(AuthConfig{}, handlerDependencies{Store: problems.New(pool), Contests: &contests.Store{Pool: pool}, Submissions: &submissions.Store{Pool: pool}, Posts: posts.New(pool), Tags: &tags.Store{Pool: pool}, Profiles: profiles.New(pool)})`。次の各パスについて、記録を空にしてから匿名のGETを送り、問い合わせが1件以上あり、`missing`が空であることを確かめる（ステータスは見ない）。idはすべて`11111111-1111-4111-8111-111111111111`。

- 対象（今は期限がない）：`/contests`、`/contests/<id>`、`/contests/<id>/problems/<id>`、`/contests/<id>/problems/<id>/submissions`、`/contests/<id>/standings`、`/contests/<id>/submissions`、`/contests/<id>/submissions/<id>`、`/submissions/<id>`、`/problems/<id>/submissions`、`/problems/<id>/submissions/<id>`、`/featured`、`/featured/schedule`、`/featured/standings?at=2026-10-19T14:00:00Z`
- 対照（今も期限がある。保つ挙動）：`/problems`、`/tags`、`/users/alice`、`/posts`、`/contests/<id>/editorials`、`/problems/<id>/samples`

計画時に`go test -overlay`で実測した。変更前は、対象の13本すべてで、解放処理より後のハンドラーの問い合わせが`missing`に入る。対照の6本では`missing`は空である。

- [ ] **Step 2: 失敗を確かめる**

Run: Task 1のStep 2と同じ形で`-run 'TestPublicRoutesHaveDeadlinePostgres'`
Expected: `--- FAIL`（対象の行で、期限のない問い合わせ）

- [ ] **Step 3: 実装する**

`public`の中で、`handler`を「`context.WithTimeout(r.Context(), 10*time.Second)`の文脈で`handler`を呼ぶ関数」で包み、`release`がfalseの登録と、`releaseBefore`の`next`の両方でそれを使う。コメントは1行で、認証付きのルートと同じ期限を公開ハンドラーにも付けること、解放処理には自分の期限があることを書く。裁定のとおり、個別の10秒（Filesに挙げた7か所）を消して`r.Context()`を使い、`publicImage`は`p.contentImage(w, r, "")`だけにする。使われなくなるimport（`context`、`time`）を消す。

保つ挙動：認証付きのルートの期限（`auth.require`）と`releaseBefore`の期限は変えない。→ `TestAuthenticationPreservesHandlerDeadline`、Step 1の対照の行。

- [ ] **Step 4: 通ることを確かめる**

Step 2のコマンドで`--- PASS`。あわせて`-run 'TestAuthenticationPreservesHandlerDeadline|TestProblemRoutesWithoutDatabase|TestPublicProblemErrors|TestPublicDocumentation|TestTagsPostgres|TestProfilesPostgres|TestUserEditorialsPostgres|Featured|TestContestsPostgres|TestSharedSubmissionVisibilityPostgres'`が`ok`。

- [ ] **Step 5: グラフを更新してコミットする**

変更したファイルをすべて`git add`。メッセージ：`fix: 公開ルートの共通処理で、ハンドラーに10秒の期限を付ける`。本文に「コンテスト、公開提出、定期便のハンドラーにも期限が付く。公開ハンドラーに個別に書いていた同じ期限は消した」と書く。

---

## 検証コマンド（最後のコミットの後）

1. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && timeout 1800 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_test?sslmode=disable' go test -count=1 -v ./... 2>&1 | grep -E '^(--- FAIL|FAIL|ok|panic)|TEST_DATABASE_URL required'` → `ok`の行だけ（SKIPしてよいのは`TestBrowserFixture`と、`TEST_JUDGE_CPP_IMAGE`が要る`submissions/runner_test.go`のDockerテストだけ）
2. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command go vet ./...` → 警告なし
3. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command gofmt -l .` → 出力なし
4. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/api && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command make package package-judge` → 終了コード0
5. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command npm run typecheck`、同じ形で`npm run build` → エラーなし（webは変えないが、コントローラーが全体を確かめる）
6. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 npx playwright test --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b1` → すべてpassed
7. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability/web && timeout 2400 nix develop /home/loop/judge/.claude/worktrees/refactor-maintainability --command env CI=1 TEST_DATABASE_URL='postgresql://openoj_test:test-only-password@127.0.0.1:25432/openoj_web?sslmode=disable' npx playwright test --config playwright.account.config.ts --output=/tmp/claude-1000/-home-loop-judge/c1f089e1-4d8a-4bf1-a73b-69b8400c9d0f/scratchpad/pw-b1-account` → すべてpassed。実際のBFFが実際のGo APIへ送るヘッダーで、画像のアップロード（`images.spec.ts`）、コンテスト、プロフィール、提出が415にならないことの確認を兼ねる（D03）。
8. `cd /home/loop/judge/.claude/worktrees/refactor-maintainability && git status --short && git log --oneline -7` → 作業ツリーはきれい。D04、D05、D03、D02、D01、D27の6件のコミットがこの順に並ぶ

## Out of scope（報告のみ）

- `setterInvitation`と`venue`はメソッドで、`posts.editorials`は`r.URL.Path`の先頭（`/contests/`か`/problems/`か）で分岐している。D01の変更内容が名指しした4つのハンドラーではないので分けない。D04により、HEADが書き込み側に入ることはなくなる。
- BFFの`limitedJSON`（`web/server/utils/private-request.ts:16`）は、ブラウザーからの`Content-Type`を小文字の`application/json`で比べる（大文字は415）。D03はAPIの判定なので変えない。
- BFFがJSONの`null`や`false`など偽の値を本文として受け取ると、ofetchは本文もContent-Typeも送らない。D03の後、画像のアップロードとテストファイルの開始では、この誤った入力への応答が400から415に変わる。どちらも誤りの応答で、画面の文言は変わらない。
- 公開ルートでは、解放処理の期限（最大10秒×2）とハンドラーの10秒が別々なので、合計でAPI Gatewayの20秒を超えうる。今の期限ありの公開ルートと同じ構造で、D27の変更内容（他の公開ルートと同じ期限）の外にある。
- `routes.go:150`の定期便の説明の「（日本時間）」は、D19（B7）で扱う。
- `FIXES.md`の「まとまりと順序」の状態とコミットの欄は、コントローラーが更新する。
