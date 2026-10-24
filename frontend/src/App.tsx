import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {EventsOn, Go, inbox, NETWORK_LABELS} from './api';
import ChatList from './ChatList';
import Thread from './Thread';
import Accounts from './Accounts';
import NewChat from './NewChat';
import Settings from './Settings';
import {applyTheme, Theme} from './theme';
import NetworkIcon from './NetworkIcon';

type Filter = 'all' | 'unread' | string;
// 'inbox' holds one-to-one chats and is labelled People.
type Category = 'inbox' | 'groups' | 'marketplace' | 'requests';

const CATEGORIES: {id: Category; label: string; alwaysShown?: boolean}[] = [
    {id: 'inbox', label: 'People', alwaysShown: true},
    {id: 'groups', label: 'Groups', alwaysShown: true},
    {id: 'marketplace', label: 'Market'},
];
// Like the Messenger app, requests aren't a tab: they open from a row under the tabs.

export default function App() {
    const [chats, setChats] = useState<inbox.Chat[]>([]);
    const [accounts, setAccounts] = useState<inbox.Account[]>([]);
    const [selected, setSelected] = useState<string>('');
    const [filter, setFilter] = useState<Filter>('all');
    const [category, setCategory] = useState<Category>('inbox');
    // Shows hidden chats instead of the tabs.
    const [showHidden, setShowHidden] = useState(false);
    const [query, setQuery] = useState('');
    const [showAccounts, setShowAccounts] = useState(false);
    const [showNewChat, setShowNewChat] = useState(false);
    const [showSettings, setShowSettings] = useState(false);
    const [theme, setTheme] = useState<Theme>('system');

    // The saved setting wins over the early copy index.html applied.
    useEffect(() => {
        Go.GetSettings().then(s => {
            const t = (s?.theme as Theme) || 'system';
            applyTheme(t);
            setTheme(t);
        }).catch(console.error);
    }, []);
    const [startError, setStartError] = useState('');
    const [messageTick, setMessageTick] = useState(0);
    const selectedRef = useRef(selected);
    selectedRef.current = selected;
    const chatsRef = useRef(chats);
    chatsRef.current = chats;

    const loadChats = useCallback(() => Go.Chats().then(c => setChats(c ?? [])).catch(console.error), []);

    // Selects a chat and switches to the tab and filters that show it.
    const showChat = useCallback((roomId: string, list: inbox.Chat[]) => {
        const chat = list.find(c => c.roomId === roomId);
        setCategory((chat?.category as Category) || 'inbox');
        setShowHidden(!!chat?.hidden);
        setFilter('all');
        setQuery('');
        setSelected(roomId);
    }, []);

    // A chat opened from New chat may have only just been created, so fetch
    // the list fresh before working out which tab it's in.
    const openNewChat = useCallback(async (roomId: string) => {
        setShowNewChat(false);
        const list = await Go.Chats().catch(() => null) ?? chatsRef.current;
        setChats(list);
        showChat(roomId, list);
    }, [showChat]);
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
        const offOpen = EventsOn('inbox:open', (roomId: string) => showChat(roomId, chatsRef.current));
        return () => {
            offChange();
            offAccounts();
            offError();
            offOpen();
        };
    }, [loadChats, loadAccounts, showChat]);

    // A chat's history can finish loading without any new message to trigger
    // a refresh, so check back while something is still loading.
    const historyLoading = accounts.some(a => a.historyPending > 0) || chats.some(c => c.historyLoading);
    useEffect(() => {
        if (!historyLoading) return;
        const timer = window.setInterval(() => {
            loadChats();
            loadAccounts();
        }, 10_000);
        return () => window.clearInterval(timer);
    }, [historyLoading, loadChats, loadAccounts]);

    // Closing the window only hides it now; the first time, the backend says
    // Pigeonhole is still running.
    useEffect(() => {
        const onVisibility = () => {
            if (document.visibilityState === 'hidden') Go.WindowHidden().catch(console.error);
        };
        document.addEventListener('visibilitychange', onVisibility);
        return () => document.removeEventListener('visibilitychange', onVisibility);
    }, []);

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

    const categoryStats = useMemo(() => {
        const stats: Record<Category, {total: number; unread: number}> = {
            inbox: {total: 0, unread: 0}, groups: {total: 0, unread: 0},
            marketplace: {total: 0, unread: 0}, requests: {total: 0, unread: 0},
        };
        for (const c of chats) {
            if (c.hidden) continue;
            const s = stats[(c.category as Category) || 'inbox'] ?? stats.inbox;
            s.total++;
            if (c.unread > 0) s.unread++;
        }
        return stats;
    }, [chats]);

    // People and Groups are always shown; the other tabs only when they have chats.
    const tabs = CATEGORIES.filter(t => t.alwaysShown || categoryStats[t.id].total > 0);

    // If a conditional tab empties (e.g. the last request was accepted), go back to People.
    useEffect(() => {
        const tab = CATEGORIES.find(t => t.id === category);
        if (!tab?.alwaysShown && categoryStats[category].total === 0) setCategory('inbox');
    }, [category, categoryStats]);

    const hiddenCount = useMemo(() => chats.filter(c => c.hidden).length, [chats]);

    // Leave the hidden view once it's empty (e.g. after unhiding the last chat).
    useEffect(() => {
        if (showHidden && hiddenCount === 0) setShowHidden(false);
    }, [showHidden, hiddenCount]);

    const visibleChats = useMemo(() => {
        const q = query.trim().toLowerCase();
        return chats.filter(c => {
            // Search looks across every category, hidden chats included, so
            // nothing is out of reach.
            if (!q && showHidden !== c.hidden) return false;
            if (!q && !showHidden && (c.category || 'inbox') !== category) return false;
            if (filter === 'unread' && c.unread === 0) return false;
            if (filter !== 'all' && filter !== 'unread' && c.network !== filter) return false;
            return !q || c.name.toLowerCase().includes(q) || c.preview.toLowerCase().includes(q);
        });
    }, [chats, category, filter, query, showHidden]);

    const selectedChat = chats.find(c => c.roomId === selected);
    const totalUnread = showHidden ? chats.filter(c => c.hidden && c.unread > 0).length : categoryStats[category].unread;

    // Hiding the open chat closes it, since it leaves the list you're looking at.
    const setChatHidden = async (roomId: string, hidden: boolean) => {
        await Go.SetChatHidden(roomId, hidden);
        setSelected('');
        loadChats();
    };
    const emptyText = accounts.length === 0
        ? 'Connect an account to get started.'
        : showHidden ? 'No hidden chats.'
        : category === 'requests' ? 'No message requests.'
            : category === 'marketplace' ? 'No Marketplace conversations.'
                : category === 'groups' ? 'No group chats yet.'
                    : 'No conversations yet. They\'ll appear as your accounts sync.';

    return (
        <div className="app">
            <aside className="sidebar">
                <div className="sidebar-header">
                    <h1>Pigeonhole</h1>
                    <div className="header-actions">
                        <button className="icon-button" title="New chat" disabled={accounts.length === 0}
                                onClick={() => setShowNewChat(true)}>
                            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
                                 strokeLinecap="round" strokeLinejoin="round">
                                <path d="M12 20h9"/>
                                <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>
                            </svg>
                        </button>
                        <button className="icon-button" title="Settings" onClick={() => setShowSettings(true)}>
                            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
                                 strokeLinecap="round" strokeLinejoin="round">
                                <circle cx="12" cy="12" r="3"/>
                                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>
                            </svg>
                        </button>
                        <button className="icon-button" title="Accounts" onClick={() => setShowAccounts(true)}>
                            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                                <circle cx="12" cy="8" r="4"/>
                                <path d="M4 21c0-4 4-6 8-6s8 2 8 6"/>
                            </svg>
                            {accounts.some(a => a.state !== 'connected' && a.state !== 'connecting') && <span className="dot-alert"/>}
                        </button>
                    </div>
                </div>
                <input
                    className="search"
                    placeholder="Search"
                    value={query}
                    onChange={e => setQuery(e.target.value)}
                />
                {!query.trim() && showHidden && (
                    <button className="requests-row" onClick={() => setShowHidden(false)}>
                        <span>← Back to chats</span>
                        <span className="muted">Hidden chats</span>
                    </button>
                )}
                {!query.trim() && !showHidden && category !== 'requests' && (
                    <div className="tabs" role="tablist">
                        {tabs.map(t => (
                            <button key={t.id} role="tab" aria-selected={category === t.id}
                                    className={`tab ${category === t.id ? 'active' : ''}`}
                                    onClick={() => setCategory(t.id)}>
                                <span className="tab-label">{t.label}</span>
                                {categoryStats[t.id].unread > 0 && <span className="tab-count">{categoryStats[t.id].unread}</span>}
                            </button>
                        ))}
                    </div>
                )}
                {!query.trim() && !showHidden && categoryStats.requests.total > 0 && (
                    category === 'requests'
                        ? <button className="requests-row" onClick={() => setCategory('inbox')}>
                            <span>← Back to chats</span>
                            <span className="muted">Message requests</span>
                        </button>
                        : <button className="requests-row" onClick={() => setCategory('requests')}>
                            <span>Message requests</span>
                            <span className={categoryStats.requests.unread > 0 ? 'tab-count' : 'muted'}>
                                {categoryStats.requests.unread || categoryStats.requests.total}
                            </span>
                        </button>
                )}
                <div className="filters">
                    <FilterChip active={filter === 'all'} onClick={() => setFilter('all')}>All</FilterChip>
                    <FilterChip active={filter === 'unread'} onClick={() => setFilter('unread')}>
                        Unread{totalUnread > 0 && ` ${totalUnread}`}
                    </FilterChip>
                    {networks.map(n => (
                        <FilterChip key={n} active={filter === n} onClick={() => setFilter(n)}>
                            <NetworkIcon network={n} size={14}/>{NETWORK_LABELS[n] ?? n}
                        </FilterChip>
                    ))}
                </div>
                {startError && <div className="banner error">{startError}</div>}
                <ChatList chats={visibleChats} selected={selected} onSelect={setSelected} emptyText={emptyText}/>
                {!query.trim() && !showHidden && hiddenCount > 0 && (
                    <button className="requests-row hidden-row" onClick={() => {
                        setShowHidden(true);
                        setFilter('all');
                    }}>
                        <span className="muted">Hidden chats</span>
                        <span className="muted">{hiddenCount}</span>
                    </button>
                )}
            </aside>
            <main className="main">
                {selectedChat
                    ? <Thread key={selectedChat.roomId} chat={selectedChat} tick={messageTick}
                              onSetHidden={hidden => setChatHidden(selectedChat.roomId, hidden)}/>
                    : <div className="empty-thread">
                        <p>Select a conversation</p>
                        {accounts.length === 0 &&
                            <button className="primary" onClick={() => setShowAccounts(true)}>Connect an account</button>}
                    </div>}
            </main>
            {showAccounts && <Accounts accounts={accounts} onClose={() => setShowAccounts(false)}/>}
            {showNewChat && <NewChat onClose={() => setShowNewChat(false)} onOpened={openNewChat}/>}
            {showSettings && <Settings theme={theme} onTheme={setTheme} onClose={() => setShowSettings(false)}/>}
        </div>
    );
}

function FilterChip(props: {active: boolean; onClick: () => void; children: React.ReactNode}) {
    return <button className={`chip ${props.active ? 'active' : ''}`} onClick={props.onClick}>{props.children}</button>;
}
