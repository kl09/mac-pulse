// Package native reads what macOS only exposes through C APIs: SMC fans, temperatures and
// power, IOReport energy and cluster frequencies, per-process rusage, the Wi-Fi link and the
// thermal state. Read-only and root-free; every function may be called from any goroutine.
package native

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework IOKit -framework Foundation -framework CoreWLAN -lIOReport
*/
import "C"

import "errors"

// ErrUnavailable means the source is closed to this process or absent on this machine;
// callers show "unavailable" and do not log it.
var ErrUnavailable = errors.New("native: source unavailable")
