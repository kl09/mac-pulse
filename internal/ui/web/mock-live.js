// Mock-only behaviour for browser design work: values that tick every 2 s, answers to JS → Go messages,
// URL variants and probes. Never embedded in the binary (mock.js stays strict JSON for the Go round-trip test).
//
// Variants: first=1 (no rates yet)  desktop=1 (no battery, no GPU)  calm=1 (no alerts, no sleep blockers)
//           pressure=warning|critical  lowdisk=1  lowbat=1  bits=1  f=1  nonet=1  many=300  notice=1  still=1
//           nohistory=1 (every point null)  deaf=1 (history requests go unanswered)
//           nofans=1  nopower=1 (power.* and freq_mhz null)  nosensors=1  nowifi=1  noinfo=1 (net_info null)
//           charging=1  thermal=fair|serious|critical  cores=16  bt=40  long=1 (long system strings)
//           fail=1 (actions answer with an error)
//           busy=1 (cleanup_scan, cleanup_list, cleanup and browser_tabs answer err.busy)  stale=1 (a cleanup answers err.stale)
//           deaf=list|tabs|alert (cleanup_list, browser_tabs or alert_detail goes unanswered)  slow=<browser> (its tab titles take 3 s, the others 0.4 s)
//           gone=1 (alert_detail answers alertd.gone for every id)  tabs=no_answer|gone (browser_tabs: the script failed, the browser left)
//           noclusters=1 (no core temperatures: cpu.clusters[].temp_c null, the power chip reading stays)
//           nodocker=1 (no docker CLI)  docker=stopped  nodev=1 (nothing on the Dev tab)  dev=0 (state.dev null)
//           kinds=1 (one active alert of each kind)  pinned=1  norules=1 (no app rules)  alertsoff=1
//           scan=idle|running|done (the storage scan)  order=storage,overview (settings.tab_order)
//           clean=running|done|none (cleanup being measured; measured, 4 categories of 11 have something; measured, all empty)
//           notrash=1 (the Trash refuses: no access to it, a move answers err.no_trash)
//           left=3 (a cleanup leaves three entries in place)  update=newer|latest (default: no releases published yet)
//           mic=1  camera=1  lowpower=1  wide=<px> (the popover frame as wide as a resized panel)
// Cleanup:  clean=done&click=[data-id="go_build"] .btn opens the review of 60 entries (50 by name and one row for the rest);
//           add ;.item:nth-child(2) input;.item:nth-child(5) input to uncheck two, ;.review .danger to remove what is left checked.
//           With click= the listing and the removal answer at once, so one chain of clicks reaches every step.
// Alerts:   tab=detail:alert (the newest alert)  tab=detail:alert:<id> (ids: state.alerts in mock.js; the last two recent
//           ones have no detail, an id that is in neither list answers "gone")
// Popovers: click=.sleep .help (a hint)   click=.app:has(.name[title="Google Chrome"]) > .row .help (what an app is;
//           INFO below answers for a bundle, a described system process and a tool with a man page, anything else is unknown)
// Driving:  click=<selector>[;<selector>…]  type=<text for the search field>  point=<0..1 across the chart>
//           keys=<key>[;<key>…], e.g. keys=meta-2;Escape  focus=<selector>  scroll=<px> (every scroll container)  tall=<px>
//           ask=1 (what Go calls after the quit-heaviest shortcut)
// Probes:   probe=shift (layout must not move across ticks)
//           probe=perf (ms per state push) → <pre id="probe">
//           probe=controls (sizes, transition times and confirm order as JSON)
//           probe=map with tab=storage&scan=done (treemap cells tile the box, sized by bytes)
//           probe=cal with tab=detail:clock&lang=ru|en (the week starts on Monday or Sunday, today under its weekday)
//           probe=settings with tab=settings&lang=en&tall=300 (keys every section can set, unknown section, back to the menu)
//           probe=contract (busy and unanswered lookups leave no button disabled, a stale review collapses, tab titles
//           land on the row of their browser, a refused alert id is asked for once)
(function () {
  'use strict';

  var q = new URLSearchParams(location.search);
  var seed = 7;
  // How the lookups answer; probe=contract switches these between its steps.
  var MODE = { busy: q.has('busy'), stale: q.has('stale'), deaf: q.get('deaf'), gone: q.has('gone'), slow: q.get('slow') };

  function rnd() {
    seed = (seed * 1103515245 + 12345) % 2147483648;
    return seed / 2147483648;
  }

  function wobble(v, by, lo, hi) { return Math.max(lo, Math.min(hi, v + (rnd() - 0.5) * 2 * by)); }

  function shift(arr, v) {
    arr.push(v);
    if (arr.length > 60) arr.shift();
  }

  function state() { return window.MP_MOCK.state; }

  function push() { window.mp.onState(state()); }

  function tick() {
    var s = state(), c = s.cpu, sp = s.spark;
    s.time += 2000;
    s.has_rates = s.apps.has_rates = true;
    if (s.net) s.net.has_rates = true;

    c.cores = c.cores.map(function (v) { return wobble(v, 12, 0, 100); });
    c.total = c.cores.reduce(function (a, b) { return a + b; }, 0) / c.cores.length;
    c.system = c.total * 0.33;
    c.user = c.total - c.system;
    c.uptime_s += 2;
    if (c.temp_c != null) c.temp_c = wobble(c.temp_c, 1.5, 40, 99);
    s.memory.app = wobble(s.memory.app, 9e7, 8e9, 15e9);
    s.memory.used = s.memory.app + s.memory.wired + s.memory.compressed;
    s.network.down_rate = wobble(s.network.down_rate || 4e6, 9e5, 0, 9e6);
    s.network.up_rate = wobble(s.network.up_rate || 1.2e5, 4e4, 0, 9e5);
    s.network.down_today += s.network.down_rate * 2;
    s.network.up_today += s.network.up_rate * 2;
    s.disk.read_rate = wobble(s.disk.read_rate || 1.4e6, 5e5, 0, 6e6);
    s.disk.write_rate = wobble(s.disk.write_rate || 3.4e5, 2e5, 0, 3e6);
    s.disk.written_today += s.disk.write_rate * 2;
    if (s.gpu) s.gpu.util = wobble(s.gpu.util, 6, 0, 100);
    if (s.battery) s.battery.power_w = wobble(s.battery.power_w, 1.2, 3, 40);
    tickDetail(s);

    shift(sp.cpu, c.total);
    shift(sp.memory, s.memory.used / s.memory.total * 100);
    shift(sp.gpu, s.gpu ? s.gpu.util : 0);
    shift(sp.temp, c.temp_c || 0);
    shift(sp.power, s.power && s.power.system_w != null ? s.power.system_w : s.battery ? s.battery.power_w : 0);
    shift(sp.disk_read, s.disk.read_rate);
    shift(sp.disk_write, s.disk.write_rate);
    shift(sp.net_down, s.network.down_rate);
    shift(sp.net_up, s.network.up_rate);

    s.apps.items.forEach(function (a) {
      a.procs.forEach(function (p) { p.cpu = wobble(p.cpu || rnd() * 3, 1 + p.cpu * 0.15, 0, 400); });
      a.cpu = a.procs.reduce(function (sum, p) { return sum + p.cpu; }, 0);
    });
    s.apps.items.sort(function (a, b) { return b.cpu - a.cpu || b.memory - a.memory; });
    if (s.net) {
      s.net.apps.forEach(function (a) {
        a.down_rate = wobble(a.down_rate, a.down_rate * 0.2, 0, 9e6);
        a.up_rate = wobble(a.up_rate, a.up_rate * 0.2, 0, 9e5);
        a.down_total += Math.round(a.down_rate * 2);
        a.up_total += Math.round(a.up_rate * 2);
      });
      s.net.connections.forEach(function (k) {
        k.down_rate = wobble(k.down_rate, k.down_rate * 0.2, 0, 9e6);
        k.bytes_in += Math.round(k.down_rate * 2);
      });
    }
    push();
  }

  function tickDetail(s) {
    var p = s.power, m = s.memory, from = 0;
    if (p.system_w != null) {
      p.cpu_w = wobble(p.cpu_w, 0.8, 0.2, 30);
      p.gpu_w = wobble(p.gpu_w, 0.3, 0.01, 12);
      p.system_w = wobble(p.system_w, 1.2, 6, 60);
    }
    s.cpu.clusters.forEach(function (k) {
      var mine = s.cpu.cores.slice(from, from + k.cores), load = mine.reduce(function (a, b) { return a + b; }, 0) / mine.length;
      from += k.cores;
      if (k.freq_mhz != null || p.cpu_w != null) k.freq_mhz = load < 3 ? null : Math.round(1200 + load * 30);
      if (k.temp_c != null) k.temp_c = wobble(k.temp_c, 1, 35, 99);
    });
    ['page_in_rate', 'swap_in_rate', 'compress_rate', 'decompress_rate'].forEach(function (k) { m[k] = wobble(m[k], m[k] * 0.3 + 2e4, 0, 9e6); });
    s.disk.read_total += s.disk.read_rate * 2;
    s.disk.write_total += s.disk.write_rate * 2;
    if (s.gpu) {
      s.gpu.renderer = wobble(s.gpu.renderer, 5, 0, 100);
      s.gpu.tiler = wobble(s.gpu.tiler, 3, 0, 100);
      if (s.gpu.temp_c != null) s.gpu.temp_c = wobble(s.gpu.temp_c, 0.8, 35, 99);
    }
    s.sensors.fans.forEach(function (f) { if (f.rpm) f.rpm = wobble(f.rpm, 60, f.min, f.max); });
    s.sensors.temps.forEach(function (t) { t.temp_c = wobble(t.temp_c, 0.7, 24, 104); });
    if (s.net_info && s.net_info.wifi) s.net_info.wifi.rssi = Math.round(wobble(s.net_info.wifi.rssi, 2, -80, -35));
    s.apps.items.forEach(function (a) {
      if (a.energy_mw == null) return;
      a.energy_mw = wobble(a.energy_mw, a.energy_mw * 0.15 + 2, 0, 9000);
      a.disk_read_rate = wobble(a.disk_read_rate, a.disk_read_rate * 0.3 + 2e3, 0, 9e6);
      a.disk_write_rate = wobble(a.disk_write_rate, a.disk_write_rate * 0.3 + 2e3, 0, 9e6);
    });
  }

  var POINTS = { '1h': [60, 60], '12h': [144, 300], '24h': [144, 600], '7d': [168, 3600], '30d': [180, 14400], '90d': [180, 43200], '1y': [183, 172800] };
  var SERIES = {
    cpu: [['total', 'percent', 34, 22]],
    memory: [['used', 'bytes', 18.5e9, 2.5e9]],
    network: [['down', 'bytes_per_s', 1.1e6, 1e6], ['up', 'bytes_per_s', 7e4, 6e4]],
    disk: [['read', 'bytes_per_s', 1.6e6, 1.5e6], ['write', 'bytes_per_s', 5e5, 4.5e5]],
    gpu: [['util', 'percent', 30, 24]],
    temp: [['cpu', 'celsius', 58, 13]],
    battery: [['percent', 'percent', 62, 36]],
    power: [['system', 'watts', 14, 7], ['cpu', 'watts', 5, 4], ['gpu', 'watts', 1.5, 1.4]]
  };
  var ACTIONS = { public_ip: '203.0.113.42', ping: '16 ms', export: 'Desktop/mac-pulse-20261001-114502.png', copy: '',
    export_csv: 'Desktop/mac-pulse-20261001-114502-system.csv', speedtest: '', eject: '' };
  var KEYS = { export: 'act.saved', copy: 'act.copied', export_csv: 'act.saved', eject: 'act.ejected' };
  var ERRORS = { public_ip: 'Get "https://1.1.1.1/cdn-cgi/trace": context deadline exceeded', ping: 'ping 1.1.1.1: no reply within 2 s',
    export: 'write ~/Desktop: operation not permitted', copy: 'Clipboard unavailable', export_csv: '', speedtest: '',
    eject: 'Volume Backup on disk4s1 failed to unmount: dissented by PID 611 (/System/Library/CoreServices/Finder.app)' };
  var ERROR_KEYS = { export_csv: 'err.not_saved', speedtest: 'err.timeout' };
  // A networkQuality result: 42.6 Mbit/s down, 65.7 up, 70 RPM, 48.6 ms.
  var SPEED = { down: 5320625, up: 8216264, rpm: 70.28, rtt_ms: 48.59 };
  var TOP = {
    cpu: [['Google Chrome', 38.2], ['GoLand', 21.4], ['Docker', 12.0], ['WindowServer', 9.3], ['iTerm', 2.1]],
    memory: [['GoLand', 4.6e9], ['Docker', 4.4e9], ['Google Chrome', 3.9e9], ['WindowServer', 1.2e9], ['Telegram', 4.8e8]]
  };

  // What a finished scan of the home folder says, and the tree Go would keep: storage_open answers one folder of it.
  var SCANNED = { state: 'done', root: '~', files: 412380, bytes: 187.4e9, denied: 2 };
  function node(name, gb, files, extra) {
    var n = { name: name, bytes: gb * 1e9, files: files, dir: true, denied: false, category: '' };
    Object.keys(extra || {}).forEach(function (k) { n[k] = extra[k]; });
    return n;
  }
  var FILE = { dir: false };
  var TREE = {
    '': [node('Library', 83.8, 301200), node('Movies', 52.6, 214), node('go', 31.9, 96500), node('Downloads', 14.2, 1890),
      node('backup.dmg', 4.6, 1, FILE), node('Documents', 0, 0, { denied: true }), node('', 0.3, 12575, FILE)],
    Library: [node('Caches', 31.2, 88400, { category: 'app_caches' }), node('Application Support', 28.4, 142000), node('Containers', 14.9, 51200),
      node('Developer', 6.1, 9800), node('Logs', 1.9, 2400, { category: 'logs' }), node('Mail', 0, 0, { denied: true }), node('', 1.3, 7400, FILE)],
    'Library/Caches': [node('go-build', 12.4, 61200, { category: 'go_build' }), node('Homebrew', 7.9, 37, { category: 'homebrew' }),
      node('com.google.Chrome', 4.2, 18100), node('pip', 2.1, 3300, { category: 'pip' }), node('com.spotify.client', 1.8, 2900), node('', 2.8, 2863, FILE)],
    Movies: [node('Screen Recordings', 31, 190), node('holiday-2025.mov', 14.8, 1, FILE), node('talk.mp4', 6.8, 1, FILE)],
    go: [node('pkg', 22.3, 71000), node('src', 9.1, 25400), node('bin', 0.5, 100)],
    Downloads: [node('Xcode_17.xip', 9.8, 1, FILE), node('ubuntu-25.04.iso', 3.9, 1, FILE), node('', 0.5, 1888, FILE)]
  };

  function level(path) {
    var cut = path.lastIndexOf('/'), siblings = TREE[cut < 0 ? '' : path.slice(0, cut)] || [];
    var me = path === '' ? SCANNED : siblings.filter(function (c) { return c.name === path.slice(cut + 1); })[0] || { bytes: 0, files: 0 };
    return { path: path, bytes: me.bytes, files: me.files, children: TREE[path] || [] };
  }

  // What cleanup_scan finds: [bytes, entries] of the categories that exist on this Mac. Four have something to remove.
  var MEASURED = { app_caches: [4.2e9, 61], logs: [0, 0], npm: [0, 0], go_build: [12.4e9, 60], pip: [0, 0], homebrew: [7.9e9, 37], trash: [0.9e9, 12] };
  var NAMES = { app_caches: ['com.google.Chrome', 'com.spotify.client', 'com.jetbrains.goland', 'com.tinyspeck.slackmacgap', 'org.mozilla.firefox',
    'com.microsoft.VSCode', 'ru.keepcoder.Telegram', 'com.docker.docker'], homebrew: ['downloads', 'api', 'Cask', 'bootsnap'],
    trash: ['mac-pulse-go_build-20261001-183000', 'Screen Recording 2026-09-28 at 11.02.44.mov', 'old-project.zip'] };

  // The entries of a category folder as Go lists them, largest first; their sizes add up to the category.
  var ENTRIES = {};
  function entries(c) {
    if (ENTRIES[c.id]) return ENTRIES[c.id];
    var weights = [], sum = 0, names = NAMES[c.id] || [];
    for (var i = 0; i < c.items; i++) { weights.push(Math.pow(0.93, i)); sum += weights[i]; }
    ENTRIES[c.id] = weights.map(function (w, i) {
      // A Go build cache is 256 folders named by a byte.
      return { name: names[i] || (c.id === 'go_build' ? (i * 37 % 256 + 256).toString(16).slice(1) : 'entry-' + (i + 1)), bytes: Math.round(c.bytes * w / sum) };
    });
    return ENTRIES[c.id];
  }

  // Every listing has an id; a cleanup must name the newest listing of its category, as Go checks.
  var listed = {}, listings = 0;
  function reviewOf(c) {
    var all = entries(c), rest = all.slice(50);
    listed[c.id] = String(++listings);
    return { id: listed[c.id], items: all.slice(0, 50), rest: rest.length, rest_bytes: rest.reduce(function (n, e) { return n + e.bytes; }, 0) };
  }

  function measured(categories) {
    ENTRIES = {};
    return categories.map(function (c) {
      var m = q.get('clean') === 'none' ? [0, 0] : MEASURED[c.id], denied = c.id === 'trash' && q.has('notrash');
      return { id: c.id, denied: denied, bytes: m && !denied ? m[0] : 0, items: m && !denied ? m[1] : 0, permanent: c.permanent };
    });
  }

  var TITLES = ['kl09/mac-pulse: a free system monitor for the menu bar', 'Activity Monitor User Guide for Mac - Apple Support',
    'Squarified Treemaps (Bruls, Huizing, van Wijk)', 'Intl.Locale.prototype.getWeekInfo() - JavaScript | MDN', 'Inbox (3)', 'New Tab'];
  // lsof -F ftn of a real process, as Go lists it: the working folder and regular files, then sockets.
  var DETAIL = { threads: 14, chain: [], more: 0, args: [],
    files: ['/Users/alex/go/src/github.com/kl09/mac-pulse', '/dev/null', '/Users/alex/Library/Logs/gopls.log', '/private/var/folders/zz/T/gopls-cache.db'],
    sockets: ['IPv4 127.0.0.1:8799', 'IPv6 *:8799', 'unix socket × 2'] };
  var UPDATES = { newer: ['upd.newer', '0.6.0'], latest: ['upd.latest', '0.5.0'] };

  function history(metric, range) {
    var canned = window.MP_MOCK.history;
    if (metric === canned.metric && range === canned.range) return canned;
    var n = POINTS[range][0], step = POINTS[range][1];
    seed = metric.length * 31 + n;
    return {
      metric: metric, range: range, start: state().time - n * step * 1000, step_s: step,
      series: SERIES[metric].map(function (spec, k) {
        var points = [];
        for (var i = 0; i < n; i++) {
          var gap = range !== '1h' && i > n * 0.27 && i < n * 0.39;
          var wave = Math.sin(i / n * 9 + k) * 0.6 + (rnd() - 0.5) * 0.7;
          points.push(gap ? null : Math.max(0, spec[2] + spec[3] * wave));
        }
        return { name: spec[0], unit: spec[1], points: points };
      }),
      top_apps: step >= 43200 ? [] : (TOP[metric] || (metric === 'network' ? canned.top_apps.map(function (a) { return [a.name, a.value]; }) : [])).map(function (a) {
        var known = state().apps.items.filter(function (x) { return x.name === a[0]; })[0];
        return { name: a[0], icon: known ? known.icon : '', system: !!known && known.system, value: a[1] };
      })
    };
  }

  // What Go answers to app_info: a bundle, system processes the dictionaries describe, a tool with a man page.
  var INFO = {
    'Google Chrome': { title: 'Google Chrome', version: '154.0.8037.92', bundle_id: 'com.google.Chrome', category: 'productivity',
      developer: 'Copyright 2026 Google LLC. All rights reserved.', path: '/Applications/Google Chrome.app', signing: 'developer',
      signer: 'Google LLC (EQHXZ8M8AV)', executable_only: true, parent: 'launchd', user: 'alex', killable: true },
    WindowServer: { desc_key: 'proc.WindowServer', path: '/System/Library/PrivateFrameworks/SkyLight.framework/Versions/A/Resources/WindowServer',
      signing: 'apple', parent: 'launchd', user: '_windowserver' },
    kernel_task: { desc_key: 'proc.kernel_task', signing: 'apple', user: 'root' },
    fsnotifier: { path: '/Applications/GoLand.app/Contents/bin/fsnotifier', signing: 'unverified', parent: 'goland (GoLand)', user: 'alex', killable: true },
    htop: { manual: 'interactive process viewer', path: '/opt/homebrew/Cellar/htop/3.3.0/bin/htop', signing: 'adhoc',
      parent: 'zsh (iTerm)', user: 'alex', killable: true }
  };

  function info(msg) {
    var name = msg.name;
    state().apps.items.forEach(function (a) {
      a.procs.forEach(function (p) { if (a.name === msg.name && p.pid === msg.pid) name = p.name; });
    });
    var known = INFO[name] || { path: '/Users/alex/go/bin/' + name, signing: 'none', parent: msg.name, user: 'alex', killable: true };
    var out = { name: msg.name, pid: msg.pid, title: name, version: '', bundle_id: '', developer: '', category: '', path: '', signing: '',
      signer: '', executable_only: false, desc_key: '', manual: '', parent: '', since: state().time - 5 * 3600e3, user: '', killable: false };
    Object.keys(known).forEach(function (k) { out[k] = known[k]; });
    return out;
  }

  // A chain of click= selectors runs in one go, so the answers it clicks on come at once.
  function later(answer, ms) {
    if (q.has('click')) answer();
    else setTimeout(answer, ms);
  }

  function send(msg) {
    var s = state();
    if (msg.type === 'app_info') {
      setTimeout(function () {
        var bad = q.has('fail');
        window.mp.onAction({ action: 'app_info', ok: !bad, text: '', key: bad ? 'info.gone' : '', info: bad ? { name: msg.name, pid: msg.pid } : info(msg) });
      }, q.has('probe') ? 0 : 60);
    }
    if (msg.type === 'alert_detail' && MODE.deaf !== 'alert') {
      setTimeout(function () {
        function match(a) { return a.id === msg.id; }
        var row = s.alerts.active.concat(s.alerts.recent).filter(match)[0], detail = window.MP_MOCK.alert_details.filter(match)[0];
        if (!row || MODE.gone) { window.mp.onAction({ action: 'alert_detail', ok: false, text: msg.id, key: 'alertd.gone' }); return; }
        // An alert the mock has no detail for stands for one stored by a build that recorded none.
        var bare = { recorded: false, icon: '', unit: '', limit: 0, below: false, hold_s: 0, fired_at: 0, fired: null, peak: null, avg: null,
          current: null, start: 0, step_s: 0, points: [], top: [], top_unit: '', pid_count: 0, procs: [], facts: [] };
        Object.keys(row).forEach(function (k) { bare[k] = row[k]; });
        window.mp.onAction({ action: 'alert_detail', ok: true, text: '', key: '', alert: detail || bare });
      }, q.has('probe') ? 0 : 60);
    }
    if (msg.type === 'tab') setTimeout(push, 0);
    if (msg.type === 'set') { s.settings[msg.key] = msg.value; push(); }
    if (msg.type === 'login_item') { s.settings.launch_at_login = msg.enabled; push(); }
    if (msg.type === 'history' && !q.has('deaf')) {
      setTimeout(function () {
        var hist = history(msg.metric, msg.range);
        if (q.has('nohistory')) {
          hist = { metric: hist.metric, range: hist.range, start: hist.start, step_s: hist.step_s, top_apps: [],
            series: hist.series.map(function (one) { return { name: one.name, unit: one.unit, points: one.points.map(function () { return null; }) }; }) };
        }
        window.mp.onHistory(hist);
      }, q.has('probe') ? 0 : 120);
    }
    if (msg.type in ACTIONS) {
      setTimeout(function () {
        var bad = q.has('fail'), r = { action: msg.type, ok: !bad, text: (bad ? ERRORS : ACTIONS)[msg.type], key: (bad ? ERROR_KEYS : KEYS)[msg.type] || '' };
        if (msg.type === 'speedtest' && !bad) r.values = SPEED;
        if (msg.type === 'eject' && !bad) {
          r.text = s.disk.volumes.filter(function (v) { return v.mount === msg.mount; })[0].name;
          s.disk.volumes = s.disk.volumes.filter(function (v) { return v.mount !== msg.mount; });
        }
        window.mp.onAction(r);
      }, q.has('probe') ? 0 : msg.type === 'speedtest' ? 1500 : 400);
    }
    if (msg.type === 'pin') { s.pinned = msg.enabled; push(); }
    if (msg.type === 'storage_scan') {
      s.storage.scan = { state: 'running', root: '~', files: 18250, bytes: 9.4e9, denied: 2 };
      push();
      setTimeout(function () {
        if (s.storage.scan.state !== 'running') return;
        s.storage.scan = SCANNED;
        push();
        window.mp.onAction({ action: 'storage_scan', ok: true, text: '', key: '' });
      }, q.has('probe') ? 0 : 1500);
    }
    if (msg.type === 'storage_cancel') { s.storage.scan = { state: 'cancelled', root: '~', files: 0, bytes: 0, denied: 0 }; push(); }
    if (msg.type === 'storage_clear') { s.storage.scan = { state: 'idle', root: '', files: 0, bytes: 0, denied: 0 }; push(); }
    if (msg.type === 'storage_open') setTimeout(function () { window.mp.onStorage(level(msg.path)); }, 0);
    var busy = MODE.busy && /^(cleanup|cleanup_list|cleanup_scan|browser_tabs)$/.test(msg.type);
    if (busy) {
      later(function () { window.mp.onAction({ action: msg.type, ok: false, text: msg.name || '', key: 'err.busy' }); }, 100);
      return;
    }
    if (msg.type === 'cleanup_scan') {
      s.storage.cleanup.state = 'running';
      push();
      setTimeout(function () {
        s.storage.cleanup = { state: 'done', categories: measured(s.storage.cleanup.categories) };
        push();
        window.mp.onAction({ action: 'cleanup_scan', ok: true, text: '', key: '' });
      }, q.has('probe') ? 0 : 600);
    }
    if (msg.type === 'cleanup_list' && MODE.deaf !== 'list') {
      later(function () {
        var c = s.storage.cleanup.categories.filter(function (x) { return x.id === msg.category; })[0];
        window.mp.onAction({ action: 'cleanup_list', ok: true, text: c.id, key: '', review: reviewOf(c) });
      }, 300);
    }
    if (msg.type === 'cleanup') {
      later(function () {
        var c = s.storage.cleanup.categories.filter(function (x) { return x.id === msg.category; })[0], left = Number(q.get('left') || 0);
        var stale = !c.items || MODE.stale || msg.review !== listed[c.id];
        var r = { action: 'cleanup', ok: false, text: c.id, key: stale ? 'err.stale' : q.has('notrash') && !msg.permanent ? 'err.no_trash' : '' };
        if (!r.key) {
          // Go takes the names of the review and, with rest, every entry the review did not name; left of them stay in place.
          var all = entries(c), gone = all.filter(function (e, i) { return i < 50 ? msg.items.indexOf(e.name) >= 0 : msg.rest; }).slice(left);
          var freed = gone.reduce(function (n, e) { return n + e.bytes; }, 0);
          r = { action: 'cleanup', ok: true, text: c.id, key: 'act.cleaned', values: { bytes: freed, items: gone.length, failed: left } };
          if (!msg.permanent && gone.length) r.list = ['mac-pulse-' + c.id + '-20261002-041500', '~/Library/Caches'];
          // What was moved lands in the Trash, which is measured again: one wrapper folder more.
          var trash = s.storage.cleanup.categories.filter(function (x) { return x.id === 'trash'; })[0];
          if (!msg.permanent && gone.length && trash && !trash.denied) {
            trash.bytes += freed; trash.items += 1;
            delete ENTRIES.trash;
          }
          // What stayed in place is measured again: it keeps its row and its share of the size.
          ENTRIES[c.id] = all.filter(function (e) { return gone.indexOf(e) < 0; });
          c.bytes -= freed;
          c.items = ENTRIES[c.id].length;
          push();
        }
        window.mp.onAction(r);
      }, 400);
    }
    if (msg.type === 'browser_tabs' && MODE.deaf !== 'tabs') {
      setTimeout(function () {
        var bad = q.has('fail');
        // Each browser answers with titles of its own, so a probe can tell whose list a row shows.
        var key = bad ? 'err.automation' : { no_answer: 'act.no_answer', gone: 'info.gone' }[q.get('tabs')] || '';
        bad = !!key;
        window.mp.onAction({ action: 'browser_tabs', ok: !bad, text: msg.name, key: key,
          list: bad ? [] : [msg.name + ' — Start Page'].concat(TITLES) });
      }, MODE.slow === msg.name ? 3000 : 400);
    }
    if (msg.type === 'proc_detail') {
      setTimeout(function () {
        var bad = q.has('fail'), d = JSON.parse(JSON.stringify(DETAIL));
        d.chain = [msg.name, 'launchd'];
        if (msg.args) d.args = ['/opt/homebrew/bin/gopls', '-mode=stdio', '-logfile=/Users/alex/Library/Logs/gopls.log'];
        window.mp.onAction(bad ? { action: 'proc_detail', ok: false, text: '', key: 'info.gone' } : { action: 'proc_detail', ok: true, text: '', key: '', detail: d });
      }, 60);
    }
    if (msg.type === 'update_check') {
      setTimeout(function () {
        var u = UPDATES[q.get('update')] || ['upd.none', ''];
        window.mp.onAction(q.has('fail') ? { action: 'update_check', ok: false, text: '', key: 'err.update' } : { action: 'update_check', ok: true, text: u[1], key: u[0] });
      }, 400);
    }
    if (msg.type === 'reveal' && q.has('fail')) window.mp.onNotice({ level: 'error', texts: [{ key: 'err.reveal' }] });
    if (msg.type === 'quit_app') {
      setTimeout(function () {
        if (msg.pids.indexOf(509) >= 0) { window.mp.onNotice(window.MP_MOCK.notice); return; }
        // A signal answers by name; a stopped or continued process stays in the list.
        if (msg.signal) {
          window.mp.onAction({ action: 'quit_app', ok: true, text: msg.signal, key: 'act.signal' });
          if (msg.signal === 'STOP' || msg.signal === 'CONT') return;
        }
        s.apps.items.forEach(function (a) {
          a.procs = a.procs.filter(function (p) { return msg.pids.indexOf(p.pid) < 0; });
          a.pid_count = a.procs.length;
        });
        s.apps.items = s.apps.items.filter(function (a) { return a.procs.length; });
        push();
      }, 500);
    }
  }

  function variants() {
    var s = state();
    if (q.has('first')) {
      s.has_rates = s.apps.has_rates = s.net.has_rates = false;
      s.network.down_rate = s.network.up_rate = s.disk.read_rate = s.disk.write_rate = 0;
      Object.keys(s.spark).forEach(function (k) { s.spark[k] = []; });
      s.apps.items.forEach(function (a) { a.cpu = 0; a.procs.forEach(function (p) { p.cpu = 0; }); });
      s.net.apps.concat(s.net.connections, s.net.talkers).forEach(function (r) { r.down_rate = r.up_rate = 0; });
    }
    if (q.has('desktop')) s.battery = s.gpu = null;
    if (q.has('calm')) { s.alerts.active = []; s.sleep_blockers = []; }
    if (q.has('pressure')) s.memory.pressure = q.get('pressure');
    if (q.has('lowdisk')) s.disk.free = s.disk.total * 0.04;
    if (q.has('lowbat')) s.battery.percent = 8;
    if (q.has('bits')) s.settings.net_unit = 'bits';
    if (q.has('f')) s.settings.temp_unit = 'F';
    if (q.has('nonet')) s.net = null;
    if (q.has('nofans')) s.sensors.fans = [];
    if (q.has('nopower')) {
      s.power = { system_w: null, adapter_w: null, cpu_w: null, gpu_w: null };
      s.cpu.clusters.forEach(function (k) { k.freq_mhz = null; });
      if (s.gpu) s.gpu.freq_mhz = null;
    }
    if (q.has('nosensors')) {
      s.sensors.temps = [];
      s.cpu.temp_c = null;
      s.cpu.clusters.forEach(function (k) { k.temp_c = null; });
      if (s.gpu) s.gpu.temp_c = null;
    }
    if (q.has('noclusters')) s.cpu.clusters.forEach(function (k) { k.temp_c = null; });
    if (q.has('nowifi')) s.net_info.wifi = null;
    if (q.has('noinfo')) s.net_info = null;
    if (q.has('thermal')) {
      s.sensors.thermal = q.get('thermal');
      if (/serious|critical/.test(q.get('thermal'))) s.alerts.active.unshift({ id: 'thermal', kind: 'thermal', params: {}, app: '', since: s.time - 240000, until: 0 });
    }
    if (q.has('kinds')) {
      s.alerts.active = [
        ['battery_low', '', { value: 14, limit: 20 }], ['bt_battery', 'Magic Trackpad', { app: 'Magic Trackpad', value: 9 }],
        ['memory_used', '', { value: 93, limit: 90 }], ['swap', '', { value: 6.2, limit: 4 }],
        ['app_rule', 'Google Chrome', { app: 'Google Chrome', metric: 'memory', value: 8.4, limit: 8, unit: 'GB', minutes: 10 }]
      ].map(function (a) { return { id: a[0], kind: a[0], params: a[2], app: a[1], since: s.time - 90000, until: 0 }; });
    }
    if (q.has('pinned')) s.pinned = true;
    if (q.get('scan') === 'running') s.storage.scan = { state: 'running', root: '~', files: 18250, bytes: 9.4e9, denied: 2 };
    if (q.get('scan') === 'done') s.storage.scan = SCANNED;
    if (q.get('clean') === 'done' || q.get('clean') === 'none') s.storage.cleanup = { state: 'done', categories: measured(s.storage.cleanup.categories) };
    if (q.get('clean') === 'running') s.storage.cleanup.state = 'running';
    if (q.has('order')) s.settings.tab_order = q.get('order').split(',');
    if (q.has('mic')) s.media.mic = { active: true, apps: ['Telegram', 'zoom.us'] };
    if (q.has('camera')) s.media.camera.active = true;
    if (q.has('lowpower') && s.battery) s.battery.low_power = true;
    if (q.has('norules')) s.settings.alert_rules = [];
    if (q.has('alertsoff')) s.settings.alerts = false;
    if (q.has('nodocker')) s.dev.docker = 'missing';
    if (q.has('docker')) s.dev.docker = q.get('docker');
    if (s.dev.docker !== 'ok') s.dev.containers = [];
    if (q.has('nodev')) s.dev.projects = s.dev.agents = s.dev.containers = [];
    if (q.get('dev') === '0') s.dev = null;
    if (q.has('charging') && s.battery) {
      s.battery.state = 'charging';
      s.battery.adapter = { name: '70W USB-C Power Adapter', watts: 68 };
      s.battery.unplugged_at = null;
      s.power.adapter_w = 41.5;
    }
    if (q.has('cores')) {
      var n = Number(q.get('cores')), eff = Math.min(4, n);
      s.cpu.cores = [];
      for (var c = 0; c < n; c++) s.cpu.cores.push(Math.round(rnd() * 90));
      s.cpu.clusters = [{ name: 'Efficiency', cores: eff, freq_mhz: 1840, temp_c: 51.2 }, { name: 'Performance', cores: n - eff, freq_mhz: 3310, temp_c: 71.4 }];
    }
    for (var d = s.sensors.bluetooth.length; d < Number(q.get('bt') || 0); d++) {
      s.sensors.bluetooth.push({ name: 'Device ' + d, kind: d % 3 ? 'Mouse' : '', levels: [{ part: 'main', percent: (d * 13) % 101 }] });
    }
    if (q.has('long')) {
      // A home folder of millions of files: the widest numbers the Storage list has to hold next to a long name.
      TREE[''][0].files = 12345678;
      TREE[''][0].bytes = 983.8e9;
      TREE[''].splice(1, 0, node('Project archives and backup copies for the years 2019–2026', 128.4, 2345678));
      s.cpu.model = 'Apple M4 Pro with a deliberately long model string for truncation checks';
      s.disk.model = 'APPLE SSD AP1024Z EXTRA LONG MEDIA NAME REV 2026';
      s.disk.volumes[1].name = 'Time Machine Backup of Alex’s MacBook Pro 2026 — Encrypted';
      s.sensors.bluetooth[0].name = 'Alex’s extraordinarily long-named AirPods Pro (2nd generation) USB-C';
      s.net_info.interfaces[0].name = 'en0-very-long-interface-name';
      s.net_info.interfaces[0].ipv4 = ['192.168.100.200', '10.211.55.2', '172.16.254.254'];
      s.net_info.dns = ['2001:4860:4860::8888', '2001:4860:4860::8844', '192.168.100.1', '198.51.100.53'];
      if (s.net_info.wifi) s.net_info.wifi.security = 'WPA2/WPA3 Personal Transitional Enterprise';
      if (s.battery) s.battery.adapter = { name: 'Third-Party 140W USB-C GaN Travel Power Adapter with a Long Name', watts: 140 };
    }
    for (var i = s.apps.items.length; i < Number(q.get('many') || 0); i++) {
      s.apps.items.push({ name: 'com.apple.daemon-' + i, icon: '', pid_count: 1, cpu: 0, memory: 3e6 + i * 1e5, system: true, killable: false,
        disk_read_rate: null, disk_write_rate: null, energy_mw: null,
        procs: [{ pid: 60000 + i, name: 'com.apple.daemon-' + i, cpu: 0, memory: 3e6 + i * 1e5, system: true, killable: false }] });
    }
  }

  function drive() {
    (q.get('click') || '').split(';').filter(Boolean).forEach(function (selector) {
      var target = document.querySelector(selector);
      if (target) target.click();
    });
    if (q.has('type')) {
      var input = document.querySelector('.search input') || document.querySelector('.d-clock .rule-form input');
      input.value = q.get('type');
      input.dispatchEvent(new Event('input'));
    }
    if (q.has('point')) {
      var svg = document.querySelector('.chart svg'), rect = svg.getBoundingClientRect();
      svg.dispatchEvent(new PointerEvent('pointermove', { clientX: rect.left + rect.width * Number(q.get('point')), clientY: rect.top + 40 }));
    }
    if (q.has('focus')) document.querySelector(q.get('focus')).focus();
    (q.get('keys') || '').split(';').filter(Boolean).forEach(function (combo) {
      var key = combo.replace('meta-', '');
      (document.activeElement || document).dispatchEvent(new KeyboardEvent('keydown', { key: key, metaKey: key !== combo, bubbles: true, cancelable: true }));
    });
    if (q.has('scroll')) document.querySelectorAll('.scroll, .review .items').forEach(function (el) { el.scrollTop = Number(q.get('scroll')); });
    if (q.has('notice')) window.mp.onNotice(window.MP_MOCK.notice);
    if (q.has('ask')) window.mp.ask();
  }

  function report(name, failures) {
    var pre = document.createElement('pre');
    pre.id = 'probe';
    pre.textContent = name + (failures.length ? ' FAIL\n' + failures.join('\n') : ' ok');
    document.body.appendChild(pre);
    document.title = pre.textContent.split('\n')[0];
  }

  // Boxes of everything structural must be identical before and after several ticks.
  function probeShift() {
    function boxes() {
      var out = [];
      document.querySelectorAll('.top, .tile, .tile-h, .big, .rates, .cap, .kv, .row, .alert, .foot, .seg, .chart svg, ' +
        '.facts > div, .stats > *, .donut, .ring-legend > *, .cluster, .bar-row, .vol, .if, .act, .acts, .fan > *, .sensor > *, .group-b, .levels').forEach(function (el) {
        var r = el.getBoundingClientRect();
        if (r.width) out.push([el.className.baseVal === undefined ? el.className.split(' ')[0] : 'svg', Math.round(r.left), Math.round(r.top), Math.round(r.width), Math.round(r.height)].join(' '));
      });
      return out;
    }
    var before = boxes(), bad = [];
    for (var i = 0; i < 6; i++) tick();
    var after = boxes();
    if (before.length !== after.length) bad.push('box count ' + before.length + ' → ' + after.length);
    before.forEach(function (b, k) { if (after[k] !== b) bad.push(b + ' → ' + after[k]); });
    report('shift (' + before.length + ' boxes)', bad.slice(0, 20));
  }

  // Sizes, the longest transition of each control and the button order of an open confirm.
  function probeControls() {
    function box(selector) {
      var r = document.querySelector(selector).getBoundingClientRect();
      return [r.width, r.height];
    }
    function ms(selector) {
      var times = getComputedStyle(document.querySelector(selector)).transitionDuration.split(',');
      return Math.round(Math.max.apply(null, times.map(parseFloat)) * 1000);
    }
    window.mp.show('settings:menubar');
    var open = '.set-sec:not([hidden]) ';
    var out = { switch: box(open + 'input.switch'), check: box(open + 'input.check'),
      native_checkboxes: document.querySelectorAll('input[type=checkbox]:not(.switch):not(.check)').length };
    var slow = [ms(open + 'input.switch'), ms(open + 'input.check')];
    window.mp.show('settings');
    out.btn = box('.settings-foot .btn')[1];
    window.mp.show('processes');
    document.querySelector('.kill:not([aria-disabled="true"])').click();
    out.transitions_ms = [ms('.btn:not(.primary)'), ms('.btn.primary')].concat(slow);
    out.confirm = Array.prototype.map.call(document.querySelectorAll('.confirm .btn'), function (b) {
      return b.classList.contains('danger') ? 'force' : b.classList.contains('primary') ? 'quit' : 'cancel';
    });
    var pre = document.createElement('pre');
    pre.id = 'probe';
    pre.textContent = JSON.stringify(out);
    document.body.appendChild(pre);
  }

  // The cells of the treemap must tile its box without overlapping, the first one sized by its share of the bytes.
  function probeMap() {
    var box = document.querySelector('.treemap').getBoundingClientRect(), bad = [], area = 0;
    var cells = Array.prototype.map.call(document.querySelectorAll('.treemap > button'), function (b) { return b.getBoundingClientRect(); });
    cells.forEach(function (a, i) {
      area += a.width * a.height;
      cells.slice(i + 1).forEach(function (b, k) {
        var w = Math.min(a.right, b.right) - Math.max(a.left, b.left), h = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
        if (w > 0.5 && h > 0.5) bad.push('cells ' + i + ' and ' + (i + 1 + k) + ' overlap by ' + w.toFixed(1) + ' × ' + h.toFixed(1));
      });
    });
    var all = box.width * box.height, share = cells[0].width * cells[0].height / all, want = TREE[''][0].bytes / SCANNED.bytes;
    if (Math.abs(area / all - 1) > 0.01) bad.push('cells cover ' + (area / all).toFixed(3) + ' of the box');
    if (Math.abs(share - want) > 0.01) bad.push('first cell is ' + share.toFixed(3) + ' of the box, its bytes are ' + want.toFixed(3));
    report('map (' + cells.length + ' cells)', bad);
  }

  // The week starts on Sunday in en, on Monday in ru, and today stands under its own weekday.
  function probeCal() {
    var first = q.get('lang') === 'en' ? 0 : 1, now = new Date(), col = (now.getDay() - first + 7) % 7, bad = [];
    var heads = document.querySelectorAll('.cal .th'), today = document.querySelector('.cal .today');
    var want = new Intl.DateTimeFormat(q.get('lang'), { weekday: 'short' }).format(new Date(2024, 0, first ? 1 : 7));
    if (heads[0].textContent !== want) bad.push('week starts on ' + heads[0].textContent + ', want ' + want);
    if (today.textContent !== String(now.getDate())) bad.push('today is ' + today.textContent);
    if (Math.abs(today.getBoundingClientRect().left - heads[col].getBoundingClientRect().left) > 1) bad.push('today is not under column ' + col);
    report('cal ' + q.get('lang'), bad);
  }

  // Settings: every control of every route is operated and what it sends is listed, so the keys the
  // page can set are compared across versions (routes=settings reads a page with no sections); an
  // unknown section opens the menu; Back from a section restores the menu's scroll and focus.
  function probeSettings() {
    var real = window.MP_MOCK_LIVE.send, sent = {}, at = '', done = new WeakSet(), bad = [];
    window.MP_MOCK_LIVE.send = function (msg) {
      if (msg.type !== 'tab') sent[at][msg.type === 'set' ? msg.key : '(' + msg.type + ')'] = 1;
      real(msg);
    };
    (q.get('routes') || 'settings,settings:general,settings:appearance,settings:menubar,settings:layout,settings:alerts,settings:shortcuts').split(',').forEach(function (route) {
      sent[at = route] = {};
      for (var guard = 0; guard < 2000; guard++) {
        window.mp.show(route);
        var next = Array.prototype.filter.call(document.querySelectorAll('#view > .tab:not([hidden]) :is(button, input, select)'), function (c) {
          return !done.has(c) && !c.closest('.set-sec[hidden], .set-menu[hidden], [data-sec]');
        })[0];
        if (!next) break;
        done.add(next);
        if (next.tagName === 'BUTTON' || next.type === 'checkbox') { next.click(); continue; }
        if (next.tagName === 'SELECT') next.selectedIndex = (next.selectedIndex + 1) % next.options.length;
        else next.value = next.type === 'number' ? next.min || 1 : 'Probe';
        next.dispatchEvent(new Event('input', { bubbles: true }));
        next.dispatchEvent(new Event('change', { bubbles: true }));
      }
    });
    window.MP_MOCK_LIVE.send = real;
    var all = {};
    var lines = Object.keys(sent).map(function (route) {
      Object.keys(sent[route]).forEach(function (k) { all[k] = (all[k] || 0) + 1; });
      return route + ': ' + Object.keys(sent[route]).sort().join(' ');
    });
    lines.push('all: ' + Object.keys(all).sort().join(' '));
    Object.keys(all).forEach(function (k) { if (all[k] > 1) bad.push(k + ' is sent from ' + all[k] + ' routes'); });
    var menu = document.querySelector('.set-menu');
    if (menu) {
      window.mp.show('settings:nope');
      if (menu.hidden || document.querySelector('.set-sec:not([hidden])')) bad.push('settings:nope did not open the menu');
      menu.scrollTop = 37;
      var top = menu.scrollTop, row = menu.querySelector('[data-sec="alerts"]');
      row.click();
      if (!menu.hidden || document.querySelector('.set-sec:not([hidden]) h2').textContent !== row.querySelector('b').textContent) bad.push('the row did not push its section');
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
      if (menu.hidden || menu.scrollTop !== top || !top) bad.push('back: menu scroll ' + menu.scrollTop + ', want ' + top);
      if (document.activeElement !== row) bad.push('back: focus is not on the row');
      row.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true }));
      if (document.activeElement.dataset.sec !== 'layout' || menu.hidden) bad.push('arrow up did not move to the row above');
      lines.push('menu scroll kept: ' + top);
    }
    report('settings\n' + lines.join('\n') + '\n', bad);
  }

  // The lookups of the contract, one case after another on the page's own controls. Run with
  // tab=storage&clean=done and --virtual-time-budget=400000: two cases wait out the page's failsafe timers.
  function probeContract() {
    var bad = [], real = window.MP_MOCK_LIVE.send, asked = 0, at = 0;
    window.MP_MOCK_LIVE.send = function (msg) {
      if (msg.type === 'alert_detail') asked++;
      real(msg);
    };
    function $(sel) { return document.querySelector(sel); }
    function note() { return $('.cleanup .note').textContent; }
    function dead() { return document.querySelectorAll('.cleanup .btn:disabled').length; }
    function check(ok, what) { if (!ok) bad.push(what); }
    function app(name) { return $('.app:has(.name[title="' + name + '"])'); }
    function titles(name) { return app(name).querySelector('.titles').textContent; }
    function askTitles(name) {
      app(name).querySelector('.kid-acts .btn').click();
      app(name).querySelector('.titles .primary').click();
    }
    var busyText = window.MP_I18N.en['err.busy'];
    [[0, function () { MODE.busy = true; $('.cat .btn').click(); }],
      [300, function () { check(!dead() && note() === busyText, 'cleanup_list busy: ' + dead() + ' disabled, note "' + note() + '"'); $('.cleanup .acts .btn').click(); }],
      [300, function () {
        check(!dead() && !$('.cleanup .acts').hidden && note() === busyText, 'cleanup_scan busy: ' + dead() + ' disabled, note "' + note() + '"');
        MODE.busy = false;
        $('.cat .btn').click();
      }],
      [500, function () { check(!!$('.review'), 'no review after Show'); MODE.busy = true; $('.review .danger').click(); }],
      [300, function () {
        check(!!$('.review') && !$('.review .danger').disabled && note() === busyText, 'cleanup busy: the review is gone or its button is dead, note "' + note() + '"');
        MODE.busy = false;
        MODE.stale = true;
        $('.review .danger').click();
      }],
      [600, function () {
        check(!$('.review') && !dead() && note().indexOf(window.MP_I18N.en['clean.changed']) >= 0, 'cleanup stale: review ' + !!$('.review') + ', note "' + note() + '"');
        MODE.stale = false;
        MODE.deaf = 'list';
        $('.cat .btn').click();
      }],
      [300, function () { check(dead() > 0, 'an unanswered cleanup_list does not hold the buttons at all'); }],
      [61000, function () {
        check(!dead() && note() === window.MP_I18N.en['act.no_answer'], 'cleanup_list unanswered for 61 s: ' + dead() + ' disabled, note "' + note() + '"');
        MODE.deaf = '';
        window.mp.show('processes');
      }],
      [200, function () {
        app('Google Chrome').querySelector('.row').click();
        app('Safari').querySelector('.row').click();
        MODE.slow = 'Google Chrome';
        askTitles('Google Chrome');
        askTitles('Safari');
      }],
      [800, function () {
        check(titles('Safari').indexOf('Safari — Start Page') >= 0, 'Safari row: "' + titles('Safari').slice(0, 60) + '"');
        check(titles('Google Chrome').indexOf(window.MP_I18N.en['info.loading']) === 0, 'Chrome row before its answer: "' + titles('Google Chrome').slice(0, 60) + '"');
        check(!!app('Google Chrome').querySelector('.titles .btn'), 'the waiting step has no Cancel');
      }],
      [3000, function () {
        check(titles('Google Chrome').indexOf('Google Chrome — Start Page') >= 0, 'Chrome row: "' + titles('Google Chrome').slice(0, 60) + '"');
        check(titles('Safari').indexOf('Safari — Start Page') >= 0, 'Safari row after Chrome answered: "' + titles('Safari').slice(0, 60) + '"');
        app('Google Chrome').querySelector('.titles .btn').click();
        askTitles('Google Chrome');
        app('Google Chrome').querySelector('.titles .btn').click();
      }],
      [3500, function () {
        check(titles('Google Chrome') === '' && $('#toast').hidden, 'an answer after Cancel: titles "' + titles('Google Chrome').slice(0, 40) + '", toast ' + !$('#toast').hidden);
        MODE.busy = true;
        askTitles('Google Chrome');
      }],
      [300, function () {
        check(titles('Google Chrome').indexOf(busyText) === 0, 'browser_tabs busy: "' + titles('Google Chrome').slice(0, 60) + '"');
        MODE.busy = false;
        MODE.deaf = 'tabs';
        app('Google Chrome').querySelector('.titles .btn').click();
        askTitles('Google Chrome');
      }],
      [126000, function () {
        check(titles('Google Chrome').indexOf(window.MP_I18N.en['act.no_answer']) === 0, 'browser_tabs unanswered for 126 s: "' + titles('Google Chrome').slice(0, 60) + '"');
        MODE.deaf = '';
        MODE.gone = true;
        asked = 0;
        window.mp.show('detail:alert:app_cpu:Nope:1');
      }],
      [200, push], [2100, push], [2100, push], [2100, push], [2100, push], [11000, push],
      [100, function () {
        check(asked === 1 && $('.a-note').textContent === window.MP_I18N.en['alertd.gone'], 'a refused alert id: asked ' + asked + ' times, note "' + $('.a-note').textContent + '"');
        MODE.gone = false;
        MODE.deaf = 'alert';
        asked = 0;
        window.mp.show('overview');
        window.mp.show('detail:alert:app_cpu:Nope:2');
      }],
      [2100, push], [2100, push], [2100, push], [2100, push], [2100, push], [11000, push],
      [100, function () {
        check(asked === 3 && $('.a-note').textContent === window.MP_I18N.en['alertd.gone'], 'an unanswered alert id: asked ' + asked + ' times, note "' + $('.a-note').textContent + '"');
        window.MP_MOCK_LIVE.send = real;
        report('contract', bad);
      }]
    ].forEach(function (step) {
      at += step[0];
      setTimeout(function () {
        try { step[1](); } catch (e) { bad.push(String(e && e.stack || e)); report('contract', bad); }
      }, at);
    });
  }

  function probePerf() {
    var t0 = performance.now(), runs = 50;
    for (var i = 0; i < runs; i++) tick();
    report('perf ' + ((performance.now() - t0) / runs).toFixed(2) + ' ms per push, ' + document.querySelectorAll('*').length + ' nodes', []);
  }

  window.MP_MOCK_LIVE = {
    send: send,
    start: function () {
      // tall=<px>: the popover frame grows so one screenshot shows a whole scrolling screen.
      if (q.has('tall')) document.body.style.height = q.get('tall') + 'px';
      if (q.has('wide')) {
        document.body.style.width = q.get('wide') + 'px';
        window.dispatchEvent(new Event('resize'));
      }
      variants();
      push();
      // Synchronous, so it also runs without --virtual-time-budget, which freezes performance.now().
      if (q.get('probe') === 'perf') probePerf();
      setTimeout(function () {
        drive();
        if (q.get('probe') === 'shift') probeShift();
        if (q.get('probe') === 'controls') probeControls();
        if (q.get('probe') === 'map') probeMap();
        if (q.get('probe') === 'cal') probeCal();
        if (q.get('probe') === 'settings') probeSettings();
        if (q.get('probe') === 'contract') probeContract();
      }, 200);
      if (!q.has('still') && !q.has('probe')) setInterval(tick, 2000);
    }
  };
})();
