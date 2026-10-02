package collector

import (
	"context"
	"errors"
	"html"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Who signed the code at a path; "" when codesign gave no answer. SignApple and SignDeveloper
// are verified against Apple's anchors; SignUnverified is a signature whose certificate does
// not lead to them, so the name it carries is anybody's claim.
const (
	SignApple      = "apple"
	SignDeveloper  = "developer"
	SignAdHoc      = "adhoc"
	SignNone       = "none"
	SignUnverified = "unverified"
)

const (
	// whatis reads the whole manual index on a cold cache: 2.6 s measured, tens of milliseconds after.
	describeTimeout = 5 * time.Second
	// A bundle writes its own Info.plist and a program its own manual page: both are read up
	// to describeLimit bytes and shown up to describeRunes per field.
	describeLimit = 1 << 20
	describeRunes = 200
	// The certificate Apple signs every Mac App Store app with, whoever wrote the app.
	appStoreAuthority = "Apple Mac OS Application Signing"
	// The certificate leads to Apple's root and is a Developer ID or a Mac App Store one.
	developerRequirement = "anchor apple generic and (certificate leaf[field.1.2.840.113635.100.6.1.9] exists" +
		" or certificate leaf[field.1.2.840.113635.100.6.1.13] exists)"
)

var (
	// plutil prints the top-level keys of a property list with one tab; a nested dictionary's
	// keys (a document type's CFBundleTypeName) sit deeper and are not the bundle's own.
	plistString = regexp.MustCompile(`(?m)^\t<key>([^<]+)</key>\n\t<string>([^<]*)</string>`)
	// A process name is whatever its file is called; only a plain word is looked up in the manual.
	manualName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
)

// Description is what the disk says about a running app or process.
type Description struct {
	// The four below come from a bundle's Info.plist; "" for a bare executable or a key it lacks.
	Name      string
	Version   string
	BundleID  string
	Copyright string
	// Category is LSApplicationCategoryType without "public.app-category.": "developer-tools".
	Category string
	// Signing is one of the Sign constants; Signer names the developer for SignDeveloper.
	Signing string
	Signer  string
	// ExecutableOnly is true for a bundle: Signing then speaks for the program file alone. The
	// resources beside it (an Electron app's scripts, plug-ins) are sealed by the same signature
	// but checking that seal reads the whole bundle, minutes for a large app, so it is not done.
	ExecutableOnly bool
	// Manual is the one-line summary of the tool's man page, in English as the page has it.
	Manual string
}

// Describer caches a Description per path and executable for the session: what they are does
// not change while the process runs. The zero value is ready; any goroutine may call it.
type Describer struct {
	// lookup is held across the commands of one path, so a burst of calls runs them once.
	lookup sync.Mutex
	// lookups counts the paths read from disk.
	lookups int

	mu    sync.Mutex
	cache map[[2]string]Description
}

// capped keeps the first describeLimit bytes a command prints and swallows the rest.
type capped struct {
	buf []byte
}

func (c *capped) Write(p []byte) (int, error) {
	c.buf = append(c.buf, p[:min(len(p), describeLimit-len(c.buf))]...)
	return len(p), nil
}

// Describe reads the Info.plist of a .app path and the signature of exe, the program that
// runs: a bundle's seal says nothing about a file dropped into the bundle. An exe that is
// not absolute stands for path itself. A path that is no bundle gets the man page summary
// of the command manual instead; "" skips that lookup. It runs up to four commands, so it
// belongs off the message goroutine.
func (d *Describer) Describe(ctx context.Context, path, exe, manual string) Description {
	if !filepath.IsAbs(path) {
		return Description{}
	}
	if !filepath.IsAbs(exe) {
		exe = path
	}
	key := [2]string{path, exe}
	d.lookup.Lock()
	defer d.lookup.Unlock()
	d.mu.Lock()
	desc, ok := d.cache[key]
	d.mu.Unlock()
	if ok {
		return desc
	}
	d.lookups++
	final := true
	switch {
	case strings.HasSuffix(path, ".app"):
		out, _, done := describeOutput(ctx, nil, "plutil", "-convert", "xml1", "-o", "-", "--", filepath.Join(path, "Contents", "Info.plist"))
		desc, final = parseInfoPlist(out), done
	case manualName.MatchString(manual):
		// The process PATH is the system's four directories, and man derives its search path from
		// it: a tool from Homebrew or ~/go/bin keeps its page next to its own bin directory.
		pages := filepath.Join(filepath.Dir(path), "..", "share", "man") + ":/usr/share/man:/usr/local/share/man:/opt/homebrew/share/man"
		out, _, done := describeOutput(ctx, []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "MANPATH=" + pages}, "whatis", manual)
		desc.Manual, final = parseWhatis(out, manual), done
	}
	out, _, done := describeOutput(ctx, nil, "codesign", "-dv", "--verbose=2", "--", exe)
	signing, signer := parseCodesign(out)
	// The name codesign displays is whatever the certificate says, and anyone can make a
	// certificate called "Software Signing": only a verified chain decides.
	if signing == SignUnverified {
		verify := []string{"--verify", "--ignore-resources", "--", exe}
		_, apple, appleDone := describeOutput(ctx, nil, "codesign", append([]string{"-R=anchor apple"}, verify...)...)
		_, developer, developerDone := describeOutput(ctx, nil, "codesign", append([]string{"-R=" + developerRequirement}, verify...)...)
		switch {
		case !appleDone || !developerDone:
			signing, done = "", false
		case apple:
			signing = SignApple
		case developer:
			signing, desc.Signer = SignDeveloper, signer
		}
	}
	desc.Signing, desc.ExecutableOnly = signing, strings.HasSuffix(path, ".app")
	if final && done {
		d.mu.Lock()
		if d.cache == nil {
			d.cache = map[[2]string]Description{}
		}
		d.cache[key] = desc
		d.mu.Unlock()
	}
	return desc
}

// describeOutput runs one lookup. ok is exit status 0. done is false when the command was
// cut short, so that its answer is not cached; an exit status is an answer (codesign: not
// signed, whatis: no page).
func describeOutput(ctx context.Context, env []string, name string, args ...string) (out []byte, ok, done bool) {
	ctx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()
	var output capped
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = env, &output, &output
	// whatis is a shell script: its children outlive the kill and keep the output pipe open.
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	var exit *exec.ExitError
	return output.buf, err == nil, ctx.Err() == nil && (err == nil || errors.As(err, &exit))
}

// parseInfoPlist reads `plutil -convert xml1` of an Info.plist.
// A regexp over plutil's own layout instead of a plist decoder; a name localized in
// InfoPlist.strings (Chrome's copyright) is not read.
func parseInfoPlist(raw []byte) Description {
	keys := map[string]string{}
	for _, m := range plistString.FindAllSubmatch(raw, -1) {
		keys[string(m[1])] = html.UnescapeString(string(m[2]))
	}
	first := func(names ...string) string {
		for _, name := range names {
			if v := strings.TrimSpace(CleanText(keys[name], describeRunes)); v != "" {
				return v
			}
		}
		return ""
	}
	category := CleanText(strings.TrimPrefix(keys["LSApplicationCategoryType"], "public.app-category."), describeRunes)
	// Nineteen game genres ("action-games") read as one word.
	if strings.HasSuffix(category, "-games") {
		category = "games"
	}
	return Description{
		Name:      first("CFBundleDisplayName", "CFBundleName"),
		Version:   first("CFBundleShortVersionString", "CFBundleVersion"),
		BundleID:  first("CFBundleIdentifier"),
		Copyright: first("NSHumanReadableCopyright", "CFBundleGetInfoString"),
		Category:  category,
	}
}

// parseCodesign reads `codesign -dv --verbose=2`: SignNone, SignAdHoc, or SignUnverified with
// the name on the first Authority, the certificate that signed the code. codesign displays
// that name without checking where the certificate comes from.
func parseCodesign(out []byte) (signing, signer string) {
	var authority, team string
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasSuffix(line, "code object is not signed at all"):
			return SignNone, ""
		case line == "Signature=adhoc":
			return SignAdHoc, ""
		case strings.HasPrefix(line, "Authority=") && authority == "":
			authority = strings.TrimPrefix(line, "Authority=")
		case strings.HasPrefix(line, "TeamIdentifier="):
			team = strings.TrimPrefix(line, "TeamIdentifier=")
		}
	}
	if authority == "" {
		return "", ""
	}
	// "Developer ID Application: Google LLC (EQHXZ8M8AV)" names the developer; an App Store
	// certificate names only Apple, who is not the developer, and the team id is then all there is.
	if _, name, ok := strings.Cut(authority, ": "); ok {
		authority = name
	}
	known := team != "" && team != "not set"
	switch {
	case authority == appStoreAuthority && known:
		authority = "App Store, team " + team
	case authority == appStoreAuthority:
		authority = "App Store"
	case known && !strings.Contains(authority, team):
		authority += " (" + team + ")"
	}
	return SignUnverified, CleanText(authority, describeRunes)
}

// parseWhatis picks the summary of the command name out of `whatis name`, which also lists
// every page that merely mentions the word. Only sections 1 and 8 are commands.
func parseWhatis(out []byte, name string) string {
	for line := range strings.Lines(string(out)) {
		pages, summary, ok := strings.Cut(line, " - ")
		if !ok {
			continue
		}
		for page := range strings.SplitSeq(pages, ",") {
			title, section, _ := strings.Cut(strings.TrimSpace(page), "(")
			if title == name && (strings.HasPrefix(section, "1") || strings.HasPrefix(section, "8")) {
				return strings.TrimSpace(CleanText(summary, describeRunes))
			}
		}
	}
	return ""
}

// StartedAt is when a process was launched; the zero time when the kernel does not say.
func StartedAt(ctx context.Context, pid int32) time.Time {
	ms, err := (&process.Process{Pid: pid}).CreateTimeWithContext(ctx)
	if err != nil || ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}
