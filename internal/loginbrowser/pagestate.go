package loginbrowser

import (
	"encoding/json"
	"fmt"
)

// The Windows window (WebView2) can't await a script's promise the way
// WebKit's callAsyncJavaScript does, so pages report values by posting
// messages instead: {"kind": "storage" | "extracted", "value": {...}}.
// Building those scripts and reading the messages is plain Go, kept here so
// it can be tested on any platform.

// pageState collects what a login page has reported so far.
type pageState struct {
	storage   map[string]string
	extracted map[string]any
}

type pageMessage struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

// apply records one message posted by extractScript or storageScript.
func (p *pageState) apply(message string) error {
	var msg pageMessage
	if err := json.Unmarshal([]byte(message), &msg); err != nil {
		return err
	}
	switch msg.Kind {
	case "storage":
		var storage map[string]string
		if err := json.Unmarshal(msg.Value, &storage); err != nil {
			return err
		}
		p.storage = storage
	case "extracted":
		var extracted map[string]any
		if err := json.Unmarshal(msg.Value, &extracted); err != nil {
			return err
		}
		p.extracted = extracted
	default:
		return fmt.Errorf("unknown page message %q", msg.Kind)
	}
	return nil
}

func (p *pageState) snapshot(url string, cookies []Cookie) *Snapshot {
	return &Snapshot{URL: url, Cookies: cookies, LocalStorage: p.storage, Extracted: p.extracted}
}

// extractScript wraps the bridge's ExtractJS (an expression that resolves to
// the field values) so it runs once per page, in the top frame only, after
// the document exists, and posts its result. It's added as a document-created
// script, so a full page load runs it again; in-page navigation keeps it
// running.
func extractScript(extractJS string) string {
	return `(() => {
	if (window.top !== window || window.__pigeonholeExtract) return;
	window.__pigeonholeExtract = true;
	const run = async () => {
		try {
			const value = await (` + extractJS + `);
			window.chrome.webview.postMessage(JSON.stringify({kind: "extracted", value}));
		} catch (e) {}
	};
	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", run, {once: true});
	} else {
		run();
	}
})();`
}

// storageScript posts the requested localStorage values.
func storageScript(keys []string) string {
	keysJSON, _ := json.Marshal(keys)
	return `(() => {
	const value = {};
	try {
		for (const k of ` + string(keysJSON) + `) {
			const v = localStorage.getItem(k);
			if (v !== null) value[k] = v;
		}
	} catch (e) {}
	window.chrome.webview.postMessage(JSON.stringify({kind: "storage", value}));
})();`
}
