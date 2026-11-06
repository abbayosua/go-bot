package tools

import (
	"context"
	"path/filepath"
	"strings"

	"gobot/internal/store"
)

// PluginBridge loads enabled plugins for one bot into live tools: folder +
// manifest on disk, install/enable rows in the store, tools visible to that
// bot only. A broken folder is skipped with its name recorded, never fatal.
type PluginBridge struct {
	store *store.Store
	root  string
}

// NewPluginBridge builds the bridge: root is the plugins directory.
func NewPluginBridge(st *store.Store, root string) *PluginBridge {
	return &PluginBridge{store: st, root: root}
}

// LiveTools returns one bot's enabled plugin tools.
func (b *PluginBridge) LiveTools(ctx context.Context, botID string) ([]Tool, []string) {
	plugins, err := b.store.ListPlugins(ctx, botID)
	if err != nil {
		return nil, []string{"plugins list: " + err.Error()}
	}
	var out []Tool
	var skipped []string
	for _, plugin := range plugins {
		if !plugin.Enabled {
			continue
		}
		dir := plugin.Path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(b.root, filepath.Clean("/"+dir))
		}
		manifest, err := LoadManifest(dir)
		if err != nil {
			skipped = append(skipped, plugin.Name+": "+err.Error())
			continue
		}
		for _, def := range manifest.Tools {
			def := def
			if strings.TrimSpace(def.Name) == "" {
				continue
			}
			out = append(out, &pluginTool{plugin: plugin.Name, def: def})
		}
	}
	return out, skipped
}
