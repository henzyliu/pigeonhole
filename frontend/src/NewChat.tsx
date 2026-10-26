import {useEffect, useRef, useState} from 'react';
import {errorText, Go, inbox} from './api';
import Avatar from './Avatar';

const SEARCH_DELAY_MS = 350;

export default function NewChat(props: {onClose: () => void; onOpened: (roomId: string) => void}) {
    const [query, setQuery] = useState('');
    const [results, setResults] = useState<inbox.Person[] | null>(null);
    const [searching, setSearching] = useState(false);
    const [opening, setOpening] = useState('');
    const [error, setError] = useState('');
    // Results can arrive out of order while typing; only keep the latest.
    const searchSeq = useRef(0);

    useEffect(() => {
        const q = query.trim();
        if (q.length < 2) {
            setResults(null);
            setSearching(false);
            return;
        }
        const seq = ++searchSeq.current;
        setSearching(true);
        const timer = window.setTimeout(() => {
            Go.SearchPeople(q)
                .then(people => {
                    if (seq !== searchSeq.current) return;
                    setResults(people ?? []);
                    setError('');
                })
                .catch(e => seq === searchSeq.current && setError(errorText(e)))
                .finally(() => seq === searchSeq.current && setSearching(false));
        }, SEARCH_DELAY_MS);
        return () => window.clearTimeout(timer);
    }, [query]);

    const open = async (p: inbox.Person) => {
        setOpening(p.network + p.id);
        setError('');
        try {
            props.onOpened(p.roomId || await Go.OpenChat(p.network, p.loginId, p.id));
        } catch (e) {
            setError(errorText(e));
        } finally {
            setOpening('');
        }
    };

    return (
        <div className="modal-backdrop" onClick={props.onClose}>
            <div className="modal" onClick={e => e.stopPropagation()}>
                <div className="modal-header">
                    <h2>New chat</h2>
                    <button className="icon-button" onClick={props.onClose}>✕</button>
                </div>
                <input className="search modal-search" placeholder="Search people by name" value={query}
                       onChange={e => setQuery(e.target.value)} autoFocus/>
                {error && <div className="banner error">{error}</div>}
                <ul className="people-list">
                    {results?.map(p => {
                        const key = p.network + p.id;
                        return (
                            <li key={key}>
                                <button className="person-row" disabled={!!opening} onClick={() => open(p)}>
                                    <Avatar name={p.name} src={p.avatar} size={36} network={p.network}/>
                                    <span className="person-name">{p.name}</span>
                                    <span className="muted person-action">
                                        {opening === key ? 'Opening…' : p.roomId ? 'Open chat' : p.networkName}
                                    </span>
                                </button>
                            </li>
                        );
                    })}
                </ul>
                {searching && <p className="muted">Searching…</p>}
                {!searching && results?.length === 0 && <p className="muted">No one found.</p>}
                {results === null && !searching && (
                    <p className="fine-print">
                        Find someone on any of your connected accounts to start or reopen a chat with them.
                        They aren't notified unless you send a message. On Messenger and Instagram, opening a
                        new chat may move it to the top of your list on your other devices.
                    </p>
                )}
            </div>
        </div>
    );
}
