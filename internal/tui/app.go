package tui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/example/yocto-lens/internal/analyzer"
	"github.com/example/yocto-lens/internal/model"
)

const (
	minWidth  = 100
	minHeight = 30
)

type ViewState int

const (
	StateScanning ViewState = iota
	StateDashboard
	StateDetail
	StateError
)

type AnalysisMode int

const (
	ModeStaticAnalysis AnalysisMode = iota
	ModeStyleCheck
)

func (m AnalysisMode) String() string {
	if m == ModeStyleCheck {
		return "Style Check"
	}
	return "Static Analysis"
}

type progressMsg model.ScanProgress

type scanDoneMsg struct {
	Report model.Report
	Err    error
}

type scanChannelsMsg struct {
	Progress <-chan model.ScanProgress
	Done     <-chan scanDoneMsg
}

type findingItem struct {
	Finding model.Finding
}

func (f findingItem) Title() string {
	return f.Finding.Title
}

func (f findingItem) Description() string {
	return fmt.Sprintf(
		"%s %s:%d",
		severityBadge(f.Finding.Severity),
		compactPath(f.Finding.File, 90),
		f.Finding.Line,
	)
}

func (f findingItem) FilterValue() string {
	return strings.Join([]string{
		f.Finding.Title,
		f.Finding.File,
		filepath.Base(f.Finding.File),
		f.Finding.Layer,
		f.Finding.RuleID,
		string(f.Finding.Severity),
		f.Finding.Message,
	}, " ")
}

type App struct {
	paths []string

	report   model.Report
	progress model.ScanProgress

	list    list.Model
	spinner spinner.Model

	mode AnalysisMode

	state      ViewState
	width      int
	height     int
	errMessage string

	progressCh <-chan model.ScanProgress
	doneCh     <-chan scanDoneMsg
}

func NewLoading(paths []string) App {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return App{
		paths:   paths,
		state:   StateScanning,
		mode:    ModeStaticAnalysis,
		width:   120,
		height:  38,
		spinner: s,
		progress: model.ScanProgress{
			Phase:       model.PhaseStarting,
			CurrentPath: strings.Join(paths, ", "),
		},
	}
}

func New(r model.Report) App {
	app := NewLoading(nil)
	app.report = r
	app.state = StateDashboard
	app.mode = ModeStaticAnalysis
	app.list = newFindingList(app.visibleFindings())
	return app
}

func (a App) Init() tea.Cmd {
	if a.state == StateScanning {
		return tea.Batch(
			a.spinner.Tick,
			startScan(a.paths),
		)
	}
	return nil
}

func startScan(paths []string) tea.Cmd {
	return func() tea.Msg {
		progressCh := make(chan model.ScanProgress, 128)
		doneCh := make(chan scanDoneMsg, 1)

		go func() {
			report, err := analyzer.AnalyzeWithProgress(paths, func(p model.ScanProgress) {
				select {
				case progressCh <- p:
				default:
				}
			})

			doneCh <- scanDoneMsg{
				Report: report,
				Err:    err,
			}
			close(progressCh)
		}()

		return scanChannelsMsg{
			Progress: progressCh,
			Done:     doneCh,
		}
	}
}

func waitScan(progressCh <-chan model.ScanProgress, doneCh <-chan scanDoneMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case done := <-doneCh:
			return done
		case p, ok := <-progressCh:
			if !ok {
				return progressMsg(model.ScanProgress{
					Phase: model.PhaseDone,
					Done:  true,
				})
			}
			return progressMsg(p)
		}
	}
}

type findingDelegate struct{}

func (d findingDelegate) Height() int {
	return 3
}

func (d findingDelegate) Spacing() int {
	return 0
}

func (d findingDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d findingDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	finding, ok := item.(findingItem)
	if !ok {
		return
	}

	width := m.Width()
	if width < 20 {
		width = 20
	}

	bg := colorBg1
	titleFg := colorAccent
	descFg := colorText

	if index == m.Index() {
		bg = colorSelected
		titleFg = colorBg0
		descFg = colorBg0
	}

	title := lipgloss.NewStyle().
		Width(width).
		Background(bg).
		Foreground(titleFg).
		Bold(true).
		PaddingLeft(1).
		Render(truncate(finding.Finding.Title, width-2))

	desc := lipgloss.NewStyle().
		Width(width).
		Background(bg).
		Foreground(descFg).
		PaddingLeft(1).
		Render(
			severityBadge(finding.Finding.Severity) +
				" " +
				compactPath(finding.Finding.File, width-15) +
				fmt.Sprintf(":%d", finding.Finding.Line),
		)

	blank := lipgloss.NewStyle().
		Width(width).
		Background(colorBg1).
		Render("")

	fmt.Fprint(w, title+"\n"+desc+"\n"+blank)
}

func newFindingList(findings []model.Finding) list.Model {
	items := make([]list.Item, 0, len(findings))
	for _, finding := range findings {
		items = append(items, findingItem{Finding: finding})
	}

	l := list.New(items, findingDelegate{}, 80, 20)
	l.Title = ""
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetShowFilter(false)

	return l
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		if a.width < minWidth {
			a.width = minWidth
		}
		if a.height < minHeight {
			a.height = minHeight
		}

	case scanChannelsMsg:
		a.progressCh = m.Progress
		a.doneCh = m.Done
		return a, waitScan(a.progressCh, a.doneCh)

	case progressMsg:
		a.progress = model.ScanProgress(m)
		if a.state == StateScanning && a.progressCh != nil && a.doneCh != nil {
			return a, waitScan(a.progressCh, a.doneCh)
		}

	case scanDoneMsg:
		if m.Err != nil {
			a.state = StateError
			a.errMessage = m.Err.Error()
			return a, nil
		}
		a.report = m.Report
		a.mode = ModeStaticAnalysis
		a.list = newFindingList(a.visibleFindings())
		a.state = StateDashboard
		return a, nil

	case spinner.TickMsg:
		if a.state == StateScanning {
			var cmd tea.Cmd
			a.spinner, cmd = a.spinner.Update(msg)
			return a, cmd
		}

	case tea.KeyMsg:
		switch m.String() {
		case "q", "ctrl+c":
			return a, tea.Quit
		case "m":
			if a.state == StateDashboard {
				a.toggleMode()
			}
			return a, nil
		case "enter":
			if a.state == StateDashboard && a.list.SelectedItem() != nil {
				a.state = StateDetail
			}
			return a, nil
		case "esc":
			if a.state == StateDetail {
				a.state = StateDashboard
			}
			return a, nil
		}
	}

	if a.state == StateDashboard {
		var cmd tea.Cmd
		a.list, cmd = a.list.Update(msg)
		return a, cmd
	}

	return a, nil
}

func (a *App) toggleMode() {
	if a.mode == ModeStaticAnalysis {
		a.mode = ModeStyleCheck
	} else {
		a.mode = ModeStaticAnalysis
	}

	a.list = newFindingList(a.visibleFindings())
}

func (a App) visibleFindings() []model.Finding {
	return filterFindingsByMode(a.report.Findings, a.mode)
}

func (a App) View() string {
	if a.width < minWidth {
		a.width = minWidth
	}
	if a.height < minHeight {
		a.height = minHeight
	}

	switch a.state {
	case StateScanning:
		return a.scanView()
	case StateDetail:
		return a.detailView()
	case StateError:
		return a.errorView()
	default:
		return a.dashboardView()
	}
}

func (a App) scanView() string {
	w := a.contentWidth()

	body := strings.Join([]string{
		a.spinner.View() + " " + styleBold(colorAccent).Render("Scanning Yocto metadata"),
		"",
		label("Phase") + " " + normal(string(a.progress.Phase)),
		label("Layers") + " " + normal(fmt.Sprintf("%d", a.progress.LayersFound)),
		label("Recipes") + " " + normal(fmt.Sprintf("%d", a.progress.RecipesFound)),
		label("bbappends") + " " + normal(fmt.Sprintf("%d", a.progress.AppendsFound)),
		label("Patches") + " " + normal(fmt.Sprintf("%d", a.progress.PatchesFound)),
		label("Files") + " " + normal(fmt.Sprintf("%d", a.progress.FilesProcessed)),
		label("Findings") + " " + normal(fmt.Sprintf("%d", a.progress.FindingsFound)),
		"",
		label("Current") + " " + muted(compactPath(a.progress.CurrentPath, w-18)),
		"",
		progressBar(w-8, a.progress),
		"",
		muted("Large Yocto workspaces can take time. Generated build output is skipped automatically."),
		muted("q quit"),
	}, "\n")

	screen := lipgloss.JoinVertical(
		lipgloss.Left,
		a.header(w),
		panel("scanner", body, w, 18),
	)

	return rootFrame(a.width, a.height, screen)
}

func (a App) errorView() string {
	w := a.contentWidth()
	body := styleBold(colorRed).Render("Scan failed") + "\n\n" + normal(a.errMessage) + "\n\n" + muted("q quit")
	return rootFrame(a.width, a.height, panel("error", body, w, 10))
}

func (a App) dashboardView() string {
	w := a.contentWidth()

	header := a.header(w)
	modeBar := a.modeBar(w)
	overview := a.overviewPanel(w)

	gap := 1
	leftW := int(float64(w) * 0.64)
	rightW := w - leftW - gap

	if leftW < 60 {
		leftW = 60
		rightW = w - leftW - gap
	}
	if rightW < 34 {
		rightW = 34
		leftW = w - rightW - gap
	}

	mainHeight := a.height - 16
	if mainHeight < 12 {
		mainHeight = 12
	}

	listWidth := leftW - 4
	listHeight := mainHeight - 5

	if listWidth < 20 {
		listWidth = 20
	}
	if listHeight < 4 {
		listHeight = 4
	}

	a.list.SetSize(listWidth, listHeight)

	findingsBody := lipgloss.JoinVertical(
		lipgloss.Left,
		listBox(a.list.View(), listWidth, listHeight),
		a.pageIndicator(listWidth),
	)

	findings := panel(
		"findings",
		findingsBody,
		leftW,
		mainHeight,
	)

	inspector := panel(
		"inspector",
		a.previewText(rightW-4, mainHeight-2),
		rightW,
		mainHeight,
	)

	mainRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		findings,
		strings.Repeat(" ", gap),
		inspector,
	)

	footer := a.footer(w)

	screen := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		modeBar,
		overview,
		mainRow,
		footer,
	)

	return rootFrame(a.width, a.height, screen)
}

func (a App) detailView() string {
	w := a.contentWidth()

	selected, ok := a.list.SelectedItem().(findingItem)
	if !ok {
		return rootFrame(a.width, a.height, panel("detail", "No finding selected.", w, a.height-4))
	}

	f := selected.Finding

	body := strings.Join([]string{
		detailHeader(f, w-6),
		section("Problem", f.Message, w-6),
		section("Why it matters", f.WhyItMatters, w-6),
		section("Recommended fix", f.Remediation, w-6),
		muted("esc back • q quit"),
	}, "\n\n")

	screen := lipgloss.JoinVertical(
		lipgloss.Left,
		a.header(w),
		a.modeBar(w),
		panel("finding detail", body, w, a.height-6),
	)

	return rootFrame(a.width, a.height, screen)
}

func (a App) header(w int) string {
	left := styleBold(colorAccent).Render(" YOCTO LENS ")
	right := muted("Yocto / BitBake metadata scanner")

	used := lipgloss.Width(left) + lipgloss.Width(right)
	spaces := w - used
	if spaces < 1 {
		spaces = 1
	}

	return lipgloss.NewStyle().
		Width(w).
		Background(colorBg0).
		Render(left + strings.Repeat(" ", spaces) + right)
}

func (a App) modeBar(w int) string {
	staticCount := len(filterFindingsByMode(a.report.Findings, ModeStaticAnalysis))
	styleCount := len(filterFindingsByMode(a.report.Findings, ModeStyleCheck))

	modeLabel := lipgloss.NewStyle().
		Foreground(colorText).
		Background(colorBg3).
		Bold(true).
		Padding(0, 1).
		Render("Mode")

	staticTab := modeTab(fmt.Sprintf("Static Analysis %d", staticCount), a.mode == ModeStaticAnalysis)
	styleTab := modeTab(fmt.Sprintf("Style Check %d", styleCount), a.mode == ModeStyleCheck)

	left := modeLabel + " " + staticTab + " " + styleTab

	return lipgloss.NewStyle().
		Width(w).
		Background(colorBg0).
		Render(left)
}

func modeTab(name string, active bool) string {
	if active {
		return lipgloss.NewStyle().
			Foreground(colorBg0).
			Background(colorAccent).
			Bold(true).
			Padding(0, 2).
			Render(name)
	}

	return lipgloss.NewStyle().
		Foreground(colorMuted).
		Background(colorBg3).
		Bold(true).
		Padding(0, 2).
		Render(name)
}

func (a App) overviewPanel(w int) string {
	visible := a.visibleFindings()
	high, medium, low := counts(visible)
	score := riskScore(visible)

	gap := 1
	cardCount := 6
	cardWidth := (w - (gap * (cardCount - 1))) / cardCount
	if cardWidth < 14 {
		cardWidth = 14
	}

	cards := lipgloss.JoinHorizontal(
		lipgloss.Top,
		statCard("Layers", fmt.Sprintf("%d", len(a.report.Layers)), cardWidth),
		strings.Repeat(" ", gap),
		statCard("Recipes", fmt.Sprintf("%d", len(a.report.Recipes)), cardWidth),
		strings.Repeat(" ", gap),
		statCard("bbappends", fmt.Sprintf("%d", len(a.report.Appends)), cardWidth),
		strings.Repeat(" ", gap),
		statCard("Patches", fmt.Sprintf("%d", len(a.report.Patches)), cardWidth),
		strings.Repeat(" ", gap),
		statCard("Findings", fmt.Sprintf("%d", len(visible)), cardWidth),
		strings.Repeat(" ", gap),
		statCard("Risk", riskLabel(score), cardWidth),
	)

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		cards,
		"",
		issueMix(w-4, high, medium, low),
	)

	return panel("overview", body, w, 8)
}

func statCard(name string, value string, totalWidth int) string {
	contentWidth := totalWidth - 4
	if contentWidth < 8 {
		contentWidth = 8
	}

	title := lipgloss.NewStyle().
		Foreground(colorMuted).
		Background(colorBg1).
		Bold(true).
		Render(name)

	val := lipgloss.NewStyle().
		Foreground(colorAccent).
		Background(colorBg1).
		Bold(true).
		Render(value)

	body := title + "\n" + val

	return lipgloss.NewStyle().
		Width(contentWidth).
		Height(2).
		Background(colorBg1).
		Foreground(colorText).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Render(body)
}

func issueMix(width int, high int, medium int, low int) string {
	total := high + medium + low
	if total == 0 {
		return styleBold(colorGreen).Render("Issue Mix clean")
	}

	labelText := "Issue Mix "
	labelStyled := styleBold(colorMuted).Render(labelText)

	statsText := fmt.Sprintf("High %d Medium %d Low %d", high, medium, low)
	statsStyled := severityColorText(model.SeverityHigh, fmt.Sprintf("High %d", high)) +
		muted(" ") +
		severityColorText(model.SeverityMedium, fmt.Sprintf("Medium %d", medium)) +
		muted(" ") +
		severityColorText(model.SeverityLow, fmt.Sprintf("Low %d", low))

	barWidth := width - lipgloss.Width(labelText) - lipgloss.Width(statsText) - 6
	if barWidth < 20 {
		barWidth = 20
	}

	highCells := high * barWidth / total
	mediumCells := medium * barWidth / total
	lowCells := low * barWidth / total

	used := highCells + mediumCells + lowCells
	if used < barWidth {
		highCells += barWidth - used
	}

	bar := lipgloss.NewStyle().Foreground(colorRed).Render(strings.Repeat("━", highCells)) +
		lipgloss.NewStyle().Foreground(colorYellow).Render(strings.Repeat("━", mediumCells)) +
		lipgloss.NewStyle().Foreground(colorGreen).Render(strings.Repeat("━", lowCells))

	return labelStyled + bar + muted(" ") + statsStyled
}

func (a App) previewText(width int, height int) string {
	selected, ok := a.list.SelectedItem().(findingItem)
	if !ok {
		return muted("No finding selected.")
	}

	f := selected.Finding

	lines := []string{
		styleBold(colorText).Render(f.Title),
		"",
		label("Severity") + " " + severityText(f.Severity),
		label("Rule") + " " + normal(emptyDash(f.RuleID)),
		label("Layer") + " " + normal(emptyDash(f.Layer)),
		label("File") + " " + normal(compactPath(f.File, width-12)),
		label("Line") + " " + normal(fmt.Sprintf("%d", f.Line)),
		"",
		styleBold(colorGreen).Render("Problem"),
		wrapText(emptyDash(f.Message), width),
		"",
		styleBold(colorGreen).Render("Fix"),
		wrapText(emptyDash(f.Remediation), width),
		"",
		muted("Press / to fuzzy-search rule, severity, file, layer, or title."),
		muted("Press enter for full detail."),
	}

	out := strings.Join(lines, "\n")
	return trimHeight(out, height)
}

func detailHeader(f model.Finding, width int) string {
	lines := []string{
		styleBold(colorText).Render(f.Title),
		"",
		label("Severity") + " " + severityText(f.Severity),
		label("Rule") + " " + normal(emptyDash(f.RuleID)),
		label("Layer") + " " + normal(emptyDash(f.Layer)),
		label("File") + " " + normal(compactPath(f.File, width-12)),
		label("Line") + " " + normal(fmt.Sprintf("%d", f.Line)),
	}

	return strings.Join(lines, "\n")
}

func section(name string, text string, width int) string {
	return styleBold(colorGreen).Render(name) + "\n" + normal(wrapText(emptyDash(text), width))
}

func progressBar(width int, p model.ScanProgress) string {
	if width < 20 {
		width = 20
	}

	position := p.FilesProcessed % width
	if position == 0 {
		position = width / 2
	}

	left := strings.Repeat("━", position)
	right := strings.Repeat("━", width-position)

	return lipgloss.NewStyle().Foreground(colorAccent).Render(left) +
		lipgloss.NewStyle().Foreground(colorBorder).Render(right)
}

func panel(title string, body string, totalWidth int, totalHeight int) string {
	if totalWidth < 30 {
		totalWidth = 30
	}
	if totalHeight < 4 {
		totalHeight = 4
	}

	contentWidth := totalWidth - 4
	contentHeight := totalHeight - 3

	if contentWidth < 10 {
		contentWidth = 10
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	body = fitPanelBody(body, contentHeight)

	titleLine := lipgloss.NewStyle().
		Width(totalWidth).
		Foreground(colorMuted).
		Background(colorBg0).
		Bold(true).
		Render(title)

	box := lipgloss.NewStyle().
		Width(contentWidth).
		Height(contentHeight).
		Background(colorBg1).
		Foreground(colorText).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Render(body)

	return lipgloss.JoinVertical(lipgloss.Left, titleLine, box)
}

func fitPanelBody(body string, height int) string {
	lines := strings.Split(body, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}

	for len(lines) < height {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

func trimHeight(body string, height int) string {
	lines := strings.Split(body, "\n")
	if len(lines) <= height {
		return body
	}
	return strings.Join(lines[:height], "\n")
}

func listBox(view string, width int, height int) string {
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(colorBg1).
		Foreground(colorText).
		Render(view)
}

func (a App) pageIndicator(width int) string {
	totalPages := a.list.Paginator.TotalPages
	currentPage := a.list.Paginator.Page + 1

	if totalPages <= 0 {
		totalPages = 1
	}
	if currentPage <= 0 {
		currentPage = 1
	}

	maxPagesToShow := 9
	start := currentPage - 4
	if start < 1 {
		start = 1
	}

	end := start + maxPagesToShow - 1
	if end > totalPages {
		end = totalPages
	}

	if end-start+1 < maxPagesToShow {
		start = end - maxPagesToShow + 1
		if start < 1 {
			start = 1
		}
	}

	parts := []string{}

	if start > 1 {
		parts = append(parts, muted("1"))
		if start > 2 {
			parts = append(parts, muted("…"))
		}
	}

	for i := start; i <= end; i++ {
		page := fmt.Sprintf("%d", i)
		if i == currentPage {
			parts = append(parts, lipgloss.NewStyle().
				Foreground(colorBg0).
				Background(colorSelected).
				Bold(true).
				Padding(0, 1).
				Render(page))
		} else {
			parts = append(parts, muted(page))
		}
	}

	if end < totalPages {
		if end < totalPages-1 {
			parts = append(parts, muted("…"))
		}
		parts = append(parts, muted(fmt.Sprintf("%d", totalPages)))
	}

	line := "Pages " + strings.Join(parts, " ")

	return lipgloss.NewStyle().
		Width(width).
		Background(colorBg1).
		Align(lipgloss.Center).
		Render(muted(line))
}

func (a App) footer(w int) string {
	parts := []string{
		key("↑/k") + muted(" up"),
		key("↓/j") + muted(" down"),
		key("m") + muted(" mode"),
		key("/") + muted(" fuzzy search"),
		key("enter") + muted(" details"),
		key("esc") + muted(" back"),
		key("q") + muted(" quit"),
	}

	return lipgloss.NewStyle().
		Width(w).
		Background(colorBg0).
		Align(lipgloss.Center).
		Render(strings.Join(parts, muted(" • ")))
}

func rootFrame(width int, height int, content string) string {
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(colorBg0).
		Foreground(colorText).
		Padding(0, 1).
		Render(content)
}

func (a App) contentWidth() int {
	w := a.width - 2
	if w < minWidth-2 {
		w = minWidth - 2
	}
	return w
}

func filterFindingsByMode(findings []model.Finding, mode AnalysisMode) []model.Finding {
	filtered := make([]model.Finding, 0, len(findings))

	for _, finding := range findings {
		isStyle := isStyleFinding(finding)

		if mode == ModeStyleCheck && isStyle {
			filtered = append(filtered, finding)
		}

		if mode == ModeStaticAnalysis && !isStyle {
			filtered = append(filtered, finding)
		}
	}

	return filtered
}

func isStyleFinding(f model.Finding) bool {
	rule := strings.ToLower(f.RuleID)
	title := strings.ToLower(f.Title)
	message := strings.ToLower(f.Message)

	staticRules := []string{
		"autorev",
		"srcrev",
		"floating",
		"lic_files_chksum",
		"license",
		"checksum",
		"patch",
		"cve",
		"security",
		"vulnerability",
		"privileged",
		"host-network",
		"missing",
		"pinned",
	}

	for _, keyword := range staticRules {
		if strings.Contains(rule, keyword) ||
			strings.Contains(title, keyword) ||
			strings.Contains(message, keyword) {
			return false
		}
	}

	styleRules := []string{
		"style",
		"format",
		"formatting",
		"indent",
		"indentation",
		"whitespace",
		"spacing",
		"naming",
		"lowercase",
		"uppercase",
		"case",
		"order",
		"ordering",
		"sort",
		"sorted",
		"comment",
		"comments",
		"trailing",
		"blank-line",
		"blank_line",
		"line-length",
		"line_length",
		"recipe-name",
		"variable-order",
		"section-order",
		"metadata-style",
		"bbappend-style",
	}

	for _, keyword := range styleRules {
		if strings.Contains(rule, keyword) ||
			strings.Contains(title, keyword) ||
			strings.Contains(message, keyword) {
			return true
		}
	}

	return false
}

func counts(findings []model.Finding) (int, int, int) {
	var high int
	var medium int
	var low int

	for _, finding := range findings {
		switch finding.Severity {
		case model.SeverityCritical, model.SeverityHigh:
			high++
		case model.SeverityMedium:
			medium++
		default:
			low++
		}
	}

	return high, medium, low
}

func riskScore(findings []model.Finding) int {
	score := 0

	for _, finding := range findings {
		switch finding.Severity {
		case model.SeverityCritical:
			score += 15
		case model.SeverityHigh:
			score += 10
		case model.SeverityMedium:
			score += 5
		case model.SeverityLow:
			score += 1
		default:
			score += 1
		}
	}

	return score
}

func riskLabel(score int) string {
	switch {
	case score >= 100:
		return "critical"
	case score >= 50:
		return "high"
	case score >= 20:
		return "medium"
	case score > 0:
		return "low"
	default:
		return "clean"
	}
}

func severityText(sev model.Severity) string {
	return severityColorText(sev, string(sev))
}

func severityBadge(sev model.Severity) string {
	return severityColorText(sev, "["+string(sev)+"]")
}

func severityColorText(sev model.Severity, text string) string {
	return styleBold(severityColor(sev)).Render(text)
}

func severityColor(sev model.Severity) lipgloss.Color {
	switch sev {
	case model.SeverityCritical:
		return colorPurple
	case model.SeverityHigh:
		return colorRed
	case model.SeverityMedium:
		return colorYellow
	case model.SeverityLow:
		return colorGreen
	default:
		return colorMuted
	}
}

func compactPath(path string, max int) string {
	if max < 12 {
		max = 12
	}

	clean := filepath.ToSlash(path)
	if lipgloss.Width(clean) <= max {
		return clean
	}

	parts := strings.Split(clean, "/")
	if len(parts) >= 3 {
		tail := strings.Join(parts[len(parts)-3:], "/")
		out := "…/" + tail
		if lipgloss.Width(out) <= max {
			return out
		}
	}

	return truncate("…"+clean, max)
}

func truncate(s string, max int) string {
	if max <= 1 {
		return "…"
	}

	runes := []rune(s)
	if len(runes) <= max {
		return s
	}

	return string(runes[:max-1]) + "…"
}

func wrapText(text string, width int) string {
	if width < 20 {
		width = 20
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return "-"
	}

	var lines []string
	var current strings.Builder

	for _, word := range words {
		if current.Len() == 0 {
			current.WriteString(word)
			continue
		}

		if current.Len()+1+len(word) > width {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(word)
			continue
		}

		current.WriteString(" ")
		current.WriteString(word)
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	return strings.Join(lines, "\n")
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func styleBold(color lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(color)
}

func normal(s string) string {
	return lipgloss.NewStyle().
		Foreground(colorText).
		Background(colorBg1).
		Render(s)
}

func muted(s string) string {
	return lipgloss.NewStyle().
		Foreground(colorMuted).
		Background(colorBg1).
		Render(s)
}

func label(s string) string {
	return styleBold(colorMuted).Render(s + ":")
}

func key(s string) string {
	return styleBold(colorAccent).Render(s)
}

var (
	colorBg0 = lipgloss.Color("#282828")
	colorBg1 = lipgloss.Color("#1d2021")
	colorBg2 = lipgloss.Color("#282828")
	colorBg3 = lipgloss.Color("#3c3836")

	colorBorder   = lipgloss.Color("#504945")
	colorSelected = lipgloss.Color("#d8a657")

	colorText  = lipgloss.Color("#d4be98")
	colorWhite = lipgloss.Color("#fbf1c7")
	colorMuted = lipgloss.Color("#928374")

	colorAccent = lipgloss.Color("#d8a657")
	colorPurple = lipgloss.Color("#d3869b")
	colorRed    = lipgloss.Color("#ea6962")
	colorYellow = lipgloss.Color("#d8a657")
	colorBlue   = lipgloss.Color("#7daea3")
	colorCyan   = lipgloss.Color("#89b482")
	colorGreen  = lipgloss.Color("#a9b665")
)
