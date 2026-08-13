package document

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxSearchResults = 1000

// SearchResult is a source-backed, width-independent match.
type SearchResult struct {
	SourceOffset int
	Preview      string
}

// SearchSummary exposes bounded results and whether additional matches exist.
type SearchSummary struct {
	Results   []SearchResult
	Truncated bool
}

// Search finds Unicode case-insensitive literal matches while preserving exact
// original UTF-8 byte offsets.
func Search(source, query string) SearchSummary {
	queryRunes := []rune(strings.TrimSpace(query))
	if len(queryRunes) == 0 {
		return SearchSummary{}
	}

	type runeAt struct {
		value  rune
		offset int
	}
	sourceRunes := make([]runeAt, 0, utf8.RuneCountInString(source))
	for offset, value := range source {
		sourceRunes = append(sourceRunes, runeAt{value: value, offset: offset})
	}

	results := make([]SearchResult, 0)
	for index := 0; index+len(queryRunes) <= len(sourceRunes); index++ {
		matched := true
		for queryIndex, queryRune := range queryRunes {
			if !runesEqualFold(sourceRunes[index+queryIndex].value, queryRune) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		offset := sourceRunes[index].offset
		if len(results) == maxSearchResults {
			return SearchSummary{Results: results, Truncated: true}
		}
		results = append(results, SearchResult{SourceOffset: offset, Preview: searchPreview(source, offset, len([]byte(string(queryRunes))))})
		index += len(queryRunes) - 1
	}
	return SearchSummary{Results: results}
}

func runesEqualFold(left, right rune) bool {
	if left == right {
		return true
	}
	for folded := unicode.SimpleFold(left); folded != left; folded = unicode.SimpleFold(folded) {
		if folded == right {
			return true
		}
	}
	return false
}

func SearchAnchor(blocks []Block, result SearchResult) (blockOffset int, fraction float64, ok bool) {
	if len(blocks) == 0 {
		return 0, 0, false
	}
	index := sort.Search(len(blocks), func(index int) bool { return blocks[index].End > result.SourceOffset })
	if index >= len(blocks) {
		index = len(blocks) - 1
	}
	block := blocks[index]
	fraction = 0
	if block.End > block.Start {
		fraction = float64(max(block.Start, min(block.End, result.SourceOffset))-block.Start) / float64(block.End-block.Start)
	}
	return block.Start, max(0, min(1, fraction)), true
}

func searchPreview(source string, offset, matchBytes int) string {
	start := max(0, offset-40)
	end := min(len(source), offset+matchBytes+60)
	for start > 0 && !utf8.RuneStart(source[start]) {
		start--
	}
	for end < len(source) && !utf8.RuneStart(source[end]) {
		end++
	}
	preview := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, source[start:end])
	return strings.Join(strings.Fields(preview), " ")
}
