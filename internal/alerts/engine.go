package alerts

import (
	"cmp"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/settings"
)

const (
	// A rule must hold for the alert_hold setting, or its finding's own hold, before it
	// becomes an alert, and stay quiet this long before the alert closes.
	// Time the Mac spent asleep between two samples counts towards the hold.
	clearAfter = 30 * time.Second
	// One notification per rule and app within cooldown, however often the alert reopens.
	cooldown  = time.Hour
	maxRecent = 20
	// maxKindRunes cuts the kind of a stored alert that is logged as unknown.
	maxKindRunes = 40
)

type Alert struct {
	// ID is stable for as long as the alert stays open.
	ID string
	// Kind is one of the Kind constants.
	Kind string
	// Params fill the dictionary strings alert.<Kind>.title and alert.<Kind>.detail.
	Params map[string]any
	// App is "" for a system-wide alert and the device name for KindBTBattery.
	App   string
	Since time.Time
	// Until is zero while the alert is active.
	Until time.Time
}

// Engine guards the alert state; any goroutine may call it.
type Engine struct {
	notify func(Alert)

	mu      sync.Mutex
	tracked map[subject]*tracked
	// recent holds closed alerts, newest first.
	recent   []Record
	notified map[subject]time.Time
	rss      map[string]rssTrack
	// pre holds the last minutes of every system metric, by kind: a new alert starts its
	// samples from them.
	pre map[string]*Series
}

type subject struct {
	kind   string
	app    string
	metric string
}

// tracked is a rule that holds now or did moments ago; it is an alert once active.
type tracked struct {
	Record
	active   bool
	lastSeen time.Time
}

// New takes the delivery of a fired alert and the closed alerts of an earlier run, newest
// first. notify runs on the goroutine that calls Check, after the lock is released, so a
// slow one delays that caller.
func New(notify func(Alert), closed []Record) *Engine {
	e := &Engine{
		notify:   notify,
		recent:   restored(closed),
		tracked:  map[subject]*tracked{},
		notified: map[subject]time.Time{},
		rss:      map[string]rssTrack{},
		pre:      map[string]*Series{},
	}
	for _, kind := range systemKinds {
		e.pre[kind] = &Series{}
	}
	return e
}

// restored is what New keeps of the alerts a file held: the newest maxRecent that the engine
// could have written itself. Any program of the user can write that file, and one number
// that is not finite would stop every state from encoding.
func restored(closed []Record) []Record {
	var out []Record
	for _, r := range closed {
		switch {
		case len(out) == maxRecent:
			return out
		case wellFormed(r):
			out = append(out, r)
		default:
			slog.Warn("stored alert dropped: not one the engine writes", "kind", collector.CleanText(r.Kind, maxKindRunes))
		}
	}
	return out
}

func wellFormed(r Record) bool {
	numbers := []float64{}
	for _, v := range r.Params {
		switch v := v.(type) {
		case float64:
			numbers = append(numbers, v)
		case int, string:
		default:
			return false
		}
	}
	if d := r.Detail; d != nil {
		c := d.Context
		numbers = append(numbers, d.Limit, d.Fired, d.Peak, d.Last, d.Sum)
		for _, row := range append(slices.Clone(c.Top), c.Procs...) {
			numbers = append(numbers, row.Value)
		}
		for _, f := range c.Facts {
			numbers = append(numbers, f.Value)
		}
		// A bucket nothing was sampled in is NaN, so the points themselves are not judged.
		if len(d.Series.V) > maxPoints || (len(d.Series.V) > 0 && d.Series.Step <= 0) ||
			len(c.Top) > contextRows || len(c.Procs) > contextRows || len(c.Facts) > maxFacts {
			return false
		}
	}
	return slices.Contains(kinds, r.Kind) && !slices.ContainsFunc(numbers, func(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) })
}

// Check applies the rules to one sample; the sample's own time is the clock.
func (e *Engine) Check(snap *collector.Snapshot, cfg settings.Settings) {
	now := snap.Time
	var fired []Alert
	e.mu.Lock()
	e.trackRSS(snap.Apps, now)
	var found []finding
	if cfg.Alerts {
		found = findings(snap, cfg, e.rss)
	}
	// After a backward clock step a time from before it would hold its rule or its alert
	// for the length of the step.
	maps.DeleteFunc(e.tracked, func(_ subject, t *tracked) bool { return now.Before(t.lastSeen) })
	maps.DeleteFunc(e.notified, func(_ subject, at time.Time) bool { return now.Sub(at) >= cooldown || now.Before(at) })
	for _, f := range found {
		if a, notify := e.hold(snap, cfg, f); notify {
			fired = append(fired, a)
		}
	}
	// After hold: a record that starts on this sample copies the pre-roll and then adds the
	// sample itself.
	for kind, series := range e.pre {
		if v, ok := value(snap, subject{kind: kind}); ok {
			series.slide(now, v)
		}
	}
	e.release(snap, cfg)
	e.mu.Unlock()
	for _, a := range fired {
		e.notify(a)
	}
}

// hold is Check for one rule that holds in snap: it starts or continues its record and fires
// the alert once the rule has held long enough. notify says the alert is to be delivered.
func (e *Engine) hold(snap *collector.Snapshot, cfg settings.Settings, f finding) (fired Alert, notify bool) {
	now := snap.Time
	s := subject{kind: f.kind, app: f.app, metric: f.metric}
	t := e.tracked[s]
	if t == nil {
		t = &tracked{Record: Record{
			Alert:  Alert{ID: fmt.Sprintf("%s%s:%s:%d", f.kind, f.metric, f.app, now.Unix()), Kind: f.kind, App: f.app, Since: now},
			Detail: &Detail{Unit: unitOf(s)},
		}}
		if pre := e.pre[s.kind]; pre != nil {
			t.Detail.Series = pre.clone()
		}
		e.tracked[s] = t
	}
	t.Params, t.lastSeen = f.params, now
	hold := cmp.Or(f.hold, time.Duration(cfg.AlertHold)*time.Second)
	t.Detail.Limit, t.Detail.Below, t.Detail.Hold = f.limit, f.below, hold
	v, measured := value(snap, s)
	if measured {
		t.Detail.observe(now, v, true)
	}
	if t.active || now.Sub(t.Since) < hold {
		return Alert{}, false
	}
	t.active = true
	t.Detail.FiredAt, t.Detail.Fired, t.Detail.Context = now, v, capture(snap, s)
	if _, recently := e.notified[s]; recently {
		return Alert{}, false
	}
	e.notified[s] = now
	return t.Alert, true
}

// release is Check for the rules that do not hold in snap: one that never fired is forgotten,
// an alert closes once its rule has been quiet for clearAfter.
func (e *Engine) release(snap *collector.Snapshot, cfg settings.Settings) {
	now := snap.Time
	for s, t := range e.tracked {
		switch {
		case t.lastSeen.Equal(now):
		case !t.active:
			// The rule let go before it became an alert: the hold starts over.
			delete(e.tracked, s)
		case now.Sub(t.lastSeen) >= clearAfter || s.kind != KindBTBattery && muted(cfg, s.app):
			t.Until = t.lastSeen
			e.recent = append([]Record{t.Record}, e.recent[:min(len(e.recent), maxRecent-1)]...)
			delete(e.tracked, s)
		// What the metric does after the rule lets go is the end of the story the samples tell.
		default:
			if v, ok := value(snap, s); ok {
				t.Detail.observe(now, v, false)
			}
		}
	}
}

// trackRSS moves every app's low-water mark. The map is rebuilt per sample so an app
// that quit is forgotten and warms up again on relaunch.
// The window tumbles instead of sliding, so growth that straddles a reset
// goes unseen; keep per-minute RSS samples if a slow leak must be caught.
func (e *Engine) trackRSS(apps []collector.App, now time.Time) {
	rss := make(map[string]rssTrack, len(apps))
	for _, app := range apps {
		track, ok := e.rss[app.Name]
		switch {
		case !ok:
			track = rssTrack{low: app.RSS, lowAt: now, warmUntil: now.Add(appWarmUp)}
		case now.Before(track.warmUntil) || app.RSS < track.low || now.Sub(track.lowAt) >= appMemoryWindow:
			track.low, track.lowAt = app.RSS, now
		}
		rss[app.Name] = track
	}
	e.rss = rss
}

// Active returns the open alerts, oldest first.
func (e *Engine) Active() []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()
	var active []Alert
	for _, t := range e.tracked {
		if t.active {
			active = append(active, t.Alert)
		}
	}
	slices.SortFunc(active, func(x, y Alert) int {
		return cmp.Or(x.Since.Compare(y.Since), cmp.Compare(x.ID, y.ID))
	})
	return active
}

// Recent returns at most 20 closed alerts, newest first.
func (e *Engine) Recent() []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.recent) == 0 {
		return nil
	}
	recent := make([]Alert, 0, len(e.recent))
	for _, r := range e.recent {
		recent = append(recent, r.Alert)
	}
	return recent
}

// Record returns the alert with id, open or closed, with what was recorded about it.
func (e *Engine) Record(id string) (Record, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, t := range e.tracked {
		if t.active && t.ID == id {
			return Record{Alert: t.Alert, Detail: t.Detail.clone()}, true
		}
	}
	// A closed record is never written again.
	if i := slices.IndexFunc(e.recent, func(r Record) bool { return r.ID == id }); i >= 0 {
		return e.recent[i], true
	}
	return Record{}, false
}

// Closed returns what a restart should bring back, at most 20 records, newest first: the
// open alerts, closed at the last time their rule held, and then the recent ones.
func (e *Engine) Closed() []Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	var closed []Record
	for _, t := range e.tracked {
		if t.active {
			r := Record{Alert: t.Alert, Detail: t.Detail.clone()}
			r.Until = t.lastSeen
			closed = append(closed, r)
		}
	}
	slices.SortFunc(closed, func(x, y Record) int { return cmp.Or(y.Since.Compare(x.Since), cmp.Compare(x.ID, y.ID)) })
	closed = append(closed, e.recent...)
	return closed[:min(len(closed), maxRecent)]
}
