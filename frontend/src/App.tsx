import {KeyboardEvent as ReactKeyboardEvent, useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {EventsOn, Go, inbox, NETWORK_LABELS} from './api';
import {isMod, MOD, ShortcutsHelp} from './shortcuts';
import ChatList from './ChatList';
import MessageResults from './MessageResults';
import Thread from './Thread';
import Welcome from './Welcome';
import Accounts from './Accounts';
import NewChat from './NewChat';
import Settings from './Settings';
import {applyTheme, Theme} from './theme';
import {useUpdateStatus} from './updates';
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


const NO_ONE: string[] = [];

// signedOutOf reports whether every account on a network has been signed
// out by it, so nothing new arrives in its chats.
function signedOutOf(accounts: inbox.Account[], network: string): boolean {
    const on = accounts.filter(a => a.network === network);
    return on.length > 0 && on.every(a => a.needsLogin);
}
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
    // Spins the Refresh button for a moment after reconnecting.
    const [refreshing, setRefreshing] = useState(false);
    const refresh = useCallback(() => {
        setRefreshing(true);
        Go.Refresh().catch(console.error);
        window.setTimeout(() => setRefreshing(false), 1500);
    }, []);
    const [showShortcuts, setShowShortcuts] = useState(false);
    // A search result being opened: its chat opens at that message. seq
    // makes opening the same chat again (at another result) reload it.
    const [jump, setJump] = useState<{roomId: string; eventId: string; seq: number} | null>(null);
    // The first-run welcome, for someone with no chats yet.
    const [showWelcome, setShowWelcome] = useState(false);
    useEffect(() => {
        Go.ShowWelcome().then(setShowWelcome).catch(console.error);
    }, []);
    const finishWelcome = useCallback(() => {
        setShowWelcome(false);
        Go.FinishWelcome().catch(console.error);
    }, []);
    const searchRef = useRef<HTMLInputElement>(null);
    // The chats as listed, for moving through them from the keyboard.
    const visibleRef = useRef<inbox.Chat[]>([]);
    // Which search result Enter opens, while typing in the search box.
    const [searchIndex, setSearchIndex] = useState(0);
    // The network to sign in to again, when Accounts was opened for that.
    const [signInNetwork, setSignInNetwork] = useState('');
    const openSignIn = useCallback((network: string) => {
        setSignInNetwork(network);
        setShowAccounts(true);
    }, []);
    const selectedRef = useRef(selected);
    selectedRef.current = selected;
    const [showNewChat, setShowNewChat] = useState(false);
    const [showSettings, setShowSettings] = useState(false);
    const dialogOpen = showNewChat || showSettings || showAccounts || showShortcuts || showWelcome;
    const dialogOpenRef = useRef(dialogOpen);
    dialogOpenRef.current = dialogOpen;

    // Moves to the chat above or below the open one in the list; with
    // unreadOnly, to the nearest one with unread messages.
    const moveChat = useCallback((step: 1 | -1, unreadOnly: boolean) => {
        const list = visibleRef.current;
        let i = list.findIndex(c => c.roomId === selectedRef.current);
        if (i < 0) i = step > 0 ? -1 : list.length;
        for (let j = i + step; j >= 0 && j < list.length; j += step) {
            if (!unreadOnly || list[j].unread > 0) {
                setSelected(list[j].roomId);
                return;
            }
        }
    }, []);

    // Keyboard shortcuts (see shortcuts.tsx). They use ⌘ on a Mac and Ctrl
    // on Windows, and replace the webview's own (⌘R would reload it).
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (isMod(e) && !e.shiftKey) {
                const key = e.key.toLowerCase();
                const action = {
                    r: refresh,
                    k: () => {
                        searchRef.current?.focus();
                        searchRef.current?.select();
                    },
                    n: () => setShowNewChat(true),
                    ',': () => setShowSettings(true),
                    '/': () => setShowShortcuts(s => !s),
                }[key];
                if (action && (!dialogOpenRef.current || key === 'r' || key === '/')) {
                    e.preventDefault();
                    action();
                }
            } else if (e.altKey && !e.metaKey && !e.ctrlKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
                if (dialogOpenRef.current) return;
                e.preventDefault();
                moveChat(e.key === 'ArrowUp' ? -1 : 1, e.shiftKey);
            }
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [refresh, moveChat]);
    const [theme, setTheme] = useState<Theme>('system');
    // Whether People, Groups and Marketplace are separate tabs (the default)
    // or one list. Message requests stay apart either way.
    const [separateTabs, setSeparateTabs] = useState(true);
    const updateStatus = useUpdateStatus();

    // The saved setting wins over the early copy index.html applied.
    useEffect(() => {
        Go.GetSettings().then(s => {
            const t = (s?.theme as Theme) || 'system';
            applyTheme(t);
            setTheme(t);
            setSeparateTabs(!s?.singleChatList);
        }).catch(console.error);
    }, []);
    const [startError, setStartError] = useState('');
    const [messageTick, setMessageTick] = useState(0);
    // Who is typing in each chat, by room ID.
    const [typing, setTyping] = useState<Record<string, string[]>>({});
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
            if (change.kind === 'typing') {
                Go.Typing().then(t => setTyping(t ?? {})).catch(console.error);
            }
            if (change.kind === 'messages' && change.roomId === selectedRef.current) {
                setMessageTick(t => t + 1);
            }
        });
        const offAccounts = EventsOn('inbox:accounts', loadAccounts);
        // Clicking a "signed you out" notification goes straight to signing in.
        const offSignIn = EventsOn('accounts:signin', openSignIn);
        const offError = EventsOn('inbox:error', (e: string) => setStartError(e));
        // Clicking a desktop notification opens that chat.
        const offOpen = EventsOn('inbox:open', (roomId: string) => showChat(roomId, chatsRef.current));
        // Clicking the "update available" notification opens Settings.
        const offUpdate = EventsOn('update:open', () => setShowSettings(true));
        return () => {
            offChange();
            offAccounts();
            offSignIn();
            offError();
            offOpen();
            offUpdate();
        };
    }, [loadChats, loadAccounts, showChat, openSignIn]);

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
            // Muted chats don't add to the tab counts, like the app badge.
            if (c.unread > 0 && !c.muted && !c.mutedOnNetwork) s.unread++;
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

    // A jump only applies while its chat stays open.
    useEffect(() => {
        if (jump && selected !== jump.roomId) setJump(null);
    }, [selected, jump]);

    const visibleChats = useMemo(() => {
        const q = query.trim().toLowerCase();
        return chats.filter(c => {
            // Search looks across every category, hidden chats included, so
            // nothing is out of reach.
            if (!q && showHidden !== c.hidden) return false;
            if (!q && !showHidden) {
                const cat = c.category || 'inbox';
                if (separateTabs ? cat !== category : (cat === 'requests') !== (category === 'requests')) return false;
            }
            if (filter === 'unread' && c.unread === 0) return false;
            if (filter !== 'all' && filter !== 'unread' && c.network !== filter) return false;
            return !q || c.name.toLowerCase().includes(q) || c.preview.toLowerCase().includes(q);
        });
    }, [chats, category, filter, query, showHidden, separateTabs]);
    visibleRef.current = visibleChats;

    // In the search box, ↑ ↓ pick a result and Enter opens it; Escape
    // clears the search and leaves the box.
    useEffect(() => setSearchIndex(0), [query]);
    const highlighted = query.trim() ? visibleChats[Math.min(searchIndex, visibleChats.length - 1)]?.roomId : undefined;
    const onSearchKey = (e: ReactKeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            const last = visibleChats.length - 1;
            setSearchIndex(i => Math.max(0, Math.min(last, Math.min(i, last) + (e.key === 'ArrowDown' ? 1 : -1))));
        } else if (e.key === 'Enter' && !e.nativeEvent.isComposing) {
            const chat = visibleChats[Math.min(searchIndex, visibleChats.length - 1)];
            if (chat) {
                e.preventDefault();
                showChat(chat.roomId, chats);
                searchRef.current?.blur();
            }
        } else if (e.key === 'Escape') {
            e.preventDefault();
            setQuery('');
            searchRef.current?.blur();
        }
    };

    const selectedChat = chats.find(c => c.roomId === selected);
    const totalUnread = showHidden ? chats.filter(c => c.hidden && c.unread > 0).length
        : !separateTabs && category !== 'requests'
            ? categoryStats.inbox.unread + categoryStats.groups.unread + categoryStats.marketplace.unread
            : categoryStats[category].unread;

    // Hiding the open chat closes it, since it leaves the list you're looking at.
    const setChatHidden = async (roomId: string, hidden: boolean) => {
        await Go.SetChatHidden(roomId, hidden);
        if (selectedRef.current === roomId) setSelected('');
        loadChats();
    };
    const emptyText = accounts.length === 0
        ? 'Connect an account to get started.'
        : showHidden ? 'No hidden chats.'
        : category === 'requests' ? 'No message requests.'
            : !separateTabs ? 'No conversations yet. They\'ll appear as your accounts sync.'
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
                        <button className={`icon-button refresh-button ${refreshing ? 'spinning' : ''}`}
                                title={`Refresh: reconnect all accounts (${navigator.platform.startsWith('Mac') ? '⌘' : 'Ctrl+'}R)`}
                                aria-label="Refresh" onClick={refresh}>
                            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
                                 strokeLinecap="round" strokeLinejoin="round">
                                <path d="M20 11a8 8 0 1 0-2.3 5.7"/>
                                <path d="M20 4v7h-7"/>
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
                    ref={searchRef}
                    className="search"
                    placeholder={`Search (${MOD}K)`}
                    value={query}
                    onChange={e => setQuery(e.target.value)}
                    onKeyDown={onSearchKey}
                />
                {!query.trim() && showHidden && (
                    <button className="requests-row" onClick={() => setShowHidden(false)}>
                        <span>← Back to chats</span>
                        <span className="muted">Hidden chats</span>
                    </button>
                )}
                {separateTabs && !query.trim() && !showHidden && category !== 'requests' && (
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
                {accounts.filter(a => a.needsLogin).map(a => (
                    <button key={a.network + a.id} className="update-banner signed-out-banner" onClick={() => openSignIn(a.network)}>
                        {a.networkName} signed you out{a.name ? ` (${a.name})` : ''}. <span>Sign in again</span>
                    </button>
                ))}
                {updateStatus?.state === 'available' && (
                    <button className="update-banner" onClick={() => setShowSettings(true)}>
                        Pigeonhole {updateStatus.latest?.version} is available. <span>Update</span>
                    </button>
                )}
                {(() => {
                    const list = (
                        <ChatList chats={visibleChats} selected={selected} highlighted={highlighted} onSelect={setSelected}
                                  emptyText={query.trim() ? 'No chats match.' : emptyText} typing={typing}
                                  onSetHidden={(roomId, hidden) => setChatHidden(roomId, hidden).catch(console.error)}
                                  onSetMuted={(roomId, muted) => Go.SetChatMuted(roomId, muted).then(loadChats).catch(console.error)}/>
                    );
                    // While searching, matching messages follow the matching chats.
                    return query.trim() ? (
                        <div className="search-scroll">
                            <div className="results-heading">Chats</div>
                            {list}
                            <MessageResults query={query} onOpen={r => {
                                setJump(j => ({roomId: r.roomId, eventId: r.eventId, seq: (j?.seq ?? 0) + 1}));
                                showChat(r.roomId, chats);
                            }}/>
                        </div>
                    ) : list;
                })()}
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
                    ? <Thread key={selectedChat.roomId + (jump?.roomId === selectedChat.roomId ? ':' + jump.seq : '')}
                              jumpTo={jump?.roomId === selectedChat.roomId ? jump.eventId : undefined}
                              chat={selectedChat} tick={messageTick}
                              typing={typing[selectedChat.roomId] ?? NO_ONE}
                              signedOut={signedOutOf(accounts, selectedChat.network)}
                              onSignIn={() => openSignIn(selectedChat.network)}
                              onSetHidden={hidden => setChatHidden(selectedChat.roomId, hidden)}/>
                    : <div className="empty-thread">
                        <p>Select a conversation</p>
                        {accounts.length === 0 &&
                            <button className="primary" onClick={() => setShowAccounts(true)}>Connect an account</button>}
                    </div>}
            </main>
            {showNewChat && <NewChat onClose={() => setShowNewChat(false)} onOpened={openNewChat}/>}
            {showSettings && <Settings theme={theme} onTheme={setTheme} onClose={() => setShowSettings(false)}
                                       onSeparateTabs={setSeparateTabs}
                                       onShowShortcuts={() => {
                                           setShowSettings(false);
                                           setShowShortcuts(true);
                                       }}
                                       onShowWelcome={() => {
                                           setShowSettings(false);
                                           setShowWelcome(true);
                                       }}/>}
            {showShortcuts && <ShortcutsHelp onClose={() => setShowShortcuts(false)}/>}
            {showWelcome && <Welcome accounts={accounts} onConnect={openSignIn} onFinish={finishWelcome}/>}
            {showAccounts && <Accounts key={signInNetwork} accounts={accounts} signIn={signInNetwork} onClose={() => {
                setShowAccounts(false);
                setSignInNetwork('');
            }}/>}
        </div>
    );
}

function FilterChip(props: {active: boolean; onClick: () => void; children: React.ReactNode}) {
    return <button className={`chip ${props.active ? 'active' : ''}`} onClick={props.onClick}>{props.children}</button>;
}
