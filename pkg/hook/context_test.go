package hook

import "testing"

func TestContextIsAfterTool(t *testing.T) {
	cases := []struct {
		name string
		ctx  *Context
		want bool
	}{
		{"canonical after-tool", &Context{Event: CanonicalEventAfterTool}, true},
		{"canonical before-tool", &Context{Event: CanonicalEventBeforeTool}, false},
		{"legacy post-tool event type", &Context{EventType: EventTypePostToolUse}, true},
		{"legacy pre-tool event type", &Context{EventType: EventTypePreToolUse}, false},
		{
			"canonical event wins over event type",
			&Context{Event: CanonicalEventBeforeTool, EventType: EventTypePostToolUse},
			false,
		},
	}

	for _, tc := range cases {
		if got := tc.ctx.IsAfterTool(); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestContextToolFailed(t *testing.T) {
	cases := []struct {
		name string
		ctx  *Context
		want bool
	}{
		{"not run", &Context{}, false},
		{"succeeded", &Context{ToolExecuted: true, ToolSucceeded: true}, false},
		{"failed", &Context{ToolExecuted: true}, true},
	}

	for _, tc := range cases {
		if got := tc.ctx.ToolFailed(); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
