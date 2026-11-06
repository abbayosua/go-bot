package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"gobot/internal/config"
	"gobot/internal/store"
)

// runBackup implements `go-bot backup --out file` and `go-bot restore --in
// file`: the restore drill is the same command the runbook uses.
func runBackup(verb string, args []string) error {
	fs := flag.NewFlagSet("go-bot "+verb, flag.ContinueOnError)
	data := fs.String("data", "", "data directory (or GOBOT_DATA)")
	file := fs.String("out", "", "backup file (backup)")
	in := fs.String("in", "", "backup file (restore)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := []string{}
	if *data != "" {
		rest = append(rest, "--data", *data)
	}
	cfg, err := config.Load(rest)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()
	switch verb {
	case "backup":
		if *file == "" {
			return errors.New("--out is required")
		}
		archive, err := st.ExportAll(ctx)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(archive)
		if err != nil {
			return err
		}
		if err := os.WriteFile(*file, raw, 0o600); err != nil {
			return err
		}
		fmt.Printf("backup wrote %d bytes to %s\n", len(raw), *file)
		return nil
	case "restore":
		if *in == "" {
			return errors.New("--in is required")
		}
		raw, err := os.ReadFile(*in)
		if err != nil {
			return err
		}
		var parsed json.RawMessage
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return err
		}
		return st.ImportAll(ctx, parsed)
	default:
		return errors.New("unknown backup verb")
	}
}
