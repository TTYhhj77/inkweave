package cloudsync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/inkweave/inkweave/internal/storage"
)

func TestFolderThreeWaySyncAndConflictResolution(t *testing.T) {
	ctx := context.Background()
	cloudDir := filepath.Join(t.TempDir(), "onedrive-inkweave")
	prov := NewFolderProvider(cloudDir)
	if _, err := prov.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	// Device A creates starter Thai novel & syncs to cloud folder
	wsA, _ := storage.NewWorkspace(t.TempDir())
	repoA, err := wsA.CreateStarterNovel()
	if err != nil {
		t.Fatalf("CreateStarterNovel: %v", err)
	}
	resA, err := SyncProject(ctx, repoA, prov)
	if err != nil {
		t.Fatalf("SyncProject A: %v", err)
	}
	if resA.Status != "ok" || resA.Uploaded < 5 {
		t.Fatalf("expected initial upload >=5 files, got %+v", resA)
	}

	// Verify ListRemoteNovels discovers it
	novels, err := prov.ListRemoteNovels(ctx)
	if err != nil || len(novels) != 1 || novels[0].Slug != "shadows-of-glass-harbor" {
		t.Fatalf("unexpected remote novels: %+v err=%v", novels, err)
	}

	// Device B pulls the novel from cloud folder
	wsB, _ := storage.NewWorkspace(t.TempDir())
	repoB, err := PullRemoteNovel(ctx, wsB, prov, novels[0].Slug)
	if err != nil {
		t.Fatalf("PullRemoteNovel: %v", err)
	}
	chB, err := repoB.GetChapter("ch-001")
	if err != nil || !strings.Contains(chB.Content, "คิริน") {
		t.Fatalf("pulled chapter mismatch: %v", err)
	}

	// Device A and Device B make diverged edits to chapter 1 -> triggers 3-way conflict
	chA, _ := repoA.GetChapter("ch-001")
	chA.Content += "\n\nฉบับแก้ไขจาก PC"
	_ = repoA.SaveChapter(chA)
	if _, err := SyncProject(ctx, repoA, prov); err != nil {
		t.Fatalf("Sync A push: %v", err)
	}

	chB.Content += "\n\nฉบับแก้ไขจาก Android"
	_ = repoB.SaveChapter(chB)
	resB, err := SyncProject(ctx, repoB, prov)
	if err != nil {
		t.Fatalf("Sync B diverged: %v", err)
	}
	if resB.Status != "conflicts" || len(resB.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict on diverged chapter, got %+v", resB)
	}
	if resB.Conflicts[0].Base == "" || !strings.Contains(resB.Conflicts[0].Remote, "PC") {
		t.Fatalf("expected Base and Remote text populated in conflict: %+v", resB.Conflicts[0])
	}

	// Resolve conflict on Device B and re-sync Device A cleanly
	merged := resB.Conflicts[0].Local + "\n\nรวมฉบับ PC + Android"
	if err := ResolveConflict(ctx, repoB, prov, resB.Conflicts[0].Path, merged); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	resA2, err := SyncProject(ctx, repoA, prov)
	if err != nil {
		t.Fatalf("Sync A pull resolved: %v", err)
	}
	if resA2.Downloaded < 1 {
		t.Fatalf("expected A to download resolved chapter, got %+v", resA2)
	}
	chAFinal, _ := repoA.GetChapter("ch-001")
	if !strings.Contains(chAFinal.Content, "รวมฉบับ PC + Android") {
		t.Fatalf("resolved content not propagated to Device A")
	}
	snaps, _ := repoA.ListSnapshots("ch-001")
	if len(snaps) == 0 {
		t.Fatalf("expected automatic snapshot before remote pull overwrite")
	}
}

func TestWebDAVSyncServer(t *testing.T) {
	var mu sync.Mutex
	files := map[string][]byte{}
	dirs := map[string]bool{"/dav/Inkweave": true}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "writer" || p != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()

		clean := "/" + strings.Trim(r.URL.Path, "/")
		switch r.Method {
		case "MKCOL":
			if dirs[clean] {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			dirs[clean] = true
			w.WriteHeader(http.StatusCreated)
		case "PROPFIND":
			depth := r.Header.Get("Depth")
			var respXML strings.Builder
			respXML.WriteString(`<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:">`)
			if depth == "0" {
				fmt.Fprintf(&respXML, `<D:response><D:href>%s/</D:href><D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop></D:propstat></D:response>`, clean)
			} else {
				for d := range dirs {
					if strings.HasPrefix(d, clean+"/") {
						rest := strings.TrimPrefix(d, clean+"/")
						if !strings.Contains(rest, "/") {
							fmt.Fprintf(&respXML, `<D:response><D:href>%s/</D:href><D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop></D:propstat></D:response>`, d)
						}
					}
				}
			}
			respXML.WriteString(`</D:multistatus>`)
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(respXML.String()))
		case http.MethodPut:
			data, _ := io.ReadAll(r.Body)
			files[clean] = data
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			data, exists := files[clean]
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(data)
		case http.MethodDelete:
			delete(files, clean)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	prov := NewWebDAVProvider(srv.URL+"/dav", "/Inkweave", "writer", "secret")
	if _, err := prov.TestConnection(ctx); err != nil {
		t.Fatalf("WebDAV TestConnection: %v", err)
	}

	ws, _ := storage.NewWorkspace(t.TempDir())
	repo, _ := ws.CreateStarterNovel()
	res, err := SyncProject(ctx, repo, prov)
	if err != nil {
		t.Fatalf("WebDAV SyncProject: %v", err)
	}
	if res.Status != "ok" || res.Uploaded < 5 {
		t.Fatalf("unexpected WebDAV sync result: %+v", res)
	}

	novels, err := prov.ListRemoteNovels(ctx)
	if err != nil || len(novels) != 1 {
		t.Fatalf("WebDAV ListRemoteNovels failed: %+v err=%v", novels, err)
	}
}

type mockDriveNode struct {
	ID       string
	Name     string
	MimeType string
	ParentID string
	Data     []byte
	Trashed  bool
}

func TestGoogleDriveOAuthRefreshAndTwoDeviceSync(t *testing.T) {
	var mu sync.Mutex
	nodes := map[string]*mockDriveNode{}
	nextID := 1
	accessToken := "initial-tok"
	refreshedCalled := false

	newID := func() string {
		nextID++
		return fmt.Sprintf("drv-%d", nextID)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			mu.Lock()
			accessToken = "refreshed-tok"
			refreshedCalled = true
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "refreshed-tok",
				"refresh_token": "ref-1",
				"expires_in":    3600,
			})
			return
		}

		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		case r.URL.Path == "/drive/v3/about":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user": map[string]string{
					"emailAddress": "author@gmail.com",
					"displayName":  "Thai Novelist",
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/drive/v3/files":
			q := r.URL.Query().Get("q")
			var matches []map[string]string
			for _, n := range nodes {
				if n.Trashed {
					continue
				}
				if strings.Contains(q, "name='") {
					start := strings.Index(q, "name='") + 6
					end := strings.Index(q[start:], "'")
					wantName := q[start : start+end]
					if n.Name != wantName {
						continue
					}
				}
				if strings.Contains(q, "' in parents") {
					idx := strings.Index(q, "' in parents")
					sub := q[:idx]
					lastQ := strings.LastIndex(sub, "'")
					wantParent := sub[lastQ+1:]
					if wantParent == "root" {
						if n.ParentID != "" {
							continue
						}
					} else if n.ParentID != wantParent {
						continue
					}
				}
				if strings.Contains(q, driveFolderMime) && n.MimeType != driveFolderMime {
					continue
				}
				matches = append(matches, map[string]string{"id": n.ID, "name": n.Name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": matches})

		case r.Method == http.MethodPost && r.URL.Path == "/drive/v3/files":
			var meta struct {
				Name     string   `json:"name"`
				MimeType string   `json:"mimeType"`
				Parents  []string `json:"parents"`
			}
			_ = json.NewDecoder(r.Body).Decode(&meta)
			pid := ""
			if len(meta.Parents) > 0 {
				pid = meta.Parents[0]
			}
			id := newID()
			nodes[id] = &mockDriveNode{ID: id, Name: meta.Name, MimeType: meta.MimeType, ParentID: pid}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})

		case r.Method == http.MethodPost && r.URL.Path == "/upload/drive/v3/files":
			_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			mr := multipart.NewReader(r.Body, params["boundary"])
			var meta struct {
				Name    string   `json:"name"`
				Parents []string `json:"parents"`
			}
			var media []byte
			partIdx := 0
			for {
				p, err := mr.NextPart()
				if err != nil {
					break
				}
				b, _ := io.ReadAll(p)
				if partIdx == 0 {
					_ = json.Unmarshal(b, &meta)
				} else {
					media = b
				}
				partIdx++
			}
			pid := ""
			if len(meta.Parents) > 0 {
				pid = meta.Parents[0]
			}
			id := newID()
			nodes[id] = &mockDriveNode{ID: id, Name: meta.Name, ParentID: pid, Data: media}
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})

		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/"):
			id, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/upload/drive/v3/files/"))
			n, ok := nodes[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			n.Data, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": n.ID})

		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/drive/v3/files/"):
			id, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/drive/v3/files/"))
			if n, ok := nodes[id]; ok {
				n.Trashed = true
			}
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/drive/v3/files/"):
			id, _ := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/drive/v3/files/"))
			n, ok := nodes[id]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(n.Data)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	// Verify OAuth PKCE code exchange
	ver, chal, err := GeneratePKCE()
	if err != nil || ver == "" || chal == "" {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}
	tokRes, err := ExchangeAuthCode(ctx, srv.Client(), srv.URL+"/token", srv.URL, "cid", "", "auth-code", "http://127.0.0.1/cb", ver)
	if err != nil || tokRes.Email != "author@gmail.com" {
		t.Fatalf("ExchangeAuthCode failed: %+v err=%v", tokRes, err)
	}

	// Start with an EXPIRED token to prove automatic refresh fires transparently
	prov := NewGDriveProvider("cid", "", "Inkweave", "expired-tok", "ref-1", time.Now().Add(-5*time.Minute))
	prov.APIBase = srv.URL
	prov.TokenURL = srv.URL + "/token"
	var persistedEmail string
	prov.OnTokenRefresh = func(access, refresh string, expiry time.Time, email string) {
		persistedEmail = access
	}

	email, err := prov.TestConnection(ctx)
	if err != nil || email != "author@gmail.com" || !refreshedCalled || persistedEmail != "refreshed-tok" {
		t.Fatalf("TestConnection + auto-refresh failed: email=%q refreshed=%v cb=%q err=%v", email, refreshedCalled, persistedEmail, err)
	}

	ws1, _ := storage.NewWorkspace(t.TempDir())
	repo1, _ := ws1.CreateStarterNovel()
	res1, err := SyncProject(ctx, repo1, prov)
	if err != nil || res1.Status != "ok" || res1.Uploaded < 5 {
		t.Fatalf("GDrive SyncProject 1 failed: %+v err=%v", res1, err)
	}

	novels, err := prov.ListRemoteNovels(ctx)
	if err != nil || len(novels) != 1 {
		t.Fatalf("GDrive ListRemoteNovels failed: %+v err=%v", novels, err)
	}

	ws2, _ := storage.NewWorkspace(t.TempDir())
	repo2, err := PullRemoteNovel(ctx, ws2, prov, novels[0].Slug)
	if err != nil {
		t.Fatalf("GDrive PullRemoteNovel failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo2.Dir, "inkweave.yaml")); err != nil {
		t.Fatalf("expected pulled inkweave.yaml on Device 2")
	}
}
