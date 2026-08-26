package bubble_tea

import (
	"testing"
	"time"
	tuiconfig "tungo/internal/config/tui"

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
	if msg != nil {
		t.Fatalf("expected no message when stopped, got %T", msg)
	}
}

func TestLogViewportUpdateCmd_ChangesChannelFires(t *testing.T) {
	changes := make(chan struct{}, 2)
	changes <- struct{}{}
	changes <- struct{}{}

	started := time.Now()
	msg := logViewportUpdateCmd(
		&logViewportTestChangeFeed{changes: changes},
		make(chan struct{}),
		7,
	)()
	if elapsed := time.Since(started); elapsed < logViewportRefreshDelay {
		t.Fatalf("log update returned after %v, want at least %v", elapsed, logViewportRefreshDelay)
	}
	tick, ok := msg.(logViewportTickMsg)
	if !ok {
		t.Fatalf("expected logViewportTickMsg, got %T", msg)
	}
	if tick.seq != 7 {
		t.Fatalf("expected seq=7 when changes fires, got %d", tick.seq)
	}
	if len(changes) != 0 {
		t.Fatal("expected pending changes to be batched into the update")
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
	lv.ensure(120, 40, tuiconfig.Configuration{ShowFooter: true}, "", "hint")

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
	lv.ensure(120, 40, tuiconfig.Configuration{ShowFooter: true}, "", "hint")

	if !lv.ready {
		t.Error("expected ready to be true after ensure")
	}
}

func TestLogViewportRefresh_NilFeed(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.refresh(nil, tuiconfig.Configuration{})

	content := lv.viewport.View()
	if content == "" {
		t.Error("expected non-empty viewport content")
	}
}

func TestLogViewportRefresh_WithFeed(t *testing.T) {
	feed := &logViewportTestFeed{lines: []string{"line1", "line2", "line3"}}
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.refresh(feed, tuiconfig.Configuration{})

	content := lv.viewport.View()
	if content == "" {
		t.Error("expected non-empty viewport content")
	}
}

func TestLogViewportRefresh_FollowMode(t *testing.T) {
	feed := &logViewportTestFeed{lines: []string{"a", "b", "c"}}
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.follow = true
	lv.refresh(feed, tuiconfig.Configuration{})

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
	lv.ensure(80, 10, tuiconfig.Configuration{}, "", "")

	// First refresh to populate content
	lv.refresh(feed, tuiconfig.Configuration{})

	// Scroll up and disable follow
	lv.viewport.SetYOffset(3)
	lv.follow = false

	// Second refresh should preserve offset
	lv.refresh(feed, tuiconfig.Configuration{})

	if lv.follow {
		t.Error("expected follow to remain false when not at bottom")
	}
}

func TestLogViewportRefresh_DoesNotRestoreDisabledFollowAtBottom(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	lv := newLogViewport()
	lv.ensure(80, 10, tuiconfig.Configuration{}, "", "")
	lv.refresh(&logViewportTestFeed{lines: lines}, tuiconfig.Configuration{})
	lv.follow = false
	offset := lv.viewport.YOffset()

	lv.refresh(&logViewportTestFeed{lines: append(lines, "new")}, tuiconfig.Configuration{})

	if lv.follow {
		t.Fatal("expected follow to remain disabled")
	}
	if lv.viewport.YOffset() != offset {
		t.Fatalf("viewport offset = %d, want %d", lv.viewport.YOffset(), offset)
	}
}

func TestLogViewportUpdateKeys(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")

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
			_ = lv.update(msg, nil, tuiconfig.Configuration{})
		})
	}
}

func TestLogViewportUpdateKeys_ScrollUp(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.follow = true

	msg := tea.KeyPressMsg(tea.Key{Code: 'k'})
	_ = lv.update(msg, nil, tuiconfig.Configuration{})

	if lv.follow {
		t.Error("expected follow to be false after scroll up")
	}
}

func TestLogViewportUpdateKeys_ScrollDown(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.follow = false

	msg := tea.KeyPressMsg(tea.Key{Code: 'j'})
	_ = lv.update(msg, nil, tuiconfig.Configuration{})
}

func TestLogViewportUpdateKeys_SpaceTogglesFollow(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.follow = true
	_ = lv.startUpdates(nil, tuiconfig.Configuration{})
	stopped := lv.waitStop

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})
	if cmd := lv.update(msg, nil, tuiconfig.Configuration{}); cmd != nil {
		t.Fatal("expected no log update while follow is disabled")
	}
	if lv.follow {
		t.Error("expected follow to be false after space toggle")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("expected disabled follow to stop log updates")
	}

	if cmd := lv.update(msg, nil, tuiconfig.Configuration{}); cmd == nil {
		t.Fatal("expected log updates to resume with follow")
	}
	if !lv.follow {
		t.Error("expected follow to be true after second space toggle")
	}
}

func TestLogViewportView(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
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
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.follow = true

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp})
	_ = lv.update(msg, nil, tuiconfig.Configuration{})

	if lv.follow {
		t.Error("expected follow to be false after PageUp")
	}
}

func TestLogViewportHomeSetsFollowFalse(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.follow = true

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyHome})
	_ = lv.update(msg, nil, tuiconfig.Configuration{})

	if lv.follow {
		t.Error("expected follow to be false after Home")
	}
}

func TestLogViewportEndSetsFollowTrue(t *testing.T) {
	lv := newLogViewport()
	lv.ensure(80, 24, tuiconfig.Configuration{}, "", "")
	lv.follow = false

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd})
	_ = lv.update(msg, nil, tuiconfig.Configuration{})

	if !lv.follow {
		t.Error("expected follow to be true after End")
	}
}
