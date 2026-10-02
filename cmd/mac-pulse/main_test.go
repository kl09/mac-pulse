package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDebugMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
		want   []json.RawMessage
	}{
		{name: "not set"},
		{name: "the name of a message without fields", action: "ping", want: []json.RawMessage{json.RawMessage(`{"type":"ping"}`)}},
		{name: "a name outside the list is not JSON", action: "update_check"},
		{
			name: "a whole message", action: `{"type":"set","key":"theme","value":"nord"}`,
			want: []json.RawMessage{json.RawMessage(`{"type":"set","key":"theme","value":"nord"}`)},
		},
		{
			name: "a list of messages, in order", action: `[{"type":"cleanup_scan"}, {"type":"cleanup","category":"go_build"}]`,
			want: []json.RawMessage{json.RawMessage(`{"type":"cleanup_scan"}`), json.RawMessage(`{"type":"cleanup","category":"go_build"}`)},
		},
		{name: "script instead of JSON", action: `alert(1)`},
		{name: "a list cut short", action: `[{"type":"ping"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, debugMessages(tt.action))
		})
	}
}

func TestStorageHome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		account string
		env     string
		debug   bool
		want    string
	}{
		{name: "the account's home, which the environment agrees with", account: "/Users/alex", env: "/Users/alex", want: "/Users/alex"},
		{name: "an environment that claims another home switches storage off", account: "/Users/alex", env: "/tmp/x"},
		{name: "an environment that claims the root: /Library/Caches is nobody's cache", account: "/Users/alex", env: "/"},
		{name: "no HOME at all", account: "/Users/alex", env: ""},
		{name: "a debug run takes a scratch home", account: "/Users/alex", env: "/tmp/scratch/", debug: true, want: "/tmp/scratch"},
		{name: "a debug run does not take the root", account: "/Users/alex", env: "/", debug: true},
		{name: "a debug run does not take the root spelled another way", account: "/Users/alex", env: "/./", debug: true},
		{name: "a debug run does not take a relative home", account: "/Users/alex", env: "home", debug: true},
		{name: "an account whose home is the root", account: "/", env: "/"},
		{name: "an account without a home", account: "", env: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, storageHome(tt.account, tt.env, tt.debug))
		})
	}
}

// A test binary is linked without the Makefile's -ldflags.
func TestVersionLine(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "mac-pulse dev", versionLine())
}
