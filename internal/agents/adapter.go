package agents

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// ErrNoFinalResponse reports that a session was read successfully but has not
// produced a completed final response yet. Adapters return it so the reader can
// tell an empty session apart from a failed read.
var ErrNoFinalResponse = errors.New("no completed final response is available")

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

// StatusEvent reports a provider-neutral lifecycle change for an agent pane.
// Err is set when the live event stream can no longer be read.
type StatusEvent struct {
	Status string
	Err    error
}

// Adapter retrieves completed final responses for one agent family in
// chronological order.
type Adapter interface {
	FinalResponses(context.Context, Session) ([]FinalResponse, error)
}

// Registry maps a Herdr agent name to the adapter that reads its sessions.
type Registry map[string]Adapter

// FinalResponses dispatches to the adapter registered for the session's agent.
func (r Registry) FinalResponses(ctx context.Context, session Session) ([]FinalResponse, error) {
	adapter, found := r[session.Agent]
	if !found {
		return nil, fmt.Errorf(
			"Renderd cannot read %q sessions yet; supported agents: %s",
			session.Agent,
			strings.Join(r.Supported(), ", "),
		)
	}
	return adapter.FinalResponses(ctx, session)
}

// LatestFinal returns the newest completed response from the session history.
func (r Registry) LatestFinal(ctx context.Context, session Session) (FinalResponse, error) {
	responses, err := r.FinalResponses(ctx, session)
	if err != nil {
		return FinalResponse{}, err
	}
	if len(responses) == 0 {
		return FinalResponse{}, ErrNoFinalResponse
	}
	return responses[len(responses)-1], nil
}

// Supported lists the registered agent names in stable order.
func (r Registry) Supported() []string {
	return slices.Sorted(maps.Keys(r))
}
