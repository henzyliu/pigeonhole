import {useEffect, useState} from 'react';
import {errorText, Go} from './api';
import {main} from '../wailsjs/go/models';
import {applyTheme, Theme} from './theme';

const THEMES: {id: Theme; label: string}[] = [
    {id: 'system', label: 'System'},
    {id: 'light', label: 'Light'},
    {id: 'dark', label: 'Dark'},
];

export default function Settings(props: {theme: Theme; onTheme: (t: Theme) => void; onClose: () => void}) {
    const [error, setError] = useState('');
    const [version, setVersion] = useState<main.VersionInfo | null>(null);

    useEffect(() => {
        Go.Version().then(setVersion).catch(console.error);
    }, []);

    // Switch instantly, then save; put it back if saving fails.
    const choose = async (theme: Theme) => {
        const previous = props.theme;
        setError('');
        applyTheme(theme);
        props.onTheme(theme);
        try {
            await Go.SetTheme(theme);
        } catch (e) {
            applyTheme(previous);
            props.onTheme(previous);
            setError(errorText(e));
        }
    };

    return (
        <div className="modal-backdrop" onClick={props.onClose}>
            <div className="modal" onClick={e => e.stopPropagation()}>
                <div className="modal-header">
                    <h2>Settings</h2>
                    <button className="icon-button" onClick={props.onClose}>✕</button>
                </div>
                {error && <div className="banner error">{error}</div>}
                <div className="setting">
                    <div>
                        <div className="setting-label">Appearance</div>
                        <div className="muted setting-hint">System follows your computer's light or dark mode.</div>
                    </div>
                    <div className="tabs setting-choice" role="radiogroup" aria-label="Appearance">
                        {THEMES.map(t => (
                            <button key={t.id} role="radio" aria-checked={props.theme === t.id}
                                    className={`tab ${props.theme === t.id ? 'active' : ''}`} onClick={() => choose(t.id)}>
                                <span className="tab-label">{t.label}</span>
                            </button>
                        ))}
                    </div>
                </div>
                {version && (
                    <div className="setting about">
                        <div>
                            <div className="setting-label">About</div>
                            <div className="muted setting-hint">{buildLine(version)}</div>
                        </div>
                        <div className="about-version">Pigeonhole {version.version}</div>
                    </div>
                )}
            </div>
        </div>
    );
}

// Which build this is, e.g. "Build ba8c578 · main · 24 Oct 2026". Builds
// not made by scripts/build.sh (like wails dev) don't carry these.
function buildLine(v: main.VersionInfo): string {
    if (!v.commit) return 'Development build';
    const date = v.buildDate ? new Date(v.buildDate + 'T00:00:00').toLocaleDateString([], {dateStyle: 'medium'}) : '';
    return ['Build ' + v.commit, v.branch, date].filter(Boolean).join(' · ');
}
