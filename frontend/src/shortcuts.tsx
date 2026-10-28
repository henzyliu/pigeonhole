import {KeyboardEvent as ReactKeyboardEvent, useEffect} from 'react';

// Keyboard shortcuts use ⌘ on a Mac and Ctrl elsewhere.
export const IS_MAC = navigator.platform.toUpperCase().startsWith('MAC');
export const MOD = IS_MAC ? '⌘' : 'Ctrl+';
const ALT = IS_MAC ? '⌥' : 'Alt+';

// isMod reports whether ⌘ (Mac) or Ctrl (elsewhere) is held, and only that.
export function isMod(e: KeyboardEvent | ReactKeyboardEvent): boolean {
    return (IS_MAC ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey) && !e.altKey;
}

export const SHORTCUTS: {keys: string; what: string}[] = [
    {keys: `${MOD}K`, what: 'Jump to a chat (then ↑ ↓ and Enter)'},
    {keys: `${MOD}F`, what: 'Search the open chat (then Enter for older matches)'},
    {keys: `${MOD}N`, what: 'New chat'},
    {keys: `${ALT}↑  ${ALT}↓`, what: 'Previous or next chat'},
    {keys: `${ALT}⇧↑  ${ALT}⇧↓`, what: 'Previous or next unread chat'},
    {keys: `${MOD}R`, what: 'Refresh: reconnect all accounts'},
    {keys: `${MOD},`, what: 'Settings'},
    {keys: `${MOD}/`, what: 'Show these shortcuts'},
    {keys: '↑', what: 'Edit your last message (in an empty message box)'},
    {keys: 'Esc', what: 'Close a dialog, or stop editing'},
];

// The list of shortcuts, opened with ⌘/ (Ctrl+/).
export function ShortcutsHelp(props: {onClose: () => void}) {
    useCloseOnEscape(props.onClose);
    return (
        <div className="modal-backdrop" onClick={props.onClose}>
            <div className="modal shortcuts-modal" onClick={e => e.stopPropagation()}>
                <div className="modal-header">
                    <h2>Keyboard shortcuts</h2>
                    <button className="icon-button" onClick={props.onClose}>✕</button>
                </div>
                <dl className="shortcut-list">
                    {SHORTCUTS.map(s => (
                        <div key={s.what}>
                            <dt>{s.keys.split('  ').map(k => <kbd key={k}>{k}</kbd>)}</dt>
                            <dd>{s.what}</dd>
                        </div>
                    ))}
                </dl>
            </div>
        </div>
    );
}

// useCloseOnEscape closes a dialog on Escape, unless something inside it
// (a picker, say) already handled the key.
export function useCloseOnEscape(onClose: () => void) {
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape' && !e.defaultPrevented) onClose();
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [onClose]);
}
