package bubble_tea

import (
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

// validClientConfigurationJSON returns a JSON string for a minimal valid client Configuration.
func validClientConfigurationJSON() string {
	cfg := validClientConfiguration()
	data, err := json.Marshal(cfg)
	if err != nil {
		panic("failed to marshal valid client configuration: " + err.Error())
	}
	return string(data)
}

// validClientConfiguration builds a Configuration that passes Validate().
func validClientConfiguration() clientConfiguration.Configuration {
	return clientConfiguration.Configuration{
		ClientID: 1,
		Protocol: settings.TCP,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				TunName:    "tcptun0",
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				Server:     mustIPHost("10.0.0.1"),
				Port:       8080,
				DNSv4:      []string{"1.1.1.1"},
			},
			MTU:           1500,
			Protocol:      settings.TCP,
			Encryption:    settings.ChaCha20Poly1305,
			DialTimeoutMs: 5000,
		},
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
		X25519PublicKey:  make([]byte, 32),
	}
}

func mustIPHost(raw string) settings.Host {
	h, err := settings.IPHost(raw)
	if err != nil {
		panic(err)
	}
	return h
}

// ---------------------------------------------------------------------------
// parseClientConfigurationJSON
// ---------------------------------------------------------------------------

func TestParseClientConfigurationJSON_ValidJSON(t *testing.T) {
	input := validClientConfigurationJSON()
	cfg, err := parseClientConfigurationJSON(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.ClientID != 1 {
		t.Fatalf("expected ClientID 1, got %d", cfg.ClientID)
	}
}

func TestParseClientConfigurationJSON_InvalidJSON(t *testing.T) {
	_, err := parseClientConfigurationJSON("{not valid json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseClientConfigurationJSON_ValidJSONFailsValidate(t *testing.T) {
	// ClientID=0 will fail Validate()
	input := `{"ClientID":0,"Protocol":"TCP"}`
	_, err := parseClientConfigurationJSON(input)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestParseClientConfigurationJSON_ControlCharacters(t *testing.T) {
	// Embed zero-width spaces and other control chars around valid JSON.
	raw := validClientConfigurationJSON()
	withControl := "\u200b" + raw + "\u200b"
	cfg, err := parseClientConfigurationJSON(withControl)
	if err != nil {
		t.Fatalf("expected control chars to be stripped and parse to succeed, got %v", err)
	}
	if cfg.ClientID != 1 {
		t.Fatalf("expected ClientID 1, got %d", cfg.ClientID)
	}
}

// ---------------------------------------------------------------------------
// sanitizeConfigurationJSON
// ---------------------------------------------------------------------------

func TestSanitizeConfigurationJSON_NormalString(t *testing.T) {
	input := `{"key":"value"}`
	result := sanitizeConfigurationJSON(input)
	if result != input {
		t.Fatalf("expected %q, got %q", input, result)
	}
}

func TestSanitizeConfigurationJSON_ControlCharsStripped(t *testing.T) {
	// \x00 (NUL), \x01 (SOH), \u200b (zero-width space, category Cf)
	input := "abc\x00\x01\u200bdef"
	result := sanitizeConfigurationJSON(input)
	if result != "abcdef" {
		t.Fatalf("expected %q, got %q", "abcdef", result)
	}
}

func TestSanitizeConfigurationJSON_WhitespacePreserved(t *testing.T) {
	input := "{\n  \"key\": \"value\"\n}\t"
	result := sanitizeConfigurationJSON(input)
	if result != input {
		t.Fatalf("expected whitespace preserved %q, got %q", input, result)
	}
}

func TestSanitizeConfigurationJSON_EmptyString(t *testing.T) {
	result := sanitizeConfigurationJSON("")
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// summarizeInvalidConfigurationError
// ---------------------------------------------------------------------------

func TestSummarizeInvalidConfigurationError_NilError(t *testing.T) {
	result := summarizeInvalidConfigurationError(nil)
	if result != "" {
		t.Fatalf("expected empty string for nil error, got %q", result)
	}
}

func TestSummarizeInvalidConfigurationError_ShortMessage(t *testing.T) {
	err := errors.New("something went wrong")
	result := summarizeInvalidConfigurationError(err)
	if result != "something went wrong" {
		t.Fatalf("expected %q, got %q", "something went wrong", result)
	}
}

func TestSummarizeInvalidConfigurationError_LongMessageTruncated(t *testing.T) {
	long := strings.Repeat("x", 200)
	err := errors.New(long)
	result := summarizeInvalidConfigurationError(err)
	if len(result) != 120 {
		t.Fatalf("expected truncated length 120, got %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Fatalf("expected truncated message to end with '...', got %q", result)
	}
}

func TestSummarizeInvalidConfigurationError_StripsPrefix(t *testing.T) {
	err := errors.New("invalid client configuration (tcp): port must be > 0")
	result := summarizeInvalidConfigurationError(err)
	if result != "port must be > 0" {
		t.Fatalf("expected prefix stripped, got %q", result)
	}
}

func TestSummarizeInvalidConfigurationError_NormalizesSpaces(t *testing.T) {
	err := errors.New("too   many    spaces   here")
	result := summarizeInvalidConfigurationError(err)
	if result != "too many spaces here" {
		t.Fatalf("expected normalized spaces, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// isInvalidClientConfigurationError
// ---------------------------------------------------------------------------

func TestIsInvalidClientConfigurationError_Nil(t *testing.T) {
	if isInvalidClientConfigurationError(nil) {
		t.Fatal("expected false for nil error")
	}
}

func TestIsInvalidClientConfigurationError_InvalidClientConfiguration(t *testing.T) {
	err := errors.New("invalid client configuration: bad field")
	if !isInvalidClientConfigurationError(err) {
		t.Fatal("expected true for 'invalid client configuration'")
	}
}

func TestIsInvalidClientConfigurationError_InvalidCharacter(t *testing.T) {
	err := errors.New("invalid character 'x' in string")
	if !isInvalidClientConfigurationError(err) {
		t.Fatal("expected true for 'invalid character'")
	}
}

func TestIsInvalidClientConfigurationError_CannotUnmarshal(t *testing.T) {
	err := errors.New("cannot unmarshal number into Go struct field")
	if !isInvalidClientConfigurationError(err) {
		t.Fatal("expected true for 'cannot unmarshal'")
	}
}

func TestIsInvalidClientConfigurationError_UnexpectedEOF(t *testing.T) {
	err := errors.New("unexpected eof")
	if !isInvalidClientConfigurationError(err) {
		t.Fatal("expected true for 'unexpected eof'")
	}
}

func TestIsInvalidClientConfigurationError_UnrelatedError(t *testing.T) {
	err := errors.New("network timeout while connecting")
	if isInvalidClientConfigurationError(err) {
		t.Fatal("expected false for unrelated error")
	}
}

// ---------------------------------------------------------------------------
// serverPeerDisplayName
// ---------------------------------------------------------------------------

func TestServerPeerDisplayName_WithName(t *testing.T) {
	peer := serverConfiguration.AllowedPeer{Name: "alpha", ClientID: 1}
	result := serverPeerDisplayName(peer)
	if result != "alpha" {
		t.Fatalf("expected %q, got %q", "alpha", result)
	}
}

func TestServerPeerDisplayName_EmptyName(t *testing.T) {
	peer := serverConfiguration.AllowedPeer{Name: "", ClientID: 42}
	result := serverPeerDisplayName(peer)
	if result != "client-42" {
		t.Fatalf("expected %q, got %q", "client-42", result)
	}
}

func TestServerPeerDisplayName_WhitespaceOnlyName(t *testing.T) {
	peer := serverConfiguration.AllowedPeer{Name: "   \t  ", ClientID: 7}
	result := serverPeerDisplayName(peer)
	if result != "client-7" {
		t.Fatalf("expected %q, got %q", "client-7", result)
	}
}

// ---------------------------------------------------------------------------
// newConfiguratorSessionModel
// ---------------------------------------------------------------------------

func TestNewConfiguratorSessionModel_AllDependencies(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ServerConfigManager: &sessionServerConfigManagerStub{},
	}
	model, err := newConfiguratorSessionModel(opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if model.screen != configuratorScreenMode {
		t.Fatalf("expected initial screen to be mode, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_MissingObserver(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ServerConfigManager: &sessionServerConfigManagerStub{},
	}
	_, err := newConfiguratorSessionModel(opts)
	if err == nil {
		t.Fatal("expected error for missing Observer, got nil")
	}
}

func TestNewConfiguratorSessionModel_MissingSelector(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ServerConfigManager: &sessionServerConfigManagerStub{},
	}
	_, err := newConfiguratorSessionModel(opts)
	if err == nil {
		t.Fatal("expected error for missing Selector, got nil")
	}
}

func TestNewConfiguratorSessionModel_MissingCreator(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Deleter:             sessionDeleterStub{},
		ServerConfigManager: &sessionServerConfigManagerStub{},
	}
	_, err := newConfiguratorSessionModel(opts)
	if err == nil {
		t.Fatal("expected error for missing Creator, got nil")
	}
}

func TestNewConfiguratorSessionModel_MissingDeleter(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		ServerConfigManager: &sessionServerConfigManagerStub{},
	}
	_, err := newConfiguratorSessionModel(opts)
	if err == nil {
		t.Fatal("expected error for missing Deleter, got nil")
	}
}

func TestNewConfiguratorSessionModel_MissingServerConfigManager(t *testing.T) {
	opts := ConfiguratorSessionOptions{
		Observer: sessionObserverStub{},
		Selector: sessionSelectorStub{},
		Creator:  sessionCreatorStub{},
		Deleter:  sessionDeleterStub{},
	}
	_, err := newConfiguratorSessionModel(opts)
	if err == nil {
		t.Fatal("expected error for missing ServerConfigManager, got nil")
	}
}

// ---------------------------------------------------------------------------
// writeServerClientConfigFile
// ---------------------------------------------------------------------------

func withBubbleTeaResolveServerConfigDir(t *testing.T, fn func() (string, error)) {
	t.Helper()
	prev := resolveServerConfigDir
	resolveServerConfigDir = fn
	t.Cleanup(func() { resolveServerConfigDir = prev })
}

func TestDefaultWriteServerClientConfigFile_WritesCorrectPathAndContent(t *testing.T) {
	tmpDir := t.TempDir()
	withBubbleTeaResolveServerConfigDir(t, func() (string, error) { return tmpDir, nil })

	data := []byte(`{"clientID":5}`)
	path, err := writeServerClientConfigFile(5, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(tmpDir, "client_configuration.json.5")
	if path != expected {
		t.Fatalf("expected path %s, got %s", expected, path)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("failed to read written file: %v", readErr)
	}
	if string(got) != string(data) {
		t.Fatalf("expected content %q, got %q", data, got)
	}
}

func TestDefaultWriteServerClientConfigFile_ResolverError(t *testing.T) {
	withBubbleTeaResolveServerConfigDir(t, func() (string, error) {
		return "", errors.New("resolver broken")
	})

	_, err := writeServerClientConfigFile(1, []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "resolver broken") {
		t.Fatalf("expected resolver error, got %v", err)
	}
}
