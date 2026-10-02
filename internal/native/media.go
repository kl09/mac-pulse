package native

/*
#cgo LDFLAGS: -framework CoreAudio -framework CoreMediaIO
#include "native.h"
*/
import "C"

// At most this many recording processes are named; a handful is already unusual.
const maxMicUsers = 16

// LowPowerMode reports whether macOS runs in Low Power Mode.
func LowPowerMode() bool {
	return C.mpLowPowerMode() != 0
}

// MediaUse reports whether the microphone and the camera are in use and, on macOS 14 and
// later, which processes record: micPIDs and micBundles go index for index, a bundle id may
// be "". macOS does not say who uses the camera. It asks for neither permission and shows
// no prompt.
func MediaUse() (mic bool, micPIDs []int32, micBundles []string, camera bool) {
	var (
		users         [maxMicUsers]C.mpMicUser
		cmic, ccamera C.int
	)
	n := int(C.mpMediaRead(&cmic, &ccamera, &users[0], maxMicUsers))
	for _, user := range users[:n] {
		micPIDs = append(micPIDs, int32(user.pid))
		micBundles = append(micBundles, C.GoString(&user.bundle[0]))
	}
	return cmic != 0, micPIDs, micBundles, ccamera != 0
}
