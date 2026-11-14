package job

import (
	"path/filepath"
	"testing"
)

type testHistoryItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (t testHistoryItem) GetID() string {
	return t.ID
}

func TestHistoryStoreBasicOperations(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "history.json")

	store := NewHistoryStoreWithLimit[testHistoryItem](storePath, 3)

	if store.Count() != 0 {
		t.Fatalf("expected 0 items, got %d", store.Count())
	}

	_ = store.Add(testHistoryItem{ID: "1", Title: "One"})
	_ = store.Add(testHistoryItem{ID: "2", Title: "Two"})

	if store.Count() != 2 {
		t.Fatalf("expected 2 items, got %d", store.Count())
	}

	items := store.All()
	if items[0].ID != "2" || items[1].ID != "1" {
		t.Fatalf("expected newest item first, got %+v", items)
	}

	// Add more to test bounding limit
	_ = store.Add(testHistoryItem{ID: "3", Title: "Three"})
	_ = store.Add(testHistoryItem{ID: "4", Title: "Four"})

	if store.Count() != 3 {
		t.Fatalf("expected capped at 3 items, got %d", store.Count())
	}
	items = store.All()
	if items[0].ID != "4" || items[2].ID != "2" {
		t.Fatalf("expected oldest dropped, got %+v", items)
	}

	// Delete item
	_ = store.Delete("3")
	if store.Count() != 2 {
		t.Fatalf("expected 2 items after delete, got %d", store.Count())
	}

	// Reload from disk to verify persistence
	reloaded := NewHistoryStoreWithLimit[testHistoryItem](storePath, 3)
	if reloaded.Count() != 2 {
		t.Fatalf("expected reloaded store to have 2 items, got %d", reloaded.Count())
	}

	// Clear all
	_ = store.ClearAll()
	if store.Count() != 0 {
		t.Fatalf("expected 0 items after ClearAll, got %d", store.Count())
	}
}
