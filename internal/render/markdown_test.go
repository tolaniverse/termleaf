package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownRendersAtNarrowWidths(t *testing.T) {
	const width = 8
	output, err := Markdown("# Narrow\n\nA small terminal still renders.", width)
	if err != nil {
		t.Fatalf("render narrow Markdown: %v", err)
	}
	if output == "" {
		t.Fatal("rendered output is empty")
	}
	for _, line := range strings.Split(output, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("rendered line width = %d, want <= %d: %q", got, width, line)
		}
	}
}
