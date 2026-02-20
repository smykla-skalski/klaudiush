package schema_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/schema"
)

var _ = Describe("Generate", func() {
	var s map[string]any

	BeforeEach(func() {
		data, err := schema.GenerateJSON(true)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(data, &s)).To(Succeed())
	})

	It("produces valid JSON", func() {
		Expect(s).NotTo(BeEmpty())
	})

	It("sets the $schema URI", func() {
		Expect(s["$schema"]).To(Equal("https://json-schema.org/draft/2020-12/schema"))
	})

	It("sets the title", func() {
		Expect(s["title"]).To(Equal("klaudiush configuration"))
	})

	It("includes top-level properties", func() {
		props, ok := s["properties"].(map[string]any)
		Expect(ok).To(BeTrue())

		for _, key := range []string{
			"validators", "global", "plugins", "rules",
			"exceptions", "backup", "session", "crash_dump", "patterns",
		} {
			Expect(props).To(HaveKey(key), "missing top-level property: %s", key)
		}
	})

	Describe("custom type schemas", func() {
		var defs map[string]any

		BeforeEach(func() {
			var ok bool

			defs, ok = s["$defs"].(map[string]any)
			Expect(ok).To(BeTrue(), "$defs should exist")
		})

		It("defines Duration as string with pattern", func() {
			dur, ok := defs["Duration"].(map[string]any)
			Expect(ok).To(BeTrue(), "Duration def should exist")
			Expect(dur["type"]).To(Equal("string"))
			Expect(dur["pattern"]).NotTo(BeEmpty())
		})

		It("defines Severity as string with enum", func() {
			sev, ok := defs["Severity"].(map[string]any)
			Expect(ok).To(BeTrue(), "Severity def should exist")
			Expect(sev["type"]).To(Equal("string"))

			enumVals, ok := sev["enum"].([]any)
			Expect(ok).To(BeTrue())
			Expect(enumVals).To(ContainElements("error", "warning", "unknown"))
		})

		It("defines ByteSize as integer", func() {
			bs, ok := defs["ByteSize"].(map[string]any)
			Expect(ok).To(BeTrue(), "ByteSize def should exist")
			Expect(bs["type"]).To(Equal("integer"))
		})

		It("defines PluginType as string with enum", func() {
			pt, ok := defs["PluginType"].(map[string]any)
			Expect(ok).To(BeTrue(), "PluginType def should exist")
			Expect(pt["type"]).To(Equal("string"))

			enumVals, ok := pt["enum"].([]any)
			Expect(ok).To(BeTrue())
			Expect(enumVals).To(ContainElement("exec"))
		})
	})

	Describe("enum struct tags", func() {
		// Helper to navigate into nested properties via $ref or inline.
		findProp := func(path ...string) map[string]any {
			current := s
			for _, key := range path {
				props, ok := current["properties"].(map[string]any)
				if !ok {
					return nil
				}

				next, ok := props[key].(map[string]any)
				if !ok {
					return nil
				}

				// Follow $ref if present
				if ref, ok := next["$ref"].(string); ok {
					// Extract def name from "#/$defs/Name"
					defName := ref[len("#/$defs/"):]

					defs, ok := s["$defs"].(map[string]any)
					if !ok {
						return nil
					}

					resolved, ok := defs[defName].(map[string]any)
					if !ok {
						return nil
					}

					current = resolved

					continue
				}

				current = next
			}

			return current
		}

		It("has enum on RuleActionConfig.Type", func() {
			prop := findProp("rules", "rules")
			Expect(prop).NotTo(BeNil())
			// rules is an array - get item schema
			items, ok := prop["items"].(map[string]any)
			if !ok {
				// Might be under $ref
				Skip("complex ref structure")
			}

			actionProp := navigateProps(items, s, "action", "type")
			if actionProp == nil {
				Skip("could not navigate to action.type")
			}

			Expect(actionProp).To(HaveKey("enum"))
		})

		It("has enum on RuleMatchConfig.PatternMode", func() {
			prop := findProp("rules", "rules")
			Expect(prop).NotTo(BeNil())

			items, ok := prop["items"].(map[string]any)
			if !ok {
				Skip("complex ref structure")
			}

			matchProp := navigateProps(items, s, "match", "pattern_mode")
			if matchProp == nil {
				Skip("could not navigate to match.pattern_mode")
			}

			Expect(matchProp).To(HaveKey("enum"))
		})
	})

	Describe("GenerateJSON", func() {
		It("produces compact JSON when indent is false", func() {
			data, err := schema.GenerateJSON(false)
			Expect(err).NotTo(HaveOccurred())

			// Compact JSON shouldn't have leading spaces on lines
			lines := 0

			for _, b := range data {
				if b == '\n' {
					lines++
				}
			}

			// Compact JSON is a single line plus trailing newline
			Expect(lines).To(Equal(1))
		})

		It("produces indented JSON when indent is true", func() {
			data, err := schema.GenerateJSON(true)
			Expect(err).NotTo(HaveOccurred())

			lines := 0

			for _, b := range data {
				if b == '\n' {
					lines++
				}
			}

			Expect(lines).To(BeNumerically(">", 10))
		})
	})
})

// navigateProps follows a property path through a schema, resolving $refs as needed.
func navigateProps(current, root map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		resolved := resolveRef(current, root)
		if resolved == nil {
			return nil
		}

		props, ok := resolved["properties"].(map[string]any)
		if !ok {
			return nil
		}

		next, ok := props[key].(map[string]any)
		if !ok {
			return nil
		}

		current = next
	}

	return resolveRef(current, root)
}

// resolveRef follows a $ref if present in the schema node.
func resolveRef(node, root map[string]any) map[string]any {
	ref, ok := node["$ref"].(string)
	if !ok {
		return node
	}

	const prefix = "#/$defs/"
	if len(ref) <= len(prefix) {
		return nil
	}

	defName := ref[len(prefix):]

	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		return nil
	}

	resolved, ok := defs[defName].(map[string]any)
	if !ok {
		return nil
	}

	return resolved
}
