import Clsx from 'clsx';
import Heading from '@theme/Heading';
import Translate from '@docusaurus/Translate';
import Styles from './styles.module.css';

function Feature({Svg, title, description, accent}) {
    return (
        <div className={Clsx('col col--4', Styles.featureColumn)}>
            <div className={Styles.featureCard}>
                <div className={Styles.featureVisual} style={{'--feature-accent': accent}}>
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
                    <Heading as="h2" className={Styles.sectionTitle}>Built to stay small, explicit, and measurable</Heading>
                    <p className={Styles.sectionText}>
                        TunGo is opinionated about what belongs on the hot path and what should stay outside it. That shows up in
                        the codebase, the transport design, and the benchmark results.
                    </p>
                </div>
                <div className="row">
                    <Feature
                        Svg={require('@site/static/svg/racing-car.svg').default}
                        accent="#009fc9"
                        title={<Translate id="feature.cpuFast.title">CPU-Fast</Translate>}
                        description={<Translate id="feature.cpuFast.description">No runtime allocations. Negligible CPU usage under load.</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/raspberry-pi.svg').default}
                        accent="#13b88a"
                        title={<Translate id="feature.ramEfficient.title">RAM-Efficient</Translate>}
                        description={<Translate id="feature.ramEfficient.description">≈5–15 MB RSS under load, ≈5–8 MB idle</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/key.svg').default}
                        accent="#ff7b54"
                        title={<Translate id="feature.secure.title">Secure</Translate>}
                        description={<Translate id="feature.secure.description">Noise IK handshake, X25519 key agreement, ChaCha20-Poly1305 AEAD</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/cable.svg').default}
                        accent="#006ad6"
                        title={<Translate id="feature.multiTransport.title">Multi-Transport Support</Translate>}
                        description={<Translate id="feature.multiTransport.description">UDP — high performance, TCP — reliable fallback, WebSocket/WSS — stealth mode</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/monitor.svg').default}
                        accent="#6d5dfc"
                        title={<Translate id="feature.platforms.title">Supported Platforms</Translate>}
                        description={<Translate id="feature.platforms.description">Linux (client and server), macOS (client), Windows (client)</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/github.svg').default}
                        accent="#0f172a"
                        title={<Translate id="feature.openSource.title">Open Source</Translate>}
                        description={<Translate id="feature.openSource.description">License: AGPLv3</Translate>}
                    />
                </div>
            </div>
        </section>
    );
}
