package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/storage"
)

func TestBuildStorage(t *testing.T) {
	t.Parallel()

	const home = "/Users/alex"
	measured := storage.Measure(t.Context(), home)
	measured[7].Bytes, measured[7].Entries = 4096, []string{home + "/Library/Caches/go-build/ab"}
	tests := []struct {
		name        string
		in          StorageInput
		wantScan    StorageScan
		wantCleanup string
		wantGoBuild CleanupCategory
	}{
		{
			name: "nothing asked for yet", wantScan: StorageScan{State: "idle"}, wantCleanup: "idle",
			wantGoBuild: CleanupCategory{ID: "go_build"},
		},
		{
			name:     "a running scan of the home folder, a measurement under way",
			in:       StorageInput{Scan: StorageScan{State: "running", Root: home, Files: 7, Bytes: 9, Denied: 1}, Measuring: true},
			wantScan: StorageScan{State: "running", Root: "~", Files: 7, Bytes: 9, Denied: 1}, wantCleanup: "running",
			wantGoBuild: CleanupCategory{ID: "go_build"},
		},
		{
			name:     "a finished scan below home, categories measured",
			in:       StorageInput{Scan: StorageScan{State: "done", Root: home + "/proj"}, Measured: measured},
			wantScan: StorageScan{State: "done", Root: "~/proj"}, wantCleanup: "done",
			wantGoBuild: CleanupCategory{ID: "go_build", Bytes: 4096, Items: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BuildStorage(tt.in, home)

			assert.Equal(t, tt.wantScan, got.Scan)
			assert.Equal(t, tt.wantCleanup, got.Cleanup.State)
			require.Len(t, got.Cleanup.Categories, 11)
			assert.Equal(t, tt.wantGoBuild, got.Cleanup.Categories[7])
			assert.Equal(t, CleanupCategory{ID: "trash", Permanent: true}, got.Cleanup.Categories[10])
		})
	}
}

func TestBuildLevel(t *testing.T) {
	t.Parallel()

	const home = "/Users/alex"
	many := &storage.Node{Name: "many", Dir: true}
	for i := range maxLevelEntries + 3 {
		many.Children = append(many.Children, &storage.Node{Name: fmt.Sprintf("f%03d", i), Bytes: int64(1000 - i), Files: 1})
	}
	caches := &storage.Node{Name: "Caches", Dir: true, Bytes: 30, Files: 3, Children: []*storage.Node{
		{Name: "go-build", Dir: true, Bytes: 10, Files: 1},
		{Name: "evil\u202ename", Bytes: 20, Files: 2},
		{Name: "closed", Dir: true, Denied: true},
	}}
	tree := &storage.Node{Name: "alex", Dir: true, Bytes: 99, Files: 9, Children: []*storage.Node{
		{Name: "Library", Dir: true, Bytes: 37, Files: 4, Children: []*storage.Node{caches, {Name: "Logs", Bytes: 7, Files: 1}}},
		many,
		{Name: "file", Bytes: 5, Files: 1},
	}}
	tests := []struct {
		name   string
		rel    string
		want   StorageLevel
		wantOK bool
	}{
		{
			name: "children by size, a cleanup category marked, names cleaned", rel: "Library/Caches", wantOK: true,
			want: StorageLevel{Path: "Library/Caches", Bytes: 30, Files: 3, Children: []StorageEntry{
				{Name: "evilname", Bytes: 20, Files: 2},
				{Name: "go-build", Bytes: 10, Files: 1, Dir: true, Category: "go_build"},
				{Name: "closed", Dir: true, Denied: true},
			}},
		},
		{
			name: "a file where a cleanup category's folder would be is not that category", rel: "Library", wantOK: true,
			want: StorageLevel{Path: "Library", Bytes: 37, Files: 4, Children: []StorageEntry{
				{Name: "Caches", Bytes: 30, Files: 3, Dir: true, Category: "app_caches"}, {Name: "Logs", Bytes: 7, Files: 1},
			}},
		},
		{name: "no such folder", rel: "Music"},
		{name: "a file is not a level", rel: "file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := BuildLevel(tree, home, tt.rel, storage.Categories(home))

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("the tail beyond the cap is one nameless entry", func(t *testing.T) {
		t.Parallel()

		got, ok := BuildLevel(tree, home, "many", nil)

		require.True(t, ok)
		require.Len(t, got.Children, maxLevelEntries+1)
		assert.Equal(t, "f000", got.Children[0].Name)
		assert.Equal(t, StorageEntry{Bytes: 800 + 799 + 798, Files: 3}, got.Children[maxLevelEntries])
	})

	t.Run("the root of an empty scan", func(t *testing.T) {
		t.Parallel()

		got, ok := BuildLevel(&storage.Node{Name: "alex", Dir: true}, home, "", nil)

		require.True(t, ok)
		assert.Equal(t, StorageLevel{Children: []StorageEntry{}}, got)
	})
}

func TestCleanTarget(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	entries := []string{"/Users/alex/Library/Caches/go-build/ab"}
	measured := []storage.Measured{
		{Category: storage.Category{ID: "go_build"}, Entries: entries, At: now.Add(-time.Minute)},
		{Category: storage.Category{ID: "logs"}, Entries: []string{}, At: now},
		{Category: storage.Category{ID: "npm"}, Entries: entries, At: now.Add(-5*time.Minute - time.Second)},
		{Category: storage.Category{ID: "pip"}, Entries: entries, At: now.Add(-5 * time.Minute)},
	}
	measured = append(measured, storage.Measured{Category: storage.Category{ID: "trash", Permanent: true}, Entries: entries, At: now})
	// id is the review id of a category as the cleanup_list answer carried it.
	id := func(category string) string {
		i := slices.IndexFunc(measured, func(m storage.Measured) bool { return m.ID == category })
		return strconv.FormatInt(measured[i].At.UnixNano(), 10)
	}
	tests := []struct {
		name     string
		measured []storage.Measured
		msg      Message
		noTrash  bool
		wantOK   bool
	}{
		{name: "a category measured a minute ago", measured: measured, msg: Message{Category: "go_build", Review: id("go_build")}, wantOK: true},
		{name: "measured exactly five minutes ago", measured: measured, msg: Message{Category: "pip", Review: id("pip")}, wantOK: true},
		{name: "a measurement older than five minutes", measured: measured, msg: Message{Category: "npm", Review: id("npm")}},
		{name: "a category with nothing to remove", measured: measured, msg: Message{Category: "logs", Review: id("logs")}},
		{name: "a category the measurement does not have", measured: measured, msg: Message{Category: "yarn", Review: id("go_build")}},
		{name: "nothing measured yet", msg: Message{Category: "go_build", Review: id("go_build")}},
		{name: "a cleanup that names no review", measured: measured, msg: Message{Category: "go_build"}},
		{
			name:     "the review of a listing that was replaced: the category was listed or measured again",
			measured: measured, msg: Message{Category: "go_build", Review: strconv.FormatInt(now.Add(-2*time.Minute).UnixNano(), 10)},
		},
		{name: "the review of another category", measured: measured, msg: Message{Category: "go_build", Review: id("pip")}},
		{
			name: "for good, with a Trash nobody has seen fail", measured: measured,
			msg: Message{Category: "go_build", Review: id("go_build"), Permanent: true},
		},
		{
			name: "for good, after the category's last cleanup found no Trash", measured: measured,
			msg: Message{Category: "go_build", Review: id("go_build"), Permanent: true}, noTrash: true, wantOK: true,
		},
		{
			name: "the Trash itself is emptied for good without that", measured: measured,
			msg: Message{Category: "trash", Review: id("trash"), Permanent: true}, wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := CleanTarget(tt.measured, tt.msg, tt.noTrash, now)

			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.msg.Category, got.ID)
				assert.Equal(t, entries, got.Entries)
			} else {
				assert.Empty(t, got.Entries, "a refusal names nothing to remove")
			}
		})
	}
}

// reviewHome is a home folder whose Go build cache holds maxReviewItems+3 entries, e00 the
// largest, next to data no cleanup may touch.
func reviewHome(t *testing.T) (home, dir string) {
	t.Helper()
	home = t.TempDir()
	dir = filepath.Join(home, "Library", "Caches", "go-build")
	require.NoError(t, os.MkdirAll(filepath.Join(home, "Library", "Application Support", "Keep"), 0o700))
	require.NoError(t, os.MkdirAll(dir, 0o700))
	for i := range maxReviewItems + 3 {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("e%02d", i)), make([]byte, (maxReviewItems+3-i)*4096), 0o600))
	}
	return home, dir
}

func TestBuildReview(t *testing.T) {
	t.Parallel()

	home, dir := reviewHome(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bidi\u202e"), make([]byte, 1<<20), 0o600))
	full := storage.Measure(t.Context(), home, "go_build")[0]
	few := storage.Measure(t.Context(), home, "logs")[0]
	few.Entries, few.Sizes = []string{"/h/Library/Logs/small", "/h/Library/Logs/big"}, []int64{4096, 8192}
	tests := []struct {
		name string
		m    storage.Measured
		want CleanupReview
		// wantFirst and wantLast are the names of the first and the last item, when there are more than fit a literal.
		wantFirst, wantLast string
		wantItems           int
	}{
		{name: "nothing to remove", m: storage.Measure(t.Context(), home, "npm")[0], want: CleanupReview{Items: []CleanupItem{}}},
		{
			name: "a few entries, largest first", m: few,
			want: CleanupReview{Items: []CleanupItem{{Name: "big", Bytes: 8192}, {Name: "small", Bytes: 4096}}},
		},
		{
			name: "the largest fifty by name, a name made safe to show, the others summed", m: full,
			wantItems: maxReviewItems, wantFirst: "bidi", wantLast: "e48",
			want: CleanupReview{Rest: 4, RestBytes: (4 + 3 + 2 + 1) * 4096},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BuildReview(tt.m)

			assert.Equal(t, strconv.FormatInt(tt.m.At.UnixNano(), 10), got.ID, "the id is the time of the measurement the review shows")
			got.ID = ""
			if tt.wantItems > 0 {
				require.Len(t, got.Items, tt.wantItems)
				assert.Equal(t, tt.wantFirst, got.Items[0].Name)
				assert.Equal(t, tt.wantLast, got.Items[tt.wantItems-1].Name)
				got.Items = nil
			}
			assert.Equal(t, tt.want, *got)
		})
	}
}

//nolint:funlen // one table for the function under test; the length is the cases.
func TestCleanSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		names []string
		rest  bool
		// prepare changes the folder between the review and the removal.
		prepare func(t *testing.T, home, dir string)
		// want are the names selected, in the order of the fresh listing.
		want    []string
		wantErr string
	}{
		{name: "two rows of the review", names: []string{"e01", "e00"}, want: []string{"e00", "e01"}},
		{name: "the last row alone is every entry the review did not name", rest: true, want: []string{"e50", "e51", "e52"}},
		{name: "rows and the last row", names: []string{"e07"}, rest: true, want: []string{"e07", "e50", "e51", "e52"}},
		{name: "an empty selection", wantErr: "nothing selected"},
		{name: "a name that climbs out", names: []string{"../x"}, wantErr: "not an entry of the review"},
		{name: "a sibling folder reached through the parent", names: []string{"../../Application Support/Keep"}, wantErr: "not an entry"},
		{name: "an absolute path", names: []string{"/abs"}, wantErr: "not an entry of the review"},
		{
			name: "an absolute path of a real entry", names: []string{"<dir>/e00"}, wantErr: "not an entry of the review",
		},
		{name: "a path below an entry", names: []string{"a/b"}, wantErr: "not an entry of the review"},
		{name: "the category folder itself", names: []string{"."}, wantErr: "not an entry of the review"},
		{name: "the parent folder", names: []string{".."}, wantErr: "not an entry of the review"},
		{name: "an empty name", names: []string{""}, wantErr: "not an entry of the review"},
		{name: "a name the listing does not have", names: []string{"e00", "nope"}, wantErr: "not an entry of the review"},
		{name: "a name twice", names: []string{"e03", "e03"}, wantErr: "named twice"},
		{name: "an entry the review summed, named on its own", names: []string{"e51"}, wantErr: "not an entry of the review"},
		{name: "an entry the review summed, named next to the last row", names: []string{"e51"}, rest: true, wantErr: "not an entry"},
		{
			name: "an entry that appeared after the review is not taken by the last row", names: []string{"e02"}, rest: true,
			prepare: func(t *testing.T, _, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "new"), make([]byte, 1<<20), 0o600))
			},
			want: []string{"e02", "e50", "e51", "e52"},
		},
		{
			name: "an entry that appeared after the review, named", names: []string{"new"}, wantErr: "not an entry of the review",
			prepare: func(t *testing.T, _, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "new"), make([]byte, 1<<20), 0o600))
			},
		},
		{
			name: "an entry that went away after the review is left out", names: []string{"e04", "e05"},
			prepare: func(t *testing.T, _, dir string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(dir, "e04")))
			},
			want: []string{"e05"},
		},
		{
			name: "everything selected went away", names: []string{"e04"}, wantErr: "nothing selected is there",
			prepare: func(t *testing.T, _, dir string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(dir, "e04")))
			},
		},
		{
			name: "the category folder became a link to another folder", names: []string{"e00"}, wantErr: "nothing selected is there",
			prepare: func(t *testing.T, home, dir string) {
				t.Helper()
				require.NoError(t, os.RemoveAll(dir))
				require.NoError(t, os.Symlink(filepath.Join(home, "Library", "Application Support"), dir))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			home, dir := reviewHome(t)
			listed := storage.Measure(t.Context(), home, "go_build")[0]
			if tt.prepare != nil {
				tt.prepare(t, home, dir)
			}
			for i, name := range tt.names {
				tt.names[i] = strings.ReplaceAll(name, "<dir>", dir)
			}

			got, err := CleanSelection(listed, storage.Measure(t.Context(), home, "go_build")[0], tt.names, tt.rest)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, got.Entries, "a refusal names nothing to remove")
				return
			}
			require.NoError(t, err)
			want := make([]string, 0, len(tt.want))
			var bytes int64
			for i, name := range tt.want {
				want = append(want, filepath.Join(dir, name))
				bytes += got.Sizes[i]
			}
			assert.Equal(t, want, got.Entries)
			assert.Equal(t, bytes, got.Bytes)
			assert.Equal(t, "go_build", got.ID)
			assert.Equal(t, dir, got.Dir)
		})
	}
}

func TestScannedPath(t *testing.T) {
	t.Parallel()

	tree := &storage.Node{Name: "alex", Dir: true, Children: []*storage.Node{
		{Name: "src", Dir: true, Children: []*storage.Node{{Name: "blob"}}},
	}}
	tests := []struct {
		name string
		tree *storage.Node
		rel  string
		want string
	}{
		{name: "a node of the tree", tree: tree, rel: "src/blob", want: "/Users/alex/src/blob"},
		{name: "the root", tree: tree, rel: "", want: "/Users/alex"},
		{name: "a path that is not in the tree", tree: tree, rel: "Documents"},
		{name: "a path that climbs out names no node", tree: tree, rel: "src/../../other"},
		{name: "nothing scanned", rel: "src"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ScannedPath(tt.tree, "/Users/alex", tt.rel))
		})
	}
}

// What a cleanup leaves behind is measured again: the page must not show an emptied row
// while an entry is still on disk, nor a Trash without what just went into it.
func TestRemeasured(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	dir := filepath.Join(home, "Library", "Caches", "go-build")
	for _, name := range []string{"moves", "stays"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, name), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name, "o"), make([]byte, 8192), 0o600))
	}
	measured := storage.Measure(t.Context(), home)
	target, ok := CleanTarget(measured, Message{Category: "go_build", Review: BuildReview(measured[7]).ID}, false, time.Now())
	require.True(t, ok)
	// UF_IMMUTABLE of chflags(2): the folder can be neither renamed nor removed.
	require.NoError(t, syscall.Chflags(filepath.Join(dir, "stays"), 0x2))
	t.Cleanup(func() { _ = syscall.Chflags(filepath.Join(dir, "stays"), 0) })
	done, err := storage.Clean(t.Context(), home, target, false)
	require.NoError(t, err)
	require.Equal(t, 1, done.Failed)

	got := Remeasured(measured, storage.Measure(t.Context(), home, "go_build", "trash"))

	rows := BuildCleanup(got)
	require.Len(t, rows, len(measured))
	assert.Equal(t, CleanupCategory{ID: "go_build", Bytes: rows[7].Bytes, Items: 1}, rows[7])
	assert.GreaterOrEqual(t, rows[7].Bytes, int64(8192), "the entry left in place keeps its size on the row")
	assert.Equal(t, CleanupCategory{ID: "trash", Bytes: rows[10].Bytes, Items: 1, Permanent: true}, rows[10])
	assert.GreaterOrEqual(t, rows[10].Bytes, int64(8192), "the Trash row counts what the cleanup put there")
	assert.Equal(t, []string{filepath.Join(dir, "stays")}, got[7].Entries)
	assert.Len(t, measured[7].Entries, 2, "the earlier measurement is not written over")
}
