package document

import (
	"path/filepath"
	"testing"
)

func TestIndexBlocksDetectsStandaloneImage(t *testing.T) {
	source := "Before.\n\n![architecture](images/flow.png)\n\nAfter.\n"
	blocks := IndexBlocks(source)
	if len(blocks) != 3 || blocks[1].Image == nil {
		t.Fatalf("standalone image was not indexed: %+v", blocks)
	}
	if blocks[1].Image.Destination != "images/flow.png" || blocks[1].Image.Alt != "architecture" {
		t.Fatalf("image metadata = %+v", blocks[1].Image)
	}
}

func TestResolveImagesUsesDocumentDirectory(t *testing.T) {
	documentPath := filepath.Join(t.TempDir(), "docs", "guide.md")
	blocks := ResolveImages(IndexBlocks("![flow](../images/flow.png)\n"), documentPath)
	want := filepath.Join(filepath.Dir(documentPath), "..", "images", "flow.png")
	if blocks[0].Image == nil || blocks[0].Image.Destination != want {
		t.Fatalf("resolved destination = %+v, want %q", blocks[0].Image, want)
	}
}

func TestIndexBlocksDoesNotPromoteInlineImage(t *testing.T) {
	blocks := IndexBlocks("Text beside ![icon](icon.png) remains a paragraph.\n")
	if len(blocks) != 1 || blocks[0].Image != nil {
		t.Fatalf("inline image was promoted: %+v", blocks)
	}
}

func TestIndexBlocksExtractsMermaidSource(t *testing.T) {
	source := "Before.\n\n```mermaid\nsequenceDiagram\n  A->>B: hello\n```\n\nAfter.\n"
	blocks := IndexBlocks(source)
	if len(blocks) != 3 {
		t.Fatalf("block count = %d, want 3", len(blocks))
	}
	want := "sequenceDiagram\n  A->>B: hello"
	if blocks[1].Mermaid == nil || *blocks[1].Mermaid != want {
		t.Fatalf("Mermaid = %v, want %q", blocks[1].Mermaid, want)
	}
}

func TestIndexBlocksDetectsEmptyMermaidFence(t *testing.T) {
	blocks := IndexBlocks("```mermaid\n```\n")
	if len(blocks) != 1 || blocks[0].Mermaid == nil || *blocks[0].Mermaid != "" {
		t.Fatalf("empty Mermaid metadata = %+v", blocks)
	}
}

func TestIndexBlocksPreservesFencedDiagram(t *testing.T) {
	source := "# Title\n\nBefore.\n\n```mermaid\nsequenceDiagram\n  A->>B: hello\n\n  B-->>A: hi\n```\n\nAfter.\n"
	blocks := IndexBlocks(source)
	if len(blocks) != 4 {
		t.Fatalf("block count = %d, want 4", len(blocks))
	}
	got := source[blocks[2].Start:blocks[2].End]
	want := "```mermaid\nsequenceDiagram\n  A->>B: hello\n\n  B-->>A: hi\n```\n\n"
	if got != want {
		t.Fatalf("diagram block = %q, want %q", got, want)
	}
}

func TestIndexBlocksKeepsFenceWithoutInfoMarker(t *testing.T) {
	source := "Before.\n\n```\ncode\n```\n"
	blocks := IndexBlocks(source)
	if len(blocks) != 2 {
		t.Fatalf("block count = %d, want 2", len(blocks))
	}
	if got := source[blocks[1].Start:blocks[1].End]; got != "```\ncode\n```\n" {
		t.Fatalf("fence block = %q", got)
	}
}

func TestIndexBlocksKeepsLongFenceIntact(t *testing.T) {
	source := "````markdown\n```go\ninside\n```\n````\n\nAfter.\n"
	blocks := IndexBlocks(source)
	if len(blocks) != 2 {
		t.Fatalf("block count = %d, want 2", len(blocks))
	}
	if got := source[blocks[0].Start:blocks[0].End]; got != "````markdown\n```go\ninside\n```\n````\n\n" {
		t.Fatalf("outer fence was split: %q", got)
	}
}

func TestIndexBlocksKeepsLooseListTogether(t *testing.T) {
	source := "- first paragraph\n\n  continuation\n\n- second item\n\nAfter.\n"
	blocks := IndexBlocks(source)
	if len(blocks) != 2 {
		t.Fatalf("block count = %d, want list and paragraph", len(blocks))
	}
}

func TestReferenceContextCollectsDefinitions(t *testing.T) {
	source := "[docs][guide]\n\n[guide]: https://example.com\n"
	if got := ReferenceContext(source); got != "[guide]: https://example.com" {
		t.Fatalf("ReferenceContext() = %q", got)
	}
}

func TestIndexBlocksCoversUnclosedFence(t *testing.T) {
	source := "Intro\n\n```go\nfmt.Println(\"hello\")"
	blocks := IndexBlocks(source)
	if len(blocks) != 2 {
		t.Fatalf("block count = %d, want 2", len(blocks))
	}
	if got := source[blocks[1].Start:blocks[1].End]; got != "```go\nfmt.Println(\"hello\")" {
		t.Fatalf("unclosed fence block = %q", got)
	}
}
