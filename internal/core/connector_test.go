package core

import (
	"context"
	"errors"
	"testing"
)

type fakeConnector struct {
	name  string
	tools []ConnectorTool
	fail  bool
}

func (f *fakeConnector) Name() string { return f.name }

func (f *fakeConnector) Tools(_ context.Context) ([]ConnectorTool, error) {
	if f.fail {
		return nil, errors.New("down")
	}
	return f.tools, nil
}

func (f *fakeConnector) Call(_ context.Context, tool string, _ map[string]any) (string, error) {
	if f.fail {
		return "", errors.New("down")
	}
	return f.name + ":" + tool, nil
}

func (f *fakeConnector) Health(_ context.Context) (string, error) {
	if f.fail {
		return "", errors.New("down")
	}
	return f.name + " ok", nil
}

func TestCatalogRegistersAndLooksUp(t *testing.T) {
	ctx := context.Background()
	catalog := NewCatalog()
	if len(catalog.Names()) != 0 {
		t.Fatal("catalog not empty")
	}
	first := &fakeConnector{name: "b", tools: []ConnectorTool{{Name: "ping"}}}
	second := &fakeConnector{name: "a", tools: []ConnectorTool{{Name: "pong"}}}
	catalog.Register(first)
	catalog.Register(second)
	if names := catalog.Names(); len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("names %v", names)
	}
	found, ok := catalog.Lookup("a")
	if !ok {
		t.Fatal("lookup missed")
	}
	tools, err := found.Tools(ctx)
	if err != nil || len(tools) != 1 || tools[0].Name != "pong" {
		t.Fatalf("tools %+v err %v", tools, err)
	}
	out, err := found.Call(ctx, "pong", nil)
	if err != nil || out != "a:pong" {
		t.Fatalf("call %q err %v", out, err)
	}
	if _, ok := catalog.Lookup("missing"); ok {
		t.Fatal("missing lookup hit")
	}
}

func TestConnectorContractCoversFailure(t *testing.T) {
	ctx := context.Background()
	dead := &fakeConnector{name: "dead", fail: true}
	if _, err := dead.Tools(ctx); err == nil {
		t.Fatal("tools on dead connector")
	}
	if _, err := dead.Call(ctx, "x", nil); err == nil {
		t.Fatal("call on dead connector")
	}
	if _, err := dead.Health(ctx); err == nil {
		t.Fatal("health on dead connector")
	}
}
