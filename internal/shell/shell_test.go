package shell

import (
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestAsset(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"index.html": {Data: []byte("<html>")},
		"app.js":     {Data: []byte("js")},
		"icon.png":   {Data: []byte("\x89PNGself")},
	}
	png, calculator := "\x89PNG", "path=%2FSystem%2FApplications%2FCalculator.app"
	tests := []struct {
		name       string
		path       string
		query      string
		wantPrefix string
		wantMIME   string
	}{
		{name: "embedded page", path: "/index.html", wantPrefix: "<html>", wantMIME: "text/html; charset=utf-8"},
		{name: "embedded script", path: "/app.js", wantPrefix: "js", wantMIME: "text/javascript; charset=utf-8"},
		{name: "missing file", path: "/mock.js"},
		{name: "path escape", path: "/../shell.go"},
		{name: "bundle icon, then again from the cache", path: "/icon", query: calculator, wantPrefix: png, wantMIME: "image/png"},
		{name: "embedded icon for mac-pulse itself", path: "/icon", query: "path=self", wantPrefix: "\x89PNGself", wantMIME: "image/png"},
		{name: "icon without path", path: "/icon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.query == calculator && testing.Short() {
				t.Skip("draws an installed app's icon through AppKit")
			}

			// The second call answers from the icon cache and must match the first.
			asset(assets, tt.path, tt.query)
			body, mimeType := asset(assets, tt.path, tt.query)

			if tt.wantPrefix == "" {
				assert.Nil(t, body)
			} else {
				assert.Contains(t, string(body[:min(len(body), 16)]), tt.wantPrefix)
			}
			assert.Equal(t, tt.wantMIME, mimeType)
		})
	}
}

// Registering a known id needs the main run loop, so only the rejection is reachable here.
func TestSetHotkey_UnknownID(t *testing.T) {
	t.Parallel()

	for _, id := range []int{-1, 2} {
		assert.EqualError(t, SetHotkey(id, true), fmt.Sprintf("register global shortcut: unknown id %d", id))
	}
}
