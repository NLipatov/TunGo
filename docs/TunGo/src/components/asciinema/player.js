import React, { useEffect, useRef } from 'react';
import { create } from 'asciinema-player';
import 'asciinema-player/dist/bundle/asciinema-player.css';

const player = ({ castPath, cols = 80, rows = 24, autoPlay = true, preload = true }) => {
    const containerRef = useRef(null);

    useEffect(() => {
        const playerInstance = create(castPath, containerRef.current, {
            cols,
            rows,
            autoPlay,
            preload,
        });
        return () => {
            if (playerInstance && playerInstance.destroy) {
                playerInstance.destroy();
            }
        };
    }, [castPath, cols, rows, autoPlay, preload]);


    return <div ref={containerRef} />;
};

export default player;
