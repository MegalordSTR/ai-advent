package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ai-agent-cli/internal/agent"
	"ai-agent-cli/internal/deepseek"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	maxMessages    = 100
	maxInputLength = 4096
)

type agentSelectModel struct {
	agents []agent.Agent
	cursor int
	client *deepseek.DeepSeekClient
}

func newAgentSelectModel(agents []agent.Agent, client *deepseek.DeepSeekClient) agentSelectModel {
	return agentSelectModel{agents: agents, client: client}
}

func (m agentSelectModel) Init() tea.Cmd {
	return nil
}

func (m agentSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		case "enter", " ":
			if len(m.agents) > 0 {
				selected := m.agents[m.cursor]
				chatModel := newChatModel(selected, m.client)
				return chatModel, chatModel.Init()
			}
		}
	}
	return m, nil
}

func (m agentSelectModel) View() string {
	var b strings.Builder
	b.WriteString("Select an agent:\n\n")
	for i, agent := range m.agents {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s %s\n", cursor, agent.Name)
		if agent.Description != "" {
			fmt.Fprintf(&b, "   %s\n", agent.Description)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nPress ↑/↓ to navigate, Enter to select, Ctrl+C or q to quit.")
	return b.String()
}

type chatModel struct {
	agent    agent.Agent
	messages []deepseek.Message
	textarea textarea.Model
	viewport viewport.Model
	status   string
	waiting  bool
	errMsg   string
	client   *deepseek.DeepSeekClient
	ready    bool
}

func newChatModel(agent agent.Agent, client *deepseek.DeepSeekClient) chatModel {
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Ctrl+Enter to add new line)"
	ta.Focus()
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	// Custom keymap: Enter submits, Ctrl+Enter inserts newline
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+enter"))

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return chatModel{
		agent:    agent,
		textarea: ta,
		viewport: vp,
		status:   "Ready",
		client:   client,
	}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, tea.WindowSize())
}

type chatResponseMsg struct {
	content string
	err     error
}

func (m chatModel) sendToDeepSeek() tea.Msg {
	if m.client == nil {
		return chatResponseMsg{err: fmt.Errorf("client not initialized")}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Build messages: system prompt + history
	messages := []deepseek.Message{
		{Role: "system", Content: m.agent.Prompt},
	}
	messages = append(messages, m.messages...)
	content, err := m.client.Chat(ctx, messages)
	return chatResponseMsg{content: content, err: err}
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if m.waiting {
		switch msg := msg.(type) {
		case chatResponseMsg:
			m.waiting = false
			if msg.err != nil {
				m.errMsg = fmt.Sprintf("Error: %v", msg.err)
				m.status = "Error"
			} else {
				m.errMsg = ""
				m.messages = append(m.messages, deepseek.Message{Role: "assistant", Content: msg.content})
				if len(m.messages) > maxMessages {
					m.messages = m.messages[len(m.messages)-maxMessages:]
				}
				m.status = "Ready"
				m.textarea.Reset()
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
			}
			return m, nil
		}
		// While waiting, ignore other inputs
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.textarea.Focused() {
				input := strings.TrimSpace(m.textarea.Value())
				if input == "" {
					// Ignore empty enter
					return m, nil
				}
				if input == "/exit" {
					return m, tea.Quit
				}
				if len(input) > maxInputLength {
					m.errMsg = fmt.Sprintf("Input too long (max %d characters)", maxInputLength)
					return m, nil
				}
				// Add user message
				m.messages = append(m.messages, deepseek.Message{Role: "user", Content: input})
				if len(m.messages) > maxMessages {
					m.messages = m.messages[len(m.messages)-maxMessages:]
				}
				m.waiting = true
				m.status = "Thinking..."
				m.errMsg = ""
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
				// Do not pass Enter to textarea
				return m, tea.Batch(m.sendToDeepSeekCmd())
			}
		}
	case tea.WindowSizeMsg:
		// Calculate heights: textarea = 3 lines, help line + status line = 2 lines, total = 5
		viewportHeight := msg.Height - 5
		if viewportHeight < 1 {
			viewportHeight = 1
		}
		// Save current scroll position
		oldYOffset := m.viewport.YOffset
		m.viewport = viewport.New(msg.Width, viewportHeight)
		m.viewport.YOffset = oldYOffset
		// Ensure YOffset is within bounds
		m.viewport.SetContent(m.renderMessages())
		m.textarea.SetWidth(msg.Width)
		m.ready = true
		m.viewport.GotoBottom()
	}

	// Pass all other messages to textarea and viewport
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m chatModel) sendToDeepSeekCmd() tea.Cmd {
	return func() tea.Msg {
		return m.sendToDeepSeek()
	}
}

func (m chatModel) renderMessages() string {
	var b strings.Builder
	width := m.viewport.Width
	if width <= 0 {
		width = 80
	}
	style := lipgloss.NewStyle().Width(width)

	agentLine := fmt.Sprintf("Agent: %s", m.agent.Name)
	b.WriteString(style.Render(agentLine))
	b.WriteString("\n\n")

	for _, msg := range m.messages {
		prefix := "You: "
		if msg.Role == "assistant" {
			prefix = "Assistant: "
		}
		line := fmt.Sprintf("%s%s", prefix, msg.Content)
		b.WriteString(style.Render(line))
		b.WriteString("\n\n")
	}
	return b.String()
}

func (m chatModel) View() string {
	if !m.ready {
		return "Initializing..."
	}
	statusLine := fmt.Sprintf("Status: %s", m.status)
	if m.errMsg != "" {
		statusLine = fmt.Sprintf("%s | Error: %s", statusLine, m.errMsg)
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s\n%s",
		m.viewport.View(),
		m.textarea.View(),
		helpStyle.Render("Press Enter to send, Ctrl+C to quit, /exit to exit, PgUp/PgDown to scroll"),
		statusLine)
}

var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
