// mac-pulse frontend. The state it draws and the messages it sends are the types of the Go package internal/ui.
// Classic script on purpose: ES modules do not load from file://, and the same files must open in a browser.
// Every view is built once and then patched in place (text nodes, widths, path data) on each 2 s state push.
(function () {
  'use strict';

  var params = new URLSearchParams(location.search);
  var handler = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.mp;
  var mock = !handler || params.has('mock');
  var mode = params.get('mode') === 'window' ? 'window' : 'popover';
  var wide = mode === 'window';
  var TABS = ['overview', 'processes', 'network', 'history', 'dev', 'storage', 'settings'];
  // settings.Tiles in Go: the blocks of the Overview in their default order.
  var TILE_ORDER = ['cpu', 'memory', 'network', 'disk', 'gpu', 'battery', 'sensors', 'apps', 'sleep'];
  var DETAILS = { cpu: 1, gpu: 1, memory: 1, disk: 1, network: 1, battery: 1, sensors: 1, clock: 1, alert: 1 };
  // A name from the URL must be a key of the table itself: "constructor" is on every object.
  function has(table, key) { return Object.prototype.hasOwnProperty.call(table, key); }
  var asked = params.get('tab') || '';
  // detail:alert:<id> names one alert; without an id the screen shows the newest one.
  var alertId = asked.indexOf('detail:alert:') === 0 ? asked.slice(13) : '';
  if (alertId) asked = 'detail:alert';
  // The screens of Settings in menu order, each with the key of its title. settings:<id> names one;
  // plain settings, or an id that is not here, is their menu. Go has the same list in ui.tabs.
  var SECTIONS = { general: 'set.general', appearance: 'set.appearance', menubar: 'set.menu_bar', layout: 'set.arrange', alerts: 'set.alerts', shortcuts: 'set.shortcuts' };
  var section = asked.indexOf('settings:') === 0 && has(SECTIONS, asked.slice(9)) ? asked.slice(9) : '';
  if (asked.indexOf('settings:') === 0) asked = 'settings';
  var current = TABS.indexOf(asked) >= 0 || asked.indexOf('detail:') === 0 && has(DETAILS, asked.slice(7)) ? asked : 'overview';
  var root = document.documentElement;
  var state = null;
  var settings = { temp_unit: 'C', net_unit: 'bytes', tab_order: TABS.slice(0, -1) };
  var DASH = '—';
  var SVG = 'http://www.w3.org/2000/svg';
  var SLOTS = 60;

  // Each i18n/<code>.js registers its dictionary in window.MP_I18N; English fills any gap.
  var I18N = window.MP_I18N;
  var lang = '', dict = null, plurals = null, formats = {};

  // matchLang maps BCP 47 tags to a shipped language, as settings.MatchLanguage does in Go.
  function matchLang(tags) {
    for (var i = 0; i < tags.length; i++) {
      var tag = tags[i] || '', base = tag.split('-')[0];
      if (I18N[tag]) return tag;
      if (base === 'zh') return /Hant|TW|HK|MO/.test(tag) ? 'zh-Hant' : 'zh-Hans';
      if (base === 'pt') return 'pt-BR';
      if (I18N[base]) return base;
    }
    return 'en';
  }

  // fx prints a number the way the interface language writes it: fx(1.5, 1) is "1,5" in German.
  function fx(n, digits, least) {
    var key = digits + '/' + least;
    var f = formats[key] || (formats[key] = new Intl.NumberFormat(lang,
      { minimumFractionDigits: least == null ? digits : least, maximumFractionDigits: digits, useGrouping: false }));
    return f.format(n);
  }

  // count prints a whole number with the digit grouping of the interface language: "412,380".
  function count(n) {
    return (formats.count || (formats.count = new Intl.NumberFormat(lang))).format(n);
  }

  // t looks a string up, picks the plural form for p.n and fills the {placeholders} from p.
  function t(key, p) {
    var v = dict[key];
    if (v == null) v = I18N.en[key];
    if (v == null) return key;
    if (typeof v === 'object') v = v[plurals.select(p.n)] || v.other;
    return v.replace(/\{(\w+)\}/g, function (m, name) {
      var x = p && p[name];
      return x == null ? m : typeof x === 'number' ? fx(x, 2, 0) : x;
    });
  }

  // named translates a label Go made up (a sensor group, a core cluster); a name the system gave stays as it is.
  function named(prefix, raw) { return I18N.en[prefix + raw] ? t(prefix + raw) : raw; }

  function dateFormat(key, options) {
    return formats[key] || (formats[key] = new Intl.DateTimeFormat(lang, options));
  }

  var ICONS = {
    cpu: 'M5.5 4h5A1.5 1.5 0 0 1 12 5.5v5a1.5 1.5 0 0 1-1.5 1.5h-5A1.5 1.5 0 0 1 4 10.5v-5A1.5 1.5 0 0 1 5.5 4ZM6.5 6.5h3v3h-3ZM6 2v2M10 2v2M6 12v2M10 12v2M2 6h2M2 10h2M12 6h2M12 10h2',
    memory: 'M3 4.5h10a1 1 0 0 1 1 1v4a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1v-4a1 1 0 0 1 1-1ZM5 10.5v2M8 10.5v2M11 10.5v2M5.5 7v1M8 7v1M10.5 7v1',
    network: 'M8 2a6 6 0 1 0 0 12A6 6 0 0 0 8 2ZM2 8h12M8 2c2 1.8 2.6 3.8 2.6 6S10 12.2 8 14M8 2C6 3.8 5.4 5.8 5.4 8S6 12.2 8 14',
    disk: 'M4.2 3.5h7.6a1 1 0 0 1 .95.68L14 8v3.5a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V8l1.25-3.82a1 1 0 0 1 .95-.68ZM2 8h12M11.5 10.25h.01',
    gpu: 'M8 2.5 14 5.5 8 8.5 2 5.5ZM2.6 8.3 8 11l5.4-2.7M2.6 10.8 8 13.5l5.4-2.7',
    battery: 'M3 5h8.5a1 1 0 0 1 1 1v4a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1ZM14.25 7v2M4.5 7.25v1.5M6.75 7.25v1.5',
    system: 'M12.76 6.84L14.33 7.04L14.33 8.96L12.76 9.16L12.19 10.54L13.15 11.80L11.80 13.15L10.54 12.19L9.16 12.76L8.96 14.33L7.04 14.33L6.84 12.76L5.46 12.19L4.20 13.15L2.85 11.80L3.81 10.54L3.24 9.16L1.67 8.96L1.67 7.04L3.24 6.84L3.81 5.46L2.85 4.20L4.20 2.85L5.46 3.81L6.84 3.24L7.04 1.67L8.96 1.67L9.16 3.24L10.54 3.81L11.80 2.85L13.15 4.20L12.19 5.46ZM8 6a2 2 0 1 0 0 4a2 2 0 0 0 0-4Z',
    window: 'M6.5 3.5h-2a1 1 0 0 0-1 1v7a1 1 0 0 0 1 1h7a1 1 0 0 0 1-1v-2M9 3h4v4M13 3 7.5 8.5',
    panel: 'M6.5 3.5h-2a1 1 0 0 0-1 1v7a1 1 0 0 0 1 1h7a1 1 0 0 0 1-1v-2M12 8.5H7.5V4M7.5 8.5 13 3',
    search: 'M7 2.5a4.5 4.5 0 1 0 0 9a4.5 4.5 0 0 0 0-9ZM10.3 10.3 13.5 13.5',
    chevron: 'M6 3.5 10.5 8 6 12.5',
    close: 'M4 4l8 8M12 4l-8 8',
    down: 'M8 2.5v11M3.5 9 8 13.5 12.5 9',
    up: 'M8 13.5v-11M3.5 7 8 2.5 12.5 7',
    alert: 'M8 2.2 14.3 13H1.7ZM8 6.5v3M8 11.2h.01',
    info: 'M8 2a6 6 0 1 0 0 12A6 6 0 0 0 8 2ZM8 7.5v3.5M8 5h.01',
    help: 'M8 2a6 6 0 1 0 0 12A6 6 0 0 0 8 2ZM6.2 6.5a1.8 1.8 0 1 1 2.8 1.5c-.6.4-1 .8-1 1.5M8 11.3h.01',
    moon: 'M13 9.5A5.5 5.5 0 0 1 6.5 3a5.5 5.5 0 1 0 6.5 6.5Z',
    app: 'M4.5 2.5h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2h-7a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2ZM5.5 6l2 2-2 2M9 10h1.5',
    sort: 'M4 6l4 4 4-4',
    sensors: 'M9.5 9.2V3.5a1.5 1.5 0 0 0-3 0v5.7a3 3 0 1 0 3 0ZM8 6.5v4.3',
    fan: 'M8 6.6a1.4 1.4 0 1 0 0 2.8a1.4 1.4 0 0 0 0-2.8ZM8 6.6C7 4.4 8 2.4 10.2 2.6c.4 1.8-.6 3.2-2.2 4ZM9.4 8c2.2-1 4.2 0 4 2.2c-1.8.4-3.2-.6-4-2.2ZM8 9.4c1 2.2 0 4.2-2.2 4c-.4-1.8.6-3.2 2.2-4ZM6.6 8c-2.2 1-4.2 0-4-2.2c1.8-.4 3.2.6 4 2.2Z',
    bolt: 'M9 2 4 9h3.5L7 14l5-7H8.5Z',
    pin: 'M6 2.5h4l-.5 3.7 2 2.3h-7l2-2.3ZM8 8.5v5',
    swap: 'M3.5 5.5h8.5M9.5 3 12 5.5 9.5 8M12.5 10.5H4M6.5 8 4 10.5 6.5 13',
    mic: 'M8 2a2 2 0 0 0-2 2v4a2 2 0 0 0 4 0V4a2 2 0 0 0-2-2ZM4 7.5a4 4 0 0 0 8 0M8 11.5V14M6 14h4',
    camera: 'M3 4.5h6.5a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1v-5a1 1 0 0 1 1-1ZM10.5 7.2 14 5v6l-3.5-2.2',
    folder: 'M2 4.5a1 1 0 0 1 1-1h3l1.5 1.5H13a1 1 0 0 1 1 1v5.5a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1Z',
    file: 'M4.5 2H9l3 3v8a1 1 0 0 1-1 1H4.5a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1ZM9 2v3h3',
    appearance: 'M8 2a6 6 0 1 0 0 12A6 6 0 0 0 8 2ZM8 2v12M8 5l4.9-.4M8 8h6M8 11l4.9.4',
    menubar: 'M3 3.5h10a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1ZM2 6.5h12M10 5h2',
    layout: 'M2.5 2.5H7V7H2.5ZM9 2.5h4.5V7H9ZM2.5 9H7v4.5H2.5ZM9 9h4.5v4.5H9Z',
    bell: 'M8 2.5A3.5 3.5 0 0 0 4.5 6v2.5L3 11h10l-1.5-2.5V6A3.5 3.5 0 0 0 8 2.5ZM6.8 13a1.3 1.3 0 0 0 2.4 0',
    keys: 'M3 4.5h10a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1v-5a1 1 0 0 1 1-1ZM4.5 7h.01M7 7h.01M9.5 7h.01M12 7h.01M5.5 9.5h5'
  };

  function h(tag, cls, text) {
    var el = document.createElement(tag);
    if (cls) el.className = cls;
    if (text != null) el.textContent = text;
    return el;
  }

  function add(parent) {
    for (var i = 1; i < arguments.length; i++) if (arguments[i]) parent.appendChild(arguments[i]);
    return parent;
  }

  // put, css and attr skip the DOM write when the value has not changed since the last tick.
  function put(node, text) {
    if (node.$t !== text) { node.$t = text; node.textContent = text; }
  }

  function css(node, prop, value) {
    var key = '$' + prop;
    if (node[key] !== value) { node[key] = value; node.style[prop] = value; }
  }

  function attr(node, name, value) {
    var key = '$' + name;
    if (node[key] !== value) { node[key] = value; node.setAttribute(name, value); }
  }

  function icon(name, cls) {
    var svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('viewBox', '0 0 16 16');
    svg.setAttribute('class', 'i' + (cls ? ' ' + cls : ''));
    svg.setAttribute('aria-hidden', 'true');
    var path = document.createElementNS(SVG, 'path');
    path.setAttribute('d', ICONS[name]);
    svg.appendChild(path);
    return svg;
  }

  // "self" is mac-pulse's own row: the embedded icon.png, which mock mode loads straight from disk.
  // A system process outside any bundle gets the gear instead of the generic app placeholder.
  function appIcon(path, system) {
    var glyph = system ? 'system' : 'app';
    var url = !path ? '' : path === 'self' ? (mock ? 'icon.png' : 'icon?path=self')
      : mock ? (window.MP_MOCK_ICONS || {})[path] || '' : 'icon?path=' + encodeURIComponent(path);
    if (!url) return icon(glyph, 'app-i');
    var img = h('img', 'app-i');
    img.alt = '';
    img.onerror = function () { img.replaceWith(icon(glyph, 'app-i')); };
    img.src = url;
    return img;
  }

  function scale(n, base, units) {
    var i = 0;
    while (n >= base && i < units.length - 1) { n /= base; i++; }
    return [i === 0 || n >= 99.95 ? fx(n, 0) : fx(n, 1, 0), units[i]];
  }

  // Memory counts in 1024s like Activity Monitor; disk callers pass 1000 to match Finder and Disk Utility.
  function bytes(n, base) {
    return n == null ? [DASH, ''] : scale(n, base || 1024, ['B', 'KB', 'MB', 'GB', 'TB']);
  }

  // Only network rates follow the bits setting (disk callers pass 'disk'); totals stay in bytes,
  // the unit data volumes are sold in.
  function rate(n, unit) {
    if (n == null) return [DASH, ''];
    return (unit || settings.net_unit) === 'bits'
      ? scale(n * 8, 1000, ['b/s', 'kb/s', 'Mb/s', 'Gb/s'])
      : scale(n, unit === 'disk' ? 1000 : 1024, ['B/s', 'KB/s', 'MB/s', 'GB/s']);
  }

  function join(pair) { return pair[1] ? pair[0] + ' ' + pair[1] : pair[0]; }

  function temp(c) {
    if (c == null) return DASH;
    return settings.temp_unit === 'F' ? Math.round(c * 9 / 5 + 32) + ' °F' : Math.round(c) + ' °C';
  }

  function dur(s) {
    if (s == null) return DASH;
    var m = Math.floor(s / 60), hr = Math.floor(m / 60), d = Math.floor(hr / 24);
    if (d) return t('dur.dh', { d: d, h: hr % 24 });
    if (hr) return t('dur.hm', { h: hr, m: m % 60 });
    return m ? t('dur.m', { m: m }) : t('dur.s', { s: Math.floor(s) });
  }

  function pct(v) { return Math.round(v) + '%'; }

  function watts(v) { return v == null ? DASH : fx(v, v < 10 ? 2 : 1) + ' W'; }

  function mhz(v) { return v == null ? DASH : v >= 1000 ? fx(v / 1000, 2) + ' GHz' : Math.round(v) + ' MHz'; }

  function percent(v) { return fx(v, 1) + '%'; }

  // The 24-hour clock in every language: four time labels share the width of a chart.
  function clock(ms) { return dateFormat('clock', { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(ms); }

  function day(ms) { return dateFormat('day', { month: 'short', day: 'numeric' }).format(ms); }

  function dateTime(ms) {
    return dateFormat('date_time', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).format(ms);
  }

  var THERMAL_STATES = ['nominal', 'fair', 'serious', 'critical'];

  // measure writes a value of an alert's detail in its unit.
  function measure(unit, v) {
    if (v == null) return DASH;
    // A disk at 9.4% free against a limit of 10% must not read "10%".
    if (unit === 'percent') return v < 20 && v % 1 ? percent(v) : pct(v);
    if (unit === 'bytes') return join(bytes(v));
    if (unit === 'bytes_per_s') return join(rate(v, 'disk'));
    if (unit === 'celsius') return temp(v);
    if (unit === 'watts') return watts(v);
    if (unit === 'milliwatts') return Math.round(v) + ' mW';
    if (unit === 'seconds') return dur(v);
    return t('thermal.' + (THERMAL_STATES[v] || 'critical'));
  }

  function alertText(a) {
    return [t('alert.' + a.kind + '.title', a.params), t('alert.' + a.kind + '.detail', a.params)];
  }

  // sync keeps one DOM row per item key, in item order, so hover, expansion and confirm state survive a tick.
  function sync(parent, items, keyOf, create, update) {
    var rows = parent.$rows || (parent.$rows = new Map());
    var seen = new Set();
    items.forEach(function (item, i) {
      var key = keyOf(item);
      if (seen.has(key)) key += '#' + i;
      seen.add(key);
      var row = rows.get(key);
      if (!row) { row = create(item); rows.set(key, row); }
      update(row, item);
      if (parent.children[i] !== row) parent.insertBefore(row, parent.children[i] || null);
    });
    rows.forEach(function (row, key) {
      if (!seen.has(key)) { row.remove(); rows.delete(key); }
    });
  }

  function send(msg) {
    if (!mock) { handler.postMessage(msg); return; }
    console.log('→ go', JSON.stringify(msg));
    if (window.MP_MOCK_LIVE) window.MP_MOCK_LIVE.send(msg);
  }

  function set(key, value) { send({ type: 'set', key: key, value: value }); }

  // cols makes the group a grid: the up and down arrows then move by a row.
  function arrowKeys(el, cols) {
    el.addEventListener('keydown', function (e) {
      var step = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -cols, ArrowDown: cols }[e.key];
      if (!step) return;
      var buttons = Array.prototype.filter.call(el.children, function (b) { return !b.hidden; });
      var next = buttons[buttons.indexOf(document.activeElement) + step];
      if (next) { next.focus(); next.click(); }
      e.preventDefault();
    });
  }

  // One explanation is open at a time. Its "?" toggles it; a click elsewhere, Esc, a scroll or
  // another screen closes it.
  var pop = null;

  function closePop() {
    if (!pop) return false;
    pop.el.remove();
    pop.btn.setAttribute('aria-expanded', 'false');
    pop = null;
    return true;
  }

  // Under its button and inside the panel; above the button when there is no room below.
  function placePop() {
    var r = pop.btn.getBoundingClientRect(), b = document.body.getBoundingClientRect(), w = pop.el.offsetWidth, tall = pop.el.offsetHeight;
    var top = r.bottom - b.top + 6;
    if (top + tall > b.height - 8) top = Math.max(8, r.top - b.top - tall - 6);
    pop.el.style.left = Math.max(8, Math.min(r.right - b.left - w + 8, b.width - w - 8)) + 'px';
    pop.el.style.top = Math.min(top, Math.max(8, b.height - tall - 8)) + 'px';
  }

  // help is a "?" button; fill writes the explanation into the popover each time it opens.
  function help(label, fill) {
    var b = h('button', 'help');
    b.appendChild(icon('help'));
    b.setAttribute('aria-label', label);
    b.setAttribute('aria-expanded', 'false');
    b.title = label;
    b.onclick = function (e) {
      // The button may sit in a row or a label that has a click of its own.
      e.stopPropagation();
      e.preventDefault();
      var again = pop && pop.btn === b;
      closePop();
      if (again) return;
      pop = { el: h('div', 'help-pop'), btn: b };
      pop.el.setAttribute('role', 'status');
      fill(pop.el);
      document.body.appendChild(pop.el);
      b.setAttribute('aria-expanded', 'true');
      placePop();
    };
    return b;
  }

  // A hint whose text is fixed: one paragraph per dictionary key, or per [term, key] pair.
  function hintOf(title, lines) {
    return help(t('hint.about', { name: title }), function (el) {
      lines.forEach(function (line) {
        if (typeof line === 'string') add(el, h('p', '', t(line)));
        else add(el, add(h('p'), h('b', '', t(line[0])), document.createTextNode(' — ' + t(line[1]))));
      });
    });
  }

  var SLEEP_HINT = ['sleep.help', ['sleep.display', 'sleep.help_display'], ['sleep.system', 'sleep.help_system'], 'sleep.help_normal'];

  // The "?" of an app or of one of its processes (pid; 0 is the app itself). Go reads the bundle,
  // the signature and the manual on demand, so the popover opens at once and fills when the answer comes.
  // proc gives the process whose details and signals the popover offers: the row's own, or for an
  // app row the app's main process. They are for this user's own processes only.
  function infoButton(appName, pid, proc) {
    return help('', function (el) {
      pop.want = { name: appName(), pid: pid, proc: proc && proc() };
      add(el, h('p', 'dim', t('info.loading')));
      send({ type: 'app_info', name: pop.want.name, pid: pid });
    });
  }

  function showInfo(r) {
    var i = r.info;
    if (!pop || !pop.want || !i || pop.want.name !== i.name || pop.want.pid !== i.pid) return;
    var el = pop.el;
    el.textContent = '';
    if (!r.ok) { add(el, h('p', 'dim', r.text)); return; }
    add(el, add(h('div', 'help-h'), h('b', '', i.title), i.version && h('span', '', i.version)));
    // A bundle says what it is by its own facts; a bare executable nobody described says so plainly.
    // The line of a man page is what the program says about itself, not what mac-pulse knows: it is labelled so.
    if (i.desc_key) add(el, h('p', '', t(i.desc_key)));
    else if (i.manual) add(el, add(h('p'), h('span', 'dim', t('info.manual') + ': '), document.createTextNode(i.manual)));
    else if (!i.bundle_id) add(el, h('p', 'dim', t('info.no_desc')));
    var sign = { apple: t('info.sign_apple'), developer: t('info.signed_by', { name: i.signer }), adhoc: t('info.sign_adhoc'), none: t('info.sign_none'),
      unverified: t('info.sign_unverified') };
    var since = i.since && dateFormat('since', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(i.since);
    var list = h('dl');
    // What the bundle calls itself is its own claim: the heading stays the name of the folder on disk.
    [['info.bundle_name', i.bundle_name !== i.title && i.bundle_name], ['info.category', i.category && named('cat.', i.category)],
      ['info.developer', i.developer], ['info.bundle_id', i.bundle_id],
      ['info.signature', (sign[i.signing] || t('info.sign_unknown')) + (i.executable_only ? ' · ' + t('info.exec_only') : '')], ['info.parent', i.parent], ['info.since', since],
      ['info.user', i.user], ['info.path', i.path]].forEach(function (f) {
      if (f[1]) add(list, h('dt', '', t(f[0])), h('dd', '', f[1]));
    });
    add(el, list, h('p', 'dim', t(i.killable ? 'info.can_quit' : 'info.cannot_quit')));
    // Go reads the details of this user's processes only: a system process it may not quit is somebody else's.
    var p = pop.want.proc;
    if (p && (p.killable || !p.system)) {
      var more = h('button', 'btn sm', t('details'));
      more.onclick = function () { askDetail(false); };
      pop.detail = add(h('div', 'detail-box'), more);
      el.appendChild(pop.detail);
      if (pop.want.detail) askDetail(false);
    }
    placePop();
  }

  // The command line is read only when its button is pressed, and neither Go nor the page keeps it.
  function askDetail(args) {
    pop.detail.textContent = '';
    add(pop.detail, h('p', 'dim', t('info.loading')));
    send({ type: 'proc_detail', name: pop.want.name, pid: pop.want.proc.pid, args: args });
    placePop();
  }

  var SIGNALS = ['TERM', 'KILL', 'HUP', 'INT', 'STOP', 'CONT'];

  function showDetail(r) {
    if (!pop || !pop.detail) return;
    var box = pop.detail, d = r.detail, want = pop.want;
    box.textContent = '';
    if (!r.ok) { add(box, h('p', 'dim', r.text)); placePop(); return; }
    add(box, add(h('dl'), h('dt', '', t('det.threads')), h('dd', '', String(d.threads)),
      h('dt', '', t('det.chain')), h('dd', '', d.chain.join(' ← ') || DASH)));
    [['det.files', d.files], ['det.sockets', d.sockets]].forEach(function (f) {
      add(box, h('p', 'dim', t(f[0]) + ': ' + f[1].length), f[1].length && h('pre', '', f[1].join('\n')));
    });
    if (d.more) add(box, h('p', 'dim', t('det.more', { n: d.more })));
    if (d.args.length) add(box, h('p', 'dim', t('det.args_label') + ':'), h('pre', '', d.args.join(' ')));
    else {
      var line = h('button', 'btn sm', t('det.args'));
      line.onclick = function () { askDetail(true); };
      add(box, add(h('p'), line));
    }
    // A signal goes the way Quit does: Go checks the pid again on a fresh process scan before it sends anything.
    var signals = h('p');
    function menu() {
      var pick = add(h('select', 'field'), new Option(t('sig.label'), ''));
      SIGNALS.forEach(function (name) { pick.appendChild(new Option(name + ' — ' + t('sig.' + name), name)); });
      pick.onchange = function () {
        var name = pick.value;
        function fire() {
          send({ type: 'quit_app', app: want.name, pids: [want.proc.pid], signal: name });
          closePop();
        }
        if (name === 'CONT') { fire(); return; }
        var cancel = h('button', 'btn sm', t('cancel')), ok = h('button', 'btn sm danger', t('sig.send'));
        cancel.onclick = menu;
        ok.onclick = fire;
        signals.textContent = '';
        add(signals, h('b', '', t('sig.ask', { signal: name, name: want.proc.name + ' (' + want.proc.pid + ')' })), add(h('span', 'btns'), cancel, ok));
        cancel.focus();
        placePop();
      };
      signals.textContent = '';
      signals.appendChild(pick);
    }
    if (want.proc.killable) { menu(); box.appendChild(signals); }
    placePop();
  }

  function segmented(items, onPick, label) {
    var el = h('div', 'seg sm');
    el.setAttribute('role', 'tablist');
    el.setAttribute('aria-label', label);
    var buttons = items.map(function (item) {
      var b = h('button', '', item[1]);
      b.setAttribute('role', 'tab');
      b.onclick = function () { onPick(item[0]); };
      el.appendChild(b);
      return b;
    });
    arrowKeys(el);
    return {
      el: el,
      buttons: buttons,
      set: function (value) {
        buttons.forEach(function (b, i) {
          var on = items[i][0] === value;
          attr(b, 'aria-selected', String(on));
          b.tabIndex = on ? 0 : -1;
        });
      }
    };
  }

  function tile(cls, iconName, label) {
    var el = h('section', 'tile ' + cls);
    var side = h('span', 'side');
    el.appendChild(add(h('header', 'tile-h'), icon(iconName), h('span', '', label), side));
    return { el: el, side: side };
  }

  // A tile or strip that opens a detail screen: the whole surface is one button, like a Control Center module.
  function opens(el, name) {
    el.classList.add('go');
    el.setAttribute('role', 'button');
    el.tabIndex = 0;
    el.setAttribute('aria-label', t('details_of', { name: t('m.' + name) }));
    el.dataset.go = 'detail:' + name;
    el.onclick = function () { show('detail:' + name); };
    el.onkeydown = function (e) {
      if (e.target !== el || (e.key !== 'Enter' && e.key !== ' ')) return;
      e.preventDefault();
      show('detail:' + name);
    };
    return icon('chevron', 'go-i');
  }

  function bigValue() {
    var num = h('span', 'num', DASH), unit = h('span', 'unit'), side = h('span', 'side');
    return { el: add(h('div', 'big'), num, unit, side), num: num, unit: unit, side: side };
  }

  function meter(parts) {
    var el = h('div', 'meter'), segs = [];
    for (var i = 0; i < parts; i++) segs.push(el.appendChild(h('i')));
    return { el: el, segs: segs };
  }

  function fill(seg, percent) {
    var w = Math.max(0, Math.min(100, percent));
    seg.hidden = w < 0.5;
    css(seg, 'width', w.toFixed(1) + '%');
  }

  function keyed(label, cls) {
    var value = h('b', '', DASH);
    return { el: add(h('div', 'kv'), h('span', 'key ' + cls, label), value), value: value };
  }

  // Sparklines hold up to 60 points and hug the right edge while the history is still filling up.
  function sparkPath(points, max, top, base, closed) {
    if (points.length < 2) return '';
    var step = 100 / (SLOTS - 1), x0 = 100 - (points.length - 1) * step, d = '';
    for (var i = 0; i < points.length; i++) {
      var y = base + (top - base) * Math.min(points[i] / max, 1);
      d += (i ? 'L' : 'M') + (x0 + i * step).toFixed(2) + ' ' + y.toFixed(2);
    }
    return closed ? d + 'L100 ' + base + 'L' + x0.toFixed(2) + ' ' + base + 'Z' : d;
  }

  function sparkSvg(height) {
    var svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('class', 'spark');
    svg.setAttribute('viewBox', '0 0 100 ' + height);
    svg.setAttribute('preserveAspectRatio', 'none');
    svg.setAttribute('aria-hidden', 'true');
    return svg;
  }

  function sparkPart(svg, cls) {
    var area = document.createElementNS(SVG, 'path'), line = document.createElementNS(SVG, 'path');
    area.setAttribute('class', 'area ' + cls);
    line.setAttribute('class', 'line ' + cls);
    svg.appendChild(area);
    svg.appendChild(line);
    return function (points, max, top, base) {
      attr(area, 'd', sparkPath(points, max, top, base, true));
      attr(line, 'd', sparkPath(points, max, top, base, false));
    };
  }

  // The svg fills a sized box: left in the flow, its viewBox ratio would dictate the tile height.
  function sparkBox(svg) { return add(h('div', 'spark-box'), svg); }

  function spark(height) {
    var svg = sparkSvg(height), axis = document.createElementNS(SVG, 'path');
    axis.setAttribute('class', 'axis');
    axis.setAttribute('d', 'M0 ' + (height - 0.5) + 'H100');
    svg.appendChild(axis);
    var draw = sparkPart(svg, '');
    return { el: sparkBox(svg), set: function (points, max) { draw(points, max, 1, height - 0.5); } };
  }

  // The two halves scale independently (upload would be a flat line next to download); no axis is shown.
  function mirror(height) {
    var svg = sparkSvg(height), mid = height / 2;
    var axis = document.createElementNS(SVG, 'path');
    axis.setAttribute('class', 'axis');
    axis.setAttribute('d', 'M0 ' + mid + 'H100');
    svg.appendChild(axis);
    var down = sparkPart(svg, 'down'), up = sparkPart(svg, 'up');
    return {
      el: sparkBox(svg),
      set: function (downs, ups) {
        down(downs, Math.max.apply(null, downs.concat(1024)), 1, mid - 0.5);
        up(ups, Math.max.apply(null, ups.concat(1024)), height - 1, mid + 0.5);
      }
    };
  }

  function buildOverview() {
    var el = h('div', 'tab scroll');
    // The newest alert has a banner; the others wait behind one row, so the tiles stay in sight.
    var alerts = h('div', 'alerts'), moreAlerts = h('button', 'btn link sm alerts-more'), allAlerts = false;
    moreAlerts.onclick = function () {
      allAlerts = !allAlerts;
      update(state);
    };
    var grid = h('div', 'grid');
    // What macOS tells without the Camera or Microphone permission: who records, and that the camera is on.
    var micText = h('span'), micRow = add(h('div'), icon('mic'), micText);
    var camRow = add(h('div'), icon('camera'), h('span', '', t('media.camera')), hintOf(t('media.camera'), ['media.camera_hint']));
    var inUse = add(h('section', 'tile in-use'), micRow, camRow);
    inUse.setAttribute('aria-label', t('media.title'));

    var cpu = tile('t-cpu', 'cpu', t('m.cpu')), cpuBig = bigValue(), cpuSpark = spark(26);
    var cores = h('div', 'cores'), split = meter(2);
    var user = h('b', '', DASH), sys = h('b', '', DASH), coreTemp = h('b', '', DASH), chipTemp = h('b', '', DASH);
    var coreCell = add(h('span', 'heat'), document.createTextNode(t('cpu.hottest') + ' '), coreTemp);
    var chipCell = add(h('span', 'heat'), document.createTextNode(t('cpu.temp') + ' '), chipTemp);
    chipCell.title = t('cpu.temp_tip');
    add(cpu.el, cpuBig.el, cpuSpark.el, cores, split.el, add(h('div', 'cap'),
      add(h('span', 'key'), document.createTextNode(t('cpu.user')), user),
      add(h('span', 'key k2'), document.createTextNode(t('cpu.system')), sys), coreCell, chipCell));
    cores.setAttribute('role', 'img');

    var mem = tile('t-mem', 'memory', t('m.memory')), memBig = bigValue(), memBar = meter(4);
    var pressure = h('span', 'pill');
    mem.side.replaceWith(pressure);
    var parts = [keyed(t('mem.app'), 'k1'), keyed(t('mem.wired'), 'k2'), keyed(t('mem.compressed'), 'k3'), keyed(t('mem.cached'), 'k4')];
    var swap = keyed(t('mem.swap'), 'ring');
    add(mem.el, memBig.el, memBar.el);
    parts.concat(swap).forEach(function (p) { mem.el.appendChild(p.el); });

    var net = tile('t-net', 'network', t('m.network')), netSpark = mirror(40);
    var down = rateLine('down'), up = rateLine('up');
    var downToday = h('b', '', DASH), upToday = h('b', '', DASH);
    add(net.el, add(h('div', 'rates'), add(h('div'), down.el, up.el), netSpark.el),
      add(h('div', 'cap'), h('span', '', t('today')), add(h('span', 'cap end'), icon('down'), downToday, icon('up'), upToday)));

    var disk = tile('t-disk', 'disk', t('m.disk')), diskBig = bigValue(), diskBar = meter(1);
    var read = h('b', '', DASH), write = h('b', '', DASH);
    add(disk.el, diskBig.el, diskBar.el, add(h('div', 'cap'),
      add(h('span'), document.createTextNode(t('disk.r') + ' '), read), add(h('span', 'end'), document.createTextNode(t('disk.w') + ' '), write)));

    var gpu = tile('t-gpu', 'gpu', t('m.gpu')), gpuBig = bigValue(), gpuSpark = spark(30);
    add(gpu.el, gpuBig.el, gpuSpark.el);

    var bat = tile('t-bat', 'battery', t('m.battery')), batBig = bigValue(), batBar = meter(1);
    var lowPower = h('span', 'pill', t('bat.low_power'));
    lowPower.dataset.level = 'warning';
    lowPower.title = t('bat.low_power_mode');
    bat.side.before(lowPower);
    var power = h('b', '', DASH), health = h('b', '', DASH), cycles = h('b', '', DASH);
    add(bat.el, batBig.el, batBar.el, add(h('div', 'cap'), power,
      add(h('span'), document.createTextNode(t('bat.health_short') + ' '), health), add(h('span', 'end'), document.createTextNode(t('bat.cycles_short') + ' '), cycles)));

    [[cpu, 'cpu'], [mem, 'memory'], [net, 'network'], [disk, 'disk'], [gpu, 'gpu'], [bat, 'battery']].forEach(function (t) {
      t[0].el.firstChild.appendChild(opens(t[0].el, t[1]));
    });

    var strip = h('section', 'tile strip'), thermal = h('span', 'pill');
    var stripTemp = h('b', '', DASH), stripFans = h('b', '', DASH), stripPower = h('b', '', DASH);
    var coreIcon = icon('cpu'), tempCell = add(h('span', 'cap'), coreIcon, stripTemp);
    var fansCell = add(h('span', 'cap'), icon('fan'), stripFans), powerCell = add(h('span', 'cap'), icon('bolt'), stripPower);
    add(strip, icon('sensors'), h('span', 'lbl', t('m.sensors')), thermal, tempCell, fansCell, powerCell, opens(strip, 'sensors'));

    var apps = h('section', 'tile top-apps'), appList = h('div', 'list');
    add(apps, add(h('div', 'row head'), h('span', 'title', t('top_apps')), h('span', 'th r', t('m.cpu')), h('span', 'th r', t('m.memory'))), appList);

    var sleep = tile('sleep', 'moon', t('sleep.title')), sleepList = h('div', 'list');
    sleep.el.firstChild.appendChild(hintOf(t('sleep.title'), SLEEP_HINT));
    sleep.el.appendChild(sleepList);
    var foot = h('div', 'foot');
    // One grid in the order of settings.tile_order: the six metric tiles take a column each,
    // the sensors strip, the top apps and the sleep list a whole row.
    var TILES = { cpu: cpu.el, memory: mem.el, network: net.el, disk: disk.el, gpu: gpu.el, battery: bat.el, sensors: strip, apps: apps, sleep: sleep.el };
    var ROW = { sensors: true, apps: true, sleep: true }, placed = '';
    Object.keys(TILES).forEach(function (id) { grid.appendChild(TILES[id]); });
    add(el, alerts, moreAlerts, inUse, grid, foot);

    function rateLine(dir) {
      var num = h('span', 'num', DASH), unit = h('span', 'unit');
      return { el: add(h('div', 'rate ' + dir), icon(dir), num, unit), num: num, unit: unit };
    }

    function pair(target, value) {
      put(target.num, value[0]);
      put(target.unit, value[1]);
    }

    function update(s) {
      var active = s.alerts.active.slice().reverse(), extra = active.length - 1;
      moreAlerts.hidden = extra < 1;
      put(moreAlerts, allAlerts ? t('alerts.less') : t('alerts.more', { n: extra }));
      attr(moreAlerts, 'aria-expanded', String(allAlerts));
      sync(alerts, allAlerts ? active : active.slice(0, 1), function (a) { return a.id; }, function () {
        var row = h('div', 'alert'), title = h('b'), detail = h('span'), since = h('time'), tops = h('div', 'tops');
        row.$ = { title: title, detail: detail, since: since, tops: tops, open: opensAlert(row), mute: muteButton(row) };
        return add(row, row.$.open, icon('alert'), title, detail, row.$.mute, since, icon('chevron', 'go-i'), tops);
      }, function (row, a) {
        row.$.app = alertApp(a);
        row.$.id = a.id;
        row.$.mute.hidden = !row.$.app || s.settings.alert_muted.indexOf(a.app) >= 0;
        var text = alertText(a);
        put(row.$.title, text[0]);
        put(row.$.detail, text[1]);
        attr(row, 'title', text.join(' — '));
        attr(row.$.open, 'aria-label', t('details_of', { name: text[0] }));
        put(row.$.since, dur(Math.max(0, s.time - a.since) / 1000));
        // What keeps the fans on: under a thermal alert, the three heaviest apps, each a link into Apps.
        var hot = a.kind === 'thermal' ? s.apps.items.slice(0, 3) : [];
        row.classList.toggle('wrap', hot.length > 0);
        row.$.tops.hidden = !hot.length;
        sync(row.$.tops, hot, function (x) { return x.name; }, function () {
          var b = h('button', 'btn link sm');
          b.onclick = function () { findApp(b.$name); };
          return b;
        }, function (b, x) {
          b.$name = x.name;
          put(b, x.name + ' ' + pct(x.cpu));
        });
      });
      alerts.hidden = !s.alerts.active.length;

      var c = s.cpu;
      var core = hottest(c);
      coreCell.hidden = core == null;
      chipCell.hidden = c.temp_c == null;
      put(coreTemp, temp(core));
      put(chipTemp, temp(c.temp_c));
      pair(cpuBig, [String(Math.round(c.total)), '%']);
      put(cpuBig.side, t('cpu.load', { v: fx(c.load[0], 2) }));
      cpuSpark.set(s.spark.cpu, 100);
      while (cores.children.length < c.cores.length) cores.appendChild(h('b')).appendChild(h('i'));
      while (cores.children.length > c.cores.length) cores.lastChild.remove();
      c.cores.forEach(function (v, i) { css(cores.children[i].firstChild, 'height', Math.round(v) + '%'); });
      attr(cores, 'aria-label', t('n.core', { n: c.cores.length }) + ': ' + c.cores.map(pct).join(', '));
      fill(split.segs[0], c.user);
      fill(split.segs[1], c.system);
      put(user, pct(c.user));
      put(sys, pct(c.system));

      var m = s.memory;
      pair(memBig, bytes(m.used));
      put(memBig.side, t('of', { total: join(bytes(m.total)) }));
      put(pressure, t('pressure.' + m.pressure));
      attr(pressure, 'data-level', m.pressure);
      attr(pressure, 'title', t('mem.pressure_tip', { level: t('pressure.' + m.pressure) }));
      [m.app, m.wired, m.compressed, m.cached].forEach(function (v, i) {
        fill(memBar.segs[i], v / m.total * 100);
        put(parts[i].value, join(bytes(v)));
      });
      put(swap.value, join(bytes(m.swap_used)));
      attr(swap.el, 'title', t('mem.swap') + ': ' + t('x_of_y', { x: join(bytes(m.swap_used)), y: join(bytes(m.swap_total)) }));

      pair(down, rate(s.has_rates ? s.network.down_rate : null));
      pair(up, rate(s.has_rates ? s.network.up_rate : null));
      netSpark.set(s.spark.net_down, s.spark.net_up);
      put(downToday, join(bytes(s.network.down_today)));
      put(upToday, join(bytes(s.network.up_today)));

      var d = s.disk, freePct = d.total ? d.free / d.total * 100 : 100;
      pair(diskBig, bytes(d.free, 1000));
      put(diskBig.side, t('disk.free_tile', { total: join(bytes(d.total, 1000)) }));
      put(disk.side, t('today_n', { x: join(bytes(d.written_today, 1000)) }));
      attr(disk.side, 'title', t('disk.written_tip'));
      fill(diskBar.segs[0], 100 - freePct);
      attr(diskBar.el, 'data-level', freePct < s.settings.alert_disk_free / 2 ? 'crit' : freePct < s.settings.alert_disk_free ? 'warn' : '');
      put(read, join(rate(s.has_rates ? d.read_rate : null, 'disk')));
      put(write, join(rate(s.has_rates ? d.write_rate : null, 'disk')));

      var media = s.media;
      inUse.hidden = !media.mic.active && !media.camera.active;
      micRow.hidden = !media.mic.active;
      camRow.hidden = !media.camera.active;
      put(micText, media.mic.apps.length ? t('media.mic', { apps: media.mic.apps.join(', ') }) : t('media.mic_on'));

      if (s.gpu) {
        pair(gpuBig, [String(Math.round(s.gpu.util)), '%']);
        put(gpuBig.side, t('gpu.memory', { x: join(bytes(s.gpu.memory)) }));
        gpuSpark.set(s.spark.gpu, 100);
      }

      var b = s.battery;
      if (b) {
        var left = b.time_remaining_s;
        // The pill takes the place of the state: both do not fit the header of a half-width tile.
        lowPower.hidden = !b.low_power;
        bat.side.hidden = b.low_power;
        put(bat.side, t('bat.' + b.state));
        pair(batBig, [String(b.percent), '%']);
        put(batBig.side, b.state === 'plugged' ? '' : left == null ? t('bat.estimating') : t(b.state === 'charging' ? 'bat.to_full' : 'bat.left', { t: dur(left) }));
        fill(batBar.segs[0], b.percent);
        attr(batBar.el, 'data-level', b.state !== 'battery' ? '' : b.percent <= 10 ? 'crit' : b.percent <= 20 ? 'warn' : '');
        put(power, b.state === 'plugged' && b.power_w < 0.05 ? '' : fx(b.power_w, 1) + ' W');
        put(health, b.health + '%');
        put(cycles, String(b.cycles));
        attr(bat.el, 'title', t('bat.temp_tip', { t: temp(b.temp_c) }));
      }
      var order = s.settings.tile_order, absent = { gpu: !s.gpu, battery: !b, sleep: !s.sleep_blockers.length };
      if (order.join() !== placed) {
        placed = order.join();
        order.forEach(function (id) { grid.appendChild(TILES[id]); });
      }
      Object.keys(TILES).forEach(function (id) { TILES[id].hidden = absent[id] || order.indexOf(id) < 0; });
      // A run of half-width tiles with an odd count ends in one that takes the whole row.
      var run = [];
      order.concat('').forEach(function (id) {
        var tile = TILES[id];
        if (tile && tile.hidden) return;
        if (tile && !ROW[id]) { tile.classList.remove('wide'); run.push(tile); return; }
        if (run.length % 2) run[run.length - 1].classList.add('wide');
        run = [];
      });

      var sn = s.sensors;
      put(thermal, t('thermal.' + sn.thermal));
      attr(thermal, 'data-level', THERMAL[sn.thermal]);
      attr(thermal, 'title', t('thermal.tip'));
      // No room for two named readings on one line: the core is marked by the CPU icon, the names are in the tooltip.
      css(coreIcon, 'display', core == null ? 'none' : '');
      put(stripTemp, temp(core == null ? c.temp_c : core));
      attr(tempCell, 'title', (core == null ? '' : t('cpu.hottest') + ' ' + temp(core) + ' · ') + t('cpu.temp') + ' ' + temp(c.temp_c));
      fansCell.hidden = !sn.fans.length;
      put(stripFans, fansText(sn.fans));
      powerCell.hidden = s.power.system_w == null;
      put(stripPower, watts(s.power.system_w));

      var top = s.apps.items.slice(0, 5), peak = Math.max(100, top.length ? top[0].cpu : 0);
      sync(appList, top, function (a) { return a.name; }, function (a) {
        var row = h('div', 'row link-row'), name = h('span', 'name'), bar = meter(1), cpuCell = h('span', 'r'), memCell = h('span', 'r dim');
        row.$ = { name: name, bar: bar.segs[0], cpu: cpuCell, mem: memCell };
        row.onclick = function () { findApp(row.$.name.textContent); };
        return add(row, appIcon(a.icon, a.system), name, bar.el, cpuCell, memCell);
      }, function (row, a) {
        put(row.$.name, a.name);
        attr(row.$.name, 'title', a.name);
        row.classList.toggle('sys', a.system);
        fill(row.$.bar, s.apps.has_rates ? a.cpu / peak * 100 : 0);
        put(row.$.cpu, s.apps.has_rates ? percent(a.cpu) : DASH);
        put(row.$.mem, join(bytes(a.memory)));
      });

      sync(sleepList, s.sleep_blockers, function (k) { return k.pid + k.kind + k.name; }, function () {
        var row = h('div', 'row'), app = h('span', 'name'), why = h('span', 'name dim'), kind = h('span', 'badge');
        row.$ = { app: app, why: why, kind: kind };
        return add(row, app, why, kind);
      }, function (row, k) {
        put(row.$.app, k.app);
        put(row.$.why, k.name);
        attr(row, 'title', k.app + ' (' + k.pid + '): ' + k.name + ' — ' + k.kind);
        put(row.$.kind, t(/Display/.test(k.kind) ? 'sleep.display' : 'sleep.system'));
      });

      put(foot, t('foot', { uptime: dur(c.uptime_s), load: c.load.map(function (v) { return fx(v, 2); }).join(' ') }));
    }

    return { el: el, update: update };
  }

  // An alert row opens the alert's own screen through a button stretched under its text; Mute and
  // the row's other buttons are siblings of that button and lie above it, so no button holds another.
  function opensAlert(row) {
    var b = h('button', 'a-open');
    row.classList.add('go', 'link-row');
    b.onclick = function () {
      alertRow = b;
      openTab('detail:alert:' + row.$.id);
    };
    return b;
  }

  // The Overview's Top apps lead to the Apps tab with the name already searched.
  function findApp(name) {
    show('processes');
    tabs.processes.search(name);
  }

  // The app an alert is about. bt_battery carries a device name there: nothing to find in Apps, nothing to mute.
  function alertApp(a) { return a.kind === 'bt_battery' ? '' : a.app; }

  // Mute adds the app of an alert row to alert_muted; Go then drops that app's alerts.
  function muteButton(row) {
    var b = h('button', 'btn link sm', t('alerts.mute'));
    b.onclick = function () {
      var muted = state.settings.alert_muted;
      if (muted.indexOf(row.$.app) < 0) set('alert_muted', muted.concat(row.$.app));
    };
    return b;
  }

  function buildProcesses() {
    var el = h('div', 'tab');
    var input = h('input');
    input.type = 'search';
    input.placeholder = t('apps.search');
    input.setAttribute('aria-label', t('apps.search'));
    input.spellcheck = false;
    input.autocomplete = 'off';
    var card = h('section', 'tile table'), list = h('div', 'list scroll');
    var empty = h('div', 'empty'), emptyText = h('span'), emptyLink = h('button', 'btn link sm', t('apps.show_system'));
    add(empty, emptyText, emptyLink);
    // The window shows all four value columns. The popover has room for three: its last one is Disk
    // or Energy, the header sorts by it and a second click swaps the two.
    var COLS = wide ? ['cpu', 'memory', 'disk', 'energy'] : ['cpu', 'memory', 'disk'];
    var HEADS = { cpu: 'm.cpu', memory: 'm.memory', disk: 'm.disk', energy: 'col.energy' };
    // What a column sorts by; disk and energy are measured for the user's own processes only.
    var RANK = {
      cpu: function (a) { return a.cpu; },
      memory: function (a) { return a.memory; },
      disk: function (a) { return a.disk_read_rate == null ? -1 : a.disk_read_rate + a.disk_write_rate; },
      energy: function (a) { return a.energy_mw == null ? -1 : a.energy_mw; }
    };
    var count = h('span', 'th'), head = add(h('div', 'row thead procs'), count);
    var heads = COLS.map(function (col, i) {
      var b = h('button', 'th r');
      b.onclick = function () {
        if (!wide && i === 2 && sortBy === COLS[2]) COLS[2] = COLS[2] === 'disk' ? 'energy' : 'disk';
        setSort(COLS[i]);
      };
      return head.appendChild(b);
    });
    count.style.gridColumn = '1 / 4';
    add(card, head, list, empty);
    var sysBtn = h('button', 'btn sm');
    sysBtn.onclick = emptyLink.onclick = function () {
      if (state) send({ type: 'set', key: 'apps_show_system', value: !state.settings.apps_show_system });
    };
    // The four sorts by name: in the panel Energy is otherwise found only by a second click on the Disk header.
    var sorter = h('select', 'field');
    Object.keys(HEADS).forEach(function (col) { sorter.appendChild(new Option(t(HEADS[col]), col)); });
    sorter.setAttribute('aria-label', t('apps.sort'));
    sorter.title = t('apps.sort');
    sorter.onchange = function () {
      if (!wide && RANK[sorter.value] && COLS.indexOf(sorter.value) < 0) COLS[2] = sorter.value;
      setSort(sorter.value);
    };
    add(el, add(h('div', 'apps-bar'), add(h('label', 'search'), icon('search'), input), sorter, sysBtn), card);

    // Rows under SMALL percent (of one core, or of total memory when sorted by memory) fold into one "… N more" row.
    var SMALL = 1;
    var sortBy = 'cpu', query = '', open = new Set(), hasRates = false, memTotal = 0;
    var showSmall = false, smallKids = new Set();
    var asking = null, sent = null, kidsOf = '';
    // Tab titles of a browser, by app name: asked for by a button, shown until the row is folded, kept nowhere.
    var titles = new Map();

    function setSort(by) {
      sortBy = by;
      sorter.value = by;
      heads.forEach(function (b, i) {
        var on = COLS[i] === by, swaps = !wide && i === 2;
        b.textContent = t(HEADS[COLS[i]]);
        b.prepend(icon(swaps ? 'swap' : 'sort'));
        b.firstChild.style.visibility = on || swaps ? '' : 'hidden';
        b.title = swaps ? t('apps.swap_tip') : '';
        attr(b, 'aria-pressed', String(on));
      });
      if (state) update(state);
    }

    function cellText(col, a) {
      if (col === 'cpu') return hasRates ? percent(a.cpu) : DASH;
      if (col === 'memory') return join(bytes(a.memory));
      var v = RANK[col](a);
      return v < 0 || !hasRates ? DASH : col === 'disk' ? join(rate(v, 'disk')) : Math.round(v) + ' mW';
    }

    function valueCells(row) {
      return COLS.map(function (col, i) { return row.appendChild(h('span', i ? 'r dim' : 'r')); });
    }

    function putCells(cells, a) {
      cells.forEach(function (c, i) { put(c, cellText(COLS[i], a)); });
    }
    input.oninput = function () {
      query = input.value.trim().toLowerCase();
      if (state) update(state);
    };

    function matches(app) {
      return app.name.toLowerCase().indexOf(query) >= 0 || app.procs.some(function (p) {
        return p.name.toLowerCase().indexOf(query) >= 0 || String(p.pid) === query;
      });
    }

    // Go applies the same rule again before it signals anything; this only explains the disabled button.
    // A process that is neither system nor killable is mac-pulse itself.
    function blocked(item) {
      if (item.system) return t('apps.blocked_system');
      return item.killable ? '' : t('apps.blocked_self');
    }

    // A process carries no disk or energy figure: under those sorts the processes of an app fold by CPU.
    function isSmall(item) {
      var by = item.pid != null && sortBy !== 'memory' ? 'cpu' : sortBy;
      if (by === 'cpu') return item.cpu < SMALL;
      return by === 'memory' ? item.memory < memTotal * SMALL / 100 : !(RANK[by](item) > 0);
    }

    // fold moves the small rows behind a "… N more" row; a search, a list without rates yet
    // and a list that would fold entirely (or by a single row) stay as they are.
    function fold(items, expanded) {
      if (query || !hasRates) return items;
      var small = items.filter(isSmall);
      if (small.length < 2 || small.length === items.length) return items;
      var more = { more: true, n: small.length, cpu: 0, memory: 0 };
      small.forEach(function (x) { more.cpu += x.cpu; more.memory += x.memory; });
      var big = items.filter(function (x) { return !isSmall(x); });
      return big.concat(more, expanded ? small : []);
    }

    function createMore(cls, toggle) {
      var row = h('button', 'row more ' + cls);
      row.onclick = toggle;
      return row;
    }

    function updateMore(row, m, expanded) {
      put(row, t('more.sum', { n: m.n, cpu: percent(m.cpu), memory: join(bytes(m.memory)) }));
      attr(row, 'aria-expanded', String(expanded));
    }

    function cancelAsk() {
      if (!asking) return false;
      asking.box.remove();
      asking.row.classList.remove('asking');
      asking = null;
      return true;
    }

    function clearSent() {
      if (!sent) return;
      sent.box.remove();
      sent.row.classList.remove('asking');
      sent = null;
    }

    function ask(row, app, label, pid, pidsOf) {
      cancelAsk();
      clearSent();
      var box = h('div', 'confirm');
      // macOS order: the destructive button apart on the left, Cancel, then the default one. Return presses only Quit.
      var cancel = h('button', 'btn sm', t('cancel')), force = h('button', 'btn sm danger', t('quit.force')), quit = h('button', 'btn sm primary', t('quit'));
      // The sentence is cut at the name, which keeps its own style and position in every word order.
      var words = t('quit.ask', { name: '\n' }).split('\n');
      var q = add(h('span', 'q'), words[0] && h('span', '', words[0]), h('span', 'name', label), pid && h('span', 'pid', 'pid ' + pid), h('span', '', words[1]));
      var btns = add(h('span', 'btns'), force, cancel, quit);
      add(row, add(box, q, btns));
      row.classList.add('asking');
      asking = { row: row, box: box };
      cancel.onclick = cancelAsk;
      force.onclick = function () { act(true); };
      quit.onclick = function () { act(false); };
      quit.focus();

      function act(forced) {
        send({ type: 'quit_app', app: app, pids: pidsOf(), force: forced });
        q.textContent = t(forced ? 'quit.forcing' : 'quit.doing', { name: label });
        btns.remove();
        asking = null;
        // The next process scan is up to 6 s away; until then the quit app still looks alive.
        sent = { row: row, box: box, q: q, until: Date.now() + 7000 };
      }
    }

    function killButton(label) {
      var b = h('button', 'kill');
      b.appendChild(icon('close'));
      b.setAttribute('aria-label', t('quit.one', { name: label }));
      return b;
    }

    function setBlocked(b, reason) {
      attr(b, 'title', reason || t('quit.more'));
      attr(b, 'aria-disabled', String(!!reason));
    }

    function createApp(app) {
      var wrap = h('div', 'app'), row = h('div', 'row procs'), kinds = h('div', 'kids'), kids = h('div', 'kids'), tabsBox = h('div', 'titles');
      var chev = h('button', 'chev'), name = h('span', 'name'), pids = h('span', 'count');
      var kill = killButton(app.name);
      var reveal = h('button', 'btn link sm', t('act.reveal')), showTabs = h('button', 'btn link sm', t('tabs.show'));
      var details = h('button', 'btn link sm', t('details'));
      var acts = add(h('div', 'kid-acts'), showTabs, details, reveal);
      // The explanation comes before the request: only Continue makes macOS show its Automation prompt.
      showTabs.onclick = function () {
        titles.set(wrap.$.app.name, { step: 'explain' });
        updateApp(wrap, wrap.$.app);
        drawTitles(wrap);
      };
      chev.appendChild(icon('chevron'));
      chev.setAttribute('aria-label', t('apps.procs_of', { name: app.name }));
      chev.setAttribute('aria-expanded', 'false');
      var what = infoButton(function () { return wrap.$.app.name; }, 0, function () { return wrap.$.own; });
      // The same popover as "?", with the details of the main process already asked for.
      details.onclick = function () {
        closePop();
        what.click();
        pop.want.detail = true;
      };
      add(row, chev, appIcon(app.icon, app.system), add(h('span', 'app-n'), name, pids, what));
      wrap.$ = { name: name, pids: pids, cells: valueCells(row), kinds: kinds, kids: kids, acts: acts, chev: chev, kill: kill, what: what, app: app,
        titles: tabsBox, showTabs: showTabs, reveal: reveal, details: details };
      row.appendChild(kill);
      // Go resolves the pid to its bundle or executable itself and takes only a process of this user.
      reveal.onclick = function () { send({ type: 'reveal', pid: wrap.$.own.pid }); };
      row.onclick = function (e) {
        // A click on Cancel or OK bubbles here after the confirm box is gone.
        if (e.target.closest('.confirm, .kill') || row.classList.contains('asking')) return;
        var name = wrap.$.app.name;
        if (open.has(name)) open.delete(name); else open.add(name);
        updateApp(wrap, wrap.$.app);
      };
      kill.onclick = function () {
        var app = wrap.$.app;
        if (blocked(app)) { toast({ level: 'info', text: blocked(app) }); return; }
        ask(row, app.name, app.name + (app.pid_count > 1 ? ' (' + t('n.process', { n: app.pid_count }) + ')' : ''), 0, function () {
          // Read when the answer is given: the row may have been drawn from a state without processes.
          return wrap.$.app.procs.filter(function (p) { return p.killable; }).map(function (p) { return p.pid; });
        });
      };
      return add(wrap, row, kinds, tabsBox, kids, acts);
    }

    function drawTitles(wrap) {
      var box = wrap.$.titles, name = wrap.$.app.name, st = titles.get(name);
      box.textContent = '';
      if (!st) return;
      var close = h('button', 'btn sm', t(st.step === 'done' ? 'tabs.hide' : 'cancel'));
      close.onclick = function () {
        clearTimeout(st.timer);
        titles.delete(name);
        updateApp(wrap, wrap.$.app);
        drawTitles(wrap);
      };
      if (st.step === 'explain') {
        var go = h('button', 'btn sm primary', t('tabs.continue'));
        go.onclick = function () {
          st.step = 'wait';
          // Go waits two minutes for the user's answer to macOS; past that no answer is coming.
          st.timer = setTimeout(function () { gotTitles(name, { ok: false, text: t('act.no_answer') }); }, 125000);
          send({ type: 'browser_tabs', name: name });
          drawTitles(wrap);
        };
        add(box, h('p', '', t('tabs.explain', { name: name })), add(h('span', 'btns'), close, go));
        go.focus();
      } else if (st.step === 'wait') {
        add(box, add(h('p', 'dim'), document.createTextNode(t('info.loading')), close));
      } else if (st.error) {
        var allow = h('button', 'btn sm', t('tabs.settings'));
        allow.onclick = function () { send({ type: 'open', target: 'privacy_automation' }); };
        // Only a refusal of macOS is settled in System Settings; a timeout or a browser that left is not.
        add(box, h('p', 'error', st.error), add(h('span', 'btns'), close, st.denied && allow));
      } else {
        var list = h('div', 'scroll');
        st.list.forEach(function (title) { list.appendChild(h('div', 'name', title)).title = title; });
        add(box, add(h('p', 'dim'), document.createTextNode(t('tabs.title', { n: st.list.length })), close), list);
      }
    }

    // The answer for one browser goes to that browser's row, whichever row asked last; one that
    // nobody waits for any more (Cancel, a folded row) is dropped.
    function gotTitles(name, r) {
      var st = titles.get(name), wrap = list.$rows && list.$rows.get(name);
      if (!st || st.step !== 'wait') return;
      clearTimeout(st.timer);
      st.step = 'done';
      st.error = r.ok ? '' : r.text;
      st.denied = r.key === 'err.automation';
      st.list = r.list || [];
      // A failed request leaves its line and gives the button back.
      if (wrap) { updateApp(wrap, wrap.$.app); drawTitles(wrap); }
    }

    // A browser is many processes; argv (Chromium, Firefox) or the name (Safari) tells what each is
    // for. A renderer is a site instance, not a tab, so the rows count processes and promise no tab count.
    var KINDS = ['tab', 'extension', 'gpu', 'browser'];
    function kindSums(app) {
      var sums = {};
      app.procs.forEach(function (p) {
        if (!p.kind) return;
        var k = p.kind === 'utility' ? 'browser' : p.kind, x = sums[k] || (sums[k] = { kind: k, n: 0, cpu: 0, memory: 0 });
        x.n++;
        x.cpu += p.cpu;
        x.memory += p.memory;
      });
      return KINDS.map(function (k) { return sums[k]; }).filter(Boolean);
    }

    function createKind() {
      var row = h('div', 'row kid kind'), name = h('span', 'name'), n = h('span', 'count');
      row.appendChild(add(h('span', 'app-n'), name, n));
      row.$ = { name: name, n: n, cells: valueCells(row) };
      return row;
    }

    // A helper named after its app ("Google Chrome Helper (GPU)") drops the prefix the parent row already shows.
    function short(p, app) {
      return p.name.indexOf(app + ' Helper') === 0 ? p.name.slice(app.length + 1) : p.name;
    }

    function createKid(p) {
      var row = h('div', 'row kid'), pid = h('span', 'pid', String(p.pid)), name = h('span', 'name');
      var kill = killButton(p.name + ' (' + p.pid + ')'), app = kidsOf;
      var what = infoButton(function () { return app; }, p.pid, function () { return row.$.p; });
      attr(what, 'aria-label', t('info.what', { name: p.name }));
      what.title = what.getAttribute('aria-label');
      add(row, pid, add(h('span', 'app-n'), name, what));
      row.$ = { name: name, cells: valueCells(row), kill: kill, p: p };
      kill.onclick = function () {
        if (blocked(row.$.p)) toast({ level: 'info', text: blocked(row.$.p) });
        else ask(row, app, row.$.name.textContent, p.pid, function () { return [p.pid]; });
      };
      return add(row, kill);
    }

    function updateKid(row, p) {
      row.$.p = p;
      row.classList.toggle('sys', p.system);
      setBlocked(row.$.kill, blocked(p));
      put(row.$.name, short(p, kidsOf));
      attr(row.$.name, 'title', p.name);
      putCells(row.$.cells, p);
    }

    function updateApp(wrap, app) {
      var r = wrap.$, isOpen = open.has(app.name);
      r.app = app;
      put(r.name, app.name);
      attr(r.name, 'title', app.name);
      attr(r.what, 'aria-label', t('info.what', { name: app.name }));
      attr(r.what, 'title', r.what.getAttribute('aria-label'));
      put(r.pids, app.pid_count > 1 ? String(app.pid_count) : '');
      putCells(r.cells, app);
      attr(r.chev, 'aria-expanded', String(isOpen));
      wrap.classList.toggle('sys', app.system);
      setBlocked(r.kill, blocked(app));
      kidsOf = app.name;
      r.own = app.procs.filter(function (p) { return p.killable; })[0] || (app.icon === 'self' ? app.procs[0] : null);
      // Chromium browsers and Safari answer AppleScript; Firefox has no dictionary to ask.
      r.showTabs.hidden = !!(titles.get(app.name) && !titles.get(app.name).error) || /^Firefox/.test(app.name) || !app.procs.some(function (p) { return p.kind === 'browser'; });
      r.reveal.hidden = r.details.hidden = !r.own;
      r.acts.hidden = !isOpen || r.reveal.hidden && r.showTabs.hidden;
      // A row that was filtered out and came back is a new one: its titles are drawn again.
      if (isOpen && titles.get(app.name) && !r.titles.firstChild) drawTitles(wrap);
      if (!isOpen) {
        clearTimeout((titles.get(app.name) || {}).timer);
        if (titles.delete(app.name)) drawTitles(wrap);
        [r.kinds, r.kids].forEach(function (box) {
          if (box.firstChild) { box.textContent = ''; box.$rows = null; }
        });
        return;
      }
      sync(r.kinds, kindSums(app), function (x) { return x.kind; }, createKind, function (row, x) {
        put(row.$.name, t('kind.' + x.kind));
        put(row.$.n, t('n.process', { n: x.n }));
        putCells(row.$.cells, x);
      });
      var expanded = smallKids.has(app.name);
      sync(r.kids, fold(app.procs, expanded), function (p) { return p.more ? 'more' : p.pid; }, function (p) {
        return p.more ? createMore('kid', function () {
          if (!smallKids.delete(app.name)) smallKids.add(app.name);
          updateApp(wrap, wrap.$.app);
        }) : createKid(p);
      }, function (row, p) {
        if (p.more) updateMore(row, p, expanded); else updateKid(row, p);
      });
    }

    function update(s) {
      // Only the state made for this tab lists the processes of an app; the first draw may come from another one.
      s.apps.items.forEach(function (a) { a.procs = a.procs || []; });
      hasRates = s.apps.has_rates;
      memTotal = s.memory.total;
      if (sent && !sent.error && Date.now() > sent.until) clearSent();
      var showSystem = s.settings.apps_show_system;
      var shown = showSystem ? s.apps.items : s.apps.items.filter(function (a) { return !a.system; });
      put(sysBtn, showSystem ? t('apps.hide_system') : t('apps.show_system_n', { n: s.apps.items.length - shown.length }));
      var items = query ? shown.filter(matches) : shown;
      if (sortBy !== 'cpu') items = items.slice().sort(function (a, b) { return RANK[sortBy](b) - RANK[sortBy](a); });
      put(count, query ? t('x_of_y', { x: items.length, y: t('n.app', { n: shown.length }) }) : t('n.app', { n: shown.length }));
      sync(list, fold(items, showSmall), function (a) { return a.more ? '\n' : a.name; }, function (a) {
        return a.more ? createMore('', function () { showSmall = !showSmall; update(state); }) : createApp(a);
      }, function (row, a) {
        if (a.more) updateMore(row, a, showSmall); else updateApp(row, a);
      });
      empty.hidden = items.length > 0;
      list.hidden = !items.length;
      var hiddenN = s.apps.items.length - shown.length;
      var hiddenMatches = query && !showSystem ? s.apps.items.filter(function (a) { return a.system && matches(a); }).length : 0;
      put(emptyText, query
        ? hiddenMatches ? t('apps.sys_matches', { n: hiddenMatches }) : t('apps.no_match', { q: input.value.trim() })
        : hasRates ? (hiddenN ? t('apps.nothing_hidden', { n: hiddenN }) : t('apps.nothing')) : t('apps.waiting'));
      emptyLink.hidden = !hiddenMatches;
    }

    setSort('cpu');
    return {
      el: el,
      update: update,
      cancel: cancelAsk,
      focusSearch: function () { input.focus(); input.select(); },
      search: function (text) { input.value = text; input.oninput(); },
      // The quit-heaviest shortcut: the confirm of the first app by CPU that mac-pulse may quit.
      // An alert's Quit button names its app.
      askTop: function (name) {
        // Before the second process scan every CPU is 0 and the first row would pass for the heaviest.
        var app = state && state.apps.has_rates && state.apps.items.filter(function (a) { return !blocked(a) && (!name || a.name === name); })[0];
        if (!app) return;
        input.value = query = '';
        showSmall = true;
        setSort('cpu');
        list.$rows.get(app.name).$.kill.onclick();
      },
      // The answer to browser_tabs: Go names the browser in the answer's own text.
      onTabs: function (r) { gotTitles(r.about, r); },
      // An error that follows a quit within a few seconds belongs to the row that asked for it.
      notice: function (n) {
        if (!sent || n.level !== 'error') return false;
        sent.error = true;
        sent.box.classList.add('error');
        sent.q.textContent = n.text;
        var ok = h('button', 'btn sm', t('ok'));
        ok.onclick = clearSent;
        sent.box.appendChild(ok);
        return true;
      }
    };
  }

  function buildNetwork() {
    // sub labels the totals line under the rate ("since opened"): they count from the first report that saw the row.
    function rates(downRate, upRate, downTotal, upTotal, since) {
      return [
        { t: t('col.down'), i: 'down', sub: since, w: wide ? '96px' : '78px', r: true,
          a: function (x, n) { return join(rate(n.has_rates ? x[downRate] : null)); },
          b: downTotal && function (x) { return join(bytes(x[downTotal])); } },
        { t: t('col.up'), i: 'up', sub: since, w: wide ? '96px' : '78px', r: true,
          a: function (x, n) { return join(rate(n.has_rates ? x[upRate] : null)); },
          b: upTotal && function (x) { return join(bytes(x[upTotal])); } }
      ];
    }

    function remote(c) {
      var ip = c.remote_ip.indexOf(':') >= 0 ? '[' + c.remote_ip + ']' : c.remote_ip;
      return (c.host || ip) + ':' + c.remote_port;
    }

    var FLEX = 'minmax(0, 1fr)';
    var SUBS = {
      apps: {
        label: t('net.apps'), rows: function (n) { return n.apps; }, key: function (a) { return a.name; }, noun: 'n.app',
        cols: [{ t: t('col.app'), w: FLEX, icon: true, a: function (a) { return a.name; },
          b: !wide && function (a) { return t('n.process', { n: a.pid_count }) + ' · ' + t('n.connection', { n: a.connections }); } }]
          .concat(wide ? [
            { t: t('col.processes'), w: '90px', r: true, a: function (a) { return String(a.pid_count); } },
            { t: t('net.connections'), w: '100px', r: true, a: function (a) { return String(a.connections); } }
          ] : [], rates('down_rate', 'up_rate', 'down_total', 'up_total', t('net.since_opened')))
      },
      connections: {
        label: t('net.connections'), rows: function (n) { return n.connections; }, noun: 'n.connection',
        key: function (c) { return [c.pid, c.proto, c.local, c.remote_ip, c.remote_port].join('|'); },
        cols: (wide ? [
          { t: t('col.app'), w: 'minmax(0, .9fr)', a: function (c) { return c.app; }, b: function (c) { return 'pid ' + c.pid + ' · ' + c.proto; } },
          { t: t('col.local'), w: 'minmax(0, 1.2fr)', a: function (c) { return c.local; } },
          { t: t('col.remote'), w: 'minmax(0, 1.6fr)', a: remote, b: function (c) { return c.host ? c.remote_ip : ''; } },
          { t: t('col.state'), w: '96px', a: function (c) { return c.state || DASH; } }
        ] : [
          { t: t('col.remote'), w: FLEX, a: remote,
            b: function (c) { return (c.host ? c.remote_ip + ' · ' : '') + c.app + ' · ' + c.proto + (c.state ? ' · ' + c.state : ''); },
            tip: function (c) { return c.app + ' (' + c.pid + ') ' + c.proto + '\n' + c.local + ' → ' + c.remote_ip + ':' + c.remote_port + (c.host ? '\n' + c.host : ''); } }
        ]).concat(rates('down_rate', 'up_rate', 'bytes_in', 'bytes_out'))
      },
      listening: {
        label: t('net.listening'), rows: function (n) { return n.listening; }, noun: 'n.port',
        key: function (l) { return [l.pid, l.proto, l.addr, l.port].join('|'); },
        cols: [
          { t: t('col.app'), w: FLEX, a: function (l) { return l.app; }, b: !wide && function (l) { return l.dir; } },
          { t: 'PID', w: '56px', r: true, a: function (l) { return String(l.pid); } },
          { t: t('col.protocol'), w: '56px', a: function (l) { return l.proto; } },
          { t: t('col.address'), w: wide ? 'minmax(0, 1fr)' : '132px', r: !wide, a: function (l) { return l.addr + ':' + l.port; } }
        ].concat(wide ? [{ t: t('col.folder'), w: 'minmax(0, 1.6fr)', a: function (l) { return l.dir || DASH; } }] : [])
      },
      talkers: {
        label: t('net.hosts'), rows: function (n) { return n.talkers; }, key: function (x) { return x.ip; }, noun: 'n.host',
        cols: [{ t: t('col.host'), w: FLEX, a: function (x) { return x.host || x.ip; },
          b: function (x) { return (x.host ? x.ip + ' · ' : '') + x.apps.join(', '); },
          tip: function (x) { return (x.host ? x.host + '\n' : '') + x.ip + '\n' + t('n.connection', { n: x.connections }) + ': ' + x.apps.join(', '); } }]
          .concat(wide ? [{ t: t('net.connections'), w: '100px', r: true, a: function (x) { return String(x.connections); } }] : [],
            rates('down_rate', 'up_rate'))
      },
      today: {
        label: t('today'), rows: function (n) { return n.today; }, key: function (x) { return x.name; }, noun: 'n.app',
        note: t('net.today_note'),
        cols: [
          { t: t('col.app'), w: FLEX, icon: true, a: function (x) { return x.name; } },
          { t: t('col.down'), i: 'down', w: wide ? '110px' : '74px', r: true, a: function (x) { return join(bytes(x.down)); } },
          { t: t('col.up'), i: 'up', w: wide ? '110px' : '74px', r: true, a: function (x) { return join(bytes(x.up)); } },
          { t: t('col.total'), w: wide ? '110px' : '74px', r: true, a: function (x) { return join(bytes(x.down + x.up)); } }
        ]
      }
    };
    var order = ['apps', 'connections', 'listening', 'talkers', 'today'];
    var sub = has(SUBS, params.get('sub')) ? params.get('sub') : 'apps';

    var el = h('div', 'tab net');
    var picker = segmented(order.map(function (k) { return [k, SUBS[k].label]; }), pick, t('net.views'));
    var card = h('section', 'tile table'), head = h('div', 'row thead'), list = h('div', 'list scroll'), empty = h('div', 'empty');
    var total = h('span'), more = h('button', 'btn link sm', t('open_window'));
    more.appendChild(icon('window'));
    more.onclick = function () { send({ type: 'open_window' }); };
    add(card, head, list, empty);
    add(el, picker.el, card, add(h('div', 'net-foot'), total, !wide && more));

    function pick(name) {
      sub = name;
      picker.set(sub);
      var spec = SUBS[sub];
      card.style.setProperty('--cols', spec.cols.map(function (c) { return c.w; }).join(' '));
      head.textContent = '';
      spec.cols.forEach(function (c) {
        var th = h('span', 'th' + (c.r ? ' r' : '') + (c.sub ? ' sub' : ''));
        add(th, add(h('span'), c.i && icon(c.i), document.createTextNode(c.t)), c.sub && h('small', '', c.sub));
        head.appendChild(th);
      });
      list.textContent = '';
      list.$rows = null;
      if (state) update(state);
    }

    function create(item) {
      var spec = SUBS[sub], row = h('div', 'row');
      row.$ = spec.cols.map(function (c) {
        var cell = h('div', 'cell' + (c.r ? ' r' : '')), a = h('span', 'a'), b = c.b && h('span', 'b');
        add(row, add(cell, c.icon && appIcon(item.icon, item.system), add(h('div'), a, b)));
        return { cell: cell, a: a, b: b };
      });
      return row;
    }

    function update(s) {
      var spec = SUBS[sub], items = s.net ? spec.rows(s.net) : [];
      sync(list, items, spec.key, create, function (row, item) {
        var two = false;
        row.classList.toggle('idle', sub === 'apps' && !item.down_rate && !item.up_rate);
        row.classList.toggle('sys', !!item.system);
        spec.cols.forEach(function (c, i) {
          var a = c.a(item, s.net), b = c.b ? c.b(item, s.net) : '';
          if (b) two = true;
          put(row.$[i].a, a);
          if (c.b) put(row.$[i].b, b);
          if (!c.r) attr(row.$[i].cell, 'title', c.tip ? c.tip(item) : b ? a + '\n' + b : a);
        });
        // A row is two lines tall only when some cell has a second line (a listening port without a folder has none).
        row.classList.toggle('two', two);
      });
      empty.hidden = items.length > 0;
      list.hidden = !items.length;
      put(empty, s.net ? t('net.empty') : t('net.starting'));
      put(total, s.net ? spec.note || t(spec.noun, { n: items.length }) : '');
    }

    pick(sub);
    return { el: el, update: update };
  }

  var RANGES = ['1h', '12h', '24h', '7d', '30d', '90d', '1y'];
  // Series name → dictionary key.
  var NAMES = { total: 'm.cpu', used: 'series.used', down: 'col.down', up: 'col.up', read: 'disk.read', write: 'disk.write', util: 'm.gpu', cpu: 'cpu.temp', percent: 'series.charge' };
  var POWER_NAMES = { system: 'power.system', cpu: 'm.cpu', gpu: 'm.gpu' };

  // One history chart: range picker, axes, crosshair, min / avg / max. The History tab and every detail screen own one.
  function historyChart(metric, range, onData) {
    // waiting: Go answers only into a visible view, so a request made while the panel is closed gets no reply.
    var data = null, asked = 0, waiting = false, plot = null;
    var chart = h('section', 'tile chart');
    var ranges = segmented(RANGES.map(function (v) { return [v, t('range.' + v)]; }), function (v) { range = v; ask(true); }, t('range.label'));
    var legend = h('div', 'legend');
    var svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('tabindex', '0');
    svg.setAttribute('role', 'img');
    var tip = h('div', 'tip'), stats = h('div', 'stats'), note = h('div', 'note', t('hist.loading'));
    tip.hidden = true;
    // An alert's chart has no range: it is handed its points through draw().
    ranges.el.hidden = !range;
    add(chart, add(h('div', 'hist-bar'), legend, ranges.el), svg, note, tip, stats);

    function ask(stale) {
      asked = Date.now();
      waiting = true;
      ranges.set(range);
      if (stale) { chart.classList.add('stale'); point(-1); }
      send({ type: 'history', metric: metric, range: range });
    }

    function label(name) {
      if (data.label) return data.label;
      var key = (data.metric === 'power' ? POWER_NAMES : NAMES)[name];
      return key ? t(key) : name;
    }

    function show(unit, v) {
      if (v == null) return DASH;
      if (unit === 'percent') return pct(v);
      if (unit === 'bytes') return join(bytes(v));
      if (unit === 'bytes_per_s') return join(rate(v, data.metric === 'disk' ? 'disk' : ''));
      if (unit === 'watts') return watts(v);
      return temp(v);
    }

    function when(i) {
      var ms = data.start + i * data.step_s * 1000;
      // A point of the 90-day and 1-year ranges spans half a day or more: the hour would be noise.
      return data.step_s >= 43200 ? day(ms) : data.step_s >= 3600 ? day(ms) + ', ' + clock(ms) : clock(ms);
    }

    function niceMax(v, base) {
      if (!(v > 0)) return 1;
      var unit = 1;
      while (v / unit >= base) unit *= base;
      var x = v / unit, p = Math.pow(10, Math.floor(Math.log10(x))), m = x / p;
      return (m <= 1 ? 1 : m <= 2 ? 2 : m <= 5 ? 5 : 10) * p * unit;
    }

    function node(tag, attrs, text) {
      var e = document.createElementNS(SVG, tag);
      for (var k in attrs) e.setAttribute(k, attrs[k]);
      if (text != null) e.textContent = text;
      return svg.appendChild(e);
    }

    function render() {
      if (!data) return;
      chart.classList.remove('stale');
      chart.dataset.metric = data.metric;
      var full = Math.max(svg.clientWidth, 200), W = full - 52, H = svg.clientHeight || 150, top = 8, bottom = H - 18;
      var unit = data.series.length ? data.series[0].unit : 'percent';
      var n = Math.max.apply(null, data.series.map(function (s) { return s.points.length; }).concat(0));
      var all = [];
      data.series.forEach(function (s) { s.points.forEach(function (v) { if (v != null) all.push(v); }); });
      var lo = 0, hi = 100, bits = data.metric === 'network' && settings.net_unit === 'bits';
      // The limit of an alert is on the scale too, and the scale fits its values: free disk space
      // moves around 10%, an app's CPU, percent of one core, goes past 100.
      var scaled = all.concat(data.limit || []), peak = Math.max.apply(null, scaled.concat(0));
      if (unit === 'celsius' && all.length) {
        lo = Math.floor(Math.min.apply(null, scaled) / 10) * 10;
        hi = Math.max(lo + 10, Math.ceil(peak / 10) * 10);
      } else if (unit !== 'percent' || peak > 100 || data.from) {
        hi = niceMax(peak * (bits ? 8 : 1), bits || data.metric === 'disk' || unit === 'watts' || unit === 'percent' ? 1000 : 1024) / (bits ? 8 : 1);
      }
      var x = function (i) { return n > 1 ? i * W / (n - 1) : W; };
      var y = function (v) { return bottom - (v - lo) / (hi - lo) * (bottom - top); };

      svg.setAttribute('viewBox', '0 0 ' + full + ' ' + H);
      svg.textContent = '';
      // Without a single point there is no scale to label: only the baseline is drawn.
      (all.length ? [lo, (lo + hi) / 2, hi] : [lo]).forEach(function (v) {
        node('path', { class: 'gl', d: 'M0 ' + y(v).toFixed(1) + 'H' + W });
        if (all.length) node('text', { x: full, y: (y(v) + 3.5).toFixed(1), 'text-anchor': 'end' }, show(unit, v));
      });
      // An alert: the time it lasted is shaded and its limit drawn across.
      if (data.from && n > 1) {
        var at = function (ms) { return Math.max(0, Math.min(W, (ms - data.start) / (data.step_s * 1000) * W / (n - 1))); };
        node('rect', { class: 'band', x: at(data.from).toFixed(1), y: top, width: Math.max(2, at(data.to) - at(data.from)).toFixed(1), height: bottom - top });
      }
      if (data.limit) node('path', { class: 'lim', d: 'M0 ' + y(data.limit).toFixed(1) + 'H' + W });
      if (n > 1) {
        [0, 1, 2, 3].forEach(function (k) {
          var i = Math.round(k * (n - 1) / 3), ms = data.start + i * data.step_s * 1000;
          node('text', { x: x(i).toFixed(1), y: H - 3, 'text-anchor': k === 0 ? 'start' : k === 3 ? 'end' : 'middle' },
            data.step_s >= 3600 ? day(ms) : clock(ms));
        });
      }

      data.series.forEach(function (s, k) {
        var line = '', area = '', start = -1, last = '';
        for (var i = 0; i <= s.points.length; i++) {
          var v = i < s.points.length ? s.points[i] : null;
          if (v == null) {
            if (start >= 0) area += 'L' + x(i - 1).toFixed(1) + ' ' + bottom + 'L' + x(start).toFixed(1) + ' ' + bottom + 'Z';
            // A run of one point has no segment to stroke; a dot keeps the first minute visible.
            if (start === i - 1) node('circle', { class: 'pt s' + k, cx: last.split(' ')[0], cy: last.split(' ')[1], r: 2 });
            start = -1;
            continue;
          }
          var pt = last = x(i).toFixed(1) + ' ' + y(Math.min(Math.max(v, lo), hi)).toFixed(1);
          line += (start < 0 ? 'M' : 'L') + pt;
          area += (start < 0 ? 'M' : 'L') + pt;
          if (start < 0) start = i;
        }
        node('path', { class: 'ar s' + k, d: area });
        node('path', { class: 'ln s' + k, d: line });
      });
      var cross = node('path', { class: 'cross', d: '' });
      var dots = data.series.map(function (s, k) { return node('circle', { class: 'dot s' + k, r: 3.5 }); });
      var at = plot ? plot.at : -1;
      plot = { n: n, x: x, y: y, lo: lo, hi: hi, unit: unit, cross: cross, dots: dots, W: W, top: top, bottom: bottom };
      point(Math.min(at, n - 1));

      legend.textContent = '';
      stats.textContent = '';
      add(stats, h('span'), h('span', 'th', t('stat.min')), h('span', 'th', t('stat.avg')), h('span', 'th', t('stat.max')));
      data.series.forEach(function (s, k) {
        var vals = s.points.filter(function (v) { return v != null; });
        var sum = vals.reduce(function (a, b) { return a + b; }, 0);
        legend.appendChild(h('span', 'key s' + k, label(s.name)));
        add(stats, h('span', 'key s' + k + ' dim', label(s.name)),
          h('b', '', vals.length ? show(unit, Math.min.apply(null, vals)) : DASH),
          h('b', '', vals.length ? show(unit, sum / vals.length) : DASH),
          h('b', '', vals.length ? show(unit, Math.max.apply(null, vals)) : DASH));
      });
      if (data.limit) legend.appendChild(h('span', 'key lim', t('alertd.limit') + ' ' + show(unit, data.limit)));
      if (data.from) legend.appendChild(h('span', 'key band', t('alertd.span')));
      svg.setAttribute('aria-label', data.label || t(all.length ? 'hist.aria' : 'hist.aria_empty', { names: legend.textContent, range: t('range.' + data.range) }));
      note.hidden = all.length > 0;
      note.textContent = data.empty || t('hist.empty');
    }

    function point(i) {
      if (!plot || i < 0 || i >= plot.n) {
        if (plot) {
          plot.cross.setAttribute('d', '');
          plot.dots.forEach(function (d) { d.setAttribute('visibility', 'hidden'); });
          plot.at = -1;
        }
        tip.hidden = true;
        return;
      }
      plot.at = i;
      var px = plot.x(i);
      plot.cross.setAttribute('d', 'M' + px.toFixed(1) + ' ' + plot.top + 'V' + plot.bottom);
      tip.textContent = '';
      tip.appendChild(h('time', '', when(i)));
      data.series.forEach(function (s, k) {
        var v = s.points[i], dot = plot.dots[k];
        dot.setAttribute('visibility', v == null ? 'hidden' : 'visible');
        if (v != null) { dot.setAttribute('cx', px.toFixed(1)); dot.setAttribute('cy', plot.y(Math.min(Math.max(v, plot.lo), plot.hi)).toFixed(1)); }
        var key = h('i', 'line-key s' + k);
        key.style.background = 'var(--c' + k + ')';
        add(tip, add(h('div'), key, h('b', '', v == null ? t(data.metric === 'power' ? 'hist.no_data' : 'hist.not_running') : show(plot.unit, v)), h('span', '', label(s.name))));
      });
      tip.hidden = false;
      var box = svg.getBoundingClientRect(), frame = chart.getBoundingClientRect();
      tip.style.left = (box.left - frame.left + (px > plot.W / 2 ? px - tip.offsetWidth - 10 : px + 10)) + 'px';
      tip.style.top = (box.top - frame.top + 6) + 'px';
    }

    svg.addEventListener('pointermove', function (e) {
      if (!plot) return;
      var rect = svg.getBoundingClientRect();
      point(Math.max(0, Math.min(plot.n - 1, Math.round((e.clientX - rect.left) / plot.W * (plot.n - 1)))));
    });
    svg.addEventListener('pointerleave', function () { point(-1); });
    svg.addEventListener('blur', function () { point(-1); });
    svg.addEventListener('keydown', function (e) {
      if (!plot || (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight')) return;
      var at = plot.at < 0 ? plot.n - 1 : plot.at + (e.key === 'ArrowRight' ? 1 : -1);
      point(Math.max(0, Math.min(plot.n - 1, at)));
      e.preventDefault();
    });

    return {
      el: chart,
      ask: ask,
      resize: render,
      setMetric: function (m) { metric = m; ask(true); },
      // Re-asks once a minute, and every second while a request from a hidden view is still unanswered.
      tick: function () { if (Date.now() - asked > (waiting ? 1000 : 60000)) ask(false); },
      draw: function (d) { data = d; render(); },
      onHistory: function (hist) {
        if (hist.metric !== metric || hist.range !== range) return;
        waiting = false;
        data = hist;
        render();
        if (onData) onData(hist);
      }
    };
  }

  function buildHistory() {
    var METRICS = ['cpu', 'memory', 'network', 'disk', 'gpu', 'temp', 'battery'].map(function (m) { return [m, t('m.' + m)]; });
    var has = function (list, v) { return list.some(function (x) { return x[0] === v; }); };
    var metric = has(METRICS, params.get('metric')) ? params.get('metric') : 'cpu';

    var el = h('div', 'tab scroll');
    var ranked = h('section', 'tile ranked'), rankTitle = h('span', 'title'), rankList = h('div', 'list');
    add(ranked, add(h('div', 'card-h'), rankTitle), rankList);
    ranked.hidden = true;
    var chart = historyChart(metric, RANGES.indexOf(params.get('range')) >= 0 ? params.get('range') : '1h', function (data) {
      var peak = data.top_apps.length ? data.top_apps[0].value : 0;
      ranked.dataset.metric = data.metric;
      ranked.hidden = !data.top_apps.length;
      put(rankTitle, t(data.metric === 'network' ? 'hist.top_total' : 'hist.top_avg', { range: t('range.' + data.range) }));
      sync(rankList, data.top_apps, function (a) { return a.name; }, function (a) {
        var row = h('div', 'row'), name = h('span', 'name'), bar = meter(1), value = h('span', 'r');
        row.$ = { name: name, bar: bar.segs[0], value: value };
        return add(row, appIcon(a.icon, a.system), name, bar.el, value);
      }, function (row, a) {
        row.classList.toggle('sys', a.system);
        put(row.$.name, a.name);
        fill(row.$.bar, peak ? a.value / peak * 100 : 0);
        put(row.$.value, data.metric === 'cpu' ? percent(a.value) : join(bytes(a.value)));
      });
    });
    var metrics = segmented(METRICS, pick, t('hist.metric'));
    var recent = tile('recent', 'alert', t('alerts.recent')), recentList = h('div', 'list');
    recent.el.appendChild(recentList);
    var csvOut = h('span', 'act-out');
    add(el, metrics.el, chart.el, add(h('div', 'pair'), ranked, recent.el),
      add(h('div', 'acts flat'), action('export_csv', t('act.export_csv'), csvOut), csvOut));

    function pick(v) {
      metric = v;
      metrics.set(v);
      chart.setMetric(v);
    }

    function update(s) {
      var there = { gpu: !!s.gpu, battery: !!s.battery, temp: s.cpu.temp_c != null };
      METRICS.forEach(function (m, i) { metrics.buttons[i].hidden = there[m[0]] === false; });
      if (there[metric] === false) pick('cpu'); else chart.tick();

      recent.el.hidden = !s.alerts.recent.length;
      sync(recentList, s.alerts.recent, function (a) { return a.id + a.since; }, function () {
        var row = h('div', 'row'), title = h('span', 'name'), detail = h('span', 'name dim detail'), at = h('span', 'dim at');
        row.$ = { title: title, detail: detail, at: at, open: opensAlert(row), mute: muteButton(row) };
        // Two lines of their own: the title shares its line with Mute alone, so it rarely has to wrap.
        return add(row, row.$.open, add(h('div', 'line'), title, row.$.mute, icon('chevron', 'go-i')), add(h('div', 'line'), detail, at));
      }, function (row, a) {
        var text = alertText(a);
        row.$.app = alertApp(a);
        row.$.id = a.id;
        attr(row.$.open, 'aria-label', t('details_of', { name: text[0] }));
        row.$.mute.hidden = !row.$.app || s.settings.alert_muted.indexOf(a.app) >= 0;
        put(row.$.title, text[0]);
        put(row.$.detail, text[1]);
        attr(row, 'title', text.join(' — '));
        put(row.$.at, day(a.since) + ', ' + clock(a.since) + ' · ' + dur(Math.max(0, a.until - a.since) / 1000));
      });
    }

    metrics.set(metric);
    return {
      el: el, update: update, resize: chart.resize, onHistory: chart.onHistory,
      shown: function () {
        csvOut.classList.remove('error');
        put(csvOut, '');
        chart.ask(true);
      }
    };
  }

  function buildSettings() {
    // The menu and the open section scroll apart: side by side where the view is wide enough,
    // otherwise the section covers the menu and the header's back button returns to it.
    var el = h('div', 'tab settings'), menu = h('nav', 'set-menu scroll'), pane = h('div', 'set-pane scroll');
    var box = h('div', 'narrow'), list = h('div', 'tile group'), secs = {}, rows = {}, last = '';
    var ICON = { general: 'system', alerts: 'bell', shortcuts: 'keys' };
    Object.keys(SECTIONS).forEach(function (id) {
      var b = h('button', 'set-row'), sum = h('span', 'hint'), glyph = add(h('span', 'set-i'), icon(ICON[id] || id));
      b.dataset.sec = id;
      b.onclick = function () { openTab('settings:' + id); };
      rows[id] = list.appendChild(add(b, glyph, add(h('span', 'lbl'), h('b', '', t(SECTIONS[id])), sum), icon('chevron', 'go-i')));
      b.$ = sum;
      secs[id] = box.appendChild(add(h('div', 'set-sec'), h('h2', '', t(SECTIONS[id]))));
    });
    // In the sidebar an arrow opens the section it lands on; over the menu alone it only moves, and Enter opens.
    list.onkeydown = function (e) {
      var ids = Object.keys(SECTIONS), next = rows[ids[ids.indexOf(document.activeElement.dataset.sec) + ({ ArrowUp: -1, ArrowDown: 1 }[e.key] || NaN)]];
      if (!next) return;
      e.preventDefault();
      next.focus();
      if (el.classList.contains('split')) next.click();
    };

    // A group whose title would repeat its section's has none.
    function group(sec, title, hint) {
      var g = h('section', 'tile group');
      add(secs[sec], title && add(h('div', 'group-h'), h('span', '', title), hint), g);
      return g;
    }

    // A segmented control must not sit in a <label>: clicking the text would press its first button.
    function row(parent, label, hint, control) {
      var lbl = add(h('span', 'lbl'), document.createTextNode(label), hint && h('span', 'hint', hint));
      var r = add(h(control.tagName === 'DIV' ? 'div' : 'label', 'set'), lbl, control);
      parent.appendChild(r);
      return r;
    }

    function toggle(onChange) {
      var input = h('input', 'switch');
      input.type = 'checkbox';
      input.setAttribute('role', 'switch');
      if (onChange) input.onchange = function () { onChange(input.checked); };
      return input;
    }

    function number(key, min, max, unitText, toView, fromView) {
      var input = h('input'), unit = h('span', '', unitText);
      input.type = 'number';
      input.onchange = function () {
        var lo = toView(min), hi = toView(max), n = Number(input.value);
        // An emptied or unreadable field goes back to the saved value rather than to the minimum.
        if (input.value.trim() === '' || !isFinite(n)) { input.value = state ? Math.round(toView(state.settings[key])) : ''; return; }
        var v = Math.round(Math.max(lo, Math.min(hi, n)));
        input.value = v;
        set(key, Math.max(min, Math.min(max, Math.round(fromView(v)))));
      };
      return { el: add(h('span', 'num-in'), input, unit), input: input, unit: unit, key: key, min: min, max: max, toView: toView };
    }

    var same = function (v) { return v; };
    // The only request the Settings make, and only on this button: Go asks GitHub for the latest release tag.
    var updOut = h('span', 'act-out'), releases = h('button', 'btn link sm', t('upd.open'));
    put(updOut, t('upd.note'));
    releases.hidden = true;
    releases.onclick = function () { send({ type: 'open', target: 'releases' }); };
    var check = action('update_check', t('upd.check'), updOut, { wait: 12000, done: function (r) { releases.hidden = r.key !== 'upd.newer'; } });
    var version = params.get('version') || 'dev';
    add(menu, add(h('div', 'about'), appIcon('self'), add(h('div'), h('b', '', 'mac-pulse'), h('span', '', version))), list);

    // Each swatch carries its theme's own tokens (data-skin, data-theme), so it shows the real colours.
    var look = group('appearance'), swatches = h('div', 'swatches');
    swatches.setAttribute('role', 'radiogroup');
    swatches.setAttribute('aria-label', t('set.theme'));
    var radios = SKINS.map(function (id) {
      var b = h('button', 'swatch'), preview = h('span', 'swatch-p');
      b.setAttribute('role', 'radio');
      preview.dataset.skin = id;
      add(preview, add(h('i'), h('b'), h('b'), h('b')));
      b.onclick = function () { set('theme', id); };
      return swatches.appendChild(add(b, preview, h('span', 'name', t('theme.' + id))));
    });
    arrowKeys(swatches, 5);
    var modes = segmented(['auto', 'light', 'dark'].map(function (m) { return [m, t('set.mode_' + m)]; }),
      function (m) { set('appearance', m); }, t('set.appearance'));
    var modeNote = h('div', 'hint');
    add(look, modes.el, modeNote, swatches);

    // A list the user orders and switches the entries of: the menu bar items, the tabs, the Overview tiles.
    // The setting holds the entries that are on, in order; those that are off follow them.
    // least: Go refuses an empty menu bar and an empty tab strip, so the last entry stays on.
    // sideways: the menu bar list reads left to right in the menu bar, and keeps its checkboxes.
    function ordered(key, items, least, sideways) {
      var list = h('div');
      return { el: list, update: function (cfg) {
        var now = cfg[key], ids = items.map(function (i) { return i[0]; });
        sync(list, now.concat(ids.filter(function (id) { return now.indexOf(id) < 0; })), same, function (id) {
          var on = sideways ? h('input', 'check') : toggle(), up = h('button', 'mini up'), down = h('button', 'mini down');
          var label = items[ids.indexOf(id)][1];
          on.type = 'checkbox';
          up.appendChild(icon('chevron'));
          down.appendChild(icon('chevron'));
          up.setAttribute('aria-label', t(sideways ? 'set.move_left' : 'set.move_up_n', { name: label }));
          down.setAttribute('aria-label', t(sideways ? 'set.move_right' : 'set.move_down_n', { name: label }));
          up.title = sideways ? t('set.move_up') : up.getAttribute('aria-label');
          down.title = sideways ? t('set.move_down') : down.getAttribute('aria-label');
          var r = sideways ? add(h('label', 'set'), on, h('span', 'lbl', label), up, down) : add(h('label', 'set'), h('span', 'lbl', label), up, down, on);
          r.$ = { on: on, up: up, down: down };
          on.onchange = function () {
            var cur = state.settings[key];
            set(key, on.checked ? cur.concat(id) : cur.filter(function (k) { return k !== id; }));
          };
          var move = function (step) {
            return function (e) {
              e.preventDefault();
              var cur = state.settings[key].slice(), i = cur.indexOf(id);
              cur.splice(i, 1);
              cur.splice(i + step, 0, id);
              set(key, cur);
            };
          };
          up.onclick = move(-1);
          down.onclick = move(1);
          return r;
        }, function (r, id) {
          var i = now.indexOf(id);
          r.$.on.checked = i >= 0;
          r.$.on.disabled = i >= 0 && now.length <= least;
          r.$.up.hidden = r.$.down.hidden = i < 0;
          r.$.up.disabled = i <= 0;
          r.$.down.disabled = i === now.length - 1;
        });
      } };
    }

    var bar = group('menubar');
    var BAR = [['cpu', t('m.cpu')], ['mem', t('m.memory')], ['net', t('m.network')], ['temp', t('m.temperature')],
      ['battery', t('m.battery')], ['disk', t('m.disk')]];
    var barList = ordered('menu_bar', BAR, 1, true);
    bar.appendChild(barList.el);
    bar.appendChild(h('div', 'group-note', t('set.menu_bar_note')));
    var compact = toggle(function (on) { set('menu_bar_compact', on); });
    row(bar, t('set.compact'), t('set.compact_hint'), compact);
    var graphs = toggle(function (on) { set('menu_bar_graph', on); });
    row(bar, t('set.graphs'), t('set.graphs_hint'), graphs);
    var separate = toggle(function (on) { set('menu_bar_separate', on); });
    row(bar, t('set.separate'), t('set.separate_hint'), separate);
    // The clock is an item of its own, so the metrics item looks the same with and without it.
    var clockOn = toggle(function (on) { set('clock', on); });
    row(bar, t('set.clock'), t('set.clock_hint'), clockOn);
    var clockDate = toggle(function (on) { set('clock_date', on); }), clockSeconds = toggle(function (on) { set('clock_seconds', on); });
    var hours = segmented([['auto', t('set.mode_auto')], ['12', '12'], ['24', '24']], function (v) { set('clock_hours', v); }, t('set.clock_hours'));
    var toClock = h('button', 'btn', t('act.show'));
    toClock.onclick = function () { show('detail:clock'); };
    var clockRows = [row(bar, t('set.clock_date'), '', clockDate), row(bar, t('set.clock_seconds'), '', clockSeconds),
      row(bar, t('set.clock_hours'), '', hours.el), row(bar, t('clock.world'), t('set.clock_world_hint'), toClock)];

    var TILE_NAMES = { sensors: 'm.sensors', apps: 'top_apps', sleep: 'sleep.title' };
    var arrange = group('layout');
    var tabList = ordered('tab_order', TABS.slice(0, -1).map(function (id) { return [id, t('tab.' + id)]; }), 1);
    var tileList = ordered('tile_order', TILE_ORDER.map(function (id) { return [id, t(TILE_NAMES[id] || 'm.' + id)]; }), 0);
    var reset = h('button', 'btn rule-new', t('set.reset'));
    reset.onclick = function () {
      set('tab_order', TABS.slice(0, -1));
      set('tile_order', TILE_ORDER);
    };
    add(arrange, h('div', 'group-sub', t('set.arrange_tabs')), tabList.el, h('div', 'group-sub', t('set.arrange_tiles')), tileList.el, reset);

    var alertGroup = group('alerts');
    var alertsOn = toggle(function (on) { set('alerts', on); });
    row(alertGroup, t('set.alerts'), t('set.alerts_hint'), alertsOn);
    var cpuAt = number('alert_cpu', 1, 100, '%', same, same);
    var tempAt = number('alert_temp', 40, 110, '°C',
      function (c) { return settings.temp_unit === 'F' ? c * 9 / 5 + 32 : c; },
      function (v) { return settings.temp_unit === 'F' ? (v - 32) * 5 / 9 : v; });
    var diskAt = number('alert_disk_free', 1, 50, '%', same, same);
    // Zero switches one of these three off; the hold time is how long a condition must last before it alerts.
    var batAt = number('alert_battery', 0, 50, '%', same, same), memAt = number('alert_memory', 0, 99, '%', same, same);
    var swapAt = number('alert_swap', 0, 64, 'GB', same, same), holdAt = number('alert_hold', 10, 600, t('unit.s'), same, same);
    var limits = [cpuAt, tempAt, diskAt, batAt, memAt, swapAt, holdAt];
    var thresholds = [
      row(alertGroup, t('set.cpu_above'), '', cpuAt.el),
      row(alertGroup, t('set.temp_above'), t('temp.rule_hint'), tempAt.el),
      row(alertGroup, t('set.disk_below'), '', diskAt.el),
      row(alertGroup, t('set.battery_below'), t('set.zero_off'), batAt.el),
      row(alertGroup, t('set.memory_above'), t('set.zero_off'), memAt.el),
      row(alertGroup, t('set.swap_above'), t('set.zero_off'), swapAt.el),
      row(alertGroup, t('set.hold'), t('set.hold_hint'), holdAt.el)
    ];

    // One line of a list the user edits by removing from it: a muted app, an app rule.
    function removable(onRemove) {
      var r = h('div', 'set'), a = h('span'), b = h('span', 'hint'), x = h('button', 'mini');
      x.appendChild(icon('close'));
      x.onclick = function () { onRemove(r.$.item); };
      r.$ = { a: a, b: b, x: x };
      return add(r, add(h('span', 'lbl'), a, b), x);
    }

    var mutedGroup = group('alerts', t('set.muted')), mutedList = h('div'), mutedNone = h('div', 'group-note', t('set.muted_none'));
    add(mutedGroup, mutedList, mutedNone);

    // A rule is written as a sentence: the dictionary string is cut at its {placeholders}, each of
    // which becomes a control, so every language keeps its own word order.
    var ruleGroup = group('alerts', t('set.rules'), hintOf(t('set.rules'), ['rule.help_cpu', 'rule.help_memory', 'rule.help_fire']));
    var ruleList = h('div'), form = h('div', 'rule-form');
    var ruleApp = h('input', 'field'), names = h('datalist'), ruleLimit = h('input', 'field'), ruleMin = h('input', 'field');
    var ruleMetric = add(h('select', 'field'), new Option(t('rule.cpu'), 'cpu'), new Option(t('rule.memory'), 'memory'));
    var ruleAdd = h('button', 'btn primary', t('set.add')), ruleCancel = h('button', 'btn', t('cancel')), ruleNew = h('button', 'btn rule-new', t('set.add_rule'));
    var ruleNone = h('div', 'group-note', t('set.rules_none')), turnOn = h('button', 'btn link sm', t('set.alerts_on'));
    var ruleOff = add(h('div', 'rule-off'), icon('alert'), h('span', '', t('set.rules_off')), turnOn);
    var ruleProblem = h('div', 'hint error');
    ruleProblem.setAttribute('role', 'alert');
    names.id = 'rule-apps';
    ruleApp.setAttribute('list', names.id);
    ruleApp.placeholder = t('col.app');
    ruleApp.setAttribute('aria-label', t('col.app'));
    ruleApp.spellcheck = false;
    ruleMetric.setAttribute('aria-label', t('hist.metric'));
    ruleLimit.type = ruleMin.type = 'number';
    ruleLimit.value = 150;
    ruleLimit.setAttribute('aria-label', t('rule.limit'));
    ruleMin.value = 5;
    ruleMin.setAttribute('aria-label', t('rule.minutes'));
    var controls = { app: ruleApp, limit: ruleLimit, metric: ruleMetric, minutes: ruleMin };
    t('rule.sentence').split(/\{(\w+)\}/).forEach(function (part, i) {
      if (i % 2) add(form, controls[part]);
      else if (part.trim()) add(form, h('span', '', part.trim()));
    });
    add(form, names, ruleProblem, add(h('span', 'rule-btns'), ruleCancel, ruleAdd));
    form.hidden = true;
    add(ruleGroup, h('div', 'group-note', t('set.rules_note')), ruleOff, ruleList, ruleNone, form, ruleNew);
    turnOn.onclick = function () { set('alerts', true); };

    // The same bounds Go checks: a name of at most 200 bytes, a positive limit, 1–60 minutes, at most
    // 20 rules. What the user can only guess at is said under the fields: a name too long, a rule already there.
    function draft() {
      var rule = { app: ruleApp.value.trim(), metric: ruleMetric.value, limit: Number(ruleLimit.value), minutes: Number(ruleMin.value) };
      var have = state ? state.settings.alert_rules : [], same = JSON.stringify([rule.app, rule.metric, rule.limit, rule.minutes]);
      var problem = new TextEncoder().encode(rule.app).length > 200 ? 'rule.long'
        : have.some(function (x) { return JSON.stringify([x.app, x.metric, x.limit, x.minutes]) === same; }) ? 'rule.dup' : '';
      put(ruleProblem, problem && t(problem));
      ruleProblem.hidden = !problem;
      var ok = rule.app && !problem && rule.limit > 0 && rule.minutes >= 1 && rule.minutes <= 60 && rule.minutes % 1 === 0;
      return ok && state && have.length < 20 ? rule : null;
    }
    function editRule(on) {
      form.hidden = !on;
      ruleNew.hidden = on;
      if (on) ruleApp.focus();
    }
    form.oninput = function () { ruleAdd.disabled = !draft(); };
    ruleNew.onclick = function () { editRule(true); };
    ruleCancel.onclick = function () { editRule(false); };
    ruleAdd.onclick = function () {
      var rule = draft();
      if (!rule) return;
      set('alert_rules', state.settings.alert_rules.concat(rule));
      ruleApp.value = '';
      editRule(false);
    };

    var general = group('general');
    // Every language is listed under its own name, so the list reads the same whatever the current one is.
    var language = add(h('select', 'field'), new Option(t('set.system'), 'system'));
    Object.keys(I18N).forEach(function (code) { language.appendChild(new Option(I18N[code]['lang.name'], code)); });
    language.onchange = function () { set('language', language.value); };
    row(general, t('set.language'), '', language);
    var login = toggle(function (on) { send({ type: 'login_item', enabled: on }); });
    row(general, t('set.login'), t('set.login_hint'), login);
    var dock = toggle(function (on) { set('show_in_dock', on); });
    row(general, t('set.dock'), t('set.dock_hint'), dock);
    var onTop = toggle(function (on) { set('window_on_top', on); });
    row(general, t('set.on_top'), t('set.on_top_hint'), onTop);
    var units = group('general', t('set.units'));
    var tempUnit = segmented([['C', '°C'], ['F', '°F']], function (v) { set('temp_unit', v); }, t('set.temp_unit'));
    var netUnit = segmented([['bytes', 'MB/s'], ['bits', 'Mb/s']], function (v) { set('net_unit', v); }, t('set.net_unit'));
    // The labels differ only in case, which a screen reader does not voice.
    netUnit.buttons[0].setAttribute('aria-label', t('set.mbytes'));
    netUnit.buttons[1].setAttribute('aria-label', t('set.mbits'));
    row(units, t('m.temperature'), '', tempUnit.el);
    row(units, t('set.net_speed'), t('set.net_speed_hint'), netUnit.el);

    add(group('general'), add(h('div', 'set upd'), add(h('span', 'lbl'), h('b', '', 'mac-pulse ' + version), updOut, releases), check));

    var keys = group('shortcuts');
    var hotkey = toggle(function (on) { set('hotkey', on); });
    row(keys, t('set.hotkey'), t('set.hotkey_hint'), hotkey);
    var hotkeyQuit = toggle(function (on) { set('hotkey_quit', on); });
    row(keys, t('set.hotkey_quit'), t('set.hotkey_quit_hint'), hotkeyQuit);
    // What the open panel answers to (the keydown handler at the end of this file); nothing here can be changed.
    var inPanel = group('shortcuts', t('keys.title'));
    [['⌘1 … ⌘6', 'keys.tabs'], ['⌘,', 'tab.settings'], ['⌘F', 'keys.search'], ['⌘[', 'keys.back'], ['Esc', 'keys.esc']].forEach(function (k) {
      add(inPanel, add(h('div', 'set'), h('span', 'lbl', t(k[1])), h('kbd', '', k[0])));
    });

    var quit = h('button', 'btn', t('set.quit'));
    quit.onclick = function () { send({ type: 'quit' }); };
    add(menu, add(h('div', 'settings-foot'), quit));
    add(el, menu, add(pane, box));

    function update(s) {
      var cfg = s.settings;
      radios.forEach(function (b, i) {
        var on = SKINS[i] === root.dataset.skin;
        attr(b, 'aria-checked', String(on));
        b.tabIndex = on ? 0 : -1;
        attr(b.firstChild, 'data-theme', SKIN_MODE[SKINS[i]] || root.dataset.theme);
      });
      modes.set(cfg.appearance);
      // A theme drawn for one mode keeps it: the segment says so instead of looking broken.
      var only = SKIN_MODE[root.dataset.skin];
      modeNote.hidden = !only;
      put(modeNote, only ? t('set.' + only + '_only') : '');
      tempUnit.set(cfg.temp_unit);
      netUnit.set(cfg.net_unit);

      barList.update(cfg);
      tabList.update(cfg);
      tileList.update(cfg);
      separate.checked = cfg.menu_bar_separate;
      dock.checked = cfg.show_in_dock;
      clockOn.checked = cfg.clock;
      clockDate.checked = cfg.clock_date;
      clockSeconds.checked = cfg.clock_seconds;
      hours.set(cfg.clock_hours);
      clockRows.forEach(function (r) { r.hidden = !cfg.clock; });

      alertsOn.checked = cfg.alerts;
      put(tempAt.unit, cfg.temp_unit === 'F' ? '°F' : '°C');
      limits.forEach(function (f, i) {
        thresholds[i].classList.toggle('off', !cfg.alerts);
        f.input.disabled = !cfg.alerts;
        f.input.min = Math.round(f.toView(f.min));
        f.input.max = Math.round(f.toView(f.max));
        if (document.activeElement !== f.input) f.input.value = Math.round(f.toView(cfg[f.key]));
      });
      compact.checked = cfg.menu_bar_compact;
      graphs.checked = cfg.menu_bar_graph;

      mutedNone.hidden = cfg.alert_muted.length > 0;
      sync(mutedList, cfg.alert_muted, same, function () {
        return removable(function (name) { set('alert_muted', state.settings.alert_muted.filter(function (x) { return x !== name; })); });
      }, function (r, name) {
        r.$.item = name;
        put(r.$.a, name);
        attr(r.$.x, 'aria-label', t('set.remove', { name: name }));
      });
      sync(ruleList, cfg.alert_rules, JSON.stringify, function () {
        // By its place in the list: two rules may read the same, and only the one pressed goes.
        return removable(function (at) { set('alert_rules', state.settings.alert_rules.filter(function (x, i) { return i !== at; })); });
      }, function (r, rule) {
        r.$.item = cfg.alert_rules.indexOf(rule);
        put(r.$.a, rule.app);
        // Go skips every alert of a muted app, the user's own rules included.
        put(r.$.b, t('rule.line', { metric: t(rule.metric === 'cpu' ? 'm.cpu' : 'm.memory'), limit: rule.limit,
          unit: rule.metric === 'cpu' ? '%' : 'GB', minutes: rule.minutes }) + (cfg.alert_muted.indexOf(rule.app) < 0 ? '' : ' · ' + t('rule.muted')));
        attr(r.$.x, 'aria-label', t('set.remove', { name: rule.app }));
        r.classList.toggle('off', !cfg.alerts);
      });
      ruleOff.hidden = cfg.alerts;
      ruleNone.hidden = cfg.alert_rules.length > 0;
      ruleNew.disabled = cfg.alert_rules.length >= 20;
      var apps = s.apps.items.filter(function (a) { return !a.system; }).map(function (a) { return a.name; }).sort();
      sync(names, apps, same, function (name) { return new Option(name); }, function () {});
      ruleAdd.disabled = !draft();

      language.value = cfg.language || 'system';
      login.checked = cfg.launch_at_login;
      hotkey.checked = !!cfg.hotkey;
      hotkeyQuit.checked = cfg.hotkey_quit;
      onTop.checked = cfg.window_on_top;

      // What each section is set to now, under its title in the menu.
      put(rows.general.$, [dict['lang.name'], cfg.temp_unit === 'F' ? '°F' : '°C', cfg.net_unit === 'bits' ? 'Mb/s' : 'MB/s'].join(' · '));
      put(rows.appearance.$, t('theme.' + root.dataset.skin) + ' · ' + t('set.mode_' + (only || cfg.appearance)));
      put(rows.menubar.$, cfg.menu_bar.map(function (id) { return BAR.filter(function (i) { return i[0] === id; })[0][1]; }).join(', ') +
        (cfg.menu_bar_compact ? ' · ' + t('set.sum.compact') : ''));
      put(rows.layout.$, t('n.tab', { n: cfg.tab_order.length }) + ' · ' + t('n.tile', { n: cfg.tile_order.length }));
      put(rows.alerts.$, !cfg.alerts ? t('set.sum.alerts_off') : t('set.sum.alerts_on') +
        (cfg.alert_rules.length ? ' · ' + t('n.rule', { n: cfg.alert_rules.length }) : ''));
      put(rows.shortcuts.$, [cfg.hotkey && '⌃⌥P', cfg.hotkey_quit && '⌃⌥K'].filter(Boolean).join(' · ') || t('set.sum.keys_off'));
    }

    // open shows the menu ('') or a section and says whether the section covers the menu. The width is
    // measured: a panel the user has widened gets the sidebar like the window does.
    function open(sec) {
      var split = el.clientWidth >= 620, on = sec || (split ? last || 'general' : '');
      if (!menu.hidden) menu.$top = menu.scrollTop;
      el.classList.toggle('split', split);
      menu.hidden = !split && !!on;
      pane.hidden = !on;
      Object.keys(SECTIONS).forEach(function (id) {
        secs[id].hidden = id !== on;
        if (split && id === on) rows[id].setAttribute('aria-current', 'page');
        else rows[id].removeAttribute('aria-current');
      });
      // display: none drops the scroll offset of the menu too.
      menu.scrollTop = menu.$top || 0;
      if (on && on !== last) pane.scrollTop = 0;
      last = on || last;
      return !split && !!on;
    }

    return {
      el: el, update: update, language: language, scroller: pane, open: open, rows: rows,
      resize: function () { if (el.classList.contains('split') !== el.clientWidth >= 620) show('settings'); },
      // goTo brings a limit's field or the app rules into view: where an alert's screen sends the user to change them.
      goTo: function (key) {
        var field = limits.filter(function (f) { return f.key === key; })[0];
        if (field) field.input.focus();
        else ruleGroup.scrollIntoView({ block: 'center' });
      }
    };
  }

  // Detail screens. Per-app sparklines and sensor min / max are not in the
  // contract: they are collected here from the states that arrive and live only as long as the page.
  var series = new Map(), extremes = new Map(), stamp = 0;
  var THERMAL = { nominal: '', fair: '', serious: 'warning', critical: 'critical' };

  function record(s) {
    if (s.time === stamp) return;
    stamp = s.time;
    function push(kind, name, v) {
      var key = kind + '\n' + name, a = series.get(key);
      if (!a) series.set(key, a = []);
      a.push(v);
      if (a.length > SLOTS) a.shift();
      a.at = stamp;
    }
    if (s.apps.has_rates) {
      s.apps.items.forEach(function (a) {
        push('cpu', a.name, a.cpu);
        push('memory', a.name, a.memory);
        if (a.energy_mw != null) push('energy', a.name, a.energy_mw);
        if (a.disk_read_rate != null) push('disk', a.name, a.disk_read_rate + a.disk_write_rate);
      });
    }
    if (s.net && s.net.has_rates) s.net.apps.forEach(function (a) { push('net', a.name, a.down_rate + a.up_rate); });
    series.forEach(function (a, key) { if (a.at !== stamp) series.delete(key); });
    s.sensors.temps.forEach(function (t) {
      var e = extremes.get(t.key);
      if (!e) extremes.set(t.key, [t.temp_c, t.temp_c]);
      else { e[0] = Math.min(e[0], t.temp_c); e[1] = Math.max(e[1], t.temp_c); }
    });
  }

  // The hottest core sensor (the clusters carry it), or null when the Mac reports none: cpu.temp_c is the
  // power-management chip, which reads 20–40 °C lower under load, so it is never shown under the core's name.
  function hottest(c) {
    var v = c.clusters.map(function (k) { return k.temp_c; }).filter(function (x) { return x != null; });
    return v.length ? Math.max.apply(null, v) : null;
  }

  function deg(c) { return Math.round(settings.temp_unit === 'F' ? c * 9 / 5 + 32 : c); }

  function fansText(fans) {
    var on = fans.filter(function (f) { return f.rpm > 0; });
    if (!on.length) return t('off');
    var rpm = Math.round(on.reduce(function (sum, f) { return sum + f.rpm; }, 0) / on.length);
    return (on.length > 1 ? on.length + ' × ' : '') + rpm + ' rpm';
  }

  function card(title, cls) {
    var el = h('section', 'tile card' + (cls ? ' ' + cls : '')), side = h('span', 'side');
    add(el, add(h('div', 'card-h'), h('span', '', title), side));
    return { el: el, side: side };
  }

  function hint(text) { return h('div', 'hint', text); }

  // The status line of work that is running: a spinner, the word for the work set apart from
  // the text around it, then what is known so far.
  function progress(word) {
    var el = h('div', 'progress'), so = h('span', 'so-far');
    el.setAttribute('role', 'status');
    return { el: add(el, h('span', 'spin'), h('b', 'word', word), so), so: so };
  }

  // facts(['Uptime', 'Load']) is a label / value list; set() takes the values in the same order.
  function facts(labels, cls) {
    var el = h('div', 'facts' + (cls ? ' ' + cls : ''));
    var values = labels.map(function (label) {
      var v = h('b', '', DASH), name = h('span', '', label);
      name.title = label;
      add(el, add(h('div'), name, v));
      return v;
    });
    return { el: el, values: values, set: function (texts) { texts.forEach(function (t, i) { put(values[i], t); attr(values[i], 'title', t); }); } };
  }

  function hero(cls) {
    var el = h('section', 'tile hero ' + cls), big = bigValue(), pill = h('span', 'pill'), sub = h('div', 'sub');
    big.side.replaceWith(pill);
    pill.hidden = true;
    add(el, big.el, sub);
    return { el: el, num: big.num, unit: big.unit, pill: pill, sub: sub };
  }

  function setPill(pill, text, level) {
    pill.hidden = !text;
    put(pill, text);
    attr(pill, 'data-level', level || '');
  }

  // ring draws up to parts.length arcs on a circle whose circumference is 100 user units.
  function ring(parts) {
    var svg = document.createElementNS(SVG, 'svg'), box = h('div', 'donut'), label = h('div', 'ring-l');
    svg.setAttribute('viewBox', '0 0 36 36');
    svg.setAttribute('aria-hidden', 'true');
    var arcs = [''].concat(parts).map(function (cls, i) {
      var c = document.createElementNS(SVG, 'circle');
      c.setAttribute('cx', 18);
      c.setAttribute('cy', 18);
      c.setAttribute('r', 15.9155);
      c.setAttribute('class', i ? 'arc ' + cls : 'track');
      return svg.appendChild(c);
    }).slice(1);
    return {
      el: add(box, svg, label), label: label,
      set: function (fractions) {
        var from = 0;
        fractions.forEach(function (f, i) {
          var len = Math.max(0, f * 100 - (fractions.length > 1 ? 0.9 : 0));
          attr(arcs[i], 'stroke-dasharray', len.toFixed(2) + ' ' + (100 - len).toFixed(2));
          attr(arcs[i], 'stroke-dashoffset', (25 - from * 100).toFixed(2));
          from += f;
        });
      }
    };
  }

  // live is the 60-point (2 min) chart of a detail screen: the tile sparkline grown taller, with a hover
  // read-out in the card header and min / avg / max underneath. names: [[label, key class], …], one or two.
  function live(title, names, fmt) {
    var c = card(title, 'live'), two = names.length === 2;
    var graph = two ? mirror(64) : spark(56), cursor = h('i', 'cursor');
    var stats = h('div', 'stats'), cells = [], sets = [[]], at = -1;
    add(stats, h('span'), h('span', 'th', t('stat.min')), h('span', 'th', t('stat.avg')), h('span', 'th', t('stat.max')));
    names.forEach(function (n) {
      var row = [h('b', '', DASH), h('b', '', DASH), h('b', '', DASH)];
      add(stats, h('span', 'key dim ' + n[1], n[0]), row[0], row[1], row[2]);
      cells.push(row);
    });
    cursor.hidden = true;
    graph.el.appendChild(cursor);
    graph.el.style.height = (two ? 64 : 56) + 'px';
    add(c.el, graph.el, stats);

    function read() {
      var i = at < 0 ? -1 : at - (SLOTS - sets[0].length);
      cursor.hidden = i < 0;
      if (i < 0) { put(c.side, t('live.window')); return; }
      css(cursor, 'left', (at / (SLOTS - 1) * 100).toFixed(2) + '%');
      // Assumes the 2 s tick; points sampled while the panel was closed are 5 s apart.
      put(c.side, sets.map(function (p) { return fmt(p[i]); }).join(' / ') + ' · ' + t('live.ago', { s: (SLOTS - 1 - at) * 2 }));
    }

    graph.el.addEventListener('pointermove', function (e) {
      var r = graph.el.getBoundingClientRect();
      at = Math.max(0, Math.min(SLOTS - 1, Math.round((e.clientX - r.left) / r.width * (SLOTS - 1))));
      read();
    });
    graph.el.addEventListener('pointerleave', function () { at = -1; read(); });

    return {
      el: c.el,
      set: function (a, max, b) {
        sets = two ? [a, b] : [a];
        if (two) graph.set(a, b); else graph.set(a, max);
        sets.forEach(function (p, k) {
          var sum = p.reduce(function (x, y) { return x + y; }, 0);
          put(cells[k][0], p.length ? fmt(Math.min.apply(null, p)) : DASH);
          put(cells[k][1], p.length ? fmt(sum / p.length) : DASH);
          put(cells[k][2], p.length ? fmt(Math.max.apply(null, p)) : DASH);
        });
        read();
      }
    };
  }

  // ranking orders apps by rank(app) (null = not measurable, left out) and shows five with a sparkline each.
  function ranking(title, kind, heads, rank, cells, note) {
    var c = card(title, 'rank'), list = h('div', 'list'), empty = hint(t('rank.empty'));
    var head = add(h('div', 'row head'), h('span', 'th', t('col.app')));
    heads.forEach(function (t) { head.appendChild(h('span', 'th r', t)); });
    c.el.style.setProperty('--n', heads.length);
    add(c.el, head, list, empty, note && hint(note));
    return {
      el: c.el,
      set: function (apps, ready) {
        var items = apps.filter(function (a) { return rank(a) != null; })
          .sort(function (a, b) { return rank(b) - rank(a); }).slice(0, 5);
        sync(list, items, function (a) { return a.name; }, function (a) {
          var row = h('div', 'row link-row'), name = h('span', 'name'), svg = sparkSvg(14);
          row.$ = { name: name, draw: sparkPart(svg, ''), cells: heads.map(function () { return h('span', 'r'); }) };
          row.onclick = function () { findApp(row.$.name.textContent); };
          add(row, appIcon(a.icon, a.system), name, sparkBox(svg));
          row.$.cells.forEach(function (cell) { row.appendChild(cell); });
          return row;
        }, function (row, a) {
          var points = series.get(kind + '\n' + a.name) || [];
          put(row.$.name, a.name);
          attr(row.$.name, 'title', a.name);
          row.classList.toggle('sys', !!a.system);
          row.$.draw(points, Math.max.apply(null, points.concat(1e-9)), 1, 13);
          cells(a).forEach(function (text, i) { put(row.$.cells[i], ready ? text : DASH); });
        });
        head.hidden = list.hidden = !items.length;
        empty.hidden = items.length > 0;
      }
    };
  }

  // An action button sends one message; Go answers through window.mp.onAction and the text lands in
  // out, or in a toast when the button has no line of its own. Nothing is stored: leaving the screen clears it.
  // opts: cls (button classes), msg (builds a message with fields), wait (ms before giving up), done (sees the answer).
  var acting = {};
  function action(type, label, out, opts) {
    opts = opts || {};
    var btn = h('button', 'btn ' + (opts.cls || ''), label);
    btn.onclick = function () {
      // One request of a kind at a time: Go drops a repeat, and its answer would land on the wrong button.
      if (acting[type]) return;
      btn.classList.add('busy');
      if (out) {
        out.classList.remove('error');
        put(out, '');
      }
      var done = acting[type] = function (r) {
        if (acting[type] !== done) return;
        delete acting[type];
        btn.classList.remove('busy');
        if (opts.done) opts.done(r);
        if (!out) { toast({ level: r.ok ? 'info' : 'error', text: r.text }); return; }
        out.classList.toggle('error', !r.ok);
        put(out, r.text);
        attr(out, 'title', r.text);
      };
      // Go gives up on its own first: after 5 s, 30 s for an eject or 40 s for the speed test.
      setTimeout(function () { done({ ok: false, text: t('act.no_answer') }); }, opts.wait || 8000);
      send(opts.msg ? opts.msg() : { type: type });
    };
    return btn;
  }

  var SCREENS = {
    cpu: function (L, R) {
      var hd = hero('t-cpu'), split = meter(2);
      var user = h('b', '', DASH), sys = h('b', '', DASH), idle = h('b', '', DASH);
      add(hd.el, split.el, add(h('div', 'cap'),
        add(h('span', 'key'), document.createTextNode(t('cpu.user')), user),
        add(h('span', 'key k2'), document.createTextNode(t('cpu.system')), sys),
        add(h('span', 'key ring'), document.createTextNode(t('cpu.idle')), idle)));
      var usage = live(t('usage'), [[t('cpu.total'), '']], pct);
      var cores = card(t('cpu.cores')), clusters = h('div', 'clusters');
      cores.el.appendChild(clusters);
      var info = facts([t('cpu.load_avg'), t('cpu.uptime'), t('cpu.hottest'), t('cpu.temp'), t('cpu.power')]);
      info.values[3].previousSibling.title = t('cpu.temp_tip');
      var infoCard = card(t('details'));
      infoCard.el.appendChild(info.el);
      var apps = ranking(t('top_apps'), 'cpu', [t('m.cpu')], function (a) { return a.cpu; }, function (a) { return [percent(a.cpu)]; });
      add(L, usage.el, cores.el, infoCard.el);
      add(R, apps.el);

      return { hero: hd.el, metric: 'cpu', update: function (s) {
        var c = s.cpu, n = c.cores.length, from = 0;
        put(hd.num, String(Math.round(c.total)));
        put(hd.unit, '%');
        setPill(hd.pill, t('thermal.pill', { state: t('thermal.' + s.sensors.thermal) }), THERMAL[s.sensors.thermal]);
        attr(hd.pill, 'title', t('thermal.tip'));
        put(hd.sub, [c.model, t('n.core', { n: n }) + (c.clusters.length ? ' (' + c.clusters.map(function (k) {
          return k.cores + k.name.charAt(0);
        }).reverse().join(' + ') + ')' : '')].filter(Boolean).join(' · '));
        fill(split.segs[0], c.user);
        fill(split.segs[1], c.system);
        put(user, pct(c.user));
        put(sys, pct(c.system));
        put(idle, pct(Math.max(0, 100 - c.user - c.system)));
        usage.set(s.spark.cpu, 100);
        put(cores.side, t('n.core', { n: n }));

        var groups = c.clusters.length ? c.clusters : [{ name: 'All', cores: n, freq_mhz: null, temp_c: null }];
        sync(clusters, groups, function (g) { return g.name; }, function () {
          var box = h('div', 'cluster'), name = h('span', 'name'), avg = h('b'), freq = h('span'), heat = h('span'), bars = h('div', 'cores');
          box.$ = { name: name, avg: avg, freq: freq, heat: heat, bars: bars };
          bars.setAttribute('role', 'img');
          heat.title = t('cpu.cluster_tip');
          return add(box, add(h('div', 'cap'), name, avg, freq, heat), bars);
        }, function (box, g) {
          var mine = c.cores.slice(from, from + g.cores), bars = box.$.bars;
          from += g.cores;
          var name = named('cluster.', g.name);
          put(box.$.name, name);
          put(box.$.avg, mine.length ? pct(mine.reduce(function (a, b) { return a + b; }, 0) / mine.length) : DASH);
          // A null frequency is an idle cluster unless IOReport gave nothing at all (then power is null too).
          put(box.$.freq, g.freq_mhz == null ? (s.power.cpu_w == null ? DASH : t('idle')) : mhz(g.freq_mhz));
          put(box.$.heat, g.temp_c == null ? DASH : t('max_n', { v: temp(g.temp_c) }));
          while (bars.children.length < mine.length) bars.appendChild(h('b')).appendChild(h('i'));
          while (bars.children.length > mine.length) bars.lastChild.remove();
          mine.forEach(function (v, i) { css(bars.children[i].firstChild, 'height', Math.round(v) + '%'); });
          attr(bars, 'aria-label', name + ': ' + mine.map(pct).join(', '));
          attr(bars, 'title', mine.map(pct).join('  '));
        });

        info.set([c.load.map(function (v) { return fx(v, 2); }).join('  '), dur(c.uptime_s), temp(hottest(c)), temp(c.temp_c), watts(s.power.cpu_w)]);
        info.values[2].parentNode.hidden = hottest(c) == null;
        apps.set(s.apps.items, s.apps.has_rates);
      } };
    },

    gpu: function (L, R) {
      var hd = hero('t-gpu');
      var load = card(t('gpu.utilization')), rows = [t('gpu.device'), t('gpu.renderer'), t('gpu.tiler')].map(function (label) {
        var m = meter(1), v = h('b', '', DASH);
        add(load.el, add(h('div', 'bar-row'), h('span', '', label), m.el, v));
        return { bar: m.segs[0], value: v };
      });
      var usage = live(t('usage'), [[t('gpu.device'), '']], pct);
      var info = facts([t('gpu.mem_used'), t('gpu.mem_alloc'), t('m.power'), t('gpu.frequency'), t('m.temperature')]);
      var infoCard = card(t('details'));
      add(infoCard.el, info.el, hint(t('gpu.note')));
      add(L, load.el, usage.el);
      add(R, infoCard.el);

      return { hero: hd.el, metric: 'gpu', update: function (s) {
        var g = s.gpu;
        if (!g) return;
        put(hd.num, String(Math.round(g.util)));
        put(hd.unit, '%');
        put(hd.sub, g.model || t('gpu.fallback'));
        [g.util, g.renderer, g.tiler].forEach(function (v, i) {
          fill(rows[i].bar, v);
          put(rows[i].value, pct(v));
        });
        usage.set(s.spark.gpu, 100);
        info.set([join(bytes(g.memory)), join(bytes(g.memory_alloc)), watts(s.power.gpu_w),
          g.freq_mhz == null ? (s.power.gpu_w == null ? DASH : t('idle')) : mhz(g.freq_mhz), temp(g.temp_c)]);
      } };
    },

    memory: function (L, R) {
      var hd = hero('t-mem'), donut = ring(['k1', 'k2', 'k3', 'k4']);
      var PARTS = [['k1', 'app'], ['k2', 'wired'], ['k3', 'compressed'], ['k4', 'cached'], ['ring', 'free']];
      var legend = h('div', 'ring-legend'), cells = PARTS.map(function (p) {
        var size = h('b', '', DASH), share = h('span', 'r dim', DASH);
        add(legend, h('span', 'key ' + p[0], t('mem.' + p[1])), size, share);
        return { size: size, share: share };
      });
      add(hd.el, add(h('div', 'ring-row'), donut.el, legend));
      var used = live(t('mem.used'), [[t('mem.used'), '']], pct);
      var swap = card(t('mem.swap')), swapBar = meter(1);
      var paging = facts([t('mem.page_in'), t('mem.page_out'), t('mem.swap_in'), t('mem.swap_out'), t('mem.compress'), t('mem.decompress')], 'two');
      add(swap.el, swapBar.el, h('div', 'card-h sub-h', t('mem.paging')), paging.el);
      var apps = ranking(t('top_apps'), 'memory', [t('m.memory')], function (a) { return a.memory; }, function (a) { return [join(bytes(a.memory))]; });
      add(L, used.el, swap.el);
      add(R, apps.el);

      return { hero: hd.el, metric: 'memory', update: function (s) {
        var m = s.memory, ready = s.has_rates;
        var value = bytes(m.used);
        put(hd.num, value[0]);
        put(hd.unit, value[1]);
        put(hd.sub, t('mem.used_of', { total: join(bytes(m.total)) }));
        setPill(hd.pill, t('mem.pressure_tip', { level: t('pressure.' + m.pressure) }), m.pressure === 'normal' ? '' : m.pressure);
        donut.set([m.app, m.wired, m.compressed, m.cached].map(function (v) { return v / m.total; }));
        put(donut.label, pct(m.used / m.total * 100));
        PARTS.forEach(function (p, i) {
          put(cells[i].size, join(bytes(m[p[1]])));
          put(cells[i].share, pct(m[p[1]] / m.total * 100));
        });
        used.set(s.spark.memory, 100);
        put(swap.side, m.swap_total ? t('x_of_y', { x: join(bytes(m.swap_used)), y: join(bytes(m.swap_total)) }) : t('mem.swap_unused'));
        fill(swapBar.segs[0], m.swap_total ? m.swap_used / m.swap_total * 100 : 0);
        paging.set([m.page_in_rate, m.page_out_rate, m.swap_in_rate, m.swap_out_rate, m.compress_rate, m.decompress_rate]
          .map(function (v) { return join(rate(ready ? v : null, 'disk')); }));
        apps.set(s.apps.items, true);
      } };
    },

    disk: function (L, R) {
      var hd = hero('t-disk');
      var vols = card(t('disk.volumes')), volList = h('div', 'list');
      vols.el.appendChild(volList);
      var io = live(t('disk.activity'), [[t('disk.read'), 'down'], [t('disk.write'), 'up']], function (v) { return join(rate(v, 'disk')); });
      var info = facts([t('disk.reading'), t('disk.writing'), t('disk.written_today'), t('disk.read_boot'), t('disk.written_boot')]);
      var infoCard = card(t('details'));
      infoCard.el.appendChild(info.el);
      var apps = ranking(t('top_apps'), 'disk', [t('disk.read'), t('disk.write')],
        function (a) { return a.disk_read_rate == null ? null : a.disk_read_rate + a.disk_write_rate; },
        function (a) { return [join(rate(a.disk_read_rate, 'disk')), join(rate(a.disk_write_rate, 'disk'))]; },
        t('disk.own_note'));
      var clean = cleanupCard(), toMap = h('button', 'tile strip go');
      add(toMap, icon('folder'), h('span', 'lbl', t('stor.map')), icon('chevron', 'go-i'));
      toMap.onclick = function () { show('storage'); };
      add(L, vols.el, toMap, io.el);
      add(R, infoCard.el, apps.el);
      var ejected = {};

      return { hero: hd.el, metric: 'disk', shown: function () { vols.el.after(clean.el); }, update: function (s) {
        var d = s.disk, free = bytes(d.free, 1000), ready = s.has_rates;
        clean.update(s);
        toMap.hidden = !shownTab('storage');
        put(hd.num, free[0]);
        put(hd.unit, free[1]);
        put(hd.sub, [t('disk.free_of', { total: join(bytes(d.total, 1000)) }), d.model].filter(Boolean).join(' · '));
        setPill(hd.pill, d.smart ? 'S.M.A.R.T. ' + t('smart.' + d.smart) : '', d.smart === 'failing' ? 'critical' : '');
        // The volume list refreshes every 30 s: an ejected volume leaves the screen at once and is
        // forgotten when Go stops listing it, so mounting it again shows it.
        var volumes = d.volumes.filter(function (v) { return !ejected[v.mount]; });
        Object.keys(ejected).forEach(function (m) {
          if (!d.volumes.some(function (v) { return v.mount === m; })) delete ejected[m];
        });
        put(vols.side, t('n.volume', { n: volumes.length }));
        sync(volList, volumes, function (v) { return v.mount; }, function (v) {
          var row = h('div', 'vol'), name = h('span', 'name'), fs = h('span', 'badge'), space = h('span', 'dim'), bar = meter(1);
          var reveal = h('button', 'btn link sm', t('act.reveal'));
          // Go takes a mount only if its last sample lists it; a path of our own making is refused.
          reveal.onclick = function () { send({ type: 'reveal', mount: v.mount }); };
          var eject = action('eject', t('act.eject'), null, {
            cls: 'sm',
            // A disk with pending writes takes its time; Go gives up after 30 s.
            wait: 31000,
            msg: function () { return { type: 'eject', mount: v.mount }; },
            done: function (r) {
              if (!r.ok) return;
              ejected[v.mount] = true;
              if (state) tabs[current].update(state);
            }
          });
          row.$ = { name: name, fs: fs, space: space, bar: bar, eject: eject };
          return add(row, add(h('div', 'vol-h'), name, fs, space), add(h('div', 'vol-b'), bar.el, reveal, eject));
        }, function (row, v) {
          row.$.eject.hidden = !v.ejectable;
          // The volume list refreshes every 30 s; the boot volume follows the hero's number.
          var left = v.mount === '/' ? d.free : v.free, freePct = v.total ? left / v.total * 100 : 100;
          put(row.$.name, v.name);
          attr(row, 'title', v.name + ' — ' + v.mount);
          put(row.$.fs, v.fs.toUpperCase());
          put(row.$.space, t('disk.x_free_of', { free: join(bytes(left, 1000)), total: join(bytes(v.total, 1000)) }));
          fill(row.$.bar.segs[0], 100 - freePct);
          attr(row.$.bar.el, 'data-level', freePct < s.settings.alert_disk_free / 2 ? 'crit' : freePct < s.settings.alert_disk_free ? 'warn' : '');
        });
        io.set(s.spark.disk_read, 0, s.spark.disk_write);
        info.set([join(rate(ready ? d.read_rate : null, 'disk')), join(rate(ready ? d.write_rate : null, 'disk')),
          join(bytes(d.written_today, 1000)), join(bytes(d.read_total, 1000)), join(bytes(d.write_total, 1000))]);
        apps.set(s.apps.items, s.apps.has_rates);
      } };
    },

    network: function (L, R) {
      var el = h('section', 'tile hero t-net'), sub = h('div', 'sub');
      var halves = [['down', t('net.download')], ['up', t('net.upload')]].map(function (d) {
        var big = bigValue(), today = h('span', 'end');
        el.appendChild(add(h('div', 'half ' + d[0]), add(h('div', 'cap'), icon(d[0]), h('span', '', d[1]), today), big.el));
        return { num: big.num, unit: big.unit, today: today };
      });
      el.appendChild(sub);
      var traffic = live(t('net.traffic'), [[t('col.down'), 'down'], [t('col.up'), 'up']], function (v) { return join(rate(v)); });
      var totals = h('div', 'totals'), sums = [[t('today'), 'today'], [t('days', { n: 7 }), '7d'], [t('days', { n: 30 }), '30d']].map(function (x) {
        var down = h('b', '', DASH), up = h('b', '', DASH);
        add(totals, h('span', '', x[0]), add(h('span', 'cap'), icon('down'), down), add(h('span', 'cap'), icon('up'), up));
        return { down: down, up: up, key: x[1] };
      });
      traffic.el.appendChild(totals);
      var ifs = card(t('net.interfaces')), ifList = h('div', 'list'), noIfs = hint(t('net.no_if'));
      add(ifs.el, ifList, noIfs);
      var wifi = card('Wi-Fi'), noWifi = hint(t('net.no_wifi'));
      var link = facts([t('wifi.signal'), t('wifi.noise'), t('wifi.channel'), t('wifi.width'), t('wifi.speed'), t('wifi.standard'), t('wifi.security')], 'two');
      add(wifi.el, link.el, noWifi);
      var inet = card(t('net.internet')), route = facts([t('net.router'), 'DNS']);
      var ipOut = h('b', 'act-out'), pingOut = h('b', 'act-out'), speedOut = h('b', 'act-out');
      add(inet.el, route.el,
        add(h('div', 'act'), h('span', '', t('net.public_ip')), ipOut, action('public_ip', t('act.show'), ipOut)),
        add(h('div', 'act'), h('span', '', 'Ping 1.1.1.1'), pingOut, action('ping', 'Ping', pingOut)),
        hint(t('net.cf_note')),
        add(h('div', 'act'), h('span', '', t('net.speedtest')), speedOut, action('speedtest', t('act.run'), speedOut, { wait: 45000 })),
        hint(t('net.speed_note')));
      var apps = ranking(t('top_apps'), 'net', [t('col.down'), t('col.up')], function (a) { return a.down_rate + a.up_rate; },
        function (a) { return [join(rate(a.down_rate)), join(rate(a.up_rate))]; });
      var more = h('button', 'btn link sm', t('net.open_inspector'));
      more.appendChild(icon('chevron'));
      more.onclick = function () { show('network'); };
      apps.el.appendChild(more);
      var dev = card(t('net.dev')), devList = h('div', 'list');
      add(dev.el, devList, hint(t('net.dev_note')));
      add(L, traffic.el, ifs.el, wifi.el);
      add(R, inet.el, apps.el, dev.el);

      return { hero: el, metric: 'network', outs: [ipOut, pingOut, speedOut], update: function (s) {
        var info = s.net_info, ready = s.has_rates;
        [s.network.down_rate, s.network.up_rate].forEach(function (v, i) {
          var r = rate(ready ? v : null);
          put(halves[i].num, r[0]);
          put(halves[i].unit, r[1]);
        });
        put(halves[0].today, t('today_n', { x: join(bytes(s.network.down_today)) }));
        put(halves[1].today, t('today_n', { x: join(bytes(s.network.up_today)) }));
        var main = info && info.interfaces.filter(function (i) { return i.primary; })[0];
        var w = info && info.wifi;
        function ifName(i) { return w && w.interface === i.name ? 'Wi-Fi (' + i.name + ')' : i.name; }
        put(sub, main ? [ifName(main), main.ipv4[0] || main.ipv6[0]].filter(Boolean).join(' · ') : info ? t('net.no_route') : '');
        attr(sub, 'title', sub.textContent);
        traffic.set(s.spark.net_down, 0, s.spark.net_up);
        sums.forEach(function (x) {
          put(x.down, join(bytes(s.network['down_' + x.key])));
          put(x.up, join(bytes(s.network['up_' + x.key])));
        });

        var list = info ? info.interfaces : [];
        sync(ifList, list, function (i) { return i.name; }, function () {
          var row = h('div', 'if'), name = h('b'), tag = h('span', 'badge', t('net.primary')), v4 = h('span', 'a'), rest = h('span', 'b');
          row.$ = { name: name, tag: tag, v4: v4, rest: rest };
          return add(row, add(h('div', 'if-h'), name, tag, v4), rest);
        }, function (row, i) {
          var rest = [i.mac].concat(i.ipv6).filter(Boolean).join(' · ');
          put(row.$.name, ifName(i));
          row.$.tag.hidden = !i.primary;
          put(row.$.v4, i.ipv4.join(', ') || DASH);
          put(row.$.rest, rest);
          attr(row, 'title', [i.name].concat(i.ipv4, i.ipv6, i.mac).filter(Boolean).join('\n'));
        });
        noIfs.hidden = !info || list.length > 0;
        put(ifs.side, info ? '' : t('reading'));

        wifi.el.hidden = !info;
        link.el.hidden = !w;
        noWifi.hidden = !!w;
        put(wifi.side, w ? w.interface : '');
        if (w) {
          link.set([w.rssi + ' dBm', w.noise + ' dBm', w.channel + ' · ' + w.band_ghz + ' GHz', w.width_mhz + ' MHz',
            Math.round(w.tx_rate_mbps) + ' Mbps', w.phy || DASH, w.security || DASH]);
        }
        route.set([info && info.router || DASH, info && info.dns.join(', ') || DASH]);

        apps.set(s.net ? s.net.apps : [], !!s.net && s.net.has_rates);
        var servers = s.net ? s.net.listening.filter(function (l) { return l.dir; }) : [];
        dev.el.hidden = !servers.length;
        put(dev.side, t('n.port', { n: servers.length }));
        sync(devList, servers, function (l) { return [l.pid, l.proto, l.addr, l.port].join('|'); }, function () {
          var row = h('div', 'row two link-row'), app = h('span', 'a'), dir = h('span', 'b'), port = h('b', 'r');
          row.$ = { app: app, dir: dir, port: port };
          row.onclick = function () { findApp(row.$.app.textContent); };
          return add(row, add(h('div', 'cell'), add(h('div'), app, dir)), port);
        }, function (row, l) {
          put(row.$.app, l.app);
          put(row.$.dir, l.dir);
          put(row.$.port, ':' + l.port);
          attr(row, 'title', l.app + ' (pid ' + l.pid + ') ' + l.proto + ' ' + l.addr + ':' + l.port + '\n' + l.dir);
        });
      } };
    },

    battery: function (L, R) {
      var el = h('section', 'tile hero t-bat'), donut = ring(['k1']), pill = h('span', 'pill');
      var left = h('b', 'lead', DASH), sub = h('div', 'sub');
      add(el, add(h('div', 'ring-row'), donut.el, add(h('div', 'ring-side'), pill, left, sub)));
      var draw = live(t('bat.draw'), [[t('power.system'), '']], watts);
      var info = facts([t('bat.health'), t('bat.cycles'), t('bat.design'), t('bat.full'), t('m.temperature'), t('bat.voltage'), t('bat.current'), t('bat.on_for')], 'two');
      var infoCard = card(t('details'));
      // Low Power Mode is shown, not switched: turning it on or off needs administrator rights.
      var lowPower = h('b', 'act-out'), toSettings = h('button', 'btn', t('bat.open_settings'));
      toSettings.onclick = function () { send({ type: 'open', target: 'battery' }); };
      add(infoCard.el, info.el, add(h('div', 'act'), h('span', '', t('bat.low_power_mode')), lowPower, toSettings), hint(t('bat.low_power_note')));
      var awake = card(t('sleep.title')), awakeList = h('div', 'list sleep');
      awake.el.firstChild.appendChild(hintOf(t('sleep.title'), SLEEP_HINT));
      awake.el.appendChild(awakeList);
      var apps = ranking(t('top_apps'), 'energy', [t('col.energy')], function (a) { return a.energy_mw; },
        function (a) { return [Math.round(a.energy_mw) + ' mW']; },
        t('bat.energy_note'));
      add(L, draw.el, infoCard.el);
      add(R, awake.el, apps.el);

      return { hero: el, metric: 'battery', metrics: [['battery', t('series.charge')], ['power', t('m.power')]], update: function (s) {
        var b = s.battery;
        if (!b) return;
        var remaining = b.time_remaining_s, low = b.state !== 'battery' ? '' : b.percent <= 10 ? 'crit' : b.percent <= 20 ? 'warn' : '';
        donut.set([b.percent / 100]);
        attr(donut.el, 'data-level', low);
        put(donut.label, b.percent + '%');
        setPill(pill, t('bat.' + b.state), low === 'crit' ? 'critical' : low === 'warn' ? 'warning' : '');
        put(left, b.state === 'plugged' ? t(b.percent === 100 ? 'bat.full_charge' : 'bat.not_charging')
          : remaining == null ? t('bat.estimating') : t(b.state === 'charging' ? 'bat.to_full' : 'bat.left', { t: dur(remaining) }));
        put(sub, b.adapter ? [b.adapter.name || t('bat.adapter'), b.adapter.watts ? b.adapter.watts + ' W' : ''].filter(Boolean).join(' · ') : t('bat.no_adapter'));
        attr(sub, 'title', sub.textContent);
        draw.set(s.spark.power, Math.max.apply(null, s.spark.power.concat(1)) * 1.15);
        info.set([b.health + '%', String(b.cycles), b.design_mah ? b.design_mah + ' mAh' : DASH, b.max_mah ? b.max_mah + ' mAh' : DASH,
          temp(b.temp_c), b.voltage_v ? fx(b.voltage_v, 2) + ' V' : DASH, fx(b.amperage_a, 2) + ' A',
          b.unplugged_at == null ? DASH : dur(Math.max(0, s.time - b.unplugged_at) / 1000)]);
        info.values[7].parentNode.hidden = b.state !== 'battery';
        put(lowPower, t(b.low_power ? 'state.on' : 'state.off'));

        awake.el.hidden = !s.sleep_blockers.length;
        sync(awakeList, s.sleep_blockers, function (k) { return k.pid + k.kind + k.name; }, function () {
          var row = h('div', 'row'), app = h('span', 'name'), why = h('span', 'name dim'), kind = h('span', 'badge');
          row.$ = { app: app, why: why, kind: kind };
          return add(row, app, why, kind);
        }, function (row, k) {
          put(row.$.app, k.app);
          put(row.$.why, k.name);
          attr(row, 'title', k.app + ' (' + k.pid + '): ' + k.name + ' — ' + k.kind);
          put(row.$.kind, t(/Display/.test(k.kind) ? 'sleep.display' : 'sleep.system'));
        });
        apps.set(s.apps.items, s.apps.has_rates);
      } };
    },

    sensors: function (L, R) {
      var hd = hero('t-cpu');
      var fans = card(t('sens.fans')), fanList = h('div', 'list'), noFans = hint(t('sens.no_fans'));
      add(fans.el, fanList, noFans);
      var power = card(t('m.power')), draw = facts([t('power.system'), t('power.adapter'), t('m.cpu'), t('m.gpu')], 'two');
      power.el.appendChild(draw.el);
      var temps = card(t('sens.temps')), groups = h('div', 'groups'), noTemps = hint(t('sens.no_temps'));
      add(temps.el, groups, noTemps);
      var blue = card('Bluetooth'), blueList = h('div', 'list'), noBlue = hint(t('sens.no_bt'));
      add(blue.el, blueList, noBlue);
      add(L, fans.el, power.el, blue.el);
      add(R, temps.el);
      var closed = new Set(), seeded = false, order = {}, shownAt = 0;

      return { hero: hd.el, metric: 'temp', shown: function () { shownAt = Date.now(); }, update: function (s) {
        var sn = s.sensors, chip = s.cpu.temp_c, core = hottest(s.cpu), hot = core == null ? chip : core;
        put(hd.num, hot == null ? DASH : String(deg(hot)));
        put(hd.unit, hot == null ? '' : settings.temp_unit === 'F' ? '°F' : '°C');
        setPill(hd.pill, t('thermal.pill', { state: t('thermal.' + sn.thermal) }), THERMAL[sn.thermal]);
        attr(hd.pill, 'title', t('thermal.tip'));
        put(hd.sub, (core == null ? t('cpu.temp') : t('cpu.hottest') + (chip == null ? '' : ' · ' + t('cpu.temp') + ' ' + temp(chip))) + (sn.temps.length ? ' · ' + t('n.sensor', { n: sn.temps.length }) : ''));

        put(fans.side, sn.fans.length ? fansText(sn.fans) : '');
        sync(fanList, sn.fans, function (f) { return sn.fans.indexOf(f); }, function (f) {
          var row = h('div', 'fan'), bar = meter(1), rpm = h('b'), range = h('span', 'dim');
          row.$ = { bar: bar.segs[0], rpm: rpm, range: range };
          return add(row, icon('fan'), h('span', '', t('sens.fan', { n: sn.fans.indexOf(f) + 1 })), bar.el, rpm, range);
        }, function (row, f) {
          fill(row.$.bar, f.max > f.min ? (f.rpm - f.min) / (f.max - f.min) * 100 : 0);
          put(row.$.rpm, f.rpm > 0 ? Math.round(f.rpm) + ' rpm' : t('off'));
          put(row.$.range, Math.round(f.min) + '–' + Math.round(f.max));
          attr(row, 'title', t('sens.fan_tip', { min: Math.round(f.min), max: Math.round(f.max) }));
        });
        noFans.hidden = sn.fans.length > 0;
        draw.set([watts(s.power.system_w), watts(s.power.adapter_w), watts(s.power.cpu_w), watts(s.power.gpu_w)]);

        var names = [], by = {};
        sn.temps.forEach(function (t) {
          if (!by[t.group]) { by[t.group] = []; names.push(t.group); }
          by[t.group].push(t);
        });
        // Hottest first, ranked once per group and again when it is toggled: re-sorting on every
        // sample made the rows jump. A long group (an M4 Pro has ~100 keys per CPU cluster) starts folded.
        names.forEach(function (g) {
          var rank = order[g];
          if (!rank) {
            rank = order[g] = {};
            by[g].slice().sort(function (a, b) { return b.temp_c - a.temp_c; }).forEach(function (t, i) { rank[t.key] = i; });
          }
          by[g].sort(function (a, b) { return (a.key in rank ? rank[a.key] : 1e9) - (b.key in rank ? rank[b.key] : 1e9); });
        });
        if (!seeded && names.length) {
          seeded = true;
          names.forEach(function (g) { if (by[g].length > 12) closed.add(g); });
        }
        put(temps.side, sn.temps.length ? t('sens.range_note') : '');
        noTemps.hidden = names.length > 0;
        // The sensor list comes with the first detail sample, a second or two after the screen opens:
        // a Mac that reports a CPU temperature is not said to have no sensors before that.
        put(noTemps, t(hot != null && Date.now() - shownAt < 10000 ? 'reading' : 'sens.no_temps'));
        sync(groups, names, function (g) { return g; }, function (g) {
          var box = h('div', 'group-t'), head = h('button', 'group-b'), name = h('span', 'name'), peak = h('b'), list = h('div', 'list');
          box.$ = { head: head, name: name, peak: peak, list: list };
          head.onclick = function () {
            if (!closed.delete(g)) closed.add(g);
            delete order[g];
            if (state) tabs[current].update(state);
          };
          return add(box, add(head, icon('chevron'), name, peak), list);
        }, function (box, g) {
          var rows = by[g], open = !closed.has(g);
          put(box.$.name, named('group.', g));
          put(box.$.peak, temp(Math.max.apply(null, rows.map(function (t) { return t.temp_c; }))));
          attr(box.$.head, 'title', t('sens.hottest', { n: rows.length }));
          attr(box.$.head, 'aria-expanded', String(open));
          box.$.list.hidden = !open;
          sync(box.$.list, open ? rows : [], function (t) { return t.key; }, function () {
            var row = h('div', 'sensor'), key = h('span', 'key-n'), bar = meter(1), now = h('b'), range = h('span', 'dim');
            row.$ = { key: key, bar: bar, now: now, range: range };
            return add(row, key, bar.el, now, range);
          }, function (row, t) {
            var e = extremes.get(t.key) || [t.temp_c, t.temp_c];
            put(row.$.key, t.key);
            // The mini bar spans 20–110 °C, the range the SMC sensors of a Mac actually move in.
            fill(row.$.bar.segs[0], (t.temp_c - 20) / 90 * 100);
            attr(row.$.bar.el, 'data-level', t.temp_c >= s.settings.alert_temp ? 'crit' : t.temp_c >= s.settings.alert_temp - 15 ? 'warn' : '');
            put(row.$.now, temp(t.temp_c));
            put(row.$.range, deg(e[0]) + '–' + deg(e[1]) + '°');
          });
        });

        put(blue.side, sn.bluetooth.length ? t('n.device', { n: sn.bluetooth.length }) : '');
        noBlue.hidden = sn.bluetooth.length > 0;
        sync(blueList, sn.bluetooth, function (d) { return d.name; }, function () {
          var row = h('div', 'row bt'), name = h('span', 'name'), kind = h('span', 'dim'), levels = h('span', 'levels');
          row.$ = { name: name, kind: kind, levels: levels };
          return add(row, add(h('span', 'bt-n'), name, kind), levels);
        }, function (row, d) {
          put(row.$.name, d.name);
          put(row.$.kind, d.kind);
          attr(row, 'title', d.name + (d.kind ? ' — ' + d.kind : ''));
          sync(row.$.levels, d.levels, function (l) { return l.part; }, function () {
            return h('span', 'pill');
          }, function (pill, l) {
            put(pill, (I18N.en['bt.' + l.part] ? t('bt.' + l.part) + ' ' : '') + l.percent + '%');
            attr(pill, 'data-level', l.percent <= 10 ? 'critical' : l.percent <= 20 ? 'warning' : '');
          });
        });
      } };
    }
  };

  // Dev: what a developer has running. Projects are the user's listening processes by working folder,
  // agents are AI coding sessions, containers come from the docker CLI. Read-only; Go fills
  // state.dev only while this tab is the visible one.
  function buildDev() {
    var el = h('div', 'tab scroll dev'), none = h('div', 'empty'), docker = h('div', 'foot');

    // text(item) is the two lines of a row; find(item), when given, is what the row searches Apps for.
    function section(title, keyOf, text, find) {
      var c = card(title), list = h('div', 'list');
      add(c.el.firstChild, h('span', 'th r', t('m.cpu')), h('span', 'th r', t('m.memory')));
      c.el.appendChild(list);
      el.appendChild(c.el);
      return function (items, now) {
        c.el.hidden = !items.length;
        sync(list, items, keyOf, function () {
          var row = h('div', 'row two'), a = h('span', 'a'), b = h('span', 'b'), cpu = h('span', 'r'), mem = h('span', 'r dim');
          row.$ = { a: a, b: b, cpu: cpu, mem: mem };
          row.classList.toggle('link-row', !!find);
          row.onclick = function () { if (find && find(row.$.item)) findApp(find(row.$.item)); };
          return add(row, add(h('div', 'cell'), add(h('div'), a, b)), cpu, mem);
        }, function (row, x) {
          var lines = text(x, now);
          row.$.item = x;
          put(row.$.a, lines[0]);
          put(row.$.b, lines[1]);
          attr(row, 'title', lines.join('\n'));
          put(row.$.cpu, x.cpu == null ? DASH : percent(x.cpu));
          put(row.$.mem, join(bytes(x.memory)));
        });
      };
    }

    function dots(parts) { return parts.filter(Boolean).join(' · '); }

    var projects = section(t('dev.projects'), function (x) { return x.dir; }, function (x) {
      return [dots([x.name, x.ports.map(function (port) { return ':' + port; }).join(' ')]),
        dots([x.dir, x.apps.join(', '), x.containers ? t('n.container', { n: x.containers }) : ''])];
    }, function (x) { return x.pids.length ? String(x.pids[0]) : ''; });
    var agents = section(t('dev.agents'), function (x) { return x.pid; }, function (x, now) {
      return [dots([x.name, t('n.process', { n: x.procs }), dur(Math.max(0, now - x.since) / 1000)]),
        dots([x.dir, x.energy_mw == null ? '' : Math.round(x.energy_mw) + ' mW'])];
    }, function (x) { return String(x.pid); });
    var containers = section(t('dev.containers'), function (x) { return x.id; }, function (x) {
      return [dots([x.name, x.project]), dots([x.image, x.status, x.ports])];
    });
    add(el, none, docker);

    return { el: el, update: function (s) {
      var d = s.dev || { docker: 'ok', projects: [], agents: [], containers: [] };
      projects(d.projects, s.time);
      agents(d.agents, s.time);
      containers(d.containers, s.time);
      none.hidden = d.projects.length + d.agents.length + d.containers.length > 0;
      put(none, s.dev ? t('dev.empty') : t('reading'));
      docker.hidden = d.docker === 'ok';
      put(docker, d.docker === 'ok' ? '' : t('dev.docker_' + d.docker));
    } };
  }

  // squarify lays values (largest first) out in a w × h box as rectangles close to squares
  // (Bruls, Huizing, van Wijk); it returns [x, y, width, height] for each value, in order.
  function squarify(values, w, h) {
    var total = values.reduce(function (a, b) { return a + b; }, 0), scale = w * h / total;
    var out = [], x = 0, y = 0, i = 0;
    while (i < values.length) {
      // A row along the short side grows while its worst aspect ratio gets better.
      var side = Math.min(w, h), row = [], sum = 0, worst = Infinity;
      while (i + row.length < values.length) {
        var v = values[i + row.length] * scale, s = sum + v, first = row.length ? row[0] : v;
        var ratio = Math.max(side * side * first / (s * s), s * s / (side * side * v));
        if (row.length && ratio > worst) break;
        row.push(v);
        sum = s;
        worst = ratio;
      }
      var thick = sum / side, at = 0;
      for (var k = 0; k < row.length; k++) {
        var len = row[k] / thick;
        out.push(w >= h ? [x, y + at, thick, len] : [x + at, y, len, thick]);
        at += len;
      }
      if (w >= h) { x += thick; w -= thick; } else { y += thick; h -= thick; }
      i += row.length;
    }
    return out;
  }

  // Storage: a map of what takes the space, and under it the cleanup card. Nothing is read until a
  // button asks for it; Go fills state.storage only for a view that shows this tab or the Disk screen.
  function buildStorage() {
    var el = h('div', 'tab scroll storage'), cleaner = cleanupCard();

    // The map. Go keeps the scanned tree; the page asks for one folder at a time (storage_open → onStorage).
    var map = card(t('stor.map')), status = h('div', 'hint'), intro = hint(t('stor.intro'));
    var home = h('button', 'btn primary', t('stor.scan_home')), choose = h('button', 'btn', t('stor.choose'));
    var cancel = h('button', 'btn sm', t('cancel')), clear = h('button', 'btn link sm', t('stor.clear'));
    var buttons = add(h('div', 'acts flat'), home, choose), scanning = progress(t('stor.scanning'));
    var scannedFiles = h('span'), scannedBytes = h('span');
    add(scanning.so, scannedFiles, scannedBytes);
    scanning.el.appendChild(cancel);
    var crumbs = h('nav', 'crumbs'), tree = h('div', 'treemap'), list = h('div', 'list'), none = hint(t('stor.empty'));
    var head = h('div', 'row thead'), level = null, path = '', root = '', asked = false, sortBy = 'bytes';
    var SORTS = {
      bytes: function (a, b) { return b.bytes - a.bytes; },
      files: function (a, b) { return b.files - a.files; },
      name: function (a, b) { return a.name.localeCompare(b.name, lang); }
    };
    var heads = [['name', 'stor.col_name'], ['files', 'stor.col_files'], ['bytes', 'stor.col_size']].map(function (col, i) {
      var b = h('button', i ? 'th r' : 'th', t(col[1]));
      b.prepend(icon('sort'));
      b.onclick = function () { sortBy = col[0]; draw(); };
      head.appendChild(b);
      return b;
    });
    heads[0].style.gridColumn = '1 / 3';
    crumbs.setAttribute('aria-label', t('stor.path'));
    map.side.replaceWith(clear);
    var found = add(h('div', 'found'), crumbs, tree, head, list, none);
    add(map.el, intro, buttons, scanning.el, status, found);
    el.appendChild(map.el);
    home.onclick = function () { send({ type: 'storage_scan', root: 'home' }); };
    choose.onclick = function () { send({ type: 'storage_scan', root: 'choose' }); };
    cancel.onclick = function () { send({ type: 'storage_cancel' }); };
    clear.onclick = function () { send({ type: 'storage_clear' }); };

    function open(to) {
      path = to;
      send({ type: 'storage_open', path: to });
    }

    function child(name) { return path ? path + '/' + name : name; }

    // A folder the scan went into opens; a file, a refused folder and the row of everything small do not.
    function enters(c) { return c.dir && !c.denied && c.name !== ''; }

    function label(c) { return c.name || t('stor.rest'); }

    // Where macOS settles a refused folder: Desktop, Documents and Downloads of the home folder are
    // asked for one by one, ~/Library and the Trash open with Full Disk Access. Any other refusal
    // is the folder's own permissions, which no pane of System Settings changes.
    function pane(name) {
      if (root !== '~') return '';
      var top = child(name).split('/')[0], direct = !path;
      if (direct && /^(Desktop|Documents|Downloads)$/.test(top)) return 'privacy_files';
      return top === 'Library' || top === '.Trash' ? 'privacy_full_disk' : '';
    }

    function draw() {
      found.hidden = !level;
      if (!level) return;
      var parts = level.path ? level.path.split('/') : [];
      crumbs.textContent = '';
      [root].concat(parts).forEach(function (name, i) {
        if (i) crumbs.appendChild(icon('chevron'));
        if (i === parts.length) { crumbs.appendChild(h('b', 'name', name)); return; }
        var b = h('button', 'btn link sm', name);
        b.onclick = function () { open(parts.slice(0, i).join('/')); };
        crumbs.appendChild(b);
      });
      crumbs.appendChild(h('span', 'dim', join(bytes(level.bytes, 1000))));

      // The thirty largest entries are drawn; whatever else the folder holds is one last cell.
      var big = level.children.filter(function (c) { return c.bytes > 0 && c.name; }).sort(SORTS.bytes).slice(0, 30);
      var rest = level.bytes - big.reduce(function (sum, c) { return sum + c.bytes; }, 0);
      if (rest > level.bytes / 200) big.push({ name: '', bytes: rest, dir: false });
      var W = tree.clientWidth || 370, H = tree.clientHeight || 150;
      var boxes = squarify(big.map(function (c) { return c.bytes; }), W, H);
      tree.textContent = '';
      tree.hidden = !big.length;
      big.forEach(function (c, i) {
        var r = boxes[i], cell = h('button', c.name ? c.dir ? '' : 'file' : 'rest');
        cell.style.cssText = 'left:' + (r[0] / W * 100) + '%;top:' + (r[1] / H * 100) + '%;width:' + (r[2] / W * 100) + '%;height:' + (r[3] / H * 100) + '%';
        cell.title = label(c) + ' — ' + join(bytes(c.bytes, 1000));
        cell.tabIndex = -1;
        if (r[2] > 44 && r[3] > 18) add(cell, h('span', 'name', label(c)), r[3] > 34 && h('span', 'dim', join(bytes(c.bytes, 1000))));
        if (enters(c)) cell.onclick = function () { open(child(c.name)); };
        else cell.disabled = true;
        tree.appendChild(cell);
      });

      heads.forEach(function (b, i) {
        var on = ['name', 'files', 'bytes'][i] === sortBy;
        b.firstChild.style.visibility = on ? '' : 'hidden';
        attr(b, 'aria-pressed', String(on));
      });
      // The row of everything too small to list stays last under every sort.
      var rows = level.children.filter(function (c) { return c.name; }).sort(SORTS[sortBy])
        .concat(level.children.filter(function (c) { return !c.name; }));
      none.hidden = rows.length > 0;
      head.hidden = !rows.length;
      list.textContent = '';
      rows.forEach(function (c) {
        var row = h('div', 'row'), name = h('span', 'name', label(c)), files = h('span', 'r dim'), size = h('span', 'r');
        var reveal = h('button', 'kill');
        reveal.appendChild(icon('search'));
        reveal.title = t('act.reveal');
        reveal.setAttribute('aria-label', t('act.reveal') + ': ' + c.name);
        // Go takes a path only if the scanned tree has it, and builds the real one itself.
        reveal.onclick = function (e) {
          e.stopPropagation();
          send({ type: 'reveal', path: child(c.name) });
        };
        var tag = c.denied ? h('span', 'badge', t('stor.no_access')) : null, clean = null;
        if (c.category) {
          clean = h('button', 'btn link sm', t('stor.cleanup'));
          clean.onclick = function (e) {
            e.stopPropagation();
            cleaner.el.scrollIntoView({ block: 'start' });
          };
        }
        name.title = c.name;
        add(row, c.name ? icon(c.dir ? 'folder' : 'file', 'app-i') : h('span'), add(h('span', 'app-n'), name, tag, clean));
        if (c.denied) {
          row.classList.add('denied');
          add(row, add(h('span', 'r'), pane(c.name) && allow(pane(c.name))));
        } else {
          put(files, c.dir || !c.name ? count(c.files) : '');
          put(size, join(bytes(c.bytes, 1000)));
          add(row, files, size);
        }
        if (c.name) row.appendChild(reveal);
        row.classList.toggle('dim', !c.name);
        if (enters(c)) {
          row.classList.add('link-row');
          row.onclick = function () { open(child(c.name)); };
        }
        list.appendChild(row);
      });
    }

    function update(s) {
      // The state of another tab carries no storage: what is on screen stays until this tab's own state comes.
      if (!s.storage) return;
      var scan = s.storage.scan, running = scan.state === 'running', done = scan.state === 'done';
      root = scan.root;
      intro.hidden = buttons.hidden = running || done;
      scanning.el.hidden = !running;
      clear.hidden = !done;
      var files = t('stor.scanned', { n: scan.files, files: count(scan.files) }), unread = scan.denied ? t('stor.denied_n', { n: scan.denied }) : '';
      put(scannedFiles, files);
      put(scannedBytes, join(bytes(scan.bytes, 1000)));
      status.hidden = scan.state === 'idle';
      // Under the running counters: the folder being read. Once done, the crumbs above the map name it and its size.
      put(status, running ? [scan.root, unread].filter(Boolean).join(' · ') : done ? [files, unread].filter(Boolean).join(' · ') : status.hidden ? '' : t('stor.' + scan.state));
      // The page asks for the folder it shows once the scan is done; a new scan starts from its root again.
      if (!done) { level = null; path = ''; draw(); }
      else if (!asked) open(path);
      asked = done;
      cleaner.update(s);
    }

    return {
      el: el,
      update: update,
      resize: draw,
      shown: function () { el.appendChild(cleaner.el); },
      onStorage: function (got) {
        level = got;
        path = got.path;
        draw();
      }
    };
  }

  // Where macOS settles a refused folder: a link to the pane of System Settings that gives the access.
  function allow(target) {
    var b = h('button', 'btn link sm', t('stor.allow'));
    b.title = t('stor.allow_tip');
    b.onclick = function (e) {
      e.stopPropagation();
      send({ type: 'open', target: target });
    };
    return b;
  }

  // The cleanup of well-known caches is one card for the Storage tab and the Disk screen: the screen
  // that is shown takes it in, so a measurement and an open review are the same in both places.
  var cleanup = null;
  function cleanupCard() {
    if (!cleanup) cleanup = buildCleanup();
    return cleanup;
  }

  // Nothing is listed until a measurement found something to remove. Show opens the review of one
  // category: the entries a removal would take, each one the user may keep. Its button is the
  // confirmation, and Go removes only what the review showed and left checked.
  function buildCleanup() {
    var own = card(t('stor.cleanup'), 'cleanup'), measure = h('button', 'btn', t('stor.measure')), cats = h('div', 'cats');
    var COST = { app_caches: 'cost.app_caches', logs: 'cost.logs', xcode_derived: 'cost.rebuild', simulator_caches: 'cost.simulator',
      npm: 'cost.redownload', yarn: 'cost.redownload', pnpm: 'cost.redownload', go_build: 'cost.rebuild', pip: 'cost.redownload',
      homebrew: 'cost.redownload', trash: 'cost.trash' };
    var measuring = progress(t('clean.measuring')), note = h('div', 'hint note'), quiet = hint(''), denied = hint(''), deniedText = h('span');
    var measureRow = add(h('div', 'acts flat'), measure);
    // listing is the id of the category whose entries were asked for; review the open one; busy the removal that runs.
    var listing = '', listTimer = 0, review = null, busy = null, busyTimer = 0;
    add(denied, deniedText, allow('privacy_full_disk'));
    add(own.el, hint(t('stor.cleanup_intro')), measureRow, measuring.el, note, cats, quiet, denied);
    measure.onclick = function () {
      say('');
      closeReview();
      send({ type: 'cleanup_scan' });
    };

    function say(text, error) {
      put(note, text);
      note.hidden = !text;
      note.classList.toggle('error', !!error);
    }

    function closeReview() {
      if (!review) return false;
      review.box.remove();
      review = null;
      if (state) update(state);
      return true;
    }

    // Esc takes back the question about deleting for good first, then the review.
    function cancelStep() {
      if (!review || !review.ask) return closeReview();
      review.ask.remove();
      review.ask = null;
      review.foot.hidden = false;
      put(review.heading, t('clean.review'));
      return true;
    }

    // got is Go's listing of the category: the largest entries by name, the rest as one row.
    function openReview(row, c, got) {
      closeReview();
      var items = got.items.map(function (it) { return { name: it.name, bytes: it.bytes, on: true }; });
      if (got.rest) items.push({ name: '', count: got.rest, bytes: got.rest_bytes, on: true });
      // The heading says where the entries go: to the Trash, or for good.
      var box = h('div', 'review'), list = h('div', 'items'), sum = h('div', 'sum');
      var heading = h('div', 'review-h', t(c.permanent ? 'clean.review_permanent' : 'clean.review'));
      var no = h('button', 'btn sm', t('clean.cancel')), yes = h('button', 'btn sm danger'), foot = add(h('div', 'btns'), no, yes);
      var all = items.reduce(function (n, it) { return n + it.bytes; }, 0);
      list.setAttribute('role', 'group');
      list.setAttribute('aria-label', heading.textContent);
      function selected() { return items.reduce(function (n, it) { return n + (it.on ? it.bytes : 0); }, 0); }
      function total() {
        var size = join(bytes(selected(), 1000));
        put(sum, t('clean.selected', { size: size, total: join(bytes(all, 1000)) }));
        put(yes, t(c.permanent ? 'clean.delete_size' : 'clean.move_size', { size: size }));
        yes.disabled = !!busy || !items.some(function (it) { return it.on; });
      }
      // One box above the list checks or clears every row; it shows a dash while the rows differ.
      var every = h('input', 'check'), checks = [];
      every.type = 'checkbox';
      every.checked = true;
      every.onchange = function () {
        items.forEach(function (it, i) { it.on = checks[i].checked = every.checked; });
        total();
      };
      function mirror() {
        var on = items.filter(function (it) { return it.on; }).length;
        every.checked = on === items.length;
        every.indeterminate = on > 0 && on < items.length;
      }
      if (items.length > 1) list.appendChild(add(h('label', 'item every'), every, h('span', 'name', t('clean.all'))));
      items.forEach(function (it) {
        var line = h('label', 'item'), check = h('input', 'check'), name = h('span', 'name', it.name || t('clean.rest', { n: it.count }));
        check.type = 'checkbox';
        check.checked = true;
        checks.push(check);
        check.onchange = function () { it.on = check.checked; mirror(); total(); };
        name.title = it.name;
        line.classList.toggle('rest', !it.name);
        list.appendChild(add(line, check, name, h('span', 'r', join(bytes(it.bytes, 1000)))));
      });
      function remove(permanent) {
        busy = { id: c.id, permanent: permanent, name: t('clean.' + c.id) };
        // Go answers when the removal is over; if no answer comes the buttons must not stay dead.
        clearTimeout(busyTimer);
        busyTimer = setTimeout(function () { busy = null; if (state) update(state); }, 120000);
        // review names the listing these rows came from: Go refuses a removal checked against another one.
        send({ type: 'cleanup', category: c.id, review: got.id, permanent: permanent,
          items: items.filter(function (it) { return it.on && it.name; }).map(function (it) { return it.name; }),
          rest: items.some(function (it) { return it.on && !it.name; }) });
        if (state) update(state);
      }
      no.onclick = closeReview;
      yes.onclick = function () { remove(c.permanent); };
      add(box, heading, list, sum, hint(t(COST[c.id])), foot);
      row.appendChild(box);
      review = { id: c.id, box: box, foot: foot, yes: yes, total: total, heading: heading, ask: null,
        // The Trash did not take the entries and nothing was touched: deleting them for good is a second, separate question.
        askPermanent: function (lead) {
          var back = h('button', 'btn sm', t('clean.cancel')), go = h('button', 'btn sm danger', t('clean.delete'));
          var words = { size: join(bytes(selected(), 1000)), name: t('clean.' + c.id) };
          review.ask = add(h('div', 'confirm'), h('span', 'q error', lead), h('span', 'q', t('clean.ask_permanent', words)), add(h('span', 'btns'), back, go));
          foot.hidden = true;
          put(heading, t('clean.review_permanent'));
          box.appendChild(review.ask);
          back.onclick = cancelStep;
          go.onclick = function () {
            cancelStep();
            put(heading, t('clean.review_permanent'));
            remove(true);
          };
          // What cannot be undone is never one Return away.
          back.focus();
        } };
      total();
      (c.permanent ? no : yes).focus();
    }

    function update(s) {
      // The state of another tab carries no storage: what is on screen stays until a state with it comes.
      if (!s.storage) return;
      var clean = s.storage.cleanup, measured = clean.state === 'done';
      measureRow.hidden = clean.state === 'running';
      measuring.el.hidden = !measureRow.hidden;
      measure.disabled = !!busy || !!listing;
      // Only what a removal would take something from is listed, the largest first.
      var full = measured ? clean.categories.filter(function (c) { return !c.denied && c.items > 0 && c.bytes > 0; }) : [];
      var locked = measured ? clean.categories.filter(function (c) { return c.denied; }) : [];
      full.sort(function (a, b) { return b.bytes - a.bytes; });
      sync(cats, full, function (c) { return c.id; }, function () {
        var row = h('div', 'cat'), name = h('span', 'name'), size = h('span', 'r'), go = h('button', 'btn sm', t('clean.show'));
        row.$ = { name: name, size: size, go: go };
        go.onclick = function () {
          listing = row.$.c.id;
          say('');
          // Go lists a category within seconds; with no answer at all the buttons must not stay disabled.
          clearTimeout(listTimer);
          listTimer = setTimeout(function () { cleanup.onListed({ ok: false, text: t('act.no_answer') }); }, 60000);
          send({ type: 'cleanup_list', category: listing });
          if (state) update(state);
        };
        return add(row, add(h('div', 'cat-h'), name, size, go));
      }, function (row, c) {
        row.$.c = c;
        row.dataset.id = c.id;
        put(row.$.name, t('clean.' + c.id));
        put(row.$.size, join(bytes(c.bytes, 1000)));
        row.classList.toggle('open', !!review && review.id === c.id);
        row.$.go.hidden = row.classList.contains('open');
        row.$.go.disabled = !!busy || !!listing;
        row.$.go.classList.toggle('busy', listing === c.id);
      });
      // A category that a re-measurement found empty takes its review with it.
      if (review && !review.box.isConnected) review = null;
      if (review) {
        review.total();
        review.yes.classList.toggle('busy', !!busy);
        review.box.classList.toggle('busy', !!busy);
      }
      var empty = measured ? clean.categories.length - full.length - locked.length : 0;
      quiet.hidden = !measured || full.length > 0 && !empty;
      put(quiet, full.length ? t('clean.hidden', { n: empty }) : t('clean.nothing'));
      denied.hidden = !locked.length;
      put(deniedText, t('clean.denied', { names: locked.map(function (c) { return t('clean.' + c.id); }).join(', ') }));
    }

    say('');
    return {
      el: own.el,
      update: update,
      cancel: cancelStep,
      say: say,
      // The answer to cleanup_list; false when no row of this page asked for it.
      onListed: function (r) {
        var id = listing, row = cats.querySelector('[data-id="' + id + '"]');
        if (!id) return false;
        listing = '';
        clearTimeout(listTimer);
        if (!r.ok) say(r.text, true);
        // Nothing left to list: the state that came with the answer has already dropped the row.
        else if (row && r.review.items.length) openReview(row, row.$.c, r.review);
        if (state) update(state);
        return true;
      },
      // The answer to cleanup; false when no review of this page asked for it.
      onCleaned: function (r) {
        var job = busy;
        if (!job) return false;
        busy = null;
        clearTimeout(busyTimer);
        if (r.key === 'err.no_trash' && !job.permanent && review) {
          review.askPermanent(r.text);
        } else if (r.key === 'err.busy') {
          // Nothing was started: the review stays as it is for another try.
          say(r.text, true);
        } else if (r.ok) {
          // A move to the Trash names the folder made there and the folder to drag its contents back to.
          var words = { size: join(bytes(r.values.bytes, 1000)), folder: r.list && r.list[0], from: r.list && r.list[1] };
          say(job.name + ': ' + [r.values.items && t(job.permanent ? 'clean.done_permanent' : 'clean.done', words),
            r.values.failed && t('clean.failed', { n: r.values.failed })].filter(Boolean).join(' · '));
          closeReview();
        } else {
          // err.stale: the folder is not what the review listed any more, so the review goes and Show lists it again.
          say(job.name + ': ' + (r.key === 'err.stale' ? t('clean.changed') : r.text), true);
          closeReview();
        }
        if (state) update(state);
        return true;
      }
    };
  }

  // The screen the clock's status item opens: the local time, the world clocks of
  // settings.clock_zones and a month calendar. No events: nothing here asks macOS for anything.
  function buildClock() {
    var el = h('div', 'tab scroll detail d-clock'), now = hero('t-clock');
    var world = card(t('clock.world')), zones = h('div', 'list'), none = hint(t('clock.none'));
    var input = h('input', 'field'), picks = h('div', 'zone-list scroll'), plus = h('button', 'btn', t('set.add'));
    // Every IANA zone the web view knows; Go checks the name again before it saves it.
    var ZONES = Intl.supportedValuesOf ? Intl.supportedValuesOf('timeZone') : [];
    function city(z) { return z.split('/').pop().replace(/_/g, ' '); }
    input.placeholder = t('clock.zone');
    input.setAttribute('aria-label', t('clock.zone'));
    input.spellcheck = false;
    var problem = hint('');
    // The zone the field names: its exact id, or the one zone whose city reads as the text ("moscow").
    function typed() {
      var z = input.value.trim();
      if (!ZONES.length || ZONES.indexOf(z) >= 0) return z;
      var hits = ZONES.filter(function (id) { return city(id).toLowerCase() === z.toLowerCase(); });
      return hits.length === 1 ? hits[0] : '';
    }
    function draft() {
      var z = typed(), have = settings.clock_zones, full = have.length >= 8;
      put(problem, full ? t('clock.max') : input.value.trim() && !z ? t('clock.no_zone') : '');
      problem.hidden = !problem.textContent;
      return z && have.indexOf(z) < 0 && !full ? z : '';
    }
    // The zones to pick from: all of them while the field is empty, then those whose name has the typed text.
    function suggest() {
      var text = input.value.trim().toLowerCase().replace(/ /g, '_'), have = settings.clock_zones;
      var hits = have.length >= 8 ? [] : ZONES.filter(function (z) { return have.indexOf(z) < 0 && z.toLowerCase().indexOf(text) >= 0; });
      picks.textContent = '';
      picks.hidden = !hits.length;
      hits.forEach(function (z) {
        var b = h('button', 'row');
        b.onclick = function () { choose(z); };
        picks.appendChild(add(b, h('span', 'name', city(z)), h('span', 'dim', z)));
      });
    }
    function choose(z) {
      set('clock_zones', settings.clock_zones.concat(z));
      input.value = '';
      picks.hidden = true;
    }
    var form = add(h('div', 'rule-form'), input, plus);
    input.onfocus = suggest;
    input.oninput = function () { plus.disabled = !draft(); suggest(); };
    input.onkeydown = function (e) { if (e.key === 'Enter') plus.click(); };
    // A press on the list must not take the focus from the field: the list would close under the pointer.
    picks.onmousedown = function (e) { e.preventDefault(); };
    world.el.addEventListener('focusout', function (e) { if (!world.el.contains(e.relatedTarget)) picks.hidden = true; });
    picks.hidden = true;
    plus.onclick = function () { if (draft()) choose(draft()); };
    add(world.el, zones, none, form, picks, problem);

    var month = h('span'), today = h('button', 'btn link sm', t('today')), prev = h('button', 'mini prev'), next = h('button', 'mini');
    var days = h('div', 'cal'), cal = add(h('section', 'tile card'), add(h('div', 'card-h'), month, add(h('span', 'cal-nav'), today, prev, next)), days);
    var shown = null, drawn = '';
    prev.appendChild(icon('chevron'));
    next.appendChild(icon('chevron'));
    prev.setAttribute('aria-label', t('clock.prev'));
    next.setAttribute('aria-label', t('clock.next'));
    function turn(step) {
      return function () {
        var from = shown || new Date();
        shown = step ? new Date(from.getFullYear(), from.getMonth() + step, 1) : null;
        update();
      };
    }
    prev.onclick = turn(-1);
    next.onclick = turn(1);
    today.onclick = turn(0);
    add(el, now.el, cal, world.el);

    function time(at, zone) {
      var cycle = { 12: 'h12', 24: 'h23' }[settings.clock_hours], key = 'time/' + cycle + '/' + zone;
      if (!formats[key]) {
        var o = { hour: 'numeric', minute: '2-digit', hourCycle: cycle, timeZone: zone };
        // "9:05 PM" but "09:05": a 24-hour clock keeps its leading zero, as the menu bar item does.
        if (/h2/.test(new Intl.DateTimeFormat(lang, o).resolvedOptions().hourCycle)) o.hour = '2-digit';
        formats[key] = new Intl.DateTimeFormat(lang, o);
      }
      return formats[key].format(at);
    }

    function drawMonth(at) {
      var first = shown || new Date(at.getFullYear(), at.getMonth(), 1), y = first.getFullYear(), m = first.getMonth();
      var key = [lang, y, m, at.toDateString()].join();
      today.hidden = !shown;
      if (key === drawn) return;
      drawn = key;
      // The week starts where the language's region starts it. Older WebKit has the weekInfo
      // property, newer the getWeekInfo() method; without either, Monday.
      var locale = new Intl.Locale(lang), week = (locale.getWeekInfo ? locale.getWeekInfo() : locale.weekInfo) || { firstDay: 1, weekend: [6, 7] };
      put(month, dateFormat('month', { month: 'long', year: 'numeric' }).format(first));
      days.textContent = '';
      for (var i = 0; i < 7; i++) {
        // 1 January 2024 was a Monday.
        days.appendChild(h('span', 'th', dateFormat('weekday', { weekday: 'short' }).format(new Date(2024, 0, week.firstDay + i))));
      }
      // getDay() counts from Sunday; the week info counts Monday as 1 and Sunday as 7.
      var lead = ((first.getDay() || 7) - week.firstDay + 7) % 7, count = new Date(y, m + 1, 0).getDate();
      for (var d = 1; d <= count; d++) {
        var date = new Date(y, m, d), cell = h('span', '', String(d));
        if (d === 1) cell.style.gridColumnStart = lead + 1;
        if (week.weekend.indexOf(date.getDay() || 7) >= 0) cell.className = 'dim';
        if (date.toDateString() === at.toDateString()) cell.className = 'today';
        days.appendChild(cell);
      }
    }

    // Redrawn with each state push, so the minute turns up to 2 s late; a timer of its own if that shows.
    function update() {
      var at = new Date(), list = settings.clock_zones;
      put(now.num, time(at));
      put(now.sub, dateFormat('date_long', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }).format(at));
      drawMonth(at);
      none.hidden = list.length > 0;
      sync(zones, list, function (z) { return z; }, function (z) {
        var row = h('div', 'row two'), a = h('span', 'a', city(z)), b = h('span', 'b'), clock = h('b', 'r'), x = h('button', 'mini');
        x.appendChild(icon('close'));
        x.setAttribute('aria-label', t('set.remove', { name: z }));
        x.onclick = function () { set('clock_zones', settings.clock_zones.filter(function (k) { return k !== z; })); };
        row.$ = { b: b, clock: clock };
        return add(row, add(h('div', 'cell'), add(h('div'), a, b)), clock, x);
      }, function (row, z) {
        attr(row, 'title', z);
        // A zone the settings hold but this web view cannot format keeps its row, so that it can be removed.
        try {
          put(row.$.clock, time(at, z));
          // The weekday there and the offset from UTC, as the language writes them: "Sat · GMT+9".
          put(row.$.b, dateFormat('zone/' + z, { weekday: 'short', timeZoneName: 'shortOffset', timeZone: z }).format(at).replace(', ', ' · '));
        } catch (e) {
          put(row.$.clock, DASH);
          put(row.$.b, '');
        }
      });
      plus.disabled = !draft();
    }

    return { el: el, update: update };
  }

  function buildDetail(name) {
    // The actions sit in a footer outside the scroller: Save image captures what is on screen.
    var el = h('div', 'tab detail d-' + name), body = h('div', 'scroll body');
    var left = h('div', 'col'), right = h('div', 'col');
    var screen = SCREENS[name](left, right);
    var chart = historyChart(screen.metric, '1h');
    var head = add(h('div', 'card-h'), h('span', '', t('tab.history')));
    if (screen.metrics) {
      var which = segmented(screen.metrics, function (v) { which.set(v); chart.setMetric(v); }, t('hist.metric'));
      which.set(screen.metric);
      head.appendChild(which.el);
    }
    chart.el.prepend(head);
    right.appendChild(chart.el);
    var out = h('span', 'act-out');
    var outs = (screen.outs || []).concat(out);
    add(el, add(body, screen.hero, add(h('div', 'pair'), left, right)),
      add(h('div', 'acts'), action('copy', t('act.copy'), out), action('export', t('act.export'), out), out));
    return {
      el: el,
      scroller: body,
      update: function (s) { screen.update(s); chart.tick(); },
      shown: function () {
        outs.forEach(function (o) { o.classList.remove('error'); put(o, ''); });
        chart.ask(true);
        if (screen.shown) screen.shown();
      },
      resize: chart.resize,
      onHistory: chart.onHistory
    };
  }

  // What each kind of alert measures: the name of the metric, the detail screen that shows it live,
  // the setting that holds its limit (a kind without one has a fixed rule) and the colour of its chart.
  var ALERT_KINDS = {
    cpu: { name: 'm.cpu', screen: 'cpu', limit: 'alert_cpu', color: 'cpu' },
    temp: { name: 'cpu.temp', screen: 'sensors', limit: 'alert_temp', color: 'temp' },
    thermal: { name: 'cpu.temp', screen: 'sensors', color: 'temp' },
    disk: { name: 'alertd.disk_free', screen: 'disk', limit: 'alert_disk_free', color: 'disk' },
    memory: { name: 'series.used', screen: 'memory', color: 'memory' },
    memory_used: { name: 'series.used', screen: 'memory', limit: 'alert_memory', color: 'memory' },
    swap: { name: 'mem.swap', screen: 'memory', limit: 'alert_swap', color: 'memory' },
    battery_low: { name: 'series.charge', screen: 'battery', limit: 'alert_battery', color: 'battery' },
    bt_battery: { name: 'series.charge', screen: 'sensors', color: 'battery' },
    app_cpu: { name: 'm.cpu', color: 'cpu' },
    app_memory: { name: 'm.memory', color: 'memory' },
    app_rule: { name: 'm.cpu', color: 'cpu' }
  };
  // The readings captured when an alert fired → dictionary key of their label.
  var ALERT_FACTS = {
    cpu_user: 'cpu.user', cpu_system: 'cpu.system', thermal: 'alertd.thermal', cpu_temp: 'cpu.temp',
    mem_app: 'mem.app', mem_wired: 'mem.wired', mem_compressed: 'mem.compressed', mem_cached: 'mem.cached', mem_free: 'mem.free', mem_swap: 'mem.swap',
    disk_free: 'mem.free', disk_total: 'col.total', disk_write: 'disk.writing',
    bat_level: 'series.charge', bat_draw: 'bat.draw', bat_left: 'alertd.time_left',
    bt_main: 'series.charge', bt_left: 'bt.left', bt_right: 'bt.right', bt_case: 'bt.case'
  };
  var TOP_BY = { percent: 'm.cpu', bytes: 'm.memory', bytes_per_s: 'disk.writing', milliwatts: 'col.energy' };

  // One alert: what happened, why its rule fired, the metric around that time, what the Mac was doing and
  // what can be done. The lists of the state only name the alert; the rest is asked for here.
  function buildAlert() {
    var el = h('div', 'tab detail d-alert'), body = h('div', 'scroll body');
    var note = h('div', 'tile a-note');
    var mark = h('span', 'a-mark'), name = h('b', 'a-title'), pill = h('span', 'pill'), why = h('p', 'a-why'), rule = h('div', 'hint');
    var head = add(h('section', 'tile hero'), add(h('div', 'a-top'), mark, name, pill), why, rule);
    var what = card(t('alertd.what')), whatList = h('div', 'facts');
    var nums = card(t('alertd.numbers')), numList = h('div', 'facts two');
    var chart = historyChart('', ''), chartTitle = h('span');
    chart.el.prepend(add(h('div', 'card-h'), chartTitle));
    var ctx = card(t('alertd.context')), ctxList = h('div', 'facts');
    var topHead = h('div', 'card-h sub-h'), topList = h('div', 'list'), procHead = h('div', 'card-h sub-h'), procList = h('div', 'list');
    var todo = card(t('alertd.do')), acts = h('div', 'a-acts');
    var noDetail = hint(t('alertd.none'));
    add(what.el, whatList, noDetail);
    nums.el.appendChild(numList);
    add(ctx.el, ctxList, topHead, topList, procHead, procList);
    todo.el.appendChild(acts);
    add(el, add(body, note, head,
      add(h('div', 'pair'), add(h('div', 'col'), what.el, nums.el), add(h('div', 'col'), chart.el)),
      add(h('div', 'pair'), add(h('div', 'col'), ctx.el), add(h('div', 'col'), todo.el))));

    // id is the alert on screen, got its detail, or 'gone' once Go has answered that it keeps no such
    // alert or has not answered three requests; a gone alert is not asked for again.
    var id = '', got = null, asked = 0, tries = 0;

    function ask() {
      asked = Date.now();
      tries++;
      send({ type: 'alert_detail', id: id });
    }

    // Without an id in the route the newest alert is shown: the one a notification was just about.
    function pick() {
      var a = state.alerts.active, r = state.alerts.recent;
      id = alertId || (a.length ? a[a.length - 1].id : r.length ? r[0].id : '');
      if (got && got.id !== id) got = null;
      if (id && !got) ask();
    }

    function pairs(list, items) {
      sync(list, items, function (p) { return p[0]; }, function (p) {
        var name = h('span', '', p[0]);
        name.title = p[0];
        return add(h('div'), name, h('b'));
      }, function (row, p) {
        put(row.lastChild, p[1]);
        attr(row.lastChild, 'title', p[1]);
      });
    }

    function rows(list, items, unit) {
      sync(list, items, function (x) { return x.name + x.pid; }, function (x) {
        var row = h('div', 'row'), value = h('span', 'r');
        row.$ = value;
        return add(row, x.pid ? h('span', 'dim pid', String(x.pid)) : appIcon(x.icon), h('span', 'name', x.name), value);
      }, function (row, x) { put(row.$, unit === 'percent' ? percent(x.value) : measure(unit, x.value)); });
    }

    function button(key, label, onClick) { return { key: key, label: label, go: onClick }; }

    // What can be done about the alert, by its kind: nothing here does more than the screens it leads to.
    function actions(a, kind) {
      var app = alertApp(a), list = [];
      if (app) {
        var running = state.apps.items.filter(function (x) { return x.name === app; })[0];
        list.push(button('apps', t('alertd.show_app'), function () { findApp(app); }));
        if (running && running.killable && !running.system) {
          list.push(button('quit', t('quit.more'), function () {
            show('processes');
            tabs.processes.askTop(app);
          }));
        }
        if (state.settings.alert_muted.indexOf(app) < 0) {
          list.push(button('mute', t('alertd.mute'), function () { set('alert_muted', state.settings.alert_muted.concat(app)); }));
        }
        list.push(button('rule', t(a.kind === 'app_rule' ? 'alertd.to_rule' : 'alertd.add_rule'), function () {
          openTab('settings:alerts');
          tabs.settings.goTo('');
        }));
      }
      if (kind.screen && !(kind.screen === 'battery' && !state.battery)) {
        list.push(button('screen', t('details_of', { name: t('m.' + kind.screen) }), function () { show('detail:' + kind.screen); }));
      }
      if (kind.limit) {
        list.push(button('limit', t('alertd.to_limit'), function () {
          openTab('settings:alerts');
          tabs.settings.goTo(kind.limit);
        }));
      }
      sync(acts, list, function (b) { return b.key; }, function () { return h('button', 'btn'); }, function (btn, b) {
        put(btn, b.label);
        btn.onclick = b.go;
      });
    }

    function render() {
      var a = got && got !== 'gone' ? got : null;
      note.hidden = !!a;
      put(note, !id || got === 'gone' ? t('alertd.gone') : t('reading'));
      head.hidden = what.el.hidden = todo.el.hidden = !a;
      nums.el.hidden = chart.el.hidden = ctx.el.hidden = !a || !a.recorded;
      if (!a) return;
      var kind = ALERT_KINDS[a.kind] || {}, memory = a.kind === 'app_rule' && a.params.metric === 'memory';
      var metric = t(memory ? 'm.memory' : kind.name || 'hist.metric'), text = alertText(a), app = alertApp(a);
      var active = !a.until, end = a.until || state.time;
      if (mark.$id !== a.id) {
        mark.$id = a.id;
        mark.textContent = '';
        mark.appendChild(app ? appIcon(a.icon) : icon('alert'));
      }
      put(name, text[0]);
      setPill(pill, t(active ? 'alertd.active' : 'alertd.ended'), active ? 'warning' : '');
      noDetail.hidden = a.recorded;
      why.hidden = rule.hidden = !a.recorded;
      pairs(whatList, [
        [a.kind === 'bt_battery' ? t('alertd.device') : app ? t('col.app') : t('hist.metric'), a.app || metric],
        [t('alertd.since'), dateTime(a.since)],
        a.recorded && [t('alertd.fired'), dateTime(a.fired_at)],
        [t('alertd.until'), active ? t('alertd.not_yet') : dateTime(a.until)],
        [t('alertd.lasted'), dur(Math.max(0, end - a.since) / 1000)]
      ].filter(Boolean));
      actions(a, kind);
      if (!a.recorded) return;

      var val = function (v) { return measure(a.unit, v); };
      put(why, t('alertd.why.' + a.kind + (a.kind === 'app_rule' ? memory ? '_memory' : '_cpu' : ''), {
        app: a.app, value: val(a.fired), limit: val(a.limit), hold: dur(a.hold_s), window: dur((a.params.minutes || 0) * 60)
      }));
      put(rule, t(a.kind === 'app_rule' ? 'alertd.rule.own' : kind.limit ? 'alertd.rule.settings' : 'alertd.rule.fixed') + (a.kind === 'temp' ? ' ' + t('temp.rule_hint') : ''));
      pairs(numList, [
        [t('alertd.at_fire'), val(a.fired)],
        [t(a.below ? 'alertd.lowest' : 'alertd.peak'), val(a.peak)],
        [t('alertd.avg'), val(a.avg)],
        active && [t('alertd.now'), val(a.current)],
        a.limit > 0 && [t('alertd.limit'), (a.below ? '< ' : '> ') + val(a.limit)],
        [t('alertd.hold'), dur(a.hold_s)]
      ].filter(Boolean));

      put(chartTitle, metric);
      pairs(ctxList, a.facts.map(function (f) { return [t(ALERT_FACTS[f.key] || 'hist.metric'), measure(f.unit, f.value)]; }));
      ctx.el.hidden = !a.facts.length && !a.top.length && !a.procs.length;
      topHead.hidden = topList.hidden = !a.top.length;
      put(topHead, t('top_apps') + ' · ' + t(TOP_BY[a.top_unit] || 'm.cpu'));
      rows(topList, a.top, a.top_unit);
      procHead.hidden = procList.hidden = !a.procs.length;
      put(procHead, t('alertd.procs') + ' · ' + t('n.process', { n: a.pid_count }));
      rows(procList, a.procs, a.unit);
    }

    return {
      el: el,
      scroller: body,
      update: function () {
        if (!id) pick();
        else if (!got && tries >= 3) got = 'gone';
        // An open alert moves on: its numbers and its chart are read again every ten seconds, and once more when it ends.
        else if (Date.now() - asked > (!got ? 2000 : got !== 'gone' && !got.until ? 10000 : Infinity)) ask();
        render();
      },
      shown: function () {
        id = '';
        tries = 0;
        if (state) pick();
        render();
      },
      resize: chart.resize,
      onDetail: function (r) {
        // A refusal is about the alert being waited for, or names the one on screen.
        if (r.alert ? r.alert.id !== id : got && r.about !== id) return;
        got = r.alert || 'gone';
        if (state) render();
        var a = r.alert;
        if (!a || !a.recorded) return;
        var kind = ALERT_KINDS[a.kind] || {}, memory = a.kind === 'app_rule' && a.params.metric === 'memory';
        // The shade ends with the alert, or with the last sample of one that is still open.
        chart.draw({
          metric: memory ? 'memory' : kind.color, range: '', start: a.start, step_s: a.step_s,
          series: [{ name: 'value', unit: a.unit, points: a.points }], label: t(memory ? 'm.memory' : kind.name || 'hist.metric'),
          limit: a.limit, from: a.since, to: a.until || a.start + a.points.length * a.step_s * 1000, empty: t('alertd.no_samples')
        });
      }
    };
  }

  var builders = {
    overview: buildOverview, processes: buildProcesses, network: buildNetwork, history: buildHistory, dev: buildDev, storage: buildStorage,
    settings: buildSettings, 'detail:clock': buildClock, 'detail:alert': buildAlert
  };
  var tabs = {};
  var back = document.getElementById('back');
  var title = document.getElementById('title');
  var view = document.getElementById('view');
  var tabBar = document.getElementById('tabs');
  var gear = document.getElementById('open-settings');
  var pin = document.getElementById('pin');
  var toastBox = document.getElementById('toast');
  var toastTimer = 0, before = 'overview';
  // alertFrom is the tab the alert screen was opened from and goes back to, alertRow the button that opened it.
  var alertFrom = 'overview', alertRow = null;
  // inSection is true while a section of Settings covers their menu: Back, Esc and ⌘[ then return to the menu.
  var inSection = false;

  function isDetail(name) { return name.indexOf('detail:') === 0; }

  // A detail screen is pushed over the Overview, which stays built and gets its scroll position back.
  function show(name) {
    if (state && (name === 'detail:gpu' && !state.gpu || name === 'detail:battery' && !state.battery)) name = 'overview';
    var from = current, detail = isDetail(name), was = isDetail(from) || inSection;
    // display: none drops the scroll offset, so it is kept by hand.
    if (tabs[from]) (tabs[from].scroller || tabs[from].el).$top = (tabs[from].scroller || tabs[from].el).scrollTop;
    if (name !== 'settings') before = name;
    if (name === 'detail:alert' && !isDetail(from) && from !== 'settings') alertFrom = from;
    closePop();
    current = name;
    if (!tabs[name]) {
      tabs[name] = builders[name] ? builders[name]() : buildDetail(name.slice(7));
      view.appendChild(tabs[name].el);
    }
    var el = tabs[name].el;
    Object.keys(tabs).forEach(function (t) { tabs[t].el.hidden = t !== name; });
    var scroller = tabs[name].scroller || el;
    scroller.scrollTop = detail && !isDetail(from) ? 0 : scroller.$top || 0;
    inSection = name === 'settings' && tabs.settings.open(section);
    var deep = detail || inSection;
    el.classList.remove('push', 'pop');
    if (deep !== was) {
      void el.offsetWidth;
      el.classList.add(deep ? 'push' : 'pop');
    }
    tabBar.hidden = deep;
    back.hidden = title.hidden = !deep;
    if (deep) {
      put(title, t(inSection ? SECTIONS[section] : 'm.' + name.slice(7)));
      backText.data = t('tab.' + (inSection ? 'settings' : name === 'detail:alert' ? alertFrom : 'overview'));
    }
    var home = isDetail(before) ? 'overview' : before;
    Array.prototype.forEach.call(tabBar.children, function (b) {
      var on = b.dataset.tab === name;
      b.setAttribute('aria-selected', String(on));
      b.tabIndex = on || (name === 'settings' && b.dataset.tab === home) ? 0 : -1;
    });
    // A hidden strip has no width to measure, so the one a detail screen gave way to is fitted now.
    fitTabs();
    gear.setAttribute('aria-pressed', String(name === 'settings'));
    document.querySelector('.top').classList.remove('scrolled');
    send({ type: 'tab', name: inSection || name === 'settings' && section ? 'settings:' + section : name, mode: mode });
    // shown comes first: it takes the shared cleanup card in and starts what update then reads.
    if (tabs[name].shown) tabs[name].shown();
    if (state) tabs[name].update(state);
    // Keyboard focus follows the push and comes back to the tile that opened the screen.
    if (deep && !was) back.focus();
    else if (!detail && isDetail(from)) {
      var origin = from === 'detail:alert' ? alertRow : el.querySelector('[data-go="' + from + '"]');
      if (origin && origin.isConnected) origin.focus();
    }
  }

  // The labels never change; a row they do not fit gets less padding. The width is measured, never
  // assumed: it changes with the language, the font of the theme, the tabs shown and the size of the
  // panel. Only a row that fits in neither form scrolls, and then keeps the open tab in view.
  function fitTabs() {
    if (!dict || tabBar.hidden) return;
    Array.prototype.forEach.call(tabBar.children, function (b) { put(b, t('tab.' + b.dataset.tab)); });
    ['full', 'tight'].some(function (form) {
      tabBar.dataset.fit = form;
      return tabBar.scrollWidth <= tabBar.clientWidth;
    });
    var on = tabBar.querySelector('[aria-selected="true"]');
    if (on && tabBar.scrollWidth > tabBar.clientWidth) on.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    fadeTabs();
  }

  // The strip fades on the side where tabs are scrolled out of sight; its own 2 px of padding do not count.
  function fadeTabs() {
    tabBar.classList.toggle('more-left', tabBar.scrollLeft > 3);
    tabBar.classList.toggle('more-right', tabBar.scrollLeft + tabBar.clientWidth < tabBar.scrollWidth - 3);
  }

  function toast(n) {
    var close = h('button', 'mini');
    close.appendChild(icon('close'));
    close.setAttribute('aria-label', t('dismiss'));
    close.onclick = function () { toastBox.hidden = true; };
    toastBox.textContent = '';
    toastBox.className = n.level === 'error' ? 'error' : '';
    add(toastBox, icon(n.level === 'error' ? 'alert' : 'info'), h('span', '', n.text), close);
    toastBox.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { toastBox.hidden = true; }, 8000);
  }

  // The tabs of settings.tab_order, in that order; one left out is hidden. Settings is not
  // among them: its gear is always there.
  var tabOrder = '';
  function applyTabOrder() {
    if (settings.tab_order.join() === tabOrder) return;
    tabOrder = settings.tab_order.join();
    Array.prototype.slice.call(tabBar.children).forEach(function (b) { b.hidden = settings.tab_order.indexOf(b.dataset.tab) < 0; });
    settings.tab_order.forEach(function (name) {
      var b = tabBar.querySelector('[data-tab="' + name + '"]');
      if (b) tabBar.appendChild(b);
    });
    fitTabs();
    // A hidden tab gives way to the first one, also when the page was opened on it by ?tab=.
    if (!shownTab(current)) show(settings.tab_order[0]);
  }

  // Settings and the detail screens are always there; a tab is there while settings.tab_order has it.
  function shownTab(name) { return name === 'settings' || isDetail(name) || settings.tab_order.indexOf(name) >= 0; }

  // openTab shows a tab asked for by name (Back, Esc, a shortcut, a link); a tab the user has hidden gives way to the first one.
  function openTab(name) {
    if (name.indexOf('detail:alert') === 0) {
      alertId = name.slice(13);
      name = 'detail:alert';
    }
    if (name.split(':')[0] === 'settings') {
      section = has(SECTIONS, name.slice(9)) ? name.slice(9) : '';
      name = 'settings';
    }
    show(shownTab(name) ? name : settings.tab_order[0]);
  }

  // Back leaves a detail screen for the Overview; an alert, for the tab its row is on; a section of
  // Settings, for their menu and the row that opened it.
  function goBack() {
    var row = inSection && tabs.settings.rows[section];
    openTab(row ? 'settings' : current === 'detail:alert' ? alertFrom : 'overview');
    if (row) row.focus({ preventScroll: true });
  }

  window.mp = {
    send: send,
    onState: function (s) {
      state = s;
      settings = s.settings;
      applyTheme();
      applyTabOrder();
      attr(pin, 'aria-pressed', String(s.pinned));
      var want = forcedLang || (s.settings.language && s.settings.language !== 'system' ? s.settings.language : s.system_lang || guessedLang);
      if (want !== lang) { setLang(want); rebuild(); }
      if (current === 'detail:gpu' && !s.gpu || current === 'detail:battery' && !s.battery) { show('overview'); return; }
      record(s);
      // One unexpected field must not leave the tab frozen at "—" for good.
      try { tabs[current].update(s); } catch (e) {
        toast({ level: 'error', text: t('err.display') });
        send({ type: 'log', message: String(e && e.stack || e) });
      }
      // The list under an open explanation reorders with every sample.
      if (pop && !pop.btn.isConnected) closePop();
      else if (pop) placePop();
    },
    onHistory: function (hist) {
      if (tabs[current].onHistory) tabs[current].onHistory(hist);
    },
    // onStorage answers storage_open: one folder of the scanned tree.
    onStorage: function (level) {
      if (tabs.storage) tabs.storage.onStorage(level);
    },
    // An answer nobody on screen asked for (MAC_PULSE_ACTION, or the screen was left) becomes a toast.
    onAction: function (r) {
      // about is what Go's text names (a browser, an alert id, a category) before the key turns it into a sentence.
      r.about = r.text;
      if (r.key) r.text = t(r.key, { text: r.text });
      // The speed test answers in numbers: bytes per second both ways, the idle round trip, Apple's responsiveness score.
      if (r.action === 'speedtest' && r.values) {
        r.text = [t('col.down') + ' ' + join(rate(r.values.down)), t('col.up') + ' ' + join(rate(r.values.up)),
          Math.round(r.values.rtt_ms) + ' ms', Math.round(r.values.rpm) + ' RPM'].join(' · ');
      }
      if (r.action === 'app_info') showInfo(r);
      else if (r.action === 'proc_detail') showDetail(r);
      else if (r.action === 'alert_detail') { if (tabs['detail:alert']) tabs['detail:alert'].onDetail(r); }
      else if (r.action === 'browser_tabs') { if (tabs.processes) tabs.processes.onTabs(r); }
      else if (r.action === 'cleanup_list' && cleanup && cleanup.onListed(r)) return;
      else if (r.action === 'cleanup' && cleanup && cleanup.onCleaned(r)) return;
      else if (r.action === 'cleanup_scan' && !r.ok && cleanup) cleanup.say(r.text, true);
      // What a finished scan or measurement found is in state.storage; only a failure has words of its own.
      else if (r.ok && (r.action === 'storage_scan' || r.action === 'cleanup_scan' || r.action === 'cleanup_list')) return;
      else if (acting[r.action]) acting[r.action](r);
      else {
        var about = { ping: 'Ping 1.1.1.1: ', public_ip: t('net.public_ip') + ': ', speedtest: t('net.speedtest') + ': ' }[r.action] || '';
        toast({ level: r.ok ? 'info' : 'error', text: about + r.text });
      }
    },
    // show opens a tab the shell asks for (the Settings menu item, a mac-pulse:// link); Go has checked the name.
    show: openTab,
    // ask opens the quit confirm of the heaviest killable app (the ⌃⌥K shortcut).
    ask: function () {
      show('processes');
      tabs.processes.askTop();
    },
    onNotice: function (n) {
      n = { level: n.level, text: n.texts.map(function (x) { return t(x.key, x.params); }).join('; ') };
      if (current === 'processes' && tabs.processes.notice(n)) return;
      toast(n);
    }
  };

  window.onerror = function (message, source, line) {
    send({ type: 'log', message: message + ' (' + source + ':' + line + ')' });
  };

  root.dataset.mode = mode;
  root.classList.toggle('mock', mock);
  root.classList.toggle('backdrop', mock && params.has('backdrop'));
  var darkQuery = window.matchMedia('(prefers-color-scheme: dark)');
  // The ids of settings.Themes in Go; themes.css has a block for each but aqua, which is app.css itself.
  var SKINS = ['aqua', 'graphite', 'nord', 'catppuccin', 'solarized', 'gruvbox', 'terminal', 'paper', 'contrast', 'vivid'];
  // A theme drawn for one mode keeps it whatever the appearance setting, ?theme= or the system says.
  var SKIN_MODE = { terminal: 'dark', paper: 'light' };
  // The charts are SVG on var() colours, so the two attributes repaint the page with no rebuild.
  function applyTheme() {
    var skin = SKINS.indexOf(params.get('skin')) >= 0 ? params.get('skin') : SKINS.indexOf(settings.theme) >= 0 ? settings.theme : 'catppuccin';
    var forced = SKIN_MODE[skin] || (settings.appearance !== 'auto' && settings.appearance) || params.get('theme');
    var theme = forced === 'light' || forced === 'dark' ? forced : darkQuery.matches ? 'dark' : 'light';
    if (root.dataset.theme !== theme) root.dataset.theme = theme;
    if (root.dataset.skin === skin) return;
    root.dataset.skin = skin;
    // A theme brings its own font, and the labels their own widths.
    fitTabs();
  }
  applyTheme();
  darkQuery.addEventListener('change', applyTheme);

  var toWindow = document.getElementById('open-window');
  // The same corner button leads out of the panel into the window and back.
  toWindow.appendChild(icon(wide ? 'panel' : 'window'));
  toWindow.onclick = function () { send({ type: wide ? 'open_panel' : 'open_window' }); };
  // A pinned panel stays open when the user clicks elsewhere; the shell reports the state back in state.pinned.
  pin.appendChild(icon('pin'));
  pin.onclick = function () { send({ type: 'pin', enabled: pin.getAttribute('aria-pressed') !== 'true' }); };
  gear.appendChild(icon('system'));
  gear.onclick = function () {
    if (current !== 'settings') openTab('settings');
    else show(shownTab(before) ? before : settings.tab_order[0]);
  };
  var backText = back.appendChild(document.createTextNode(''));
  back.prepend(icon('chevron'));

  // ?lang= pins the language (screenshots). Until the first state says what "system" means, the web view's own list does.
  var forcedLang = params.get('lang') && matchLang([params.get('lang')]);
  var guessedLang = matchLang(navigator.languages || []);

  function setLang(code) {
    lang = I18N[code] ? code : 'en';
    dict = I18N[lang];
    plurals = new Intl.PluralRules(lang);
    formats = {};
    // The lang attribute also picks the Chinese or the Japanese glyphs for the shared Han characters.
    root.lang = lang;
    fitTabs();
    tabBar.setAttribute('aria-label', t('nav.sections'));
    backText.data = t('tab.overview');
    toWindow.title = t(wide ? 'open_panel' : 'open_window');
    toWindow.setAttribute('aria-label', toWindow.title);
    pin.title = t('pin');
    pin.setAttribute('aria-label', t('pin'));
    gear.title = t('tab.settings') + ' (⌘,)';
    gear.setAttribute('aria-label', t('tab.settings'));
  }

  // Every view bakes its labels in when it is built, so a new language builds the views again.
  function rebuild() {
    var scroller = tabs[current].scroller || tabs[current].el, at = scroller.scrollTop, picking = document.activeElement === tabs[current].language;
    Object.keys(tabs).forEach(function (name) { tabs[name].el.remove(); });
    tabs = {};
    cleanup = null;
    toastBox.hidden = true;
    show(current);
    (tabs[current].scroller || tabs[current].el).scrollTop = at;
    if (picking) tabs[current].language.focus();
  }

  setLang(forcedLang || guessedLang);
  back.onclick = goBack;
  tabBar.onclick = function (e) {
    var b = e.target.closest('button');
    if (b) show(b.dataset.tab);
  };
  arrowKeys(tabBar);
  tabBar.addEventListener('scroll', fadeTabs);

  // Any scroll container (the tab or a table list) puts a hairline under the header while scrolled.
  var top = document.querySelector('.top');
  document.addEventListener('scroll', function (e) {
    // A list inside an explanation scrolls without closing it.
    if (pop && pop.el.contains(e.target)) return;
    top.classList.toggle('scrolled', e.target.scrollTop > 0);
    closePop();
  }, true);

  document.addEventListener('pointerdown', function (e) {
    root.classList.remove('keys');
    if (pop && !pop.el.contains(e.target) && !pop.btn.contains(e.target)) closePop();
  }, true);
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Tab' || e.key.indexOf('Arrow') === 0) root.classList.add('keys');
    if (e.key === 'Escape') {
      // Esc closes an open explanation or confirm, then leaves a detail screen; only after that does the shell close the panel.
      var swallowed = closePop() || tabs.processes && tabs.processes.cancel() || cleanup && cleanup.el.offsetParent && cleanup.cancel();
      if (!swallowed && (isDetail(current) || inSection)) { goBack(); swallowed = true; }
      if (swallowed) { e.preventDefault(); e.stopPropagation(); }
      return;
    }
    if (!e.metaKey || e.altKey || e.ctrlKey) return;
    if (e.key >= '1' && e.key <= '6' && settings.tab_order[Number(e.key) - 1]) show(settings.tab_order[Number(e.key) - 1]);
    else if (e.key === '[' && (isDetail(current) || inSection)) goBack();
    else if (e.key === ',') openTab('settings');
    else if (e.key === 'f') { openTab('processes'); if (current === 'processes') tabs.processes.focusSearch(); }
    else return;
    e.preventDefault();
  }, true);

  var resizing = 0;
  window.addEventListener('resize', function () {
    cancelAnimationFrame(resizing);
    resizing = requestAnimationFrame(function () {
      fitTabs();
      if (tabs[current].resize) tabs[current].resize();
    });
  });

  show(current);

  if (mock) {
    var pending = 3;
    ['mock.js', 'mock-icons.js', 'mock-live.js'].forEach(function (src) {
      var script = document.createElement('script');
      script.src = src;
      script.onload = script.onerror = function () {
        if (--pending || !window.MP_MOCK_LIVE) return;
        window.MP_MOCK_LIVE.start();
        show(current);
      };
      document.head.appendChild(script);
    });
  }
})();
