#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>

#include "native.h"

int mpGPURead(mpGPU *out) {
    @autoreleasepool {
        io_service_t service = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("IOAccelerator"));
        if (!service) {
            return -1;
        }
        NSDictionary *stats = CFBridgingRelease(IORegistryEntryCreateCFProperty(service, CFSTR("PerformanceStatistics"), kCFAllocatorDefault, 0));
        id model = CFBridgingRelease(IORegistryEntryCreateCFProperty(service, CFSTR("model"), kCFAllocatorDefault, 0));
        IOObjectRelease(service);
        NSNumber *util = stats[@"Device Utilization %"], *memory = stats[@"In use system memory"];
        if (![stats isKindOfClass:NSDictionary.class] || !util || !memory) {
            return -1;
        }
        memset(out, 0, sizeof(*out));
        out->util = util.intValue;
        out->memory = memory.unsignedLongLongValue;
        // Absent on some accelerators; those read 0.
        out->renderer = [stats[@"Renderer Utilization %"] intValue];
        out->tiler = [stats[@"Tiler Utilization %"] intValue];
        out->alloc = [stats[@"Alloc system memory"] unsignedLongLongValue];
        // Intel-era accelerators publish the model as NUL-terminated data.
        if ([model isKindOfClass:NSData.class]) {
            model = [[NSString alloc] initWithData:model encoding:NSUTF8StringEncoding];
        }
        if ([model isKindOfClass:NSString.class]) {
            strlcpy(out->model, [model UTF8String] ?: "", sizeof(out->model));
        }
        return 0;
    }
}
