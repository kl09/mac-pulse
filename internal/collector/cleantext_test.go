package collector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		maxRunes int
		want     string
	}{
		{name: "plain text is kept", in: "Google Chrome", want: "Google Chrome"},
		{name: "bidi override and isolate are dropped", in: "evil\u202etxt.exe\u2066!\u2069", want: "eviltxt.exe!"},
		{name: "control characters are dropped", in: "a\nb\tc\x00d\x7f", want: "abcd"},
		{name: "cut by runes, not bytes", in: "Élève modèle", maxRunes: 5, want: "Élève"},
		{name: "dropped characters do not count towards the cut", in: "\u202eab\ncd", maxRunes: 3, want: "abc"},
		{name: "shorter than the cut", in: "ab", maxRunes: 200, want: "ab"},
		{name: "zero does not cut", in: strings.Repeat("é", 500), want: strings.Repeat("é", 500)},
		{name: "empty", in: "", maxRunes: 3, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, CleanText(tt.in, tt.maxRunes))
		})
	}
}
