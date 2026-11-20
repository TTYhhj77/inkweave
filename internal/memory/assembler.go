package memory

import (
	"fmt"
	"strings"

	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/storage"
)

// AssembleRequest specifies parameters for building a mode-budgeted 4-tier context.
type AssembleRequest struct {
	Mode          model.AgentMode
	Skill         *model.Skill
	ChapterID     string
	UserQuery     string
	SelectedText  string
	UltraMaxCap   int
}

// AssembledContext holds both LLM prompt parts and inspectable token diagnostics.
type AssembledContext struct {
	SystemPrefix   string
	DynamicContext string
	Breakdown      model.ContextBreakdown
}

type modeBudget struct {
	TotalCap      int
	TailProseMax  int
	BibleLimit    int
	SummaryLimit  int
	IncludeGraph  bool
}

func budgetForMode(mode model.AgentMode, ultraCap int) modeBudget {
	switch mode {
	case model.ModeEco:
		return modeBudget{
			TotalCap:     6000,
			TailProseMax: 600,
			BibleLimit:   3,
			SummaryLimit: 3,
			IncludeGraph: false,
		}
	case model.ModeUltra:
		capVal := 48000
		if ultraCap >= 16000 {
			capVal = ultraCap
		}
		return modeBudget{
			TotalCap:     capVal,
			TailProseMax: 4200,
			BibleLimit:   14,
			SummaryLimit: 30,
			IncludeGraph: true,
		}
	default: // Balanced
		return modeBudget{
			TotalCap:     18000,
			TailProseMax: 1800,
			BibleLimit:   8,
			SummaryLimit: 12,
			IncludeGraph: true,
		}
	}
}

// AssembleContext deterministically builds the 4-tier context within the selected mode's token budget.
func AssembleContext(repo *storage.ProjectRepo, idx *IndexDB, req AssembleRequest) (*AssembledContext, error) {
	if req.Mode == "" {
		req.Mode = model.ModeBalanced
	}
	b := budgetForMode(req.Mode, req.UltraMaxCap)
	proj, err := repo.GetProject()
	if err != nil {
		return nil, err
	}
	outline, _ := repo.GetOutline()
	chapters, _ := repo.ListChapters()
	chars, _ := repo.ListCharacters()
	lore, _ := repo.ListLore("all")
	states, _ := repo.GetCharacterStates()
	rels, _ := repo.GetRelationships()
	threads, _ := repo.GetPlotThreads()
	timeline, _ := repo.GetTimeline()

	var sections []model.ContextSection
	var truncated []string

	// ============================================================
	// TIER 0 — Cache-Stable Prefix (Story DNA + Hard Rules + Skill)
	// ============================================================
	var tier0Parts []string
	dna := formatStoryDNA(proj, lore)
	dnaTok := EstimateTokens(dna)
	tier0Parts = append(tier0Parts, dna)
	sections = append(sections, model.ContextSection{
		ID:       "tier0-story-dna",
		Tier:     0,
		Title:    "Story DNA, POV, Style & Hard World Rules",
		Tokens:   dnaTok,
		Included: true,
		Preview:  TrimToTokens(dna, 120),
	})

	skillText := ""
	skillID := "general"
	if req.Skill != nil {
		skillID = req.Skill.ID
		skillText = fmt.Sprintf("## Active Skill: %s (%s)\n%s", req.Skill.Name, req.Skill.ID, req.Skill.Instructions)
		skillTok := EstimateTokens(skillText)
		tier0Parts = append(tier0Parts, skillText)
		sections = append(sections, model.ContextSection{
			ID:       "tier0-skill",
			Tier:     0,
			Title:    "Skill Instructions: " + req.Skill.Name,
			Tokens:   skillTok,
			Included: true,
			Preview:  TrimToTokens(skillText, 120),
		})
	}
	systemPrefix := strings.Join(tier0Parts, "\n\n")
	tier0Tokens := EstimateTokens(systemPrefix)

	remaining := b.TotalCap - tier0Tokens
	if remaining < 800 {
		remaining = 800 // preserve minimal dynamic working set
	}

	var dynamicParts []string
	var tier1Tok, tier2Tok, tier3Tok int

	addDynamic := func(sec model.ContextSection, content string, tierCounter *int) {
		if strings.TrimSpace(content) == "" {
			return
		}
		tok := EstimateTokens(content)
		if tok <= remaining {
			dynamicParts = append(dynamicParts, content)
			remaining -= tok
			*tierCounter += tok
			sec.Tokens = tok
			sec.Included = true
			sec.Preview = TrimToTokens(content, 110)
			sections = append(sections, sec)
			return
		}
		if remaining >= 180 {
			trimmed := TrimToTokens(content, remaining)
			tTok := EstimateTokens(trimmed)
			dynamicParts = append(dynamicParts, trimmed)
			remaining -= tTok
			*tierCounter += tTok
			sec.Tokens = tTok
			sec.Included = true
			sec.Reason = "trimmed to mode budget"
			sec.Preview = TrimToTokens(trimmed, 110)
			sections = append(sections, sec)
			truncated = append(truncated, sec.Title)
			return
		}
		sec.Tokens = tok
		sec.Included = false
		sec.Reason = "skipped (token budget reached)"
		sec.Preview = TrimToTokens(content, 80)
		sections = append(sections, sec)
		truncated = append(truncated, sec.Title)
	}

	// ============================================================
	// TIER 1 — Active Chapter Working Set (Beats + Prose + POV Depth + States)
	// ============================================================
	var activeChapter *model.Chapter
	for i := range chapters {
		if chapters[i].ID == req.ChapterID {
			activeChapter = &chapters[i]
			break
		}
	}
	if activeChapter == nil && len(chapters) > 0 {
		activeChapter = &chapters[len(chapters)-1]
	}

	// Find scene characters mentioned in active chapter, selection, or user query
	scanText := req.UserQuery + "\n" + req.SelectedText
	if activeChapter != nil {
		scanText += "\n" + activeChapter.Title + "\n" + activeChapter.POV + "\n" + TailByTokens(activeChapter.Content, 1200)
	}
	presentChars := detectMentionedCharacters(chars, scanText)

	if activeChapter != nil {
		var beatText string
		if co := findChapterOutline(outline, activeChapter.ID); co != nil {
			beatText = formatChapterOutline(*co)
		}
		tail := TailByTokens(activeChapter.Content, b.TailProseMax)
		chBlock := fmt.Sprintf("## Active Chapter (#%d: %s)\n- Chapter ID: %s | POV: %s | Day: %s | Status: %s\n",
			activeChapter.Number, activeChapter.Title, activeChapter.ID, activeChapter.POV, activeChapter.TimelineDay, activeChapter.Status)
		if beatText != "" {
			chBlock += beatText + "\n"
		}
		if req.SelectedText != "" {
			chBlock += "\n### Author Selected Passage\n" + req.SelectedText + "\n"
		}
		if tail != "" {
			chBlock += "\n### Recent Manuscript Prose\n" + tail
		}
		addDynamic(model.ContextSection{
			ID:    "tier1-active-chapter",
			Tier:  1,
			Title: "Active Chapter Beats & Recent Prose",
		}, chBlock, &tier1Tok)
	}

	// Deep Character Profiles for POV / Active Cast
	if len(presentChars) > 0 {
		maxChars := len(presentChars)
		if req.Mode == model.ModeEco && maxChars > 2 {
			maxChars = 2
		}
		var charBlocks []string
		for i := 0; i < maxChars; i++ {
			charBlocks = append(charBlocks, formatDeepCharacter(presentChars[i], req.Mode))
		}
		addDynamic(model.ContextSection{
			ID:    "tier1-cast-psychology",
			Tier:  1,
			Title: fmt.Sprintf("Active Cast Psychology & Voice (%d characters)", maxChars),
		}, "## Active Character Profiles & Voices\n"+strings.Join(charBlocks, "\n\n"), &tier1Tok)
	}

	// Active Character States (Knowledge boundaries, injuries, inventory)
	stateText := formatRelevantStates(states, activeChapter, presentChars)
	if stateText != "" {
		addDynamic(model.ContextSection{
			ID:    "tier1-character-states",
			Tier:  1,
			Title: "Character States & Knowledge Boundaries",
		}, stateText, &tier1Tok)
	}

	// ============================================================
	// TIER 2 — Selective Entity Recall (Dual FTS5 + Graph + Open Threads + Timeline)
	// ============================================================
	if idx != nil && strings.TrimSpace(req.UserQuery+req.SelectedText) != "" {
		searchQ := strings.TrimSpace(req.UserQuery)
		if searchQ == "" {
			searchQ = TrimToTokens(req.SelectedText, 30)
		}
		hits, _ := idx.Search(searchQ, []string{"location", "faction", "item", "rule", "character"}, b.BibleLimit)
		if len(hits) > 0 {
			var lines []string
			for _, h := range hits {
				lines = append(lines, fmt.Sprintf("- **[%s] %s** (`%s`): %s", h.Kind, h.Title, h.RefID, h.Snippet))
			}
			addDynamic(model.ContextSection{
				ID:    "tier2-lore-recall",
				Tier:  2,
				Title: fmt.Sprintf("Selective FTS5 Recall (%d matches)", len(hits)),
			}, "## Recalled Story Bible Entries\n"+strings.Join(lines, "\n"), &tier2Tok)
		}
	}

	if b.IncludeGraph {
		relText := formatRelationships(rels, chars, presentChars)
		if relText != "" {
			addDynamic(model.ContextSection{
				ID:    "tier2-relationships",
				Tier:  2,
				Title: "Asymmetric Character Relationships",
			}, relText, &tier2Tok)
		}
	}

	threadText := formatOpenThreads(threads, presentChars, req.Mode)
	if threadText != "" {
		addDynamic(model.ContextSection{
			ID:    "tier2-plot-threads",
			Tier:  2,
			Title: "Active Plot Threads & Foreshadowing",
		}, threadText, &tier2Tok)
	}

	if req.Mode != model.ModeEco && len(timeline) > 0 {
		tlText := formatTimeline(timeline, 10)
		addDynamic(model.ContextSection{
			ID:    "tier2-timeline",
			Tier:  2,
			Title: "Chronological Timeline Anchor",
		}, tlText, &tier2Tok)
	}

	// ============================================================
	// TIER 3 — Hierarchical Rolling Chapter Summaries
	// ============================================================
	summaryText := formatRollingSummaries(chapters, activeChapter, b.SummaryLimit)
	if summaryText != "" {
		addDynamic(model.ContextSection{
			ID:    "tier3-rolling-summaries",
			Tier:  3,
			Title: "Prior Chapter Summaries",
		}, summaryText, &tier3Tok)
	}

	dynamicContext := strings.Join(dynamicParts, "\n\n")
	totalTokens := tier0Tokens + tier1Tok + tier2Tok + tier3Tok

	return &AssembledContext{
		SystemPrefix:   systemPrefix,
		DynamicContext: dynamicContext,
		Breakdown: model.ContextBreakdown{
			Mode:           req.Mode,
			SkillID:        skillID,
			TotalTokens:    totalTokens,
			BudgetLimit:    b.TotalCap,
			Tier0Tokens:    tier0Tokens,
			Tier1Tokens:    tier1Tok,
			Tier2Tokens:    tier2Tok,
			Tier3Tokens:    tier3Tok,
			CacheFriendly:  req.Mode != model.ModeEco,
			Sections:       sections,
			TruncatedItems: truncated,
		},
	}, nil
}

func formatStoryDNA(p model.Project, lore []model.LoreEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Novel Studio Context: %s\n", p.Title)
	fmt.Fprintf(&b, "- Language: %s | Genre: %s | POV: %s | Tense: %s\n", p.Language, p.Genre, p.POV, p.Tense)
	if p.Synopsis != "" {
		fmt.Fprintf(&b, "- Premise: %s\n", p.Synopsis)
	}
	if len(p.StyleRules) > 0 {
		b.WriteString("\n## Author Style Rules (Mandatory)\n")
		for _, r := range p.StyleRules {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}
	if len(p.AntiCliches) > 0 {
		b.WriteString("\n## Forbidden AI Clichés (NEVER use)\n")
		fmt.Fprintf(&b, "- %s\n", strings.Join(p.AntiCliches, " | "))
	}
	// Include hard world rules in Tier-0 so they remain cache-stable
	var hardRules []string
	for _, item := range lore {
		if item.Category == "rules" {
			hardRules = append(hardRules, fmt.Sprintf("- **%s**: %s", item.Name, item.Summary))
		}
	}
	if len(hardRules) > 0 {
		b.WriteString("\n## Core World Laws\n")
		b.WriteString(strings.Join(hardRules, "\n"))
	}
	return strings.TrimSpace(b.String())
}

func findChapterOutline(o model.Outline, chapterID string) *model.ChapterOutline {
	for _, act := range o.Acts {
		for _, arc := range act.Arcs {
			for i := range arc.Chapters {
				if arc.Chapters[i].ChapterID == chapterID {
					return &arc.Chapters[i]
				}
			}
		}
	}
	return nil
}

func formatChapterOutline(co model.ChapterOutline) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Chapter Goal: %s (Target: %d words)\nBeats:\n", co.Goal, co.TargetWords)
	for i, bt := range co.Beats {
		mark := "[ ]"
		if bt.Completed {
			mark = "[x]"
		}
		fmt.Fprintf(&b, "%d. %s %s", i+1, mark, bt.Summary)
		if bt.Turning != "" {
			fmt.Fprintf(&b, " (Turning: %s)", bt.Turning)
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func detectMentionedCharacters(chars []model.Character, text string) []model.Character {
	lower := strings.ToLower(text)
	var matched []model.Character
	for _, c := range chars {
		found := strings.Contains(lower, strings.ToLower(c.ID)) || strings.Contains(lower, strings.ToLower(c.Name))
		if !found {
			for _, a := range c.Aliases {
				if a != "" && strings.Contains(lower, strings.ToLower(a)) {
					found = true
					break
				}
			}
		}
		if found {
			matched = append(matched, c)
		}
	}
	if len(matched) == 0 && len(chars) > 0 {
		// Fall back to protagonist/deuteragonist if nothing specifically matched
		for _, c := range chars {
			if c.Role == "protagonist" || len(matched) < 2 {
				matched = append(matched, c)
			}
		}
	}
	return matched
}

func formatDeepCharacter(c model.Character, mode model.AgentMode) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s (`%s`, role: %s, arc: %s)\n", c.Name, c.ID, c.Role, c.ArcType)
	if c.CoreParadox != "" {
		fmt.Fprintf(&b, "- Core Paradox: %s\n", c.CoreParadox)
	}
	if c.Mask != "" || c.TrueSelf != "" {
		fmt.Fprintf(&b, "- Mask vs True Self: [%s] vs [%s]\n", c.Mask, c.TrueSelf)
	}
	if c.VoiceGuide != "" {
		fmt.Fprintf(&b, "- Voice Guide: %s\n", c.VoiceGuide)
	}
	if c.AvoidsSaying != "" {
		fmt.Fprintf(&b, "- Avoids Saying: %s\n", c.AvoidsSaying)
	}
	if mode != model.ModeEco {
		if c.FormativeWound != "" || c.Misbelief != "" {
			fmt.Fprintf(&b, "- Wound -> Misbelief: %s -> %s\n", c.FormativeWound, c.Misbelief)
		}
		if c.ExternalWant != "" || c.InternalNeed != "" {
			fmt.Fprintf(&b, "- Want vs Need: %s vs %s\n", c.ExternalWant, c.InternalNeed)
		}
		if len(c.DefenseMechanisms) > 0 {
			fmt.Fprintf(&b, "- Defenses: %s\n", strings.Join(c.DefenseMechanisms, ", "))
		}
		if len(c.SpeechExamples) > 0 {
			fmt.Fprintf(&b, "- Speech Samples: \"%s\"\n", strings.Join(c.SpeechExamples, `" / "`))
		}
		if len(c.ArcPhases) > 0 {
			var phases []string
			for _, p := range c.ArcPhases {
				tag := ""
				if p.IsRegression {
					tag = " [RELAPSE]"
				}
				phases = append(phases, fmt.Sprintf("%s (%d%s)", p.Label, p.TrajectoryValue, tag))
			}
			fmt.Fprintf(&b, "- Non-Linear Arc Phases: %s\n", strings.Join(phases, " -> "))
		}
	}
	return strings.TrimSpace(b.String())
}

func formatRelevantStates(states []model.CharacterState, ch *model.Chapter, present []model.Character) string {
	if len(states) == 0 {
		return ""
	}
	presentSet := map[string]bool{}
	for _, c := range present {
		presentSet[c.ID] = true
	}
	var lines []string
	for _, st := range states {
		if ch != nil && st.ChapterID != ch.ID && !presentSet[st.CharacterID] {
			continue
		}
		lines = append(lines, fmt.Sprintf("- `%s` @ %s | Location: %s | Condition: %s | Emotion: %s | Inventory: [%s] | Knows: [%s] | UNAWARE OF: [%s]",
			st.CharacterID, st.ChapterID, st.Location, st.PhysicalCondition, st.EmotionalState,
			strings.Join(st.Inventory, ", "), strings.Join(st.Knows, "; "), strings.Join(st.UnawareOf, "; ")))
	}
	if len(lines) == 0 {
		return ""
	}
	return "## Character States & Knowledge Boundaries (DO NOT violate Unaware-Of)\n" + strings.Join(lines, "\n")
}

func formatRelationships(rels []model.Relationship, allChars []model.Character, present []model.Character) string {
	if len(rels) == 0 {
		return ""
	}
	nameByID := map[string]string{}
	for _, c := range allChars {
		nameByID[c.ID] = c.Name
	}
	presentSet := map[string]bool{}
	for _, c := range present {
		presentSet[c.ID] = true
	}
	var lines []string
	for _, r := range rels {
		if len(presentSet) > 0 && !presentSet[r.FromCharID] && !presentSet[r.ToCharID] {
			continue
		}
		fromName := nameByID[r.FromCharID]
		if fromName == "" {
			fromName = r.FromCharID
		}
		toName := nameByID[r.ToCharID]
		if toName == "" {
			toName = r.ToCharID
		}
		lines = append(lines, fmt.Sprintf("- %s -> %s: %s (Trust: %d, Tension: %d | Public: %s | Hidden Agenda: %s)",
			fromName, toName, r.Label, r.Trust, r.Tension, r.PublicFace, r.HiddenAgenda))
	}
	if len(lines) == 0 {
		return ""
	}
	return "## Asymmetric Relationships\n" + strings.Join(lines, "\n")
}

func formatOpenThreads(threads []model.PlotThread, present []model.Character, mode model.AgentMode) string {
	var lines []string
	for _, th := range threads {
		if th.Status == "paid_off" || th.Status == "abandoned" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- [%s/%s] %s (Planted: %s): %s", th.Type, th.Status, th.Title, th.PlantedChapter, th.Notes))
		if mode == model.ModeEco && len(lines) >= 3 {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "## Open Plot Threads & Foreshadowing\n" + strings.Join(lines, "\n")
}

func formatTimeline(events []model.TimelineEvent, max int) string {
	start := 0
	if len(events) > max {
		start = len(events) - max
	}
	var lines []string
	for _, ev := range events[start:] {
		lines = append(lines, fmt.Sprintf("- [%s | %s] %s -> %s", ev.DayLabel, ev.Location, ev.Summary, ev.Impact))
	}
	if len(lines) == 0 {
		return ""
	}
	return "## Timeline Ledger\n" + strings.Join(lines, "\n")
}

func formatRollingSummaries(chapters []model.Chapter, active *model.Chapter, max int) string {
	var prior []model.Chapter
	for _, ch := range chapters {
		if active != nil && ch.ID == active.ID {
			continue
		}
		if strings.TrimSpace(ch.Summary) != "" {
			prior = append(prior, ch)
		}
	}
	if len(prior) == 0 {
		return ""
	}
	if len(prior) > max {
		prior = prior[len(prior)-max:]
	}
	var lines []string
	for _, ch := range prior {
		lines = append(lines, fmt.Sprintf("- Ch.%d %s: %s", ch.Number, ch.Title, ch.Summary))
	}
	return "## Prior Chapter Rolling Summaries\n" + strings.Join(lines, "\n")
}
