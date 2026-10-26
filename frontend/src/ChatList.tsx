import {useEffect, useRef, useState} from 'react';
import {formatTime, inbox, NETWORK_LABELS} from './api';
import Avatar from './Avatar';
import {splitLinks, typingText} from './links';

export default function ChatList(props: {
    chats: inbox.Chat[];
    selected: string;
    onSelect: (roomId: string) => void;
    onSetHidden: (roomId: string, hidden: boolean) => void;
    onSetMuted: (roomId: string, muted: boolean) => void;
    emptyText: string;
    typing: Record<string, string[]>;
}) {
    // The chat's ⋯ menu, shown on hover. Acting from here doesn't open the
    // chat, so nothing is marked as read.
    const [menu, setMenu] = useState<{chat: inbox.Chat; x: number; y: number} | null>(null);

    if (props.chats.length === 0) {
        return <div className="chat-list-empty">{props.emptyText}</div>;
    }
    return (
        <>
        <ul className="chat-list" onScroll={() => setMenu(null)}>
            {props.chats.map(chat => (
                <li key={chat.roomId}
                    className={`chat-row ${chat.roomId === props.selected ? 'selected' : ''} ${chat.unread > 0 ? 'unread' : ''} ${menu?.chat.roomId === chat.roomId ? 'menu-open' : ''}`}
                    onClick={() => props.onSelect(chat.roomId)}>
                    <Avatar name={chat.name} src={chat.avatar} size={44} network={chat.network}/>
                    <div className="chat-row-body">
                        <div className="chat-row-top">
                            <span className="chat-name">{chat.name || 'Unnamed chat'}</span>
                            {(chat.muted || chat.mutedOnNetwork) && <MutedIcon chat={chat}/>}
                            <span className="chat-time">{formatTime(chat.lastTs)}</span>
                        </div>
                        <div className="chat-row-bottom">
                            {props.typing[chat.roomId]?.length
                                ? <span className="chat-preview typing-preview">{typingText(props.typing[chat.roomId], chat.isDm)}</span>
                                : chat.draft && chat.roomId !== props.selected
                                ? <span className="chat-preview"><span className="draft-label">Draft:</span> {chat.draft.replace(/\s+/g, ' ')}</span>
                                : <span className="chat-preview">{chat.lastFromMe && 'You: '}{plainPreview(chat.preview)}</span>}
                            {chat.unread > 0
                                ? <span className={`badge ${chat.muted || chat.mutedOnNetwork ? 'badge-muted' : ''}`}>{chat.unread > 99 ? '99+' : chat.unread}</span>
                                : <LatestStatus chat={chat}/>}
                        </div>
                    </div>
                    <button className="row-menu-button" aria-label="Chat options" title="More"
                            aria-haspopup="menu" aria-expanded={menu?.chat.roomId === chat.roomId}
                            onClick={e => {
                                e.stopPropagation();
                                const r = e.currentTarget.getBoundingClientRect();
                                setMenu(m => m?.chat.roomId === chat.roomId ? null : {chat, x: r.right, y: r.bottom + 4});
                            }}>
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                            <circle cx="5" cy="12" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="19" cy="12" r="2"/>
                        </svg>
                    </button>
                </li>
            ))}
        </ul>
        {menu && (
            <ContextMenu x={menu.x} y={menu.y} alignRight onClose={() => setMenu(null)} items={[{
                label: menu.chat.muted ? 'Unmute chat' : 'Mute chat',
                onClick: () => props.onSetMuted(menu.chat.roomId, !menu.chat.muted),
            }, {
                label: menu.chat.hidden ? 'Unhide chat' : 'Hide chat',
                onClick: () => props.onSetHidden(menu.chat.roomId, !menu.chat.hidden),
            }]}/>
        )}
        </>
    );
}

// A small menu below its button (alignRight lines its right edge up with x),
// kept inside the window. It closes on a pick, a click elsewhere, Escape, or
// the window losing focus.
function ContextMenu(props: {
    x: number;
    y: number;
    alignRight?: boolean;
    items: {label: string; onClick: () => void}[];
    onClose: () => void;
}) {
    const ref = useRef<HTMLDivElement>(null);
    const [pos, setPos] = useState({left: props.x, top: props.y});

    useEffect(() => {
        const el = ref.current;
        if (el) {
            const margin = 8;
            const left = props.alignRight ? props.x - el.offsetWidth : props.x;
            setPos({
                left: Math.max(margin, Math.min(left, window.innerWidth - el.offsetWidth - margin)),
                top: Math.min(props.y, window.innerHeight - el.offsetHeight - margin),
            });
        }
        // The ⋯ button toggles the menu itself, so a press on it isn't
        // "elsewhere".
        const onDown = (e: MouseEvent) => {
            const target = e.target as Element;
            if (!ref.current?.contains(target) && !target.closest?.('.row-menu-button')) props.onClose();
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') props.onClose();
        };
        document.addEventListener('mousedown', onDown);
        document.addEventListener('keydown', onKey);
        window.addEventListener('blur', props.onClose);
        return () => {
            document.removeEventListener('mousedown', onDown);
            document.removeEventListener('keydown', onKey);
            window.removeEventListener('blur', props.onClose);
        };
    }, [props.x, props.y, props.alignRight, props.onClose]);

    return (
        <div ref={ref} className="context-menu" role="menu" style={pos} onContextMenu={e => e.preventDefault()}>
            {props.items.map(item => (
                <button key={item.label} role="menuitem" onClick={() => {
                    props.onClose();
                    item.onClick();
                }}>{item.label}</button>
            ))}
        </div>
    );
}

const MAX_SEEN_AVATARS = 3;

// Like Messenger's chat list: when your message is the latest, the people
// who've seen it (small photos), or a filled tick once delivered and an
// outline tick once sent.
function LatestStatus({chat}: {chat: inbox.Chat}) {
    if (!chat.lastFromMe) return null;
    const seenBy = chat.seenBy ?? [];
    if (seenBy.length > 0) {
        return (
            <span className="list-status list-seen" title={`Seen by ${seenBy.map(r => r.name).join(', ')}`}>
                {seenBy.slice(0, MAX_SEEN_AVATARS).map((r, i) => <Avatar key={i} name={r.name} src={r.avatar} size={16}/>)}
                {seenBy.length > MAX_SEEN_AVATARS && <span className="seen-more">+{seenBy.length - MAX_SEEN_AVATARS}</span>}
            </span>
        );
    }
    switch (chat.lastStatus) {
        case 'pending':
            return null;
        case 'fail':
        case 'retriable':
            return <span className="list-status tick failed" title="Failed to send">!</span>;
        case 'delivered':
            return <span className="list-status tick delivered" title="Delivered"><Check/></span>;
        default:
            return <span className="list-status tick sent" title="Sent"><Check/></span>;
    }
}

function Check() {
    return (
        <svg width="9" height="9" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="2"
             strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M2.5 6.5 5 9l4.5-6"/>
        </svg>
    );
}

// Previews are plain text, so links show as their short labels.
function plainPreview(text: string): string {
    return splitLinks(text).map(seg => seg.text).join('');
}

// Shown beside a muted chat's name: muted in Pigeonhole (⋯ → Mute), or in
// the network's own app, which Pigeonhole follows.
function MutedIcon({chat}: {chat: inbox.Chat}) {
    const where = chat.muted ? 'Muted in Pigeonhole' : `Muted in ${NETWORK_LABELS[chat.network] ?? chat.network}`;
    return (
        <svg className="chat-muted" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
             strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" role="img" aria-label={where}>
            <title>{where}</title>
            <path d="M13.7 21a2 2 0 0 1-3.4 0"/>
            <path d="M18.6 13A17.9 17.9 0 0 1 18 8"/>
            <path d="M6.3 6.3A5.9 5.9 0 0 0 6 8c0 7-3 9-3 9h14"/>
            <path d="M18 8a6 6 0 0 0-9.3-5"/>
            <path d="M2 2l20 20"/>
        </svg>
    );
}
