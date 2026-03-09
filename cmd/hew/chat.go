package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	hew "github.com/cosgroveb/hew"
)

// chatModel holds the viewport and buffers for chat rendering.
type chatModel struct {
	viewport      viewport.Model
	content       strings.Builder // committed content
	streamBuf     strings.Builder // accumulating streaming tokens
	pendingCmd    string          // pending command display (styled)
	pendingCmdRaw string          // raw command text for later commit
	streaming     bool
	wasAtBottom   bool
	hasNew        bool // new content arrived while scrolled up
	verbose       bool
	styles        *styles
}

func newChatModel(width, height int, s *styles, verbose bool) chatModel {
	vp := viewport.New(width, height)
	return chatModel{
		viewport: vp,
		styles:   s,
		verbose:  verbose,
	}
}

func (c *chatModel) appendToken(text string) {
	if !c.streaming {
		c.streaming = true
		c.wasAtBottom = c.viewport.AtBottom()
		c.streamBuf.Reset()
	}
	c.streamBuf.WriteString(text)
}

func (c *chatModel) commitStream(authoritative string) {
	c.content.WriteString(authoritative)
	c.content.WriteString("\n\n")
	c.streamBuf.Reset()
	c.streaming = false
}

func (c *chatModel) commitPartialStream() {
	if c.streamBuf.Len() > 0 {
		c.content.WriteString(c.streamBuf.String())
		c.content.WriteString("\n\n")
		c.streamBuf.Reset()
	}
	c.streaming = false
}

func (c *chatModel) resetStreaming() {
	c.streamBuf.Reset()
	c.streaming = false
}

func (c *chatModel) appendEvent(e hew.Event) {
	switch ev := e.(type) {
	case hew.EventResponse:
		if c.streaming {
			c.commitStream(ev.Message.Content)
		} else {
			c.content.WriteString(ev.Message.Content)
			c.content.WriteString("\n\n")
		}
	case hew.EventCommandStart:
		c.pendingCmdRaw = ev.Command
		c.pendingCmd = c.styles.Chat.CommandPending.Render(
			fmt.Sprintf("%s running: %s", iconPending, summarizeCommand(ev.Command)),
		)
	case hew.EventCommandDone:
		icon := iconSuccess
		style := c.styles.Chat.CommandSuccess
		if ev.Err != nil {
			icon = iconError
			style = c.styles.Chat.CommandError
		}
		// Include the actual command text
		if c.pendingCmdRaw != "" {
			c.content.WriteString(style.Render(fmt.Sprintf("%s ran: %s", icon, summarizeCommand(c.pendingCmdRaw))))
		} else {
			c.content.WriteString(style.Render(fmt.Sprintf("%s ran: command", icon)))
		}
		c.content.WriteString("\n")
		if ev.Output != "" {
			c.content.WriteString(c.styles.Chat.CommandOutput.Render(truncateOutput(ev.Output, 20)))
			c.content.WriteString("\n")
		}
		c.content.WriteString("\n")
		c.pendingCmd = ""
		c.pendingCmdRaw = ""
	case hew.EventFormatError:
		c.content.WriteString(c.styles.Chat.Warning.Render(
			fmt.Sprintf("%s No bash block found in response — format error", iconWarning),
		))
		c.content.WriteString("\n\n")
	case hew.EventDebug:
		if ev.Message == "querying model..." {
			c.resetStreaming()
		}
		if c.verbose {
			c.content.WriteString(c.styles.Chat.Debug.Render(fmt.Sprintf("[hew] %s", ev.Message)))
			c.content.WriteString("\n")
		}
		// Token events are not defined; ignore.
	default:
	}
}

func (c *chatModel) updateViewport() {
	full := c.content.String() + c.pendingCmd + c.streamBuf.String()
	// Preserve scroll position if not at bottom
	wasBottom := c.wasAtBottom
	if !c.streaming {
		wasBottom = c.viewport.AtBottom()
	}
	c.viewport.SetContent(full)
	if wasBottom {
		c.viewport.GotoBottom()
		c.hasNew = false
	} else if c.viewport.TotalLineCount() > c.viewport.VisibleLineCount() {
		c.hasNew = true
	}
}

func (c *chatModel) resize(width, height int) {
	c.viewport.Width = width
	c.viewport.Height = height
}

func summarizeCommand(cmd string) string {
	if len(cmd) > 60 {
		return cmd[:57] + "..."
	}
	return cmd
}

func truncateOutput(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	truncated := strings.Join(lines[:maxLines], "\n")
	return truncated + fmt.Sprintf("\n... (%d more lines)", len(lines)-maxLines)
}
