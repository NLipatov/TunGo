import Clsx from 'clsx';
import Link from '@docusaurus/Link';
import Heading from '@theme/Heading';
import Translate from '@docusaurus/Translate';
import Styles from './styles.module.css';

function Feature({Svg, title, description, to, external = false}) {
    return (
        <div className={Clsx('col col--4', Styles.featureColumn)}>
            <Link
                className={Styles.featureCardLink}
                to={to}
                {...(external ? {target: '_blank', rel: 'noreferrer'} : {})}>
                <article className={Styles.featureCard}>
                    <div className={Styles.featureVisual}>
                        <Svg className={Styles.featureSvg} role="img"/>
                    </div>
                    <div className={Styles.featureBody}>
                        <Heading as="h3" className={Styles.featureTitle}>{title}</Heading>
                        <div className={Styles.featureDescription}>{description}</div>
                    </div>
                </article>
            </Link>
        </div>
    );
}

export default function Features() {
    return (
        <section className={Styles.features}>
            <div className="container">
                <div className="row">
                    <Feature
                        Svg={require('@site/static/svg/racing-car.svg').default}
                        title={<Translate id="feature.cpuFast.title">Fast</Translate>}
                        description={<Translate id="feature.cpuFast.description">0 allocs/op on the hot path.</Translate>}
                        to="/benchmarks"
                    />
                    <Feature
                        Svg={require('@site/static/svg/raspberry-pi.svg').default}
                        title={<Translate id="feature.ramEfficient.title">Memory</Translate>}
                        description={<Translate id="feature.ramEfficient.description">≈5–15 MB RSS under load.</Translate>}
                        to="/benchmarks"
                    />
                    <Feature
                        Svg={require('@site/static/svg/key.svg').default}
                        title={<Translate id="feature.secure.title">Crypto</Translate>}
                        description={<Translate id="feature.secure.description">Noise IK, X25519, ChaCha20-Poly1305.</Translate>}
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/cable.svg').default}
                        title={<Translate id="feature.multiTransport.title">Transports</Translate>}
                        description={<Translate id="feature.multiTransport.description">UDP, TCP, WebSocket/WSS.</Translate>}
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/monitor.svg').default}
                        title={<Translate id="feature.platforms.title">Platforms</Translate>}
                        description={<Translate id="feature.platforms.description">Linux, macOS, Windows.</Translate>}
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/github.svg').default}
                        title={<Translate id="feature.openSource.title">OSS</Translate>}
                        description={<Translate id="feature.openSource.description">AGPLv3-licensed.</Translate>}
                        to="https://github.com/NLipatov/TunGo"
                        external
                    />
                </div>
            </div>
        </section>
    );
}
