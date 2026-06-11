package handlers

import (
	"database/sql"
	"log"
	"time"

	"github.com/coaether/server/models"
	"github.com/coaether/server/protocol"
)

// SessionService manages session lifecycle and DB synchronization.
type SessionService struct {
	DB  *sql.DB
	Bus *protocol.MessageBus
}

// NewSessionService creates a new SessionService.
func NewSessionService(db *sql.DB, bus *protocol.MessageBus) *SessionService {
	return &SessionService{DB: db, Bus: bus}
}

// MarkRunning updates a session status to 'running'.
func (s *SessionService) MarkRunning(sessionID string) error {
	_, err := s.DB.Exec(
		`UPDATE sessions SET status = $1, updated_at = NOW() WHERE id = $2 AND status = 'pending'`,
		models.SessionRunning, sessionID,
	)
	if err != nil {
		log.Printf("[SessionService] MarkRunning %s: %v", sessionID[:8], err)
	}
	return err
}

// MarkCompleted updates a session status to 'completed' with output.
func (s *SessionService) MarkCompleted(sessionID, output string) error {
	now := time.Now()
	_, err := s.DB.Exec(
		`UPDATE sessions SET status = $1, output_log = $2, completed_at = $3, updated_at = $3 WHERE id = $4`,
		models.SessionCompleted, output, now, sessionID,
	)
	if err != nil {
		log.Printf("[SessionService] MarkCompleted %s: %v", sessionID[:8], err)
	}
	return err
}

// MarkFailed updates a session status to 'failed' with error details.
func (s *SessionService) MarkFailed(sessionID, errMsg string) error {
	now := time.Now()
	_, err := s.DB.Exec(
		`UPDATE sessions SET status = $1, error_log = $2, completed_at = $3, updated_at = $3 WHERE id = $4`,
		models.SessionFailed, errMsg, now, sessionID,
	)
	if err != nil {
		log.Printf("[SessionService] MarkFailed %s: %v", sessionID[:8], err)
	}
	return err
}

// CleanStaleDBSessions marks timed-out pending/running sessions as failed.
func (s *SessionService) CleanStaleDBSessions(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	result, err := s.DB.Exec(
		`UPDATE sessions SET status = $1, error_log = 'session timeout',
		 completed_at = NOW(), updated_at = NOW()
		 WHERE status IN ('pending', 'running') AND updated_at < $2`,
		models.SessionFailed, cutoff,
	)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}
