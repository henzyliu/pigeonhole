import {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import data from '@emoji-mart/data';
import {Picker} from 'emoji-mart';

// The full emoji picker, for reacting with any emoji. It floats beside the
// button that opened it (above it when there's room), kept inside the window.
// The emoji list is bundled, so it never downloads anything.
export default function EmojiPicker(props: {anchor: DOMRect; onPick: (emoji: string) => void; onClose: () => void}) {
    const ref = useRef<HTMLDivElement>(null);
    const [pos, setPos] = useState<{left: number; top: number} | null>(null);
    const {onClose} = props;
    // The picker is made once, so it calls the latest onPick through this.
    const onPick = useRef(props.onPick);
    onPick.current = props.onPick;

    useEffect(() => {
        const theme = document.documentElement.dataset.theme;
        const picker = new Picker({
            data, locale: 'en', set: 'native', autoFocus: true,
            theme: theme === 'dark' || theme === 'light' ? theme : 'auto',
            previewPosition: 'none', skinTonePosition: 'search', maxFrequentRows: 2,
            onEmojiSelect: (e: {native?: string}) => e.native && onPick.current(e.native),
        });
        ref.current?.replaceChildren(picker as unknown as Node);
    }, []);

    // Measure once the picker has drawn, then place it.
    useLayoutEffect(() => {
        const el = ref.current!;
        const place = () => {
            const margin = 8, w = el.offsetWidth || 352, h = el.offsetHeight || 380;
            const a = props.anchor;
            const top = a.top - h - 6 >= margin ? a.top - h - 6 : Math.min(a.bottom + 6, window.innerHeight - h - margin);
            const left = Math.min(Math.max(margin, a.left + a.width / 2 - w / 2), window.innerWidth - w - margin);
            setPos({left, top: Math.max(margin, top)});
        };
        place();
        const observer = new ResizeObserver(place);
        observer.observe(el);
        return () => observer.disconnect();
    }, [props.anchor]);

    useEffect(() => {
        const onDown = (e: MouseEvent) => {
            if (!ref.current?.contains(e.target as Node)) onClose();
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

    return createPortal(
        <div ref={ref} className="emoji-picker" role="dialog" aria-label="Choose a reaction"
             style={pos ? {left: pos.left, top: pos.top} : {visibility: 'hidden'}}/>,
        document.body,
    );
}
