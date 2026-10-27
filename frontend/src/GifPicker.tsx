import {UIEvent, useEffect, useRef, useState} from 'react';
import {errorText, gifs, Go} from './api';

// The GIF picker: trending GIFs, or GIPHY search results, in two columns.
// GIPHY is only contacted while this is open.
export default function GifPicker(props: {onPick: (gif: gifs.GIF) => void; onClose: () => void}) {
    const [query, setQuery] = useState('');
    const [results, setResults] = useState<gifs.GIF[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [hasMore, setHasMore] = useState(true);
    const ref = useRef<HTMLDivElement>(null);
    // Which search the results belong to, so a slow answer to an old one
    // doesn't overwrite a newer one.
    const searchRef = useRef(0);
    const {onClose} = props;

    const load = async (q: string, offset: number) => {
        const search = offset === 0 ? ++searchRef.current : searchRef.current;
        setLoading(true);
        setError('');
        try {
            const page = await Go.SearchGIFs(q, offset) ?? [];
            if (search !== searchRef.current) return;
            setResults(prev => offset === 0 ? page : [...prev, ...page.filter(g => !prev.some(p => p.id === g.id))]);
            setHasMore(page.length > 0);
        } catch (e) {
            if (search === searchRef.current) setError(errorText(e));
        } finally {
            if (search === searchRef.current) setLoading(false);
        }
    };

    // Search as you type, once typing pauses.
    useEffect(() => {
        const timer = window.setTimeout(() => {
            ref.current?.querySelector('.gif-grid')?.scrollTo({top: 0});
            load(query, 0);
        }, query ? 350 : 0);
        return () => window.clearTimeout(timer);
    }, [query]);

    useEffect(() => {
        const onDown = (e: MouseEvent) => {
            const target = e.target as Element;
            if (!ref.current?.contains(target) && !target.closest?.('.gif-button')) onClose();
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        document.addEventListener('mousedown', onDown);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onDown);
            document.removeEventListener('keydown', onKey);
        };
    }, [onClose]);

    // More results as you scroll near the end.
    const onScroll = (e: UIEvent<HTMLDivElement>) => {
        const el = e.currentTarget;
        if (!loading && hasMore && el.scrollHeight - el.scrollTop - el.clientHeight < 300) {
            load(query, results.length);
        }
    };

    // Two columns, each GIF going into the shorter one, so the grid stays even.
    const columns: gifs.GIF[][] = [[], []];
    const heights = [0, 0];
    for (const g of results) {
        const col = heights[0] <= heights[1] ? 0 : 1;
        columns[col].push(g);
        heights[col] += g.previewWidth > 0 ? g.previewHeight / g.previewWidth : 1;
    }

    return (
        <div ref={ref} className="gif-picker" role="dialog" aria-label="Send a GIF">
            <input className="search" autoFocus placeholder="Search GIPHY" value={query}
                   onChange={e => setQuery(e.target.value)}/>
            <div className="gif-grid" onScroll={onScroll}>
                {columns.map((col, i) => (
                    <div key={i} className="gif-column">
                        {col.map(g => (
                            <button key={g.id} className="gif" title={g.title || 'GIF'} onClick={() => props.onPick(g)}
                                    style={{aspectRatio: g.previewWidth > 0 && g.previewHeight > 0 ? `${g.previewWidth} / ${g.previewHeight}` : '1'}}>
                                <img src={g.preview} alt={g.title || 'GIF'} loading="lazy"/>
                            </button>
                        ))}
                    </div>
                ))}
                {!loading && !error && results.length === 0 && <p className="muted gif-empty">No GIFs found.</p>}
            </div>
            {error && <div className="banner error">{error}</div>}
            <div className="gif-footer">Powered by GIPHY</div>
        </div>
    );
}
