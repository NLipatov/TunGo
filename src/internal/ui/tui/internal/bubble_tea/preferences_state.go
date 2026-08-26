package bubble_tea

import (
	"fmt"

	tuiconfig "tungo/internal/config/tui"
)

type Preferences struct {
	current tuiconfig.Configuration
	save    func(tuiconfig.Configuration) error
}

func newDefaultPreferences() *Preferences {
	return newPreferences(tuiconfig.Default())
}

func newPreferences(prefs tuiconfig.Configuration) *Preferences {
	return &Preferences{current: prefs, save: tuiconfig.Save}
}

func (p *Preferences) Current() tuiconfig.Configuration {
	return p.current
}

func (p *Preferences) update(prefs tuiconfig.Configuration) error {
	err := p.save(prefs)
	p.current = prefs
	return err
}

func settingsSaveNotice(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("Settings changed for this session but could not be saved: %v", err)
}

func (p *Preferences) DisableAutoConnect() error {
	if !p.current.AutoConnect {
		return nil
	}
	updated := p.current
	updated.AutoConnect = false
	return p.update(updated)
}

func LoadPreferences() *Preferences {
	configuration, err := tuiconfig.Load()
	if err != nil {
		return newDefaultPreferences()
	}
	return newPreferences(configuration)
}
