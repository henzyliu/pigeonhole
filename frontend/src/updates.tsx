import {useEffect, useState} from 'react';
import {EventsOn, Go} from './api';
import {main} from '../wailsjs/go/models';

// The app's update status (see updates.go), kept current by "update:status"
// events.
export function useUpdateStatus(): main.UpdateStatus | null {
    const [status, setStatus] = useState<main.UpdateStatus | null>(null);
    useEffect(() => {
        Go.GetUpdateStatus().then(setStatus).catch(console.error);
        return EventsOn('update:status', (s: main.UpdateStatus) => setStatus(s));
    }, []);
    return status;
}

// The update part of Settings → About.
export function UpdateRow({status}: {status: main.UpdateStatus | null}) {
    const [error, setError] = useState('');
    if (!status) return null;
    const latest = status.latest;
    const check = () => {
        setError('');
        Go.CheckForUpdates().catch(console.error);
    };
    const install = () => {
        setError('');
        Go.InstallUpdate().catch(e => setError(String(e)));
    };
    const whatsNew = latest && (
        <button className="link" onClick={() => Go.OpenUpdatePage().catch(console.error)}>What's new</button>
    );
    switch (status.state) {
        case 'checking':
            return <div className="update-row muted">Checking for updates…</div>;
        case 'available':
            return (
                <div className="update-row">
                    <div className="update-text">
                        <strong>Pigeonhole {latest?.version} is available.</strong> {whatsNew}
                        {(status.error || error) && <div className="update-error">{status.error || error}</div>}
                    </div>
                    {status.canInstall
                        ? <button className="primary" onClick={install}>Install update</button>
                        : <button onClick={() => Go.OpenUpdatePage().catch(console.error)}>Download</button>}
                </div>
            );
        case 'downloading':
            return (
                <div className="update-row">
                    <div className="update-text">
                        Downloading Pigeonhole {latest?.version}… {Math.round(status.progress * 100)}%
                        <div className="update-progress"><div style={{width: `${status.progress * 100}%`}}/></div>
                    </div>
                </div>
            );
        case 'installing':
            return <div className="update-row">Installing… Pigeonhole will restart in a moment.</div>;
        case 'error':
            return (
                <div className="update-row">
                    <div className="update-text update-error">{status.error}</div>
                    <button className="link" onClick={check}>Try again</button>
                </div>
            );
        case 'upToDate':
            return (
                <div className="update-row muted">
                    <div className="update-text">You're up to date.</div>
                    <button className="link" onClick={check}>Check for updates</button>
                </div>
            );
        default:
            return (
                <div className="update-row">
                    <div className="update-text"/>
                    <button className="link" onClick={check}>Check for updates</button>
                </div>
            );
    }
}
