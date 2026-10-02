package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// file is written as settings.json unless empty.
		file string
		want func(*Settings)
		// wantBackup says the file must be kept as settings.json.bak, byte for byte.
		wantBackup bool
	}{
		{name: "missing file gives defaults", want: func(*Settings) {}},
		{
			name: "saved values override defaults",
			file: `{"temp_unit":"F","net_unit":"bits","menu_bar":["mem","net"],"alerts":false,"alert_cpu":80,"alert_temp":100,` +
				`"alert_disk_free":5,"apps_show_system":true,"hotkey":true}`,
			want: func(s *Settings) {
				s.TempUnit, s.NetUnit, s.MenuBar, s.Alerts = "F", "bits", []string{"mem", "net"}, false
				s.AlertCPU, s.AlertTemp, s.AlertDiskFree, s.AppsShowSystem, s.Hotkey = 80, 100, 5, true, true
			},
		},
		{
			name: "missing keys keep their defaults",
			file: `{"alert_cpu":50}`,
			want: func(s *Settings) { s.AlertCPU = 50 },
		},
		{
			name: "unknown keys are ignored",
			file: `{"accent":"blue","launch_at_login":true,"net_unit":"bits"}`,
			want: func(s *Settings) { s.NetUnit = "bits" },
		},
		{
			name: "invalid value keeps the default, valid neighbours load",
			file: `{"temp_unit":"K","alert_cpu":500,"menu_bar":[],"alert_temp":70}`,
			want: func(s *Settings) { s.AlertTemp = 70 },
		},
		{name: "file that is not JSON gives defaults and is kept aside", file: `{"temp_unit":`, want: func(*Settings) {}, wantBackup: true},
		{
			name: "one bad byte among good settings: the file is kept aside before a Set can overwrite it",
			file: "{\"temp_unit\":\"F\",\x00\"alert_cpu\":50}", want: func(*Settings) {}, wantBackup: true,
		},
		{name: "file with a JSON array gives defaults and is kept aside", file: `[1,2]`, want: func(*Settings) {}, wantBackup: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if tt.file != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), []byte(tt.file), 0o600))
			}
			want := Default()
			tt.want(&want)

			s, err := Open(dir)

			require.NoError(t, err)
			assert.Equal(t, want, s.Get())
			if !tt.wantBackup {
				assert.NoFileExists(t, filepath.Join(dir, fileName+".bak"))
				return
			}
			require.NoError(t, s.Set("alert_cpu", json.RawMessage(`70`)))
			kept, err := os.ReadFile(filepath.Join(dir, fileName+".bak"))
			require.NoError(t, err)
			assert.Equal(t, tt.file, string(kept), "what the user had is still on disk after the next Set")
		})
	}
}

func TestStore_Set(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		value   string
		want    func(*Settings)
		wantErr error
		// wantFile is false when nothing may be written.
		wantFile bool
	}{
		{name: "valid value is saved", key: "alert_cpu", value: `70`, want: func(s *Settings) { s.AlertCPU = 70 }, wantFile: true},
		{
			name: "slice value is saved", key: "menu_bar", value: `["temp","cpu"]`,
			want: func(s *Settings) { s.MenuBar = []string{"temp", "cpu"} }, wantFile: true,
		},
		{name: "invalid value changes nothing", key: "alert_cpu", value: `700`, want: func(*Settings) {}, wantErr: ErrInvalidValue},
		{name: "unknown key changes nothing", key: "nope", value: `1`, want: func(*Settings) {}, wantErr: ErrUnknownKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// The directory does not exist yet: the first Set creates it.
			dir := filepath.Join(t.TempDir(), "mac-pulse")
			s, err := Open(dir)
			require.NoError(t, err)
			want := Default()
			tt.want(&want)

			err = s.Set(tt.key, json.RawMessage(tt.value))

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, want, s.Get())
			info, statErr := os.Stat(filepath.Join(dir, fileName))
			if !tt.wantFile {
				require.ErrorIs(t, statErr, os.ErrNotExist)
				return
			}
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			assert.NoFileExists(t, filepath.Join(dir, fileName+".tmp"))
			reopened, err := Open(dir)
			require.NoError(t, err)
			assert.Equal(t, want, reopened.Get())
		})
	}
}
