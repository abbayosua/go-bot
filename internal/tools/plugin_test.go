package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gobot/internal/store"
)

func writePlugin(t *testing.T, manifest string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "upper")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadManifestValidates(t *testing.T) {
	good := writePlugin(t, `{"name":"upper","description":"Upcases.","tools":[{"name":"shout","description":"Shout.","kind":"prompt","template":"LOUD: {{text}}"}]}`)
	manifest, err := LoadManifest(good)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "upper" || len(manifest.Tools) != 1 {
		t.Fatalf("manifest %+v", manifest)
	}
	for _, raw := range []string{
		`{"description":"x","tools":[]}`,
		`{"name":"x","tools":[{"name":"","kind":"prompt","template":"t"}]}`,
		`{"name":"x","tools":[{"name":"a","kind":"exec"}]}`,
		`{"name":"x","tools":[{"name":"a","kind":"prompt"}]}`,
		`{"name":"x","tools":[{"name":"a","kind":"http"}]}`,
		`not json`,
	} {
		dir := writePlugin(t, raw)
		if _, err := LoadManifest(dir); err == nil {
			t.Fatalf("invalid manifest accepted: %s", raw)
		}
	}
}

func TestPluginBridgeScopesToBot(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a, err := st.CreateBot(ctx, "A", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateBot(ctx, "B", "", "")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "upper")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"),
		[]byte(`{"name":"upper","description":"Upcases.","tools":[{"name":"shout","description":"Shout.","kind":"prompt","template":"LOUD: {{text}}"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InstallPlugin(ctx, "upper", "upper", a.ID); err != nil {
		t.Fatal(err)
	}
	bridge := NewPluginBridge(st, root)
	liveA, skipped := bridge.LiveTools(ctx, a.ID)
	if len(skipped) != 0 || len(liveA) != 1 || liveA[0].Name() != "plug_upper_shout" {
		t.Fatalf("live %+v skipped %v", liveA, skipped)
	}
	liveB, _ := bridge.LiveTools(ctx, b.ID)
	if len(liveB) != 0 {
		t.Fatalf("other bot sees plugin tools: %+v", liveB)
	}
	registry := WrapMemory(NewRegistry(t.TempDir()), st).WithExtra(liveA)
	out, err := registry.Run(ctx, a.ID, "plug_upper_shout", json.RawMessage(`{"text":"hi"}`))
	if err != nil || out != "LOUD: hi" {
		t.Fatalf("prompt tool %q err %v", out, err)
	}
	// Disable hides the tool.
	plugins, err := st.ListPlugins(ctx, a.ID)
	if err != nil || len(plugins) != 1 {
		t.Fatal(plugins, err)
	}
	if err := st.SetPluginEnabled(ctx, plugins[0].ID, false); err != nil {
		t.Fatal(err)
	}
	off, _ := bridge.LiveTools(ctx, a.ID)
	if len(off) != 0 {
		t.Fatalf("disabled plugin still live: %+v", off)
	}
}
