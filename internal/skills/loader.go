package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inkweave/inkweave/internal/model"
	"github.com/inkweave/inkweave/internal/storage"
	builtin "github.com/inkweave/inkweave/skills"
)

// ListSkills loads the 9 embedded built-in skills and merges any custom skills in repoDir/skills/*.md.
func ListSkills(repoDir string) ([]model.Skill, error) {
	byID := map[string]model.Skill{}

	entries, err := fs.ReadDir(builtin.BuiltinFS, ".")
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			raw, err := fs.ReadFile(builtin.BuiltinFS, e.Name())
			if err != nil {
				continue
			}
			var sk model.Skill
			body, err := storage.ParseFrontmatter(raw, &sk)
			if err != nil {
				continue
			}
			if sk.ID == "" {
				sk.ID = strings.TrimSuffix(e.Name(), ".md")
			}
			sk.Instructions = body
			sk.BuiltIn = true
			sk.Custom = false
			byID[sk.ID] = sk
		}
	}

	if repoDir != "" {
		customDir := filepath.Join(repoDir, "skills")
		if cEntries, err := os.ReadDir(customDir); err == nil {
			for _, e := range cEntries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				raw, err := os.ReadFile(filepath.Join(customDir, e.Name()))
				if err != nil {
					continue
				}
				var sk model.Skill
				body, err := storage.ParseFrontmatter(raw, &sk)
				if err != nil {
					continue
				}
				if sk.ID == "" {
					sk.ID = strings.TrimSuffix(e.Name(), ".md")
				}
				sk.Instructions = body
				if existing, ok := byID[sk.ID]; ok {
					sk.BuiltIn = existing.BuiltIn
				}
				sk.Custom = true
				byID[sk.ID] = sk
			}
		}
	}

	out := make([]model.Skill, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// GetSkill returns one skill by ID.
func GetSkill(repoDir, id string) (*model.Skill, error) {
	all, err := ListSkills(repoDir)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("skill %s not found", id)
}

// SaveCustomSkill writes a custom or overridden skill Markdown file into <repoDir>/skills/<id>.md.
func SaveCustomSkill(repoDir string, sk model.Skill) error {
	if sk.ID == "" {
		sk.ID = storage.Slugify(sk.Name, "custom-skill")
	}
	if sk.Name == "" {
		sk.Name = sk.ID
	}
	dir := filepath.Join(repoDir, "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := storage.MarshalFrontmatter(sk, sk.Instructions)
	if err != nil {
		return err
	}
	return storage.WriteAtomic(filepath.Join(dir, sk.ID+".md"), raw)
}

// DeleteCustomSkill removes a custom skill override file from <repoDir>/skills/.
func DeleteCustomSkill(repoDir, id string) error {
	path := filepath.Join(repoDir, "skills", id+".md")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
