// Package update keeps Pigeonhole up to date from its GitHub releases (see
// .github/workflows/release.yml): it checks the latest release, downloads
// the right file for this computer, and installs it (install_*.go).
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// LatestURL is the GitHub API address of the newest published release.
// Tests point it at a local server.
var LatestURL = "https://api.github.com/repos/henzyliu/pigeonhole/releases/latest"

var client = &http.Client{Timeout: 30 * time.Second}

// Release is a published version and its download for this computer.
type Release struct {
	Version string `json:"version"` // without the "v"
	PageURL string `json:"pageUrl"` // the release page, with its notes
	Notes   string `json:"notes"`
	// Asset is the file for this system ("" if the release has none).
	Asset     string `json:"asset"`
	AssetURL  string `json:"assetUrl"`
	AssetSize int64  `json:"assetSize"`
}

// assetSuffix is the end of the release file meant for this system; see
// the file names in release.yml.
func assetSuffix(goos string) string {
	switch goos {
	case "darwin":
		return "-mac.dmg"
	case "windows":
		return "-windows-setup.exe"
	}
	return ""
}

// Latest fetches the newest release.
func Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LatestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Pigeonhole-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no releases yet")
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("couldn't read the release: %w", err)
	}
	r := &Release{Version: strings.TrimPrefix(body.TagName, "v"), PageURL: body.HTMLURL, Notes: body.Body}
	if suffix := assetSuffix(runtime.GOOS); suffix != "" {
		for _, a := range body.Assets {
			if strings.HasSuffix(a.Name, suffix) {
				r.Asset, r.AssetURL, r.AssetSize = a.Name, a.URL, a.Size
				break
			}
		}
	}
	return r, nil
}

// Newer reports whether version a is newer than b. Both are "X.Y.Z" (a
// leading "v" is ignored); anything unparseable is never newer.
func Newer(a, b string) bool {
	pa, okA := parse(a)
	pb, okB := parse(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Download saves the release's file for this system in dir, reporting
// progress as it goes, and checks it arrived whole.
func Download(ctx context.Context, r *Release, dir string, progress func(done, total int64)) (string, error) {
	if r.AssetURL == "" {
		return "", errors.New("this release has no download for this computer")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.AssetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Pigeonhole-updater")
	// Downloads can take longer than the API timeout.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	path := filepath.Join(dir, filepath.Base(r.Asset))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	total := r.AssetSize
	if total <= 0 {
		total = resp.ContentLength
	}
	w := &progressWriter{w: f, total: total, report: progress}
	_, err = io.Copy(w, resp.Body)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil && r.AssetSize > 0 && w.done != r.AssetSize {
		err = fmt.Errorf("download was incomplete (%d of %d bytes)", w.done, r.AssetSize)
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

type progressWriter struct {
	w      io.Writer
	done   int64
	total  int64
	report func(done, total int64)
	last   time.Time
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.report != nil && (time.Since(p.last) > 100*time.Millisecond || p.done == p.total) {
		p.last = time.Now()
		p.report(p.done, p.total)
	}
	return n, err
}
