package document

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Block identifies a parser-derived top-level Markdown source range without
// retaining the parsed AST.
type Block struct {
	Start   int
	End     int
	Words   int
	Image   *Image
	Mermaid *string
}

// Image describes a standalone Markdown image without decoding its payload.
type Image struct {
	Destination string
	Alt         string
}

// IndexBlocks parses Markdown once and records compact source ranges for its
// top-level semantic nodes. Lists, block quotes, and fenced blocks stay intact.
func IndexBlocks(source string) []Block {
	if source == "" {
		return nil
	}

	root := goldmark.New().Parser().Parse(text.NewReader([]byte(source)))
	type indexedNode struct {
		start int
		node  ast.Node
	}
	nodes := make([]indexedNode, 0, root.ChildCount())
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		start, ok := nodeStart(node)
		if !ok {
			continue
		}
		start = lineStart(source, start)
		if fenced, isFence := node.(*ast.FencedCodeBlock); isFence && fenced.Info == nil {
			start = previousLineStart(source, start)
		}
		nodes = append(nodes, indexedNode{start: start, node: node})
	}
	if len(nodes) == 0 {
		return []Block{{Start: 0, End: len(source), Words: len(strings.Fields(source))}}
	}
	nodes[0].start = 0 // retain leading reference definitions and comments

	blocks := make([]Block, 0, len(nodes))
	for index, indexed := range nodes {
		start := indexed.start
		end := len(source)
		if index+1 < len(nodes) {
			end = nodes[index+1].start
		}
		if end <= start {
			continue
		}
		block := Block{Start: start, End: end, Words: len(strings.Fields(source[start:end]))}
		block.Image = standaloneImage(indexed.node, []byte(source))
		block.Mermaid = mermaidSource(indexed.node, []byte(source))
		blocks = append(blocks, block)
	}
	return blocks
}

// ResolveImages resolves local destinations relative to the Markdown file.
func ResolveImages(blocks []Block, documentPath string) []Block {
	resolved := append([]Block(nil), blocks...)
	baseDir := filepath.Dir(documentPath)
	for index := range resolved {
		if resolved[index].Image == nil {
			continue
		}
		imageCopy := *resolved[index].Image
		parsed, err := url.Parse(imageCopy.Destination)
		if err == nil && parsed.Scheme == "" && !filepath.IsAbs(parsed.Path) {
			parsed.Path = filepath.Join(baseDir, filepath.FromSlash(parsed.Path))
			imageCopy.Destination = parsed.String()
		}
		resolved[index].Image = &imageCopy
	}
	return resolved
}

func mermaidSource(node ast.Node, source []byte) *string {
	fence, ok := node.(*ast.FencedCodeBlock)
	if !ok || !strings.EqualFold(string(fence.Language(source)), "mermaid") {
		return nil
	}
	var diagram strings.Builder
	for index := 0; index < fence.Lines().Len(); index++ {
		segment := fence.Lines().At(index)
		diagram.Write(segment.Value(source))
	}
	value := strings.TrimSpace(diagram.String())
	return &value
}

func standaloneImage(node ast.Node, source []byte) *Image {
	paragraph, ok := node.(*ast.Paragraph)
	if !ok || paragraph.ChildCount() != 1 {
		return nil
	}
	imageNode, ok := paragraph.FirstChild().(*ast.Image)
	if !ok {
		return nil
	}
	return &Image{
		Destination: string(imageNode.Destination),
		Alt:         string(imageNode.Text(source)),
	}
}

func nodeStart(root ast.Node) (int, bool) {
	start := 0
	found := false
	if fenced, ok := root.(*ast.FencedCodeBlock); ok && fenced.Info != nil {
		start = fenced.Info.Segment.Start
		found = true
	}
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || node.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		lines := node.Lines()
		for index := 0; index < lines.Len(); index++ {
			candidate := lines.At(index).Start
			if !found || candidate < start {
				start = candidate
				found = true
			}
		}
		return ast.WalkContinue, nil
	})
	return start, found
}

func previousLineStart(source string, currentStart int) int {
	if currentStart <= 0 {
		return 0
	}
	previousEnd := currentStart - 1
	if newline := strings.LastIndexByte(source[:previousEnd], '\n'); newline >= 0 {
		return newline + 1
	}
	return 0
}

func lineStart(source string, offset int) int {
	if offset <= 0 {
		return 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	if newline := strings.LastIndexByte(source[:offset], '\n'); newline >= 0 {
		return newline + 1
	}
	return 0
}

// ReferenceContext returns link-reference definitions so independently
// rendered semantic blocks retain document-level link resolution.
func ReferenceContext(source string) string {
	lines := strings.Split(source, "\n")
	definitions := make([]string, 0)
	capturing := false
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		isDefinition := indent <= 3 && strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]:")
		if isDefinition {
			definitions = append(definitions, line)
			capturing = true
			continue
		}
		if capturing && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			definitions = append(definitions, line)
			continue
		}
		capturing = false
	}
	return strings.Join(definitions, "\n")
}
