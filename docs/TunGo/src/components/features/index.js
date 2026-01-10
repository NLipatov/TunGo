import Clsx from 'clsx';
import Heading from '@theme/Heading';
import Translate from '@docusaurus/Translate';
import Styles from './styles.module.css';

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
                    <Feature
                        Svg={require('@site/static/svg/racing-car.svg').default}
                        title={<Translate id="feature.cpuFast.title">CPU-Fast</Translate>}
                        description={<Translate id="feature.cpuFast.description">No runtime allocations. Negligible CPU usage under load.</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/raspberry-pi.svg').default}
                        title={<Translate id="feature.ramEfficient.title">RAM-Efficient</Translate>}
                        description={<Translate id="feature.ramEfficient.description">≈5–15 MB RSS under load, ≈5–8 MB idle</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/key.svg').default}
                        title={<Translate id="feature.secure.title">Secure</Translate>}
                        description={<Translate id="feature.secure.description">End-to-end tunnel with ChaCha20 encryption</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/cable.svg').default}
                        title={<Translate id="feature.multiTransport.title">Multi-Transport Support</Translate>}
                        description={<Translate id="feature.multiTransport.description">UDP — high performance, TCP — reliable fallback, WebSocket/WSS — stealth mode</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/monitor.svg').default}
                        title={<Translate id="feature.platforms.title">Supported Platforms</Translate>}
                        description={<Translate id="feature.platforms.description">Linux (client and server), macOS (client), Windows (client)</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/github.svg').default}
                        title={<Translate id="feature.openSource.title">Open Source</Translate>}
                        description={<Translate id="feature.openSource.description">License: AGPLv3</Translate>}
                    />
                </div>
            </div>
        </section>
    );
}
