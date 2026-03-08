package mcp

import (
	"sync"
	"time"
)

type Activity struct {
	Timestamp time.Time `json:"timestamp"`
	Actor     string    `json:"actor"`
	Tool      string    `json:"tool"`
	Detail    string    `json:"detail"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

var activityStore = struct {
	sync.RWMutex
	events []Activity
}{}

func recordActivity(event Activity) {
	activityStore.Lock()
	defer activityStore.Unlock()
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	activityStore.events = append(activityStore.events, event)
	if len(activityStore.events) > 500 {
		activityStore.events = activityStore.events[len(activityStore.events)-500:]
	}
}

func RecentActivity(limit int) []Activity {
	activityStore.RLock()
	defer activityStore.RUnlock()
	if limit <= 0 || limit > len(activityStore.events) {
		limit = len(activityStore.events)
	}
	start := len(activityStore.events) - limit
	out := make([]Activity, limit)
	copy(out, activityStore.events[start:])
	return out
}
