package main

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"judge/api/internal/submissions"
)

// The pool keeps one primary judge host running and starts burst hosts for
// scheduled contests (ADR 0012). Burst hosts stop themselves after their hold
// expires, so the bridge only starts hosts and extends holds.
const (
	holdTag            = "JudgeHoldUntil"
	installedDigestTag = "JudgeInstalledDigest"
	fallbackHold       = 30 * time.Minute
	backlogAge         = 2 * time.Minute
	startTimeout       = 5 * time.Minute
	// Anyone can create contests, so admitted windows are capped per 24 hours.
	windowBudget = 8 * time.Hour
)

var errRetry = errors.New("instance state is changing")

type host struct {
	ID, Name, Role, State, Digest string
	Hold, Launched                time.Time
}

type fleet interface {
	Hosts(ctx context.Context) ([]host, error)
	Hold(ctx context.Context, ids []string, until time.Time) error
	Start(ctx context.Context, id string) error
}

type capacity struct {
	fleet           fleet
	minParticipants int
}

type window struct{ Opens, Closes time.Time }

type demand struct {
	Now     time.Time
	Contest time.Time // latest end of an admitted active contest window; zero when none
	Capped  bool
	Queued  int64
	Waiting time.Duration // oldest request already sent to SQS and not yet received
}

type plan struct {
	Holds               map[string]time.Time
	Starts, Stale, Slow []string
}

func (c *capacity) run(ctx context.Context, db *pgxpool.Pool, digest string) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := c.apply(ctx, db, digest); err != nil {
		observe("failure", "platform", "capacity_failed", "", "")
	}
}

func (c *capacity) apply(ctx context.Context, db *pgxpool.Pool, digest string) error {
	d, err := loadDemand(ctx, db, c.minParticipants)
	if err != nil {
		return err
	}
	hosts, err := c.fleet.Hosts(ctx)
	if err != nil {
		return err
	}
	if d.Capped {
		observe("capacity_capped", "", "", "", "")
	}
	return c.execute(ctx, planCapacity(d, hosts, digest))
}

func (c *capacity) execute(ctx context.Context, p plan) error {
	// Tag before starting, so a booting worker already sees its hold.
	groups := map[time.Time][]string{}
	for id, until := range p.Holds {
		groups[until] = append(groups[until], id)
	}
	holds := make([]time.Time, 0, len(groups))
	for until := range groups {
		holds = append(holds, until)
	}
	sort.Slice(holds, func(i, j int) bool { return holds[i].Before(holds[j]) })
	for _, until := range holds {
		sort.Strings(groups[until])
		if err := c.fleet.Hold(ctx, groups[until], until); err != nil {
			return err
		}
	}
	for _, id := range p.Starts {
		err := c.fleet.Start(ctx, id)
		if errors.Is(err, errRetry) {
			continue
		}
		if err != nil {
			return err
		}
		observe("capacity_start", "", "", "", "", "instanceId", id)
	}
	// A stale host would judge every request as JE; only a rollout may bring it back.
	for _, id := range p.Stale {
		observe("failure", "platform", "capacity_stale_instance", "", "", "instanceId", id)
	}
	for _, id := range p.Slow {
		observe("failure", "platform", "capacity_start_timeout", "", "", "instanceId", id)
	}
	return nil
}

func planCapacity(d demand, hosts []host, digest string) plan {
	p := plan{Holds: map[string]time.Time{}}
	active := func(h host) bool { return h.State == "pending" || h.State == "running" }
	rank := func(h host) int {
		switch {
		case active(h):
			return 0
		case h.State == "stopped" && h.Digest == digest:
			return 1
		default:
			return 2
		}
	}
	var bursts []host
	primaryUp := false
	for _, h := range hosts {
		switch h.Role {
		case "primary":
			primaryUp = primaryUp || active(h)
			ensure(&p, h, d.Now, digest)
		case "burst":
			bursts = append(bursts, h)
		}
	}
	// Extra demand extends running hosts before starting stopped ones.
	sort.SliceStable(bursts, func(i, j int) bool {
		if rank(bursts[i]) != rank(bursts[j]) {
			return rank(bursts[i]) < rank(bursts[j])
		}
		return bursts[i].Name < bursts[j].Name
	})
	running := 0
	for _, h := range bursts {
		if active(h) {
			running++
		}
	}
	fallback := 0
	if !primaryUp && d.Queued > 0 {
		fallback = 1
	}
	if d.Waiting >= backlogAge {
		fallback = max(fallback, running+1)
	}
	want := map[string]time.Time{}
	if d.Contest.After(d.Now) {
		for _, h := range bursts {
			want[h.ID] = d.Contest
		}
	}
	for _, h := range bursts[:min(fallback, len(bursts))] {
		want[h.ID] = maxTime(want[h.ID], d.Now.Add(fallbackHold))
	}
	for _, h := range bursts {
		until, ok := want[h.ID]
		if !ok || !ensure(&p, h, d.Now, digest) {
			continue
		}
		if until.After(h.Hold) {
			p.Holds[h.ID] = until
		}
	}
	return p
}

// ensure records how to bring a wanted host up and reports whether it will run.
func ensure(p *plan, h host, now time.Time, digest string) bool {
	switch h.State {
	case "running":
		return true
	case "pending":
		if now.Sub(h.Launched) > startTimeout {
			p.Slow = append(p.Slow, h.ID)
		}
		return true
	case "stopped":
		if h.Digest != digest {
			p.Stale = append(p.Stale, h.ID)
			return false
		}
		p.Starts = append(p.Starts, h.ID)
		return true
	default:
		// Stopping hosts are started on a later tick.
		return false
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func loadDemand(ctx context.Context, db *pgxpool.Pool, minParticipants int) (demand, error) {
	var d demand
	var waiting float64
	err := db.QueryRow(ctx, `SELECT statement_timestamp(), COUNT(*),
  COALESCE(EXTRACT(EPOCH FROM statement_timestamp()-MIN(dispatched_at)),0)::float8
  FROM submissions WHERE runtime=ANY($1::text[]) AND status='QUEUED' AND dispatched_at IS NOT NULL`,
		submissions.IsolateRuntimeIDs()).Scan(&d.Now, &d.Queued, &waiting)
	if err != nil {
		return d, err
	}
	d.Waiting = time.Duration(waiting * float64(time.Second))
	// Windows open 30 minutes early and cover at most the first 3 hours, when submissions peak.
	rows, err := db.Query(ctx, `SELECT c.starts_at-interval '30 minutes', LEAST(c.ends_at, c.starts_at+interval '3 hours')+interval '15 minutes'
  FROM contests c
  WHERE c.starts_at <= $1::timestamptz+interval '30 minutes' AND c.starts_at > $1::timestamptz-interval '48 hours'
    AND (SELECT COUNT(*) FROM contest_participants p WHERE p.contest_id=c.id) >= $2
  ORDER BY 1, 2`, d.Now, minParticipants)
	if err != nil {
		return d, err
	}
	windows, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (window, error) {
		var w window
		return w, row.Scan(&w.Opens, &w.Closes)
	})
	if err != nil {
		return d, err
	}
	d.Contest, d.Capped = admitWindows(windows, d.Now)
	return d, nil
}

// admitWindows returns the latest end of the admitted windows open at now. Windows are
// admitted in opening order while the admitted total within 24 hours fits windowBudget.
func admitWindows(windows []window, now time.Time) (until time.Time, capped bool) {
	var admitted []window
	for _, w := range windows {
		used := w.Closes.Sub(w.Opens)
		for _, a := range admitted {
			if a.Closes.After(w.Opens.Add(-24 * time.Hour)) {
				used += a.Closes.Sub(a.Opens)
			}
		}
		open := !w.Opens.After(now) && w.Closes.After(now)
		if used > windowBudget {
			capped = capped || open
			continue
		}
		admitted = append(admitted, w)
		if open {
			until = maxTime(until, w.Closes)
		}
	}
	return until, capped
}

type ec2Fleet struct {
	client *ec2.Client
	pool   string
}

func (f ec2Fleet) Hosts(ctx context.Context) ([]host, error) {
	pages := ec2.NewDescribeInstancesPaginator(f.client, &ec2.DescribeInstancesInput{Filters: []ec2types.Filter{
		{Name: aws.String("tag:JudgePool"), Values: []string{f.pool}},
		{Name: aws.String("instance-state-name"), Values: []string{"pending", "running", "stopping", "stopped"}},
	}})
	var hosts []host
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, reservation := range page.Reservations {
			for _, instance := range reservation.Instances {
				h := host{ID: aws.ToString(instance.InstanceId), Launched: aws.ToTime(instance.LaunchTime)}
				if instance.State != nil {
					h.State = string(instance.State.Name)
				}
				for _, tag := range instance.Tags {
					value := aws.ToString(tag.Value)
					switch aws.ToString(tag.Key) {
					case "Name":
						h.Name = value
					case "JudgeRole":
						h.Role = value
					case installedDigestTag:
						h.Digest = value
					case holdTag:
						// An unreadable hold is treated as expired and rewritten.
						h.Hold, _ = time.Parse(time.RFC3339, value)
					}
				}
				hosts = append(hosts, h)
			}
		}
	}
	return hosts, nil
}

func (f ec2Fleet) Hold(ctx context.Context, ids []string, until time.Time) error {
	_, err := f.client.CreateTags(ctx, &ec2.CreateTagsInput{Resources: ids, Tags: []ec2types.Tag{
		{Key: aws.String(holdTag), Value: aws.String(until.UTC().Format(time.RFC3339))},
	}})
	return err
}

func (f ec2Fleet) Start(ctx context.Context, id string) error {
	_, err := f.client.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{id}})
	var api smithy.APIError
	if errors.As(err, &api) && api.ErrorCode() == "IncorrectInstanceState" {
		return errRetry
	}
	return err
}
