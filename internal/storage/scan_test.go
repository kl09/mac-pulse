package storage

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:funlen // one table for the function under test; the length is the cases.
func TestScan(t *testing.T) {
	t.Parallel()

	// 3 MiB and 2 MiB in two levels, a small folder, a folder that cannot be read, one hidden
	// two small folders deep, and a symlink to a large tree.
	root := filepath.Join(t.TempDir(), "tree")
	for file, size := range map[string]int{"a/big": 3 << 20, "a/b/mid": 2 << 20, "a/b/tiny": 100, "small/note": 10_000} {
		path := filepath.Join(root, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600))
	}
	for _, dir := range []string{"closed", "deep/down/closed"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o700))
		require.NoError(t, os.Chmod(filepath.Join(root, dir), 0))
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, dir), 0o700) })
	}
	require.NoError(t, os.Symlink("/Applications", filepath.Join(root, "link")))
	linked := filepath.Join(filepath.Dir(root), "linked")
	require.NoError(t, os.Symlink(root, linked))
	fileLink := filepath.Join(filepath.Dir(root), "file-link")
	require.NoError(t, os.Symlink(filepath.Join(root, "a/big"), fileLink))
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		root    string
		wantErr error
		// wantErrText is checked when wantErr is nil.
		wantErrText string
	}{
		{name: "a tree", ctx: t.Context(), root: root},
		{name: "a cancelled scan returns no tree", ctx: cancelled, root: root, wantErr: context.Canceled},
		{name: "a file is not scanned", ctx: t.Context(), root: filepath.Join(root, "a/big"), wantErrText: "not a folder"},
		{name: "a root that is a symlink to a folder is that folder; the links inside it stay unfollowed", ctx: t.Context(), root: linked},
		{name: "a symlink to a file is not scanned", ctx: t.Context(), root: fileLink, wantErrText: "not a folder"},
		{name: "a folder that does not exist", ctx: t.Context(), root: filepath.Join(root, "nowhere"), wantErr: fs.ErrNotExist},
		{name: "a folder that cannot be read", ctx: t.Context(), root: filepath.Join(root, "closed"), wantErr: fs.ErrPermission},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var reported []Progress

			tree, err := Scan(tt.ctx, tt.root, func(p Progress) { reported = append(reported, p) })

			if tt.wantErr != nil || tt.wantErrText != "" {
				require.Error(t, err)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				}
				require.ErrorContains(t, err, tt.wantErrText)
				assert.Nil(t, tree)
				assert.Empty(t, reported)
				return
			}
			require.NoError(t, err)
			// Folders take blocks of their own: the files are the floor.
			assert.GreaterOrEqual(t, tree.Bytes, int64(5<<20+10_100))
			assert.Less(t, tree.Bytes, int64(5<<20+200<<10), "neither /Applications behind the symlink nor anything else is counted")
			assert.Equal(t, 5, tree.Files, "big, mid, tiny, note and the symlink itself")
			assert.Equal(t, []Progress{{Files: tree.Files, Bytes: tree.Bytes, Denied: 2}}, reported, "one last report with the totals")

			// Only what is 1 MiB and more or denied has a node; a small folder stays when a denied one is below it.
			shape := func(n *Node) []string {
				var names []string
				for _, c := range n.Children {
					names = append(names, c.Name)
				}
				return names
			}
			assert.Equal(t, "tree", tree.Name)
			assert.Equal(t, []string{"a", "closed", "deep"}, shape(tree))
			a, b := tree.Find("a"), tree.Find("a/b")
			require.NotNil(t, b)
			assert.Equal(t, []string{"b", "big"}, shape(a))
			assert.Equal(t, []string{"mid"}, shape(b))
			assert.Equal(t, 3, a.Files)
			assert.True(t, a.Dir)
			assert.GreaterOrEqual(t, a.Bytes, int64(5<<20))
			assert.Equal(t, &Node{Name: "big", Bytes: 3 << 20, Files: 1}, tree.Find("a/big"))
			assert.Equal(t, &Node{Name: "mid", Bytes: 2 << 20, Files: 1}, tree.Find("a/b/mid"))
			assert.True(t, tree.Find("closed").Denied)
			assert.True(t, tree.Find("deep/down/closed").Denied)
			assert.False(t, tree.Find("deep").Denied)
			assert.Nil(t, tree.Find("small"))
			assert.Nil(t, tree.Find("link"))
		})
	}

	t.Run("no progress callback", func(t *testing.T) {
		t.Parallel()

		tree, err := Scan(t.Context(), root, nil)

		require.NoError(t, err)
		assert.Equal(t, 5, tree.Files)
	})

	t.Run("a folder on another volume is a mount point: not entered and not counted", func(t *testing.T) {
		t.Parallel()

		info, err := os.Lstat(root)
		require.NoError(t, err)
		// Every folder below the root is then on a volume other than the walk's.
		w := walker{dev: device(info) + 1, readDir: os.ReadDir}

		tree, err := w.node(t.Context(), root, info)

		require.NoError(t, err)
		assert.True(t, w.crossed)
		assert.Empty(t, tree.Children)
		assert.Equal(t, 1, tree.Files, "the symlink at the top; no file of a subfolder")
		assert.Less(t, tree.Bytes, int64(64<<10))
		assert.Equal(t, Progress{Files: 1, Bytes: tree.Bytes}, w.progress)
	})
}

func TestNode_Find(t *testing.T) {
	t.Parallel()

	blob := &Node{Name: "blob"}
	src := &Node{Name: "src", Dir: true, Children: []*Node{blob}}
	root := &Node{Name: "home", Dir: true, Children: []*Node{{Name: "Movies", Dir: true}, src}}
	tests := []struct {
		name string
		rel  string
		want *Node
	}{
		{name: "the root itself", rel: "", want: root},
		{name: "a child", rel: "src", want: src},
		{name: "a grandchild", rel: "src/blob", want: blob},
		{name: "no such child", rel: "Music"},
		{name: "below a file", rel: "src/blob/x"},
		{name: "a parent reference names no node", rel: "src/.."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Same(t, tt.want, root.Find(tt.rel))
		})
	}
}
