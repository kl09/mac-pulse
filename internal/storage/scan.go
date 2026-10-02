// Package storage answers what takes the space in a folder and what of it is safe to
// remove. Nothing here runs on its own: every call is one the user asked for by a button.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const (
	// minNodeBytes keeps the tree of a home folder with a million files at a few thousand nodes.
	minNodeBytes = 1 << 20
	// setpriority(2) on macOS: the calling thread, and the priority that also throttles its disk I/O.
	prioDarwinThread = 3
	prioDarwinBG     = 0x1000
)

// Node is a folder or a file of a scanned tree.
type Node struct {
	Name string
	// Bytes is the space on disk of the node and everything below it, Files the count of
	// files there; both include what has no node of its own.
	Bytes int64
	Files int
	Dir   bool
	// Denied marks a folder macOS did not let the scan read.
	Denied bool
	// Children holds only the folders and files of 1 MiB and more and the denied folders.
	Children []*Node
}

// Progress is how far a running scan has come.
type Progress struct {
	Files  int
	Bytes  int64
	Denied int
}

// walker sizes entries of one volume; one goroutine uses it.
type walker struct {
	// dev is the volume the walk started on: a folder on another one is a mount point.
	dev int32
	// crossed is set once the walk has met such a folder.
	crossed bool
	// readDir lists a folder by the path node was given: os.ReadDir, or a listing confined to
	// one opened folder.
	readDir    func(path string) ([]fs.DirEntry, error)
	progress   Progress
	reported   time.Time
	onProgress func(Progress)
}

// Scan sizes the tree under root, which must be a folder or a symlink to one, without
// following the symlinks inside it or entering another volume. It locks its goroutine to a thread of background priority.
// onProgress is called on that goroutine, at most once a second, and once more with the
// totals before a tree is returned. A cancelled ctx returns its error and no tree.
//
// A hard link and an APFS clone count once per name, so the sum may exceed the
// space taken on disk; count inodes if the difference ever misleads.
func Scan(ctx context.Context, root string, onProgress func(Progress)) (*Node, error) {
	// Only the folder the user chose may be a link.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan %s: not a folder", root)
	}
	defer background()()
	w := walker{dev: device(info), readDir: os.ReadDir, reported: time.Now(), onProgress: onProgress}
	tree, err := w.node(ctx, resolved, info)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	if tree.Denied {
		return nil, fmt.Errorf("scan %s: %w", root, fs.ErrPermission)
	}
	if onProgress != nil {
		onProgress(w.progress)
	}
	return tree, nil
}

// Find returns the node at rel, a slash-separated path below n; "" is n itself and nil
// means the tree has no such node.
func (n *Node) Find(rel string) *Node {
	if rel == "" {
		return n
	}
	name, rest, _ := strings.Cut(rel, "/")
	for _, child := range n.Children {
		if child.Name == name {
			return child.Find(rest)
		}
	}
	return nil
}

// node sizes the entry at path, info being its lstat: a symlink is a file of its own size
// and is never followed. The only error is the cancelled ctx's.
func (w *walker) node(ctx context.Context, path string, info fs.FileInfo) (*Node, error) {
	n := &Node{Name: info.Name(), Bytes: info.Sys().(*syscall.Stat_t).Blocks * 512, Dir: info.IsDir()}
	w.progress.Bytes += n.Bytes
	if !n.Dir {
		n.Files = 1
		w.progress.Files++
		return n, nil
	}
	entries, err := w.readDir(path)
	if errors.Is(err, fs.ErrPermission) {
		n.Denied = true
		w.progress.Denied++
	}
	// Any other error is a folder that went away under the scan: it counts as empty.
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := e.Info()
		if err != nil || w.foreign(info) {
			continue
		}
		child, err := w.node(ctx, filepath.Join(path, e.Name()), info)
		if err != nil {
			return nil, err
		}
		n.Bytes, n.Files = n.Bytes+child.Bytes, n.Files+child.Files
		// A folder with children is one with a denied folder somewhere below: it must stay reachable.
		if child.Bytes >= minNodeBytes || child.Denied || len(child.Children) > 0 {
			n.Children = append(n.Children, child)
		}
	}
	if w.onProgress != nil && time.Since(w.reported) >= time.Second {
		w.reported = time.Now()
		w.onProgress(w.progress)
	}
	return n, nil
}

// foreign is true for a folder on another volume than the walk's, and remembers that it met one.
func (w *walker) foreign(info fs.FileInfo) bool {
	if info.IsDir() && device(info) != w.dev {
		w.crossed = true
		return true
	}
	return false
}

func device(info fs.FileInfo) int32 {
	return info.Sys().(*syscall.Stat_t).Dev
}

// background moves the calling goroutine to a thread of background priority until the
// returned function is called: a walk over a home folder must not compete with what the
// user is doing.
func background() (restore func()) {
	runtime.LockOSThread()
	lowered := syscall.Setpriority(prioDarwinThread, 0, prioDarwinBG) == nil
	return func() {
		// A thread still at background priority must not go back to the scheduler: left
		// locked, it ends with this goroutine.
		if !lowered || syscall.Setpriority(prioDarwinThread, 0, 0) == nil {
			runtime.UnlockOSThread()
		}
	}
}
