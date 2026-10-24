import {useEffect, useState} from 'react';
import {errorText, Go, inbox} from './api';

// A full-window view of a chat's photos: the one you clicked, fitted to the
// window, with the chat's other photos a step away (arrow keys or the side
// buttons). Open hands it to the computer's photo viewer; Save asks where.
export default function ImageViewer(props: {images: inbox.Message[]; index: number; onIndex: (i: number) => void; onClose: () => void}) {
    const {images, index} = props;
    const image = images[index];
    const [error, setError] = useState('');
    const hasPrev = index > 0;
    const hasNext = index < images.length - 1;

    useEffect(() => {
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') props.onClose();
            else if (e.key === 'ArrowLeft' && hasPrev) props.onIndex(index - 1);
            else if (e.key === 'ArrowRight' && hasNext) props.onIndex(index + 1);
        };
        document.addEventListener('keydown', onKeyDown);
        return () => document.removeEventListener('keydown', onKeyDown);
    }, [index, hasPrev, hasNext, props]);

    useEffect(() => setError(''), [index]);

    if (!image) return null;
    const run = (fn: () => Promise<void>) => fn().catch(e => setError(errorText(e)));

    return (
        <div className="viewer" onClick={props.onClose}>
            <div className="viewer-bar" onClick={e => e.stopPropagation()}>
                <div className="viewer-info">
                    <div>{image.isMe ? 'You' : image.senderName}</div>
                    <div className="viewer-date">
                        {new Date(image.ts).toLocaleString([], {dateStyle: 'medium', timeStyle: 'short'})}
                        {images.length > 1 && ` · ${index + 1} of ${images.length}`}
                    </div>
                </div>
                {error && <div className="viewer-error">{error}</div>}
                <button className="viewer-button" title="Open in your default app for photos"
                        onClick={() => run(() => Go.OpenMedia(image.mediaUrl, image.fileName))}>Open</button>
                <button className="viewer-button" onClick={() => run(() => Go.SaveMedia(image.mediaUrl, image.fileName))}>Save</button>
                <button className="viewer-button" aria-label="Close" title="Close (Esc)" onClick={props.onClose}>✕</button>
            </div>
            <div className="viewer-stage">
                {hasPrev && (
                    <button className="viewer-nav prev" aria-label="Previous photo" onClick={e => {
                        e.stopPropagation();
                        props.onIndex(index - 1);
                    }}>‹</button>
                )}
                <img key={image.eventId} src={image.mediaUrl} alt={image.fileName} onClick={e => e.stopPropagation()}/>
                {hasNext && (
                    <button className="viewer-nav next" aria-label="Next photo" onClick={e => {
                        e.stopPropagation();
                        props.onIndex(index + 1);
                    }}>›</button>
                )}
            </div>
            {image.body && (
                <div className="viewer-caption" onClick={e => e.stopPropagation()}>{image.body}</div>
            )}
        </div>
    );
}
