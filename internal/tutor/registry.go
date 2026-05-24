package tutor

import "sync"

// Registry is a thread-safe holder for the active tutor Service. The admin
// "reload" endpoint atomically swaps the service so questions in flight
// keep using the old instance and new questions pick up the new config.
type Registry struct {
	mu  sync.RWMutex
	svc *Service
}

// NewRegistry returns an empty registry. Set the service before any caller
// asks for it.
func NewRegistry() *Registry { return &Registry{} }

// Get returns the current service or nil if none is registered. Nil is the
// signal the ws handler uses to report "tutor desactivado" gracefully.
func (r *Registry) Get() *Service {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.svc
}

// Set installs a new service and closes the previous one (if any) so its
// background goroutines (rate limiter, ticker) are released. Pass nil to
// disable the tutor entirely.
func (r *Registry) Set(svc *Service) {
	r.mu.Lock()
	old := r.svc
	r.svc = svc
	r.mu.Unlock()
	if old != nil && old != svc {
		old.Close()
	}
}

// Close releases the held service. Safe to call multiple times.
func (r *Registry) Close() {
	r.Set(nil)
}
