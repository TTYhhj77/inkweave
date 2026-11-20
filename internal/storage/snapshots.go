package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/inkweave/inkweave/internal/model"
)

type snapshotFile struct {
	ID        string    `json:"id"`
	ChapterID string    `json:"chapter_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	WordCount int       `json:"word_count"`
	Content   string    `json:"content"`
}

func (r *ProjectRepo) snapshotsDir(chapterID string) string {
	return filepath.Join(r.Dir, ".inkweave", "snapshots", chapterID)
}

// CreateSnapshot saves a restorable pre-edit snapshot of a chapter.
func (r *ProjectRepo) CreateSnapshot(chapterID string, reason string) (*model.Snapshot, error) {
	ch, err := r.GetChapter(chapterID)
	if err != nil {
		return nil, err
	}
	dir := r.snapshotsDir(chapterID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	id := fmt.Sprintf("%s-%s", now.Format("20060102T150405.000"), uuid.NewString()[:6])
	snap := snapshotFile{
		ID:        id,
		ChapterID: chapterID,
		Reason:    reason,
		CreatedAt: now,
		WordCount: ch.WordCount,
		Content:   ch.Content,
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, id+".json")
	if err := WriteAtomic(path, data); err != nil {
		return nil, err
	}
	r.pruneSnapshots(dir, 30)

	preview := ch.Content
	if len([]rune(preview)) > 140 {
		preview = string([]rune(preview)[:140]) + "…"
	}
	return &model.Snapshot{
		ID:        snap.ID,
		ChapterID: snap.ChapterID,
		Reason:    snap.Reason,
		CreatedAt: snap.CreatedAt,
		WordCount: snap.WordCount,
		Preview:   preview,
	}, nil
}

// ListSnapshots returns recent snapshots for a chapter, newest first.
func (r *ProjectRepo) ListSnapshots(chapterID string) ([]model.Snapshot, error) {
	dir := r.snapshotsDir(chapterID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Snapshot{}, nil
		}
		return nil, err
	}
	var out []model.Snapshot
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var sf snapshotFile
		if json.Unmarshal(raw, &sf) != nil {
			continue
		}
		preview := sf.Content
		if len([]rune(preview)) > 140 {
			preview = string([]rune(preview)[:140]) + "…"
		}
		out = append(out, model.Snapshot{
			ID:        sf.ID,
			ChapterID: sf.ChapterID,
			Reason:    sf.Reason,
			CreatedAt: sf.CreatedAt,
			WordCount: sf.WordCount,
			Preview:   preview,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// RestoreSnapshot rolls back a chapter to a saved snapshot (creating a safety snapshot of current text first).
func (r *ProjectRepo) RestoreSnapshot(chapterID, snapshotID string) (*model.Chapter, error) {
	path := filepath.Join(r.snapshotsDir(chapterID), snapshotID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("snapshot %s not found: %w", snapshotID, err)
	}
	var sf snapshotFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		return nil, err
	}
	_, _ = r.CreateSnapshot(chapterID, "auto-before-restore:"+snapshotID)
	ch, err := r.GetChapter(chapterID)
	if err != nil {
		return nil, err
	}
	ch.Content = sf.Content
	if err := r.SaveChapter(ch); err != nil {
		return nil, err
	}
	return ch, nil
}

func (r *ProjectRepo) pruneSnapshots(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= keep {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for i := 0; i < len(names)-keep; i++ {
		_ = os.Remove(filepath.Join(dir, names[i]))
	}
}
