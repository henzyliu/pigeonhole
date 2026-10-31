import {useId, useState} from 'react';
import {initials} from './api';
import NetworkIcon from './NetworkIcon';

// presence is a Discord friend's status, shown as a dot; online is how many
// friends are online in a Discord group, shown as a small green count.
export default function Avatar(props: {name: string; src?: string; size: number; network?: string; presence?: string; online?: number}) {
    const [failed, setFailed] = useState(false);
    const style = {width: props.size, height: props.size, fontSize: props.size * 0.38};
    return (
        <div className="avatar" style={style}>
            {props.src && !failed
                ? <img src={props.src} alt="" onError={() => setFailed(true)}/>
                : <span>{initials(props.name || '?')}</span>}
            {props.network && <span className="avatar-net"><NetworkIcon network={props.network} size={15}/></span>}
            {props.presence && <PresenceDot presence={props.presence} size={Math.max(8, Math.round(props.size * 0.3))}/>}
            {!props.presence && !!props.online && (
                <span className="avatar-online" title={`${props.online} ${props.online === 1 ? 'friend' : 'friends'} online`}>
                    {props.online > 9 ? '9+' : props.online}
                </span>
            )}
        </div>
    );
}

export const PRESENCE_LABELS: Record<string, string> = {
    online: 'Online',
    idle: 'Idle',
    dnd: 'Do not disturb',
};

// Discord's status marks: a green dot when online, a yellow moon when idle,
// and a red dot with a bar for do not disturb.
export function PresenceDot(props: {presence: string; size: number}) {
    const maskId = useId();
    const label = PRESENCE_LABELS[props.presence] ?? props.presence;
    return (
        <svg className="avatar-presence" width={props.size} height={props.size} viewBox="0 0 10 10" role="img" aria-label={label}>
            <title>{label}</title>
            {props.presence === 'idle' ? <>
                <mask id={maskId}>
                    <rect width="10" height="10" fill="#fff"/>
                    <circle cx="2.6" cy="2.6" r="3.6" fill="#000"/>
                </mask>
                <circle cx="5" cy="5" r="5" fill="#F0B232" mask={`url(#${maskId})`}/>
            </> : props.presence === 'dnd' ? <>
                <circle cx="5" cy="5" r="5" fill="#F23F43"/>
                <rect x="2.4" y="4.1" width="5.2" height="1.8" rx="0.9" fill="#fff"/>
            </> : <circle cx="5" cy="5" r="5" fill="#23A55A"/>}
        </svg>
    );
}
