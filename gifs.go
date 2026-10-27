package main

import (
	"unified-inbox/internal/gifs"
)

// giphyKey is GIPHY's API key, stamped in at build time (see
// scripts/build.sh and release.yml) so it isn't in the source. Without
// one, the GIF button doesn't show.
var giphyKey string

var giphy = &gifs.Client{Key: giphyKey}

// GIFsEnabled reports whether this build can search for GIFs.
func (a *App) GIFsEnabled() bool { return giphy.Enabled() }

// SearchGIFs searches GIPHY, or lists trending GIFs for an empty query.
func (a *App) SearchGIFs(query string, offset int) ([]gifs.GIF, error) {
	return giphy.Search(a.ctx, query, offset)
}

// SendGIF sends a GIF from the picker: an MP4 to WhatsApp, which plays GIFs
// as looping videos, and the GIF itself everywhere else.
func (a *App) SendGIF(roomID, gifID string) error {
	files, err := giphy.Files(a.ctx, gifID)
	if err != nil {
		return err
	}
	network, err := a.svc.ChatNetwork(a.ctx, roomID)
	if err != nil {
		return err
	}
	if network == "whatsapp" && files.MP4 != "" {
		data, err := gifs.Download(a.ctx, files.MP4)
		if err != nil {
			return err
		}
		return a.svc.SendGIFVideo(a.ctx, roomID, data, files.Width, files.Height)
	}
	data, err := gifs.Download(a.ctx, files.GIF)
	if err != nil {
		return err
	}
	return a.svc.SendData(a.ctx, roomID, "GIF.gif", data)
}
