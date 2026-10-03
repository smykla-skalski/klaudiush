package harness_test

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

func post(model *harness.ScriptedModel, path, body string) string {
	resp, err := http.Post(model.URL()+path, "application/json", strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())

	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())

	return string(data)
}

// sseData returns the decoded data payloads of a server-sent event stream.
func sseData(stream string) []map[string]any {
	var events []map[string]any

	scanner := bufio.NewScanner(strings.NewReader(stream))
	for scanner.Scan() {
		line, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok || line == "[DONE]" {
			continue
		}

		var event map[string]any
		Expect(json.Unmarshal([]byte(line), &event)).To(Succeed())
		events = append(events, event)
	}

	return events
}

func scriptPrompt(script harness.Script) string {
	prompt, err := script.Prompt("run it")
	Expect(err).NotTo(HaveOccurred())

	return prompt
}

func jsonString(s string) string {
	data, err := json.Marshal(s)
	Expect(err).NotTo(HaveOccurred())

	return string(data)
}

var _ = Describe("ScriptedModel", func() {
	var (
		model  *harness.ScriptedModel
		script harness.Script
	)

	BeforeEach(func() {
		model = harness.NewScriptedModel()
		DeferCleanup(model.Close)

		script = harness.Script{
			Turns: [][]harness.Call{
				{{Tool: "Bash", Args: map[string]any{"command": "touch a"}}},
				{
					{Tool: "Bash", Args: map[string]any{"command": "touch b"}},
					{Tool: "apply_patch", Input: "*** Begin Patch\n*** End Patch\n"},
				},
			},
			Final: "all done",
		}
	})

	Context("Anthropic Messages API", func() {
		messages := func(extra string) string {
			return `{"stream":true,"messages":[{"role":"user","content":[{"type":"text","text":` +
				jsonString(scriptPrompt(script)) + `}]}` + extra + `]}`
		}

		It("streams the first turn as tool_use blocks", func() {
			events := sseData(post(model, "/v1/messages", messages("")))
			Expect(events[0]["type"]).To(Equal("message_start"))

			var blocks []map[string]any

			for _, event := range events {
				if event["type"] == "content_block_start" {
					blocks = append(blocks, event["content_block"].(map[string]any))
				}
			}

			Expect(blocks).To(HaveLen(1))
			Expect(blocks[0]["name"]).To(Equal("Bash"))
			Expect(post(model, "/v1/messages", messages(""))).To(ContainSubstring(`touch a`))
		})

		It("moves to the next turn after the calls were made and ends with the final text", func() {
			afterFirst := `,{"role":"assistant","content":[{"type":"tool_use","id":"t1"}]},` +
				`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1"}]}`
			second := post(model, "/v1/messages", messages(afterFirst))
			Expect(second).To(ContainSubstring("touch b"))
			Expect(second).To(ContainSubstring(`"stop_reason":"tool_use"`))

			afterAll := afterFirst + `,{"role":"assistant","content":[{"type":"tool_use","id":"t2"},` +
				`{"type":"tool_use","id":"t3"}]}`
			Expect(post(model, "/v1/messages", messages(afterAll))).To(ContainSubstring("all done"))
		})

		It("answers a non-streaming side request without a script", func() {
			body := post(model, "/v1/messages", `{"messages":[{"role":"user","content":"title?"}]}`)

			var message map[string]any
			Expect(json.Unmarshal([]byte(body), &message)).To(Succeed())
			Expect(message["stop_reason"]).To(Equal("end_turn"))
			Expect(body).To(ContainSubstring(`"text":"ok"`))
		})

		It("rejects a body that is not JSON", func() {
			Expect(post(model, "/v1/messages", "{")).To(ContainSubstring("unexpected"))
		})
	})

	Context("OpenAI Responses API", func() {
		input := func(extra string) string {
			return `{"input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"sys"}]},` +
				`{"type":"message","role":"user","content":[{"type":"input_text","text":` +
				jsonString(
					scriptPrompt(script),
				) + `}]}` + extra + `]}`
		}

		It("emits function and custom tool calls per turn", func() {
			first := post(model, "/v1/responses", input(""))
			Expect(first).To(ContainSubstring(`"type":"function_call"`))
			Expect(first).To(ContainSubstring("response.completed"))

			second := post(
				model,
				"/v1/responses",
				input(`,{"type":"function_call"},{"type":"function_call_output"}`),
			)
			Expect(second).To(ContainSubstring(`"type":"custom_tool_call"`))
			Expect(second).To(ContainSubstring("touch b"))

			final := post(model, "/v1/responses", input(
				`,{"type":"function_call"},{"type":"function_call"},{"type":"custom_tool_call"}`))
			Expect(final).To(ContainSubstring("all done"))
		})

		It("rejects a body that is not JSON", func() {
			Expect(post(model, "/v1/responses", "{")).To(ContainSubstring("unexpected"))
		})
	})

	Context("OpenAI chat completions", func() {
		chat := func(stream bool, extra string) string {
			flag := "false"
			if stream {
				flag = "true"
			}

			return `{"stream":` + flag + `,"messages":[{"role":"system","content":"sys"},` +
				`{"role":"user","content":` + jsonString(scriptPrompt(script)) + `}` + extra + `]}`
		}

		It("streams tool calls and finishes with [DONE]", func() {
			stream := post(model, "/v1/chat/completions", chat(true, ""))
			Expect(stream).To(ContainSubstring(`"finish_reason":"tool_calls"`))
			Expect(stream).To(HaveSuffix("data: [DONE]\n\n"))
		})

		It("answers with the final text once every call was made", func() {
			made := `,{"role":"assistant","tool_calls":[{},{},{}]}`
			body := post(model, "/v1/chat/completions", chat(false, made))
			Expect(body).To(ContainSubstring("all done"))
			Expect(body).To(ContainSubstring(`"finish_reason":"stop"`))
		})

		It("rejects a body that is not JSON", func() {
			Expect(post(model, "/v1/chat/completions", "{")).To(ContainSubstring("unexpected"))
		})
	})

	It("answers connectivity probes and 404s other paths", func() {
		resp, err := http.Head(model.URL() + "/api/hello")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Body.Close()).To(Succeed())

		resp, err = http.Post(model.URL()+"/v1/other", "application/json", strings.NewReader("{}"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		Expect(resp.Body.Close()).To(Succeed())
	})

	It("records requests so a check can see what reached the model", func() {
		post(model, "/v1/messages", `{"messages":[{"role":"user","content":"POL001 reason"}]}`)
		Expect(model.Saw("POL001")).To(BeTrue())
		Expect(model.Saw("EVID001")).To(BeFalse())
		Expect(model.Requests()).To(HaveLen(1))
		Expect(model.Requests()[0].Path).To(Equal("/v1/messages"))
	})
})

var _ = Describe("Script", func() {
	It("uses a default final text", func() {
		model := harness.NewScriptedModel()
		DeferCleanup(model.Close)

		prompt := scriptPrompt(harness.Script{})
		body := post(
			model,
			"/v1/messages",
			`{"messages":[{"role":"user","content":`+jsonString(prompt)+`}]}`,
		)
		Expect(body).To(ContainSubstring(`"text":"done"`))
	})

	It("ignores a marker with a payload that does not decode", func() {
		model := harness.NewScriptedModel()
		DeferCleanup(model.Close)

		for _, text := range []string{"KLAUDIUSH-HARNESS-SCRIPT:@@@", "KLAUDIUSH-HARNESS-SCRIPT:bm90IGpzb24="} {
			body := post(
				model,
				"/v1/messages",
				`{"messages":[{"role":"user","content":`+jsonString(text)+`}]}`,
			)
			Expect(body).To(ContainSubstring(`"text":"ok"`))
		}
	})
})
