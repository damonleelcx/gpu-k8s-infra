package queue

import (
	"context"
	"sync"
	"time"
)

// Task represents a submitted GPU task (backed by K8s Job name).
type Task struct {
	ID        string            `json:"id"`
	JobName   string            `json:"jobName"`
	Spec      interface{}       `json:"spec,omitempty"`
	Status    string            `json:"status"` // queued, running, succeeded, failed
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// Store is an in-memory task queue store (production could use Redis/DB).
type Store struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

// NewStore creates a new in-memory task store.
func NewStore() *Store {
	return &Store{tasks: make(map[string]*Task)}
}

// Put saves or updates a task.
func (s *Store) Put(ctx context.Context, t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	s.tasks[t.ID] = t
	return nil
}

// Get returns a task by ID.
func (s *Store) Get(ctx context.Context, id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, nil
	}
	return t, nil
}

// GetByJobName returns a task by K8s job name.
func (s *Store) GetByJobName(ctx context.Context, jobName string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tasks {
		if t.JobName == jobName {
			return t, nil
		}
	}
	return nil, nil
}

// List returns all tasks, optionally filtered by status.
func (s *Store) List(ctx context.Context, statusFilter string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Task
	for _, t := range s.tasks {
		if statusFilter == "" || t.Status == statusFilter {
			out = append(out, t)
		}
	}
	return out, nil
}

// UpdateStatus sets status and UpdatedAt.
func (s *Store) UpdateStatus(ctx context.Context, id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil
	}
	t.Status = status
	t.UpdatedAt = time.Now().UTC()
	return nil
}

// Delete removes a task.
func (s *Store) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, id)
	return nil
}
