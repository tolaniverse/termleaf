package page

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/termleaf/internal/document"
	"github.com/tolaniverse/termleaf/internal/render"
)

func TestRenderAllIncludesMermaidDiagram(t *testing.T) {
	source := "# Flow\n\n```mermaid\ngraph LR\nA[Read] --> B[Done]\n```\n\nAfter.\n"
	blocks := document.IndexBlocks(source)
	output, err := RenderAll(source, "", blocks, 80, render.NewCache(1<<20), nil)
	if err != nil {
		t.Fatalf("render all: %v", err)
	}
	plain := ansi.Strip(output)
	if !strings.Contains(plain, "Read") || !strings.Contains(plain, "Done") || strings.Contains(plain, "```mermaid") {
		t.Fatalf("rendered document = %q", plain)
	}
}

func TestRenderAllIncludesStandaloneImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	fixture := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			fixture.Set(x, y, color.RGBA{R: 220, G: 80, B: 40, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create image: %v", err)
	}
	if err := png.Encode(file, fixture); err != nil {
		_ = file.Close()
		t.Fatalf("encode image: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close image: %v", err)
	}

	source := "# Before\n\n![sample](" + path + ")\n\nAfter.\n"
	blocks := document.IndexBlocks(source)
	output, err := RenderAll(source, "", blocks, 20, render.NewCache(1<<20), nil)
	if err != nil {
		t.Fatalf("render all: %v", err)
	}
	plain := ansi.Strip(output)
	if !strings.Contains(plain, "Before") || !strings.Contains(plain, "After") || strings.Contains(plain, "[image:") {
		t.Fatalf("rendered document = %q", plain)
	}
}

func TestDiscoverStopsAfterRequestedNeighbor(t *testing.T) {
	source := strings.Repeat("Paragraph words that wrap across the terminal.\n\n", 30)
	blocks := document.IndexBlocks(source)
	cache := render.NewCache(1 << 20)
	layout, err := Discover(source, "", blocks, 24, 4, cache, nil, Layout{}, 1)
	if err != nil {
		t.Fatalf("discover pages: %v", err)
	}
	if len(layout.Pages) != 2 {
		t.Fatalf("discovered page count = %d, want 2", len(layout.Pages))
	}
	if layout.Complete {
		t.Fatal("layout completed despite an early discovery target")
	}

	extended, err := Discover(source, "", blocks, 24, 4, cache, nil, layout, 2)
	if err != nil {
		t.Fatalf("extend discovery: %v", err)
	}
	if len(extended.Pages) != 3 {
		t.Fatalf("extended page count = %d, want 3", len(extended.Pages))
	}
	if extended.Pages[0].Slices[0] != layout.Pages[0].Slices[0] {
		t.Fatal("extending discovery changed an existing page")
	}
}

func TestIncrementalDiscoveryMatchesFullLayoutWithoutGaps(t *testing.T) {
	source := strings.Repeat("A single long semantic block with wrapping words. ", 80)
	blocks := document.IndexBlocks(source)
	cache := render.NewCache(1 << 20)
	full, err := Plan(source, "", blocks, 20, 4, cache)
	if err != nil {
		t.Fatalf("plan full layout: %v", err)
	}

	incremental := Layout{}
	for target := 0; !incremental.Complete; target++ {
		incremental, err = Discover(source, "", blocks, 20, 4, cache, nil, incremental, target)
		if err != nil {
			t.Fatalf("discover target %d: %v", target, err)
		}
	}
	if !reflect.DeepEqual(incremental.Pages, full.Pages) {
		t.Fatal("incremental page slices differ from full planning")
	}

	previousBlock, previousEnd := 0, 0
	for pageIndex, planned := range incremental.Pages {
		for _, slice := range planned.Slices {
			if slice.Block == previousBlock && slice.StartLine != previousEnd {
				t.Fatalf("page %d block %d starts at %d after %d", pageIndex+1, slice.Block, slice.StartLine, previousEnd)
			}
			if slice.Block > previousBlock && slice.StartLine != 0 {
				t.Fatalf("page %d new block %d starts at line %d", pageIndex+1, slice.Block, slice.StartLine)
			}
			previousBlock, previousEnd = slice.Block, slice.EndLine
		}
	}
	if previousBlock != incremental.NextBlock-1 || incremental.NextLine != 0 {
		t.Fatalf("final cursor block=%d line=%d after slice block=%d end=%d", incremental.NextBlock, incremental.NextLine, previousBlock, previousEnd)
	}
}

func TestPlanCoversEveryRenderedLine(t *testing.T) {
	source := "# Heading\n\n" + strings.Repeat("A paragraph with words. ", 20) + "\n\n```go\nline one\nline two\nline three\n```\n"
	blocks := document.IndexBlocks(source)
	cache := render.NewCache(1 << 20)
	layout, err := Plan(source, document.ReferenceContext(source), blocks, 30, 5, cache)
	if err != nil {
		t.Fatalf("plan pages: %v", err)
	}
	if len(layout.Pages) < 2 {
		t.Fatalf("page count = %d, want multiple pages", len(layout.Pages))
	}

	for pageIndex, planned := range layout.Pages {
		content, err := layout.Render(pageIndex, source, document.ReferenceContext(source), blocks, 30, cache, nil)
		if err != nil {
			t.Fatalf("render page %d: %v", pageIndex+1, err)
		}
		if got := len(strings.Split(content, "\n")); got > 5 {
			t.Fatalf("page %d has %d lines, height is 5", pageIndex+1, got)
		}
		if len(planned.Slices) == 0 {
			t.Fatalf("page %d has no slices", pageIndex+1)
		}
	}
}

func TestRenderResolvesDocumentReferenceLinks(t *testing.T) {
	source := "[Read the guide][guide]\n\n## Interlude\n\n[guide]: https://example.com/guide\n\n## End\n"
	blocks := document.IndexBlocks(source)
	context := document.ReferenceContext(source)
	layout, err := Plan(source, context, blocks, 80, 40, render.NewCache(1<<20))
	if err != nil {
		t.Fatalf("plan pages: %v", err)
	}
	content, err := layout.Render(0, source, context, blocks, 80, render.NewCache(1<<20), nil)
	if err != nil {
		t.Fatalf("render page: %v", err)
	}
	if !strings.Contains(ansi.Strip(content), "https://example.com/guide") {
		t.Fatalf("reference URL missing from rendered page: %q", ansi.Strip(content))
	}
}

func TestPlanWordStatsSumToBlockWords(t *testing.T) {
	source := strings.Repeat("word ", 50)
	blocks := document.IndexBlocks(source)
	layout, err := Plan(source, document.ReferenceContext(source), blocks, 12, 3, render.NewCache(1<<20))
	if err != nil {
		t.Fatalf("plan pages: %v", err)
	}
	total := 0
	for _, planned := range layout.Pages {
		total += planned.Words
	}
	if total != blocks[0].Words {
		t.Fatalf("page words total = %d, block words = %d", total, blocks[0].Words)
	}
}
