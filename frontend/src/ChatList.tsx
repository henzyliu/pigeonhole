import {formatTime, inbox} from './api';
import Avatar from './Avatar';

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
                            <span className="chat-preview">{chat.preview}</span>
                            {chat.unread > 0 && <span className="badge">{chat.unread > 99 ? '99+' : chat.unread}</span>}
                        </div>
                    </div>
                </li>
            ))}
        </ul>
    );
}
