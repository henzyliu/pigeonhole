import {FormEvent, useEffect, useState} from 'react';
import {BrowserOpenURL, errorText, Go, inbox, NETWORK_LABELS} from './api';
import NetworkIcon from './NetworkIcon';

type View =
    | {kind: 'list'}
    | {kind: 'flows'; network: string; flows: inbox.LoginFlow[]}
    | {kind: 'step'; step: inbox.LoginStep}
    | {kind: 'done'; network: string};

// Recommended flows first: password login is the least fiddly for most people.
const FLOW_ORDER = ['messenger-lite', 'instagram-password', 'android', 'messenger-lite-android', 'messenger', 'instagram', 'facebook'];

export default function Accounts(props: {accounts: inbox.Account[]; onClose: () => void}) {
    const [view, setView] = useState<View>({kind: 'list'});
    const [networks, setNetworks] = useState<inbox.LoginFlow[]>([]);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

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

    const showStep = (step: inbox.LoginStep) => {
        if (step.type === 'complete') {
            setView({kind: 'done', network: step.network});
        } else {
            setView({kind: 'step', step});
        }
    };

    const pickNetwork = (network: string) => run(async () => {
        const flows = await Go.LoginFlows(network) ?? [];
        flows.sort((a, b) => rank(a.id) - rank(b.id));
        setView({kind: 'flows', network, flows});
    });

    const startFlow = (network: string, flowID: string) => run(async () => showStep(await Go.StartLogin(network, flowID)));

    const submit = (step: inbox.LoginStep, values: Record<string, string>) =>
        run(async () => showStep(await Go.SubmitLogin(step.sessionId, values)));

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

    return (
        <div className="modal-backdrop" onClick={close}>
            <div className="modal" onClick={e => e.stopPropagation()}>
                <div className="modal-header">
                    <h2>{view.kind === 'list' ? 'Accounts' : `Connect ${NETWORK_LABELS[networkOf(view)] ?? ''}`}</h2>
                    <button className="icon-button" onClick={close}>✕</button>
                </div>
                {error && <div className="banner error">{error}</div>}

                {view.kind === 'list' && <>
                    {props.accounts.length > 0 && (
                        <ul className="account-list">
                            {props.accounts.map(a => (
                                <li key={a.network + a.id}>
                                    <NetworkIcon network={a.network} size={22}/>
                                    <div className="account-info">
                                        <div>{a.name || a.id} <span className="muted">· {a.networkName}</span></div>
                                        <div className={`account-state ${a.state}`}>{stateLabel(a)}</div>
                                    </div>
                                    <button className="link" disabled={busy}
                                            onClick={() => run(() => Go.Logout(a.network, a.id))}>Log out</button>
                                </li>
                            ))}
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
                        mautrix bridges, so Meta may occasionally ask you to verify the login.
                    </p>
                </>}

                {view.kind === 'flows' && <>
                    <p className="muted">How do you want to sign in?</p>
                    <div className="flow-list">
                        {view.flows.map((f, i) => (
                            <button key={f.id} className="flow-button" disabled={busy} onClick={() => startFlow(view.network, f.id)}>
                                <strong>{f.name}{i === 0 && <span className="pill">Recommended</span>}</strong>
                                <span>{f.description}</span>
                            </button>
                        ))}
                    </div>
                    <button className="link" onClick={() => setView({kind: 'list'})}>← Back</button>
                </>}

                {view.kind === 'step' && <StepForm key={stepKey(view.step)} step={view.step} busy={busy}
                                                   onSubmit={v => submit(view.step, v)}/>}

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
            return 'Signed out — please log in again';
        default:
            return a.error || a.state;
    }
}

function StepForm(props: {step: inbox.LoginStep; busy: boolean; onSubmit: (values: Record<string, string>) => void}) {
    const {step} = props;
    const [values, setValues] = useState<Record<string, string>>(() =>
        Object.fromEntries((step.fields ?? []).map(f => [f.id, f.default || f.options?.[0] || ''])));

    const onSubmit = (e: FormEvent) => {
        e.preventDefault();
        props.onSubmit(values);
    };

    if (step.type === 'display_and_wait') {
        return <div className="step">
            <p>{step.instructions}</p>
            {step.displayData && step.displayType !== 'nothing' && <pre className="code-display">{step.displayData}</pre>}
            <p className="muted">Waiting…</p>
        </div>;
    }

    return (
        <form className="step" onSubmit={onSubmit}>
            {step.type === 'cookies' ? <>
                <ol className="instructions">
                    <li>Open <button type="button" className="link" onClick={() => BrowserOpenURL(step.cookieUrl)}>{step.cookieUrl}</button> in your browser and log in.</li>
                    <li>Open developer tools (⌥⌘I on Mac, F12 on Windows) → Network tab, and reload the page.</li>
                    <li>Right-click the first request → Copy → <strong>Copy as cURL</strong>, and paste it below.</li>
                </ol>
                <p className="muted">Needed cookies: {(step.cookieNames ?? []).join(', ')}</p>
                <textarea className="cookie-input" rows={6} placeholder="curl 'https://…' -H 'cookie: …'"
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
                        <input type={f.type === 'password' ? 'password' : f.type === 'email' ? 'email' : 'text'}
                               value={values[f.id] ?? ''} required={!f.optional} autoFocus={i === 0}
                               autoComplete={f.type === 'password' ? 'current-password' : 'off'}
                               onChange={e => setValues(v => ({...v, [f.id]: e.target.value}))}/>
                        {f.description && <small className="muted">{f.description}</small>}
                    </label>
                ))}
            </>}
            <div className="modal-actions">
                <button className="primary" type="submit" disabled={props.busy}>{props.busy ? 'Working…' : 'Continue'}</button>
            </div>
        </form>
    );
}
