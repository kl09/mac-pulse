// Command icon draws the mac-pulse app icon (three bars on a dark blue body) into an .iconset directory for iconutil
// and, as a second file, the 64 px PNG the frontend shows for mac-pulse's own row.
// Everything is computed from signed distances, so each size is rendered natively
// and the output is identical on every run.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

// Geometry is in units of the icon's side. The body follows Apple's icon grid:
// 824 of 1024 px wide with a 185 px corner radius.
const (
	bodyHalf   = 0.402
	bodyRadius = 0.181
)

type rgb struct{ r, g, b float64 }

// bar is one rounded column of the chart: centre, half size, corner radius, colour.
type bar struct {
	cx, cy, hw, hh, r float64
	col               rgb
}

var (
	bodyStart = rgb{0.169, 0.184, 0.467}
	bodyEnd   = rgb{0.059, 0.071, 0.188}
	// Three metrics side by side, in the panel's own blue, purple and green.
	bars = []bar{
		{0.3193, 0.6104, 0.0635, 0.1221, 0.045, rgb{0.302, 0.639, 1}},
		{0.5, 0.5127, 0.0635, 0.2197, 0.045, rgb{0.753, 0.518, 0.988}},
		{0.6807, 0.5664, 0.0635, 0.1660, 0.045, rgb{0.204, 0.827, 0.6}},
	}
	baseline = bar{0.5, 0.7725, 0.2734, 0.0068, 0.0068, rgb{1, 1, 1}}
)

func main() {
	if len(os.Args) != 3 {
		log.Fatal("usage: icon <dir.iconset> <icon64.png>")
	}
	files := map[string]int{os.Args[2]: 64}
	for _, points := range []int{16, 32, 128, 256, 512} {
		for scale, suffix := range map[int]string{1: "", 2: "@2x"} {
			files[filepath.Join(os.Args[1], fmt.Sprintf("icon_%dx%d%s.png", points, points, suffix))] = points * scale
		}
	}
	for name, px := range files {
		f, err := os.Create(name)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, render(px)); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}

func render(px int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	pixel := 1 / float64(px)
	for y := range px {
		for x := range px {
			u, v := (float64(x)+0.5)*pixel, (float64(y)+0.5)*pixel
			body := coverage(bodyDistance(u, v), pixel)
			// A soft shadow under the body, as on every macOS icon.
			shadow := 0.28 * math.Exp(-math.Max(bodyDistance(u, v-0.012), 0)/0.014)
			col := mix(bodyStart, bodyEnd, (u+v-0.2)/1.6)
			col = mix(col, baseline.col, 0.18*coverage(baseline.distance(u, v), pixel))
			for _, b := range bars {
				col = mix(col, b.col, coverage(b.distance(u, v), pixel))
			}
			alpha := body + (1-body)*shadow
			if alpha > 0 {
				col = mix(rgb{}, col, body/alpha)
			}
			img.SetNRGBA(x, y, color.NRGBA{R: channel(col.r), G: channel(col.g), B: channel(col.b), A: channel(alpha)})
		}
	}
	return img
}

// coverage turns a signed distance into antialiased opacity: 1 inside, 0 outside, a one-pixel ramp between.
func coverage(distance, pixel float64) float64 {
	return math.Min(math.Max(0.5-distance/pixel, 0), 1)
}

func bodyDistance(u, v float64) float64 {
	return bar{cx: 0.5, cy: 0.5, hw: bodyHalf, hh: bodyHalf, r: bodyRadius}.distance(u, v)
}

// distance is the signed distance to the rounded rectangle: negative inside.
func (b bar) distance(u, v float64) float64 {
	dx := math.Abs(u-b.cx) - (b.hw - b.r)
	dy := math.Abs(v-b.cy) - (b.hh - b.r)
	outside := math.Hypot(math.Max(dx, 0), math.Max(dy, 0))
	return outside + math.Min(math.Max(dx, dy), 0) - b.r
}

func mix(a, b rgb, t float64) rgb {
	t = math.Min(math.Max(t, 0), 1)
	return rgb{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

func channel(v float64) uint8 {
	return uint8(math.Round(math.Min(math.Max(v, 0), 1) * 255))
}
