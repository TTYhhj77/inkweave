package cloudsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/inkweave/inkweave/internal/gitsync"
	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/storage"
	"gopkg.in/yaml.v3"
)

const manifestFileName = "sync-manifest.json"
const maxBaseCacheBytes = 512 * 1024

// RemoteFileEntry describes one tracked novel file in the remote sync-manifest.json.
type RemoteFileEntry struct {
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	RemoteID  string    `json:"remote_id,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SyncManifest is stored per novel on the remote provider as sync-manifest.json.
type SyncManifest struct {
	NovelSlug string                     `json:"novel_slug"`
	Title     string                     `json:"title"`
	UpdatedAt time.Time                  `json:"updated_at"`
	Files     map[string]RemoteFileEntry `json:"files"`
}

// LocalFileState records the last successfully synced SHA-256 per relative file path.
type LocalFileState struct {
	SHA256   string `json:"sha256"`
	RemoteID string `json:"remote_id,omitempty"`
}

// LocalSyncState is persisted strictly inside .inkweave/cloud-sync-<kind>.json (never synced).
type LocalSyncState struct {
	Provider   string                    `json:"provider"`
	LastSyncAt time.Time                 `json:"last_sync_at"`
	Files      map[string]LocalFileState `json:"files"`
}

// RemoteNovel summarizes a novel discovered on Google Drive, WebDAV, or a Synced Folder.
type RemoteNovel struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	FileCount int       `json:"file_count"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SyncStatus summarizes local unsynced edits relative to the last cloud sync.
type SyncStatus struct {
	Provider          string    `json:"provider"`
	LastSyncAt        time.Time `json:"last_sync_at,omitempty"`
	TrackedCount      int       `json:"tracked_count"`
	ModifiedSinceSync []string  `json:"modified_since_sync"`
}

// CloudSyncResult reports the outcome of a 3-way cloud sync operation.
type CloudSyncResult struct {
	Provider   string                 `json:"provider"`
	Status     string                 `json:"status"` // ok | conflicts
	Message    string                 `json:"message"`
	Uploaded   int                    `json:"uploaded"`
	Downloaded int                    `json:"downloaded"`
	Deleted    int                    `json:"deleted"`
	Conflicts  []gitsync.ConflictFile `json:"conflicts,omitempty"`
	SyncedAt   time.Time              `json:"synced_at"`
}

// Provider is implemented by Google Drive, WebDAV, and Synced Folder drivers.
type Provider interface {
	Kind() string
	TestConnection(ctx context.Context) (string, error)
	ListRemoteNovels(ctx context.Context) ([]RemoteNovel, error)
	ReadManifest(ctx context.Context, slug string) (*SyncManifest, error)
	WriteManifest(ctx context.Context, slug string, m *SyncManifest) error
	UploadFile(ctx context.Context, slug, relPath string, data []byte, existingRemoteID string) (string, error)
	DownloadFile(ctx context.Context, slug, relPath, remoteID string) ([]byte, error)
	DeleteFile(ctx context.Context, slug, relPath, remoteID string) error
}

// IsTrackedNovelFile filters strictly to author-authored novel files, excluding caches & git internals.
func IsTrackedNovelFile(relPath string) bool {
	p := filepath.ToSlash(relPath)
	if p == "" || strings.HasPrefix(p, ".inkweave/") || p == ".inkweave" ||
		strings.HasPrefix(p, ".git/") || p == ".git" || strings.HasSuffix(p, ".tmp") ||
		p == manifestFileName {
		return false
	}
	return p == "inkweave.yaml" ||
		p == ".gitignore" ||
		strings.HasPrefix(p, "manuscript/") ||
		strings.HasPrefix(p, "bible/") ||
		strings.HasPrefix(p, "continuity/") ||
		strings.HasPrefix(p, "skills/")
}

type scannedFile struct {
	Hash string
	Size int64
	Data []byte
}

func scanLocalNovel(repoDir string) (map[string]scannedFile, error) {
	out := map[string]scannedFile{}
	err := filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rErr := filepath.Rel(repoDir, path)
		if rErr != nil {
			return nil
		}
		slash := filepath.ToSlash(rel)
		if d.IsDir() {
			if slash == ".inkweave" || slash == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsTrackedNovelFile(slash) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[slash] = scannedFile{
			Hash: sha256Hex(raw),
			Size: int64(len(raw)),
			Data: raw,
		}
		return nil
	})
	return out, err
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func stateFilePath(repoDir, kind string) string {
	return filepath.Join(repoDir, ".inkweave", "cloud-sync-"+kind+".json")
}

func loadLocalState(repoDir, kind string) LocalSyncState {
	st := LocalSyncState{Provider: kind, Files: map[string]LocalFileState{}}
	raw, err := os.ReadFile(stateFilePath(repoDir, kind))
	if err != nil {
		return st
	}
	if json.Unmarshal(raw, &st) == nil && st.Files == nil {
		st.Files = map[string]LocalFileState{}
	}
	return st
}

func saveLocalState(repoDir string, st LocalSyncState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(stateFilePath(repoDir, st.Provider), data)
}

func cacheBaseBytes(repoDir, hash string, data []byte) {
	if hash == "" || len(data) > maxBaseCacheBytes {
		return
	}
	p := filepath.Join(repoDir, ".inkweave", "cloud-base", hash)
	if _, err := os.Stat(p); err == nil {
		return
	}
	_ = storage.WriteAtomic(p, data)
}

func readBaseCache(repoDir, hash string) string {
	if hash == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(repoDir, ".inkweave", "cloud-base", hash))
	if err != nil {
		return ""
	}
	return string(raw)
}

// ProjectSyncStatus returns local file count and paths modified since the last cloud sync.
func ProjectSyncStatus(repo *storage.ProjectRepo, kind string) (SyncStatus, error) {
	localFiles, err := scanLocalNovel(repo.Dir)
	if err != nil {
		return SyncStatus{}, err
	}
	st := loadLocalState(repo.Dir, kind)
	var dirty []string
	for p, lf := range localFiles {
		if prev, ok := st.Files[p]; !ok || prev.SHA256 != lf.Hash {
			dirty = append(dirty, p)
		}
	}
	for p := range st.Files {
		if _, ok := localFiles[p]; !ok {
			dirty = append(dirty, p+" (deleted)")
		}
	}
	sort.Strings(dirty)
	return SyncStatus{
		Provider:          kind,
		LastSyncAt:        st.LastSyncAt,
		TrackedCount:      len(localFiles),
		ModifiedSinceSync: dirty,
	}, nil
}

// SyncProject performs 3-way incremental sync between a local novel repo and any cloud Provider.
func SyncProject(ctx context.Context, repo *storage.ProjectRepo, prov Provider) (CloudSyncResult, error) {
	proj, err := repo.GetProject()
	if err != nil {
		return CloudSyncResult{}, err
	}
	slug := proj.Slug
	if slug == "" {
		slug = storage.Slugify(proj.Title, proj.ID)
	}

	localFiles, err := scanLocalNovel(repo.Dir)
	if err != nil {
		return CloudSyncResult{}, err
	}
	baseState := loadLocalState(repo.Dir, prov.Kind())

	manifest, err := prov.ReadManifest(ctx, slug)
	if err != nil {
		return CloudSyncResult{}, err
	}
	if manifest == nil {
		manifest = &SyncManifest{
			NovelSlug: slug,
			Title:     proj.Title,
			Files:     map[string]RemoteFileEntry{},
		}
	}
	if manifest.Files == nil {
		manifest.Files = map[string]RemoteFileEntry{}
	}

	pathSet := map[string]bool{}
	for p := range baseState.Files {
		pathSet[p] = true
	}
	for p := range localFiles {
		pathSet[p] = true
	}
	for p := range manifest.Files {
		pathSet[p] = true
	}
	var allPaths []string
	for p := range pathSet {
		if IsTrackedNovelFile(p) {
			allPaths = append(allPaths, p)
		}
	}
	sort.Strings(allPaths)

	now := time.Now().UTC()
	nextState := LocalSyncState{
		Provider:   prov.Kind(),
		LastSyncAt: now,
		Files:      map[string]LocalFileState{},
	}
	nextManifest := &SyncManifest{
		NovelSlug: slug,
		Title:     proj.Title,
		UpdatedAt: now,
		Files:     map[string]RemoteFileEntry{},
	}
	for k, v := range manifest.Files {
		nextManifest.Files[k] = v
	}

	var res CloudSyncResult
	res.Provider = prov.Kind()
	res.SyncedAt = now

	for _, relPath := range allPaths {
		baseEntry, hasBase := baseState.Files[relPath]
		localEntry, hasLocal := localFiles[relPath]
		remoteEntry, hasRemote := manifest.Files[relPath]

		baseHash := ""
		if hasBase {
			baseHash = baseEntry.SHA256
		}
		localChanged := (hasLocal != hasBase) || (hasLocal && hasBase && localEntry.Hash != baseHash)
		remoteChanged := (hasRemote != hasBase) || (hasRemote && hasBase && remoteEntry.SHA256 != baseHash)

		switch {
		case !localChanged && !remoteChanged:
			if hasLocal {
				rid := remoteEntry.RemoteID
				if rid == "" {
					rid = baseEntry.RemoteID
				}
				nextState.Files[relPath] = LocalFileState{SHA256: localEntry.Hash, RemoteID: rid}
				cacheBaseBytes(repo.Dir, localEntry.Hash, localEntry.Data)
			}

		case hasLocal && hasRemote && localEntry.Hash == remoteEntry.SHA256:
			nextState.Files[relPath] = LocalFileState{SHA256: localEntry.Hash, RemoteID: remoteEntry.RemoteID}
			cacheBaseBytes(repo.Dir, localEntry.Hash, localEntry.Data)

		case !hasLocal && !hasRemote:
			// deleted on both sides

		case localChanged && !remoteChanged:
			if hasLocal {
				rid, err := prov.UploadFile(ctx, slug, relPath, localEntry.Data, remoteEntry.RemoteID)
				if err != nil {
					return res, fmt.Errorf("upload %s: %w", relPath, err)
				}
				nextManifest.Files[relPath] = RemoteFileEntry{
					SHA256:    localEntry.Hash,
					Size:      localEntry.Size,
					RemoteID:  rid,
					UpdatedAt: now,
				}
				nextState.Files[relPath] = LocalFileState{SHA256: localEntry.Hash, RemoteID: rid}
				cacheBaseBytes(repo.Dir, localEntry.Hash, localEntry.Data)
				res.Uploaded++
			} else if hasRemote {
				if err := prov.DeleteFile(ctx, slug, relPath, remoteEntry.RemoteID); err != nil {
					return res, fmt.Errorf("delete remote %s: %w", relPath, err)
				}
				delete(nextManifest.Files, relPath)
				res.Deleted++
			}

		case !localChanged && remoteChanged:
			if hasRemote {
				data, err := prov.DownloadFile(ctx, slug, relPath, remoteEntry.RemoteID)
				if err != nil {
					return res, fmt.Errorf("download %s: %w", relPath, err)
				}
				snapshotChapterIfNeeded(repo, relPath, prov.Kind())
				target := filepath.Join(repo.Dir, filepath.FromSlash(relPath))
				if err := storage.WriteAtomic(target, data); err != nil {
					return res, err
				}
				h := sha256Hex(data)
				nextState.Files[relPath] = LocalFileState{SHA256: h, RemoteID: remoteEntry.RemoteID}
				cacheBaseBytes(repo.Dir, h, data)
				res.Downloaded++
			} else {
				snapshotChapterIfNeeded(repo, relPath, prov.Kind())
				_ = os.Remove(filepath.Join(repo.Dir, filepath.FromSlash(relPath)))
				res.Deleted++
			}

		default:
			if relPath == "inkweave.yaml" && hasLocal && hasRemote {
				if rBytes, dErr := prov.DownloadFile(ctx, slug, relPath, remoteEntry.RemoteID); dErr == nil {
					if mergedBytes, ok := tryAutoMergeProjectYAML(localEntry.Data, rBytes); ok {
						if err := storage.WriteAtomic(filepath.Join(repo.Dir, "inkweave.yaml"), mergedBytes); err == nil {
							rid, uErr := prov.UploadFile(ctx, slug, relPath, mergedBytes, remoteEntry.RemoteID)
							if uErr == nil {
								h := sha256Hex(mergedBytes)
								nextManifest.Files[relPath] = RemoteFileEntry{
									SHA256:    h,
									Size:      int64(len(mergedBytes)),
									RemoteID:  rid,
									UpdatedAt: now,
								}
								nextState.Files[relPath] = LocalFileState{SHA256: h, RemoteID: rid}
								cacheBaseBytes(repo.Dir, h, mergedBytes)
								continue
							}
						}
					}
				}
			}
			localText := ""
			if hasLocal {
				localText = string(localEntry.Data)
			}
			remoteText := ""
			if hasRemote {
				if rBytes, dErr := prov.DownloadFile(ctx, slug, relPath, remoteEntry.RemoteID); dErr == nil {
					remoteText = string(rBytes)
				}
			}
			baseText := readBaseCache(repo.Dir, baseHash)
			res.Conflicts = append(res.Conflicts, gitsync.ConflictFile{
				Path:   relPath,
				Base:   baseText,
				Local:  localText,
				Remote: remoteText,
			})
		}
	}

	if len(res.Conflicts) > 0 {
		_ = prov.WriteManifest(ctx, slug, nextManifest)
		_ = saveLocalState(repo.Dir, nextState)
		res.Status = "conflicts"
		res.Message = fmt.Sprintf("%d file(s) need conflict resolution (%s)", len(res.Conflicts), prov.Kind())
		return res, nil
	}

	if err := prov.WriteManifest(ctx, slug, nextManifest); err != nil {
		return res, fmt.Errorf("write remote manifest: %w", err)
	}
	if err := saveLocalState(repo.Dir, nextState); err != nil {
		return res, err
	}

	res.Status = "ok"
	if res.Uploaded == 0 && res.Downloaded == 0 && res.Deleted == 0 {
		res.Message = fmt.Sprintf("Already up to date (%s)", prov.Kind())
	} else {
		res.Message = fmt.Sprintf("Synced with %s: ↑%d uploaded, ↓%d downloaded, %d deleted",
			prov.Kind(), res.Uploaded, res.Downloaded, res.Deleted)
	}
	return res, nil
}

// ResolveConflict writes the chosen content locally and uploads it to the remote provider.
func ResolveConflict(ctx context.Context, repo *storage.ProjectRepo, prov Provider, relPath, resolvedContent string) error {
	proj, err := repo.GetProject()
	if err != nil {
		return err
	}
	slug := proj.Slug
	if slug == "" {
		slug = storage.Slugify(proj.Title, proj.ID)
	}

	snapshotChapterIfNeeded(repo, relPath, prov.Kind()+"-resolve")
	data := []byte(resolvedContent)
	target := filepath.Join(repo.Dir, filepath.FromSlash(relPath))
	if err := storage.WriteAtomic(target, data); err != nil {
		return err
	}

	manifest, err := prov.ReadManifest(ctx, slug)
	if err != nil {
		return err
	}
	if manifest == nil {
		manifest = &SyncManifest{NovelSlug: slug, Title: proj.Title, Files: map[string]RemoteFileEntry{}}
	}
	existingID := manifest.Files[relPath].RemoteID
	rid, err := prov.UploadFile(ctx, slug, relPath, data, existingID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	h := sha256Hex(data)
	manifest.UpdatedAt = now
	manifest.Files[relPath] = RemoteFileEntry{
		SHA256:    h,
		Size:      int64(len(data)),
		RemoteID:  rid,
		UpdatedAt: now,
	}
	if err := prov.WriteManifest(ctx, slug, manifest); err != nil {
		return err
	}

	st := loadLocalState(repo.Dir, prov.Kind())
	st.LastSyncAt = now
	st.Files[relPath] = LocalFileState{SHA256: h, RemoteID: rid}
	cacheBaseBytes(repo.Dir, h, data)
	return saveLocalState(repo.Dir, st)
}

// PullRemoteNovel downloads a complete novel from any cloud provider into the local Workspace.
func PullRemoteNovel(ctx context.Context, ws *storage.Workspace, prov Provider, slug string) (*storage.ProjectRepo, error) {
	manifest, err := prov.ReadManifest(ctx, slug)
	if err != nil {
		return nil, err
	}
	if manifest == nil {
		return nil, fmt.Errorf("remote novel %q not found on %s", slug, prov.Kind())
	}

	destDir := filepath.Join(ws.BaseDir(), storage.Slugify(slug, "cloud-novel"))
	if _, err := os.Stat(filepath.Join(destDir, "inkweave.yaml")); err == nil {
		repo, err := storage.OpenRepo(destDir)
		if err != nil {
			return nil, err
		}
		_, err = SyncProject(ctx, repo, prov)
		return repo, err
	}

	if err := os.MkdirAll(filepath.Join(destDir, ".inkweave", "snapshots"), 0o755); err != nil {
		return nil, err
	}

	st := LocalSyncState{
		Provider:   prov.Kind(),
		LastSyncAt: time.Now().UTC(),
		Files:      map[string]LocalFileState{},
	}

	for relPath, entry := range manifest.Files {
		if !IsTrackedNovelFile(relPath) {
			continue
		}
		data, err := prov.DownloadFile(ctx, slug, relPath, entry.RemoteID)
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", relPath, err)
		}
		target := filepath.Join(destDir, filepath.FromSlash(relPath))
		if err := storage.WriteAtomic(target, data); err != nil {
			return nil, err
		}
		h := sha256Hex(data)
		st.Files[relPath] = LocalFileState{SHA256: h, RemoteID: entry.RemoteID}
		cacheBaseBytes(destDir, h, data)
	}

	repo, err := storage.OpenRepo(destDir)
	if err != nil {
		return nil, err
	}
	_ = saveLocalState(destDir, st)
	return repo, nil
}

func snapshotChapterIfNeeded(repo *storage.ProjectRepo, relPath, reason string) {
	if !strings.HasPrefix(relPath, "manuscript/chapters/") || !strings.HasSuffix(relPath, ".md") {
		return
	}
	base := filepath.Base(relPath)
	if ch, err := repo.GetChapter(base); err == nil {
		_, _ = repo.CreateSnapshot(ch.ID, "before-"+reason)
	}
}

func tryAutoMergeProjectYAML(localBytes, remoteBytes []byte) ([]byte, bool) {
	var lp, rp model.Project
	if yaml.Unmarshal(localBytes, &lp) != nil || yaml.Unmarshal(remoteBytes, &rp) != nil {
		return nil, false
	}
	winner := lp
	if rp.UpdatedAt.After(lp.UpdatedAt) {
		winner = rp
	}
	// Normalize UpdatedAt to compare semantic fields cleanly
	lpCmp, rpCmp := lp, rp
	lpCmp.UpdatedAt, rpCmp.UpdatedAt = time.Time{}, time.Time{}
	b1, _ := yaml.Marshal(lpCmp)
	b2, _ := yaml.Marshal(rpCmp)
	if string(b1) != string(b2) {
		return nil, false
	}
	out, err := yaml.Marshal(winner)
	return out, err == nil
}
