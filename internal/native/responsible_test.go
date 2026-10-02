package native

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResponsiblePID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pid       int32
		wantKnown bool
	}{
		{name: "own process answers to someone", pid: int32(os.Getpid()), wantKnown: true},
		{name: "launchd belongs to root", pid: 1},
		{name: "no such process", pid: 1 << 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ResponsiblePID(tt.pid)

			if tt.wantKnown {
				assert.Positive(t, got)
			} else {
				assert.Zero(t, got)
			}
		})
	}
}
