package httpapi

import (
	"context"
	"net/http"
	"time"

	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
)

type testFileStore interface {
	Begin(context.Context, string, string, string, int64, string) (testfiles.Upload, error)
	Complete(context.Context, string, string, string) (problems.TestFile, error)
	Download(context.Context, string, string, string) (testfiles.Download, error)
}

type handlerDependencies struct {
	Contests             *contests.Store
	Images               *images.Store
	Notifications        *notifications.Store
	DispatchJudge        func(context.Context) error
	JudgeRuntime         string
	JudgeEnabledRuntimes string
	Submissions          *submissions.Store
	JudgeImage           string
	Posts                *posts.Store
	Operators            map[string]bool
	Store                problems.Repository
	Profiles             profiles.Repository
	Verifier             TokenVerifier
	Files                testFileStore
}

type problemHandler struct {
	Store   problems.Repository
	Files   testFileStore
	Judging submissions.Config
}
type postHandler struct {
	Posts     *posts.Store
	Operators map[string]bool
}
type (
	profileHandler    struct{ Profiles profiles.Repository }
	submissionHandler struct{ submissions.Service }
	contestHandler    struct {
		Contests    *contests.Store
		Submissions *submissions.Store
		Judging     submissions.Config
	}
)

type (
	imageHandler        struct{ Store *images.Store }
	notificationHandler struct{ Store *notifications.Store }
)

// releaseBefore keeps local and end-of-contest reads current without coupling unrelated APIs to release.
func releaseBefore(store *contests.Store, next actorHandler) actorHandler {
	return func(w http.ResponseWriter, r *http.Request, actor string) {
		if store != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			err := store.Release(ctx)
			cancel()
			if err != nil {
				authError(w, 503, "database_unavailable")
				return
			}
		}
		next(w, r, actor)
	}
}

func registerRoutes(mux *http.ServeMux, d handlerDependencies) {
	docs := newPublicDocumentation()
	documentPublicRoute(docs, "GET /health", publicOperation{Summary: "稼働状態", Response: healthResponse{}})
	judging := submissions.Config{JudgeImage: d.JudgeImage, JudgeRuntime: d.JudgeRuntime, JudgeEnabledRuntimes: d.JudgeEnabledRuntimes}
	problems := problemHandler{Store: d.Store, Files: d.Files, Judging: judging}
	posts := postHandler{Posts: d.Posts, Operators: d.Operators}
	profiles := profileHandler{Profiles: d.Profiles}
	submissions := submissionHandler{Service: submissions.Service{Config: judging, Submissions: d.Submissions, Problems: d.Store, DispatchJudge: d.DispatchJudge}}
	contests := contestHandler{Contests: d.Contests, Submissions: d.Submissions, Judging: judging}
	images := imageHandler{Store: d.Images}
	notifications := notificationHandler{Store: d.Notifications}
	auth := authentication{Verifier: d.Verifier, Profiles: d.Profiles}
	public := func(pattern string, handler http.HandlerFunc, release bool, doc publicOperation) {
		documentPublicRoute(docs, pattern, doc)
		if !release {
			mux.HandleFunc(pattern, handler)
			return
		}
		released := releaseBefore(d.Contests, func(w http.ResponseWriter, r *http.Request, _ string) { handler(w, r) })
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			released(w, r, "")
		})
	}

	private := func(pattern string, handler actorHandler, release bool) {
		if release {
			handler = releaseBefore(d.Contests, handler)
		}
		mux.HandleFunc(pattern, auth.require(handler, true, 10*time.Second))
	}
	public("GET /users/{handle}", profiles.publicProfile, false, publicOperation{Summary: "公開プロフィール", Response: publicProfileResponse{}})
	public("GET /contests", contests.publicContest, true, publicOperation{Summary: "コンテスト一覧", Description: "開始日時の降順。各要素のproblemsは空配列です。問題一覧は詳細APIで取得します。", Response: publicContestPage{}, Parameters: offsetParameters()})
	public("GET /contests/{id}", contests.publicContest, true, publicOperation{Summary: "コンテスト詳細", Description: "開始前も出題順と配点を公開します。問題ID・タイトル・制限値は作成者と担当の作問者・テスターだけに返します。", Response: publicContestResponse{}})
	public("GET /contests/{id}/problems/{problem}", contests.publicContest, true, publicOperation{Summary: "コンテストの問題", Description: "開始後に取得可能です。解説は終了後に公開されます。publishedAtはコンテスト終了日時です。", Response: publicProblemResponse{}})
	public("GET /contests/{id}/problems/{problem}/submissions", contests.publicContest, true, publicOperation{Summary: "コンテストの問題別提出一覧", Description: "コンテスト終了後に取得可能です。ソースコードとケース別結果は含みません。mine=1は認証が必要なため公開APIでは利用できません。", Response: publicSubmissionPage{}, Parameters: offsetParameters()})
	public("GET /contests/{id}/standings", contests.publicContest, true, publicOperation{Summary: "コンテスト順位表", Response: publicStandingsResponse{}})
	public("GET /contests/{id}/submissions", contests.publicContest, true, publicOperation{Summary: "コンテスト提出一覧", Description: "コンテスト終了後に取得可能です。提出日時の降順。ソースコードとケース別結果は含みません。", Response: publicSubmissionPage{}, Parameters: offsetParameters()})
	public("GET /contests/{id}/submissions/{submission}", contests.publicContest, true, publicOperation{Summary: "コンテスト提出詳細", Description: "コンテスト終了後に取得可能です。非公開テストの入出力やチェッカー診断は返しません。", Response: publicSubmissionResponse{}})
	public("GET /images/{id}", images.publicImage, false, publicOperation{Summary: "公開画像", Description: "公開コンテンツから参照される画像をPNGまたはJPEGで返します。", Image: true})
	public("GET /runtimes", submissions.runtimes, false, publicOperation{Summary: "利用可能な言語", Response: runtimesResponse{}})
	public("GET /problems", problems.publicProblems, true, publicOperation{Summary: "公開問題一覧", Description: "公開日時の降順。本文・解説は詳細APIで取得します。", Response: publicProblemPage{}, Parameters: catalogueParameters()})
	public("GET /problems/{id}", problems.publicProblems, true, publicOperation{Summary: "公開問題詳細", Description: "公開版の問題文・解説・制限値。timeLimitMsとmemoryLimitMbは数値の文字列です。下書きは返しません。", Response: publicProblemResponse{}})
	public("GET /problems/{id}/samples", problems.publicSamples, true, publicOperation{Summary: "公開サンプルケース", Description: "公開版でサンプルに指定したケースを保存順に返します。空ならitemsは空配列です。空白・改行を保持します。Markdownだけに書かれた例は抽出しません。inputFile/outputFileがある場合はurlから本文を取得してください。URLは10分間有効で、非公開化しても発行済みURLは期限まで利用できます。", Response: itemsResponse[publicSample]{}})
	public("GET /problems/{id}/submissions", submissions.publicProblemSubmissions, true, publicOperation{Summary: "公開問題の提出一覧", Description: "提出日時の降順。下書き・サンプル実行・開催中コンテストの提出は非公開です。ソースコードとケース別結果は含みません。mine=1は公開APIでは利用できません。", Response: publicSubmissionPage{}, Parameters: offsetParameters()})
	public("GET /problems/{id}/submissions/{submission}", submissions.publicProblemSubmissions, true, publicOperation{Summary: "公開提出詳細", Description: "ソースコードと公開採点結果。非公開テストの入出力やチェッカー診断は返しません。", Response: publicSubmissionResponse{}})
	public("GET /posts", posts.publicPosts, false, publicOperation{Summary: "公開記事一覧", Description: "公開日時の降順。本文は詳細APIで取得します。", Response: publicPostPage{}, Parameters: catalogueParameters()})
	public("GET /posts/{id}", posts.publicPosts, false, publicOperation{Summary: "公開記事詳細", Response: publicPostResponse{}})
	registerDocumentation(mux, docs)
	private("GET /my/images", images.contentImage, false)
	private("POST /my/images", images.contentImage, false)
	private("GET /my/images/{id}", images.contentImage, false)
	private("DELETE /my/images/{id}", images.contentImage, false)
	private("GET /my/notifications", notifications.notifications, false)
	private("POST /my/notifications/read", notifications.notifications, false)
	private("POST /my/problems/{id}/tester-invitation", problems.testerInvitation, false)
	private("GET /my/tester-invitations/{token}", problems.testerInvitation, false)
	private("POST /my/tester-invitations/{token}", problems.testerInvitation, false)
	private("GET /my/contests", contests.contest, true)
	private("GET /my/contests/{id}", contests.contest, true)
	private("PUT /my/contests/{id}", contests.contest, true)
	private("POST /my/contests/{id}/participation", contests.contest, true)
	private("GET /my/contests/{id}/problems/{problem}", contests.contest, true)
	private("GET /my/contests/{id}/problems/{problem}/submissions", contests.contest, true)
	private("GET /my/contests/{id}/submissions", contests.contest, true)
	private("GET /my/contests/{id}/submissions/{submission}", contests.contest, true)
	private("GET /my/solved-problems", problems.solvedProblems, true)
	private("GET /my/difficulty-votes/{id}", problems.difficultyVote, true)
	private("PUT /my/difficulty-votes/{id}", problems.difficultyVote, true)
	private("DELETE /my/difficulty-votes/{id}", problems.difficultyVote, true)
	private("GET /my/favorites/{id}", problems.favorite, true)
	private("PUT /my/favorites/{id}", problems.favorite, true)
	private("POST /my/submissions", submissions.submission, true)
	private("GET /my/submissions", submissions.submission, false)
	private("GET /my/submissions/{id}", submissions.submission, false)
	private("PUT /my/problems/{id}/publication", problems.publishProblem, true)
	private("GET /my/posts", posts.privatePost, false)
	private("GET /my/posts/{id}", posts.privatePost, false)
	private("PUT /my/posts/{id}", posts.privatePost, false)
	private("DELETE /my/posts/{id}", posts.privatePost, false)
	private("PUT /my/posts/{id}/publication", posts.privatePost, false)
	mux.HandleFunc("GET /auth/me", auth.require(func(w http.ResponseWriter, r *http.Request, owner string) {
		writeAuthJSON(w, 200, map[string]string{"id": owner})
	}, false, 10*time.Second))
	mux.HandleFunc("GET /my/profile", auth.require(profiles.profile, false, 10*time.Second))
	mux.HandleFunc("PUT /my/profile", auth.require(profiles.profile, false, 10*time.Second))
	private("GET /my/problems", problems.problem, true)
	private("GET /my/problems/{id}", problems.problem, true)
	private("GET /my/problems/{id}/submissions", submissions.problemSubmissions, true)
	private("GET /my/problems/{id}/submissions/{submission}", submissions.problemSubmissions, true)
	private("PUT /my/problems/{id}", problems.problem, true)
	private("DELETE /my/problems/{id}", problems.problem, true)
	mux.HandleFunc("POST /my/problems/{id}/test-files", auth.require(problems.testFile, true, 25*time.Second))
	mux.HandleFunc("GET /my/problems/{id}/test-files/{file}", auth.require(problems.testFile, true, 25*time.Second))
	mux.HandleFunc("POST /my/problems/{id}/test-files/{file}/complete", auth.require(problems.testFile, true, 25*time.Second))
}
