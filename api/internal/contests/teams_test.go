package contests

import (
	"strings"
	"testing"
	"time"
)

func TestTeamNames(t *testing.T) {
	for in, want := range map[string]string{
		"  Ｔｅａｍ　 Alpha ": "Team Alpha",
		"ﾁｰﾑ":            "チーム",
		"a  b":           "a b",
	} {
		if got := NormalizeTeamName(in); got != want || !ValidTeamName(in) {
			t.Errorf("NormalizeTeamName(%q) = %q (valid %t), want %q", in, got, ValidTeamName(in), want)
		}
	}
	if teamKey("TEAM alpha") != teamKey("ｔｅａｍ　ＡＬＰＨＡ") {
		t.Error("names that differ by case or width must share a key")
	}
	if !ValidTeamName(strings.Repeat("あ", 40)) {
		t.Error("40 characters must be valid")
	}
	for _, bad := range []string{"", "   ", strings.Repeat("あ", 41), "a\u200bb", "a\u202eb", "a\nb", "a\tb", "a\u2028b", "a\ufeffb", "a\ue000b", "a\x00b",
		// Default-ignorable characters outside the format category would copy a name under another key.
		"Alpha\ufe0f", "Alpha\ufe00", "Al\u034fpha", "Alpha\u180b", "Alpha\U000e0100", "Alpha\u3164", "Alpha\u17b4",
		// Blank characters and marks alone show no name.
		"\u3164", "\uffa0", "\u115f", "\u1160", "\u2800", "\u17b4", "\u0301", "\u0301\u0302"} {
		if ValidTeamName(bad) {
			t.Errorf("ValidTeamName(%q) = true", bad)
		}
	}
	for _, good := range []string{"!!", "★", "é", "e\u0301", "가", "チーム1"} {
		if !ValidTeamName(good) {
			t.Errorf("ValidTeamName(%q) = false", good)
		}
	}
}

func TestVenuePassphrases(t *testing.T) {
	if got := NormalizePassphrase("　Ｖｅｎｕｅ-2026 "); got != "Venue-2026" {
		t.Errorf("NormalizePassphrase = %q", got)
	}
	for in, want := range map[string]bool{
		"abcd": true, " ａｂｃ ": false, "abc": false, strings.Repeat("x", 64): true, strings.Repeat("x", 65): false,
		"two words": true, "a\u200bbcd": false, "ab\ncd": false, "": false, "abcd\ufe0f": false, "\u2800\u2800\u2800\u2800": false,
	} {
		if got := ValidPassphrase(in); got != want {
			t.Errorf("ValidPassphrase(%q) = %t, want %t", in, got, want)
		}
	}
	if samePassphrase("", "") || !samePassphrase("Venue", "Venue") || samePassphrase("venue", "Venue") {
		t.Error("passphrases must match exactly, and an empty stored one never")
	}
}

func TestJoinCodes(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		code := newJoinCode()
		if len(code) != codeLength || strings.Trim(code, codeAlphabet) != "" || seen[code] {
			t.Fatalf("join code %q", code)
		}
		seen[code] = true
	}
	if strings.ContainsAny(codeAlphabet, "0O1IL") {
		t.Fatal("ambiguous characters in the alphabet")
	}
	if got := NormalizeJoinCode(" abcde-ＦＧＨＪＫ "); got != "ABCDEFGHJK" {
		t.Errorf("NormalizeJoinCode = %q", got)
	}
	for _, in := range []JoinInput{{Team: &TeamAction{Action: "join", Code: " - "}}, {Team: &TeamAction{Action: "leave"}}, {Team: &TeamAction{Action: "join", Code: strings.Repeat("A", 65)}}} {
		if ValidJoin(in) != ErrInvalidJoin {
			t.Errorf("ValidJoin(%+v) accepted", in.Team)
		}
	}
	if ValidJoin(JoinInput{Team: &TeamAction{Action: "create", Name: "a\u200bb"}}) != ErrInvalidTeamName || ValidJoin(JoinInput{}) != nil {
		t.Error("ValidJoin")
	}
}

func TestParticipationSettings(t *testing.T) {
	penalty, one, two, fifty, big := 5, 1, 2, 50, 51
	yes, no := true, false
	start := time.Now().Add(time.Hour)
	base := func() Input {
		return Input{Title: "T", StartsAt: start, EndsAt: start.Add(time.Hour), PenaltyMinutes: &penalty, Problems: []Problem{{ID: "11111111-1111-4111-8111-111111111111", Points: 1}}}
	}
	short, valid, spaced := "abc", "abcd", "  ａｂｃｄ  "
	for name, tc := range map[string]struct {
		change func(*Input)
		want   bool
	}{
		"legacy":                   {func(*Input) {}, true},
		"size 2":                   {func(in *Input) { in.TeamMode, in.MaxTeamSize = &yes, Nullable[int]{Value: &two, Sent: true} }, true},
		"size 50":                  {func(in *Input) { in.TeamMode, in.MaxTeamSize = &yes, Nullable[int]{Value: &fifty, Sent: true} }, true},
		"size 1":                   {func(in *Input) { in.TeamMode, in.MaxTeamSize = &yes, Nullable[int]{Value: &one, Sent: true} }, false},
		"size 51":                  {func(in *Input) { in.MaxTeamSize = Nullable[int]{Value: &big, Sent: true} }, false},
		"unlimited":                {func(in *Input) { in.TeamMode, in.MaxTeamSize = &yes, Nullable[int]{Sent: true} }, true},
		"division without phrase":  {func(in *Input) { in.OnsiteDivision = &yes }, false},
		"division with short":      {func(in *Input) { in.OnsiteDivision, in.OnsitePassphrase = &yes, &short }, false},
		"division":                 {func(in *Input) { in.OnsiteDivision, in.OnsitePassphrase = &yes, &spaced }, true},
		"phrase ignored when off":  {func(in *Input) { in.OnsiteDivision, in.OnsitePassphrase = &no, &short }, true},
		"phrase alone is not read": {func(in *Input) { in.OnsitePassphrase = &valid }, true},
	} {
		in := base()
		tc.change(&in)
		if got := ValidInput(in); got != tc.want {
			t.Errorf("%s: ValidInput = %t", name, got)
		}
	}
	in := base()
	in.OnsiteDivision, in.OnsitePassphrase = &yes, &spaced
	if p := in.passphrase(); p == nil || *p != "abcd" {
		t.Errorf("stored passphrase %v", p)
	}
	in.OnsiteDivision = &no
	if in.passphrase() != nil {
		t.Error("a contest without the division stores no passphrase")
	}

	// Drafts keep anything and report it; an individual draft ignores its size, and one without the division its passphrase.
	duration := 60
	d := Draft{Title: "T", StartsAt: &start, DurationMinutes: &duration, PenaltyMinutes: &penalty, Problems: []DraftProblem{{ID: "11111111-1111-4111-8111-111111111111", Points: &penalty}}}
	d.TeamMode, d.MaxTeamSize = &no, Nullable[int]{Value: &one}
	d.OnsiteDivision, d.OnsitePassphrase = &no, &short
	if issues := d.contentIssues(time.Now()); len(issues) != 0 {
		t.Errorf("issues %v", issues)
	}
	if in := d.input(); in.MaxTeamSize.Value != nil || in.OnsitePassphrase != nil || *in.TeamMode || *in.OnsiteDivision || !ValidInput(in) {
		t.Errorf("input %+v", in)
	}
	d.TeamMode, d.OnsiteDivision = &yes, &yes
	if issues := d.contentIssues(time.Now()); strings.Join(issues, ",") != IssueTeamSize+","+IssuePassphrase {
		t.Errorf("issues %v", issues)
	}
	d.MaxTeamSize.Value, d.OnsitePassphrase = &two, &spaced
	if issues := d.contentIssues(time.Now()); len(issues) != 0 {
		t.Errorf("issues %v", issues)
	}
	if in := d.input(); *in.MaxTeamSize.Value != 2 || !in.MaxTeamSize.Sent || *in.passphrase() != "abcd" || !ValidInput(in) {
		t.Errorf("input %+v", in)
	}
}
