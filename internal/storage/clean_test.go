package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// userImmutable is UF_IMMUTABLE of chflags(2): the file can be neither renamed nor removed.
const userImmutable = 0x2

// fakeHome builds a home folder with caches of known sizes, a Trash and data no cleanup may touch.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for file, size := range map[string]int{
		"Library/Caches/com.example.app/c":      2 << 20,
		"Library/Caches/com.example.app/d/e":    1 << 20,
		"Library/Caches/loose.db":               4096,
		"Library/Caches/com.apple.fake/x":       1 << 20,
		"Library/Caches/CloudKit/x":             1 << 20,
		"Library/Caches/mac-pulse-webview/x":    1 << 20,
		"Library/Caches/go-build/ab/o":          2 << 20,
		"Library/Caches/go-build/cd/o":          1 << 20,
		"Library/Caches/go-build/trim.txt":      4096,
		"Library/Logs/app.log":                  8192,
		"Library/Application Support/Keep/data": 1 << 20,
		".Trash/old.zip":                        1 << 20,
	} {
		path := filepath.Join(home, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600))
	}
	return home
}

func TestCategories(t *testing.T) {
	t.Parallel()

	categories := Categories("/Users/alex")

	ids := make([]string, 0, len(categories))
	for _, c := range categories {
		ids = append(ids, c.ID)
		require.True(t, strings.HasPrefix(c.Dir, "/Users/alex/"), "%s cleans %s, outside the home folder", c.ID, c.Dir)
		for _, never := range []string{"Application Support", "Containers", "Keychains", "Mail", "Photos"} {
			assert.NotContains(t, c.Dir, never, c.ID)
		}
		assert.Equal(t, c.ID == "trash", c.Permanent, c.ID)
	}
	assert.Equal(t, []string{
		"app_caches", "logs", "xcode_derived", "simulator_caches", "npm", "yarn", "pnpm", "go_build", "pip", "homebrew", "trash",
	}, ids)
}

//nolint:funlen // one table for the function under test; the length is the cases.
func TestMeasure(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		// prepare changes the fake home before it is measured.
		prepare func(t *testing.T, home string)
		// want maps a category id to what it must hold; categories not named must be absent.
		want map[string]Measured
	}{
		{
			name: "each category lists its own entries; Apple's caches, mac-pulse's own and the folders of other categories are left out",
			want: map[string]Measured{
				"app_caches": {Present: true, Bytes: 3<<20 + 4096, Entries: []string{"Library/Caches/com.example.app", "Library/Caches/loose.db"}},
				"logs":       {Present: true, Bytes: 8192, Entries: []string{"Library/Logs/app.log"}},
				"go_build": {Present: true, Bytes: 3<<20 + 4096, Entries: []string{
					"Library/Caches/go-build/ab", "Library/Caches/go-build/cd", "Library/Caches/go-build/trim.txt",
				}},
				"trash": {Present: true, Bytes: 1 << 20, Entries: []string{".Trash/old.zip"}},
			},
		},
		{
			name: "a folder that cannot be read is present and denied, an entry that cannot be read is not listed",
			prepare: func(t *testing.T, home string) {
				t.Helper()
				for _, dir := range []string{".Trash", "Library/Caches/go-build/ab"} {
					require.NoError(t, os.Chmod(filepath.Join(home, dir), 0))
					t.Cleanup(func() { _ = os.Chmod(filepath.Join(home, dir), 0o700) })
				}
			},
			want: map[string]Measured{
				"app_caches": {Present: true, Bytes: 3<<20 + 4096, Entries: []string{"Library/Caches/com.example.app", "Library/Caches/loose.db"}},
				"logs":       {Present: true, Bytes: 8192, Entries: []string{"Library/Logs/app.log"}},
				"go_build":   {Present: true, Bytes: 1<<20 + 4096, Entries: []string{"Library/Caches/go-build/cd", "Library/Caches/go-build/trim.txt"}},
				"trash":      {Present: true, Denied: true, Entries: []string{}},
			},
		},
		{
			name: "a category folder that is a symlink is absent, and a symlink inside one is an entry of its own size",
			prepare: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.RemoveAll(filepath.Join(home, "Library/Logs")))
				require.NoError(t, os.Symlink(filepath.Join(home, "Library/Application Support"), filepath.Join(home, "Library/Logs")))
				require.NoError(t, os.Symlink(filepath.Join(home, "Library/Application Support"), filepath.Join(home, "Library/Caches/go-build/link")))
			},
			want: map[string]Measured{
				"app_caches": {Present: true, Bytes: 3<<20 + 4096, Entries: []string{"Library/Caches/com.example.app", "Library/Caches/loose.db"}},
				"go_build": {Present: true, Bytes: 3<<20 + 4096, Entries: []string{
					"Library/Caches/go-build/ab", "Library/Caches/go-build/cd", "Library/Caches/go-build/link", "Library/Caches/go-build/trim.txt",
				}},
				"trash": {Present: true, Bytes: 1 << 20, Entries: []string{".Trash/old.zip"}},
			},
		},
		{
			name: "a category folder reached through a symlinked parent is absent, though the link stays inside the home folder",
			prepare: func(t *testing.T, home string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(home, "Documents/store"), 0o700))
				require.NoError(t, os.WriteFile(filepath.Join(home, "Documents/store/thesis.txt"), []byte("x"), 0o600))
				require.NoError(t, os.Symlink(filepath.Join(home, "Documents"), filepath.Join(home, "Library/pnpm")))
			},
			want: map[string]Measured{
				"app_caches": {Present: true, Bytes: 3<<20 + 4096, Entries: []string{"Library/Caches/com.example.app", "Library/Caches/loose.db"}},
				"logs":       {Present: true, Bytes: 8192, Entries: []string{"Library/Logs/app.log"}},
				"go_build": {Present: true, Bytes: 3<<20 + 4096, Entries: []string{
					"Library/Caches/go-build/ab", "Library/Caches/go-build/cd", "Library/Caches/go-build/trim.txt",
				}},
				"trash": {Present: true, Bytes: 1 << 20, Entries: []string{".Trash/old.zip"}},
			},
		},
		{name: "a cancelled measurement reads nothing", ctx: cancelled, want: map[string]Measured{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			home := fakeHome(t)
			if tt.prepare != nil {
				tt.prepare(t, home)
			}
			ctx := tt.ctx
			if ctx == nil {
				ctx = t.Context()
			}

			got := Measure(ctx, home)

			categories := Categories(home)
			require.Len(t, got, len(categories))
			for i, m := range got {
				want := tt.want[m.ID]
				want.Category = categories[i]
				entries := make([]string, 0, len(want.Entries))
				for _, entry := range want.Entries {
					entries = append(entries, filepath.Join(home, entry))
				}
				want.Entries = entries
				// A folder takes blocks of its own: the files are the floor, 64 KiB of folders the ceiling.
				assert.GreaterOrEqual(t, m.Bytes, want.Bytes, m.ID)
				assert.Less(t, m.Bytes, want.Bytes+64<<10, m.ID)
				assert.WithinDuration(t, time.Now(), m.At, time.Minute, m.ID)
				require.Len(t, m.Sizes, len(m.Entries), m.ID)
				var sum int64
				for _, size := range m.Sizes {
					sum += size
				}
				assert.Equal(t, m.Bytes, sum, "%s: the sizes of the entries add up to the category", m.ID)
				want.Bytes, want.Sizes, want.At = m.Bytes, m.Sizes, m.At
				assert.Equal(t, want, m, m.ID)
			}
		})
	}

	t.Run("only the categories asked for", func(t *testing.T) {
		t.Parallel()

		got := Measure(t.Context(), fakeHome(t), "trash", "logs")

		require.Len(t, got, 2)
		assert.Equal(t, "logs", got[0].ID)
		assert.Equal(t, "trash", got[1].ID)
		assert.Len(t, got[1].Entries, 1)
	})

	t.Run("a folder on another volume is a mount point: neither listed nor counted", func(t *testing.T) {
		t.Parallel()

		home := fakeHome(t)
		m := Measured{Category: Categories(home)[7], Entries: []string{}}
		info, err := os.Lstat(m.Dir)
		require.NoError(t, err)

		// Every folder of the tree is then on a volume other than the walk's.
		got := measure(t.Context(), m, device(info)+1)

		assert.Equal(t, []string{filepath.Join(m.Dir, "trim.txt")}, got.Entries)
		assert.Less(t, got.Bytes, int64(64<<10))
	})
}

//nolint:funlen // one table for the function under test; the length is the cases.
func TestClean(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	// Paths are relative to the fake home.
	buildCache := []string{"Library/Caches/go-build/ab", "Library/Caches/go-build/cd", "Library/Caches/go-build/trim.txt"}
	untouched := []string{
		"Library/Application Support/Keep/data", "Library/Caches/com.example.app/c", "Library/Caches/com.apple.fake/x", ".Trash/old.zip",
		"Library/Logs/app.log",
	}

	tests := []struct {
		name     string
		ctx      context.Context
		category string
		// prepare changes the fake home, or the measurement, after measuring.
		prepare   func(t *testing.T, home string, m *Measured)
		permanent bool
		// twice cleans a second, equal set of entries right after the first.
		twice       bool
		want        Cleaned
		wantErr     string
		wantNoTrash bool
		// wantTrashed are the names inside the one wrapper folder of the Trash, wantLeft and
		// wantGone the paths that must and must not exist; the category folder itself always stays.
		wantTrashed []string
		wantLeft    []string
		wantGone    []string
	}{
		{
			name: "entries move into one wrapper folder in the Trash, the category folder and everything else stay", category: "go_build",
			want: Cleaned{Bytes: 3<<20 + 4096, Items: 3}, wantTrashed: []string{"ab", "cd", "trim.txt"},
			wantLeft: append([]string{"Library/Caches/go-build"}, untouched...), wantGone: buildCache,
		},
		{
			name: "a second cleanup in the same second gets a wrapper of its own", category: "go_build", twice: true,
			want: Cleaned{Bytes: 3<<20 + 4096, Items: 3}, wantLeft: untouched, wantGone: buildCache,
		},
		{
			name: "permanent removes for good and makes no wrapper", category: "go_build", permanent: true,
			want: Cleaned{Bytes: 3<<20 + 4096, Items: 3}, wantLeft: append([]string{"Library/Caches/go-build"}, untouched...), wantGone: buildCache,
		},
		{
			name: "the Trash is emptied only for good", category: "trash", permanent: true,
			want: Cleaned{Bytes: 1 << 20, Items: 1}, wantLeft: append([]string{".Trash"}, untouched[:3]...), wantGone: []string{".Trash/old.zip"},
		},
		{
			name: "the Trash cannot be moved into the Trash", category: "trash",
			wantErr: "cannot go to the Trash", wantLeft: untouched,
		},
		{
			name: "an entry that went away since the measurement counts as failed, the others move", category: "go_build",
			prepare: func(t *testing.T, home string, _ *Measured) {
				t.Helper()
				require.NoError(t, os.RemoveAll(filepath.Join(home, "Library/Caches/go-build/cd")))
			},
			want: Cleaned{Bytes: 2<<20 + 4096, Items: 2, Failed: 1}, wantTrashed: []string{"ab", "trim.txt"},
			wantLeft: untouched, wantGone: buildCache,
		},
		{
			name: "a Trash that cannot be written leaves everything in place", category: "go_build",
			prepare: func(t *testing.T, home string, _ *Measured) {
				t.Helper()
				require.NoError(t, os.Chmod(filepath.Join(home, ".Trash"), 0o500))
				t.Cleanup(func() { _ = os.Chmod(filepath.Join(home, ".Trash"), 0o700) })
			},
			wantNoTrash: true, wantLeft: append(buildCache, untouched...),
		},
		{
			name: "a Trash that is a link to another folder of the home is no Trash", category: "go_build",
			prepare: func(t *testing.T, home string, _ *Measured) {
				t.Helper()
				require.NoError(t, os.RemoveAll(filepath.Join(home, ".Trash")))
				require.NoError(t, os.Mkdir(filepath.Join(home, "Documents"), 0o700))
				require.NoError(t, os.Symlink(filepath.Join(home, "Documents"), filepath.Join(home, ".Trash")))
			},
			wantNoTrash: true, wantLeft: append(buildCache, untouched[:3]...),
		},
		{
			name: "a Trash that is a file is no Trash", category: "go_build",
			prepare: func(t *testing.T, home string, _ *Measured) {
				t.Helper()
				require.NoError(t, os.RemoveAll(filepath.Join(home, ".Trash")))
				require.NoError(t, os.WriteFile(filepath.Join(home, ".Trash"), nil, 0o600))
			},
			wantNoTrash: true, wantLeft: append(buildCache, untouched[:3]...),
		},
		{
			name: "a home without a Trash folder gets one", category: "go_build",
			prepare: func(t *testing.T, home string, _ *Measured) {
				t.Helper()
				require.NoError(t, os.RemoveAll(filepath.Join(home, ".Trash")))
			},
			want: Cleaned{Bytes: 3<<20 + 4096, Items: 3}, wantTrashed: []string{"ab", "cd", "trim.txt"},
			wantLeft: untouched[:3], wantGone: buildCache,
		},
		{
			name: "a category folder reached through a symlinked parent inside the home folder is refused", category: "go_build",
			prepare: func(t *testing.T, home string, _ *Measured) {
				t.Helper()
				require.NoError(t, os.Rename(filepath.Join(home, "Library/Caches"), filepath.Join(home, "Documents")))
				require.NoError(t, os.Symlink(filepath.Join(home, "Documents"), filepath.Join(home, "Library/Caches")))
			},
			wantErr: "not a folder inside the home folder", wantLeft: append(buildCache, untouched...),
		},
		{
			name: "an entry that will not move stays and counts as failed", category: "go_build",
			prepare: func(t *testing.T, home string, _ *Measured) {
				t.Helper()
				locked := filepath.Join(home, "Library/Caches/go-build/cd")
				require.NoError(t, syscall.Chflags(locked, userImmutable))
				t.Cleanup(func() { _ = syscall.Chflags(locked, 0) })
			},
			want: Cleaned{Bytes: 2<<20 + 4096, Items: 2, Failed: 1}, wantTrashed: []string{"ab", "trim.txt"},
			wantLeft: append([]string{"Library/Caches/go-build/cd/o"}, untouched...), wantGone: []string{buildCache[0], buildCache[2]},
		},
		{
			name: "a category folder replaced by a symlink is refused", category: "go_build",
			prepare: func(t *testing.T, home string, m *Measured) {
				t.Helper()
				require.NoError(t, os.Rename(m.Dir, m.Dir+".real"))
				require.NoError(t, os.Symlink(filepath.Join(home, "Library/Application Support"), m.Dir))
				m.Entries = []string{filepath.Join(m.Dir, "Keep")}
			},
			permanent: true, wantErr: "not a folder inside the home folder", wantLeft: untouched,
		},
		{
			name: "a category folder that resolves outside the home folder is refused", category: "logs",
			prepare: func(t *testing.T, home string, _ *Measured) {
				t.Helper()
				outside := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(outside, "Logs"), nil, 0o600))
				require.NoError(t, os.Rename(filepath.Join(home, "Library"), filepath.Join(outside, "Library")))
				require.NoError(t, os.Symlink(filepath.Join(outside, "Library"), filepath.Join(home, "Library")))
			},
			permanent: true, wantErr: "not a folder inside the home folder", wantLeft: untouched,
		},
		{
			name: "an entry outside the category folder is refused and nothing is removed", category: "go_build",
			prepare: func(t *testing.T, home string, m *Measured) {
				t.Helper()
				m.Entries = append(m.Entries, filepath.Join(home, "Library/Application Support/Keep"))
			},
			permanent: true, wantErr: "an entry is not inside", wantLeft: append(buildCache, untouched...),
		},
		{
			name: "an entry that climbs out of the category folder is refused", category: "go_build",
			prepare: func(t *testing.T, home string, m *Measured) {
				t.Helper()
				m.Entries = []string{m.Dir + "/../../Application Support/Keep"}
			},
			permanent: true, wantErr: "an entry is not inside", wantLeft: append(buildCache, untouched...),
		},
		{
			name: "an entry that names the category folder itself or its parent is not removed", category: "go_build",
			prepare: func(t *testing.T, _ string, m *Measured) {
				t.Helper()
				m.Entries = []string{m.Dir + "/.", m.Dir + "/.."}
			},
			permanent: true, want: Cleaned{Failed: 2}, wantLeft: append(buildCache, untouched...),
		},
		{
			name: "a cancelled cleanup removes nothing and counts every entry as failed", category: "go_build", ctx: cancelled,
			want: Cleaned{Failed: 3}, wantLeft: append(buildCache, untouched...),
		},
		{
			name: "a category that is gone is an error", category: "go_build",
			prepare: func(t *testing.T, _ string, m *Measured) {
				t.Helper()
				require.NoError(t, os.RemoveAll(m.Dir))
			},
			wantErr: "no such file or directory", wantLeft: untouched,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			home := fakeHome(t)
			measured := Measure(t.Context(), home)
			var m Measured
			for _, c := range measured {
				if c.ID == tt.category {
					m = c
				}
			}
			require.NotEmpty(t, m.Entries)
			if tt.prepare != nil {
				tt.prepare(t, home, &m)
			}
			ctx := tt.ctx
			if ctx == nil {
				ctx = t.Context()
			}

			got, err := Clean(ctx, home, m, tt.permanent)

			switch {
			case tt.wantNoTrash:
				require.ErrorIs(t, err, ErrNoTrash)
			case tt.wantErr != "":
				require.ErrorContains(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
			}
			assert.GreaterOrEqual(t, got.Bytes, tt.want.Bytes)
			assert.Less(t, got.Bytes, tt.want.Bytes+64<<10)
			wrappers, _ := filepath.Glob(filepath.Join(home, ".Trash", "mac-pulse-*"))
			assert.Equal(t, strings.Join(wrappers, ""), got.Wrapper, "the folder in the Trash is named when there is one")
			got.Bytes, got.Wrapper = tt.want.Bytes, ""
			assert.Equal(t, tt.want, got)
			if tt.twice {
				// The same names again: a reused wrapper would have failed the move or replaced the first set.
				for _, entry := range m.Entries {
					require.NoError(t, os.WriteFile(entry, []byte("again"), 0o600))
				}
				again, err := Clean(t.Context(), home, m, false)
				require.NoError(t, err)
				assert.Equal(t, 3, again.Items)
				both, _ := filepath.Glob(filepath.Join(home, ".Trash", "mac-pulse-go_build-*"))
				require.Len(t, both, 2)
				require.Len(t, wrappers, 1)
				info, err := os.Stat(filepath.Join(wrappers[0], "ab"))
				require.NoError(t, err)
				assert.True(t, info.IsDir(), "the first cleanup's folder is still in its own wrapper")
				return
			}
			if tt.wantTrashed == nil {
				assert.Empty(t, wrappers, "no wrapper folder is left behind")
			} else {
				require.Len(t, wrappers, 1)
				assert.Regexp(t, `/mac-pulse-`+tt.category+`-\d{8}-\d{6}-[A-Z2-7]{8}$`, wrappers[0], "nobody can take the name beforehand")
				entries, err := os.ReadDir(wrappers[0])
				require.NoError(t, err)
				names := make([]string, 0, len(entries))
				for _, e := range entries {
					if e.Name() != noteName {
						names = append(names, e.Name())
					}
				}
				assert.Equal(t, tt.wantTrashed, names)
				note, err := os.ReadFile(filepath.Join(wrappers[0], noteName))
				require.NoError(t, err)
				assert.Contains(t, string(note), "from "+m.Dir+"\n", "the note names the folder to drag the entries back to")
			}
			for _, path := range tt.wantLeft {
				_, err := os.Stat(filepath.Join(home, path))
				require.NoError(t, err, path)
			}
			for _, path := range tt.wantGone {
				assert.NoFileExists(t, filepath.Join(home, path))
				assert.NoDirExists(t, filepath.Join(home, path))
			}
		})
	}

	t.Run("an entry that is or holds a folder of another volume stays, the files beside it move", func(t *testing.T) {
		t.Parallel()

		home := fakeHome(t)
		m := Measure(t.Context(), home, "go_build")[0]
		info, err := os.Lstat(m.Dir)
		require.NoError(t, err)
		root, err := os.OpenRoot(m.Dir)
		require.NoError(t, err)
		defer func() { _ = root.Close() }()
		var moved []string

		// Every folder of the tree is then on a volume other than the walk's.
		got := remove(t.Context(), m, root, device(info)+1, func(name string) error {
			moved = append(moved, name)
			return nil
		})

		assert.Equal(t, Cleaned{Bytes: got.Bytes, Items: 1, Failed: 2}, got)
		assert.Equal(t, []string{"trim.txt"}, moved)
	})
}

// A program of the same user swaps the category folder for a link to the home folder while
// the cleanup runs, so that the entry "Documents" would resolve to ~/Documents.
func TestClean_FolderSwappedMidRun(t *testing.T) {
	t.Parallel()

	for _, permanent := range []bool{false, true} {
		t.Run(fmt.Sprintf("permanent %t", permanent), func(t *testing.T) {
			t.Parallel()

			for range 40 {
				home := t.TempDir()
				logs := filepath.Join(home, "Library", "Logs")
				files := []string{"Documents/thesis.txt", "Library/Logs/0-trigger/f", "Library/Logs/Documents/decoy"}
				// Sizing the buffer keeps the cleanup busy between its first and its last removal.
				for i := range 100 {
					files = append(files, fmt.Sprintf("Library/Logs/1-buffer/d%d/f%d", i%10, i))
				}
				for _, file := range files {
					require.NoError(t, os.MkdirAll(filepath.Join(home, filepath.Dir(file)), 0o700))
					require.NoError(t, os.WriteFile(filepath.Join(home, file), []byte("x"), 0o600))
				}
				m := Measure(t.Context(), home, "logs")[0]
				require.Len(t, m.Entries, 3)
				var stop atomic.Bool
				swapped := make(chan struct{})
				go func() {
					defer close(swapped)
					for !stop.Load() {
						if _, err := os.Lstat(filepath.Join(logs, "0-trigger")); err != nil {
							break
						}
					}
					_ = os.Rename(logs, logs+".real")
					_ = os.Symlink(home, logs)
				}()

				got, err := Clean(t.Context(), home, m, permanent)
				stop.Store(true)
				<-swapped

				require.NoError(t, err)
				assert.Equal(t, Cleaned{Bytes: got.Bytes, Items: 3, Wrapper: got.Wrapper}, got, "every entry left the folder that was checked")
				require.FileExists(t, filepath.Join(home, "Documents", "thesis.txt"))
				entries, err := os.ReadDir(logs + ".real")
				require.NoError(t, err)
				assert.Empty(t, entries)
				if !permanent {
					assert.FileExists(t, filepath.Join(got.Wrapper, "Documents", "decoy"))
					assert.NoFileExists(t, filepath.Join(got.Wrapper, "Documents", "thesis.txt"))
				}
			}
		})
	}
}
