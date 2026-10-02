package main

import (
	"sync"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/netinspect"
	"github.com/kl09/mac-pulse/internal/storage"
	"github.com/kl09/mac-pulse/internal/ui"
)

// latest is the newest result of every collector and what the views on screen need. The
// sampler, the inspector, the docker loop, the shell's message goroutine and the Storage
// tab's jobs all write it.
type latest struct {
	mu  sync.Mutex
	cur current
}

type current struct {
	snap *collector.Snapshot
	// net is nil while the inspector is stopped.
	net *netinspect.Report
	// docker is nil while no visible view is on the dev tab and until the CLI first answers.
	docker  *collector.DockerReport
	visible bool
	// allApps is true while a visible view lists every app.
	allApps bool
	// sendNet is true while a visible view shows the network report; the dev tab runs the
	// inspector for its listeners alone.
	sendNet bool
	pinned  bool
	// storageTab is true while a visible view shows the cleanup: the storage tab or the disk
	// detail, the only screens that get storage.
	storageTab bool
	// storage is the on-demand scan and cleanup measurement; tree is the last finished scan,
	// rooted at storage.Scan.Root, nil before one and after Clear.
	storage ui.StorageInput
	tree    *storage.Node
}

func (l *latest) update(change func(*current)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	change(&l.cur)
}

func (l *latest) get() current {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cur
}
