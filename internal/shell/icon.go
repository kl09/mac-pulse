package shell

/*
#include <stdlib.h>
#include "shell.h"
*/
import "C"

import (
	"sync"
	"unsafe"
)

// Never evicts; a few hundred apps × ~6 KB. Add an LRU if RSS shows it.
var icons = iconCache{png: map[string][]byte{}}

type iconCache struct {
	mu  sync.Mutex
	png map[string][]byte
}

// appIcon returns the Finder icon of the bundle or file at path as a 64 px PNG
// (32 pt @2x); nil for an empty path. The frontend loads it as mp://app/icon?path=….
//
// The lock also serialises AppKit drawing and keeps one render per path;
// per-key singleflight if the first open ever stutters.
func appIcon(path string) []byte {
	if path == "" {
		return nil
	}
	icons.mu.Lock()
	defer icons.mu.Unlock()
	if png, ok := icons.png[path]; ok {
		return png
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var n C.int
	buf := C.mpAppIcon(cpath, &n)
	defer C.free(buf)
	png := C.GoBytes(buf, n)
	icons.png[path] = png
	return png
}
