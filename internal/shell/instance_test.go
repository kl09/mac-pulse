package shell

import (
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockInstance(t *testing.T) {
	t.Parallel()

	// The "held" case signals this very process, which would die of an unhandled SIGUSR2.
	shown := make(chan os.Signal, 1)
	signal.Notify(shown, syscall.SIGUSR2)
	t.Cleanup(func() { signal.Stop(shown) })

	blocked := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocked, nil, 0o600))
	tests := []struct {
		name string
		dir  string
		// earlier is what another LockInstance call did to dir first: "", "held", "released"
		// or "releasing" (released a moment after the second call started).
		earlier     string
		wantRunning bool
		wantErr     string
	}{
		{name: "free lock in a new directory", dir: filepath.Join(t.TempDir(), "new", "dir")},
		{name: "held lock asks the holder to show its panel", dir: t.TempDir(), earlier: "held", wantRunning: true},
		{name: "released lock is free again", dir: t.TempDir(), earlier: "released"},
		{name: "lock released while the second caller waits is taken over", dir: t.TempDir(), earlier: "releasing"},
		{name: "directory is a file", dir: blocked, wantErr: "create lock directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.earlier != "" {
				release, running, err := LockInstance(tt.dir)
				require.NoError(t, err)
				require.False(t, running)
				switch tt.earlier {
				case "released":
					release()
				case "releasing":
					time.AfterFunc(2*lockRetryDelay, release)
				default:
					defer release()
				}
			}

			release, running, err := LockInstance(tt.dir)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantRunning, running)
			if tt.wantRunning {
				assert.Nil(t, release)
				select {
				case <-shown:
				case <-time.After(5 * time.Second):
					assert.Fail(t, "the holder got no SIGUSR2")
				}
				return
			}
			defer release()
			pid, err := os.ReadFile(filepath.Join(tt.dir, "lock"))
			require.NoError(t, err)
			assert.Equal(t, strconv.Itoa(os.Getpid()), string(pid))
		})
	}
}
