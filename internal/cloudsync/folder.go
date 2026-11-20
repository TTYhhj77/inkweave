package cloudsync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inkweave/inkweave/internal/storage"
)

// FolderProvider syncs novels against any mounted directory (OneDrive, Dropbox,
// Google Drive Desktop, iCloud Drive, Syncthing, or NAS share).
type FolderProvider struct {
	RootDir string
}

func NewFolderProvider(rootDir string) *FolderProvider {
	return &FolderProvider{RootDir: strings.TrimSpace(rootDir)}
}

func (f *FolderProvider) Kind() string { return "folder" }

func (f *FolderProvider) TestConnection(ctx context.Context) (string, error) {
	if f.RootDir == "" {
		return "", fmt.Errorf("folder path is empty")
	}
	if err := os.MkdirAll(f.RootDir, 0o755); err != nil {
		return "", fmt.Errorf("cannot access folder %s: %w", f.RootDir, err)
	}
	probe := filepath.Join(f.RootDir, ".inkweave-write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return "", fmt.Errorf("folder is not writable: %w", err)
	}
	_ = os.Remove(probe)
	return f.RootDir, nil
}

func (f *FolderProvider) ListRemoteNovels(ctx context.Context) ([]RemoteNovel, error) {
	if f.RootDir == "" {
		return nil, fmt.Errorf("folder path is empty")
	}
	entries, err := os.ReadDir(f.RootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RemoteNovel{}, nil
		}
		return nil, err
	}
	var out []RemoteNovel
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := f.ReadManifest(ctx, e.Name())
		if err != nil || m == nil {
			continue
		}
		out = append(out, RemoteNovel{
			Slug:      m.NovelSlug,
			Title:     m.Title,
			FileCount: len(m.Files),
			UpdatedAt: m.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (f *FolderProvider) novelDir(slug string) string {
	return filepath.Join(f.RootDir, slug)
}

func (f *FolderProvider) ReadManifest(ctx context.Context, slug string) (*SyncManifest, error) {
	if f.RootDir == "" {
		return nil, fmt.Errorf("folder path is empty")
	}
	p := filepath.Join(f.novelDir(slug), manifestFileName)
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m SyncManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (f *FolderProvider) WriteManifest(ctx context.Context, slug string, m *SyncManifest) error {
	if err := os.MkdirAll(f.novelDir(slug), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(filepath.Join(f.novelDir(slug), manifestFileName), raw)
}

func (f *FolderProvider) UploadFile(ctx context.Context, slug, relPath string, data []byte, existingRemoteID string) (string, error) {
	target := filepath.Join(f.novelDir(slug), filepath.FromSlash(relPath))
	if err := storage.WriteAtomic(target, data); err != nil {
		return "", err
	}
	return relPath, nil
}

func (f *FolderProvider) DownloadFile(ctx context.Context, slug, relPath, remoteID string) ([]byte, error) {
	target := filepath.Join(f.novelDir(slug), filepath.FromSlash(relPath))
	return os.ReadFile(target)
}

func (f *FolderProvider) DeleteFile(ctx context.Context, slug, relPath, remoteID string) error {
	target := filepath.Join(f.novelDir(slug), filepath.FromSlash(relPath))
	err := os.Remove(target)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
