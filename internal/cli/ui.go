package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/drewjocham/mongork/internal/migration"
	"github.com/drewjocham/mongork/internal/observability"
	"github.com/drewjocham/mongork/internal/schema/diff"
	"github.com/drewjocham/mongork/mcp"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	uiTickEvery        = 1 * time.Second
	lockStaleThreshold = 30 * time.Second
	maxStreamEvents    = 80
)

var (
	uiHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("63")).
			Padding(0, 1)
	uiMetaStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	uiPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
	uiFooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			BorderTop(true).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	uiErrorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	uiCardTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	uiCardValueStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230"))
)

func newUICmd() *cobra.Command {
	var resumeFile string
	var helpKeys bool
	cmd := &cobra.Command{
		Use:     "ui",
		Aliases: []string{"tui", "bubbletea"},
		Short:   "Launch interactive Bubble Tea operations console",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if helpKeys {
				fmt.Fprintln(cmd.OutOrStdout(), uiHotkeysHelpText())
				return nil
			}
			s, err := getServices(cmd.Context())
			if err != nil {
				return err
			}
			model := newUIModel(cmd.Context(), s, resumeFile)
			p := tea.NewProgram(model, tea.WithAltScreen())
			_, err = p.Run()
			return err
		},
	}
	cmd.Flags().StringVar(&resumeFile, "resume-file", ".mongork.resume", "Path to oplog resume token file")
	cmd.Flags().BoolVar(&helpKeys, "help-keys", false, "Print UI hotkeys and exit")
	return cmd
}

func (m uiModel) viewOps() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Sort: ops=%s • collections=%s\n\n", m.opsSortLabel(), m.collectionSortLabel())
	summary := lipgloss.JoinHorizontal(
		lipgloss.Top,
		renderCard("Connections", fmt.Sprintf("%d / %d", m.resource.ConnectionsCurrent, m.resource.ConnectionsAvailable)),
		" ",
		renderCard("Resident MB", fmt.Sprintf("%.1f", m.resource.ResidentMemoryMB)),
		" ",
		renderCard("Virtual MB", fmt.Sprintf("%.1f", m.resource.VirtualMemoryMB)),
		" ",
		renderCard("Active Ops", fmt.Sprintf("%d", len(m.currentOps))),
	)
	b.WriteString(summary)
	b.WriteString("\n\nCurrent Ops:\n")
	sortedOps := m.sortedCurrentOps()
	if len(sortedOps) == 0 {
		b.WriteString("  no active ops\n")
	} else {
		for _, op := range sortedOps {
			fmt.Fprintf(
				&b,
				"  [%ds] %s %s client=%s\n",
				op.RunningSecs,
				op.Operation,
				truncate(op.Namespace, 34),
				truncate(op.Client, 18),
			)
			if op.Description != "" {
				fmt.Fprintf(&b, "      %s\n", truncate(op.Description, 80))
			}
		}
	}

	b.WriteString("\nTop Collection Growth Deltas:\n")
	growthPairs := m.sortedGrowthSignals(8)
	if len(growthPairs) == 0 {
		b.WriteString("  no growth deltas yet (waiting for next refresh)\n")
	} else {
		for _, g := range growthPairs {
			sign := "+"
			if g.Delta < 0 {
				sign = ""
			}
			fmt.Fprintf(&b, "  %-28s %s%d docs\n", truncate(g.Collection, 28), sign, g.Delta)
		}
	}

	b.WriteString("\nTop Collection Counts:\n")
	if len(m.collStats) == 0 {
		b.WriteString("  no collection stats available\n")
		return b.String()
	}
	stats := m.sortedCollectionStats()
	limit := 8
	if len(stats) < limit {
		limit = len(stats)
	}
	for i := 0; i < limit; i++ {
		s := stats[i]
		fmt.Fprintf(&b, "  %-28s count=%d size=%dB idx=%dB\n", truncate(s.Collection, 28), s.Count, s.SizeBytes, s.IndexBytes)
	}
	return b.String()
}
func (m uiModel) renderHeader() string {
	title := uiHeaderStyle.Render("mongork • operations console")
	subtitle := uiMetaStyle.Render(
		fmt.Sprintf(
			"tab: %s • %s",
			m.tabs[m.activeTab],
			time.Now().Format("2006-01-02 15:04:05"),
		),
	)
	tabs := renderTabs(m.tabs, m.activeTab)
	return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, tabs)
}

func (m uiModel) renderFooter() string {
	parts := []string{"TAB/SHIFT+TAB or h/l or 1-5", "q quit", "? hotkeys", "R refresh"}
	switch m.activeTab {
	case 0:
		parts = append(parts, "↑/↓ select", "r rollback", "y/n confirm")
	case 1:
		parts = append(parts, "↑/↓ select", "p pause", "i/u/d filter", "enter inspector")
	case 3:
		parts = append(parts, "K set kill switch")
	case 4:
		parts = append(parts, "ops focus view", "o cycle op sort", "c cycle collection sort")
	}
	if m.actionNote != "" {
		parts = append(parts, m.actionNote)
	}
	if m.err != nil {
		parts = append(parts, uiErrorStyle.Render("error: "+m.err.Error()))
	}
	return uiFooterStyle.Render(strings.Join(parts, " • "))
}

func renderCard(title string, value string) string {
	return uiPanelStyle.
		BorderForeground(lipgloss.Color("240")).
		Render(uiCardTitleStyle.Render(title) + "\n" + uiCardValueStyle.Render(value))
}

func uiHotkeysHelpText() string {
	return strings.Join([]string{
		"Hotkeys",
		"  Global: q quit • ? toggle help • R refresh • TAB/SHIFT+TAB switch tabs • h/l switch tabs • 1..5 jump tab",
		"  List nav: ↑/↓ or j/k move • g/G jump top/bottom",
		"  Migrations: r rollback selected applied migration • y confirm • n/esc cancel",
		"  Live Stream: p pause/resume • i/u/d toggle op filters • enter toggle inspector",
		"  Ops: operator telemetry dashboard (resource, active ops, growth deltas)",
		"       o cycle op sort • c cycle collection sort",
		"  Playbook: K set kill switch (stop signal)",
	}, "\n")
}

type uiTickMsg time.Time

type uiRefreshMsg struct {
	status      []migration.MigrationStatus
	applied     []migration.MigrationRecord
	drifts      []migration.ChecksumDrift
	lock        migration.LockInfo
	dryRun      string
	stream      []oplogEntry
	mcpEvents   []mcp.Activity
	schemaDiffs []diff.Diff
	playbook    playbookState
	resource    observability.ResourceSummary
	currentOps  []observability.CurrentOpInfo
	collStats   []observability.CollectionStats
	resumeToken string
	resumeAt    time.Time
	err         error
}

type uiRollbackResultMsg struct {
	target string
	err    error
}

type playbookState struct {
	Key       string
	UpdatedAt time.Time
	LastID    string
	Done      int64
	Total     int64
	Stopped   bool
}

type uiModel struct {
	ctx        context.Context
	services   *Services
	resumeFile string

	tabs      []string
	activeTab int
	width     int
	height    int

	status     []migration.MigrationStatus
	applied    []migration.MigrationRecord
	driftByVer map[string]bool
	lock       migration.LockInfo
	dryRun     string
	actionNote string

	streamEvents  []oplogEntry
	selectedIdx   int
	selectedMig   int
	paused        bool
	inspectorOpen bool
	opFilters     map[string]bool
	lastSeenTS    time.Time
	epsRing       []int
	epsPos        int
	lastEpsTick   time.Time

	mcpEvents   []mcp.Activity
	schemaDiffs []diff.Diff
	playbook    playbookState
	resource    observability.ResourceSummary
	currentOps  []observability.CurrentOpInfo
	collStats   []observability.CollectionStats

	resumeToken string
	resumeAt    time.Time
	showHelp    bool
	err         error

	lastCollectionCounts map[string]int64
	collectionGrowth     map[string]int64
	opsSortMode          int
	collectionSortMode   int
}

func newUIModel(ctx context.Context, s *Services, resumeFile string) uiModel {
	return uiModel{
		ctx:                  ctx,
		services:             s,
		resumeFile:           resumeFile,
		tabs:                 []string{"Migrations", "Live Stream", "MCP Activity", "Playbook", "Ops"},
		driftByVer:           map[string]bool{},
		epsRing:              make([]int, 30),
		opFilters:            map[string]bool{"i": true, "u": true, "d": true},
		lastEpsTick:          time.Now(),
		lastCollectionCounts: map[string]int64{},
		collectionGrowth:     map[string]int64{},
		opsSortMode:          0,
		collectionSortMode:   0,
	}
}

func (m uiModel) Init() tea.Cmd {
	return tea.Batch(m.tickCmd(), m.refreshCmd())
}

func (m uiModel) tickCmd() tea.Cmd {
	return tea.Tick(uiTickEvery, func(t time.Time) tea.Msg { return uiTickMsg(t) })
}

func (m uiModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		msg := uiRefreshMsg{}
		engine := m.services.Engine
		client := m.services.MongoClient
		db := client.Database(m.services.Config.Mongo.Database)

		status, err := engine.GetStatus(m.ctx)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.status = status

		applied, err := engine.ListApplied(m.ctx)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.applied = applied

		drifts, err := engine.ChecksumDrifts(m.ctx)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.drifts = drifts

		lock, err := engine.GetLockInfo(m.ctx)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.lock = lock

		plan, err := engine.Plan(m.ctx, migration.DirectionUp, "")
		if err == nil {
			live, e1 := diff.InspectLive(m.ctx, db)
			target := diff.FromRegistry()
			if e1 == nil {
				diffs := diff.Compare(live, target)
				msg.dryRun = summarizeDryRun(plan, diffs)
				msg.schemaDiffs = diffs
			} else {
				msg.dryRun = fmt.Sprintf("Pending migrations: %d (schema diff unavailable: %v)", len(plan), e1)
			}
		}

		streamCfg := oplogConfig{limit: 25}
		if !m.lastSeenTS.IsZero() {
			streamCfg.from = m.lastSeenTS.UTC().Format(time.RFC3339)
		}
		filter, err := buildFilter(streamCfg)
		if err == nil {
			streamEntries, e2 := fetchOplog(m.ctx, client, filter, streamCfg.limit)
			if e2 == nil {
				sort.Slice(streamEntries, func(i, j int) bool {
					return streamEntries[i].TS.T < streamEntries[j].TS.T
				})
				msg.stream = streamEntries
			}
		}

		msg.mcpEvents = mcp.RecentActivity(80)
		msg.playbook = readPlaybookState(m.ctx, db)
		if resource, e3 := observability.BuildResourceSummary(m.ctx, client); e3 == nil {
			msg.resource = resource
		}
		if currentOps, e4 := observability.CurrentOperations(m.ctx, client, 8); e4 == nil {
			msg.currentOps = currentOps
		}
		if collStats, e5 := observability.CollectionStatistics(m.ctx, db, ""); e5 == nil {
			msg.collStats = collStats
		}
		msg.resumeToken, msg.resumeAt = readResumeToken(m.resumeFile)
		return msg
	}
}

func (m uiModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case uiTickMsg:
		return m, tea.Batch(m.tickCmd(), m.refreshCmd())
	case uiRefreshMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.status = msg.status
		m.applied = msg.applied
		m.lock = msg.lock
		m.dryRun = msg.dryRun
		m.resumeToken = msg.resumeToken
		m.resumeAt = msg.resumeAt
		m.mcpEvents = msg.mcpEvents
		m.schemaDiffs = msg.schemaDiffs
		m.playbook = msg.playbook
		m.resource = msg.resource
		m.currentOps = msg.currentOps
		m.collStats = msg.collStats
		if len(msg.collStats) > 0 {
			growth := map[string]int64{}
			next := map[string]int64{}
			for _, s := range msg.collStats {
				next[s.Collection] = s.Count
				prev, ok := m.lastCollectionCounts[s.Collection]
				if ok {
					growth[s.Collection] = s.Count - prev
				}
			}
			m.collectionGrowth = growth
			m.lastCollectionCounts = next
		}
		m.driftByVer = map[string]bool{}
		for _, drift := range msg.drifts {
			m.driftByVer[drift.Version] = drift.DriftDetected
		}
		if !m.paused {
			before := len(m.streamEvents)
			for _, event := range msg.stream {
				m.streamEvents = append(m.streamEvents, event)
				if eventTime := eventTime(event); eventTime.After(m.lastSeenTS) {
					m.lastSeenTS = eventTime
				}
			}
			if len(m.streamEvents) > maxStreamEvents {
				m.streamEvents = m.streamEvents[len(m.streamEvents)-maxStreamEvents:]
			}
			added := len(m.streamEvents) - before
			if added > 0 {
				m.bumpEPS(added)
			}
		}
		filtered := m.filteredStreamEvents()
		if m.selectedIdx >= len(filtered) {
			m.selectedIdx = max(0, len(filtered)-1)
		}
		if m.selectedMig >= len(m.status) {
			m.selectedMig = max(0, len(m.status)-1)
		}
		return m, nil
	case uiRollbackResultMsg:
		if msg.err != nil {
			m.err = msg.err
			m.actionNote = ""
		} else {
			m.err = nil
			m.actionNote = fmt.Sprintf("Rollback complete (target kept: %s)", msg.target)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "1", "2", "3", "4", "5":
			m.activeTab = int(msg.String()[0] - '1')
			if m.activeTab >= len(m.tabs) {
				m.activeTab = len(m.tabs) - 1
			}
			return m, nil
		case "h":
			m.activeTab--
			if m.activeTab < 0 {
				m.activeTab = len(m.tabs) - 1
			}
			return m, nil
		case "l":
			m.activeTab = (m.activeTab + 1) % len(m.tabs)
			return m, nil
		case "R":
			return m, m.refreshCmd()
		}
		if m.showHelp && msg.String() != "q" && msg.String() != "ctrl+c" && msg.String() != "?" {
			return m, nil
		}
		if m.activeTab == 0 && m.rollbackConfirming() {
			switch msg.String() {
			case "y", "Y":
				target := m.rollbackTarget()
				if target == "" {
					m.actionNote = ""
					m.err = fmt.Errorf("selected migration is not applied")
					return m, nil
				}
				m.actionNote = "Running rollback..."
				return m, tea.Batch(m.rollbackCmd(target), m.refreshCmd())
			case "n", "N", "esc":
				m.actionNote = "Rollback cancelled."
				m.clearRollbackPrompt()
				return m, nil
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.activeTab = (m.activeTab + 1) % len(m.tabs)
		case "shift+tab":
			m.activeTab--
			if m.activeTab < 0 {
				m.activeTab = len(m.tabs) - 1
			}
		case "up", "k":
			if m.activeTab == 1 && m.selectedIdx > 0 {
				m.selectedIdx--
			}
			if m.activeTab == 0 && m.selectedMig > 0 {
				m.selectedMig--
			}
		case "down", "j":
			if m.activeTab == 1 && m.selectedIdx < len(m.filteredStreamEvents())-1 {
				m.selectedIdx++
			}
			if m.activeTab == 0 && m.selectedMig < len(m.status)-1 {
				m.selectedMig++
			}
		case "g", "home":
			if m.activeTab == 1 {
				m.selectedIdx = 0
			}
			if m.activeTab == 0 {
				m.selectedMig = 0
			}
		case "G", "end":
			if m.activeTab == 1 {
				m.selectedIdx = max(0, len(m.filteredStreamEvents())-1)
			}
			if m.activeTab == 0 {
				m.selectedMig = max(0, len(m.status)-1)
			}
		case "p":
			if m.activeTab == 1 {
				m.paused = !m.paused
			}
		case "i":
			if m.activeTab == 1 {
				m.toggleOpFilter("i")
			}
		case "u":
			if m.activeTab == 1 {
				m.toggleOpFilter("u")
			}
		case "d":
			if m.activeTab == 1 {
				m.toggleOpFilter("d")
			}
		case "enter":
			if m.activeTab == 1 {
				m.inspectorOpen = !m.inspectorOpen
			}
		case "r":
			if m.activeTab == 0 {
				target := m.rollbackTarget()
				if target == "" {
					m.actionNote = ""
					m.err = fmt.Errorf("select an applied migration to roll back")
					return m, nil
				}
				m.actionNote = fmt.Sprintf("Confirm rollback to %s? [y/N]", target)
			}
		case "K":
			if m.activeTab == 3 {
				_ = setStopSignal(m.ctx, m.services.MongoClient.Database(m.services.Config.Mongo.Database), true)
			}
		case "o", "O":
			if m.activeTab == 4 {
				m.cycleOpSort()
			}
		case "c", "C":
			if m.activeTab == 4 {
				m.cycleCollectionSort()
			}
		}
	}
	return m, nil
}

func (m uiModel) View() string {
	header := m.renderHeader()
	body := ""
	switch m.activeTab {
	case 0:
		body = m.viewMigrations()
	case 1:
		body = m.viewLiveStream()
	case 2:
		body = m.viewMCP()
	case 3:
		body = m.viewPlaybook()
	case 4:
		body = m.viewOps()
	}
	content := uiPanelStyle.Render(body)
	footer := m.renderFooter()
	out := lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
	if m.showHelp {
		help := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1).
			Render(m.hotkeysHelp())
		out = lipgloss.JoinVertical(lipgloss.Left, out, help)
	}
	return out
}

func (m *uiModel) cycleOpSort() {
	m.opsSortMode = (m.opsSortMode + 1) % 3
}

func (m *uiModel) cycleCollectionSort() {
	m.collectionSortMode = (m.collectionSortMode + 1) % 3
}

func (m uiModel) opsSortLabel() string {
	switch m.opsSortMode {
	case 1:
		return "namespace"
	case 2:
		return "runtime asc"
	default:
		return "runtime desc"
	}
}

func (m uiModel) collectionSortLabel() string {
	switch m.collectionSortMode {
	case 1:
		return "size bytes"
	case 2:
		return "index bytes"
	default:
		return "count"
	}
}

func (m uiModel) sortedCurrentOps() []observability.CurrentOpInfo {
	ops := append([]observability.CurrentOpInfo(nil), m.currentOps...)
	sort.Slice(ops, func(i, j int) bool {
		switch m.opsSortMode {
		case 1:
			if ops[i].Namespace == ops[j].Namespace {
				return ops[i].RunningSecs > ops[j].RunningSecs
			}
			return ops[i].Namespace < ops[j].Namespace
		case 2:
			return ops[i].RunningSecs < ops[j].RunningSecs
		default:
			return ops[i].RunningSecs > ops[j].RunningSecs
		}
	})
	return ops
}

func (m uiModel) sortedCollectionStats() []observability.CollectionStats {
	stats := append([]observability.CollectionStats(nil), m.collStats...)
	sort.Slice(stats, func(i, j int) bool {
		switch m.collectionSortMode {
		case 1:
			return stats[i].SizeBytes > stats[j].SizeBytes
		case 2:
			return stats[i].IndexBytes > stats[j].IndexBytes
		default:
			return stats[i].Count > stats[j].Count
		}
	})
	return stats
}

func (m uiModel) viewMigrations() string {
	var b strings.Builder
	appliedCount := 0
	pendingCount := 0
	driftCount := 0
	for _, st := range m.status {
		if st.Applied {
			appliedCount++
		} else {
			pendingCount++
		}
		if m.driftByVer[st.Version] {
			driftCount++
		}
	}
	summaryRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		renderCard("Applied", fmt.Sprintf("%d", appliedCount)),
		" ",
		renderCard("Pending", fmt.Sprintf("%d", pendingCount)),
		" ",
		renderCard("Checksum Drift", fmt.Sprintf("%d", driftCount)),
	)
	b.WriteString(summaryRow)
	b.WriteString("\n")
	b.WriteString(renderLockStatus(m.lock))
	b.WriteString("\n")
	b.WriteString("Dry-Run Preview: ")
	b.WriteString(m.dryRun)
	b.WriteString("\n\nTimeline:\n")

	appliedAt := map[string]time.Time{}
	for _, rec := range m.applied {
		appliedAt[rec.Version] = rec.AppliedAt
	}
	headVersion := ""
	for _, st := range m.status {
		if st.Applied {
			headVersion = st.Version
		}
	}
	for _, st := range m.status {
		badge := "[PENDING]"
		if st.Applied {
			badge = "[APPLIED]"
		}
		if m.driftByVer[st.Version] {
			badge += " ⚠ DRIFT"
		}
		if st.Version == headVersion {
			badge += " [HEAD]"
		}
		at := "-"
		if t, ok := appliedAt[st.Version]; ok {
			at = t.Format(time.RFC3339)
		}
		cursor := " "
		if m.selectedMig < len(m.status) && m.status[m.selectedMig].Version == st.Version {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s %s %s (%s)\n", cursor, badge, st.Version, at)
		fmt.Fprintf(&b, "│   %s\n", st.Description)
	}
	if len(m.status) > 0 && m.selectedMig < len(m.status) {
		selected := m.status[m.selectedMig]
		b.WriteString("\nSelected:\n")
		fmt.Fprintf(&b, "version: %s\n", selected.Version)
		fmt.Fprintf(&b, "state: %s\n", map[bool]string{true: "applied", false: "pending"}[selected.Applied])
		fmt.Fprintf(&b, "description: %s\n", selected.Description)
		if selected.Applied {
			b.WriteString("action: press 'r' then 'y' to roll back to this version\n")
		}
	}
	if m.rollbackConfirming() {
		b.WriteString("\n⚠ Rollback confirmation pending: press 'y' to continue or 'n' to cancel.\n")
	}
	return b.String()
}

func (m uiModel) viewLiveStream() string {
	var b strings.Builder
	pauseState := "running"
	if m.paused {
		pauseState = "paused"
	}
	lag := "-"
	if !m.lastSeenTS.IsZero() {
		lag = time.Since(m.lastSeenTS).Truncate(time.Second).String()
	}
	fmt.Fprintf(&b, "Live Stream (%s) • EPS %s • Lag %s\n", pauseState, renderSparkline(m.epsRing, m.epsPos), lag)
	fmt.Fprintf(&b, "Filters: %s %s %s\n", m.renderFilterBadge("i"), m.renderFilterBadge("u"), m.renderFilterBadge("d"))
	if m.resumeToken != "" {
		fmt.Fprintf(&b, "Resume token (%s): %s\n", m.resumeAt.Format(time.RFC3339), truncate(m.resumeToken, 80))
	}
	filtered := m.filteredStreamEvents()
	var inserts, updates, deletes int
	for _, ev := range filtered {
		switch ev.Op {
		case "i":
			inserts++
		case "u":
			updates++
		case "d":
			deletes++
		}
	}
	counters := lipgloss.JoinHorizontal(
		lipgloss.Top,
		renderCard("Events", fmt.Sprintf("%d", len(filtered))),
		" ",
		renderCard("Insert/Update/Delete", fmt.Sprintf("%d / %d / %d", inserts, updates, deletes)),
		" ",
		renderCard("Connections", fmt.Sprintf("%d / %d", m.resource.ConnectionsCurrent, m.resource.ConnectionsAvailable)),
		" ",
		renderCard("Resident MB", fmt.Sprintf("%.1f", m.resource.ResidentMemoryMB)),
	)
	b.WriteString(counters)
	b.WriteString("\n")
	b.WriteString("\nTop Growth Signals:\n")
	growthPairs := m.sortedGrowthSignals(5)
	if len(growthPairs) == 0 {
		b.WriteString("  no growth deltas yet (waiting for next refresh)\n")
	} else {
		for _, g := range growthPairs {
			sign := "+"
			if g.Delta < 0 {
				sign = ""
			}
			fmt.Fprintf(&b, "  %s %s%d docs\n", g.Collection, sign, g.Delta)
		}
	}

	b.WriteString("\nCurrent Ops Snapshot:\n")
	if len(m.currentOps) == 0 {
		b.WriteString("  no active ops\n")
	} else {
		for _, op := range m.currentOps {
			fmt.Fprintf(
				&b,
				"  [%ss] %s %s (%s)\n",
				fmt.Sprintf("%d", op.RunningSecs),
				op.Operation,
				truncate(op.Namespace, 28),
				truncate(op.Description, 34),
			)
		}
	}
	b.WriteString("\nEvents:\n")
	for i, ev := range filtered {
		cursor := " "
		if i == m.selectedIdx {
			cursor = ">"
		}
		out := ev.ToOutput()
		fmt.Fprintf(&b, "%s %s %s %s\n", cursor, out.Timestamp.Format("15:04:05"), out.Operation, out.Namespace)
	}
	if len(filtered) > 0 && m.selectedIdx < len(filtered) && m.inspectorOpen {
		selected := filtered[m.selectedIdx]
		raw, _ := bson.MarshalExtJSON(selected.O, true, true)
		key, _ := bson.MarshalExtJSON(selected.O2, true, true)
		modal := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			BorderForeground(lipgloss.Color("62")).
			Render(fmt.Sprintf("Inspector\nop: %s\nns: %s\no: %s\no2: %s", selected.Op, selected.NS, string(raw), string(key)))
		fmt.Fprintf(&b, "\n%s\n", modal)
	} else if len(filtered) > 0 && m.selectedIdx < len(filtered) {
		b.WriteString("\nPress Enter to open inspector modal.\n")
	}
	return b.String()
}

func (m uiModel) viewMCP() string {
	var b strings.Builder
	var okCount, errCount int
	for _, e := range m.mcpEvents {
		if e.Success {
			okCount++
		} else {
			errCount++
		}
	}
	row := lipgloss.JoinHorizontal(
		lipgloss.Top,
		renderCard("MCP Calls", fmt.Sprintf("%d", len(m.mcpEvents))),
		" ",
		renderCard("Success/Failure", fmt.Sprintf("%d / %d", okCount, errCount)),
		" ",
		renderCard("Schema Drift Entries", fmt.Sprintf("%d", len(m.schemaDiffs))),
	)
	b.WriteString(row)
	b.WriteString("\nAI Activity Log\n")
	for _, e := range m.mcpEvents {
		status := "✅"
		if !e.Success {
			status = "❌"
		}
		fmt.Fprintf(&b, "[%s] %s %s %s %s\n", e.Timestamp.Format("15:04:05"), status, e.Actor, e.Tool, e.Detail)
		if e.Error != "" {
			fmt.Fprintf(&b, "  error: %s\n", e.Error)
		}
	}
	b.WriteString("\nSchema Diff (Registered vs Live)\n")
	if len(m.schemaDiffs) == 0 {
		b.WriteString("No schema drift detected.\n")
		return b.String()
	}
	for _, d := range m.schemaDiffs {
		prefix := "+"
		if strings.HasPrefix(d.Action, "Drop") {
			prefix = "-"
		}
		fmt.Fprintf(
			&b,
			"%s %-14s %-36s current=%s proposed=%s\n",
			prefix,
			d.Action,
			d.Target,
			truncate(d.Current, 28),
			truncate(d.Proposed, 28),
		)
	}
	return b.String()
}

func (m uiModel) viewPlaybook() string {
	var b strings.Builder
	b.WriteString("Zero-Downtime Playbook\n")
	if m.playbook.Key == "" {
		b.WriteString("No checkpoint data found in migration_progress.\n")
	} else {
		fmt.Fprintf(
			&b,
			"Checkpoint: %s\nLast ID: %s\nUpdated: %s\n",
			m.playbook.Key,
			m.playbook.LastID,
			m.playbook.UpdatedAt.Format(time.RFC3339),
		)
		if m.playbook.Total > 0 {
			p := float64(m.playbook.Done) / float64(m.playbook.Total)
			fmt.Fprintf(&b, "Progress: %s %d/%d\n", renderProgressBar(p, 24), m.playbook.Done, m.playbook.Total)
		}
	}
	if m.playbook.Stopped {
		b.WriteString("Safety switch: STOP requested\n")
	} else {
		b.WriteString("Safety switch: clear\n")
	}
	b.WriteString("Press K to set Kill Script stop signal.\n")
	return b.String()
}

func renderTabs(tabs []string, active int) string {
	items := make([]string, 0, len(tabs))
	for i, tab := range tabs {
		label := fmt.Sprintf("%d. %s", i+1, tab)
		style := lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("245")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
		if i == active {
			style = style.
				Bold(true).
				Foreground(lipgloss.Color("229")).
				Background(lipgloss.Color("62")).
				BorderForeground(lipgloss.Color("62"))
		}
		items = append(items, style.Render(label))
	}
	return strings.Join(items, " ")
}

func renderLockStatus(lock migration.LockInfo) string {
	if !lock.Active {
		return "🟢 LOCK FREE"
	}
	age := time.Since(lock.AcquiredAt)
	if age > lockStaleThreshold {
		return fmt.Sprintf("🟡 LOCK STALE host=%s pid=%d age=%s", lock.Host, lock.PID, age.Truncate(time.Second))
	}
	return fmt.Sprintf("🔴 LOCK HELD host=%s pid=%d age=%s", lock.Host, lock.PID, age.Truncate(time.Second))
}

func summarizeDryRun(plan []string, diffs []diff.Diff) string {
	if len(plan) == 0 && len(diffs) == 0 {
		return "No pending changes."
	}
	var addIdx, addVal int
	for _, d := range diffs {
		if d.Action == "AddIndex" {
			addIdx++
		}
		if d.Action == "AddValidator" {
			addVal++
		}
	}
	return fmt.Sprintf("Will run %d migration(s), add %d index(es), add %d validator rule(s).", len(plan), addIdx, addVal)
}

func readResumeToken(path string) (string, time.Time) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}
	}
	stat, _ := os.Stat(path)
	mod := time.Time{}
	if stat != nil {
		mod = stat.ModTime()
	}
	return string(data), mod
}

func readPlaybookState(ctx context.Context, db *mongo.Database) playbookState {
	state := playbookState{}
	var progressDoc bson.M
	err := db.Collection("migration_progress").FindOne(ctx, bson.M{}).Decode(&progressDoc)
	if err == nil {
		state.Key, _ = progressDoc["_id"].(string)
		if lastID, ok := progressDoc["last_id"]; ok {
			state.LastID = fmt.Sprintf("%v", lastID)
		}
		if updated, ok := progressDoc["updated"].(bson.DateTime); ok {
			state.UpdatedAt = updated.Time()
		}
		if oid, ok := progressDoc["last_id"].(bson.ObjectID); ok && oid != bson.NilObjectID {
			done, _ := db.Collection("customers").CountDocuments(ctx, bson.M{"_id": bson.M{"$lte": oid}})
			total, _ := db.Collection("customers").CountDocuments(ctx, bson.M{})
			state.Done = done
			state.Total = total
		}
	}
	var controlDoc struct {
		Stop bool `bson:"stop"`
	}
	if err := db.Collection("migration_control").FindOne(ctx, bson.M{"_id": "global"}).Decode(&controlDoc); err == nil {
		state.Stopped = controlDoc.Stop
	}
	return state
}

func setStopSignal(ctx context.Context, db *mongo.Database, stop bool) error {
	_, err := db.Collection("migration_control").UpdateOne(
		ctx,
		bson.M{"_id": "global"},
		bson.M{"$set": bson.M{"stop": stop, "updated_at": time.Now().UTC()}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

func renderSparkline(values []int, pos int) string {
	if len(values) == 0 {
		return ""
	}
	chars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	maxVal := 1
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	var b strings.Builder
	for i := 0; i < len(values); i++ {
		v := values[(pos+i)%len(values)]
		idx := (v * (len(chars) - 1)) / maxVal
		if idx < 0 {
			idx = 0
		}
		if idx >= len(chars) {
			idx = len(chars) - 1
		}
		b.WriteString(chars[idx])
	}
	return b.String()
}

func renderProgressBar(p float64, width int) string {
	if p < 0 {
		p = 0
	}
	if p > 1 {
		p = 1
	}
	filled := int(p * float64(width))
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

func truncate(v string, maxLen int) string {
	if len(v) <= maxLen {
		return v
	}
	return v[:maxLen] + "…"
}

func eventTime(e oplogEntry) time.Time {
	if e.Wall != nil {
		return *e.Wall
	}
	return time.Unix(int64(e.TS.T), 0)
}

func (m *uiModel) bumpEPS(added int) {
	now := time.Now()
	if now.Sub(m.lastEpsTick) >= time.Second {
		m.epsPos = (m.epsPos + 1) % len(m.epsRing)
		m.epsRing[m.epsPos] = added
		m.lastEpsTick = now
		return
	}
	m.epsRing[m.epsPos] += added
}

func (m uiModel) rollbackTarget() string {
	if len(m.status) == 0 || m.selectedMig >= len(m.status) {
		return ""
	}
	target := m.status[m.selectedMig]
	if !target.Applied {
		return ""
	}
	return target.Version
}

func (m uiModel) rollbackConfirming() bool {
	return strings.HasPrefix(m.actionNote, "Confirm rollback to ")
}

func (m *uiModel) clearRollbackPrompt() {
	if m.rollbackConfirming() {
		m.actionNote = ""
	}
}

func (m uiModel) rollbackCmd(target string) tea.Cmd {
	return func() tea.Msg {
		err := m.services.Engine.Down(m.ctx, target)
		return uiRollbackResultMsg{target: target, err: err}
	}
}

func (m uiModel) filteredStreamEvents() []oplogEntry {
	if len(m.streamEvents) == 0 {
		return nil
	}
	filtered := make([]oplogEntry, 0, len(m.streamEvents))
	for _, ev := range m.streamEvents {
		if m.opFilters[ev.Op] {
			filtered = append(filtered, ev)
		}
	}
	return filtered
}

func (m *uiModel) toggleOpFilter(op string) {
	if m.opFilters == nil {
		m.opFilters = map[string]bool{"i": true, "u": true, "d": true}
	}
	m.opFilters[op] = !m.opFilters[op]
	filtered := m.filteredStreamEvents()
	if m.selectedIdx >= len(filtered) {
		m.selectedIdx = max(0, len(filtered)-1)
	}
}

func (m uiModel) renderFilterBadge(op string) string {
	label := map[string]string{"i": "insert", "u": "update", "d": "delete"}[op]
	if label == "" {
		label = op
	}
	if m.opFilters[op] {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("[" + label + "]")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[" + label + "]")
}

func (m uiModel) hotkeysHelp() string {
	return uiHotkeysHelpText()
}

type growthSignal struct {
	Collection string
	Delta      int64
}

func (m uiModel) sortedGrowthSignals(limit int) []growthSignal {
	if limit <= 0 {
		limit = 5
	}
	out := make([]growthSignal, 0, len(m.collectionGrowth))
	for name, delta := range m.collectionGrowth {
		out = append(out, growthSignal{Collection: name, Delta: delta})
	}
	sort.Slice(out, func(i, j int) bool {
		ai := out[i].Delta
		if ai < 0 {
			ai = -ai
		}
		aj := out[j].Delta
		if aj < 0 {
			aj = -aj
		}
		if ai == aj {
			return out[i].Collection < out[j].Collection
		}
		return ai > aj
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
