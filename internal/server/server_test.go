package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEngineAuthAndEndToEndAPI(t *testing.T) {
	eng, err := StartEngine("127.0.0.1", 0, t.TempDir())
	if err != nil {
		t.Fatalf("StartEngine: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = eng.Shutdown(ctx)
	}()

	base := strings.Split(eng.BaseURL, "/?")[0]

	// Unauthenticated request to protected route must be rejected
	resp, err := http.Get(base + "/api/projects")
	if err != nil {
		t.Fatalf("GET /api/projects: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized, got %d", resp.StatusCode)
	}

	// Authenticated request succeeds and sees auto-seeded Thai starter novel
	req, _ := http.NewRequest(http.MethodGet, base+"/api/projects", nil)
	req.Header.Set("Authorization", "Bearer "+eng.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("auth GET /api/projects: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}

	var projects []map[string]any
	if err := json.Unmarshal(body, &projects); err != nil || len(projects) != 1 {
		t.Fatalf("expected 1 seeded starter project, got %s", string(body))
	}
	pid := projects[0]["id"].(string)

	// Verify Context Preview endpoint across modes
	prevReq, _ := http.NewRequest(http.MethodPost, base+"/api/projects/"+pid+"/context/preview",
		strings.NewReader(`{"mode":"balanced","skill_id":"character-architect","chapter_id":"ch-001","prompt":"วิเคราะห์คิริน"}`))
	prevReq.Header.Set("Authorization", "Bearer "+eng.Token)
	prevReq.Header.Set("Content-Type", "application/json")
	prevResp, err := http.DefaultClient.Do(prevReq)
	if err != nil || prevResp.StatusCode != http.StatusOK {
		t.Fatalf("context preview failed: %v status=%v", err, prevResp)
	}
	_ = prevResp.Body.Close()

	// Verify Cloud Sync config + Synced Folder end-to-end
	cloudTarget := t.TempDir()
	cfgPayload, _ := json.Marshal(map[string]any{
		"cloud_sync": map[string]any{
			"active_provider": "folder",
			"local_folder": map[string]any{
				"path":    cloudTarget,
				"enabled": true,
			},
		},
	})
	putReq, _ := http.NewRequest(http.MethodPut, base+"/api/cloud/config", strings.NewReader(string(cfgPayload)))
	putReq.Header.Set("Authorization", "Bearer "+eng.Token)
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil || putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/cloud/config failed: %v", err)
	}
	_ = putResp.Body.Close()

	syncReq, _ := http.NewRequest(http.MethodPost, base+"/api/projects/"+pid+"/cloud/folder/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+eng.Token)
	syncResp, err := http.DefaultClient.Do(syncReq)
	if err != nil || syncResp.StatusCode != http.StatusOK {
		t.Fatalf("POST cloud sync failed: %v status=%v", err, syncResp)
	}
	var syncRes map[string]any
	_ = json.NewDecoder(syncResp.Body).Decode(&syncRes)
	_ = syncResp.Body.Close()
	if syncRes["status"] != "ok" || int(syncRes["uploaded"].(float64)) < 5 {
		t.Fatalf("unexpected cloud sync response: %+v", syncRes)
	}
}
