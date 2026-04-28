package skill

import (
	"os"
	"path/filepath"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	Skills []*Skill

	skills map[string]*Skill

	// alias 索引
	aliases map[string]string // alias -> skillName
}

func LoadSkills(dir string) (*Registry, error) {
	var skills []*Skill
	m1 := make(map[string]*Skill)
	m2 := make(map[string]string)

	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// 目录不存在，返回空注册表
		return &Registry{Skills: skills, skills: m1, aliases: m2}, nil
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil || info.IsDir() {
			return nil
		}
		if info.Name() == "SKILL.md" {
			s, loadErr := LoadSkill(path)
			if loadErr != nil {
				return nil // 跳过加载失败的 skill
			}
			m1[s.Name] = s

			for _, a := range s.Aliases {
				m2[a] = s.Name
			}
			skills = append(skills, s)
		}
		return nil
	})

	return &Registry{Skills: skills, skills: m1, aliases: m2}, err
}

func (r *Registry) Get(name string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 精确命中
	if s, ok := r.skills[name]; ok {
		return s
	}
	// alias 命中
	if s, ok := r.aliases[name]; ok {
		return r.skills[s]
	}

	return nil
}

func (r *Registry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	return out
}
