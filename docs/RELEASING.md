# Publishing and releasing mac-pulse

This is the maintainer's guide: how to publish the repository for the first time, how to cut a
release, and what the signing options mean for the people who download the app.

- [1. First publication](#1-first-publication)
- [2. Cutting a release](#2-cutting-a-release)
- [3. Signing levels](#3-signing-levels)
- [4. Homebrew](#4-homebrew)
- [5. Checklist before going public](#5-checklist-before-going-public)

## 1. First publication

### Create the repository

The module path, the update check and every link in the documentation assume
`github.com/kl09/mac-pulse`. If the name changes, change it in `go.mod` and the imports,
`internal/netinspect/update.go`, `packaging/Info.plist` (the bundle identifier), `README.md`,
`SECURITY.md`, `CHANGELOG.md` and `.github/`.

1. The repository lives under the personal account `kl09`; no organisation is needed.
2. Create the repository `kl09/mac-pulse`: **no** README, licence or .gitignore (the repository has them). You can create it private first and make it public after the checks in section 5.

### First commit and push

```sh
cd mac-pulse
git init -b main
git add .
git status            # read the list: no bin/, dist/, .DS_Store, nothing personal
git commit -m "mac-pulse 1.0.0"
git remote add origin git@github.com:kl09/mac-pulse.git
git push -u origin main
```

The commit carries the name and e-mail from `git config user.name` and `user.email`, and they
become public. Set them first if you want something other than your defaults; GitHub offers a
private `…@users.noreply.github.com` address under Settings → Emails.

Wait for the CI run on `main` (Actions tab) to turn green before tagging.

### Repository settings

- **About** (the gear on the repository page):
  - Description: `Free, open-source system monitor for the macOS menu bar with a network inspector. No root, no helper, no telemetry.`
  - Topics: `macos`, `menubar`, `menu-bar`, `system-monitor`, `apple-silicon`, `activity-monitor`, `network-monitor`, `golang`, `macos-app`
  - Untick "Packages" and "Deployments"; keep "Releases".
- **Settings → General → Social preview**: upload `docs/social-preview.png` (1280 × 640).
- **Settings → General → Features**: enable **Discussions** (the issue template chooser links to it) and keep Issues on. Under Pull Requests, "Allow squash merging" alone keeps the history linear.
- **Settings → Code security**: enable **Private vulnerability reporting** (`SECURITY.md` points to it), Dependabot alerts, and secret scanning with push protection.
- **Settings → Rules → Rulesets → New branch ruleset** for `main`: require a pull request before merging, require the status check `build` (the CI job) to pass, block force pushes and deletions. As the only maintainer you can add yourself to the bypass list.
- **Settings → Actions → General**: "Workflow permissions" can stay read-only; the release workflow asks for `contents: write` itself.

## 2. Cutting a release

The in-app **Check for updates** compares the running version with the tag of the latest GitHub
release, so tags must be `v` plus the version in `Info.plist`, and **the first tag must be `v1.0.0`**.
Until a release exists, the button answers "No releases published yet".

### Steps

1. Set the version in `packaging/Info.plist`: `CFBundleShortVersionString` to `X.Y.Z` and `CFBundleVersion` to the next integer. (For 1.0.0 both are already set.)

   ```sh
   plutil -replace CFBundleShortVersionString -string 1.0.1 packaging/Info.plist
   plutil -replace CFBundleVersion -string 6 packaging/Info.plist
   ```

2. In `CHANGELOG.md`, move the "Unreleased" entries under a new `## [X.Y.Z] - YYYY-MM-DD` heading and add the link at the bottom. The release notes are cut from this section.
3. Commit, and wait for CI on `main`.
4. Tag and push:

   ```sh
   git tag -a v1.0.1 -m "mac-pulse 1.0.1"
   git push origin v1.0.1
   ```

### What the release workflow does

`.github/workflows/release.yml` runs on the tag, on an Apple Silicon runner:

1. fails if the tag does not match `CFBundleShortVersionString`;
2. imports a Developer ID certificate and stores notary credentials, only if the secrets of section 3 exist;
3. runs `make release`: clean, tests, `dist/mac-pulse.app`, `dist/mac-pulse-X.Y.Z-arm64.dmg`, notarization (skipped without credentials), `dist/mac-pulse-X.Y.Z-arm64.zip`, `dist/SHA256SUMS.txt`;
4. verifies the signature;
5. creates the GitHub Release `vX.Y.Z` with the three files; the text is the CHANGELOG section, a note on opening a non-notarized app when that applies, and the checksums.

If the workflow fails, fix the cause, delete the tag (`git push --delete origin vX.Y.Z; git tag -d vX.Y.Z`) and tag again.

### The same by hand

```sh
make release                      # everything into dist/
gh release create v1.0.1 dist/mac-pulse-1.0.1-arm64.dmg dist/mac-pulse-1.0.1-arm64.zip dist/SHA256SUMS.txt \
  --title "mac-pulse 1.0.1" --notes "See CHANGELOG.md" --verify-tag
```

`gh` is the GitHub CLI (`brew install gh`, then `gh auth login`). Without it, use **Releases → Draft
a new release** on GitHub and attach the three files.

Single steps: `make bundle`, `make dmg`, `make zip`, `make checksums`.

### Intel

`make universal dmg zip checksums ARCH=universal` builds an app with arm64 and x86_64 slices and
names the files `…-universal.…`. The x86_64 slice compiles and starts under Rosetta; it has **not**
been run on an Intel Mac, where the SMC keys and IOReport channels are different. Do not advertise
Intel support before somebody has tested it. The release workflow builds arm64 only.

## 3. Signing levels

What the user sees depends on how the app is signed.

| Level | Cost | What the user sees on first open |
|---|---|---|
| Ad hoc (today) | free | macOS 15 and later: "mac-pulse" Not Opened — "Apple could not verify…"; the user must go to System Settings → Privacy & Security → **Open Anyway**. macOS 13–14: the same warning, and right-click → Open also works. |
| Developer ID, not notarized | 99 USD/year | The same refusal as ad hoc. Not worth doing alone. |
| Developer ID + notarization | 99 USD/year | One ordinary question: "mac-pulse is an app downloaded from the Internet. Are you sure you want to open it?" → Open. |

Two side effects of the ad hoc signature beyond the warning: its identity changes with every build,
so after an update macOS may ask again for the permissions it had granted (Desktop, Documents,
Downloads, Automation); and Homebrew's main repository does not accept the app (section 4).

### Ad hoc (the default)

Nothing to set up. `make bundle` signs with `codesign -s -` and the hardened runtime. The README
tells users how to open the app.

### Developer ID and notarization

One-time setup:

1. Join the [Apple Developer Program](https://developer.apple.com/programs/) (99 USD/year) as an individual or an organisation. Note your **Team ID** (developer.apple.com → Account → Membership details).
2. Create the certificate: Xcode → Settings → Accounts → your team → Manage Certificates → **+** → **Developer ID Application**. Without Xcode: create a certificate signing request in Keychain Access (Certificate Assistant → Request a Certificate From a Certificate Authority, saved to disk), upload it at developer.apple.com → Certificates → **+** → Developer ID Application, download the `.cer` and double-click it.
3. Check the identity and copy its full name:

   ```sh
   security find-identity -v -p codesigning
   #  1) ABCDEF…  "Developer ID Application: Your Name (TEAMID1234)"
   ```

4. Create an app-specific password for notarization: [account.apple.com](https://account.apple.com) → Sign-In and Security → App-Specific Passwords.
5. Store the notary credentials in your keychain under a profile name:

   ```sh
   xcrun notarytool store-credentials mac-pulse-notary \
     --apple-id you@example.com --team-id TEAMID1234 --password abcd-efgh-ijkl-mnop
   ```

Then a signed and notarized release is:

```sh
make release DEVELOPER_ID="Developer ID Application: Your Name (TEAMID1234)" NOTARY_PROFILE=mac-pulse-notary
```

What that runs, should you need the commands one by one:

```sh
# sign the app with the hardened runtime and a secure timestamp
codesign --force --options runtime --timestamp --entitlements packaging/entitlements.plist \
  -s "Developer ID Application: Your Name (TEAMID1234)" dist/mac-pulse.app
codesign --verify --deep --strict --verbose=2 dist/mac-pulse.app

# build and sign the disk image (make dmg does both)
codesign --force --timestamp -s "Developer ID Application: Your Name (TEAMID1234)" dist/mac-pulse-1.0.0-arm64.dmg

# send it to Apple and wait (usually a few minutes)
xcrun notarytool submit dist/mac-pulse-1.0.0-arm64.dmg --keychain-profile mac-pulse-notary --wait
# if the status is Invalid, read why:
xcrun notarytool log <submission-id> --keychain-profile mac-pulse-notary

# attach the ticket so the check works offline
xcrun stapler staple dist/mac-pulse-1.0.0-arm64.dmg
xcrun stapler staple dist/mac-pulse.app

# what Gatekeeper will say
spctl --assess --type execute --verbose dist/mac-pulse.app          # accepted, source=Notarized Developer ID
spctl --assess --type open --context context:primary-signature --verbose dist/mac-pulse-1.0.0-arm64.dmg
```

`make sign DEVELOPER_ID=…` re-signs an existing `dist/mac-pulse.app`; `make notarize NOTARY_PROFILE=…`
submits and staples. With the variables empty both print a line and do nothing.

This path has not been exercised yet, because the project has no Developer ID: expect to adjust a
detail on the first run. Once you sign with a Developer ID, remove the "not notarized" steps from
the README's Install section and FAQ.

### Secrets for the release workflow

Add these under **Settings → Secrets and variables → Actions → New repository secret**. If
`DEVELOPER_ID` or the certificate is missing, the workflow signs ad hoc; if the notary secrets are
missing, it signs but does not notarize.

| Secret | Value |
|---|---|
| `DEVELOPER_ID` | the identity name, e.g. `Developer ID Application: Your Name (TEAMID1234)` |
| `CERTIFICATE_P12_BASE64` | the certificate and its private key as base64 (below) |
| `CERTIFICATE_PASSWORD` | the password you chose when exporting the `.p12` |
| `NOTARY_APPLE_ID` | the Apple ID e-mail |
| `NOTARY_TEAM_ID` | the Team ID |
| `NOTARY_PASSWORD` | the app-specific password |

Exporting the certificate: Keychain Access → login → My Certificates → select "Developer ID
Application: …" (the row must expand to show a private key) → File → Export Items… → format
Personal Information Exchange (.p12) → choose a password. Then:

```sh
base64 -i DeveloperID.p12 | pbcopy    # paste as CERTIFICATE_P12_BASE64
rm DeveloperID.p12
```

## 4. Homebrew

### The main cask repository

Not available to mac-pulse today, for two reasons:

- **Signing.** [Acceptable Casks](https://docs.brew.sh/Acceptable-Casks) requires that an app "must pass Homebrew's Gatekeeper checks", and Homebrew 5.0.0 (November 2025) announced that casks failing Gatekeeper checks would be disabled in `homebrew/cask` in September 2026. An ad hoc signed app does not qualify; a notarized one does.
- **Notability.** The [acceptance policy](https://docs.brew.sh/Package-Acceptance-Policy) asks for at least 75 stars, 30 forks or 30 watchers (three times that when the author submits the cask), and normally a repository older than 30 days.

Check both pages again before submitting; the rules change.

### Your own tap

A tap is a repository named `kl09/homebrew-tap` with the file `Casks/mac-pulse.rb`:

```ruby
cask "mac-pulse" do
  version "1.0.0"
  sha256 "<the dmg line of SHA256SUMS.txt>"

  url "https://github.com/kl09/mac-pulse/releases/download/v#{version}/mac-pulse-#{version}-arm64.dmg"
  name "mac-pulse"
  desc "System monitor for the menu bar with a network inspector"
  homepage "https://github.com/kl09/mac-pulse"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on macos: ">= :ventura"
  depends_on arch: :arm64

  app "mac-pulse.app"

  zap trash: [
    "~/Library/Application Support/mac-pulse",
    "~/Library/LaunchAgents/com.kl09.mac-pulse.plist",
    "~/Library/Preferences/com.kl09.mac-pulse.plist",
  ]
end
```

Users would then run `brew install --cask kl09/tap/mac-pulse`. With an ad hoc signed app the
download is still quarantined, so the Gatekeeper steps of the README still apply after
installation, and Homebrew has deprecated the `--no-quarantine` flag that used to avoid them. A tap
is therefore of little use before notarization. Once the app is notarized, publish the tap,
replace the commented "planned" note in the README with the real command, and update `version` and
`sha256` on every release.

## 5. Checklist before going public

- [ ] `git status` and `git ls-files` show no `bin/`, `dist/`, `.DS_Store`, editor folders or notes that were meant to stay internal.
- [ ] No personal data in the tree: search the fixtures (`internal/*/testdata`), the mock data (`internal/ui/web/mock*.js`) and the documentation for your user name, host name, real IP and MAC addresses, serial numbers, e-mail addresses and private project names.

  ```sh
  grep -rniE "$(id -un)|$(scutil --get LocalHostName)" . --exclude-dir=.git --exclude-dir=bin --exclude-dir=dist
  ```

- [ ] Every image in `docs/` was rendered from mock data and shows no third-party logo. Look at each one.
- [ ] The copyright holder in `LICENSE` is the name you want. Licence headers in source files are not needed.
- [ ] `THIRD-PARTY-NOTICES.md` lists what `go version -m dist/mac-pulse.app/Contents/MacOS/mac-pulse` lists.
- [ ] The README matches the app: the network requests table, the permissions, the files written, the shortcuts, the requirements.
- [ ] `make lint` and `make test` pass locally; CI is green on `main`.
- [ ] `make release` works; `hdiutil attach` shows the app and the Applications link; `codesign --verify --deep --strict` accepts the app in the image.
- [ ] The downloaded image behaves as the README says on a clean account: download the `.dmg` from the release page **with a browser** (so that it is quarantined), on another Mac or a fresh user account, drag the app to Applications, go through the Gatekeeper steps exactly as written, open the panel, check the menu bar item, a storage scan (the folder permission question must name mac-pulse), an alert notification, Launch at login, and Check for updates (it must say you are up to date).
- [ ] After the release exists: the badges in the README render, and the "latest release" link downloads the image.
- [ ] Private vulnerability reporting and Discussions are enabled, because `SECURITY.md` and the issue chooser link to them.
