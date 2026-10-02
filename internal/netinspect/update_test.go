package netinspect

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatestRelease(t *testing.T) {
	t.Parallel()

	// What api.github.com answered for a repository without releases.
	const notFound = `{"message":"Not Found",` +
		`"documentation_url":"https://docs.github.com/rest/releases/releases#get-the-latest-release","status":"404"}`

	tests := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr string
		// wantNoRelease expects ErrNoRelease.
		wantNoRelease bool
	}{
		{name: "a release", status: http.StatusOK, body: `{"tag_name":"v0.6.0","name":"mac-pulse 0.6","assets":[]}`, want: "v0.6.0"},
		{name: "no release published yet", status: http.StatusNotFound, body: notFound, wantNoRelease: true},
		{name: "rate limited", status: http.StatusForbidden, body: `{"message":"API rate limit exceeded"}`, wantErr: "status 403"},
		{name: "a redirect is not followed", status: http.StatusMovedPermanently, wantErr: "status 301"},
		{name: "an answer without a tag", status: http.StatusOK, body: `{"name":"mac-pulse"}`, wantErr: "no tag"},
		{name: "not JSON", status: http.StatusOK, body: "<html>", wantErr: "decode latest release"},
		{
			name: "a release with 200 KB of notes before its tag", status: http.StatusOK,
			body: `{"body":"` + strings.Repeat("x", 200<<10) + `","tag_name":"v9.9.9"}`, want: "v9.9.9",
		},
		{
			name: "a body past 1 MiB is cut before its tag", status: http.StatusOK,
			body: `{"body":"` + strings.Repeat("x", 1<<20) + `","tag_name":"v9.9.9"}`, wantErr: "decode latest release",
		},
		{
			name: "a tag is cleaned and cut", status: http.StatusOK,
			body: `{"tag_name":"v1.0\u202e` + strings.Repeat("9", 100) + `"}`, want: "v1.0" + strings.Repeat("9", releaseTagRunes-4),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
				w.Header().Set("Location", "/elsewhere")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)

			got, err := latestRelease(t.Context(), server.URL)

			assert.Equal(t, tt.want, got)
			switch {
			case tt.wantNoRelease:
				require.ErrorIs(t, err, ErrNoRelease)
			case tt.wantErr != "":
				require.ErrorContains(t, err, tt.wantErr)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
	}{
		{name: "numbers compare as numbers, not as text", latest: "v0.10.0", current: "0.9.1", want: true},
		{name: "the same version", latest: "v0.5.0", current: "0.5.0"},
		{name: "the leading v is not part of the first number", latest: "v1.1.0", current: "1.0.0", want: true},
		{name: "the same version past 1.0", latest: "v1.0.0", current: "1.0.0"},
		{name: "an older release", latest: "v0.4.9", current: "0.5.0"},
		{name: "a patch release", latest: "0.5.1", current: "0.5.0", want: true},
		{name: "a missing part is zero", latest: "v0.5", current: "0.5.0"},
		{name: "a missing part is zero, the other way", latest: "v0.5.0", current: "0.5"},
		{name: "a longer version is newer", latest: "v0.5.0.1", current: "0.5.0", want: true},
		{name: "a release candidate counts by its digits", latest: "v0.5.1-rc1", current: "0.5.0", want: true},
		{name: "a release candidate of the running version is not newer", latest: "v0.5.0-rc1", current: "0.5.0"},
		{name: "a development build is older than any release", latest: "v0.0.1", current: "dev", want: true},
		{name: "a tag that is no version is not newer", latest: "nightly", current: "0.5.0"},
		{name: "no tag", latest: "", current: "0.5.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Newer(tt.latest, tt.current))
		})
	}
}
