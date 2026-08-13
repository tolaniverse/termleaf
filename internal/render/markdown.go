package render

import (
	"errors"
	"fmt"

	"charm.land/glamour/v2"
)

// Markdown renders Markdown as terminal-styled text at the requested cell width.
func Markdown(source string, width int) (output string, err error) {
	if width < 1 {
		width = 1
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(true),
	)
	if err != nil {
		return "", fmt.Errorf("create markdown renderer: %w", err)
	}
	defer func() {
		if closeErr := renderer.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close markdown renderer: %w", closeErr))
		}
	}()

	output, err = renderer.Render(source)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return output, nil
}
