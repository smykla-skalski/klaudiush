package parser

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	prMergeEndpointSegments = 6
	reposSegment            = "repos"
	pullsSegment            = "pulls"
)

// ParsePRMergeEndpoint reports whether a normalized REST endpoint is the one
// that merges a pull request, repos/{owner}/{repo}/pulls/{number}/merge, and
// returns its owner/repo and pull request number.
func ParsePRMergeEndpoint(endpoint string) (string, int, bool) {
	segments := strings.Split(endpoint, "/")
	if len(segments) != prMergeEndpointSegments {
		return "", 0, false
	}

	if segments[0] != reposSegment || segments[3] != pullsSegment || segments[5] != mergeSubCmd {
		return "", 0, false
	}

	if segments[1] == "" || segments[2] == "" {
		return "", 0, false
	}

	number, err := strconv.Atoi(segments[4])
	if err != nil || number <= 0 {
		return "", 0, false
	}

	return segments[1] + "/" + segments[2], number, true
}

// IsPRMergeRequest reports whether a method and normalized endpoint merge a
// pull request through the REST API.
func IsPRMergeRequest(method, endpoint string) bool {
	if method != methodPUT {
		return false
	}

	_, _, ok := ParsePRMergeEndpoint(endpoint)

	return ok
}

// ParseRequestFields reads the top-level string fields of a request body: a
// JSON object, or httpie and xh request items, one per line. The bool is false
// when the body is neither, so its fields cannot be known.
func ParseRequestFields(body string) (map[string]string, bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return map[string]string{}, true
	}

	if strings.HasPrefix(trimmed, "{") {
		return parseJSONFields(trimmed)
	}

	return parseRequestItems(trimmed)
}

// parseJSONFields reads the string values of a JSON object body.
func parseJSONFields(body string) (map[string]string, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, false
	}

	fields := make(map[string]string, len(raw))

	for key, value := range raw {
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			continue
		}

		fields[key] = text
	}

	return fields, true
}

// parseRequestItems reads "key=value" and "key:=<JSON string>" items. Any other
// item form makes the whole body unknown rather than partly read.
func parseRequestItems(body string) (map[string]string, bool) {
	fields := map[string]string{}

	for line := range strings.SplitSeq(body, "\n") {
		if key, rawJSON, found := strings.Cut(line, ":="); found && !strings.Contains(key, "=") {
			var text string
			if err := json.Unmarshal([]byte(rawJSON), &text); err != nil {
				return nil, false
			}

			fields[key] = text

			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found || key == "" || strings.HasPrefix(value, "=") {
			return nil, false
		}

		fields[key] = value
	}

	return fields, true
}
