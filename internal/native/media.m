#import <CoreAudio/CoreAudio.h>
#import <CoreMediaIO/CMIOHardware.h>
#import <Foundation/Foundation.h>

#include "native.h"

// The process objects are macOS 14 API. Spelled as codes the file still builds for 13, where
// reading the list fails and only the state of the device is known.
enum {
    kProcessObjectList = 'prs#',
    kProcessPID = 'ppid',
    kProcessBundleID = 'pbid',
    kProcessIsRunningInput = 'piri',
};

static BOOL audioRead(AudioObjectID object, AudioObjectPropertySelector selector, UInt32 size, void *out) {
    AudioObjectPropertyAddress address = {selector, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    return AudioObjectGetPropertyData(object, &address, 0, NULL, &size, out) == noErr;
}

static int micUsers(mpMicUser *users, int max) {
    AudioObjectPropertyAddress address = {kProcessObjectList, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain};
    UInt32 size = 0;
    if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &address, 0, NULL, &size) != noErr || size == 0) {
        return 0;
    }
    AudioObjectID *processes = malloc(size);
    int n = 0;
    if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &address, 0, NULL, &size, processes) == noErr) {
        for (UInt32 i = 0; i < size / sizeof *processes && n < max; i++) {
            UInt32 recording = 0;
            pid_t pid = 0;
            if (!audioRead(processes[i], kProcessIsRunningInput, sizeof recording, &recording) || !recording ||
                !audioRead(processes[i], kProcessPID, sizeof pid, &pid)) {
                continue;
            }
            CFStringRef bundle = NULL;
            audioRead(processes[i], kProcessBundleID, sizeof bundle, &bundle);
            users[n].pid = pid;
            strlcpy(users[n].bundle, ((NSString *)CFBridgingRelease(bundle)).UTF8String ?: "", sizeof users[n].bundle);
            n++;
        }
    }
    free(processes);
    return n;
}

static BOOL cameraRunning(void) {
    CMIOObjectPropertyAddress address = {kCMIOHardwarePropertyDevices, kCMIOObjectPropertyScopeGlobal, kCMIOObjectPropertyElementMain};
    UInt32 size = 0, used = 0;
    if (CMIOObjectGetPropertyDataSize(kCMIOObjectSystemObject, &address, 0, NULL, &size) != kCMIOHardwareNoError || size == 0) {
        return NO;
    }
    CMIOObjectID *devices = malloc(size);
    BOOL running = NO;
    if (CMIOObjectGetPropertyData(kCMIOObjectSystemObject, &address, 0, NULL, size, &used, devices) == kCMIOHardwareNoError) {
        address.mSelector = kCMIODevicePropertyDeviceIsRunningSomewhere;
        for (UInt32 i = 0; i < used / sizeof *devices && !running; i++) {
            UInt32 on = 0, read = 0;
            running = CMIOObjectGetPropertyData(devices[i], &address, 0, NULL, sizeof on, &read, &on) == kCMIOHardwareNoError && on;
        }
    }
    free(devices);
    return running;
}

int mpMediaRead(int *mic, int *camera, mpMicUser *users, int max) {
    @autoreleasepool {
        int n = micUsers(users, max);
        AudioObjectID input = kAudioObjectUnknown;
        UInt32 running = 0;
        // An app may record from a device that is not the default one, so a recording process counts too.
        *mic = n > 0 || (audioRead(kAudioObjectSystemObject, kAudioHardwarePropertyDefaultInputDevice, sizeof input, &input) &&
                         audioRead(input, kAudioDevicePropertyDeviceIsRunningSomewhere, sizeof running, &running) && running);
        *camera = cameraRunning();
        return n;
    }
}

int mpLowPowerMode(void) {
    return NSProcessInfo.processInfo.lowPowerModeEnabled;
}
