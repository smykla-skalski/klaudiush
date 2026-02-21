// Package schema generates JSON Schema from the klaudiush config types.
package schema

import (
	"encoding/json"

	"github.com/cockroachdb/errors"
	"github.com/invopop/jsonschema"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	schemaURI = "https://json-schema.org/draft/2020-12/schema"
	title     = "klaudiush configuration"
)

// Generate produces a JSON Schema from the config.Config struct.
func Generate() *jsonschema.Schema {
	r := &jsonschema.Reflector{
		ExpandedStruct: true,
	}

	s := r.Reflect(&config.Config{})
	s.Version = schemaURI
	s.Title = title

	return s
}

// GenerateJSON produces a JSON Schema as bytes.
// When indent is true, the output is pretty-printed.
func GenerateJSON(indent bool) ([]byte, error) {
	s := Generate()

	var (
		data []byte
		err  error
	)

	if indent {
		data, err = json.MarshalIndent(s, "", "  ")
	} else {
		data, err = json.Marshal(s)
	}

	if err != nil {
		return nil, errors.Wrap(err, "marshaling schema to JSON")
	}

	// Append trailing newline for file output.
	return append(data, '\n'), nil
}
