package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tolaniverse/termleaf/internal/document"
	"github.com/tolaniverse/termleaf/internal/page"
)

func TestPaginator(t *testing.T) {
	tests := []struct {
		name     string
		current  int
		total    int
		width    int
		complete bool
		want     string
	}{
		{name: "all pages", current: 1, total: 4, width: 80, complete: true, want: "◀ [1] 2 3 4 ▶"},
		{name: "middle", current: 13, total: 42, width: 80, complete: true, want: "◀ 1 … 12 [13] 14 … 42 ▶"},
		{name: "narrow", current: 13, total: 42, width: 30, complete: true, want: "◀ [13/42] ▶"},
		{name: "unknown total", current: 2, total: 3, width: 80, want: "◀ 1 [2] 3 … ? ▶"},
		{name: "unknown narrow", current: 2, total: 3, width: 30, want: "◀ [2/?] ▶"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := paginator(test.current, test.total, test.complete, test.width); got != test.want {
				t.Fatalf("paginator() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadingTime(t *testing.T) {
	tests := []struct {
		words int
		want  string
	}{
		{words: 0, want: "0 min"},
		{words: 100, want: "<1 min"},
		{words: 225, want: "~1 min"},
		{words: 226, want: "~2 min"},
	}
	for _, test := range tests {
		if got := readingTime(test.words); got != test.want {
			t.Errorf("readingTime(%d) = %q, want %q", test.words, got, test.want)
		}
	}
}

func TestRestoreProgressSurvivesRepagination(t *testing.T) {
	model := New(Config{PageMode: true})
	model.bodyHeight = 10
	model.layout.Pages = make([]page.Page, 10)
	model.layout.Complete = true
	model.restoreProgress(0.5)

	if model.page != 5 {
		t.Fatalf("restored page = %d, want 5", model.page)
	}
	if got := model.ProgressFraction(); got < 0.55 || got > 0.56 {
		t.Fatalf("ProgressFraction() = %f, want about 0.556", got)
	}
}

func TestSearchJumpsUsingSemanticAnchor(t *testing.T) {
	source := "# Start\n\nFirst section.\n\n## Target\n\nFind the needle here.\n"
	blocks := document.IndexBlocks(source)
	model := New(Config{Markdown: source, Blocks: blocks})
	updated, renderCmd := model.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	model = updated.(Model)
	updated, _ = model.Update(renderCmd())
	model = updated.(Model)
	model.searchResults = document.Search(source, "needle").Results
	updated, jumpCmd := model.jumpToSearchResult(0)
	model = updated.(Model)
	if jumpCmd == nil {
		t.Fatal("search result did not create a jump")
	}
	updated, _ = model.Update(jumpCmd())
	model = updated.(Model)
	anchor := model.restoredAnchor
	if !anchor.Valid || anchor.SourceOffset != blocks[3].Start {
		t.Fatalf("search anchor = %+v, want source %d", anchor, blocks[3].Start)
	}
}

func TestBookmarkToggleAndNavigation(t *testing.T) {
	source := "# One\n\nFirst.\n\n# Two\n\nSecond.\n"
	blocks := document.IndexBlocks(source)
	model := New(Config{Markdown: source, PageMode: true, Blocks: blocks})
	model.layout = page.Layout{Complete: true, Pages: []page.Page{
		{Slices: []page.Slice{{Block: 0, StartLine: 0, EndLine: 1, BlockLines: 1}}},
		{Slices: []page.Slice{{Block: 2, StartLine: 0, EndLine: 1, BlockLines: 1}}},
	}}
	model.page = 1
	model.toggleBookmark()
	if len(model.bookmarks) != 1 || model.bookmarks[0].Anchor.SourceOffset != blocks[2].Start {
		t.Fatalf("bookmark = %+v", model.bookmarks)
	}
	model.toggleBookmark()
	if len(model.bookmarks) != 0 {
		t.Fatalf("bookmark was not removed: %+v", model.bookmarks)
	}
}

func TestNextBookmarkIsRelativeToCurrentPosition(t *testing.T) {
	model := New(Config{Bookmarks: []Bookmark{
		{Anchor: page.Anchor{SourceOffset: 10, Valid: true}, Label: "first"},
		{Anchor: page.Anchor{SourceOffset: 30, Valid: true}, Label: "second"},
	}})
	model.blocks = []document.Block{{Start: 0, End: 20}, {Start: 20, End: 40}}
	model.spans = []page.Span{{Block: 0, StartLine: 0, EndLine: 10, Lines: 10}, {Block: 1, StartLine: 11, EndLine: 20, Lines: 9}}
	model.viewport.SetHeight(5)
	model.viewport.SetContent(strings.Repeat("line\n", 30))
	model.viewport.SetYOffset(5)
	if got := model.nextBookmarkIndex(); got != 0 {
		t.Fatalf("next bookmark before first = %d, want 0", got)
	}
	model.viewport.SetYOffset(12)
	if got := model.nextBookmarkIndex(); got != 1 {
		t.Fatalf("next bookmark between entries = %d, want 1", got)
	}
}

func TestOpenVisibleDiagramAndPanWithoutRerender(t *testing.T) {
	source := "# Diagram\n\n```mermaid\nsequenceDiagram\nAlice->>Bob: Hello\n```\n"
	blocks := document.IndexBlocks(source)
	model := New(Config{Markdown: source, Blocks: blocks})
	model.width, model.height, model.bodyHeight, model.contentWidth = 30, 12, 10, 28
	model.spans = []page.Span{{Block: 0, StartLine: 0, EndLine: 1, Lines: 1}, {Block: 1, StartLine: 2, EndLine: 8, Lines: 6}}
	model.viewport.SetWidth(28)
	model.viewport.SetHeight(10)
	model.viewport.SetContent(strings.Repeat("line\n", 12))

	updated, cmd := model.openVisibleDiagram(1)
	model = updated.(Model)
	if cmd == nil || !model.diagramMode || !model.diagramLoading {
		t.Fatalf("diagram did not open: %+v", model)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.diagramLoading || !strings.Contains(model.diagramCanvas, "Alice") {
		t.Fatalf("diagram canvas = %q", model.diagramCanvas)
	}
	canvas := model.diagramCanvas
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model = updated.(Model)
	if model.diagramCanvas != canvas {
		t.Fatal("panning rerendered the diagram canvas")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.(Model).diagramMode {
		t.Fatal("escape did not close diagram mode")
	}
}

func TestRapidResizePreservesPendingSemanticAnchor(t *testing.T) {
	source := "# Start\n\n" + strings.Repeat("Pending anchor remains stable through resize storms. ", 40)
	blocks := document.IndexBlocks(source)
	anchor := page.Anchor{SourceOffset: blocks[1].Start, BlockFraction: 0.5, Valid: true}
	model := New(Config{Markdown: source, PageMode: true, Blocks: blocks, RestoredAnchor: anchor})
	updated, firstCmd := model.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	model = updated.(Model)
	updated, secondCmd := model.Update(tea.WindowSizeMsg{Width: 28, Height: 8})
	model = updated.(Model)
	if firstCmd == nil || secondCmd == nil {
		t.Fatal("resize did not start anchor discovery")
	}
	if !model.restoredAnchor.Valid || model.restoredAnchor.SourceOffset != anchor.SourceOffset {
		t.Fatalf("pending anchor = %+v, want %+v", model.restoredAnchor, anchor)
	}
	updated, renderCmd := model.Update(secondCmd())
	model = updated.(Model)
	if renderCmd == nil {
		t.Fatal("latest resize did not render restored page")
	}
	got := model.layout.AnchorAt(blocks, model.page)
	if !got.Valid || got.SourceOffset != anchor.SourceOffset {
		t.Fatalf("restored anchor = %+v, want source offset %d", got, anchor.SourceOffset)
	}
}

func TestPageModeRestoresSemanticAnchorThroughResize(t *testing.T) {
	source := "# Start\n\n" + strings.Repeat("Semantic position survives terminal reflow. ", 40) + "\n\nEnd.\n"
	blocks := document.IndexBlocks(source)
	anchor := page.Anchor{SourceOffset: blocks[1].Start, BlockFraction: 0.5, Valid: true}
	model := New(Config{Markdown: source, PageMode: true, Blocks: blocks, RestoredAnchor: anchor})
	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	model = updated.(Model)
	updated, cmd = model.Update(cmd())
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("anchor discovery did not request page rendering")
	}
	got := model.layout.AnchorAt(blocks, model.page)
	if !got.Valid || got.SourceOffset != anchor.SourceOffset {
		t.Fatalf("restored anchor = %+v, want source offset %d", got, anchor.SourceOffset)
	}
}

func TestScrollModeRestoresSemanticAnchorThroughResize(t *testing.T) {
	source := "# Start\n\n" + strings.Repeat("Semantic scroll restoration text. ", 50)
	blocks := document.IndexBlocks(source)
	anchor := page.Anchor{SourceOffset: blocks[1].Start, BlockFraction: 0.5, Valid: true}
	model := New(Config{Markdown: source, Blocks: blocks, RestoredAnchor: anchor})
	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 28, Height: 10})
	model = updated.(Model)
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	got := model.SemanticAnchor()
	if !got.Valid || got.SourceOffset != anchor.SourceOffset {
		t.Fatalf("restored anchor = %+v, want source offset %d", got, anchor.SourceOffset)
	}
}

func TestPageModeUsesProgressWhenLineAnchorIsUnavailable(t *testing.T) {
	source := strings.Repeat("Paragraph text that creates pages.\n\n", 20)
	model := New(Config{
		Markdown:         source,
		PageMode:         true,
		RestoredProgress: 0.5,
		Blocks:           document.IndexBlocks(source),
	})
	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 24, Height: 6})
	model = updated.(Model)
	updated, cmd = model.Update(cmd())
	model = updated.(Model)
	if !model.layout.Complete {
		t.Fatal("progress fallback did not complete layout discovery")
	}
	if model.page == 0 || model.page >= model.pageCount()-1 {
		t.Fatalf("restored page = %d of %d, want a middle page", model.page+1, model.pageCount())
	}
	if cmd == nil {
		t.Fatal("progress restoration did not render the restored page")
	}
}

func TestIncompleteProgressAccountsForPositionWithinLongBlock(t *testing.T) {
	model := New(Config{PageMode: true})
	model.blocks = []document.Block{{Start: 0, End: 100}}
	model.layout.Pages = []page.Page{{Slices: []page.Slice{{Block: 0, StartLine: 0, EndLine: 10, BlockLines: 100}}}}
	if got := model.ProgressFraction(); got < 0.099 || got > 0.101 {
		t.Fatalf("ProgressFraction() = %f, want 0.1", got)
	}
}

func TestPageModeDiscoversIncrementallyAndPrefetchesAfterNavigation(t *testing.T) {
	source := "# Test\n\n" + strings.Repeat("A paragraph that wraps into terminal lines.\n\n", 30)
	model := New(Config{
		Name:     "test.md",
		Markdown: source,
		PageMode: true,
		Blocks:   document.IndexBlocks(source),
	})

	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("resize did not start discovery")
	}
	updated, cmd = model.Update(cmd())
	model = updated.(Model)
	if len(model.layout.Pages) != 2 || model.layout.Complete {
		t.Fatalf("initial discovery pages=%d complete=%v, want 2 incomplete pages", len(model.layout.Pages), model.layout.Complete)
	}
	updated, cmd = model.Update(cmd())
	model = updated.(Model)
	if cmd != nil {
		t.Fatal("initial page unexpectedly prefetched beyond its existing neighbor")
	}

	updated, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model = updated.(Model)
	if cmd == nil || model.page != 1 {
		t.Fatalf("next-page navigation page=%d command=%v", model.page, cmd != nil)
	}
	updated, cmd = model.Update(cmd())
	model = updated.(Model)
	if cmd == nil || !model.planning {
		t.Fatal("displaying page 2 did not prefetch page 3")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if len(model.layout.Pages) != 3 {
		t.Fatalf("prefetched pages=%d, want 3", len(model.layout.Pages))
	}

	updated, cmd = model.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	model = updated.(Model)
	if cmd == nil || !model.planning {
		t.Fatal("G did not start final-page discovery")
	}
	updated, cmd = model.Update(cmd())
	model = updated.(Model)
	if !model.layout.Complete || model.page != model.pageCount()-1 || cmd == nil {
		t.Fatalf("G result complete=%v page=%d/%d render=%v", model.layout.Complete, model.page+1, model.pageCount(), cmd != nil)
	}
}

func TestNavigationIsDisabledWhileResizeLayoutIsPending(t *testing.T) {
	model := New(Config{PageMode: true, Markdown: "# Test"})
	model.width = 80
	model.height = 24
	model.layout.Pages = make([]page.Page, 3)

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	resizing := updated.(Model)
	generation := resizing.generation
	updated, cmd := resizing.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	afterKey := updated.(Model)
	if cmd != nil {
		t.Fatal("navigation returned a command while layout was pending")
	}
	if afterKey.generation != generation || afterKey.page != resizing.page {
		t.Fatalf("pending navigation changed state: generation %d -> %d, page %d -> %d", generation, afterKey.generation, resizing.page, afterKey.page)
	}
}

func TestPagePositionUsesRenderedLineOffset(t *testing.T) {
	model := New(Config{PageMode: true, RestoredLine: 25})
	model.bodyHeight = 10
	model.layout.Pages = make([]page.Page, 5)
	model.layout.Complete = true
	model.restoreLine(25)

	if model.page != 2 {
		t.Fatalf("restored page = %d, want 2", model.page)
	}
	if got := model.LineOffset(); got != 20 {
		t.Fatalf("LineOffset() = %d, want 20", got)
	}
}
