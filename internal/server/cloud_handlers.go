package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/inkweave/inkweave/internal/cloudsync"
	"github.com/inkweave/inkweave/internal/model"
)

func (e *Engine) buildCloudProvider(kind string) (cloudsync.Provider, error) {
	cfg := e.Cfg.GetConfig()
	switch kind {
	case "gdrive":
		access, refresh, expiry := e.Cfg.GetGDriveTokens()
		secret := e.Cfg.GetGDriveClientSecret()
		if access == "" && refresh == "" {
			return nil, errors.New("Google Drive is not connected yet")
		}
		prov := cloudsync.NewGDriveProvider(
			cfg.CloudSync.GoogleDrive.ClientID,
			secret,
			cfg.CloudSync.GoogleDrive.RootFolder,
			access,
			refresh,
			expiry,
		)
		prov.OnTokenRefresh = func(newAccess, newRefresh string, newExp time.Time, email string) {
			_ = e.Cfg.SetGDriveTokens(newAccess, newRefresh, newExp, email)
		}
		return prov, nil

	case "webdav":
		pass := e.Cfg.GetWebDAVPassword()
		return cloudsync.NewWebDAVProvider(
			cfg.CloudSync.WebDAV.BaseURL,
			cfg.CloudSync.WebDAV.RootPath,
			cfg.CloudSync.WebDAV.Username,
			pass,
		), nil

	case "folder":
		return cloudsync.NewFolderProvider(cfg.CloudSync.LocalFolder.Path), nil

	default:
		return nil, fmt.Errorf("unknown cloud sync provider: %s", kind)
	}
}

func (e *Engine) gdriveRedirectURI(r *http.Request) string {
	host := r.Host
	if host == "" {
		host = fmt.Sprintf("127.0.0.1:%d", e.Port)
	}
	return fmt.Sprintf("http://%s/api/oauth/gdrive/callback", host)
}

func (e *Engine) handleGetCloudConfig(w http.ResponseWriter, r *http.Request) {
	cfg := e.Cfg.GetConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"cloud_sync":          cfg.CloudSync,
		"gdrive_redirect_uri": e.gdriveRedirectURI(r),
		"has_gdrive_secret":   e.Cfg.GetGDriveClientSecret() != "",
	})
}

func (e *Engine) handleUpdateCloudConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CloudSync          *model.CloudSyncConfig `json:"cloud_sync"`
		GDriveClientSecret *string                `json:"gdrive_client_secret"`
		GDriveAccessToken  *string                `json:"gdrive_access_token"`
		WebDAVPassword     *string                `json:"webdav_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.CloudSync != nil {
		cfg := e.Cfg.GetConfig()
		cfg.CloudSync = *body.CloudSync
		if err := e.Cfg.UpdateConfig(cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if body.GDriveClientSecret != nil {
		if err := e.Cfg.SetGDriveClientSecret(*body.GDriveClientSecret); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if body.GDriveAccessToken != nil && strings.TrimSpace(*body.GDriveAccessToken) != "" {
		if err := e.Cfg.SetGDriveTokens(strings.TrimSpace(*body.GDriveAccessToken), "", time.Time{}, ""); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if body.WebDAVPassword != nil && *body.WebDAVPassword != "" {
		if err := e.Cfg.SetWebDAVPassword(*body.WebDAVPassword); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	cfg := e.Cfg.GetConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"cloud_sync":          cfg.CloudSync,
		"gdrive_redirect_uri": e.gdriveRedirectURI(r),
		"has_gdrive_secret":   e.Cfg.GetGDriveClientSecret() != "",
	})
}

func (e *Engine) handleTestCloudProvider(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("provider")
	prov, err := e.buildCloudProvider(kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	detail, err := prov.TestConnection(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if kind == "gdrive" && detail != "" && strings.Contains(detail, "@") {
		access, refresh, exp := e.Cfg.GetGDriveTokens()
		_ = e.Cfg.SetGDriveTokens(access, refresh, exp, detail)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "detail": detail})
}

func (e *Engine) handleGDriveOAuthStart(w http.ResponseWriter, r *http.Request) {
	cfg := e.Cfg.GetConfig()
	clientID := strings.TrimSpace(cfg.CloudSync.GoogleDrive.ClientID)
	if clientID == "" {
		writeError(w, http.StatusBadRequest, errors.New("กรุณาใส่ Google OAuth Client ID และบันทึกก่อนเชื่อมต่อ"))
		return
	}
	verifier, challenge, err := cloudsync.GeneratePKCE()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	stateBytes := make([]byte, 16)
	_, _ = rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)
	redirectURI := e.gdriveRedirectURI(r)

	e.mu.Lock()
	// Prune stale states older than 10 minutes
	cutoff := time.Now().Add(-10 * time.Minute)
	for k, v := range e.oauthStates {
		if v.CreatedAt.Before(cutoff) {
			delete(e.oauthStates, k)
		}
	}
	e.oauthStates[state] = oauthPending{
		Verifier:    verifier,
		RedirectURI: redirectURI,
		CreatedAt:   time.Now(),
	}
	e.mu.Unlock()

	authURL := cloudsync.BuildOAuthURL(clientID, redirectURI, state, challenge)
	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url":     authURL,
		"redirect_uri": redirectURI,
	})
}

func (e *Engine) handleGDriveOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		renderOAuthHTML(w, false, "Google Drive authorization canceled: "+errParam)
		return
	}
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		renderOAuthHTML(w, false, "Missing state or authorization code.")
		return
	}

	e.mu.Lock()
	pending, ok := e.oauthStates[state]
	if ok {
		delete(e.oauthStates, state)
	}
	e.mu.Unlock()
	if !ok {
		renderOAuthHTML(w, false, "OAuth state expired or invalid. Please try connecting again.")
		return
	}

	cfg := e.Cfg.GetConfig()
	res, err := cloudsync.ExchangeAuthCode(
		r.Context(),
		nil,
		"",
		"",
		cfg.CloudSync.GoogleDrive.ClientID,
		e.Cfg.GetGDriveClientSecret(),
		code,
		pending.RedirectURI,
		pending.Verifier,
	)
	if err != nil {
		renderOAuthHTML(w, false, "Token exchange failed: "+err.Error())
		return
	}
	if err := e.Cfg.SetGDriveTokens(res.AccessToken, res.RefreshToken, res.Expiry, res.Email); err != nil {
		renderOAuthHTML(w, false, "Failed to save credentials locally: "+err.Error())
		return
	}
	msg := "เชื่อมต่อ Google Drive สำเร็จแล้ว!"
	if res.Email != "" {
		msg += " (" + res.Email + ")"
	}
	renderOAuthHTML(w, true, msg)
}

func renderOAuthHTML(w http.ResponseWriter, ok bool, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	statusText := "Inkweave Google Drive Sync"
	color := "#3B7A57"
	if !ok {
		color = "#A63A3A"
	}
	script := ""
	if ok {
		script = `<script>if (window.opener) { window.opener.postMessage({ type: 'inkweave-gdrive-connected' }, '*'); setTimeout(() => window.close(), 1200); }</script>`
	}
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="th"><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:system-ui,sans-serif;background:#FAF8F4;color:#1E1D1B;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
.card{background:#FFF;border:1px solid #E2DACD;border-radius:16px;padding:28px 32px;max-width:420px;text-align:center}
h2{margin:0 0 10px 0;color:%s}p{color:#6E675E;line-height:1.6}</style></head>
<body><div class="card"><h2>%s</h2><p>%s</p><p><small>คุณสามารถปิดหน้าต่างนี้แล้วกลับไปที่ Inkweave Studio ได้เลย</small></p></div>%s</body></html>`,
		html.EscapeString(statusText), color, html.EscapeString(statusText), html.EscapeString(message), script)
}

func (e *Engine) handleGDriveDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := e.Cfg.ClearGDriveTokens(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (e *Engine) handleListCloudNovels(w http.ResponseWriter, r *http.Request) {
	prov, err := e.buildCloudProvider(r.PathValue("provider"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	list, err := prov.ListRemoteNovels(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (e *Engine) handlePullCloudNovel(w http.ResponseWriter, r *http.Request) {
	prov, err := e.buildCloudProvider(r.PathValue("provider"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Slug) == "" {
		writeError(w, http.StatusBadRequest, errors.New("novel slug is required"))
		return
	}
	repo, err := cloudsync.PullRemoteNovel(r.Context(), e.Workspace, prov, body.Slug)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	sum, _ := repo.Summary()
	writeJSON(w, http.StatusCreated, sum)
}

func (e *Engine) handleProjectCloudStatus(w http.ResponseWriter, r *http.Request) {
	repo, _, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	st, err := cloudsync.ProjectSyncStatus(repo, r.PathValue("provider"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (e *Engine) handleProjectCloudSync(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	prov, err := e.buildCloudProvider(r.PathValue("provider"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	res, err := cloudsync.SyncProject(r.Context(), repo, prov)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, res)
}

func (e *Engine) handleProjectCloudResolve(w http.ResponseWriter, r *http.Request) {
	repo, idx, err := e.getProjectAndIndex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	prov, err := e.buildCloudProvider(r.PathValue("provider"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
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
	if err := cloudsync.ResolveConflict(r.Context(), repo, prov, body.Path, body.Content); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if idx != nil {
		_ = idx.ReindexProject(repo)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"resolved": true})
}
