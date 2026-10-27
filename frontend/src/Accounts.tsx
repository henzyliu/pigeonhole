import {FormEvent, useEffect, useRef, useState} from 'react';
import {BrowserOpenURL, errorText, Go, inbox, NETWORK_LABELS} from './api';
import NetworkIcon from './NetworkIcon';
import {useCloseOnEscape} from './shortcuts';

type View =
    | {kind: 'list'}
    | {kind: 'flows'; network: string; flows: inbox.LoginFlow[]}
    // flows is the network's full list of sign-in methods, for "Other ways to sign in".
    | {kind: 'step'; step: inbox.LoginStep; flows?: inbox.LoginFlow[]}
    | {kind: 'done'; network: string};

// Sign-in methods in recommended order (each network's first one gets the
// Recommended badge). Cookie logins come first for both networks: password
// login from a "new device" tends to trip Meta's checks (Instagram's CAPTCHA
// needs a built-in browser; Messenger can push it into account recovery).
// For Messenger, facebook.com beats messenger.com: messenger.com sends
// two-step verification to facebook.com and hands the login back with
// cross-site cookies, which the login window's WebKit blocks, so it loops.
// WhatsApp links like WhatsApp Web: scan a QR code, or type a pairing code.
// Discord's "token" flow is its browser login, ranked ahead of the "qr"
// flow both networks share; WhatsApp has no "token" flow, so its QR stays first.
const FLOW_ORDER = [
    'facebook', 'messenger', 'messenger-lite', 'messenger-lite-android',
    'instagram', 'instagram-password', 'android',
    'token', 'qr', 'phone',
];

const META_NETWORKS = new Set(['messenger', 'instagram']);

export default function Accounts(props: {accounts: inbox.Account[]; signIn?: string; onClose: () => void}) {
    const [view, setView] = useState<View>({kind: 'list'});
    const [networks, setNetworks] = useState<inbox.LoginFlow[]>([]);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    // The account whose "Log out" is waiting for confirmation.
    const [confirmingLogout, setConfirmingLogout] = useState('');

    useEffect(() => {
        Go.Networks().then(setNetworks);
    }, []);

    const run = async (fn: () => Promise<void>) => {
        setBusy(true);
        setError('');
        try {
            await fn();
        } catch (e) {
            setError(errorText(e));
        } finally {
            setBusy(false);
        }
    };

    // Keeps the list of sign-in methods attached as the login moves through steps.
    const showStep = (step: inbox.LoginStep, flows?: inbox.LoginFlow[]) => {
        if (step.type === 'complete') {
            setView({kind: 'done', network: step.network});
        } else {
            setView(v => ({kind: 'step', step, flows: flows ?? (v.kind === 'step' || v.kind === 'flows' ? v.flows : undefined)}));
        }
    };

    const loadFlows = async (network: string) => {
        const flows = await Go.LoginFlows(network) ?? [];
        return flows.sort((a, b) => rank(a.id) - rank(b.id));
    };

    // Picking a network goes straight to its recommended sign-in (the login
    // window, for cookie methods); the rest are under "Other ways to sign in".
    const pickNetwork = (network: string) => run(async () => {
        const flows = await loadFlows(network);
        if (flows.length === 0) throw new Error(`No sign-in methods available for ${NETWORK_LABELS[network] ?? network}`);
        showStep(await Go.StartLogin(network, flows[0].id), flows);
    });

    const startFlow = (network: string, flowID: string, flows: inbox.LoginFlow[]) =>
        run(async () => showStep(await Go.StartLogin(network, flowID), flows));

    const showOtherWays = (step: inbox.LoginStep, flows?: inbox.LoginFlow[]) => run(async () => {
        Go.CancelLogin(step.sessionId);
        setView({kind: 'flows', network: step.network, flows: flows ?? await loadFlows(step.network)});
    });

    const submit = (step: inbox.LoginStep, values: Record<string, string>) =>
        run(async () => showStep(await Go.SubmitLogin(step.sessionId, values)));

    // Opened to sign in to a network again: start right away. (The same
    // account signing in again takes over its old chats.)
    const signInStarted = useRef(false);
    useEffect(() => {
        if (props.signIn && !signInStarted.current) {
            signInStarted.current = true;
            pickNetwork(props.signIn);
        }
    });

    // Display-and-wait steps (e.g. "approve on your phone") advance on their own.
    useEffect(() => {
        if (view.kind === 'step' && view.step.type === 'display_and_wait') {
            Go.SubmitLogin(view.step.sessionId, {}).then(showStep).catch(e => setError(errorText(e)));
        }
    }, [view]);

    const close = () => {
        if (view.kind === 'step') Go.CancelLogin(view.step.sessionId);
        props.onClose();
    };
    useCloseOnEscape(close);

    return (
        <div className="modal-backdrop" onClick={close}>
            <div className="modal" onClick={e => e.stopPropagation()}>
                <div className="modal-header">
                    <h2>{view.kind === 'list' ? 'Accounts' : `Connect ${NETWORK_LABELS[networkOf(view)] ?? ''}`}</h2>
                    <button className="icon-button" onClick={close}>✕</button>
                </div>
                {/* On login steps the error shows next to Continue instead (see StepForm). */}
                {error && view.kind !== 'step' && <div className="banner error">{error}</div>}

                {view.kind === 'list' && <>
                    {props.accounts.length > 0 && (
                        <ul className="account-list">
                            {props.accounts.map(a => {
                                const key = a.network + a.id;
                                return (
                                    <li key={key} className={confirmingLogout === key ? 'confirming' : ''}>
                                        <NetworkIcon network={a.network} size={22}/>
                                        <div className="account-info">
                                            <div>{a.name || a.id} <span className="muted">· {a.networkName}</span></div>
                                            {confirmingLogout === key
                                                ? <div className="logout-warning">
                                                    This also removes your {a.networkName} chats from this computer.
                                                    They stay on {a.networkName}.
                                                </div>
                                                : <>
                                                    <div className={`account-state ${a.state}`}>{stateLabel(a)}</div>
                                                    {a.historyPending > 0 && (
                                                        <div className="account-syncing">
                                                            Loading history · {a.historyPending} {a.historyPending === 1 ? 'chat' : 'chats'} left
                                                        </div>
                                                    )}
                                                </>}
                                        </div>
                                        {a.needsLogin && confirmingLogout !== key && (
                                            <button className="link" disabled={busy} onClick={() => pickNetwork(a.network)}>Sign in again</button>
                                        )}
                                        {confirmingLogout === key ? <>
                                            <button className="link" disabled={busy} onClick={() => setConfirmingLogout('')}>Cancel</button>
                                            <button className="link danger" disabled={busy} onClick={() => run(async () => {
                                                await Go.Logout(a.network, a.id);
                                                setConfirmingLogout('');
                                            })}>Log out</button>
                                        </> : (
                                            <button className="link" disabled={busy} onClick={() => setConfirmingLogout(key)}>Log out</button>
                                        )}
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                    <p className="muted">Connect an account:</p>
                    <div className="network-grid">
                        {networks.map(n => (
                            <button key={n.id} className="network-button" disabled={busy} onClick={() => pickNetwork(n.id)}>
                                <NetworkIcon network={n.id} size={28}/>{n.name}
                            </button>
                        ))}
                    </div>
                    <p className="fine-print">
                        Your messages stay on this computer. Connecting uses the same unofficial method as the
                        mautrix bridges, so Meta may occasionally ask you to verify the login. WhatsApp links
                        like WhatsApp Web, and unlinks if your phone is offline for about two weeks. Discord
                        covers direct and group messages (not servers); its rules don't allow unofficial
                        clients, so use it at your own risk.
                    </p>
                </>}

                {view.kind === 'flows' && <>
                    <p className="muted">Other ways to sign in</p>
                    {META_NETWORKS.has(view.network) && <p className="fine-print">
                        Username-and-password sign-in is often challenged or blocked by Meta for apps like this.
                        If it fails, use the recommended option, which logs in on the real website.
                    </p>}
                    <div className="flow-list">
                        {view.flows.map((f, i) => (
                            <button key={f.id} className="flow-button" disabled={busy} onClick={() => startFlow(view.network, f.id, view.flows)}>
                                <strong>{f.name}{i === 0 && <span className="pill">Recommended</span>}</strong>
                                <span>{f.description}</span>
                            </button>
                        ))}
                    </div>
                    <button className="link" onClick={() => setView({kind: 'list'})}>← Back</button>
                </>}

                {view.kind === 'step' && <>
                    <StepForm key={stepKey(view.step)} step={view.step} busy={busy} error={error}
                              onSubmit={v => submit(view.step, v)}
                              onBrowserLogin={() => run(async () => showStep(await Go.BrowserLogin(view.step.sessionId)))}
                              onChooseOther={() => showOtherWays(view.step, view.flows)}/>
                    {!busy && (view.flows?.length ?? 0) > 1 && (
                        <button className="link other-ways" onClick={() => showOtherWays(view.step, view.flows)}>
                            Other ways to sign in
                        </button>
                    )}
                </>}

                {view.kind === 'done' && <>
                    <p>Connected! Your conversations will start appearing in a moment.</p>
                    <div className="modal-actions">
                        <button className="primary" onClick={props.onClose}>Done</button>
                    </div>
                </>}
            </div>
        </div>
    );
}

// Each step gets a fresh form; reusing one would carry over the previous
// step's values (e.g. the password) and miss the new step's defaults.
function stepKey(step: inbox.LoginStep): string {
    return [step.type, step.instructions, ...(step.fields ?? []).map(f => f.id), ...(step.cookieNames ?? [])].join('|');
}

function rank(flowID: string) {
    const i = FLOW_ORDER.indexOf(flowID);
    return i === -1 ? FLOW_ORDER.length : i;
}

function networkOf(view: View): string {
    switch (view.kind) {
        case 'flows':
        case 'done':
            return view.network;
        case 'step':
            return view.step.network;
        default:
            return '';
    }
}

function stateLabel(a: inbox.Account): string {
    switch (a.state) {
        case 'connected':
            return 'Connected';
        case 'connecting':
            return 'Connecting…';
        case 'transient_disconnect':
            return 'Reconnecting…';
        case 'bad_credentials':
            return `Signed out by ${a.networkName}`;
        default:
            return a.error || a.state;
    }
}

function StepForm(props: {
    step: inbox.LoginStep;
    busy: boolean;
    error: string;
    onSubmit: (values: Record<string, string>) => void;
    onChooseOther: () => void;
    onBrowserLogin: () => void;
}) {
    const {step} = props;
    // Errors sit right above Continue and scroll into view, since long steps
    // (like pasting cookies) push the top of the dialog off-screen.
    const errorRef = useRef<HTMLDivElement>(null);
    useEffect(() => {
        if (props.error) errorRef.current?.scrollIntoView({block: 'nearest', behavior: 'smooth'});
    }, [props.error]);
    const errorBanner = props.error ? <div ref={errorRef} className="banner error">{props.error}</div> : null;
    const [values, setValues] = useState<Record<string, string>>(() =>
        Object.fromEntries((step.fields ?? []).map(f => [f.id, f.default || f.options?.[0] || ''])));

    const onSubmit = (e: FormEvent) => {
        e.preventDefault();
        props.onSubmit(values);
    };

    // Some steps (like Instagram's CAPTCHA) only work inside an embedded
    // browser. Pasting can't satisfy them, so explain and offer a way out.
    if (step.type === 'cookies' && step.browserAvailable) {
        const site = step.cookieUrl ? new URL(step.cookieUrl).hostname.replace(/^www\./, '') : 'the website';
        return <div className="step">
            {step.instructions && <p>{step.instructions}</p>}
            <p>A login window will open on <strong>{site}</strong>. Log in as usual, including any security
                checks; the window closes by itself once you're in.</p>
            {errorBanner}
            <div className="modal-actions">
                <button className="primary" onClick={props.onBrowserLogin} disabled={props.busy}>
                    {props.busy ? 'Waiting for you to log in…' : 'Log in in a window'}
                </button>
            </div>
            {!step.needsBrowser && !props.busy && (
                <details className="paste-fallback">
                    <summary>Or paste cookies instead</summary>
                    <PasteCookies step={step} busy={props.busy} onSubmit={props.onSubmit}/>
                </details>
            )}
        </div>;
    }

    if (step.type === 'cookies' && step.needsBrowser) {
        return <div className="step">
            {step.instructions && <p className="muted">{NETWORK_LABELS[step.network] ?? step.network} says: “{step.instructions}”</p>}
            <p>
                This check needs a built-in browser, which Pigeonhole doesn't have yet, so it can't be
                finished here. Instead, log in on the website in your own browser and use the
                <strong> Cookies</strong> sign-in option to bring that login across.
            </p>
            <div className="modal-actions">
                <button className="primary" onClick={props.onChooseOther}>Choose another sign-in method</button>
            </div>
        </div>;
    }

    // WhatsApp can ask to confirm a new linked device with a passkey, which
    // needs the system's passkey prompt; the other sign-in method avoids it.
    if (step.type === 'webauthn') {
        return <div className="step">
            <p>{NETWORK_LABELS[step.network] ?? step.network} asked to confirm this login with a passkey, which
                Pigeonhole can't do yet.</p>
            <div className="modal-actions">
                <button className="primary" onClick={props.onChooseOther}>Choose another sign-in method</button>
            </div>
        </div>;
    }

    if (step.type === 'display_and_wait') {
        return <div className="step">
            <p>{step.instructions}</p>
            {step.qrImage
                ? <img className="qr-code" src={step.qrImage} alt="QR code to scan"/>
                : step.displayData && step.displayType !== 'nothing' && <pre className="code-display">{step.displayData}</pre>}
            <p className="muted">Waiting…</p>
            {errorBanner}
        </div>;
    }

    return (
        <form className="step" onSubmit={onSubmit}>
            {step.type === 'cookies' ? <>
                {step.instructions && <p>{step.instructions}</p>}
                <CookieInstructions step={step}/>
                <textarea className="cookie-input" rows={6} placeholder="name=value; name=value…  or a curl command"
                          value={values.cookies ?? ''} onChange={e => setValues({cookies: e.target.value})} autoFocus/>
            </> : <>
                {step.instructions && <p>{step.instructions}</p>}
                {(step.fields ?? []).map((f, i) => f.options?.length ? (
                    <fieldset key={f.id} className="field choice-field">
                        <legend>{f.name}</legend>
                        {f.options.map(option => (
                            <label key={option} className={`choice ${values[f.id] === option ? 'selected' : ''}`}>
                                <input type="radio" name={f.id} value={option} checked={values[f.id] === option}
                                       onChange={() => setValues(v => ({...v, [f.id]: option}))}/>
                                {option}
                            </label>
                        ))}
                        {f.description && <small className="muted">{f.description}</small>}
                    </fieldset>
                ) : (
                    <label key={f.id} className="field">
                        <span>{f.name}</span>
                        <input type={f.type === 'password' ? 'password' : f.type === 'email' ? 'email' : f.type === 'phone_number' ? 'tel' : 'text'}
                               value={values[f.id] ?? ''} required={!f.optional} autoFocus={i === 0}
                               autoComplete={f.type === 'password' ? 'current-password' : 'off'}
                               onChange={e => setValues(v => ({...v, [f.id]: e.target.value}))}/>
                        {f.description && <small className="muted">{f.description}</small>}
                    </label>
                ))}
            </>}
            {errorBanner}
            <div className="modal-actions">
                <button className="primary" type="submit" disabled={props.busy}>{props.busy ? 'Working…' : 'Continue'}</button>
            </div>
        </form>
    );
}

function CookieInstructions({step}: {step: inbox.LoginStep}) {
    return <>
        <ol className="instructions">
            <li>Open <button type="button" className="link" onClick={() => BrowserOpenURL(step.cookieUrl)}>{step.cookieUrl}</button> in your browser and log in.</li>
            <li>Open developer tools (⌥⌘I on Mac, F12 on Windows) → Network tab, then reload the page.</li>
            <li>Click the first request and copy the value of the <strong>cookie</strong> request header
                (or right-click it → Copy → <strong>Copy as cURL</strong>), and paste it below.</li>
        </ol>
        <p className="muted">Needed cookies: {(step.cookieNames ?? []).join(', ')}</p>
    </>;
}

// The manual fallback for cookie steps when using the login window isn't wanted.
function PasteCookies(props: {step: inbox.LoginStep; busy: boolean; onSubmit: (values: Record<string, string>) => void}) {
    const [cookies, setCookies] = useState('');
    return (
        <form className="step" onSubmit={e => {
            e.preventDefault();
            props.onSubmit({cookies});
        }}>
            <CookieInstructions step={props.step}/>
            <textarea className="cookie-input" rows={5} placeholder="name=value; name=value…  or a curl command"
                      value={cookies} onChange={e => setCookies(e.target.value)}/>
            <div className="modal-actions">
                <button type="submit" disabled={props.busy || !cookies.trim()}>Continue with pasted cookies</button>
            </div>
        </form>
    );
}
