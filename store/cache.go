// Copyright 2015, David Howden
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package store

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/net/context"
)

// RWFileSystem is an interface which includes http.FileSystem and a Create method
// for creating files.
type RWFileSystem interface {
	FileSystem

	// Create a file with associated path, returns an io.WriteCloser.  Only when Close()
	// returns can it be assumed that the file has been written.
	Create(ctx context.Context, path string) (io.WriteCloser, error)

	// Wait blocks until any pending write calls have been completed.
	Wait() error
}

// dir implements RWFileSystem by extending the behaviour of http.Dir to include a Create
// method which creates files under the root.
type dir struct {
	FileSystem

	root string
}

// Dir creates a new RWFileSystem with the specified root (similar to http.Dir)
func Dir(root string) RWFileSystem {
	return &dir{
		NewFileSystem(http.Dir(root), fmt.Sprintf("local (%v)", root)),
		root,
	}
}

func (d *dir) absPath(path string) (string, error) {
	cleanPath := filepath.Clean(d.root + "/" + path)
	path, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("error finding absolute path for: '%v' ('%v'): %v", path, cleanPath, err)
	}

	absRoot, err := filepath.Abs(d.root)
	if err != nil {
		return "", fmt.Errorf("error finding absolute path for root '%v': %v", d.root, err)
	}

	if !strings.HasPrefix(filepath.Dir(path), absRoot) {
		return "", fmt.Errorf("invalid path ('%v' is outside '%v'): %v", filepath.Dir(path), absRoot, path)
	}
	return path, nil
}

// Create a file rooted in the Dir file system.
func (d *dir) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	absPath, err := d.absPath(path)
	if err != nil {
		return nil, err
	}
	err = os.MkdirAll(filepath.Dir(absPath), os.ModePerm)
	if err != nil {
		return nil, err
	}
	return os.Create(absPath)
}

// Wait implements RWFileSystem.
func (d *dir) Wait() error { return nil }

// CachedFileSystem is an implemetation of http.FileServer which caches the results of
// calls to src in a RWFileSystem.
type CachedFileSystem struct {
	src   FileSystem
	cache RWFileSystem

	mu       sync.RWMutex // protects errCache
	errCache map[string]error

	errCh chan<- error
	wg    sync.WaitGroup
}

// setError sets an error for a path.  This is intended to prevent the src being continually
// queried after returning a non-temporary failure to retrieve a path.
// TODO: Better handle temporary errors!
func (c *CachedFileSystem) setError(path string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.errCache[path] = err
}

func (c *CachedFileSystem) error(path string) (err error, exists bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	err, ok := c.errCache[path]
	return err, ok
}

// Open implements FileSystem.  If the required file isn't in the cache
// then the file is opened from the src, and then concurrently copied into the
// cache (with errors passed back on the filesystem error channel).
func (c *CachedFileSystem) Open(ctx context.Context, path string) (http.File, error) {
	if err, ok := c.error(path); ok {
		return nil, fmt.Errorf("cached error: %v", err)
	}

	f, err := c.cache.Open(ctx, path)
	if err == nil {
		return f, nil
	}

	f, err = c.src.Open(ctx, path)
	if err != nil {
		c.setError(path, err)
		return nil, err
	}

	go func() { // TODO: improve this so that we don't have to fetch the file again!
		c.wg.Add(1)
		defer c.wg.Done()

		src, err := c.src.Open(ctx, path)
		if err != nil {
			c.errCh <- fmt.Errorf("error opening file for second time: %v", err)
			return
		}
		defer func() {
			err := src.Close()
			if err != nil {
				c.errCh <- err
			}
		}()

		cache, err := c.cache.Create(ctx, path)
		if err != nil {
			c.errCh <- fmt.Errorf("error creating file in cache: %v", err)
			return
		}
		defer func() {
			err := cache.Close()
			if err != nil {
				c.errCh <- err
			}
		}()

		_, err = io.Copy(cache, src)
		if err != nil {
			c.errCh <- fmt.Errorf("error copying src file data into cache: %v", err)
		}
	}()

	return f, nil
}

// Wait implements RWFileSystem.
func (c *CachedFileSystem) Wait() error {
	c.wg.Wait()
	return nil
}

// NewCachedFileSystem implements http.FileSystem and caches every request made to
// src in cache.  The returned error channel passes back any errors which occur when
// files are being concurrently copied into the cache.
func NewCachedFileSystem(src FileSystem, cache RWFileSystem) (*CachedFileSystem, <-chan error) {
	errCh := make(chan error)
	return &CachedFileSystem{
		src:      src,
		cache:    cache,
		errCh:    errCh,
		errCache: make(map[string]error),
	}, errCh
}
