// AppleSMC user client: selector 2 with an 80-byte struct both ways. Read commands only.
#include <IOKit/IOKitLib.h>
#include <string.h>

#include "native.h"

typedef struct {
    uint8_t major, minor, build, reserved;
    uint16_t release;
} smcVersion;
typedef struct {
    uint16_t version, length;
    uint32_t cpuPLimit, gpuPLimit, memPLimit;
} smcPLimit;
typedef struct {
    uint32_t dataSize, dataType;
    uint8_t dataAttributes;
} smcInfo;
typedef struct {
    uint32_t key;
    smcVersion vers;
    smcPLimit pLimit;
    smcInfo info;
    uint8_t result, status, data8;
    uint32_t data32;
    uint8_t bytes[32];
} smcParam;

_Static_assert(sizeof(smcParam) == 80, "SMC param struct must be 80 bytes");

enum { smcSelector = 2, smcCmdRead = 5, smcCmdKeyAt = 8, smcCmdInfo = 9 };

static io_connect_t smcConn;

int mpSMCOpen(void) {
    io_service_t service = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("AppleSMC"));
    if (!service) {
        return -1;
    }
    kern_return_t kr = IOServiceOpen(service, mach_task_self(), 0, &smcConn);
    IOObjectRelease(service);
    return kr;
}

static int smcCall(smcParam *in, smcParam *out) {
    size_t n = sizeof(*out);
    memset(out, 0, sizeof(*out));
    kern_return_t kr = IOConnectCallStructMethod(smcConn, smcSelector, in, sizeof(*in), out, &n);
    return kr ? kr : out->result;
}

int mpSMCKeyInfo(uint32_t key, uint32_t *type, uint32_t *size) {
    smcParam in = {.key = key, .data8 = smcCmdInfo}, out;
    int rc = smcCall(&in, &out);
    *type = out.info.dataType;
    *size = out.info.dataSize;
    return rc;
}

int mpSMCRead(uint32_t key, uint32_t size, uint8_t *bytes) {
    smcParam in = {.key = key, .data8 = smcCmdRead, .info.dataSize = size}, out;
    int rc = smcCall(&in, &out);
    memcpy(bytes, out.bytes, sizeof(out.bytes));
    return rc;
}

int mpSMCKeyAt(uint32_t index, uint32_t *key) {
    smcParam in = {.data8 = smcCmdKeyAt, .data32 = index}, out;
    int rc = smcCall(&in, &out);
    *key = out.key;
    return rc;
}
