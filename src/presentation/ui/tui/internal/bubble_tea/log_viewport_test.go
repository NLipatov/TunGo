package bubble_tea

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type logViewportTestFeed struct {
	lines []string
}

func (f *logViewportTestFeed) Tail(limit int) []string {
	if limit >= len(f.lines) {
		return f.lines
	}
	return f.lines[len(f.lines)-limit:]
}

func (f *logViewportTestFeed) TailInto(dst []string, limit int) int {
	src := f.Tail(limit)
	return copy(dst, src)
}

type logViewportTestChangeFeed struct {
	logViewportTestFeed
	changes <-chan struct{}
}

func (f *logViewportTestChangeFeed) Changes() <-chan struct{} {
	return f.changes
}

func TestLogViewportUpdateCmd_ReturnsCommand(t *testing.T) {
	tests := []struct {
		name string
		feed RuntimeLogFeed
	}{
		{name: "nil feed"},
		{name: "plain feed", feed: &logViewportTestFeed{}},
		{name: "change feed", feed: &logViewportTestChangeFeed{changes: make(chan struct{})}},
		{name: "nil changes", feed: &logViewportTestChangeFeed{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cmd := logViewportUpdateCmd(tt.feed, make(chan struct{}), 5); cmd == nil {
				t.Fatal("expected non-nil command")
			}
		})
	}
}

func TestLogViewportUpdateCmd_StopChannelClosed(t *testing.T) {
	stop := make(chan struct{})
	close(stop)

	msg := logViewportUpdateCmd(
		&logViewportTestChangeFeed{changes: make(chan struct{})},
		stop,
		7,
	)()
	tick, ok := msg.(logViewportTickMsg)
	if !ok {
		t.Fatalf("expected logViewportTickMsg, got %T", msg)
	}
	if tick.seq != 0 {
		t.Fatalf("expected seq=0 when stop closed, got %d", tick.seq)
	}
}

func TestLogViewportUpdateCmd_ChangesChannelFires(t *testing.T) {
	changes := make(chan struct{}, 1)
	changes <- struct{}{}

	msg := logViewportUpdateCmd(
		&logViewportTestChangeFeed{changes: changes},
		make(chan struct{}),
		7,
	)()
	tick, ok := msg.(logViewportTickMsg)
	if !ok {
		t.Fatalf("expected logViewportTickMsg, got %T", msg)
	}
	if tick.seq != 7 {
		t.Fatalf("expected seq=7 when changes fires, got %d", tick.seq)
	}
}

func TestNewLogViewport(t *testing.T) {
	lv := newLogViewport()
	if !lv.ready {
		t.Error("expected ready to be true")
	}
	if !lv.follow {
		t.Error("expected follow to be true")
	}
	if lv.tickSeq != 0 {
		t.Error("expected tickSeq to be 0")
	}
	if lv.waitStop != nil {
		t.Error("expected waitStop to be nil")
	}
}

func TestLogViewportEnsure(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(120, 40, UIPreferences{ShowFooter: true}, "", "hint")

	w := lv.viewport.Width()
	h := lv.viewport.Height()
	if w <= 0 {
		t.Errorf("expected positive width, got %d", w)
	}
	if h <= 0 {
		t.Errorf("expected positive height, got %d", h)
	}
}

func TestLogViewportEnsure_NotReady(t *testing.T) {
	lv := newLogViewport()
	lv.ready = false
	lv.ensure(120, 40, UIPreferences{ShowFooter: true}, "", "hint")

	if !lv.ready {
		t.Error("expected ready to be true after ensure")
	}
}

func TestLogViewportRefresh_NilFeed(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.refresh(nil, UIPreferences{})

	content := lv.viewport.View()
	if content == "" {
		t.Error("expected non-empty viewport content")
	}
}

func TestLogViewportRefresh_WithFeed(t *testing.T) {
	feed := &logViewportTestFeed{lines: []string{"line1", "line2", "line3"}}
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.refresh(feed, UIPreferences{})

	content := lv.viewport.View()
	if content == "" {
		t.Error("expected non-empty viewport content")
	}
}

func TestLogViewportRefresh_FollowMode(t *testing.T) {
	feed := &logViewportTestFeed{lines: []string{"a", "b", "c"}}
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.follow = true
	lv.refresh(feed, UIPreferences{})

	if !lv.follow {
		t.Error("expected follow to remain true after refresh")
	}
}

func TestLogViewportRefresh_PreservesOffset(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	feed := &logViewportTestFeed{lines: lines}
	lv := newLogViewport()
	lv.ensure(80, 10, UIPreferences{}, "", "")

	// First refresh to populate content
	lv.refresh(feed, UIPreferences{})

	// Scroll up and disable follow
	lv.viewport.SetYOffset(3)
	lv.follow = false

	// Second refresh should preserve offset
	lv.refresh(feed, UIPreferences{})

	if lv.follow {
		t.Error("expected follow to remain false when not at bottom")
	}
}

func TestLogViewportRestartWait(t *testing.T) {
	lv := newLogViewport()
	if lv.waitStop != nil {
		t.Error("expected nil waitStop initially")
	}

	lv.restartWait()
	if lv.waitStop == nil {
		t.Error("expected non-nil waitStop after restart")
	}

	ch := lv.waitStop
	lv.restartWait()
	select {
	case <-ch:
	default:
		t.Error("expected old waitStop channel to be closed")
	}
	if lv.waitStop == nil {
		t.Error("expected new waitStop channel")
	}
}

func TestLogViewportStopWait(t *testing.T) {
	lv := newLogViewport()
	lv.restartWait()
	ch := lv.waitStop

	lv.stopWait()
	if lv.waitStop != nil {
		t.Error("expected nil waitStop after stop")
	}
	select {
	case <-ch:
	default:
		t.Error("expected channel to be closed")
	}

	// double stop should not panic
	lv.stopWait()
}

func TestLogViewportUpdateKeys(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")

	tests := []struct {
		name string
		code rune
	}{
		{"PageUp", tea.KeyPgUp},
		{"PageDown", tea.KeyPgDown},
		{"Home", tea.KeyHome},
		{"End", tea.KeyEnd},
		{"Space", tea.KeySpace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tea.KeyPressMsg(tea.Key{Code: tt.code})
			_ = lv.updateKeys(msg)
		})
	}
}

func TestLogViewportUpdateKeys_ScrollUp(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.follow = true

	msg := tea.KeyPressMsg(tea.Key{Code: 'k'})
	_ = lv.updateKeys(msg)

	if lv.follow {
		t.Error("expected follow to be false after scroll up")
	}
}

func TestLogViewportUpdateKeys_ScrollDown(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.follow = false

	msg := tea.KeyPressMsg(tea.Key{Code: 'j'})
	_ = lv.updateKeys(msg)
}

func TestLogViewportUpdateKeys_SpaceTogglesFollow(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.follow = true

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})
	_ = lv.updateKeys(msg)
	if lv.follow {
		t.Error("expected follow to be false after space toggle")
	}

	_ = lv.updateKeys(msg)
	if !lv.follow {
		t.Error("expected follow to be true after second space toggle")
	}
}

func TestLogViewportView(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	result := lv.view()
	if result == "" {
		t.Error("expected non-empty view")
	}
}

func TestLogViewportTickMsg(t *testing.T) {
	msg := logViewportTickMsg{seq: 42}
	if msg.seq != 42 {
		t.Error("unexpected seq")
	}
}

func TestLogViewportPageUpSetsFollowFalse(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.follow = true

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp})
	_ = lv.updateKeys(msg)

	if lv.follow {
		t.Error("expected follow to be false after PageUp")
	}
}

func TestLogViewportHomeSetsFollowFalse(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.follow = true

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyHome})
	_ = lv.updateKeys(msg)

	if lv.follow {
		t.Error("expected follow to be false after Home")
	}
}

func TestLogViewportEndSetsFollowTrue(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, UIPreferences{}, "", "")
	lv.follow = false

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd})
	_ = lv.updateKeys(msg)

	if !lv.follow {
		t.Error("expected follow to be true after End")
	}
}
