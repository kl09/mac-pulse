package web

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/settings"
)

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssRule    = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	themeBlock = regexp.MustCompile(`^\[data-skin="(\w+)"\]\[data-theme="(light|dark)"\]`)
	skinList   = regexp.MustCompile(`var SKINS = \[([^\]]*)\]`)
	skinModes  = regexp.MustCompile(`var SKIN_MODE = \{([^}]*)\}`)
)

// colour is straight (not premultiplied) sRGB with channels and alpha in 0–1.
type colour struct{ r, g, b, a float64 }

// over paints c on an opaque background.
func (c colour) over(bg colour) colour {
	return colour{c.r*c.a + bg.r*(1-c.a), c.g*c.a + bg.g*(1-c.a), c.b*c.a + bg.b*(1-c.a), 1}
}

// contrast is the WCAG 2 ratio of two opaque colours.
func (c colour) contrast(other colour) float64 {
	a, b := c.luminance(), other.luminance()
	return (math.Max(a, b) + 0.05) / (math.Min(a, b) + 0.05)
}

func (c colour) luminance() float64 {
	linear := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.r) + 0.7152*linear(c.g) + 0.0722*linear(c.b)
}

// stylesheet maps a selector to its declarations, a later rule for the same selector winning.
type stylesheet map[string]map[string]string

func readStylesheet(t *testing.T, files ...string) stylesheet {
	t.Helper()
	sheet := stylesheet{}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, rule := range cssRule.FindAllStringSubmatch(cssComment.ReplaceAllString(string(raw), ""), -1) {
			selector := strings.Join(strings.Fields(rule[1]), " ")
			if sheet[selector] == nil {
				sheet[selector] = map[string]string{}
			}
			for decl := range strings.SplitSeq(rule[2], ";") {
				if name, value, ok := strings.Cut(decl, ":"); ok {
					sheet[selector][strings.TrimSpace(name)] = strings.TrimSpace(value)
				}
			}
		}
	}
	return sheet
}

// tokens are the custom properties <html> ends up with for one theme in one mode, in cascade order.
func (s stylesheet) tokens(skin, mode string) map[string]string {
	selectors := []string{":root"}
	if mode == "dark" {
		selectors = append(selectors, `:root[data-theme="dark"]`)
	}
	if skin != "aqua" {
		selectors = append(selectors, `:root:not([data-skin="aqua"])`, `[data-skin="`+skin+`"]`,
			`[data-skin="`+skin+`"][data-theme="`+mode+`"]`, `:root:not([data-skin="aqua"])[data-theme="`+mode+`"]`)
	}
	tokens := map[string]string{}
	for _, selector := range selectors {
		for name, value := range s[selector] {
			tokens[name] = value
		}
	}
	return tokens
}

// splitArgs cuts the arguments of a CSS function at its top-level commas.
func splitArgs(inner string) []string {
	var args []string
	depth, start := 0, 0
	for i, r := range inner {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		case r == ',' && depth == 0:
			args = append(args, strings.TrimSpace(inner[start:i]))
			start = i + 1
		}
	}
	return append(args, strings.TrimSpace(inner[start:]))
}

// themeModes lists the modes each theme has a colour block for; aqua, being app.css itself, has both.
func themeModes(sheet stylesheet) map[string][]string {
	modes := map[string][]string{"aqua": {"light", "dark"}}
	for selector := range sheet {
		if m := themeBlock.FindStringSubmatch(selector); m != nil && !slices.Contains(modes[m[1]], m[2]) {
			modes[m[1]] = append(modes[m[1]], m[2])
		}
	}
	return modes
}

// parseColour evaluates the colour syntax the stylesheets use: hex, rgba(), transparent, var() and color-mix(in srgb, …).
func parseColour(t *testing.T, tokens map[string]string, expr string) colour {
	t.Helper()
	expr = strings.TrimSpace(expr)
	name, inner, isCall := strings.Cut(strings.TrimSuffix(expr, ")"), "(")
	args := splitArgs(inner)

	switch {
	case expr == "transparent":
		return colour{}
	case strings.HasPrefix(expr, "#"):
		hex := expr[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		require.NoError(t, err, expr)
		require.Len(t, hex, 6, expr)
		return colour{float64(v>>16) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255, 1}
	case isCall && name == "var":
		value, ok := tokens[args[0]]
		require.True(t, ok, "token %s is not defined", args[0])
		return parseColour(t, tokens, value)
	case isCall && name == "rgba":
		require.Len(t, args, 4, expr)
		var n [4]float64
		for i, arg := range args {
			v, err := strconv.ParseFloat(arg, 64)
			require.NoError(t, err, expr)
			n[i] = v
		}
		return colour{n[0] / 255, n[1] / 255, n[2] / 255, n[3]}
	case isCall && name == "color-mix":
		require.Len(t, args, 3, expr)
		require.Equal(t, "in srgb", args[0], expr)
		cut := strings.LastIndexByte(args[1], ' ')
		share, err := strconv.ParseFloat(strings.TrimSuffix(args[1][cut+1:], "%"), 64)
		require.NoError(t, err, expr)
		p := share / 100
		x, y := parseColour(t, tokens, args[1][:cut]), parseColour(t, tokens, args[2])
		// CSS mixes premultiplied: a transparent side gives up its share of alpha and no colour.
		alpha := x.a*p + y.a*(1-p)
		if alpha == 0 {
			return colour{}
		}
		mix := func(a, b float64) float64 { return (a*x.a*p + b*y.a*(1-p)) / alpha }
		return colour{mix(x.r, y.r), mix(x.g, y.g), mix(x.b, y.b), alpha}
	}
	require.Failf(t, "unknown colour syntax", "%q", expr)
	return colour{}
}

// The user picks a theme by eye from ten: each must exist on both sides of the bridge and not
// be another theme in a different colour.
func TestThemes_Set(t *testing.T) {
	t.Parallel()

	sheet := readStylesheet(t, "app.css", "themes.css")
	modes := themeModes(sheet)
	script, err := os.ReadFile("app.js")
	require.NoError(t, err)

	t.Run("ids", func(t *testing.T) {
		t.Parallel()

		var skins []string
		for _, id := range strings.Split(string(skinList.FindSubmatch(script)[1]), ",") {
			skins = append(skins, strings.Trim(id, " '"))
		}
		oneMode := map[string][]string{}
		for _, pair := range strings.Split(string(skinModes.FindSubmatch(script)[1]), ",") {
			id, mode, _ := strings.Cut(pair, ":")
			oneMode[strings.TrimSpace(id)] = []string{strings.Trim(mode, " '")}
		}

		assert.Equal(t, settings.Themes, skins, "SKINS in app.js")
		for _, id := range settings.Themes {
			want := []string{"light", "dark"}
			if fixed, ok := oneMode[id]; ok {
				want = fixed
			}
			assert.ElementsMatch(t, want, modes[id], "modes of %s in themes.css against SKIN_MODE in app.js", id)
			assert.Equal(t, strings.Join(oneMode[id], ""), settings.ThemeMode[id], "settings.ThemeMode of %s against SKIN_MODE in app.js", id)
		}
		assert.Len(t, modes, len(settings.Themes), "a theme in themes.css that the settings do not know")
	})

	t.Run("distinct", func(t *testing.T) {
		t.Parallel()

		// A theme's shape, and its tile colour in the first mode it has: two themes that share
		// all but one of these read as the same theme in another colour.
		shape := func(id string) []string {
			tokens := sheet.tokens(id, modes[id][0])
			tile := parseColour(t, tokens, "var(--tile)")
			return []string{tokens["--r-tile"], tokens["--border-w"], tokens["--font"], tokens["--gap"], fmt.Sprint(tile)}
		}
		for i, a := range settings.Themes {
			for _, b := range settings.Themes[i+1:] {
				differ := 0
				for k, v := range shape(a) {
					if v != shape(b)[k] {
						differ++
					}
				}
				assert.GreaterOrEqual(t, differ, 2, "%s and %s differ in too little of radius, frame, font, density and tile fill", a, b)
			}
		}
	})
}

// The numbers are read all day: every text and mark must hold WCAG AA on its tile in each mode a theme ships.
func TestThemes(t *testing.T) {
	t.Parallel()

	sheet := readStylesheet(t, "app.css", "themes.css")
	for id, list := range themeModes(sheet) {
		for _, mode := range list {
			t.Run(id+"/"+mode, func(t *testing.T) {
				t.Parallel()

				tokens := sheet.tokens(id, mode)
				value := func(name string) colour { return parseColour(t, tokens, "var("+name+")") }
				bg := value("--bg")
				if bg.a == 0 {
					bg = value("--backdrop") // aqua: the shell's material stands behind the page
				}
				// vivid gives each metric its own tile colour in rules under the theme's selector.
				tiles := map[string]colour{"": value("--tile").over(bg)}
				for selector, decls := range sheet {
					if tile, ok := decls["--tile"]; ok && strings.HasPrefix(selector, `[data-skin="`+id+`"][data-theme="`+mode+`"] `) {
						tiles[selector] = parseColour(t, tokens, tile).over(bg)
					}
				}
				minText := 4.5
				if id == "contrast" {
					minText = 7
				}

				// Dim text also sits on a pill, on an inactive segment and on the alert banner of the page.
				surfaces := map[string]colour{
					"an inactive segment on the page": value("--seg").over(bg),
					"the warning banner":              value("--warn-bg").over(bg),
				}
				for where, tile := range tiles {
					hover := value("--hover").over(tile)
					for _, name := range []string{"--text", "--text-2", "--text-3", "--sys", "--crit-text"} {
						assert.GreaterOrEqual(t, value(name).over(tile).contrast(tile), minText, "%s on the tile %s", name, where)
						assert.GreaterOrEqual(t, value(name).over(hover).contrast(hover), minText, "%s on a hovered row of the tile %s", name, where)
					}
					for _, name := range []string{"--blue", "--purple", "--green", "--indigo", "--teal", "--bat"} {
						assert.GreaterOrEqual(t, value(name).over(tile).contrast(tile), 3.0, "mark %s on the tile %s", name, where)
					}
					surfaces["a pill on the tile "+where] = value("--track").over(tile)
					surfaces["an inactive segment on the tile "+where] = value("--seg").over(tile)
				}
				for where, surface := range surfaces {
					for _, name := range []string{"--text", "--text-2"} {
						assert.GreaterOrEqual(t, value(name).over(surface).contrast(surface), minText, "%s on %s", name, where)
					}
				}
				// A nearly full volume turns its bar from the disk colour to the warning one.
				assert.NotEqual(t, value("--indigo"), value("--warn"), "the disk colour is the warning colour")

				// aqua keeps the switch of System Settings, a white knob on the system green, below 3:1 as it is there.
				if id == "aqua" {
					return
				}
				// A theme may restyle the switch in rules of its own; the tokens are what the rest get.
				part := func(selector, property, token string) colour {
					if own, ok := sheet[`:root[data-skin="`+id+`"] input.switch`+selector][property]; ok {
						return parseColour(t, tokens, own)
					}
					return value(token)
				}
				on := part(":checked", "background-color", "--ok").over(tiles[""])
				off := part(":not(:checked)", "background-color", "--off").over(tiles[""])
				assert.GreaterOrEqual(t, part(":checked::after", "background", "--knob").over(on).contrast(on), 3.0, "switch knob, on")
				assert.GreaterOrEqual(t, part(":not(:checked)::after", "background", "--knob").over(off).contrast(off), 3.0, "switch knob, off")
				primary := value("--primary").over(tiles[""])
				assert.GreaterOrEqual(t, value("--on-primary").over(primary).contrast(primary), 4.5, "label on the primary button")
				assert.GreaterOrEqual(t, value("--focus").over(bg).contrast(bg), 3.0, "focus ring on the page")
			})
		}
	}
}
