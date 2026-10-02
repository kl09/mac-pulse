#ifndef MP_NATIVE_H
#define MP_NATIVE_H

#include <stdint.h>

// SMC calls return 0, a kern_return_t or the SMC result byte. Keys and types are FourCC.
int mpSMCOpen(void);
int mpSMCKeyInfo(uint32_t key, uint32_t *type, uint32_t *size);
// bytes holds 32.
int mpSMCRead(uint32_t key, uint32_t size, uint8_t *bytes);
int mpSMCKeyAt(uint32_t index, uint32_t *key);

typedef struct {
    double cpuW, gpuW, eMHz, pMHz, gpuMHz;
} mpPower;
// Averages since the previous call: 0 on success, 1 when this call only took the first
// sample, -1 when IOReport is unavailable. Not reentrant.
int mpPowerRead(mpPower *out);

typedef struct {
    char name[32];
    int rssi, noise, channel, band, width, phy;
    long security;
    double txRate;
} mpWiFi;
// 0 when the interface is associated.
int mpWiFiRead(mpWiFi *out);
int mpThermalState(void);

typedef struct {
    char model[64];
    int util, renderer, tiler;
    uint64_t memory, alloc;
} mpGPU;
// 0 when an IOAccelerator publishes its utilization.
int mpGPURead(mpGPU *out);

typedef struct {
    int pid;
    char bundle[128];
} mpMicUser;
// Sets mic and camera to 1 when the device is in use and fills users, at most max of them, with
// the processes that record; returns how many it filled, always 0 before macOS 14. Asks for no permission.
int mpMediaRead(int *mic, int *camera, mpMicUser *users, int max);
int mpLowPowerMode(void);

#endif
