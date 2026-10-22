import {useState} from 'react';
import {initials} from './api';
import NetworkIcon from './NetworkIcon';

export default function Avatar(props: {name: string; src?: string; size: number; network?: string}) {
    const [failed, setFailed] = useState(false);
    const style = {width: props.size, height: props.size, fontSize: props.size * 0.38};
    return (
        <div className="avatar" style={style}>
            {props.src && !failed
                ? <img src={props.src} alt="" onError={() => setFailed(true)}/>
                : <span>{initials(props.name || '?')}</span>}
            {props.network && <span className="avatar-net"><NetworkIcon network={props.network} size={15}/></span>}
        </div>
    );
}
