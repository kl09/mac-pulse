package collector

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

const (
	rootDisk = "disk0"
	// An eject first flushes what is still unwritten, which a slow external disk takes seconds for.
	ejectTimeout = 30 * time.Second

	SMARTVerified = "verified"
	SMARTFailing  = "failing"
)

var (
	plistSMART = regexp.MustCompile(`<key>SMARTStatus</key>\s*<string>([^<]*)</string>`)
	plistModel = regexp.MustCompile(`<key>MediaName</key>\s*<string>([^<]*)</string>`)
)

func readDisk(ctx context.Context) (Disk, error) {
	usage, err := disk.UsageWithContext(ctx, "/")
	if err != nil {
		return Disk{}, fmt.Errorf("disk usage: %w", err)
	}
	counters, err := disk.IOCountersWithContext(ctx, rootDisk)
	if err != nil {
		return Disk{}, fmt.Errorf("disk io counters: %w", err)
	}
	return Disk{
		Total:      usage.Total,
		Free:       usage.Free,
		ReadBytes:  counters[rootDisk].ReadBytes,
		WriteBytes: counters[rootDisk].WriteBytes,
	}, nil
}

// readVolumes skips a volume whose usage cannot be read: one stale mount must not hide the rest.
func readVolumes(ctx context.Context) ([]Volume, error) {
	parts, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("disk partitions: %w", err)
	}
	volumes := make([]Volume, 0, len(parts))
	for _, p := range parts {
		// Finder shows the boot volume and what is under /Volumes; /System/Volumes/*, devfs
		// and autofs maps are plumbing.
		if p.Mountpoint != "/" && !ejectable(p.Mountpoint) {
			continue
		}
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue
		}
		v := Volume{
			Name: filepath.Base(p.Mountpoint), Mount: p.Mountpoint, FS: p.Fstype, Total: usage.Total, Free: usage.Free,
			Ejectable: ejectable(p.Mountpoint),
		}
		if p.Mountpoint == "/" {
			v.Name = rootVolumeName()
		}
		volumes = append(volumes, v)
	}
	slices.SortFunc(volumes, func(x, y Volume) int {
		if (x.Mount == "/") != (y.Mount == "/") {
			if x.Mount == "/" {
				return -1
			}
			return 1
		}
		return cmp.Compare(x.Name, y.Name)
	})
	return volumes, nil
}

// Eject unmounts the volume at mount, which must be one of the last sample's Ejectable volumes.
// The error carries diskutil's own words for the user.
func Eject(ctx context.Context, mount string) error {
	// mount comes from the web view; diskutil would take "/Volumes/../" for the boot volume.
	if !ejectable(mount) || filepath.Clean(mount) != mount {
		return fmt.Errorf("eject %q: not a volume under /Volumes", mount)
	}
	ctx, cancel := context.WithTimeout(ctx, ejectTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "diskutil", "eject", mount).CombinedOutput()
	if err == nil {
		return nil
	}
	if text := strings.TrimSpace(string(out)); text != "" {
		return errors.New(text)
	}
	return fmt.Errorf("diskutil eject: %w", err)
}

// ejectable is the whole rule: the boot volume is the one mount outside /Volumes that is shown.
// A network share and an internal partition under /Volumes get the button too;
// ask `diskutil info` for Ejectable if that gets in the way.
func ejectable(mount string) bool {
	return strings.HasPrefix(mount, "/Volumes/")
}

// rootVolumeName finds the name Finder shows for "/": the symlink in /Volumes that points at it.
func rootVolumeName() string {
	entries, err := os.ReadDir("/Volumes")
	if err != nil {
		return "/"
	}
	for _, e := range entries {
		if target, err := os.Readlink(filepath.Join("/Volumes", e.Name())); err == nil && target == "/" {
			return e.Name()
		}
	}
	return "/"
}

// readSMART costs ~40 ms, so the sampler calls it only while the disk screen is open.
func readSMART(ctx context.Context) (model, smart string, err error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "diskutil", "info", "-plist", rootDisk).Output()
	if err != nil {
		return "", "", fmt.Errorf("diskutil info: %w", err)
	}
	return parseSMART(bytes.NewReader(out))
}

// parseSMART reads `diskutil info -plist`. smart is "" for any status other than Verified
// and Failing ("Not Supported" on external enclosures).
//
// Two regexps instead of a plist decoder; the keys are flat strings at the top level.
func parseSMART(r io.Reader) (model, smart string, err error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", "", fmt.Errorf("read diskutil info: %w", err)
	}
	m := plistModel.FindSubmatch(raw)
	if m == nil {
		return "", "", fmt.Errorf("diskutil key %q missing", "MediaName")
	}
	model = html.UnescapeString(string(m[1]))
	if s := plistSMART.FindSubmatch(raw); s != nil {
		if status := strings.ToLower(string(s[1])); status == SMARTVerified || status == SMARTFailing {
			smart = status
		}
	}
	return model, smart, nil
}
