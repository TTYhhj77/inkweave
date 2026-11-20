package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/inkweave/inkweave/internal/agent"
	"github.com/inkweave/inkweave/internal/config"
	"github.com/inkweave/inkweave/internal/exporter"
	"github.com/inkweave/inkweave/internal/gitsync"
	"github.com/inkweave/inkweave/internal/llm"
	"github.com/inkweave/inkweave/internal/memory"
	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/skills"
	"github.com/inkweave/inkweave/internal/storage"
	"github.com/inkweave/inkweave/internal/tools"
	"github.com/inkweave/inkweave/internal/webui"
)

const Version = "0.1.0"

type oauthPending struct {
	Verifier    string
	RedirectURI string
	CreatedAt   time.Time
}

// Engine encapsulates the full Inkweave runtime (shared between PC CLI and Android gomobile).
type Engine struct {
	Cfg       *config.Manager
	Workspace *storage.Workspace
	Proposals *tools.ProposalStore
	Orch      *agent.Orchestrator

	mu          sync.Mutex
	indexes     map[string]*memory.IndexDB
	oauthStates map[string]oauthPending

	httpServer *http.Server
	listener   net.Listener
	Port       int
	Token      string
	BaseURL    string
}

// StartEngine initializes config, workspace, auth token, and binds the HTTP+SSE server.
func StartEngine(host string, port int, dataDir string) (*Engine, error) {
	cfg, err := config.NewManager(dataDir)
	if err != nil {
		return nil, err
	}
	ws, err := storage.NewWorkspace(cfg.ProjectsDir())
	if err != nil {
		return nil, err
	}
	// Auto-seed starter Thai novel on brand new install so first launch isn't blank
	if projects, _ := ws.ListProjects(); len(projects) == 0 {
		_, _ = ws.CreateStarterNovel()
	}

	tokBytes := make([]byte, 24)
	_, _ = rand.Read(tokBytes)
	token := hex.EncodeToString(tokBytes)

	proposals := tools.NewProposalStore()
	eng := &Engine{
		Cfg:         cfg,
		Workspace:   ws,
		Proposals:   proposals,
		Orch:        agent.NewOrchestrator(cfg, proposals),
		indexes:     map[string]*memory.IndexDB{},
		oauthStates: map[string]oauthPending{},
		Token:       token,
	}

	if host == "" {
		host = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, err
	}
	tcpAddr := ln.Addr().(*net.TCPAddr)
	eng.listener = ln
	eng.Port = tcpAddr.Port
	displayHost := host
	if displayHost == "0.0.0.0" {
		displayHost = "127.0.0.1"
	}
	eng.BaseURL = fmt.Sprintf("http://%s:%d/?token=%s", displayHost, eng.Port, eng.Token)

	eng.httpServer = &http.Server{
		Handler:           eng.routes(),
		ReadHeaderTimeout: 15 * time.Second,
	}
	go func() {
		_ = eng.httpServer.Serve(ln)
	}()

	return eng, nil
}

func (e *Engine) Shutdown(ctx context.Context) error {
	var err error
	if e.httpServer != nil {
		err = e.httpServer.Shutdown(ctx)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, idx := range e.indexes {
		_ = idx.Close()
		delete(e.indexes, id)
	}
	return err
}

func (e *Engine) getProjectAndIndex(id string) (*storage.ProjectRepo, *memory.IndexDB, error) {
	repo, err := e.Workspace.OpenByID(id)
	if err != nil {
		return nil, nil, err
	}
	proj, err := repo.GetProject()
	if err != nil {
		return nil, nil, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if idx, ok := e.indexes[proj.ID]; ok {
		return repo, idx, nil
	}
	idx, err := memory.OpenIndex(repo.Dir)
	if err != nil {
		return repo, nil, err
	}
	_ = idx.ReindexProject(repo)
	e.indexes[proj.ID] = idx
	return repo, idx, nil
}

func (e *Engine) routes() http.Handler {
	mux := http.NewServeMux()
	uiHandler := webui.Handler()

	// Health (public local probe)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"version": Version,
			"port":    e.Port,
		})
	})

	// Settings & Multi-API Providers
	mux.HandleFunc("GET /api/settings", e.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", e.handleUpdateSettings)
	mux.HandleFunc("POST /api/providers", e.handleUpsertProvider)
	mux.HandleFunc("DELETE /api/providers/{id}", e.handleDeleteProvider)
	mux.HandleFunc("POST /api/providers/{id}/test", e.handleTestProvider)

	// Projects
	mux.HandleFunc("GET /api/projects", e.handleListProjects)
	mux.HandleFunc("POST /api/projects", e.handleCreateProject)
	mux.HandleFunc("POST /api/projects/starter", e.handleCreateStarter)
	mux.HandleFunc("POST /api/projects/clone", e.handleCloneProject)
	mux.HandleFunc("GET /api/projects/{id}", e.handleGetProject)
	mux.HandleFunc("PUT /api/projects/{id}", e.handleUpdateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", e.handleDeleteProject)

	// Outline
	mux.HandleFunc("GET /api/projects/{id}/outline", e.handleGetOutline)
	mux.HandleFunc("PUT /api/projects/{id}/outline", e.handleSaveOutline)

	// Chapters & Snapshots
	mux.HandleFunc("GET /api/projects/{id}/chapters", e.handleListChapters)
	mux.HandleFunc("POST /api/projects/{id}/chapters", e.handleSaveChapter)
	mux.HandleFunc("PUT /api/projects/{id}/chapters/{cid}", e.handleSaveChapter)
	mux.HandleFunc("DELETE /api/projects/{id}/chapters/{cid}", e.handleDeleteChapter)
	mux.HandleFunc("POST /api/projects/{id}/chapters/reorder", e.handleReorderChapters)
	mux.HandleFunc("GET /api/projects/{id}/chapters/{cid}/snapshots", e.handleListSnapshots)
	mux.HandleFunc("POST /api/projects/{id}/chapters/{cid}/snapshots", e.handleCreateSnapshot)
	mux.HandleFunc("POST /api/projects/{id}/chapters/{cid}/snapshots/{sid}/restore", e.handleRestoreSnapshot)

	// Characters (Deep Non-Linear Atelier)
	mux.HandleFunc("GET /api/projects/{id}/characters", e.handleListCharacters)
	mux.HandleFunc("POST /api/projects/{id}/characters", e.handleSaveCharacter)
	mux.HandleFunc("PUT /api/projects/{id}/characters/{cid}", e.handleSaveCharacter)
	mux.HandleFunc("DELETE /api/projects/{id}/characters/{cid}", e.handleDeleteCharacter)

	// Lore (locations | factions | items | rules)
	mux.HandleFunc("GET /api/projects/{id}/lore", e.handleListLore)
	mux.HandleFunc("POST /api/projects/{id}/lore", e.handleSaveLore)
	mux.HandleFunc("PUT /api/projects/{id}/lore/{lid}", e.handleSaveLore)
	mux.HandleFunc("DELETE /api/projects/{id}/lore/{category}/{lid}", e.handleDeleteLore)

	// Continuity Ledgers
	mux.HandleFunc("GET /api/projects/{id}/continuity/timeline", e.handleGetTimeline)
	mux.HandleFunc("PUT /api/projects/{id}/continuity/timeline", e.handleSaveTimeline)
	mux.HandleFunc("GET /api/projects/{id}/continuity/threads", e.handleGetThreads)
	mux.HandleFunc("PUT /api/projects/{id}/continuity/threads", e.handleSaveThreads)
	mux.HandleFunc("GET /api/projects/{id}/continuity/relationships", e.handleGetRelationships)
	mux.HandleFunc("PUT /api/projects/{id}/continuity/relationships", e.handleSaveRelationships)
	mux.HandleFunc("GET /api/projects/{id}/continuity/states", e.handleGetStates)
	mux.HandleFunc("PUT /api/projects/{id}/continuity/states", e.handleSaveStates)
	mux.HandleFunc("POST /api/projects/{id}/continuity/audit", e.handleAuditContinuity)

	// Skills
	mux.HandleFunc("GET /api/projects/{id}/skills", e.handleListSkills)
	mux.HandleFunc("POST /api/projects/{id}/skills", e.handleSaveSkill)
	mux.HandleFunc("DELETE /api/projects/{id}/skills/{sid}", e.handleDeleteSkill)

	// Memory Search & Context Inspector
	mux.HandleFunc("POST /api/projects/{id}/search", e.handleSearch)
	mux.HandleFunc("POST /api/projects/{id}/reindex", e.handleReindex)
	mux.HandleFunc("POST /api/projects/{id}/context/preview", e.handleContextPreview)

	// Agent Run (SSE), History, Proposals, Token Ledger
	mux.HandleFunc("POST /api/projects/{id}/agent/run", e.handleAgentRun)
	mux.HandleFunc("GET /api/projects/{id}/agent/history", e.handleAgentHistory)
	mux.HandleFunc("DELETE /api/projects/{id}/agent/history", e.handleClearAgentHistory)
	mux.HandleFunc("GET /api/projects/{id}/proposals", e.handleListProposals)
	mux.HandleFunc("POST /api/projects/{id}/proposals/{pid}/apply", e.handleApplyProposal)
	mux.HandleFunc("POST /api/projects/{id}/proposals/{pid}/reject", e.handleRejectProposal)
	mux.HandleFunc("GET /api/projects/{id}/tokens", e.handleListTokens)

	// Pure-Go Git Sync
	mux.HandleFunc("GET /api/projects/{id}/git/status", e.handleGitStatus)
	mux.HandleFunc("POST /api/projects/{id}/git/init", e.handleGitInit)
	mux.HandleFunc("POST /api/projects/{id}/git/remote", e.handleGitRemote)
	mux.HandleFunc("POST /api/projects/{id}/git/commit", e.handleGitCommit)
	mux.HandleFunc("POST /api/projects/{id}/git/pull", e.handleGitPull)
	mux.HandleFunc("POST /api/projects/{id}/git/push", e.handleGitPush)
	mux.HandleFunc("POST /api/projects/{id}/git/sync", e.handleGitSync)
	mux.HandleFunc("GET /api/projects/{id}/git/log", e.handleGitLog)
	mux.HandleFunc("POST /api/projects/{id}/git/resolve", e.handleGitResolve)
	mux.HandleFunc("POST /api/projects/{id}/git/restore", e.handleGitRestore)

	// Pure-Go Cloud Sync (Google Drive, WebDAV, Synced Folder)
	mux.HandleFunc("GET /api/cloud/config", e.handleGetCloudConfig)
	mux.HandleFunc("PUT /api/cloud/config", e.handleUpdateCloudConfig)
	mux.HandleFunc("POST /api/cloud/{provider}/test", e.handleTestCloudProvider)
	mux.HandleFunc("POST /api/oauth/gdrive/start", e.handleGDriveOAuthStart)
	mux.HandleFunc("GET /api/oauth/gdrive/callback", e.handleGDriveOAuthCallback)
	mux.HandleFunc("POST /api/oauth/gdrive/disconnect", e.handleGDriveDisconnect)
	mux.HandleFunc("GET /api/cloud/{provider}/novels", e.handleListCloudNovels)
	mux.HandleFunc("POST /api/cloud/{provider}/pull-novel", e.handlePullCloudNovel)
	mux.HandleFunc("GET /api/projects/{id}/cloud/{provider}/status", e.handleProjectCloudStatus)
	mux.HandleFunc("POST /api/projects/{id}/cloud/{provider}/sync", e.handleProjectCloudSync)
	mux.HandleFunc("POST /api/projects/{id}/cloud/{provider}/resolve", e.handleProjectCloudResolve)

	// Export
	mux.HandleFunc("GET /api/projects/{id}/export/markdown", e.handleExportMarkdown)
	mux.HandleFunc("GET /api/projects/{id}/export/epub", e.handleExportEPUB)

	// Static SPA fallback
	mux.Handle("/", uiHandler)

	return e.authMiddleware(mux)
}

func (e *Engine) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow local Vite dev server CORS cleanly
		if origin := r.Header.Get("Origin"); origin != "" &&
			(strings.HasPrefix(origin, "http://127.0.0.1") || strings.HasPrefix(origin, "http://localhost")) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// If landing on UI with ?token=..., set SameSite=Strict cookie for downloads/assets
		if qTok := r.URL.Query().Get("token"); qTok != "" && e.validToken(qTok) {
			http.SetCookie(w, &http.Cookie{
				Name:     "inkweave_token",
				Value:    e.Token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
		}

		if strings.HasPrefix(r.URL.Path, "/api/") &&
			r.URL.Path != "/api/health" &&
			r.URL.Path != "/api/oauth/gdrive/callback" {
			tok := extractToken(r)
			if !e.validToken(tok) {
				writeError(w, http.StatusUnauthorized, errors.New("unauthorized: missing or invalid session token"))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (e *Engine) validToken(candidate string) bool {
	if candidate == "" || e.Token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(e.Token)) == 1
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if q := r.URL.Query().Get("token"); q != "" {
		return q
	}
	if c, err := r.Cookie("inkweave_token"); err == nil {
		return c.Value
	}
	return ""
}

// --- Settings & Providers ---

func (e *Engine) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg := e.Cfg.GetConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"config":           cfg,
		"has_github_token": e.Cfg.GetGitHubToken() != "",
	})
}

func (e *Engine) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Config      *model.AppConfig `json:"config"`
		GitHubToken *string          `json:"github_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Config != nil {
		if err := e.Cfg.UpdateConfig(*body.Config); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if body.GitHubToken != nil {
		if err := e.Cfg.SetGitHubToken(*body.GitHubToken); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"config":           e.Cfg.GetConfig(),
		"has_github_token": e.Cfg.GetGitHubToken() != "",
	})
}

func (e *Engine) handleUpsertProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider model.ProviderConfig `json:"provider"`
		APIKey   string               `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Provider.ID == "" {
		body.Provider.ID = "prov-" + uuid.NewString()[:6]
	}
	if err := e.Cfg.UpsertProvider(body.Provider, body.APIKey); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, e.Cfg.GetConfig())
}

func (e *Engine) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := e.Cfg.DeleteProvider(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, e.Cfg.GetConfig())
}

func (e *Engine) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Model string `json:"model"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	router := llm.NewRouter(e.Cfg)
	prov, err := router.BuildProvider(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := prov.TestConnection(ctx, body.Model); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Projects ---

func (e *Engine) handleListProjects(w http.ResponseWriter, r *http.Request) {
	list, err := e.Workspace.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var p model.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	repo, err := e.Workspace.CreateProject(p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// Create initial chapter 1 so author can start typing immediately
	_ = repo.SaveChapter(&model.Chapter{
		ID:      "ch-001",
		Number:  1,
		Title:   "บทที่ 1",
		Status:  "draft",
		Content: "",
	})
	sum, _ := repo.Summary()
	writeJSON(w, http.StatusCreated, sum)
}

func (e *Engine) handleCreateStarter(w http.ResponseWriter, r *http.Request) {
	repo, err := e.Workspace.CreateStarterNovel()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	sum, _ := repo.Summary()
	writeJSON(w, http.StatusCreated, sum)
}

func (e *Engine) handleCloneProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL  string `json:"url"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.URL) == "" {
		writeError(w, http.StatusBadRequest, errors.New("repository URL is required"))
		return
	}
	slug := body.Slug
	if slug == "" {
		base := strings.TrimSuffix(filepath.Base(strings.TrimRight(body.URL, "/")), ".git")
		slug = storage.Slugify(base, "cloned-"+uuid.NewString()[:6])
	}
	dest := filepath.Join(e.Workspace.BaseDir(), slug)
	if err := gitsync.Clone(body.URL, dest, e.Cfg.GetGitHubToken()); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	repo, err := storage.OpenRepo(dest)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("cloned repository does not contain inkweave.yaml: %w", err))
		return
	}
	sum, _ := repo.Summary()
	writeJSON(w, http.StatusCreated, sum)
}

func (e *Engine) handleGetProject(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	sum, err := repo.Summary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (e *Engine) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var p model.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	existing, _ := repo.GetProject()
	p.ID = existing.ID
	p.CreatedAt = existing.CreatedAt
	if p.Slug == "" {
		p.Slug = existing.Slug
	}
	if err := repo.SaveProject(p); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	sum, _ := repo.Summary()
	writeJSON(w, http.StatusOK, sum)
}

func (e *Engine) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e.mu.Lock()
	if idx, ok := e.indexes[id]; ok {
		_ = idx.Close()
		delete(e.indexes, id)
	}
	e.mu.Unlock()
	if err := e.Workspace.DeleteProject(id); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --- Outline ---

func (e *Engine) handleGetOutline(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	o, err := repo.GetOutline()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (e *Engine) handleSaveOutline(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var o model.Outline
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := repo.SaveOutline(o); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// --- Chapters & Snapshots ---

func (e *Engine) handleListChapters(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	list, err := repo.ListChapters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleSaveChapter(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var ch model.Chapter
	if err := json.NewDecoder(r.Body).Decode(&ch); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cid := r.PathValue("cid"); cid != "" && ch.ID == "" {
		ch.ID = cid
	}
	if err := repo.SaveChapter(&ch); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, ch)
}

func (e *Engine) handleDeleteChapter(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := repo.DeleteChapter(r.PathValue("cid")); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (e *Engine) handleReorderChapters(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		OrderedIDs []string `json:"ordered_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := repo.ReorderChapters(body.OrderedIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	list, _ := repo.ListChapters()
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	snaps, err := repo.ListSnapshots(r.PathValue("cid"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, snaps)
}

func (e *Engine) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Reason == "" {
		body.Reason = "manual-checkpoint"
	}
	snap, err := repo.CreateSnapshot(r.PathValue("cid"), body.Reason)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusCreated, snap)
}

func (e *Engine) handleRestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	ch, err := repo.RestoreSnapshot(r.PathValue("cid"), r.PathValue("sid"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, ch)
}

// --- Characters ---

func (e *Engine) handleListCharacters(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	chars, err := repo.ListCharacters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, chars)
}

func (e *Engine) handleSaveCharacter(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var c model.Character
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cid := r.PathValue("cid"); cid != "" && c.ID == "" {
		c.ID = cid
	}
	if err := repo.SaveCharacter(&c); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, c)
}

func (e *Engine) handleDeleteCharacter(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := repo.DeleteCharacter(r.PathValue("cid")); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --- Lore ---

func (e *Engine) handleListLore(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	cat := r.URL.Query().Get("category")
	items, err := repo.ListLore(cat)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (e *Engine) handleSaveLore(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var item model.LoreEntry
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if lid := r.PathValue("lid"); lid != "" && item.ID == "" {
		item.ID = lid
	}
	if err := repo.SaveLore(&item); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, item)
}

func (e *Engine) handleDeleteLore(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := repo.DeleteLore(r.PathValue("category"), r.PathValue("lid")); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// --- Continuity ---

func (e *Engine) handleGetTimeline(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	list, _ := repo.GetTimeline()
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleSaveTimeline(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var list []model.TimelineEvent
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := repo.SaveTimeline(list); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleGetThreads(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	list, _ := repo.GetPlotThreads()
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleSaveThreads(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var list []model.PlotThread
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := repo.SavePlotThreads(list); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleGetRelationships(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	list, _ := repo.GetRelationships()
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleSaveRelationships(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var list []model.Relationship
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := repo.SaveRelationships(list); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleGetStates(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	list, _ := repo.GetCharacterStates()
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleSaveStates(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var list []model.CharacterState
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := repo.SaveCharacterStates(list); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleAuditContinuity(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	res, err := tools.Execute(tools.ExecContext{Repo: repo, Index: idx}, "audit_continuity", json.RawMessage(`{}`))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(res.Output))
}

// --- Skills ---

func (e *Engine) handleListSkills(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	list, err := skills.ListSkills(repo.Dir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleSaveSkill(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var sk model.Skill
	if err := json.NewDecoder(r.Body).Decode(&sk); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := skills.SaveCustomSkill(repo.Dir, sk); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	list, _ := skills.ListSkills(repo.Dir)
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := skills.DeleteCustomSkill(repo.Dir, r.PathValue("sid")); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	list, _ := skills.ListSkills(repo.Dir)
	writeJSON(w, http.StatusOK, list)
}

// --- Search & Context Inspector ---

func (e *Engine) handleSearch(w http.ResponseWriter, r *http.Request) {
	_, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Query string   `json:"query"`
		Kinds []string `json:"kinds"`
		Limit int      `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	hits, err := idx.Search(body.Query, body.Kinds, body.Limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, hits)
}

func (e *Engine) handleReindex(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := idx.ReindexProject(repo); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (e *Engine) handleContextPreview(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Mode         model.AgentMode `json:"mode"`
		SkillID      string          `json:"skill_id"`
		ChapterID    string          `json:"chapter_id"`
		Prompt       string          `json:"prompt"`
		SelectedText string          `json:"selected_text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	sel := agent.SelectSkillHeuristic(repo.Dir, body.SkillID, body.Prompt, body.SelectedText)
	sk := sel.Skill
	appCfg := e.Cfg.GetConfig()
	ctxData, err := memory.AssembleContext(repo, idx, memory.AssembleRequest{
		Mode:         body.Mode,
		Skill:        sk,
		ChapterID:    body.ChapterID,
		UserQuery:    body.Prompt,
		SelectedText: body.SelectedText,
		UltraMaxCap:  appCfg.UI.UltraBudgetCap,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, ctxData.Breakdown)
}

// --- Agent SSE Stream, History, Proposals, Tokens ---

func (e *Engine) handleAgentRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	repo, idx, err := e.getProjectAndIndex(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var req agent.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	proj, _ := repo.GetProject()
	req.ProjectID = proj.ID

	evCh, err := e.Orch.Run(r.Context(), repo, idx, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	for ev := range evCh {
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func (e *Engine) handleAgentHistory(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	proj, _ := repo.GetProject()
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = "default-" + proj.ID
	}
	msgs, err := idx.ListChatMessages(sessionID, 60)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (e *Engine) handleClearAgentHistory(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	proj, _ := repo.GetProject()
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		sessionID = "default-" + proj.ID
	}
	if idx != nil {
		if err := idx.ClearChatMessages(sessionID); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cleared": true})
}

func (e *Engine) handleListProposals(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	proj, _ := repo.GetProject()
	writeJSON(w, http.StatusOK, e.Proposals.ListByProject(proj.ID))
}

func (e *Engine) handleApplyProposal(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	pid := r.PathValue("pid")
	prop, ok := e.Proposals.Get(pid)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("proposal not found"))
		return
	}
	ch, err := tools.ApplyProposal(repo, idx, prop)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposal": prop, "chapter": ch})
}

func (e *Engine) handleRejectProposal(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("pid")
	prop, ok := e.Proposals.Get(pid)
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("proposal not found"))
		return
	}
	prop.Status = "rejected"
	writeJSON(w, http.StatusOK, prop)
}

func (e *Engine) handleListTokens(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	proj, _ := repo.GetProject()
	list, err := idx.ListTokenUsage(proj.ID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// --- Pure-Go Git Sync ---

func (e *Engine) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	st, err := gitsync.Status(repo.Dir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        st,
		"smart_message": gitsync.SmartCommitMessage(repo.Dir),
	})
}

func (e *Engine) handleGitInit(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := gitsync.Init(repo.Dir); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	st, _ := gitsync.Status(repo.Dir)
	writeJSON(w, http.StatusOK, st)
}

func (e *Engine) handleGitRemote(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := gitsync.Init(repo.Dir); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := gitsync.SetRemote(repo.Dir, body.URL); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	st, _ := gitsync.Status(repo.Dir)
	writeJSON(w, http.StatusOK, st)
}

func (e *Engine) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := gitsync.Init(repo.Dir); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cfg := e.Cfg.GetConfig()
	hash, err := gitsync.Commit(repo.Dir, body.Message, cfg.GitAuthor.Name, cfg.GitAuthor.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hash": hash})
}

func (e *Engine) handleGitPull(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	cfg := e.Cfg.GetConfig()
	res, err := gitsync.Pull(repo.Dir, e.Cfg.GetGitHubToken(), cfg.GitAuthor.Name, cfg.GitAuthor.Email)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, res)
}

func (e *Engine) handleGitPush(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := gitsync.Push(repo.Dir, e.Cfg.GetGitHubToken()); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (e *Engine) handleGitSync(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	cfg := e.Cfg.GetConfig()
	res, err := gitsync.Sync(repo.Dir, e.Cfg.GetGitHubToken(), body.Message, cfg.GitAuthor.Name, cfg.GitAuthor.Email)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, res)
}

func (e *Engine) handleGitLog(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	commits, err := gitsync.Log(repo.Dir, 30)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, commits)
}

func (e *Engine) handleGitResolve(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg := e.Cfg.GetConfig()
	hash, err := gitsync.ResolveConflict(repo.Dir, body.Path, body.Content, cfg.GitAuthor.Name, cfg.GitAuthor.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, map[string]any{"hash": hash})
}

func (e *Engine) handleGitRestore(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var body struct {
		Hash string `json:"hash"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	content, err := gitsync.FileAtCommit(repo.Dir, body.Hash, body.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	target := filepath.Join(repo.Dir, filepath.FromSlash(body.Path))
	if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(repo.Dir)) {
		writeError(w, http.StatusBadRequest, errors.New("invalid path"))
		return
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"restored": true})
}

// --- Export ---

func (e *Engine) handleExportMarkdown(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	proj, _ := repo.GetProject()
	includeBible := r.URL.Query().Get("bible") != "0"
	data, err := exporter.ExportMarkdown(repo, includeBible)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.md"`, proj.Slug))
	_, _ = w.Write(data)
}

func (e *Engine) handleExportEPUB(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	proj, _ := repo.GetProject()
	data, err := exporter.ExportEPUB(repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/epub+zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.epub"`, proj.Slug))
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
