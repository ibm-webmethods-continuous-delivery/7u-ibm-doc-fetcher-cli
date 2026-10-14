package ibmdocs

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeSearch serves canned search pages and records the last request query.
func fakeSearch(t *testing.T, pages []string, got *string) *Client {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r.URL.RawQuery
		if calls >= len(pages) {
			t.Fatalf("unexpected extra search call %d", calls+1)
		}
		_, _ = w.Write([]byte(pages[calls]))
		calls++
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, 5*time.Second, 0, "", "", nil)
}

func TestResolveSourceURL_ExactHrefMatch(t *testing.T) {
	var q string
	c := fakeSearch(t, []string{`{"hits":2,"next":-1,"topics":[
		{"href":"SSGOVO/other.html","fullurl":"https://www.ibm.com/docs/en/x?topic=wrong"},
		{"href":"SSGOVO/co_configuring_git_account.html?cp=ABC","fullurl":"https://www.ibm.com/docs/en/wm-integration-ipaas?topic=repositories-configuring-git-accounts"}
	]}`}, &q)

	got, err := c.ResolveSourceURL("SSGOVO/co_configuring_git_account.html", "Configuring Git accounts", "en")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://www.ibm.com/docs/en/wm-integration-ipaas?topic=repositories-configuring-git-accounts"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !contains(q, "products=SSGOVO") {
		t.Errorf("search not scoped to the internal product id: %q", q)
	}
}

func TestResolveSourceURL_NoGuessingOnMiss(t *testing.T) {
	var q string
	c := fakeSearch(t, []string{`{"hits":1,"next":-1,"topics":[
		{"href":"SSGOVO/similar_title.html","fullurl":"https://www.ibm.com/docs/en/x?topic=close-but-wrong"}
	]}`}, &q)

	got, err := c.ResolveSourceURL("SSGOVO/wanted.html", "Wanted", "en")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty result on miss, got %q", got)
	}
}

func TestResolveSourceURL_Pagination(t *testing.T) {
	var q string
	c := fakeSearch(t, []string{
		`{"hits":2,"next":1,"topics":[{"href":"P/a.html","fullurl":"u-a"}]}`,
		`{"hits":2,"next":-1,"topics":[{"href":"P/b.html","fullurl":"u-b"}]}`,
	}, &q)

	got, err := c.ResolveSourceURL("P/b.html", "B", "en")
	if err != nil || got != "u-b" {
		t.Errorf("got %q, %v; want u-b", got, err)
	}
}

func TestResolveSourceURL_Unresolvable(t *testing.T) {
	c := New("http://127.0.0.1:1", time.Second, 0, "", "", nil) // must not be contacted
	for _, tc := range []struct{ href, title string }{
		{"no-slash.html", "T"},
		{"P/a.html", ""},
	} {
		got, err := c.ResolveSourceURL(tc.href, tc.title, "en")
		if err != nil || got != "" {
			t.Errorf("%+v: got %q, %v; want empty, nil", tc, got, err)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestResolveHrefBySlug_ExactSlugOnly(t *testing.T) {
	var q string
	c := fakeSearch(t, []string{`{"hits":2,"next":-1,"topics":[
		{"href":"SSGOVO/ta-working_with_projects.html","fullurl":"https://www.ibm.com/docs/en/wm-integration-ipaas?topic=managing-projects"},
		{"href":"SSGOVO/publishing_projects_in_env.html?cp=X","fullurl":"https://www.ibm.com/docs/en/wm-integration-ipaas?topic=publishes-publishing-projects"}
	]}`}, &q)

	got, err := c.ResolveHrefBySlug("publishes-publishing-projects", "en", "SSGOVO")
	if err != nil {
		t.Fatal(err)
	}
	if got != "SSGOVO/publishing_projects_in_env.html" {
		t.Errorf("got %q", got)
	}
}

func TestResolveHrefBySlug_MissReturnsEmpty(t *testing.T) {
	var q string
	c := fakeSearch(t, []string{`{"hits":1,"next":-1,"topics":[
		{"href":"SSGOVO/x.html","fullurl":"https://www.ibm.com/docs/en/p?topic=other"}
	]}`}, &q)
	got, err := c.ResolveHrefBySlug("wanted", "en", "SSGOVO")
	if err != nil || got != "" {
		t.Errorf("got %q, %v; want empty, nil", got, err)
	}
}

func TestDo_RetriesOn429ThenSucceeds(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"hits":0,"next":-1,"topics":[]}`))
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second, 0, "", "", nil)
	c.retryBase = time.Millisecond

	if _, err := c.Search("q", "en", 10, 0); err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	retried, failed := c.RateLimitStats()
	if retried != 2 || failed != 0 {
		t.Errorf("stats retried=%d failed=%d, want 2/0", retried, failed)
	}
}

func TestDo_PersistentRateLimitIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second, 0, "", "", nil)
	c.retryBase = time.Millisecond

	_, err := c.Search("q", "en", 10, 0)
	ae, ok := err.(*APIError)
	if !ok || ae.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want *APIError 429, got %v", err)
	}
	if !contains(ae.Error(), "RATE LIMITED") {
		t.Errorf("error text must flag rate limiting: %q", ae.Error())
	}
	retried, failed := c.RateLimitStats()
	if retried != maxRateLimitRetries || failed != 1 {
		t.Errorf("stats retried=%d failed=%d, want %d/1", retried, failed, maxRateLimitRetries)
	}
}

func TestClient_PacingAndRatchet(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"hits":0,"next":-1,"topics":[]}`))
	}))
	defer srv.Close()

	initialDelay := 10 * time.Millisecond
	c := New(srv.URL, 5*time.Second, initialDelay, "", "", nil)
	c.retryBase = time.Millisecond

	if _, err := c.Search("q", "en", 10, 0); err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Active delay must have ratcheted up to at least minRatchetDelay (1000ms)
	if c.CurrentDelay() < minRatchetDelay {
		t.Errorf("expected ratcheted delay >= %v, got %v", minRatchetDelay, c.CurrentDelay())
	}
}

func TestClient_NegativeCache(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"hits":0,"next":-1,"topics":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second, 0, "", "", nil)

	// First lookup misses in search index
	u1, err1 := c.ResolveSourceURL("SSGOVO/unknown.html", "Unknown Topic", "en")
	if err1 != nil || u1 != "" {
		t.Errorf("expected empty on miss, got %q, %v", u1, err1)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}

	// Second lookup for same topic should be served from negative cache (no network call)
	u2, err2 := c.ResolveSourceURL("SSGOVO/unknown.html", "Unknown Topic", "en")
	if err2 != nil || u2 != "" {
		t.Errorf("expected empty on cached miss, got %q, %v", u2, err2)
	}
	if calls != 1 {
		t.Errorf("expected negative cache hit to avoid extra calls, but got %d total calls", calls)
	}

	// Same for slug resolution
	s1, errS1 := c.ResolveHrefBySlug("unknown-slug", "en", "SSGOVO")
	if errS1 != nil || s1 != "" {
		t.Errorf("expected empty on slug miss, got %q, %v", s1, errS1)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}

	s2, errS2 := c.ResolveHrefBySlug("unknown-slug", "en", "SSGOVO")
	if errS2 != nil || s2 != "" {
		t.Errorf("expected empty on cached slug miss, got %q, %v", s2, errS2)
	}
	if calls != 2 {
		t.Errorf("expected negative cache to skip 2nd slug lookup, got %d calls", calls)
	}
}

func TestIsStaticAsset(t *testing.T) {
	for _, tc := range []struct {
		href string
		want bool
	}{
		{"SSGOVO/openapi.yaml", true},
		{"SSGOVO/spec.json?cp=X", true},
		{"SSGOVO/logo.PNG", true},
		{"SSGOVO/doc.pdf", true},
		{"SSGOVO/page.html", false},
		{"SSGOVO/page.html?cp=123", false},
		{"SSGOVO/configuring_sni", false},
	} {
		got := IsStaticAsset(tc.href)
		if got != tc.want {
			t.Errorf("IsStaticAsset(%q) = %v, want %v", tc.href, got, tc.want)
		}
	}
}
