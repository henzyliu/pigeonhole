import {Fragment, KeyboardEvent, useCallback, useEffect, useLayoutEffect, useRef, useState} from 'react';
import {BrowserOpenURL, errorText, Go, inbox, NETWORK_LABELS, OnFileDrop, OnFileDropOff} from './api';
import Avatar from './Avatar';
import NetworkIcon from './NetworkIcon';

const PAGE = 50;
const QUICK_REACTIONS = ['❤️', '😂', '😮', '😢', '👍'];

export default function Thread(props: {chat: inbox.Chat; tick: number; onSetHidden: (hidden: boolean) => Promise<void>}) {
    const {chat} = props;
    const [messages, setMessages] = useState<inbox.Message[]>([]);
    const [draft, setDraft] = useState('');
    const [replyTo, setReplyTo] = useState<inbox.Message | null>(null);
    const [error, setError] = useState('');
    const [hasMore, setHasMore] = useState(true);
    const scroller = useRef<HTMLDivElement>(null);
    const composer = useRef<HTMLTextAreaElement>(null);
    const stickToBottom = useRef(true);
    const restoreFrom = useRef<number | null>(null);
    const messagesRef = useRef(messages);
    messagesRef.current = messages;
    const hasMoreRef = useRef(hasMore);
    hasMoreRef.current = hasMore;
    const loadedOnce = useRef(false);

    // Reload the newest page, keeping any older pages already loaded.
    const refresh = useCallback(async () => {
        if (!hasMoreRef.current) {
            // Everything stored is already showing, so reload all of it (and
            // a page more): older messages the bridge has just fetched then
            // appear at the top, without moving what you're looking at.
            const loaded = messagesRef.current.length;
            const all = await Go.Messages(chat.roomId, 0, loaded + PAGE) ?? [];
            const el = scroller.current;
            if (el && !stickToBottom.current && all.length > loaded) {
                restoreFrom.current = el.scrollHeight - el.scrollTop;
            }
            setMessages(all);
            if (all.length >= loaded + PAGE) setHasMore(true);
            return;
        }
        const latest = await Go.Messages(chat.roomId, 0, PAGE) ?? [];
        setMessages(prev => {
            if (prev.length === 0 || latest.length === 0) return latest;
            const cutoff = latest[0].ts;
            return [...prev.filter(m => m.ts < cutoff), ...latest];
        });
        if (latest.length < PAGE) setHasMore(false);
    }, [chat.roomId]);

    // Opening a chat loads at once; later changes (which arrive in bursts
    // while history loads) are coalesced.
    useEffect(() => {
        const timer = window.setTimeout(() => {
            loadedOnce.current = true;
            refresh().catch(e => setError(errorText(e)));
            Go.MarkRead(chat.roomId).catch(console.error);
        }, loadedOnce.current ? 200 : 0);
        return () => window.clearTimeout(timer);
    }, [refresh, props.tick, chat.roomId]);

    // Files dropped anywhere on the window go to the open conversation.
    useEffect(() => {
        OnFileDrop((_x, _y, paths) => {
            if (!paths?.length) return;
            setError('');
            stickToBottom.current = true;
            Go.SendFiles(chat.roomId, paths).catch(e => setError(errorText(e)));
        }, false);
        return () => OnFileDropOff();
    }, [chat.roomId]);

    const attach = () => {
        setError('');
        stickToBottom.current = true;
        Go.PickAndSendFiles(chat.roomId).catch(e => setError(errorText(e)));
    };

    useLayoutEffect(() => {
        const el = scroller.current;
        if (!el) return;
        if (restoreFrom.current !== null) {
            el.scrollTop = el.scrollHeight - restoreFrom.current;
            restoreFrom.current = null;
        } else if (stickToBottom.current) {
            el.scrollTop = el.scrollHeight;
        }
    }, [messages]);

    const loadOlder = async () => {
        if (!hasMore || messages.length === 0) return;
        const older = await Go.Messages(chat.roomId, messages[0].ts, PAGE) ?? [];
        if (older.length < PAGE) setHasMore(false);
        if (older.length) {
            restoreFrom.current = scroller.current!.scrollHeight - scroller.current!.scrollTop;
            setMessages(prev => [...older, ...prev]);
        }
    };

    const onScroll = () => {
        const el = scroller.current!;
        stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        if (el.scrollTop < 120) loadOlder().catch(console.error);
    };

    const send = async () => {
        const text = draft.trim();
        if (!text) return;
        setDraft('');
        setError('');
        stickToBottom.current = true;
        try {
            await Go.SendText(chat.roomId, text, replyTo?.eventId ?? '');
            setReplyTo(null);
        } catch (e) {
            setDraft(text);
            setError(errorText(e));
        }
    };

    const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
        if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
            e.preventDefault();
            send();
        }
    };

    // Replying jumps straight to the message box so you can start typing.
    const startReply = (msg: inbox.Message) => {
        setReplyTo(msg);
        composer.current?.focus();
    };

    const react = (msg: inbox.Message, key: string) =>
        Go.React(chat.roomId, msg.eventId, key).catch(e => setError(errorText(e)));

    const byId = new Map(messages.map(m => [m.eventId, m]));
    // Sent / Delivered only shows under your newest message, like Messenger.
    const latestMine = [...messages].reverse().find(m => m.isMe && !m.redacted)?.eventId;

    return (
        <section className="thread">
            <header className="thread-header">
                <Avatar name={chat.name} src={chat.avatar} size={36}/>
                <div>
                    <div className="thread-title">{chat.name}</div>
                    <div className="thread-subtitle"><NetworkIcon network={chat.network} size={13}/>{NETWORK_LABELS[chat.network] ?? chat.network}</div>
                </div>
                <button className="link thread-hide"
                        title={chat.hidden ? 'Move this chat back to your chat list' : 'Hide this chat until a new message arrives. Only in Pigeonhole.'}
                        onClick={() => props.onSetHidden(!chat.hidden).catch(e => setError(errorText(e)))}>
                    {chat.hidden ? 'Unhide' : 'Hide chat'}
                </button>
            </header>
            <div className="messages" ref={scroller} onScroll={onScroll}>
                {chat.historyLoading && !hasMore && (
                    <div className="history-loading">
                        Loading older messages from {NETWORK_LABELS[chat.network] ?? chat.network}…
                    </div>
                )}
                {messages.map((m, i) => {
                    const prev = messages[i - 1];
                    const newDay = !prev || new Date(prev.ts).toDateString() !== new Date(m.ts).toDateString();
                    const grouped = !newDay && prev && prev.isMe === m.isMe && prev.senderName === m.senderName && m.ts - prev.ts < 5 * 60_000;
                    return (
                        <Fragment key={m.eventId}>
                            {newDay && <div className="day-divider">{new Date(m.ts).toLocaleDateString([], {weekday: 'long', month: 'long', day: 'numeric'})}</div>}
                            <MessageRow msg={m} grouped={!!grouped} showSender={!chat.isDm}
                                        isLatestMine={m.eventId === latestMine}
                                        replied={m.replyTo ? byId.get(m.replyTo) : undefined}
                                        onReply={() => startReply(m)} onReact={key => react(m, key)}/>
                        </Fragment>
                    );
                })}
            </div>
            {error && <div className="banner error">{error}</div>}
            {replyTo && (
                <div className="reply-bar">
                    <div>
                        <div className="reply-label">Replying to {replyTo.isMe ? 'yourself' : replyTo.senderName}</div>
                        <div className="reply-snippet">{snippet(replyTo)}</div>
                    </div>
                    <button className="icon-button" onClick={() => setReplyTo(null)}>✕</button>
                </div>
            )}
            <div className="composer">
                <button className="icon-button attach" title="Send photos or files" onClick={attach}>
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                        <path d="M21 11.5l-8.6 8.6a5.5 5.5 0 01-7.8-7.8l8.6-8.6a3.7 3.7 0 015.2 5.2l-8.6 8.6a1.8 1.8 0 01-2.6-2.6l7.9-7.9"/>
                    </svg>
                </button>
                <textarea ref={composer} rows={1} placeholder={`Message ${chat.name}`} value={draft}
                          onChange={e => setDraft(e.target.value)} onKeyDown={onKeyDown} autoFocus/>
                <button className="primary" onClick={send} disabled={!draft.trim()}>Send</button>
            </div>
        </section>
    );
}

function snippet(m: inbox.Message): string {
    if (m.redacted) return 'Message deleted';
    if (m.body) return m.body;
    return {image: 'Photo', video: 'Video', audio: 'Voice message', file: m.fileName}[m.kind] ?? '';
}

function MessageRow(props: {
    msg: inbox.Message;
    grouped: boolean;
    showSender: boolean;
    isLatestMine: boolean;
    replied?: inbox.Message;
    onReply: () => void;
    onReact: (key: string) => void;
}) {
    const {msg} = props;
    const [picking, setPicking] = useState(false);
    const actionsRef = useRef<HTMLDivElement>(null);

    // Once opened, the emoji bar stays until you pick one, click elsewhere or
    // press Escape. Closing on mouse-out made it vanish while moving to it.
    useEffect(() => {
        if (!picking) return;
        const onMouseDown = (e: MouseEvent) => {
            if (!actionsRef.current?.contains(e.target as Node)) setPicking(false);
        };
        const onKeyDown = (e: globalThis.KeyboardEvent) => {
            if (e.key === 'Escape') setPicking(false);
        };
        document.addEventListener('mousedown', onMouseDown);
        document.addEventListener('keydown', onKeyDown);
        return () => {
            document.removeEventListener('mousedown', onMouseDown);
            document.removeEventListener('keydown', onKeyDown);
        };
    }, [picking]);
    // The toolbar is always rendered but floats beside the bubble and only
    // fades in after a short hover (see .msg-actions), so the layout never
    // shifts and quick scrolling doesn't flash it.
    return (
        <div className={`msg-row ${msg.isMe ? 'me' : 'them'} ${props.grouped ? 'grouped' : ''} ${picking ? 'picking' : ''}`}>
            {!msg.isMe && <div className="msg-avatar">{!props.grouped && <Avatar name={msg.senderName} src={msg.senderAvatar} size={28}/>}</div>}
            <div className="msg-col">
                {props.showSender && !msg.isMe && !props.grouped && <div className="msg-sender">{msg.senderName}</div>}
                {msg.replyTo && (
                    <div className="msg-quote">{props.replied ? snippet(props.replied) : 'Original message'}</div>
                )}
                <div className="msg-line">
                    <div className={`bubble ${msg.kind} ${msg.redacted ? 'redacted' : ''}`} title={new Date(msg.ts).toLocaleString()}>
                        <MessageBody msg={msg}/>
                    </div>
                    {!msg.redacted && (
                        <div className="msg-actions" role="toolbar" ref={actionsRef}>
                            <button title="React" aria-label="React" onClick={() => setPicking(p => !p)}>
                                <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
                                     strokeLinecap="round" strokeLinejoin="round">
                                    <circle cx="12" cy="12" r="9"/>
                                    <path d="M8.5 14.5a4.5 4.5 0 0 0 7 0"/>
                                    <path d="M9 9.5h.01M15 9.5h.01"/>
                                </svg>
                            </button>
                            <button title="Reply" aria-label="Reply" onClick={props.onReply}>
                                <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
                                     strokeLinecap="round" strokeLinejoin="round">
                                    <path d="M9 14 4 9l5-5"/>
                                    <path d="M4 9h10.5a5.5 5.5 0 0 1 0 11H11"/>
                                </svg>
                            </button>
                            {picking && (
                                <div className="reaction-picker">
                                    {QUICK_REACTIONS.map(r => (
                                        <button key={r} onClick={() => {
                                            props.onReact(r);
                                            setPicking(false);
                                        }}>{r}</button>
                                    ))}
                                </div>
                            )}
                        </div>
                    )}
                </div>
                {msg.reactions?.length > 0 && (
                    <div className="reactions">
                        {msg.reactions.map(r => (
                            <span key={r.key} className={`reaction ${r.mine ? 'mine' : ''}`}>{r.key}{r.count > 1 && ` ${r.count}`}</span>
                        ))}
                    </div>
                )}
                <MessageStatus msg={msg} isDm={!props.showSender} isLatestMine={props.isLatestMine}/>
            </div>
        </div>
    );
}

const MAX_SEEN_AVATARS = 5;

// What shows under a message, in priority order: a send problem, who has seen
// it, then (on your newest message only) Sent / Delivered.
function MessageStatus(props: {msg: inbox.Message; isDm: boolean; isLatestMine: boolean}) {
    const {msg} = props;
    if (msg.isMe && (msg.status === 'pending' || msg.status === 'fail' || msg.status === 'retriable')) {
        return <div className={`msg-status ${msg.status}`}>{msg.status === 'pending' ? 'Sending…' : 'Failed to send'}</div>;
    }
    if (msg.seenBy?.length) {
        return <SeenBy readers={msg.seenBy} isDm={props.isDm} isMe={msg.isMe}/>;
    }
    if (props.isLatestMine && msg.status === 'delivered') {
        return <div className="msg-status">Delivered</div>;
    }
    if (props.isLatestMine && msg.status === 'success') {
        return <div className="msg-status">Sent</div>;
    }
    return null;
}

// Messenger-style read receipts: "Seen" under your message in a one-to-one
// chat, and small avatars of who has read this far in a group.
function SeenBy(props: {readers: inbox.Reader[]; isDm: boolean; isMe: boolean}) {
    const {readers} = props;
    if (readers.length === 0) return null;
    if (props.isDm) {
        return props.isMe ? <div className="msg-status seen">Seen</div> : null;
    }
    const names = readers.map(r => r.name).join(', ');
    return (
        <div className="seen-by" title={`Seen by ${names}`}>
            {readers.slice(0, MAX_SEEN_AVATARS).map((r, i) => <Avatar key={i} name={r.name} src={r.avatar} size={16}/>)}
            {readers.length > MAX_SEEN_AVATARS && <span className="seen-more">+{readers.length - MAX_SEEN_AVATARS}</span>}
        </div>
    );
}

function MessageBody({msg}: {msg: inbox.Message}) {
    if (msg.redacted) return <em>Message deleted</em>;
    const caption = msg.body ? <div className="caption">{linkify(msg.body)}</div> : null;
    switch (msg.kind) {
        case 'image':
            return <>
                <img className="media" src={msg.mediaUrl} alt={msg.fileName} loading="lazy"
                     style={msg.width && msg.height ? {aspectRatio: `${msg.width} / ${msg.height}`} : undefined}/>
                {caption}
            </>;
        case 'video':
            return <><video className="media" src={msg.mediaUrl} controls preload="metadata"/>{caption}</>;
        case 'audio':
            return <audio src={msg.mediaUrl} controls preload="metadata"/>;
        case 'file':
            return <><a href={msg.mediaUrl} download={msg.fileName}>📎 {msg.fileName}</a>{caption}</>;
        case 'emote':
            return <em>{linkify(msg.body)}</em>;
        default:
            return <>{linkify(msg.body)}{msg.edited && <span className="edited"> (edited)</span>}</>;
    }
}

const URL_RE = /(https?:\/\/[^\s]+)/g;

// Links open in the default browser. A plain target="_blank" link does
// nothing in the Mac webview and opens a bare Edge window on Windows.
function linkify(text: string) {
    return text.split(URL_RE).map((part, i) =>
        i % 2 === 1 ? <a key={i} href={part} onClick={e => {
            e.preventDefault();
            BrowserOpenURL(part);
        }}>{part}</a> : part);
}
