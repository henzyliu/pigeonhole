import {UIEvent, useEffect, useRef, useState} from 'react';
import {errorText, formatTime, Go, inbox} from './api';
import {displayURL, openLink, splitLinks} from './links';
import MediaViewer from './MediaViewer';
import {useCloseOnEscape} from './shortcuts';

type Kind = 'media' | 'files' | 'links';

const TABS: {id: Kind; label: string}[] = [
    {id: 'media', label: 'Media'},
    {id: 'files', label: 'Files'},
    {id: 'links', label: 'Links'},
];

// Must match SharedPageSize in Go.
const PAGE = 60;

// The Shared panel: everything exchanged in a chat, by kind, newest first.
// Photos open in the viewer; files open in their app; links in the
// browser, and anything can be shown where it was sent.
export default function SharedPanel(props: {chat: inbox.Chat; onShowInChat: (eventId: string) => void; onClose: () => void}) {
    const [kind, setKind] = useState<Kind>('media');
    const [items, setItems] = useState<inbox.Message[] | null>(null);
    const [hasMore, setHasMore] = useState(false);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [viewing, setViewing] = useState(-1);
    const loadRef = useRef(0);
    useCloseOnEscape(viewing < 0 ? props.onClose : () => {});

    const load = async (offset: number) => {
        const load = offset === 0 ? ++loadRef.current : loadRef.current;
        setLoading(true);
        setError('');
        try {
            const page = await Go.ChatShared(props.chat.roomId, kind, offset) ?? [];
            if (load !== loadRef.current) return;
            setItems(prev => offset === 0 || !prev ? page : [...prev, ...page]);
            setHasMore(page.length >= PAGE);
        } catch (e) {
            if (load === loadRef.current) setError(errorText(e));
        } finally {
            if (load === loadRef.current) setLoading(false);
        }
    };

    useEffect(() => {
        setItems(null);
        load(0);
    }, [kind, props.chat.roomId]);

    const onScroll = (e: UIEvent<HTMLDivElement>) => {
        const el = e.currentTarget;
        if (!loading && hasMore && el.scrollHeight - el.scrollTop - el.clientHeight < 300) load(items?.length ?? 0);
    };

    const media = kind === 'media' ? (items ?? []).filter(m => m.mediaUrl) : [];
    const empty = {media: 'No photos or videos yet.', files: 'No files yet.', links: 'No links yet.'}[kind];

    return (
        <aside className="shared-panel" aria-label="Shared in this chat">
            <div className="shared-header">
                <h2>Shared</h2>
                <button className="icon-button" title="Close (Esc)" aria-label="Close" onClick={props.onClose}>✕</button>
            </div>
            <div className="tabs" role="tablist">
                {TABS.map(t => (
                    <button key={t.id} role="tab" aria-selected={kind === t.id}
                            className={`tab ${kind === t.id ? 'active' : ''}`} onClick={() => setKind(t.id)}>
                        <span className="tab-label">{t.label}</span>
                    </button>
                ))}
            </div>
            {error && <div className="banner error">{error}</div>}
            <div className="shared-body" onScroll={onScroll}>
                {items === null && !error && <p className="muted shared-note">Loading…</p>}
                {items?.length === 0 && <p className="muted shared-note">{empty}</p>}

                {kind === 'media' && media.length > 0 && (
                    <div className="shared-grid">
                        {media.map((m, i) => (
                            <button key={m.eventId} className="shared-tile" title={`${m.isMe ? 'You' : m.senderName}, ${formatTime(m.ts)}`}
                                    onClick={() => setViewing(i)}>
                                {m.kind === 'video'
                                    ? (m.thumbnail
                                        ? <img src={m.thumbnail} alt="" loading="lazy"/>
                                        : <video src={m.mediaUrl} preload="metadata" muted/>)
                                    : <img src={m.mediaUrl} alt={m.fileName} loading="lazy"/>}
                                {m.kind === 'video' && <span className="shared-play" aria-hidden="true">▶</span>}
                            </button>
                        ))}
                    </div>
                )}

                {kind === 'files' && items?.map(m => (
                    <div key={m.eventId} className="shared-row">
                        <button className="shared-row-main" onClick={() => Go.OpenMedia(m.mediaUrl, m.fileName).catch(e => setError(errorText(e)))}>
                            <span className="shared-icon" aria-hidden="true">{m.kind === 'audio' ? '🎤' : '📎'}</span>
                            <span className="shared-text">
                                <span className="shared-title">{m.kind === 'audio' && !m.fileName ? 'Voice message' : m.fileName || 'File'}</span>
                                <span className="shared-meta">{m.isMe ? 'You' : m.senderName} · {formatTime(m.ts)}</span>
                            </span>
                        </button>
                        <button className="link shared-jump" onClick={() => props.onShowInChat(m.eventId)}>Show in chat</button>
                    </div>
                ))}

                {kind === 'links' && items?.flatMap(m => splitLinks(m.body).filter(s => s.url).map((s, i) => (
                    <div key={m.eventId + i} className="shared-row">
                        <button className="shared-row-main" title={s.url} onClick={() => openLink(s.url!)}>
                            <span className="shared-icon" aria-hidden="true">🔗</span>
                            <span className="shared-text">
                                <span className="shared-title">{m.previews?.find(p => p.url === s.url)?.title || displayURL(s.url!)}</span>
                                <span className="shared-meta">{m.isMe ? 'You' : m.senderName} · {formatTime(m.ts)}</span>
                            </span>
                        </button>
                        <button className="link shared-jump" onClick={() => props.onShowInChat(m.eventId)}>Show in chat</button>
                    </div>
                )))}

                {loading && items !== null && <p className="muted shared-note">Loading…</p>}
            </div>
            {viewing >= 0 && media[viewing] && (
                <MediaViewer items={media} index={viewing} onIndex={setViewing} onClose={() => setViewing(-1)}/>
            )}
        </aside>
    );
}
