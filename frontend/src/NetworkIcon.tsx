import {useId} from 'react';

// Brand marks for each network, drawn as inline SVG so they stay crisp at any
// size and need no image assets. Gradients follow Meta's published logos.
export default function NetworkIcon(props: {network: string; size?: number; className?: string; title?: string}) {
    const size = props.size ?? 16;
    const gradientId = useId();
    const common = {
        width: size,
        height: size,
        viewBox: '0 0 24 24',
        className: `network-icon ${props.className ?? ''}`,
        role: 'img',
        'aria-label': props.title ?? props.network,
    };

    switch (props.network) {
        case 'messenger':
            return (
                <svg {...common}>
                    <defs>
                        <radialGradient id={gradientId} cx="0.19" cy="0.99" r="1.09" fx="0.19" fy="0.99">
                            <stop offset="0" stopColor="#0099FF"/>
                            <stop offset="0.61" stopColor="#A033FF"/>
                            <stop offset="0.93" stopColor="#FF5280"/>
                            <stop offset="1" stopColor="#FF7061"/>
                        </radialGradient>
                    </defs>
                    {/* Speech bubble */}
                    <path fill={`url(#${gradientId})`}
                          d="M12 0C5.24 0 0 4.95 0 11.64c0 3.5 1.43 6.52 3.77 8.61.2.18.31.42.32.68l.07 2.14a.96.96 0 0 0 1.35.85l2.39-1.05a.96.96 0 0 1 .64-.05c1.09.3 2.26.46 3.46.46 6.76 0 12-4.95 12-11.64S18.76 0 12 0z"/>
                    {/* Lightning bolt */}
                    <path fill="#fff"
                          d="M4.8 15.04 8.32 9.45a1.8 1.8 0 0 1 2.61-.48l2.8 2.1c.26.2.61.2.86 0l3.79-2.87c.5-.38 1.17.22.82.75l-3.52 5.6a1.8 1.8 0 0 1-2.6.48l-2.8-2.1a.72.72 0 0 0-.87 0L5.62 15.8c-.5.39-1.17-.22-.82-.75z"/>
                </svg>
            );
        case 'instagram':
            return (
                <svg {...common}>
                    <defs>
                        <radialGradient id={gradientId} cx="0.27" cy="1.07" r="1.3">
                            <stop offset="0" stopColor="#FFDD55"/>
                            <stop offset="0.1" stopColor="#FFDD55"/>
                            <stop offset="0.5" stopColor="#FF543E"/>
                            <stop offset="1" stopColor="#C837AB"/>
                        </radialGradient>
                    </defs>
                    <rect width="24" height="24" rx="6.5" fill={`url(#${gradientId})`}/>
                    {/* Camera outline, lens and flash */}
                    <rect x="5" y="5" width="14" height="14" rx="4.2" fill="none" stroke="#fff" strokeWidth="1.8"/>
                    <circle cx="12" cy="12" r="3.3" fill="none" stroke="#fff" strokeWidth="1.8"/>
                    <circle cx="16.1" cy="7.9" r="1.05" fill="#fff"/>
                </svg>
            );
        default:
            return <span className="network-icon fallback" style={{width: size, height: size}}/>;
    }
}
