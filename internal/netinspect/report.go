package netinspect

// Report is one nettop poll: rates are bytes per second against the previous
// poll and totals are counted from the first time the pid was seen.
type Report struct {
	HasRates  bool
	Apps      []AppTraffic
	Conns     []Conn
	Listening []Listener
	Talkers   []Talker
}

type AppTraffic struct {
	App string
	// BundlePath is the .app directory, "" for a process outside any bundle.
	BundlePath string
	PIDs       []int32
	DownRate   float64
	UpRate     float64
	DownTotal  uint64
	UpTotal    uint64
	Conns      int
}

type Conn struct {
	App        string
	PID        int32
	Proto      string
	Local      string
	RemoteIP   string
	RemotePort string
	Host       string
	State      string
	BytesIn    uint64
	BytesOut   uint64
	DownRate   float64
	UpRate     float64
}

type Listener struct {
	App   string
	PID   int32
	Proto string
	Addr  string
	Port  string
	// Dir is the process's working directory with the home directory as "~"; "" for
	// another user's process and for "/", which is every GUI app.
	Dir string
}

type Talker struct {
	IP       string
	Host     string
	Apps     []string
	DownRate float64
	UpRate   float64
	Conns    int
}
