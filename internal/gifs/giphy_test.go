package gifs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGiphy(t *testing.T) {
	var lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastQuery = r.URL.RawQuery
		if r.URL.Query().Get("api_key") != "k" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/search", "/trending":
			fmt.Fprint(w, `{"data":[
				{"id":"a1","title":"Cat GIF","images":{"fixed_width":{"url":"https://media1.giphy.com/a1/200w.gif","webp":"https://media1.giphy.com/a1/200w.webp","width":"200","height":"150"}}},
				{"id":"bad","title":"Elsewhere","images":{"fixed_width":{"url":"https://evil.example/x.gif"}}}]}`)
		case "/a1":
			fmt.Fprint(w, `{"data":{"id":"a1","title":"Cat GIF","images":{
				"downsized":{"url":"https://media1.giphy.com/a1/giphy-downsized.gif","width":"400","height":"300"},
				"original":{"url":"https://media1.giphy.com/a1/giphy.gif","mp4":"https://media1.giphy.com/a1/giphy.mp4"}}}}`)
		case "/limited":
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()
	BaseURL = srv.URL
	ctx := context.Background()

	c := &Client{Key: "k"}
	got, err := c.Search(ctx, " cats ", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a1" || got[0].Preview != "https://media1.giphy.com/a1/200w.webp" || got[0].PreviewWidth != 200 {
		t.Fatalf("search results (links off GIPHY dropped): %+v", got)
	}
	if !strings.Contains(lastQuery, "q=cats") || !strings.Contains(lastQuery, "rating=pg-13") {
		t.Fatalf("query: %s", lastQuery)
	}
	if _, err = c.Search(ctx, "", 24); err != nil || !strings.Contains(lastQuery, "offset=24") {
		t.Fatalf("an empty search shows trending: %v %s", err, lastQuery)
	}

	f, err := c.Files(ctx, "a1")
	if err != nil || f.GIF != "https://media1.giphy.com/a1/giphy-downsized.gif" || f.MP4 != "https://media1.giphy.com/a1/giphy.mp4" || f.Width != 400 {
		t.Fatalf("files: %+v, %v", f, err)
	}
	if _, err = c.Files(ctx, "../a1"); err == nil {
		t.Fatal("odd IDs should be refused")
	}
	if _, err = Download(ctx, "https://evil.example/x.gif"); err == nil {
		t.Fatal("only GIPHY files should be downloaded")
	}
	if err = (&Client{Key: "k"}).get(ctx, "/limited", map[string][]string{}, nil); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("rate limit: %v", err)
	}
	if (&Client{}).Enabled() {
		t.Fatal("no key, no GIFs")
	}
	if _, err = (&Client{Key: "wrong"}).Search(ctx, "x", 0); err == nil || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("a bad key fails without showing the key: %v", err)
	}
}
