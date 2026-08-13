package document

import (
	"strings"
	"testing"
)

func TestSearchIsCaseInsensitiveAndSourceBacked(t *testing.T) {
	source := "# Alpha\n\nFirst target.\n\n## Beta\n\nSecond TARGET.\n"
	results := Search(source, "target").Results
	if len(results) != 2 {
		t.Fatalf("result count = %d, want 2", len(results))
	}
	if results[0].SourceOffset >= results[1].SourceOffset {
		t.Fatalf("results out of order: %+v", results)
	}
	blocks := IndexBlocks(source)
	offset, fraction, ok := SearchAnchor(blocks, results[1])
	if !ok || offset != blocks[3].Start || fraction <= 0 || fraction > 1 {
		t.Fatalf("search anchor = offset %d fraction %.2f ok=%v", offset, fraction, ok)
	}
}

func TestSearchPreservesOffsetsAcrossUnicodeCaseFolding(t *testing.T) {
	source := "K prefix before TARGET and σ before Σ"
	results := Search(source, "target").Results
	if len(results) != 1 || source[results[0].SourceOffset:results[0].SourceOffset+6] != "TARGET" {
		t.Fatalf("Unicode-offset search = %+v", results)
	}
	sigma := Search(source, "ς").Results
	if len(sigma) != 2 {
		t.Fatalf("sigma case-fold results = %+v", sigma)
	}
}

func TestSearchReportsTruncation(t *testing.T) {
	summary := Search(strings.Repeat("x ", maxSearchResults+5), "x")
	if len(summary.Results) != maxSearchResults || !summary.Truncated {
		t.Fatalf("summary results=%d truncated=%v", len(summary.Results), summary.Truncated)
	}
}

func TestSearchEmptyQueryReturnsNoResults(t *testing.T) {
	if results := Search("text", "  ").Results; results != nil {
		t.Fatalf("empty search = %+v", results)
	}
}
