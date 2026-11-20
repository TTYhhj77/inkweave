package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// WebDAVProvider implements Pure-Go WebDAV sync (Nextcloud, ownCloud, Synology NAS, Koofr, Box).
type WebDAVProvider struct {
	BaseURL  string
	RootPath string
	Username string
	Password string
	Client   *http.Client
}

func NewWebDAVProvider(baseURL, rootPath, username, password string) *WebDAVProvider {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	root := "/" + strings.Trim(strings.TrimSpace(rootPath), "/")
	if root == "/" {
		root = "/Inkweave"
	}
	return &WebDAVProvider{
		BaseURL:  base,
		RootPath: root,
		Username: username,
		Password: password,
		Client:   &http.Client{Timeout: 45 * time.Second},
	}
}

func (w *WebDAVProvider) Kind() string { return "webdav" }

func (w *WebDAVProvider) buildURL(parts ...string) string {
	joined := strings.TrimRight(w.RootPath, "/")
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			joined += "/" + encodePathSegments(p)
		}
	}
	return w.BaseURL + joined
}

func encodePathSegments(p string) string {
	segs := strings.Split(strings.Trim(p, "/"), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

func (w *WebDAVProvider) doReq(ctx context.Context, method, targetURL string, body []byte, headers map[string]string) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL, rdr)
	if err != nil {
		return nil, err
	}
	if w.Username != "" {
		req.SetBasicAuth(w.Username, w.Password)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return w.Client.Do(req)
}

func (w *WebDAVProvider) ensureCollection(ctx context.Context, relDirs ...string) error {
	acc := ""
	if err := w.mkcol(ctx, w.BaseURL+encodePathSegments(w.RootPath)); err != nil {
		return err
	}
	for _, seg := range relDirs {
		if seg == "" || seg == "." {
			continue
		}
		if acc == "" {
			acc = seg
		} else {
			acc += "/" + seg
		}
		if err := w.mkcol(ctx, w.buildURL(acc)); err != nil {
			return err
		}
	}
	return nil
}

func (w *WebDAVProvider) mkcol(ctx context.Context, dirURL string) error {
	resp, err := w.doReq(ctx, "MKCOL", dirURL, nil, nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	// 201 Created, 405 Method Not Allowed (already exists), 301/200/204
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusMethodNotAllowed ||
		resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("webdav MKCOL %s -> HTTP %d", dirURL, resp.StatusCode)
}

func (w *WebDAVProvider) TestConnection(ctx context.Context) (string, error) {
	if w.BaseURL == "" {
		return "", fmt.Errorf("WebDAV Base URL is empty")
	}
	if err := w.ensureCollection(ctx); err != nil {
		return "", err
	}
	resp, err := w.doReq(ctx, "PROPFIND", w.buildURL(), nil, map[string]string{"Depth": "0"})
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return w.BaseURL + w.RootPath, nil
	}
	return "", fmt.Errorf("webdav HTTP %d", resp.StatusCode)
}

func (w *WebDAVProvider) ListRemoteNovels(ctx context.Context) ([]RemoteNovel, error) {
	if w.BaseURL == "" {
		return nil, fmt.Errorf("WebDAV Base URL is empty")
	}
	if err := w.ensureCollection(ctx); err != nil {
		return nil, err
	}
	propBody := `<?xml version="1.0" encoding="utf-8" ?><D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>`
	resp, err := w.doReq(ctx, "PROPFIND", w.buildURL(), []byte(propBody), map[string]string{
		"Depth":        "1",
		"Content-Type": "application/xml; charset=utf-8",
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("webdav PROPFIND HTTP %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)

	var ms struct {
		Responses []struct {
			Href     string `xml:"href"`
			Propstat struct {
				Prop struct {
					Resourcetype struct {
						Collection *struct{} `xml:"collection"`
					} `xml:"resourcetype"`
				} `xml:"prop"`
			} `xml:"propstat"`
		} `xml:"response"`
	}
	if err := xml.Unmarshal(raw, &ms); err != nil {
		return []RemoteNovel{}, nil
	}

	rootTrim := strings.TrimRight(w.RootPath, "/")
	var novels []RemoteNovel
	seen := map[string]bool{}
	for _, r := range ms.Responses {
		if r.Propstat.Prop.Resourcetype.Collection == nil {
			continue
		}
		u, err := url.Parse(r.Href)
		hrefPath := r.Href
		if err == nil {
			hrefPath = u.Path
		}
		hrefPath = strings.TrimRight(hrefPath, "/")
		if hrefPath == "" || strings.HasSuffix(hrefPath, rootTrim) {
			continue
		}
		slug := path.Base(hrefPath)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		if m, err := w.ReadManifest(ctx, slug); err == nil && m != nil {
			novels = append(novels, RemoteNovel{
				Slug:      m.NovelSlug,
				Title:     m.Title,
				FileCount: len(m.Files),
				UpdatedAt: m.UpdatedAt,
			})
		}
	}
	sort.Slice(novels, func(i, j int) bool { return novels[i].UpdatedAt.After(novels[j].UpdatedAt) })
	return novels, nil
}

func (w *WebDAVProvider) ReadManifest(ctx context.Context, slug string) (*SyncManifest, error) {
	if w.BaseURL == "" {
		return nil, fmt.Errorf("WebDAV Base URL is empty")
	}
	resp, err := w.doReq(ctx, http.MethodGet, w.buildURL(slug, manifestFileName), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("webdav GET manifest HTTP %d", resp.StatusCode)
	}
	var m SyncManifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (w *WebDAVProvider) WriteManifest(ctx context.Context, slug string, m *SyncManifest) error {
	if err := w.ensureCollection(ctx, slug); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	resp, err := w.doReq(ctx, http.MethodPut, w.buildURL(slug, manifestFileName), data, map[string]string{
		"Content-Type": "application/json",
	})
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webdav PUT manifest HTTP %d", resp.StatusCode)
	}
	return nil
}

func (w *WebDAVProvider) UploadFile(ctx context.Context, slug, relPath string, data []byte, existingRemoteID string) (string, error) {
	dirPart := path.Dir(strings.Trim(relPath, "/"))
	segs := []string{slug}
	if dirPart != "." && dirPart != "" {
		segs = append(segs, strings.Split(dirPart, "/")...)
	}
	if err := w.ensureCollection(ctx, segs...); err != nil {
		return "", err
	}
	resp, err := w.doReq(ctx, http.MethodPut, w.buildURL(slug, relPath), data, map[string]string{
		"Content-Type": "application/octet-stream",
	})
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("webdav PUT %s HTTP %d", relPath, resp.StatusCode)
	}
	return relPath, nil
}

func (w *WebDAVProvider) DownloadFile(ctx context.Context, slug, relPath, remoteID string) ([]byte, error) {
	resp, err := w.doReq(ctx, http.MethodGet, w.buildURL(slug, relPath), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("webdav GET %s HTTP %d", relPath, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (w *WebDAVProvider) DeleteFile(ctx context.Context, slug, relPath, remoteID string) error {
	resp, err := w.doReq(ctx, http.MethodDelete, w.buildURL(slug, relPath), nil, nil)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("webdav DELETE %s HTTP %d", relPath, resp.StatusCode)
}
