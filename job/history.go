package job

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// HistoryRecord defines the minimal contract for an item stored in HistoryStore.
type HistoryRecord interface {
	GetID() string
}

// HistoryStore persists a bounded collection of items to JSON, newest first.
type HistoryStore[T HistoryRecord] struct {
	mu       sync.RWMutex
	filePath string
	items    []T
	limit    int
}

// NewHistoryStore creates a HistoryStore with the default cap of 100 entries.
func NewHistoryStore[T HistoryRecord](filePath string) *HistoryStore[T] {
	return NewHistoryStoreWithLimit[T](filePath, 100)
}

// NewHistoryStoreWithLimit creates a HistoryStore with a custom capacity limit.
func NewHistoryStoreWithLimit[T HistoryRecord](filePath string, limit int) *HistoryStore[T] {
	if limit <= 0 {
		limit = 100
	}
	s := &HistoryStore[T]{
		filePath: filePath,
		items:    make([]T, 0),
		limit:    limit,
	}
	s.load()
	return s
}

func (s *HistoryStore[T]) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	var loaded []T
	if err := json.Unmarshal(data, &loaded); err == nil {
		s.items = loaded
	}
}

func (s *HistoryStore[T]) saveLocked() error {
	_ = os.MkdirAll(filepath.Dir(s.filePath), 0755)
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// All returns a copy of history items, newest first.
func (s *HistoryStore[T]) All() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cp := make([]T, len(s.items))
	copy(cp, s.items)
	return cp
}

// Add prepends a new completed item to the top of history and bounds size to limit.
func (s *HistoryStore[T]) Add(item T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items = append([]T{item}, s.items...)
	if len(s.items) > s.limit {
		s.items = s.items[:s.limit]
	}
	return s.saveLocked()
}

// Delete removes an item matching the specified ID.
func (s *HistoryStore[T]) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]T, 0, len(s.items))
	for _, it := range s.items {
		if it.GetID() != id {
			filtered = append(filtered, it)
		}
	}
	s.items = filtered
	return s.saveLocked()
}

// ClearAll removes all history entries.
func (s *HistoryStore[T]) ClearAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items = make([]T, 0)
	return s.saveLocked()
}

// Clear is an alias for ClearAll.
func (s *HistoryStore[T]) Clear() error {
	return s.ClearAll()
}

// Count returns the number of items currently in history.
func (s *HistoryStore[T]) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.items)
}
