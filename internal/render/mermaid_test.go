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

func TestMermaidFlowchartFallsBackWithoutMMDC(t *testing.T) {
	output := NewCache(1<<20).Mermaid("graph LR\nA[Start] --> B[Finish]", 80, nil)
	plain := ansi.Strip(output)
	if !strings.Contains(plain, "Mermaid preview unavailable") || !strings.Contains(plain, "embedded renderer supports") {
		t.Fatalf("flowchart fallback = %q", plain)
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
