# Contributing to mac-pulse

Thank you for helping. Bug reports with a Mac model and macOS version are as valuable as code:
sensor names differ between chips, and most of them have been seen on only a few machines.

## Ground rules

mac-pulse stays without root, a privileged helper, a kernel or network extension, telemetry and
automatic network requests. A change that needs one of these will not be merged. For anything larger
than a fix, open an issue first so the approach can be agreed before you write the code.

## Build, test, lint

You need macOS 13 or later, the Xcode Command Line Tools and the Go version named in `go.mod`.

```sh
make build   # bin/mac-pulse, ad hoc signed
make run     # build and start
make test    # go test -race ./...
make lint    # gofumpt, golangci-lint, C and Objective-C warnings as errors
make bundle  # dist/mac-pulse.app
```

`make lint` needs [gofumpt](https://github.com/mvdan/gofumpt) and
[golangci-lint](https://golangci-lint.run) v2 on your `PATH`:

```sh
go install mvdan.cc/gofumpt@v0.12.0
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.1
```

Tests that read hardware (SMC, IOReport, GPU, Wi-Fi) skip themselves on a machine where the source
does not answer. `make test TESTFLAGS=-short` also skips the one test that creates a temporary
keychain; CI runs it that way.

Only one instance runs: starting a second `bin/mac-pulse` opens the panel of the first and exits.
Quit the installed app before `make run`.

## Layout

| Path | What |
|---|---|
| `cmd/mac-pulse` | `main`, the session that connects everything, the MCP server |
| `internal/collector` | samplers: CPU, memory, disk, processes, battery, sensors, Docker |
| `internal/native` | cgo: SMC, IOReport, GPU, `libproc` |
| `internal/netinspect` | `nettop`, reverse DNS, public IP, ping, speed test, update check |
| `internal/store` | history and its file |
| `internal/alerts` | rules and the alert detail |
| `internal/storage` | folder scan and cleanup |
| `internal/settings` | settings and their file |
| `internal/shell` | the Objective-C shell: menu bar items, panel, web view |
| `internal/ui` | builds the state the page receives, handles its messages |
| `internal/ui/web` | the page: `index.html`, `app.js`, `app.css`, `themes.css`, `i18n/` |
| `packaging` | `Info.plist`, entitlements, the icon generator |

## Code style

- Go is formatted with gofumpt and must pass `.golangci.yml` (lines up to 140 columns, functions up to 100 lines).
- Tests are table-driven, use testify, and parse fixtures from `testdata/` rather than calling the live system where possible. Fixtures must not contain real user names, host names, serial numbers or addresses.
- A reading that cannot be obtained is `null` in the state and `—` or "Unavailable" on screen, never 0.
- The page is plain JavaScript without a framework, a bundler or a dependency. Every visible string goes through the dictionaries.
- Comments say why, in English. Every network request or file write outside `~/Library/Application Support/mac-pulse/` must follow an explicit user action and be described in the README.

## Working on the panel in a browser

The page runs without the app, on mock data. Open it in Chrome or Safari:

```sh
open "internal/ui/web/index.html?mock=1&theme=dark&tab=overview&lang=en&backdrop=1"
```

Useful parameters (the full list is at the top of `internal/ui/web/mock-live.js`):

| Parameter | Values |
|---|---|
| `tab` | `overview`, `processes`, `network`, `history`, `dev`, `storage`, `settings`, `settings:appearance`, `detail:cpu`, `detail:alert:<id>` … |
| `theme` | `light`, `dark` |
| `skin` | `catppuccin`, `aqua`, `graphite`, `nord`, `solarized`, `gruvbox`, `terminal`, `paper`, `contrast`, `vivid` |
| `lang` | a language code, e.g. `de` |
| `mode` | `window` for the detached window layout |
| `click` | CSS selectors to click after load, separated by `;` |
| `still` | `1` stops the values from ticking |

`mock.js`, `mock-icons.js` and `mock-live.js` are not embedded in the binary. Mock data must stay
fictional: no real machine data, and no third-party logos.

`sh internal/ui/web/gallery.sh` renders every theme with headless Chrome into `dist/themes/` (ignored
by git) for a side-by-side check after a change to `themes.css` or `app.css`. The screenshots in
`docs/screenshots/` are made from the mock page the same way; never from a real machine.

## Adding a language

1. Copy `internal/ui/web/i18n/en.js` to `i18n/<code>.js` and translate the values. Keep the keys and the `{placeholders}`.
2. Add `<script src="i18n/<code>.js"></script>` to `internal/ui/web/index.html`.
3. Add the code to `Languages` in `internal/settings/settings.go`.
4. Run `go test ./internal/ui/...`: `TestReadDictionary` fails if a dictionary lacks a key that `en.js` has, has one it does not, or is not listed in `index.html`.

To fix a translation, edit the value in the language's file; nothing else is needed.

## Adding a theme

1. Add `[data-skin="<id>"][data-theme="light"]` and `[data-theme="dark"]` blocks to `internal/ui/web/themes.css`, defining the same variables as an existing theme.
2. Add the id to `SKINS` in `internal/ui/web/app.js` (and to `SKIN_MODE` if the theme has one mode only), and to `Themes` (and `ThemeMode`) in `internal/settings/settings.go`.
3. Add its name as `theme.<id>` to every dictionary.
4. Run `go test ./internal/ui/web/`: `TestThemes` checks that every theme defines every variable and that text reaches WCAG AA contrast on its backgrounds.

## Debug environment variables

These work in any build:

| Variable | Effect |
|---|---|
| `MAC_PULSE_OPEN=1` | opens the panel at start |
| `MAC_PULSE_TAB=<tab>` | the tab to open, as in a `mac-pulse://` link |
| `MAC_PULSE_WINDOW=1` | starts in window mode |

These are ignored unless `MAC_PULSE_DEBUG=1` is set as well, so that a normal run never writes a
file or acts because of its environment:

| Variable | Effect |
|---|---|
| `MAC_PULSE_DEBUG=1` | enables the hooks below and Inspect Element in the panel; lets `-storage` and `-caches` use a `HOME` other than the account's |
| `MAC_PULSE_PANEL_FRAME=x,y,w,h` | places the panel, in screen points from the bottom left |
| `MAC_PULSE_SNAPSHOT=<file.png>` | writes a PNG of the panel shortly after start and on every `SIGUSR1` |
| `MAC_PULSE_ACTION=<message>` | sends one of the page's messages to the core (a name such as `ping`, or JSON) |
| `MAC_PULSE_STATUS_FIXED=1` | draws the menu bar item from a fixed sample |

## Pull requests

- One topic per pull request; describe what changes for the user.
- `make lint` and `make test` must pass. CI runs both on macOS.
- Update `README.md` and add a line under "Unreleased" in `CHANGELOG.md` when behaviour changes.
- By contributing you agree that your contribution is licensed under the [MIT License](LICENSE).
