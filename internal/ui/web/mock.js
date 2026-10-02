// Mock data for browser design work: open index.html?mock=1 from disk. Never embedded in the binary.
// The value after "window.MP_MOCK =" is strict JSON: internal/ui/state_test.go round-trips it through
// the Go types in internal/ui/state.go, so every key must exist there and every Go field must be here.
window.MP_MOCK = {
  "state": {
    "time": 1790777400000,
    "has_rates": true,
    "cpu": {
      "total": 37.2,
      "user": 24.9,
      "system": 12.3,
      "cores": [81.0, 62.5, 55.1, 48.7, 44.0, 38.2, 31.6, 27.9, 12.4, 9.8, 3.1, 1.2],
      "load": [4.21, 3.87, 3.52],
      "uptime_s": 1391000,
      "temp_c": 54.1,
      "model": "Apple M4 Pro",
      "clusters": [
        {"name": "Efficiency", "cores": 4, "freq_mhz": 2570, "temp_c": 58.6},
        {"name": "Performance", "cores": 8, "freq_mhz": 3104, "temp_c": 70.9}
      ]
    },
    "memory": {
      "total": 25769803776,
      "used": 19616759808,
      "app": 12545163264,
      "wired": 3640655872,
      "compressed": 3430940672,
      "cached": 5683281920,
      "free": 419430400,
      "pressure": "normal",
      "swap_used": 3732930560,
      "swap_total": 5368709120,
      "page_in_rate": 1261568.0,
      "page_out_rate": 0.0,
      "swap_in_rate": 49152.0,
      "swap_out_rate": 0.0,
      "compress_rate": 3391488.0,
      "decompress_rate": 868352.0
    },
    "disk": {
      "total": 994662584320,
      "free": 227633266688,
      "read_rate": 1468006.4,
      "write_rate": 348160.0,
      "written_today": 40802189312,
      "model": "APPLE SSD AP1024Z",
      "smart": "verified",
      "read_total": 1893400870912,
      "write_total": 1201542234112,
      "volumes": [
        {"name": "Macintosh HD", "mount": "/", "fs": "apfs", "total": 994662584320, "free": 227633266688, "ejectable": false},
        {"name": "Backup", "mount": "/Volumes/Backup", "fs": "apfs", "total": 2000398934016, "free": 164282499072, "ejectable": true},
        {"name": "SAMSUNG T7", "mount": "/Volumes/SAMSUNG T7", "fs": "exfat", "total": 500107862016, "free": 412316860416, "ejectable": true}
      ]
    },
    "network": {
      "down_rate": 4508876.8,
      "up_rate": 126976.0,
      "down_today": 6652166144,
      "up_today": 429916160,
      "down_7d": 48318382080,
      "up_7d": 3221225472,
      "down_30d": 214748364800,
      "up_30d": 15032385536
    },
    "gpu": {
      "util": 41.0,
      "memory": 5011390464,
      "model": "Apple M4 Pro",
      "renderer": 38.0,
      "tiler": 12.0,
      "memory_alloc": 6627000320,
      "freq_mhz": 744,
      "temp_c": 55.3
    },
    "battery": {
      "percent": 78,
      "state": "battery",
      "time_remaining_s": 15480,
      "power_w": 11.3,
      "health": 93,
      "cycles": 119,
      "temp_c": 31.2,
      "design_mah": 6249,
      "max_mah": 5831,
      "voltage_v": 12.61,
      "amperage_a": 0.896,
      "adapter": null,
      "unplugged_at": 1790770680000,
      "low_power": false
    },
    "power": {"system_w": 13.2, "adapter_w": 0.0, "cpu_w": 4.8, "gpu_w": 1.6},
    "sensors": {
      "thermal": "nominal",
      "fans": [
        {"rpm": 2412, "min": 2317, "max": 7826},
        {"rpm": 0, "min": 2317, "max": 7826}
      ],
      "temps": [
        {"group": "Power management chip", "key": "TPD0", "temp_c": 54.1},
        {"group": "Power management chip", "key": "TPD1", "temp_c": 52.7},
        {"group": "Power management chip", "key": "TPD2", "temp_c": 51.3},
        {"group": "CPU performance", "key": "Tp00", "temp_c": 56.5},
        {"group": "CPU performance", "key": "Tp01", "temp_c": 54.0},
        {"group": "CPU performance", "key": "Tp02", "temp_c": 59.5},
        {"group": "CPU performance", "key": "Tp03", "temp_c": 54.9},
        {"group": "CPU performance", "key": "Tp04", "temp_c": 53.3},
        {"group": "CPU performance", "key": "Tp05", "temp_c": 59.6},
        {"group": "CPU performance", "key": "Tp06", "temp_c": 69.4},
        {"group": "CPU performance", "key": "Tp07", "temp_c": 67.2},
        {"group": "CPU performance", "key": "Tp08", "temp_c": 66.5},
        {"group": "CPU performance", "key": "Tp09", "temp_c": 56.2},
        {"group": "CPU performance", "key": "Tp0A", "temp_c": 62.2},
        {"group": "CPU performance", "key": "Tp0B", "temp_c": 57.3},
        {"group": "CPU performance", "key": "Tp0C", "temp_c": 55.3},
        {"group": "CPU performance", "key": "Tp0D", "temp_c": 54.0},
        {"group": "CPU performance", "key": "Tp0E", "temp_c": 56.1},
        {"group": "CPU performance", "key": "Tp0F", "temp_c": 69.6},
        {"group": "CPU performance", "key": "Tp0G", "temp_c": 67.7},
        {"group": "CPU performance", "key": "Tp0H", "temp_c": 67.3},
        {"group": "CPU performance", "key": "Tp0I", "temp_c": 67.2},
        {"group": "CPU performance", "key": "Tp0J", "temp_c": 55.7},
        {"group": "CPU performance", "key": "Tp0K", "temp_c": 57.9},
        {"group": "CPU performance", "key": "Tp0L", "temp_c": 63.9},
        {"group": "CPU performance", "key": "Tp0M", "temp_c": 65.9},
        {"group": "CPU performance", "key": "Tp0N", "temp_c": 68.2},
        {"group": "CPU performance", "key": "Tp0O", "temp_c": 68.7},
        {"group": "CPU performance", "key": "Tp0P", "temp_c": 53.6},
        {"group": "CPU performance", "key": "Tp0Q", "temp_c": 63.5},
        {"group": "CPU performance", "key": "Tp0R", "temp_c": 64.8},
        {"group": "CPU performance", "key": "Tp0S", "temp_c": 61.6},
        {"group": "CPU performance", "key": "Tp0T", "temp_c": 55.4},
        {"group": "CPU performance", "key": "Tp0U", "temp_c": 61.0},
        {"group": "CPU performance", "key": "Tp0V", "temp_c": 53.7},
        {"group": "CPU performance", "key": "Tp0W", "temp_c": 69.8},
        {"group": "CPU performance", "key": "Tp0X", "temp_c": 68.4},
        {"group": "CPU performance", "key": "Tp0Y", "temp_c": 62.4},
        {"group": "CPU performance", "key": "Tp0Z", "temp_c": 57.7},
        {"group": "CPU performance", "key": "Tp0a", "temp_c": 69.3},
        {"group": "CPU performance", "key": "Tp0b", "temp_c": 62.9},
        {"group": "CPU performance", "key": "Tp0c", "temp_c": 68.8},
        {"group": "CPU performance", "key": "Tp0d", "temp_c": 68.1},
        {"group": "CPU performance", "key": "Tp0e", "temp_c": 61.7},
        {"group": "CPU performance", "key": "Tp0f", "temp_c": 59.9},
        {"group": "CPU performance", "key": "Tp0g", "temp_c": 63.4},
        {"group": "CPU performance", "key": "Tp0h", "temp_c": 60.2},
        {"group": "CPU performance", "key": "Tp0i", "temp_c": 55.1},
        {"group": "CPU performance", "key": "Tp0j", "temp_c": 57.8},
        {"group": "CPU performance", "key": "Tp0k", "temp_c": 67.4},
        {"group": "CPU performance", "key": "Tp0l", "temp_c": 52.8},
        {"group": "CPU performance", "key": "Tp0m", "temp_c": 52.9},
        {"group": "CPU performance", "key": "Tp0n", "temp_c": 63.9},
        {"group": "CPU performance", "key": "Tp0o", "temp_c": 57.3},
        {"group": "CPU performance", "key": "Tp0p", "temp_c": 62.2},
        {"group": "CPU performance", "key": "Tp0q", "temp_c": 61.0},
        {"group": "CPU performance", "key": "Tp0r", "temp_c": 58.5},
        {"group": "CPU performance", "key": "Tp0s", "temp_c": 70.9},
        {"group": "CPU performance", "key": "Tp0t", "temp_c": 55.7},
        {"group": "CPU performance", "key": "Tp0u", "temp_c": 59.8},
        {"group": "CPU performance", "key": "Tp0v", "temp_c": 55.9},
        {"group": "CPU performance", "key": "Tp0w", "temp_c": 64.0},
        {"group": "CPU performance", "key": "Tp0x", "temp_c": 57.2},
        {"group": "CPU performance", "key": "Tp0y", "temp_c": 58.8},
        {"group": "CPU performance", "key": "Tp0z", "temp_c": 66.2},
        {"group": "CPU performance", "key": "Tp10", "temp_c": 58.1},
        {"group": "CPU performance", "key": "Tp11", "temp_c": 62.6},
        {"group": "CPU performance", "key": "Tp12", "temp_c": 69.2},
        {"group": "CPU performance", "key": "Tp13", "temp_c": 53.9},
        {"group": "CPU performance", "key": "Tp14", "temp_c": 53.2},
        {"group": "CPU performance", "key": "Tp15", "temp_c": 56.3},
        {"group": "CPU performance", "key": "Tp16", "temp_c": 66.5},
        {"group": "CPU performance", "key": "Tp17", "temp_c": 63.7},
        {"group": "CPU performance", "key": "Tp18", "temp_c": 56.5},
        {"group": "CPU performance", "key": "Tp19", "temp_c": 58.3},
        {"group": "CPU performance", "key": "Tp1A", "temp_c": 55.4},
        {"group": "CPU performance", "key": "Tp1B", "temp_c": 60.7},
        {"group": "CPU performance", "key": "Tp1C", "temp_c": 52.8},
        {"group": "CPU performance", "key": "Tp1D", "temp_c": 65.2},
        {"group": "CPU performance", "key": "Tp1E", "temp_c": 69.0},
        {"group": "CPU performance", "key": "Tp1F", "temp_c": 70.1},
        {"group": "CPU performance", "key": "Tp1G", "temp_c": 66.0},
        {"group": "CPU performance", "key": "Tp1H", "temp_c": 70.2},
        {"group": "CPU performance", "key": "Tp1I", "temp_c": 52.3},
        {"group": "CPU performance", "key": "Tp1J", "temp_c": 57.5},
        {"group": "CPU performance", "key": "Tp1K", "temp_c": 70.4},
        {"group": "CPU performance", "key": "Tp1L", "temp_c": 66.7},
        {"group": "CPU performance", "key": "Tp1M", "temp_c": 59.8},
        {"group": "CPU performance", "key": "Tp1N", "temp_c": 69.9},
        {"group": "CPU performance", "key": "Tp1O", "temp_c": 63.8},
        {"group": "CPU performance", "key": "Tp1P", "temp_c": 67.5},
        {"group": "CPU performance", "key": "Tp1Q", "temp_c": 57.6},
        {"group": "CPU performance", "key": "Tp1R", "temp_c": 55.6},
        {"group": "CPU performance", "key": "Tp1S", "temp_c": 60.4},
        {"group": "CPU performance", "key": "Tp1T", "temp_c": 54.6},
        {"group": "CPU performance", "key": "Tp1U", "temp_c": 59.3},
        {"group": "CPU performance", "key": "Tp1V", "temp_c": 70.3},
        {"group": "CPU performance", "key": "Tp1W", "temp_c": 58.3},
        {"group": "CPU performance", "key": "Tp1X", "temp_c": 52.2},
        {"group": "CPU performance", "key": "Tp1Y", "temp_c": 52.9},
        {"group": "CPU performance", "key": "Tp1Z", "temp_c": 55.2},
        {"group": "CPU performance", "key": "Tp1a", "temp_c": 66.9},
        {"group": "CPU performance", "key": "Tp1b", "temp_c": 58.9},
        {"group": "CPU efficiency", "key": "Te05", "temp_c": 55.4},
        {"group": "CPU efficiency", "key": "Te0L", "temp_c": 57.0},
        {"group": "CPU efficiency", "key": "Te0P", "temp_c": 58.6},
        {"group": "CPU efficiency", "key": "Te0S", "temp_c": 56.2},
        {"group": "GPU", "key": "Tg0G", "temp_c": 52.2},
        {"group": "GPU", "key": "Tg0H", "temp_c": 54.0},
        {"group": "GPU", "key": "Tg1U", "temp_c": 55.8},
        {"group": "GPU", "key": "Tg1k", "temp_c": 53.1},
        {"group": "Battery", "key": "TB0T", "temp_c": 30.4},
        {"group": "Battery", "key": "TB1T", "temp_c": 31.0},
        {"group": "Battery", "key": "TB2T", "temp_c": 31.6},
        {"group": "Airflow", "key": "TaLP", "temp_c": 36.8},
        {"group": "Airflow", "key": "TaRF", "temp_c": 38.0},
        {"group": "Palm rest", "key": "Ts0P", "temp_c": 29.2},
        {"group": "Palm rest", "key": "Ts1P", "temp_c": 30.0},
        {"group": "Wireless", "key": "TW0P", "temp_c": 41},
        {"group": "Other", "key": "TC00", "temp_c": 38.8},
        {"group": "Other", "key": "TC01", "temp_c": 33.6},
        {"group": "Other", "key": "TC02", "temp_c": 57.5},
        {"group": "Other", "key": "TC03", "temp_c": 42.4},
        {"group": "Other", "key": "TC04", "temp_c": 36.6},
        {"group": "Other", "key": "TC05", "temp_c": 32.6},
        {"group": "Other", "key": "TC06", "temp_c": 32.5},
        {"group": "Other", "key": "TC07", "temp_c": 35.6},
        {"group": "Other", "key": "TC08", "temp_c": 49.3},
        {"group": "Other", "key": "TC09", "temp_c": 35.0},
        {"group": "Other", "key": "TC0a", "temp_c": 32.1},
        {"group": "Other", "key": "TC0b", "temp_c": 44.2},
        {"group": "Other", "key": "TC10", "temp_c": 37.7},
        {"group": "Other", "key": "TC11", "temp_c": 57.9},
        {"group": "Other", "key": "TC12", "temp_c": 34.3},
        {"group": "Other", "key": "TC13", "temp_c": 45.3},
        {"group": "Other", "key": "TC14", "temp_c": 51.9},
        {"group": "Other", "key": "TC15", "temp_c": 42.1},
        {"group": "Other", "key": "TC16", "temp_c": 57.7},
        {"group": "Other", "key": "TC17", "temp_c": 43.9},
        {"group": "Other", "key": "TC18", "temp_c": 37.5},
        {"group": "Other", "key": "TC19", "temp_c": 42.1},
        {"group": "Other", "key": "TC1a", "temp_c": 32.0},
        {"group": "Other", "key": "TC1b", "temp_c": 42.4},
        {"group": "Other", "key": "TD00", "temp_c": 37.7},
        {"group": "Other", "key": "TD01", "temp_c": 55.0},
        {"group": "Other", "key": "TD02", "temp_c": 53.4},
        {"group": "Other", "key": "TD03", "temp_c": 44.5},
        {"group": "Other", "key": "TD04", "temp_c": 31.9},
        {"group": "Other", "key": "TD05", "temp_c": 37.9},
        {"group": "Other", "key": "TD06", "temp_c": 37.5},
        {"group": "Other", "key": "TD07", "temp_c": 36.6},
        {"group": "Other", "key": "TD08", "temp_c": 37.2},
        {"group": "Other", "key": "TD09", "temp_c": 54.5},
        {"group": "Other", "key": "TD0a", "temp_c": 34.8},
        {"group": "Other", "key": "TD0b", "temp_c": 32.4},
        {"group": "Other", "key": "TD10", "temp_c": 56.1},
        {"group": "Other", "key": "TD11", "temp_c": 46.3},
        {"group": "Other", "key": "TD12", "temp_c": 57.7},
        {"group": "Other", "key": "TD13", "temp_c": 41.9},
        {"group": "Other", "key": "TD14", "temp_c": 55.3},
        {"group": "Other", "key": "TD15", "temp_c": 48.7},
        {"group": "Other", "key": "TD16", "temp_c": 52.4},
        {"group": "Other", "key": "TD17", "temp_c": 51.1},
        {"group": "Other", "key": "TD18", "temp_c": 44.3},
        {"group": "Other", "key": "TD19", "temp_c": 33.5},
        {"group": "Other", "key": "TD1a", "temp_c": 36.7},
        {"group": "Other", "key": "TD1b", "temp_c": 54.6},
        {"group": "Other", "key": "TH00", "temp_c": 55.3},
        {"group": "Other", "key": "TH01", "temp_c": 56.0},
        {"group": "Other", "key": "TH02", "temp_c": 40.1},
        {"group": "Other", "key": "TH03", "temp_c": 48.7},
        {"group": "Other", "key": "TH04", "temp_c": 52.6},
        {"group": "Other", "key": "TH05", "temp_c": 48.3},
        {"group": "Other", "key": "TH06", "temp_c": 53.0},
        {"group": "Other", "key": "TH07", "temp_c": 45.3},
        {"group": "Other", "key": "TH08", "temp_c": 48.7},
        {"group": "Other", "key": "TH09", "temp_c": 49.5},
        {"group": "Other", "key": "TH0a", "temp_c": 38.2},
        {"group": "Other", "key": "TH0b", "temp_c": 55.9},
        {"group": "Other", "key": "TH10", "temp_c": 56.8},
        {"group": "Other", "key": "TH11", "temp_c": 33.0},
        {"group": "Other", "key": "TH12", "temp_c": 57.2},
        {"group": "Other", "key": "TH13", "temp_c": 57.0},
        {"group": "Other", "key": "TH14", "temp_c": 49.0},
        {"group": "Other", "key": "TH15", "temp_c": 32.2},
        {"group": "Other", "key": "TH16", "temp_c": 55.3},
        {"group": "Other", "key": "TH17", "temp_c": 34.4},
        {"group": "Other", "key": "TH18", "temp_c": 57.2},
        {"group": "Other", "key": "TH19", "temp_c": 49.0},
        {"group": "Other", "key": "TH1a", "temp_c": 32.6},
        {"group": "Other", "key": "TH1b", "temp_c": 35.5},
        {"group": "Other", "key": "TM00", "temp_c": 48.2},
        {"group": "Other", "key": "TM01", "temp_c": 46.4},
        {"group": "Other", "key": "TM02", "temp_c": 51.2},
        {"group": "Other", "key": "TM03", "temp_c": 56.0},
        {"group": "Other", "key": "TM04", "temp_c": 36.9},
        {"group": "Other", "key": "TM05", "temp_c": 31.1},
        {"group": "Other", "key": "TM06", "temp_c": 55.9},
        {"group": "Other", "key": "TM07", "temp_c": 31.4},
        {"group": "Other", "key": "TM08", "temp_c": 54.7},
        {"group": "Other", "key": "TM09", "temp_c": 34.1},
        {"group": "Other", "key": "TM0a", "temp_c": 52.9},
        {"group": "Other", "key": "TM0b", "temp_c": 52.1},
        {"group": "Other", "key": "TM10", "temp_c": 54.7},
        {"group": "Other", "key": "TM11", "temp_c": 45.9},
        {"group": "Other", "key": "TM12", "temp_c": 54.7},
        {"group": "Other", "key": "TM13", "temp_c": 36.4},
        {"group": "Other", "key": "TM14", "temp_c": 49.1},
        {"group": "Other", "key": "TM15", "temp_c": 39.9},
        {"group": "Other", "key": "TM16", "temp_c": 55.1},
        {"group": "Other", "key": "TM17", "temp_c": 51.9},
        {"group": "Other", "key": "TM18", "temp_c": 43.7},
        {"group": "Other", "key": "TM19", "temp_c": 45.2},
        {"group": "Other", "key": "TM1a", "temp_c": 31.7},
        {"group": "Other", "key": "TM1b", "temp_c": 31.9},
        {"group": "Other", "key": "TP00", "temp_c": 47.1},
        {"group": "Other", "key": "TP01", "temp_c": 44.2},
        {"group": "Other", "key": "TP02", "temp_c": 54.3},
        {"group": "Other", "key": "TP03", "temp_c": 47.4},
        {"group": "Other", "key": "TP04", "temp_c": 34.7},
        {"group": "Other", "key": "TP05", "temp_c": 40.8},
        {"group": "Other", "key": "TP06", "temp_c": 51.7},
        {"group": "Other", "key": "TP07", "temp_c": 45.1},
        {"group": "Other", "key": "TP08", "temp_c": 31.3},
        {"group": "Other", "key": "TP09", "temp_c": 53.6},
        {"group": "Other", "key": "TP0a", "temp_c": 53.3},
        {"group": "Other", "key": "TP0b", "temp_c": 33.3},
        {"group": "Other", "key": "TP10", "temp_c": 45.7},
        {"group": "Other", "key": "TP11", "temp_c": 41.3}
      ],
      "bluetooth": [
        {"name": "Alex’s AirPods Pro", "kind": "Headphones", "levels": [{"part": "left", "percent": 99}, {"part": "right", "percent": 95}, {"part": "case", "percent": 64}]},
        {"name": "Magic Trackpad", "kind": "Trackpad", "levels": [{"part": "main", "percent": 18}]},
        {"name": "MX Keys", "kind": "Keyboard", "levels": [{"part": "main", "percent": 70}]}
      ]
    },
    "net_info": {
      "interfaces": [
        {"name": "en0", "mac": "00:00:5e:00:53:aa", "ipv4": ["192.0.2.5"], "ipv6": ["fe80::a1", "2001:db8:9640:4f00::77c1"], "primary": true},
        {"name": "en7", "mac": "00:00:5e:00:53:11", "ipv4": ["192.0.2.114"], "ipv6": [], "primary": false}
      ],
      "router": "192.0.2.1",
      "dns": ["192.0.2.1", "198.51.100.53"],
      "wifi": {"interface": "en0", "rssi": -52, "noise": -91, "channel": 44, "band_ghz": 5.0, "width_mhz": 80, "tx_rate_mbps": 864.0, "phy": "802.11ax", "security": "WPA3 Personal"}
    },
    "sleep_blockers": [
      {
        "pid": 45640,
        "app": "caffeinate",
        "kind": "PreventUserIdleSystemSleep",
        "name": "caffeinate command-line tool"
      },
      {
        "pid": 4101,
        "app": "Google Chrome",
        "kind": "PreventUserIdleDisplaySleep",
        "name": "Video Wake Lock"
      },
      {
        "pid": 325,
        "app": "powerd",
        "kind": "PreventUserIdleSystemSleep",
        "name": "Powerd - Prevent sleep while display is on"
      }
    ],
    "spark": {
      "cpu": [32.2, 31.4, 41.4, 34.0, 43.1, 42.0, 38.5, 46.9, 40.3, 47.4, 42.0, 42.5, 47.6, 53.7, 41.7, 42.3, 47.6, 51.3, 43.8, 39.1, 46.5, 29.7, 40.7, 29.6, 25.3, 23.1, 24.3, 30.9, 19.3, 24.5, 24.5, 19.5, 21.9, 14.0, 14.1, 16.9, 25.2, 22.1, 21.5, 27.2, 26.7, 26.0, 35.8, 36.2, 30.9, 38.2, 39.4, 46.9, 46.3, 40.8, 53.3, 40.7, 46.4, 52.6, 43.3, 48.8, 41.5, 51.1, 51.9, 47.9],
      "memory": [75.3, 75.1, 75.6, 75.7, 75.9, 75.9, 76.4, 76.6, 76.3, 76.6, 76.1, 76.7, 76.6, 76.8, 76.6, 76.1, 76.0, 76.1, 75.4, 75.6, 75.2, 74.9, 74.6, 75.0, 74.3, 74.2, 74.1, 74.3, 73.5, 73.7, 73.7, 73.9, 73.8, 73.8, 73.3, 73.5, 73.5, 74.0, 74.2, 73.7, 73.9, 74.2, 74.4, 74.8, 75.1, 75.0, 75.0, 75.6, 75.7, 76.0, 76.5, 76.4, 76.4, 76.5, 76.6, 76.1, 76.8, 76.7, 76.7, 76.5],
      "gpu": [37.0, 39.0, 38.0, 47.0, 42.0, 45.0, 48.0, 49.0, 52.0, 50.0, 50.0, 52.0, 51.0, 54.0, 49.0, 58.0, 53.0, 46.0, 45.0, 44.0, 41.0, 36.0, 42.0, 41.0, 32.0, 30.0, 23.0, 21.0, 22.0, 20.0, 26.0, 17.0, 14.0, 25.0, 21.0, 16.0, 22.0, 17.0, 25.0, 32.0, 33.0, 33.0, 30.0, 34.0, 34.0, 44.0, 44.0, 49.0, 46.0, 47.0, 55.0, 59.0, 59.0, 59.0, 60.0, 59.0, 53.0, 55.0, 53.0, 47.0],
      "temp": [61.1, 62.1, 62.6, 64.0, 65.1, 64.5, 65.9, 66.3, 66.5, 65.6, 65.4, 65.5, 65.4, 65.2, 65.9, 66.2, 65.7, 64.6, 64.5, 64.3, 62.3, 62.9, 62.8, 62.0, 61.4, 60.3, 59.2, 60.0, 58.6, 59.2, 59.3, 58.0, 57.8, 58.9, 58.5, 57.5, 57.6, 57.9, 59.8, 60.0, 59.1, 61.0, 61.8, 61.8, 61.7, 62.7, 62.4, 62.7, 65.1, 64.9, 65.1, 66.2, 65.5, 66.6, 66.6, 65.4, 65.5, 65.4, 65.1, 65.5],
      "power": [10.3, 11.2, 10.7, 13.5, 12.2, 12.8, 13.5, 14.7, 13.5, 15.1, 14.0, 14.1, 14.0, 12.4, 13.5, 12.6, 11.8, 13.9, 11.6, 12.2, 12.5, 11.6, 10.5, 10.6, 10.3, 10.6, 8.2, 9.2, 8.0, 7.8, 9.1, 8.1, 8.2, 8.8, 9.3, 8.0, 8.6, 8.5, 8.8, 9.6, 9.2, 9.9, 10.1, 11.9, 11.6, 12.6, 13.2, 11.5, 12.8, 14.3, 14.3, 12.4, 12.6, 13.7, 12.7, 13.2, 12.7, 14.4, 14.6, 14.7],
      "disk_read": [1045983.0, 1555292.0, 1681840.0, 1518272.0, 2117320.0, 2301140.0, 1957973.0, 2506891.0, 2245201.0, 2357344.0, 2701056.0, 2615979.0, 2191876.0, 2322687.0, 2316693.0, 2129236.0, 1942369.0, 1901280.0, 2017370.0, 1441069.0, 1615187.0, 1384481.0, 960742.0, 989104.0, 1007946.0, 785164.0, 365549.0, 799636.0, 562540.0, 576733.0, 0, 6965.0, 0, 276276.0, 0, 0, 162318.0, 542062.0, 584217.0, 356598.0, 421502.0, 1039150.0, 979377.0, 1220099.0, 1008974.0, 1153539.0, 1701809.0, 1694392.0, 1621850.0, 2285404.0, 2214765.0, 2415766.0, 2053170.0, 2584854.0, 2134221.0, 2634608.0, 2371049.0, 2265107.0, 2338491.0, 2488647.0],
      "disk_write": [352549.0, 373878.0, 525885.0, 507025.0, 524068.0, 581302.0, 593266.0, 663740.0, 717426.0, 735093.0, 858565.0, 747101.0, 794743.0, 703717.0, 725825.0, 618351.0, 642113.0, 545023.0, 676917.0, 583405.0, 442479.0, 458310.0, 515926.0, 256953.0, 377835.0, 230777.0, 197508.0, 236559.0, 88838.0, 83544.0, 101505.0, 154612.0, 0, 102143.0, 75354.0, 69881.0, 32558.0, 45234.0, 6567.0, 64465.0, 94480.0, 307989.0, 240809.0, 272414.0, 308467.0, 549844.0, 611278.0, 614115.0, 567284.0, 601893.0, 653550.0, 727599.0, 679873.0, 769976.0, 736744.0, 912207.0, 910731.0, 794165.0, 700210.0, 850676.0],
      "net_down": [3435988.0, 3867005.0, 3795764.0, 4613955.0, 5056623.0, 5390660.0, 5284324.0, 5881701.0, 5446745.0, 5895861.0, 5760876.0, 6167975.0, 5701316.0, 5597958.0, 5813140.0, 5545765.0, 5754753.0, 5420626.0, 5392855.0, 4949986.0, 4671082.0, 4505780.0, 3530939.0, 3079871.0, 3523637.0, 2146866.0, 2524907.0, 2126254.0, 1125510.0, 1874365.0, 1765215.0, 1309917.0, 1361934.0, 1432235.0, 632774.0, 1185447.0, 1293100.0, 1879311.0, 2067006.0, 2358875.0, 2361127.0, 3069362.0, 3162287.0, 3540919.0, 3344797.0, 3473686.0, 3964353.0, 4594277.0, 4607874.0, 5804924.0, 5728012.0, 6036457.0, 6212913.0, 6409508.0, 6252764.0, 5681116.0, 6629370.0, 6487905.0, 6054569.0, 5913746.0],
      "net_up": [122427.0, 94725.0, 144505.0, 122948.0, 119723.0, 138484.0, 173177.0, 146229.0, 183275.0, 200829.0, 173088.0, 166865.0, 172159.0, 182865.0, 184910.0, 171485.0, 167808.0, 126869.0, 124141.0, 122984.0, 144825.0, 109294.0, 116726.0, 73862.0, 68256.0, 72830.0, 89914.0, 84141.0, 76938.0, 48048.0, 57724.0, 51487.0, 49741.0, 27761.0, 76029.0, 35246.0, 86181.0, 87828.0, 36650.0, 69989.0, 99196.0, 116009.0, 92367.0, 89833.0, 94968.0, 148918.0, 112331.0, 143320.0, 123988.0, 154484.0, 187005.0, 141851.0, 188278.0, 172181.0, 197250.0, 186573.0, 156922.0, 195969.0, 167591.0, 135020.0]
    },
    "apps": {
      "has_rates": true,
      "items": [
        {
          "name": "Google Chrome",
          "icon": "/Applications/Google Chrome.app",
          "pid_count": 14,
          "cpu": 181.5,
          "memory": 4349493248,
          "system": false,
          "killable": true,
          "disk_read_rate": 262144.0,
          "disk_write_rate": 917504.0,
          "energy_mw": 2140.0,
          "procs": [
            {"pid": 4101, "name": "Google Chrome", "cpu": 38.2, "memory": 641728512, "system": false, "killable": true, "kind": "browser"},
            {"pid": 4117, "name": "Google Chrome Helper (GPU)", "cpu": 41.5, "memory": 406847488, "system": false, "killable": true, "kind": "gpu"},
            {"pid": 4120, "name": "Google Chrome Helper", "cpu": 2.1, "memory": 100663296, "system": false, "killable": true, "kind": "utility"},
            {"pid": 4130, "name": "Google Chrome Helper (Renderer)", "cpu": 61.0, "memory": 778043392, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4131, "name": "Google Chrome Helper (Renderer)", "cpu": 22.4, "memory": 543162368, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4132, "name": "Google Chrome Helper (Renderer)", "cpu": 9.8, "memory": 315621376, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4133, "name": "Google Chrome Helper (Renderer)", "cpu": 4.4, "memory": 301989888, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4134, "name": "Google Chrome Helper (Renderer)", "cpu": 1.2, "memory": 251658240, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4135, "name": "Google Chrome Helper (Renderer)", "cpu": 0.6, "memory": 224395264, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4136, "name": "Google Chrome Helper (Renderer)", "cpu": 0.3, "memory": 207618048, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4137, "name": "Google Chrome Helper (Renderer)", "cpu": 0.0, "memory": 179306496, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4138, "name": "Google Chrome Helper (Renderer)", "cpu": 0.0, "memory": 174063616, "system": false, "killable": true, "kind": "tab"},
            {"pid": 4139, "name": "Google Chrome Helper (Renderer)", "cpu": 0.0, "memory": 125829120, "system": false, "killable": true, "kind": "extension"},
            {"pid": 4140, "name": "Google Chrome Helper (Renderer)", "cpu": 0.0, "memory": 98566144, "system": false, "killable": true, "kind": "extension"}
          ]
        },
        {
          "name": "GoLand",
          "icon": "/Applications/GoLand.app",
          "pid_count": 4,
          "cpu": 46.9,
          "memory": 4830789632,
          "system": false,
          "killable": true,
          "disk_read_rate": 1048576.0,
          "disk_write_rate": 196608.0,
          "energy_mw": 960.0,
          "procs": [
            {"pid": 2210, "name": "goland", "cpu": 34.6, "memory": 3577741312, "system": false, "killable": true, "kind": ""},
            {"pid": 2388, "name": "fsnotifier", "cpu": 0.0, "memory": 3145728, "system": false, "killable": true, "kind": ""},
            {"pid": 2391, "name": "gopls", "cpu": 12.3, "memory": 1245708288, "system": false, "killable": true, "kind": ""},
            {"pid": 9133, "name": "zsh", "cpu": 0.0, "memory": 4194304, "system": true, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "Docker",
          "icon": "/Applications/Docker.app",
          "pid_count": 3,
          "cpu": 26.4,
          "memory": 4810866688,
          "system": false,
          "killable": true,
          "disk_read_rate": 81920.0,
          "disk_write_rate": 1258291.2,
          "energy_mw": 410.0,
          "procs": [
            {"pid": 1720, "name": "com.docker.backend", "cpu": 6.4, "memory": 190840832, "system": false, "killable": true, "kind": ""},
            {"pid": 1755, "name": "com.docker.virtualization", "cpu": 18.9, "memory": 4294967296, "system": false, "killable": true, "kind": ""},
            {"pid": 1702, "name": "Docker Desktop", "cpu": 1.1, "memory": 325058560, "system": false, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "Finder",
          "icon": "/System/Library/CoreServices/Finder.app",
          "pid_count": 1,
          "cpu": 19.1,
          "memory": 412090368,
          "system": true,
          "killable": true,
          "disk_read_rate": 40960.0,
          "disk_write_rate": 8192.0,
          "energy_mw": 96.0,
          "procs": [
            {"pid": 611, "name": "Finder", "cpu": 19.1, "memory": 412090368, "system": true, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "WindowServer",
          "icon": "",
          "pid_count": 1,
          "cpu": 17.3,
          "memory": 1300234240,
          "system": true,
          "killable": false,
          "disk_read_rate": null,
          "disk_write_rate": null,
          "energy_mw": null,
          "procs": [
            {"pid": 382, "name": "WindowServer", "cpu": 17.3, "memory": 1300234240, "system": true, "killable": false, "kind": ""}
          ]
        },
        {
          "name": "iTerm",
          "icon": "/Applications/iTerm.app",
          "pid_count": 3,
          "cpu": 6.6,
          "memory": 447741952,
          "system": false,
          "killable": true,
          "disk_read_rate": 0.0,
          "disk_write_rate": 4096.0,
          "energy_mw": 85.0,
          "procs": [
            {"pid": 3051, "name": "iTerm2", "cpu": 5.2, "memory": 432013312, "system": false, "killable": true, "kind": ""},
            {"pid": 3090, "name": "zsh", "cpu": 0.0, "memory": 6291456, "system": true, "killable": true, "kind": ""},
            {"pid": 3412, "name": "htop", "cpu": 1.4, "memory": 9437184, "system": false, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "kernel_task",
          "icon": "",
          "pid_count": 1,
          "cpu": 4.8,
          "memory": 0,
          "system": true,
          "killable": false,
          "disk_read_rate": null,
          "disk_write_rate": null,
          "energy_mw": null,
          "procs": [
            {"pid": 0, "name": "kernel_task", "cpu": 4.8, "memory": 0, "system": true, "killable": false, "kind": ""}
          ]
        },
        {
          "name": "Safari",
          "icon": "/Applications/Safari.app",
          "pid_count": 5,
          "cpu": 3.4,
          "memory": 813694976,
          "system": false,
          "killable": true,
          "disk_read_rate": 0.0,
          "disk_write_rate": 0.0,
          "energy_mw": 31.0,
          "procs": [
            {"pid": 5101, "name": "Safari", "cpu": 0.9, "memory": 281018368, "system": false, "killable": true, "kind": "browser"},
            {"pid": 5105, "name": "com.apple.WebKit.WebContent", "cpu": 1.8, "memory": 262144000, "system": false, "killable": true, "kind": "tab"},
            {"pid": 5106, "name": "com.apple.WebKit.WebContent", "cpu": 0.4, "memory": 162529280, "system": false, "killable": true, "kind": "tab"},
            {"pid": 5110, "name": "com.apple.WebKit.Networking", "cpu": 0.3, "memory": 44040192, "system": false, "killable": true, "kind": "utility"},
            {"pid": 5111, "name": "com.apple.WebKit.GPU", "cpu": 0.0, "memory": 63963136, "system": false, "killable": true, "kind": "gpu"}
          ]
        },
        {
          "name": "Firefox",
          "icon": "/Applications/Firefox.app",
          "pid_count": 4,
          "cpu": 2.6,
          "memory": 734003200,
          "system": false,
          "killable": true,
          "disk_read_rate": 0.0,
          "disk_write_rate": 8192.0,
          "energy_mw": 24.0,
          "procs": [
            {"pid": 5301, "name": "firefox", "cpu": 1.2, "memory": 398458880, "system": false, "killable": true, "kind": "browser"},
            {"pid": 5310, "name": "plugin-container", "cpu": 1.1, "memory": 209715200, "system": false, "killable": true, "kind": "tab"},
            {"pid": 5311, "name": "plugin-container", "cpu": 0.3, "memory": 83886080, "system": false, "killable": true, "kind": "gpu"},
            {"pid": 5312, "name": "plugin-container", "cpu": 0.0, "memory": 41943040, "system": false, "killable": true, "kind": "utility"}
          ]
        },
        {
          "name": "Telegram",
          "icon": "/Applications/Telegram.app",
          "pid_count": 1,
          "cpu": 2.2,
          "memory": 509607936,
          "system": false,
          "killable": true,
          "disk_read_rate": 0.0,
          "disk_write_rate": 12288.0,
          "energy_mw": 22.0,
          "procs": [
            {"pid": 2950, "name": "Telegram", "cpu": 2.2, "memory": 509607936, "system": false, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "mac-pulse",
          "icon": "self",
          "pid_count": 2,
          "cpu": 1.1,
          "memory": 94371840,
          "system": false,
          "killable": true,
          "disk_read_rate": 0.0,
          "disk_write_rate": 2048.0,
          "energy_mw": 14.0,
          "procs": [
            {"pid": 45511, "name": "mac-pulse", "cpu": 1.1, "memory": 93323264, "system": false, "killable": false, "kind": ""},
            {"pid": 45640, "name": "nettop", "cpu": 0.0, "memory": 1048576, "system": true, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "com.apple.WebKit.WebContent.Helper.Extra.Long.Process.Name",
          "icon": "",
          "pid_count": 1,
          "cpu": 0.8,
          "memory": 161480704,
          "system": true,
          "killable": true,
          "disk_read_rate": null,
          "disk_write_rate": null,
          "energy_mw": null,
          "procs": [
            {"pid": 5120, "name": "com.apple.WebKit.WebContent.Helper.Extra.Long.Process.Name", "cpu": 0.8, "memory": 161480704, "system": true, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "Notes",
          "icon": "/System/Applications/Notes.app",
          "pid_count": 1,
          "cpu": 0.6,
          "memory": 157286400,
          "system": false,
          "killable": true,
          "disk_read_rate": 0.0,
          "disk_write_rate": 0.0,
          "energy_mw": 2.0,
          "procs": [
            {"pid": 5401, "name": "Notes", "cpu": 0.6, "memory": 157286400, "system": false, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "Music",
          "icon": "/System/Applications/Music.app",
          "pid_count": 1,
          "cpu": 0.4,
          "memory": 115343360,
          "system": false,
          "killable": true,
          "disk_read_rate": 0.0,
          "disk_write_rate": 0.0,
          "energy_mw": 1.5,
          "procs": [
            {"pid": 5402, "name": "Music", "cpu": 0.4, "memory": 115343360, "system": false, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "Preview",
          "icon": "/System/Applications/Preview.app",
          "pid_count": 1,
          "cpu": 0.2,
          "memory": 52428800,
          "system": false,
          "killable": true,
          "disk_read_rate": 0.0,
          "disk_write_rate": 0.0,
          "energy_mw": 0.5,
          "procs": [
            {"pid": 5403, "name": "Preview", "cpu": 0.2, "memory": 52428800, "system": false, "killable": true, "kind": ""}
          ]
        },
        {
          "name": "mDNSResponder",
          "icon": "",
          "pid_count": 1,
          "cpu": 0.1,
          "memory": 12582912,
          "system": true,
          "killable": false,
          "disk_read_rate": null,
          "disk_write_rate": null,
          "energy_mw": null,
          "procs": [
            {"pid": 509, "name": "mDNSResponder", "cpu": 0.1, "memory": 12582912, "system": true, "killable": false, "kind": ""}
          ]
        }
      ]
    },
    "net": {
      "has_rates": true,
      "apps": [
        {
          "name": "Google Chrome",
          "icon": "/Applications/Google Chrome.app",
          "system": false,
          "pid_count": 14,
          "down_rate": 4026531.84,
          "up_rate": 121344.0,
          "down_total": 524288000,
          "up_total": 23068672,
          "connections": 2
        },
        {
          "name": "Docker",
          "icon": "/Applications/Docker.app",
          "system": false,
          "pid_count": 3,
          "down_rate": 419840.0,
          "up_rate": 3174.4,
          "down_total": 1610612736,
          "up_total": 9437184,
          "connections": 1
        },
        {
          "name": "Telegram",
          "icon": "/Applications/Telegram.app",
          "system": false,
          "pid_count": 1,
          "down_rate": 12697.6,
          "up_rate": 2252.8,
          "down_total": 25165824,
          "up_total": 6291456,
          "connections": 1
        },
        {
          "name": "GoLand",
          "icon": "/Applications/GoLand.app",
          "system": false,
          "pid_count": 4,
          "down_rate": 1536.0,
          "up_rate": 409.6,
          "down_total": 4194304,
          "up_total": 2097152,
          "connections": 2
        },
        {
          "name": "mDNSResponder",
          "icon": "",
          "system": true,
          "pid_count": 1,
          "down_rate": 310.0,
          "up_rate": 180.0,
          "down_total": 0,
          "up_total": 0,
          "connections": 1
        },
        {
          "name": "apsd",
          "icon": "",
          "system": true,
          "pid_count": 1,
          "down_rate": 0.0,
          "up_rate": 120.0,
          "down_total": 47666,
          "up_total": 555987,
          "connections": 1
        },
        {
          "name": "Safari",
          "icon": "/Applications/Safari.app",
          "system": false,
          "pid_count": 3,
          "down_rate": 0.0,
          "up_rate": 0.0,
          "down_total": 2097152,
          "up_total": 307200,
          "connections": 1
        },
        {
          "name": "iTerm",
          "icon": "/Applications/iTerm.app",
          "system": false,
          "pid_count": 3,
          "down_rate": 0.0,
          "up_rate": 0.0,
          "down_total": 0,
          "up_total": 64,
          "connections": 1
        }
      ],
      "connections": [
        {
          "app": "Google Chrome",
          "pid": 4120,
          "proto": "tcp4",
          "local": "192.0.2.5:62832",
          "remote_ip": "198.51.100.78",
          "remote_port": "443",
          "host": "edge-78.example.net",
          "state": "Established",
          "bytes_in": 432013312,
          "bytes_out": 18874368,
          "down_rate": 3355443.2,
          "up_rate": 98304.0
        },
        {
          "app": "Google Chrome",
          "pid": 4120,
          "proto": "quic4",
          "local": "192.0.2.5:49906",
          "remote_ip": "198.51.100.4",
          "remote_port": "443",
          "host": "",
          "state": "",
          "bytes_in": 92274688,
          "bytes_out": 4194304,
          "down_rate": 655360.0,
          "up_rate": 23040.0
        },
        {
          "app": "Docker",
          "pid": 1720,
          "proto": "tcp4",
          "local": "192.0.2.5:50211",
          "remote_ip": "203.0.113.91",
          "remote_port": "443",
          "host": "server-203-0-113-91.cdn.example.net",
          "state": "Established",
          "bytes_in": 1610612736,
          "bytes_out": 9437184,
          "down_rate": 419840.0,
          "up_rate": 3174.4
        },
        {
          "app": "Telegram",
          "pid": 2950,
          "proto": "tcp4",
          "local": "192.0.2.5:51877",
          "remote_ip": "203.0.113.51",
          "remote_port": "443",
          "host": "",
          "state": "Established",
          "bytes_in": 25165824,
          "bytes_out": 6291456,
          "down_rate": 12697.6,
          "up_rate": 2252.8
        },
        {
          "app": "GoLand",
          "pid": 2210,
          "proto": "tcp4",
          "local": "192.0.2.5:52010",
          "remote_ip": "203.0.113.33",
          "remote_port": "443",
          "host": "server-203-0-113-33.cdn.example.net",
          "state": "Established",
          "bytes_in": 3145728,
          "bytes_out": 524288,
          "down_rate": 1536.0,
          "up_rate": 409.6
        },
        {
          "app": "GoLand",
          "pid": 2391,
          "proto": "tcp4",
          "local": "127.0.0.1:53100",
          "remote_ip": "127.0.0.1",
          "remote_port": "63342",
          "host": "",
          "state": "Established",
          "bytes_in": 942080,
          "bytes_out": 1126400,
          "down_rate": 0.0,
          "up_rate": 0.0
        },
        {
          "app": "identityservicesd",
          "pid": 616,
          "proto": "tcp6",
          "local": "fe80::c1%utun4:1046",
          "remote_ip": "fe80::c2%utun4",
          "remote_port": "1025",
          "host": "",
          "state": "Established",
          "bytes_in": 85251,
          "bytes_out": 18652,
          "down_rate": 0.0,
          "up_rate": 0.0
        },
        {
          "app": "apsd",
          "pid": 353,
          "proto": "tcp4",
          "local": "192.0.2.5:62101",
          "remote_ip": "198.51.100.133",
          "remote_port": "5223",
          "host": "",
          "state": "Established",
          "bytes_in": 47666,
          "bytes_out": 555987,
          "down_rate": 0.0,
          "up_rate": 120.0
        },
        {
          "app": "mDNSResponder",
          "pid": 509,
          "proto": "udp6",
          "local": "*:5353",
          "remote_ip": "*",
          "remote_port": "*",
          "host": "",
          "state": "",
          "bytes_in": 21811105,
          "bytes_out": 12699922,
          "down_rate": 310.0,
          "up_rate": 180.0
        },
        {
          "app": "Safari",
          "pid": 5110,
          "proto": "tcp6",
          "local": "2001:db8:b6c0:1d00::a9c1:55120",
          "remote_ip": "2001:db8:4001:82b::200e",
          "remote_port": "443",
          "host": "edge-200e.example.net",
          "state": "Established",
          "bytes_in": 2097152,
          "bytes_out": 307200,
          "down_rate": 0.0,
          "up_rate": 0.0
        },
        {
          "app": "iTerm",
          "pid": 3412,
          "proto": "tcp4",
          "local": "192.0.2.5:55402",
          "remote_ip": "203.0.113.4",
          "remote_port": "22",
          "host": "lb-203-0-113-4.example.com",
          "state": "SynSent",
          "bytes_in": 0,
          "bytes_out": 64,
          "down_rate": 0.0,
          "up_rate": 0.0
        },
        {
          "app": "rapportd",
          "pid": 702,
          "proto": "udp4",
          "local": "192.0.2.5:3722",
          "remote_ip": "*",
          "remote_port": "*",
          "host": "",
          "state": "",
          "bytes_in": 1840,
          "bytes_out": 0,
          "down_rate": 0.0,
          "up_rate": 0.0
        }
      ],
      "listening": [
        {
          "app": "ControlCenter",
          "pid": 640,
          "proto": "tcp",
          "addr": "*",
          "port": "7000",
          "dir": ""
        },
        {
          "app": "Docker",
          "pid": 1720,
          "proto": "tcp",
          "addr": "127.0.0.1",
          "port": "49153",
          "dir": ""
        },
        {
          "app": "GoLand",
          "pid": 2210,
          "proto": "tcp",
          "addr": "127.0.0.1",
          "port": "63342",
          "dir": "~/go/src/github.com/kl09/mac-pulse"
        },
        {
          "app": "mDNSResponder",
          "pid": 509,
          "proto": "udp",
          "addr": "*",
          "port": "5353",
          "dir": ""
        },
        {
          "app": "node",
          "pid": 51200,
          "proto": "tcp",
          "addr": "127.0.0.1",
          "port": "3000",
          "dir": "~/work/storefront"
        },
        {
          "app": "main",
          "pid": 52110,
          "proto": "tcp",
          "addr": "*",
          "port": "8080",
          "dir": "~/src/billing-api/cmd/server"
        }
      ],
      "talkers": [
        {
          "ip": "198.51.100.78",
          "host": "edge-78.example.net",
          "apps": [
            "Google Chrome"
          ],
          "down_rate": 3355443.2,
          "up_rate": 98304.0,
          "connections": 1
        },
        {
          "ip": "198.51.100.4",
          "host": "",
          "apps": [
            "Google Chrome"
          ],
          "down_rate": 655360.0,
          "up_rate": 23040.0,
          "connections": 1
        },
        {
          "ip": "203.0.113.91",
          "host": "server-203-0-113-91.cdn.example.net",
          "apps": [
            "Docker",
            "GoLand"
          ],
          "down_rate": 419840.0,
          "up_rate": 3174.4,
          "connections": 2
        },
        {
          "ip": "fe80::c2%utun4",
          "host": "",
          "apps": [
            "identityservicesd"
          ],
          "down_rate": 0.0,
          "up_rate": 0.0,
          "connections": 1
        }
      ],
      "today": [
        {
          "name": "Google Chrome",
          "icon": "/Applications/Google Chrome.app",
          "down": 4164943872,
          "up": 220200960
        },
        {
          "name": "Docker",
          "icon": "/Applications/Docker.app",
          "down": 1702887424,
          "up": 50331648
        },
        {
          "name": "Telegram",
          "icon": "/Applications/Telegram.app",
          "down": 325058560,
          "up": 100663296
        },
        {
          "name": "softwareupdated",
          "icon": "",
          "down": 293601280,
          "up": 2097152
        },
        {
          "name": "GoLand",
          "icon": "/Applications/GoLand.app",
          "down": 96468992,
          "up": 32505856
        }
      ]
    },
    "alerts": {
      "active": [
        {
          "id": "app_cpu:Google Chrome:1790776920",
          "kind": "app_cpu",
          "params": {"app": "Google Chrome", "value": 212, "minutes": 5},
          "app": "Google Chrome",
          "since": 1790776920000,
          "until": 0
        }
      ],
      "recent": [
        {
          "id": "temp::1790772000",
          "kind": "temp",
          "params": {"value": 97, "limit": 95, "unit": "°C"},
          "app": "",
          "since": 1790772000000,
          "until": 1790772300000
        },
        {
          "id": "disk::1790687400",
          "kind": "disk",
          "params": {"value": 9.4, "limit": 10},
          "app": "",
          "since": 1790687400000,
          "until": 1790691000000
        },
        {
          "id": "app_rulecpu:GoLand:1790683800",
          "kind": "app_rule",
          "params": {"app": "GoLand", "metric": "cpu", "value": 240, "limit": 200, "unit": "%", "minutes": 5},
          "app": "GoLand",
          "since": 1790683800000,
          "until": 1790684400000
        },
        {
          "id": "battery_low::1790680200",
          "kind": "battery_low",
          "params": {"value": 18, "limit": 20},
          "app": "",
          "since": 1790680200000,
          "until": 1790681100000
        }
      ]
    },
    "settings": {
      "temp_unit": "C",
      "net_unit": "bytes",
      "menu_bar": [
        "cpu",
        "mem",
        "net"
      ],
      "alerts": true,
      "alert_cpu": 90,
      "alert_temp": 95,
      "alert_disk_free": 10,
      "apps_show_system": false,
      "hotkey": false,
      "language": "system",
      "theme": "aqua",
      "appearance": "auto",
      "alert_battery": 20,
      "alert_memory": 0,
      "alert_swap": 0,
      "alert_hold": 60,
      "alert_muted": ["Docker Desktop"],
      "alert_rules": [
        {"app": "Google Chrome", "metric": "memory", "limit": 8, "minutes": 10}
      ],
      "menu_bar_compact": false,
      "menu_bar_graph": false,
      "window_on_top": false,
      "hotkey_quit": false,
      "clock": false,
      "clock_date": false,
      "clock_seconds": false,
      "clock_hours": "auto",
      "clock_zones": ["Asia/Tokyo", "America/New_York"],
      "menu_bar_separate": false,
      "show_in_dock": false,
      "tab_order": ["overview", "processes", "network", "history", "dev", "storage"],
      "tile_order": ["cpu", "memory", "network", "disk", "gpu", "battery", "sensors", "apps", "sleep"],
      "launch_at_login": true
    },
    "system_lang": "en",
    "pinned": false,
    "dev": {
      "docker": "ok",
      "containers": [
        {"id": "a1b2c3d4e5f6", "name": "shop-postgres-1", "image": "postgres:17-alpine", "state": "running", "status": "Up 7 days (healthy)", "ports": "0.0.0.0:5432->5432/tcp", "project": "shop", "dir": "~/src/shop", "cpu": 0.2, "memory": 62914560, "pids": 7},
        {"id": "9c1e02b7a5d4", "name": "shop-redis-1", "image": "redis:8-alpine", "state": "running", "status": "Up 7 days", "ports": "0.0.0.0:6379->6379/tcp", "project": "shop", "dir": "~/src/shop", "cpu": 0.4, "memory": 11534336, "pids": 6},
        {"id": "f20b6d83c1aa", "name": "scratch", "image": "alpine:3.22", "state": "running", "status": "Up 2 minutes", "ports": "", "project": "", "dir": "", "cpu": null, "memory": null, "pids": null}
      ],
      "projects": [
        {"name": "shop", "dir": "~/src/shop", "ports": [8080, 9090], "apps": ["shop", "node"], "pids": [7301, 7340], "cpu": 14.2, "memory": 412090368, "containers": 2},
        {"name": "docs", "dir": "~/work/docs", "ports": [8765], "apps": ["python3"], "pids": [8112], "cpu": 0.1, "memory": 28311552, "containers": 0}
      ],
      "agents": [
        {"name": "codex", "pid": 6120, "dir": "~/go/src/github.com/kl09/mac-pulse", "since": 1790770200000, "procs": 9, "cpu": 48.5, "memory": 1289748480, "energy_mw": 910.5},
        {"name": "codex", "pid": 6644, "dir": "~/work/docs", "since": 1790776500000, "procs": 2, "cpu": 0.3, "memory": 203423744, "energy_mw": null}
      ]
    },
    "media": {"mic": {"active": false, "apps": []}, "camera": {"active": false}},
    "storage": {
      "scan": {"state": "idle", "root": "", "files": 0, "bytes": 0, "denied": 0},
      "cleanup": {
        "state": "idle",
        "categories": [
          {"id": "app_caches", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "logs", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "xcode_derived", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "simulator_caches", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "npm", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "yarn", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "pnpm", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "go_build", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "pip", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "homebrew", "denied": false, "bytes": 0, "items": 0, "permanent": false},
          {"id": "trash", "denied": false, "bytes": 0, "items": 0, "permanent": true}
        ]
      }
    }
  },
  "history": {
    "metric": "network",
    "range": "24h",
    "start": 1790691000000,
    "step_s": 600,
    "series": [
      {
        "name": "down",
        "unit": "bytes_per_s",
        "points": [413277.0, 1029728.0, 1102064.0, 1059234.0, 996736.0, 1298455.0, 1352559.0, 1959330.0, 1156710.0, 1964488.0, 2079778.0, 1351722.0, 2169014.0, 1925580.0, 2077701.0, 1395162.0, 1409372.0, 1347786.0, 1874543.0, 1352411.0, 1008858.0, 963532.0, 690323.0, 341370.0, 281872.0, 922930.0, 258156.0, 830222.0, 44936.0, 0, 187070.0, 0, 0, 569515.0, 504457.0, 455499.0, 311173.0, 656735.0, 754939.0, 436937.0, null, null, null, null, null, null, null, null, null, null, null, null, null, null, null, null, null, null, 1724329.0, 1501082.0, 1198205.0, 1109425.0, 1062896.0, 1674910.0, 1146508.0, 748460.0, 1334502.0, 1310284.0, 636494.0, 209286.0, 160963.0, 0, 138437.0, 0, 0, 0, 181525.0, 498954.0, 366918.0, 47328.0, 89819.0, 259263.0, 178936.0, 222418.0, 33588.0, 357113.0, 1173602.0, 425967.0, 929221.0, 1175069.0, 1528028.0, 974947.0, 1133656.0, 1203780.0, 1440951.0, 1557722.0, 2133475.0, 2065943.0, 2114946.0, 1251121.0, 1252796.0, 1920804.0, 2069956.0, 1580748.0, 1626761.0, 942499.0, 1249204.0, 1694390.0, 1480915.0, 1397096.0, 1399996.0, 542273.0, 285413.0, 222543.0, 497103.0, 567502.0, 751013.0, 456539.0, 325023.0, 405179.0, 66254.0, 155157.0, 0, 426308.0, 0, 665643.0, 455389.0, 188745.0, 102827.0, 332663.0, 836246.0, 1014472.0, 530639.0, 604420.0, 1183465.0, 1352560.0, 1255262.0, 1179869.0, 1648432.0, 1112984.0, 1466243.0, 1669208.0, 2203341.0, 1888818.0]
      },
      {
        "name": "up",
        "unit": "bytes_per_s",
        "points": [85019.0, 65754.0, 56688.0, 62921.0, 111893.0, 100846.0, 80571.0, 66525.0, 98603.0, 111466.0, 97072.0, 87486.0, 112261.0, 126854.0, 81899.0, 67254.0, 82421.0, 83352.0, 94766.0, 59865.0, 91186.0, 81912.0, 61688.0, 37446.0, 78714.0, 32803.0, 58905.0, 18030.0, 13327.0, 42949.0, 11553.0, 49885.0, 20626.0, 1269.0, 3909.0, 17065.0, 34372.0, 54601.0, 8817.0, 28141.0, null, null, null, null, null, null, null, null, null, null, null, null, null, null, null, null, null, null, 90893.0, 85482.0, 71982.0, 57611.0, 69933.0, 69194.0, 100822.0, 43998.0, 89809.0, 37478.0, 40938.0, 64034.0, 58941.0, 30340.0, 2680.0, 25259.0, 16289.0, 47864.0, 2011.0, 12140.0, 45315.0, 0, 18780.0, 46244.0, 46995.0, 6542.0, 10883.0, 17732.0, 75904.0, 40885.0, 76840.0, 91964.0, 63295.0, 64661.0, 111887.0, 95615.0, 77928.0, 109328.0, 87517.0, 87019.0, 71527.0, 118107.0, 127536.0, 108905.0, 125831.0, 66536.0, 75883.0, 86551.0, 111439.0, 106114.0, 65763.0, 51719.0, 56876.0, 54951.0, 75950.0, 24703.0, 57660.0, 49066.0, 50136.0, 43596.0, 30657.0, 11481.0, 9771.0, 11994.0, 38280.0, 0, 5707.0, 42680.0, 15154.0, 8101.0, 10910.0, 47933.0, 39489.0, 85423.0, 85311.0, 97551.0, 58837.0, 53190.0, 59058.0, 88412.0, 105496.0, 92815.0, 82503.0, 95724.0, 109421.0, 113096.0]
      }
    ],
    "top_apps": [
      {
        "name": "Google Chrome",
        "icon": "/Applications/Google Chrome.app",
        "system": false,
        "value": 4410310656.0
      },
      {
        "name": "Docker",
        "icon": "/Applications/Docker.app",
        "system": false,
        "value": 1753219072.0
      },
      {
        "name": "Telegram",
        "icon": "/Applications/Telegram.app",
        "system": false,
        "value": 425721856.0
      },
      {
        "name": "softwareupdated",
        "icon": "",
        "system": true,
        "value": 295698432.0
      },
      {
        "name": "GoLand",
        "icon": "/Applications/GoLand.app",
        "system": false,
        "value": 128974848.0
      }
    ]
  },
  "alert_details": [
    {"id": "app_cpu:Google Chrome:1790776920", "kind": "app_cpu", "params": {"app": "Google Chrome", "value": 212, "minutes": 5}, "app": "Google Chrome", "since": 1790776920000, "until": 0, "recorded": true, "icon": "/Applications/Google Chrome.app", "unit": "percent", "limit": 150, "below": false, "hold_s": 300, "fired_at": 1790777220000, "fired": 212.0, "peak": 236.6, "avg": 199.6, "current": 181.5, "start": 1790776920000, "step_s": 10, "points": [178.1, 173.7, 176.3, 177.1, 188.2, 203.1, 213.0, 213.3, 225.4, 231.8, 222.5, 220.6, 225.3, 217.0, 218.1, 223.7, 207.3, 190.6, 196.9, 178.3, 185.3, 169.5, 181.8, 172.1, 169.8, 190.5, 186.4, 195.2, 202.0, 222.2, 212.4, 230.6, 221.3, 228.9, 236.6, 232.3, 213.0, 222.6, 203.8, 201.2, 189.2, 175.4, 170.4, 167.0, 173.8, 176.0, 186.6, 183.0], "top": [{"name": "Google Chrome", "icon": "/Applications/Google Chrome.app", "pid": 0, "value": 212.0}, {"name": "GoLand", "icon": "/Applications/GoLand.app", "pid": 0, "value": 46.9}, {"name": "Docker", "icon": "/Applications/Docker.app", "pid": 0, "value": 26.4}, {"name": "Finder", "icon": "/System/Library/CoreServices/Finder.app", "pid": 0, "value": 10.1}, {"name": "WindowServer", "icon": "", "pid": 0, "value": 8.7}], "top_unit": "percent", "pid_count": 14, "procs": [{"name": "Google Chrome Helper (Renderer)", "icon": "", "pid": 4130, "value": 88.4}, {"name": "Google Chrome Helper (GPU)", "icon": "", "pid": 4117, "value": 52.0}, {"name": "Google Chrome", "icon": "", "pid": 4101, "value": 40.3}, {"name": "Google Chrome Helper (Renderer)", "icon": "", "pid": 4131, "value": 21.9}, {"name": "Google Chrome Helper (Renderer)", "icon": "", "pid": 4132, "value": 6.2}], "facts": []},
    {"id": "temp::1790772000", "kind": "temp", "params": {"value": 97, "limit": 95, "unit": "°C"}, "app": "", "since": 1790772000000, "until": 1790772300000, "recorded": true, "icon": "", "unit": "celsius", "limit": 95, "below": false, "hold_s": 60, "fired_at": 1790772060000, "fired": 97.2, "peak": 99.4, "avg": 96.6, "current": null, "start": 1790771400000, "step_s": 10, "points": [74.1, 74.6, 74.9, 74.7, 74.6, 74.7, 75.4, 75.9, 75.6, 76.6, 76.6, 75.9, 77.1, 76.6, 77.0, 77.1, 77.3, 77.6, 78.3, 79.0, 78.4, 79.7, 79.4, 80.4, 80.4, 80.8, 80.8, 81.0, 80.8, 81.7, 82.1, 83.0, 82.2, 82.9, 83.8, 83.7, 84.4, 84.7, 84.9, 85.7, 86.7, 87.0, 87.0, 87.2, 88.6, 87.9, 89.3, 88.6, 89.7, 90.0, 91.2, 91.8, 91.1, 91.8, 92.6, 93.2, 93.4, 94.9, 94.5, 95.6, 96.4, 97.5, 97.0, 97.5, 97.9, 98.3, 99.1, 99.4, 98.7, 98.6, 98.5, 98.9, 97.7, 98.3, 97.6, 96.5, 96.2, 96.0, 95.6, 94.9, 95.0, 93.9, 94.2, 94.1, 94.0, 94.6, 94.3, 95.0, 95.2, 95.7, 93.4, 90.1, 87.6], "top": [{"name": "Google Chrome", "icon": "/Applications/Google Chrome.app", "pid": 0, "value": 244.0}, {"name": "GoLand", "icon": "/Applications/GoLand.app", "pid": 0, "value": 131.5}, {"name": "Docker", "icon": "/Applications/Docker.app", "pid": 0, "value": 64.2}, {"name": "WindowServer", "icon": "", "pid": 0, "value": 22.8}, {"name": "Telegram", "icon": "/Applications/Telegram.app", "pid": 0, "value": 9.3}], "top_unit": "percent", "pid_count": 0, "procs": [], "facts": [{"key": "thermal", "value": 1, "unit": "state"}, {"key": "cpu_temp", "value": 97.2, "unit": "celsius"}]},
    {"id": "disk::1790687400", "kind": "disk", "params": {"value": 9.4, "limit": 10}, "app": "", "since": 1790687400000, "until": 1790691000000, "recorded": true, "icon": "", "unit": "percent", "limit": 10, "below": true, "hold_s": 60, "fired_at": 1790687460000, "fired": 9.9, "peak": 9.4, "avg": 9.7, "current": null, "start": 1790685540000, "step_s": 30, "points": [10.6, 10.6, 10.6, 10.6, 10.6, 10.6, 10.6, 10.6, 10.5, 10.5, 10.5, 10.5, 10.5, 10.5, 10.5, 10.4, 10.5, 10.5, 10.4, 10.4, 10.4, 10.4, 10.4, 10.4, 10.4, 10.4, 10.3, 10.3, 10.3, 10.3, 10.3, 10.3, 10.3, 10.2, 10.2, 10.2, 10.2, 10.2, 10.2, 10.2, 10.2, 10.2, 10.2, 10.2, 10.1, 10.2, 10.1, 10.1, 10.1, 10.1, 10.1, 10.0, 10.1, 10.0, 10.0, 10.0, 10.0, 10.0, 10.0, 10.0, null, null, null, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.9, 9.8, 9.8, 9.9, 9.8, 9.8, 9.8, 9.9, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.8, 9.7, 9.8, 9.7, 9.7, 9.7, 9.8, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.7, 9.6, 9.6, 9.6, 9.6, 9.7, 9.7, 9.6, 9.7, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.6, 9.5, 9.6, 9.6, 9.6, 9.5, 9.5, 9.6, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.5, 9.4, 9.5, 9.4, 9.4, 9.4, 10.8, 11.9, 12.6], "top": [{"name": "Docker", "icon": "/Applications/Docker.app", "pid": 0, "value": 48234496.0}, {"name": "GoLand", "icon": "/Applications/GoLand.app", "pid": 0, "value": 3145728.0}, {"name": "Google Chrome", "icon": "/Applications/Google Chrome.app", "pid": 0, "value": 917504.0}], "top_unit": "bytes_per_s", "pid_count": 0, "procs": [], "facts": [{"key": "disk_free", "value": 49123456000, "unit": "bytes"}, {"key": "disk_total", "value": 494384795648, "unit": "bytes"}, {"key": "disk_write", "value": 52428800.0, "unit": "bytes_per_s"}]}
  ],
  "notice": {
    "level": "error",
    "texts": [{"key": "quit.not_owned", "params": {"pid": 509, "name": "mDNSResponder"}}]
  }
};
