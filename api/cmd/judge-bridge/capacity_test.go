package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/jackc/pgx/v5"

	"judge/api/internal/database"
)

const poolDigest = "sha256:current"

var poolNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func poolHosts(bursts ...host) []host {
	hosts := []host{{ID: "i-primary", Name: "judge-dev-judge-worker", Role: "primary", State: "running", Digest: poolDigest}}
	for i, h := range bursts {
		h.ID = fmt.Sprintf("i-burst%d", i+2)
		h.Name = fmt.Sprintf("judge-dev-judge-worker-%d", i+2)
		h.Role = "burst"
		if h.Digest == "" {
			h.Digest = poolDigest
		}
		hosts = append(hosts, h)
	}
	return hosts
}

func TestPlanCapacity(t *testing.T) {
	stopped, running := host{State: "stopped"}, host{State: "running"}
	contest := poolNow.Add(3 * time.Hour)
	fallback := poolNow.Add(fallbackHold)
	for _, tc := range []struct {
		name  string
		d     demand
		hosts []host
		want  plan
	}{
		{"idle pool stays stopped", demand{}, poolHosts(stopped, stopped), plan{}},
		{
			"contest starts every burst host",
			demand{Contest: contest},
			poolHosts(stopped, stopped),
			plan{Holds: map[string]time.Time{"i-burst2": contest, "i-burst3": contest}, Starts: []string{"i-burst2", "i-burst3"}},
		},
		{
			"holds are never shortened",
			demand{Contest: contest},
			poolHosts(host{State: "running", Hold: contest.Add(time.Hour)}, host{State: "running", Hold: poolNow.Add(time.Hour)}),
			plan{Holds: map[string]time.Time{"i-burst3": contest}},
		},
		{"expired contest adds nothing", demand{Contest: poolNow}, poolHosts(stopped, stopped), plan{}},
		{
			"stopped primary with queued work starts a burst host",
			demand{Queued: 1},
			append([]host{{ID: "i-primary", Role: "primary", State: "stopped", Digest: poolDigest}}, poolHosts(stopped, stopped)[1:]...),
			plan{Holds: map[string]time.Time{"i-burst2": fallback}, Starts: []string{"i-primary", "i-burst2"}},
		},
		{
			"stopped primary without work only restarts itself",
			demand{},
			append([]host{{ID: "i-primary", Role: "primary", State: "stopped", Digest: poolDigest}}, poolHosts(stopped, stopped)[1:]...),
			plan{Starts: []string{"i-primary"}},
		},
		{
			"backlog adds one more host",
			demand{Waiting: backlogAge},
			poolHosts(stopped, running),
			plan{Holds: map[string]time.Time{"i-burst3": fallback, "i-burst2": fallback}, Starts: []string{"i-burst2"}},
		},
		{"short wait adds nothing", demand{Waiting: backlogAge - time.Second, Queued: 3}, poolHosts(stopped, stopped), plan{}},
		{
			"stale host is skipped for a current one",
			demand{Contest: contest},
			poolHosts(host{State: "stopped", Digest: "sha256:old"}, stopped),
			plan{Holds: map[string]time.Time{"i-burst3": contest}, Starts: []string{"i-burst3"}, Stale: []string{"i-burst2"}},
		},
		{
			"stopping host waits for the next tick",
			demand{Contest: contest},
			poolHosts(host{State: "stopping"}, stopped),
			plan{Holds: map[string]time.Time{"i-burst3": contest}, Starts: []string{"i-burst3"}},
		},
		{
			"slow start is reported",
			demand{Contest: contest},
			poolHosts(host{State: "pending", Launched: poolNow.Add(-startTimeout - time.Second)}, host{State: "pending", Launched: poolNow}),
			plan{Holds: map[string]time.Time{"i-burst2": contest, "i-burst3": contest}, Slow: []string{"i-burst2"}},
		},
		{"unrelated hosts are ignored", demand{Contest: contest}, append(poolHosts(), host{ID: "i-other", State: "stopped"}), plan{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.d.Now = poolNow
			got := planCapacity(tc.d, tc.hosts, poolDigest)
			if tc.want.Holds == nil {
				tc.want.Holds = map[string]time.Time{}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAdmitWindows(t *testing.T) {
	at := func(hours float64) time.Time { return poolNow.Add(time.Duration(hours * float64(time.Hour))) }
	contest := func(start float64) window { return window{Opens: at(start - 0.5), Closes: at(start + 3.25)} }
	until, capped := admitWindows([]window{contest(-10), contest(-5), contest(0)}, poolNow)
	if !until.IsZero() || !capped {
		t.Fatal("a third contest window within 24 hours must be capped")
	}
	until, capped = admitWindows([]window{contest(-25), contest(-5), contest(0)}, poolNow)
	if !until.IsZero() || !capped {
		t.Fatal("the budget counts every admitted window closing within 24 hours")
	}
	until, capped = admitWindows([]window{contest(-30), contest(0), {Opens: at(-0.1), Closes: at(1)}}, poolNow)
	if !until.Equal(at(3.25)) || capped {
		t.Fatalf("an old window must not consume today's budget: %v %v", until, capped)
	}
}

type fakeFleet struct {
	hosts []host
	calls []string
	start map[string]error
}

func (f *fakeFleet) Hosts(context.Context) ([]host, error) {
	f.calls = append(f.calls, "hosts")
	return f.hosts, nil
}

func (f *fakeFleet) Hold(_ context.Context, ids []string, until time.Time) error {
	f.calls = append(f.calls, "hold "+strings.Join(ids, ",")+" "+until.Format(time.RFC3339))
	return nil
}

func (f *fakeFleet) Start(_ context.Context, id string) error {
	f.calls = append(f.calls, "start "+id)
	return f.start[id]
}

func captureLogs(t *testing.T) *bytes.Buffer {
	var out bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&out, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &out
}

func logEvents(t *testing.T, out *bytes.Buffer) []map[string]any {
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestExecuteTagsBeforeStartingAndRetriesStateChanges(t *testing.T) {
	out := captureLogs(t)
	fleet := &fakeFleet{start: map[string]error{"i-b": errRetry}}
	c := capacity{fleet: fleet}
	until := poolNow.Add(time.Hour)
	p := plan{Holds: map[string]time.Time{"i-b": until, "i-a": until}, Starts: []string{"i-a", "i-b"}, Stale: []string{"i-c"}, Slow: []string{"i-d"}}
	if err := c.execute(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	want := []string{"hold i-a,i-b " + until.Format(time.RFC3339), "start i-a", "start i-b"}
	if !reflect.DeepEqual(fleet.calls, want) {
		t.Fatalf("calls %v, want %v", fleet.calls, want)
	}
	var reasons []any
	for _, event := range logEvents(t, out) {
		reasons = append(reasons, event["event"], event["reason"])
	}
	wantEvents := []any{"capacity_start", nil, "failure", "capacity_stale_instance", "failure", "capacity_start_timeout"}
	if !reflect.DeepEqual(reasons, wantEvents) {
		t.Fatalf("events %v, want %v", reasons, wantEvents)
	}
	fleet.start["i-a"] = errors.New("InsufficientInstanceCapacity")
	if err := c.execute(context.Background(), plan{Starts: []string{"i-a"}}); err == nil {
		t.Fatal("a failed start must be reported")
	}
}

func TestEC2FleetReadsTagsAndMapsStateErrors(t *testing.T) {
	var forms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(data))
		forms = append(forms, form)
		w.Header().Set("Content-Type", "text/xml")
		switch form.Get("Action") {
		case "DescribeInstances":
			_, _ = fmt.Fprint(w, `<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><requestId>r</requestId><reservationSet><item><reservationId>r-1</reservationId><instancesSet><item>
<instanceId>i-0123456789abcdef0</instanceId><instanceState><code>80</code><name>stopped</name></instanceState><launchTime>2026-10-01T11:00:00.000Z</launchTime>
<tagSet><item><key>Name</key><value>judge-dev-judge-worker-2</value></item><item><key>JudgeRole</key><value>burst</value></item>
<item><key>JudgeInstalledDigest</key><value>sha256:current</value></item><item><key>JudgeHoldUntil</key><value>2026-10-01T15:00:00Z</value></item></tagSet>
</item></instancesSet></item></reservationSet></DescribeInstancesResponse>`)
		case "CreateTags":
			_, _ = fmt.Fprint(w, `<CreateTagsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><requestId>r</requestId><return>true</return></CreateTagsResponse>`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `<Response><Errors><Error><Code>IncorrectInstanceState</Code><Message>stopping</Message></Error></Errors><RequestID>r</RequestID></Response>`)
		}
	}))
	defer server.Close()
	cfg := aws.Config{Region: "ap-northeast-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}
	f := ec2Fleet{pool: "judge-dev", client: ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.RetryMaxAttempts = 1
	})}
	ctx := context.Background()
	hosts, err := f.Hosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []host{{
		ID: "i-0123456789abcdef0", Name: "judge-dev-judge-worker-2", Role: "burst", State: "stopped", Digest: "sha256:current",
		Hold: time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC), Launched: time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC),
	}}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("hosts %+v", hosts)
	}
	if forms[0].Get("Filter.1.Name") != "tag:JudgePool" || forms[0].Get("Filter.1.Value.1") != "judge-dev" {
		t.Fatalf("pool filter missing: %v", forms[0])
	}
	if err := f.Hold(ctx, []string{"i-0123456789abcdef0"}, time.Date(2026, 10, 1, 21, 0, 0, 0, time.FixedZone("JST", 9*3600))); err != nil {
		t.Fatal(err)
	}
	if forms[1].Get("Tag.1.Key") != holdTag || forms[1].Get("Tag.1.Value") != "2026-10-01T12:00:00Z" {
		t.Fatalf("hold tag %v", forms[1])
	}
	if err := f.Start(ctx, "i-0123456789abcdef0"); !errors.Is(err, errRetry) {
		t.Fatalf("state change must be retried later: %v", err)
	}
}

func TestCapacityRunsOnlyOnScheduleAndReadsDemand(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	schema := fmt.Sprintf("test_capacity_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO user_profiles(owner_id,handle) SELECT 'user'||n,'user'||n FROM generate_series(0,10) n`)
	contest := func(id string, start, end string, participants int) {
		exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at) VALUES ($1,'user0','c','',clock_timestamp()+$2::interval,clock_timestamp()+$3::interval)`, id, start, end)
		exec(`INSERT INTO contest_participants(contest_id,owner_id) SELECT $1,'user'||n FROM generate_series(1,$2::int) n`, id, participants)
	}
	contest("aaaaaaaa-0000-4000-8000-000000000001", "10 minutes", "2 hours", 10)
	contest("aaaaaaaa-0000-4000-8000-000000000002", "10 minutes", "5 hours", 9)
	contest("aaaaaaaa-0000-4000-8000-000000000003", "50 minutes", "5 hours", 10)
	submission := func(id, status, dispatched string) {
		exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,judge_attempt,status,dispatched_at)
 VALUES ($1,'user1',$1,1,'test','cpp17-isolate','source','{}',gen_random_uuid(),$2,clock_timestamp()-$3::interval)`, id, status, dispatched)
	}
	submission("bbbbbbbb-0000-4000-8000-000000000001", "QUEUED", "3 minutes")
	submission("bbbbbbbb-0000-4000-8000-000000000002", "RUNNING", "1 hour")
	exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,judge_attempt)
 VALUES ('bbbbbbbb-0000-4000-8000-000000000003','user1','bbbbbbbb-0000-4000-8000-000000000003',1,'test','cpp17-isolate','source','{}',gen_random_uuid())`)
	d, err := loadDemand(ctx, db, 10)
	if err != nil {
		t.Fatal(err)
	}
	if d.Queued != 1 || d.Waiting < 179*time.Second || d.Waiting > 10*time.Minute || d.Capped {
		t.Fatalf("queue demand %+v", d)
	}
	if got := d.Contest.Sub(d.Now); got < 2*time.Hour+14*time.Minute || got > 2*time.Hour+15*time.Minute {
		t.Fatalf("contest hold ends %v after now", got)
	}
	// Leave nothing for dispatch, which would otherwise need S3 and SQS.
	exec(`UPDATE submissions SET status='DONE' WHERE dispatched_at IS NULL`)
	fleet := &fakeFleet{hosts: poolHosts(host{State: "stopped"})}
	b := bridge{db: db, runtime: poolDigest, capacity: &capacity{fleet: fleet, minParticipants: 10}}
	for _, raw := range []string{`{}`, `{"source":"aws.events","detail-type":"Other"}`} {
		if _, err := b.handle(ctx, json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if len(fleet.calls) != 0 {
		t.Fatalf("a wake-up called EC2: %v", fleet.calls)
	}
	if _, err := b.handle(ctx, json.RawMessage(`{"source":"aws.events","detail-type":"Scheduled Event"}`)); err != nil {
		t.Fatal(err)
	}
	if len(fleet.calls) != 3 || fleet.calls[0] != "hosts" || !strings.HasPrefix(fleet.calls[1], "hold i-burst2 ") || fleet.calls[2] != "start i-burst2" {
		t.Fatalf("schedule calls %v", fleet.calls)
	}
}
