package lmsensors

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// A filesystem is an interface to a filesystem, used for testing.
type filesystem interface {
	ReadDir(name string) ([]fs.DirEntry, error)
	ReadFile(filename string) (string, error)
	Readlink(name string) (string, error)
	Stat(name string) (os.FileInfo, error)
	WalkDir(root string, walkFn fs.WalkDirFunc) error
}

// A systemFilesystem is a filesystem which uses operations on the host filesystem.
type systemFilesystem struct{}

func (s *systemFilesystem) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (s *systemFilesystem) ReadFile(filename string) (string, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (s *systemFilesystem) Readlink(name string) (string, error) {
	return os.Readlink(name)
}

func (s *systemFilesystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (s *systemFilesystem) WalkDir(root string, walkFn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, walkFn)
}
