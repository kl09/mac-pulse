package ui

import (
	"cmp"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/storage"
)

const (
	// maxLevelEntries is how many rows of one folder reach the page; a treemap draws thirty.
	maxLevelEntries = 200
	// maxReviewItems is how many entries of a category a review lists by name; a Go build
	// cache has 256, a browser cache thousands, and the rest is one row.
	maxReviewItems = 50
	// A review names the sizes that were measured: past this age they are not what is on
	// disk any more, and the category is listed again first.
	measurementFresh = 5 * time.Minute
)

// StorageInput is what the session holds of the Storage tab's on-demand work.
type StorageInput struct {
	// Scan is the scan as the state carries it, except that Root is still the absolute path.
	Scan StorageScan
	// Measuring is true while a cleanup_scan runs.
	Measuring bool
	// Measured is nil until the first cleanup_scan has finished.
	Measured []storage.Measured
}

func BuildStorage(in StorageInput, home string) *Storage {
	out := &Storage{Scan: in.Scan, Cleanup: Cleanup{State: "idle"}}
	out.Scan.State = cmp.Or(out.Scan.State, "idle")
	out.Scan.Root = displayText(collector.TildePath(home, in.Scan.Root))
	measured := in.Measured
	switch {
	case in.Measuring:
		out.Cleanup.State = "running"
	case measured != nil:
		out.Cleanup.State = "done"
	}
	// Before the first measurement the page still lists every category, unsized.
	if measured == nil {
		for _, c := range storage.Categories(home) {
			measured = append(measured, storage.Measured{Category: c})
		}
	}
	out.Cleanup.Categories = BuildCleanup(measured)
	return out
}

func BuildCleanup(measured []storage.Measured) []CleanupCategory {
	out := make([]CleanupCategory, 0, len(measured))
	for _, m := range measured {
		out = append(out, CleanupCategory{
			ID: m.ID, Denied: m.Denied, Bytes: m.Bytes, Items: len(m.Entries), Permanent: m.Permanent,
		})
	}
	return out
}

// BuildLevel is the folder at rel of the tree scanned at the absolute path root; ok is
// false when the tree has no folder there. A child that is the folder of a cleanup
// category carries its id.
// The page sends a name back as it got it, cleaned, so a folder whose name loses
// a control character here cannot be opened or revealed; match on cleaned names if one turns up.
func BuildLevel(tree *storage.Node, root, rel string, categories []storage.Category) (level StorageLevel, ok bool) {
	node := tree.Find(rel)
	if node == nil || !node.Dir {
		return StorageLevel{}, false
	}
	level = StorageLevel{Path: rel, Bytes: node.Bytes, Files: node.Files, Children: []StorageEntry{}}
	children := slices.SortedStableFunc(slices.Values(node.Children), func(a, b *storage.Node) int { return cmp.Compare(b.Bytes, a.Bytes) })
	rest := StorageEntry{}
	for i, child := range children {
		if i >= maxLevelEntries {
			rest.Bytes += child.Bytes
			rest.Files += child.Files
			continue
		}
		entry := StorageEntry{Name: displayText(child.Name), Bytes: child.Bytes, Files: child.Files, Dir: child.Dir, Denied: child.Denied}
		dir := filepath.Join(root, filepath.FromSlash(path.Join(rel, child.Name)))
		if at := slices.IndexFunc(categories, func(c storage.Category) bool { return c.Dir == dir }); at >= 0 && child.Dir {
			entry.Category = categories[at].ID
		}
		level.Children = append(level.Children, entry)
	}
	if len(children) > maxLevelEntries {
		level.Children = append(level.Children, rest)
	}
	return level, true
}

// reviewOrder is the entries of m as a review shows them, largest first: indexes into
// m.Entries. BuildReview and CleanSelection must split the same listing the same way.
func reviewOrder(m storage.Measured) []int {
	order := make([]int, len(m.Entries))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(m.Sizes[b], m.Sizes[a]) })
	return order
}

// reviewID names one measurement of a category. A later one has a later time, so an id a
// page still holds stops matching once the category is listed or measured again.
func reviewID(m storage.Measured) string {
	return strconv.FormatInt(m.At.UnixNano(), 10)
}

func BuildReview(m storage.Measured) *CleanupReview {
	out := &CleanupReview{ID: reviewID(m), Items: []CleanupItem{}}
	for i, at := range reviewOrder(m) {
		if i < maxReviewItems {
			out.Items = append(out.Items, CleanupItem{Name: displayText(filepath.Base(m.Entries[at])), Bytes: m.Sizes[at]})
			continue
		}
		out.Rest++
		out.RestBytes += m.Sizes[at]
	}
	return out
}

// CleanSelection is what a cleanup message may remove: the entries of fresh, a listing of
// the category made just now, that the user left checked in the review of listed. names are
// rows of that review, and rest is its last row: every entry it did not list by name.
// Nothing the page sends is used as a path: a name only picks an entry the review showed,
// and an entry that has appeared since is never selected. A name the review did not show,
// one named twice and an empty selection refuse the whole message.
//
// Two entries whose names differ only in control characters are one name on the
// page, and only the larger can be picked; match on the raw name if one turns up.
func CleanSelection(listed, fresh storage.Measured, names []string, rest bool) (storage.Measured, error) {
	shown, picked := map[string]string{}, map[string]bool{}
	for i, at := range reviewOrder(listed) {
		name := displayText(filepath.Base(listed.Entries[at]))
		if _, taken := shown[name]; i < maxReviewItems && !taken {
			shown[name] = listed.Entries[at]
		} else if i >= maxReviewItems && rest {
			picked[listed.Entries[at]] = true
		}
	}
	for _, name := range names {
		entry, ok := shown[name]
		if !ok || picked[entry] {
			return storage.Measured{}, fmt.Errorf("select %s: %.300q is not an entry of the review, or is named twice", listed.ID, name)
		}
		picked[entry] = true
	}
	out := fresh
	out.Bytes, out.Entries, out.Sizes = 0, nil, nil
	for i, entry := range fresh.Entries {
		if picked[entry] {
			out.Bytes, out.Entries, out.Sizes = out.Bytes+fresh.Sizes[i], append(out.Entries, entry), append(out.Sizes, fresh.Sizes[i])
		}
	}
	if len(out.Entries) == 0 {
		return storage.Measured{}, errors.New("select " + listed.ID + ": nothing selected is there")
	}
	return out, nil
}

// CleanTarget is the listing a cleanup message may remove from: the one its review showed.
// ok is false when the category was not measured, has nothing to remove, was measured too
// long before now, or was listed or measured again since that review: the rows the user left
// checked would then mean other entries. It is false too for a message that asks to delete
// for good what can go to the Trash, unless noTrash says the category's last cleanup found
// no Trash: that the Trash comes first is Go's rule, not only the page's.
func CleanTarget(measured []storage.Measured, msg Message, noTrash bool, now time.Time) (target storage.Measured, ok bool) {
	i := slices.IndexFunc(measured, func(m storage.Measured) bool { return m.ID == msg.Category })
	if i < 0 || len(measured[i].Entries) == 0 || now.Sub(measured[i].At) > measurementFresh || msg.Review != reviewID(measured[i]) {
		return storage.Measured{}, false
	}
	if msg.Permanent && !measured[i].Permanent && !noTrash {
		return storage.Measured{}, false
	}
	return measured[i], true
}

// Remeasured is measured with the categories of again in place of their earlier measurement.
// It returns a copy: a state being built elsewhere still reads measured.
func Remeasured(measured, again []storage.Measured) []storage.Measured {
	out := slices.Clone(measured)
	for _, m := range again {
		if i := slices.IndexFunc(out, func(old storage.Measured) bool { return old.ID == m.ID }); i >= 0 {
			out[i] = m
		}
	}
	return out
}

// ScannedPath is the absolute path of the node a reveal message's path picks in the tree
// scanned at root; "" when nothing is scanned or the tree has no such node.
func ScannedPath(tree *storage.Node, root, rel string) string {
	if tree == nil || tree.Find(rel) == nil {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}
