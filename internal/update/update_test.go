package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.9.3", "0.9.2", true},
		{"v0.10.0", "0.9.2", true}, // numeric, not alphabetical
		{"0.9.2", "0.9.2", false},
		{"0.9.1", "0.9.2", false},
		{"1.0.0", "0.99.99", true},
		{"0.9", "0.9.2", false},
		{"nonsense", "0.9.2", false},
		{"0.9.3", "", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// Latest reads the release and picks this system's file; Download fetches
// it with progress and rejects a short download.
func TestLatestAndDownload(t *testing.T) {
	payload := []byte("pretend installer contents")
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			fmt.Fprintf(w, `{"tag_name":"v0.10.0","html_url":"%[1]s/page","body":"notes","assets":[
				{"name":"Pigeonhole-0.10.0-mac.dmg","browser_download_url":"%[1]s/file","size":%[2]d},
				{"name":"Pigeonhole-0.10.0-windows-portable.zip","browser_download_url":"%[1]s/zip","size":1},
				{"name":"Pigeonhole-0.10.0-windows-setup.exe","browser_download_url":"%[1]s/file","size":%[2]d}]}`,
				srv.URL, len(payload))
		case "/file":
			_, _ = w.Write(payload)
		case "/short":
			_, _ = w.Write(payload[:5])
		}
	}))
	defer srv.Close()
	LatestURL = srv.URL + "/latest"

	r, err := Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "0.10.0" || r.PageURL != srv.URL+"/page" || r.Notes != "notes" {
		t.Fatalf("unexpected release: %+v", r)
	}
	if suffix := assetSuffix(runtime.GOOS); suffix != "" {
		if r.AssetURL != srv.URL+"/file" || r.AssetSize != int64(len(payload)) {
			t.Fatalf("picked the wrong file for %s: %+v", runtime.GOOS, r)
		}
	}

	r.AssetURL, r.Asset, r.AssetSize = srv.URL+"/file", "Pigeonhole-0.10.0-mac.dmg", int64(len(payload))
	var lastDone int64
	path, err := Download(context.Background(), r, t.TempDir(), func(done, total int64) { lastDone = done })
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != string(payload) || lastDone != int64(len(payload)) {
		t.Fatalf("download: %q, progress %d", data, lastDone)
	}

	r.AssetURL = srv.URL + "/short"
	if _, err = Download(context.Background(), r, t.TempDir(), nil); err == nil {
		t.Fatal("a short download should fail")
	}
}
