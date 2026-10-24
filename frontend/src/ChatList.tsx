import {useEffect, useRef, useState} from 'react';
import {formatTime, inbox} from './api';
import Avatar from './Avatar';
import {splitLinks} from './links';

export default function ChatList(props: {
    chats: inbox.Chat[];
    selected: string;
    onSelect: (roomId: string) => void;
    onSetHidden: (roomId: string, hidden: boolean) => void;
    emptyText: string;
}) {
    // The right-click menu, at the pointer. Acting from here doesn't open the
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
                    className={`chat-row ${chat.roomId === props.selected ? 'selected' : ''} ${chat.unread > 0 ? 'unread' : ''}`}
                    onClick={() => props.onSelect(chat.roomId)}
                    onContextMenu={e => {
                        e.preventDefault();
                        setMenu({chat, x: e.clientX, y: e.clientY});
                    }}>
                    <Avatar name={chat.name} src={chat.avatar} size={44} network={chat.network}/>
                    <div className="chat-row-body">
                        <div className="chat-row-top">
                            <span className="chat-name">{chat.name || 'Unnamed chat'}</span>
                            <span className="chat-time">{formatTime(chat.lastTs)}</span>
                        </div>
                        <div className="chat-row-bottom">
                            <span className="chat-preview">{chat.lastFromMe && 'You: '}{plainPreview(chat.preview)}</span>
                            {chat.unread > 0
                                ? <span className="badge">{chat.unread > 99 ? '99+' : chat.unread}</span>
                                : <LatestStatus chat={chat}/>}
                        </div>
                    </div>
                </li>
            ))}
        </ul>
        {menu && (
            <ContextMenu x={menu.x} y={menu.y} onClose={() => setMenu(null)} items={[{
                label: menu.chat.hidden ? 'Unhide chat' : 'Hide chat',
                onClick: () => props.onSetHidden(menu.chat.roomId, !menu.chat.hidden),
            }]}/>
        )}
        </>
    );
}

// A small menu at the pointer, kept inside the window. It closes on a pick,
// a click elsewhere, Escape, or the window losing focus.
function ContextMenu(props: {x: number; y: number; items: {label: string; onClick: () => void}[]; onClose: () => void}) {
    const ref = useRef<HTMLDivElement>(null);
    const [pos, setPos] = useState({left: props.x, top: props.y});

    useEffect(() => {
        const el = ref.current;
        if (el) {
            const margin = 8;
            setPos({
                left: Math.min(props.x, window.innerWidth - el.offsetWidth - margin),
                top: Math.min(props.y, window.innerHeight - el.offsetHeight - margin),
            });
        }
        const onDown = (e: MouseEvent) => {
            if (!ref.current?.contains(e.target as Node)) props.onClose();
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
    }, [props.x, props.y, props.onClose]);

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
