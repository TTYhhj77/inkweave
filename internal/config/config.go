package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/inkweave/inkweave/internal/model"
)

// Secrets holds API keys, GitHub PAT, and Cloud Sync tokens strictly outside novel repositories.
type Secrets struct {
	ProviderKeys       map[string]string `json:"provider_keys"`
	GitHubToken        string            `json:"github_token"`
	GDriveClientSecret string            `json:"gdrive_client_secret,omitempty"`
	GDriveAccessToken  string            `json:"gdrive_access_token,omitempty"`
	GDriveRefreshToken string            `json:"gdrive_refresh_token,omitempty"`
	GDriveTokenExpiry  time.Time         `json:"gdrive_token_expiry,omitempty"`
	WebDAVPassword     string            `json:"webdav_password,omitempty"`
}

// Manager persists AppConfig and Secrets safely in the local dataDir.
type Manager struct {
	mu      sync.RWMutex
	dataDir string
	cfg     model.AppConfig
	secrets Secrets
}

// DefaultDataDir returns ~/.inkweave (or OS app dir fallback).
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".inkweave-data")
	}
	return filepath.Join(home, ".inkweave")
}

// NewManager initializes config & secrets storage under dataDir.
func NewManager(dataDir string) (*Manager, error) {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	projectsDir := filepath.Join(dataDir, "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		return nil, err
	}

	m := &Manager{
		dataDir: dataDir,
		secrets: Secrets{ProviderKeys: map[string]string{}},
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) DataDir() string {
	return m.dataDir
}

func (m *Manager) ProjectsDir() string {
	return filepath.Join(m.dataDir, "projects")
}

func defaultAppConfig() model.AppConfig {
	cfg := model.AppConfig{
		Providers: []model.ProviderConfig{
			{
				ID:           "mock-studio",
				Name:         "Built-in Studio Demo (Offline)",
				Kind:         "mock",
				BaseURL:      "",
				DefaultModel: "inkweave-demo-v1",
				Enabled:      true,
				HasAPIKey:    true,
			},
			{
				ID:           "openai",
				Name:         "OpenAI",
				Kind:         "openai",
				BaseURL:      "https://api.openai.com/v1",
				DefaultModel: "gpt-4.1",
				Enabled:      false,
			},
			{
				ID:           "anthropic",
				Name:         "Anthropic Claude",
				Kind:         "anthropic",
				BaseURL:      "https://api.anthropic.com",
				DefaultModel: "claude-sonnet-4-20250514",
				Enabled:      false,
			},
			{
				ID:           "gemini",
				Name:         "Google Gemini",
				Kind:         "gemini",
				BaseURL:      "https://generativelanguage.googleapis.com",
				DefaultModel: "gemini-2.5-flash",
				Enabled:      false,
			},
			{
				ID:           "openrouter",
				Name:         "OpenRouter",
				Kind:         "openai-compat",
				BaseURL:      "https://openrouter.ai/api/v1",
				DefaultModel: "anthropic/claude-sonnet-4",
				Enabled:      false,
			},
			{
				ID:           "deepseek",
				Name:         "DeepSeek",
				Kind:         "openai-compat",
				BaseURL:      "https://api.deepseek.com/v1",
				DefaultModel: "deepseek-chat",
				Enabled:      false,
			},
			{
				ID:           "ollama",
				Name:         "Ollama (Local)",
				Kind:         "openai-compat",
				BaseURL:      "http://127.0.0.1:11434/v1",
				DefaultModel: "qwen2.5:14b",
				Enabled:      false,
			},
		},
		Routing: model.ModelRouting{
			Primary:   model.ModelSlot{ProviderID: "mock-studio", Model: "inkweave-demo-v1"},
			Utility:   model.ModelSlot{ProviderID: "mock-studio", Model: "inkweave-demo-v1"},
			Architect: model.ModelSlot{ProviderID: "mock-studio", Model: "inkweave-demo-v1"},
		},
		UI: model.UIPreferences{
			Theme:          "paper",
			Language:       "th",
			FontFamily:     "sarabun",
			FontSize:       18,
			LineHeight:     1.88,
			ReducedMotion:  false,
			DefaultMode:    model.ModeBalanced,
			ApprovalMode:   model.ApprovalReview,
			UltraBudgetCap: 48000,
		},
	}
	cfg.CloudSync.ActiveProvider = "github"
	cfg.CloudSync.GoogleDrive.RootFolder = "Inkweave"
	cfg.CloudSync.WebDAV.RootPath = "/Inkweave"
	cfg.GitAuthor.Name = "Inkweave Author"
	cfg.GitAuthor.Email = "author@inkweave.local"
	return cfg
}

func (m *Manager) configPath() string {
	return filepath.Join(m.dataDir, "config.json")
}

func (m *Manager) secretsPath() string {
	return filepath.Join(m.dataDir, "secrets.json")
}

func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfgPath := m.configPath()
	if raw, err := os.ReadFile(cfgPath); err == nil {
		var loaded model.AppConfig
		if uErr := json.Unmarshal(raw, &loaded); uErr == nil {
			m.cfg = loaded
		} else {
			m.cfg = defaultAppConfig()
		}
	} else {
		m.cfg = defaultAppConfig()
		_ = m.saveConfigLocked()
	}

	secPath := m.secretsPath()
	if raw, err := os.ReadFile(secPath); err == nil {
		var sec Secrets
		if uErr := json.Unmarshal(raw, &sec); uErr == nil {
			if sec.ProviderKeys == nil {
				sec.ProviderKeys = map[string]string{}
			}
			m.secrets = sec
		}
	}
	m.syncAPIKeyFlagsLocked()
	return nil
}

func (m *Manager) syncAPIKeyFlagsLocked() {
	for i := range m.cfg.Providers {
		p := &m.cfg.Providers[i]
		if p.Kind == "mock" {
			p.HasAPIKey = true
			continue
		}
		key := m.secrets.ProviderKeys[p.ID]
		p.HasAPIKey = key != ""
	}
	if m.cfg.CloudSync.ActiveProvider == "" {
		m.cfg.CloudSync.ActiveProvider = "github"
	}
	if m.cfg.CloudSync.GoogleDrive.RootFolder == "" {
		m.cfg.CloudSync.GoogleDrive.RootFolder = "Inkweave"
	}
	if m.cfg.CloudSync.WebDAV.RootPath == "" {
		m.cfg.CloudSync.WebDAV.RootPath = "/Inkweave"
	}
	m.cfg.CloudSync.GoogleDrive.Connected = m.secrets.GDriveRefreshToken != "" || m.secrets.GDriveAccessToken != ""
	m.cfg.CloudSync.WebDAV.HasPass = m.secrets.WebDAVPassword != ""
}

func (m *Manager) GetConfig() model.AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := m.cfg
	out.Providers = append([]model.ProviderConfig(nil), m.cfg.Providers...)
	return out
}

func (m *Manager) UpdateConfig(cfg model.AppConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	m.syncAPIKeyFlagsLocked()
	return m.saveConfigLocked()
}

func (m *Manager) UpsertProvider(p model.ProviderConfig, apiKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.secrets.ProviderKeys == nil {
		m.secrets.ProviderKeys = map[string]string{}
	}
	if apiKey != "" {
		m.secrets.ProviderKeys[p.ID] = apiKey
		if err := m.saveSecretsLocked(); err != nil {
			return err
		}
	}

	replaced := false
	for i := range m.cfg.Providers {
		if m.cfg.Providers[i].ID == p.ID {
			m.cfg.Providers[i] = p
			replaced = true
			break
		}
	}
	if !replaced {
		m.cfg.Providers = append(m.cfg.Providers, p)
	}
	m.syncAPIKeyFlagsLocked()
	return m.saveConfigLocked()
}

func (m *Manager) DeleteProvider(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id == "mock-studio" {
		return nil
	}
	filtered := make([]model.ProviderConfig, 0, len(m.cfg.Providers))
	for _, p := range m.cfg.Providers {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	m.cfg.Providers = filtered
	delete(m.secrets.ProviderKeys, id)
	_ = m.saveSecretsLocked()
	return m.saveConfigLocked()
}

func (m *Manager) GetProviderKey(providerID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.secrets.ProviderKeys[providerID]
}

func (m *Manager) SetGitHubToken(token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets.GitHubToken = token
	return m.saveSecretsLocked()
}

func (m *Manager) GetGitHubToken() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.secrets.GitHubToken
}

func (m *Manager) GetGDriveTokens() (access string, refresh string, expiry time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.secrets.GDriveAccessToken, m.secrets.GDriveRefreshToken, m.secrets.GDriveTokenExpiry
}

func (m *Manager) SetGDriveTokens(access, refresh string, expiry time.Time, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets.GDriveAccessToken = access
	if refresh != "" {
		m.secrets.GDriveRefreshToken = refresh
	}
	m.secrets.GDriveTokenExpiry = expiry
	if email != "" {
		m.cfg.CloudSync.GoogleDrive.AccountEmail = email
	}
	m.syncAPIKeyFlagsLocked()
	if err := m.saveSecretsLocked(); err != nil {
		return err
	}
	return m.saveConfigLocked()
}

func (m *Manager) ClearGDriveTokens() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets.GDriveAccessToken = ""
	m.secrets.GDriveRefreshToken = ""
	m.secrets.GDriveTokenExpiry = time.Time{}
	m.cfg.CloudSync.GoogleDrive.AccountEmail = ""
	m.syncAPIKeyFlagsLocked()
	if err := m.saveSecretsLocked(); err != nil {
		return err
	}
	return m.saveConfigLocked()
}

func (m *Manager) SetGDriveClientSecret(secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets.GDriveClientSecret = secret
	return m.saveSecretsLocked()
}

func (m *Manager) GetGDriveClientSecret() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.secrets.GDriveClientSecret
}

func (m *Manager) SetWebDAVPassword(pass string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secrets.WebDAVPassword = pass
	m.syncAPIKeyFlagsLocked()
	if err := m.saveSecretsLocked(); err != nil {
		return err
	}
	return m.saveConfigLocked()
}

func (m *Manager) GetWebDAVPassword() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.secrets.WebDAVPassword
}

func (m *Manager) saveConfigLocked() error {
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(m.configPath(), data, 0o644)
}

func (m *Manager) saveSecretsLocked() error {
	data, err := json.MarshalIndent(m.secrets, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(m.secretsPath(), data, 0o600)
}

func writeAtomic(target string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
