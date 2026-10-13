package trash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RHamers/claudenator/internal/session"
)

// Store moves session files into and out of a trash directory, recording
// enough metadata (original path, deletion time) to restore or prune them.
type Store struct {
	Dir string
}

type metaRecord struct {
	OriginalPath string    `json:"originalPath"`
	DeletedAt    time.Time `json:"deletedAt"`
}

func TrashPath(dir, sessionID string) string {
	return filepath.Join(dir, sessionID+".jsonl")
}

func MetaPath(dir, sessionID string) string {
	return filepath.Join(dir, sessionID+".meta.json")
}

func readMeta(path string) (metaRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return metaRecord{}, err
	}
	var m metaRecord
	if err := json.Unmarshal(data, &m); err != nil {
		return metaRecord{}, err
	}
	return m, nil
}

func writeMeta(path string, m metaRecord) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Move relocates sess's on-disk file into the trash directory and records
// its original path so it can be restored later.
func (s Store) Move(sess session.Session) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	if err := os.Rename(sess.FilePath, TrashPath(s.Dir, sess.ID)); err != nil {
		return err
	}
	return writeMeta(MetaPath(s.Dir, sess.ID), metaRecord{
		OriginalPath: sess.FilePath,
		DeletedAt:    time.Now(),
	})
}

// Restore moves a trashed session's file back to its original path.
func (s Store) Restore(sessionID string) error {
	meta, err := readMeta(MetaPath(s.Dir, sessionID))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(meta.OriginalPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(TrashPath(s.Dir, sessionID), meta.OriginalPath); err != nil {
		return err
	}
	return os.Remove(MetaPath(s.Dir, sessionID))
}

// Prune permanently deletes trashed sessions older than maxAge.
func (s Store) Prune(maxAge time.Duration, now time.Time) error {
	matches, err := filepath.Glob(filepath.Join(s.Dir, "*.meta.json"))
	if err != nil {
		return err
	}
	for _, metaPath := range matches {
		meta, err := readMeta(metaPath)
		if err != nil {
			continue
		}
		if now.Sub(meta.DeletedAt) <= maxAge {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(metaPath), ".meta.json")
		os.Remove(TrashPath(s.Dir, id))
		os.Remove(metaPath)
	}
	return nil
}
