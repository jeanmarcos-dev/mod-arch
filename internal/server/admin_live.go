package server

import (
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/store"
	"github.com/jeanmarcos/modos-cpu/internal/telemetry"
)

const (
	liveDefaultWindowMinutes = 120
	liveMaxWindowMinutes     = 1440
	liveCacheTTL             = 2 * time.Second
)

type liveCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type liveCountry struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

type liveNow struct {
	SessionsActive       int         `json:"sessions_active"`
	WSConnections        int64       `json:"ws_connections"`
	SessionsWithUser     int         `json:"sessions_with_user"`
	SessionsAnonymous    int         `json:"sessions_anonymous"`
	PeakSessions         int         `json:"peak_sessions"`
	SessionsCreatedTotal uint64      `json:"sessions_created_total"`
	Sections             []liveCount `json:"sections"`
	Demos                []liveCount `json:"demos"`
}

type liveResponse struct {
	GeneratedAt   int64            `json:"generated_at"`
	WindowMinutes int              `json:"window_minutes"`
	Now           liveNow          `json:"now"`
	Window        store.LiveWindow `json:"window"`
	Tutor         store.LiveTutor  `json:"tutor"`
	Countries     []liveCountry    `json:"countries"`
}

type liveWindowData struct {
	window store.LiveWindow
	tutor  store.LiveTutor
}

type liveCacheEntry struct {
	at   time.Time
	data liveWindowData
}

type liveCache struct {
	mu      sync.Mutex
	entries map[int]liveCacheEntry
}

func (c *liveCache) get(window int, now time.Time, load func() (liveWindowData, error)) (liveWindowData, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[window]; ok && now.Sub(e.at) < liveCacheTTL {
		return e.data, nil
	}
	data, err := load()
	if err != nil {
		return liveWindowData{}, err
	}
	if c.entries == nil {
		c.entries = map[int]liveCacheEntry{}
	}
	c.entries[window] = liveCacheEntry{at: now, data: data}
	return data, nil
}

func (s *Server) requireEnvAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.admin.enabled {
			http.NotFound(w, r)
			return
		}
		c, err := r.Cookie(AdminCookie)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if _, err := s.verifyCookie(c.Value); err != nil {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

func parseLiveWindow(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return liveDefaultWindowMinutes
	}
	return min(n, liveMaxWindowMinutes)
}

func (s *Server) handleAdminLive(w http.ResponseWriter, r *http.Request) {
	st := s.manager.Store()
	if st == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "persistence disabled"})
		return
	}
	window := parseLiveWindow(r.URL.Query().Get("window"))
	now := time.Now()
	data, err := s.live.get(window, now, func() (liveWindowData, error) {
		lw, err := st.LiveWindowStats(r.Context(), now, window)
		if err != nil {
			return liveWindowData{}, err
		}
		lt, err := st.LiveTutorStats(r.Context(), now, window)
		if err != nil {
			return liveWindowData{}, err
		}
		return liveWindowData{window: lw, tutor: lt}, nil
	})
	if err != nil {
		s.logger.Warn("live dashboard query failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "live stats unavailable"})
		return
	}
	resp := liveResponse{
		GeneratedAt:   now.UnixMilli(),
		WindowMinutes: window,
		Window:        data.window,
		Tutor:         data.tutor,
	}
	resp.Now, resp.Countries = s.liveNow()
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) liveNow() (liveNow, []liveCountry) {
	live := s.manager.All()
	total, peak := s.manager.Metrics()
	out := liveNow{
		SessionsActive:       len(live),
		WSConnections:        telemetry.Snapshot().ActiveWSConnections,
		PeakSessions:         peak,
		SessionsCreatedTotal: total,
	}
	sections := map[string]int{}
	demos := map[string]int{}
	countries := map[string]int{}
	for _, sess := range live {
		if sess.UserID() != "" {
			out.SessionsWithUser++
		} else {
			out.SessionsAnonymous++
		}
		if section := sess.Presence().Section; section != "" {
			sections[section]++
		}
		if program := sess.Program(); program != "" {
			demos[program]++
		}
		if code := sess.ClientInfo().GeoCountry; code != "" {
			countries[code]++
		}
	}
	out.Sections = sortedLiveCounts(sections)
	out.Demos = sortedLiveCounts(demos)
	byCountry := make([]liveCountry, 0, len(countries))
	for code, n := range countries {
		byCountry = append(byCountry, liveCountry{Code: code, Count: n})
	}
	sort.Slice(byCountry, func(i, j int) bool {
		if byCountry[i].Count != byCountry[j].Count {
			return byCountry[i].Count > byCountry[j].Count
		}
		return byCountry[i].Code < byCountry[j].Code
	})
	return out, byCountry
}

func sortedLiveCounts(m map[string]int) []liveCount {
	out := make([]liveCount, 0, len(m))
	for name, n := range m {
		out = append(out, liveCount{Name: name, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}
