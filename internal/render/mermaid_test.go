package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMermaidRendersSequenceDiagram(t *testing.T) {
	source := "sequenceDiagram\nAlice->>Bob: Hello\nBob-->>Alice: Hi"
	output := NewCache(1<<20).Mermaid(source, 80, nil)
	plain := ansi.Strip(output)
	if strings.Contains(plain, "preview unavailable") {
		t.Fatalf("sequence diagram fell back: %q", plain)
	}
	for _, expected := range []string{"Alice", "Bob", "Hello", "Hi"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("rendered diagram missing %q: %q", expected, plain)
		}
	}
}

func TestMermaidRendersFlowchartWithoutMMDC(t *testing.T) {
	output := NewCache(1<<20).Mermaid("flowchart LR\nA[Start] --> B{Ready?}\nB -->|Yes| C[Finish]", 80, nil)
	plain := ansi.Strip(output)
	if strings.Contains(plain, "Mermaid preview unavailable") || strings.Contains(plain, "flowchart LR") {
		t.Fatalf("flowchart fell back = %q", plain)
	}
	for _, expected := range []string{"Start", "Ready?", "Yes", "Finish", "►"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("flowchart missing %q: %q", expected, plain)
		}
	}
}

func TestMermaidRendersReverseFlowchartDirections(t *testing.T) {
	rightToLeft := ansi.Strip(NewCache(1<<20).Mermaid("graph RL\nA[Start] --> B[Finish]", 40, nil))
	if !strings.Contains(rightToLeft, "◄") || strings.Index(rightToLeft, "Finish") > strings.Index(rightToLeft, "Start") {
		t.Fatalf("RL flowchart = %q", rightToLeft)
	}
	bottomToTop := ansi.Strip(NewCache(1<<20).Mermaid("graph BT\nA[Start] --> B[Finish]", 40, nil))
	if !strings.Contains(bottomToTop, "▲") || strings.Index(bottomToTop, "Finish") > strings.Index(bottomToTop, "Start") {
		t.Fatalf("BT flowchart = %q", bottomToTop)
	}
}

func TestMermaidRendersSingleLineFlowchart(t *testing.T) {
	output := ansi.Strip(NewCache(1<<20).Mermaid("flowchart LR; A[Start] --> B[Finish]", 50, nil))
	if !strings.Contains(output, "Start") || !strings.Contains(output, "Finish") || strings.Contains(output, "preview unavailable") {
		t.Fatalf("single-line flowchart = %q", output)
	}
}

func TestMermaidCountsSemicolonStatements(t *testing.T) {
	source := "flowchart LR;" + strings.Repeat("A-->B;", maxMermaidStatements)
	output := NewCache(1<<20).Mermaid(source, 80, nil)
	if !strings.Contains(output, "statements") {
		t.Fatalf("semicolon statement limit was bypassed: %q", output)
	}
}

func TestMermaidRendersVerticalFlowchartWithinWidth(t *testing.T) {
	output := NewCache(1<<20).Mermaid("graph TD\nA[Start] --> B[Finish]", 24, nil)
	plain := ansi.Strip(output)
	if !strings.Contains(plain, "Start") || !strings.Contains(plain, "Finish") || !strings.Contains(plain, "▼") {
		t.Fatalf("vertical flowchart = %q", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if width := ansi.StringWidth(line); width > 24 {
			t.Fatalf("flowchart line width = %d: %q", width, line)
		}
	}
}

func TestMermaidFallsBackWhenDiagramIsTooWide(t *testing.T) {
	source := "sequenceDiagram\nAlice->>Bob: A message wider than a tiny terminal"
	output := NewCache(1<<20).Mermaid(source, 12, nil)
	if !strings.Contains(output, "Mermaid pr") || !strings.Contains(output, "sequenceDia") {
		t.Fatalf("fallback = %q", output)
	}
	for _, line := range strings.Split(output, "\n") {
		if width := ansi.StringWidth(line); width > 12 {
			t.Fatalf("fallback line width = %d, want <= 12: %q", width, line)
		}
	}
}

func TestMermaidSanitizesSuccessfulOutput(t *testing.T) {
	output := NewCache(1<<20).Mermaid("graph LR\nA[unsafe\x1b]52;c;payload\a label] --> B", 120, nil)
	if strings.ContainsAny(output, "\x1b\a") {
		t.Fatalf("unsafe rendered output = %q", output)
	}
}

func TestMermaidCapsFallbackLines(t *testing.T) {
	source := "sequenceDiagram\n" + strings.Repeat("Alice->>Bob: message\n", maxMermaidStatements+20)
	output := NewCache(1<<20).Mermaid(source, 80, nil)
	if lineCount(output) > maxMermaidFallbackLines+2 || !strings.Contains(output, "truncated") {
		t.Fatalf("uncapped fallback has %d lines", lineCount(output))
	}
}

func TestMermaidSanitizesFallbackSource(t *testing.T) {
	output := NewCache(1024).Mermaid("unsupported\n\x1b]52;c;payload\a", 80, nil)
	if strings.ContainsAny(output, "\x1b\a") {
		t.Fatalf("unsafe fallback = %q", output)
	}
}
