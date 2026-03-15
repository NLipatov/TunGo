import Clsx from 'clsx';
import Heading from '@theme/Heading';
import Translate from '@docusaurus/Translate';
import Styles from './styles.module.css';

function Feature({Svg, title, description}) {
    return (
        <div className={Clsx('col col--4', Styles.featureColumn)}>
            <div className={Styles.featureCard}>
                <div className={Styles.featureVisual}>
                    <Svg className={Styles.featureSvg} role="img"/>
                </div>
                <div className={Styles.featureBody}>
                    <Heading as="h3" className={Styles.featureTitle}>{title}</Heading>
                    <div className={Styles.featureDescription}>{description}</div>
                </div>
            </div>
        </div>
    );
}

export default function Features() {
    return (
        <section className={Styles.features}>
            <div className="container">
                <div className={Styles.sectionIntro}>
                    <span className={Styles.eyebrow}>Core characteristics</span>
                    <Heading as="h2" className={Styles.sectionTitle}>Small surface area, explicit tradeoffs</Heading>
                    <p className={Styles.sectionText}>
                        TunGo tries to stay boring on the hot path and opinionated around it. The feature set is compact on
                        purpose, and the implementation is optimized for readability as much as for speed.
                    </p>
                </div>
                <div className="row">
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
                        description={<Translate id="feature.secure.description">Noise IK handshake, X25519 key agreement, ChaCha20-Poly1305 AEAD</Translate>}
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
