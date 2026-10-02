#ifndef MP_SHELL_H
#define MP_SHELL_H

// Every function except mpRun may be called from any thread; all but mpAppIcon and
// mpPreferredLanguages dispatch their work to the main queue.

// Blocks on the main thread until mpQuit. tab and version are URL-escaped query values;
// debug turns on the Web Inspector, the web views' context menu and the MAC_PULSE_PANEL_FRAME hook.
// The panel opens under the status item at 420 x 704 pt until the user drags or resizes it; from
// then on it opens where it was left, kept in the defaults under "mac-pulse panel frame", until
// the menu's Reset Panel Position or until that place is no longer on any screen.
void mpRun(const char *tab, const char *version, int open, int window, int debug);
void mpQuit(void);
// json: [{"symbol":"cpu","text":"37%","warn":true,"points":[0.2,0.4]}, ...]; symbol is an SF Symbol
// name or ""; points, when present, draws a sparkline of values in 0..1 before the text. "[]"
// takes the main item out of the menu bar until the next non-empty call.
void mpSetStatus(const char *json);
// A status item of its own, named "mac-pulse-<name>" for AppKit; json is as for mpSetStatus and "[]"
// removes the item. A click opens the panel on tab, a validated tab name or "": under the item,
// or where the user left the panel.
// A removed item, the main one included, comes back where the user had dragged it.
void mpSetExtraStatus(const char *name, const char *tab, const char *json);
// The clock item, named "clock" among the extra ones: format is an NSDateFormatter template
// ("jmm", "EEEdMMMHHmmss"), locale a BCP 47 tag; an empty format removes the item.
void mpSetClock(const char *format, const char *locale);
// 1 shows the app in the Dock (activation policy regular), 0 hides it (accessory); no restart.
// While it is shown, a click on the Dock icon sends {"type":"open_window"}.
void mpSetDock(int show);
// Shows a choose-a-folder dialog and blocks until it closes or mpQuit is called: never call it
// from the main thread. Returns the malloc'd path, or NULL when the user cancelled or the app is quitting.
char *mpChooseFolder(void);
// compact draws the status item in a smaller font with tighter gaps; 0 is the default look.
void mpSetStatusStyle(int compact);
// Evaluates js in every visible web view.
void mpEval(const char *js);
// Titles of the status item's right-click menu, in menu order.
void mpBackToPanel(void);
void mpSetMenu(const char *open, const char *window, const char *export, const char *settings, const char *quit, const char *reset);
// Returns the user's preferred languages as malloc'd comma-separated BCP 47 tags.
char *mpPreferredLanguages(void);
// tab is a tab name the caller has validated, or "" to keep the panel's current tab.
void mpShowPanel(const char *tab);
// The shell answers with a visibility message that carries the new pinned state.
void mpSetPinned(int on);
void mpSetWindowOnTop(int on);
// mode 1 forces the panel and the window light, 2 dark; 0 follows the system. The status item is not touched.
void mpSetAppearance(int mode);
void mpOpenWindow(void);
// Writes the visible web view to path, the main status item (when there is one) to <path without
// .png>-status.png and every extra one to <path without .png>-status-<name>.png.
void mpSnapshot(const char *path);
// Writes the view on screen to a new PNG at path and reveals it in Finder. Waits up to 5 s for
// the main queue: never call it from the main thread. Returns NULL or a malloc'd reason.
char *mpExport(const char *path);
void mpCopy(const char *text);
// Registers (on) or releases a global shortcut: id 0 is Ctrl+Opt+P, which toggles the panel, and
// id 1 is Ctrl+Opt+K, which sends {"type":"hotkey","name":"quit"}. Returns an OSStatus.
// Off the main thread it waits up to 2 s for the main queue and answers kMPTimeoutErr after that.
int mpSetHotkey(int id, int on);
void mpNotify(const char *title, const char *body, const char *tab);
// Asks the application with pid to quit as Cmd+Q would; 0 when pid is not an application.
// Waits up to 2 s for the main queue, so never call it from the main thread; a queue that does
// not answer in time counts as asked.
int mpTerminateApp(int pid);
// Returns a malloc'd PNG (32 pt @2x) of the Finder icon for path; the caller frees it.
void *mpAppIcon(const char *path, int *len);

#endif
