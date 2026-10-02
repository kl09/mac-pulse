package netinspect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kl09/mac-pulse/internal/collector"
)

const (
	releaseAPI     = "https://api.github.com/repos/kl09/mac-pulse/releases/latest"
	releaseTimeout = 10 * time.Second
	// A release with long notes and many assets is hundreds of kilobytes of JSON.
	releaseMaxBytes = 1 << 20
	releaseTagRunes = 40
)

// ErrNoRelease means the repository has published no release yet, which is an answer, not a failure.
var ErrNoRelease = errors.New("no release published")

// LatestRelease asks GitHub for the tag of the newest mac-pulse release. It is the update
// check's one request and runs only when the user presses the button; nothing is downloaded.
func LatestRelease(ctx context.Context) (tag string, err error) {
	return latestRelease(ctx, releaseAPI)
}

func latestRelease(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, releaseTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{
		Transport:     &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	// GitHub answers 404 both for a repository without releases and for a private one.
	case http.StatusNotFound:
		return "", ErrNoRelease
	default:
		return "", fmt.Errorf("request latest release: status %d", resp.StatusCode)
	}
	// The answer is remote input: only the tag is taken, cleaned and cut.
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, releaseMaxBytes)).Decode(&release); err != nil {
		return "", fmt.Errorf("decode latest release: %w", err)
	}
	tag := collector.CleanText(release.Tag, releaseTagRunes)
	if tag == "" {
		return "", errors.New("latest release has no tag")
	}
	return tag, nil
}

// Newer reports whether the release tag latest is a later version than current: dotted
// numbers compared left to right, a leading "v" ignored, "dev" older than any release.
func Newer(latest, current string) bool {
	return slices.Compare(versionNumbers(latest), versionNumbers(current)) > 0
}

// versionNumbers reads "v0.10.0" as 0, 10 and "v0.5.1-rc1" as 0, 5, 1: each part counts by
// its leading digits, and one without any counts as 0. Trailing zeros go, so that "0.5" and
// "0.5.0" are one version and "dev" is none.
func versionNumbers(version string) []int {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	numbers := make([]int, len(parts))
	for i, part := range parts {
		if end := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); end >= 0 {
			part = part[:end]
		}
		numbers[i], _ = strconv.Atoi(part)
	}
	for len(numbers) > 0 && numbers[len(numbers)-1] == 0 {
		numbers = numbers[:len(numbers)-1]
	}
	return numbers
}
