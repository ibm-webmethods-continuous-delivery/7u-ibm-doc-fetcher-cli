package cmd

import (
	"testing"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/ibmdocs"
)

func hitWith(productKey, title, breadcrumb, url string) ibmdocs.SearchHit {
	h := ibmdocs.SearchHit{
		Title:             title,
		FullURL:           url,
		ProductBreadCrumb: breadcrumb,
	}
	h.Product.Key = productKey
	return h
}

func TestDeduplicateLatest_KeepsHighestVersionPerTitle(t *testing.T) {
	hits := []ibmdocs.SearchHit{
		hitWith("wm-integration-ipaas", "Public APIs", "v1.0", "https://example.com/v1"),
		hitWith("wm-integration-ipaas", "Public APIs", "v2.0", "https://example.com/v2"),
	}

	got := deduplicateLatest(hits)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].FullURL != "https://example.com/v2" {
		t.Errorf("FullURL = %q, want the higher-breadcrumb version (v2)", got[0].FullURL)
	}
}

func TestDeduplicateLatest_DistinctTitlesBothKept(t *testing.T) {
	hits := []ibmdocs.SearchHit{
		hitWith("kubecost", "Architecture overview", "v1.0", "https://example.com/arch"),
		hitWith("kubecost", "Installation guide", "v1.0", "https://example.com/install"),
	}

	got := deduplicateLatest(hits)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (distinct titles must not be merged)", len(got))
	}
}

func TestDeduplicateLatest_DistinctProductsBothKept(t *testing.T) {
	hits := []ibmdocs.SearchHit{
		hitWith("product-a", "Overview", "v1.0", "https://example.com/a"),
		hitWith("product-b", "Overview", "v1.0", "https://example.com/b"),
	}

	got := deduplicateLatest(hits)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (same title under different products must not be merged)", len(got))
	}
}
