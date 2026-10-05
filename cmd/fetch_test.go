package cmd

import "testing"

func TestTopicIDFromHref(t *testing.T) {
	cases := []struct {
		href, want string
	}{
		{"SSGOVO/wmint_public_apis.html", "wmint_public_apis"},
		{"SSW0JQG_2.x/architecture/user-metrics.html?cp=SSW0JQG_3.0.x", "user-metrics"},
		{"SSZMH3N_1.0.325/src/pages/ecosystem/action-ansible/index.html", "index"},
		{"no-extension-or-slash", "no-extension-or-slash"},
		{"openapi/spec.yaml", "spec.yaml"}, // non-.html suffix is left intact
	}
	for _, c := range cases {
		got := topicIDFromHref(c.href)
		if got != c.want {
			t.Errorf("topicIDFromHref(%q) = %q, want %q", c.href, got, c.want)
		}
	}
}
