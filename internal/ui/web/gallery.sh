#!/bin/sh
# Renders the theme gallery out of the mock page in headless Chrome:
#   dist/themes/<id>-<mode>-overview.png, …-settings.png and …-controls.png (Apps with the quit question
#   open: primary, secondary and destructive buttons), 420 × 704, three per theme and mode;
#   dist/themes/contact-sheet.png, every Overview side by side, light themes first.
# Run after any change to themes.css or app.css: sh internal/ui/web/gallery.sh
set -eu

CHROME=${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}
WEB=$(cd "$(dirname "$0")" && pwd)
OUT=$WEB/../../../dist/themes
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$OUT"

# terminal is drawn for dark only and paper for light only (SKIN_MODE in app.js).
LIGHT="aqua graphite nord catppuccin solarized gruvbox paper contrast vivid"
DARK="aqua graphite nord catppuccin solarized gruvbox terminal contrast vivid"

# A Chrome window is never narrower than 500 px, so the page sits in an iframe of the panel's size.
frame() {
  printf '<iframe src="file://%s/index.html?mock=1&still=1&skin=%s&theme=%s&tab=%s" style="width:420px;height:704px;border:0"></iframe>' "$WEB" "$1" "$2" "$3"
}

shoot() {
  "$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-prefers-reduced-motion --virtual-time-budget=4000 \
    --window-size="$3" --force-device-scale-factor="$4" --screenshot="$2" "file://$1" 2>/dev/null
}

sheet=$TMP/sheet.html
printf '<body style="margin:12px;display:grid;grid-template-columns:repeat(6,210px);gap:12px;background:#8a8a8a;font:600 12px system-ui">' > "$sheet"
for mode in light dark; do
  [ "$mode" = light ] && ids=$LIGHT || ids=$DARK
  for id in $ids; do
    for shot in overview settings controls; do
      tab=$shot
      [ "$shot" = controls ] && tab="processes&ask=1"
      printf '<body style="margin:0">%s' "$(frame "$id" "$mode" "$tab")" > "$TMP/one.html"
      shoot "$TMP/one.html" "$OUT/$id-$mode-$shot.png" 420,704 1
    done
    printf '<div style="height:374px">%s %s<div style="margin-top:6px;transform:scale(.5);transform-origin:0 0">%s</div></div>' \
      "$id" "$mode" "$(frame "$id" "$mode" overview)" >> "$sheet"
  done
done
shoot "$sheet" "$OUT/contact-sheet.png" 1344,1182 2
ls "$OUT"/*.png | wc -l
