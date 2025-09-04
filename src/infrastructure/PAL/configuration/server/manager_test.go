package server

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// --- Mocks ---

type ManagerMockResolver struct {
	Path string
	Err  error
}

func (m *ManagerMockResolver) Resolve() (string, error) {
	return m.Path, m.Err
}

type ManagerMockStat struct {
	Err error
}

func (m *ManagerMockStat) Stat(_ string) (os.FileInfo, error) {
	return nil, m.Err
}

type ManagerMockWriter struct {
	WrittenData interface{}
	Err         error
	WriteCalls  int
}

func (m *ManagerMockWriter) Write(data interface{}) error {
	m.WriteCalls++
	m.WrittenData = data
	return m.Err
}

type ManagerMockReader struct {
	Config    *Configuration
	Err       error
	ReadCalls int
}

func (m *ManagerMockReader) read() (*Configuration, error) {
	m.ReadCalls++
	return m.Config, m.Err
}

// --- Tests ---

func TestManager_Configuration_FileNotExists_WritesDefault(t *testing.T) {
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: fs.ErrNotExist}
	writer := &ManagerMockWriter{}
	defaultConf := NewDefaultConfiguration()
	reader := &ManagerMockReader{Config: defaultConf}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	conf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if writer.WriteCalls != 1 {
		t.Errorf("expected Write to be called once, got %d", writer.WriteCalls)
	}

	confWritten, ok := writer.WrittenData.(Configuration)
	if !ok {
		t.Fatalf("written data is not Configuration")
	}

	defaultConfData, defaultConfErr := json.Marshal(defaultConf)
	if defaultConfErr != nil {
		t.Fatalf("unexpected error: %v", defaultConfErr)
	}

	confData, confErr := json.Marshal(confWritten)
	if confErr != nil {
		t.Fatalf("unexpected error: %v", confErr)
	}

	if !bytes.Equal(confData, defaultConfData) {
		t.Errorf("expected written config to equal defaultConfig")
	}

	if conf != defaultConf {
		t.Errorf("expected returned config to be defaultConf pointer")
	}
}

func TestManager_Configuration_WriteDefaultError(t *testing.T) {
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: fs.ErrNotExist}
	writer := &ManagerMockWriter{Err: errors.New("write fail")}
	reader := &ManagerMockReader{Config: NewDefaultConfiguration()}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	_, err := manager.Configuration()
	if err == nil || !strings.Contains(err.Error(), "could not write default configuration") {
		t.Fatalf("expected write default error, got: %v", err)
	}
}

func TestManager_Configuration_StatError_ReturnsError(t *testing.T) {
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: errors.New("permission denied")}
	writer := &ManagerMockWriter{}
	reader := &ManagerMockReader{}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected error due to stat error, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestManager_Configuration_ResolverError_ReturnsError(t *testing.T) {
	resolver := &ManagerMockResolver{Err: errors.New("resolve error")}
	statMock := &ManagerMockStat{}
	writer := &ManagerMockWriter{}
	reader := &ManagerMockReader{}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected error due to resolver error, got nil")
	}
	if !strings.Contains(err.Error(), "resolve error") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestManager_Configuration_ReaderError_ReturnsError(t *testing.T) {
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: nil}
	writer := &ManagerMockWriter{}
	reader := &ManagerMockReader{Err: errors.New("read error")}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected error due to reader error, got nil")
	}
	if !strings.Contains(err.Error(), "read error") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestManager_IncrementClientCounter_Success(t *testing.T) {
	initialConf := NewDefaultConfiguration()
	initialValue := initialConf.ClientCounter

	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: nil}
	writer := &ManagerMockWriter{}
	reader := &ManagerMockReader{Config: initialConf}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	err := manager.IncrementClientCounter()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if writer.WriteCalls != 1 {
		t.Errorf("expected writer.Write called once, got %d", writer.WriteCalls)
	}

	confWritten, ok := writer.WrittenData.(Configuration)
	if !ok {
		t.Fatalf("written data is not Configuration")
	}

	expected := initialValue + 1
	if confWritten.ClientCounter != expected {
		t.Errorf("expected ClientCounter %d, got %d", expected, confWritten.ClientCounter)
	}
}

func TestManager_IncrementClientCounter_ConfigError(t *testing.T) {
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: nil}
	writer := &ManagerMockWriter{}
	reader := &ManagerMockReader{Err: errors.New("read error")}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	err := manager.IncrementClientCounter()
	if err == nil {
		t.Fatal("expected error due to config read failure, got nil")
	}
	if !strings.Contains(err.Error(), "read error") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestManager_IncrementClientCounter_WriteError(t *testing.T) {
	initialConf := NewDefaultConfiguration()
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: nil}
	writer := &ManagerMockWriter{Err: errors.New("write fail")}
	reader := &ManagerMockReader{Config: initialConf}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	err := manager.IncrementClientCounter()
	if err == nil || !strings.Contains(err.Error(), "write fail") {
		t.Fatalf("expected writer error, got: %v", err)
	}
}

func TestManager_InjectEdKeys_Success(t *testing.T) {
	initialConf := NewDefaultConfiguration()
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: nil}
	writer := &ManagerMockWriter{}
	reader := &ManagerMockReader{Config: initialConf}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	pub, priv, _ := ed25519.GenerateKey(nil)

	err := manager.InjectEdKeys(pub, priv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if writer.WriteCalls != 1 {
		t.Errorf("expected writer.Write called once, got %d", writer.WriteCalls)
	}

	confWritten, ok := writer.WrittenData.(Configuration)
	if !ok {
		t.Fatalf("written data is not Configuration")
	}

	if !pub.Equal(confWritten.Ed25519PublicKey) {
		t.Errorf("public key mismatch")
	}
	if !priv.Equal(confWritten.Ed25519PrivateKey) {
		t.Errorf("private key mismatch")
	}
}

func TestManager_InjectEdKeys_ConfigError(t *testing.T) {
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: nil}
	writer := &ManagerMockWriter{}
	reader := &ManagerMockReader{Err: errors.New("read error")}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	err := manager.InjectEdKeys(nil, nil)
	if err == nil {
		t.Fatal("expected error due to config read failure, got nil")
	}
	if !strings.Contains(err.Error(), "invalid public key length") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestManager_InjectEdKeys_WriteError(t *testing.T) {
	initialConf := NewDefaultConfiguration()
	resolver := &ManagerMockResolver{Path: "/fake/path"}
	statMock := &ManagerMockStat{Err: nil}
	writer := &ManagerMockWriter{Err: errors.New("write fail")}
	reader := &ManagerMockReader{Config: initialConf}

	manager := &Manager{
		resolver: resolver,
		stat:     statMock,
		writer:   writer,
		reader:   reader,
	}

	pub, priv, _ := ed25519.GenerateKey(nil)

	err := manager.InjectEdKeys(pub, priv)
	if err == nil || !strings.Contains(err.Error(), "write fail") {
		t.Fatalf("expected writer error, got: %v", err)
	}
}

// --- NewManager / NewManagerWithReader constructors

func TestNewManager_ErrorFromResolver(t *testing.T) {
	resolver := &ManagerMockResolver{Err: errors.New("resolve error")}
	_, err := NewManager(resolver, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve server configuration path") {
		t.Fatalf("expected resolve error, got %v", err)
	}
}

func TestNewManagerWithReader_ErrorFromResolver(t *testing.T) {
	resolver := &ManagerMockResolver{Err: errors.New("resolve error")}
	_, err := NewManagerWithReader(resolver, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve server configuration path") {
		t.Fatalf("expected resolve error, got %v", err)
	}
}
