// Package web holds the frontend the shell serves to its web views at mp://app/.
package web

import "embed"

// FS lists every shipped file by name: mock.js, mock-icons.js and mock-live.js stay on
// disk for browser work and out of the binary. icon.png is drawn by packaging/icon.
//
//go:embed index.html app.css themes.css app.js icon.png i18n/*.js
var FS embed.FS
