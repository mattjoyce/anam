package api_test

import (
	"net/url"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

func TestTimelineQueryOf_TypePattern(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		typ   string
		wants bool
	}{
		{"absent", "", true},
		{"exact", "health.sleep", true},
		{"prefix", "activity.exercise-session.", true},
		{"bare domain", "health.", false},
		{"no namespace", "sleep", false},
		{"double dot", "health.sleep..", false},
		{"wildcard", "health.*", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := url.Values{}
			if tc.typ != "" {
				v.Set("type", tc.typ)
			}
			q, err := api.TimelineQueryOf(v)
			if (err == nil) != tc.wants {
				t.Fatalf("TimelineQueryOf(type=%q) err = %v, want ok=%v", tc.typ, err, tc.wants)
			}
			if err == nil && q.Type != tc.typ {
				t.Errorf("Type = %q, want %q", q.Type, tc.typ)
			}
		})
	}
}
