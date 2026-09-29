// Package sample contains functions for trying tastecheck against the
// builtin uber-go ruleset: one idiomatic function and one that violates
// several of its rules.
package sample

import (
	"errors"
	"log"
	"os"
	"sync"
)

// Counter is a concurrency-safe counter.
type Counter struct {
	mu    sync.Mutex
	count int
}

// Incr increments the counter and returns the new value.
func (c *Counter) Incr() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count++
	return c.count
}

// Load reads the file at path, logs and returns errors, exits on a missing
// file, and starts a background goroutine that is never stopped.
func Load(path string, v any) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Fatal("failed to find file: " + path)
		}
		log.Printf("failed to read file: %v", err)
		return nil, errors.New("failed to read file: " + err.Error())
	}
	s := v.(string)
	go func() {
		for {
			log.Println("loaded", s)
		}
	}()
	return b, nil
}
