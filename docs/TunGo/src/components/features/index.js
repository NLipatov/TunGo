import Clsx from 'clsx';
import Heading from '@theme/Heading';
import Styles from './styles.module.css';

const FeatureList = [
    {
        title: 'CPU-Fast',
        Svg: require('@site/static/svg/racing-car.svg').default,
        description: (
            <>
                <strong>No runtime allocations</strong><br/>
                <strong>Negligible</strong> CPU usage under load.
            </>
        ),
    },
    {
        title: 'RAM-Efficient',
        Svg: require('@site/static/svg/raspberry-pi.svg').default,
        description: (
            <div style={{display: "flex", flexDirection: "column", gap: 2}}>
                <span>≈5–15&nbsp;MB <abbr title="Resident Set Size">RSS</abbr> under load</span>
                <span>≈5–8&nbsp;MB idle</span>
            </div>
        ),
    },
    {
        title: 'Secure',
        Svg: require('@site/static/svg/key.svg').default,
        description: (
            <>
                End-to-end tunnel with <strong>ChaCha20</strong> encryption
            </>
        ),
    },
    {
        title: 'Multi-Transport Support',
        Svg: require('@site/static/svg/cable.svg').default,
        description: (
            <div className={Styles.featureDescriptionList}>
                <div><strong>UDP</strong> — high performance</div>
                <div><strong>TCP</strong> — reliable fallback</div>
                <div><strong>WebSocket/WSS</strong> — stealth mode, DPI bypass</div>
            </div>
        ),
    },
    {
        title: 'Supported Platforms',
        Svg: require('@site/static/svg/monitor.svg').default,
        description: (
            <div className={Styles.featureDescriptionList}>
                <div>
                    <strong>Linux</strong> (client and server mode)
                </div>
                <div>
                    <strong>macOS</strong> (client mode)    
                </div>
                <div>
                    <strong>Windows</strong> (client mode)
                </div>
            </div>
        ),
    },
    {
        title: 'Open Source',
        Svg: require('@site/static/svg/github.svg').default,
        description: (
            <>
                License: <strong>AGPLv3</strong>
            </>
        ),
    },
];

function Feature({Svg, title, description}) {
    return (
        <div className={Clsx('col col--3')}>
            <div className="text--center">
                <Svg className={Styles.featureSvg} role="img"/>
            </div>
            <div className="text--center padding-horiz--md">
            <Heading as="h3">{title}</Heading>
                <div>{description}</div>
            </div>
        </div>
    );
}

export default function Features() {
    return (
        <section className={Styles.features}>
            <div className="container">
                <div className="row" style={{justifyContent: "center"}}>
                    {FeatureList.map((props, idx) => (
                        <Feature key={idx} {...props} />
                    ))}
                </div>
            </div>
        </section>
    );
}
