// Package schema generates JSON Schema from the klaudiush config types.
package schema

import (
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/invopop/jsonschema"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	schemaURI    = "https://json-schema.org/draft/2020-12/schema"
	titleFmt     = "klaudiush configuration v%d"
	schemaURLFmt = "https://klaudiu.sh/schema/v%d/config.json"
)

// Generate produces a JSON Schema from the config.Config struct.
func Generate() *jsonschema.Schema {
	r := &jsonschema.Reflector{
		ExpandedStruct: true,
	}

	s := r.Reflect(&config.Config{})
	s.Version = schemaURI
	s.Title = fmt.Sprintf(titleFmt, config.CurrentConfigVersion)

	return s
}

// Filename returns the versioned schema filename, e.g. "config.v1.schema.json".
func Filename() string {
	return fmt.Sprintf("config.v%d.schema.json", config.CurrentConfigVersion)
}

// SchemaURL returns the public URL for the current schema version.
func SchemaURL() string {
	return fmt.Sprintf(schemaURLFmt, config.CurrentConfigVersion)
}

// SchemaDirective returns the Taplo schema directive line for TOML files.
func SchemaDirective() string {
	return "#:schema " + SchemaURL()
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
