package bubble_tea

type Preferences struct {
	current UIPreferences
}

func newDefaultPreferences() *Preferences {
	return &Preferences{current: newUIPreferences(ThemeLight, "en", StatsUnitsBiBytes)}
}

func newPreferences(prefs UIPreferences) *Preferences {
	return &Preferences{current: prefs}
}

func (p *Preferences) Current() UIPreferences {
	return p.current
}

func (p *Preferences) update(prefs UIPreferences) {
	p.current = prefs
}

func (p *Preferences) DisableAutoConnect() error {
	if !p.current.AutoConnect {
		return nil
	}
	updated := p.current
	updated.AutoConnect = false
	if err := savePreferencesToDisk(updated); err != nil {
		return err
	}
	p.current = updated
	return nil
}
