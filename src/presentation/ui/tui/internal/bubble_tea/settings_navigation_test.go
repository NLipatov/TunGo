package bubble_tea

import "testing"

// ---------------------------------------------------------------------------
// nextModePreference
// ---------------------------------------------------------------------------

func TestNextModePreference_ForwardCycles(t *testing.T) {
	cases := []struct{ in, want ModePreference }{
		{ModePreferenceNone, ModePreferenceClient},
		{ModePreferenceClient, ModePreferenceServer},
		{ModePreferenceServer, ModePreferenceNone},
	}
	for _, c := range cases {
		got := nextModePreference(c.in, 1)
		if got != c.want {
			t.Errorf("nextModePreference(%q, 1) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNextModePreference_BackwardCycles(t *testing.T) {
	cases := []struct{ in, want ModePreference }{
		{ModePreferenceNone, ModePreferenceServer},
		{ModePreferenceClient, ModePreferenceNone},
		{ModePreferenceServer, ModePreferenceClient},
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
	if got != ModePreferenceClient {
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
	prefs := UIPreferences{AutoSelectMode: ModePreferenceClient}
	got := settingsVisibleRowCount(prefs, true)
	if got != settingsRowsCount {
		t.Errorf("got %d, want %d", got, settingsRowsCount)
	}
}

func TestSettingsVisibleRowCount_ServerSupported_ModeServer_AutoConnectHidden(t *testing.T) {
	prefs := UIPreferences{AutoSelectMode: ModePreferenceServer}
	got := settingsVisibleRowCount(prefs, true)
	if got != settingsRowsCount-1 {
		t.Errorf("got %d, want %d", got, settingsRowsCount-1)
	}
}

func TestSettingsVisibleRowCount_ServerSupported_ModeNone_AutoConnectHidden(t *testing.T) {
	prefs := UIPreferences{AutoSelectMode: ModePreferenceNone}
	got := settingsVisibleRowCount(prefs, true)
	if got != settingsRowsCount-1 {
		t.Errorf("got %d, want %d", got, settingsRowsCount-1)
	}
}

func TestSettingsVisibleRowCount_NoServer_AlwaysOneLessThanTotal(t *testing.T) {
	want := settingsRowsCount - 1
	for _, m := range []ModePreference{ModePreferenceNone, ModePreferenceClient, ModePreferenceServer} {
		prefs := UIPreferences{AutoSelectMode: m}
		if got := settingsVisibleRowCount(prefs, false); got != want {
			t.Errorf("serverSupported=false mode=%q: got %d, want %d", m, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// applySettingsChange: Mode and AutoConnect rows
// ---------------------------------------------------------------------------

func TestApplySettingsChange_ModeRow_CyclesForward(t *testing.T) {
	p := newUIPreferencesProvider(UIPreferences{AutoSelectMode: ModePreferenceNone})
	got := applySettingsChange(p, settingsModeRow, 1, true)
	if got.AutoSelectMode != ModePreferenceClient {
		t.Errorf("got %q, want ModePreferenceClient", got.AutoSelectMode)
	}
}

func TestApplySettingsChange_ModeRow_CyclesBackward(t *testing.T) {
	p := newUIPreferencesProvider(UIPreferences{AutoSelectMode: ModePreferenceClient})
	got := applySettingsChange(p, settingsModeRow, -1, true)
	if got.AutoSelectMode != ModePreferenceNone {
		t.Errorf("got %q, want ModePreferenceNone", got.AutoSelectMode)
	}
}

func TestApplySettingsChange_AutoConnectRow_TogglesOn(t *testing.T) {
	p := newUIPreferencesProvider(UIPreferences{AutoConnect: false})
	got := applySettingsChange(p, settingsAutoConnectRow, 1, true)
	if !got.AutoConnect {
		t.Error("expected AutoConnect toggled on")
	}
}

func TestApplySettingsChange_AutoConnectRow_TogglesOff(t *testing.T) {
	p := newUIPreferencesProvider(UIPreferences{AutoConnect: true})
	got := applySettingsChange(p, settingsAutoConnectRow, 1, true)
	if got.AutoConnect {
		t.Error("expected AutoConnect toggled off")
	}
}

func TestApplySettingsChange_NoServer_VisibleModePosition_MapsToAutoConnect(t *testing.T) {
	// When !serverSupported, cursor=settingsModeRow → visibleCursorToSettingsRow → settingsAutoConnectRow.
	p := newUIPreferencesProvider(UIPreferences{AutoConnect: false})
	got := applySettingsChange(p, settingsModeRow, 1, false)
	if !got.AutoConnect {
		t.Error("expected AutoConnect to toggle when cursor is at Mode position with !serverSupported")
	}
}
