package skill

// https://code.claude.com/docs/en/skills 参见
type frontmatter struct {
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	Description  string   `yaml:"description"`
	WhenToUse    string   `yaml:"when_to_use"`
	ArgumentHint string   `yaml:"argument-hint"`
	AllowedTools []string `yaml:"allowed-tools"`
	Model        string   `yaml:"model"`
	Context      string   `yaml:"context"`
	Agent        string   `yaml:"chat"`
	Metadata     Metadata `yaml:"metadata"`
}

type Metadata struct {
	Requires MetadataRequires `yaml:"requires,omitempty"`
	CLIHelp  string           `yaml:"cliHelp,omitempty"`
}

type MetadataRequires struct {
	Bins []string `yaml:"bins,omitempty"`
}

type Skill struct {
	// 基本信息
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version,omitempty"`
	Description string   `yaml:"description"`
	Aliases     []string `yaml:"aliases,omitempty"`

	// 触发相关
	WhenToUse    string   `yaml:"when_to_use,omitempty"`
	ArgumentHint string   `yaml:"argument-hint,omitempty"`
	Arguments    []string `yaml:"arguments,omitempty"`

	// 权限和行为控制
	AllowedTools           []string `yaml:"allowed-tools,omitempty"`
	Model                  string   `yaml:"model,omitempty"`
	DisableModelInvocation bool     `yaml:"disable-model-invocation,omitempty"`
	UserInvocable          bool     `yaml:"user-invocable,omitempty"`

	// agent有关
	Context string `yaml:"context,omitempty"` // inline | fork
	Agent   string `yaml:"chat,omitempty"`

	// 其他
	Paths []string `yaml:"paths,omitempty"`
	Shell string   `yaml:"shell,omitempty"`

	Metadata Metadata `yaml:"metadata,omitempty"`

	// 运行时数据
	BaseDir string            // skill 目录
	Content string            // markdown body
	Files   map[string]string // reference files（可选 preload）
}
