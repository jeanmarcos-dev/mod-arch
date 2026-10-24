package server

import (
	"context"
	"net/http"
	"time"

	"github.com/jeanmarcos/modos-cpu/internal/sessions"
	"github.com/jeanmarcos/modos-cpu/internal/store"
)

const (
	trackEventPageView  = "page_view"
	trackMaxNameLen     = 100
	trackMaxDurationMS  = 21_600_000
	trackLimitPerMinute = 60
	feedbackTimeout     = 3 * time.Second
)

type trackLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time
	start  time.Time
	count  int
}

func newTrackLimiter(limit int, window time.Duration) *trackLimiter {
	return &trackLimiter{limit: limit, window: window, now: time.Now}
}

func (l *trackLimiter) allow() bool {
	now := l.now()
	if l.start.IsZero() || now.Sub(l.start) >= l.window {
		l.start = now
		l.count = 0
	}
	if l.count >= l.limit {
		return false
	}
	l.count++
	return true
}

func (s *Server) wsUserIDFunc(r *http.Request, sess *sessions.Session, role sessions.Role) func() string {
	if role != sessions.RoleViewer {
		return sess.UserID
	}
	noUser := func() string { return "" }
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return noUser
	}
	own := s.manager.Get(c.Value)
	if own == nil {
		return noUser
	}
	return own.UserID
}

func (s *Server) recordUsage(sess *sessions.Session, role sessions.Role, userID func() string, kind, name string, durationMS *int64) {
	if s.usage == nil {
		return
	}
	uid := ""
	if userID != nil {
		uid = userID()
	}
	s.usage.Record(store.UsageEvent{
		SessionID:  sess.ID,
		UserID:     uid,
		Kind:       kind,
		Name:       name,
		DurationMS: durationMS,
		Role:       role.String(),
	})
}

func (s *Server) handleTrack(cmd Command, sess *sessions.Session, role sessions.Role, userID func() string) {
	s.recordUsage(sess, role, userID, store.UsageKindPageView, cmd.Name, cmd.DurationMS)
}

func (s *Server) handleTutorFeedback(cmd Command, sess *sessions.Session) {
	st := s.manager.Store()
	if st == nil || cmd.Rating == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), feedbackTimeout)
	defer cancel()
	if _, err := st.RateTutorQA(ctx, sess.ID, cmd.RequestID, *cmd.Rating, time.Now().UnixMilli()); err != nil {
		s.logger.Warn("tutor feedback failed", "err", err, "session", sess.ID)
	}
}
