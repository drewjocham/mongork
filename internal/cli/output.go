package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/drewjocham/mongork/internal/jsonutil"
)

func renderWithOutput(
	w io.Writer,
	format string,
	unsupportedErr error,
	renderTable func(io.Writer) error,
	renderJSON func(io.Writer) error,
) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return renderJSON(w)
	case "table", "":
		return renderTable(w)
	default:
		return fmt.Errorf("%w: %s", unsupportedErr, format)
	}
}

func encodePrettyJSON(w io.Writer, value any) error {
	encoder := jsonutil.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
