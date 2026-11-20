// Package applog provides a thread-safe categorized ring buffer for live runtime logs.
package applog

import (
	"sync"
	"time"
)

type Entry struct {
	Time     string `json:"time"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

type Buffer struct {
	mu      sync.Mutex
	max     int
	entries []Entry
}

var Global = New(500)

func New(max int) *Buffer {
	if max <= 0 {
		max = 500
	}
	return &Buffer{max: max, entries: []Entry{}}
}

func (b *Buffer) Append(category, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, Entry{
		Time:     time.Now().Format("15:04:05"),
		Category: category,
		Message:  message,
	})
	if len(b.entries) > b.max {
		b.entries = b.entries[len(b.entries)-b.max:]
	}
}

func (b *Buffer) List(category string) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Entry, 0, len(b.entries))
	for _, e := range b.entries {
		if category == "" || category == "all" || e.Category == category {
			out = append(out, e)
		}
	}
	return out
}

func (b *Buffer) Clear(category string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if category == "" || category == "all" {
		b.entries = []Entry{}
		return
	}
	kept := make([]Entry, 0, len(b.entries))
	for _, e := range b.entries {
		if e.Category != category {
			kept = append(kept, e)
		}
	}
	b.entries = kept
}
