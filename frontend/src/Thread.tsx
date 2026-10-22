import {Fragment, KeyboardEvent, useCallback, useEffect, useLayoutEffect, useRef, useState} from 'react';
import {errorText, Go, inbox, NETWORK_LABELS, OnFileDrop, OnFileDropOff} from './api';
import Avatar from './Avatar';
import NetworkIcon from './NetworkIcon';

const PAGE = 50;
const QUICK_REACTIONS = ['❤️', '😂', '😮', '😢', '👍'];

export default function Thread(props: {chat: inbox.Chat; tick: number}) {
    const {chat} = props;
    const [messages, setMessages] = useState<inbox.Message[]>([]);
    const [draft, setDraft] = useState('');
    const [replyTo, setReplyTo] = useState<inbox.Message | null>(null);
    const [error, setError] = useState('');
    const [hasMore, setHasMore] = useState(true);
    const scroller = useRef<HTMLDivElement>(null);
    const stickToBottom = useRef(true);
    const restoreFrom = useRef<number | null>(null);

    // Reload the newest page, keeping any older pages already loaded.
    const refresh = useCallback(async () => {
        const latest = await Go.Messages(chat.roomId, 0, PAGE) ?? [];
        setMessages(prev => {
            if (prev.length === 0 || latest.length === 0) return latest;
            const cutoff = latest[0].ts;
            return [...prev.filter(m => m.ts < cutoff), ...latest];
        });
        if (latest.length < PAGE) setHasMore(false);
    }, [chat.roomId]);

    useEffect(() => {
        refresh().catch(e => setError(errorText(e)));
        Go.MarkRead(chat.roomId).catch(console.error);
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

    const react = (msg: inbox.Message, key: string) =>
        Go.React(chat.roomId, msg.eventId, key).catch(e => setError(errorText(e)));

    const byId = new Map(messages.map(m => [m.eventId, m]));

    return (
        <section className="thread">
            <header className="thread-header">
                <Avatar name={chat.name} src={chat.avatar} size={36}/>
                <div>
                    <div className="thread-title">{chat.name}</div>
                    <div className="thread-subtitle"><NetworkIcon network={chat.network} size={13}/>{NETWORK_LABELS[chat.network] ?? chat.network}</div>
                </div>
            </header>
            <div className="messages" ref={scroller} onScroll={onScroll}>
                {messages.map((m, i) => {
                    const prev = messages[i - 1];
                    const newDay = !prev || new Date(prev.ts).toDateString() !== new Date(m.ts).toDateString();
                    const grouped = !newDay && prev && prev.isMe === m.isMe && prev.senderName === m.senderName && m.ts - prev.ts < 5 * 60_000;
                    return (
                        <Fragment key={m.eventId}>
                            {newDay && <div className="day-divider">{new Date(m.ts).toLocaleDateString([], {weekday: 'long', month: 'long', day: 'numeric'})}</div>}
                            <MessageRow msg={m} grouped={!!grouped} showSender={!chat.isDm}
                                        replied={m.replyTo ? byId.get(m.replyTo) : undefined}
                                        onReply={() => setReplyTo(m)} onReact={key => react(m, key)}/>
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
                <textarea rows={1} placeholder={`Message ${chat.name}`} value={draft}
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
    replied?: inbox.Message;
    onReply: () => void;
    onReact: (key: string) => void;
}) {
    const {msg} = props;
    const [hover, setHover] = useState(false);
    return (
        <div className={`msg-row ${msg.isMe ? 'me' : 'them'} ${props.grouped ? 'grouped' : ''}`}
             onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}>
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
                    {hover && !msg.redacted && (
                        <div className="msg-actions">
                            {QUICK_REACTIONS.map(r => <button key={r} onClick={() => props.onReact(r)}>{r}</button>)}
                            <button onClick={props.onReply} title="Reply">↩</button>
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
                {msg.isMe && (msg.status === 'pending' || msg.status === 'fail' || msg.status === 'retriable') && (
                    <div className={`msg-status ${msg.status}`}>{msg.status === 'pending' ? 'Sending…' : 'Failed to send'}</div>
                )}
            </div>
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

function linkify(text: string) {
    return text.split(URL_RE).map((part, i) =>
        i % 2 === 1 ? <a key={i} href={part} target="_blank" rel="noreferrer">{part}</a> : part);
}
