package dev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/localserver"
)

var errFixtureReviewUnavailable = errors.New("fixture review TUI unavailable")

type fixtureReviewOptions struct {
	FixturesDir     string
	Results         []conformanceResult
	StartAt         string
	FilterStatus    string
	FilterAccuracy  string
	AllowOpenViewer bool
}

type reviewServer struct {
	URL      string
	server   *http.Server
	listener net.Listener
	dataDir  string
}

func (s *reviewServer) Close() {
	if s == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if s.server != nil {
		_ = s.server.Shutdown(ctx)
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.dataDir != "" {
		_ = os.RemoveAll(s.dataDir)
	}
}

type fixtureReviewItem struct {
	Result    conformanceResult
	Manifest  fixtureManifest
	RelPath   string
	Path      string
	Ephemeral bool
}

type reviewFocus int

const (
	reviewFocusNav reviewFocus = iota
	reviewFocusDiff
	reviewFocusComments
	reviewFocusFilter
)

type fixtureReviewModel struct {
	ctx       context.Context
	root      string
	items     []fixtureReviewItem
	selected  int
	width     int
	height    int
	focus     reviewFocus
	statusMsg string
	textarea  textarea.Model
	filter    textinput.Model
	progress  progress.Model
	server    *reviewServer
}

func runFixtureReviewTUI(ctx context.Context, out io.Writer, opts fixtureReviewOptions) error {
	if len(opts.Results) == 0 {
		return fmt.Errorf("no fixtures discovered")
	}
	rootAbs, err := filepath.Abs(opts.FixturesDir)
	if err != nil {
		return err
	}
	items := make([]fixtureReviewItem, 0, len(opts.Results))
	for _, result := range opts.Results {
		items = append(items, fixtureReviewItem{
			Result:   result,
			Manifest: result.Fixture.Manifest,
			RelPath:  result.Fixture.RelPath,
			Path:     filepath.Join(result.Fixture.Dir, "fixture.json"),
		})
	}
	m := newFixtureReviewModel(ctx, rootAbs, items, opts.StartAt, opts.FilterStatus, opts.FilterAccuracy)
	if opts.AllowOpenViewer {
		srv, err := startFixtureReviewServer(rootAbs)
		if err != nil {
			return err
		}
		defer srv.Close()
		m.server = srv
	}
	_, err = tea.NewProgram(m, tea.WithOutput(out)).Run()
	return err
}

func runFixtureCandidateReviewTUI(ctx context.Context, out io.Writer, snapshot fixtureSnapshot, opts fixtureOptions) (string, fixtureOptions, error) {
	if !isTerminalWriter(out) {
		return "", opts, errFixtureReviewUnavailable
	}
	status := strings.TrimSpace(opts.ReviewStatus)
	if status == "" {
		status = "pending"
	}
	result := conformanceResult{
		Status:  "candidate",
		Current: snapshot,
		Fixture: conformanceFixture{
			RelPath:  fixtureName(opts.Name, snapshot.Name),
			Manifest: fixtureManifest{Name: snapshot.Name, Status: "approved", ReviewStatus: status, Accuracy: opts.Accuracy, ReviewComments: opts.ReviewComments},
		},
	}
	item := fixtureReviewItem{Result: result, Manifest: result.Fixture.Manifest, RelPath: result.Fixture.RelPath, Ephemeral: true}
	m := newFixtureReviewModel(ctx, "", []fixtureReviewItem{item}, "", "", "")
	final, err := tea.NewProgram(m, tea.WithOutput(out)).Run()
	if err != nil {
		return "", opts, err
	}
	reviewed, ok := final.(fixtureReviewModel)
	if !ok || len(reviewed.items) == 0 {
		return "skip", opts, nil
	}
	manifest := reviewed.items[0].Manifest
	opts.ReviewStatus = manifest.ReviewStatus
	opts.Accuracy = manifest.Accuracy
	opts.ReviewComments = manifest.ReviewComments
	opts.ReviewedAt = manifest.ReviewedAt
	switch manifest.ReviewStatus {
	case "skipped":
		return "skip", opts, nil
	case "reviewed":
		return "approved", opts, nil
	default:
		return "skip", opts, nil
	}
}

func newFixtureReviewModel(ctx context.Context, root string, items []fixtureReviewItem, startAt, filterStatus, filterAccuracy string) fixtureReviewModel {
	ta := textarea.New()
	ta.Placeholder = "Review comments..."
	ta.ShowLineNumbers = false
	ta.SetHeight(6)
	ta.SetWidth(80)

	filter := textinput.New()
	filter.Placeholder = "filter fixtures"
	filter.CharLimit = 120
	filter.Width = 48

	m := fixtureReviewModel{
		ctx:      ctx,
		root:     root,
		items:    items,
		focus:    reviewFocusNav,
		textarea: ta,
		filter:   filter,
		progress: progress.New(progress.WithDefaultGradient()),
	}
	if filterStatus != "" || filterAccuracy != "" {
		m.filter.SetValue(strings.TrimSpace(strings.Join([]string{filterStatus, filterAccuracy}, " ")))
	}
	m.selected = m.resumeIndex(startAt)
	m.loadSelectedComments()
	return m
}

func (m fixtureReviewModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m fixtureReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(maxInt(40, msg.Width-36))
		m.progress.Width = maxInt(16, msg.Width-42)
		return m, nil
	case tea.KeyMsg:
		if m.focus == reviewFocusComments {
			switch msg.String() {
			case "esc", "tab":
				m.persistComments()
				m.focus = reviewFocusNav
				m.textarea.Blur()
				return m, nil
			case "ctrl+c":
				m.persistComments()
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			m.persistComments()
			return m, cmd
		}
		if m.focus == reviewFocusFilter {
			switch msg.String() {
			case "esc", "enter", "tab":
				m.focus = reviewFocusNav
				m.filter.Blur()
				m.selected = m.clampToVisible(m.selected)
				m.loadSelectedComments()
				return m, nil
			case "ctrl+c":
				m.persistComments()
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.filter, cmd = m.filter.Update(msg)
			m.selected = m.clampToVisible(m.selected)
			m.loadSelectedComments()
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.persistComments()
			return m, tea.Quit
		case "tab":
			m.focus = reviewFocusComments
			m.textarea.Focus()
			return m, textarea.Blink
		case "/":
			m.focus = reviewFocusFilter
			m.filter.Focus()
			return m, textinput.Blink
		case "j", "down":
			m.move(1)
			return m, nil
		case "k", "up":
			m.move(-1)
			return m, nil
		case "r":
			m.mark("reviewed")
			return m, nil
		case "s":
			m.mark("skipped")
			return m, nil
		case "1":
			m.setAccuracy("accurate")
			return m, nil
		case "2":
			m.setAccuracy("partially_accurate")
			return m, nil
		case "3":
			m.setAccuracy("inaccurate")
			return m, nil
		case "4":
			m.setAccuracy("unsure")
			return m, nil
		case "o":
			m.openCurrent()
			return m, nil
		}
	}
	return m, nil
}

func (m fixtureReviewModel) View() string {
	if len(m.items) == 0 {
		return "No fixtures.\n"
	}
	navW := 32
	if m.width > 0 && m.width < 90 {
		navW = 24
	}
	mainW := maxInt(48, m.width-navW-4)
	left := navStyle.Width(navW).Render(m.navView(navW))
	right := mainStyle.Width(mainW).Render(m.detailView(mainW))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m *fixtureReviewModel) persistComments() {
	if len(m.items) == 0 || m.selected < 0 || m.selected >= len(m.items) {
		return
	}
	comments := reviewCommentsFromText(m.textarea.Value())
	m.items[m.selected].Manifest.ReviewComments = comments
	if m.items[m.selected].Ephemeral {
		return
	}
	if err := writeFixtureReviewManifest(m.items[m.selected]); err != nil {
		m.statusMsg = err.Error()
	}
}

func (m *fixtureReviewModel) loadSelectedComments() {
	if len(m.items) == 0 || m.selected < 0 || m.selected >= len(m.items) {
		m.textarea.SetValue("")
		return
	}
	m.textarea.SetValue(strings.Join(m.items[m.selected].Manifest.ReviewComments, "\n"))
}

func (m *fixtureReviewModel) move(delta int) {
	m.persistComments()
	visible := m.visibleIndexes()
	if len(visible) == 0 {
		return
	}
	pos := 0
	for i, idx := range visible {
		if idx == m.selected {
			pos = i
			break
		}
	}
	pos = (pos + delta + len(visible)) % len(visible)
	m.selected = visible[pos]
	m.loadSelectedComments()
}

func (m *fixtureReviewModel) mark(status string) {
	if len(m.items) == 0 {
		return
	}
	now := time.Now().UTC()
	m.items[m.selected].Manifest.ReviewStatus = status
	m.items[m.selected].Manifest.ReviewedAt = &now
	m.persistComments()
	m.statusMsg = fmt.Sprintf("%s marked %s", m.items[m.selected].RelPath, status)
}

func (m *fixtureReviewModel) setAccuracy(accuracy string) {
	if len(m.items) == 0 {
		return
	}
	m.items[m.selected].Manifest.Accuracy = accuracy
	m.persistComments()
	m.statusMsg = fmt.Sprintf("%s accuracy: %s", m.items[m.selected].RelPath, accuracy)
}

func (m *fixtureReviewModel) openCurrent() {
	if m.server == nil || len(m.items) == 0 {
		m.statusMsg = "viewer is unavailable for this review"
		return
	}
	rel := m.items[m.selected].RelPath
	u := m.server.URL + "/dev/fixtures/review?fixture=" + url.QueryEscape(rel)
	if err := cmdutil.OpenBrowser(u); err != nil {
		m.statusMsg = err.Error()
		return
	}
	m.statusMsg = "opened " + rel
}

func (m fixtureReviewModel) navView(width int) string {
	total, done, skipped := m.reviewTotals()
	pct := 0.0
	if total > 0 {
		pct = float64(done+skipped) / float64(total)
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Fixture Review") + "\n")
	b.WriteString(fmt.Sprintf("%d/%d complete, %d skipped\n", done+skipped, total, skipped))
	b.WriteString(m.progress.ViewAs(pct) + "\n")
	if m.filter.Value() != "" || m.focus == reviewFocusFilter {
		b.WriteString("Filter: " + m.filter.View() + "\n")
	} else {
		b.WriteString("Filter: / to search\n")
	}
	b.WriteString("\n")
	for _, idx := range m.visibleIndexes() {
		item := m.items[idx]
		cursor := " "
		if idx == m.selected {
			cursor = ">"
		}
		label := item.RelPath
		if len(label) > width-12 {
			label = "..." + label[len(label)-(width-15):]
		}
		b.WriteString(fmt.Sprintf("%s %-7s %-7s %s\n", cursor, reviewBadge(item.Manifest.ReviewStatus), item.Result.Status, label))
	}
	if len(m.visibleIndexes()) == 0 {
		b.WriteString("No fixtures match filter.\n")
	}
	b.WriteString("\nKeys: j/k move  r reviewed  s skip  1-4 accuracy  o open  tab comments  q quit")
	return b.String()
}

func (m fixtureReviewModel) detailView(width int) string {
	if len(m.items) == 0 || m.selected < 0 || m.selected >= len(m.items) {
		return ""
	}
	item := m.items[m.selected]
	manifest := item.Manifest
	var b strings.Builder
	b.WriteString(titleStyle.Render(item.RelPath) + "\n")
	b.WriteString(fmt.Sprintf("Status: %s  Conformance: %s  Accuracy: %s\n", reviewBadge(manifest.ReviewStatus), item.Result.Status, valueOr(manifest.Accuracy, "unset")))
	b.WriteString(fmt.Sprintf("Taxonomy: %s/%s/%s/%s\n", manifest.Language, manifest.Domain, manifest.Framework, manifest.Type))
	if manifest.ReviewedAt != nil {
		b.WriteString("Reviewed: " + manifest.ReviewedAt.Format(time.RFC3339) + "\n")
	}
	if item.Result.Error != "" {
		b.WriteString(errorStyle.Render("Error: "+item.Result.Error) + "\n")
	}
	snap := item.Result.Golden
	if snap.Name == "" {
		snap = item.Result.Current
	}
	b.WriteString(fmt.Sprintf("Golden: %d elements, %d connectors, %d views, %d facts, %d decisions\n", len(snap.Elements), len(snap.Connectors), len(snap.Views), len(snap.Facts), len(snap.Decisions)))
	if item.Result.Diff.Changed {
		b.WriteString(fmt.Sprintf("Delta: facts %+d, elements %+d\n", item.Result.Diff.FactDelta, item.Result.Diff.ElementDelta))
		b.WriteString(diffBlock("missing facts", item.Result.Diff.MissingFacts, width))
		b.WriteString(diffBlock("extra facts", item.Result.Diff.ExtraFacts, width))
		b.WriteString(diffBlock("changed facts", item.Result.Diff.ChangedFacts, width))
		b.WriteString(diffBlock("missing elements", item.Result.Diff.MissingElements, width))
		b.WriteString(diffBlock("extra elements", item.Result.Diff.ExtraElements, width))
		b.WriteString(diffBlock("changed decisions", item.Result.Diff.ChangedDecisions, width))
		b.WriteString(diffBlock("changed views", item.Result.Diff.ChangedViews, width))
		b.WriteString(diffBlock("changed connectors", item.Result.Diff.ChangedConnectors, width))
	}
	b.WriteString("\nComments")
	if m.focus == reviewFocusComments {
		b.WriteString(" (editing, esc/tab to leave)")
	}
	b.WriteString("\n")
	b.WriteString(m.textarea.View())
	if m.statusMsg != "" {
		b.WriteString("\n" + hintStyle.Render(m.statusMsg))
	}
	return b.String()
}

func (m fixtureReviewModel) visibleIndexes() []int {
	query := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	var out []int
	for i, item := range m.items {
		if query == "" || strings.Contains(strings.ToLower(strings.Join([]string{
			item.RelPath,
			item.Result.Status,
			reviewStatus(item.Manifest.ReviewStatus),
			item.Manifest.Accuracy,
			item.Manifest.Language,
			item.Manifest.Domain,
			item.Manifest.Framework,
			item.Manifest.Type,
		}, " ")), query) {
			out = append(out, i)
		}
	}
	return out
}

func (m fixtureReviewModel) clampToVisible(idx int) int {
	visible := m.visibleIndexes()
	if len(visible) == 0 {
		return idx
	}
	for _, visibleIdx := range visible {
		if visibleIdx == idx {
			return idx
		}
	}
	return visible[0]
}

func (m fixtureReviewModel) resumeIndex(startAt string) int {
	if startAt != "" {
		for i, item := range m.items {
			if item.RelPath == startAt {
				return i
			}
		}
	}
	for i, item := range m.items {
		if reviewStatus(item.Manifest.ReviewStatus) == "pending" {
			return i
		}
	}
	for i, item := range m.items {
		if item.Result.Status == "drift" && reviewStatus(item.Manifest.ReviewStatus) != "reviewed" {
			return i
		}
	}
	return 0
}

func (m fixtureReviewModel) reviewTotals() (total, done, skipped int) {
	for _, item := range m.items {
		total++
		switch reviewStatus(item.Manifest.ReviewStatus) {
		case "reviewed":
			done++
		case "skipped":
			skipped++
		}
	}
	return total, done, skipped
}

func writeFixtureReviewManifest(item fixtureReviewItem) error {
	if strings.TrimSpace(item.Path) == "" {
		return nil
	}
	var manifest fixtureManifest
	if err := readJSONFile(item.Path, &manifest); err != nil {
		return err
	}
	manifest.ReviewStatus = item.Manifest.ReviewStatus
	manifest.Accuracy = item.Manifest.Accuracy
	manifest.ReviewComments = sortedReviewComments(item.Manifest.ReviewComments)
	manifest.ReviewedAt = item.Manifest.ReviewedAt
	return writePrettyJSON(item.Path, manifest)
}

func startFixtureReviewServer(root string) (*reviewServer, error) {
	dataDir, err := os.MkdirTemp("", "tld-fixture-review-*")
	if err != nil {
		return nil, err
	}
	app, err := localserver.Bootstrap(dataDir, localserver.ServeOptions{Host: "127.0.0.1", Port: "0", DevFixturesDir: root})
	if err != nil {
		_ = os.RemoveAll(dataDir)
		return nil, err
	}
	ln, err := net.Listen("tcp", app.Addr)
	if err != nil {
		_ = os.RemoveAll(dataDir)
		return nil, err
	}
	srv := &http.Server{Handler: app.Handler}
	go func() { _ = srv.Serve(ln) }()
	return &reviewServer{URL: "http://" + ln.Addr().String(), server: srv, listener: ln, dataDir: dataDir}, nil
}

func reviewCommentsFromText(value string) []string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func sortedReviewComments(comments []string) []string {
	out := make([]string, 0, len(comments))
	for _, comment := range comments {
		comment = strings.TrimSpace(comment)
		if comment != "" {
			out = append(out, comment)
		}
	}
	return out
}

func reviewStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "reviewed", "skipped":
		return value
	default:
		return "pending"
	}
}

func reviewBadge(value string) string {
	return reviewStatus(value)
}

func diffBlock(label string, values []string, width int) string {
	if len(values) == 0 {
		return ""
	}
	limit := minInt(len(values), 6)
	var b strings.Builder
	b.WriteString(label + ":\n")
	for _, value := range values[:limit] {
		if width > 20 && len(value) > width-8 {
			value = value[:width-11] + "..."
		}
		b.WriteString("  - " + value + "\n")
	}
	if len(values) > limit {
		b.WriteString(fmt.Sprintf("  ... %d more\n", len(values)-limit))
	}
	return b.String()
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	navStyle   = lipgloss.NewStyle().Padding(1, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))
	mainStyle  = lipgloss.NewStyle().Padding(1, 1)
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
