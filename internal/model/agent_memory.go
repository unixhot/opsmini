package model

import "time"

// AgentMemory is one durable fact the agent learned about this host, carried
// across sessions.
//
// The transcript already remembers a conversation, but only that conversation.
// Next week's session has no idea that nginx lives in /etc/nginx/conf.d or that
// backups go to /data/backup, so every session starts by rediscovering the same
// things. This table holds those facts.
//
// It deliberately stores no dialogue: "user asked about disk" is not a fact worth
// keeping, "the data disk is mounted at /data" is.
type AgentMemory struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Subject is the entity the fact is about: a service, a path, a host.
	// It is what the agent matches against a new question to decide relevance.
	Subject string `gorm:"size:128;index" json:"subject"`
	// Fact is one sentence, in the language the user speaks.
	Fact string `gorm:"size:512" json:"fact"`
	// SourceSession lets an operator trace a doubtful fact back to where it came
	// from, and delete the memory of a session that went wrong.
	SourceSession string `gorm:"size:40;index" json:"source_session"`
	// Confidence is the agent's own 0..1 estimate. Facts stated by the user score
	// high; inferences from a single observation score low. Eviction uses it.
	Confidence float64 `gorm:"default:0.7" json:"confidence"`
	// ExpiresAt bounds the usefulness of time-sensitive facts (an IP, a version).
	// Nil means "no expiry".
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}
