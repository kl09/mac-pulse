package native

/*
#include <dlfcn.h>
#include <sys/types.h>

// A private libSystem symbol, looked up by name: a direct reference would stop the app from
// launching on a macOS that drops it.
static int mpResponsiblePID(int pid) {
    pid_t (*responsible)(pid_t) = (pid_t (*)(pid_t))dlsym(RTLD_DEFAULT, "responsibility_get_pid_responsible_for_pid");
    return responsible ? responsible(pid) : 0;
}
*/
import "C"

// ResponsiblePID is the process macOS holds responsible for pid: the app a helper works
// for, even when launchd is its parent. 0 means unknown, or that this macOS lacks the call.
func ResponsiblePID(pid int32) int32 {
	return max(int32(C.mpResponsiblePID(C.int(pid))), 0)
}
