package tui

import (
	"encoding/json"
	"strings"
	"tungo/infrastructure/PAL/configuration/client"
	"unicode"
)

// ConfigurationParser converts raw JSON strings into Configuration,
// stripping out unwanted control and formatting characters.
type ConfigurationParser struct{}

func NewConfigurationParser() *ConfigurationParser {
	return &ConfigurationParser{}
}

// FromJson cleans the input and unmarshals it into a Configuration.
func (c *ConfigurationParser) FromJson(input string) (client.Configuration, error) {
	clean := strings.TrimSpace(c.sanitize(input))

	var cfg client.Configuration
	if err := json.Unmarshal([]byte(clean), &cfg); err != nil {
		return client.Configuration{}, err
	}
	if err := cfg.Validate(); err != nil {
		return client.Configuration{}, err
	}
	return cfg, nil
}

// sanitize removes all Unicode control (Cc) and format (Cf) characters
// except for JSON-legal whitespace: space, tab, newline, and carriage return.
func (c *ConfigurationParser) sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(r)
		case unicode.IsControl(r) || unicode.In(r, unicode.Cf):
			// drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
