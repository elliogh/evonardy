// Package storage publishes immutable local files under an exclusively owned root.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Store struct {
	root *os.Root
	lock *os.File
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	lock, err := root.OpenFile(".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	if err := lockFile(lock); err != nil {
		lock.Close()
		root.Close()
		return nil, fmt.Errorf("data-dir is owned by another writer or locking is unavailable: %w", err)
	}
	return &Store{root: root, lock: lock}, nil
}

func (s *Store) Close() error { return errors.Join(s.root.Close(), s.lock.Close()) }

// WriteNew syncs a temporary file and atomically links it to the final name.
// Link fails if the target already exists, preserving immutable artifacts.
func (s *Store) WriteNew(name string, data []byte) error {
	if !filepath.IsLocal(name) || name == "." {
		return fmt.Errorf("invalid relative artifact path")
	}
	dir := filepath.Dir(name)
	if err := s.root.MkdirAll(dir, 0700); err != nil {
		return err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	temp := filepath.Join(dir, ".tmp-"+hex.EncodeToString(token[:]))
	f, err := s.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer s.root.Remove(temp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := s.root.Link(temp, name); err != nil {
		return err
	}
	// Directory fsync is best effort: not every filesystem supports it.
	if directory, err := s.root.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}
