package bubble_tea

// UIPreferencesProvider exposes read-only access to UI preferences.
type UIPreferencesProvider interface {
	Preferences() UIPreferences
}

// uiPreferencesProvider holds current UIPreferences and allows replacement.
type uiPreferencesProvider struct {
	current UIPreferences
}

func newDefaultUIPreferencesProvider() *uiPreferencesProvider {
	return &uiPreferencesProvider{current: newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)}
}

func newUIPreferencesProvider(prefs UIPreferences) *uiPreferencesProvider {
	return &uiPreferencesProvider{current: prefs}
}

func (p *uiPreferencesProvider) Preferences() UIPreferences {
	return p.current
}

func (p *uiPreferencesProvider) update(prefs UIPreferences) {
	p.current = prefs
}
