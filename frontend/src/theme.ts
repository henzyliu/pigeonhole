export type Theme = 'system' | 'light' | 'dark';

// Mirrored in localStorage so index.html can apply it before first paint;
// the saved setting in Go (settings.json) is the source of truth.
export const THEME_CACHE_KEY = 'pigeonhole.theme';

// "system" leaves data-theme unset so the CSS follows prefers-color-scheme.
export function applyTheme(theme: Theme) {
    const root = document.documentElement;
    if (theme === 'system') {
        delete root.dataset.theme;
    } else {
        root.dataset.theme = theme;
    }
    try {
        localStorage.setItem(THEME_CACHE_KEY, theme);
    } catch {
        // Storage can be unavailable; the Go-side setting still applies.
    }
}
