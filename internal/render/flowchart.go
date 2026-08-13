package render

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

type flowNode struct {
	id    string
	label string
	shape byte
}

type flowEdge struct {
	from  string
	to    string
	label string
}

type flowDiagram struct {
	direction string
	nodes     map[string]flowNode
	order     []string
	edges     []flowEdge
}

var flowNodePattern = regexp.MustCompile(`^\s*([[:alnum:]_-]+)(?:\s*(\[\[[^]]*\]\]|\(\([^)]*\)\)|\(\[[^]]*\]\)|\[\([^)]*\)\]|\[[^]]*\]|\{[^}]*\}|\([^)]*\)))?`)

func isFlowchart(source string) bool {
	first := firstMermaidStatement(source)
	fields := strings.Fields(strings.TrimSuffix(first, ";"))
	return len(fields) > 0 && (fields[0] == "graph" || fields[0] == "flowchart")
}

func renderFlowchart(source string, width int) (string, error) {
	diagram, err := parseFlowchart(source)
	if err != nil {
		return "", err
	}
	if diagram.direction == "LR" || diagram.direction == "RL" {
		return renderHorizontalFlow(diagram, width, diagram.direction == "RL"), nil
	}
	return renderVerticalFlow(diagram, width, diagram.direction == "BT"), nil
}

func parseFlowchart(source string) (flowDiagram, error) {
	result := flowDiagram{direction: "TD", nodes: make(map[string]flowNode)}
	lines := strings.Split(source, "\n")
	declarationSeen := false
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !declarationSeen {
			declaration, remainder, _ := strings.Cut(line, ";")
			fields := strings.Fields(strings.TrimSpace(declaration))
			if len(fields) == 0 || (fields[0] != "graph" && fields[0] != "flowchart") {
				return result, errorsf("not a flowchart")
			}
			if len(fields) > 1 {
				result.direction = strings.ToUpper(fields[1])
				if result.direction == "TB" {
					result.direction = "TD"
				}
			}
			declarationSeen = true
			line = strings.TrimSpace(remainder)
			if line == "" {
				continue
			}
		}
		for _, statement := range strings.Split(line, ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if statement == "end" || strings.HasPrefix(statement, "subgraph ") || strings.HasPrefix(statement, "classDef ") || strings.HasPrefix(statement, "class ") || strings.HasPrefix(statement, "style ") || strings.HasPrefix(statement, "linkStyle ") || strings.HasPrefix(statement, "click ") {
				continue
			}
			if err := result.parseStatement(statement); err != nil {
				return result, err
			}
		}
	}
	if !declarationSeen {
		return result, errorsf("flowchart declaration is missing")
	}
	if len(result.nodes) == 0 {
		return result, errorsf("flowchart has no nodes")
	}
	return result, nil
}

func (diagram *flowDiagram) parseStatement(statement string) error {
	current, rest, ok := parseFlowNode(statement)
	if !ok {
		return errorsf("unsupported flowchart statement %q", ansi.Truncate(statement, 40, "…"))
	}
	diagram.addNode(current)
	for strings.TrimSpace(rest) != "" {
		label, afterArrow, ok := parseFlowArrow(rest)
		if !ok {
			// Mermaid metadata after a node does not affect the terminal drawing.
			if strings.HasPrefix(strings.TrimSpace(rest), ":::") {
				return nil
			}
			return errorsf("unsupported flowchart connection near %q", ansi.Truncate(strings.TrimSpace(rest), 32, "…"))
		}
		next, remaining, ok := parseFlowNode(afterArrow)
		if !ok {
			return errorsf("flowchart connection has no destination")
		}
		diagram.addNode(next)
		diagram.edges = append(diagram.edges, flowEdge{from: current.id, to: next.id, label: cleanFlowLabel(label)})
		current = next
		rest = remaining
	}
	return nil
}

func parseFlowNode(value string) (flowNode, string, bool) {
	match := flowNodePattern.FindStringSubmatchIndex(value)
	if match == nil {
		return flowNode{}, value, false
	}
	id := value[match[2]:match[3]]
	label := id
	shape := byte('[')
	if match[4] >= 0 {
		syntax := value[match[4]:match[5]]
		label = trimFlowShape(syntax)
		shape = syntax[0]
	}
	return flowNode{id: id, label: cleanFlowLabel(label), shape: shape}, value[match[1]:], true
}

func parseFlowArrow(value string) (label, rest string, ok bool) {
	value = strings.TrimSpace(value)
	arrowEnd := -1
	for index := 0; index < len(value); index++ {
		if value[index] == '>' {
			arrowEnd = index + 1
			break
		}
	}
	if arrowEnd < 0 || arrowEnd > 12 {
		return "", value, false
	}
	arrow := value[:arrowEnd]
	if !strings.ContainsAny(arrow, "-=.") {
		return "", value, false
	}
	rest = strings.TrimSpace(value[arrowEnd:])
	if strings.HasPrefix(rest, "|") {
		end := strings.Index(rest[1:], "|")
		if end < 0 {
			return "", value, false
		}
		label = rest[1 : end+1]
		rest = strings.TrimSpace(rest[end+2:])
	}
	return label, rest, true
}

func (diagram *flowDiagram) addNode(node flowNode) {
	if existing, ok := diagram.nodes[node.id]; ok {
		if node.label != node.id {
			existing.label = node.label
			existing.shape = node.shape
			diagram.nodes[node.id] = existing
		}
		return
	}
	diagram.nodes[node.id] = node
	diagram.order = append(diagram.order, node.id)
}

func renderHorizontalFlow(diagram flowDiagram, width int, reverse bool) string {
	lines := make([]string, 0, max(1, len(diagram.edges))*4)
	if len(diagram.edges) == 0 {
		for _, id := range diagram.order {
			lines = append(lines, renderFlowBox(diagram.nodes[id], max(8, width))...)
		}
		return strings.Join(lines, "\n")
	}
	for edgeIndex, edge := range diagram.edges {
		left, right := diagram.nodes[edge.from], diagram.nodes[edge.to]
		arrowHead := "►"
		if reverse {
			left, right = right, left
			arrowHead = "◄"
		}
		available := max(16, width-9)
		leftWidth := min(flowBoxWidth(left), available/2)
		rightWidth := min(flowBoxWidth(right), available-leftWidth)
		leftBox := renderFlowBoxWidth(left, leftWidth)
		rightBox := renderFlowBoxWidth(right, rightWidth)
		arrow := "───" + arrowHead
		if edge.label != "" {
			arrow = "─" + ansi.Truncate(edge.label, max(1, width-leftWidth-rightWidth-5), "…") + "─" + arrowHead
		}
		gap := strings.Repeat(" ", max(1, ansi.StringWidth(arrow)+2))
		lines = append(lines, leftBox[0]+gap+rightBox[0], leftBox[1]+" "+arrow+" "+rightBox[1], leftBox[2]+gap+rightBox[2])
		if edgeIndex+1 < len(diagram.edges) {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func renderVerticalFlow(diagram flowDiagram, width int, reverse bool) string {
	lines := make([]string, 0, max(1, len(diagram.edges))*8)
	if len(diagram.edges) == 0 {
		for _, id := range diagram.order {
			lines = append(lines, centerFlowLines(renderFlowBox(diagram.nodes[id], width), width)...)
		}
		return strings.Join(lines, "\n")
	}
	for edgeIndex, edge := range diagram.edges {
		fromNode, toNode := diagram.nodes[edge.from], diagram.nodes[edge.to]
		arrowHead := "▼"
		if reverse {
			fromNode, toNode = toNode, fromNode
			arrowHead = "▲"
		}
		from := centerFlowLines(renderFlowBox(fromNode, width), width)
		to := centerFlowLines(renderFlowBox(toNode, width), width)
		lines = append(lines, from...)
		connector := "│"
		if edge.label != "" {
			connector += " " + edge.label
		}
		lines = append(lines, centerFlowLine(connector, width), centerFlowLine(arrowHead, width))
		lines = append(lines, to...)
		if edgeIndex+1 < len(diagram.edges) {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func renderFlowBox(node flowNode, width int) []string {
	return renderFlowBoxWidth(node, min(flowBoxWidth(node), max(6, width)))
}

func renderFlowBoxWidth(node flowNode, width int) []string {
	width = max(6, width)
	label := ansi.Truncate(node.label, width-4, "…")
	padding := width - 2 - ansi.StringWidth(label)
	leftPadding := padding / 2
	rightPadding := padding - leftPadding
	topLeft, topRight, bottomLeft, bottomRight := "┌", "┐", "└", "┘"
	if node.shape == '{' || node.shape == '(' {
		topLeft, topRight, bottomLeft, bottomRight = "╭", "╮", "╰", "╯"
	}
	return []string{
		topLeft + strings.Repeat("─", width-2) + topRight,
		"│" + strings.Repeat(" ", leftPadding) + label + strings.Repeat(" ", rightPadding) + "│",
		bottomLeft + strings.Repeat("─", width-2) + bottomRight,
	}
}

func flowBoxWidth(node flowNode) int { return max(8, ansi.StringWidth(node.label)+4) }

func centerFlowLines(lines []string, width int) []string {
	centered := make([]string, len(lines))
	for index, line := range lines {
		centered[index] = centerFlowLine(line, width)
	}
	return centered
}

func centerFlowLine(line string, width int) string {
	return strings.Repeat(" ", max(0, (width-ansi.StringWidth(line))/2)) + line
}

func trimFlowShape(value string) string {
	return strings.Trim(value, "[]{}() ")
}

func cleanFlowLabel(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func firstMermaidStatement(source string) string {
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "%%") {
			return line
		}
	}
	return ""
}

func errorsf(format string, values ...any) error { return fmt.Errorf(format, values...) }
