package page

import (
	"strings"
	"testing"

	"github.com/tolaniverse/termleaf/internal/document"
	"github.com/tolaniverse/termleaf/internal/render"
)

func TestPageAnchorSurvivesReflow(t *testing.T) {
	source := "# Start\n\n" + strings.Repeat("A long paragraph with many words for wrapping. ", 30) + "\n\n## Target\n\nTarget content.\n"
	blocks := document.IndexBlocks(source)
	cache := render.NewCache(1 << 20)
	wide, err := Plan(source, "", blocks, 70, 10, cache)
	if err != nil {
		t.Fatalf("plan wide layout: %v", err)
	}
	pageIndex := min(1, len(wide.Pages)-1)
	anchor := wide.AnchorAt(blocks, pageIndex)
	if !anchor.Valid {
		t.Fatal("wide page did not produce an anchor")
	}

	narrow, restoredPage, err := DiscoverAnchor(source, "", blocks, 28, 6, cache, nil, anchor)
	if err != nil {
		t.Fatalf("restore narrow layout: %v", err)
	}
	got := narrow.AnchorAt(blocks, restoredPage)
	if !got.Valid || got.SourceOffset != anchor.SourceOffset {
		t.Fatalf("restored anchor = %+v, want source offset %d", got, anchor.SourceOffset)
	}
}

func TestContinuousAnchorSurvivesReflow(t *testing.T) {
	source := "# Heading\n\n" + strings.Repeat("Words wrap differently as width changes. ", 40) + "\n\nEnd.\n"
	blocks := document.IndexBlocks(source)
	cache := render.NewCache(1 << 20)
	_, wideSpans, err := RenderAllWithSpans(source, "", blocks, 70, cache, nil)
	if err != nil {
		t.Fatalf("render wide: %v", err)
	}
	wideLine := wideSpans[1].StartLine + max(1, wideSpans[1].Lines/2)
	anchor := AnchorForLine(blocks, wideSpans, wideLine)

	_, narrowSpans, err := RenderAllWithSpans(source, "", blocks, 24, cache, nil)
	if err != nil {
		t.Fatalf("render narrow: %v", err)
	}
	narrowLine := LineForAnchor(blocks, narrowSpans, anchor)
	restored := AnchorForLine(blocks, narrowSpans, narrowLine)
	if restored.SourceOffset != anchor.SourceOffset {
		t.Fatalf("restored source offset = %d, want %d", restored.SourceOffset, anchor.SourceOffset)
	}
	if difference := restored.BlockFraction - anchor.BlockFraction; difference < -0.1 || difference > 0.1 {
		t.Fatalf("restored fraction = %.3f, want near %.3f", restored.BlockFraction, anchor.BlockFraction)
	}
}
