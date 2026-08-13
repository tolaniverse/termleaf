package page

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/termleaf/internal/document"
	"github.com/tolaniverse/termleaf/internal/render"
)

// Slice selects rendered lines from one semantic block.
type Slice struct {
	Block      int
	StartLine  int
	EndLine    int
	Words      int
	BlockLines int
}

// Page contains only source references and statistics, not rendered content.
type Page struct {
	Slices []Slice
	Words  int
}

// Anchor identifies a reading location independently of terminal dimensions.
type Anchor struct {
	SourceOffset  int
	BlockFraction float64
	Valid         bool
}

// Span maps one semantic block into continuous-mode rendered lines.
type Span struct {
	Block     int
	StartLine int
	EndLine   int
	Lines     int
}

// Layout holds discovered pages and the cursor needed to continue discovery.
type Layout struct {
	Pages     []Page
	NextBlock int
	NextLine  int
	Complete  bool
}

// Discover extends an existing layout through targetPage. A negative target
// discovers the entire document. Existing pages are immutable and reused.
func Discover(source, referenceContext string, blocks []document.Block, width, height int, cache *render.Cache, mmdc *render.MMDCRenderer, existing Layout, targetPage int) (Layout, error) {
	height = max(1, height)
	layout := existing

	for !layout.Complete && (targetPage < 0 || len(layout.Pages) <= targetPage) {
		planned, nextBlock, nextLine, complete, err := buildPage(
			source, referenceContext, blocks, width, height, cache, mmdc, layout.NextBlock, layout.NextLine,
		)
		if err != nil {
			return Layout{}, err
		}
		layout.NextBlock = nextBlock
		layout.NextLine = nextLine
		layout.Complete = complete
		if len(planned.Slices) > 0 {
			layout.Pages = append(layout.Pages, planned)
		}
		if complete && len(layout.Pages) == 0 {
			layout.Pages = append(layout.Pages, Page{})
		}
	}
	return layout, nil
}

// Plan discovers all pages. It is retained for non-interactive callers and tests.
func Plan(source, referenceContext string, blocks []document.Block, width, height int, cache *render.Cache) (Layout, error) {
	return Discover(source, referenceContext, blocks, width, height, cache, nil, Layout{}, -1)
}

// DiscoverAnchor extends layout until it contains the semantic anchor.
func DiscoverAnchor(source, referenceContext string, blocks []document.Block, width, height int, cache *render.Cache, mmdc *render.MMDCRenderer, anchor Anchor) (Layout, int, error) {
	layout := Layout{}
	for !layout.Complete {
		previousPages := len(layout.Pages)
		var err error
		layout, err = Discover(source, referenceContext, blocks, width, height, cache, mmdc, layout, previousPages)
		if err != nil {
			return Layout{}, 0, err
		}
		if pageIndex, ok := pagesForAnchor(layout.Pages[previousPages:], previousPages, blocks, anchor); ok {
			return layout, pageIndex, nil
		}
	}
	return layout, max(0, len(layout.Pages)-1), nil
}

// AnchorAt returns the semantic location at the top of a page.
func (l Layout) AnchorAt(blocks []document.Block, pageIndex int) Anchor {
	if pageIndex < 0 || pageIndex >= len(l.Pages) || len(l.Pages[pageIndex].Slices) == 0 {
		return Anchor{}
	}
	slice := l.Pages[pageIndex].Slices[0]
	if slice.Block < 0 || slice.Block >= len(blocks) {
		return Anchor{}
	}
	fraction := 0.0
	if slice.BlockLines > 0 {
		fraction = float64(slice.StartLine) / float64(slice.BlockLines)
	}
	return Anchor{SourceOffset: blocks[slice.Block].Start, BlockFraction: fraction, Valid: true}
}

// PageForAnchor finds the discovered page containing an anchor.
func (l Layout) PageForAnchor(blocks []document.Block, anchor Anchor) (int, bool) {
	return pagesForAnchor(l.Pages, 0, blocks, anchor)
}

func pagesForAnchor(pages []Page, pageOffset int, blocks []document.Block, anchor Anchor) (int, bool) {
	if !anchor.Valid {
		return 0, false
	}
	blockIndex := blockForOffset(blocks, anchor.SourceOffset)
	for pageIndex, page := range pages {
		for _, slice := range page.Slices {
			if slice.Block != blockIndex {
				continue
			}
			targetLine := int(math.Round(max(0, min(1, anchor.BlockFraction)) * float64(slice.BlockLines)))
			if targetLine >= slice.StartLine && (targetLine < slice.EndLine || slice.EndLine == slice.BlockLines) {
				return pageOffset + pageIndex, true
			}
		}
	}
	return 0, false
}

func blockForOffset(blocks []document.Block, offset int) int {
	if len(blocks) == 0 {
		return 0
	}
	index := 0
	for candidate, block := range blocks {
		if block.Start > offset {
			break
		}
		index = candidate
	}
	return index
}

func buildPage(source, referenceContext string, blocks []document.Block, width, height int, cache *render.Cache, mmdc *render.MMDCRenderer, blockIndex, lineIndex int) (Page, int, int, bool, error) {
	planned := Page{}
	remaining := height

	for blockIndex < len(blocks) && remaining > 0 {
		if lineIndex == 0 && len(planned.Slices) > 0 {
			remaining-- // Layout.Render inserts one separator line between blocks.
			if remaining == 0 {
				break
			}
		}
		block := blocks[blockIndex]
		rendered, err := renderBlock(cache, mmdc, block, source[block.Start:block.End], referenceContext, width)
		if err != nil {
			return Page{}, blockIndex, lineIndex, false, fmt.Errorf("render block %d: %w", blockIndex, err)
		}
		lines := renderedLines(rendered)
		if lineIndex >= len(lines) {
			blockIndex++
			lineIndex = 0
			continue
		}

		take := min(remaining, len(lines)-lineIndex)
		words := visibleWords(lines[lineIndex : lineIndex+take])
		planned.Slices = append(planned.Slices, Slice{
			Block: blockIndex, StartLine: lineIndex, EndLine: lineIndex + take, Words: words, BlockLines: len(lines),
		})
		planned.Words += words
		remaining -= take
		lineIndex += take
		if lineIndex >= len(lines) {
			blockIndex++
			lineIndex = 0
		}
	}
	return planned, blockIndex, lineIndex, blockIndex >= len(blocks), nil
}

// RenderAll assembles all semantic blocks for continuous scroll mode.
func RenderAll(source, referenceContext string, blocks []document.Block, width int, cache *render.Cache, mmdc *render.MMDCRenderer) (string, error) {
	content, _, err := RenderAllWithSpans(source, referenceContext, blocks, width, cache, mmdc)
	return content, err
}

// RenderAllWithSpans renders continuous mode and maps blocks to rendered lines.
func RenderAllWithSpans(source, referenceContext string, blocks []document.Block, width int, cache *render.Cache, mmdc *render.MMDCRenderer) (string, []Span, error) {
	var output strings.Builder
	spans := make([]Span, 0, len(blocks))
	lineOffset := 0
	for blockIndex, block := range blocks {
		rendered, err := renderBlock(cache, mmdc, block, source[block.Start:block.End], referenceContext, width)
		if err != nil {
			return "", nil, fmt.Errorf("render block %d: %w", blockIndex, err)
		}
		if output.Len() > 0 {
			output.WriteByte('\n')
			lineOffset++
		}
		trimmed := strings.Trim(rendered, "\n")
		lines := renderedLines(trimmed)
		output.WriteString(trimmed)
		spans = append(spans, Span{Block: blockIndex, StartLine: lineOffset, EndLine: lineOffset + len(lines), Lines: len(lines)})
		lineOffset += len(lines)
	}
	return output.String(), spans, nil
}

// Render assembles one discovered page from bounded cached block renderings.
func (l Layout) Render(pageIndex int, source, referenceContext string, blocks []document.Block, width int, cache *render.Cache, mmdc *render.MMDCRenderer) (string, error) {
	if pageIndex < 0 || pageIndex >= len(l.Pages) {
		return "", fmt.Errorf("page %d out of range", pageIndex+1)
	}

	var output strings.Builder
	for _, slice := range l.Pages[pageIndex].Slices {
		if slice.Block < 0 || slice.Block >= len(blocks) {
			return "", fmt.Errorf("block %d out of range", slice.Block)
		}
		block := blocks[slice.Block]
		rendered, err := renderBlock(cache, mmdc, block, source[block.Start:block.End], referenceContext, width)
		if err != nil {
			return "", fmt.Errorf("render block %d: %w", slice.Block, err)
		}
		lines := renderedLines(rendered)
		if slice.StartLine > len(lines) || slice.EndLine > len(lines) {
			return "", fmt.Errorf("rendered block %d changed during layout", slice.Block)
		}
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		output.WriteString(strings.Join(lines[slice.StartLine:slice.EndLine], "\n"))
	}
	return output.String(), nil
}

func renderBlock(cache *render.Cache, mmdc *render.MMDCRenderer, block document.Block, source, referenceContext string, width int) (string, error) {
	if block.Image != nil {
		return cache.Image(block.Image.Destination, block.Image.Alt, width), nil
	}
	if block.Mermaid != nil {
		rendered, err := cache.Render(source, width)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(rendered, "\n") + "\n\n[ v ] View Mermaid Diagram", nil
	}
	if referenceContext != "" && !strings.Contains(source, referenceContext) {
		source += "\n\n" + referenceContext
	}
	return cache.Render(source, width)
}

// AnchorForLine maps a continuous-mode rendered line to a semantic anchor.
func AnchorForLine(blocks []document.Block, spans []Span, line int) Anchor {
	if len(spans) == 0 {
		return Anchor{}
	}
	selected := spans[0]
	for _, span := range spans {
		if line < span.StartLine {
			break
		}
		selected = span
		if line < span.EndLine {
			break
		}
	}
	if selected.Block < 0 || selected.Block >= len(blocks) {
		return Anchor{}
	}
	fraction := 0.0
	if selected.Lines > 0 {
		fraction = float64(max(0, min(selected.Lines, line-selected.StartLine))) / float64(selected.Lines)
	}
	return Anchor{SourceOffset: blocks[selected.Block].Start, BlockFraction: fraction, Valid: true}
}

// LineForAnchor maps a semantic anchor into continuous-mode rendered lines.
func LineForAnchor(blocks []document.Block, spans []Span, anchor Anchor) int {
	if !anchor.Valid || len(spans) == 0 {
		return 0
	}
	blockIndex := blockForOffset(blocks, anchor.SourceOffset)
	for _, span := range spans {
		if span.Block == blockIndex {
			return span.StartLine + int(math.Round(max(0, min(1, anchor.BlockFraction))*float64(span.Lines)))
		}
	}
	return 0
}

func renderedLines(rendered string) []string {
	trimmed := strings.Trim(rendered, "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func visibleWords(lines []string) int {
	return len(strings.Fields(ansi.Strip(strings.Join(lines, "\n"))))
}
