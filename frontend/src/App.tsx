import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {EventsOn, Go, inbox, NETWORK_LABELS} from './api';
import ChatList from './ChatList';
import Thread from './Thread';
import Accounts from './Accounts';

type Filter = 'all' | 'unread' | string;

export default function App() {
    const [chats, setChats] = useState<inbox.Chat[]>([]);
    const [accounts, setAccounts] = useState<inbox.Account[]>([]);
    const [selected, setSelected] = useState<string>('');
    const [filter, setFilter] = useState<Filter>('all');
    const [query, setQuery] = useState('');
    const [showAccounts, setShowAccounts] = useState(false);
    const [startError, setStartError] = useState('');
    const [messageTick, setMessageTick] = useState(0);
    const selectedRef = useRef(selected);
    selectedRef.current = selected;

    const loadChats = useCallback(() => Go.Chats().then(c => setChats(c ?? [])).catch(console.error), []);
    const loadAccounts = useCallback(() => Go.Accounts().then(a => setAccounts(a ?? [])).catch(console.error), []);

    useEffect(() => {
        loadChats();
        loadAccounts();
        Go.StartupError().then(e => e && setStartError(e));
        // Chat list changes arrive in bursts during sync; coalesce them.
        let chatTimer: number | undefined;
        const offChange = EventsOn('inbox:change', (change: {kind: string; roomId?: string}) => {
            if (change.kind === 'rooms' || change.kind === 'messages') {
                window.clearTimeout(chatTimer);
                chatTimer = window.setTimeout(loadChats, 150);
            }
            if (change.kind === 'messages' && change.roomId === selectedRef.current) {
                setMessageTick(t => t + 1);
            }
        });
        const offAccounts = EventsOn('inbox:accounts', loadAccounts);
        const offError = EventsOn('inbox:error', (e: string) => setStartError(e));
        // Clicking a desktop notification opens that chat.
        const offOpen = EventsOn('inbox:open', (roomId: string) => {
            setFilter('all');
            setQuery('');
            setSelected(roomId);
        });
        return () => {
            offChange();
            offAccounts();
            offError();
            offOpen();
        };
    }, [loadChats, loadAccounts]);

    // Keep the backend informed so it can skip notifications for the chat
    // you're already reading.
    useEffect(() => {
        const report = () => Go.SetView(selectedRef.current, document.hasFocus());
        report();
        window.addEventListener('focus', report);
        window.addEventListener('blur', report);
        return () => {
            window.removeEventListener('focus', report);
            window.removeEventListener('blur', report);
        };
    }, [selected]);

    useEffect(() => {
        if (accounts.length === 0 && chats.length === 0) return;
        if (accounts.length === 0) setShowAccounts(true);
    }, [accounts, chats]);

    const networks = useMemo(() => Array.from(new Set(accounts.map(a => a.network))), [accounts]);

    const visibleChats = useMemo(() => {
        const q = query.trim().toLowerCase();
        return chats.filter(c => {
            if (filter === 'unread' && c.unread === 0) return false;
            if (filter !== 'all' && filter !== 'unread' && c.network !== filter) return false;
            return !q || c.name.toLowerCase().includes(q) || c.preview.toLowerCase().includes(q);
        });
    }, [chats, filter, query]);

    const selectedChat = chats.find(c => c.roomId === selected);
    const totalUnread = chats.reduce((n, c) => n + (c.unread > 0 ? 1 : 0), 0);

    return (
        <div className="app">
            <aside className="sidebar">
                <div className="sidebar-header">
                    <h1>Inbox</h1>
                    <button className="icon-button" title="Accounts" onClick={() => setShowAccounts(true)}>
                        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <circle cx="12" cy="8" r="4"/>
                            <path d="M4 21c0-4 4-6 8-6s8 2 8 6"/>
                        </svg>
                        {accounts.some(a => a.state !== 'connected' && a.state !== 'connecting') && <span className="dot-alert"/>}
                    </button>
                </div>
                <input
                    className="search"
                    placeholder="Search"
                    value={query}
                    onChange={e => setQuery(e.target.value)}
                />
                <div className="filters">
                    <FilterChip active={filter === 'all'} onClick={() => setFilter('all')}>All</FilterChip>
                    <FilterChip active={filter === 'unread'} onClick={() => setFilter('unread')}>
                        Unread{totalUnread > 0 && ` ${totalUnread}`}
                    </FilterChip>
                    {networks.map(n => (
                        <FilterChip key={n} active={filter === n} onClick={() => setFilter(n)}>
                            <span className={`net-dot net-${n}`}/>{NETWORK_LABELS[n] ?? n}
                        </FilterChip>
                    ))}
                </div>
                {startError && <div className="banner error">{startError}</div>}
                <ChatList chats={visibleChats} selected={selected} onSelect={setSelected}
                          emptyText={accounts.length === 0 ? 'Connect an account to get started.' : 'No conversations yet. They\'ll appear as your accounts sync.'}/>
            </aside>
            <main className="main">
                {selectedChat
                    ? <Thread key={selectedChat.roomId} chat={selectedChat} tick={messageTick}/>
                    : <div className="empty-thread">
                        <p>Select a conversation</p>
                        {accounts.length === 0 &&
                            <button className="primary" onClick={() => setShowAccounts(true)}>Connect an account</button>}
                    </div>}
            </main>
            {showAccounts && <Accounts accounts={accounts} onClose={() => setShowAccounts(false)}/>}
        </div>
    );
}

function FilterChip(props: {active: boolean; onClick: () => void; children: React.ReactNode}) {
    return <button className={`chip ${props.active ? 'active' : ''}`} onClick={props.onClick}>{props.children}</button>;
}
