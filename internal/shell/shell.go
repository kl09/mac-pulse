// Package shell is the Cocoa side of mac-pulse: a menu bar status item, a panel that
// drops from it and an optional window, each hosting a WKWebView with the embedded frontend.
package shell

/*
#cgo CFLAGS: -fobjc-arc -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework Carbon
#include <stdlib.h>
#include "shell.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"
	"unsafe"
)

const (
	// A stuck OnMessage fills this many slots before messages are dropped; the main thread never waits.
	messageBacklog = 256
	// Carbon's eventHotKeyExistsErr.
	hotkeyTaken = -9878
)

type Options struct {
	// Assets is served at mp://app/; index.html is the entry point.
	Assets fs.FS
	// OnMessage receives every JS→Go message and the shell's own events as one JSON
	// object. Calls are serial, in arrival order, on one goroutine that is not the main
	// thread; Run returns only after the last call has.
	OnMessage func(msg []byte)
	// Tab is passed to the frontend as the ?tab= query parameter.
	Tab string
	// Version is shown in Settings; it travels as the ?version= query parameter.
	Version string
	// Open shows the panel at start and keeps it open until the status item is clicked.
	Open bool
	// Window opens the detached window at start.
	Window bool
}

// StatusItem is one segment of the menu bar title: an SF Symbol followed by a short text.
type StatusItem struct {
	Symbol string `json:"symbol"`
	Text   string `json:"text"`
	// Warn paints the segment in the system warning colour.
	Warn bool `json:"warn,omitempty"`
	// Points draws a sparkline before the text, oldest first, 0–1 each. Empty, as it always
	// is with the menu_bar_graph setting off, the segment has no graph.
	Points []float64 `json:"points,omitempty"`
}

// assets and messages are written once by Run before the run loop starts. Only the main
// thread sends on messages, and it is inside the run loop whenever it does.
var (
	assets   fs.FS
	messages chan []byte
)

// AppKit only works on the process's first thread, and init runs on it.
func init() {
	runtime.LockOSThread()
}

// Run must be called from the main goroutine; it returns after Quit. MAC_PULSE_DEBUG in
// the environment enables the Web Inspector.
func Run(o Options) {
	assets = o.Assets
	messages = make(chan []byte, messageBacklog)
	delivered := make(chan struct{})
	go func() {
		defer close(delivered)
		for msg := range messages {
			if o.OnMessage != nil {
				o.OnMessage(msg)
			}
		}
	}()
	tab, version := C.CString(url.QueryEscape(o.Tab)), C.CString(url.QueryEscape(o.Version))
	defer C.free(unsafe.Pointer(tab))
	defer C.free(unsafe.Pointer(version))
	C.mpRun(tab, version, boolInt(o.Open), boolInt(o.Window), boolInt(os.Getenv("MAC_PULSE_DEBUG") != ""))
	close(messages)
	<-delivered
}

func Quit() {
	C.mpQuit()
}

// SetStatus draws the main status item. No items removes it from the menu bar, which is
// what the separate-items mode does; any item puts it back where it was.
func SetStatus(items []StatusItem) {
	withCString(statusJSON(items), func(s *C.char) { C.mpSetStatus(s) })
}

// SetExtraStatus draws a status item of its own, "mac-pulse-<id>" to AppKit, which keeps
// where the user ⌘-drags it. A click opens the panel on tab ("" keeps the panel's tab), under
// the item or where the user left the panel; the tab reaches the page unchecked, so the caller
// passes only a name from its whitelist. No items removes the item.
func SetExtraStatus(id, tab string, items []StatusItem) {
	cid, ctab, cjson := C.CString(id), C.CString(tab), C.CString(statusJSON(items))
	defer C.free(unsafe.Pointer(cid))
	defer C.free(unsafe.Pointer(ctab))
	defer C.free(unsafe.Pointer(cjson))
	C.mpSetExtraStatus(cid, ctab, cjson)
}

func statusJSON(items []StatusItem) string {
	// A slice of two-string structs always marshals; nil must not become "null", which the shell drops.
	b, _ := json.Marshal(append([]StatusItem{}, items...))
	return string(b)
}

// SetClock shows the clock as an extra status item with id "clock", whose click opens the
// panel on detail:clock. template is an NSDateFormatter template such as "jmm" or
// "EEEdMMMHHmmss", locale a BCP 47 tag; an empty template removes the item.
func SetClock(template, locale string) {
	ctemplate, clocale := C.CString(template), C.CString(locale)
	defer C.free(unsafe.Pointer(ctemplate))
	defer C.free(unsafe.Pointer(clocale))
	C.mpSetClock(ctemplate, clocale)
}

// SetDock shows or hides the app's Dock icon at once, without a restart. A click on the
// icon arrives as a {type:"open_window"} message.
func SetDock(show bool) {
	C.mpSetDock(boolInt(show))
}

// ChooseFolder asks the user for a folder and returns its path, "" when they cancel or the
// app quits meanwhile. It blocks while the dialog is up, so it must not be called from the
// main thread or the message goroutine.
func ChooseFolder() string {
	path := C.mpChooseFolder()
	if path == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(path))
	return C.GoString(path)
}

// Push calls window.mp.<fn>(<payload>) in every visible web view; payload is one JSON value.
func Push(fn string, payload []byte) {
	withCString("window.mp."+fn+"("+string(payload)+")", func(s *C.char) { C.mpEval(s) })
}

// SetMenu names the items of the status item's right-click menu, in menu order.
func SetMenu(open, window, export, settings, quit, reset string) {
	titles := []*C.char{C.CString(open), C.CString(window), C.CString(export), C.CString(settings), C.CString(quit), C.CString(reset)}
	C.mpSetMenu(titles[0], titles[1], titles[2], titles[3], titles[4], titles[5])
	for _, title := range titles {
		C.free(unsafe.Pointer(title))
	}
}

// PreferredLanguages is the user's macOS language list as BCP 47 tags, best first.
func PreferredLanguages() []string {
	list := C.mpPreferredLanguages()
	defer C.free(unsafe.Pointer(list))
	return strings.Split(C.GoString(list), ",")
}

// ShowPanel opens the panel on tab; "" keeps the tab it is on. The tab reaches the page
// unchecked, so the caller passes only a name from its whitelist.
func ShowPanel(tab string) {
	withCString(tab, func(s *C.char) { C.mpShowPanel(s) })
}

// SetPinned keeps the panel open when the user clicks elsewhere, until the status item is
// clicked; the shell reports the change in its visibility message.
func SetPinned(on bool) {
	C.mpSetPinned(boolInt(on))
}

// SetWindowOnTop floats the detached window above other apps' windows.
func SetWindowOnTop(on bool) {
	C.mpSetWindowOnTop(boolInt(on))
}

// SetAppearance forces the panel and the window light or dark; any other mode follows
// macOS. The status item always follows the menu bar.
func SetAppearance(mode string) {
	C.mpSetAppearance(map[string]C.int{"light": 1, "dark": 2}[mode])
}

// SetStatusStyle switches every status item to the compact layout: a smaller font and
// tighter gaps. Off, which is the default, they use the menu bar's own font size.
func SetStatusStyle(compact bool) {
	C.mpSetStatusStyle(boolInt(compact))
}

func OpenWindow() {
	C.mpOpenWindow()
}

// BackToPanel closes the detached window and opens the panel under the menu bar instead.
func BackToPanel() {
	C.mpBackToPanel()
}

// Snapshot writes the visible web view (the window if open, else the panel) to a PNG at
// path, the main status item, when there is one, next to it as <path without .png>-status.png
// and every extra status item as <path without .png>-status-<id>.png.
func Snapshot(path string) {
	withCString(path, func(s *C.char) { C.mpSnapshot(s) })
}

// Export writes the view on screen (the window if open, else the panel) to a new PNG at path
// and reveals it in Finder; an existing file is never overwritten. It waits for the main
// thread, so it must not be called from it.
func Export(path string) error {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	if failure := C.mpExport(cpath); failure != nil {
		defer C.free(unsafe.Pointer(failure))
		return fmt.Errorf("export panel to %s: %s", path, C.GoString(failure))
	}
	return nil
}

// Copy replaces the clipboard with text.
func Copy(text string) {
	withCString(text, func(s *C.char) { C.mpCopy(s) })
}

const (
	// HotkeyPanel is ⌃⌥P, which toggles the panel from any app.
	HotkeyPanel = 0
	// HotkeyQuit is ⌃⌥K, which arrives as a {type:"hotkey", name:"quit"} message.
	HotkeyQuit = 1
)

// SetHotkey registers or releases one of the two global shortcuts. Repeating a call is a
// no-op. Once Run has started it must not be called from the main thread; it gives up with
// an error when the main thread does not answer within 2 s, as it does not once the app quits.
func SetHotkey(id int, on bool) error {
	if id != HotkeyPanel && id != HotkeyQuit {
		return fmt.Errorf("register global shortcut: unknown id %d", id)
	}
	switch status := C.mpSetHotkey(C.int(id), boolInt(on)); status {
	case 0:
		return nil
	case hotkeyTaken:
		return fmt.Errorf("register global shortcut: ⌃⌥%c is taken by another app", "PK"[id])
	default:
		return fmt.Errorf("register global shortcut: OSStatus %d", int(status))
	}
}

// TerminateApp asks the application that owns pid to quit the way Cmd+Q does, so it may
// show its "Save changes?" dialog; false means pid is not an application and needs a signal.
// It waits up to 2 s for the main thread and must not be called from it.
func TerminateApp(pid int32) bool {
	return C.mpTerminateApp(C.int(pid)) != 0
}

// Notify posts a user notification. Inside the .app bundle it is mac-pulse's own, and a click
// on it opens the panel on tab, which reaches the page unchecked like ShowPanel's; a bare
// binary has no notification identity and sends it through osascript as Script Editor.
func Notify(title, body, tab string) {
	ctitle, cbody, ctab := C.CString(title), C.CString(body), C.CString(tab)
	defer C.free(unsafe.Pointer(ctitle))
	defer C.free(unsafe.Pointer(cbody))
	defer C.free(unsafe.Pointer(ctab))
	C.mpNotify(ctitle, cbody, ctab)
}

func withCString(s string, call func(*C.char)) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	call(cs)
}

func boolInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

//export mpMessage
func mpMessage(msg *C.char) {
	select {
	case messages <- []byte(C.GoString(msg)):
	default:
		slog.Warn("shell message dropped: OnMessage is not keeping up")
	}
}

// mpAsset returns a malloc'd body and MIME type for mp://app<path>?<query>, or nil.
// The shell calls it on a background queue.
//
//export mpAsset
func mpAsset(cpath, cquery *C.char, n *C.int, mimeType **C.char) unsafe.Pointer {
	body, mimeName := asset(assets, C.GoString(cpath), C.GoString(cquery))
	if body == nil {
		return nil
	}
	*n = C.int(len(body))
	*mimeType = C.CString(mimeName)
	return C.CBytes(body)
}

// asset resolves /icon?path=<bundle> to that bundle's icon (the embedded icon.png for
// path "self") and any other path to a file in assets.
func asset(assets fs.FS, urlPath, query string) (body []byte, mimeType string) {
	name := strings.TrimPrefix(urlPath, "/")
	switch {
	case name == "icon":
		q, _ := url.ParseQuery(query)
		mimeType = "image/png"
		if p := q.Get("path"); p == "self" && assets != nil {
			body, _ = fs.ReadFile(assets, "icon.png")
		} else {
			body = appIcon(p)
		}
	case assets != nil:
		// fs.ReadFile rejects rooted and ".." paths itself.
		body, _ = fs.ReadFile(assets, name)
		mimeType = mime.TypeByExtension(path.Ext(name))
	}
	if body == nil {
		return nil, ""
	}
	return body, mimeType
}
