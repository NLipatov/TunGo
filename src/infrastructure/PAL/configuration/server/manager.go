package server

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"time"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/stat"
)

type ConfigurationManager interface {
	Configuration() (*Configuration, error)
	IncrementClientCounter() error
	InjectX25519Keys(public, private []byte) error
	AddAllowedPeer(peer AllowedPeer) error
	ListAllowedPeers() ([]AllowedPeer, error)
	SetAllowedPeerEnabled(clientID int, enabled bool) error
	EnsureIPv6Subnets() error
	InvalidateCache()
}

type Manager struct {
	resolver client.Resolver
	reader   Reader
	writer   Writer
	stat     stat.Stat
}

func NewManager(resolver client.Resolver, stat stat.Stat) (ConfigurationManager, error) {
	path, pathErr := resolver.Resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to resolve server configuration path: %w", pathErr)
	}

	return NewManagerWithReader(
		resolver,
		NewTTLReader(newDefaultReader(path, stat), time.Minute*15),
		stat,
	)
}

func NewManagerWithReader(
	resolver client.Resolver,
	reader Reader,
	stat stat.Stat,
) (ConfigurationManager, error) {
	path, pathErr := resolver.Resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to resolve server configuration path: %w", pathErr)
	}

	return &Manager{
		resolver: resolver,
		writer:   newDefaultWriter(path),
		reader:   reader,
		stat:     stat,
	}, nil
}

func (c *Manager) Configuration() (*Configuration, error) {
	path, pathErr := c.resolver.Resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to read configuration: %w", pathErr)
	}

	_, statErr := c.stat.Stat(path)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			configuration := NewDefaultConfiguration()
			writeErr := c.writer.Write(*configuration)
			if writeErr != nil {
				return nil, fmt.Errorf("could not write default configuration: %w", writeErr)
			}
		} else {
			return nil, statErr
		}
	}

	return c.reader.read()
}

func (c *Manager) update(fn func(*Configuration) error) error {
	conf, err := c.Configuration()
	if err != nil {
		return err
	}
	if err := fn(conf); err != nil {
		return err
	}
	return c.writer.Write(*conf)
}

func (c *Manager) IncrementClientCounter() error {
	return c.update(func(conf *Configuration) error {
		conf.ClientCounter++
		return nil
	})
}

func (c *Manager) InjectX25519Keys(public, private []byte) error {
	if len(public) != 32 {
		return fmt.Errorf("invalid public key length: got %d, want 32", len(public))
	}
	if len(private) != 32 {
		return fmt.Errorf("invalid private key length: got %d, want 32", len(private))
	}
	return c.update(func(conf *Configuration) error {
		conf.X25519PublicKey = append([]byte(nil), public...)
		conf.X25519PrivateKey = append([]byte(nil), private...)
		return nil
	})
}

func (c *Manager) AddAllowedPeer(peer AllowedPeer) error {
	if len(peer.PublicKey) != 32 {
		return fmt.Errorf("invalid public key length: got %d, want 32", len(peer.PublicKey))
	}
	return c.update(func(conf *Configuration) error {
		conf.AllowedPeers = append(conf.AllowedPeers, peer)
		return nil
	})
}

func (c *Manager) ListAllowedPeers() ([]AllowedPeer, error) {
	conf, err := c.Configuration()
	if err != nil {
		return nil, err
	}

	peers := make([]AllowedPeer, len(conf.AllowedPeers))
	for i := range conf.AllowedPeers {
		peers[i] = AllowedPeer{
			Name:      conf.AllowedPeers[i].Name,
			PublicKey: append([]byte(nil), conf.AllowedPeers[i].PublicKey...),
			Enabled:   conf.AllowedPeers[i].Enabled,
			ClientID:  conf.AllowedPeers[i].ClientID,
		}
	}
	return peers, nil
}

func (c *Manager) SetAllowedPeerEnabled(clientID int, enabled bool) error {
	if clientID <= 0 {
		return fmt.Errorf("invalid client id %d", clientID)
	}

	return c.update(func(conf *Configuration) error {
		for i := range conf.AllowedPeers {
			if conf.AllowedPeers[i].ClientID != clientID {
				continue
			}
			conf.AllowedPeers[i].Enabled = enabled
			return nil
		}
		return fmt.Errorf("allowed peer with ClientID %d not found", clientID)
	})
}

// EnsureIPv6Subnets sets default IPv6 tunnel subnets if not already configured.
func (c *Manager) EnsureIPv6Subnets() error {
	conf, err := c.Configuration()
	if err != nil {
		return err
	}

	defaults := []netip.Prefix{
		netip.MustParsePrefix("fd00::/64"),
		netip.MustParsePrefix("fd00:1::/64"),
		netip.MustParsePrefix("fd00:2::/64"),
	}
	changed := false
	for i, s := range conf.AllSettingsPtrs() {
		if !s.IPv6Subnet.IsValid() {
			s.IPv6Subnet = defaults[i]
			changed = true
		}
	}
	if !changed {
		return nil
	}

	conf.EnsureDefaults()
	return c.writer.Write(*conf)
}

// InvalidateCache clears the cached configuration if the reader supports it.
// Implements CacheInvalidator interface.
func (c *Manager) InvalidateCache() {
	if ttlReader, ok := c.reader.(*TTLReader); ok {
		ttlReader.InvalidateCache()
	}
}
