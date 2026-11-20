package cloudsync

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultDriveAPIBase  = "https://www.googleapis.com"
	defaultDriveTokenURL = "https://oauth2.googleapis.com/token"
	driveOAuthAuthURL    = "https://accounts.google.com/o/oauth2/v2/auth"
	driveFolderMime      = "application/vnd.google-apps.folder"
)

// GDriveProvider implements Pure-Go Google Drive REST API v3 with OAuth 2.0 PKCE + Auto-Refresh.
type GDriveProvider struct {
	ClientID       string
	ClientSecret   string
	RootFolder     string
	AccessToken    string
	RefreshToken   string
	TokenExpiry    time.Time
	APIBase        string
	TokenURL       string
	OnTokenRefresh func(access, refresh string, expiry time.Time, email string)
	Client         *http.Client

	mu          sync.Mutex
	folderCache map[string]string // path key -> Drive folder ID
}

func NewGDriveProvider(clientID, clientSecret, rootFolder, access, refresh string, expiry time.Time) *GDriveProvider {
	if strings.TrimSpace(rootFolder) == "" {
		rootFolder = "Inkweave"
	}
	return &GDriveProvider{
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		RootFolder:   strings.TrimSpace(rootFolder),
		AccessToken:  strings.TrimSpace(access),
		RefreshToken: strings.TrimSpace(refresh),
		TokenExpiry:  expiry,
		APIBase:      defaultDriveAPIBase,
		TokenURL:     defaultDriveTokenURL,
		Client:       &http.Client{Timeout: 60 * time.Second},
		folderCache:  map[string]string{},
	}
}

func (g *GDriveProvider) Kind() string { return "gdrive" }

// GeneratePKCE produces a cryptographic OAuth 2.0 PKCE verifier & S256 challenge.
func GeneratePKCE() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// BuildOAuthURL constructs the Google OAuth 2.0 consent screen URL using minimal drive.file scope.
func BuildOAuthURL(clientID, redirectURI, state, challenge string) string {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("redirect_uri", redirectURI)
	v.Set("response_type", "code")
	v.Set("scope", "https://www.googleapis.com/auth/drive.file https://www.googleapis.com/auth/userinfo.email")
	v.Set("access_type", "offline")
	v.Set("prompt", "consent")
	v.Set("state", state)
	v.Set("code_challenge", challenge)
	v.Set("code_challenge_method", "S256")
	return driveOAuthAuthURL + "?" + v.Encode()
}

type OAuthTokenResult struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	Email        string    `json:"email"`
}

// ExchangeAuthCode exchanges an OAuth authorization code + PKCE verifier for tokens.
func ExchangeAuthCode(ctx context.Context, client *http.Client, tokenURL, apiBase, clientID, clientSecret, code, redirectURI, verifier string) (*OAuthTokenResult, error) {
	if tokenURL == "" {
		tokenURL = defaultDriveTokenURL
	}
	if apiBase == "" {
		apiBase = defaultDriveAPIBase
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("oauth token exchange HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, err
	}
	exp := time.Now().UTC().Add(time.Duration(tok.ExpiresIn) * time.Second)
	res := &OAuthTokenResult{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Expiry:       exp,
	}
	if email, err := fetchDriveEmail(ctx, client, apiBase, tok.AccessToken); err == nil {
		res.Email = email
	}
	return res, nil
}

func fetchDriveEmail(ctx context.Context, client *http.Client, apiBase, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(apiBase, "/")+"/drive/v3/about?fields=user(displayName,emailAddress)", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var about struct {
		User struct {
			EmailAddress string `json:"emailAddress"`
			DisplayName  string `json:"displayName"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&about); err != nil {
		return "", err
	}
	if about.User.EmailAddress != "" {
		return about.User.EmailAddress, nil
	}
	return about.User.DisplayName, nil
}

func (g *GDriveProvider) ensureAccessToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	tok := g.AccessToken
	ref := g.RefreshToken
	exp := g.TokenExpiry
	g.mu.Unlock()

	if tok != "" && (exp.IsZero() || time.Now().UTC().Before(exp.Add(-60*time.Second)) || ref == "") {
		return tok, nil
	}
	if ref == "" {
		if tok == "" {
			return "", fmt.Errorf("Google Drive is not connected")
		}
		return tok, nil
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", g.ClientID)
	if g.ClientSecret != "" {
		form.Set("client_secret", g.ClientSecret)
	}
	form.Set("refresh_token", ref)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("refresh Google Drive token HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var refreshed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &refreshed); err != nil {
		return "", err
	}
	newExp := time.Now().UTC().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
	g.mu.Lock()
	g.AccessToken = refreshed.AccessToken
	if refreshed.RefreshToken != "" {
		g.RefreshToken = refreshed.RefreshToken
	}
	g.TokenExpiry = newExp
	cb := g.OnTokenRefresh
	g.mu.Unlock()

	if cb != nil {
		cb(refreshed.AccessToken, refreshed.RefreshToken, newExp, "")
	}
	return refreshed.AccessToken, nil
}

func (g *GDriveProvider) TestConnection(ctx context.Context) (string, error) {
	tok, err := g.ensureAccessToken(ctx)
	if err != nil {
		return "", err
	}
	email, err := fetchDriveEmail(ctx, g.Client, g.APIBase, tok)
	if err != nil {
		return "", fmt.Errorf("Google Drive connection failed: %w", err)
	}
	if _, err := g.ensureFolderPath(ctx, nil); err != nil {
		return "", err
	}
	if email == "" {
		email = "Google Drive Ready"
	}
	return email, nil
}

func escapeDriveQuery(val string) string {
	val = strings.ReplaceAll(val, `\`, `\\`)
	return strings.ReplaceAll(val, `'`, `\'`)
}

func (g *GDriveProvider) findChild(ctx context.Context, parentID, name, mimeFilter string) (string, error) {
	tok, err := g.ensureAccessToken(ctx)
	if err != nil {
		return "", err
	}
	q := fmt.Sprintf("name='%s' and trashed=false", escapeDriveQuery(name))
	if parentID != "" {
		q += fmt.Sprintf(" and '%s' in parents", escapeDriveQuery(parentID))
	} else {
		q += " and 'root' in parents"
	}
	if mimeFilter != "" {
		q += fmt.Sprintf(" and mimeType='%s'", mimeFilter)
	}
	u := fmt.Sprintf("%s/drive/v3/files?q=%s&fields=files(id,name)&pageSize=10",
		strings.TrimRight(g.APIBase, "/"), url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("drive list HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Files) > 0 {
		return out.Files[0].ID, nil
	}
	return "", nil
}

func (g *GDriveProvider) createFolder(ctx context.Context, parentID, name string) (string, error) {
	tok, err := g.ensureAccessToken(ctx)
	if err != nil {
		return "", err
	}
	meta := map[string]any{
		"name":     name,
		"mimeType": driveFolderMime,
	}
	if parentID != "" {
		meta["parents"] = []string{parentID}
	}
	body, _ := json.Marshal(meta)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(g.APIBase, "/")+"/drive/v3/files?fields=id", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("drive mkdir %s HTTP %d: %s", name, resp.StatusCode, string(raw))
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (g *GDriveProvider) ensureFolderPath(ctx context.Context, relSegments []string) (string, error) {
	allSegs := append([]string{g.RootFolder}, relSegments...)
	parentID := ""
	key := ""
	for _, seg := range allSegs {
		if seg == "" || seg == "." {
			continue
		}
		if key == "" {
			key = seg
		} else {
			key += "/" + seg
		}
		g.mu.Lock()
		cached := g.folderCache[key]
		g.mu.Unlock()
		if cached != "" {
			parentID = cached
			continue
		}
		foundID, err := g.findChild(ctx, parentID, seg, driveFolderMime)
		if err != nil {
			return "", err
		}
		if foundID == "" {
			foundID, err = g.createFolder(ctx, parentID, seg)
			if err != nil {
				return "", err
			}
		}
		g.mu.Lock()
		g.folderCache[key] = foundID
		g.mu.Unlock()
		parentID = foundID
	}
	return parentID, nil
}

func (g *GDriveProvider) ListRemoteNovels(ctx context.Context) ([]RemoteNovel, error) {
	rootID, err := g.findChild(ctx, "", g.RootFolder, driveFolderMime)
	if err != nil {
		return nil, err
	}
	if rootID == "" {
		return []RemoteNovel{}, nil
	}
	tok, err := g.ensureAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf("'%s' in parents and mimeType='%s' and trashed=false", escapeDriveQuery(rootID), driveFolderMime)
	u := fmt.Sprintf("%s/drive/v3/files?q=%s&fields=files(id,name)&pageSize=50",
		strings.TrimRight(g.APIBase, "/"), url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("drive list novels HTTP %d", resp.StatusCode)
	}
	var out struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	var novels []RemoteNovel
	for _, f := range out.Files {
		g.mu.Lock()
		g.folderCache[g.RootFolder+"/"+f.Name] = f.ID
		g.mu.Unlock()
		if m, err := g.ReadManifest(ctx, f.Name); err == nil && m != nil {
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

func (g *GDriveProvider) ReadManifest(ctx context.Context, slug string) (*SyncManifest, error) {
	rootID, err := g.findChild(ctx, "", g.RootFolder, driveFolderMime)
	if err != nil || rootID == "" {
		return nil, err
	}
	novelID, err := g.findChild(ctx, rootID, slug, driveFolderMime)
	if err != nil || novelID == "" {
		return nil, err
	}
	fileID, err := g.findChild(ctx, novelID, manifestFileName, "")
	if err != nil || fileID == "" {
		return nil, err
	}
	raw, err := g.DownloadFile(ctx, slug, manifestFileName, fileID)
	if err != nil {
		return nil, err
	}
	var m SyncManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (g *GDriveProvider) WriteManifest(ctx context.Context, slug string, m *SyncManifest) error {
	folderID, err := g.ensureFolderPath(ctx, []string{slug})
	if err != nil {
		return err
	}
	existingID, _ := g.findChild(ctx, folderID, manifestFileName, "")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	_, err = g.uploadToParent(ctx, folderID, manifestFileName, data, "application/json", existingID)
	return err
}

func (g *GDriveProvider) UploadFile(ctx context.Context, slug, relPath string, data []byte, existingRemoteID string) (string, error) {
	clean := strings.Trim(relPath, "/")
	dirPart := path.Dir(clean)
	baseName := path.Base(clean)

	segs := []string{slug}
	if dirPart != "." && dirPart != "" {
		segs = append(segs, strings.Split(dirPart, "/")...)
	}
	parentID, err := g.ensureFolderPath(ctx, segs)
	if err != nil {
		return "", err
	}
	if existingRemoteID == "" {
		existingRemoteID, _ = g.findChild(ctx, parentID, baseName, "")
	}
	return g.uploadToParent(ctx, parentID, baseName, data, "text/plain; charset=utf-8", existingRemoteID)
}

func (g *GDriveProvider) uploadToParent(ctx context.Context, parentID, name string, data []byte, contentType, existingID string) (string, error) {
	tok, err := g.ensureAccessToken(ctx)
	if err != nil {
		return "", err
	}
	base := strings.TrimRight(g.APIBase, "/")

	if existingID != "" {
		u := fmt.Sprintf("%s/upload/drive/v3/files/%s?uploadType=media&fields=id", base, url.PathEscape(existingID))
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", contentType)
		resp, err := g.Client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var res struct {
				ID string `json:"id"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&res)
			if res.ID != "" {
				return res.ID, nil
			}
			return existingID, nil
		}
		if resp.StatusCode != http.StatusNotFound {
			raw, _ := io.ReadAll(resp.Body)
			return "", fmt.Errorf("drive patch HTTP %d: %s", resp.StatusCode, string(raw))
		}
	}

	// Multipart create inside parentID
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, err := mw.CreatePart(metaHeader)
	if err != nil {
		return "", err
	}
	metaJSON, _ := json.Marshal(map[string]any{
		"name":    name,
		"parents": []string{parentID},
	})
	_, _ = metaPart.Write(metaJSON)

	mediaHeader := make(textproto.MIMEHeader)
	mediaHeader.Set("Content-Type", contentType)
	mediaPart, err := mw.CreatePart(mediaHeader)
	if err != nil {
		return "", err
	}
	_, _ = mediaPart.Write(data)
	_ = mw.Close()

	u := fmt.Sprintf("%s/upload/drive/v3/files?uploadType=multipart&fields=id", base)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "multipart/related; boundary="+mw.Boundary())
	resp, err := g.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("drive multipart upload HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (g *GDriveProvider) DownloadFile(ctx context.Context, slug, relPath, remoteID string) ([]byte, error) {
	tok, err := g.ensureAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	if remoteID == "" {
		return nil, fmt.Errorf("missing Google Drive file ID for %s", relPath)
	}
	u := fmt.Sprintf("%s/drive/v3/files/%s?alt=media", strings.TrimRight(g.APIBase, "/"), url.PathEscape(remoteID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("drive download HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return io.ReadAll(resp.Body)
}

func (g *GDriveProvider) DeleteFile(ctx context.Context, slug, relPath, remoteID string) error {
	if remoteID == "" {
		return nil
	}
	tok, err := g.ensureAccessToken(ctx)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/drive/v3/files/%s", strings.TrimRight(g.APIBase, "/"), url.PathEscape(remoteID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, strings.NewReader(`{"trashed":true}`))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.Client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("drive trash HTTP %d", resp.StatusCode)
}
