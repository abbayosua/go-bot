package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gobot/internal/llm"
)

const maxOutput = 64 * 1024

type Tool interface {
	Name() string
	Spec() llm.Tool
	Run(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry holds the tools one bot can call and the workspace they act in.
type Registry struct {
	root  string
	tools map[string]Tool
}

func NewRegistry(root string) *Registry {
	r := &Registry{root: root, tools: map[string]Tool{}}
	for _, tool := range []Tool{
		&shellTool{root: root},
		&readFileTool{root: root},
		&writeFileTool{root: root},
		&listFilesTool{root: root},
		&webFetchTool{},
	} {
		r.tools[tool.Name()] = tool
	}
	return r
}

func (r *Registry) Root() string { return r.root }

func (r *Registry) Specs() []llm.Tool {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	specs := make([]llm.Tool, 0, len(names))
	for _, name := range names {
		specs = append(specs, r.tools[name].Spec())
	}
	return specs
}

func (r *Registry) Has(name string) bool { _, ok := r.tools[name]; return ok }

func (r *Registry) Run(ctx context.Context, name string, args json.RawMessage) (string, error) {
	tool, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool %q", name)
	}
	out, err := tool.Run(ctx, args)
	if err != nil {
		return "", err
	}
	return truncate(out), nil
}

func truncate(s string) string {
	if len(s) <= maxOutput {
		return s
	}
	return s[:maxOutput] + fmt.Sprintf("\n[output truncated at %d bytes]", maxOutput)
}

// resolve turns a workspace-relative path into an absolute one that cannot
// escape the root, symlinks included.
func resolve(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return root, nil
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("path must be relative to the workspace")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the workspace")
	}
	target := filepath.Join(root, clean)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	// Walk up to the nearest existing ancestor so a new file still gets checked.
	probe := target
	for {
		if _, statErr := os.Lstat(probe); statErr == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	realProbe, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", err
	}
	if realProbe != realRoot && !strings.HasPrefix(realProbe, realRoot+string(filepath.Separator)) {
		return "", errors.New("path escapes the workspace through a symlink")
	}
	return target, nil
}
