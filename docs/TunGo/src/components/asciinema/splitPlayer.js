import React from 'react';
import Player from '@site/src/components/asciinema/player';

const SplitPlayer = ({ castA, castB }) => (
    <div style={{ display: 'flex', gap: '1rem' }}>
        <div style={{ flex: 1 }}>
            <Player castPath={castA} />
        </div>
        <div style={{ flex: 1 }}>
            <Player castPath={castB} />
        </div>
    </div>
);

export default SplitPlayer;
