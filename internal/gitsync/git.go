package gitsync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/utils/merkletrie"
)

type StatusInfo struct {
	Initialized  bool     `json:"initialized"`
	Branch       string   `json:"branch"`
	RemoteURL    string   `json:"remote_url"`
	DirtyFiles   []string `json:"dirty_files"`
	HasConflicts bool     `json:"has_conflicts"`
}

type CommitInfo struct {
	Hash    string    `json:"hash"`
	Short   string    `json:"short"`
	Message string    `json:"message"`
	Author  string    `json:"author"`
	When    time.Time `json:"when"`
}

type ConflictFile struct {
	Path   string `json:"path"`
	Base   string `json:"base"`
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

type SyncResult struct {
	Status        string         `json:"status"` // ok | conflicts | pushed | pulled | up_to_date
	Message       string         `json:"message"`
	CommitHash    string         `json:"commit_hash,omitempty"`
	Conflicts     []ConflictFile `json:"conflicts,omitempty"`
	AppliedRemote []string       `json:"applied_remote,omitempty"`
}

func authMethod(token string) transport.AuthMethod {
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return &http.BasicAuth{
		Username: "x-access-token",
		Password: token,
	}
}

func ensureIgnore(dir string) {
	p := filepath.Join(dir, ".gitignore")
	if raw, err := os.ReadFile(p); err == nil {
		if !strings.Contains(string(raw), ".inkweave/") {
			_ = os.WriteFile(p, []byte(strings.TrimRight(string(raw), "\n")+"\n.inkweave/\n"), 0o644)
		}
		return
	}
	_ = os.WriteFile(p, []byte(".inkweave/\n*.tmp\n"), 0o644)
}

func Init(dir string) error {
	ensureIgnore(dir)
	_, err := git.PlainOpen(dir)
	if err == nil {
		return nil
	}
	_, err = git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
		Bare:        false,
	})
	return err
}

func SetRemote(dir, remoteURL string) error {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return err
	}
	_ = repo.DeleteRemote("origin")
	if strings.TrimSpace(remoteURL) == "" {
		return nil
	}
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{strings.TrimSpace(remoteURL)},
	})
	return err
}

func Status(dir string) (StatusInfo, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return StatusInfo{Initialized: false, Branch: "main"}, nil
		}
		return StatusInfo{}, err
	}
	info := StatusInfo{Initialized: true, Branch: "main"}
	if head, err := repo.Head(); err == nil && head.Name().IsBranch() {
		info.Branch = head.Name().Short()
	}
	if rem, err := repo.Remote("origin"); err == nil && len(rem.Config().URLs) > 0 {
		info.RemoteURL = rem.Config().URLs[0]
	}
	wt, err := repo.Worktree()
	if err != nil {
		return info, nil
	}
	st, err := wt.Status()
	if err != nil {
		return info, nil
	}
	for path, fs := range st {
		if strings.HasPrefix(path, ".inkweave/") || path == ".inkweave" {
			continue
		}
		if fs.Worktree != git.Unmodified || fs.Staging != git.Unmodified {
			info.DirtyFiles = append(info.DirtyFiles, path)
		}
	}
	return info, nil
}

func SmartCommitMessage(dir string) string {
	st, err := Status(dir)
	if err != nil || len(st.DirtyFiles) == 0 {
		return "Update novel manuscript"
	}
	if len(st.DirtyFiles) == 1 {
		return "Update " + st.DirtyFiles[0]
	}
	return fmt.Sprintf("Update %d novel files (%s)", len(st.DirtyFiles), st.DirtyFiles[0])
}

func Commit(dir, message, authorName, authorEmail string) (string, error) {
	ensureIgnore(dir)
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return "", err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	st, err := wt.Status()
	if err != nil {
		return "", err
	}
	staged := 0
	for path, fs := range st {
		if strings.HasPrefix(path, ".inkweave/") || path == ".inkweave" {
			continue
		}
		if fs.Worktree == git.Deleted {
			_, _ = wt.Remove(path)
			staged++
		} else if fs.Worktree != git.Unmodified || fs.Staging != git.Unmodified {
			_, _ = wt.Add(path)
			staged++
		}
	}
	if staged == 0 {
		head, err := repo.Head()
		if err != nil {
			return "", nil
		}
		return head.Hash().String(), nil
	}
	if strings.TrimSpace(message) == "" {
		message = SmartCommitMessage(dir)
	}
	if authorName == "" {
		authorName = "Inkweave Author"
	}
	if authorEmail == "" {
		authorEmail = "author@inkweave.local"
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

func Push(dir, token string) error {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return err
	}
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       authMethod(token),
	})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

// Pull fetches origin and fast-forwards when possible, or detects diverged edits and returns
// structured ConflictFile entries for any file modified on both sides.
func Pull(dir, token, authorName, authorEmail string) (SyncResult, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return SyncResult{}, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return SyncResult{}, err
	}

	branchName := "main"
	if head, hErr := repo.Head(); hErr == nil && head.Name().IsBranch() {
		branchName = head.Name().Short()
	}

	pullErr := wt.Pull(&git.PullOptions{
		RemoteName:    "origin",
		ReferenceName: plumbing.NewBranchReferenceName(branchName),
		Auth:          authMethod(token),
	})
	if pullErr == nil {
		return SyncResult{Status: "pulled", Message: "Pulled latest changes from remote"}, nil
	}
	if errors.Is(pullErr, git.NoErrAlreadyUpToDate) {
		return SyncResult{Status: "up_to_date", Message: "Already up to date"}, nil
	}
	if errors.Is(pullErr, transport.ErrEmptyRemoteRepository) {
		return SyncResult{Status: "up_to_date", Message: "Remote repository is empty"}, nil
	}

	// Handle diverged history / non-fast-forward cleanly
	return handleDivergedSync(repo, wt, branchName, token, authorName, authorEmail)
}

func handleDivergedSync(repo *git.Repository, wt *git.Worktree, branchName, token, authorName, authorEmail string) (SyncResult, error) {
	err := repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Auth:       authMethod(token),
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return SyncResult{}, err
	}

	headRef, err := repo.Head()
	if err != nil {
		return SyncResult{}, err
	}
	localCommit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return SyncResult{}, err
	}

	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", branchName), true)
	if err != nil {
		return SyncResult{}, err
	}
	remoteCommit, err := repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return SyncResult{}, err
	}

	bases, err := localCommit.MergeBase(remoteCommit)
	if err != nil || len(bases) == 0 {
		return SyncResult{}, fmt.Errorf("no common merge ancestor found")
	}
	baseCommit := bases[0]
	baseTree, _ := baseCommit.Tree()
	localTree, _ := localCommit.Tree()
	remoteTree, _ := remoteCommit.Tree()

	localChanges, _ := object.DiffTree(baseTree, localTree)
	remoteChanges, _ := object.DiffTree(baseTree, remoteTree)

	localTouched := map[string]bool{}
	for _, ch := range localChanges {
		localTouched[changePath(ch)] = true
	}

	var conflicts []ConflictFile
	var applied []string

	for _, ch := range remoteChanges {
		p := changePath(ch)
		if p == "" || strings.HasPrefix(p, ".inkweave/") {
			continue
		}
		if localTouched[p] {
			// Both sides changed this file!
			bContent := readTreeFile(baseTree, p)
			lContent := readTreeFile(localTree, p)
			rContent := readTreeFile(remoteTree, p)
			if lContent != rContent {
				conflicts = append(conflicts, ConflictFile{
					Path:   p,
					Base:   bContent,
					Local:  lContent,
					Remote: rContent,
				})
				continue
			}
		}
		// Non-conflicting remote change: apply to working tree
		action, _ := ch.Action()
		targetPath := filepath.Join(wt.Filesystem.Root(), filepath.FromSlash(p))
		if action == merkletrie.Delete {
			_ = os.Remove(targetPath)
			_, _ = wt.Remove(p)
		} else {
			rContent := readTreeFile(remoteTree, p)
			_ = os.MkdirAll(filepath.Dir(targetPath), 0o755)
			_ = os.WriteFile(targetPath, []byte(rContent), 0o644)
			_, _ = wt.Add(p)
		}
		applied = append(applied, p)
	}

	if len(conflicts) > 0 {
		return SyncResult{
			Status:        "conflicts",
			Message:       fmt.Sprintf("%d file(s) need conflict resolution", len(conflicts)),
			Conflicts:     conflicts,
			AppliedRemote: applied,
		}, nil
	}

	// Clean merge commit recording both parents
	if authorName == "" {
		authorName = "Inkweave Author"
	}
	if authorEmail == "" {
		authorEmail = "author@inkweave.local"
	}
	hash, err := wt.Commit("Merge remote changes", &git.CommitOptions{
		Author:  &object.Signature{Name: authorName, Email: authorEmail, When: time.Now()},
		Parents: []plumbing.Hash{localCommit.Hash, remoteCommit.Hash},
	})
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{
		Status:        "pulled",
		Message:       "Merged remote changes cleanly",
		CommitHash:    hash.String(),
		AppliedRemote: applied,
	}, nil
}

// ResolveConflict writes the chosen merged file and creates a merge commit linking HEAD and origin/<branch>.
func ResolveConflict(dir, path, resolvedContent, authorName, authorEmail string) (string, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return "", err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, []byte(resolvedContent), 0o644); err != nil {
		return "", err
	}
	_, _ = wt.Add(path)

	headRef, err := repo.Head()
	if err != nil {
		return "", err
	}
	parents := []plumbing.Hash{headRef.Hash()}
	branch := "main"
	if headRef.Name().IsBranch() {
		branch = headRef.Name().Short()
	}
	if remRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", branch), true); err == nil {
		if remRef.Hash() != headRef.Hash() {
			parents = append(parents, remRef.Hash())
		}
	}
	if authorName == "" {
		authorName = "Inkweave Author"
	}
	if authorEmail == "" {
		authorEmail = "author@inkweave.local"
	}
	h, err := wt.Commit("Resolve conflict in "+path, &git.CommitOptions{
		Author:  &object.Signature{Name: authorName, Email: authorEmail, When: time.Now()},
		Parents: parents,
	})
	if err != nil {
		return "", err
	}
	return h.String(), nil
}

// Sync runs commit (if dirty) -> pull (handling conflicts) -> push.
func Sync(dir, token, message, authorName, authorEmail string) (SyncResult, error) {
	if err := Init(dir); err != nil {
		return SyncResult{}, err
	}
	st, err := Status(dir)
	if err != nil {
		return SyncResult{}, err
	}
	if len(st.DirtyFiles) > 0 {
		if _, err := Commit(dir, message, authorName, authorEmail); err != nil {
			return SyncResult{}, err
		}
	}
	if st.RemoteURL == "" {
		return SyncResult{Status: "ok", Message: "Committed locally (no remote configured)"}, nil
	}
	pullRes, err := Pull(dir, token, authorName, authorEmail)
	if err != nil {
		return SyncResult{}, err
	}
	if pullRes.Status == "conflicts" {
		return pullRes, nil
	}
	if err := Push(dir, token); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Status: "pushed", Message: "Synced & pushed to remote", CommitHash: pullRes.CommitHash}, nil
}

func Log(dir string, limit int) ([]CommitInfo, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return []CommitInfo{}, nil
	}
	if limit <= 0 {
		limit = 25
	}
	iter, err := repo.Log(&git.LogOptions{})
	if err != nil {
		return []CommitInfo{}, nil
	}
	var out []CommitInfo
	_ = iter.ForEach(func(c *object.Commit) error {
		if len(out) >= limit {
			return errors.New("limit")
		}
		h := c.Hash.String()
		out = append(out, CommitInfo{
			Hash:    h,
			Short:   h[:7],
			Message: strings.TrimSpace(c.Message),
			Author:  c.Author.Name,
			When:    c.Author.When,
		})
		return nil
	})
	return out, nil
}

func FileAtCommit(dir, commitHash, relPath string) (string, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return "", err
	}
	c, err := repo.CommitObject(plumbing.NewHash(commitHash))
	if err != nil {
		return "", err
	}
	tree, err := c.Tree()
	if err != nil {
		return "", err
	}
	f, err := tree.File(filepath.ToSlash(relPath))
	if err != nil {
		return "", err
	}
	return f.Contents()
}

func Clone(remoteURL, destDir, token string) error {
	_, err := git.PlainClone(destDir, false, &git.CloneOptions{
		URL:  strings.TrimSpace(remoteURL),
		Auth: authMethod(token),
	})
	return err
}

func changePath(ch *object.Change) string {
	if ch.To.Name != "" {
		return ch.To.Name
	}
	return ch.From.Name
}

func readTreeFile(tree *object.Tree, path string) string {
	if tree == nil {
		return ""
	}
	f, err := tree.File(path)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	rc, err := f.Reader()
	if err != nil {
		return ""
	}
	defer rc.Close()
	_, _ = buf.ReadFrom(rc)
	return buf.String()
}
