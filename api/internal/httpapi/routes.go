package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"judge/api/internal/accounts"
	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
	"judge/api/internal/tags"
	"judge/api/internal/testfiles"
)

type testFileStore interface {
	Begin(context.Context, string, string, string, int64, string) (testfiles.Upload, error)
	Complete(context.Context, string, string, string) (problems.TestFile, error)
	Download(context.Context, string, string, string) (testfiles.Download, error)
}

type handlerDependencies struct {
	Accounts             *accounts.Store
	CognitoUsers         CognitoUsers
	UserPoolID           string
	Contests             *contests.Store
	Images               *images.Store
	Notifications        *notifications.Store
	DispatchJudge        func(context.Context) error
	JudgeRuntime         string
	JudgeEnabledRuntimes string
	Submissions          *submissions.Store
	JudgeImage           string
	Posts                *posts.Store
	Tags                 *tags.Store
	Operators            map[string]bool
	Store                *problems.Store
	Profiles             *profiles.Store
	Verifier             TokenVerifier
	Files                testFileStore
	LocalFiles           http.Handler
}

type problemHandler struct {
	Store   *problems.Store
	Files   testFileStore
	Judging submissions.Config
}
type postHandler struct {
	Posts     *posts.Store
	Operators map[string]bool
}
type (
	profileHandler    struct{ Profiles *profiles.Store }
	submissionHandler struct {
		submissions.Service
		Operators map[string]bool
	}
	contestHandler struct {
		Contests    *contests.Store
		Submissions *submissions.Store
		Judging     submissions.Config
		Operators   map[string]bool
	}
)

type (
	imageHandler        struct{ Store *images.Store }
	notificationHandler struct{ Store *notifications.Store }
	tagHandler          struct{ Store *tags.Store }
)

// releaseBefore keeps local and end-of-contest reads current without coupling unrelated APIs to release.
func releaseBefore(store *contests.Store, problemStore *problems.Store, judging submissions.Config, next actorHandler) actorHandler {
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
		if problemStore != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			err := problemStore.ReleaseScheduled(ctx)
			if err == nil {
				err = problemStore.ReleaseFeatured(ctx, submissions.RuntimeIDs(), judging.ContentRuntimeIDs())
			}
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
	if d.LocalFiles != nil {
		mux.Handle("/local-test-files", d.LocalFiles)
	}
	docs := newPublicDocumentation()
	documentPublicRoute(docs, "GET /health", publicOperation{Summary: "稼働状態", Response: healthResponse{}})
	judging := submissions.Config{JudgeImage: d.JudgeImage, JudgeRuntime: d.JudgeRuntime, JudgeEnabledRuntimes: d.JudgeEnabledRuntimes}
	problems := problemHandler{Store: d.Store, Files: d.Files, Judging: judging}
	posts := postHandler{Posts: d.Posts, Operators: d.Operators}
	profiles := profileHandler{Profiles: d.Profiles}
	submissions := submissionHandler{Service: submissions.Service{Config: judging, Submissions: d.Submissions, Problems: d.Store, DispatchJudge: d.DispatchJudge}, Operators: d.Operators}
	contests := contestHandler{Contests: d.Contests, Submissions: d.Submissions, Judging: judging, Operators: d.Operators}
	images := imageHandler{Store: d.Images}
	notifications := notificationHandler{Store: d.Notifications}
	tagList := tagHandler{Store: d.Tags}
	auth := authentication{Verifier: d.Verifier, Profiles: d.Profiles}
	public := func(pattern string, handler http.HandlerFunc, release bool, doc publicOperation) {
		documentPublicRoute(docs, pattern, doc)
		// Public handlers get the deadline of the routes that require an account; the release keeps its own.
		limited := func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			handler(w, r.WithContext(ctx))
		}
		if !release {
			mux.HandleFunc(pattern, limited)
			return
		}
		released := releaseBefore(d.Contests, d.Store, judging, func(w http.ResponseWriter, r *http.Request, _ string) { limited(w, r) })
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			released(w, r, "")
		})
	}

	private := func(pattern string, handler actorHandler, release bool) {
		if release {
			handler = releaseBefore(d.Contests, d.Store, judging, handler)
		}
		mux.HandleFunc(pattern, auth.require(handler, true, 10*time.Second))
	}
	public("GET /users/{handle}", profiles.publicProfile, false, publicOperation{Summary: "公開プロフィール", Response: publicProfileResponse{}})
	public("GET /contests", anonymous(contests.contestList), true, publicOperation{Summary: "コンテスト一覧", Description: "開始日時の降順。各要素のproblemsは空配列です。問題一覧は詳細APIで取得します。", Response: publicContestPage{}, Parameters: offsetParameters()})
	public("GET /contests/{id}", anonymous(contests.contestDetail), true, publicOperation{Summary: "コンテスト詳細", Description: "開始前も出題順と配点を公開します。問題ID・タイトル・制限値は作成者と担当の作問者・テスターだけに返します。", Response: publicContestResponse{}})
	public("GET /contests/{id}/problems/{problem}", anonymous(contests.contestProblem), true, publicOperation{Summary: "コンテストの問題", Description: "開始後に取得可能です。解説は終了後に公開されます。publishedAtはコンテスト終了日時です。authorはコンテストのオーナー、authorsはオーナーを先頭とするコンテストの作成者です。", Response: publicContestProblem{}})
	public("GET /contests/{id}/problems/{problem}/submissions", anonymous(contests.contestProblemSubmissions), true, publicOperation{Summary: "コンテストの問題別提出一覧", Description: "コンテスト終了後に取得可能です。順位表を凍結したコンテストでは、凍結後の提出を結果の公開まで含みません。ソースコードとケース別結果は含みません。mine=1とmine=teamは認証が必要なため公開APIでは利用できません。", Response: publicSubmissionPage{}, Parameters: onsiteSubmissionParameters(offsetParameter("先頭からスキップする件数。1ページ最大50件。"))})
	public("GET /contests/{id}/standings", anonymous(contests.contestStandings), true, publicOperation{Summary: "コンテスト順位表", Description: "順位表を凍結したコンテストでは、結果の公開まで凍結前の提出だけで計算し、凍結後の提出の件数をfrozenで返します。チーム戦では、チームの行はteamにチーム名とメンバーを返してhandleを空文字にし、個人で参加した人の行はhandleを返してteamをnullにします。onsiteはオンサイト区分の行です。", Response: publicStandingsResponse{}, Parameters: []*huma.Param{onsiteParameter("onsiteを指定すると、オンサイト区分の行だけを返します。rankはオンサイトの中で振り直した順位、overallRankは全体の順位です。区分のないコンテストでは空配列です。")}})
	public("GET /contests/{id}/submissions", anonymous(contests.contestSubmissions), true, publicOperation{Summary: "コンテスト提出一覧", Description: "コンテスト終了後に取得可能です。順位表を凍結したコンテストでは、凍結後の提出を結果の公開まで含みません。提出日時の降順。ソースコードとケース別結果は含みません。mine=1とmine=teamは認証が必要なため公開APIでは利用できません。", Response: publicContestSubmissionPage{}, Parameters: append(onsiteSubmissionParameters(offsetParameter("先頭からスキップする件数。1ページの件数はlimitで指定します。")), limitParameter(), problemFilterParameter())})
	public("GET /contests/{id}/editorials", posts.editorials, true, publicOperation{Summary: "コンテストのユーザー解説", Description: "コンテスト終了後に、各問題を解説する公開記事を公開日時の昇順で返します。problemIdで問題ごとにまとめてください。", Response: publicEditorialList{}})
	public("GET /contests/{id}/submissions/{submission}", anonymous(contests.contestSubmission), true, publicOperation{Summary: "コンテスト提出詳細", Description: "コンテスト終了後に取得可能です。凍結後の提出は結果の公開まで取得できません。非公開テストの入出力やチェッカー診断は返しません。", Response: publicSubmissionResponse{}})
	public("GET /images/{id}", images.publicImage, true, publicOperation{Summary: "公開画像", Description: "公開コンテンツから参照される画像をPNGまたはJPEGで返します。", Image: true})
	public("GET /runtimes", submissions.runtimes, false, publicOperation{Summary: "利用可能な言語", Response: runtimesResponse{}})
	public("GET /submissions/{submission}", submissions.publicProblemSubmissions, true, publicOperation{Summary: "提出IDから公開提出を取得", Description: "問題・コンテストの提出詳細と同じ公開範囲です。非公開の提出や公開待ちの提出は404を返します。", Response: publicSubmissionResponse{}})
	public("GET /featured", problems.featured, true, publicOperation{Summary: "定期便", Description: "毎週月・木22:00 (JST)のEasy/Hard各一問。新作の解説と他者の提出は翌22時に公開。20回ずつ出題履歴を返します。", Response: featuredPageResponse{}, Parameters: offsetParameters()})
	public("GET /featured/schedule", problems.featuredSchedule, true, publicOperation{Summary: "定期便の予定", Description: "次回から8回分の予定。各回のEasy/Hardに難易度・新作／復刻／欠番を返します。confirmed=trueの次回は次回予告と同じ選出で、2回目以降は今の応募が残ると仮定した見込みです。問題名・問題ID・作者名は返しません。復刻する問題は出題時に選ぶため、復刻の枠では難易度が空です（次回を除く）。", Response: featuredPlanResponse{}})
	public("GET /featured/standings", problems.featuredStandings, true, publicOperation{Summary: "定期便の順位表", Description: "対象回の公開から翌22時までの公開問題への提出を集計します。Hardの点数、Easyの点数の順に比べ、各問題は満点を先に、次に1つの提出の最高点（100倍の整数、部分点のない問題はnull）を比べます。部分点のない回は両方正解、Hardのみ、Easyのみ、0完の順。同じ点数は同順位で、同順位内はその点数に到達した提出時刻順（両方0点は最初の提出順）。その点数に届いた経過時間と、それまでの不正解数を表示し、誤答ペナルティはありません。作問者・テスターと確認用提出は対象外。提出の本文やIDは返しません。", Response: featuredStandingsResponse{}, Parameters: featuredStandingsParameters()})
	public("GET /problems", problems.publicProblems, true, publicOperation{Summary: "公開問題一覧", Description: "公開日時の降順。本文・解説は詳細APIで取得します。", Response: publicProblemPage{}, Parameters: problemParameters()})
	public("GET /problems/{id}", problems.publicProblems, true, publicOperation{Summary: "公開問題詳細", Description: "公開版の問題文・解説・制限値。定期便の新作はeditorialHidden=trueの間、解説を返しません。editorialRevealAtが解禁時刻です。timeLimitMsとmemoryLimitMbは数値の文字列です。下書きは返しません。", Response: publicProblemResponse{}})
	public("GET /problems/{id}/editorials", posts.editorials, true, publicOperation{Summary: "公開問題のユーザー解説", Description: "問題を解説する公開記事を公開日時の昇順で返します。公式解説を誰でも読めるようになるまで、itemsは空配列です。", Response: publicEditorialList{}})
	public("GET /problems/{id}/samples", problems.publicSamples, true, publicOperation{Summary: "公開サンプルケース", Description: "公開版でサンプルに指定したケースを保存順に返します。空ならitemsは空配列です。空白・改行を保持します。Markdownだけに書かれた例は抽出しません。inputFile/outputFileがある場合はurlから本文を取得してください。URLは10分間有効で、非公開化しても発行済みURLは期限まで利用できます。", Response: itemsResponse[publicSample]{}})
	public("GET /problems/{id}/submissions", submissions.publicProblemSubmissions, true, publicOperation{Summary: "公開問題の提出一覧", Description: "提出日時の降順。下書き・サンプル実行・開催中コンテストの提出と、結果の公開前のコンテストの凍結後の提出は非公開です。ソースコードとケース別結果は含みません。mine=1は公開APIでは利用できません。", Response: publicSubmissionPage{}, Parameters: offsetParameters()})
	public("GET /problems/{id}/submissions/{submission}", submissions.publicProblemSubmissions, true, publicOperation{Summary: "公開提出詳細", Description: "ソースコードと公開採点結果。非公開テストの入出力やチェッカー診断は返しません。", Response: publicSubmissionResponse{}})
	public("GET /posts", posts.publicPosts, false, publicOperation{Summary: "公開記事一覧", Description: "公開日時の降順。本文は詳細APIで取得します。", Response: publicPostPage{}, Parameters: catalogueParameters()})
	public("GET /tags", tagList.list, true, publicOperation{Summary: "使われているタグ", Description: "公開中の問題と記事に付いたタグを、使われている数の多い順に最大1000件返します。定期便の新作のタグは解説の公開まで数えません。", Response: publicTagList{}})
	public("GET /posts/{id}", posts.publicPosts, true, publicOperation{Summary: "公開記事詳細", Description: "problemsは記事が解説する問題です。公式解説を誰でも読める問題だけを返します。", Response: publicPostResponse{}})
	registerDocumentation(mux, docs)
	private("GET /my/images", images.contentImage, false)
	private("POST /my/images", images.contentImage, false)
	private("GET /my/images/{id}", images.contentImage, true)
	private("DELETE /my/images/{id}", images.contentImage, false)
	private("GET /my/notifications", notifications.notifications, false)
	private("POST /my/notifications/read", notifications.notifications, false)
	private("POST /my/problems/{id}/tester-invitation", problems.testerInvitation, false)
	private("GET /my/tester-invitations/{token}", problems.testerInvitation, false)
	private("POST /my/tester-invitations/{token}", problems.testerInvitation, false)
	private("GET /my/contests", contests.contestList, true)
	private("GET /my/contest-schedule", contests.contestSchedule, false)
	private("GET /my/contests/{id}", contests.contestDetail, true)
	private("PUT /my/contests/{id}", contests.saveContest, true)
	private("GET /my/contests/drafts", contests.contestDrafts, false)
	private("GET /my/contests/{id}/draft", contests.contestDraft, false)
	private("PUT /my/contests/{id}/draft", contests.saveContestDraft, false)
	private("DELETE /my/contests/{id}/draft", contests.deleteContestDraft, false)
	private("PUT /my/contests/{id}/publication", contests.publishContestDraft, true)
	private("POST /my/contests/{id}/participation", contests.joinContest, true)
	mux.HandleFunc("GET /my/contests/{id}/clarifications", auth.require(contests.clarifications, false, 10*time.Second))
	private("POST /my/contests/{id}/clarifications", contests.clarifications, false)
	private("PUT /my/contests/{id}/clarifications/{clarification}", contests.updateClarification, false)
	private("POST /my/contests/{id}/setter-invitation", contests.setterInvitation, false)
	private("PUT /my/contests/{id}/setter-invitation", contests.setterInvitation, false)
	private("GET /my/setter-invitations/{token}", contests.setterInvitation, false)
	private("POST /my/setter-invitations/{token}", contests.setterInvitation, false)
	private("DELETE /my/contests/{id}/setters/{handle}", contests.setter, false)
	private("GET /my/contests/{id}/problems/{problem}", contests.contestProblem, true)
	private("GET /my/contests/{id}/problems/{problem}/submissions", contests.contestProblemSubmissions, true)
	private("GET /my/contests/{id}/submissions", contests.contestSubmissions, true)
	private("GET /my/contests/{id}/submissions/{submission}", contests.contestSubmission, true)
	private("GET /my/contests/{id}/standings", contests.contestStandings, true)
	private("POST /my/contests/{id}/reveal", contests.revealContest, true)
	private("GET /my/contests/{id}/team", contests.myTeam, false)
	private("PUT /my/contests/{id}/team/code", contests.renewTeamCode, false)
	private("PUT /my/contests/{id}/team/name", contests.renameTeam, false)
	private("DELETE /my/contests/{id}/team/members/{handle}", contests.removeTeamMember, false)
	private("GET /my/contests/{id}/onsite", contests.venue, false)
	private("PUT /my/contests/{id}/onsite", contests.venue, false)
	private("GET /my/contests/{id}/participants", contests.entrants, false)
	private("PUT /my/contests/{id}/participants/{handle}/onsite", contests.setParticipantOnsite, false)
	private("PUT /my/contests/{id}/teams/{team}/onsite", contests.setTeamOnsite, false)
	private("PUT /my/contests/{id}/teams/{team}/name", contests.renameEntrantTeam, false)
	private("GET /my/solved-problems", problems.solvedProblems, true)
	private("GET /my/difficulty-votes/{id}", problems.difficultyVote, true)
	private("PUT /my/difficulty-votes/{id}", problems.difficultyVote, true)
	private("DELETE /my/difficulty-votes/{id}", problems.difficultyVote, true)
	private("GET /my/favorites/{id}", problems.favorite, true)
	private("PUT /my/favorites/{id}", problems.favorite, true)
	private("POST /my/submissions", submissions.submission, true)
	private("GET /my/submissions", submissions.submission, false)
	private("POST /my/code-runs", submissions.codeRuns, false)
	private("GET /my/code-runs/latest", submissions.codeRuns, false)
	private("GET /my/submissions/{id}", submissions.submission, false)
	private("GET /my/submissions/{submission}/public", submissions.problemSubmissions, true)
	private("GET /my/featured", problems.featuredApplication, true)
	private("GET /my/featured/schedule", problems.myFeaturedSchedule, true)
	private("GET /my/featured/next", problems.myFeaturedPreview, true)
	private("PUT /my/problems/{id}/featured", problems.featuredApplication, true)
	private("GET /my/problems/{id}/publication", problems.publishedProblems, true)
	private("PUT /my/problems/{id}/publication", problems.publishProblem, true)
	private("GET /my/posts", posts.privatePost, false)
	private("GET /my/posts/{id}", posts.privatePost, true)
	private("PUT /my/posts/{id}", posts.privatePost, true)
	private("DELETE /my/posts/{id}", posts.privatePost, false)
	private("PUT /my/posts/{id}/publication", posts.privatePost, false)
	mux.HandleFunc("GET /auth/me", auth.require(func(w http.ResponseWriter, r *http.Request, owner string) {
		writeAuthJSON(w, 200, map[string]string{"id": owner})
	}, false, 10*time.Second))
	account := accountHandler{Users: d.CognitoUsers, PoolID: d.UserPoolID, Accounts: d.Accounts}
	private("GET /my/account", account.account, false)
	private("POST /my/password", account.password, false)
	// Releasing first publishes problems of contests that just ended, so they stay instead of being deleted as drafts.
	private("DELETE /my/account", account.remove, true)
	mux.HandleFunc("GET /my/profile", auth.require(profiles.profile, false, 10*time.Second))
	mux.HandleFunc("PUT /my/profile", auth.require(profiles.profile, false, 10*time.Second))
	private("GET /my/problems", problems.problem, true)
	private("GET /my/problems/{id}", problems.problem, true)
	private("GET /my/problems/{id}/submissions", submissions.problemSubmissions, true)
	private("GET /my/problems/{id}/submissions/{submission}", submissions.problemSubmissions, true)
	private("GET /my/problems/{id}/rejudge", submissions.rejudge, false)
	private("POST /my/problems/{id}/rejudge", submissions.rejudge, false)
	private("PUT /my/problems/{id}", problems.problem, true)
	private("DELETE /my/problems/{id}", problems.problem, true)
	mux.HandleFunc("POST /my/problems/{id}/test-files", auth.require(problems.testFile, true, 25*time.Second))
	mux.HandleFunc("GET /my/problems/{id}/test-files/{file}", auth.require(problems.testFile, true, 25*time.Second))
	mux.HandleFunc("POST /my/problems/{id}/test-files/{file}/complete", auth.require(problems.testFile, true, 25*time.Second))
}
