package bubble_tea

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

type logViewportTickMsg struct {
	seq uint64
}

type logViewport struct {
	viewport viewport.Model
	ready    bool
	follow   bool
	scratch  []string
	waitStop chan struct{}
	tickSeq  uint64
}

func newLogViewport() logViewport {
	return logViewport{
		viewport: viewport.New(viewport.WithWidth(1), viewport.WithHeight(8)),
		ready:    true,
		follow:   true,
	}
}

func (v *logViewport) ensure(width, height int, prefs UIPreferences, subtitle, hint string) {
	contentWidth, viewportHeight := computeLogsViewportSize(width, height, prefs, subtitle, hint)
	if !v.ready {
		v.viewport = viewport.New(viewport.WithWidth(contentWidth), viewport.WithHeight(viewportHeight))
		v.ready = true
		return
	}
	v.viewport.SetWidth(contentWidth)
	v.viewport.SetHeight(viewportHeight)
}

func (v *logViewport) refresh(feed RuntimeLogFeed, prefs UIPreferences) {
	lines := runtimeLogSnapshot(feed, &v.scratch)
	wasAtBottom := v.viewport.AtBottom()
	offset := v.viewport.YOffset()
	content := renderLogsViewportContent(lines, v.viewport.Width(), resolveUIStyles(prefs))
	v.viewport.SetContent(content)
	if v.follow || wasAtBottom {
		v.viewport.GotoBottom()
		v.follow = true
		return
	}
	v.viewport.SetYOffset(offset)
}

func (v *logViewport) restartWait() {
	v.stopWait()
	v.waitStop = make(chan struct{})
}

func (v *logViewport) stopWait() {
	if v.waitStop != nil {
		close(v.waitStop)
		v.waitStop = nil
	}
}

func (v *logViewport) updateKeys(msg tea.KeyPressMsg, keys selectorKeyMap) tea.Cmd {
	switch msg.Key().Code {
	case tea.KeyPgUp:
		v.viewport.PageUp()
		v.follow = false
		return nil
	case tea.KeyPgDown:
		v.viewport.PageDown()
		v.follow = v.viewport.AtBottom()
		return nil
	case tea.KeyHome:
		v.viewport.GotoTop()
		v.follow = false
		return nil
	case tea.KeyEnd:
		v.viewport.GotoBottom()
		v.follow = true
		return nil
	case tea.KeySpace:
		v.follow = !v.follow
		if v.follow {
			v.viewport.GotoBottom()
		}
		return nil
	}

	switch {
	case key.Matches(msg, keys.Up):
		v.viewport.ScrollUp(1)
		v.follow = false
	case key.Matches(msg, keys.Down):
		v.viewport.ScrollDown(1)
		v.follow = v.viewport.AtBottom()
	}
	return nil
}

func (v logViewport) view() string {
	return v.viewport.View()
}

func logViewportTickCmd(seq uint64) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return logViewportTickMsg{seq: seq}
	})
}

func logViewportUpdateCmd(feed RuntimeLogFeed, stop <-chan struct{}, seq uint64) tea.Cmd {
	changeFeed, ok := feed.(RuntimeLogChangeFeed)
	if ok {
		changes := changeFeed.Changes()
		if changes != nil {
			return func() tea.Msg {
				select {
				case <-stop:
					return logViewportTickMsg{}
				case <-changes:
					return logViewportTickMsg{seq: seq}
				}
			}
		}
	}
	return logViewportTickCmd(seq)
}
