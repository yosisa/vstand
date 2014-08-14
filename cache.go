package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type ByModTime []os.FileInfo

func (s ByModTime) Len() int {
	return len(s)
}

func (s ByModTime) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s ByModTime) Less(i, j int) bool {
	return s[i].ModTime().Before(s[j].ModTime())
}

type CacheDir struct {
	path    string
	maxSize int64
	Size    int64
	items   []os.FileInfo
	closeCh chan struct{}
}

func NewCacheDir(path string, maxSize int64) *CacheDir {
	c := &CacheDir{
		path:    path,
		maxSize: maxSize,
		closeCh: make(chan struct{}),
	}
	if maxSize > 0 {
		go c.KeepSize()
	}
	return c
}

func (c *CacheDir) Update() error {
	files, err := ioutil.ReadDir(c.path)
	if err != nil {
		return err
	}

	var items []os.FileInfo
	var total int64
	for _, fi := range files {
		if !fi.IsDir() {
			items = append(items, fi)
			total += fi.Size()
		}
	}
	sort.Sort(ByModTime(items))
	c.items = items
	c.Size = total
	log.Printf("Cache dir size: %d", c.Size)

	return nil
}

func (c *CacheDir) Shrink() {
	if c.maxSize == 0 {
		return
	}

	for c.Size > c.maxSize {
		item := c.items[0]
		c.items = c.items[1:]

		log.Printf("Remove %s", item.Name())
		if err := os.Remove(filepath.Join(c.path, item.Name())); err == nil {
			c.Size -= item.Size()
		}
	}
}

func (c *CacheDir) KeepSize() {
	c.Update()
	c.Shrink()
	tick := time.Tick(time.Minute)
	for {
		select {
		case <-tick:
			c.Update()
			c.Shrink()
		case <-c.closeCh:
			return
		}
	}
}

func (c *CacheDir) Close() {
	close(c.closeCh)
}
