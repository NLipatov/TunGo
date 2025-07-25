package server

import "time"

type TTLReader struct {
	reader         Reader
	ttl            time.Duration
	cache          *Configuration
	cacheExpiresAt time.Time
}

func NewTTLReader(reader Reader, ttl time.Duration) *TTLReader {
	return &TTLReader{
		reader: reader,
		ttl:    ttl,
	}
}

func (t *TTLReader) read() (*Configuration, error) {
	if t.cache != nil && time.Now().Before(t.cacheExpiresAt) {
		return t.cache, nil
	}

	configuration, err := t.reader.read()
	if err != nil {
		return nil, err
	}

	t.cache = configuration
	t.cacheExpiresAt = time.Now().Add(t.ttl)
	return configuration, nil
}
