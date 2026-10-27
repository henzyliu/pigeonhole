import {useState} from 'react';
import {inbox, NETWORK_LABELS} from './api';
import icon from './assets/pigeonhole.png';
import NetworkIcon from './NetworkIcon';
import {IS_MAC, MOD, useCloseOnEscape} from './shortcuts';

const NETWORKS = ['messenger', 'instagram', 'whatsapp', 'discord'];

// The first-run welcome: what Pigeonhole is, connecting accounts, and a few
// tips. Shown once, until finished or skipped.
export default function Welcome(props: {
    accounts: inbox.Account[];
    onConnect: (network: string) => void;
    onFinish: () => void;
}) {
    const [step, setStep] = useState(0);
    // Escape leaves it, as finishing does.
    useCloseOnEscape(props.onFinish);
    const connected = (network: string) => props.accounts.some(a => a.network === network && !a.needsLogin);
    const anyConnected = NETWORKS.some(connected);

    return (
        <div className="welcome" role="dialog" aria-label="Welcome to Pigeonhole">
            <div className="welcome-card">
                {step === 0 && <>
                    <img className="welcome-icon" src={icon} alt=""/>
                    <h1>Welcome to Pigeonhole</h1>
                    <p className="welcome-lead">Messenger, Instagram, WhatsApp and Discord, together in one app.</p>
                    <ul className="welcome-points">
                        <li>
                            <strong>Private by design.</strong> Your messages are stored on this computer only.
                            There's no Pigeonhole account and no Pigeonhole server.
                        </li>
                        <li>
                            <strong>Always there.</strong> Closing the window keeps Pigeonhole running in
                            the {IS_MAC ? 'menu bar' : 'system tray'}, so messages and notifications keep coming.
                        </li>
                        <li>
                            <strong>Free and open source.</strong> Anyone can see how it works.
                        </li>
                    </ul>
                    <div className="welcome-actions">
                        <button className="primary" autoFocus onClick={() => setStep(1)}>Get started</button>
                    </div>
                </>}

                {step === 1 && <>
                    <h1>Connect your accounts</h1>
                    <p className="welcome-lead">Sign in to the ones you use. You can add more later from Accounts.</p>
                    <div className="welcome-networks">
                        {NETWORKS.map(n => (
                            <button key={n} className={`network-button ${connected(n) ? 'connected' : ''}`}
                                    onClick={() => props.onConnect(n)}>
                                <NetworkIcon network={n} size={28}/>
                                <span>{NETWORK_LABELS[n]}</span>
                                {connected(n) && <span className="welcome-check" aria-label="Connected">✓</span>}
                            </button>
                        ))}
                    </div>
                    <p className="fine-print">
                        Pigeonhole connects the same unofficial way as the mautrix bridges. Meta may
                        occasionally ask you to confirm it's you, and WhatsApp links like WhatsApp Web.
                        Discord's rules don't allow unofficial apps, so use it at your own risk.
                    </p>
                    <div className="welcome-actions">
                        <button className="link" onClick={() => setStep(0)}>← Back</button>
                        <button className="primary" onClick={() => setStep(2)}>{anyConnected ? 'Continue' : 'Skip for now'}</button>
                    </div>
                </>}

                {step === 2 && <>
                    <h1>{anyConnected ? "You're all set" : 'Ready when you are'}</h1>
                    <p className="welcome-lead">
                        {anyConnected
                            ? 'Your chats will appear as they sync. Older messages keep loading in the background.'
                            : 'Connect an account any time from the Accounts button at the top of the chat list.'}
                    </p>
                    <ul className="welcome-tips">
                        <li><kbd>{MOD}K</kbd> jumps to any chat</li>
                        <li><kbd>{MOD}R</kbd> refreshes if messages seem slow to arrive</li>
                        <li><kbd>{MOD}/</kbd> shows all the keyboard shortcuts</li>
                        <li>Notifications, appearance and updates are in Settings</li>
                    </ul>
                    <div className="welcome-actions">
                        <button className="link" onClick={() => setStep(1)}>← Back</button>
                        <button className="primary" autoFocus onClick={props.onFinish}>Start messaging</button>
                    </div>
                </>}

                <div className="welcome-dots" aria-hidden="true">
                    {[0, 1, 2].map(i => <span key={i} className={i === step ? 'on' : ''}/>)}
                </div>
            </div>
        </div>
    );
}
