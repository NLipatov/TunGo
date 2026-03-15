import Clsx from 'clsx';
import Link from '@docusaurus/Link';
import Heading from '@theme/Heading';
import Translate from '@docusaurus/Translate';
import Styles from './styles.module.css';

function Feature({Svg, title, description, to, context, cta, external = false}) {
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
                        <span className={Styles.featureContext}>{context}</span>
                        <Heading as="h3" className={Styles.featureTitle}>{title}</Heading>
                        <div className={Styles.featureDescription}>{description}</div>
                    </div>
                    <div className={Styles.featureFooter}>
                        <span className={Styles.featureAction}>{cta}</span>
                        <span className={Styles.featureArrow} aria-hidden="true">→</span>
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
                <div className={Styles.sectionIntro}>
                    <span className={Styles.eyebrow}>Core characteristics</span>
                    <Heading as="h2" className={Styles.sectionTitle}>Small surface. Hard edges.</Heading>
                    <p className={Styles.sectionText}>
                        Lean runtime. Clear design.
                    </p>
                </div>
                <div className="row">
                    <Feature
                        Svg={require('@site/static/svg/racing-car.svg').default}
                        title={<Translate id="feature.cpuFast.title">Fast</Translate>}
                        description={<Translate id="feature.cpuFast.description">Allocation-free hot path.</Translate>}
                        context="Dataplane"
                        cta="Benchmarks"
                        to="/benchmarks"
                    />
                    <Feature
                        Svg={require('@site/static/svg/raspberry-pi.svg').default}
                        title={<Translate id="feature.ramEfficient.title">Lean memory</Translate>}
                        description={<Translate id="feature.ramEfficient.description">≈5–15 MB RSS under load.</Translate>}
                        context="Footprint"
                        cta="Benchmarks"
                        to="/benchmarks"
                    />
                    <Feature
                        Svg={require('@site/static/svg/key.svg').default}
                        title={<Translate id="feature.secure.title">Secure</Translate>}
                        description={<Translate id="feature.secure.description">Noise IK. X25519. ChaCha20-Poly1305.</Translate>}
                        context="Cryptography"
                        cta="Quick start"
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/cable.svg').default}
                        title={<Translate id="feature.multiTransport.title">Transports</Translate>}
                        description={<Translate id="feature.multiTransport.description">UDP, TCP, WebSocket/WSS.</Translate>}
                        context="Connectivity"
                        cta="Quick start"
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/monitor.svg').default}
                        title={<Translate id="feature.platforms.title">Platforms</Translate>}
                        description={<Translate id="feature.platforms.description">Linux, macOS, Windows.</Translate>}
                        context="Deployment"
                        cta="Quick start"
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/github.svg').default}
                        title={<Translate id="feature.openSource.title">Open source</Translate>}
                        description={<Translate id="feature.openSource.description">AGPLv3-licensed.</Translate>}
                        context="Project"
                        cta="Repository"
                        to="https://github.com/NLipatov/TunGo"
                        external
                    />
                </div>
            </div>
        </section>
    );
}
