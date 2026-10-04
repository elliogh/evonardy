// Package storage publishes immutable local files under an exclusively owned root.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

type Store struct {
	root *os.Root
	lock *os.File
	mu   sync.Mutex
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
	return s.write(name, data, false)
}

// WriteAtomic replaces mutable session or metadata snapshots, never model files.
func (s *Store) WriteAtomic(name string, data []byte) error {
	return s.write(name, data, true)
}

func (s *Store) write(name string, data []byte, replace bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	if replace {
		err = s.root.Rename(temp, name)
	} else {
		err = s.root.Link(temp, name)
	}
	if err != nil {
		return err
	}
	// Directory fsync is best effort: not every filesystem supports it.
	if directory, err := s.root.Open(dir); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func (s *Store) Read(name string, limit int64) ([]byte, error) {
	if !filepath.IsLocal(name) || limit < 1 {
		return nil, fmt.Errorf("invalid artifact read")
	}
	f, err := s.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("artifact is not a regular file within its size limit")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("artifact exceeds size limit")
	}
	return data, nil
}

func (s *Store) List(name string) ([]os.DirEntry, error) {
	if !filepath.IsLocal(name) {
		return nil, fmt.Errorf("invalid directory")
	}
	f, err := s.root.Open(name)
	if os.IsNotExist(err) {
		return []os.DirEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(10001)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(entries) > 10000 {
		return nil, fmt.Errorf("artifact directory exceeds 10000 entries")
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		if a.Name() < b.Name() {
			return -1
		}
		if a.Name() > b.Name() {
			return 1
		}
		return 0
	})
	return entries, nil
}

// PublishNew makes a complete immutable bundle visible in one directory rename.
// The data-directory lock and Store mutex serialize writers; existing targets fail.
func (s *Store) PublishNew(name string, files map[string][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !filepath.IsLocal(name) || name == "." || len(files) == 0 {
		return fmt.Errorf("invalid package path")
	}
	if _, err := s.root.Lstat(name); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(name)
	if err := s.root.MkdirAll(parent, 0700); err != nil {
		return err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	temp := filepath.Join(parent, ".tmp-"+hex.EncodeToString(token[:]))
	if err := s.root.Mkdir(temp, 0700); err != nil {
		return err
	}
	defer s.root.RemoveAll(temp)
	for file, data := range files {
		if file == "." || filepath.Base(file) != file || !filepath.IsLocal(file) {
			return fmt.Errorf("invalid bundle filename")
		}
		f, err := s.root.OpenFile(filepath.Join(temp, file), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
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
	}
	if dir, err := s.root.Open(temp); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	if err := s.root.Rename(temp, name); err != nil {
		return err
	}
	if dir, err := s.root.Open(parent); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
