package cmd

import "testing"

func TestTruncateStr(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 10, "short"},
		{"exactlyten", 10, "exactlyten"},
		{"this is way too long", 10, "this is..."},
	}
	for _, c := range cases {
		got := truncateStr(c.in, c.n)
		if got != c.want {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
		if len(got) > c.n {
			t.Errorf("truncateStr(%q, %d) = %q, exceeds max length %d", c.in, c.n, got, c.n)
		}
	}
}
