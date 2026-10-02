#import <CoreWLAN/CoreWLAN.h>
#import <Foundation/Foundation.h>

#include "native.h"

// The SSID and BSSID need Location permission, which mac-pulse never asks for; they are not read.
int mpWiFiRead(mpWiFi *out) {
    @autoreleasepool {
        CWInterface *interface = CWWiFiClient.sharedWiFiClient.interface;
        CWChannel *channel = interface.wlanChannel;
        if (!interface.powerOn || !channel) {
            return -1;
        }
        memset(out, 0, sizeof(*out));
        strlcpy(out->name, interface.interfaceName.UTF8String ?: "", sizeof(out->name));
        out->rssi = (int)interface.rssiValue;
        out->noise = (int)interface.noiseMeasurement;
        out->channel = (int)channel.channelNumber;
        out->band = (int)channel.channelBand;
        out->width = (int)channel.channelWidth;
        out->phy = (int)interface.activePHYMode;
        out->security = (long)interface.security;
        out->txRate = interface.transmitRate;
        return 0;
    }
}

int mpThermalState(void) {
    return (int)NSProcessInfo.processInfo.thermalState;
}
