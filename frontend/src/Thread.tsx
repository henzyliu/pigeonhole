import {ClipboardEvent, CSSProperties, Fragment, KeyboardEvent, useCallback, useEffect, useLayoutEffect, useRef, useState} from 'react';
import {errorText, gifs, Go, inbox, NETWORK_LABELS, OnFileDrop, OnFileDropOff} from './api';
import Avatar from './Avatar';
import {linkify, openLink, typingText} from './links';
import EmojiPicker from './EmojiPicker';
import GifPicker from './GifPicker';
import MediaViewer from './MediaViewer';
import SharedPanel from './SharedPanel';
import NetworkIcon from './NetworkIcon';
import {isMod, MOD} from './shortcuts';

const PAGE = 50;
const QUICK_REACTIONS = ['❤️', '😂', '😮', '😢', '👍'];

export default function Thread(props: {
    chat: inbox.Chat;
    tick: number;
    typing: string[];
    signedOut: boolean;
    onSignIn: () => void;
    onSetHidden: (hidden: boolean) => Promise<void>;
    // A message to open the chat at (a search result), instead of the end.
    jumpTo?: string;
}) {
    const {chat} = props;
    const [messages, setMessages] = useState<inbox.Message[]>([]);
    // The message to scroll to once loaded, and the one briefly highlighted.
    const scrollTarget = useRef(props.jumpTo ?? '');
    const [flashing, setFlashing] = useState('');
    const flash = (eventId: string) => {
        setFlashing(eventId);
        window.setTimeout(() => setFlashing(f => f === eventId ? '' : f), 2500);
    };
    // Find in this chat (⌘F): matching message IDs newest first, and which
    // one is showing.
    const [findOpen, setFindOpen] = useState(false);
    const [findQuery, setFindQuery] = useState('');
    const [matches, setMatches] = useState<string[]>([]);
    const [matchIndex, setMatchIndex] = useState(0);
    const findInput = useRef<HTMLInputElement>(null);
    // The Shared panel (photos, files and links in this chat).
    const [sharedOpen, setSharedOpen] = useState(false);
    // The message being typed starts from the chat's saved draft.
    const [draft, setDraft] = useState(chat.draft ?? '');
    const [replyTo, setReplyTo] = useState<inbox.Message | null>(null);
    // The draft's reply, until that message has loaded and can be shown.
    const pendingReply = useRef(chat.draftReply ?? '');
    // The GIF picker, shown above the message box; only offered when this
    // build has a GIPHY key.
    const [gifsEnabled, setGifsEnabled] = useState(false);
    const [pickingGif, setPickingGif] = useState(false);
    useEffect(() => {
        Go.GIFsEnabled().then(setGifsEnabled).catch(() => setGifsEnabled(false));
    }, []);
    const closeGifs = useCallback(() => setPickingGif(false), []);
    const sendGif = (gif: gifs.GIF) => {
        setPickingGif(false);
        setError('');
        stickToBottom.current = true;
        Go.SendGIF(chat.roomId, gif.id).catch(e => setError(errorText(e)));
        composer.current?.focus();
    };
    // The message being edited, which borrows the message box; the draft
    // that was there comes back afterwards.
    const [editing, setEditing] = useState<inbox.Message | null>(null);
    const draftBeforeEdit = useRef('');
    const [error, setError] = useState('');
    const [hasMore, setHasMore] = useState(true);
    // The photo or video open in the full-window viewer, by event ID.
    const [viewing, setViewing] = useState<string | null>(null);
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
            const first = !loadedOnce.current;
            loadedOnce.current = true;
            // Opened at a search result: load back as far as it, and scroll there.
            if (first && scrollTarget.current) {
                stickToBottom.current = false;
                Go.MessagesThrough(chat.roomId, scrollTarget.current).then(list => {
                    setMessages(list ?? []);
                    setHasMore(true);
                }).catch(e => {
                    scrollTarget.current = '';
                    setError(errorText(e));
                });
            } else {
                refresh().catch(e => setError(errorText(e)));
            }
            Go.MarkRead(chat.roomId).catch(console.error);
        }, loadedOnce.current ? 200 : 0);
        return () => window.clearTimeout(timer);
    }, [refresh, props.tick, chat.roomId]);

    // Restore the draft's reply once its message is loaded.
    useEffect(() => {
        if (!pendingReply.current || messages.length === 0) return;
        const msg = messages.find(m => m.eventId === pendingReply.current);
        if (msg) {
            pendingReply.current = '';
            setReplyTo(r => r ?? msg);
        }
    }, [messages]);

    // Save the draft shortly after typing stops, and straight away when
    // leaving the chat, so it's there next time (even after quitting).
    const saved = useRef({text: chat.draft ?? '', reply: chat.draftReply ?? ''});
    const unsaved = useRef<{text: string; reply: string} | null>(null);
    const saveDraft = useCallback(() => {
        const next = unsaved.current;
        unsaved.current = null;
        if (!next || (next.text === saved.current.text && next.reply === saved.current.reply)) return;
        saved.current = next;
        Go.SetDraft(chat.roomId, next.text, next.reply).catch(console.error);
    }, [chat.roomId]);
    useEffect(() => {
        if (editing) return;
        const text = draft.trim() ? draft : '';
        // A reply still waiting to load stays part of the draft.
        const reply = text ? (replyTo?.eventId ?? pendingReply.current) : '';
        unsaved.current = {text, reply};
        const timer = window.setTimeout(saveDraft, 400);
        return () => window.clearTimeout(timer);
    }, [draft, replyTo, saveDraft, editing]);
    useEffect(() => saveDraft, [saveDraft]);

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
        const target = scrollTarget.current && el.querySelector(`[data-event-id="${CSS.escape(scrollTarget.current)}"]`);
        if (target) {
            target.scrollIntoView({block: 'center'});
            flash(scrollTarget.current);
            scrollTarget.current = '';
            return;
        }
        if (restoreFrom.current !== null) {
            el.scrollTop = el.scrollHeight - restoreFrom.current;
            restoreFrom.current = null;
        } else if (stickToBottom.current) {
            el.scrollTop = el.scrollHeight;
        }
    }, [messages, props.typing]);

    // Scrolls to a message, loading older history back to it if needed.
    const goTo = (eventId: string) => {
        stickToBottom.current = false;
        const el = scroller.current?.querySelector(`[data-event-id="${CSS.escape(eventId)}"]`);
        if (el) {
            el.scrollIntoView({block: 'center'});
            flash(eventId);
            return;
        }
        scrollTarget.current = eventId;
        Go.MessagesThrough(chat.roomId, eventId).then(list => {
            setMessages(list ?? []);
            setHasMore(true);
        }).catch(e => setError(errorText(e)));
    };

    const openFind = () => {
        setFindOpen(true);
        window.setTimeout(() => {
            findInput.current?.focus();
            findInput.current?.select();
        });
    };
    const closeFind = () => {
        setFindOpen(false);
        setFindQuery('');
        setMatches([]);
        composer.current?.focus();
    };
    // Search as you type, going to the newest match.
    useEffect(() => {
        const q = findQuery.trim();
        if (!q) {
            setMatches([]);
            return;
        }
        let current = true;
        const timer = window.setTimeout(() => {
            Go.SearchChat(chat.roomId, q).then(ids => {
                if (!current) return;
                setMatches(ids ?? []);
                setMatchIndex(0);
                if (ids?.length) goTo(ids[0]);
            }).catch(e => setError(errorText(e)));
        }, 200);
        return () => {
            current = false;
            window.clearTimeout(timer);
        };
    }, [findQuery, chat.roomId]);
    // step moves to an older (+1) or newer (-1) match, wrapping around.
    const step = (dir: 1 | -1) => {
        if (matches.length === 0) return;
        const i = (matchIndex + dir + matches.length) % matches.length;
        setMatchIndex(i);
        goTo(matches[i]);
    };
    const onFindKey = (e: KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter' && !e.nativeEvent.isComposing) {
            e.preventDefault();
            step(e.shiftKey ? -1 : 1);
        } else if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
            e.preventDefault();
            step(e.key === 'ArrowUp' ? 1 : -1);
        } else if (e.key === 'Escape') {
            e.preventDefault();
            closeFind();
        }
    };
    // ⌘F / Ctrl+F opens it.
    useEffect(() => {
        const onKey = (e: globalThis.KeyboardEvent) => {
            if (isMod(e) && !e.shiftKey && e.key.toLowerCase() === 'f') {
                e.preventDefault();
                openFind();
            }
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, []);
    const findHighlight = findOpen ? findQuery.trim() : '';

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

    const startEdit = (msg: inbox.Message) => {
        if (!editing) draftBeforeEdit.current = draft;
        setEditing(msg);
        setDraft(msg.body);
        setError('');
        window.setTimeout(() => {
            const el = composer.current;
            el?.focus();
            el?.setSelectionRange(el.value.length, el.value.length);
        });
    };

    const stopEditing = () => {
        setEditing(null);
        setDraft(draftBeforeEdit.current);
    };

    // The message shows the new text at once, marked as saving, and only
    // changes for good once the network confirms it.
    const saveEdit = () => {
        const target = editing!;
        const text = draft.trim();
        if (!text) {
            setError('A message can\'t be edited to nothing. To remove it, unsend it instead.');
            return;
        }
        stopEditing();
        if (text === target.body.trim()) return;
        Go.EditMessage(chat.roomId, target.eventId, text).catch(e => setError(errorText(e)));
    };

    const unsend = (msg: inbox.Message) =>
        Go.UnsendMessage(chat.roomId, msg.eventId).catch(e => setError(errorText(e)));

    const send = async () => {
        if (editing) {
            saveEdit();
            return;
        }
        const text = draft.trim();
        if (!text) return;
        setDraft('');
        pendingReply.current = '';
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
        } else if (e.key === 'Escape' && editing) {
            e.preventDefault();
            stopEditing();
        } else if (e.key === 'ArrowUp' && !draft && !editing) {
            // Up in an empty box edits your last message, as in Slack.
            const last = [...messages].reverse().find(m => m.isMe && !m.redacted);
            if (last?.canEdit) {
                e.preventDefault();
                startEdit(last);
            }
        }
    };

    // Pasting a picture (or a copied file) sends it, like dropping it on
    // the window; pasted text goes into the box as usual.
    const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
        const files = [...e.clipboardData.items].filter(i => i.kind === 'file').map(i => i.getAsFile()).filter(f => f !== null);
        if (files.length === 0) return;
        e.preventDefault();
        setError('');
        stickToBottom.current = true;
        for (const file of files) {
            if (file.size > 25 * 1024 * 1024) {
                setError(`${file.name || 'The pasted file'} is larger than 25 MB`);
                continue;
            }
            const name = file.name && file.name !== 'image.png' ? file.name : `Pasted image.${file.type.split('/')[1] || 'png'}`;
            readBase64(file)
                .then(data => Go.SendPastedFile(chat.roomId, name, data))
                .catch(err => setError(errorText(err)));
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
    const media = messages.filter(m => (m.kind === 'image' || m.kind === 'video') && m.mediaUrl && !m.redacted);
    const viewingIndex = media.findIndex(m => m.eventId === viewing);
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
                <button className={`icon-button thread-shared ${sharedOpen ? 'active' : ''}`} title="Photos, files and links in this chat"
                        aria-label="Shared in this chat" aria-expanded={sharedOpen} onClick={() => setSharedOpen(o => !o)}>
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <rect x="3" y="4" width="18" height="16" rx="2"/>
                        <circle cx="9" cy="10" r="2"/>
                        <path d="m21 16-5-5-9 9"/>
                    </svg>
                </button>
                <button className="icon-button thread-find" title={`Search this chat (${MOD}F)`} aria-label="Search this chat"
                        onClick={() => findOpen ? closeFind() : openFind()}>
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                        <circle cx="11" cy="11" r="7"/>
                        <path d="m20 20-3.5-3.5"/>
                    </svg>
                </button>
                <button className="link thread-hide"
                        title={chat.hidden ? 'Move this chat back to your chat list' : 'Hide this chat until a new message arrives. Only in Pigeonhole.'}
                        onClick={() => props.onSetHidden(!chat.hidden).catch(e => setError(errorText(e)))}>
                    {chat.hidden ? 'Unhide' : 'Hide chat'}
                </button>
            </header>
            {findOpen && (
                <div className="find-bar">
                    <input ref={findInput} className="search" placeholder={`Search in ${chat.name || 'this chat'}`}
                           value={findQuery} onChange={e => setFindQuery(e.target.value)} onKeyDown={onFindKey}/>
                    <span className="find-count">
                        {findQuery.trim() && (matches.length
                            ? `${matchIndex + 1} of ${matches.length}${matches.length >= 500 ? '+' : ''}`
                            : 'No matches')}
                    </span>
                    <button className="icon-button" title="Older match (Enter)" aria-label="Older match"
                            disabled={matches.length === 0} onClick={() => step(1)}>↑</button>
                    <button className="icon-button" title="Newer match (Shift+Enter)" aria-label="Newer match"
                            disabled={matches.length === 0} onClick={() => step(-1)}>↓</button>
                    <button className="icon-button" title="Close (Esc)" aria-label="Close search" onClick={closeFind}>✕</button>
                </div>
            )}
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
                            <MessageRow msg={m} grouped={!!grouped} showSender={!chat.isDm} flash={m.eventId === flashing}
                                        highlight={findHighlight}
                                        isLatestMine={m.eventId === latestMine}
                                        replied={m.replyTo ? byId.get(m.replyTo) : undefined}
                                        onReply={() => startReply(m)} onReact={key => react(m, key)}
                                        onEdit={() => startEdit(m)} onUnsend={() => unsend(m)}
                                        onRetry={() => Go.RetrySend(chat.roomId, m.eventId).catch(e => setError(errorText(e)))}
                                        onOpenMedia={() => setViewing(m.eventId)}
                                        onFetch={() => Go.ResolveMedia(chat.roomId, m.eventId)}/>
                        </Fragment>
                    );
                })}
                {props.typing.length > 0 && (
                    <div className="typing-row" aria-live="polite">
                        <span className="typing-dots" aria-hidden="true"><i/><i/><i/></span>
                        {chat.isDm ? `${chat.name} is typing…` : typingText(props.typing, false)}
                    </div>
                )}
            </div>
            {error && <div className="banner error">{error}</div>}
            {props.signedOut && (
                <div className="signed-out-bar">
                    <span>{NETWORK_LABELS[chat.network] ?? chat.network} signed you out, so new messages won't arrive here.</span>
                    <button className="primary" onClick={props.onSignIn}>Sign in again</button>
                </div>
            )}
            {editing && (
                <div className="reply-bar">
                    <div>
                        <div className="reply-label">Editing message</div>
                        <div className="reply-snippet">{editing.body}</div>
                    </div>
                    <button className="icon-button" title="Cancel (Esc)" onClick={stopEditing}>✕</button>
                </div>
            )}
            {replyTo && !editing && (
                <div className="reply-bar">
                    <div>
                        <div className="reply-label">Replying to {replyTo.isMe ? 'yourself' : replyTo.senderName}</div>
                        <div className="reply-snippet">{snippet(replyTo)}</div>
                    </div>
                    <button className="icon-button" onClick={() => { pendingReply.current = ''; setReplyTo(null); }}>✕</button>
                </div>
            )}
            <div className="composer">
                {pickingGif && <GifPicker onPick={sendGif} onClose={closeGifs}/>}
                <button className="icon-button attach" title="Send photos or files" onClick={attach}>
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                        <path d="M21 11.5l-8.6 8.6a5.5 5.5 0 01-7.8-7.8l8.6-8.6a3.7 3.7 0 015.2 5.2l-8.6 8.6a1.8 1.8 0 01-2.6-2.6l7.9-7.9"/>
                    </svg>
                </button>
                {gifsEnabled && !editing && (
                    <button className={`icon-button gif-button ${pickingGif ? 'active' : ''}`} title="Send a GIF"
                            aria-label="Send a GIF" aria-expanded={pickingGif} onClick={() => setPickingGif(p => !p)}>GIF</button>
                )}
                <textarea ref={composer} rows={1} placeholder={editing ? 'Edit message' : `Message ${chat.name}`} value={draft}
                          onChange={e => setDraft(e.target.value)} onKeyDown={onKeyDown} onPaste={onPaste} autoFocus/>
                <button className="primary" onClick={send} disabled={!draft.trim()}>{editing ? 'Save' : 'Send'}</button>
            </div>
            {sharedOpen && (
                <SharedPanel chat={chat} onClose={() => setSharedOpen(false)} onShowInChat={eventId => {
                    setSharedOpen(false);
                    goTo(eventId);
                }}/>
            )}
            {viewingIndex >= 0 && (
                <MediaViewer items={media} index={viewingIndex}
                             onIndex={i => setViewing(media[i].eventId)} onClose={() => setViewing(null)}/>
            )}
        </section>
    );
}

// readBase64 reads a file as base64, to pass to Go.
function readBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result).replace(/^data:[^,]*,/, ''));
        reader.onerror = () => reject(reader.error ?? new Error('Couldn\'t read the pasted file'));
        reader.readAsDataURL(file);
    });
}

function snippet(m: inbox.Message): string {
    if (m.redacted) return 'Message deleted';
    if (m.body) return m.body;
    return {image: 'Photo', video: 'Video', audio: 'Voice message', file: m.fileName}[m.kind] ?? '';
}

function MessageRow(props: {
    msg: inbox.Message;
    grouped: boolean;
    flash?: boolean;
    // Text to mark in the message (Find in this chat).
    highlight?: string;
    showSender: boolean;
    isLatestMine: boolean;
    replied?: inbox.Message;
    onReply: () => void;
    onReact: (key: string) => void;
    onEdit: () => void;
    onUnsend: () => void;
    onRetry: () => Promise<void>;
    onOpenMedia: () => void;
    onFetch: () => Promise<void>;
}) {
    const {msg} = props;
    const [picking, setPicking] = useState(false);
    // Unsending asks first, just under the message.
    const [confirmUnsend, setConfirmUnsend] = useState(false);
    // Where the full emoji picker opens from, while it's open.
    const [moreFrom, setMoreFrom] = useState<DOMRect | null>(null);
    const {onReact} = props;
    const pickMore = useCallback((emoji: string) => {
        setMoreFrom(null);
        onReact(emoji);
    }, [onReact]);
    const closeMore = useCallback(() => setMoreFrom(null), []);
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
        <div data-event-id={msg.eventId}
             className={`msg-row ${msg.isMe ? 'me' : 'them'} ${props.grouped ? 'grouped' : ''} ${picking ? 'picking' : ''} ${props.flash ? 'flash' : ''}`}>
            {!msg.isMe && <div className="msg-avatar">{!props.grouped && <Avatar name={msg.senderName} src={msg.senderAvatar} size={28}/>}</div>}
            <div className="msg-col">
                {props.showSender && !msg.isMe && !props.grouped && <div className="msg-sender">{msg.senderName}</div>}
                {msg.replyTo && (
                    <div className="msg-quote">{props.replied ? snippet(props.replied) : 'Original message'}</div>
                )}
                <div className="msg-line">
                    <div className={`bubble ${msg.kind} ${msg.redacted ? 'redacted' : ''} ${msg.pending ? 'changing' : ''}`} title={new Date(msg.ts).toLocaleString()}>
                        <MessageBody msg={msg} highlight={props.highlight ?? ''} onOpenMedia={props.onOpenMedia} onFetch={props.onFetch}/>
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
                            {msg.canEdit && (
                                <button title="Edit" aria-label="Edit" onClick={props.onEdit}>
                                    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
                                         strokeLinecap="round" strokeLinejoin="round">
                                        <path d="M4 20h4L19 9a2.8 2.8 0 0 0-4-4L4 16v4z"/>
                                        <path d="m13.5 6.5 4 4"/>
                                    </svg>
                                </button>
                            )}
                            {msg.canUnsend && (
                                <button title="Unsend" aria-label="Unsend" onClick={() => setConfirmUnsend(true)}>
                                    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
                                         strokeLinecap="round" strokeLinejoin="round">
                                        <path d="M4 7h16M10 11v6M14 11v6"/>
                                        <path d="M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3"/>
                                    </svg>
                                </button>
                            )}
                            {picking && (
                                <div className="reaction-picker">
                                    {QUICK_REACTIONS.map(r => (
                                        <button key={r} onClick={() => {
                                            props.onReact(r);
                                            setPicking(false);
                                        }}>{r}</button>
                                    ))}
                                    <button className="more-reactions" title="More reactions" aria-label="More reactions"
                                            onClick={e => {
                                                setMoreFrom(e.currentTarget.getBoundingClientRect());
                                                setPicking(false);
                                            }}>
                                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                                            <path d="M12 5v14M5 12h14"/>
                                        </svg>
                                    </button>
                                </div>
                            )}
                            {moreFrom && <EmojiPicker anchor={moreFrom} onPick={pickMore} onClose={closeMore}/>}
                        </div>
                    )}
                </div>
                {!msg.redacted && msg.previews?.map((p, i) => <LinkPreviewCard key={i} preview={p}/>)}
                {confirmUnsend && msg.canUnsend && (
                    <div className="confirm-unsend" role="alertdialog" aria-label="Unsend message">
                        <span>Unsend for everyone?</span>
                        <button className="danger" autoFocus onClick={() => {
                            setConfirmUnsend(false);
                            props.onUnsend();
                        }}>Unsend</button>
                        <button onClick={() => setConfirmUnsend(false)}>Cancel</button>
                    </div>
                )}
                {msg.reactions?.length > 0 && (
                    <div className="reactions">
                        {msg.reactions.map(r => (
                            <span key={r.key} className={`reaction ${r.mine ? 'mine' : ''}`}>{r.key}{r.count > 1 && ` ${r.count}`}</span>
                        ))}
                    </div>
                )}
                <MessageStatus msg={msg} isDm={!props.showSender} isLatestMine={props.isLatestMine} onRetry={props.onRetry}/>
            </div>
        </div>
    );
}

// A link's title, description and picture, as the network sent them;
// clicking it opens the link in the browser.
function LinkPreviewCard({preview: p}: {preview: inbox.LinkPreview}) {
    // The picture's box is sized from the stored dimensions so the thread
    // doesn't jump when it loads; very tall pictures are cropped.
    const ratio = p.width > 0 && p.height > 0 ? Math.max(p.width / p.height, 1.2) : 1.91;
    return (
        <a className="link-preview" href={p.url} title={p.url} onClick={e => {
            e.preventDefault();
            openLink(p.url);
        }}>
            {p.image && <img src={p.image} alt="" loading="lazy" style={{aspectRatio: String(ratio)}}/>}
            <div className="link-preview-text">
                <div className="link-preview-site">{p.site}</div>
                {p.title && <div className="link-preview-title">{p.title}</div>}
                {p.description && p.description !== p.title && <div className="link-preview-desc">{p.description}</div>}
            </div>
        </a>
    );
}

const MAX_SEEN_AVATARS = 5;

// What shows under a message, in priority order: a send problem, who has seen
// it, then (on your newest message only) Sent / Delivered.
function MessageStatus(props: {msg: inbox.Message; isDm: boolean; isLatestMine: boolean; onRetry: () => Promise<void>}) {
    const {msg} = props;
    const [retrying, setRetrying] = useState(false);
    if (msg.pending) {
        return <div className="msg-status">{msg.pending === 'unsend' ? 'Unsending…' : 'Saving edit…'}</div>;
    }
    if (msg.changeFailed && !msg.redacted) {
        return <div className="msg-status fail">{msg.changeFailed === 'unsend' ? 'Couldn\'t unsend' : 'Couldn\'t edit'}</div>;
    }
    if (msg.isMe && msg.status === 'pending') {
        return <div className="msg-status">Sending…</div>;
    }
    // The bridge reports fail_retriable or fail_permanent.
    if (msg.isMe && msg.status.startsWith('fail')) {
        return (
            <div className="msg-status fail">
                Failed to send ·{' '}
                <button className="link retry-send" disabled={retrying} onClick={() => {
                    setRetrying(true);
                    props.onRetry().finally(() => setRetrying(false));
                }}>{retrying ? 'Sending…' : 'Try again'}</button>
            </div>
        );
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

function MessageBody({msg, highlight, onOpenMedia, onFetch}: {msg: inbox.Message; highlight: string; onOpenMedia: () => void; onFetch: () => Promise<void>}) {
    if (msg.redacted) return <em>Message deleted</em>;
    const caption = msg.body ? <div className="caption">{linkify(msg.body, highlight)}</div> : null;
    switch (msg.kind) {
        case 'image':
            // Always inside MediaFrame, so loading a post keeps this <img>
            // and just changes its source: the thumbnail stays up until the
            // full photo is ready, instead of a new image starting empty.
            return <>
                <MediaFrame fetchable={msg.fetchable} onFetch={onFetch}>
                    <img className="media zoomable" src={msg.mediaUrl} alt={msg.fileName} loading="lazy" onClick={onOpenMedia}
                         style={mediaSize(msg)}/>
                </MediaFrame>
                {caption}
            </>;
        case 'video':
            return <><InlineVideo msg={msg} onExpand={onOpenMedia}/>{caption}</>;
        case 'audio':
            return <audio src={msg.mediaUrl} controls preload="metadata"/>;
        case 'file':
            // Download links do nothing in the Mac webview, so files open in
            // their default app instead.
            return <><a href={msg.mediaUrl} onClick={e => {
                e.preventDefault();
                Go.OpenMedia(msg.mediaUrl, msg.fileName).catch(console.error);
            }}>📎 {msg.fileName}</a>{caption}</>;
        case 'emote':
            return <em>{linkify(msg.body, highlight)}</em>;
        default:
            return <>{linkify(msg.body, highlight)}{msg.edited && <span className="edited"> (edited)</span>}</>;
    }
}

// Matches .media's max-height.
const MEDIA_MAX_HEIGHT = 360;

// mediaSize gives a photo or video its box before the file loads, from the
// stored dimensions: otherwise it starts empty (a photo) or 300x150 (a
// video) and jumps once loaded. Sizing by width keeps the proportions when
// max-width narrows it.
function mediaSize(msg: inbox.Message): CSSProperties | undefined {
    if (!(msg.width > 0 && msg.height > 0)) return undefined;
    return {
        aspectRatio: `${msg.width} / ${msg.height}`,
        width: Math.round(msg.width * Math.min(1, MEDIA_MAX_HEIGHT / msg.height)),
    };
}

const FETCH_LABELS: Record<string, string> = {reel: 'Play reel', story: 'Load story', post: 'Load post'};

// The frame around a photo. A shared Instagram reel, story or post from
// history arrives as just a thumbnail and link (fetchable is set); the button
// asks the bridge to fetch the real thing, and the message then turns into
// it in place, so there's nothing to do after.
function MediaFrame(props: {fetchable: string; onFetch: () => Promise<void>; children: React.ReactNode}) {
    const [state, setState] = useState<'idle' | 'loading' | 'failed'>('idle');
    const [error, setError] = useState('');
    const fetch = (e: React.MouseEvent) => {
        e.stopPropagation();
        setState('loading');
        setError('');
        // On success the message turns into the media, which drops the button.
        props.onFetch().then(() => setState('idle')).catch(err => {
            setState('failed');
            setError(errorText(err));
        });
    };
    return (
        <div className="fetchable">
            {props.children}
            {props.fetchable && (
                <button className={`fetch-button ${state}`} onClick={fetch} disabled={state === 'loading'}
                        title={state === 'failed' ? error : undefined}>
                    {state === 'loading' ? 'Loading…' : state === 'failed' ? 'Try again' : <>▶ {FETCH_LABELS[props.fetchable] ?? 'Load'}</>}
                </button>
            )}
            {props.fetchable && state === 'failed' && <div className="fetch-error">{error}</div>}
        </div>
    );
}

// A video in the chat, with a button to watch it full-window. Clicking the
// video itself plays it in place, so expanding needs its own button.
//
// Until the file loads, a video has no size or picture of its own, so it
// would collapse to a small blank box and then jump to full size (most
// visibly when a fetched reel replaces its thumbnail). The stored size and
// thumbnail fill in until then.
function InlineVideo(props: {msg: inbox.Message; onExpand: () => void}) {
    const {msg} = props;
    const video = useRef<HTMLVideoElement>(null);
    return (
        <div className="inline-video">
            <video ref={video} className="media" src={msg.mediaUrl} poster={msg.thumbnail || undefined} style={mediaSize(msg)}
                   controls preload="metadata"/>
            <button className="expand-button" aria-label="Open full window" title="Open full window" onClick={() => {
                video.current?.pause();
                props.onExpand();
            }}>
                <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2"
                     strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/>
                </svg>
            </button>
        </div>
    );
}
