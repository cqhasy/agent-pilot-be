// Package expert 提供可插拔的领域专家定义，供多智能体路由与 handoff 复用。
package expert

import (
	"fmt"
	"strings"
	"sync"
)

// Definition 描述一名专家：主路由 handoff 后，会话切换到为该 ID 独立编译的 compose 图（默认实现为独立 ReAct 环）。
// Instruction 用于编译专家图的系统提示；若使用 chat.RegisterExpertCompose 自定义图，Instruction 仍可作为元数据供工厂使用。
type Definition struct {
	ID string
	// Name 面向模型与前端展示的短名称。
	Name string
	// Description 何时应交由此专家处理（写入主路由系统提示）。
	Description string
	// Instruction 专家图默认编译时的专用系统指令。
	Instruction string
}

// Registry 专家注册表，线程安全，可在初始化时注册自定义专家。
type Registry struct {
	mu       sync.RWMutex
	byID     map[string]*Definition
	order    []string
	fallback *Definition
}

// NewRegistry 创建空注册表；若需要内置默认专家可调用 DefaultRegistry。
func NewRegistry() *Registry {
	return &Registry{
		byID: make(map[string]*Definition),
	}
}

// Register 注册一名专家；同名 ID 覆盖前者。
func (r *Registry) Register(def Definition) error {
	if r == nil {
		return fmt.Errorf("expert: nil registry")
	}
	id := strings.TrimSpace(def.ID)
	if id == "" {
		return fmt.Errorf("expert: empty id")
	}
	def.ID = id
	if strings.TrimSpace(def.Name) == "" {
		def.Name = def.ID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = make(map[string]*Definition)
	}
	// 去重 order
	if _, exists := r.byID[id]; !exists {
		r.order = append(r.order, id)
	}
	cp := def
	r.byID[id] = &cp
	return nil
}

// SetFallback 设置当不处于任何专家模式时仍希望强调的说明；可选。
func (r *Registry) SetFallback(def *Definition) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = def
}

// Get 按 ID 查找专家。
func (r *Registry) Get(id string) (*Definition, bool) {
	if r == nil {
		return nil, false
	}
	id = strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byID[id]
	return d, ok
}

// List 按注册顺序返回专家副本（用于系统提示列举）。
func (r *Registry) List() []Definition {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Definition, 0, len(r.order))
	for _, id := range r.order {
		if d := r.byID[id]; d != nil {
			out = append(out, *d)
		}
	}
	return out
}

// DefaultRegistry 内置文档类与演示类专家，可按业务继续 Register 扩展。
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	_ = reg.Register(Definition{
		ID:          "document",
		Name:        "文档专家",
		Description: "长文档撰写、结构大纲、技术说明、格式规范、修订与润色、Markdown/文档导入导出相关任务。",
		Instruction: `You are now acting as the DOCUMENT specialist for this session.
Focus on: clear structure (headings, sections), consistent terminology, revision discipline, and accessibility of prose.
Prefer outlining before long prose. When the user needs imports/exports or template-driven docs, call load_skill if a matching skill exists.

Editorial / UI separation (mandatory):
- Put ONLY the readable manuscript or draft body that the user should skim for approval between the preview markers (Markdown inside). The opening line MUST be EXACTLY this, then a newline, then the body: <!--DOC_PREVIEW_START-->
- The closing line MUST be EXACTLY: <!--DOC_PREVIEW_END-->
- Do NOT append any text on the same line as START or END, and do NOT omit the closing angle bracket after -- on the start tag line. Do not output internal template or “think” tags in the user-visible text.
- Outside those markers: task status, plan-step commentary, Feishu operational notes, links to tools, and dialogue. Never instruct the user to "open Feishu to review the draft" for creative approval — review happens in this chat session (preview pane + request_user_input).
- Workflow order unless the user explicitly waives it: (1) draft inside markers → (2) request_user_input for approval or edits → (3) only after approval, create/update Feishu docs with lark-doc → (4) optional IM share.
After substantive edits, follow the creative deliverable review protocol when applicable.
Handback: once the user has approved this phase and you have finished the plan-required persistence/share for the current step, call release_expert in the same turn so the MAIN agent resumes — it owns cross-domain follow-ups (e.g. “再发到群里”, new topics). Stay in expert mode only if the user immediately asks for more drafting in this specialist thread.`,
	})
	_ = reg.Register(Definition{
		ID:          "presentation",
		Name:        "演示/PPT 专家",
		Description: "幻灯片叙事、页级大纲、演讲者备注、视觉层次（非设计工具替代）、演示节奏与受众适配。",
		Instruction: `You are now acting as the PRESENTATION specialist for this session.
Focus on: slide-level storyline, one idea per slide where possible, speaker notes, and logical flow for live delivery.
When tooling cannot render slides directly, produce structured slide outlines (title + bullets + notes) and iterate with the user.
Put the outline/deck body that the user should review inside <!--DOC_PREVIEW_START--> ... <!--DOC_PREVIEW_END-->; keep meta and status outside.
Follow the creative deliverable review protocol for slide decks and outlines.
After approval and step completion, call release_expert so the main agent handles follow-ups outside this specialist unless the user asks for more slide work here.`,
	})
	return reg
}
