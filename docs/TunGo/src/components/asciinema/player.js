import React, { useEffect, useRef, useCallback } from 'react';
import { create } from 'asciinema-player';
import 'asciinema-player/dist/bundle/asciinema-player.css';

const Player = ({ castPath, autoPlay = true, preload = true }) => {
    const containerRef = useRef(null);
    const playerRef = useRef(null);

    const initPlayer = useCallback(() => {
        if (!containerRef.current) return;

        const { width, height } = containerRef.current.getBoundingClientRect();
        const charWidth = 8;
        const charHeight = 18;
        const cols = Math.floor(width / charWidth);
        const rows = Math.max(Math.floor(height / charHeight), 40)

        if (playerRef.current && playerRef.current.dispose) {
            playerRef.current.dispose();
        }

        playerRef.current = create(castPath, containerRef.current, {
            cols,
            rows,
            autoPlay,
            preload,
        });
    }, [castPath, autoPlay, preload]);

    useEffect(() => {
        const handleResize = () => {
            initPlayer();
        };

        const debounce = (fn, delay) => {
            let timer;
            return () => {
                clearTimeout(timer);
                timer = setTimeout(fn, delay);
            };
        };

        const debouncedResize = debounce(handleResize, 200);

        window.addEventListener('resize', debouncedResize);
        initPlayer();

        return () => {
            window.removeEventListener('resize', debouncedResize);
            if (playerRef.current && playerRef.current.dispose) {
                playerRef.current.dispose();
            }
        };
    }, [initPlayer]);

    return <div ref={containerRef} style={{ width: '100%', height: '100%' }} />;
};

export default Player;
