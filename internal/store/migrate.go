package store

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

// seriesCountV1 is the length of minute.V before the three power series were appended.
const seriesCountV1 = 9

// stateV1 is state as version 1 wrote it; gob cannot decode its shorter arrays into minute.
type stateV1 struct {
	Version     int
	Minutes     []minuteV1
	Hours       []hour
	Icons       map[string]string
	Day         string
	DiskWritten uint64
	NetDown     uint64
	NetUp       uint64
	NetApps     map[string]traffic
}

type minuteV1 struct {
	At int64
	N  int32
	V  [seriesCountV1]float32
}

// decodeState reads history.gob of this version or an older known one; migrated says the
// file on disk is still in the old format.
func decodeState(raw []byte) (st state, migrated bool, err error) {
	var head struct{ Version int }
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&head); err != nil {
		return state{}, false, fmt.Errorf("decode history version: %w", err)
	}
	switch head.Version {
	case version:
		if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&st); err != nil {
			return state{}, false, fmt.Errorf("decode history: %w", err)
		}
		return st, false, nil
	case 1:
		var old stateV1
		if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&old); err != nil {
			return state{}, false, fmt.Errorf("decode history version 1: %w", err)
		}
		st = state{
			Version: version, Minutes: make([]minute, len(old.Minutes)), Hours: old.Hours, Icons: old.Icons,
			Day: old.Day, DiskWritten: old.DiskWritten, NetDown: old.NetDown, NetUp: old.NetUp, NetApps: old.NetApps,
		}
		for i, m := range old.Minutes {
			st.Minutes[i] = minute{At: m.At, N: m.N}
			// The new series follow the old ones, so they stay 0: no reading.
			copy(st.Minutes[i].V[:], m.V[:])
		}
		return st, true, nil
	default:
		return state{}, false, fmt.Errorf("history version %d is not supported", head.Version)
	}
}
