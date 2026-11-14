package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// cliClient is a tiny HTTP client for go-bot subcommands talking to a
// running server (Batch 1-5 of CLIPLAN). Base URL and token come from
// flags/env so smoke tests can point at a throwaway local server.
type cliClient struct {
	base  string
	token string
	http  *http.Client
}

func newCLIClient(base string) *cliClient {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = os.Getenv("GOBOT_URL")
	}
	if base == "" {
		base = "http://127.0.0.1:7417"
	}
	return &cliClient{
		base:  base,
		token: os.Getenv("GOBOT_TOKEN"),
		http:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *cliClient) do(method, path string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.StatusCode, err
	}
	if res.StatusCode >= 400 {
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = res.Status
		}
		return raw, res.StatusCode, fmt.Errorf("%s", msg)
	}
	return raw, res.StatusCode, nil
}

func (c *cliClient) getJSON(path string, out any) error {
	raw, _, err := c.do(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// runCLI dispatches client subcommands (CLIPLAN Batch 1-5).
func runCLI(name string, args []string) error {
	switch name {
	case "chat":
		return runChat(args)
	case "state":
		return runState(args)
	case "bots":
		return runBots(args)
	case "threads":
		return runThreads(args)
	case "approve":
		return runApprove(args)
	case "search":
		return runSearch(args)
	case "audit":
		return runAudit(args)
	case "memory":
		return runMemory(args)
	case "notes":
		return runNotes(args)
	case "board":
		return runBoard(args)
	case "cron", "routines":
		return runCron(args)
	case "skills":
		return runSkills(args)
	case "rules":
		return runRules(args)
	case "config", "model", "status", "doctor",
		"plugins", "mcp", "gateway":
		return fmt.Errorf("go-bot %s: not implemented yet (see CLIPLAN.md batch)", name)
	}
	return fmt.Errorf("unknown command %q", name)
}

// runChat implements `go-bot chat "message" [--bot ID] [--thread ID] [--url]`:
// it posts the message, then polls the thread until a new assistant message
// arrives (or a timeout hits), printing the reply as JSON.
func runChat(args []string) error {
	fs := flag.NewFlagSet("go-bot chat", flag.ContinueOnError)
	bot := fs.String("bot", "", "bot id (default: first bot from /api/state)")
	thread := fs.String("thread", "", "thread id (default: first thread of the bot)")
	base := fs.String("url", "", "server base URL (or GOBOT_URL)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: go-bot chat \"message\" [--bot ID] [--thread ID]")
	}
	c := newCLIClient(*base)
	before, threadID, err := resolveThread(c, *bot, *thread)
	if err != nil {
		return err
	}
	msg := strings.Join(fs.Args(), " ")
	raw, code, err := c.do(http.MethodPost, "/api/threads/"+threadID+"/messages", map[string]string{"content": msg})
	if err != nil {
		return fmt.Errorf("post message: %w", err)
	}
	if code != http.StatusAccepted && code != http.StatusCreated && code != http.StatusOK {
		return fmt.Errorf("post message: unexpected status %d: %s", code, strings.TrimSpace(string(raw)))
	}
	reply, err := waitAssistant(c, threadID, before, 180*time.Second)
	if err != nil {
		return err
	}
	return printJSON(reply)
}

// runState implements `go-bot state [--url]`: prints /api/state plus
// /api/health in one JSON object (bots, spaces, approval, model, version).
func runState(args []string) error {
	fs := flag.NewFlagSet("go-bot state", flag.ContinueOnError)
	base := fs.String("url", "", "server base URL (or GOBOT_URL)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c := newCLIClient(*base)
	var state any
	if err := c.getJSON("/api/state", &state); err != nil {
		return err
	}
	var health any
	if err := c.getJSON("/api/health", &health); err != nil {
		return err
	}
	return printJSON(map[string]any{"health": health, "state": state})
}
// runBots implements `go-bot bots list|create|rm`.
// `rm` has no backend endpoint; it deletes all threads of the bot
// and reports the count.
func runBots(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-bot bots list|create|rm ...")
	}
	c := newCLIClient("")
	switch args[0] {
	case "list":
		var state struct {
			Bots []any `json:"bots"`
		}
		if err := c.getJSON("/api/state", &state); err != nil {
			return err
		}
		return printJSON(state.Bots)
	case "create":
		fs := flag.NewFlagSet("go-bot bots create", flag.ContinueOnError)
		name := fs.String("name", "", "bot name (required)")
		instructions := fs.String("instructions", "", "bot instructions")
		model := fs.String("model", "", "model override")
		space := fs.String("space", "", "space id")
		base := fs.String("url", "", "server base URL (or GOBOT_URL)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*name) == "" {
			return fmt.Errorf("usage: go-bot bots create --name NAME [--instructions T] [--model M] [--space ID]")
		}
		cc := newCLIClient(*base)
		raw, _, err := cc.do(http.MethodPost, "/api/bots", map[string]string{
			"name": *name, "instructions": *instructions, "model": *model, "spaceId": *space,
		})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot bots rm BOT_ID")
		}
		botID := args[1]
		var state struct {
			Bots []struct {
				ID      string `json:"id"`
				Threads []struct {
					ID string `json:"id"`
				} `json:"threads"`
			} `json:"bots"`
		}
		if err := c.getJSON("/api/state", &state); err != nil {
			return err
		}
		var threads []string
		found := false
		for _, b := range state.Bots {
			if b.ID == botID {
				found = true
				for _, t := range b.Threads {
					threads = append(threads, t.ID)
				}
			}
		}
		if !found {
			return fmt.Errorf("bot %q not found (no backend bot delete; rm clears threads)", botID)
		}
		for _, t := range threads {
			_, _, _ = c.do(http.MethodDelete, "/api/bots/"+botID+"/threads/"+t, nil)
		}
		return printJSON(map[string]any{"bot": botID, "threadsDeleted": len(threads)})
	}
	return fmt.Errorf("usage: go-bot bots list|create|rm ...")
}

// runThreads implements `go-bot threads list|new|rm|open`.
// `open` prints the thread JSON (the web UI opens it visually instead).
func runThreads(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-bot threads list|new|rm|open ...")
	}
	c := newCLIClient("")
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("go-bot threads list", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		var state struct {
			Bots []struct {
				ID      string `json:"id"`
				Threads []any  `json:"threads"`
			} `json:"bots"`
		}
		if err := c.getJSON("/api/state", &state); err != nil {
			return err
		}
		if *bot != "" {
			for _, b := range state.Bots {
				if b.ID == *bot {
					return printJSON(b.Threads)
				}
			}
			return fmt.Errorf("bot %q not found", *bot)
		}
		all := []any{}
		for _, b := range state.Bots {
			all = append(all, b.Threads...)
		}
		return printJSON(all)
	case "new":
		fs := flag.NewFlagSet("go-bot threads new", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		title := fs.String("title", "Chat", "thread title")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		botID := *bot
		if botID == "" {
			var state struct {
				Bots []struct {
					ID string `json:"id"`
				} `json:"bots"`
			}
			if err := c.getJSON("/api/state", &state); err != nil {
				return err
			}
			if len(state.Bots) == 0 {
				return fmt.Errorf("no bots on server")
			}
			botID = state.Bots[0].ID
		}
		raw, _, err := c.do(http.MethodPost, "/api/bots/"+botID+"/threads", map[string]string{"title": *title})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "rm":
		if len(args) < 3 {
			return fmt.Errorf("usage: go-bot threads rm BOT_ID THREAD_ID")
		}
		_, _, err := c.do(http.MethodDelete, "/api/bots/"+args[1]+"/threads/"+args[2], nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"deleted": args[2]})
	case "open":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot threads open THREAD_ID")
		}
		var out any
		if err := c.getJSON("/api/threads/"+args[1], &out); err != nil {
			return err
		}
		return printJSON(out)
	}
	return fmt.Errorf("usage: go-bot threads list|new|rm|open ...")
}

// runApprove implements `go-bot approve <id> allow|deny`.
func runApprove(args []string) error {
	if len(args) != 2 || (args[1] != "allow" && args[1] != "deny") {
		return fmt.Errorf("usage: go-bot approve APPROVAL_ID allow|deny")
	}
	c := newCLIClient("")
	raw, _, err := c.do(http.MethodPost, "/api/approvals/"+args[0], map[string]string{"decision": args[1]})
	if err != nil {
		return err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	return printJSON(out)
}

// runSearch implements `go-bot search "query" [--limit N]` via GET /api/search.
func runSearch(args []string) error {
	fs := flag.NewFlagSet("go-bot search", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "max results")
	base := fs.String("url", "", "server base URL (or GOBOT_URL)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: go-bot search \"query\" [--limit N]")
	}
	c := newCLIClient(*base)
	var out any
	if err := c.getJSON("/api/search?q="+url.QueryEscape(strings.Join(fs.Args(), " "))+"&limit="+strconv.Itoa(*limit), &out); err != nil {
		return err
	}
	return printJSON(out)
}

// runAudit implements `go-bot audit [--limit N] [--csv]` via GET /api/audit.
func runAudit(args []string) error {
	fs := flag.NewFlagSet("go-bot audit", flag.ContinueOnError)
	limit := fs.Int("limit", 100, "max entries")
	csv := fs.Bool("csv", false, "output raw CSV instead of JSON")
	base := fs.String("url", "", "server base URL (or GOBOT_URL)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c := newCLIClient(*base)
	path := "/api/audit?limit=" + strconv.Itoa(*limit)
	if *csv {
		path += "&format=csv"
		raw, _, err := c.do(http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		fmt.Print(string(raw))
		return nil
	}
	var out any
	if err := c.getJSON(path, &out); err != nil {
		return err
	}
	return printJSON(out)
}

// splitTrailingFlags separates leading positional words from trailing
// --flags: Go's flag package stops parsing at the first non-flag arg, so
// commands accept `notes add "text" --bot ID` by re-parsing the tail.
func splitTrailingFlags(args []string) (words []string, flags []string) {
	i := 0
	for i < len(args) && !(len(args[i]) > 2 && args[i][:2] == "--") {
		words = append(words, args[i])
		i++
	}
	return words, args[i:]
}

func parseTrailing(fs *flag.FlagSet, args []string) []string {
	words, rest := splitTrailingFlags(args)
	if err := fs.Parse(rest); err != nil {
		return nil
	}
	return words
}

// runMemory implements `go-bot memory list|forget`.
// Memories are written by the chat `remember` tool; CLI only reads/deletes.
func runMemory(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-bot memory list|forget ...")
	}
	c := newCLIClient("")
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("go-bot memory list", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		botID, err := defaultBot(c, *bot)
		if err != nil {
			return err
		}
		var out any
		if err := c.getJSON("/api/bots/"+botID+"/memories", &out); err != nil {
			return err
		}
		return printJSON(out)
	case "forget":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot memory forget MEMORY_ID")
		}
		_, code, err := c.do(http.MethodDelete, "/api/memories/"+args[1], nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"forgot": args[1], "status": code})
	}
	return fmt.Errorf("usage: go-bot memory list|forget ...")
}

// runNotes implements `go-bot notes add|list|clear`.
func runNotes(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-bot notes add|list|clear ...")
	}
	c := newCLIClient("")
	botFor := func(explicit string) (string, error) { return defaultBot(c, explicit) }
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("go-bot notes list", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		botID, err := botFor(*bot)
		if err != nil {
			return err
		}
		var out any
		if err := c.getJSON("/api/bots/"+botID+"/scratchpad", &out); err != nil {
			return err
		}
		return printJSON(out)
	case "add":
		fs := flag.NewFlagSet("go-bot notes add", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		words := parseTrailing(fs, args[1:])
		if words == nil {
			return fmt.Errorf("flag parse error")
		}
		if len(words) == 0 {
			return fmt.Errorf("usage: go-bot notes add [--bot ID] \"text\"")
		}
		botID, err := botFor(*bot)
		if err != nil {
			return err
		}
		raw, _, err := c.do(http.MethodPost, "/api/bots/"+botID+"/scratchpad", map[string]string{"text": strings.Join(words, " ")})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "clear":
		fs := flag.NewFlagSet("go-bot notes clear", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		botID, err := botFor(*bot)
		if err != nil {
			return err
		}
		_, code, err := c.do(http.MethodDelete, "/api/bots/"+botID+"/scratchpad", nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"cleared": true, "status": code})
	}
	return fmt.Errorf("usage: go-bot notes add|list|clear ...")
}

// runBoard implements `go-bot board list|add|move|del`.
func runBoard(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-bot board list|add|move|del ...")
	}
	c := newCLIClient("")
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("go-bot board list", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		botID, err := defaultBot(c, *bot)
		if err != nil {
			return err
		}
		var out any
		if err := c.getJSON("/api/bots/"+botID+"/board", &out); err != nil {
			return err
		}
		return printJSON(out)
	case "add":
		fs := flag.NewFlagSet("go-bot board add", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		status := fs.String("status", "todo", "column: todo|doing|done")
		words := parseTrailing(fs, args[1:])
		if words == nil {
			return fmt.Errorf("flag parse error")
		}
		if len(words) == 0 {
			return fmt.Errorf("usage: go-bot board add [--bot ID] [--status S] \"title\"")
		}
		botID, err := defaultBot(c, *bot)
		if err != nil {
			return err
		}
		raw, _, err := c.do(http.MethodPost, "/api/bots/"+botID+"/board", map[string]string{"title": strings.Join(words, " "), "status": *status})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "move":
		if len(args) < 3 {
			return fmt.Errorf("usage: go-bot board move CARD_ID todo|doing|done")
		}
		raw, _, err := c.do(http.MethodPatch, "/api/board/"+args[1], map[string]string{"status": args[2]})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "del":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot board del CARD_ID")
		}
		_, code, err := c.do(http.MethodDelete, "/api/board/"+args[1], nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"deleted": args[1], "status": code})
	}
	return fmt.Errorf("usage: go-bot board list|add|move|del ...")
}

// runCron implements `go-bot cron list|add|edit|pause|resume|run|del`
// (alias `routines`). Backend has no manual-run endpoint, so `run`
// re-posts the stored prompt as a one-shot routine with the same cron;
// pause/resume flip `enabled` via PATCH.
func runCron(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-bot cron list|add|edit|pause|resume|run|del ...")
	}
	c := newCLIClient("")
	botFor := func(explicit string) (string, error) { return defaultBot(c, explicit) }
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("go-bot cron list", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		botID, err := botFor(*bot)
		if err != nil {
			return err
		}
		var out any
		if err := c.getJSON("/api/bots/"+botID+"/routines", &out); err != nil {
			return err
		}
		return printJSON(out)
	case "add":
		fs := flag.NewFlagSet("go-bot cron add", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		cron := fs.String("cron", "", "schedule e.g. '0 9 * * *' (required)")
		name := fs.String("name", "", "job name")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *cron == "" || fs.NArg() == 0 {
			return fmt.Errorf("usage: go-bot cron add --cron CRON [--bot ID] [--name N] \"prompt\"")
		}
		botID, err := botFor(*bot)
		if err != nil {
			return err
		}
		raw, _, err := c.do(http.MethodPost, "/api/bots/"+botID+"/routines", map[string]string{"cron": *cron, "prompt": strings.Join(fs.Args(), " "), "name": *name})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "edit":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot cron edit ID [--cron C] [--prompt P] [--name N]")
		}
		fs := flag.NewFlagSet("go-bot cron edit", flag.ContinueOnError)
		cron := fs.String("cron", "", "new schedule")
		prompt := fs.String("prompt", "", "new prompt")
		name := fs.String("name", "", "new name")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		body := map[string]string{}
		if *cron != "" {
			body["cron"] = *cron
		}
		if *prompt != "" {
			body["prompt"] = *prompt
		}
		if *name != "" {
			body["name"] = *name
		}
		if len(body) == 0 {
			return fmt.Errorf("usage: go-bot cron edit ID [--cron C] [--prompt P] [--name N]")
		}
		raw, _, err := c.do(http.MethodPatch, "/api/routines/"+args[1], body)
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "pause", "resume":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot cron %s ID", args[0])
		}
		enabled := args[0] == "resume"
		raw, _, err := c.do(http.MethodPatch, "/api/routines/"+args[1], map[string]bool{"enabled": enabled})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot cron run ID [--bot ID]")
		}
		// No backend manual-run; fetch the routine's prompt and re-post it
		// as a chat message on its thread so the turn executes now.
		fs := flag.NewFlagSet("go-bot cron run", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id (default: first bot)")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		botID, err := botFor(*bot)
		if err != nil {
			return err
		}
		var routines []struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
		}
		if err := c.getJSON("/api/bots/"+botID+"/routines", &routines); err != nil {
			return err
		}
		prompt := ""
		for _, r := range routines {
			if r.ID == args[1] {
				prompt = r.Prompt
			}
		}
		if prompt == "" {
			return fmt.Errorf("routine %q not found for bot %s", args[1], botID)
		}
		_, threadID, err := resolveThread(c, botID, "")
		if err != nil {
			return err
		}
		raw, _, err := c.do(http.MethodPost, "/api/threads/"+threadID+"/messages", map[string]string{"content": prompt})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "del":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot cron del ID")
		}
		_, code, err := c.do(http.MethodDelete, "/api/routines/"+args[1], nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"deleted": args[1], "status": code})
	}
	return fmt.Errorf("usage: go-bot cron list|add|edit|pause|resume|run|del ...")
}

// runSkills implements `go-bot skills list|add|rm`.
// Backend requires name + body; description optional.
func runSkills(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-bot skills list|add|rm ...")
	}
	c := newCLIClient("")
	switch args[0] {
	case "list":
		var out any
		if err := c.getJSON("/api/skills", &out); err != nil {
			return err
		}
		return printJSON(out)
	case "add":
		fs := flag.NewFlagSet("go-bot skills add", flag.ContinueOnError)
		name := fs.String("name", "", "skill name (required)")
		desc := fs.String("desc", "", "description")
		body := fs.String("body", "", "skill body (required)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*name) == "" || strings.TrimSpace(*body) == "" {
			return fmt.Errorf("usage: go-bot skills add --name N --body B [--desc D]")
		}
		raw, _, err := c.do(http.MethodPost, "/api/skills", map[string]string{"name": *name, "description": *desc, "body": *body})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot skills rm SKILL_ID")
		}
		_, code, err := c.do(http.MethodDelete, "/api/skills/"+args[1], nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"deleted": args[1], "status": code})
	}
	return fmt.Errorf("usage: go-bot skills list|add|rm ...")
}

// runRules implements `go-bot rules list|add|rm`.
// Backend shape: {scope, kind:allow|deny, tool, args}.
func runRules(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: go-bot rules list|add|rm ...")
	}
	c := newCLIClient("")
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("go-bot rules list", flag.ContinueOnError)
		bot := fs.String("bot", "", "bot id filter (scope)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		path := "/api/rules"
		if *bot != "" {
			botID, err := defaultBot(c, *bot)
			if err != nil {
				return err
			}
			path += "?botId=" + url.QueryEscape(botID)
		}
		var out any
		if err := c.getJSON(path, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "add":
		fs := flag.NewFlagSet("go-bot rules add", flag.ContinueOnError)
		scope := fs.String("scope", "", "bot id scope (empty = global)")
		kind := fs.String("kind", "allow", "allow|deny")
		tool := fs.String("tool", "", "tool name (empty = all)")
		params := fs.String("args", "", "args match (empty = all)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *kind != "allow" && *kind != "deny" {
			return fmt.Errorf("kind must be allow or deny")
		}
		scopeID := *scope
		if scopeID != "" {
			id, err := defaultBot(c, scopeID)
			if err != nil {
				return err
			}
			scopeID = id
		}
		raw, _, err := c.do(http.MethodPost, "/api/rules", map[string]string{"scope": scopeID, "kind": *kind, "tool": *tool, "args": *params})
		if err != nil {
			return err
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		return printJSON(out)
	case "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: go-bot rules rm RULE_ID")
		}
		_, code, err := c.do(http.MethodDelete, "/api/rules/"+args[1], nil)
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"deleted": args[1], "status": code})
	}
	return fmt.Errorf("usage: go-bot rules list|add|rm ...")
}

// defaultBot resolves an explicit bot id/name or falls back to the first bot.
func defaultBot(c *cliClient, explicit string) (string, error) {
	var state struct {
		Bots []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"bots"`
	}
	if err := c.getJSON("/api/state", &state); err != nil {
		return "", err
	}
	if len(state.Bots) == 0 {
		return "", fmt.Errorf("no bots on server")
	}
	if explicit == "" {
		return state.Bots[0].ID, nil
	}
	for _, b := range state.Bots {
		if b.ID == explicit || b.Name == explicit {
			return b.ID, nil
		}
	}
	return "", fmt.Errorf("bot %q not found", explicit)
}

// the first bot and its first thread from /api/state.
func resolveThread(c *cliClient, botID, threadID string) (before int, resolved string, err error) {
	var state struct {
		Bots []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Threads []struct {
				ID string `json:"id"`
			} `json:"threads"`
		} `json:"bots"`
	}
	if err := c.getJSON("/api/state", &state); err != nil {
		return 0, "", err
	}
	if len(state.Bots) == 0 {
		return 0, "", fmt.Errorf("no bots on server")
	}
	bot := state.Bots[0]
	if botID != "" {
		found := false
		for _, b := range state.Bots {
			if b.ID == botID || b.Name == botID {
				bot = b
				found = true
				break
			}
		}
		if !found {
			return 0, "", fmt.Errorf("bot %q not found", botID)
		}
	}
	if threadID != "" {
		resolved = threadID
	} else {
		if len(bot.Threads) == 0 {
			return 0, "", fmt.Errorf("bot %q has no threads", bot.ID)
		}
		resolved = bot.Threads[0].ID
	}
	var thread struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := c.getJSON("/api/threads/"+resolved, &thread); err != nil {
		return 0, "", err
	}
	return len(thread.Messages), resolved, nil
}

// waitAssistant polls the thread until a new assistant message appears past
// `before`, returning it. Polling (not SSE) keeps the CLI dependency-free.
func waitAssistant(c *cliClient, threadID string, before int, timeout time.Duration) (any, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var thread struct {
			Messages []struct {
				ID      string `json:"id"`
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := c.getJSON("/api/threads/"+threadID, &thread); err != nil {
			return nil, err
		}
		for i := len(thread.Messages) - 1; i >= 0 && i >= before; i-- {
			m := thread.Messages[i]
			if m.Role == "assistant" && strings.TrimSpace(m.Content) != "" {
				return m, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timed out waiting for assistant reply")
}
// rather than server flags or the legacy backup/restore verbs.
func isCLICommand(name string) bool {
	switch name {
	case "chat", "state", "bots", "threads", "approve",
		"search", "audit", "memory", "notes", "board",
		"cron", "routines", "skills", "rules",
		"config", "model", "status", "doctor",
		"plugins", "mcp", "gateway":
		return true
	}
	return false
}
