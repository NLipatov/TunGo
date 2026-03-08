package infrastructure

import "os"

// Hooks provide OS interaction seams for testability.
type Hooks struct {
	Stat      func(string) (os.FileInfo, error)
	Lstat     func(string) (os.FileInfo, error)
	LookPath  func(string) (string, error)
	WriteFile func(string, []byte, os.FileMode) error
	ReadFile  func(string) ([]byte, error)
	Remove    func(string) error
	Geteuid   func() int
}
