package bubble_tea

import (
	"fmt"

	tuiconfig "tungo/internal/config/tui"
)

type Preferences struct {
	current tuiconfig.Configuration
	save    func(tuiconfig.Configuration) error
}

// newDefaultPreferences creates preferences initialized with the default configuration.
func newDefaultPreferences() *Preferences {
	return newPreferences(tuiconfig.Default())
}

// newPreferences creates preferences initialized with the supplied configuration and the default persistence function.
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

// settingsSaveNotice returns an empty string when saving succeeds or a notice describing the session-only change when saving fails.
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

// LoadPreferences loads the persisted configuration and returns preferences initialized with it.
// It returns default preferences when the persisted configuration cannot be loaded.
func LoadPreferences() *Preferences {
	configuration, err := tuiconfig.Load()
	if err != nil {
		return newDefaultPreferences()
	}
	return newPreferences(configuration)
}
