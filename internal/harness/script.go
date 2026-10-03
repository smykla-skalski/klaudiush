package harness

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
)

// scriptMarker prefixes the encoded script inside a prompt. The scripted
// model looks for it in the first user message of each conversation, so a
// subagent prompt can carry its own nested script.
const scriptMarker = "KLAUDIUSH-HARNESS-SCRIPT:"

var scriptPattern = regexp.MustCompile(regexp.QuoteMeta(scriptMarker) + `([A-Za-z0-9+/=]+)`)

// Call is one tool call the scripted model makes. Args holds JSON function
// arguments; Input holds the raw text of a freeform tool (Codex apply_patch).
type Call struct {
	Tool  string         `json:"tool"`
	Args  map[string]any `json:"args,omitempty"`
	Input string         `json:"input,omitempty"`
}

// Script is what the scripted model does in one conversation: each turn is a
// set of tool calls made in one response (more than one runs them in
// parallel), then Final is the closing text.
type Script struct {
	Turns [][]Call `json:"turns"`
	Final string   `json:"final"`
}

// Prompt renders the script as a user prompt the scripted model understands.
func (s Script) Prompt(note string) (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", errors.Wrap(err, "encoding script")
	}

	return note + "\n" + scriptMarker + base64.StdEncoding.EncodeToString(data), nil
}

// findScript decodes the first script embedded in text.
func findScript(text string) (Script, bool) {
	match := scriptPattern.FindStringSubmatch(text)
	if match == nil {
		return Script{}, false
	}

	data, err := base64.StdEncoding.DecodeString(match[1])
	if err != nil {
		return Script{}, false
	}

	var script Script
	if err := json.Unmarshal(data, &script); err != nil {
		return Script{}, false
	}

	return script, true
}

// next returns the calls of the first turn not yet fully made, given how many
// tool calls the conversation already holds, or false when the script has run
// out and the model should answer with Final. Counting calls instead of
// assistant messages keeps a parallel turn whose calls a harness records one
// by one from being counted twice.
func (s Script) next(callsMade int) ([]Call, bool) {
	made := 0

	for _, turn := range s.Turns {
		made += len(turn)
		if callsMade < made {
			return turn, true
		}
	}

	return nil, false
}

func (s Script) final() string {
	if strings.TrimSpace(s.Final) == "" {
		return "done"
	}

	return s.Final
}
