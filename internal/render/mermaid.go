package render

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/er"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/sequence"
	"github.com/charmbracelet/x/ansi"
)

const (
	maxMermaidBytes         = 64 << 10
	maxMermaidLines         = 500
	maxMermaidStatements    = 300
	maxMermaidFallbackLines = 80
	maxMermaidOutputLines   = 500
)

// Mermaid renders bounded Mermaid input as terminal-native box drawing.
func (c *Cache) Mermaid(source string, width int, mmdc *MMDCRenderer) (output string) {
	digest := sha256.Sum256([]byte(source))
	backend := "embedded"
	if mmdc != nil {
		backend = mmdc.identity()
	}
	key := fmt.Sprintf("mermaid:%s:%d:%x", backend, width, digest)
	if value, ok := c.get(key); ok {
		return value
	}

	storeFallback := func(cause error) string {
		fallback := mermaidFallback(source, cause, width)
		// Subprocess failures can be transient; only embedded-only fallbacks cache.
		if mmdc == nil {
			c.put(key, fallback)
		}
		return fallback
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			output = storeFallback(fmt.Errorf("renderer failed safely"))
		}
	}()

	if err := validateMermaidInput(source); err != nil {
		return storeFallback(err)
	}
	config := diagram.DefaultConfig()
	config.BoxBorderPadding = 1
	config.PaddingBetweenX = 3
	config.PaddingBetweenY = 2
	config.SequenceParticipantSpacing = 3

	output, err := renderEmbeddedMermaid(source, config)
	if err != nil && mmdc != nil && c.MMDCEnabled() && c.protocol != ProtocolOff {
		output, err = mmdc.Render(context.Background(), source, width, c)
	}
	if err != nil {
		return storeFallback(err)
	}
	output = sanitizeTerminalDrawing(strings.TrimSpace(output))
	if lineCount(output) > maxMermaidOutputLines {
		return storeFallback(fmt.Errorf("diagram exceeds %d output lines", maxMermaidOutputLines))
	}
	if widestLine(output) > max(1, width) {
		return storeFallback(fmt.Errorf("diagram is wider than the reading column"))
	}
	c.put(key, output)
	return output
}

func renderEmbeddedMermaid(source string, config *diagram.Config) (string, error) {
	trimmed := strings.TrimSpace(source)
	switch {
	case sequence.IsSequenceDiagram(trimmed):
		parsed, err := sequence.Parse(trimmed)
		if err != nil {
			return "", fmt.Errorf("parse sequence diagram: %w", err)
		}
		output, err := sequence.Render(parsed, config)
		if err != nil {
			return "", fmt.Errorf("render sequence diagram: %w", err)
		}
		return output, nil
	case er.IsErDiagram(trimmed):
		parsed, err := er.Parse(trimmed)
		if err != nil {
			return "", fmt.Errorf("parse ER diagram: %w", err)
		}
		return er.Render(parsed, config.UseAscii), nil
	default:
		return "", fmt.Errorf("embedded renderer supports sequence and ER diagrams; install mmdc for this diagram type")
	}
}

func validateMermaidInput(source string) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("diagram is empty")
	}
	if len(source) > maxMermaidBytes {
		return fmt.Errorf("diagram exceeds %d KiB input limit", maxMermaidBytes>>10)
	}
	lines := strings.Split(source, "\n")
	if len(lines) > maxMermaidLines {
		return fmt.Errorf("diagram exceeds %d input lines", maxMermaidLines)
	}
	statements := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "%%") {
			statements++
		}
	}
	if statements > maxMermaidStatements {
		return fmt.Errorf("diagram exceeds %d statements", maxMermaidStatements)
	}
	return nil
}

func mermaidFallback(source string, cause error, width int) string {
	header := ansi.Truncate("[Mermaid preview unavailable: "+sanitizeTerminalText(cause.Error())+"]", max(1, width), "…")
	lines := strings.Split(source, "\n")
	truncated := false
	if len(lines) > maxMermaidFallbackLines {
		lines = lines[:maxMermaidFallbackLines]
		truncated = true
	}
	for index := range lines {
		lines[index] = ansi.Truncate(sanitizeTerminalText(lines[index]), max(1, width), "…")
	}
	if truncated {
		lines = append(lines, ansi.Truncate("[… Mermaid source truncated …]", max(1, width), "…"))
	}
	return header + "\n" + strings.Join(lines, "\n")
}

func sanitizeTerminalDrawing(value string) string {
	lines := strings.Split(value, "\n")
	for index := range lines {
		lines[index] = sanitizeTerminalText(lines[index])
	}
	return strings.Join(lines, "\n")
}

func widestLine(value string) int {
	widest := 0
	for _, line := range strings.Split(value, "\n") {
		widest = max(widest, ansi.StringWidth(line))
	}
	return widest
}

func lineCount(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}
