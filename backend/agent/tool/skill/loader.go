package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadSkill(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	raw := string(data)

	parts := strings.SplitN(raw, "---", 3)
	if len(parts) < 3 {
		return nil, err
	}

	var s Skill
	if err := yaml.Unmarshal([]byte(parts[1]), &s); err != nil {
		return nil, err
	}

	s.Content = parts[2]
	s.BaseDir = filepath.Dir(path)

	return &s, nil
}

// LoadReferences 加载 references 目录下的所有 .md 文件
func (s *Skill) LoadReferences() map[string]string {
	refs := make(map[string]string)
	refsDir := filepath.Join(s.BaseDir, "references")

	if info, err := os.Stat(refsDir); err == nil && info.IsDir() {
		filepath.Walk(refsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".md") {
				data, _ := os.ReadFile(path)
				name := filepath.Base(path)
				refs[name] = string(data)
			}
			return nil
		})
	}

	return refs
}

func (s *Skill) ReferenceNames() []string {
	refsDir := filepath.Join(s.BaseDir, "references")
	names := make([]string, 0)

	if info, err := os.Stat(refsDir); err == nil && info.IsDir() {
		filepath.Walk(refsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if strings.HasSuffix(path, ".md") {
				name, relErr := filepath.Rel(refsDir, path)
				if relErr == nil {
					names = append(names, relErrClean(name))
				}
			}
			return nil
		})
	}

	sort.Strings(names)
	return names
}

func (s *Skill) LoadReferenceFiles(names []string) (map[string]string, error) {
	refsDir := filepath.Join(s.BaseDir, "references")
	refs := make(map[string]string, len(names))

	for _, name := range names {
		cleanName, err := cleanReferenceName(name)
		if err != nil {
			return nil, err
		}

		path := filepath.Join(refsDir, filepath.FromSlash(cleanName))
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		refs[cleanName] = string(data)
	}

	return refs, nil
}

func cleanReferenceName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("reference name is empty")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("reference name must be relative: %s", name)
	}

	clean := relErrClean(name)
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return "", fmt.Errorf("invalid reference name: %s", name)
	}
	if !strings.HasSuffix(clean, ".md") {
		return "", fmt.Errorf("reference must be a .md file: %s", name)
	}

	return clean, nil
}

func relErrClean(name string) string {
	return filepath.ToSlash(filepath.Clean(strings.ReplaceAll(name, "\\", "/")))
}
