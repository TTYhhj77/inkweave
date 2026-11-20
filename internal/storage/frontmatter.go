package storage

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var nonSlugASCII = regexp.MustCompile(`[^a-z0-9\-]+`)

// WriteAtomic safely writes bytes to a temp file and renames it into place.
func WriteAtomic(target string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// MarshalFrontmatter serializes any struct/map into YAML frontmatter + UTF-8 Markdown body.
func MarshalFrontmatter(meta any, body string) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(meta)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	if !bytes.HasSuffix(yamlBytes, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString("---\n")
	body = strings.TrimLeft(body, "\r\n")
	if body != "" {
		buf.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes(), nil
}

// ParseFrontmatter splits `---` YAML frontmatter from Markdown body.
func ParseFrontmatter(raw []byte, out any) (string, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return text, nil
	}
	rest := text[4:]
	idx := strings.Index(rest, "\n---\n")
	endIdx := idx
	bodyOffset := 5
	if idx < 0 {
		if strings.HasSuffix(rest, "\n---") {
			endIdx = len(rest) - 4
			bodyOffset = 4
		} else {
			return text, nil
		}
	}
	yamlPart := rest[:endIdx]
	body := ""
	if endIdx+bodyOffset <= len(rest) {
		body = strings.TrimLeft(rest[endIdx+bodyOffset:], "\n")
	}
	if err := yaml.Unmarshal([]byte(yamlPart), out); err != nil {
		return "", fmt.Errorf("parse yaml frontmatter: %w", err)
	}
	return strings.TrimRight(body, "\n"), nil
}

// Slugify produces a safe, readable filename slug supporting both Latin and Thai titles.
func Slugify(input string, fallback string) string {
	s := strings.TrimSpace(strings.ToLower(input))
	if s == "" {
		return fallback
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	ascii := strings.Trim(nonSlugASCII.ReplaceAllString(b.String(), "-"), "-")
	if ascii != "" {
		return ascii
	}
	// If purely non-ASCII (e.g., Thai title), generate a deterministic short hash or fallback
	var thaiSafe strings.Builder
	for _, r := range strings.TrimSpace(input) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || (r >= 0x0E00 && r <= 0x0E7F) {
			thaiSafe.WriteRune(r)
			lastDash = false
		} else if unicode.IsSpace(r) || r == '-' {
			if !lastDash && thaiSafe.Len() > 0 {
				thaiSafe.WriteByte('-')
				lastDash = true
			}
		}
		if utf8.RuneCountInString(thaiSafe.String()) >= 40 {
			break
		}
	}
	res := strings.Trim(thaiSafe.String(), "-")
	if res == "" {
		return fallback
	}
	return res
}

// CountProseMetrics estimates word count and character count accurately for both Thai and English prose.
// For Thai (unspaced script), it counts space-delimited words plus ~4.5 Thai characters per Thai word.
func CountProseMetrics(text string) (words int, chars int) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, 0
	}
	thaiChars := 0
	nonSpaceChars := 0
	latinTokens := 0
	inLatinToken := false

	for _, r := range trimmed {
		if !unicode.IsSpace(r) {
			nonSpaceChars++
		}
		if r >= 0x0E00 && r <= 0x0E7F {
			thaiChars++
			if inLatinToken {
				inLatinToken = false
			}
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inLatinToken {
				latinTokens++
				inLatinToken = true
			}
		} else {
			inLatinToken = false
		}
	}
	thaiWords := 0
	if thaiChars > 0 {
		thaiWords = (thaiChars + 3) / 4
	}
	return latinTokens + thaiWords, nonSpaceChars
}
