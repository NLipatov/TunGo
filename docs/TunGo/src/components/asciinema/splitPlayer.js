import React from 'react';
import Player from '@site/src/components/asciinema/player';

const SplitPlayer = ({ castA, castB, cols = 80, rows = 14 }) => (
    <div style={{ display: 'flex', gap: '1rem' }}>
        <div style={{ flex: 1 }}>
            <Player castPath={castA} cols={cols} rows={rows} />
        </div>
        <div style={{ flex: 1 }}>
            <Player castPath={castB} cols={cols} rows={rows} />
        </div>
    </div>
);

export default SplitPlayer;
