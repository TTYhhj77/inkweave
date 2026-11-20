package memory

import (
	"testing"

	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/storage"
)

func TestThaiFTS5SearchAndModeBudgets(t *testing.T) {
	ws, err := storage.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	repo, err := ws.CreateStarterNovel()
	if err != nil {
		t.Fatalf("CreateStarterNovel: %v", err)
	}

	idx, err := OpenIndex(repo.Dir)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()

	if err := idx.ReindexProject(repo); err != nil {
		t.Fatalf("ReindexProject: %v", err)
	}

	// Test unspaced Thai substring phrase search via trigram FTS5
	hits, err := idx.Search("ผลึกความจำ", nil, 8)
	if err != nil {
		t.Fatalf("Search Thai: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected Thai search hits for 'ผลึกความจำ', got 0")
	}

	// Test short Thai character name/alias recall
	charHits, err := idx.Search("คิริน", []string{"character"}, 5)
	if err != nil {
		t.Fatalf("Search character: %v", err)
	}
	if len(charHits) == 0 || charHits[0].RefID != "char-kirin" {
		t.Fatalf("expected char-kirin top hit, got %+v", charHits)
	}

	// Test token budget enforcement across Eco, Balanced, Ultra
	ecoCtx, err := AssembleContext(repo, idx, AssembleRequest{
		Mode:      model.ModeEco,
		ChapterID: "ch-001",
		UserQuery: "เขียนฉากต่อจากผลึกสีขุ่น",
	})
	if err != nil {
		t.Fatalf("AssembleContext eco: %v", err)
	}
	if ecoCtx.Breakdown.TotalTokens > 6000 {
		t.Fatalf("eco mode exceeded 6000 token cap: %d", ecoCtx.Breakdown.TotalTokens)
	}

	balCtx, err := AssembleContext(repo, idx, AssembleRequest{
		Mode:      model.ModeBalanced,
		ChapterID: "ch-001",
		UserQuery: "เขียนฉากต่อจากผลึกสีขุ่น",
	})
	if err != nil {
		t.Fatalf("AssembleContext balanced: %v", err)
	}
	if balCtx.Breakdown.TotalTokens > 18000 {
		t.Fatalf("balanced mode exceeded 18000 token cap: %d", balCtx.Breakdown.TotalTokens)
	}

	ultraCtx, err := AssembleContext(repo, idx, AssembleRequest{
		Mode:      model.ModeUltra,
		ChapterID: "ch-001",
		UserQuery: "เขียนฉากต่อจากผลึกสีขุ่น",
	})
	if err != nil {
		t.Fatalf("AssembleContext ultra: %v", err)
	}
	if ultraCtx.Breakdown.TotalTokens <= ecoCtx.Breakdown.TotalTokens {
		t.Fatalf("expected ultra context (%d) to contain richer memory than eco (%d)",
			ultraCtx.Breakdown.TotalTokens, ecoCtx.Breakdown.TotalTokens)
	}
}
