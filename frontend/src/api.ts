// Thin re-export of the generated Wails bindings so components don't import
// from the generated folder directly.
export * as Go from '../wailsjs/go/main/App';
export {gifs, inbox} from '../wailsjs/go/models';
export {EventsOn, BrowserOpenURL, OnFileDrop, OnFileDropOff} from '../wailsjs/runtime/runtime';

export const NETWORK_LABELS: Record<string, string> = {
    messenger: 'Messenger',
    instagram: 'Instagram',
    whatsapp: 'WhatsApp',
    discord: 'Discord',
    gmessages: 'Google Messages',
};

export function errorText(err: unknown): string {
    if (typeof err === 'string') return err;
    if (err instanceof Error) return err.message;
    return String(err);
}

export function formatTime(ts: number): string {
    if (!ts) return '';
    const d = new Date(ts);
    const now = new Date();
    if (d.toDateString() === now.toDateString()) {
        return d.toLocaleTimeString([], {hour: 'numeric', minute: '2-digit'});
    }
    const days = (now.getTime() - d.getTime()) / 86_400_000;
    if (days < 7) return d.toLocaleDateString([], {weekday: 'short'});
    return d.toLocaleDateString([], {month: 'short', day: 'numeric'});
}

export function initials(name: string): string {
    const parts = name.trim().split(/\s+/).filter(Boolean);
    return (parts[0]?.[0] ?? '?').toUpperCase() + (parts[1]?.[0] ?? '').toUpperCase();
}
