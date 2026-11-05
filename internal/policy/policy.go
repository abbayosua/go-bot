package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gobot/internal/store"
)

// Danger stays refused in every mode. The lists are small and explicit on
// purpose: a refusal is a tool-visible fact the model can route around, not a
// permission question for the user.
var destructiveCommands = []string{
	"rm -rf /", "rm -fr /", "mkfs", "dd of=/dev/", "> /dev/sd", ":(){",
	"shutdown", "reboot", "chmod -R 777 /", "sudo ",
}

var destructiveArgs = []string{
	"user_password", "user_token", "api_key", "secret",
}

// Gate is one approval decision with the reason attached. Hold means the
// call is recorded but nobody has answered yet; only the approval card can
// clear it.
type Gate struct {
	Allow  bool
	Hold   bool
	Reason string
}

// Check runs, in order: the tool-visible safety floor, the user's durable
// rules, then the approval mode. Every branch returns the row the feed and
// the resume path can rely on.
func Check(ctx context.Context, st *store.Store, botID, threadID, mode, tool, rawArgs string) (Gate, error) {
	if reason := safetyFloor(tool, rawArgs); reason != "" {
		return Gate{Allow: false, Reason: reason}, nil
	}
	rules, err := st.ListApprovalRules(ctx, botID)
	if err != nil {
		return Gate{}, err
	}
	matches := func(rule store.ApprovalRule) bool {
		if rule.Tool != "" && rule.Tool != tool {
			return false
		}
		if args := strings.TrimSpace(rule.Args); args != "" {
			var wanted, got map[string]any
			if err := json.Unmarshal([]byte(args), &wanted); err != nil {
				return false
			}
			if err := json.Unmarshal([]byte(orEmpty(rawArgs)), &got); err != nil {
				return false
			}
			for key, value := range wanted {
				actual, ok := got[key]
				if !ok || fmt.Sprint(actual) != fmt.Sprint(value) {
					return false
				}
			}
		}
		return true
	}
	for _, rule := range rules {
		if rule.Kind != "deny" || !matches(rule) {
			continue
		}
		return Gate{Allow: false, Reason: "refused by an approval rule"}, nil
	}
	for _, rule := range rules {
		if rule.Kind != "allow" || !matches(rule) {
			continue
		}
		return Gate{Allow: true, Reason: ""}, nil
	}
	if readOnly(tool) {
		return Gate{Allow: true}, nil
	}
	if mode == "never" {
		return Gate{Allow: true}, nil
	}
	return Gate{Hold: true, Reason: "needs a person"}, nil
}

func safetyFloor(tool, rawArgs string) string {
	if tool == "shell" {
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(orEmpty(rawArgs)), &args); err == nil {
			normalized := strings.Join(strings.Fields(args.Command), " ")
			for _, pattern := range destructiveCommands {
				if strings.Contains(normalized, pattern) {
					return fmt.Sprintf("refused: %s; run something narrower", describeDanger(pattern))
				}
			}
		}
	}
	fields := secretFields(rawArgs)
	if len(fields) > 0 {
		return fmt.Sprintf("refused: the call carries secret material (%s); never route it through tools", strings.Join(fields, ", "))
	}
	return ""
}

func secretFields(rawArgs string) []string {
	var args map[string]any
	if err := json.Unmarshal([]byte(orEmpty(rawArgs)), &args); err != nil {
		return nil
	}
	var found []string
	for _, name := range destructiveArgs {
		for key := range args {
			if strings.EqualFold(key, name) || strings.Contains(strings.ToLower(key), name) {
				found = append(found, key)
			}
		}
	}
	return found
}

func describeDanger(pattern string) string {
	switch pattern {
	case "rm -rf /", "rm -fr /":
		return "would delete the filesystem root"
	case "chmod -R 777 /":
		return "would change permissions across the host"
	case "> /dev/sd", "dd of=/dev/":
		return "would write directly to a block device"
	case ":(){":
		return "fork bomb"
	case "shutdown":
		return "would power off the host"
	case "reboot":
		return "would restart the host"
	case "sudo ":
		return "runs as root"
	case "mkfs":
		return "would format a device"
	default:
		return "would damage the host"
	}
}

func readOnly(tool string) bool {
	switch tool {
	case "recall", "read_file", "list_files", "web_fetch":
		return true
	default:
		return false
	}
}

func orEmpty(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}
