package bubble_tea

import (
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"

	appConfiguration "tungo/application/configuration"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
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
	peer := appConfiguration.ServerPeer{Name: "alpha", ClientID: 1}
	result := serverPeerDisplayName(peer)
	if result != "alpha" {
		t.Fatalf("expected %q, got %q", "alpha", result)
	}
}

func TestServerPeerDisplayName_EmptyName(t *testing.T) {
	peer := appConfiguration.ServerPeer{Name: "", ClientID: 42}
	result := serverPeerDisplayName(peer)
	if result != "client-42" {
		t.Fatalf("expected %q, got %q", "client-42", result)
	}
}

func TestServerPeerDisplayName_WhitespaceOnlyName(t *testing.T) {
	peer := appConfiguration.ServerPeer{Name: "   \t  ", ClientID: 7}
	result := serverPeerDisplayName(peer)
	if result != "client-7" {
		t.Fatalf("expected %q, got %q", "client-7", result)
	}
}

// ---------------------------------------------------------------------------
// newConfiguratorSessionModel
// ---------------------------------------------------------------------------

func TestNewConfiguratorSessionModel_AllDependencies(t *testing.T) {
	opts := sessionOptionsWithControl(defaultSessionConfigurationControl())
	model, err := newConfiguratorSessionModel(opts, testSettings())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if model.screen != configuratorScreenMode {
		t.Fatalf("expected initial screen to be mode, got %v", model.screen)
	}
}

func TestNewConfiguratorSessionModel_MissingClientConfigurationControl(t *testing.T) {
	opts := ConfiguratorSessionOptions{}
	_, err := newConfiguratorSessionModel(opts, testSettings())
	if err == nil {
		t.Fatal("expected error for missing client configuration control, got nil")
	}
}
