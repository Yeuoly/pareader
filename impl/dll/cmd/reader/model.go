package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Yeuoly/pareader/impl/dll/internal/aimeio"
	pareaderhid "github.com/Yeuoly/pareader/impl/dll/internal/hid"
	"github.com/Yeuoly/pareader/impl/dll/internal/protocol"
)

type refreshNowMsg struct{}

type refreshResultMsg struct {
	card      protocol.Card
	device    pareaderhid.DeviceInfo
	connected bool
	err       error
	at        time.Time
}

type model struct {
	service  *aimeio.Service
	device   pareaderhid.DeviceInfo
	version  protocol.Version
	interval time.Duration
	width    int
	height   int
	card     protocol.Card
	lastID   string
	lastSeen time.Time
	err      error
}

var (
	colorPurple = lipgloss.Color("#A78BFA")
	colorCyan   = lipgloss.Color("#67E8F9")
	colorGreen  = lipgloss.Color("#6EE7B7")
	colorMuted  = lipgloss.Color("#64748B")
	colorText   = lipgloss.Color("#E2E8F0")
	colorPanel  = lipgloss.Color("#1E293B")
	colorError  = lipgloss.Color("#FB7185")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPurple)
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPurple).
			Background(colorPanel).
			Padding(1, 3)
	labelStyle = lipgloss.NewStyle().Foreground(colorMuted)
	idStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	helpStyle  = lipgloss.NewStyle().Foreground(colorMuted)
)

func newModel(
	service *aimeio.Service,
	device pareaderhid.DeviceInfo,
	version protocol.Version,
	interval time.Duration,
) model {
	return model{
		service:  service,
		device:   device,
		version:  version,
		interval: interval,
		card:     protocol.Card{Type: protocol.CardNone},
	}
}

func (m model) Init() tea.Cmd {
	return refreshCardState(m.service)
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height

	case refreshNowMsg:
		return m, refreshCardState(m.service)

	case refreshResultMsg:
		m.err = message.err
		if message.connected {
			m.device = message.device
		}
		if message.err == nil {
			m.card = message.card
			if identifier := cardIdentifier(message.card); identifier != "" {
				m.lastID = identifier
				m.lastSeen = message.at
			}
		}
		return m, tea.Tick(m.interval, func(time.Time) tea.Msg { return refreshNowMsg{} })
	}

	return m, nil
}

func (m model) View() string {
	product := strings.TrimSpace(m.device.Product)
	if product == "" {
		product = "PA Reader"
	}

	status, statusStyle := "等待卡片", lipgloss.NewStyle().Foreground(colorGreen)
	typeName := "—"
	identifier := "请将卡片放到读卡区域"

	if m.err != nil {
		status = "读取异常"
		statusStyle = lipgloss.NewStyle().Foreground(colorError)
		identifier = m.err.Error()
	} else {
		switch m.card.Type {
		case protocol.CardMIFARE:
			status = "已读取"
			typeName = "MIFARE / Aime"
			identifier = cardIdentifier(m.card)
		case protocol.CardFeliCa:
			status = "已读取"
			typeName = "FeliCa"
			identifier = protocol.FormatIDm(m.card.IDm)
		}
	}

	header := titleStyle.Render("PA READER") + "  " +
		labelStyle.Render(fmt.Sprintf("PRHP %d.%d", m.version.Major, m.version.Minor))
	deviceLine := labelStyle.Render(fmt.Sprintf(
		"%s  ·  %04X:%04X",
		product,
		m.device.VendorID,
		m.device.ProductID,
	))

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		statusStyle.Bold(true).Render("● "+status),
		"",
		labelStyle.Render("卡片类型"),
		lipgloss.NewStyle().Foreground(colorText).Render(typeName),
		"",
		labelStyle.Render("卡片编号"),
		idStyle.Render(identifier),
	)

	if m.card.Type == protocol.CardNone && m.lastID != "" && m.err == nil {
		body += "\n\n" + labelStyle.Render("上次读取") + "\n" +
			lipgloss.NewStyle().Foreground(colorText).Render(m.lastID+"  "+m.lastSeen.Format("15:04:05"))
	}

	panelWidth := 58
	if m.width > 0 && m.width-8 < panelWidth {
		panelWidth = max(32, m.width-8)
	}
	panel := panelStyle.Width(panelWidth).Render(body)
	footer := helpStyle.Render("q / esc 退出")
	content := lipgloss.JoinVertical(lipgloss.Left, header, deviceLine, "", panel, "", footer)

	if m.width == 0 || m.height == 0 {
		return content
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func refreshCardState(service *aimeio.Service) tea.Cmd {
	return func() tea.Msg {
		device, connected := service.DeviceInfo()
		return refreshResultMsg{
			card:      service.CurrentCard(),
			device:    device,
			connected: connected,
			err:       service.Err(),
			at:        time.Now(),
		}
	}
}

func cardIdentifier(card protocol.Card) string {
	switch card.Type {
	case protocol.CardMIFARE:
		for _, value := range card.LUID {
			if value != 0 {
				return protocol.FormatLUID(card.LUID)
			}
		}
		return fmt.Sprintf("BLOCK1:%X", card.Blocks[:16])
	case protocol.CardFeliCa:
		return protocol.FormatIDm(card.IDm)
	default:
		return ""
	}
}
