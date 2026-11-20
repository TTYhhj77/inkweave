package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/inkweave/inkweave/internal/llm"
	"github.com/inkweave/inkweave/internal/memory"
	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/storage"
)

// ProposalStore holds pending AI write proposals awaiting user review/approval.
type ProposalStore struct {
	mu        sync.RWMutex
	proposals map[string]*model.Proposal
}

func NewProposalStore() *ProposalStore {
	return &ProposalStore{proposals: map[string]*model.Proposal{}}
}

func (ps *ProposalStore) Add(p *model.Proposal) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.proposals[p.ID] = p
}

func (ps *ProposalStore) Get(id string) (*model.Proposal, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	p, ok := ps.proposals[id]
	return p, ok
}

func (ps *ProposalStore) ListByProject(projectID string) []*model.Proposal {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	var out []*model.Proposal
	for _, p := range ps.proposals {
		if p.ProjectID == projectID && p.Status == "pending" {
			out = append(out, p)
		}
	}
	return out
}

// ExecContext binds tool execution to one project repository, index, and approval mode.
type ExecContext struct {
	ProjectID    string
	SessionID    string
	Repo         *storage.ProjectRepo
	Index        *memory.IndexDB
	Proposals    *ProposalStore
	ApprovalMode model.ApprovalMode
}

// ExecResult summarizes a tool call's output and any emitted review Proposal.
type ExecResult struct {
	Output   string          `json:"output"`
	Proposal *model.Proposal `json:"proposal,omitempty"`
	Mutated  bool            `json:"mutated"`
}

// Definitions returns tool schemas filtered by allowed names (or all 20 if allowed is empty).
func Definitions(allowed []string, readOnly bool) []llm.ToolDef {
	allowMap := map[string]bool{}
	for _, a := range allowed {
		allowMap[a] = true
	}
	all := allToolDefs()
	var out []llm.ToolDef
	for _, d := range all {
		if readOnly && !IsReadOnly(d.Name) {
			continue
		}
		if len(allowMap) > 0 && !allowMap[d.Name] {
			continue
		}
		out = append(out, d)
	}
	return out
}

// IsReadOnly reports whether a tool has zero side effects on files.
func IsReadOnly(name string) bool {
	switch name {
	case "search_manuscript", "search_lore", "read_chapter", "get_outline",
		"get_character_profile", "get_character_state", "get_timeline",
		"list_open_threads", "get_relationships", "audit_continuity":
		return true
	default:
		return false
	}
}

// Execute runs any of the 20 specialized story tools safely.
func Execute(ctx ExecContext, name string, rawArgs json.RawMessage) (ExecResult, error) {
	switch name {
	case "search_manuscript":
		var a struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		hits, err := ctx.Index.Search(a.Query, []string{"chapter"}, maxInt(a.Limit, 6))
		if err != nil {
			return ExecResult{}, err
		}
		return jsonResult(hits)

	case "search_lore":
		var a struct {
			Query string   `json:"query"`
			Kinds []string `json:"kinds"`
			Limit int      `json:"limit"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		if len(a.Kinds) == 0 {
			a.Kinds = []string{"character", "location", "faction", "item", "rule"}
		}
		hits, err := ctx.Index.Search(a.Query, a.Kinds, maxInt(a.Limit, 8))
		if err != nil {
			return ExecResult{}, err
		}
		return jsonResult(hits)

	case "read_chapter":
		var a struct {
			ChapterID string `json:"chapter_id"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		ch, err := ctx.Repo.GetChapter(a.ChapterID)
		if err != nil {
			return ExecResult{}, err
		}
		return jsonResult(ch)

	case "get_outline":
		o, err := ctx.Repo.GetOutline()
		if err != nil {
			return ExecResult{}, err
		}
		return jsonResult(o)

	case "get_character_profile":
		var a struct {
			NameOrID string `json:"name_or_id"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		if strings.TrimSpace(a.NameOrID) == "" {
			chars, err := ctx.Repo.ListCharacters()
			if err != nil {
				return ExecResult{}, err
			}
			return jsonResult(chars)
		}
		c, err := ctx.Repo.GetCharacter(a.NameOrID)
		if err != nil {
			return ExecResult{}, err
		}
		return jsonResult(c)

	case "get_character_state":
		var a struct {
			ChapterID   string `json:"chapter_id"`
			CharacterID string `json:"character_id"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		states, err := ctx.Repo.GetCharacterStates()
		if err != nil {
			return ExecResult{}, err
		}
		var filtered []model.CharacterState
		for _, s := range states {
			if a.ChapterID != "" && s.ChapterID != a.ChapterID {
				continue
			}
			if a.CharacterID != "" && s.CharacterID != a.CharacterID {
				continue
			}
			filtered = append(filtered, s)
		}
		return jsonResult(filtered)

	case "get_timeline":
		tl, err := ctx.Repo.GetTimeline()
		if err != nil {
			return ExecResult{}, err
		}
		return jsonResult(tl)

	case "list_open_threads":
		threads, err := ctx.Repo.GetPlotThreads()
		if err != nil {
			return ExecResult{}, err
		}
		var open []model.PlotThread
		for _, t := range threads {
			if t.Status != "paid_off" && t.Status != "abandoned" {
				open = append(open, t)
			}
		}
		return jsonResult(open)

	case "get_relationships":
		var a struct {
			CharacterID string `json:"character_id"`
		}
		_ = json.Unmarshal(rawArgs, &a)
		rels, err := ctx.Repo.GetRelationships()
		if err != nil {
			return ExecResult{}, err
		}
		if a.CharacterID == "" {
			return jsonResult(rels)
		}
		var out []model.Relationship
		for _, r := range rels {
			if r.FromCharID == a.CharacterID || r.ToCharID == a.CharacterID {
				out = append(out, r)
			}
		}
		return jsonResult(out)

	case "audit_continuity":
		return runStaticContinuityAudit(ctx.Repo)

	// --- WRITE / PROPOSE TOOLS ---
	case "write_chapter_draft":
		var a struct {
			ChapterID string `json:"chapter_id"`
			Mode      string `json:"mode"` // append | replace
			Content   string `json:"content"`
			Summary   string `json:"summary"`
		}
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return ExecResult{}, err
		}
		ch, err := ctx.Repo.GetChapter(a.ChapterID)
		if err != nil {
			return ExecResult{}, err
		}
		before := ch.Content
		after := a.Content
		if a.Mode == "append" && strings.TrimSpace(before) != "" {
			after = strings.TrimRight(before, "\n") + "\n\n" + strings.TrimLeft(a.Content, "\n")
		}
		return handleProposalOrApply(ctx, name, "chapter", ch.ID, "Draft update: "+ch.Title, a.Summary, before, after, rawArgs)

	case "edit_chapter_section":
		var a struct {
			ChapterID   string `json:"chapter_id"`
			TargetText  string `json:"target_text"`
			Replacement string `json:"replacement"`
			Reason      string `json:"reason"`
		}
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return ExecResult{}, err
		}
		ch, err := ctx.Repo.GetChapter(a.ChapterID)
		if err != nil {
			return ExecResult{}, err
		}
		if !strings.Contains(ch.Content, a.TargetText) {
			return ExecResult{}, fmt.Errorf("target_text not found in chapter %s", ch.ID)
		}
		after := strings.Replace(ch.Content, a.TargetText, a.Replacement, 1)
		return handleProposalOrApply(ctx, name, "chapter", ch.ID, "Edit section: "+ch.Title, a.Reason, ch.Content, after, rawArgs)

	case "save_chapter_summary":
		var a struct {
			ChapterID string `json:"chapter_id"`
			Summary   string `json:"summary"`
		}
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return ExecResult{}, err
		}
		ch, err := ctx.Repo.GetChapter(a.ChapterID)
		if err != nil {
			return ExecResult{}, err
		}
		ch.Summary = a.Summary
		if err := ctx.Repo.SaveChapter(ch); err != nil {
			return ExecResult{}, err
		}
		_ = ctx.Index.ReindexProject(ctx.Repo)
		return ExecResult{Output: fmt.Sprintf(`{"saved":true,"chapter_id":%q}`, ch.ID), Mutated: true}, nil

	case "upsert_character_profile":
		var c model.Character
		if err := json.Unmarshal(rawArgs, &c); err != nil {
			return ExecResult{}, err
		}
		if c.ID != "" {
			if existing, err := ctx.Repo.GetCharacter(c.ID); err == nil {
				if c.Slug == "" {
					c.Slug = existing.Slug
				}
				if len(c.ArcPhases) == 0 {
					c.ArcPhases = existing.ArcPhases
				}
			}
		}
		if err := ctx.Repo.SaveCharacter(&c); err != nil {
			return ExecResult{}, err
		}
		_ = ctx.Index.ReindexProject(ctx.Repo)
		return ExecResult{Output: fmt.Sprintf(`{"saved":true,"character_id":%q,"name":%q}`, c.ID, c.Name), Mutated: true}, nil

	case "record_character_arc_beat":
		var a struct {
			CharacterID     string `json:"character_id"`
			Label           string `json:"label"`
			ChapterRef      string `json:"chapter_ref"`
			TrajectoryValue int    `json:"trajectory_value"`
			InternalShift   string `json:"internal_shift"`
			TriggerEvent    string `json:"trigger_event"`
			IsRegression    bool   `json:"is_regression"`
		}
		if err := json.Unmarshal(rawArgs, &a); err != nil {
			return ExecResult{}, err
		}
		c, err := ctx.Repo.GetCharacter(a.CharacterID)
		if err != nil {
			return ExecResult{}, err
		}
		c.ArcPhases = append(c.ArcPhases, model.CharacterArcPhase{
			ID:              "arc-" + uuid.NewString()[:6],
			PhaseOrder:      len(c.ArcPhases) + 1,
			Label:           a.Label,
			ChapterRef:      a.ChapterRef,
			TrajectoryValue: a.TrajectoryValue,
			InternalShift:   a.InternalShift,
			TriggerEvent:    a.TriggerEvent,
			IsRegression:    a.IsRegression,
		})
		if err := ctx.Repo.SaveCharacter(c); err != nil {
			return ExecResult{}, err
		}
		_ = ctx.Index.ReindexProject(ctx.Repo)
		return ExecResult{Output: fmt.Sprintf(`{"added_arc_phase":%q,"character_id":%q}`, a.Label, c.ID), Mutated: true}, nil

	case "upsert_lore_entry":
		var item model.LoreEntry
		if err := json.Unmarshal(rawArgs, &item); err != nil {
			return ExecResult{}, err
		}
		if item.Category == "" {
			item.Category = "locations"
		}
		if err := ctx.Repo.SaveLore(&item); err != nil {
			return ExecResult{}, err
		}
		_ = ctx.Index.ReindexProject(ctx.Repo)
		return ExecResult{Output: fmt.Sprintf(`{"saved":true,"lore_id":%q,"category":%q}`, item.ID, item.Category), Mutated: true}, nil

	case "update_character_state":
		var st model.CharacterState
		if err := json.Unmarshal(rawArgs, &st); err != nil {
			return ExecResult{}, err
		}
		all, _ := ctx.Repo.GetCharacterStates()
		updated := false
		for i := range all {
			if all[i].ChapterID == st.ChapterID && all[i].CharacterID == st.CharacterID {
				all[i] = st
				updated = true
				break
			}
		}
		if !updated {
			all = append(all, st)
		}
		if err := ctx.Repo.SaveCharacterStates(all); err != nil {
			return ExecResult{}, err
		}
		return ExecResult{Output: `{"saved":true,"state_updated":true}`, Mutated: true}, nil

	case "upsert_relationship":
		var rel model.Relationship
		if err := json.Unmarshal(rawArgs, &rel); err != nil {
			return ExecResult{}, err
		}
		if rel.ID == "" {
			rel.ID = fmt.Sprintf("rel-%s-%s", rel.FromCharID, rel.ToCharID)
		}
		all, _ := ctx.Repo.GetRelationships()
		found := false
		for i := range all {
			if all[i].ID == rel.ID || (all[i].FromCharID == rel.FromCharID && all[i].ToCharID == rel.ToCharID) {
				rel.ID = all[i].ID
				all[i] = rel
				found = true
				break
			}
		}
		if !found {
			all = append(all, rel)
		}
		if err := ctx.Repo.SaveRelationships(all); err != nil {
			return ExecResult{}, err
		}
		return ExecResult{Output: fmt.Sprintf(`{"saved":true,"relationship_id":%q}`, rel.ID), Mutated: true}, nil

	case "record_timeline_event":
		var ev model.TimelineEvent
		if err := json.Unmarshal(rawArgs, &ev); err != nil {
			return ExecResult{}, err
		}
		if ev.ID == "" {
			ev.ID = "tl-" + uuid.NewString()[:6]
		}
		all, _ := ctx.Repo.GetTimeline()
		if ev.Order == 0 {
			ev.Order = len(all) + 1
		}
		all = append(all, ev)
		if err := ctx.Repo.SaveTimeline(all); err != nil {
			return ExecResult{}, err
		}
		_ = ctx.Index.ReindexProject(ctx.Repo)
		return ExecResult{Output: fmt.Sprintf(`{"saved":true,"timeline_id":%q}`, ev.ID), Mutated: true}, nil

	case "manage_plot_thread":
		var th model.PlotThread
		if err := json.Unmarshal(rawArgs, &th); err != nil {
			return ExecResult{}, err
		}
		if th.ID == "" {
			th.ID = "th-" + uuid.NewString()[:6]
		}
		all, _ := ctx.Repo.GetPlotThreads()
		found := false
		for i := range all {
			if all[i].ID == th.ID {
				all[i] = th
				found = true
				break
			}
		}
		if !found {
			all = append(all, th)
		}
		if err := ctx.Repo.SavePlotThreads(all); err != nil {
			return ExecResult{}, err
		}
		_ = ctx.Index.ReindexProject(ctx.Repo)
		return ExecResult{Output: fmt.Sprintf(`{"saved":true,"thread_id":%q,"status":%q}`, th.ID, th.Status), Mutated: true}, nil

	default:
		return ExecResult{}, fmt.Errorf("unknown tool: %s", name)
	}
}

func handleProposalOrApply(ctx ExecContext, toolName, targetKind, targetID, title, summary, before, after string, rawArgs json.RawMessage) (ExecResult, error) {
	if ctx.ApprovalMode == model.ApprovalAutoApply || ctx.Proposals == nil {
		if _, err := ctx.Repo.CreateSnapshot(targetID, "before:"+toolName); err != nil {
			return ExecResult{}, err
		}
		ch, err := ctx.Repo.GetChapter(targetID)
		if err != nil {
			return ExecResult{}, err
		}
		ch.Content = after
		if err := ctx.Repo.SaveChapter(ch); err != nil {
			return ExecResult{}, err
		}
		_ = ctx.Index.ReindexProject(ctx.Repo)
		return ExecResult{
			Output:  fmt.Sprintf(`{"applied":true,"chapter_id":%q,"word_count":%d}`, ch.ID, ch.WordCount),
			Mutated: true,
		}, nil
	}

	prop := &model.Proposal{
		ID:          "prop-" + uuid.NewString()[:8],
		ProjectID:   ctx.ProjectID,
		SessionID:   ctx.SessionID,
		ToolName:    toolName,
		TargetKind:  targetKind,
		TargetID:    targetID,
		Title:       title,
		Summary:     summary,
		BeforeText:  before,
		AfterText:   after,
		PayloadJSON: string(rawArgs),
		Hunks:       ComputeLineDiff(before, after),
		Status:      "pending",
		CreatedAt:   time.Now().UTC(),
	}
	ctx.Proposals.Add(prop)
	return ExecResult{
		Output:   fmt.Sprintf(`{"queued_for_review":true,"proposal_id":%q,"title":%q}`, prop.ID, prop.Title),
		Proposal: prop,
	}, nil
}

// ApplyProposal commits a reviewed Proposal after creating a Snapshot Undo point.
func ApplyProposal(repo *storage.ProjectRepo, idx *memory.IndexDB, prop *model.Proposal) (*model.Chapter, error) {
	if prop.Status == "applied" {
		return repo.GetChapter(prop.TargetID)
	}
	if _, err := repo.CreateSnapshot(prop.TargetID, "before-apply:"+prop.ID); err != nil {
		return nil, err
	}
	ch, err := repo.GetChapter(prop.TargetID)
	if err != nil {
		return nil, err
	}
	ch.Content = prop.AfterText
	if err := repo.SaveChapter(ch); err != nil {
		return nil, err
	}
	prop.Status = "applied"
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	return ch, nil
}

func runStaticContinuityAudit(repo *storage.ProjectRepo) (ExecResult, error) {
	proj, _ := repo.GetProject()
	chapters, _ := repo.ListChapters()
	chars, _ := repo.ListCharacters()
	states, _ := repo.GetCharacterStates()
	threads, _ := repo.GetPlotThreads()

	var findings []string
	// Check Anti-Cliché leaks
	for _, ch := range chapters {
		for _, cliche := range proj.AntiCliches {
			if cliche != "" && strings.Contains(strings.ToLower(ch.Content), strings.ToLower(cliche)) {
				findings = append(findings, fmt.Sprintf("[AI-Slop Cliché] %s contains forbidden cliché %q", ch.Title, cliche))
			}
		}
		if strings.TrimSpace(ch.Summary) == "" && ch.WordCount > 100 {
			findings = append(findings, fmt.Sprintf("[Missing Rolling Summary] %s has %d words but no saved summary for Tier-3 memory", ch.Title, ch.WordCount))
		}
	}
	// Check characters without Paradox or Mask
	for _, c := range chars {
		if strings.TrimSpace(c.CoreParadox) == "" || strings.TrimSpace(c.Mask) == "" {
			findings = append(findings, fmt.Sprintf("[Flat Character Warning] %s lacks CoreParadox or Mask depth fields", c.Name))
		}
	}
	// Check knowledge-state conflicts (simple scan)
	for _, st := range states {
		ch, err := repo.GetChapter(st.ChapterID)
		if err != nil {
			continue
		}
		for _, secret := range st.UnawareOf {
			if len([]rune(secret)) > 6 && strings.Contains(ch.Content, secret) {
				findings = append(findings, fmt.Sprintf("[Knowledge Boundary Check] Verify %s in %s regarding unaware_of item %q", st.CharacterID, ch.Title, secret))
			}
		}
	}
	openCount := 0
	for _, th := range threads {
		if th.Status != "paid_off" && th.Status != "abandoned" {
			openCount++
		}
	}
	report := map[string]any{
		"chapters_scanned":   len(chapters),
		"characters_scanned": len(chars),
		"open_threads":       openCount,
		"findings":           findings,
	}
	return jsonResult(report)
}

func jsonResult(v any) (ExecResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{Output: string(b)}, nil
}

func maxInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func allToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		{
			Name:        "search_manuscript",
			Description: "Search across all novel chapters using Thai/English hybrid full-text search.",
			Parameters:  objSchema(map[string]any{"query": strProp("Search query"), "limit": intProp("Max hits")}),
		},
		{
			Name:        "search_lore",
			Description: "Search Story Bible characters, locations, factions, items, and world rules.",
			Parameters:  objSchema(map[string]any{"query": strProp("Search query"), "limit": intProp("Max hits")}),
		},
		{
			Name:        "read_chapter",
			Description: "Read the full prose and metadata of a specific chapter by ID or slug.",
			Parameters:  objSchema(map[string]any{"chapter_id": strProp("Chapter ID e.g. ch-001")}),
		},
		{
			Name:        "get_outline",
			Description: "Read the Acts, Arcs, and Chapter Beat Sheets of the novel.",
			Parameters:  objSchema(map[string]any{}),
		},
		{
			Name:        "get_character_profile",
			Description: "Inspect full multi-dimensional character psychology, Core Paradox, Mask/True Self, Voice Guide, and Non-Linear Arc.",
			Parameters:  objSchema(map[string]any{"name_or_id": strProp("Character ID, name, or alias (empty lists all)")}),
		},
		{
			Name:        "get_character_state",
			Description: "Inspect character location, injuries, inventory, knows, and unaware_of boundaries per chapter.",
			Parameters:  objSchema(map[string]any{"chapter_id": strProp("Optional chapter ID"), "character_id": strProp("Optional character ID")}),
		},
		{
			Name:        "get_timeline",
			Description: "Read the chronological event ledger in continuity/timeline.yaml.",
			Parameters:  objSchema(map[string]any{}),
		},
		{
			Name:        "list_open_threads",
			Description: "List active/planted foreshadowing clues, mysteries, and subplots.",
			Parameters:  objSchema(map[string]any{}),
		},
		{
			Name:        "get_relationships",
			Description: "Inspect asymmetric directed relationships, trust, tension, and hidden agendas.",
			Parameters:  objSchema(map[string]any{"character_id": strProp("Optional character ID")}),
		},
		{
			Name:        "audit_continuity",
			Description: "Run deterministic checks for forbidden AI clichés, missing summaries, character depth gaps, and knowledge boundaries.",
			Parameters:  objSchema(map[string]any{}),
		},
		{
			Name:        "write_chapter_draft",
			Description: "Append or replace prose in a chapter (creates a reviewable Diff Proposal and snapshot).",
			Parameters:  objSchema(map[string]any{"chapter_id": strProp("Chapter ID"), "mode": strProp("append or replace"), "content": strProp("Prose text"), "summary": strProp("Brief note")}),
		},
		{
			Name:        "edit_chapter_section",
			Description: "Surgically replace an exact excerpt inside a chapter (creates a reviewable Diff Proposal).",
			Parameters:  objSchema(map[string]any{"chapter_id": strProp("Chapter ID"), "target_text": strProp("Exact text to replace"), "replacement": strProp("Revised text"), "reason": strProp("Why")}),
		},
		{
			Name:        "save_chapter_summary",
			Description: "Save a compact rolling summary for a chapter into frontmatter and SQLite Tier-3 memory.",
			Parameters:  objSchema(map[string]any{"chapter_id": strProp("Chapter ID"), "summary": strProp("80-140 word summary")}),
		},
		{
			Name:        "upsert_character_profile",
			Description: "Create or update a deep non-linear character profile including Core Paradox, Mask, True Self, Blind Spot, Wound, Misbelief, and Voice.",
			Parameters:  objSchema(map[string]any{"id": strProp("Character ID"), "name": strProp("Full name"), "role": strProp("Role"), "core_paradox": strProp("Core paradox"), "mask": strProp("Social mask"), "true_self": strProp("Hidden self"), "voice_guide": strProp("Voice guide"), "arc_type": strProp("spiral|oscillating|fractured|positive|negative|flat")}),
		},
		{
			Name:        "record_character_arc_beat",
			Description: "Add a non-linear progression or regression/relapse beat to a character's trajectory curve.",
			Parameters:  objSchema(map[string]any{"character_id": strProp("Character ID"), "label": strProp("Beat label"), "chapter_ref": strProp("Chapter ID"), "trajectory_value": intProp("-100 to 100"), "internal_shift": strProp("Shift"), "trigger_event": strProp("Trigger"), "is_regression": map[string]any{"type": "boolean"}}),
		},
		{
			Name:        "upsert_lore_entry",
			Description: "Create or update a location, faction, item, or world rule in Story Bible.",
			Parameters:  objSchema(map[string]any{"id": strProp("Optional ID"), "category": strProp("locations|factions|items|rules"), "name": strProp("Name"), "summary": strProp("Summary"), "content": strProp("Markdown details")}),
		},
		{
			Name:        "update_character_state",
			Description: "Update character location, physical condition, inventory, knows, or unaware_of for a chapter.",
			Parameters:  objSchema(map[string]any{"chapter_id": strProp("Chapter ID"), "character_id": strProp("Character ID"), "location": strProp("Location"), "emotional_state": strProp("Emotion"), "physical_condition": strProp("Condition")}),
		},
		{
			Name:        "upsert_relationship",
			Description: "Record one-way directed relationship edge with trust (-100..100), tension (0..100), public_face, and hidden_agenda.",
			Parameters:  objSchema(map[string]any{"from_char_id": strProp("Source character ID"), "to_char_id": strProp("Target character ID"), "label": strProp("Dynamic label"), "trust": intProp("-100..100"), "tension": intProp("0..100"), "public_face": strProp("Public face"), "hidden_agenda": strProp("Hidden agenda")}),
		},
		{
			Name:        "record_timeline_event",
			Description: "Append a chronological event to continuity/timeline.yaml.",
			Parameters:  objSchema(map[string]any{"day_label": strProp("Day/time label"), "chapter_id": strProp("Chapter ID"), "location": strProp("Location"), "summary": strProp("What happened"), "impact": strProp("Consequences")}),
		},
		{
			Name:        "manage_plot_thread",
			Description: "Create or update a mystery, subplot, or foreshadowing thread status.",
			Parameters:  objSchema(map[string]any{"id": strProp("Thread ID"), "title": strProp("Title"), "type": strProp("mystery|subplot|foreshadowing"), "status": strProp("planted|active|escalated|paid_off"), "planted_chapter": strProp("Chapter"), "notes": strProp("Details")}),
		},
	}
}

func objSchema(props map[string]any) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
	}
}
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}
func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
