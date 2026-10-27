import {useEffect, useState} from 'react';
import {BrowserOpenURL, errorText, Go, inbox} from './api';
import NetworkIcon from './NetworkIcon';
import {main} from '../wailsjs/go/models';
import {applyTheme, Theme} from './theme';
import {UpdateRow, useUpdateStatus} from './updates';
import {MOD, useCloseOnEscape} from './shortcuts';

const THEMES: {id: Theme; label: string}[] = [
    {id: 'system', label: 'System'},
    {id: 'light', label: 'Light'},
    {id: 'dark', label: 'Dark'},
];

export default function Settings(props: {theme: Theme; onTheme: (t: Theme) => void; onClose: () => void; onShowShortcuts: () => void; onShowWelcome: () => void; onSeparateTabs: (on: boolean) => void}) {
    useCloseOnEscape(props.onClose);
    const [error, setError] = useState('');
    const [version, setVersion] = useState<main.VersionInfo | null>(null);
    const [prefs, setPrefs] = useState<main.Settings | null>(null);
    const [permission, setPermission] = useState<main.NotificationPermission | null>(null);
    const [startAtLogin, setStartAtLogin] = useState<main.StartAtLogin | null>(null);
    const [accounts, setAccounts] = useState<inbox.Account[]>([]);
    const updateStatus = useUpdateStatus();

    useEffect(() => {
        Go.Version().then(setVersion).catch(console.error);
        Go.GetSettings().then(setPrefs).catch(console.error);
        Go.StartAtLogin().then(setStartAtLogin).catch(console.error);
        Go.Accounts().then(a => setAccounts(a ?? [])).catch(console.error);
    }, []);

    const chooseStartAtLogin = async (on: boolean) => {
        const previous = startAtLogin;
        setError('');
        setStartAtLogin(s => s && main.StartAtLogin.createFrom({...s, enabled: on}));
        try {
            await Go.SetStartAtLogin(on);
        } catch (e) {
            setStartAtLogin(previous);
            setError(errorText(e));
        }
    };

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
    const accountKey = (a: inbox.Account) => `${a.network}/${a.id}`;
    const accountOn = (a: inbox.Account) => !(prefs?.mutedAccounts ?? []).includes(accountKey(a));
    const setAccountOn = (a: inbox.Account, on: boolean) => {
        const others = (prefs?.mutedAccounts ?? []).filter(k => k !== accountKey(a));
        return setPref({mutedAccounts: on ? others : [...others, accountKey(a)]},
            () => Go.SetAccountNotifications(a.network, a.id, on));
    };

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
                <div className="settings-links">
                    <button className="link" onClick={props.onShowShortcuts}>Keyboard shortcuts ({MOD}/)</button>
                    <button className="link" onClick={props.onShowWelcome}>Welcome tour</button>
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
                    <div className="setting">
                        <div>
                            <div className="setting-label">Separate chat tabs</div>
                            <div className="muted setting-hint">
                                People, Groups and Marketplace as tabs. Turn off to see every chat in one list
                                (message requests stay separate).
                            </div>
                        </div>
                        <Switch label="Separate chat tabs" on={!prefs.singleChatList}
                                onChange={on => {
                                    // The chat list changes straight away, and changes back
                                    // if the setting can't be saved.
                                    props.onSeparateTabs(on);
                                    setPref({singleChatList: !on}, () => Go.SetSeparateChats(on).catch(e => {
                                        props.onSeparateTabs(!on);
                                        throw e;
                                    }));
                                }}/>
                    </div>
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
                    {accounts.length > 0 && (
                        <div className={`setting-accounts ${notificationsOn ? '' : 'disabled'}`}>
                            <div className="muted setting-hint">Alerts from each account (chats still sync and show as unread):</div>
                            {accounts.map(a => (
                                <div key={accountKey(a)} className="setting setting-account">
                                    <div className="setting-account-name">
                                        <NetworkIcon network={a.network} size={16}/>
                                        <span>{a.name || a.id}</span>
                                        <span className="muted">· {a.networkName}</span>
                                    </div>
                                    <Switch label={`Alerts from ${a.name || a.id} on ${a.networkName}`} on={accountOn(a)}
                                            disabled={!notificationsOn} onChange={on => setAccountOn(a, on)}/>
                                </div>
                            ))}
                        </div>
                    )}
                    {notificationsOn && permission && <PermissionNote permission={permission}/>}
                </>}
                {startAtLogin?.supported && (
                    <div className="setting setting-group">
                        <div>
                            <div className="setting-label">Start at login</div>
                            <div className="muted setting-hint">
                                Opens quietly in the {trayPlace(startAtLogin.system)} when you log in. Closing the window
                                keeps Pigeonhole running there; quit it from its icon.
                            </div>
                        </div>
                        <Switch label="Start at login" on={startAtLogin.enabled} onChange={chooseStartAtLogin}/>
                    </div>
                )}
                {version && (
                    <div className="setting about">
                        <div>
                            <div className="setting-label">About</div>
                            <div className="muted setting-hint">{buildLine(version)}</div>
                            <LicenceNotice/>
                        </div>
                        <div className="about-version">Pigeonhole {version.version}</div>
                    </div>
                )}
                <UpdateRow status={updateStatus}/>
            </div>
        </div>
    );
}

const HIDDEN_TEXT = 'New message';

const SOURCE_URL = 'https://github.com/henzyliu/pigeonhole';

// The AGPL's "appropriate legal notices": the licence, no warranty, and
// where to get the source.
function LicenceNotice() {
    const open = (url: string) => () => BrowserOpenURL(url);
    return (
        <div className="muted setting-hint about-licence">
            Free software under the GNU AGPL v3 or later, with no warranty.{' '}
            <button className="link" onClick={open(SOURCE_URL)}>Source code</button>
            {' · '}
            <button className="link" onClick={open(SOURCE_URL + '/blob/main/LICENSE')}>Licence</button>
        </div>
    );
}

// Where the app's icon lives while it runs in the background.
function trayPlace(system: string): string {
    return system === 'Windows' ? 'notification area' : 'menu bar';
}

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
