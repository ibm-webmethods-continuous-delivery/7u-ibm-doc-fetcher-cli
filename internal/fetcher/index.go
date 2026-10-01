package fetcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IndexEntry records one fetch operation in index.json.
type IndexEntry struct {
	URL           string    `json:"url"`
	ProductKey    string    `json:"product_key"`
	FetchedAt     time.Time `json:"fetched_at"`
	Depth         int       `json:"depth"`
	TopicsFetched int       `json:"topics_fetched"`
	TopicsFailed  int       `json:"topics_failed"`
	Status        string    `json:"status"` // "ok" | "partial" | "failed"
}

// Index is the full index.json file.
type Index struct {
	Version int          `json:"version"`
	Entries []IndexEntry `json:"entries"`
}

// indexPath returns the path to index.json under dataDir.
func indexPath(dataDir string) string {
	return filepath.Join(dataDir, "index.json")
}

// LoadIndex reads index.json from dataDir. Returns an empty Index if the file
// does not exist yet.
func LoadIndex(dataDir string) (*Index, error) {
	path := indexPath(dataDir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Index{Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	return &idx, nil
}

// SaveIndex atomically writes index.json to dataDir.
func SaveIndex(dataDir string, idx *Index) error {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("index mkdir: %w", err)
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("index marshal: %w", err)
	}
	path := indexPath(dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("index write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("index rename: %w", err)
	}
	return nil
}

// Upsert adds or updates the entry for url in the index.
func (idx *Index) Upsert(e IndexEntry) {
	for i, existing := range idx.Entries {
		if existing.URL == e.URL {
			idx.Entries[i] = e
			return
		}
	}
	idx.Entries = append(idx.Entries, e)
}
