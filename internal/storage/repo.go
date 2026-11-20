package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/inkweave/inkweave/internal/model"
	"gopkg.in/yaml.v3"
)

var (
	ErrNotFound = errors.New("not found")
	validLore   = map[string]bool{"locations": true, "factions": true, "items": true, "rules": true}
)

// Workspace manages all local novel repositories inside a base ProjectsDir.
type Workspace struct {
	baseDir string
	mu      sync.Mutex
	repos   map[string]*ProjectRepo
}

func NewWorkspace(projectsDir string) (*Workspace, error) {
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		return nil, err
	}
	return &Workspace{
		baseDir: projectsDir,
		repos:   map[string]*ProjectRepo{},
	}, nil
}

func (w *Workspace) BaseDir() string {
	return w.baseDir
}

// ListProjects discovers all valid inkweave.yaml novel repos inside BaseDir.
func (w *Workspace) ListProjects() ([]model.ProjectSummary, error) {
	entries, err := os.ReadDir(w.baseDir)
	if err != nil {
		return nil, err
	}
	var out []model.ProjectSummary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(w.baseDir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "inkweave.yaml")); err != nil {
			continue
		}
		repo, err := OpenRepo(dir)
		if err != nil {
			continue
		}
		sum, err := repo.Summary()
		if err != nil {
			continue
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// CreateProject initializes a brand new Git-friendly novel repository.
func (w *Workspace) CreateProject(p model.Project) (*ProjectRepo, error) {
	now := time.Now().UTC()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Slug == "" {
		p.Slug = Slugify(p.Title, "novel-"+p.ID[:8])
	}
	if p.Language == "" {
		p.Language = "th"
	}
	if p.DefaultMode == "" {
		p.DefaultMode = model.ModeBalanced
	}
	if p.POV == "" {
		p.POV = "Third-person limited"
	}
	if p.Tense == "" {
		p.Tense = "Past"
	}
	if len(p.AntiCliches) == 0 {
		p.AntiCliches = defaultAntiCliches(p.Language)
	}
	p.CreatedAt = now
	p.UpdatedAt = now

	dir := filepath.Join(w.baseDir, p.Slug)
	if _, err := os.Stat(dir); err == nil {
		dir = filepath.Join(w.baseDir, p.Slug+"-"+p.ID[:6])
		p.Slug = filepath.Base(dir)
	}

	repo, err := InitRepo(dir, p)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	w.repos[p.ID] = repo
	w.mu.Unlock()
	return repo, nil
}

// OpenByID locates and opens a project repository by its Project ID or Slug.
func (w *Workspace) OpenByID(idOrSlug string) (*ProjectRepo, error) {
	w.mu.Lock()
	if r, ok := w.repos[idOrSlug]; ok {
		w.mu.Unlock()
		return r, nil
	}
	w.mu.Unlock()

	entries, err := os.ReadDir(w.baseDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(w.baseDir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "inkweave.yaml")); err != nil {
			continue
		}
		repo, err := OpenRepo(dir)
		if err != nil {
			continue
		}
		p, err := repo.GetProject()
		if err != nil {
			continue
		}
		if p.ID == idOrSlug || p.Slug == idOrSlug || e.Name() == idOrSlug {
			w.mu.Lock()
			w.repos[p.ID] = repo
			w.mu.Unlock()
			return repo, nil
		}
	}
	return nil, fmt.Errorf("project %s: %w", idOrSlug, ErrNotFound)
}

func (w *Workspace) DeleteProject(idOrSlug string) error {
	repo, err := w.OpenByID(idOrSlug)
	if err != nil {
		return err
	}
	p, _ := repo.GetProject()
	w.mu.Lock()
	delete(w.repos, p.ID)
	delete(w.repos, idOrSlug)
	w.mu.Unlock()
	return os.RemoveAll(repo.Dir)
}

func defaultAntiCliches(lang string) []string {
	if lang == "en" {
		return []string{
			"a testament to",
			"sent shivers down her spine",
			"electricity crackled between them",
			"time seemed to stand still",
			"unspoken tension hung thick in the air",
		}
	}
	return []string{
		"ราวกับต้องมนต์สะกด",
		"ความเงียบเข้าปกคลุม",
		"ดวงตาสบประสานกันเนิ่นนาน",
		"หัวใจเต้นรัวราวกับกลองรบ",
		"กาลเวลาราวกับหยุดนิ่ง",
		"พึมพำเสียงแผ่วเบาราวสายลม",
	}
}

// ProjectRepo provides thread-safe CRUD on a single novel directory.
type ProjectRepo struct {
	Dir string
	mu  sync.RWMutex
}

// InitRepo creates the directory skeleton and initial files for a novel repository.
func InitRepo(dir string, p model.Project) (*ProjectRepo, error) {
	subdirs := []string{
		"manuscript/chapters",
		"bible/characters",
		"bible/locations",
		"bible/factions",
		"bible/items",
		"bible/rules",
		"continuity",
		"skills",
		".inkweave/snapshots",
	}
	for _, sd := range subdirs {
		if err := os.MkdirAll(filepath.Join(dir, sd), 0o755); err != nil {
			return nil, err
		}
	}
	repo := &ProjectRepo{Dir: dir}
	if err := repo.EnsureGitIgnore(); err != nil {
		return nil, err
	}
	if err := repo.SaveProject(p); err != nil {
		return nil, err
	}
	if err := repo.SaveOutline(model.Outline{Premise: p.Synopsis, Acts: []model.Act{}}); err != nil {
		return nil, err
	}
	_ = repo.SaveTimeline([]model.TimelineEvent{})
	_ = repo.SavePlotThreads([]model.PlotThread{})
	_ = repo.SaveRelationships([]model.Relationship{})
	_ = repo.SaveCharacterStates([]model.CharacterState{})
	return repo, nil
}

// OpenRepo validates and opens an existing novel repository.
func OpenRepo(dir string) (*ProjectRepo, error) {
	if _, err := os.Stat(filepath.Join(dir, "inkweave.yaml")); err != nil {
		return nil, fmt.Errorf("not an inkweave novel repo: %w", err)
	}
	r := &ProjectRepo{Dir: dir}
	_ = r.EnsureGitIgnore()
	return r, nil
}

func (r *ProjectRepo) EnsureGitIgnore() error {
	path := filepath.Join(r.Dir, ".gitignore")
	if raw, err := os.ReadFile(path); err == nil {
		if strings.Contains(string(raw), ".inkweave/") {
			return nil
		}
		updated := strings.TrimRight(string(raw), "\n") + "\n.inkweave/\n"
		return WriteAtomic(path, []byte(updated))
	}
	return WriteAtomic(path, []byte(".inkweave/\n*.tmp\n"))
}

func (r *ProjectRepo) GetProject() (model.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var p model.Project
	raw, err := os.ReadFile(filepath.Join(r.Dir, "inkweave.yaml"))
	if err != nil {
		return p, err
	}
	err = yaml.Unmarshal(raw, &p)
	return p, err
}

func (r *ProjectRepo) SaveProject(p model.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.UpdatedAt = time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = p.UpdatedAt
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return WriteAtomic(filepath.Join(r.Dir, "inkweave.yaml"), data)
}

func (r *ProjectRepo) Summary() (model.ProjectSummary, error) {
	p, err := r.GetProject()
	if err != nil {
		return model.ProjectSummary{}, err
	}
	chapters, _ := r.ListChapters()
	chars, _ := r.ListCharacters()
	words, charsCount := 0, 0
	for _, ch := range chapters {
		words += ch.WordCount
		charsCount += ch.CharCount
	}
	_, gitErr := os.Stat(filepath.Join(r.Dir, ".git"))
	return model.ProjectSummary{
		Project:        p,
		ChapterCount:   len(chapters),
		TotalWords:     words,
		TotalChars:     charsCount,
		CharacterCount: len(chars),
		HasGit:         gitErr == nil,
		Path:           r.Dir,
	}, nil
}

// --- Outline ---

func (r *ProjectRepo) GetOutline() (model.Outline, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var o model.Outline
	raw, err := os.ReadFile(filepath.Join(r.Dir, "manuscript", "outline.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return o, nil
		}
		return o, err
	}
	err = yaml.Unmarshal(raw, &o)
	return o, err
}

func (r *ProjectRepo) SaveOutline(o model.Outline) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := yaml.Marshal(o)
	if err != nil {
		return err
	}
	return WriteAtomic(filepath.Join(r.Dir, "manuscript", "outline.yaml"), data)
}

// --- Chapters ---

func (r *ProjectRepo) chaptersDir() string {
	return filepath.Join(r.Dir, "manuscript", "chapters")
}

func (r *ProjectRepo) ListChapters() ([]model.Chapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listChaptersLocked()
}

func (r *ProjectRepo) listChaptersLocked() ([]model.Chapter, error) {
	entries, err := os.ReadDir(r.chaptersDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Chapter{}, nil
		}
		return nil, err
	}
	var out []model.Chapter
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(r.chaptersDir(), e.Name()))
		if err != nil {
			continue
		}
		var ch model.Chapter
		body, err := ParseFrontmatter(raw, &ch)
		if err != nil {
			continue
		}
		ch.Content = body
		ch.FileName = e.Name()
		if ch.WordCount == 0 && body != "" {
			ch.WordCount, ch.CharCount = CountProseMetrics(body)
		}
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Number != out[j].Number {
			return out[i].Number < out[j].Number
		}
		return out[i].FileName < out[j].FileName
	})
	return out, nil
}

func (r *ProjectRepo) GetChapter(idOrSlug string) (*model.Chapter, error) {
	chapters, err := r.ListChapters()
	if err != nil {
		return nil, err
	}
	for i := range chapters {
		c := &chapters[i]
		if c.ID == idOrSlug || c.Slug == idOrSlug || c.FileName == idOrSlug {
			return c, nil
		}
	}
	return nil, fmt.Errorf("chapter %s: %w", idOrSlug, ErrNotFound)
}

func (r *ProjectRepo) SaveChapter(ch *model.Chapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, _ := r.listChaptersLocked()
	var oldFile string
	if ch.ID == "" {
		ch.ID = uuid.NewString()[:8]
	}
	maxNum := 0
	for _, ex := range existing {
		if ex.Number > maxNum {
			maxNum = ex.Number
		}
		if ex.ID == ch.ID {
			oldFile = ex.FileName
			if ch.Number == 0 {
				ch.Number = ex.Number
			}
		}
	}
	if ch.Number == 0 {
		ch.Number = maxNum + 1
	}
	if ch.Slug == "" {
		ch.Slug = Slugify(ch.Title, fmt.Sprintf("chapter-%03d", ch.Number))
	}
	if ch.Status == "" {
		ch.Status = "draft"
	}
	ch.WordCount, ch.CharCount = CountProseMetrics(ch.Content)
	ch.UpdatedAt = time.Now().UTC()

	fileName := fmt.Sprintf("%03d-%s.md", ch.Number, ch.Slug)
	for _, ex := range existing {
		if ex.ID != ch.ID && ex.FileName == fileName {
			suffix := ch.ID
			if len(suffix) > 6 {
				suffix = suffix[:6]
			}
			ch.Slug = ch.Slug + "-" + suffix
			fileName = fmt.Sprintf("%03d-%s.md", ch.Number, ch.Slug)
			break
		}
	}
	ch.FileName = fileName

	raw, err := MarshalFrontmatter(ch, ch.Content)
	if err != nil {
		return err
	}
	target := filepath.Join(r.chaptersDir(), fileName)
	if err := WriteAtomic(target, raw); err != nil {
		return err
	}
	if oldFile != "" && oldFile != fileName {
		_ = os.Remove(filepath.Join(r.chaptersDir(), oldFile))
	}
	r.touchProjectUpdatedLocked()
	return nil
}

func (r *ProjectRepo) DeleteChapter(id string) error {
	ch, err := r.GetChapter(id)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.Remove(filepath.Join(r.chaptersDir(), ch.FileName)); err != nil {
		return err
	}
	r.touchProjectUpdatedLocked()
	return nil
}

func (r *ProjectRepo) ReorderChapters(orderedIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	chapters, err := r.listChaptersLocked()
	if err != nil {
		return err
	}
	byID := map[string]model.Chapter{}
	oldFiles := map[string]bool{}
	for _, c := range chapters {
		byID[c.ID] = c
		oldFiles[c.FileName] = true
	}

	var ordered []model.Chapter
	seen := map[string]bool{}
	for _, id := range orderedIDs {
		if c, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			ordered = append(ordered, c)
		}
	}
	for _, c := range chapters {
		if !seen[c.ID] {
			ordered = append(ordered, c)
		}
	}

	type pendingWrite struct {
		fileName string
		raw      []byte
	}
	usedNames := map[string]bool{}
	writes := make([]pendingWrite, 0, len(ordered))
	for i := range ordered {
		c := ordered[i]
		c.Number = i + 1
		if c.Slug == "" {
			c.Slug = Slugify(c.Title, fmt.Sprintf("chapter-%03d", c.Number))
		}
		fileName := fmt.Sprintf("%03d-%s.md", c.Number, c.Slug)
		if usedNames[fileName] {
			suffix := c.ID
			if len(suffix) > 6 {
				suffix = suffix[:6]
			}
			c.Slug = c.Slug + "-" + suffix
			fileName = fmt.Sprintf("%03d-%s.md", c.Number, c.Slug)
		}
		usedNames[fileName] = true
		c.FileName = fileName
		c.WordCount, c.CharCount = CountProseMetrics(c.Content)
		raw, err := MarshalFrontmatter(&c, c.Content)
		if err != nil {
			return err
		}
		writes = append(writes, pendingWrite{fileName: fileName, raw: raw})
	}

	for _, w := range writes {
		if err := WriteAtomic(filepath.Join(r.chaptersDir(), w.fileName), w.raw); err != nil {
			return err
		}
	}
	for old := range oldFiles {
		if !usedNames[old] {
			_ = os.Remove(filepath.Join(r.chaptersDir(), old))
		}
	}
	r.touchProjectUpdatedLocked()
	return nil
}

// --- Characters (Deep Non-Linear Schema) ---

func (r *ProjectRepo) charactersDir() string {
	return filepath.Join(r.Dir, "bible", "characters")
}

func (r *ProjectRepo) ListCharacters() ([]model.Character, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listCharactersLocked()
}

func (r *ProjectRepo) listCharactersLocked() ([]model.Character, error) {
	entries, err := os.ReadDir(r.charactersDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Character{}, nil
		}
		return nil, err
	}
	var out []model.Character
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(r.charactersDir(), e.Name()))
		if err != nil {
			continue
		}
		var ch model.Character
		body, err := ParseFrontmatter(raw, &ch)
		if err != nil {
			continue
		}
		ch.BioMarkdown = body
		ch.FileName = e.Name()
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (r *ProjectRepo) GetCharacter(idOrName string) (*model.Character, error) {
	chars, err := r.ListCharacters()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(idOrName))
	for i := range chars {
		c := &chars[i]
		if c.ID == idOrName || c.Slug == idOrName || strings.ToLower(c.Name) == q {
			return c, nil
		}
		for _, a := range c.Aliases {
			if strings.ToLower(a) == q {
				return c, nil
			}
		}
	}
	return nil, fmt.Errorf("character %s: %w", idOrName, ErrNotFound)
}

func (r *ProjectRepo) SaveCharacter(c *model.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, _ := r.listCharactersLocked()
	var oldFile string
	if c.ID == "" {
		c.ID = "char-" + uuid.NewString()[:8]
	}
	for _, ex := range existing {
		if ex.ID == c.ID {
			oldFile = ex.FileName
			break
		}
	}
	if c.Slug == "" {
		c.Slug = Slugify(c.Name, c.ID)
	}
	if c.ArcType == "" {
		c.ArcType = "oscillating"
	}
	c.UpdatedAt = time.Now().UTC()
	c.FileName = c.Slug + ".md"

	raw, err := MarshalFrontmatter(c, c.BioMarkdown)
	if err != nil {
		return err
	}
	if err := WriteAtomic(filepath.Join(r.charactersDir(), c.FileName), raw); err != nil {
		return err
	}
	if oldFile != "" && oldFile != c.FileName {
		_ = os.Remove(filepath.Join(r.charactersDir(), oldFile))
	}
	r.touchProjectUpdatedLocked()
	return nil
}

func (r *ProjectRepo) DeleteCharacter(id string) error {
	c, err := r.GetCharacter(id)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.Remove(filepath.Join(r.charactersDir(), c.FileName)); err != nil {
		return err
	}
	r.touchProjectUpdatedLocked()
	return nil
}

// --- Lore Entries (locations | factions | items | rules) ---

func (r *ProjectRepo) loreDir(category string) (string, error) {
	if !validLore[category] {
		return "", fmt.Errorf("invalid lore category: %s", category)
	}
	return filepath.Join(r.Dir, "bible", category), nil
}

func (r *ProjectRepo) ListLore(category string) ([]model.LoreEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if category == "" || category == "all" {
		var all []model.LoreEntry
		for _, cat := range []string{"locations", "factions", "items", "rules"} {
			items, err := r.listLoreLocked(cat)
			if err == nil {
				all = append(all, items...)
			}
		}
		return all, nil
	}
	return r.listLoreLocked(category)
}

func (r *ProjectRepo) listLoreLocked(category string) ([]model.LoreEntry, error) {
	dir, err := r.loreDir(category)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.LoreEntry{}, nil
		}
		return nil, err
	}
	var out []model.LoreEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var item model.LoreEntry
		body, err := ParseFrontmatter(raw, &item)
		if err != nil {
			continue
		}
		if item.Category == "" {
			item.Category = category
		}
		item.Content = body
		item.FileName = e.Name()
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (r *ProjectRepo) SaveLore(item *model.LoreEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	dir, err := r.loreDir(item.Category)
	if err != nil {
		return err
	}
	existing, _ := r.listLoreLocked(item.Category)
	var oldFile string
	if item.ID == "" {
		item.ID = "lore-" + uuid.NewString()[:8]
	}
	for _, ex := range existing {
		if ex.ID == item.ID {
			oldFile = ex.FileName
			break
		}
	}
	if item.Slug == "" {
		item.Slug = Slugify(item.Name, item.ID)
	}
	item.UpdatedAt = time.Now().UTC()
	item.FileName = item.Slug + ".md"
	raw, err := MarshalFrontmatter(item, item.Content)
	if err != nil {
		return err
	}
	if err := WriteAtomic(filepath.Join(dir, item.FileName), raw); err != nil {
		return err
	}
	if oldFile != "" && oldFile != item.FileName {
		_ = os.Remove(filepath.Join(dir, oldFile))
	}
	r.touchProjectUpdatedLocked()
	return nil
}

func (r *ProjectRepo) DeleteLore(category, id string) error {
	items, err := r.ListLore(category)
	if err != nil {
		return err
	}
	dir, err := r.loreDir(category)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == id || item.Slug == id {
			r.mu.Lock()
			defer r.mu.Unlock()
			err := os.Remove(filepath.Join(dir, item.FileName))
			r.touchProjectUpdatedLocked()
			return err
		}
	}
	return ErrNotFound
}

// --- Continuity Ledgers (timeline, threads, relationships, states) ---

func (r *ProjectRepo) GetTimeline() ([]model.TimelineEvent, error) {
	var items []model.TimelineEvent
	err := r.readYAML(filepath.Join("continuity", "timeline.yaml"), &items)
	sort.Slice(items, func(i, j int) bool { return items[i].Order < items[j].Order })
	return items, err
}

func (r *ProjectRepo) SaveTimeline(items []model.TimelineEvent) error {
	return r.writeYAML(filepath.Join("continuity", "timeline.yaml"), items)
}

func (r *ProjectRepo) GetPlotThreads() ([]model.PlotThread, error) {
	var items []model.PlotThread
	err := r.readYAML(filepath.Join("continuity", "threads.yaml"), &items)
	return items, err
}

func (r *ProjectRepo) SavePlotThreads(items []model.PlotThread) error {
	return r.writeYAML(filepath.Join("continuity", "threads.yaml"), items)
}

func (r *ProjectRepo) GetRelationships() ([]model.Relationship, error) {
	var items []model.Relationship
	err := r.readYAML(filepath.Join("continuity", "relationships.yaml"), &items)
	return items, err
}

func (r *ProjectRepo) SaveRelationships(items []model.Relationship) error {
	return r.writeYAML(filepath.Join("continuity", "relationships.yaml"), items)
}

func (r *ProjectRepo) GetCharacterStates() ([]model.CharacterState, error) {
	var items []model.CharacterState
	err := r.readYAML(filepath.Join("continuity", "states.yaml"), &items)
	return items, err
}

func (r *ProjectRepo) SaveCharacterStates(items []model.CharacterState) error {
	return r.writeYAML(filepath.Join("continuity", "states.yaml"), items)
}

func (r *ProjectRepo) readYAML(relPath string, out any) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	raw, err := os.ReadFile(filepath.Join(r.Dir, relPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return yaml.Unmarshal(raw, out)
}

func (r *ProjectRepo) writeYAML(relPath string, data any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	if err := WriteAtomic(filepath.Join(r.Dir, relPath), raw); err != nil {
		return err
	}
	r.touchProjectUpdatedLocked()
	return nil
}

func (r *ProjectRepo) touchProjectUpdatedLocked() {
	pPath := filepath.Join(r.Dir, "inkweave.yaml")
	raw, err := os.ReadFile(pPath)
	if err != nil {
		return
	}
	var p model.Project
	if yaml.Unmarshal(raw, &p) != nil {
		return
	}
	p.UpdatedAt = time.Now().UTC()
	if out, err := yaml.Marshal(p); err == nil {
		_ = WriteAtomic(pPath, out)
	}
}
