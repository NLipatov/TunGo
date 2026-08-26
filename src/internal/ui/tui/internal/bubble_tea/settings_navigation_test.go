package bubble_tea

import (
	"testing"
	tuiconfig "tungo/internal/config/tui"
)

func TestNextTheme_InvalidValueStartsFromFirstTheme(t *testing.T) {
	if got := nextTheme("invalid", 1); got != orderedThemeOptions[1] {
		t.Fatalf("nextTheme(invalid, 1) = %q, want %q", got, orderedThemeOptions[1])
	}
}

func TestNextTheme_BackwardWrapsFromFirstTheme(t *testing.T) {
	want := orderedThemeOptions[len(orderedThemeOptions)-1]
	if got := nextTheme(orderedThemeOptions[0], -1); got != want {
		t.Fatalf("nextTheme(first, -1) = %q, want %q", got, want)
	}
}

func TestNextStatsUnits_CyclesBothDirections(t *testing.T) {
	if got := nextStatsUnits(tuiconfig.StatsUnitsBytes, 1); got != tuiconfig.StatsUnitsBiBytes {
		t.Fatalf("nextStatsUnits(bytes, 1) = %q, want %q", got, tuiconfig.StatsUnitsBiBytes)
	}
	if got := nextStatsUnits(tuiconfig.StatsUnitsBytes, -1); got != tuiconfig.StatsUnitsBiBytes {
		t.Fatalf("nextStatsUnits(bytes, -1) = %q, want %q", got, tuiconfig.StatsUnitsBiBytes)
	}
}

// ---------------------------------------------------------------------------
// nextModePreference
// ---------------------------------------------------------------------------

func TestNextModePreference_ForwardCycles(t *testing.T) {
	cases := []struct{ in, want tuiconfig.ModePreference }{
		{tuiconfig.ModePreferenceNone, tuiconfig.ModePreferenceClient},
		{tuiconfig.ModePreferenceClient, tuiconfig.ModePreferenceServer},
		{tuiconfig.ModePreferenceServer, tuiconfig.ModePreferenceNone},
	}
	for _, c := range cases {
		got := nextModePreference(c.in, 1)
		if got != c.want {
			t.Errorf("nextModePreference(%q, 1) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNextModePreference_BackwardCycles(t *testing.T) {
	cases := []struct{ in, want tuiconfig.ModePreference }{
		{tuiconfig.ModePreferenceNone, tuiconfig.ModePreferenceServer},
		{tuiconfig.ModePreferenceClient, tuiconfig.ModePreferenceNone},
		{tuiconfig.ModePreferenceServer, tuiconfig.ModePreferenceClient},
	}
	for _, c := range cases {
		got := nextModePreference(c.in, -1)
		if got != c.want {
			t.Errorf("nextModePreference(%q, -1) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNextModePreference_UnknownFallsBackToNoneIndex(t *testing.T) {
	// Unknown doesn't match; idx stays at 0 (None); step=+1 → Client.
	got := nextModePreference("bogus", 1)
	if got != tuiconfig.ModePreferenceClient {
		t.Errorf("got %q, want ModePreferenceClient", got)
	}
}

// ---------------------------------------------------------------------------
// visibleCursorToSettingsRow
// ---------------------------------------------------------------------------

func TestVisibleCursorToSettingsRow_ServerSupported_AllCursorsPassThrough(t *testing.T) {
	for c := 0; c < settingsRowsCount; c++ {
		if got := visibleCursorToSettingsRow(c, true); got != c {
			t.Errorf("serverSupported=true cursor=%d: got %d, want %d", c, got, c)
		}
	}
}

func TestVisibleCursorToSettingsRow_NoServer_BelowModeRow_Unchanged(t *testing.T) {
	for c := 0; c < settingsModeRow; c++ {
		if got := visibleCursorToSettingsRow(c, false); got != c {
			t.Errorf("serverSupported=false cursor=%d: got %d, want %d", c, got, c)
		}
	}
}

func TestVisibleCursorToSettingsRow_NoServer_AtModeRow_MapsToAutoConnect(t *testing.T) {
	got := visibleCursorToSettingsRow(settingsModeRow, false)
	if got != settingsAutoConnectRow {
		t.Errorf("got %d, want settingsAutoConnectRow (%d)", got, settingsAutoConnectRow)
	}
}

// ---------------------------------------------------------------------------
// settingsVisibleRowCount
// ---------------------------------------------------------------------------

func TestSettingsVisibleRowCount_ServerSupported_ModeClient_AllRowsVisible(t *testing.T) {
	prefs := tuiconfig.Configuration{AutoSelectMode: tuiconfig.ModePreferenceClient}
	got := settingsVisibleRowCount(prefs, true)
	if got != settingsRowsCount {
		t.Errorf("got %d, want %d", got, settingsRowsCount)
	}
}

func TestSettingsVisibleRowCount_ServerSupported_ModeServer_AutoConnectHidden(t *testing.T) {
	prefs := tuiconfig.Configuration{AutoSelectMode: tuiconfig.ModePreferenceServer}
	got := settingsVisibleRowCount(prefs, true)
	if got != settingsRowsCount-1 {
		t.Errorf("got %d, want %d", got, settingsRowsCount-1)
	}
}

func TestSettingsVisibleRowCount_ServerSupported_ModeNone_AutoConnectHidden(t *testing.T) {
	prefs := tuiconfig.Configuration{AutoSelectMode: tuiconfig.ModePreferenceNone}
	got := settingsVisibleRowCount(prefs, true)
	if got != settingsRowsCount-1 {
		t.Errorf("got %d, want %d", got, settingsRowsCount-1)
	}
}

func TestSettingsVisibleRowCount_NoServer_AlwaysOneLessThanTotal(t *testing.T) {
	want := settingsRowsCount - 1
	for _, m := range []tuiconfig.ModePreference{tuiconfig.ModePreferenceNone, tuiconfig.ModePreferenceClient, tuiconfig.ModePreferenceServer} {
		prefs := tuiconfig.Configuration{AutoSelectMode: m}
		if got := settingsVisibleRowCount(prefs, false); got != want {
			t.Errorf("serverSupported=false mode=%q: got %d, want %d", m, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// applySettingsChange: Mode and AutoConnect rows
// ---------------------------------------------------------------------------

func TestApplySettingsChange_ModeRow_CyclesForward(t *testing.T) {
	p := newPreferences(tuiconfig.Configuration{AutoSelectMode: tuiconfig.ModePreferenceNone})
	got := applySettingsChange(p, settingsModeRow, 1, true)
	if got.AutoSelectMode != tuiconfig.ModePreferenceClient {
		t.Errorf("got %q, want ModePreferenceClient", got.AutoSelectMode)
	}
}

func TestApplySettingsChange_ModeRow_CyclesBackward(t *testing.T) {
	p := newPreferences(tuiconfig.Configuration{AutoSelectMode: tuiconfig.ModePreferenceClient})
	got := applySettingsChange(p, settingsModeRow, -1, true)
	if got.AutoSelectMode != tuiconfig.ModePreferenceNone {
		t.Errorf("got %q, want ModePreferenceNone", got.AutoSelectMode)
	}
}

func TestApplySettingsChange_AutoConnectRow_TogglesOn(t *testing.T) {
	p := newPreferences(tuiconfig.Configuration{AutoConnect: false})
	got := applySettingsChange(p, settingsAutoConnectRow, 1, true)
	if !got.AutoConnect {
		t.Error("expected AutoConnect toggled on")
	}
}

func TestApplySettingsChange_AutoConnectRow_TogglesOff(t *testing.T) {
	p := newPreferences(tuiconfig.Configuration{AutoConnect: true})
	got := applySettingsChange(p, settingsAutoConnectRow, 1, true)
	if got.AutoConnect {
		t.Error("expected AutoConnect toggled off")
	}
}

func TestApplySettingsChange_NoServer_VisibleModePosition_MapsToAutoConnect(t *testing.T) {
	// When !serverSupported, cursor=settingsModeRow → visibleCursorToSettingsRow → settingsAutoConnectRow.
	p := newPreferences(tuiconfig.Configuration{AutoConnect: false})
	got := applySettingsChange(p, settingsModeRow, 1, false)
	if !got.AutoConnect {
		t.Error("expected AutoConnect to toggle when cursor is at Mode position with !serverSupported")
	}
}
