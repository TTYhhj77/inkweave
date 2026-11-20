package memory

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/storage"
	_ "modernc.org/sqlite"
)

// IndexDB wraps the rebuildable pure-Go SQLite FTS5 cache stored at .inkweave/index.db.
type IndexDB struct {
	db     *sql.DB
	dbPath string
}

// OpenIndex opens (or initializes) .inkweave/index.db for a novel repository.
func OpenIndex(repoDir string) (*IndexDB, error) {
	inkDir := filepath.Join(repoDir, ".inkweave")
	if err := os.MkdirAll(inkDir, 0o755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(inkDir, "index.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, err
	}

	idx := &IndexDB{db: db, dbPath: dbPath}
	if err := idx.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return idx, nil
}

func (idx *IndexDB) Close() error {
	if idx.db == nil {
		return nil
	}
	return idx.db.Close()
}

func (idx *IndexDB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			doc_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			ref_id TEXT NOT NULL,
			title TEXT NOT NULL,
			aliases TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS docs_fts_word USING fts5(
			doc_id UNINDEXED,
			title,
			aliases,
			body,
			tokenize = 'unicode61'
		);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS docs_fts_trigram USING fts5(
			doc_id UNINDEXED,
			title,
			aliases,
			body,
			tokenize = 'trigram'
		);`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			chapter_id TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			tool_events_json TEXT NOT NULL DEFAULT '',
			mode TEXT NOT NULL DEFAULT 'balanced',
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_chat_session ON chat_messages(session_id, created_at);`,
		`CREATE TABLE IF NOT EXISTS token_ledger (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			mode TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			stage TEXT NOT NULL,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);`,
	}
	for _, s := range stmts {
		if _, err := idx.db.Exec(s); err != nil {
			return fmt.Errorf("migrate sql %q: %w", s[:minInt(len(s), 48)], err)
		}
	}
	return nil
}

type indexDoc struct {
	DocID   string
	Kind    string
	RefID   string
	Title   string
	Aliases string
	Body    string
	Updated string
}

// ReindexProject deterministically rebuilds the FTS5 index from Markdown/YAML source files.
func (idx *IndexDB) ReindexProject(repo *storage.ProjectRepo) error {
	var docs []indexDoc
	now := time.Now().UTC().Format(time.RFC3339)

	chapters, err := repo.ListChapters()
	if err != nil {
		return err
	}
	for _, ch := range chapters {
		docs = append(docs, indexDoc{
			DocID:   "chapter:" + ch.ID,
			Kind:    "chapter",
			RefID:   ch.ID,
			Title:   ch.Title,
			Aliases: strings.Join([]string{ch.Slug, ch.POV, ch.TimelineDay}, " | "),
			Body:    strings.TrimSpace(ch.Summary + "\n\n" + ch.Content),
			Updated: ch.UpdatedAt.Format(time.RFC3339),
		})
	}

	chars, err := repo.ListCharacters()
	if err != nil {
		return err
	}
	for _, c := range chars {
		bodyParts := []string{
			c.Summary,
			"Paradox: " + c.CoreParadox,
			"Mask: " + c.Mask,
			"TrueSelf: " + c.TrueSelf,
			"BlindSpot: " + c.BlindSpot,
			"Wound: " + c.FormativeWound,
			"Misbelief: " + c.Misbelief,
			"Want: " + c.ExternalWant,
			"Need: " + c.InternalNeed,
			"Voice: " + c.VoiceGuide,
			strings.Join(c.SpeechExamples, " | "),
			strings.Join(c.Secrets, " | "),
			c.BioMarkdown,
		}
		docs = append(docs, indexDoc{
			DocID:   "character:" + c.ID,
			Kind:    "character",
			RefID:   c.ID,
			Title:   c.Name,
			Aliases: strings.Join(c.Aliases, " | "),
			Body:    strings.Join(nonEmpty(bodyParts), "\n"),
			Updated: c.UpdatedAt.Format(time.RFC3339),
		})
	}

	lore, err := repo.ListLore("all")
	if err != nil {
		return err
	}
	for _, item := range lore {
		var attrPairs []string
		for k, v := range item.Attributes {
			attrPairs = append(attrPairs, k+": "+v)
		}
		docs = append(docs, indexDoc{
			DocID:   item.Category + ":" + item.ID,
			Kind:    singularKind(item.Category),
			RefID:   item.ID,
			Title:   item.Name,
			Aliases: strings.Join(append(item.Aliases, item.Tags...), " | "),
			Body:    strings.Join(nonEmpty([]string{item.Summary, strings.Join(attrPairs, "\n"), item.Content}), "\n"),
			Updated: item.UpdatedAt.Format(time.RFC3339),
		})
	}

	timeline, _ := repo.GetTimeline()
	for _, ev := range timeline {
		docs = append(docs, indexDoc{
			DocID:   "timeline:" + ev.ID,
			Kind:    "timeline",
			RefID:   ev.ID,
			Title:   ev.DayLabel + " — " + ev.Location,
			Aliases: strings.Join(ev.Participants, " | "),
			Body:    ev.Summary + "\n" + ev.Impact,
			Updated: now,
		})
	}

	threads, _ := repo.GetPlotThreads()
	for _, th := range threads {
		docs = append(docs, indexDoc{
			DocID:   "thread:" + th.ID,
			Kind:    "thread",
			RefID:   th.ID,
			Title:   th.Title,
			Aliases: strings.Join(append([]string{th.Type, th.Status}, th.RelatedCharacters...), " | "),
			Body:    th.Notes,
			Updated: now,
		})
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, clr := range []string{
		`DELETE FROM documents;`,
		`DELETE FROM docs_fts_word;`,
		`DELETE FROM docs_fts_trigram;`,
	} {
		if _, err := tx.Exec(clr); err != nil {
			return err
		}
	}

	insDoc, err := tx.Prepare(`INSERT INTO documents(doc_id, kind, ref_id, title, aliases, body, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insDoc.Close()

	insWord, err := tx.Prepare(`INSERT INTO docs_fts_word(doc_id, title, aliases, body) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insWord.Close()

	insTri, err := tx.Prepare(`INSERT INTO docs_fts_trigram(doc_id, title, aliases, body) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insTri.Close()

	for _, d := range docs {
		if _, err := insDoc.Exec(d.DocID, d.Kind, d.RefID, d.Title, d.Aliases, d.Body, d.Updated); err != nil {
			return err
		}
		if _, err := insWord.Exec(d.DocID, d.Title, d.Aliases, d.Body); err != nil {
			return err
		}
		if _, err := insTri.Exec(d.DocID, d.Title, d.Aliases, d.Body); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Search performs Thai-aware hybrid search combining FTS5 trigram, FTS5 unicode61 word match,
// and direct substring/alias matching with Reciprocal Rank Fusion.
func (idx *IndexDB) Search(query string, kinds []string, limit int) ([]model.SearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []model.SearchHit{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	kindFilter := map[string]bool{}
	for _, k := range kinds {
		if k != "" {
			kindFilter[k] = true
		}
	}

	type accHit struct {
		hit   model.SearchHit
		score float64
	}
	merged := map[string]*accHit{}

	addRank := func(docID, kind, refID, title, body string, rankIdx int, weight float64) {
		if len(kindFilter) > 0 && !kindFilter[kind] {
			return
		}
		rrf := weight / float64(10+rankIdx)
		if existing, ok := merged[docID]; ok {
			existing.score += rrf
			return
		}
		merged[docID] = &accHit{
			hit: model.SearchHit{
				DocID:   docID,
				Kind:    kind,
				RefID:   refID,
				Title:   title,
				Snippet: buildSnippet(body, query, 180),
			},
			score: rrf,
		}
	}

	// 1. Direct substring & alias matching (catches short 2-char names & exact Thai phrases)
	likePat := "%" + query + "%"
	rows, err := idx.db.Query(`
		SELECT doc_id, kind, ref_id, title, body
		FROM documents
		WHERE title LIKE ? OR aliases LIKE ? OR body LIKE ?
		LIMIT 40
	`, likePat, likePat, likePat)
	if err == nil {
		rank := 0
		for rows.Next() {
			var docID, kind, refID, title, body string
			if rows.Scan(&docID, &kind, &refID, &title, &body) == nil {
				addRank(docID, kind, refID, title, body, rank, 1.4)
				rank++
			}
		}
		_ = rows.Close()
	}

	// 2. Trigram FTS5 (crucial for unspaced Thai phrases >= 3 runes)
	if utf8.RuneCountInString(query) >= 3 {
		quoted := quoteFTSPhrase(query)
		tRows, tErr := idx.db.Query(`
			SELECT d.doc_id, d.kind, d.ref_id, d.title, d.body
			FROM docs_fts_trigram f
			JOIN documents d ON d.doc_id = f.doc_id
			WHERE docs_fts_trigram MATCH ?
			ORDER BY bm25(docs_fts_trigram)
			LIMIT 30
		`, quoted)
		if tErr == nil {
			rank := 0
			for tRows.Next() {
				var docID, kind, refID, title, body string
				if tRows.Scan(&docID, &kind, &refID, &title, &body) == nil {
					addRank(docID, kind, refID, title, body, rank, 1.3)
					rank++
				}
			}
			_ = tRows.Close()
		}
	}

	// 3. Word FTS5 (spaced / multi-keyword English & mixed queries)
	tokens := splitQueryTerms(query)
	if len(tokens) > 0 {
		ftsQuery := strings.Join(tokens, " OR ")
		wRows, wErr := idx.db.Query(`
			SELECT d.doc_id, d.kind, d.ref_id, d.title, d.body
			FROM docs_fts_word f
			JOIN documents d ON d.doc_id = f.doc_id
			WHERE docs_fts_word MATCH ?
			ORDER BY bm25(docs_fts_word)
			LIMIT 30
		`, ftsQuery)
		if wErr == nil {
			rank := 0
			for wRows.Next() {
				var docID, kind, refID, title, body string
				if wRows.Scan(&docID, &kind, &refID, &title, &body) == nil {
					addRank(docID, kind, refID, title, body, rank, 1.0)
					rank++
				}
			}
			_ = wRows.Close()
		}
	}

	res := make([]model.SearchHit, 0, len(merged))
	for _, a := range merged {
		a.hit.Score = a.score
		res = append(res, a.hit)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Score > res[j].Score
	})
	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

// --- Chat History & Token Ledger ---

func (idx *IndexDB) SaveChatMessage(msg model.ChatMessage) error {
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	_, err := idx.db.Exec(`
		INSERT OR REPLACE INTO chat_messages(id, session_id, chapter_id, role, content, tool_events_json, mode, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.ID, msg.SessionID, msg.ChapterID, msg.Role, msg.Content, msg.ToolEvents, string(msg.Mode), msg.CreatedAt.Format(time.RFC3339))
	return err
}

func (idx *IndexDB) ListChatMessages(sessionID string, limit int) ([]model.ChatMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := idx.db.Query(`
		SELECT id, session_id, chapter_id, role, content, tool_events_json, mode, created_at
		FROM chat_messages
		WHERE session_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.ChatMessage
	for rows.Next() {
		var m model.ChatMessage
		var modeStr, ts string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.ChapterID, &m.Role, &m.Content, &m.ToolEvents, &modeStr, &ts); err != nil {
			continue
		}
		m.Mode = model.AgentMode(modeStr)
		m.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, m)
	}
	// Reverse to chronological order
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (idx *IndexDB) ClearChatMessages(sessionID string) error {
	_, err := idx.db.Exec(`DELETE FROM chat_messages WHERE session_id = ?`, sessionID)
	return err
}

func (idx *IndexDB) RecordTokenUsage(entry model.TokenLedgerEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	_, err := idx.db.Exec(`
		INSERT INTO token_ledger(id, project_id, mode, provider, model, stage, input_tokens, cached_tokens, output_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ID, entry.ProjectID, string(entry.Mode), entry.Provider, entry.Model, entry.Stage, entry.InputTokens, entry.CachedTokens, entry.OutputTokens, entry.CreatedAt.Format(time.RFC3339))
	return err
}

func (idx *IndexDB) ListTokenUsage(projectID string, limit int) ([]model.TokenLedgerEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := idx.db.Query(`
		SELECT id, project_id, mode, provider, model, stage, input_tokens, cached_tokens, output_tokens, created_at
		FROM token_ledger
		WHERE project_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.TokenLedgerEntry
	for rows.Next() {
		var e model.TokenLedgerEntry
		var modeStr, ts string
		if rows.Scan(&e.ID, &e.ProjectID, &modeStr, &e.Provider, &e.Model, &e.Stage, &e.InputTokens, &e.CachedTokens, &e.OutputTokens, &ts) == nil {
			e.Mode = model.AgentMode(modeStr)
			e.CreatedAt, _ = time.Parse(time.RFC3339, ts)
			out = append(out, e)
		}
	}
	return out, nil
}

func quoteFTSPhrase(q string) string {
	escaped := strings.ReplaceAll(q, `"`, `""`)
	return `"` + escaped + `"`
}

func splitQueryTerms(q string) []string {
	raw := strings.Fields(q)
	var out []string
	for _, r := range raw {
		clean := strings.Trim(r, `"'`+"`.,;:!?()[]{}")
		if utf8.RuneCountInString(clean) >= 2 {
			out = append(out, quoteFTSPhrase(clean))
		}
	}
	return out
}

func buildSnippet(body, query string, maxRunes int) string {
	body = strings.Join(strings.Fields(body), " ")
	if body == "" {
		return ""
	}
	runes := []rune(body)
	if len(runes) <= maxRunes {
		return body
	}
	lowerBody := strings.ToLower(body)
	lowerQ := strings.ToLower(query)
	byteIdx := strings.Index(lowerBody, lowerQ)
	if byteIdx < 0 {
		return string(runes[:maxRunes]) + "…"
	}
	runeIdx := utf8.RuneCountInString(lowerBody[:byteIdx])
	start := runeIdx - maxRunes/3
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	suffix := ""
	if end < len(runes) {
		suffix = "…"
	}
	return prefix + string(runes[start:end]) + suffix
}

func singularKind(cat string) string {
	switch cat {
	case "locations":
		return "location"
	case "factions":
		return "faction"
	case "items":
		return "item"
	case "rules":
		return "rule"
	default:
		return cat
	}
}

func nonEmpty(parts []string) []string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
