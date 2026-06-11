package protocol

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MemberRole defines the role of a session member.
type MemberRole int

const (
	RoleOwner    MemberRole = 0
	RoleMember   MemberRole = 1
	RoleObserver MemberRole = 2
)

func (r MemberRole) String() string {
	switch r {
	case RoleOwner:
		return "owner"
	case RoleMember:
		return "member"
	case RoleObserver:
		return "observer"
	default:
		return "unknown"
	}
}

// EndpointInfo holds the runtime state for a connected endpoint.
type EndpointInfo struct {
	ID           string
	Addr         *Addr
	Conn         *websocket.Conn
	Metadata     map[string]any
	Capabilities []Capability
	Deliver      func(env *Envelope) error // custom delivery function
	LastSeen     time.Time
}

// SessionInfo holds the runtime state for an active session.
type SessionInfo struct {
	ID      string
	Members map[string]MemberRole // endpointID → role
	Created time.Time
}

// RouteRule defines a routing pattern.
type RouteRule struct {
	Pattern string // e.g. "agent://*/claude/*"
	Target  string // e.g. "runtime://node-001" (where to forward)
}

// DeliverFunc is a callback for delivering a message.
type DeliverFunc func(env *Envelope) error

// MessageBus is the core message routing system.
type MessageBus struct {
	mu        sync.RWMutex
	endpoints map[string]*EndpointInfo
	sessions  map[string]*SessionInfo
	store     MessageStore
	logger    *log.Logger
}

// NewMessageBus creates a new MessageBus.
func NewMessageBus() *MessageBus {
	return &MessageBus{
		endpoints: make(map[string]*EndpointInfo),
		sessions:  make(map[string]*SessionInfo),
		logger:    log.Default(),
	}
}

// SetStore sets a persistent message store.
func (b *MessageBus) SetStore(s MessageStore) { b.store = s }

// GetStore returns the current message store.
func (b *MessageBus) GetStore() MessageStore { return b.store }

// SetEndpointDeliver sets a custom delivery function for an endpoint's connection.
func (b *MessageBus) SetEndpointDeliver(conn *websocket.Conn, fn func(env *Envelope) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ep := range b.endpoints {
		if ep.Conn == conn {
			ep.Deliver = fn
			return
		}
	}
}

// ==================== Endpoint Management ====================

// Register adds an endpoint to the bus.
func (b *MessageBus) Register(id string, conn *websocket.Conn, metadata map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.endpoints[id] = &EndpointInfo{
		ID:       id,
		Addr:     ParseAddr(id),
		Conn:     conn,
		Metadata: metadata,
		LastSeen: time.Now(),
	}
	b.logger.Printf("[Bus] Endpoint registered: %s", id)
}

// Unregister removes an endpoint from the bus.
// Returns true if the endpoint was registered.
func (b *MessageBus) Unregister(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.endpoints[id]; ok {
		delete(b.endpoints, id)
		b.logger.Printf("[Bus] Endpoint unregistered: %s", id)

		// Remove from all sessions and clean up orphaned sessions
		for sid, sess := range b.sessions {
			delete(sess.Members, id)
			// If only system members remain (e.g. system://api), the session is dead
			if len(sess.Members) == 0 || b.onlySystemMembers(sess) {
				delete(b.sessions, sid)
				b.logger.Printf("[Bus] Session %s cleaned up (no real members)", sid)
			}
		}
		return true
	}
	return false
}

// GetEndpoint returns an endpoint by ID.
func (b *MessageBus) GetEndpoint(id string) *EndpointInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.endpoints[id]
}

// ==================== Session Management ====================

// CreateSession creates a new session.
func (b *MessageBus) CreateSession(sessionID string, members map[string]MemberRole) *SessionInfo {
	b.mu.Lock()
	defer b.mu.Unlock()

	sess := &SessionInfo{
		ID:      sessionID,
		Members: members,
		Created: time.Now(),
	}
	if sess.Members == nil {
		sess.Members = make(map[string]MemberRole)
	}
	b.sessions[sessionID] = sess
	b.logger.Printf("[Bus] Session created: %s (%d members)", sessionID, len(members))
	return sess
}

// GetSession returns a session by ID.
func (b *MessageBus) GetSession(sessionID string) *SessionInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.sessions[sessionID]
}

// GetEndpointSessions returns all session IDs an endpoint is a member of.
func (b *MessageBus) GetEndpointSessions(endpointID string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var ids []string
	for sid, sess := range b.sessions {
		if _, ok := sess.Members[endpointID]; ok {
			ids = append(ids, sid)
		}
	}
	return ids
}

// FindEndpointsByCapability finds endpoints that have a specific capability.
func (b *MessageBus) FindEndpointsByCapability(agentID string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []string
	for id, ep := range b.endpoints {
		for _, c := range ep.Capabilities {
			if c.ID == agentID {
				result = append(result, id)
				break
			}
		}
	}
	return result
}

// FindRuntimesForAgent finds runtime endpoints that can handle the given agent ID.
func (b *MessageBus) FindRuntimesForAgent(agentID string) []string {
	runtimes := b.FindEndpointsByCapability(agentID)
	// Filter to only runtime endpoints
	var result []string
	for _, r := range runtimes {
		addr := ParseAddr(r)
		if addr.Type == EndpointRuntime {
			result = append(result, r)
		}
	}
	return result
}

// JoinSession adds a member to a session.
func (b *MessageBus) JoinSession(sessionID, endpointID string, role MemberRole) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	sess, ok := b.sessions[sessionID]
	if !ok {
		return false
	}
	sess.Members[endpointID] = role
	b.logger.Printf("[Bus] %s joined session %s as %s", endpointID, sessionID, role)
	return true
}

// LeaveSession removes a member from a session.
func (b *MessageBus) LeaveSession(sessionID, endpointID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	sess, ok := b.sessions[sessionID]
	if !ok {
		return false
	}
	delete(sess.Members, endpointID)
	b.logger.Printf("[Bus] %s left session %s", endpointID, sessionID)

	// Clean up empty sessions
	if len(sess.Members) == 0 {
		delete(b.sessions, sessionID)
		b.logger.Printf("[Bus] Session %s deleted (no members)", sessionID)
	}
	return true
}

// EndSession removes all members and deletes the session.
func (b *MessageBus) EndSession(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.sessions, sessionID)
	b.logger.Printf("[Bus] Session ended: %s", sessionID)
}

// onlySystemMembers returns true if all session members have system:// prefix.
func (b *MessageBus) onlySystemMembers(sess *SessionInfo) bool {
	for memberID := range sess.Members {
		if !strings.HasPrefix(memberID, "system://") {
			return false
		}
	}
	return true
}

// StartGC starts periodic garbage collection of stale sessions.
func (b *MessageBus) StartGC(interval, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			b.cleanStaleSessions(maxAge)
		}
	}()
}

func (b *MessageBus) cleanStaleSessions(maxAge time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for sid, sess := range b.sessions {
		if sess.Created.Before(cutoff) && b.onlySystemMembers(sess) {
			delete(b.sessions, sid)
			b.logger.Printf("[Bus] GC: removed stale session %s (created %v)", sid, sess.Created)
		}
	}
}

// ==================== Message Delivery ====================

// shouldPersist checks whether a message type should be saved to the store.
func shouldPersist(msgType string) bool {
	switch msgType {
	case MsgPing, MsgPong, MsgHello, MsgBye, MsgAck:
		return false
	default:
		return true
	}
}

// Deliver sends an envelope to its target(s).
// Returns the number of endpoints the message was delivered to.
func (b *MessageBus) Deliver(env *Envelope) int {
	// Persist to store if applicable
	if b.store != nil && env.SessionID != "" && shouldPersist(env.Type) {
		if err := b.store.Save(env); err != nil {
			b.logger.Printf("[Bus] Store save error: %v", err)
		}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	to := ParseAddr(env.To)
	if to.IsZero() {
		b.logger.Printf("[Bus] Dropping message with empty target: %s", env.ID)
		return 0
	}

	switch to.Type {
	case EndpointSession:
		return b.deliverToSession(env, to)
	default:
		return b.deliverToEndpoint(env, to)
	}
}

// deliverToEndpoint sends the message to a specific endpoint.
func (b *MessageBus) deliverToEndpoint(env *Envelope, to *Addr) int {
	ep, ok := b.endpoints[to.Raw]
	if !ok {
		// Try prefix match for wildcard addresses (e.g. agent://node-001/claude/inst_001)
		ep = b.matchEndpoint(to)
		if ep == nil {
			b.logger.Printf("[Bus] No endpoint found for: %s", to.Raw)
			return 0
		}
	}

	deliver := ep.Deliver
	if deliver == nil {
		return 0
	}

	if err := deliver(env); err != nil {
		b.logger.Printf("[Bus] Deliver error to %s: %v", ep.ID, err)
		return 0
	}
	return 1
}

// deliverToSession broadcasts the message to all session members.
func (b *MessageBus) deliverToSession(env *Envelope, to *Addr) int {
	sess, ok := b.sessions[to.Parts[0]]
	if !ok {
		b.logger.Printf("[Bus] Session not found: %s", to.Parts[0])
		return 0
	}

	// Deliver to all connected members
	delivered := 0
	for endpointID := range sess.Members {
		if ep, ok := b.endpoints[endpointID]; ok {
			deliver := ep.Deliver
			if deliver == nil {
				continue
			}
			if err := deliver(env); err != nil {
				b.logger.Printf("[Bus] Session deliver error to %s: %v", endpointID, err)
			} else {
				delivered++
			}
		}
	}
	return delivered
}

// matchEndpoint finds an endpoint by longest prefix match.
func (b *MessageBus) matchEndpoint(addr *Addr) *EndpointInfo {
	var best *EndpointInfo
	bestLen := 0

	for _, ep := range b.endpoints {
		if ep.Addr == nil {
			continue
		}
		// Check how many parts match
		matchLen := 0
		for i := 0; i < len(addr.Parts) && i < len(ep.Addr.Parts); i++ {
			if addr.Parts[i] == ep.Addr.Parts[i] {
				matchLen++
			} else {
				break
			}
		}
		if matchLen > bestLen {
			best = ep
			bestLen = matchLen
		}
	}
	return best
}

// EndpointsByType returns all endpoints of the given address type.
func (b *MessageBus) EndpointsByType(epType EndpointType) []*EndpointInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []*EndpointInfo
	for _, ep := range b.endpoints {
		if ep.Addr != nil && ep.Addr.Type == epType {
			result = append(result, ep)
		}
	}
	return result
}

// ==================== Convenience Methods ====================

// SendMessage creates and delivers a message envelope.
func (b *MessageBus) SendMessage(from, to, msgType, sessionID string, content []ContentBlock) int {
	env := NewEnvelope(from, to, msgType, &Payload{Content: content})
	env.SessionID = sessionID
	return b.Deliver(env)
}

// SendRaw creates and delivers a raw payload envelope.
func (b *MessageBus) SendRaw(from, to, msgType, sessionID string, payload *Payload) int {
	env := NewEnvelope(from, to, msgType, payload)
	env.SessionID = sessionID
	return b.Deliver(env)
}

// BroadcastToSession sends a message to all members of a session.
func (b *MessageBus) BroadcastToSession(sessionID string, env *Envelope) {
	env.To = "session://" + sessionID
	b.Deliver(env)
}

// ==================== Message Store ====================

// MessageStore defines the interface for persistent message storage.
type MessageStore interface {
	Save(env *Envelope) error
	GetBySession(sessionID string, limit int) ([]*Envelope, error)
}

// ==================== Stats ====================

// BusStats returns statistics about the bus.
type BusStats struct {
	Endpoints int
	Sessions  int
}

func (b *MessageBus) Stats() BusStats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return BusStats{
		Endpoints: len(b.endpoints),
		Sessions:  len(b.sessions),
	}
}
