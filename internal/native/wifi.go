package native

// #include "native.h"
import "C"

import "fmt"

// WiFi describes the current link. The network name is absent on purpose: macOS gives it
// only to apps with Location permission.
type WiFi struct {
	Interface, PHY, Security       string
	RSSI, Noise, Channel, WidthMHz int
	BandGHz, TxRateMbps            float64
}

// WiFiInfo costs ~9 ms. ErrUnavailable: Wi-Fi is off or not associated.
func WiFiInfo() (WiFi, error) {
	var w C.mpWiFi
	if C.mpWiFiRead(&w) != 0 {
		return WiFi{}, fmt.Errorf("read Wi-Fi link: %w", ErrUnavailable)
	}
	return WiFi{
		Interface:  C.GoString(&w.name[0]),
		PHY:        phyName(int(w.phy)),
		Security:   securityName(int(w.security)),
		RSSI:       int(w.rssi),
		Noise:      int(w.noise),
		Channel:    int(w.channel),
		WidthMHz:   channelWidthMHz(int(w.width)),
		BandGHz:    bandGHz(int(w.band)),
		TxRateMbps: float64(w.txRate),
	}, nil
}

// ThermalState is 0 nominal, 1 fair, 2 serious, 3 critical.
func ThermalState() int {
	return int(C.mpThermalState())
}

// phyName maps CWPHYMode; "" for none or a mode newer than this table.
func phyName(mode int) string {
	names := []string{"", "802.11a", "802.11b", "802.11g", "802.11n", "802.11ac", "802.11ax", "802.11be"}
	if mode < 0 || mode >= len(names) {
		return ""
	}
	return names[mode]
}

// securityName maps CWSecurity; "" for unknown.
func securityName(security int) string {
	names := []string{
		"None", "WEP", "WPA Personal", "WPA/WPA2 Personal", "WPA2 Personal", "Personal", "Dynamic WEP",
		"WPA Enterprise", "WPA/WPA2 Enterprise", "WPA2 Enterprise", "Enterprise", "WPA3 Personal",
		"WPA3 Enterprise", "WPA2/WPA3 Personal", "OWE", "OWE Transition",
	}
	if security < 0 || security >= len(names) {
		return ""
	}
	return names[security]
}

// channelWidthMHz maps CWChannelWidth; 0 for unknown.
func channelWidthMHz(width int) int {
	widths := []int{0, 20, 40, 80, 160}
	if width < 0 || width >= len(widths) {
		return 0
	}
	return widths[width]
}

// bandGHz maps CWChannelBand; 0 for unknown.
func bandGHz(band int) float64 {
	bands := []float64{0, 2.4, 5, 6}
	if band < 0 || band >= len(bands) {
		return 0
	}
	return bands[band]
}
