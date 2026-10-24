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
    const [prefs, setPrefs] = useState<main.Settings | null>(null);
    const [permission, setPermission] = useState<main.NotificationPermission | null>(null);

    useEffect(() => {
        Go.Version().then(setVersion).catch(console.error);
        Go.GetSettings().then(setPrefs).catch(console.error);
    }, []);

    // Re-checked when you come back from the system's settings.
    useEffect(() => {
        const check = () => Go.NotificationPermission().then(setPermission).catch(console.error);
        check();
        window.addEventListener('focus', check);
        return () => window.removeEventListener('focus', check);
    }, []);

    // Like choose: switch at once, save, put it back if saving fails.
    const setPref = async (change: Partial<main.Settings>, save: () => Promise<void>) => {
        const previous = prefs;
        setError('');
        setPrefs(p => p && main.Settings.createFrom({...p, ...change}));
        try {
            await save();
        } catch (e) {
            setPrefs(previous);
            setError(errorText(e));
        }
    };
    const notificationsOn = prefs ? !prefs.notificationsOff : true;

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
                {prefs && <>
                    <div className="setting setting-group">
                        <div>
                            <div className="setting-label">Notifications</div>
                            <div className="muted setting-hint">An alert for each new message, except in the chat you have open.</div>
                        </div>
                        <Switch label="Notifications" on={notificationsOn}
                                onChange={on => setPref({notificationsOff: !on}, () => Go.SetNotifications(on))}/>
                    </div>
                    <div className={`setting ${notificationsOn ? '' : 'disabled'}`}>
                        <div>
                            <div className="setting-label">Hide message text</div>
                            <div className="muted setting-hint">Alerts say “{HIDDEN_TEXT}” instead of what was sent.</div>
                        </div>
                        <Switch label="Hide message text" on={prefs.hideMessageText} disabled={!notificationsOn}
                                onChange={hide => setPref({hideMessageText: hide}, () => Go.SetHideMessageText(hide))}/>
                    </div>
                    {notificationsOn && permission && <PermissionNote permission={permission}/>}
                </>}
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

const HIDDEN_TEXT = 'New message';

function Switch(props: {label: string; on: boolean; disabled?: boolean; onChange: (on: boolean) => void}) {
    return (
        <button role="switch" aria-checked={props.on} aria-label={props.label} disabled={props.disabled}
                className={`switch ${props.on ? 'on' : ''}`} onClick={() => props.onChange(!props.on)}>
            <span className="switch-knob"/>
        </button>
    );
}

// Whether the system is letting Pigeonhole's alerts through. Turning them off
// there overrides the switch above, so say so and link to the right page.
function PermissionNote({permission}: {permission: main.NotificationPermission}) {
    const open = () => Go.OpenNotificationSettings().catch(console.error);
    if (permission.status === 'blocked') {
        return (
            <div className="banner warning setting-banner">
                {permission.system} is blocking Pigeonhole's notifications.
                <button className="link" onClick={open}>Open {permission.system} settings</button>
            </div>
        );
    }
    if (permission.status === 'allowed') {
        return (
            <div className="muted setting-hint setting-note">
                Allowed by {permission.system}. Sounds, banners and previews are set in{' '}
                <button className="link" onClick={open}>{permission.system} settings</button>.
            </div>
        );
    }
    return null;
}

// Which build this is, e.g. "Build ba8c578 · main · 24 Oct 2026". Builds
// not made by scripts/build.sh (like wails dev) don't carry these.
function buildLine(v: main.VersionInfo): string {
    if (!v.commit) return 'Development build';
    const date = v.buildDate ? new Date(v.buildDate + 'T00:00:00').toLocaleDateString([], {dateStyle: 'medium'}) : '';
    return ['Build ' + v.commit, v.branch, date].filter(Boolean).join(' · ');
}
