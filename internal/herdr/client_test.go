package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSubscribeAgentStatusFiltersOnePaneAndStreamsStatuses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket transport test")
	}

	socketPath := shortSocketPath(t)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("sandbox does not permit Unix sockets: %v", err)
		}
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverErr <- acceptErr
			return
		}
		defer connection.Close()

		var request struct {
			ID     string `json:"id"`
			Method string `json:"method"`
			Params struct {
				Subscriptions []struct {
					Type   string `json:"type"`
					PaneID string `json:"pane_id"`
				} `json:"subscriptions"`
			} `json:"params"`
		}
		if decodeErr := json.NewDecoder(connection).Decode(&request); decodeErr != nil {
			serverErr <- decodeErr
			return
		}
		if request.ID != "renderd_status" || request.Method != "events.subscribe" {
			serverErr <- fmt.Errorf("unexpected request: %#v", request)
			return
		}
		if len(request.Params.Subscriptions) != 1 ||
			request.Params.Subscriptions[0].Type != "pane.agent_status_changed" ||
			request.Params.Subscriptions[0].PaneID != "w1:p7" {
			serverErr <- fmt.Errorf("unexpected subscriptions: %#v", request.Params.Subscriptions)
			return
		}

		encoder := json.NewEncoder(connection)
		for _, message := range []any{
			map[string]any{"id": "renderd_status", "result": map[string]any{"type": "subscription_started"}},
			map[string]any{"event": "pane.agent_status_changed", "data": map[string]any{"pane_id": "w1:p8", "workspace_id": "w1", "agent_status": "blocked"}},
			map[string]any{"event": "pane.agent_status_changed", "data": map[string]any{"pane_id": "w1:p7", "workspace_id": "w1", "agent_status": "working"}},
			map[string]any{"event": "pane.agent_status_changed", "data": map[string]any{"pane_id": "w1:p7", "workspace_id": "w1", "agent_status": "done"}},
		} {
			if encodeErr := encoder.Encode(message); encodeErr != nil {
				serverErr <- encodeErr
				return
			}
		}
		serverErr <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := (Client{SocketPath: socketPath}).SubscribeAgentStatus(ctx, "w1:p7")
	if err != nil {
		t.Fatalf("SubscribeAgentStatus() error = %v", err)
	}

	for _, want := range []string{"working", "done"} {
		select {
		case event := <-events:
			if event.Err != nil || event.Status != want {
				t.Fatalf("event = %#v, want status %q", event, want)
			}
		case <-ctx.Done():
			t.Fatalf("waiting for status %q: %v", want, ctx.Err())
		}
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("test server: %v", err)
	}
}

func TestSubscribeAgentStatusRejectsInvalidAcknowledgement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket transport test")
	}

	socketPath := shortSocketPath(t)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("sandbox does not permit Unix sockets: %v", err)
		}
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		var request any
		_ = json.NewDecoder(connection).Decode(&request)
		_ = json.NewEncoder(connection).Encode(map[string]any{
			"id": "renderd_status", "result": map[string]any{"type": "not_a_subscription"},
		})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := (Client{SocketPath: socketPath}).SubscribeAgentStatus(ctx, "w1:p7"); err == nil {
		t.Fatal("SubscribeAgentStatus() accepted an invalid acknowledgement")
	}
}

func shortSocketPath(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/tmp", "renderd-")
	if err != nil {
		t.Fatalf("create socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return filepath.Join(directory, "herdr.sock")
}
