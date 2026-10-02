package collector

import (
	"math"

	"github.com/kl09/mac-pulse/internal/native"
)

// readPower also returns the chip reading for its cluster frequencies; they are 0 when it failed.
func readPower() (Power, native.Power) {
	chip, chipErr := native.ReadPower()
	system, adapter, smcErr := native.SystemPower()
	return Power{
		SystemW:  watts(system, smcErr),
		AdapterW: watts(adapter, smcErr),
		CPUW:     watts(chip.CPUW, chipErr),
		GPUW:     watts(chip.GPUW, chipErr),
	}, chip
}

// watts is nil for a failed read and for a value no power rail can have: NaN would also
// break the JSON encoding of the whole state.
func watts(v float64, err error) *float64 {
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	return &v
}
