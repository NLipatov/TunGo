package bubble_tea

import (
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const logViewportRefreshDelay = 50 * time.Millisecond

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
	offset := v.viewport.YOffset()
	content := renderLogsViewportLines(lines, v.viewport.Width(), resolveUIStyles(prefs))
	v.viewport.SetContentLines(content)
	if v.follow {
		v.viewport.GotoBottom()
		return
	}
	v.viewport.SetYOffset(offset)
}

func (v *logViewport) startUpdates(feed RuntimeLogFeed, prefs UIPreferences) tea.Cmd {
	v.stopUpdates()
	v.tickSeq++
	v.refresh(feed, prefs)
	if !v.follow {
		return nil
	}
	v.waitStop = make(chan struct{})
	return logViewportUpdateCmd(feed, v.waitStop, v.tickSeq)
}

func (v *logViewport) stopUpdates() {
	if v.waitStop != nil {
		close(v.waitStop)
		v.waitStop = nil
	}
}

func (v *logViewport) refreshUpdates(feed RuntimeLogFeed, prefs UIPreferences) tea.Cmd {
	if !v.follow {
		return nil
	}
	v.refresh(feed, prefs)
	return logViewportUpdateCmd(feed, v.waitStop, v.tickSeq)
}

func (v *logViewport) update(msg tea.KeyPressMsg, feed RuntimeLogFeed, prefs UIPreferences) tea.Cmd {
	wasFollowing := v.follow
	switch msg.String() {
	case "pgup":
		v.viewport.PageUp()
		v.follow = false
	case "pgdown":
		v.viewport.PageDown()
		v.follow = v.viewport.AtBottom()
	case "home":
		v.viewport.GotoTop()
		v.follow = false
	case "end":
		v.viewport.GotoBottom()
		v.follow = true
	case "space":
		v.follow = !v.follow
		if v.follow {
			v.viewport.GotoBottom()
		}
	case "up", "k":
		v.viewport.ScrollUp(1)
		v.follow = false
	case "down", "j":
		v.viewport.ScrollDown(1)
		v.follow = v.viewport.AtBottom()
	}
	if v.follow == wasFollowing {
		return nil
	}
	if !v.follow {
		v.stopUpdates()
		return nil
	}
	return v.startUpdates(feed, prefs)
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
					return nil
				case <-changes:
				}

				timer := time.NewTimer(logViewportRefreshDelay)
				defer timer.Stop()
				select {
				case <-stop:
					return nil
				case <-timer.C:
				}

				// Changes are edge notifications, not a count. Discard a pending
				// edge because the next refresh reads the entire latest snapshot.
				select {
				case <-changes:
				default:
				}
				return logViewportTickMsg{seq: seq}
			}
		}
	}
	return logViewportTickCmd(seq)
}
