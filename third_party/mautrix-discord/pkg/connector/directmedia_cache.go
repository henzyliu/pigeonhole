// mautrix-discord - A Matrix-Discord puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package connector

import (
	"sync"
	"time"

	"go.mau.fi/mautrix-discord/pkg/discordid"
)

const attachmentCacheLife = 5 * time.Minute

type attachmentCacheEntry struct {
	Expiry time.Time
	URL    string
}

func (ce *attachmentCacheEntry) IsExpired() bool {
	return time.Until(ce.Expiry) <= attachmentCacheLife
}

// attachmentCache tracks expiring attachment URLs from Discord. An
// attachmentCache is safe for concurrent use by multiple goroutines.
type attachmentCache struct {
	sync.RWMutex
	cache map[discordid.MediaInfoV1]attachmentCacheEntry
}

// TODO(skip): The cache grows in an unbounded fashion.

func NewAttachmentCache() *attachmentCache {
	return &attachmentCache{
		cache: make(map[discordid.MediaInfoV1]attachmentCacheEntry),
	}
}

func (ac *attachmentCache) Get(key discordid.MediaInfoV1) (*attachmentCacheEntry, bool) {
	ac.Lock()
	defer ac.Unlock()

	cached, ok := ac.cache[key]
	if !ok {
		return nil, false
	}

	if cached.IsExpired() {
		delete(ac.cache, key)
		return nil, false
	}

	return &cached, true
}

func (ac *attachmentCache) Insert(info *discordid.MediaInfo, url string) {
	if url == "" {
		return
	}

	expiry := normalizeAttachmentExpiry(parseAttachmentExpiryFromURL(url))

	ac.Lock()
	defer ac.Unlock()

	key := info.MediaInfoV1
	entry := attachmentCacheEntry{
		URL:    url,
		Expiry: expiry,
	}

	if expiry.IsZero() || entry.IsExpired() {
		delete(ac.cache, key)
		return
	}

	ac.cache[key] = entry
}
