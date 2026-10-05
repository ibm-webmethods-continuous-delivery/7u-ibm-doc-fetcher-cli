package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ibm-webmethods-aftermarket-tools/7u-ibm-doc-fetcher-cli/internal/kb"
)

func TestWriteStubsMerged_NoStubsWritesNothing(t *testing.T) {
	dataDir := t.TempDir()
	topics := []kb.Topic{
		{ProductKey: "demo", TopicID: "overview", Markdown: "# Overview\ncontent", IsStub: false},
	}
	if err := writeStubsMerged(dataDir, "demo", topics); err != nil {
		t.Fatalf("writeStubsMerged: %v", err)
	}
	path := filepath.Join(dataDir, "kb", "demo", "_stubs_merged.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no _stubs_merged.md when there are no stub topics, got err=%v", err)
	}
}

func TestWriteStubsMerged_MergesOnlyStubTopics(t *testing.T) {
	dataDir := t.TempDir()
	topics := []kb.Topic{
		{ProductKey: "demo", TopicID: "full-topic", Markdown: "# Full topic\nreal content", IsStub: false},
		{ProductKey: "demo", TopicID: "stub-a", Markdown: "# Stub A", IsStub: true},
		{ProductKey: "demo", TopicID: "stub-b", Markdown: "# Stub B", IsStub: true},
	}
	if err := writeStubsMerged(dataDir, "demo", topics); err != nil {
		t.Fatalf("writeStubsMerged: %v", err)
	}
	path := filepath.Join(dataDir, "kb", "demo", "_stubs_merged.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read _stubs_merged.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "stub-a") || !strings.Contains(content, "stub-b") {
		t.Errorf("merged content missing stub sources: %s", content)
	}
	if strings.Contains(content, "full-topic") {
		t.Errorf("merged content should not include non-stub topics: %s", content)
	}
}

func TestRebuildIndexes_WritesProductAndTopLevelIndex(t *testing.T) {
	dataDir := t.TempDir()
	// Simulate a previously-built kb/ tree for one product with two topics.
	productDir := filepath.Join(dataDir, "kb", "demo-product")
	if err := os.MkdirAll(filepath.Join(productDir, "overview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(productDir, "public-apis"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(productDir, "overview", "en.md"),
		[]byte("# Overview\n\nSome documentation content.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(productDir, "public-apis", "en.md"),
		[]byte("---\nhttp_methods:\n  \"GET\"\n---\n\n# Public APIs\n\nGET /apis/v1/rest/widgets\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rebuildIndexes(dataDir); err != nil {
		t.Fatalf("rebuildIndexes: %v", err)
	}

	productIndex := filepath.Join(productDir, "INDEX.md")
	if _, err := os.Stat(productIndex); err != nil {
		t.Errorf("expected product INDEX.md to be created: %v", err)
	}
	topIndex := filepath.Join(dataDir, "kb", "INDEX.md")
	if data, err := os.ReadFile(topIndex); err != nil {
		t.Errorf("expected top-level kb/INDEX.md to be created: %v", err)
	} else if !strings.Contains(string(data), "demo-product") {
		t.Errorf("top-level index missing product entry: %s", data)
	}
}
