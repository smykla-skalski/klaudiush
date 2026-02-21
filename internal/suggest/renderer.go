package suggest

import (
	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/templates"
)

// Render executes the KLAUDIUSH.md template with the given data.
func Render(data *SuggestData) (string, error) {
	result, err := templates.Execute(klaudiushMDTemplate, data)
	if err != nil {
		return "", errors.Wrap(err, "rendering KLAUDIUSH.md template")
	}

	return result, nil
}
