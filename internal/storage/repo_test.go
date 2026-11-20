package storage

import (
	"testing"

	"github.com/inkweave/inkweave/internal/model"
)

func TestWorkspaceRoundTripAndStarterNovel(t *testing.T) {
	tmp := t.TempDir()
	ws, err := NewWorkspace(tmp)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}

	repo, err := ws.CreateStarterNovel()
	if err != nil {
		t.Fatalf("CreateStarterNovel: %v", err)
	}

	proj, err := repo.GetProject()
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if proj.Language != "th" || proj.Title == "" {
		t.Fatalf("unexpected project: %+v", proj)
	}

	// Verify multi-dimensional character persistence
	kirin, err := repo.GetCharacter("คิริน")
	if err != nil {
		t.Fatalf("GetCharacter by alias: %v", err)
	}
	if kirin.CoreParadox == "" || kirin.Mask == "" || kirin.TrueSelf == "" {
		t.Fatalf("expected non-empty deep character fields")
	}
	if len(kirin.ArcPhases) != 4 || !kirin.ArcPhases[2].IsRegression {
		t.Fatalf("expected 4 non-linear arc phases with regression phase, got %+v", kirin.ArcPhases)
	}

	// Verify Thai prose metrics & snapshots
	ch, err := repo.GetChapter("ch-001")
	if err != nil {
		t.Fatalf("GetChapter: %v", err)
	}
	if ch.WordCount < 50 || ch.CharCount < 200 {
		t.Fatalf("expected Thai word metrics, got words=%d chars=%d", ch.WordCount, ch.CharCount)
	}

	snap, err := repo.CreateSnapshot(ch.ID, "before-edit-test")
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	originalContent := ch.Content
	ch.Content = "ข้อความทดสอบที่แก้ไขแล้ว"
	if err := repo.SaveChapter(ch); err != nil {
		t.Fatalf("SaveChapter: %v", err)
	}

	restored, err := repo.RestoreSnapshot(ch.ID, snap.ID)
	if err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	if restored.Content != originalContent {
		t.Fatalf("snapshot restore mismatch")
	}

	// Verify projects listing
	list, err := ws.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(list) != 1 || list[0].CharacterCount != 3 || list[0].ChapterCount != 2 {
		t.Fatalf("unexpected project summary: %+v", list)
	}

	// Verify custom project creation
	customRepo, err := ws.CreateProject(model.Project{
		Title:    "The Clockwork Estuary",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("CreateProject en: %v", err)
	}
	cp, _ := customRepo.GetProject()
	if cp.Slug != "the-clockwork-estuary" {
		t.Fatalf("unexpected slug: %s", cp.Slug)
	}

	// Verify chapter reorder (including collision safety) & delete
	c1 := &model.Chapter{Title: "Same", Content: "First"}
	c2 := &model.Chapter{Title: "Same", Content: "Second"}
	if err := customRepo.SaveChapter(c1); err != nil {
		t.Fatalf("SaveChapter c1: %v", err)
	}
	if err := customRepo.SaveChapter(c2); err != nil {
		t.Fatalf("SaveChapter c2: %v", err)
	}
	if err := customRepo.ReorderChapters([]string{c2.ID, c1.ID}); err != nil {
		t.Fatalf("ReorderChapters: %v", err)
	}
	reordered, err := customRepo.ListChapters()
	if err != nil || len(reordered) != 2 || reordered[0].ID != c2.ID || reordered[1].ID != c1.ID {
		t.Fatalf("unexpected reorder result: %+v (err=%v)", reordered, err)
	}
	if err := customRepo.DeleteChapter(c2.ID); err != nil {
		t.Fatalf("DeleteChapter: %v", err)
	}
	remaining, _ := customRepo.ListChapters()
	if len(remaining) != 1 || remaining[0].ID != c1.ID {
		t.Fatalf("expected only c1 remaining, got %+v", remaining)
	}
}
