package parser

import (
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

const (
	reposSegment        = "repos"
	repositoriesSegment = "repositories"
	pullsSegment        = "pulls"
)

// ParsePRMergeEndpoint reports whether a normalized REST endpoint is the one
// that merges a pull request - repos/{owner}/{repo}/pulls/{number}/merge, or
// repositories/{id}/pulls/{number}/merge, its numeric-ID alias - and returns
// the API path of that pull request (the endpoint without /merge).
func ParsePRMergeEndpoint(endpoint string) (string, bool) {
	prPath, found := strings.CutSuffix(endpoint, "/"+mergeSubCmd)
	if !found {
		return "", false
	}

	segments := strings.Split(prPath, "/")

	var repo []string

	switch {
	case len(segments) == 5 && segments[0] == reposSegment:
		repo = segments[1:3]
	case len(segments) == 4 && segments[0] == repositoriesSegment:
		repo = segments[1:2]

		if _, err := strconv.Atoi(repo[0]); err != nil {
			return "", false
		}
	default:
		return "", false
	}

	if slices.Contains(repo, "") || segments[len(segments)-2] != pullsSegment {
		return "", false
	}

	number, err := strconv.Atoi(segments[len(segments)-1])
	if err != nil || number <= 0 {
		return "", false
	}

	return prPath, true
}

// IsPRMergeRequest reports whether a method and normalized endpoint merge a
// pull request through the REST API.
func IsPRMergeRequest(method, endpoint string) bool {
	if method != methodPUT {
		return false
	}

	_, ok := ParsePRMergeEndpoint(endpoint)

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

// parseRequestItems reads "key=value" and "key:=<JSON string>" items, one per
// line. Any other item form makes the whole body unknown rather than partly
// read.
func parseRequestItems(body string) (map[string]string, bool) {
	return ParseRequestItemList(strings.Split(body, "\n"))
}

// ParseRequestItemList reads httpie and xh request items given one by one, so
// a value may hold newlines. The bool is false when an item has another form.
func ParseRequestItemList(items []string) (map[string]string, bool) {
	fields := map[string]string{}

	for _, item := range items {
		key, value, ok := parseRequestItem(item)
		if !ok {
			return nil, false
		}

		fields[key] = value
	}

	return fields, true
}

// parseRequestItem reads one "key=value" or "key:=<JSON string>" item.
func parseRequestItem(item string) (string, string, bool) {
	if key, rawJSON, found := strings.Cut(item, ":="); found && !strings.Contains(key, "=") {
		var text string
		if err := json.Unmarshal([]byte(rawJSON), &text); err != nil {
			return "", "", false
		}

		return key, text, true
	}

	key, value, found := strings.Cut(item, "=")
	if !found || key == "" || strings.HasPrefix(value, "=") {
		return "", "", false
	}

	return key, value, true
}

// QueryFields returns every value of each query string parameter of a raw
// endpoint or URL, in order. GitHub reads request parameters from the query
// too, and gh api moves its fields there when --input carries the body. A key
// can repeat, and which value the API reads is not documented, so all are
// kept.
func QueryFields(raw string) map[string][]string {
	_, query, found := strings.Cut(raw, "?")
	if !found {
		return nil
	}

	values, err := url.ParseQuery(query)
	if err != nil {
		return nil
	}

	return values
}
