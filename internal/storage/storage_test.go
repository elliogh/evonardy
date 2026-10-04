package storage_test

import (
	"evonardy/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirLockAndAtomicImmutableFile(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := storage.Open(dir); err == nil {
		other.Close()
		t.Fatal("second writer accepted")
	}
	if err := s.WriteNew("replays/one.json", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteNew("replays/one.json", []byte("two")); err == nil {
		t.Fatal("published file overwritten")
	}
	data, err := os.ReadFile(filepath.Join(dir, "replays/one.json"))
	if err != nil || string(data) != "one" {
		t.Fatal("immutable publication changed")
	}
	if err := s.WriteNew("../escape", []byte("bad")); err == nil {
		t.Fatal("path escape accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
}

func TestStorageCannotFollowSymlinkOutsideDataDir(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Skip(err)
	}
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.WriteNew("escape/file", []byte("bad")); err == nil {
		t.Fatal("external symlink followed")
	}
	if _, err := os.Stat(filepath.Join(outside, "file")); !os.IsNotExist(err) {
		t.Fatal("file escaped root")
	}
}
