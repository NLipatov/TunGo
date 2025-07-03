package server_configuration

import "time"

type ttlReader struct {
	reader         reader
	ttl            time.Duration
	cache          *Configuration
	cacheExpiresAt time.Time
}

func NewTTLReader(reader reader, ttl time.Duration) *ttlReader {
	return &ttlReader{
		reader: reader,
		ttl:    ttl,
	}
}

func (t *ttlReader) read() (*Configuration, error) {
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
