package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/inkweave/inkweave/internal/config"
	"github.com/inkweave/inkweave/internal/model"
)

// MockProvider provides an offline, deterministic literary AI simulation so users
// and automated tests can try every skill, tool, and mode without needing an API key.
type MockProvider struct {
	id string
}

func NewMockProvider(id string) *MockProvider {
	return &MockProvider{id: id}
}

func (m *MockProvider) ID() string   { return m.id }
func (m *MockProvider) Kind() string { return "mock" }

func (m *MockProvider) TestConnection(ctx context.Context, model string) error {
	return nil
}

func (m *MockProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	out := make(chan StreamEvent, 32)
	go func() {
		defer close(out)

		hasToolResults := false
		var lastUser string
		for _, msg := range req.Messages {
			if msg.Role == "tool" {
				hasToolResults = true
			}
			if msg.Role == "user" {
				lastUser = msg.Content
			}
		}

		// Detect Ultra stages or tool usage
		sys := req.SystemPrefix + "\n" + req.DynamicContext
		lowerUser := strings.ToLower(lastUser)

		// If first turn and read tools are available and not eco-minimal, invoke a helpful inspection tool
		if !hasToolResults && len(req.Tools) > 0 && strings.Contains(lowerUser, "ต่อ") || (!hasToolResults && len(req.Tools) > 0 && strings.Contains(lowerUser, "ตัวละคร")) {
			if strings.Contains(lowerUser, "ตัวละคร") || strings.Contains(sys, "character-architect") {
				if hasTool(req.Tools, "get_character_profile") {
					args, _ := json.Marshal(map[string]any{"name_or_id": "char-kirin"})
					out <- StreamEvent{
						Type:          "thinking",
						ThinkingDelta: "กำลังตรวจสอบโปรไฟล์เชิงลึก ปมขัดแย้ง และ Non-linear Arc ของตัวละครก่อนออกแบบเพิ่มเติม...",
					}
					out <- StreamEvent{
						Type: "tool_call",
						ToolCall: &ToolCall{
							ID:        "mock-tc-1",
							Name:      "get_character_profile",
							Arguments: args,
						},
					}
					out <- StreamEvent{
						Type:  "usage",
						Usage: &Usage{InputTokens: 640, CachedTokens: 320, OutputTokens: 35},
					}
					out <- StreamEvent{Type: "done"}
					return
				}
			}
			if hasTool(req.Tools, "get_character_state") {
				args, _ := json.Marshal(map[string]any{"chapter_id": "ch-001"})
				out <- StreamEvent{
					Type:          "thinking",
					ThinkingDelta: "กำลังตรวจเช็กสถานะตัวละครและขอบเขตความรู้ ณ บทปัจจุบันเพื่อกันหลุดความต่อเนื่อง...",
				}
				out <- StreamEvent{
					Type: "tool_call",
					ToolCall: &ToolCall{
						ID:        "mock-tc-1",
						Name:      "get_character_state",
						Arguments: args,
					},
				}
				out <- StreamEvent{
					Type:  "usage",
					Usage: &Usage{InputTokens: 720, CachedTokens: 360, OutputTokens: 30},
				}
				out <- StreamEvent{Type: "done"}
				return
			}
		}

		reply := craftMockReply(sys, lastUser)
		// Stream chunks smoothly
		runes := []rune(reply)
		step := 18
		for i := 0; i < len(runes); i += step {
			end := i + step
			if end > len(runes) {
				end = len(runes)
			}
			select {
			case <-ctx.Done():
				out <- StreamEvent{Type: "error", Error: ctx.Err().Error()}
				return
			case out <- StreamEvent{Type: "delta", Delta: string(runes[i:end])}:
				time.Sleep(8 * time.Millisecond)
			}
		}
		cached := 0
		if req.EnableCache {
			cached = 850
		}
		out <- StreamEvent{
			Type: "usage",
			Usage: &Usage{
				InputTokens:  1240,
				CachedTokens: cached,
				OutputTokens: len(runes) / 2,
			},
		}
		out <- StreamEvent{Type: "done"}
	}()
	return out, nil
}

func hasTool(tools []ToolDef, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func craftMockReply(sys, user string) string {
	switch {
	case strings.Contains(sys, "CONTINUITY_CRITIC_STAGE"):
		return "✅ **ผลการตรวจความต่อเนื่องและสำนวน (Continuity & Depth Audit)**\n" +
			"- **Knowledge Boundary**: คิรินยังไม่ทราบว่าดารินลบแฟ้มช่อง 409-ข ล่วงหน้า — รักษาความไม่สมมาตรของข้อมูลถูกต้อง\n" +
			"- **Character Voice & Mask**: คิรินยังรักษาหน้ากากช่างซ่อมที่พูดน้อยและหลบเลี่ยงคำขอโทษ แต่เปิดเผยรอยร้าวทางอารมณ์ผ่านการขยับคีมทองเหลือง\n" +
			"- **Anti-AI-Slop**: ไม่พบสำนวนซ้ำซากต้องห้าม\n\n" +
			"พร้อมส่งร่างฉบับเกลาสำนวนแล้วครับ"
	case strings.Contains(sys, "ARCHITECT_STAGE"):
		return "📐 **โครงร่างจังหวะฉาก (Scene Beat Plan)**\n" +
			"1. คิรินแกล้งทำเป็นสนใจกลไกนาฬิกามากกว่ากล่องดีบุก เพื่อยึดระยะปลอดภัยทางอารมณ์\n" +
			"2. ลลิตาวางเงื่อนไขที่สะกิดความรู้สึกผิดเมื่อ 6 ปีก่อน โดยไม่เอ่ยชื่อธามตรงๆ (Subtext)\n" +
			"3. จบฉากด้วยจังหวะ Non-linear: แม้คิรินจะปฏิเสธเสียงแข็ง แต่ปลายนิ้วที่สั่นกลับแอบเก็บเศษผลึกติดโต๊ะไว้ (แสดง Core Paradox ระหว่างคำพูดกับการกระทำ)"
	case strings.Contains(sys, "character-architect"):
		return "🎭 **การออกแบบมิติเชิงลึกและเส้นโค้งแบบไม่เป็นเส้นตรง (Non-Linear Character Design)**\n\n" +
			"- **Core Paradox**: ตัวละครที่เชื่อว่าความเมตตาคือการไม่ผูกพัน แต่มือกลับซ่อมของพังของคนอื่นทุกคืน\n" +
			"- **Mask vs True Self**: หน้ากากที่เยือกเย็นด้วยเหตุผล ซ่อนความกลัวที่จะทำลายคนที่รักซ้ำรอยเดิม\n" +
			"- **Non-Linear Arc (Spiral / Relapse)**: แทนที่จะพัฒนาขึ้นเป็นเส้นตรง แนะนำให้เพิ่มช่วง *Relapse* ในองก์กลาง — เมื่อเบาะแสชี้ว่าเขามีส่วนเกี่ยวข้องจริง ตัวละครจะถอยกลับไปใช้นิสัยหนีความจริงชั่วคราวก่อนจะตัดสินใจเผชิญหน้าใหม่\n\n" +
			"*(หมายเหตุ: ขณะนี้กำลังใช้งาน Built-in Studio Demo — คุณสามารถเชื่อมต่อ API Key ของ OpenAI / Anthropic / Gemini / DeepSeek / Ollama ใน Settings เพื่อรันโมเดลจริงได้ทันที)*"
	case strings.Contains(sys, "continuity-guard"):
		return "🛡️ **รายงานตรวจความต่อเนื่องและตรรกะเรื่อง (Continuity Guard)**\n\n" +
			"- **ไทม์ไลน์**: ลำดับเหตุการณ์สอดคล้องกันดี ไม่พบช่วงเวลาซ้อนทับ\n" +
			"- **ขอบเขตความรู้ (Unaware-Of)**: ตัวละครยังคงรู้ข้อมูลเท่าที่ตนได้เห็นจริง ไม่หลุดสปอยล์จากมุมมองอื่น\n" +
			"- **ข้อสังเกต**: แนะนำย้ำสภาพไอเทมหรือบาดแผลต่อเนื่องจากฉากก่อนหน้าเล็กน้อยเพื่อเพิ่มความสมจริง\n\n" +
			"*(Demo Mode — เชื่อมต่อ API Key ใน Settings เพื่อตรวจต้นฉบับจริงด้วย AI)*"
	case strings.Contains(sys, "prose-editor"):
		return "✂️ **ข้อเสนอแนะการเกลาสำนวน (Prose Editor)**\n\n" +
			"- ตัดคำขยายซ้ำซ้อนและเปลี่ยนคำบรรยายอารมณ์ตรงๆ เป็นภาษากาย (Show, Don't Tell)\n" +
			"- สลับความยาวประโยคสั้น-ยาวในจังหวะตึงเครียดให้คนอ่านรู้สึกถึงแรงกดดันมากขึ้น\n" +
			"- ไม่พบสำนวน AI Slop ที่ซ้ำซาก\n\n" +
			"*(Demo Mode — เชื่อมต่อ API Key ใน Settings เพื่อให้ AI ส่ง Diff แก้สำนวนจริง)*"
	case strings.Contains(sys, "plot-architect"):
		return "🧭 **โครงร่างจังหวะเรื่องและพล็อต (Plot Architect)**\n\n" +
			"1. **จังหวะเปิด (Hook)**: เปิดด้วยเงื่อนไขที่บังคับให้ตัวละครตัดสินใจทันที\n" +
			"2. **แรงเสียดทาน (Complication)**: วิธีแก้ปัญหาเดิมใช้ไม่ได้ผลเพราะปมในอดีตย้อนกลับมา\n" +
			"3. **จุดพลิกท้ายตอน (Turn)**: ได้เบาะแสใหม่แต่ต้องแลกด้วยความไว้วางใจที่สั่นคลอน\n\n" +
			"*(Demo Mode — เชื่อมต่อ API Key ใน Settings เพื่อวางโครงเรื่องจริง)*"
	case strings.Contains(sys, "worldbuilder"):
		return "🌍 **โครงสร้างโลกและกฎเกณฑ์ (Worldbuilder)**\n\n" +
			"- **กฎหลักและราคาที่ต้องจ่าย**: ทุกพลังหรือกลไกในโลกควรมีข้อจำกัดและผลข้างเคียงที่จับต้องได้\n" +
			"- **ผลกระทบต่อสังคม**: ชนชั้นหรือองค์กรต่างๆ ปรับตัวต่อกฎนี้อย่างไรบ้าง\n\n" +
			"*(Demo Mode — เชื่อมต่อ API Key ใน Settings เพื่อสร้างและบันทึก Lore จริง)*"
	case strings.Contains(sys, "character-voice"):
		return "🎙️ **แนวทางน้ำเสียงและบทสนทนา (Character Voice)**\n\n" +
			"- ตัวละครที่ปิดบังความรู้สึกมักตอบคำถามด้วยคำถามกลับ หรือเปลี่ยนเรื่องไปที่วัตถุตรงหน้า\n" +
			"- ใช้ Subtext ให้สิ่งที่ *ไม่ได้พูด* ดังกว่าคำพูดจริง\n\n" +
			"*(Demo Mode — เชื่อมต่อ API Key ใน Settings เพื่อปรับบทสนทนาจริง)*"
	case strings.Contains(sys, "memory-curator"):
		return "🧠 **สรุปเหตุการณ์และอัปเดตคลังความจำ (Memory Curator)**\n\n" +
			"- บันทึกเหตุการณ์หลัก สถานะตัวละครล่าสุด และปมที่เปิดค้างไว้เรียบร้อยพร้อมใช้อ้างอิงในตอนถัดไป\n\n" +
			"*(Demo Mode — เชื่อมต่อ API Key ใน Settings เพื่อให้ AI สรุปและอัปเดตฐานความจำจริง)*"
	case strings.Contains(sys, "brainstorm-partner"):
		return "💡 **ทางเลือกและไอเดียต่อยอด (Brainstorm Partner)**\n\n" +
			"- **ทางเลือก A (แรงกดดันจากภายนอก)**: บุคคลที่สามโผล่มาขัดจังหวะพร้อมข้อมูลที่เปลี่ยนเกม\n" +
			"- **ทางเลือก B (แรงกดดันจากภายใน)**: ตัวละครเผลอทำพลาดเพราะจุดบอดของตัวเอง\n" +
			"- **ทางเลือก C (หักมุมเบาๆ)**: สิ่งที่ตามหามาตลอดถูกซ่อนไว้ในที่ที่ตัวละครมองข้ามเอง\n\n" +
			"*(Demo Mode — เชื่อมต่อ API Key ใน Settings เพื่อระดมสมองกับโมเดลจริง)*"
	case strings.Contains(user, "ตัวละคร"):
		return "🎭 **การออกแบบมิติเชิงลึกและเส้นโค้งแบบไม่เป็นเส้นตรง (Non-Linear Character Design)**\n\n" +
			"- **Core Paradox**: ตัวละครที่เชื่อว่าความเมตตาคือการไม่ผูกพัน แต่มือกลับซ่อมของพังของคนอื่นทุกคืน\n" +
			"- **Mask vs True Self**: หน้ากากที่เยือกเย็นด้วยเหตุผล ซ่อนความกลัวที่จะทำลายคนที่รักซ้ำรอยเดิม\n" +
			"- **Non-Linear Arc (Spiral / Relapse)**: แทนที่จะพัฒนาขึ้นเป็นเส้นตรง แนะนำให้เพิ่มช่วง *Relapse* ในองก์กลาง — เมื่อเบาะแสชี้ว่าเขามีส่วนเกี่ยวข้องจริง ตัวละครจะถอยกลับไปใช้นิสัยหนีความจริงชั่วคราวก่อนจะตัดสินใจเผชิญหน้าใหม่\n\n" +
			"*(หมายเหตุ: ขณะนี้กำลังใช้งาน Built-in Studio Demo — คุณสามารถเชื่อมต่อ API Key ของ OpenAI / Anthropic / Gemini / DeepSeek / Ollama ใน Settings เพื่อรันโมเดลจริงได้ทันที)*"
	default:
		return fmt.Sprintf("✍️ **Inkweave Studio (Demo Mode Response)**\n\n"+
			"เสียงฝนกระทบกระจกหน้าร้านถี่ขึ้นราวกับกำลังนับถอยหลัง คิรินวางเลนส์ขยายลงช้าๆ แล้วเช็ดปลายคีมด้วยผ้าสักหลาดทั้งที่มันไม่ได้เปื้อน\n\n"+
			"\"ถ้าผมเปิดกล่องนั้น\" เขาเอ่ยโดยไม่เงยหน้า \"คุณแน่ใจหรือว่าอยากได้ยินเสียงที่ซ่อนอยู่ข้างใน?\"\n\n"+
			"ลลิตาไม่ตอบทันที เธอเพียงเลื่อนกล่องดีบุกข้ามขอบโต๊ะมาอีกครึ่งนิ้ว — ระยะห่างเล็กน้อยที่บ่งบอกว่าวันนี้เธอไม่ได้มาเพื่อขอความช่วยเหลือ แต่มาทวงคำสัญญาที่ค้างคามานานเกินไปแล้ว\n\n"+
			"*(คำสั่งล่าสุด: %q — เชื่อมต่อ API ของคุณในเมนู ตั้งค่า > Multi-API Hub เพื่อเขียนและเรียกใช้ Tools แก้ไฟล์นิยายจริง)*",
			strings.TrimSpace(user))
	}
}

// Router resolves logical model slots (primary, utility, architect) from AppConfig + Secrets.
type Router struct {
	cfg *config.Manager
}

func NewRouter(cfg *config.Manager) *Router {
	return &Router{cfg: cfg}
}

func (r *Router) Resolve(role string) (Provider, string, error) {
	appCfg := r.cfg.GetConfig()
	var slot model.ModelSlot
	switch role {
	case "utility":
		slot = appCfg.Routing.Utility
	case "architect":
		slot = appCfg.Routing.Architect
	default:
		slot = appCfg.Routing.Primary
	}
	if slot.ProviderID == "" {
		slot = appCfg.Routing.Primary
	}
	prov, err := r.BuildProvider(slot.ProviderID)
	if err != nil {
		// Graceful fallback to primary, then mock
		if slot.ProviderID != appCfg.Routing.Primary.ProviderID && appCfg.Routing.Primary.ProviderID != "" {
			if p2, err2 := r.BuildProvider(appCfg.Routing.Primary.ProviderID); err2 == nil {
				m := appCfg.Routing.Primary.Model
				return p2, m, nil
			}
		}
		return NewMockProvider("mock-studio"), "inkweave-demo-v1", nil
	}
	mdl := slot.Model
	if mdl == "" {
		for _, pc := range appCfg.Providers {
			if pc.ID == slot.ProviderID {
				mdl = pc.DefaultModel
				break
			}
		}
	}
	return prov, mdl, nil
}

func (r *Router) BuildProvider(providerID string) (Provider, error) {
	appCfg := r.cfg.GetConfig()
	for _, pc := range appCfg.Providers {
		if pc.ID == providerID {
			if !pc.Enabled && pc.Kind != "mock" {
				return nil, fmt.Errorf("provider %s is disabled", providerID)
			}
			key := r.cfg.GetProviderKey(pc.ID)
			return NewProviderFromConfig(pc, key), nil
		}
	}
	return nil, fmt.Errorf("provider %s not found", providerID)
}

func NewProviderFromConfig(pc model.ProviderConfig, apiKey string) Provider {
	switch pc.Kind {
	case "anthropic":
		return NewAnthropicProvider(pc.ID, pc.BaseURL, apiKey)
	case "gemini":
		return NewGeminiProvider(pc.ID, pc.BaseURL, apiKey)
	case "mock":
		return NewMockProvider(pc.ID)
	default:
		return NewOpenAICompatProvider(pc.ID, pc.Kind, pc.BaseURL, apiKey)
	}
}
