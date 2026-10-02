package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/sys/unix"
)

const (
	// wrapperRandom is how many random letters end a wrapper folder's name: 40 bits.
	wrapperRandom = 8
	// searchOnly is O_SEARCH of <sys/fcntl.h>, which x/sys does not name: a folder handle for
	// the calls that work relative to it.
	searchOnly = 0x40000000 | unix.O_DIRECTORY
	// noteName is the file in a wrapper folder that says where its entries came from.
	noteName = "mac-pulse-note.txt"
)

// ErrNoTrash means the wrapper folder in ~/.Trash could not be made, or ~/.Trash is not a
// folder of its own; nothing was touched.
var ErrNoTrash = errors.New("trash is not writable")

// Category is one well-known place whose contents can be removed at the price of a
// re-download or a rebuild. Only the entries inside Dir are ever removed, never Dir itself.
type Category struct {
	ID string
	// Dir is an absolute path under the home folder.
	Dir string
	// Skip are path.Match patterns of entry names left alone: Apple's own caches and the
	// folders that are categories themselves.
	Skip []string
	// Permanent entries cannot go to the Trash: the category is the Trash.
	Permanent bool
}

// Measured is a category with what a cleanup of it would remove right now.
type Measured struct {
	Category
	// Present is false when Dir does not exist, Denied true when macOS did not let it be read.
	Present, Denied bool
	Bytes           int64
	// Entries are the absolute paths that Clean removes; an entry that cannot be read, or that
	// holds a mounted volume, is not listed.
	Entries []string
	// Sizes holds the space each of Entries takes, in the same order.
	Sizes []int64
	// At is when the category was sized.
	At time.Time
}

// Cleaned is what a cleanup did. Failed counts the entries left in place.
type Cleaned struct {
	Bytes  int64
	Items  int
	Failed int
	// Wrapper is the folder in the Trash that took the entries, "" when none went there.
	Wrapper string
}

// Categories lists the places a cleanup may touch, all under home, in display order.
// Only the standard places; a custom GOCACHE, npm cache or HOMEBREW_CACHE is not looked up.
func Categories(home string) []Category {
	lib := filepath.Join(home, "Library")
	caches := filepath.Join(lib, "Caches")
	return []Category{
		{ID: "app_caches", Dir: caches, Skip: []string{
			"com.apple.*", "CloudKit", "GeoServices", "FamilyCircle", "PassKit", "mac-pulse*", "Yarn", "go-build", "pip", "Homebrew",
		}},
		{ID: "logs", Dir: filepath.Join(lib, "Logs")},
		{ID: "xcode_derived", Dir: filepath.Join(lib, "Developer", "Xcode", "DerivedData")},
		{ID: "simulator_caches", Dir: filepath.Join(lib, "Developer", "CoreSimulator", "Caches")},
		{ID: "npm", Dir: filepath.Join(home, ".npm", "_cacache")},
		{ID: "yarn", Dir: filepath.Join(caches, "Yarn")},
		{ID: "pnpm", Dir: filepath.Join(lib, "pnpm", "store")},
		{ID: "go_build", Dir: filepath.Join(caches, "go-build")},
		{ID: "pip", Dir: filepath.Join(caches, "pip")},
		{ID: "homebrew", Dir: filepath.Join(caches, "Homebrew")},
		{ID: "trash", Dir: filepath.Join(home, ".Trash"), Permanent: true},
	}
}

// Measure sizes the categories of home named by ids, every one when none is named, in the
// order of Categories; it only reads. A category folder that is a symlink, or is reached
// through one, counts as absent: Clean would refuse it. A cancelled ctx leaves the
// categories not reached yet as absent.
func Measure(ctx context.Context, home string, ids ...string) []Measured {
	defer background()()
	categories := Categories(home)
	out := make([]Measured, 0, len(categories))
	for _, c := range categories {
		if len(ids) > 0 && !slices.Contains(ids, c.ID) {
			continue
		}
		m := Measured{Category: c, Entries: []string{}, At: time.Now()}
		info, err := os.Lstat(c.Dir)
		if err == nil && info.IsDir() && ctx.Err() == nil && inPlace(home, c.Dir) {
			m = measure(ctx, m, device(info))
		}
		out = append(out, m)
	}
	return out
}

// measure lists and sizes the entries of the category folder of m, which is on the volume dev.
func measure(ctx context.Context, m Measured, dev int32) Measured {
	m.Present = true
	entries, err := os.ReadDir(m.Dir)
	m.Denied = errors.Is(err, fs.ErrPermission)
	for _, e := range entries {
		skipped := slices.ContainsFunc(m.Skip, func(pattern string) bool {
			matched, _ := path.Match(pattern, e.Name())
			return matched
		})
		info, err := e.Info()
		w := walker{dev: dev, readDir: os.ReadDir}
		// A mount point inside a cache folder is somebody's volume, not a cache.
		if skipped || err != nil || w.foreign(info) {
			continue
		}
		node, err := w.node(ctx, filepath.Join(m.Dir, e.Name()), info)
		if err != nil {
			break
		}
		if !node.Denied && !w.crossed {
			m.Bytes, m.Entries, m.Sizes = m.Bytes+node.Bytes, append(m.Entries, filepath.Join(m.Dir, e.Name())), append(m.Sizes, node.Bytes)
		}
	}
	return m
}

// Clean removes the entries of m: into one wrapper folder
// ~/.Trash/mac-pulse-<id>-<YYYYMMDD-HHMMSS>-<random>, next to a note that names the folder
// they came from, or for good when permanent. It refuses a category folder that is a symlink,
// is reached through one or lies outside home, an entry that is not directly inside it, and a
// Permanent category without permanent. The folder is opened once and every entry is sized,
// moved and deleted through that handle: a folder swapped for a link while the cleanup runs
// leads it nowhere else. ErrNoTrash means nothing was touched; an entry that would not move,
// or that holds a mounted volume, stays and counts in Failed, as do the entries a cancelled
// ctx did not reach.
func Clean(ctx context.Context, home string, m Measured, permanent bool) (Cleaned, error) {
	info, err := refuse(home, m, permanent)
	if err != nil {
		return Cleaned{}, fmt.Errorf("clean %s: %w", m.ID, err)
	}
	rel, _ := filepath.Rel(home, m.Dir)
	dir, err := openDir(home, rel)
	if err != nil {
		return Cleaned{}, fmt.Errorf("clean %s: %w", m.ID, err)
	}
	defer func() { _ = dir.Close() }()
	root, err := os.OpenRoot(m.Dir)
	if err != nil {
		return Cleaned{}, fmt.Errorf("clean %s: %w", m.ID, err)
	}
	defer func() { _ = root.Close() }()
	held, heldErr := dir.Stat()
	rooted, rootErr := root.Stat(".")
	if heldErr != nil || rootErr != nil || !os.SameFile(info, held) || !os.SameFile(info, rooted) {
		return Cleaned{}, fmt.Errorf("clean %s: %s was replaced while it was checked", m.ID, m.Dir)
	}
	if permanent {
		return remove(ctx, m, root, device(info), nil), nil
	}
	wrapper, err := makeWrapper(home, m)
	if err != nil {
		return Cleaned{}, err
	}
	defer func() { _ = wrapper.Close() }()
	from, to := int(dir.Fd()), int(wrapper.Fd())
	done := remove(ctx, m, root, device(info), func(name string) error { return unix.Renameat(from, name, to, name) })
	done.Wrapper = wrapper.Name()
	if done.Items == 0 {
		_ = unix.Unlinkat(to, noteName, 0)
		_ = os.Remove(wrapper.Name())
		done.Wrapper = ""
	}
	return done, nil
}

// openDir opens the folder rel of home for the calls that take a folder handle. No part of
// the path below home may be a symlink. The handle cannot list the folder, so macOS hands
// one out for the Trash too, which it lets a program fill but not read.
func openDir(home, rel string) (*os.File, error) {
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(resolved, rel)
	fd, err := unix.Open(path, searchOnly|unix.O_NOFOLLOW_ANY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

// remove takes the entries of m out of root, their folder on the volume dev: move puts one
// elsewhere by its name, and nil deletes it for good.
func remove(ctx context.Context, m Measured, root *os.Root, dev int32, move func(name string) error) Cleaned {
	var done Cleaned
	for _, entry := range m.Entries {
		name := filepath.Base(entry)
		info, err := root.Lstat(name)
		if err != nil || ctx.Err() != nil {
			done.Failed++
			continue
		}
		w := walker{dev: dev, readDir: func(dir string) ([]fs.DirEntry, error) { return fs.ReadDir(root.FS(), dir) }}
		var node *Node
		if !w.foreign(info) {
			node, err = w.node(ctx, name, info)
		}
		switch {
		case err != nil:
		// A volume mounted in the entry would go to the Trash with it, unmeasured.
		case w.crossed:
			err = errors.New("holds a mounted volume")
		case move == nil:
			err = root.RemoveAll(name)
		default:
			err = move(name)
		}
		if err != nil {
			done.Failed++
			continue
		}
		done.Bytes, done.Items = done.Bytes+node.Bytes, done.Items+1
	}
	return done
}

// refuse is the gate in front of every removal: nil lets the entries of m go, and info is
// then what their folder was when it passed.
func refuse(home string, m Measured, permanent bool) (info fs.FileInfo, err error) {
	info, err = os.Lstat(m.Dir)
	if err != nil {
		return nil, err
	}
	switch {
	case !info.IsDir() || !inPlace(home, m.Dir):
		return nil, fmt.Errorf("%s is not a folder inside the home folder", m.Dir)
	case slices.ContainsFunc(m.Entries, func(entry string) bool { return filepath.Dir(entry) != m.Dir }):
		return nil, fmt.Errorf("an entry is not inside %s", m.Dir)
	case m.Permanent && !permanent:
		return nil, errors.New("its entries cannot go to the Trash")
	}
	return info, nil
}

// inPlace is true when dir lies under home and no part of its path below home is a symlink:
// a linked folder would lead the removal wherever the link points, another folder of the
// home included.
func inPlace(home, dir string) bool {
	resolved, dirErr := filepath.EvalSymlinks(dir)
	root, rootErr := filepath.EvalSymlinks(home)
	rel, relErr := filepath.Rel(home, dir)
	return dirErr == nil && rootErr == nil && relErr == nil && filepath.IsLocal(rel) && resolved == filepath.Join(root, rel)
}

// makeWrapper makes the one folder a cleanup of m puts into the Trash, so that the Trash
// shows one item instead of the 256 folders of a Go build cache, and writes the note that
// says where to drag its contents back. The name ends in random letters: nobody can take it
// beforehand to make the Trash look broken. A ~/.Trash that is a link or a file is no Trash.
func makeWrapper(home string, m Measured) (*os.File, error) {
	// A home that never used the Trash has no folder for it yet.
	if err := os.Mkdir(filepath.Join(home, ".Trash"), 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("%w: %w", ErrNoTrash, err)
	}
	trash, err := openDir(home, ".Trash")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoTrash, err)
	}
	defer func() { _ = trash.Close() }()
	name := "mac-pulse-" + m.ID + "-" + time.Now().Format("20060102-150405") + "-" + rand.Text()[:wrapperRandom]
	if err := unix.Mkdirat(int(trash.Fd()), name, 0o700); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoTrash, err)
	}
	fd, err := unix.Openat(int(trash.Fd()), name, searchOnly|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoTrash, err)
	}
	wrapper := os.NewFile(uintptr(fd), filepath.Join(home, ".Trash", name))
	// The cleanup does not depend on the note: the folder name already says what this is.
	if note, err := unix.Openat(fd, noteName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600); err == nil {
		text := "mac-pulse moved these items here from " + m.Dir + "\nTo undo the cleanup, drag them back into that folder.\n"
		_, _ = unix.Write(note, []byte(text))
		_ = unix.Close(note)
	}
	return wrapper, nil
}
