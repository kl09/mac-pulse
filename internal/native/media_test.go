package native

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMediaUse(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads this Mac's hardware")
	}

	mic, pids, bundles, _ := MediaUse()

	assert.Len(t, bundles, len(pids))
	assert.LessOrEqual(t, len(pids), maxMicUsers)
	for _, pid := range pids {
		assert.Positive(t, pid)
	}
	if len(pids) > 0 {
		assert.True(t, mic, "a recording process means the microphone is in use")
	}
}
