package native

/*
#include <errno.h>
#include <libproc.h>
#include <sys/resource.h>

static int mpRusage(int pid, struct rusage_info_v6 *ri) {
    return proc_pid_rusage(pid, RUSAGE_INFO_V6, (rusage_info_t *)ri) ? errno : 0;
}
*/
import "C"

import (
	"fmt"
	"syscall"
)

// Usage holds counters accumulated since the process started.
type Usage struct {
	DiskRead, DiskWrite, EnergyNJ uint64
}

// ProcUsage answers ErrUnavailable for another user's process and for one that has exited.
func ProcUsage(pid int32) (Usage, error) {
	var ri C.struct_rusage_info_v6
	switch errno := syscall.Errno(C.mpRusage(C.int(pid), &ri)); errno {
	case 0:
		return Usage{
			DiskRead:  uint64(ri.ri_diskio_bytesread),
			DiskWrite: uint64(ri.ri_diskio_byteswritten),
			EnergyNJ:  uint64(ri.ri_energy_nj),
		}, nil
	case syscall.EPERM, syscall.ESRCH:
		return Usage{}, fmt.Errorf("rusage of pid %d: %w", pid, ErrUnavailable)
	default:
		return Usage{}, fmt.Errorf("rusage of pid %d: %w", pid, errno)
	}
}
