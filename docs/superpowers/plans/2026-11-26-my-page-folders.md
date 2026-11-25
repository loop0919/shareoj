# マイページのフォルダ分け Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** マイページに、自分の問題・テスト中の問題・記事を自分で作ったフォルダに分けて見られる「フォルダ表示（ベータ）」を足し、利用者がスイッチで今の画面と切り替えられるようにする。

**Architecture:** フォルダと振り分けはPostgreSQLの3つの表に持ち、新しいGoパッケージ`folders`と`/my/folders`系のAPIで読み書きする。問題と記事の一覧APIは今のものを使い、画面が3つの一覧と振り分けをIDで突き合わせる。切り替えは`user_profiles`の列に持ち、オフの利用者には今のコンポーネントをそのまま出す。

**Tech Stack:** Go（pgx、`api/internal/httpapi`）、PostgreSQL、Nuxt 4（Vue 3、zod）、Playwright。ドラッグはブラウザのHTML Drag and Drop APIで作り、ライブラリは足さない。

**Spec:** [docs/superpowers/specs/2026-11-26-my-page-folders-design.md](../specs/2026-11-26-my-page-folders-design.md)。各タスクの前に、対応する節を読む。

## Global Constraints

- 上限は、フォルダ100個、深さ3段、名前は前後の空白を除いて1〜40文字、振り分けは1回100件。
- 同じ親の下に同じ名前のフォルダは作れない。最上位どうしも同じ。
- 振り分けの種類は`problem`と`post`の2つ。自分の問題とテスト中の問題は、どちらも`problem`である。
- フォルダを消すと、下のフォルダも消え、中の問題と記事は未整理に戻る。問題と記事は消さない。
- 問題と記事の一覧API（`GET /my/problems`、`GET /my/problems?role=tester`、`GET /my/posts`）は、Task 4の`author`を除いて変えない。
- ベータがオフの利用者のマイページは、DOMも通信も今と同じにする。オフのときに`/api/my/folders`を呼ばない。
- 開けない問題や他人の記事のIDと、存在しないIDは、応答で区別できないようにする。
- 配信モード（ADR 0042）の`privateAPI`は既定で閉じる。フォルダの呼び出しを通す側に足さない。
- 画面の文言。
  - スイッチ：「フォルダ表示（ベータ）」
  - タブ：「問題と記事」
  - フォルダの一覧：「すべて」「未整理」「フォルダを作る」
  - 種類：「自分の問題」「テスト中」「記事」
  - 状態：「準備中」「応募中」「公開予約中」「公開中」
  - 移動：「フォルダへ移動」「選んだ{N}件をフォルダへ移動」
  - エラー：`folder_limit`「フォルダは100個までです。使っていないフォルダを消してから、もう一度お試しください。」、`folder_depth`「フォルダの入れ子は3段までです。」、`folder_name_taken`「同じ場所に同じ名前のフォルダがあります。別の名前にしてください。」、`folder_cycle`「フォルダを、自分自身やその中のフォルダには移せません。」、そのほか「保存できませんでした。もう一度お試しください。」。どれも後ろに「何も変わっていません。」を付ける。
- 意見の送り先は`https://github.com/loop0919/shareoj/issues/new?template=feature_request.yml`。
- コメントは英語で、既存のファイルの書き方に合わせる。
- コミットは`prefix: 対応内容`の形式で、タスクごとに行う。末尾に`Co-Authored-By: Claude Fable 5.2 <noreply@anthropic.com>`を付ける。コードを変えたら`graphify update .`を実行する。
- このリポジトリは公開されている。鍵やリソースのIDを書かない。
- ブランチ`feat/my-page-folders`で作業する。pushはしない（mainへのpushは自動で配布される）。
- 移行ファイルの番号は、作業を始める時点のmainの次の番号にする。この計画では048と書く。`feat/contest-webhooks`が先にmainへ入っていたら049にする。

### テストの流し方

コマンドはリポジトリのルートで`nix develop`の中から実行する。

```bash
make db-up
export TEST_DATABASE_URL='postgresql://openoj:openoj-local@127.0.0.1:15432/openoj?sslmode=disable'
cd api && go test ./internal/httpapi/ -run TestFolders
```

Playwrightはビルド済みの`.output`に対して走る。コードを変えたら、流す前に必ずビルドする。
`web/test-results`と`web/test-results-account`は書き込めないので、出力先を別に指定する。
`bun test`はPlaywrightではない。必ず`bun run test`を使う。

```bash
cd web
bun run typecheck && bun run build
bun run test tests/folder-tree.spec.ts --output="$TMPDIR/pw" --reporter=line
bun run test -c playwright.account.config.ts tests/account/folders.spec.ts --output="$TMPDIR/pw-account" --reporter=line
```

アカウントのテスト（`tests/account`）は、本物のAPIとデータベースを使い、利用者を全部のテストで共有する。
ベータの切り替えはアカウントに残るので、フォルダのテストは専用の利用者`folders`だけでオンにする。
`alice`や`bob`でオンにすると、ほかのテストが今のタブを見つけられずに落ちる。

## Review Focus

- **同時の操作で上限を超える**：フォルダが99個のときに2つの作成が同時に来ても、100個で止まる（Task 1のテスト）。
- **フォルダごと動かして深さを超える**：2段の下を持つフォルダを、2段目のフォルダの中へ動かすと拒否される。動かすフォルダ自身の深さだけを見ると通ってしまう（Task 1とTask 5のテスト）。
- **51件以上の一覧**：問題が51件あるとき、フォルダの件数と絞り込みの件数が全件で数えられる（Task 7のテスト）。
- **消えたフォルダを指すURL**：`/my?tab=library&folder=<もうないID>`を開くと「すべて」が出る。空の一覧やエラーにしない（Task 7のテスト）。
- **ドラッグ中の失敗**：振り分けの保存が失敗したら、行が元のフォルダに戻り、理由が出る（Task 9のテスト）。

---

### Task 1: フォルダの表とAPI

**Files:**
- Create: `api/internal/database/048_content_folders.sql`
- Modify: `api/internal/database/database.go`（`//go:embed`と`migrations()`の末尾）
- Create: `api/internal/folders/store.go`
- Create: `api/internal/httpapi/folders.go`
- Modify: `api/internal/httpapi/routes.go`（`handlerDependencies.Folders`、ルート4本）
- Modify: `api/internal/httpapi/config.go:111`付近（`private.Folders = folders.New(pool)`）
- Modify: `api/internal/httpapi/contest_teams_test.go:79`（`newTeamTest`の依存に`Folders: folders.New(pool)`と`Posts: posts.New(pool)`を足す）
- Test: `api/internal/httpapi/folders_test.go`

**Interfaces:**
- Produces（SQL）：
  - `content_folders(id uuid PRIMARY KEY, owner_id text NOT NULL REFERENCES user_profiles(owner_id), parent_id uuid REFERENCES content_folders(id) ON DELETE CASCADE, name text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp())`。`name`は`CHECK (name = btrim(name) AND char_length(name) BETWEEN 1 AND 40)`。
  - 一意の索引`content_folders_sibling_name`を`(owner_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), name)`に張る。
  - `problem_folder_items(owner_id text NOT NULL, problem_id uuid NOT NULL REFERENCES problem_drafts(id) ON DELETE CASCADE, folder_id uuid NOT NULL REFERENCES content_folders(id) ON DELETE CASCADE, PRIMARY KEY (owner_id, problem_id))`と、`folder_id`の索引。
  - `post_folder_items`は同じ形で、`post_id uuid NOT NULL REFERENCES blog_posts(id) ON DELETE CASCADE`。
  - `ALTER TABLE user_profiles ADD COLUMN folder_view boolean NOT NULL DEFAULT false`。
- Produces（Go、`package folders`）：

```go
const (
	MaxFolders = 100
	MaxDepth   = 3
	MaxName    = 40
	MaxItems   = 100
)

var (
	ErrNotFound  = errors.New("folder not found")
	ErrInvalid   = errors.New("invalid folder")
	ErrLimit     = errors.New("too many folders")
	ErrDepth     = errors.New("folders nested too deep")
	ErrNameTaken = errors.New("folder name already used")
	ErrCycle     = errors.New("folder moved into itself")
)

type Folder struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`
	Name     string  `json:"name"`
}

func New(pool *pgxpool.Pool) *Store
func (s *Store) Folders(ctx context.Context, owner string) ([]Folder, error) // name, then id
func (s *Store) Create(ctx context.Context, owner, name string, parent *string) (Folder, error)
func (s *Store) Update(ctx context.Context, owner, id, name string, parent *string) (Folder, error)
func (s *Store) Delete(ctx context.Context, owner, id string) error
```

- Produces（HTTP）：

| 呼び出し | 本文 | 成功 |
| --- | --- | --- |
| `GET /my/folders` | なし | 200 `{"folders":[Folder],"items":[]}`（`items`はTask 2で埋める。空でも`[]`を返す） |
| `POST /my/folders` | `{"name":"…","parentId":null}` | 201 `Folder` |
| `PUT /my/folders/{id}` | `{"name":"…","parentId":"…"}`（両方必須） | 200 `Folder` |
| `DELETE /my/folders/{id}` | なし | 204 |

- エラーは`{"error":"<code>"}`。400 `invalid_folder`（名前が空か41文字以上、制御文字を含む、IDがUUIDでない）、404 `not_found`（フォルダか親が自分のものでない）、409 `folder_limit`、`folder_depth`、`folder_name_taken`、`folder_cycle`。
- ルートは`private(pattern, handler, false)`で足す。

- [ ] **Step 1: テストを書く**

`TestFoldersPostgres`を`newTeamTest(t, "alice", "bob")`で書く。次を確かめる。

```go
var top, child, leaf folders.Folder
f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": " コンテスト用 ", "parentId": nil}, 201), &top)
// top.Name == "コンテスト用", top.ParentID == nil
f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "第3回", "parentId": top.ID}, 201), &child)
f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "予備", "parentId": child.ID}, 201), &leaf)
f.fail("POST", "/my/folders", "alice", map[string]any{"name": "4段目", "parentId": leaf.ID}, 409, "folder_depth")
f.fail("POST", "/my/folders", "alice", map[string]any{"name": "第3回", "parentId": top.ID}, 409, "folder_name_taken")
f.fail("POST", "/my/folders", "alice", map[string]any{"name": "コンテスト用", "parentId": nil}, 409, "folder_name_taken")
f.fail("POST", "/my/folders", "alice", map[string]any{"name": "  ", "parentId": nil}, 400, "invalid_folder")
f.fail("POST", "/my/folders", "alice", map[string]any{"name": strings.Repeat("あ", 41), "parentId": nil}, 400, "invalid_folder")
f.fail("PUT", "/my/folders/"+top.ID, "alice", map[string]any{"name": "コンテスト用", "parentId": leaf.ID}, 409, "folder_cycle")
f.fail("PUT", "/my/folders/"+top.ID, "alice", map[string]any{"name": "コンテスト用", "parentId": top.ID}, 409, "folder_cycle")
```

- 他人：bobが`GET /my/folders`すると`folders`が空。bobの`PUT`と`DELETE`（aliceの`top.ID`）、bobの`POST`で`parentId`にaliceの`top.ID`は、どれも404 `not_found`。
- フォルダごと動かす：最上位に「定期便用」とその下「候補」を作り、「定期便用」を`child`（2段目）の中へ`PUT`すると409 `folder_depth`。「定期便用」を`top`の中へは200で、「候補」は3段目になる。
- 名前を変える：`PUT`で`name`だけ変えると200で、`GET`に新しい名前が出る。
- 消す：`DELETE /my/folders/{top.ID}`が204で、`content_folders`のaliceの行が0になる（下も消える）。
- 個数：`f.exec`でaliceのフォルダを99個に揃え、`f.newRequest`と`f.do`で2つの`POST`（別の名前）をゴルーチンから同時に送る。片方が201、片方が409 `folder_limit`で、行は100個。
- `GET /my/folders`の応答に`Cache-Control: no-store`が付く。

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd api && go test ./internal/httpapi/ -run TestFoldersPostgres`
Expected: コンパイルエラー（`folders`がない）。

- [ ] **Step 3: 移行ファイルと`folders.Store`を書く**

書き込みはすべて1つのトランザクションで行い、最初に`SELECT 1 FROM user_profiles WHERE owner_id=$1 AND deleted_at IS NULL FOR UPDATE`で持ち主の行をロックする。
同じ人の操作が直列になり、個数と深さを数えてから書くまでの間に、ほかの操作が割り込めない。
深さは、親の深さ（最上位の親なしは0）に、動かすフォルダの下の段数（自分だけなら1）を足して`MaxDepth`と比べる。
一意の索引の違反（`23505`、`content_folders_sibling_name`）を`ErrNameTaken`にする。
IDは`gen_random_uuid()`で作る。

- [ ] **Step 4: ハンドラとルートを書く**

`folderHandler{Store *folders.Store}`を`folders.go`に置き、`Store == nil`なら503 `database_unavailable`を返す（`favorites.go`と同じ）。
本文は`contentJSON`で読む。

- [ ] **Step 5: テストが通ることを確かめる**

Run: `cd api && go test ./internal/httpapi/ ./internal/database/ ./internal/folders/`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add api/internal/database api/internal/folders api/internal/httpapi
git commit -m "feat: マイページのフォルダを作成・変更・削除するAPIを足す"
```

---

### Task 2: 振り分けのAPI

**Files:**
- Modify: `api/internal/folders/store.go`
- Modify: `api/internal/httpapi/folders.go`、`api/internal/httpapi/routes.go`
- Test: `api/internal/httpapi/folders_test.go`

**Interfaces:**
- Consumes: Task 1の`Store`、表。
- Produces（Go）：

```go
type Ref struct {
	Kind string `json:"kind"` // "problem" or "post"
	ID   string `json:"id"`
}
type Item struct {
	Ref
	FolderID string `json:"folderId"`
}

func (s *Store) Items(ctx context.Context, owner string) ([]Item, error)
func (s *Store) Place(ctx context.Context, owner string, folder *string, refs []Ref) error
```

- Produces（HTTP）：`PUT /my/folder-items`、本文`{"folderId":null,"items":[{"kind":"problem","id":"…"}]}`、成功は204で本文なし。`folderId`が`null`なら未整理に戻す。
- `items`が0件か101件以上、`kind`が2つ以外、IDがUUIDでないときは400 `invalid_request`。`folderId`が自分のフォルダでなければ404 `not_found`。
- `GET /my/folders`の`items`を`Items`で埋める。JSONは`{"kind","id","folderId"}`が平らに並ぶ。

- [ ] **Step 1: テストを書く**

`TestFolderItemsPostgres`。aliceが問題`p1`（`f.problem`）と記事`post1`（`PUT /my/posts/{id}`、本文`{"version":0,"title":"解説","markdown":"本文","problemIds":[],"tags":[]}`）を持ち、bobが問題`p2`を持つ（`f.store.Save(f.ctx, "bob", p2, 0, draft)`）。aliceはフォルダ`fa`、bobは`fb`を作る。

- aliceが`p1`と`post1`を`fa`に入れると204で、`GET /my/folders`の`items`が2件になる。
- 同じ問題を別のフォルダに入れ直すと、行は1つのままで`folderId`が変わる。
- `folderId: null`で送ると、その行が消える。
- 見分けられないこと：aliceが、bobの`p2`、bobの記事、存在しないUUIDを1件ずつ`fa`に送る。3つとも204で本文が空、`problem_folder_items`と`post_folder_items`のaliceの行は増えない。
- aliceが`folderId`にbobの`fb`を指定すると404 `not_found`。
- テスター：`f.exec("INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'bob')", p1)`の後、bobが`p1`を`fb`に入れられる。aliceの`p1`の振り分けは変わらない。
- テスターから外す：`problem_testers`の行を消すと、bobの`GET /my/folders`の`items`から`p1`が消える。行を入れ直すと、同じ`fb`で戻る。
- 消える順：フォルダを消すと振り分けが消え、`problem_drafts`の`p1`は残る。`f.exec("DELETE FROM problem_drafts WHERE id=$1", p1)`で、aliceとbobの振り分けの行が両方消える。`blog_posts`の行を消すと`post_folder_items`の行が消える。
- 101件送ると400 `invalid_request`。`kind`が`contest`でも400。

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd api && go test ./internal/httpapi/ -run TestFolderItemsPostgres`
Expected: FAIL（404。ルートがない）。

- [ ] **Step 3: `Place`と`Items`を書く**

`Place`は、当てはまるIDだけを1つの文で入れる。当てはまらないIDは何もせず、エラーにもしない。

```sql
INSERT INTO problem_folder_items(owner_id, problem_id, folder_id)
SELECT $1, d.id, $2 FROM problem_drafts d
 WHERE d.id = ANY($3::uuid[]) AND (d.owner_id = $1 OR can_manage_problem(d.id, $1))
ON CONFLICT (owner_id, problem_id) DO UPDATE SET folder_id = EXCLUDED.folder_id
```

記事は`blog_posts`の`owner_id = $1`で同じ形にする。
`Items`は、同じ条件に今も当てはまる行だけを返す。

- [ ] **Step 4: ハンドラとルートを書き、テストが通ることを確かめる**

Run: `cd api && go test ./internal/httpapi/ -run 'TestFolders|TestFolderItems'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal/folders api/internal/httpapi
git commit -m "feat: 問題と記事をフォルダに振り分けるAPIを足す"
```

---

### Task 3: ベータの切り替えとアカウントの削除

**Files:**
- Modify: `api/internal/profiles/store.go`（`Profile.FolderView`、5つのSQL、`SetFolderView`）
- Modify: `api/internal/httpapi/profile.go`、`api/internal/httpapi/routes.go`
- Modify: `api/internal/accounts/store.go:31-53`（`cleanup`）
- Test: `api/internal/httpapi/profile_test.go`、`api/internal/httpapi/account_test.go`

**Interfaces:**
- Produces（Go）：`Profile`に`FolderView bool \`json:"folderView"\``を足す。`func (s *Store) SetFolderView(ctx context.Context, owner string, enabled bool) error`（`version`を上げない。行がなければ`ErrNotFound`、削除済みなら`ErrDeleted`）。
- Produces（HTTP）：`PUT /my/folder-view`、本文`{"enabled":true}`、成功は200 `{"enabled":true}`。`enabled`がなければ400 `invalid_request`。
- `GET /my/profile`と`PUT /my/profile`の応答の`profile`に`folderView`が入る。`PUT /my/profile`は`folderView`を受け取らず、値を変えない。
- `publicProfileResponse`は変えない。

- [ ] **Step 1: テストを書く**

`profile_test.go`のPostgreSQLを使うテストに足す（既存の組み立てに合わせる）。

- 新しい利用者の`GET /my/profile`は`"folderView":false`。
- `PUT /my/folder-view`で`true`にすると、`GET /my/profile`が`true`になり、`version`は変わらない。
- その後`PUT /my/profile`でアイコンを変えても、応答と次の`GET`の`folderView`は`true`のまま。
- `GET /users/{handle}`の本文に`folderView`という文字列がない。

`account_test.go`の「Private data is gone.」の表に足す。削除の前に、`alice-sub`のフォルダ1個と、残る問題（`public`）への振り分け1行を`exec`で入れておく。

```go
`SELECT count(*) FROM content_folders WHERE owner_id='alice-sub'`:        0,
`SELECT count(*) FROM problem_folder_items WHERE owner_id='alice-sub'`:   0,
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd api && go test ./internal/httpapi/ -run 'Profile|Account'`
Expected: FAIL

- [ ] **Step 3: 実装する**

`cleanup`に`DELETE FROM content_folders WHERE owner_id=$1`を足す。振り分けは外部キーで一緒に消える。
`account_test.go:139`の依存に`Folders`は要らない（削除は`accounts.Store`だけで終わる）。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd api && go test ./internal/...`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add api/internal
git commit -m "feat: フォルダ表示の切り替えをアカウントに保存し、アカウントの削除でフォルダを消す"
```

---

### Task 4: テスト中の問題の一覧に作者を足す

**Files:**
- Modify: `api/internal/problems/store.go:180-198`（`Summary`）、`:283-295`（`ownedSummaries`、`testedSummaries`）、`:311-326`（行の読み取り）
- Modify: `web/shared/types/account-problems.ts`（`accountListSchema`の項目に`author: z.string().default('')`）
- Test: `api/internal/httpapi/testers_test.go`

**Interfaces:**
- Produces：`Summary.Author string \`json:"author,omitempty"\``。`GET /my/problems?role=tester`の各項目に、問題の作者のハンドルが入る。`GET /my/problems`（作者として）には入らない。
- Produces（Web）：`AccountSummary['author']: string`。

- [ ] **Step 1: テストを書く**

`TestTesterInvitationsPostgres`で、testerが招待を受けた後の`GET /my/problems?role=tester`を読む箇所に足す。

```go
// items[0].Author == "alice"
// alice の GET /my/problems の本文に `"author"` がない
```

- [ ] **Step 2: 落ちることを確かめ、実装し、通ることを確かめる**

`testedSummaries`の末尾に`(SELECT handle FROM user_profiles u WHERE u.owner_id=problem_drafts.owner_id)`を1列足し、`ownedSummaries`は`NULL::text`を足す。

Run: `cd api && go test ./internal/httpapi/ ./internal/problems/`
Expected: PASS

- [ ] **Step 3: コミット**

```bash
git add api/internal/problems api/internal/httpapi web/shared/types/account-problems.ts
git commit -m "feat: テスト中の問題の一覧に作者のハンドルを足す"
```

---

### Task 5: Webの型、Webサーバーの経路、フォルダの計算

**Files:**
- Create: `web/shared/types/folders.ts`
- Modify: `web/shared/types/profile.ts`（`profileSchema`に`folderView: z.boolean().default(false)`）
- Create: `web/server/api/my/folders/index.ts`（GET、POST）、`web/server/api/my/folders/[id].ts`（PUT、DELETE）、`web/server/api/my/folder-items.ts`（PUT）、`web/server/api/my/folder-view.ts`（PUT）
- Create: `web/app/utils/folder-tree.ts`
- Create: `web/app/utils/library-entries.ts`
- Test: `web/tests/folder-tree.spec.ts`

**Interfaces:**
- Consumes: Task 1〜4のHTTP。
- Produces（`folders.ts`）：

```ts
export const maxFolders = 100, maxFolderDepth = 3, maxFolderName = 40, maxFolderItems = 100
export const folderSchema = z.object({ id: z.string().uuid(), parentId: z.string().uuid().nullable(), name: z.string() })
export const folderRefSchema = z.object({ kind: z.enum(['problem', 'post']), id: z.string().uuid() })
export const folderTreeSchema = z.object({ folders: z.array(folderSchema), items: z.array(folderRefSchema.extend({ folderId: z.string().uuid() })) })
export type Folder = z.infer<typeof folderSchema>
export type FolderRef = z.infer<typeof folderRefSchema>
```

- Produces（`folder-tree.ts`）：

```ts
export const refKey = (ref: FolderRef) => `${ref.kind}:${ref.id}`
export function sortedChildren(folders: Folder[], parentId: string | null): Folder[] // localeCompare(name, 'ja'), then id
export function folderPath(folders: Folder[], id: string): Folder[]                  // 最上位から id まで。知らない id は []
export function descendantIds(folders: Folder[], id: string): string[]               // id 自身は含めない
export function canNest(folders: Folder[], id: string | null, parentId: string | null): boolean
// id が null なら新しいフォルダ。自分自身、自分の下、深さが maxFolderDepth を超える置き先は false。
export function folderCounts(folders: Folder[], placed: Map<string, string>, keys: string[]): { all: number, unsorted: number, byFolder: Map<string, number> }
// placed は refKey → フォルダID。byFolder は下の段のフォルダの分を含む。folders にないIDを指す振り分けは未整理に数える。
```

- Produces（`library-entries.ts`）：

```ts
export type LibraryKind = 'own' | 'testing' | 'post'
export type LibraryStatus = 'preparing' | 'applying' | 'scheduled' | 'published'
export type LibraryEntry = { key: string, ref: FolderRef, kind: LibraryKind, title: string, updatedAt: string, status: LibraryStatus, problem?: AccountSummary, post?: PostSummary }
export const libraryKinds = [{ key: 'own', label: '自分の問題' }, { key: 'testing', label: 'テスト中' }, { key: 'post', label: '記事' }] as const
export const problemStatusOptions = [{ key: 'preparing', label: '準備中' }, { key: 'applying', label: '応募中' }, { key: 'scheduled', label: '公開予約中' }, { key: 'published', label: '公開中' }] as const
export function problemStatusGroup(entry: AccountSummary): LibraryStatus
export function libraryEntries(own: AccountSummary[], testing: AccountSummary[], posts: PostSummary[]): LibraryEntry[] // 更新日時の新しい順、同じなら key の順
```

`PostSummary`は`z.infer<typeof postSummarySchema>`。記事は`publishedVersion`があれば`published`、なければ`preparing`。
`problemStatusOptions`と`problemStatusGroup`は、`SavedProblems.vue:56-64`の`statusOptions`と`statusGroups`を移したものである。`SavedProblems.vue`はここから読むように直し、表示は変えない。

- Webサーバーの経路は`web/server/api/my/favorites/[id].ts`と同じ形にする。`privateHeaders`、書き込みは`requireSameOrigin`と`limitedJSON(event, 16 << 10)`、本文と応答はzodで検査し、応答が合わなければ502。`[id].ts`はIDがUUIDでなければ404。`folder-items.ts`の本文は`{ folderId: uuid | null, items: FolderRef[] }`で、`items`は1〜`maxFolderItems`件。APIのエラーの`code`は、画面では`web/app/utils/account-problems.ts`の`accountError`が読んでいるのと同じ場所（`error.data.data.code`）から読む。

- [ ] **Step 1: テストを書く**

`web/tests/folder-tree.spec.ts`。ブラウザを使わない（`web/tests/polling.spec.ts`と同じく、関数を直接読み込む）。

```ts
const f = (id: string, parentId: string | null, name: string) => ({ id, parentId, name })
const tree = [f(A, null, 'コンテスト用'), f(B, A, '第3回'), f(C, B, '予備'), f(D, null, 'あとで'), f(E, D, '候補')]

expect(sortedChildren(tree, null).map(x => x.name)).toEqual(['あとで', 'コンテスト用'])
expect(folderPath(tree, C).map(x => x.name)).toEqual(['コンテスト用', '第3回', '予備'])
expect(folderPath(tree, MISSING)).toEqual([])
expect(descendantIds(tree, A).sort()).toEqual([B, C].sort())
expect(canNest(tree, null, C)).toBe(false)   // 4段目になる
expect(canNest(tree, null, B)).toBe(true)
expect(canNest(tree, A, C)).toBe(false)      // 自分の下
expect(canNest(tree, A, A)).toBe(false)
expect(canNest(tree, D, B)).toBe(false)      // D と E で2段、B の下は3段目から
expect(canNest(tree, D, A)).toBe(true)
expect(canNest(tree, B, null)).toBe(true)

const placed = new Map([['problem:p1', B], ['problem:p2', C], ['post:x1', A], ['problem:p3', MISSING]])
const counts = folderCounts(tree, placed, ['problem:p1', 'problem:p2', 'post:x1', 'problem:p3', 'problem:p4'])
// counts.all === 5, counts.unsorted === 2, byFolder: A→3, B→2, C→1, D→0
```

`libraryEntries`：自分の問題1件、テスト中1件、記事2件（公開中と下書き）を渡し、`kind`、`status`、`key`（`problem:<id>`、`post:<id>`）、更新日時の新しい順を確かめる。

- [ ] **Step 2: 落ちることを確かめ、実装し、通ることを確かめる**

Run: `cd web && bun run typecheck && bun run build && bun run test tests/folder-tree.spec.ts --output="$TMPDIR/pw" --reporter=line`
Expected: PASS

- [ ] **Step 3: 今の画面が変わっていないことを確かめる**

Run: `cd web && bun run test -c playwright.account.config.ts tests/account/testers.spec.ts tests/account/publication.spec.ts --output="$TMPDIR/pw-account" --reporter=line`
Expected: PASS

- [ ] **Step 4: コミット**

```bash
git add web/shared web/server web/app/utils web/app/components/SavedProblems.vue web/tests/folder-tree.spec.ts
git commit -m "feat: フォルダの型とWebサーバーの経路、フォルダの計算を足す"
```

---

### Task 6: 問題の行の部品を`SavedProblems`から取り出す

フォルダ表示の一覧でも、状態の欄と操作の欄は今と同じものを出す。
`SavedProblems.vue`の中に直接書かれているので、部品に分けて両方から使う。
見た目と動きは変えない。

**Files:**
- Create: `web/app/components/SavedProblemState.vue`（状態の欄。`SavedProblems.vue:95-103`の`span.problem-state`と、そのスタイル）
- Create: `web/app/components/SavedProblemActions.vue`（操作の欄。`:106-112`の`div.problem-actions`と、取り下げの確認ダイアログ、`withdraw`、スタイル）
- Modify: `web/app/components/SavedProblems.vue`

**Interfaces:**
- Produces：`<SavedProblemState :entry="AccountSummary" />`
- Produces：`<SavedProblemActions :entry="AccountSummary" :testing="boolean" />`。取り下げに成功したら、`entry.featuredPreference`を`''`にする（今と同じく、渡された行を書き換える）。

- [ ] **Step 1: 取り出す**

- [ ] **Step 2: 今のテストが通ることを確かめる**

Run: `cd web && bun run typecheck && bun run build && bun run test -c playwright.account.config.ts tests/account/publication.spec.ts tests/account/testers.spec.ts --output="$TMPDIR/pw-account" --reporter=line && bun run test tests/navigation.spec.ts --output="$TMPDIR/pw" --reporter=line`
Expected: PASS。定期便の取り下げを確かめるテストがどれか分からなければ、`grep -rn "取り下げ" web/tests`で探して一緒に流す。

- [ ] **Step 3: コミット**

```bash
git add web/app/components
git commit -m "refactor: 自分の問題の一覧から、状態と操作の欄を部品に分ける"
```

---

### Task 7: スイッチと「問題と記事」の一覧

**Files:**
- Modify: `web/app/pages/my/index.vue`
- Create: `web/app/composables/useLibrary.ts`
- Create: `web/app/components/ContentLibrary.vue`（タブの中身。URLの状態、絞り込み、配置）
- Create: `web/app/components/FolderTree.vue`（フォルダの一覧。このタスクでは選ぶだけ）
- Create: `web/app/components/LibraryTable.vue`（中身の表）
- Create: `web/app/assets/css/library.css`
- Modify: `api/internal/httpapi/browser_fixture_test.go:136`（利用者に`folders`と`folders_many`を足す）、`:142`（依存に`Folders: folders.New(store.Pool())`）。`folders_many`には、問題を51件入れておく。新規作成の枠は1人20件なので、`store.Save(ctx, "folders_many@example.test", <UUID>, 0, draft)`を51回呼び、1回ごとに`DELETE FROM creation_quotas WHERE owner_id='folders_many@example.test'`を流す。タイトルは「大量の問題 01」〜「大量の問題 51」。
- Test: `web/tests/account/folders.spec.ts`

**Interfaces:**
- Consumes: Task 5の型と関数、Task 6の部品、`InlineSwitch`、`StatusFilter`、`ContentActions`、`DifficultyBadge`。
- Produces（`useLibrary`）：

```ts
export function useLibrary(): {
  entries: Ref<LibraryEntry[]>, folders: Ref<Folder[]>, placed: Ref<Map<string, string>>,
  loading: Ref<boolean>, message: Ref<string>,
  load(): Promise<void>,
  createFolder(name: string, parentId: string | null): Promise<Folder | undefined>,
  updateFolder(id: string, name: string, parentId: string | null): Promise<boolean>,
  deleteFolder(id: string): Promise<boolean>,
  place(refs: FolderRef[], folderId: string | null): Promise<boolean>,
}
```

`load`は、3つの一覧を`nextCursor`が空になるまで読み、`/api/my/folders`と合わせる。4つは並べて読む。
書き込みの4つは、先に画面の状態を変え、失敗したら元に戻して`message`にGlobal Constraintsの文言を入れ、`false`（`createFolder`は`undefined`）を返す。
`place`は`maxFolderItems`件ずつに分けて送る。
このタスクでは`load`だけを使う。残りはTask 8とTask 9が使う。

- Produces（URL）：`/my?tab=library`に、`folder`（フォルダのID、`unsorted`、なければ「すべて」）、`kind`（`own`、`testing`、`post`）、`status`（`preparing`など）を持つ。値が知らないものなら、付いていないものとして扱う。書き換えは`router.replace`で行う。
- Produces（`FolderTree`）：`<FolderTree :folders :counts v-model:selected />`。`selected`は`''`（すべて）、`'unsorted'`、フォルダのID。`nav`の`aria-label`は「フォルダ」、選ばれている行は`aria-current="true"`。
- Produces（`LibraryTable`）：`<LibraryTable :entries :subfolders @open="(id: string) => …" />`。`table`の`aria-label`は「問題と記事」、クラスは`content-table content-table--cards`。

**`index.vue`の動き：**

- タブの上に`<InlineSwitch>フォルダ表示（ベータ）</InlineSwitch>`と、説明の一文、意見のリンクを置く。
- 押すと`PUT /api/my/folder-view`を送り、成功したら`profile.value.folderView`を書き換える。失敗したらスイッチを戻し、`role="alert"`で伝える。
- オフのとき、テンプレートのタブと5つのコンポーネントは今のままにする。
- オンのとき、タブは「問題と記事」「コンテスト」「通知」。`tab`が`problems`、`testing`、`posts`なら`library`として扱い、`kind`が付いていなければ順に`own`、`testing`、`post`で絞り込む。
- オフのとき、`tab=library`は`problems`として扱う。

**`ContentLibrary.vue`の動き（設計書の「フォルダの一覧」「中身の一覧」「絞り込み」）：**

- 見出しの横に「新規問題」（`/problems/new?fresh=1`）と「新規記事」（`/blog/new`）。
- 一覧の上に今いる場所を出す。途中の名前は押せる。
- 開いたフォルダの直下のフォルダを、表の先頭にフォルダの行として出す。
- 列は、種類、タイトル、状態、更新日時、難易度、操作（選択の列はTask 8で足す）。問題の状態は`SavedProblemState`、操作は`SavedProblemActions`、記事は`SavedPosts.vue:40-43`と同じ中身。
- テスト中の行は、タイトルの下に「作者: {author}」を出す。`viaContest`があれば、今と同じ「コンテスト「…」の作成者として」も出す。
- 種類と状態の絞り込みは`StatusFilter`を2つ並べる。種類のほうは`aria-label`を「種類で絞り込む」にできるよう、`StatusFilter`に`label`のpropを足す（既定は今の「状態で絞り込む」）。件数は、開いているフォルダの直下の行で数える。
- フォルダの一覧の件数は、絞り込みに関係なく`folderCounts`の値を出す。
- `folder`が`folders`にないIDなら「すべて」を出す。
- フォルダが1つもないときは、フォルダの一覧の下に「フォルダを作ると、問題と記事を出す先ごとに分けられます。」と出す。

- [ ] **Step 1: テストを書く**

`web/tests/account/folders.spec.ts`。ログインは`testers.spec.ts`の`login`と同じ。利用者は`folders`。
フォルダと振り分けは、このタスクでは画面から作れないので、`page.request`で`/api/my/folders`と`/api/my/folder-items`を呼んで用意する（`Origin`ヘッダーが要る。`testers.spec.ts`の403の確認を参照）。

1. **オフでは今のまま**：ログイン直後の`/my`に、タブ「自分の問題」「テスト中の問題」「記事」があり、「問題と記事」がない。ページを開く間に`/api/my/folders`への要求が出ない（`page.on('request')`で数える）。
2. **オンにすると混ざって並ぶ**：問題「フォルダの問題A」と記事「フォルダの記事A」を画面から作る。スイッチを押すと、タブが「問題と記事」「コンテスト」「通知」になり、表「問題と記事」に両方が出る。種類の欄は「自分の問題」「記事」。再読み込みしてもオンのまま。
3. **フォルダと件数**：`page.request`で「コンテスト用」とその下「第3回」を作り、問題を「第3回」に入れる。フォルダの一覧で「コンテスト用」の件数が1、「未整理」に記事が出る。「コンテスト用」を開くと、表に「第3回」のフォルダの行が出て、押すとURLに`folder=<第3回のID>`が付き、今いる場所が「コンテスト用 / 第3回」になる。
4. **絞り込み**：「すべて」で種類を「記事」にすると記事だけになり、URLに`kind=post`が付く。状態を「準備中」にしても記事が残る（下書きは準備中）。
5. **今のURL**：オンのまま`/my?tab=posts`を開くと、「問題と記事」が開き、種類が「記事」で絞り込まれている。
6. **消えたフォルダ**：`/my?tab=library&folder=00000000-0000-4000-8000-000000000000`を開くと、「すべて」が選ばれ、両方の行が出る。
7. **テスト中の行の作者**：aliceが別のコンテキストで問題を作り、招待リンクを発行する。`folders`が受けると、表のその行に「テスト中」と「作者: alice」が出る。
8. **51件以上**：別のコンテキストで`folders_many`としてログインし、スイッチを入れる。フォルダの一覧の「すべて」と「未整理」の件数が51、状態の絞り込みの「準備中」の件数が51になり、表に「大量の問題 01」が出る（51番目に古い行で、1回目の読み込みには入らない）。
9. **オフに戻す**：スイッチを切ると今の3つのタブに戻り、「自分の問題」に「フォルダの問題A」がある。もう一度オンにすると、「第3回」に問題が入ったままである。

最後のテストは、スイッチをオンにして終える（Task 8以降のテストが続きから使う）。

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `cd web && bun run build && bun run test -c playwright.account.config.ts tests/account/folders.spec.ts --output="$TMPDIR/pw-account" --reporter=line`
Expected: FAIL（スイッチがない）。

- [ ] **Step 3: 実装する**

配置は`library.css`に書く。`40rem`以上では、フォルダの一覧を左に`15rem`、右に表を置く。
`40rem`未満の配置はTask 10で作る。このタスクでは、フォルダの一覧を表の上に積む。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `cd web && bun run typecheck && bun run build && bun run test -c playwright.account.config.ts --output="$TMPDIR/pw-account" --reporter=line`
Expected: PASS（アカウントのテスト全部。ほかのテストがオフのまま通ることも確かめる）。

- [ ] **Step 5: コミット**

```bash
git add web api/internal/httpapi/browser_fixture_test.go
git commit -m "feat: マイページにフォルダ表示（ベータ）のスイッチと、問題と記事を混ぜた一覧を足す"
```

---

### Task 8: フォルダの管理と「フォルダへ移動」

**Files:**
- Create: `web/app/components/FolderNameDialog.vue`（作る、名前を変える）
- Create: `web/app/components/FolderPicker.vue`（置き先を選ぶダイアログ）
- Modify: `web/app/components/FolderTree.vue`、`LibraryTable.vue`、`ContentLibrary.vue`、`library.css`
- Test: `web/tests/account/folders.spec.ts`

**Interfaces:**
- Consumes: `useLibrary`の`createFolder`、`updateFolder`、`deleteFolder`、`place`。`canNest`、`descendantIds`、`folderCounts`。
- Produces（`FolderPicker`）：`<FolderPicker ref="picker" :folders :title :disabled-ids />`、`picker.open(): Promise<string | null | undefined>`。選んだフォルダのID、「未整理」なら`null`、やめたら`undefined`を返す。中身は`role="radiogroup"`で、矢印キーで選べる。
- Produces（`LibraryTable`）：`v-model:selected`（`Set<string>`、`refKey`の集まり）と、`@move="(refs: FolderRef[]) => …"`を足す。
- ダイアログは`dialog.leave-dialog`と`v-dialog-dismiss`を使い、`SavedProblems.vue`の取り下げの確認と同じ作りにする。

**動き（設計書の「フォルダの一覧」「ドラッグを使わない操作」）：**

- 「フォルダを作る」は、開いているフォルダの中に作る。「すべて」か「未整理」なら最上位に作る。置けない場所（3段目を開いている）では、ボタンを無効にして理由を`title`と説明文で出す。
- 名前の入力は`maxlength="40"`。空では保存できない。
- フォルダの行の操作は、名前の変更、移動、削除の3つ。行ごとのメニューのボタンの`aria-label`は「{名前}の操作」。
- フォルダの移動は`FolderPicker`で置き先を選ぶ。`canNest`が`false`の置き先は選べない。最上位に出すには「最上位」を選ぶ。
- 削除の確認は「「{名前}」を削除しますか？」「中のフォルダ{N}個も削除されます。問題と記事{M}件は「未整理」に戻り、削除されません。」。Nが0なら1文目を出さない。開いているフォルダが消えたら「すべて」に移る。
- 表の先頭に選択の列を足す（`th.select-cell`、`ProblemSubmissionList.vue`の選択の列と同じ作り）。フォルダの行には付けない。
- 行の操作に「フォルダへ移動」のボタンを足す（`aria-label`は「{タイトル}をフォルダへ移動」）。
- 1件以上選ぶと、表の上に「選んだ{N}件をフォルダへ移動」と「選択を解除」を出す。移動が終わったら選択を解く。
- 絞り込みやフォルダを変えたら、見えなくなった行の選択を解く。
- 失敗したら、`useLibrary`の`message`を表の上に`role="alert"`で出す。

- [ ] **Step 1: テストを書く**

`folders.spec.ts`に足す。どれも画面だけで操作する。

1. **作る、変える**：「フォルダを作る」から「定期便用」を作ると、フォルダの一覧に出る。同じ名前でもう一度作ると「同じ場所に同じ名前のフォルダがあります。別の名前にしてください。何も変わっていません。」が出る。名前を「定期便」に変えると、一覧の表示が変わる。
2. **キーボードで移動**：行の「フォルダへ移動」にフォーカスし、Enter、矢印キーで「定期便」、Enterで決める。行が表から消え、「定期便」の件数が1になる。再読み込みしても入っている。マウスは使わない。
3. **まとめて移動**：2行を選び、「選んだ2件をフォルダへ移動」から「未整理」を選ぶと、2件とも「未整理」に出る。
4. **フォルダの移動**：「定期便」を「コンテスト用」の中へ動かすと、字下げされて出る。「コンテスト用」を「第3回」の中へ動かそうとすると、ダイアログで「第3回」が選べない。
5. **削除**：「コンテスト用」の削除の確認に「中のフォルダ2個も削除されます。」と件数が出る。削除すると、中にあった問題が「未整理」に出て、「自分の問題」の数は変わらない。
6. **上限の深さ**：3段目のフォルダを開くと「フォルダを作る」が無効になる。

- [ ] **Step 2: 落ちることを確かめ、実装し、通ることを確かめる**

Run: `cd web && bun run typecheck && bun run build && bun run test -c playwright.account.config.ts tests/account/folders.spec.ts --output="$TMPDIR/pw-account" --reporter=line`
Expected: PASS

- [ ] **Step 3: コミット**

```bash
git add web
git commit -m "feat: フォルダの作成・名前の変更・移動・削除と、行の「フォルダへ移動」を足す"
```

---

### Task 9: ドラッグ＆ドロップ

**Files:**
- Create: `web/app/composables/useLibraryDrag.ts`
- Modify: `web/app/components/FolderTree.vue`、`LibraryTable.vue`、`ContentLibrary.vue`、`library.css`
- Test: `web/tests/account/folders.spec.ts`

**Interfaces:**
- Produces：

```ts
export type LibraryDrag = { kind: 'items', refs: FolderRef[] } | { kind: 'folder', id: string }
export function useLibraryDrag(): { dragging: Ref<LibraryDrag | null>, start(drag: LibraryDrag, event: DragEvent): void, end(): void }
```

`dragging`は`useState`で持ち、表とフォルダの一覧が同じ値を見る。`start`は`event.dataTransfer`に`text/plain`で件数の文（「{N}件を移動」またはフォルダ名）を入れ、`effectAllowed`を`move`にする。

**動き（設計書の「ドラッグ」）：**

- 行は`draggable="true"`。選択している行をつかんだら、選択している全部を運ぶ。選択していない行をつかんだら、その1行だけを運ぶ。
- 運んでいる間、落とせるフォルダと「未整理」に`drop-target`のクラスを付け、上に乗ったものに`drop-over`を付ける。「すべて」には落とせない。
- 2件以上のときは、`setDragImage`で「{N}件を移動」と書いた小さな要素を出す。
- フォルダの行も`draggable="true"`。`canNest`が`false`のフォルダには`drop-target`を付けず、`dragover`で`preventDefault`しない。フォルダの一覧の余白に落とすと最上位に出る。
- 落としたら`place`か`updateFolder`を呼ぶ。どちらも先に画面を動かし、失敗したら戻す（`useLibrary`が行う）。
- 幅が`40rem`未満と、`(pointer: coarse)`では`draggable`を付けない。

- [ ] **Step 1: テストを書く**

1. **1行を入れる**：行を`locator.dragTo()`でフォルダ「定期便」に落とすと、行が表から消え、件数が増える。再読み込みしても入っている。
2. **まとめて入れる**：2行を選び、片方をつかんで落とすと、2件とも入る。
3. **未整理に出す**：フォルダを開き、行を「未整理」に落とすと、フォルダから出る。
4. **フォルダを入れる**：フォルダを別のフォルダに落とすと、中に入る。深さを超える置き先に落としても、何も変わらない。
5. **失敗したら戻る**：`page.route('**/api/my/folder-items', route => route.fulfill({ status: 503 }))`を掛けてから落とすと、行が表に残り、「保存できませんでした。もう一度お試しください。何も変わっていません。」が出る。`route`を外して再読み込みしても、元のフォルダにある。

- [ ] **Step 2: 落ちることを確かめ、実装し、通ることを確かめる**

Run: `cd web && bun run typecheck && bun run build && bun run test -c playwright.account.config.ts tests/account/folders.spec.ts --output="$TMPDIR/pw-account" --reporter=line`
Expected: PASS

- [ ] **Step 3: コミット**

```bash
git add web
git commit -m "feat: 問題と記事、フォルダをドラッグでフォルダに入れられるようにする"
```

---

### Task 10: スマホの表示と、フォルダの中での新規作成

**Files:**
- Modify: `web/app/components/ContentLibrary.vue`、`FolderTree.vue`、`LibraryTable.vue`、`library.css`
- Modify: `web/app/composables/useProblemDraft.ts:131-161`（最初の保存の後）
- Modify: `web/app/pages/blog/new.vue:63-75`（`save`）
- Test: `web/tests/account/folders.spec.ts`

**Interfaces:**
- Produces（URL）：`/problems/new?fresh=1&folder=<フォルダのID>`、`/blog/new?folder=<フォルダのID>`。
- Consumes: `PUT /api/my/folder-items`。

**動き：**

- `40rem`未満では、フォルダの一覧を左に出さない。表の上に、今のフォルダ名を書いたボタン（`aria-label`は「フォルダを選ぶ」、`aria-expanded`付き）を出し、押すとフォルダの一覧がその下に開く。フォルダを選ぶと閉じる。
- 表はカードになる（`content-table--cards`）。各欄に、今の一覧と同じ`lead-cell`、`corner-cell`、`line-cell`、`actions-cell`を付け、種類は`lead-cell`の先頭に出す。
- 横にはみ出さない（`document.documentElement.scrollWidth <= innerWidth`）。
- フォルダを開いているとき、「新規問題」「新規記事」のリンクに`folder`を付ける。「すべて」と「未整理」では付けない。
- 作成画面は、`folder`がUUIDで、かつ保存の前の版が0（新しく作った）のときだけ、保存が成功した直後に`PUT /api/my/folder-items`を1回送る。送るのは`router.replace`でURLから`folder`が消える前である。
- 振り分けが失敗しても、保存は成功として扱い、何も出さない（未整理に出る）。

- [ ] **Step 1: テストを書く**

1. **スマホ幅**：`page.setViewportSize({ width: 375, height: 812 })`。「フォルダを選ぶ」を押してフォルダを選ぶと、中身が切り替わる。行の「フォルダへ移動」で別のフォルダに入れられる。横にはみ出さない。`testInfo.outputPath`にスクリーンショットを残す。行に`draggable`の属性がない。
2. **フォルダの中で新規問題**：フォルダを開いて「新規問題」を押し、タイトルを入れて保存する。マイページに戻ると、そのフォルダに出る。
3. **フォルダの中で新規記事**：同じく記事で確かめる。
4. **既存の問題を開いたとき**：`/problems/new?problem=<既存>&folder=<別のフォルダ>`で保存しても、振り分けは変わらない。

- [ ] **Step 2: 落ちることを確かめ、実装し、通ることを確かめる**

Run: `cd web && bun run typecheck && bun run build && bun run test -c playwright.account.config.ts --output="$TMPDIR/pw-account" --reporter=line && bun run test --output="$TMPDIR/pw" --reporter=line`
Expected: PASS（Webのテスト全部）。

- [ ] **Step 3: コミット**

```bash
git add web
git commit -m "feat: フォルダ表示をスマホ幅に合わせ、フォルダの中で作った問題と記事をそのフォルダに入れる"
```

---

### Task 11: 文書

**Files:**
- Create: `docs/adr/0044-sort-own-content-into-folders.md`（番号は、作業を始める時点の次の番号）
- Modify: `docs/adr/README.md`（記録一覧に1行）
- Modify: `web/app/content/docs/authoring.md:78`付近
- Modify: `web/README.md:182`付近、`:322`の節
- Modify: `docs/superpowers/specs/2026-11-26-my-page-folders-design.md`（状態を「実装済み」にする）

- [ ] **Step 1: ADRを書く**

`docs/adr/0040-favorite-contest-problems-during-contest.md`の形に合わせる（状態、決定日、範囲、背景、決定、代替案、影響）。
決定に書くのは、1つのものは1つのフォルダに入れること、3種類をまたぐこと、振り分けを人ごとに持つこと、一覧のAPIを変えずに画面で突き合わせること、ベータとして利用者が切り替えること。
代替案に書くのは、複数付けられるラベル、タブごとのフォルダ、コンテストや依頼者からの自動の分類、ブラウザだけに保存する切り替え。それぞれ、採らなかった理由を設計書と会話の記録から書く。

- [ ] **Step 2: ガイドとREADMEを直す**

`authoring.md`の「保存した問題はマイページの「自分の問題」から開けます。」の後に、フォルダ表示の短い節を足す。
読み手は作問者1人に絞り、ベータであること、スイッチの場所、オフに戻せること、戻してもフォルダが残ることを、箇条書きで書く。
書いたら、`yomiyasu`スキルで推敲してから見せる。

- [ ] **Step 3: 全体を確かめる**

Run: `cd api && go vet ./... && go test ./...`、`cd web && bun run typecheck && bun run build && bun run test --output="$TMPDIR/pw" --reporter=line && bun run test -c playwright.account.config.ts --output="$TMPDIR/pw-account" --reporter=line`
Expected: PASS

- [ ] **Step 4: コミット**

```bash
git add docs web/app/content/docs/authoring.md web/README.md
git commit -m "docs: マイページのフォルダ分けのADRとガイドを足す"
graphify update .
```

---

## 配布（この計画の外）

mainに入れると自動で配布される。
移行ファイル、API、Webの順に出る必要があるので、今の自動配布がこの順で進むことを、入れる前に確かめる。
既定がオフなので、配布しただけでは誰の画面も変わらない。
配布の後、本番で自分のアカウントのスイッチを入れ、作成・振り分け・削除を1回ずつ確かめる。
