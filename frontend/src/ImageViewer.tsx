import {MouseEvent as ReactMouseEvent, useCallback, useEffect, useRef, useState} from 'react';
import {errorText, Go, inbox} from './api';

const MIN_SCALE = 1;
const MAX_SCALE = 8;
const STEP = 1.5;

// scale is relative to the photo fitted to the window; x and y pan it.
type View = {scale: number; x: number; y: number};
const FIT: View = {scale: 1, x: 0, y: 0};

// A full-window view of a chat's photos: the one you clicked, fitted to the
// window, with the chat's other photos a step away (arrow keys or the side
// buttons). Pinch, scroll, double-click or +/- zoom toward the pointer, and
// dragging pans. Open hands it to the computer's photo viewer; Save asks where.
export default function ImageViewer(props: {images: inbox.Message[]; index: number; onIndex: (i: number) => void; onClose: () => void}) {
    const {images, index} = props;
    const image = images[index];
    const [error, setError] = useState('');
    const [view, setView] = useState<View>(FIT);
    // Animate discrete zooms (buttons, keys, double-click), not continuous ones.
    const [smooth, setSmooth] = useState(false);
    const [dragging, setDragging] = useState(false);
    const viewRef = useRef(view);
    viewRef.current = view;
    const stageRef = useRef<HTMLDivElement>(null);
    const imgRef = useRef<HTMLImageElement>(null);
    // Set after a drag so the click that ends it doesn't close the viewer.
    const justDragged = useRef(false);
    const hasPrev = index > 0;
    const hasNext = index < images.length - 1;

    useEffect(() => {
        setView(FIT);
        setError('');
    }, [index]);

    // Keeps the photo covering the window when zoomed in, so it can't be
    // dragged off-screen; back at fit size it recentres.
    const clamp = useCallback((v: View): View => {
        const img = imgRef.current;
        const stage = stageRef.current;
        if (v.scale <= MIN_SCALE + 0.001 || !img || !stage) return FIT;
        const maxX = Math.max(0, (img.offsetWidth * v.scale - stage.clientWidth) / 2);
        const maxY = Math.max(0, (img.offsetHeight * v.scale - stage.clientHeight) / 2);
        return {scale: v.scale, x: Math.min(maxX, Math.max(-maxX, v.x)), y: Math.min(maxY, Math.max(-maxY, v.y))};
    }, []);

    // Zooms keeping the point under the pointer (or the middle) in place.
    const zoomTo = useCallback((scale: number, clientX?: number, clientY?: number, animate = false) => {
        const img = imgRef.current;
        const stage = stageRef.current;
        if (!img || !stage) return;
        const v = viewRef.current;
        const next = Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale));
        // The photo's untransformed centre, which scale and translate act on.
        const r = stage.getBoundingClientRect();
        const cx = r.left + img.offsetLeft + img.offsetWidth / 2;
        const cy = r.top + img.offsetTop + img.offsetHeight / 2;
        const px = clientX === undefined ? 0 : clientX - cx;
        const py = clientY === undefined ? 0 : clientY - cy;
        const k = next / v.scale;
        const nextView = clamp({scale: next, x: px - k * (px - v.x), y: py - k * (py - v.y)});
        // Updated at once, not on the next render: a trackpad can send several
        // wheel events per frame, and each must build on the last.
        viewRef.current = nextView;
        setSmooth(animate);
        setView(nextView);
    }, [clamp]);

    useEffect(() => {
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') props.onClose();
            else if (e.key === 'ArrowLeft' && hasPrev) props.onIndex(index - 1);
            else if (e.key === 'ArrowRight' && hasNext) props.onIndex(index + 1);
            else if (e.key === '+' || e.key === '=') zoomTo(viewRef.current.scale * STEP, undefined, undefined, true);
            else if (e.key === '-' || e.key === '_') zoomTo(viewRef.current.scale / STEP, undefined, undefined, true);
            else if (e.key === '0') zoomTo(MIN_SCALE, undefined, undefined, true);
        };
        document.addEventListener('keydown', onKeyDown);
        return () => document.removeEventListener('keydown', onKeyDown);
    }, [index, hasPrev, hasNext, props, zoomTo]);

    // Wheel and pinch need non-passive listeners to stop the page zooming.
    // Windows' WebView2 reports a trackpad pinch as a wheel event with Ctrl
    // held; the Mac's WebKit sends its own gesture events instead.
    useEffect(() => {
        const stage = stageRef.current;
        if (!stage) return;
        let pinching = false;
        let pinchStart = 1;
        const onWheel = (e: WheelEvent) => {
            e.preventDefault();
            if (pinching) return;
            const factor = Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.002));
            zoomTo(viewRef.current.scale * factor, e.clientX, e.clientY);
        };
        type GestureEvent = Event & {scale: number; clientX: number; clientY: number};
        const onGestureStart = (e: Event) => {
            e.preventDefault();
            pinching = true;
            pinchStart = viewRef.current.scale;
        };
        const onGestureChange = (e: Event) => {
            e.preventDefault();
            const g = e as GestureEvent;
            zoomTo(pinchStart * g.scale, g.clientX, g.clientY);
        };
        const onGestureEnd = (e: Event) => {
            e.preventDefault();
            pinching = false;
        };
        stage.addEventListener('wheel', onWheel, {passive: false});
        stage.addEventListener('gesturestart', onGestureStart);
        stage.addEventListener('gesturechange', onGestureChange);
        stage.addEventListener('gestureend', onGestureEnd);
        return () => {
            stage.removeEventListener('wheel', onWheel);
            stage.removeEventListener('gesturestart', onGestureStart);
            stage.removeEventListener('gesturechange', onGestureChange);
            stage.removeEventListener('gestureend', onGestureEnd);
        };
    }, [zoomTo]);

    const startDrag = (e: ReactMouseEvent) => {
        if (viewRef.current.scale <= MIN_SCALE || e.button !== 0) return;
        e.preventDefault();
        const start = {x: e.clientX, y: e.clientY, ox: viewRef.current.x, oy: viewRef.current.y};
        let moved = false;
        setDragging(true);
        setSmooth(false);
        const onMove = (m: MouseEvent) => {
            moved = moved || Math.abs(m.clientX - start.x) + Math.abs(m.clientY - start.y) > 3;
            setView(v => clamp({scale: v.scale, x: start.ox + m.clientX - start.x, y: start.oy + m.clientY - start.y}));
        };
        const onUp = () => {
            setDragging(false);
            justDragged.current = moved;
            window.removeEventListener('mousemove', onMove);
            window.removeEventListener('mouseup', onUp);
        };
        window.addEventListener('mousemove', onMove);
        window.addEventListener('mouseup', onUp);
    };

    const close = () => {
        if (justDragged.current) {
            justDragged.current = false;
            return;
        }
        props.onClose();
    };

    if (!image) return null;
    const run = (fn: () => Promise<void>) => fn().catch(e => setError(errorText(e)));
    const zoomed = view.scale > MIN_SCALE;

    return (
        <div className="viewer" onClick={close}>
            <div className="viewer-bar" onClick={e => e.stopPropagation()}>
                <div className="viewer-info">
                    <div>{image.isMe ? 'You' : image.senderName}</div>
                    <div className="viewer-date">
                        {new Date(image.ts).toLocaleString([], {dateStyle: 'medium', timeStyle: 'short'})}
                        {images.length > 1 && ` · ${index + 1} of ${images.length}`}
                    </div>
                </div>
                {error && <div className="viewer-error">{error}</div>}
                <div className="viewer-zoom">
                    <button className="viewer-button" aria-label="Zoom out" title="Zoom out (−)" disabled={!zoomed}
                            onClick={() => zoomTo(view.scale / STEP, undefined, undefined, true)}>−</button>
                    <button className="viewer-button viewer-percent" title="Fit to window (0)"
                            onClick={() => zoomTo(MIN_SCALE, undefined, undefined, true)}>{Math.round(view.scale * 100)}%</button>
                    <button className="viewer-button" aria-label="Zoom in" title="Zoom in (+)" disabled={view.scale >= MAX_SCALE}
                            onClick={() => zoomTo(view.scale * STEP, undefined, undefined, true)}>+</button>
                </div>
                <button className="viewer-button" title="Open in your default app for photos"
                        onClick={() => run(() => Go.OpenMedia(image.mediaUrl, image.fileName))}>Open</button>
                <button className="viewer-button" onClick={() => run(() => Go.SaveMedia(image.mediaUrl, image.fileName))}>Save</button>
                <button className="viewer-button" aria-label="Close" title="Close (Esc)" onClick={props.onClose}>✕</button>
            </div>
            <div className="viewer-stage" ref={stageRef}>
                {hasPrev && (
                    <button className="viewer-nav prev" aria-label="Previous photo" onClick={e => {
                        e.stopPropagation();
                        props.onIndex(index - 1);
                    }}>‹</button>
                )}
                <img key={image.eventId} ref={imgRef} src={image.mediaUrl} alt={image.fileName} draggable={false}
                     className={`${zoomed ? 'zoomed' : ''} ${dragging ? 'dragging' : ''} ${smooth ? 'smooth' : ''}`}
                     style={{transform: `translate(${view.x}px, ${view.y}px) scale(${view.scale})`}}
                     onClick={e => e.stopPropagation()}
                     onMouseDown={startDrag}
                     onDoubleClick={e => zoomed
                         ? zoomTo(MIN_SCALE, undefined, undefined, true)
                         : zoomTo(2.5, e.clientX, e.clientY, true)}/>
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
