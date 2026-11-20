package exporter

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/inkweave/inkweave/internal/storage"
)

// ExportMarkdown compiles the novel manuscript and optional character/world bible into Markdown.
func ExportMarkdown(repo *storage.ProjectRepo, includeBible bool) ([]byte, error) {
	proj, err := repo.GetProject()
	if err != nil {
		return nil, err
	}
	chapters, err := repo.ListChapters()
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", proj.Title)
	if proj.Author != "" {
		fmt.Fprintf(&b, "*โดย %s*\n\n", proj.Author)
	}
	if proj.Synopsis != "" {
		fmt.Fprintf(&b, "> %s\n\n", proj.Synopsis)
	}
	b.WriteString("---\n\n")

	for _, ch := range chapters {
		fmt.Fprintf(&b, "## %s\n\n", ch.Title)
		b.WriteString(strings.TrimSpace(ch.Content))
		b.WriteString("\n\n---\n\n")
	}

	if includeBible {
		chars, _ := repo.ListCharacters()
		if len(chars) > 0 {
			b.WriteString("# Story Bible — Characters\n\n")
			for _, c := range chars {
				fmt.Fprintf(&b, "## %s (%s)\n", c.Name, c.Role)
				if c.CoreParadox != "" {
					fmt.Fprintf(&b, "- **Core Paradox**: %s\n", c.CoreParadox)
				}
				if c.Summary != "" {
					fmt.Fprintf(&b, "\n%s\n", c.Summary)
				}
				b.WriteString("\n")
			}
		}
	}

	return []byte(b.String()), nil
}

// ExportEPUB generates a spec-compliant EPUB 3 archive using stdlib archive/zip.
// Entry #0 is strictly "mimetype" stored uncompressed (zip.Store).
func ExportEPUB(repo *storage.ProjectRepo) ([]byte, error) {
	proj, err := repo.GetProject()
	if err != nil {
		return nil, err
	}
	chapters, err := repo.ListChapters()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// 1. Mandatory uncompressed mimetype as very first entry
	mtHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	w, err := zw.CreateHeader(mtHeader)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte("application/epub+zip")); err != nil {
		return nil, err
	}

	// 2. META-INF/container.xml
	if err := addZipText(zw, "META-INF/container.xml", `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`); err != nil {
		return nil, err
	}

	// 3. OEBPS/styles.css (Thai-safe generous line height)
	if err := addZipText(zw, "OEBPS/styles.css", `body {
  font-family: "Sarabun", "Noto Serif Thai", Georgia, serif;
  line-height: 1.85;
  margin: 1.5em;
  color: #1e1d1b;
}
h1, h2 { line-height: 1.4; margin-bottom: 0.8em; }
p { text-indent: 1.5em; margin: 0 0 0.9em 0; }
`); err != nil {
		return nil, err
	}

	lang := proj.Language
	if lang == "" {
		lang = "th"
	}

	var manifestItems strings.Builder
	var spineItems strings.Builder
	var navItems strings.Builder

	for i, ch := range chapters {
		fname := fmt.Sprintf("chapter-%03d.xhtml", i+1)
		id := fmt.Sprintf("ch%03d", i+1)
		fmt.Fprintf(&manifestItems, `    <item id="%s" href="%s" media-type="application/xhtml+xml"/>`+"\n", id, fname)
		fmt.Fprintf(&spineItems, `    <itemref idref="%s"/>`+"\n", id)
		fmt.Fprintf(&navItems, `      <li><a href="%s">%s</a></li>`+"\n", fname, html.EscapeString(ch.Title))

		var paras strings.Builder
		for _, p := range strings.Split(strings.ReplaceAll(ch.Content, "\r\n", "\n"), "\n\n") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			escaped := strings.ReplaceAll(html.EscapeString(p), "\n", "<br/>")
			fmt.Fprintf(&paras, "  <p>%s</p>\n", escaped)
		}

		xhtml := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="%s" lang="%s">
<head>
  <meta charset="utf-8"/>
  <title>%s</title>
  <link rel="stylesheet" type="text/css" href="styles.css"/>
</head>
<body>
  <h1>%s</h1>
%s</body>
</html>`, html.EscapeString(lang), html.EscapeString(lang), html.EscapeString(ch.Title), html.EscapeString(ch.Title), paras.String())

		if err := addZipText(zw, "OEBPS/"+fname, xhtml); err != nil {
			return nil, err
		}
	}

	navXHTML := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="%s" lang="%s">
<head><meta charset="utf-8"/><title>%s</title><link rel="stylesheet" type="text/css" href="styles.css"/></head>
<body>
  <nav epub:type="toc" id="toc">
    <h1>%s</h1>
    <ol>
%s    </ol>
  </nav>
</body>
</html>`, html.EscapeString(lang), html.EscapeString(lang), html.EscapeString(proj.Title), html.EscapeString(proj.Title), navItems.String())
	if err := addZipText(zw, "OEBPS/nav.xhtml", navXHTML); err != nil {
		return nil, err
	}

	author := proj.Author
	if author == "" {
		author = "Inkweave Author"
	}
	opf := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">urn:uuid:%s</dc:identifier>
    <dc:title>%s</dc:title>
    <dc:creator>%s</dc:creator>
    <dc:language>%s</dc:language>
    <meta property="dcterms:modified">%s</meta>
  </metadata>
  <manifest>
    <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    <item id="css" href="styles.css" media-type="text/css"/>
%s  </manifest>
  <spine>
%s  </spine>
</package>`, html.EscapeString(proj.ID), html.EscapeString(proj.Title), html.EscapeString(author), html.EscapeString(lang),
		proj.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"), manifestItems.String(), spineItems.String())
	if err := addZipText(zw, "OEBPS/content.opf", opf); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func addZipText(zw *zip.Writer, name, content string) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte(content))
	return err
}
