package collector

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTabs(t *testing.T) {
	t.Parallel()

	var many strings.Builder
	wantMany := make([]string, maxTabs)
	for i := range maxTabs + 7 {
		fmt.Fprintf(&many, "tab %d\n", i)
	}
	for i := range wantMany {
		wantMany[i] = fmt.Sprintf("tab %d", i)
	}

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		// osascript ends its answer with one more newline after the script's own.
		{name: "one title per line", input: "mac-pulse — README\nInbox (3)\n\n", want: []string{"mac-pulse — README", "Inbox (3)"}},
		{name: "a tab without a title leaves no row", input: "first\n\n  \nlast\n", want: []string{"first", "last"}},
		{
			name:  "a title is cut to 200 runes and loses its bidi controls",
			input: "evil\u202etxt.exe\n" + strings.Repeat("é", 300) + "\n",
			want:  []string{"eviltxt.exe", strings.Repeat("é", maxTabRunes)},
		},
		{name: "at most 500 titles", input: many.String(), want: wantMany},
		{name: "a browser without windows", input: "\n", want: []string{}},
		{name: "no output", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, parseTabs([]byte(tt.input)))
		})
	}
}

func TestTabsScript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		app  string
		// want are the lines the script must hold; none means no script.
		want []string
	}{
		{
			name: "Safari is asked by its bundle id, only while it runs, for the name of a tab", app: "Safari",
			want: []string{`if application id "com.apple.Safari" is running then`, `tell application id "com.apple.Safari"`, "(name of t)"},
		},
		{
			name: "a Chromium browser is asked for the title", app: "Google Chrome",
			want: []string{`if application id "com.google.Chrome" is running then`, `tell application id "com.google.Chrome"`, "(title of t)"},
		},
		{name: "Firefox has no AppleScript dictionary", app: "Firefox"},
		{name: "a name that would break out of the script", app: `Safari" to quit --`},
		{name: "no name", app: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tabsScript(tt.app)

			if tt.want == nil {
				assert.Empty(t, got)
			}
			for _, line := range tt.want {
				assert.Contains(t, got, line)
			}
			assert.NotContains(t, got, `application "`, "a browser is never addressed by name")
		})
	}

	t.Run("every browser with a script has a bundle id", func(t *testing.T) {
		t.Parallel()

		for app, family := range browsers {
			assert.Equal(t, family != familyFirefox, tabsScript(app) != "", app)
		}
	})
}

// Only the refusals run: asking a real browser would put the Automation prompt on screen.
func TestBrowserTabs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		app  string
	}{
		{name: "Firefox has no AppleScript dictionary", app: "Firefox"},
		{name: "an app that is no browser", app: "Finder"},
		{name: "a name that would break out of the script", app: `Safari" to quit --`},
		{name: "no name", app: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := BrowserTabs(t.Context(), tt.app)

			require.ErrorIs(t, err, ErrNoScript)
			assert.Nil(t, got)
		})
	}
}
