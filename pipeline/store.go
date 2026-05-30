package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// StateStore defines an interface for persisting pipeline state.
type StateStore interface {
	Save(ctx context.Context, key string, state *State) error
	Load(ctx context.Context, key string) (*State, error)
	Delete(ctx context.Context, key string) error
}

// FileStateStore is a file-backed implementation of StateStore.
// State is stored as JSON files at {BaseDir}/{key}.json.
// Not safe for concurrent writes to the same key — the factory pipeline
// processes phases sequentially so this is not an issue in practice.
type FileStateStore struct {
	BaseDir string
}

// NewFileStateStore creates a FileStateStore rooted at baseDir.
func NewFileStateStore(baseDir string) *FileStateStore {
	return &FileStateStore{BaseDir: baseDir}
}

// Save persists the state to disk as a JSON file at {BaseDir}/{key}.json.
// Intermediate directories are created as needed.
func (s *FileStateStore) Save(_ context.Context, key string, state *State) error {
	p := s.StatePath(key)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state dir %s: %w", dir, err)
	}
	return SaveState(p, state)
}

// Load reads state from the JSON file at {BaseDir}/{key}.json.
func (s *FileStateStore) Load(_ context.Context, key string) (*State, error) {
	return LoadState(s.StatePath(key))
}

// Delete removes the state file at {BaseDir}/{key}.json.
func (s *FileStateStore) Delete(_ context.Context, key string) error {
	p := s.StatePath(key)
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("delete state %s: %w", p, err)
	}
	return nil
}

// StatePath returns the filesystem path for a given key.
// This is useful for passing the path to phase binaries that read state
// directly from the filesystem.
func (s *FileStateStore) StatePath(key string) string {
	return filepath.Join(s.BaseDir, key+".json")
}

// StateKey returns the canonical store key for a given repository and issue.
func StateKey(owner, repo string, issue int) string {
	return fmt.Sprintf("%s/%s/%d", owner, repo, issue)
}
