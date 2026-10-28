import {Fragment} from 'react';
import {BrowserOpenURL} from './api';

export type Segment = {text: string; url?: string};

// A Markdown-style link, [label](url), which bridges write when a message's
// formatted version links some text (Instagram reels arrive this way), or a
// bare http(s) link.
const LINK_RE = /\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)|https?:\/\/[^\s<>"]+/g;
// Punctuation that usually ends a sentence rather than a bare link.
const TRAILING = /[.,!?;:'")\]]+$/;
const MAX_LABEL = 48;

// splitLinks breaks message text into plain text and links, each link with
// the label to show for it.
export function splitLinks(text: string): Segment[] {
    const out: Segment[] = [];
    let last = 0;
    for (const m of text.matchAll(LINK_RE)) {
        let start = m.index!, end = start + m[0].length;
        let url: string, label: string;
        if (m[2]) {
            url = m[2];
            label = m[1];
        } else {
            url = m[0];
            const trail = url.match(TRAILING)?.[0] ?? '';
            // Keep a closing bracket that belongs to the URL, as in Wikipedia
            // links like .../Foo_(bar).
            const keep = trail.startsWith(')') && url.includes('(') ? 1 : 0;
            url = url.slice(0, url.length - trail.length + keep);
            end = start + url.length;
            label = url;
        }
        if (start > last) out.push({text: text.slice(last, start)});
        out.push({text: displayURL(label), url});
        last = end;
    }
    if (last < text.length) out.push({text: text.slice(last)});
    return out;
}

// Query parameters that only track where a link was shared from.
const TRACKING_PARAM = /^(utm_.*|igsh|igshid|si|fbclid|gclid|is_sponsored|is_ineligible_for_clips_chaining)$/i;

// displayURL shortens a link for display: no scheme, "www.", fragment,
// tracking parameters or trailing slash, and an ellipsis past MAX_LABEL
// characters. The rest of the query is dropped only when the path already
// names the page (instagram.com/reel/ID), not when it is what says which
// page (youtube.com/watch?v=ID). Labels that aren't links are left alone.
export function displayURL(label: string): string {
    let u: URL;
    try {
        u = new URL(label);
    } catch {
        return label;
    }
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return label;
    const path = u.pathname.replace(/\/+$/, '');
    for (const key of [...u.searchParams.keys()]) {
        if (TRACKING_PARAM.test(key)) u.searchParams.delete(key);
    }
    const query = path.split('/').filter(Boolean).length >= 2 ? '' : u.searchParams.toString();
    const s = u.host.replace(/^www\./i, '') + path + (query ? '?' + query : '');
    return s.length > MAX_LABEL ? s.slice(0, MAX_LABEL - 1) + '…' : s;
}

// typingText says who is typing: just "typing…" in a one-to-one chat.
export function typingText(names: string[], isDm: boolean): string {
    if (isDm || names.length === 0) return 'typing…';
    if (names.length === 1) return `${names[0]} is typing…`;
    if (names.length === 2) return `${names[0]} and ${names[1]} are typing…`;
    return 'Several people are typing…';
}

// Links open in the default browser. A plain target="_blank" link does
// nothing in the Mac webview and opens a bare Edge window on Windows.
export function openLink(url: string) {
    BrowserOpenURL(url);
}

export function linkify(text: string, highlight = '') {
    return splitLinks(text).map((seg, i) => seg.url
        ? <a key={i} href={seg.url} title={seg.url} onClick={e => {
            e.preventDefault();
            openLink(seg.url!);
        }}>{highlightText(seg.text, highlight)}</a>
        : <Fragment key={i}>{highlightText(seg.text, highlight)}</Fragment>);
}

// highlightText marks each occurrence of query in text, ignoring case.
export function highlightText(text: string, query: string) {
    const needle = query.toLowerCase();
    if (!needle) return text;
    const lower = text.toLowerCase();
    const parts = [];
    let from = 0;
    for (let at = lower.indexOf(needle); at >= 0; at = lower.indexOf(needle, from)) {
        if (at > from) parts.push(text.slice(from, at));
        parts.push(<mark key={at}>{text.slice(at, at + needle.length)}</mark>);
        from = at + needle.length;
    }
    if (parts.length === 0) return text;
    if (from < text.length) parts.push(text.slice(from));
    return <>{parts}</>;
}
