package app

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/tolaniverse/termleaf/internal/document"
	pager "github.com/tolaniverse/termleaf/internal/page"
	"github.com/tolaniverse/termleaf/internal/render"
)

const (
	chromeHeight    = 2
	maxReadingWidth = 96
	wordsPerMinute  = 225
)

type renderResultMsg struct {
	generation int
	content    string
	spans      []pager.Span
	err        error
}

type layoutResultMsg struct {
	generation      int
	layout          pager.Layout
	showPage        int
	showEnd         bool
	restoreProgress float64
	restoreAnchor   pager.Anchor
	err             error
}

type jumpResultMsg struct {
	generation int
	layout     pager.Layout
	page       int
	anchor     pager.Anchor
	err        error
}

type pageResultMsg struct {
	generation int
	page       int
	content    string
	err        error
}

// Config contains the reader's initial state.
type Config struct {
	Name             string
	Markdown         string
	PageMode         bool
	RestoredLine     int
	RestoredProgress float64
	RestoredAnchor   pager.Anchor
	Blocks           []document.Block
	ReferenceContext string
	MMDC             *render.MMDCRenderer
	ImageMode        render.ImageMode
	EnableMMDC       bool
	Bookmarks        []Bookmark
}

// Bookmark is an in-memory semantic bookmark.
type Bookmark struct {
	Anchor pager.Anchor
	Label  string
}

// Model is the root Bubble Tea model.
type Model struct {
	name             string
	markdown         string
	blocks           []document.Block
	referenceContext string
	mmdc             *render.MMDCRenderer
	pageMode         bool
	viewport         viewport.Model
	width            int
	height           int
	bodyHeight       int
	contentWidth     int
	content          string
	lines            []string
	spans            []pager.Span
	layout           pager.Layout
	pageContent      string
	page             int
	cache            *render.Cache
	restoredLine     int
	restoredProgress float64
	restoredAnchor   pager.Anchor
	generation       int
	loading          bool
	planning         bool
	showHelp         bool
	searching        bool
	searchInput      textinput.Model
	searchQuery      string
	searchResults    []document.SearchResult
	searchIndex      int
	bookmarks        []Bookmark
	bookmarkIndex    int
	status           string
	err              error
}

// New creates a reader model.
func New(config Config) Model {
	vp := viewport.New()
	vp.SoftWrap = false
	vp.FillHeight = true
	cache := render.NewCacheWithImages(2<<20, config.ImageMode)
	cache.EnableMMDC(config.EnableMMDC)
	searchInput := textinput.New()
	searchInput.Prompt = "/ "
	searchInput.Placeholder = "find in document"
	searchInput.CharLimit = 256
	return Model{
		name:             config.Name,
		markdown:         config.Markdown,
		blocks:           config.Blocks,
		referenceContext: config.ReferenceContext,
		mmdc:             config.MMDC,
		pageMode:         config.PageMode,
		viewport:         vp,
		restoredLine:     config.RestoredLine,
		restoredProgress: config.RestoredProgress,
		restoredAnchor:   config.RestoredAnchor,
		cache:            cache,
		searchInput:      searchInput,
		bookmarks:        append([]Bookmark(nil), config.Bookmarks...),
		loading:          true,
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		lineAnchor := m.LineOffset()
		anchorProgress := m.ProgressFraction()
		semanticAnchor := m.SemanticAnchor()
		if m.loading && m.restoredAnchor.Valid {
			semanticAnchor = m.restoredAnchor
		}
		if m.width == 0 {
			lineAnchor = m.restoredLine
			anchorProgress = m.restoredProgress
			semanticAnchor = m.restoredAnchor
		}
		m.width = msg.Width
		m.height = msg.Height
		m.bodyHeight = max(1, m.height-chromeHeight)
		m.contentWidth = max(1, min(maxReadingWidth, m.width-2))
		m.viewport.SetWidth(m.contentWidth)
		m.viewport.SetHeight(m.bodyHeight)
		m.restoredLine = lineAnchor
		m.restoredProgress = anchorProgress
		m.restoredAnchor = semanticAnchor
		m.generation++
		m.loading = true
		if m.pageMode {
			if semanticAnchor.Valid {
				m.layout = pager.Layout{}
				m.pageContent = ""
				m.planning = true
				return m, discoverAnchor(m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.bodyHeight, m.cache, m.mmdc, semanticAnchor, m.generation)
			}
			targetPage := max(0, lineAnchor/max(1, m.bodyHeight))
			discoverTarget := targetPage + 1
			progressFallback := 0.0
			if lineAnchor == 0 && anchorProgress > 0 {
				discoverTarget = -1
				progressFallback = anchorProgress
			}
			m.layout = pager.Layout{}
			m.pageContent = ""
			m.planning = true
			return m, discoverPages(m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.bodyHeight, m.cache, m.mmdc, m.layout, discoverTarget, targetPage, false, progressFallback, m.generation)
		}
		return m, renderDocument(m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.cache, m.mmdc, m.generation)

	case layoutResultMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.planning = false
		if msg.err != nil {
			m.loading = false
			m.err = msg.err
			return m, nil
		}
		m.layout = msg.layout
		if msg.restoreAnchor.Valid {
			if pageIndex, ok := m.layout.PageForAnchor(m.blocks, msg.restoreAnchor); ok {
				m.page = pageIndex
			}
			m.loading = true
			return m, renderPage(m.layout, m.page, m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.cache, m.mmdc, m.generation)
		}
		if msg.restoreProgress > 0 {
			m.page = int(math.Round(msg.restoreProgress * float64(max(0, m.pageCount()-1))))
			m.loading = true
			return m, renderPage(m.layout, m.page, m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.cache, m.mmdc, m.generation)
		}
		if msg.showPage < 0 && !msg.showEnd {
			return m, nil
		}
		if msg.showEnd {
			m.page = max(0, len(m.layout.Pages)-1)
		} else {
			m.page = min(max(0, msg.showPage), m.pageCount()-1)
		}
		m.loading = true
		return m, renderPage(m.layout, m.page, m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.cache, m.mmdc, m.generation)

	case jumpResultMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.planning = false
		if msg.err != nil {
			m.loading = false
			m.err = msg.err
			return m, nil
		}
		m.restoredAnchor = msg.anchor
		if m.pageMode {
			m.layout = msg.layout
			m.page = msg.page
			m.loading = true
			return m, renderPage(m.layout, m.page, m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.cache, m.mmdc, m.generation)
		}
		m.viewport.SetYOffset(pager.LineForAnchor(m.blocks, m.spans, msg.anchor))
		m.loading = false
		return m, nil

	case pageResultMsg:
		if msg.generation != m.generation || msg.page != m.page {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.pageContent = msg.content
		if !m.layout.Complete && !m.planning && len(m.layout.Pages) <= m.page+1 {
			m.planning = true
			return m, discoverPages(m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.bodyHeight, m.cache, m.mmdc, m.layout, m.page+1, -1, false, 0, m.generation)
		}
		return m, nil

	case renderResultMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.content = msg.content
		m.spans = msg.spans
		m.lines = strings.Split(strings.TrimSuffix(msg.content, "\n"), "\n")
		m.viewport.SetContent(msg.content)
		if m.restoredAnchor.Valid {
			m.viewport.SetYOffset(pager.LineForAnchor(m.blocks, m.spans, m.restoredAnchor))
		} else if m.restoredProgress > 0 {
			m.restoreProgress(m.restoredProgress)
		} else {
			m.restoreLine(m.restoredLine)
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.searching {
			switch msg.String() {
			case "enter":
				query := strings.TrimSpace(m.searchInput.Value())
				m.searchInput.Blur()
				m.searching = false
				m.searchQuery = query
				summary := document.Search(m.markdown, query)
				m.searchResults = summary.Results
				m.searchIndex = 0
				if len(m.searchResults) == 0 {
					m.status = fmt.Sprintf("no matches for %q", query)
					return m, nil
				}
				totalLabel := fmt.Sprintf("%d", len(m.searchResults))
				if summary.Truncated {
					totalLabel += "+"
				}
				m.status = fmt.Sprintf("match 1/%s", totalLabel)
				return m.jumpToSearchResult(0)
			case "esc":
				m.searchInput.Blur()
				m.searching = false
				return m, nil
			}
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "/":
			if m.loading {
				return m, nil
			}
			m.searching = true
			m.searchInput.SetWidth(max(10, min(60, m.contentWidth-3)))
			return m, m.searchInput.Focus()
		case "n":
			if len(m.searchResults) == 0 || m.loading {
				return m, nil
			}
			return m.jumpToSearchResult((m.searchIndex + 1) % len(m.searchResults))
		case "N":
			if len(m.searchResults) == 0 || m.loading {
				return m, nil
			}
			return m.jumpToSearchResult((m.searchIndex - 1 + len(m.searchResults)) % len(m.searchResults))
		case "m":
			if m.loading {
				return m, nil
			}
			m.toggleBookmark()
			return m, nil
		case "B":
			if len(m.bookmarks) == 0 || m.loading {
				m.status = "no bookmarks"
				return m, nil
			}
			m.bookmarkIndex = m.nextBookmarkIndex()
			label := sanitizeLabel(m.bookmarks[m.bookmarkIndex].Label)
			m.status = fmt.Sprintf("bookmark %d/%d: %s", m.bookmarkIndex+1, len(m.bookmarks), label)
			return m.jumpToAnchor(m.bookmarks[m.bookmarkIndex].Anchor)
		case "esc":
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			return m, tea.Quit
		}
		if m.pageMode {
			if m.loading || len(m.layout.Pages) == 0 {
				return m, nil
			}
			nextPage := m.page
			discoverTarget := -2
			showEnd := false
			switch msg.String() {
			case "right", "l", "space", "pagedown":
				nextPage = m.page + 1
				if nextPage >= len(m.layout.Pages) {
					if !m.layout.Complete {
						discoverTarget = nextPage
					} else {
						nextPage = max(0, len(m.layout.Pages)-1)
					}
				}
			case "left", "h", "pageup":
				nextPage = max(0, m.page-1)
			case "g", "home":
				nextPage = 0
			case "G", "end":
				if !m.layout.Complete {
					discoverTarget = -1
					showEnd = true
				} else {
					nextPage = max(0, m.pageCount()-1)
				}
			default:
				return m, nil
			}
			if discoverTarget == -2 && nextPage == m.page {
				return m, nil
			}
			m.generation++
			m.planning = false
			m.loading = true
			m.pageContent = ""
			if discoverTarget != -2 {
				m.planning = true
				return m, discoverPages(m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.bodyHeight, m.cache, m.mmdc, m.layout, discoverTarget, nextPage, showEnd, 0, m.generation)
			}
			m.page = min(nextPage, m.pageCount()-1)
			return m, renderPage(m.layout, m.page, m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.cache, m.mmdc, m.generation)
		}
	}

	if !m.pageMode {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) jumpToSearchResult(index int) (tea.Model, tea.Cmd) {
	result := m.searchResults[index]
	offset, fraction, ok := document.SearchAnchor(m.blocks, result)
	if !ok {
		m.status = "match location is unavailable"
		return m, nil
	}
	m.searchIndex = index
	m.status = fmt.Sprintf("match %d/%d: %s", index+1, len(m.searchResults), result.Preview)
	return m.jumpToAnchor(pager.Anchor{SourceOffset: offset, BlockFraction: fraction, Valid: true})
}

func (m Model) jumpToAnchor(anchor pager.Anchor) (tea.Model, tea.Cmd) {
	if !anchor.Valid {
		return m, nil
	}
	m.generation++
	m.loading = true
	if m.pageMode {
		m.planning = true
		m.pageContent = ""
		return m, jumpAnchor(m.markdown, m.referenceContext, m.blocks, m.contentWidth, m.bodyHeight, m.cache, m.mmdc, anchor, m.generation)
	}
	return m, func() tea.Msg {
		return jumpResultMsg{generation: m.generation, anchor: anchor}
	}
}

func (m *Model) toggleBookmark() {
	anchor := m.SemanticAnchor()
	if !anchor.Valid {
		m.status = "bookmark unavailable"
		return
	}
	anchor.BlockFraction = canonicalBookmarkFraction(anchor.BlockFraction)
	for index, bookmark := range m.bookmarks {
		if bookmark.Anchor.SourceOffset == anchor.SourceOffset && canonicalBookmarkFraction(bookmark.Anchor.BlockFraction) == anchor.BlockFraction {
			m.bookmarks = append(m.bookmarks[:index], m.bookmarks[index+1:]...)
			m.status = "bookmark removed"
			return
		}
	}
	label := m.bookmarkLabel(anchor)
	m.bookmarks = append(m.bookmarks, Bookmark{Anchor: anchor, Label: label})
	sort.Slice(m.bookmarks, func(i, j int) bool {
		if m.bookmarks[i].Anchor.SourceOffset == m.bookmarks[j].Anchor.SourceOffset {
			return m.bookmarks[i].Anchor.BlockFraction < m.bookmarks[j].Anchor.BlockFraction
		}
		return m.bookmarks[i].Anchor.SourceOffset < m.bookmarks[j].Anchor.SourceOffset
	})
	m.status = "bookmark added: " + label
}

func canonicalBookmarkFraction(value float64) float64 {
	return math.Round(max(0, min(1, value))*1000) / 1000
}

func (m Model) nextBookmarkIndex() int {
	current := m.SemanticAnchor()
	for index, bookmark := range m.bookmarks {
		if bookmark.Anchor.SourceOffset > current.SourceOffset ||
			(bookmark.Anchor.SourceOffset == current.SourceOffset && bookmark.Anchor.BlockFraction > current.BlockFraction+0.0005) {
			return index
		}
	}
	return 0
}

func (m Model) bookmarkLabel(anchor pager.Anchor) string {
	for _, block := range m.blocks {
		if block.Start == anchor.SourceOffset {
			label := strings.TrimSpace(m.markdown[block.Start:block.End])
			label = strings.Join(strings.Fields(label), " ")
			return ansi.Truncate(sanitizeLabel(label), 48, "…")
		}
	}
	return fmt.Sprintf("offset %d", anchor.SourceOffset)
}

func sanitizeLabel(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
}

// Bookmarks returns a copy suitable for persistence.
func (m Model) Bookmarks() []Bookmark { return append([]Bookmark(nil), m.bookmarks...) }

func renderDocument(source, referenceContext string, blocks []document.Block, width int, cache *render.Cache, mmdc *render.MMDCRenderer, generation int) tea.Cmd {
	return func() tea.Msg {
		content, spans, err := pager.RenderAllWithSpans(source, referenceContext, blocks, width, cache, mmdc)
		return renderResultMsg{generation: generation, content: content, spans: spans, err: err}
	}
}

func jumpAnchor(source, referenceContext string, blocks []document.Block, width, height int, cache *render.Cache, mmdc *render.MMDCRenderer, anchor pager.Anchor, generation int) tea.Cmd {
	return func() tea.Msg {
		layout, pageIndex, err := pager.DiscoverAnchor(source, referenceContext, blocks, width, height, cache, mmdc, anchor)
		return jumpResultMsg{generation: generation, layout: layout, page: pageIndex, anchor: anchor, err: err}
	}
}

func discoverAnchor(source, referenceContext string, blocks []document.Block, width, height int, cache *render.Cache, mmdc *render.MMDCRenderer, anchor pager.Anchor, generation int) tea.Cmd {
	return func() tea.Msg {
		layout, _, err := pager.DiscoverAnchor(source, referenceContext, blocks, width, height, cache, mmdc, anchor)
		return layoutResultMsg{generation: generation, layout: layout, restoreAnchor: anchor, err: err}
	}
}

func discoverPages(source, referenceContext string, blocks []document.Block, width, height int, cache *render.Cache, mmdc *render.MMDCRenderer, existing pager.Layout, targetPage, showPage int, showEnd bool, restoreProgress float64, generation int) tea.Cmd {
	return func() tea.Msg {
		layout, err := pager.Discover(source, referenceContext, blocks, width, height, cache, mmdc, existing, targetPage)
		return layoutResultMsg{
			generation:      generation,
			layout:          layout,
			showPage:        showPage,
			showEnd:         showEnd,
			restoreProgress: restoreProgress,
			err:             err,
		}
	}
}

func renderPage(layout pager.Layout, pageIndex int, source, referenceContext string, blocks []document.Block, width int, cache *render.Cache, mmdc *render.MMDCRenderer, generation int) tea.Cmd {
	return func() tea.Msg {
		content, err := layout.Render(pageIndex, source, referenceContext, blocks, width, cache, mmdc)
		return pageResultMsg{generation: generation, page: pageIndex, content: content, err: err}
	}
}

// SemanticAnchor returns the current width-independent reading position.
func (m Model) SemanticAnchor() pager.Anchor {
	if m.pageMode {
		return m.layout.AnchorAt(m.blocks, m.page)
	}
	return pager.AnchorForLine(m.blocks, m.spans, m.viewport.YOffset())
}

// LineOffset returns the current rendered-line position for persistence.
func (m Model) LineOffset() int {
	if m.pageMode {
		return m.page * max(1, m.bodyHeight)
	}
	return m.viewport.YOffset()
}

// ProgressFraction returns the normalized reading position.
func (m Model) ProgressFraction() float64 {
	if m.pageMode {
		if m.layout.Complete {
			if m.pageCount() <= 1 {
				return 0
			}
			return float64(m.page) / float64(m.pageCount()-1)
		}
		if m.page < 0 || m.page >= len(m.layout.Pages) || len(m.blocks) == 0 {
			return 0
		}
		slices := m.layout.Pages[m.page].Slices
		if len(slices) == 0 {
			return 0
		}
		last := slices[len(slices)-1]
		blockFraction := 1.0
		if last.BlockLines > 0 {
			blockFraction = float64(last.EndLine) / float64(last.BlockLines)
		}
		return min(1, (float64(last.Block)+blockFraction)/float64(len(m.blocks)))
	}
	if len(m.lines) == 0 {
		return 0
	}
	return m.viewport.ScrollPercent()
}

func (m *Model) restoreProgress(progress float64) {
	progress = max(0, min(1, progress))
	if m.pageMode {
		m.page = int(math.Round(progress * float64(max(0, m.pageCount()-1))))
		return
	}
	maximumOffset := max(0, len(m.lines)-m.bodyHeight)
	m.viewport.SetYOffset(int(math.Round(progress * float64(maximumOffset))))
}

func (m *Model) restoreLine(line int) {
	line = max(0, line)
	if m.pageMode {
		m.page = min(line/max(1, m.bodyHeight), m.pageCount()-1)
		m.page = max(0, m.page)
		return
	}
	m.viewport.SetYOffset(line)
}

func (m Model) pageCount() int {
	return max(1, len(m.layout.Pages))
}

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return m.terminalView("Initializing…")
	}

	header := m.headerView()
	var body string
	switch {
	case m.searching:
		body = "Search\n\n" + m.searchInput.View() + "\n\nenter search · esc cancel"
	case m.showHelp:
		body = m.helpView()
	case m.err != nil:
		body = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Unable to render document:\n" + m.err.Error())
	case m.loading && ((!m.pageMode && m.content == "") || (m.pageMode && m.pageContent == "")):
		body = "Rendering…"
	case m.pageMode:
		body = m.pageContent
	default:
		body = m.viewport.View()
	}

	body = lipgloss.NewStyle().
		Width(m.contentWidth).
		Height(m.bodyHeight).
		Render(body)
	body = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, body)

	content := lipgloss.JoinVertical(lipgloss.Left, header, body, m.footerView())
	return m.terminalView(content)
}

func (m Model) terminalView(content string) tea.View {
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Termleaf — " + m.name
	return view
}

func (m Model) headerView() string {
	name := ansi.Truncate(m.name, max(1, m.width-2), "…")
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		Width(m.width).
		Align(lipgloss.Center).
		Render(name)
}

func (m Model) helpView() string {
	if m.pageMode {
		return "Termleaf page mode\n\n←/h  previous page\n→/l/space  next page\n/ search · n/N matches\nm toggle bookmark · B next bookmark\ng/G  first/last page\n? or esc  close help\nq  save and quit"
	}
	return "Termleaf scroll mode\n\nj/k or ↑/↓  scroll\nspace/b  page down/up\n/ search · n/N matches\nm toggle bookmark · B next bookmark\ng/G  start/end\n? or esc  close help\nq  save and quit"
}

func (m Model) footerView() string {
	var footer string
	if m.pageMode {
		pageText := paginator(m.page+1, m.pageCount(), m.layout.Complete, m.width)
		words := 0
		if m.page >= 0 && m.page < len(m.layout.Pages) {
			words = m.layout.Pages[m.page].Words
		}
		progressPrefix := "~"
		if m.layout.Complete {
			progressPrefix = ""
		}
		footer = fmt.Sprintf("%s   %d words · %s · %s%d%%", pageText, words, readingTime(words), progressPrefix, int(math.Round(m.ProgressFraction()*100)))
	} else {
		footer = fmt.Sprintf("j/k scroll · ? help · q quit   %d%%", int(math.Round(m.viewport.ScrollPercent()*100)))
	}
	if m.status != "" {
		footer = sanitizeLabel(m.status) + "   " + footer
	}
	footer = ansi.Truncate(footer, max(1, m.width), "…")
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Width(m.width).
		Align(lipgloss.Center).
		Render(footer)
}

func paginator(current, total int, complete bool, width int) string {
	if width < 42 {
		if !complete {
			return fmt.Sprintf("◀ [%d/?] ▶", current)
		}
		return fmt.Sprintf("◀ [%d/%d] ▶", current, total)
	}
	if !complete {
		wanted := []int{1, current - 1, current, current + 1}
		parts := make([]string, 0, len(wanted)+2)
		last := 0
		for _, page := range wanted {
			if page < 1 || page > total || page == last {
				continue
			}
			if last > 0 && page-last > 1 {
				parts = append(parts, "…")
			}
			parts = append(parts, pageLabel(page, current))
			last = page
		}
		parts = append(parts, "…", "?")
		return "◀ " + strings.Join(parts, " ") + " ▶"
	}
	if total <= 7 {
		parts := make([]string, 0, total)
		for page := 1; page <= total; page++ {
			parts = append(parts, pageLabel(page, current))
		}
		return "◀ " + strings.Join(parts, " ") + " ▶"
	}

	wanted := []int{1, current - 1, current, current + 1, total}
	pages := make([]int, 0, len(wanted))
	for _, page := range wanted {
		if page < 1 || page > total || (len(pages) > 0 && pages[len(pages)-1] == page) {
			continue
		}
		pages = append(pages, page)
	}
	parts := make([]string, 0, len(pages)+2)
	for index, page := range pages {
		if index > 0 && page-pages[index-1] > 1 {
			parts = append(parts, "…")
		}
		parts = append(parts, pageLabel(page, current))
	}
	return "◀ " + strings.Join(parts, " ") + " ▶"
}

func pageLabel(page, current int) string {
	if page == current {
		return fmt.Sprintf("[%d]", page)
	}
	return fmt.Sprintf("%d", page)
}

func wordCount(content string) int {
	return len(strings.Fields(ansi.Strip(content)))
}

func readingTime(words int) string {
	if words == 0 {
		return "0 min"
	}
	if words < wordsPerMinute {
		return "<1 min"
	}
	minutes := int(math.Ceil(float64(words) / wordsPerMinute))
	return fmt.Sprintf("~%d min", minutes)
}

func progress(current, total int) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(current) / float64(total) * 100))
}
