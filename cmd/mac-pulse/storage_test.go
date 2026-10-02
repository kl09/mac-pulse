package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/ui"
)

// The cleanup messages in the orders a page of its own would never send them in: that the
// review comes first and the Trash before a deletion for good is Go's rule.
func TestSession_clean(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	logs := filepath.Join(home, "Library", "Logs")
	require.NoError(t, os.MkdirAll(logs, 0o700))
	for _, name := range []string{"a.log", "b.log", "c.log"} {
		require.NoError(t, os.WriteFile(filepath.Join(logs, name), []byte("x"), 0o600))
	}
	s := &session{latest: &latest{}, home: home, noTrash: map[string]bool{}}
	list := func() string {
		t.Helper()
		res, err := s.list(t.Context(), "logs")
		require.NoError(t, err)
		return res.Review.ID
	}
	clean := func(review, item string, permanent bool) string {
		t.Helper()
		res, _ := s.clean(t.Context(), ui.Message{Type: "cleanup", Category: "logs", Review: review, Items: []string{item}, Permanent: permanent})
		return res.Key
	}

	_, err := s.list(t.Context(), "logs")
	require.Error(t, err, "nothing is listed before the first measurement")
	_, err = s.measure(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "err.stale", clean("", "a.log", false), "a cleanup that names no review")
	first := list()
	assert.Equal(t, "err.stale", clean(first, "a.log", true), "for good while nobody has seen the Trash fail")
	second := list()
	assert.Equal(t, "err.stale", clean(first, "a.log", false), "the review of a listing that was replaced")
	require.FileExists(t, filepath.Join(logs, "a.log"))

	assert.Equal(t, "act.cleaned", clean(second, "a.log", false))
	assert.NoFileExists(t, filepath.Join(logs, "a.log"))
	moved, _ := filepath.Glob(filepath.Join(home, ".Trash", "mac-pulse-logs-*", "a.log"))
	assert.Len(t, moved, 1, "the entry went to the Trash")
	assert.Equal(t, "err.stale", clean(second, "b.log", false), "a cleanup measures the category again: its review is spent")

	// A Trash that is a file is no Trash.
	require.NoError(t, os.RemoveAll(filepath.Join(home, ".Trash")))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".Trash"), nil, 0o600))
	third := list()
	assert.Equal(t, "err.no_trash", clean(third, "b.log", false))
	require.FileExists(t, filepath.Join(logs, "b.log"))
	assert.Equal(t, "act.cleaned", clean(third, "b.log", true), "for good, once, after the Trash failed")
	assert.NoFileExists(t, filepath.Join(logs, "b.log"))
	assert.Equal(t, "err.stale", clean(list(), "c.log", true), "the failed Trash allowed one deletion, not the next")
	assert.FileExists(t, filepath.Join(logs, "c.log"))
}
