package resolve

import (
	"errors"
	"testing"
)

var channels = []Candidate{
	{ID: "2001", Name: "🔮general"},
	{ID: "2002", Name: "📰news"},
	{ID: "2003", Name: "cmmc-general"},
	{ID: "2006", Name: "help-forum"},
}

func TestNormalizeMatchesTheNodeRule(t *testing.T) {
	cases := map[string]string{
		"Cooey COE":   "cooey-coe",
		"🔮general":    "general",
		"📚 Book Club": "-book-club",
		"SPRS  ✨":     "sprs-",
		"A_b":         "ab",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchSteps(t *testing.T) {
	cases := map[string]string{
		"2002":       "2002",
		"📰news":      "2002",
		"📰NEWS":      "2002",
		"news":       "2002",
		"News":       "2002",
		"general":    "2001",
		"Help Forum": "2006",
	}
	for in, want := range cases {
		got, err := Match("channel", in, channels)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got.ID != want {
			t.Errorf("%q -> %s, want %s", in, got.ID, want)
		}
	}
}

func TestUnknownIDPassesThrough(t *testing.T) {
	got, err := Match("channel", "9999", channels)
	if err != nil || got.ID != "9999" {
		t.Errorf("%v %v", got, err)
	}
}

func TestNotFoundSuggestsBySubstring(t *testing.T) {
	_, err := Match("channel", "gen", channels)
	var nf *NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("err %v", err)
	}
	if len(nf.Suggestions) != 2 || nf.Suggestions[0] != "cmmc-general" || nf.Suggestions[1] != "🔮general" {
		t.Errorf("suggestions %v", nf.Suggestions)
	}
}

func TestExactBeatsNormalised(t *testing.T) {
	cands := []Candidate{{ID: "1", Name: "🔮general"}, {ID: "2", Name: "general"}}
	got, err := Match("channel", "general", cands)
	if err != nil || got.ID != "2" {
		t.Errorf("%v %v", got, err)
	}
}

func TestAmbiguousNormalisedListsCandidates(t *testing.T) {
	cands := []Candidate{{ID: "1", Name: "🔮general"}, {ID: "2", Name: "✨general"}}
	_, err := Match("channel", "general", cands)
	var amb *AmbiguousError
	if !errors.As(err, &amb) || len(amb.Candidates) != 2 {
		t.Errorf("err %v", err)
	}
}

func TestPrimaryNameBeatsAliasForGroupDMs(t *testing.T) {
	cands := []Candidate{
		{ID: "d1", Name: "kyle", AlsoNames: []string{"Kyle B"}},
		{ID: "d2", Name: "Study Group", Aliases: []string{"kyle", "Kyle B", "ana"}},
		{ID: "d3", Name: "ana, maria", Aliases: []string{"ana", "maria"}},
	}
	got, err := Match("DM", "kyle", cands)
	if err != nil || got.ID != "d1" {
		t.Errorf("kyle should be the DM, got %v %v", got, err)
	}
	got, err = Match("DM", "kyle b", cands)
	if err != nil || got.ID != "d1" {
		t.Errorf("display name should be the DM, got %v %v", got, err)
	}
	got, err = Match("DM", "maria", cands)
	if err != nil || got.ID != "d3" {
		t.Errorf("maria: %v %v", got, err)
	}
	_, err = Match("DM", "ana", cands)
	var amb *AmbiguousError
	if !errors.As(err, &amb) || len(amb.Candidates) != 2 {
		t.Errorf("ana should be ambiguous between two groups, got %v", err)
	}
}
