import {useId} from 'react';

// Brand marks for each network, drawn as inline SVG so they stay crisp at any
// size and need no image assets. Gradients follow Meta's published logos;
// WhatsApp's mark sits on its brand green, Discord's on its blurple, and
// Google Messages' bubble on Google blue.
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
        case 'whatsapp':
            return (
                <svg {...common}>
                    <rect width="24" height="24" rx="6.5" fill="#25D366"/>
                    {/* Speech bubble with a phone handset */}
                    <path fill="none" stroke="#fff" strokeWidth="1.5" strokeLinejoin="round"
                          d="M12 4.6a7.3 7.3 0 0 0-6.3 11l-.9 3.6 3.7-.95A7.3 7.3 0 1 0 12 4.6z"/>
                    <path fill="#fff"
                          d="M9.55 8.3c-.17-.38-.35-.39-.51-.4h-.44c-.15 0-.4.06-.61.29-.21.23-.8.78-.8 1.9s.82 2.2.93 2.36c.12.15 1.58 2.53 3.9 3.44 1.93.76 2.32.61 2.74.57.42-.04 1.36-.56 1.55-1.1.19-.53.19-.99.13-1.09-.06-.1-.21-.15-.44-.27-.23-.11-1.36-.67-1.57-.75-.21-.08-.36-.11-.51.12-.15.23-.59.75-.72.9-.13.15-.27.17-.5.06-.23-.12-.97-.36-1.85-1.14-.68-.61-1.14-1.36-1.28-1.59-.13-.23-.01-.35.1-.47.1-.1.23-.27.34-.4.11-.13.15-.23.23-.38.08-.15.04-.29-.02-.4-.06-.12-.5-1.25-.7-1.71z"/>
                </svg>
            );
        case 'discord':
            return (
                <svg {...common}>
                    <rect width="24" height="24" rx="6.5" fill="#5865F2"/>
                    {/* Clyde */}
                    <path fill="#fff" transform="translate(4.2 4.2) scale(0.65)"
                          d="M20.317 4.37a19.79 19.79 0 0 0-4.885-1.515.074.074 0 0 0-.079.037c-.21.375-.444.865-.608 1.25a18.27 18.27 0 0 0-5.487 0 12.64 12.64 0 0 0-.617-1.25.077.077 0 0 0-.079-.037A19.74 19.74 0 0 0 3.677 4.37a.07.07 0 0 0-.032.028C.533 9.046-.32 13.58.099 18.058a.082.082 0 0 0 .031.056 19.9 19.9 0 0 0 5.993 3.03.078.078 0 0 0 .084-.028c.462-.63.873-1.295 1.226-1.994a.076.076 0 0 0-.042-.106 13.1 13.1 0 0 1-1.872-.892.077.077 0 0 1-.008-.128c.126-.094.252-.192.372-.291a.074.074 0 0 1 .078-.01c3.928 1.793 8.18 1.793 12.061 0a.074.074 0 0 1 .079.009c.12.099.246.198.373.292a.077.077 0 0 1-.007.128 12.3 12.3 0 0 1-1.873.891.077.077 0 0 0-.041.107c.36.698.772 1.363 1.225 1.993a.076.076 0 0 0 .084.029 19.84 19.84 0 0 0 6.002-3.03.077.077 0 0 0 .032-.055c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 0 0-.031-.029zM8.02 15.331c-1.183 0-2.157-1.086-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.095 2.157 2.42 0 1.332-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.086-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.095 2.157 2.42 0 1.332-.946 2.418-2.157 2.418z"/>
                </svg>
            );
        case 'gmessages':
            return (
                <svg {...common}>
                    <rect width="24" height="24" rx="6.5" fill="#1A73E8"/>
                    {/* Speech bubble with two lines of text */}
                    <path fill="#fff"
                          d="M7.4 5.5h9.2a2.4 2.4 0 0 1 2.4 2.4v6.2a2.4 2.4 0 0 1-2.4 2.4H10l-3.6 2.6a.5.5 0 0 1-.8-.4V7.9a1.9 1.9 0 0 1 1.8-2.4z"/>
                    <path stroke="#1A73E8" strokeWidth="1.5" strokeLinecap="round" d="M8.8 9.6h6.4M8.8 12.5h4.2"/>
                </svg>
            );
        default:
            return <span className="network-icon fallback" style={{width: size, height: size}}/>;
    }
}
