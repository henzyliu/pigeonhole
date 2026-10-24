import {formatTime, inbox} from './api';
import Avatar from './Avatar';
import {splitLinks} from './links';

export default function ChatList(props: {
    chats: inbox.Chat[];
    selected: string;
    onSelect: (roomId: string) => void;
    emptyText: string;
}) {
    if (props.chats.length === 0) {
        return <div className="chat-list-empty">{props.emptyText}</div>;
    }
    return (
        <ul className="chat-list">
            {props.chats.map(chat => (
                <li key={chat.roomId}
                    className={`chat-row ${chat.roomId === props.selected ? 'selected' : ''} ${chat.unread > 0 ? 'unread' : ''}`}
                    onClick={() => props.onSelect(chat.roomId)}>
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
