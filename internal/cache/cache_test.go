package cache

import (
	"path/filepath"
	"testing"
)

func TestKeyTOC(t *testing.T) {
	cases := []struct {
		productKey, lang, want string
	}{
		{"wm-integration-ipaas", "en", "data/cache/toc/wm-integration-ipaas/en.json"},
		{"kubecost/self-hosted/3.x", "fr", "data/cache/toc/kubecost/self-hosted/3.x/fr.json"},
		{"integration-saas-lib/integration-saas/saas", "en", "data/cache/toc/integration-saas-lib/integration-saas/saas/en.json"},
	}
	for _, c := range cases {
		got := KeyTOC("data", c.productKey, c.lang)
		// normalise to forward slashes for comparison
		got = filepath.ToSlash(got)
		if got != c.want {
			t.Errorf("KeyTOC(%q,%q) = %q, want %q", c.productKey, c.lang, got, c.want)
		}
	}
}

func TestKeyContent(t *testing.T) {
	cases := []struct {
		productKey, href, lang, want string
	}{
		{
			// flat href — single path segment after product ID
			"wm-integration-ipaas",
			"SSGOVO/wmint_public_apis.html",
			"en",
			"data/cache/content/wm-integration-ipaas/wmint_public_apis/en.json",
		},
		{
			// deep path — index.html under nested dir; uniqueness preserved
			"instana-observability/standard/1.0.325",
			"SSZMH3N_1.0.325/src/pages/ecosystem/action-ansible/index.html",
			"en",
			"data/cache/content/instana-observability/standard/1.0.325/src/pages/ecosystem/action-ansible/index/en.json",
		},
		{
			// ?cp= must be stripped from cache key; path preserved
			"kubecost/self-hosted/3.x",
			"SSW0JQG_2.x/architecture/user-metrics.html?cp=SSW0JQG_3.0.x",
			"en",
			"data/cache/content/kubecost/self-hosted/3.x/architecture/user-metrics/en.json",
		},
		{
			// ?cp= with trailing params
			"kubecost/self-hosted/3.x",
			"SSW0JQG_2.x/arch/metrics.html?cp=SSW0JQG_3.0.x&foo=bar",
			"en",
			"data/cache/content/kubecost/self-hosted/3.x/arch/metrics/en.json",
		},
	}
	for _, c := range cases {
		got := KeyContent("data", c.productKey, c.href, c.lang)
		got = filepath.ToSlash(got)
		if got != c.want {
			t.Errorf("KeyContent(%q,%q,%q) = %q, want %q", c.productKey, c.href, c.lang, got, c.want)
		}
	}
}
