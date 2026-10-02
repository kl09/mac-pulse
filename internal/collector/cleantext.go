package collector

import (
	"strings"
	"unicode"
)

// CleanText makes a string the system supplied safe to show and to compare: the bidi
// controls and the control characters go (a name with U+202E would render its row reversed
// and pass for another one), and what is left is cut to maxRunes; 0 does not cut.
func CleanText(s string, maxRunes int) string {
	kept := 0
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || (maxRunes > 0 && kept >= maxRunes) {
			return -1
		}
		kept++
		return r
	}, s)
}
