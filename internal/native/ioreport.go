package native

// #include "native.h"
import "C"

import (
	"fmt"
	"sync"
)

// Power is the average since the previous ReadPower. A frequency of 0 means the cluster
// was idle for the whole interval.
type Power struct {
	CPUW, GPUW, EMHz, PMHz, GPUMHz float64
}

// powerMu guards the previous sample kept on the C side.
var powerMu sync.Mutex

// ReadPower subscribes on the first call (~190 ms) and returns ErrUnavailable from it:
// there is no interval to average over yet.
func ReadPower() (Power, error) {
	powerMu.Lock()
	defer powerMu.Unlock()
	var p C.mpPower
	if rc := C.mpPowerRead(&p); rc != 0 {
		return Power{}, fmt.Errorf("read IOReport power (%d): %w", int(rc), ErrUnavailable)
	}
	return Power{
		CPUW: float64(p.cpuW), GPUW: float64(p.gpuW),
		EMHz: float64(p.eMHz), PMHz: float64(p.pMHz), GPUMHz: float64(p.gpuMHz),
	}, nil
}
