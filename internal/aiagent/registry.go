package aiagent

import (
	"sort"
	"strings"
	"sync"
)

// Registry holds the tools available to the agent. It is safe for concurrent use
// because external MCP tools may be (re)loaded while a conversation is running.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
	order []string
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]*Tool{}}
}

// Register adds (or replaces) a tool. Tools registered later with the same name
// win, which is how an MCP tool can shadow a built-in one if the administrator
// really wants that.
func (r *Registry) Register(t Tool) {
	if t.Name == "" || t.Execute == nil {
		return
	}
	if t.Source == "" {
		t.Source = "builtin"
	}
	tool := t
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[t.Name]; !ok {
		r.order = append(r.order, t.Name)
	}
	r.tools[t.Name] = &tool
}

// RegisterAll registers a batch of tools.
func (r *Registry) RegisterAll(tools []Tool) {
	for _, t := range tools {
		r.Register(t)
	}
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Unregister removes a tool.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[name]; !ok {
		return
	}
	delete(r.tools, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// UnregisterSource removes every tool whose Source matches the prefix. It is used
// to drop the tools of an MCP server that has been disabled or reconfigured.
func (r *Registry) UnregisterSource(prefix string) {
	r.mu.Lock()
	names := make([]string, 0, len(r.tools))
	for name, t := range r.tools {
		if strings.HasPrefix(t.Source, prefix) {
			names = append(names, name)
		}
	}
	r.mu.Unlock()
	for _, n := range names {
		r.Unregister(n)
	}
}

// List returns all tools in registration order.
func (r *Registry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(r.order))
	for _, n := range r.order {
		if t, ok := r.tools[n]; ok {
			out = append(out, t)
		}
	}
	return out
}

// Names returns all registered tool names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Len returns the number of registered tools.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Definitions converts the registry into OpenAI function-calling definitions.
// allow, when non-empty, restricts the result to the listed tool names.
func (r *Registry) Definitions(allow []string) []toolDef {
	set := toSet(allow)
	tools := r.List()
	out := make([]toolDef, 0, len(tools))
	for _, t := range tools {
		if len(set) > 0 && !set[t.Name] {
			continue
		}
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, toolDef{Type: "function", Function: funcDef{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		}})
	}
	return out
}

// toSet converts a name slice into a lookup set.
func toSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// SortedNames returns tool names sorted alphabetically (used by the API).
func (r *Registry) SortedNames() []string {
	out := r.Names()
	sort.Strings(out)
	return out
}
