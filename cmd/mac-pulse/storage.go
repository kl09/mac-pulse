package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/shell"
	"github.com/kl09/mac-pulse/internal/storage"
	"github.com/kl09/mac-pulse/internal/ui"
)

// storageCommand runs a message of the Storage tab. Nothing here starts by itself: a scan,
// a measurement and a cleanup each take a button press, run off the message goroutine and
// keep their result in memory only.
func (s *session) storageCommand(ctx context.Context, msg ui.Message) {
	// Every path of this tab is built from the home folder.
	if s.home == "" {
		s.fail(msg.Type, errors.New("no home directory"))
		return
	}
	switch msg.Type {
	case "storage_scan":
		s.scan(ctx, msg.Root == "choose")
	case "storage_cancel":
		if s.cancelScan != nil {
			s.cancelScan()
		}
	case "storage_clear":
		// While a scan runs the page offers Cancel alone.
		if !s.scanBusy.Load() {
			s.latest.update(func(c *current) { c.storage.Scan, c.tree = ui.StorageScan{}, nil })
			s.refresh()
		}
	case "storage_open":
		s.openLevel(msg.Path)
	case "cleanup_scan", "cleanup_list", "cleanup":
		run := s.measure
		switch msg.Type {
		case "cleanup_list":
			run = func(ctx context.Context) (ui.Action, error) { return s.list(ctx, msg.Category) }
		case "cleanup":
			run = func(ctx context.Context) (ui.Action, error) { return s.clean(ctx, msg) }
		}
		// The page waits for the answer of each of the three: a dropped one says so.
		if !s.lookup(ctx, msg.Type, &s.cleanBusy, run) {
			s.answer(msg.Type, false, "err.busy", msg.Category)
		}
	}
}

// measure sizes every category.
func (s *session) measure(ctx context.Context) (ui.Action, error) {
	s.latest.update(func(c *current) { c.storage.Measuring = true })
	s.refresh()
	measured := storage.Measure(ctx, s.home)
	s.latest.update(func(c *current) { c.storage.Measuring, c.storage.Measured = false, measured })
	s.refresh()
	return ui.Action{}, nil
}

// list answers what a cleanup of one category would remove. The review shows what is there
// now, and the row above it the same size, so the category is measured again; that listing
// is the one a cleanup must name.
func (s *session) list(ctx context.Context, category string) (ui.Action, error) {
	if s.latest.get().storage.Measured == nil {
		return ui.Action{Key: "err.stale", Text: category}, errors.New("nothing measured yet")
	}
	again := storage.Measure(ctx, s.home, category)
	s.latest.update(func(c *current) { c.storage.Measured = ui.Remeasured(c.storage.Measured, again) })
	s.refresh()
	return ui.Action{Text: category, Review: ui.BuildReview(again[0])}, nil
}

// scan sizes the home folder, or a folder the user picks in the shell's dialog: the page
// never names a path. The tree replaces the previous one and lives until Clear or exit.
func (s *session) scan(ctx context.Context, choose bool) {
	// Checked before cancelScan is replaced: Cancel must keep stopping the scan that runs.
	if s.scanBusy.Load() {
		return
	}
	// The lookup below calls cancel when the scan ends.
	scanCtx, cancel := context.WithCancel(ctx)
	s.cancelScan = cancel
	s.lookup(ctx, "storage_scan", &s.scanBusy, func(context.Context) (ui.Action, error) {
		defer cancel()
		root := s.home
		if choose {
			root = shell.ChooseFolder()
		}
		if root == "" {
			return ui.Action{}, nil
		}
		show := func(scan ui.StorageScan, tree *storage.Node) {
			s.latest.update(func(c *current) { c.storage.Scan, c.tree = scan, tree })
			s.refresh()
		}
		show(ui.StorageScan{State: "running", Root: root}, nil)
		var last storage.Progress
		tree, err := storage.Scan(scanCtx, root, func(p storage.Progress) {
			last = p
			// The sampler's next tick carries it to the page.
			s.latest.update(func(c *current) {
				c.storage.Scan.Files, c.storage.Scan.Bytes, c.storage.Scan.Denied = p.Files, p.Bytes, p.Denied
			})
		})
		switch {
		case errors.Is(err, context.Canceled):
			show(ui.StorageScan{State: "cancelled", Root: root}, nil)
			return ui.Action{}, nil
		case err != nil:
			show(ui.StorageScan{State: "failed", Root: root}, nil)
			return ui.Action{}, err
		}
		show(ui.StorageScan{State: "done", Root: root, Files: tree.Files, Bytes: tree.Bytes, Denied: last.Denied}, tree)
		return ui.Action{}, nil
	})
}

// openLevel pushes one folder of the scanned tree to the page.
func (s *session) openLevel(rel string) {
	cur := s.latest.get()
	var level ui.StorageLevel
	ok := cur.tree != nil
	if ok {
		level, ok = ui.BuildLevel(cur.tree, cur.storage.Scan.Root, rel, storage.Categories(s.home))
	}
	if !ok {
		slog.Warn("storage_open refused: not a folder of the scanned tree", "path", rel)
		return
	}
	slog.Debug("storage level", "path", rel, "bytes", level.Bytes, "children", len(level.Children))
	// Strings, bools and integers always marshal.
	b, _ := json.Marshal(level)
	shell.Push("onStorage", b)
}

// clean removes what the user left checked in the review of one category: entries of the
// listing that review showed which a listing made now still has.
func (s *session) clean(ctx context.Context, msg ui.Message) (ui.Action, error) {
	stale := ui.Action{Key: "err.stale", Text: msg.Category}
	// One use: whatever this cleanup does, the next one starts with the Trash again.
	noTrash := s.noTrash[msg.Category]
	delete(s.noTrash, msg.Category)
	listed, ok := ui.CleanTarget(s.latest.get().storage.Measured, msg, noTrash, time.Now())
	if !ok {
		return stale, errors.New("not the review of the stored listing, or for good without a Trash that failed")
	}
	target, err := ui.CleanSelection(listed, storage.Measure(ctx, s.home, msg.Category)[0], msg.Items, msg.Rest)
	if err != nil {
		return stale, err
	}
	done, err := storage.Clean(ctx, s.home, target, msg.Permanent)
	switch {
	case errors.Is(err, storage.ErrNoTrash):
		s.noTrash[msg.Category] = true
		return ui.Action{Key: "err.no_trash", Text: msg.Category}, err
	// The folder is not what was measured any more (gone, or replaced by a link): nothing
	// was touched, and a new measurement says what is there now.
	case err != nil:
		return stale, err
	}
	// What would not move is still on its row, and the Trash row has what just went there.
	again := storage.Measure(ctx, s.home, msg.Category, "trash")
	s.latest.update(func(c *current) { c.storage.Measured = ui.Remeasured(c.storage.Measured, again) })
	s.refresh()
	res := ui.Action{
		Key: "act.cleaned", Text: msg.Category,
		Values: map[string]float64{"bytes": float64(done.Bytes), "items": float64(done.Items), "failed": float64(done.Failed)},
	}
	if done.Wrapper != "" {
		res.List = []string{filepath.Base(done.Wrapper), collector.TildePath(s.home, target.Dir)}
	}
	return res, nil
}

// printLevel is the -storage flag: it scans dir and prints its top level the way the page gets it.
func printLevel(ctx context.Context, dir string) error {
	root, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", dir, err)
	}
	tree, err := storage.Scan(ctx, root, nil)
	if err != nil {
		return err
	}
	level, _ := ui.BuildLevel(tree, root, "", storage.Categories(homeDir()))
	return printJSON(level)
}

// printCaches is the -caches flag: the cleanup categories of this home with their sizes. It only reads.
func printCaches(ctx context.Context) error {
	home := homeDir()
	if home == "" {
		return errors.New("measure caches: no home directory")
	}
	return printJSON(ui.BuildCleanup(storage.Measure(ctx, home)))
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}
