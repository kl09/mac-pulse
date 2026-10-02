// IOReport is private: the declarations below are the known ABI of libIOReport.dylib.
#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>

#include "native.h"

typedef struct IOReportSubscription *IOReportSubscriptionRef;
extern CFDictionaryRef IOReportCopyChannelsInGroup(CFStringRef group, CFStringRef subgroup, uint64_t a, uint64_t b, uint64_t c);
extern void IOReportMergeChannels(CFDictionaryRef into, CFDictionaryRef from, CFTypeRef unused);
extern IOReportSubscriptionRef IOReportCreateSubscription(void *a, CFMutableDictionaryRef desired, CFMutableDictionaryRef *subscribed,
                                                          uint64_t channelID, CFTypeRef b);
extern CFDictionaryRef IOReportCreateSamples(IOReportSubscriptionRef subscription, CFMutableDictionaryRef subscribed, CFTypeRef a);
extern CFDictionaryRef IOReportCreateSamplesDelta(CFDictionaryRef prev, CFDictionaryRef cur, CFTypeRef a);
extern CFStringRef IOReportChannelGetGroup(CFDictionaryRef channel);
extern CFStringRef IOReportChannelGetChannelName(CFDictionaryRef channel);
extern CFStringRef IOReportChannelGetUnitLabel(CFDictionaryRef channel);
extern int64_t IOReportSimpleGetIntegerValue(CFDictionaryRef channel, int32_t index);
extern int32_t IOReportStateGetCount(CFDictionaryRef channel);
extern int64_t IOReportStateGetResidency(CFDictionaryRef channel, int32_t index);

static NSString *const kChannels = @"IOReportChannels";

static BOOL tried;
static IOReportSubscriptionRef subscription;
static CFMutableDictionaryRef subscribed;
static CFDictionaryRef previous;
static uint64_t previousNs;
static NSArray<NSNumber *> *eFreqs, *pFreqs, *gpuFreqs;

// pmgr tables are (frequency, voltage) uint32 pairs; the frequency is Hz on M1-M3 and kHz on M4.
static NSArray<NSNumber *> *pmgrFreqs(NSString *property) {
    io_registry_entry_t entry = IORegistryEntryFromPath(kIOMainPortDefault, "IODeviceTree:/arm-io/pmgr");
    if (!entry) {
        return @[];
    }
    NSData *data = CFBridgingRelease(IORegistryEntryCreateCFProperty(entry, (__bridge CFStringRef)property, kCFAllocatorDefault, 0));
    IOObjectRelease(entry);
    NSMutableArray *freqs = [NSMutableArray array];
    if (![data isKindOfClass:NSData.class]) {
        return freqs;
    }
    const uint32_t *pairs = data.bytes;
    for (NSUInteger i = 0; i + 1 < data.length / 4; i += 2) {
        if (pairs[i] != 0) {
            [freqs addObject:@(pairs[i] > 100000000 ? pairs[i] / 1e6 : pairs[i] / 1e3)];
        }
    }
    return freqs;
}

// Subscribing to two energy channels instead of the ~250 in the group keeps a sample cheap.
static BOOL subscribe(void) {
    CFDictionaryRef energy = IOReportCopyChannelsInGroup(CFSTR("Energy Model"), NULL, 0, 0, 0);
    CFDictionaryRef cpu = IOReportCopyChannelsInGroup(CFSTR("CPU Stats"), CFSTR("CPU Core Performance States"), 0, 0, 0);
    CFDictionaryRef gpu = IOReportCopyChannelsInGroup(CFSTR("GPU Stats"), CFSTR("GPU Performance States"), 0, 0, 0);
    if (energy && cpu && gpu) {
        NSMutableDictionary *want = [(__bridge NSDictionary *)energy mutableCopy];
        NSMutableArray *channels = [NSMutableArray array];
        for (id channel in want[kChannels]) {
            NSString *name = (__bridge NSString *)IOReportChannelGetChannelName((__bridge CFDictionaryRef)channel);
            if ([name isEqualToString:@"CPU Energy"] || [name isEqualToString:@"GPU Energy"]) {
                [channels addObject:channel];
            }
        }
        want[kChannels] = channels;
        IOReportMergeChannels((__bridge CFDictionaryRef)want, cpu, NULL);
        IOReportMergeChannels((__bridge CFDictionaryRef)want, gpu, NULL);
        subscription = IOReportCreateSubscription(NULL, (__bridge CFMutableDictionaryRef)want, &subscribed, 0, NULL);
    }
    if (energy) {
        CFRelease(energy);
    }
    if (cpu) {
        CFRelease(cpu);
    }
    if (gpu) {
        CFRelease(gpu);
    }
    if (!subscription || !subscribed) {
        return NO;
    }
    eFreqs = pmgrFreqs(@"voltage-states1-sram");
    pFreqs = pmgrFreqs(@"voltage-states5-sram");
    gpuFreqs = pmgrFreqs(@"voltage-states9");
    return YES;
}

// Adds a channel's residency in its running states to active and residency × MHz to weighted.
// The states before the first frequency (IDLE, DOWN, OFF) have none.
static void addResidency(CFDictionaryRef channel, NSArray<NSNumber *> *freqs, double *active, double *weighted) {
    int count = IOReportStateGetCount(channel);
    int skip = MAX(count - (int)freqs.count, 0);
    for (int i = skip; i < count; i++) {
        double residency = IOReportStateGetResidency(channel, i);
        *active += residency;
        *weighted += residency * freqs[i - skip].doubleValue;
    }
}

int mpPowerRead(mpPower *out) {
    @autoreleasepool {
        if (!tried) {
            tried = YES;
            if (!subscribe()) {
                subscription = NULL;
            }
        }
        if (!subscription) {
            return -1;
        }
        CFDictionaryRef current = IOReportCreateSamples(subscription, subscribed, NULL);
        uint64_t nowNs = clock_gettime_nsec_np(CLOCK_MONOTONIC_RAW);
        if (!current) {
            return -1;
        }
        CFDictionaryRef last = previous;
        double seconds = (nowNs - previousNs) / 1e9;
        previous = current;
        previousNs = nowNs;
        if (!last) {
            return 1;
        }
        NSDictionary *delta = CFBridgingRelease(IOReportCreateSamplesDelta(last, current, NULL));
        CFRelease(last);
        if (!delta || seconds <= 0) {
            return -1;
        }
        memset(out, 0, sizeof(*out));
        double eActive = 0, eWeighted = 0, pActive = 0, pWeighted = 0, gpuActive = 0, gpuWeighted = 0;
        for (id item in delta[kChannels]) {
            CFDictionaryRef channel = (__bridge CFDictionaryRef)item;
            NSString *group = (__bridge NSString *)IOReportChannelGetGroup(channel);
            NSString *name = (__bridge NSString *)IOReportChannelGetChannelName(channel);
            if ([group isEqualToString:@"Energy Model"]) {
                NSString *unit = [(__bridge NSString *)IOReportChannelGetUnitLabel(channel)
                    stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet];
                // The unit differs between channels and chips.
                double joules = [unit isEqualToString:@"mJ"] ? 1e-3 : [unit isEqualToString:@"uJ"] ? 1e-6 : [unit isEqualToString:@"nJ"] ? 1e-9 : 0;
                double watts = IOReportSimpleGetIntegerValue(channel, 0) * joules / seconds;
                if ([name isEqualToString:@"CPU Energy"]) {
                    out->cpuW = watts;
                } else {
                    out->gpuW = watts;
                }
            } else if ([group isEqualToString:@"GPU Stats"]) {
                if ([name isEqualToString:@"GPUPH"]) {
                    addResidency(channel, gpuFreqs, &gpuActive, &gpuWeighted);
                }
            } else if ([name hasPrefix:@"ECPU"]) {
                addResidency(channel, eFreqs, &eActive, &eWeighted);
            } else if ([name hasPrefix:@"PCPU"]) {
                addResidency(channel, pFreqs, &pActive, &pWeighted);
            }
        }
        out->eMHz = eActive > 0 ? eWeighted / eActive : 0;
        out->pMHz = pActive > 0 ? pWeighted / pActive : 0;
        out->gpuMHz = gpuActive > 0 ? gpuWeighted / gpuActive : 0;
        return 0;
    }
}
