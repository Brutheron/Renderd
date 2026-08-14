package agents

import (
	"context"
	"time"
)

// Session identifies one native agent conversation attached to a Herdr pane.
type Session struct {
	Agent  string
	Kind   string
	Source string
	Value  string
}

// FinalResponse is the provider-neutral document consumed by the reader UI.
type FinalResponse struct {
	Agent       string
	SessionID   string
	TurnID      string
	Markdown    string
	CompletedAt time.Time
}

// Adapter retrieves the latest completed final response for one agent family.
type Adapter interface {
	LatestFinal(context.Context, Session) (FinalResponse, error)
}
