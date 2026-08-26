package bubble_tea

import tuiconfig "tungo/internal/config/tui"

type Preferences struct {
	current tuiconfig.Configuration
}

func newDefaultPreferences() *Preferences {
	return &Preferences{current: tuiconfig.Default()}
}

func newPreferences(prefs tuiconfig.Configuration) *Preferences {
	return &Preferences{current: prefs}
}

func (p *Preferences) Current() tuiconfig.Configuration {
	return p.current
}

func (p *Preferences) update(prefs tuiconfig.Configuration) {
	p.current = prefs
}

func (p *Preferences) DisableAutoConnect() error {
	if !p.current.AutoConnect {
		return nil
	}
	updated := p.current
	updated.AutoConnect = false
	if err := tuiconfig.Save(updated); err != nil {
		return err
	}
	p.current = updated
	return nil
}

func LoadPreferences() *Preferences {
	configuration, err := tuiconfig.Load()
	if err != nil {
		return newDefaultPreferences()
	}
	return newPreferences(configuration)
}
