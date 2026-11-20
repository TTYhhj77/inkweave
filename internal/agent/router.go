package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/inkweave/inkweave/internal/llm"
	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/skills"
)

// AutoSkillID requests automatic skill routing based on the user's prompt and selection.
const AutoSkillID = "auto"

// SkillSelection describes which skill was chosen for an agent run and how.
type SkillSelection struct {
	Skill      *model.Skill
	AutoPicked bool
	Method     string // manual | heuristic | llm | fallback
}

type weightedCue struct {
	phrase string
	weight int
}

var builtinSkillCues = map[string][]weightedCue{
	"scene-writer": {
		{"เขียนฉาก", 5}, {"เขียนต่อ", 5}, {"แต่งต่อ", 5}, {"แต่งฉาก", 5}, {"ร่างบท", 5},
		{"เขียนบท", 4}, {"เขียนตอน", 4}, {"บรรยายฉาก", 4}, {"ต่อจาก", 4}, {"เล่าต่อ", 4},
		{"continue scene", 5}, {"write scene", 5}, {"write the next", 5}, {"draft chapter", 5},
		{"keep writing", 4}, {"next scene", 4}, {"write a scene", 5},
		{"ดำเนินเรื่อง", 3}, {"ฉากเปิด", 3}, {"ฉากจบ", 3}, {"ฉากต่อสู้", 3}, {"ฉากปะทะ", 3},
		{"ฉากสารภาพ", 3}, {"บรรยาย", 2}, {"ร่างเนื้อหา", 3}, {"narrate", 2}, {"draft", 2},
	},
	"prose-editor": {
		{"เกลาสำนวน", 6}, {"เกลาภาษา", 6}, {"แก้สำนวน", 5}, {"ขัดเกลา", 5}, {"ปรับสำนวน", 5},
		{"รีไรท์", 5}, {"รีไรต์", 5}, {"ตัดคำซ้ำ", 5}, {"ai slop", 5}, {"สำนวนซ้ำซาก", 5},
		{"polish", 5}, {"rewrite", 5}, {"line edit", 5}, {"edit prose", 5}, {"tighten prose", 5},
		{"ปรับภาษา", 4}, {"แก้ภาษา", 4}, {"อ่านลื่น", 4}, {"กระชับขึ้น", 4}, {"ปรับโทน", 3},
		{"copyedit", 4}, {"proofread", 4}, {"improve prose", 5}, {"refine", 3}, {"flow better", 4},
	},
	"character-architect": {
		{"ออกแบบตัวละคร", 6}, {"สร้างตัวละคร", 6}, {"มิติตัวละคร", 6}, {"ปมตัวละคร", 5},
		{"ตัวละครใหม่", 5}, {"ความย้อนแย้ง", 5}, {"core paradox", 6}, {"non-linear", 5},
		{"จุดถดถอย", 5}, {"relapse", 5}, {"stress-test", 5}, {"stress test", 5},
		{"บาดแผลฝังใจ", 5}, {"formative wound", 5}, {"mask vs", 5}, {"หน้ากาก", 4},
		{"design character", 6}, {"create character", 6}, {"deepen character", 6}, {"character arc", 5},
		{"ตัวละคร", 3}, {"นิสัย", 2}, {"บุคลิก", 3}, {"แรงจูงใจ", 3}, {"ปมในใจ", 4},
		{"จิตวิทยา", 4}, {"เบื้องหลังตัวละคร", 4}, {"พัฒนาการตัวละคร", 4}, {"backstory", 3},
		{"psychology", 4}, {"motivation", 3}, {"protagonist", 2}, {"antagonist", 2},
	},
	"character-voice": {
		{"บทสนทนา", 6}, {"บทพูด", 6}, {"น้ำเสียง", 6}, {"สไตล์การพูด", 5}, {"วิธีพูด", 5},
		{"ปะทะคารม", 5}, {"ไดอะล็อก", 5}, {"หลุดคาแรกเตอร์", 5}, {"คาแรกเตอร์หลุด", 5},
		{"dialogue", 6}, {"character voice", 6}, {"speech pattern", 5}, {"subtext", 4},
		{"คำพูด", 4}, {"โทนเสียง", 4}, {"พูดยังไง", 4}, {"โต้ตอบ", 3}, {"พูดคุย", 3},
		{"สำเนียง", 3}, {"in character", 4}, {"sound like", 4}, {"voice", 3},
	},
	"continuity-guard": {
		{"ตรวจความต่อเนื่อง", 6}, {"เช็กความต่อเนื่อง", 6}, {"เช็คความต่อเนื่อง", 6},
		{"ความต่อเนื่อง", 5}, {"จุดขัดแย้ง", 5}, {"พล็อตโฮล", 6}, {"plot hole", 6}, {"plothole", 6},
		{"ขอบเขตความรู้", 6}, {"unaware", 5}, {"ตรรกะเรื่อง", 5}, {"เช็กตรรกะ", 5}, {"เช็คตรรกะ", 5},
		{"audit continuity", 6}, {"continuity", 5}, {"timeline", 4}, {"ไทม์ไลน์", 4},
		{"ย้อนแย้งกับตอนก่อน", 5}, {"ขัดแย้งกับ", 4}, {"ลำดับเวลา", 4}, {"สถานะตัวละคร", 3},
		{"contradiction", 5}, {"inconsistency", 5}, {"logic check", 5}, {"consistent", 3}, {"audit", 4},
	},
	"plot-architect": {
		{"วางโครงเรื่อง", 6}, {"โครงเรื่อง", 5}, {"วางพล็อต", 6}, {"beat sheet", 6},
		{"โครงจังหวะ", 5}, {"โครงสร้างเรื่อง", 5}, {"วางแผนตอน", 5}, {"หักมุม", 4},
		{"plot outline", 6}, {"story structure", 5}, {"pacing", 4}, {"outline", 5}, {"plot", 4},
		{"พล็อต", 4}, {"จังหวะเรื่อง", 4}, {"องก์", 4}, {"ปูเรื่อง", 4}, {"ปมเรื่อง", 4},
		{"ไคลแมกซ์", 4}, {"climax", 4}, {"twist", 4}, {"foreshadow", 4}, {"ตอนต่อไปควร", 3},
	},
	"worldbuilder": {
		{"สร้างโลก", 6}, {"ระบบพลัง", 6}, {"กฎของโลก", 6}, {"กฎโลก", 5}, {"คลังข้อมูล", 5},
		{"magic system", 6}, {"worldbuild", 6}, {"world-build", 6}, {"world building", 6},
		{"ประวัติศาสตร์โลก", 5}, {"ภูมิศาสตร์", 4}, {"อาณาจักร", 4}, {"เผ่าพันธุ์", 4},
		{"เวทมนตร์", 4}, {"lore", 5}, {"faction", 4}, {"setting", 4}, {"cosmology", 5},
		{"องค์กร", 3}, {"ดินแดน", 3}, {"ตำนาน", 3}, {"วัฒนธรรม", 3}, {"สถานที่", 2},
	},
	"memory-curator": {
		{"สรุปตอน", 6}, {"สรุปบท", 6}, {"สรุปเนื้อเรื่อง", 6}, {"สรุปเหตุการณ์", 6},
		{"บันทึกความจำ", 6}, {"อัปเดตความจำ", 6}, {"อัพเดทความจำ", 6}, {"คลังความจำ", 5},
		{"summarize chapter", 6}, {"update memory", 6}, {"record state", 5}, {"log event", 5},
		{"recap", 5}, {"summary", 4}, {"สรุป", 4}, {"ย่อเรื่อง", 4}, {"บันทึกสถานะ", 4},
		{"อัปเดตสถานะ", 4}, {"อัพเดทไทม์ไลน์", 4}, {"จำไว้", 3}, {"digest", 3},
	},
	"brainstorm-partner": {
		{"ระดมสมอง", 6}, {"คิดไอเดีย", 6}, {"ขอไอเดีย", 5}, {"เขียนไม่ออก", 6}, {"คิดไม่ออก", 5},
		{"ทางตัน", 6}, {"แก้ทางตัน", 6}, {"เสนอทางเลือก", 5}, {"ไม่รู้จะต่อยังไง", 5},
		{"brainstorm", 6}, {"writer's block", 6}, {"writers block", 6}, {"stuck", 5},
		{"what if", 4}, {"ไอเดีย", 4}, {"ตัน", 4}, {"ช่วยคิดหน่อย", 4}, {"จะไปทางไหนดี", 4},
		{"explore options", 4}, {"possibilities", 3}, {"ideas", 4},
	},
}

// SelectSkillHeuristic chooses the best skill purely deterministically with zero LLM tokens.
// Used for fast previewing, Eco mode, and as the primary classifier for Auto mode.
func SelectSkillHeuristic(repoDir, requestedSkillID, prompt, selectedText string) SkillSelection {
	requested := strings.TrimSpace(requestedSkillID)
	if requested != "" && requested != AutoSkillID {
		sk, _ := skills.GetSkill(repoDir, requested)
		return SkillSelection{
			Skill:      sk,
			AutoPicked: false,
			Method:     "manual",
		}
	}

	all, _ := skills.ListSkills(repoDir)
	if len(all) == 0 {
		return SkillSelection{AutoPicked: true, Method: "fallback"}
	}

	sk, topScore, _, method := scoreSkills(all, prompt, selectedText)
	if topScore == 0 {
		method = "fallback"
	}
	return SkillSelection{
		Skill:      sk,
		AutoPicked: true,
		Method:     method,
	}
}

// SelectSkill resolves either a manual skill or automatically selects the most suitable skill.
// In Balanced/Ultra modes, if the zero-token heuristic is ambiguous and a real (non-mock)
// utility LLM provider is configured, it runs a tiny classification call before falling back.
func SelectSkill(
	ctx context.Context,
	repoDir string,
	router *llm.Router,
	mode model.AgentMode,
	requestedSkillID string,
	prompt string,
	selectedText string,
) SkillSelection {
	requested := strings.TrimSpace(requestedSkillID)
	if requested != "" && requested != AutoSkillID {
		sk, _ := skills.GetSkill(repoDir, requested)
		return SkillSelection{
			Skill:      sk,
			AutoPicked: false,
			Method:     "manual",
		}
	}

	all, _ := skills.ListSkills(repoDir)
	if len(all) == 0 {
		return SkillSelection{AutoPicked: true, Method: "fallback"}
	}

	best, topScore, secondScore, method := scoreSkills(all, prompt, selectedText)

	// Confident heuristic hit -> zero extra latency and zero extra tokens.
	confident := topScore >= 5 || (topScore >= 3 && topScore-secondScore >= 2)
	if confident || mode == model.ModeEco || router == nil || strings.TrimSpace(prompt) == "" {
		if topScore == 0 {
			method = "fallback"
		}
		return SkillSelection{
			Skill:      best,
			AutoPicked: true,
			Method:     method,
		}
	}

	// Ambiguous prompt in Balanced/Ultra mode: ask utility model only if it's a real provider.
	prov, mdl, err := router.Resolve("utility")
	if err == nil && prov != nil && prov.Kind() != "mock" {
		if picked := classifySkillWithLLM(ctx, prov, mdl, all, prompt, selectedText); picked != nil {
			return SkillSelection{
				Skill:      picked,
				AutoPicked: true,
				Method:     "llm",
			}
		}
	}

	if topScore == 0 {
		method = "fallback"
	}
	return SkillSelection{
		Skill:      best,
		AutoPicked: true,
		Method:     method,
	}
}

func scoreSkills(all []model.Skill, prompt, selectedText string) (*model.Skill, int, int, string) {
	lowerPrompt := strings.ToLower(strings.TrimSpace(prompt))
	hasSelection := strings.TrimSpace(selectedText) != ""

	type candidate struct {
		skill model.Skill
		score int
	}
	scored := make([]candidate, 0, len(all))

	for _, sk := range all {
		score := 0

		// 1. Explicit mention of skill ID or name gives a strong signal
		if lowerPrompt != "" {
			if strings.Contains(lowerPrompt, strings.ToLower(sk.ID)) {
				score += 8
			}
			if sk.Name != "" && strings.Contains(lowerPrompt, strings.ToLower(sk.Name)) {
				score += 6
			}
			if sk.NameTH != "" && strings.Contains(lowerPrompt, strings.ToLower(sk.NameTH)) {
				score += 6
			}
		}

		// 2. Built-in domain cues (Thai + English)
		if cues, ok := builtinSkillCues[sk.ID]; ok && lowerPrompt != "" {
			for _, c := range cues {
				if strings.Contains(lowerPrompt, c.phrase) {
					score += c.weight
				}
			}
		}

		// 3. Selection-aware boosts (e.g., author highlighted prose to polish or rewrite)
		if hasSelection && sk.ID == "prose-editor" {
			if lowerPrompt == "" {
				score += 3
			} else if containsAny(lowerPrompt, "แก้", "ปรับ", "ดีขึ้น", "สั้นลง", "กระชับ", "improve", "better", "fix", "edit", "shorter") {
				score += 3
			}
		}

		// 4. Dynamic matching for custom skills via their name/description tokens
		if sk.Custom && lowerPrompt != "" {
			meta := strings.ToLower(sk.Name + " " + sk.NameTH + " " + sk.Description + " " + sk.DescriptionTH)
			matched := 0
			for _, tok := range extractKeywords(meta) {
				if strings.Contains(lowerPrompt, tok) {
					matched++
				}
			}
			if matched >= 2 {
				score += matched * 3
			}
		}

		scored = append(scored, candidate{skill: sk, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		// Prefer scene-writer on an exact tie when writing
		if scored[i].skill.ID == "scene-writer" {
			return true
		}
		if scored[j].skill.ID == "scene-writer" {
			return false
		}
		return scored[i].skill.ID < scored[j].skill.ID
	})

	topScore := scored[0].score
	secondScore := 0
	if len(scored) > 1 {
		secondScore = scored[1].score
	}

	if topScore == 0 {
		// Smart default: if author highlighted text with no prompt -> prose-editor, else scene-writer
		fallbackID := "scene-writer"
		if hasSelection && lowerPrompt == "" {
			fallbackID = "prose-editor"
		}
		for i := range all {
			if all[i].ID == fallbackID {
				cp := all[i]
				return &cp, 0, 0, "fallback"
			}
		}
		cp := all[0]
		return &cp, 0, 0, "fallback"
	}

	best := scored[0].skill
	return &best, topScore, secondScore, "heuristic"
}

func classifySkillWithLLM(
	ctx context.Context,
	prov llm.Provider,
	mdl string,
	all []model.Skill,
	prompt string,
	selectedText string,
) *model.Skill {
	routeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	var list strings.Builder
	byID := make(map[string]model.Skill, len(all))
	for _, sk := range all {
		byID[sk.ID] = sk
		desc := sk.Description
		if desc == "" {
			desc = sk.DescriptionTH
		}
		fmt.Fprintf(&list, "- %s: %s\n", sk.ID, desc)
	}

	userMsg := fmt.Sprintf("Author prompt: %s", prompt)
	if strings.TrimSpace(selectedText) != "" {
		userMsg += "\n(The author also highlighted a passage in the chapter editor.)"
	}

	evCh, err := prov.StreamChat(routeCtx, llm.ChatRequest{
		Model: mdl,
		SystemPrefix: "You are a skill router for a novel-writing studio. Pick the single most suitable skill_id from the list below. Reply with ONLY the exact skill_id and nothing else.\n\nAvailable skills:\n" + list.String(),
		Messages:    []llm.Message{{Role: "user", Content: userMsg}},
		Temperature: 0,
		MaxTokens:   32,
		EnableCache: false,
	})
	if err != nil {
		return nil
	}

	var out strings.Builder
	for ev := range evCh {
		if ev.Type == "delta" {
			out.WriteString(ev.Delta)
		}
		if ev.Type == "error" {
			return nil
		}
	}

	raw := strings.ToLower(strings.TrimSpace(out.String()))
	raw = strings.Trim(raw, "`\"' .,\n\r")
	if sk, ok := byID[raw]; ok {
		return &sk
	}
	// Fallback substring match in case the model wrapped the ID in a short sentence
	for id, sk := range byID {
		if strings.Contains(raw, id) {
			cp := sk
			return &cp
		}
	}
	return nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func extractKeywords(text string) []string {
	seen := map[string]struct{}{}
	var out []string
	cur := strings.Builder{}
	flush := func() {
		w := cur.String()
		cur.Reset()
		if len([]rune(w)) >= 3 {
			if _, ok := seen[w]; !ok {
				seen[w] = struct{}{}
				out = append(out, w)
			}
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}
