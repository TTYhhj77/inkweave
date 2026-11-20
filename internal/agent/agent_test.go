package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/inkweave/inkweave/internal/config"
	"github.com/inkweave/inkweave/internal/memory"
	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/storage"
	"github.com/inkweave/inkweave/internal/tools"
)

func TestEcoBalancedUltraAgentAndProposals(t *testing.T) {
	dataDir := t.TempDir()
	cfg, err := config.NewManager(dataDir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ws, err := storage.NewWorkspace(cfg.ProjectsDir())
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	repo, err := ws.CreateStarterNovel()
	if err != nil {
		t.Fatalf("CreateStarterNovel: %v", err)
	}
	proj, _ := repo.GetProject()

	idx, err := memory.OpenIndex(repo.Dir)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()
	_ = idx.ReindexProject(repo)

	proposals := tools.NewProposalStore()
	orch := NewOrchestrator(cfg, proposals)

	// Run Eco, Balanced, and Ultra turns
	for _, mode := range []model.AgentMode{model.ModeEco, model.ModeBalanced, model.ModeUltra} {
		ch, err := orch.Run(context.Background(), repo, idx, RunRequest{
			ProjectID: proj.ID,
			ChapterID: "ch-001",
			SkillID:   "character-architect",
			Mode:      mode,
			Prompt:    "ช่วยวิเคราะห์ตัวละครคิรินและออกแบบฉากต่อ",
		})
		if err != nil {
			t.Fatalf("Run(%s): %v", mode, err)
		}
		var sawContext, sawDone bool
		var stageCount int
		for ev := range ch {
			switch ev.Type {
			case "context_summary":
				sawContext = true
			case "stage":
				stageCount++
			case "done":
				sawDone = true
			case "error":
				t.Fatalf("Run(%s) error: %s", mode, ev.Error)
			}
		}
		if !sawContext || !sawDone {
			t.Fatalf("Run(%s) incomplete: sawContext=%v sawDone=%v", mode, sawContext, sawDone)
		}
		if mode == model.ModeUltra && stageCount < 4 {
			t.Fatalf("expected >=4 stages in ultra mode, got %d", stageCount)
		}
	}

	// Verify TokenLedger logged entries
	ledger, err := idx.ListTokenUsage(proj.ID, 50)
	if err != nil || len(ledger) < 3 {
		t.Fatalf("expected >=3 token ledger entries, got %d (err=%v)", len(ledger), err)
	}

	// Verify Proposal diff & snapshot apply
	rawArgs, _ := json.Marshal(map[string]any{
		"chapter_id": "ch-001",
		"mode":       "append",
		"content":    "ย่อหน้าใหม่ที่เพิ่มจากเอเจนต์เพื่อทดสอบ Diff และ Snapshot",
		"summary":    "Add closing beat",
	})
	res, err := tools.Execute(tools.ExecContext{
		ProjectID:    proj.ID,
		SessionID:    "test-session",
		Repo:         repo,
		Index:        idx,
		Proposals:    proposals,
		ApprovalMode: model.ApprovalReview,
	}, "write_chapter_draft", rawArgs)
	if err != nil {
		t.Fatalf("Execute write_chapter_draft: %v", err)
	}
	if res.Proposal == nil || len(res.Proposal.Hunks) == 0 {
		t.Fatalf("expected reviewable proposal with diff hunks")
	}

	appliedCh, err := tools.ApplyProposal(repo, idx, res.Proposal)
	if err != nil {
		t.Fatalf("ApplyProposal: %v", err)
	}
	if res.Proposal.Status != "applied" || appliedCh.WordCount == 0 {
		t.Fatalf("expected applied proposal")
	}
	snaps, err := repo.ListSnapshots("ch-001")
	if err != nil || len(snaps) == 0 {
		t.Fatalf("expected snapshot before proposal apply")
	}
}

func TestAutoSkillRouterAndStreamingEvent(t *testing.T) {
	dataDir := t.TempDir()
	cfg, err := config.NewManager(dataDir)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ws, err := storage.NewWorkspace(cfg.ProjectsDir())
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	repo, err := ws.CreateStarterNovel()
	if err != nil {
		t.Fatalf("CreateStarterNovel: %v", err)
	}
	proj, _ := repo.GetProject()
	idx, err := memory.OpenIndex(repo.Dir)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer idx.Close()
	_ = idx.ReindexProject(repo)

	cases := []struct {
		name     string
		prompt   string
		selected string
		wantID   string
	}{
		{"th_scene", "เขียนฉากต่อจากรอยร้าวโดยคุมมุมมองคิริน", "", "scene-writer"},
		{"th_character", "ช่วยออกแบบตัวละครใหม่ที่มีความย้อนแย้งและบาดแผลฝังใจ", "", "character-architect"},
		{"th_continuity", "ตรวจความต่อเนื่องของบทที่ 1 และ 2 เช็กไทม์ไลน์ด้วย", "", "continuity-guard"},
		{"th_prose", "ช่วยเกลาสำนวนย่อหน้านี้ให้อ่านลื่นและตัดคำซ้ำ", "เสียงฝนตกหนักมาก...", "prose-editor"},
		{"th_plot", "ช่วยวางโครงเรื่ององก์ 2 และจังหวะหักมุมหน่อย", "", "plot-architect"},
		{"th_voice", "ปรับบทสนทนาและน้ำเสียงของลลิตาให้ดูมีชั้นเชิงมากขึ้น", "", "character-voice"},
		{"th_world", "ช่วยสร้างโลกและออกแบบระบบพลังกับกฎของโลกนี้", "", "worldbuilder"},
		{"th_memory", "ช่วยสรุปตอนและบันทึกความจำเหตุการณ์ล่าสุด", "", "memory-curator"},
		{"th_brainstorm", "ตอนนี้เขียนไม่ออกเลย ช่วยระดมสมองคิดไอเดียแก้ทางตันหน่อย", "", "brainstorm-partner"},
		{"en_continuity", "Audit continuity and timeline contradictions in chapter 2", "", "continuity-guard"},
		{"en_prose_selection", "Tighten and improve this passage", "He walked slowly.", "prose-editor"},
		{"fallback_empty", "", "", "scene-writer"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sel := SelectSkillHeuristic(repo.Dir, AutoSkillID, tc.prompt, tc.selected)
			if sel.Skill == nil {
				t.Fatalf("expected skill, got nil")
			}
			if !sel.AutoPicked {
				t.Fatalf("expected AutoPicked=true")
			}
			if sel.Skill.ID != tc.wantID {
				t.Fatalf("prompt=%q: got %q, want %q", tc.prompt, sel.Skill.ID, tc.wantID)
			}
		})
	}

	// Verify Orchestrator emits skill_selected with AutoPicked=true when SkillID="auto"
	orch := NewOrchestrator(cfg, tools.NewProposalStore())
	ch, err := orch.Run(context.Background(), repo, idx, RunRequest{
		ProjectID: proj.ID,
		ChapterID: "ch-001",
		SkillID:   AutoSkillID,
		Mode:      model.ModeBalanced,
		Prompt:    "ตรวจความต่อเนื่องของบทที่ 1 และ 2",
	})
	if err != nil {
		t.Fatalf("Run(auto): %v", err)
	}
	var gotSkillEv *Event
	var sawDone bool
	for ev := range ch {
		if ev.Type == "skill_selected" {
			cp := ev
			gotSkillEv = &cp
		}
		if ev.Type == "done" {
			sawDone = true
		}
		if ev.Type == "error" {
			t.Fatalf("Run(auto) error: %s", ev.Error)
		}
	}
	if !sawDone || gotSkillEv == nil {
		t.Fatalf("expected skill_selected and done events, got skillEv=%v done=%v", gotSkillEv, sawDone)
	}
	if !gotSkillEv.AutoPicked || gotSkillEv.SkillID != "continuity-guard" {
		t.Fatalf("expected auto continuity-guard, got auto=%v id=%q", gotSkillEv.AutoPicked, gotSkillEv.SkillID)
	}
}
