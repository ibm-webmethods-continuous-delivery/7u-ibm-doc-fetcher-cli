package kb

import (
	"strings"
	"testing"
)

func TestExtractFrontmatter_APITopic(t *testing.T) {
	md := `# Public APIs

Last Updated: 2024-11-01

Use GET /apis/v1/rest/workflows to list workflows.
Authenticate using x-instance-api-key header.
Only admin users can call POST /apis/v1/rest/environments.
`
	fm := ExtractFrontmatter(md, "wm-integration-ipaas", "wmint_public_apis", "2024-11-01")

	if fm["product"] != "wm-integration-ipaas" {
		t.Errorf("product = %v", fm["product"])
	}
	if fm["topic"] != "wmint_public_apis" {
		t.Errorf("topic = %v", fm["topic"])
	}
	if fm["last_updated"] != "2024-11-01" {
		t.Errorf("last_updated = %v", fm["last_updated"])
	}
	methods, ok := fm["http_methods"].([]string)
	if !ok || len(methods) == 0 {
		t.Errorf("http_methods missing or empty: %v", fm["http_methods"])
	}
	if _, ok := fm["auth"]; !ok {
		t.Error("auth field missing")
	}
	if fm["requires_admin"] != true {
		t.Error("requires_admin should be true")
	}
}

func TestExtractFrontmatter_DocTopic(t *testing.T) {
	md := `# Architecture overview

This page describes the system architecture of Kubecost.
`
	fm := ExtractFrontmatter(md, "kubecost/self-hosted/3.x", "architecture", "")

	if _, ok := fm["http_methods"]; ok {
		t.Error("http_methods should not be present for non-API topic")
	}
	if _, ok := fm["auth"]; ok {
		t.Error("auth should not be present")
	}
}

func TestRenderFrontmatter(t *testing.T) {
	fm := map[string]any{
		"product":      "wm-integration-ipaas",
		"topic":        "wmint_public_apis",
		"last_updated": "2024-11-01",
		"http_methods": []string{"GET", "POST"},
		"stub":         false,
	}
	out := RenderFrontmatter(fm)
	if !strings.HasPrefix(out, "---\n") {
		t.Error("frontmatter must start with ---")
	}
	if !strings.Contains(out, "product: \"wm-integration-ipaas\"") {
		t.Errorf("product field missing: %s", out)
	}
	if !strings.Contains(out, "http_methods:") {
		t.Errorf("http_methods field missing: %s", out)
	}
	// stub: false should NOT be emitted (zero bool)
	if strings.Contains(out, "stub: false") {
		t.Error("stub: false should be omitted")
	}
}

func TestEffectiveLen(t *testing.T) {
	md := "Last Updated: 2024-01-01\n\nShort."
	got := EffectiveLen(md)
	if got != len("Short.") {
		t.Errorf("EffectiveLen = %d, want %d", got, len("Short."))
	}
}

func TestBrowserURL(t *testing.T) {
	tests := []struct {
		name       string
		productKey string
		topicID    string
		lang       string
		want       string
	}{
		{
			name:       "standard English",
			productKey: "wm-integration-ipaas",
			topicID:    "wmint_public_apis",
			lang:       "en",
			want:       "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=wmint-public-apis",
		},
		{
			name:       "French language",
			productKey: "wm-integration-ipaas",
			topicID:    "wmint_public_apis",
			lang:       "fr",
			want:       "https://www.ibm.com/docs/fr/wm-integration-ipaas?topic=wmint-public-apis",
		},
		{
			name:       "multi-segment product key",
			productKey: "integration-saas-lib/integration-saas/saas",
			topicID:    "admin_console",
			lang:       "en",
			want:       "https://www.ibm.com/docs/en/integration-saas-lib/integration-saas/saas?topic=admin-console",
		},
		{
			name:       "topic with multiple underscores",
			productKey: "kubecost/self-hosted/3.x",
			topicID:    "cost_allocation_reporting",
			lang:       "ja",
			want:       "https://www.ibm.com/docs/ja/kubecost/self-hosted/3.x?topic=cost-allocation-reporting",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BrowserURL(tt.productKey, tt.topicID, tt.lang)
			if got != tt.want {
				t.Errorf("BrowserURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBrowserURLFromFrontmatter(t *testing.T) {
	fm := map[string]any{
		"product": "wm-integration-ipaas",
		"topic":   "wmint_public_apis",
	}
	got := BrowserURLFromFrontmatter(fm, "en")
	want := "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=wmint-public-apis"
	if got != want {
		t.Errorf("BrowserURLFromFrontmatter() = %q, want %q", got, want)
	}
	
	// Test with missing frontmatter fields
	emptyFM := map[string]any{}
	got = BrowserURLFromFrontmatter(emptyFM, "en")
	if got != "" {
		t.Errorf("BrowserURLFromFrontmatter() with empty fm = %q, want empty string", got)
	}
}
