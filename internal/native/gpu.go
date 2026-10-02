package native

// #include "native.h"
import "C"

import "fmt"

// GPUStats is the PerformanceStatistics dictionary of the first IOAccelerator.
type GPUStats struct {
	Model string
	// Util, Renderer and Tiler are percent.
	Util, Renderer, Tiler int
	// Memory is in use, MemoryAlloc allocated, both in bytes.
	Memory, MemoryAlloc uint64
}

// GPU answers ErrUnavailable on a Mac without an accelerator that reports utilization.
func GPU() (GPUStats, error) {
	var g C.mpGPU
	if C.mpGPURead(&g) != 0 {
		return GPUStats{}, fmt.Errorf("gpu statistics: %w", ErrUnavailable)
	}
	return GPUStats{
		Model: C.GoString(&g.model[0]),
		Util:  int(g.util), Renderer: int(g.renderer), Tiler: int(g.tiler),
		Memory: uint64(g.memory), MemoryAlloc: uint64(g.alloc),
	}, nil
}
