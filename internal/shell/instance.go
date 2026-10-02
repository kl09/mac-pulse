package shell

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const (
	lockRetries    = 5
	lockRetryDelay = 200 * time.Millisecond
)

// LockInstance takes the single-instance lock in dir, creating dir if needed. When another
// instance holds it, running is true, that instance has been told to show its panel and
// release is nil. Otherwise release must be called on exit.
func LockInstance(dir string) (release func(), running bool, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, fmt.Errorf("create lock directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open lock file: %w", err)
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		// A pid read in the microseconds between a new holder's flock and its pid
		// write is the previous holder's. Switch to a unix socket if that ever signals a stranger.
		var pid int
		if _, err := fmt.Fscan(f, &pid); err == nil && pid > 1 {
			_ = syscall.Kill(pid, syscall.SIGUSR2)
		}
		// The holder may be on its way out (a restart flushes its history first): wait a moment.
		for range lockRetries {
			time.Sleep(lockRetryDelay)
			if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
				break
			}
		}
	}
	if err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("lock %s: %w", f.Name(), err)
	}

	// Subscribed before the pid is published: an unhandled SIGUSR2 kills the process.
	usr2 := make(chan os.Signal, 1)
	signal.Notify(usr2, syscall.SIGUSR2)
	err = f.Truncate(0)
	if err == nil {
		_, err = f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
	}
	if err != nil {
		signal.Stop(usr2)
		_ = f.Close()
		return nil, false, fmt.Errorf("write pid to %s: %w", f.Name(), err)
	}
	go func() {
		for range usr2 {
			ShowPanel("")
		}
	}()
	return func() {
		signal.Stop(usr2)
		close(usr2)
		_ = f.Truncate(0)
		// Closing alone would leave the lock with any child forked a moment ago until it execs.
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, false, nil
}
