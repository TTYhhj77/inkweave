package exporter

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/inkweave/inkweave/internal/storage"
)

func TestExportEPUB3AndMarkdown(t *testing.T) {
	ws, err := storage.NewWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	repo, err := ws.CreateStarterNovel()
	if err != nil {
		t.Fatalf("CreateStarterNovel: %v", err)
	}

	mdBytes, err := ExportMarkdown(repo, true)
	if err != nil || !strings.Contains(string(mdBytes), "คิริน") {
		t.Fatalf("ExportMarkdown failed: %v", err)
	}

	epubBytes, err := ExportEPUB(repo)
	if err != nil {
		t.Fatalf("ExportEPUB: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(epubBytes), int64(len(epubBytes)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	if len(zr.File) < 5 {
		t.Fatalf("expected >=5 entries in epub")
	}
	// Spec check: entry 0 MUST be uncompressed mimetype
	first := zr.File[0]
	if first.Name != "mimetype" || first.Method != zip.Store {
		t.Fatalf("first entry must be stored mimetype, got name=%s method=%d", first.Name, first.Method)
	}
	rc, _ := first.Open()
	mt, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(mt) != "application/epub+zip" {
		t.Fatalf("unexpected mimetype: %q", string(mt))
	}
}
