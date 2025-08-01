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
            <div style={{display: "flex", flexDirection: "column"}}>
                Server: ~8MB<br/>
                Client: ~4MB
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
        title: 'Open Source',
        Svg: require('@site/static/svg/github.svg').default,
        description: (
            <>
                License: <strong>AGPLv3</strong>
            </>
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
