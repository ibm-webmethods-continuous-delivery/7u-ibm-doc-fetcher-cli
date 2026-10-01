package fetcher

import (
	"testing"
)

func TestParseIBMDocsURL(t *testing.T) {
	cases := []struct {
		url                          string
		wantKey, wantSlug, wantLang string
	}{
		{
			"https://www.ibm.com/docs/en/wm-integration-ipaas?topic=references-public-apis",
			"wm-integration-ipaas", "references-public-apis", "en",
		},
		{
			"https://www.ibm.com/docs/en/integration-saas-lib/integration-saas/saas?topic=ipaas-admin",
			"integration-saas-lib/integration-saas/saas", "ipaas-admin", "en",
		},
		{
			"https://www.ibm.com/docs/en/kubecost/self-hosted/3.x",
			"kubecost/self-hosted/3.x", "", "en",
		},
		{
			"https://www.ibm.com/docs/fr/wm-integration-ipaas?topic=overview",
			"wm-integration-ipaas", "overview", "fr",
		},
		// bad URL
		{"not-a-url", "", "", ""},
		// missing product — lang segment is still parsed
		{"https://www.ibm.com/docs/en/", "", "", "en"},
	}
	for _, c := range cases {
		k, s, l := ParseIBMDocsURL(c.url)
		if k != c.wantKey || s != c.wantSlug || l != c.wantLang {
			t.Errorf("ParseIBMDocsURL(%q) = (%q,%q,%q), want (%q,%q,%q)",
				c.url, k, s, l, c.wantKey, c.wantSlug, c.wantLang)
		}
	}
}

func TestSlugMatches(t *testing.T) {
	cases := []struct {
		slug, href, label string
		want              bool
	}{
		// Strategy 1: label-derived slug
		{
			"administering-environments-capabilities",
			"SSC74RW/some_file.html",
			"Administering environments and capabilities",
			true,
		},
		// Strategy 2: exact filename
		{
			"wmint-public-apis",
			"SSGOVO/wmint_public_apis.html",
			"",
			true,
		},
		// Strategy 3: progressive suffix
		{
			"references-public-apis",
			"SSGOVO/wmint_public_apis.html",
			"",
			true,
		},
		// Empty href — must not match (no label match either)
		{
			"something",
			"",
			"",
			false,
		},
		// dummy_landing_page_id should not match normal slugs
		{
			"welcome",
			"dummy_landing_page_id.html",
			"Welcome",
			true, // label match is expected here
		},
		// No match
		{
			"completely-unrelated",
			"SSGOVO/wmint_public_apis.html",
			"Public APIs reference",
			false,
		},
		// Short suffix below 6-char threshold — must not match
		{
			"ref-apis",
			"SSGOVO/something_apis.html",
			"",
			false, // "apis" is 4 chars < 6, "ref-apis" != "something-apis"
		},
	}
	for _, c := range cases {
		got := SlugMatches(c.slug, c.href, c.label)
		if got != c.want {
			t.Errorf("SlugMatches(%q, %q, %q) = %v, want %v",
				c.slug, c.href, c.label, got, c.want)
		}
	}
}

func TestLabelToSlug(t *testing.T) {
	cases := []struct {
		label, want string
	}{
		{"Administering environments and capabilities", "administering-environments-capabilities"},
		{"Public APIs", "public-apis"},
		{"Overview", "overview"},
		{"Getting started with IBM Docs", "getting-started-ibm-docs"},
	}
	for _, c := range cases {
		got := labelToSlug(c.label)
		if got != c.want {
			t.Errorf("labelToSlug(%q) = %q, want %q", c.label, got, c.want)
		}
	}
}
