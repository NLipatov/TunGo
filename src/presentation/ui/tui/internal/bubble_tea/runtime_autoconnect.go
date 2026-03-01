package bubble_tea

// DisableRuntimeAutoConnect persists AutoConnect=false if it is currently enabled.
func DisableRuntimeAutoConnect() error {
	settings := loadUISettingsFromDisk()
	prefs := settings.Preferences()
	if !prefs.AutoConnect {
		return nil
	}

	prefs.AutoConnect = false
	settings.update(prefs)
	return savePreferencesToDisk(prefs)
}
