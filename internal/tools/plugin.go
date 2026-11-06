package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gobot/internal/llm"
)

// Manifest describes one plugin folder: a name, a description, and tools.
// A prompt tool expands a template with {{args}} and returns text; an http
// tool POSTs JSON args and returns the body. No code execution, by design.
type Manifest struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Tools       []PluginTool `json:"tools"`
}

// PluginTool is one tool inside a manifest.
type PluginTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Template    string `json:"template,omitempty"`
	URL         string `json:"url,omitempty"`
	Schema      string `json:"schema,omitempty"`
}

// LoadManifest reads and validates plugin.json in a folder.
func LoadManifest(dir string) (Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("bad manifest: %w", err)
	}
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Name == "" {
		return Manifest{}, errors.New("manifest needs a name")
	}
	seen := map[string]bool{}
	for i, tool := range manifest.Tools {
		tool.Name = strings.TrimSpace(tool.Name)
		if tool.Name == "" || seen[tool.Name] {
			return Manifest{}, fmt.Errorf("tool %d needs a unique name", i)
		}
		seen[tool.Name] = true
		if tool.Kind != "prompt" && tool.Kind != "http" {
			return Manifest{}, fmt.Errorf("tool %q kind must be prompt or http", tool.Name)
		}
		if tool.Kind == "prompt" && strings.TrimSpace(tool.Template) == "" {
			return Manifest{}, fmt.Errorf("tool %q needs a template", tool.Name)
		}
		if tool.Kind == "http" && strings.TrimSpace(tool.URL) == "" {
			return Manifest{}, fmt.Errorf("tool %q needs a url", tool.Name)
		}
		manifest.Tools[i] = tool
	}
	return manifest, nil
}

// pluginTool is one live manifest tool, scoped to one plugin and bot.
type pluginTool struct {
	plugin string
	def    PluginTool
}

func pluginName(plugin, tool string) string {
	clean := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
				return r
			}
			return '_'
		}, s)
	}
	return "plug_" + clean(plugin) + "_" + clean(tool)
}

func (t *pluginTool) Name() string { return pluginName(t.plugin, t.def.Name) }

func (t *pluginTool) Spec() llm.Tool {
	schema := strings.TrimSpace(t.def.Schema)
	if schema == "" {
		schema = `{"type":"object"}`
	}
	return llm.Tool{Type: "function", Function: llm.FunctionDef{
		Name:        t.Name(),
		Description: "Plugin " + t.plugin + "." + t.def.Name + ": " + t.def.Description,
		Parameters:  json.RawMessage(schema),
	}}
}

func (t *pluginTool) Run(ctx context.Context, args json.RawMessage) (string, error) {
	switch t.def.Kind {
	case "prompt":
		out := t.def.Template
		var params map[string]any
		if len(args) > 0 {
			_ = json.Unmarshal(args, &params)
		}
		raw, _ := json.Marshal(params)
		out = strings.ReplaceAll(out, "{{args}}", string(raw))
		for key, value := range params {
			out = strings.ReplaceAll(out, "{{"+key+"}}", fmt.Sprintf("%v", value))
		}
		out = strings.TrimSpace(out)
		if out == "" {
			return "", errors.New("template rendered empty")
		}
		return out, nil
	case "http":
		return postPluginHTTP(ctx, t.def.URL, args)
	default:
		return "", errors.New("unknown plugin kind")
	}
}
