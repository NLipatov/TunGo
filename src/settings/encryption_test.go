package settings

import (
	"encoding/json"
	"testing"
)

func TestEncryption_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		value   Encryption
		want    string
		wantErr bool
	}{
		{"valid ChaCha20Poly1305", ChaCha20Poly1305, `"ChaCha20Poly1305"`, false},
		{"invalid value", Encryption(99), ``, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.value.MarshalJSON()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if string(data) != tt.want {
					t.Errorf("got %s, want %s", data, tt.want)
				}
			}
		})
	}
}

func TestEncryption_UnmarshalJSON(t *testing.T) {
	t.Run("valid ChaCha20Poly1305", func(t *testing.T) {
		var e Encryption
		err := e.UnmarshalJSON([]byte(`"ChaCha20Poly1305"`))
		if err != nil {
			t.Fatalf("UnmarshalJSON() error = %v, want nil", err)
		}
		if e != ChaCha20Poly1305 {
			t.Errorf("got %v, want %v", e, ChaCha20Poly1305)
		}
	})

	t.Run("invalid string", func(t *testing.T) {
		var e Encryption
		err := e.UnmarshalJSON([]byte(`"BadAlgo"`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid JSON type", func(t *testing.T) {
		var e Encryption
		err := e.UnmarshalJSON([]byte(`123`))
		if err == nil {
			t.Fatal("expected error on non-string JSON, got nil")
		}
	})
}

func TestEncryptionJSON_RoundTrip(t *testing.T) {
	orig := ChaCha20Poly1305
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	var got Encryption
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if got != orig {
		t.Errorf("round-trip: got %v, want %v", got, orig)
	}
}
