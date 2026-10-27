// Package gifs searches GIPHY for the GIF picker and fetches the chosen
// GIF to send. GIPHY is only contacted while the picker is open; it sees
// what you search for and your IP address, nothing from your chats.
package gifs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BaseURL is GIPHY's API. Tests point it at a local server.
var BaseURL = "https://api.giphy.com/v1/gifs"

// PageSize is how many GIFs a search or the trending list returns at once.
const PageSize = 24

// maxDownload caps a GIF fetched to send; networks reject bigger files.
const maxDownload = 15 << 20

var client = &http.Client{Timeout: 20 * time.Second}

// Client talks to GIPHY with an API key. A zero Client (no key) has GIFs
// turned off.
type Client struct {
	Key string
}

// Enabled reports whether there's a key to search with.
func (c *Client) Enabled() bool { return c != nil && c.Key != "" }

// GIF is one search result, with a small preview for the picker.
type GIF struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Preview       string `json:"preview"`
	PreviewWidth  int    `json:"previewWidth"`
	PreviewHeight int    `json:"previewHeight"`
}

// Files are the versions of a GIF that get sent: the GIF itself and an MP4
// (for WhatsApp, which plays GIFs as looping videos).
type Files struct {
	Title  string
	GIF    string
	MP4    string
	Width  int
	Height int
}

type rendition struct {
	URL    string `json:"url"`
	MP4    string `json:"mp4"`
	WebP   string `json:"webp"`
	Width  string `json:"width"`
	Height string `json:"height"`
	Size   string `json:"size"`
}

type result struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Images struct {
		FixedWidth rendition `json:"fixed_width"`
		Downsized  rendition `json:"downsized"`
		Original   rendition `json:"original"`
	} `json:"images"`
}

// Trending lists popular GIFs, for when nothing has been searched yet.
func (c *Client) Trending(ctx context.Context, offset int) ([]GIF, error) {
	return c.list(ctx, "/trending", url.Values{"offset": {strconv.Itoa(offset)}})
}

// Search finds GIFs matching a query.
func (c *Client) Search(ctx context.Context, query string, offset int) ([]GIF, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return c.Trending(ctx, offset)
	}
	return c.list(ctx, "/search", url.Values{"q": {query}, "offset": {strconv.Itoa(offset)}, "lang": {"en"}})
}

func (c *Client) list(ctx context.Context, path string, params url.Values) ([]GIF, error) {
	params.Set("limit", strconv.Itoa(PageSize))
	var body struct {
		Data []result `json:"data"`
	}
	if err := c.get(ctx, path, params, &body); err != nil {
		return nil, err
	}
	out := make([]GIF, 0, len(body.Data))
	for _, r := range body.Data {
		preview := r.Images.FixedWidth
		src := preview.WebP
		if src == "" {
			src = preview.URL
		}
		if r.ID == "" || !isGiphyURL(src) {
			continue
		}
		out = append(out, GIF{
			ID: r.ID, Title: r.Title, Preview: src,
			PreviewWidth: atoi(preview.Width), PreviewHeight: atoi(preview.Height),
		})
	}
	return out, nil
}

// Files looks up a GIF's files to send.
func (c *Client) Files(ctx context.Context, id string) (*Files, error) {
	if id == "" || strings.ContainsAny(id, "/?#") {
		return nil, errors.New("not a GIPHY GIF")
	}
	var body struct {
		Data result `json:"data"`
	}
	if err := c.get(ctx, "/"+url.PathEscape(id), url.Values{}, &body); err != nil {
		return nil, err
	}
	img := body.Data.Images
	// The downsized GIF stays under 2 MB; the original can be far bigger.
	gif := img.Downsized
	if gif.URL == "" {
		gif = img.Original
	}
	f := &Files{Title: body.Data.Title, GIF: gif.URL, MP4: img.Original.MP4, Width: atoi(gif.Width), Height: atoi(gif.Height)}
	if !isGiphyURL(f.GIF) {
		return nil, errors.New("GIPHY didn't return the GIF")
	}
	if !isGiphyURL(f.MP4) {
		f.MP4 = ""
	}
	return f, nil
}

// Download fetches one of a GIF's files.
func Download(ctx context.Context, fileURL string) ([]byte, error) {
	if !isGiphyURL(fileURL) {
		return nil, errors.New("not a GIPHY file")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("couldn't get the GIF: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, err
	} else if len(data) > maxDownload {
		return nil, errors.New("that GIF is too big to send")
	}
	return data, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	if !c.Enabled() {
		return errors.New("GIFs aren't set up in this build")
	}
	params.Set("api_key", c.Key)
	params.Set("rating", "pg-13")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, BaseURL+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		// The request URL holds the key; don't pass it on in the error.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("couldn't reach GIPHY: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return errors.New("GIPHY's limit for this hour has been reached; try again later")
	default:
		return fmt.Errorf("GIPHY returned %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

// isGiphyURL accepts only HTTPS links on GIPHY's own servers, so nothing
// else is ever fetched through here.
func isGiphyURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "giphy.com" || strings.HasSuffix(host, ".giphy.com")
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
