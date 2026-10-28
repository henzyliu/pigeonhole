import {Fragment, useEffect, useRef, useState} from 'react';
import {errorText, formatTime, Go, inbox} from './api';
import Avatar from './Avatar';

// Messages matching the search box, newest first, under the matching
// chats. Clicking one opens its chat at that message.
export default function MessageResults(props: {query: string; onOpen: (r: inbox.SearchResult) => void}) {
    const [results, setResults] = useState<inbox.SearchResult[] | null>(null);
    const [hasMore, setHasMore] = useState(false);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    // Only the newest search's answer is kept, while typing.
    const searchRef = useRef(0);
    const q = props.query.trim();

    const load = async (offset: number) => {
        const search = offset === 0 ? ++searchRef.current : searchRef.current;
        setLoading(true);
        setError('');
        try {
            const page = await Go.SearchMessages(q, offset) ?? [];
            if (search !== searchRef.current) return;
            setResults(prev => offset === 0 || !prev ? page : [...prev, ...page]);
            setHasMore(page.length >= 40);
        } catch (e) {
            if (search === searchRef.current) setError(errorText(e));
        } finally {
            if (search === searchRef.current) setLoading(false);
        }
    };

    useEffect(() => {
        setResults(null);
        if ([...q].length < 2) return;
        const timer = window.setTimeout(() => load(0), 250);
        return () => window.clearTimeout(timer);
    }, [q]);

    if ([...q].length < 2) return null;
    return (
        <div className="message-results">
            <div className="results-heading">Messages</div>
            {error && <div className="banner error">{error}</div>}
            {results === null && !error && <p className="muted results-note">Searching…</p>}
            {results?.length === 0 && <p className="muted results-note">No messages found.</p>}
            <ul className="chat-list">
                {results?.map(r => (
                    <li key={r.eventId} className="chat-row result-row" onClick={() => props.onOpen(r)}>
                        <Avatar name={r.chatName} src={r.chatAvatar} size={36} network={r.network}/>
                        <div className="chat-row-body">
                            <div className="chat-row-top">
                                <span className="chat-name">{r.chatName || 'Unnamed chat'}</span>
                                <span className="chat-time">{formatTime(r.ts)}</span>
                            </div>
                            <div className="result-text">
                                {(r.isMe || !r.isDm) && <span className="result-sender">{r.isMe ? 'You' : r.senderName}: </span>}
                                <Highlight text={snippet(r.body, q)} query={q}/>
                            </div>
                        </div>
                    </li>
                ))}
            </ul>
            {hasMore && (
                <button className="link more-results" disabled={loading} onClick={() => load(results?.length ?? 0)}>
                    {loading ? 'Loading…' : 'More results'}
                </button>
            )}
        </div>
    );
}

// snippet trims a long message to the part around the first match, so the
// match is visible in a one-line row.
function snippet(text: string, query: string): string {
    const flat = text.replace(/\s+/g, ' ');
    const at = flat.toLowerCase().indexOf(query.toLowerCase());
    if (at <= 30) return flat;
    return '…' + flat.slice(at - 20);
}

// Highlight marks each occurrence of query in text (ignoring case).
function Highlight({text, query}: {text: string; query: string}) {
    const lower = text.toLowerCase();
    const needle = query.toLowerCase();
    const parts: {text: string; match: boolean}[] = [];
    let from = 0;
    for (let at = lower.indexOf(needle); at >= 0 && needle; at = lower.indexOf(needle, from)) {
        if (at > from) parts.push({text: text.slice(from, at), match: false});
        parts.push({text: text.slice(at, at + needle.length), match: true});
        from = at + needle.length;
    }
    if (from < text.length) parts.push({text: text.slice(from), match: false});
    return <>{parts.map((p, i) => p.match ? <mark key={i}>{p.text}</mark> : <Fragment key={i}>{p.text}</Fragment>)}</>;
}
