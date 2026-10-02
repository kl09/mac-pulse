package collector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInfoPlist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		input   string
		want    Description
	}{
		{
			name: "an excerpt of the iTerm bundle", fixture: "testdata/info_iterm.plist",
			want: Description{Name: "iTerm2", Version: "3.6.11", BundleID: "com.googlecode.iterm2", Copyright: "GPL v2", Category: "productivity"},
		},
		{
			name: "an excerpt of the Chrome bundle, which keeps its copyright in InfoPlist.strings", fixture: "testdata/info_chrome.plist",
			want: Description{Name: "Google Chrome", Version: "154.0.8037.92", BundleID: "com.google.Chrome"},
		},
		{
			name: "a nested key is not the bundle's, entities are decoded, a game genre is a game",
			input: "<dict>\n\t<key>CFBundleDocumentTypes</key>\n\t<array>\n\t\t<dict>\n" +
				"\t\t\t<key>CFBundleName</key>\n\t\t\t<string>Document</string>\n\t\t</dict>\n\t</array>\n" +
				"\t<key>CFBundleName</key>\n\t<string>Tom &amp; Jerry</string>\n" +
				"\t<key>CFBundleVersion</key>\n\t<string>42</string>\n" +
				"\t<key>CFBundleGetInfoString</key>\n\t<string>1.0, © Acme</string>\n" +
				"\t<key>LSApplicationCategoryType</key>\n\t<string>public.app-category.action-games</string>\n</dict>",
			want: Description{Name: "Tom & Jerry", Version: "42", Copyright: "1.0, © Acme", Category: "games"},
		},
		{name: "plutil could not read the file", input: "/x/Info.plist: file does not exist or is not readable"},
		{
			name: "what a bundle says about itself is cut and loses its bidi controls",
			input: "<dict>\n\t<key>CFBundleName</key>\n\t<string>Safari</string>\n" +
				"\t<key>CFBundleShortVersionString</key>\n\t<string>18.0 \u202egpj.exe</string>\n" +
				"\t<key>NSHumanReadableCopyright</key>\n\t<string>" + strings.Repeat("é", 10<<20) + "</string>\n</dict>",
			want: Description{Name: "Safari", Version: "18.0 gpj.exe", Copyright: strings.Repeat("é", describeRunes)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte(tt.input)
			if tt.fixture != "" {
				var err error
				raw, err = os.ReadFile(tt.fixture)
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want, parseInfoPlist(raw))
		})
	}
}

func TestParseCodesign(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fixture     string
		input       string
		wantSigning string
		wantSigner  string
	}{
		{
			name: "Developer ID names the developer", fixture: "testdata/codesign_developer.txt",
			wantSigning: SignUnverified, wantSigner: "Google LLC (EQHXZ8M8AV)",
		},
		{
			name:    "a certificate called Software Signing is not yet Apple's: the name is displayed, not verified",
			fixture: "testdata/codesign_apple.txt", wantSigning: SignUnverified, wantSigner: "Software Signing",
		},
		{
			name: "an App Store certificate names only Apple, so the team id stands in", fixture: "testdata/codesign_appstore.txt",
			wantSigning: SignUnverified, wantSigner: "App Store, team 74J34U3R6X",
		},
		{
			name:        "an App Store certificate without a team",
			input:       "Authority=Apple Mac OS Application Signing\nAuthority=Apple Root CA\nTeamIdentifier=not set\n",
			wantSigning: SignUnverified, wantSigner: "App Store",
		},
		{name: "a Homebrew build is signed by nobody in particular", fixture: "testdata/codesign_adhoc.txt", wantSigning: SignAdHoc},
		{name: "no signature at all", fixture: "testdata/codesign_unsigned.txt", wantSigning: SignNone},
		{name: "codesign said something else", input: "/gone: No such file or directory\n"},
		{name: "no output"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte(tt.input)
			if tt.fixture != "" {
				var err error
				raw, err = os.ReadFile(tt.fixture)
				require.NoError(t, err)
			}

			signing, signer := parseCodesign(raw)

			assert.Equal(t, tt.wantSigning, signing)
			assert.Equal(t, tt.wantSigner, signer)
		})
	}
}

func TestParseWhatis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fixture string
		input   string
		command string
		want    string
	}{
		{name: "a page shared by two names", fixture: "testdata/whatis_htop.txt", command: "htop", want: "interactive process viewer"},
		{name: "the command among pages that only mention the word", fixture: "testdata/whatis_ps.txt", command: "ps", want: "process status"},
		{name: "nothing appropriate", fixture: "testdata/whatis_none.txt", command: "gopls"},
		{name: "a file format of the same name is not the command", input: "rsync(5)                 - rsync wire protocol\n", command: "rsync"},
		{
			name: "a system daemon", command: "caffeinate",
			input: "caffeinate(8)            - prevent the system from sleeping on behalf of a utility\n",
			want:  "prevent the system from sleeping on behalf of a utility",
		},
		{name: "no output", command: "ls"},
		{
			name: "a page the program wrote itself is cut and loses its bidi controls", command: "evil",
			input: "evil(1) - \u202e" + strings.Repeat("x", 500) + "\n", want: strings.Repeat("x", describeRunes),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte(tt.input)
			if tt.fixture != "" {
				var err error
				raw, err = os.ReadFile(tt.fixture)
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want, parseWhatis(raw, tt.command))
		})
	}
}

func TestDescriber_Describe(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads the signatures and manual pages of the installed system")
	}

	// The linker signs a Go binary ad hoc.
	self, err := os.Executable()
	require.NoError(t, err)
	// A script may carry the manual page of a system daemon, next to its own bin directory.
	root := t.TempDir()
	forged := filepath.Join(root, "bin", "softwareupdated")
	claim := strings.Repeat("Apple system service. ", 20)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "share", "man", "man8"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(forged), 0o755))
	require.NoError(t, os.WriteFile(forged, []byte("#!/bin/sh\n"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "share", "man", "man8", "softwareupdated.8"),
		[]byte(".TH softwareupdated 8\n.SH NAME\nsoftwareupdated \\- "+claim+"\n"), 0o644))

	tests := []struct {
		name   string
		path   string
		manual string
		want   Description
	}{
		{
			name: "a system tool with a man page", path: "/bin/ls", manual: "ls",
			want: Description{Signing: SignApple, Manual: "list directory contents"},
		},
		{name: "a name that is no plain word is not looked up", path: "/bin/ls", manual: "-k ls", want: Description{Signing: SignApple}},
		{name: "a file without a signature", path: "/etc/hosts", want: Description{Signing: SignNone}},
		{name: "a binary signed ad hoc", path: self, want: Description{Signing: SignAdHoc}},
		{
			name: "an unsigned program's own manual page is cut", path: forged, manual: "softwareupdated",
			want: Description{Signing: SignNone, Manual: strings.TrimSpace(claim[:describeRunes])},
		},
		{name: "a process whose path is unknown", path: ""},
		{name: "a relative path", path: "ls", manual: "ls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var d Describer

			got := d.Describe(t.Context(), tt.path, "", tt.manual)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want, d.cache[[2]string{tt.path, tt.path}], "the answer is kept for the session")
		})
	}

	t.Run("an app Apple signed, by its verified chain", func(t *testing.T) {
		t.Parallel()

		var d Describer

		got := d.Describe(t.Context(), "/System/Applications/Calculator.app", "/System/Applications/Calculator.app/Contents/MacOS/Calculator", "")

		assert.Equal(t, SignApple, got.Signing)
		assert.Empty(t, got.Signer)
		assert.Equal(t, "com.apple.calculator", got.BundleID)
		assert.True(t, got.ExecutableOnly, "a bundle's signature is said to cover its program file alone")
	})

	t.Run("a burst of calls for one path reads it once", func(t *testing.T) {
		t.Parallel()

		var d Describer
		var wg sync.WaitGroup
		got := make([]Description, 200)

		for i := range got {
			wg.Go(func() { got[i] = d.Describe(t.Context(), "/bin/ls", "", "ls") })
		}
		wg.Wait()

		assert.Equal(t, 1, d.lookups)
		for _, desc := range got {
			assert.Equal(t, Description{Signing: SignApple, Manual: "list directory contents"}, desc)
		}
	})
}

// The signature read is the running program's, verified against Apple's anchors: neither
// the bundle around it nor the name on its certificate decides.
func TestDescriber_Describe_Signature(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads the signatures and manual pages of the installed system")
	}

	// The linker signs a Go binary ad hoc.
	self, err := os.Executable()
	require.NoError(t, err)

	t.Run("a program dropped into a copy of Apple's bundle is judged by its own signature", func(t *testing.T) {
		t.Parallel()

		bundle := filepath.Join(t.TempDir(), "Calculator.app")
		require.NoError(t, exec.CommandContext(t.Context(), "cp", "-R", "/System/Applications/Calculator.app", bundle).Run())
		dropped := filepath.Join(bundle, "Contents", "MacOS", "dropped")
		require.NoError(t, exec.CommandContext(t.Context(), "cp", self, dropped).Run())
		var d Describer

		got := d.Describe(t.Context(), bundle, dropped, "")
		own := d.Describe(t.Context(), bundle, filepath.Join(bundle, "Contents", "MacOS", "Calculator"), "")

		assert.Equal(t, SignAdHoc, got.Signing)
		assert.Empty(t, got.Signer)
		assert.Equal(t, "com.apple.calculator", got.BundleID, "the bundle still says what it is")
		assert.Equal(t, SignApple, own.Signing, "the bundle's own program keeps Apple's signature")
	})

	t.Run("a certificate that only calls itself Software Signing is not Apple's", func(t *testing.T) {
		t.Parallel()

		// A home of its own: the keychain search list codesign reads is a preference of the
		// home folder, and the real one stays as it is.
		home := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(home, "Library", "Preferences"), 0o700))
		run := func(name string, args ...string) {
			cmd := exec.CommandContext(t.Context(), name, args...)
			cmd.Dir, cmd.Env = home, append(os.Environ(), "HOME="+home)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s %v: %s", name, args, out)
		}
		keychain, tool := filepath.Join(home, "scratch.keychain"), filepath.Join(home, "tool")
		run("/usr/bin/openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", "key.pem", "-out", "cert.pem", "-days", "2",
			"-subj", "/CN=Software Signing", "-addext", "keyUsage=critical,digitalSignature", "-addext", "extendedKeyUsage=critical,codeSigning")
		run("/usr/bin/openssl", "pkcs12", "-export", "-inkey", "key.pem", "-in", "cert.pem", "-out", "id.p12", "-passout", "pass:x")
		run("security", "create-keychain", "-p", "x", keychain)
		run("security", "import", "id.p12", "-k", keychain, "-P", "x", "-T", "/usr/bin/codesign")
		// Without it codesign asks for the keychain password in a dialog.
		run("security", "set-key-partition-list", "-S", "apple-tool:,apple:", "-s", "-k", "x", keychain)
		run("security", "list-keychains", "-d", "user", "-s", keychain)
		run("cp", self, tool)
		run("codesign", "-f", "-s", "Software Signing", "--keychain", keychain, tool)
		var d Describer

		got := d.Describe(t.Context(), tool, "", "")

		assert.Equal(t, Description{Signing: SignUnverified}, got)
	})
}

func TestDescribeOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// timeout is the caller's own, shorter than describeTimeout; 0 leaves that one alone.
		timeout  time.Duration
		script   string
		wantOut  string
		wantLen  int
		wantOK   bool
		wantDone bool
		// wantWithin bounds the call when not 0.
		wantWithin time.Duration
	}{
		{name: "exit 0 is ok", script: "echo out; echo err >&2", wantOut: "out\nerr\n", wantLen: 8, wantOK: true, wantDone: true},
		{name: "an exit status is an answer, not ok", script: "echo no; exit 3", wantOut: "no\n", wantLen: 3, wantDone: true},
		{
			name: "output past the limit is swallowed and the command still ends", script: "head -c 3000000 /dev/zero",
			wantLen: describeLimit, wantOK: true, wantDone: true,
		},
		{
			name: "a child that keeps the pipe open after the kill does not hold the call", timeout: 100 * time.Millisecond,
			script: "sleep 2 & wait", wantWithin: 1500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}
			start := time.Now()

			out, ok, done := describeOutput(ctx, nil, "sh", "-c", tt.script)

			assert.Len(t, out, tt.wantLen)
			if tt.wantOut != "" {
				assert.Equal(t, tt.wantOut, string(out))
			}
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantDone, done)
			if tt.wantWithin > 0 {
				assert.Less(t, time.Since(start), tt.wantWithin)
			}
		})
	}
}

func TestStartedAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pid  int32
		want bool
	}{
		{name: "this process", pid: int32(os.Getpid()), want: true},
		{name: "a pid nothing runs under", pid: 1<<31 - 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, !StartedAt(t.Context(), tt.pid).IsZero())
		})
	}
}
