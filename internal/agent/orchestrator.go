package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/inkweave/inkweave/internal/config"
	"github.com/inkweave/inkweave/internal/llm"
	"github.com/inkweave/inkweave/internal/memory"
	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/storage"
	"github.com/inkweave/inkweave/internal/tools"
)

// RunRequest defines an author turn to execute inside a novel project.
type RunRequest struct {
	ProjectID    string          `json:"project_id"`
	SessionID    string          `json:"session_id"`
	ChapterID    string          `json:"chapter_id"`
	SkillID      string          `json:"skill_id"`
	Mode         model.AgentMode `json:"mode"`
	Prompt       string          `json:"prompt"`
	SelectedText string          `json:"selected_text"`
}

// Event is streamed over SSE to the Literary Studio UI.
type Event struct {
	Type          string                  `json:"type"` // skill_selected | stage | context_summary | token_delta | thinking_delta | tool_start | tool_result | proposal | usage | done | error
	Stage         string                  `json:"stage,omitempty"`
	Label         string                  `json:"label,omitempty"`
	SkillID       string                  `json:"skill_id,omitempty"`
	SkillName     string                  `json:"skill_name,omitempty"`
	SkillNameTH   string                  `json:"skill_name_th,omitempty"`
	AutoPicked    bool                    `json:"auto_picked,omitempty"`
	RouteMethod   string                  `json:"route_method,omitempty"`
	Delta         string                  `json:"delta,omitempty"`
	ThinkingDelta string                  `json:"thinking_delta,omitempty"`
	ToolName      string                  `json:"tool_name,omitempty"`
	ToolArgs      string                  `json:"tool_args,omitempty"`
	ToolOutput    string                  `json:"tool_output,omitempty"`
	Context       *model.ContextBreakdown `json:"context,omitempty"`
	Proposal      *model.Proposal         `json:"proposal,omitempty"`
	Usage         *llm.Usage              `json:"usage,omitempty"`
	Error         string                  `json:"error,omitempty"`
	FinalText     string                  `json:"final_text,omitempty"`
}

// Orchestrator executes Eco / Balanced / Ultra agent runs against a ProjectRepo.
type Orchestrator struct {
	Cfg       *config.Manager
	Router    *llm.Router
	Proposals *tools.ProposalStore
}

func NewOrchestrator(cfg *config.Manager, proposals *tools.ProposalStore) *Orchestrator {
	return &Orchestrator{
		Cfg:       cfg,
		Router:    llm.NewRouter(cfg),
		Proposals: proposals,
	}
}

// Run executes the agent pipeline and streams Event objects until done.
func (o *Orchestrator) Run(ctx context.Context, repo *storage.ProjectRepo, idx *memory.IndexDB, req RunRequest) (<-chan Event, error) {
	if req.SessionID == "" {
		req.SessionID = "default-" + req.ProjectID
	}
	appCfg := o.Cfg.GetConfig()
	if req.Mode == "" {
		req.Mode = appCfg.UI.DefaultMode
	}
	if req.SkillID == "" {
		req.SkillID = AutoSkillID
	}

	selection := SelectSkill(ctx, repo.Dir, o.Router, req.Mode, req.SkillID, req.Prompt, req.SelectedText)
	sk := selection.Skill
	if sk != nil {
		req.SkillID = sk.ID
	}
	assembled, err := memory.AssembleContext(repo, idx, memory.AssembleRequest{
		Mode:         req.Mode,
		Skill:        sk,
		ChapterID:    req.ChapterID,
		UserQuery:    req.Prompt,
		SelectedText: req.SelectedText,
		UltraMaxCap:  appCfg.UI.UltraBudgetCap,
	})
	if err != nil {
		return nil, err
	}

	// Save user turn in local chat history
	if idx != nil {
		_ = idx.SaveChatMessage(model.ChatMessage{
			ID:        uuid.NewString(),
			SessionID: req.SessionID,
			ChapterID: req.ChapterID,
			Role:      "user",
			Content:   req.Prompt,
			Mode:      req.Mode,
			CreatedAt: time.Now().UTC(),
		})
	}

	out := make(chan Event, 64)
	go func() {
		defer close(out)

		if sk != nil {
			out <- Event{
				Type:        "skill_selected",
				SkillID:     sk.ID,
				SkillName:   sk.Name,
				SkillNameTH: sk.NameTH,
				AutoPicked:  selection.AutoPicked,
				RouteMethod: selection.Method,
			}
		}

		spec := SpecForMode(req.Mode, appCfg.UI.UltraBudgetCap)
		out <- Event{
			Type:    "context_summary",
			Context: &assembled.Breakdown,
		}

		toolCtx := tools.ExecContext{
			ProjectID:    req.ProjectID,
			SessionID:    req.SessionID,
			Repo:         repo,
			Index:        idx,
			Proposals:    o.Proposals,
			ApprovalMode: appCfg.UI.ApprovalMode,
		}

		var allowedTools []string
		if sk != nil {
			allowedTools = sk.Tools
		}

		var finalAnswer string
		var toolEventsLog []map[string]string
		var runErr error

		if req.Mode == model.ModeUltra {
			finalAnswer, toolEventsLog, runErr = o.runUltra(ctx, out, idx, req, assembled, spec, toolCtx, allowedTools)
		} else {
			finalAnswer, toolEventsLog, runErr = o.runLoop(ctx, out, idx, req, assembled, spec, toolCtx, allowedTools)
		}

		if runErr != nil {
			out <- Event{Type: "error", Error: runErr.Error()}
			return
		}

		if idx != nil {
			evJSON, _ := json.Marshal(toolEventsLog)
			_ = idx.SaveChatMessage(model.ChatMessage{
				ID:         uuid.NewString(),
				SessionID:  req.SessionID,
				ChapterID:  req.ChapterID,
				Role:       "assistant",
				Content:    finalAnswer,
				ToolEvents: string(evJSON),
				Mode:       req.Mode,
				CreatedAt:  time.Now().UTC(),
			})
		}

		out <- Event{
			Type:      "done",
			FinalText: finalAnswer,
		}
	}()

	return out, nil
}

func (o *Orchestrator) runLoop(
	ctx context.Context,
	out chan<- Event,
	idx *memory.IndexDB,
	req RunRequest,
	assembled *memory.AssembledContext,
	spec ModeSpec,
	toolCtx tools.ExecContext,
	allowedTools []string,
) (string, []map[string]string, error) {
	stageLabel := "Drafting & Tools"
	if req.Mode == model.ModeEco {
		stageLabel = "Eco Direct Pass"
	}
	out <- Event{Type: "stage", Stage: string(req.Mode), Label: stageLabel}

	prov, mdl, err := o.Router.Resolve(spec.PrimaryRole)
	if err != nil {
		return "", nil, err
	}

	toolDefs := tools.Definitions(allowedTools, false)
	if req.Mode == model.ModeEco {
		// Eco keeps tool schemas compact (read-only + primary draft/edit)
		toolDefs = tools.Definitions(allowedTools, false)
		if len(toolDefs) > 5 {
			toolDefs = toolDefs[:5]
		}
	}

	msgs := o.loadRecentHistory(idx, req.SessionID, 6)
	msgs = append(msgs, llm.Message{Role: "user", Content: req.Prompt})

	return o.executeToolLoop(ctx, out, idx, req, prov, mdl, assembled.SystemPrefix, assembled.DynamicContext, msgs, toolDefs, spec, toolCtx, string(req.Mode))
}

func (o *Orchestrator) runUltra(
	ctx context.Context,
	out chan<- Event,
	idx *memory.IndexDB,
	req RunRequest,
	assembled *memory.AssembledContext,
	spec ModeSpec,
	toolCtx tools.ExecContext,
	allowedTools []string,
) (string, []map[string]string, error) {
	var allToolLogs []map[string]string

	// Stage 1: Architect (Reasoning + Read-only Deep Recall)
	out <- Event{Type: "stage", Stage: "architect", Label: "1/4 Architect — Scene & Character Depth Plan"}
	archProv, archModel, err := o.Router.Resolve("architect")
	if err != nil {
		return "", nil, err
	}
	readDefs := tools.Definitions(nil, true)
	archSys := assembled.SystemPrefix + "\n\n[ARCHITECT_STAGE]\nAnalyze the author's request, character paradoxes, subtext, knowledge boundaries, and outline beats. Call read-only tools if needed, then emit a concise Scene & Continuity Beat Plan."
	archPlan, logs1, err := o.executeToolLoop(ctx, out, idx, req, archProv, archModel, archSys, assembled.DynamicContext,
		[]llm.Message{{Role: "user", Content: req.Prompt}}, readDefs, ModeSpec{
			Mode: spec.Mode, MaxToolSteps: 4, EnableCache: true,
		}, toolCtx, "architect")
	if err != nil {
		return "", nil, err
	}
	allToolLogs = append(allToolLogs, logs1...)

	// Stage 2: Writer (Primary Prose Generation)
	out <- Event{Type: "stage", Stage: "writer", Label: "2/4 Writer — Literary Drafting"}
	writerProv, writerModel, err := o.Router.Resolve("primary")
	if err != nil {
		return "", nil, err
	}
	writeDefs := tools.Definitions(allowedTools, false)
	writerMsgs := o.loadRecentHistory(idx, req.SessionID, 4)
	writerMsgs = append(writerMsgs, llm.Message{
		Role:    "user",
		Content: fmt.Sprintf("Author Request:\n%s\n\nApproved Architect Beat Plan:\n%s\n\nWrite the response/prose now with zero AI clichés. Use write tools if proposing chapter edits.", req.Prompt, archPlan),
	})
	draftText, logs2, err := o.executeToolLoop(ctx, out, idx, req, writerProv, writerModel, assembled.SystemPrefix, assembled.DynamicContext,
		writerMsgs, writeDefs, spec, toolCtx, "writer")
	if err != nil {
		return "", nil, err
	}
	allToolLogs = append(allToolLogs, logs2...)

	// Stage 3: Continuity & Anti-Slop Critic
	out <- Event{Type: "stage", Stage: "critic", Label: "3/4 Critic — Continuity, Character & Anti-Slop Audit"}
	criticSys := assembled.SystemPrefix + "\n\n[CONTINUITY_CRITIC_STAGE]\nAudit the draft below strictly against Knowledge Boundaries (UnawareOf), Character Paradoxes/Voices, Timeline, and Forbidden AI Clichés. Provide concise surgical corrections if needed."
	criticOut, _, err := o.executeToolLoop(ctx, out, idx, req, archProv, archModel, criticSys, assembled.DynamicContext,
		[]llm.Message{{Role: "user", Content: "Draft to audit:\n" + draftText}}, nil, ModeSpec{
			Mode: spec.Mode, MaxToolSteps: 0, EnableCache: true,
		}, toolCtx, "critic")
	if err != nil {
		return "", nil, err
	}

	// Stage 4: Polish Summary
	out <- Event{Type: "stage", Stage: "polish", Label: "4/4 Ready — Synthesized Output"}
	combined := draftText
	if strings.TrimSpace(criticOut) != "" && !strings.Contains(draftText, criticOut) {
		combined = draftText + "\n\n---\n" + criticOut
	}
	return combined, allToolLogs, nil
}

func (o *Orchestrator) executeToolLoop(
	ctx context.Context,
	out chan<- Event,
	idx *memory.IndexDB,
	req RunRequest,
	prov llm.Provider,
	mdl string,
	systemPrefix string,
	dynamicContext string,
	msgs []llm.Message,
	toolDefs []llm.ToolDef,
	spec ModeSpec,
	toolCtx tools.ExecContext,
	stageName string,
) (string, []map[string]string, error) {
	var fullText strings.Builder
	var toolLogs []map[string]string

	for step := 0; step <= spec.MaxToolSteps; step++ {
		if ctx.Err() != nil {
			return "", toolLogs, ctx.Err()
		}
		evCh, err := prov.StreamChat(ctx, llm.ChatRequest{
			Model:          mdl,
			SystemPrefix:   systemPrefix,
			DynamicContext: dynamicContext,
			Messages:       msgs,
			Tools:          toolDefs,
			EnableCache:    spec.EnableCache,
		})
		if err != nil {
			return "", toolLogs, err
		}

		var turnText strings.Builder
		var pendingCalls []llm.ToolCall
		for ev := range evCh {
			switch ev.Type {
			case "delta":
				turnText.WriteString(ev.Delta)
				if stageName != "architect" {
					out <- Event{Type: "token_delta", Delta: ev.Delta, Stage: stageName}
				}
			case "thinking":
				out <- Event{Type: "thinking_delta", ThinkingDelta: ev.ThinkingDelta, Stage: stageName}
			case "tool_call":
				if ev.ToolCall != nil {
					pendingCalls = append(pendingCalls, *ev.ToolCall)
				}
			case "usage":
				if ev.Usage != nil {
					out <- Event{Type: "usage", Usage: ev.Usage, Stage: stageName}
					if idx != nil {
						_ = idx.RecordTokenUsage(model.TokenLedgerEntry{
							ProjectID:    req.ProjectID,
							Mode:         req.Mode,
							Provider:     prov.ID(),
							Model:        mdl,
							Stage:        stageName,
							InputTokens:  ev.Usage.InputTokens,
							CachedTokens: ev.Usage.CachedTokens,
							OutputTokens: ev.Usage.OutputTokens,
						})
					}
				}
			case "error":
				return "", toolLogs, fmt.Errorf("%s", ev.Error)
			}
		}

		if turnText.Len() > 0 {
			if fullText.Len() > 0 {
				fullText.WriteString("\n\n")
			}
			fullText.WriteString(turnText.String())
		}

		if len(pendingCalls) == 0 || step == spec.MaxToolSteps {
			break
		}

		msgs = append(msgs, llm.Message{
			Role:      "assistant",
			Content:   turnText.String(),
			ToolCalls: pendingCalls,
		})

		for _, tc := range pendingCalls {
			out <- Event{
				Type:     "tool_start",
				ToolName: tc.Name,
				ToolArgs: string(tc.Arguments),
				Stage:    stageName,
			}
			res, tErr := tools.Execute(toolCtx, tc.Name, tc.Arguments)
			outputStr := res.Output
			if tErr != nil {
				outputStr = fmt.Sprintf(`{"error":%q}`, tErr.Error())
			}
			out <- Event{
				Type:       "tool_result",
				ToolName:   tc.Name,
				ToolOutput: outputStr,
				Stage:      stageName,
			}
			if res.Proposal != nil {
				out <- Event{
					Type:     "proposal",
					Proposal: res.Proposal,
					Stage:    stageName,
				}
			}
			toolLogs = append(toolLogs, map[string]string{
				"tool":   tc.Name,
				"args":   string(tc.Arguments),
				"output": outputStr,
			})
			msgs = append(msgs, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Content:    outputStr,
			})
		}
	}

	return fullText.String(), toolLogs, nil
}

func (o *Orchestrator) loadRecentHistory(idx *memory.IndexDB, sessionID string, maxTurns int) []llm.Message {
	if idx == nil {
		return nil
	}
	msgs, err := idx.ListChatMessages(sessionID, maxTurns+1)
	if err != nil || len(msgs) <= 1 {
		return nil
	}
	// Exclude the user message we just saved right before running
	prior := msgs[:len(msgs)-1]
	var out []llm.Message
	for _, m := range prior {
		out = append(out, llm.Message{
			Role:    m.Role,
			Content: memory.TrimToTokens(m.Content, 350),
		})
	}
	return out
}
