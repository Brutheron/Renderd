package main

import (
	"slices"
	"testing"
)

func TestReaderCommandOpensRightSidePanel(t *testing.T) {
	command := readerCommand("herdr-test", pluginID, "w4:p1")

	want := []string{
		"herdr-test",
		"plugin", "pane", "open",
		"--plugin", pluginID,
		"--entrypoint", "reader",
		"--placement", "split",
		"--direction", "right",
		"--target-pane", "w4:p1",
		"--focus",
		"--env", sourcePaneEnv + "=w4:p1",
	}
	if !slices.Equal(command.Args, want) {
		t.Fatalf("reader command args = %q, want %q", command.Args, want)
	}
	if !slices.Contains(command.Args, "--target-pane") {
		t.Fatal("split panel must target the source pane")
	}
}
