package gitsync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestPureGoGitSyncAndConflictDetection(t *testing.T) {
	bareDir := filepath.Join(t.TempDir(), "remote.git")
	if _, err := git.PlainInitWithOptions(bareDir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
		Bare:        true,
	}); err != nil {
		t.Fatalf("init bare: %v", err)
	}

	// Device A (PC)
	pcDir := filepath.Join(t.TempDir(), "pc-novel")
	_ = os.MkdirAll(filepath.Join(pcDir, "manuscript", "chapters"), 0o755)
	_ = os.MkdirAll(filepath.Join(pcDir, ".inkweave"), 0o755)
	_ = os.WriteFile(filepath.Join(pcDir, ".inkweave", "index.db"), []byte("local-cache"), 0o644)
	_ = os.WriteFile(filepath.Join(pcDir, "manuscript", "chapters", "001-start.md"), []byte("Version 1\n"), 0o644)

	if err := Init(pcDir); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := SetRemote(pcDir, bareDir); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	if _, err := Commit(pcDir, "Initial chapter", "Author", "a@b.c"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := Push(pcDir, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Device B (Android) clones from bare repo
	phoneDir := filepath.Join(t.TempDir(), "phone-novel")
	if err := Clone(bareDir, phoneDir, ""); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Diverged offline edits to the same file
	_ = os.WriteFile(filepath.Join(pcDir, "manuscript", "chapters", "001-start.md"), []byte("PC edit line\n"), 0o644)
	_, _ = Commit(pcDir, "PC edit", "Author", "a@b.c")
	if err := Push(pcDir, ""); err != nil {
		t.Fatalf("PC push: %v", err)
	}

	_ = os.WriteFile(filepath.Join(phoneDir, "manuscript", "chapters", "001-start.md"), []byte("Phone edit line\n"), 0o644)
	res, err := Sync(phoneDir, "", "Phone edit", "Author", "a@b.c")
	if err != nil {
		t.Fatalf("Phone sync: %v", err)
	}
	if res.Status != "conflicts" || len(res.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict on diverged chapter, got %+v", res)
	}

	// Resolve conflict and push cleanly
	if _, err := ResolveConflict(phoneDir, res.Conflicts[0].Path, "Merged PC + Phone prose\n", "Author", "a@b.c"); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	if err := Push(phoneDir, ""); err != nil {
		t.Fatalf("Push after resolve: %v", err)
	}

	commits, err := Log(phoneDir, 10)
	if err != nil || len(commits) < 3 {
		t.Fatalf("expected commit history >=3, got %d", len(commits))
	}
}
