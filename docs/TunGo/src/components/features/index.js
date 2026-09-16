import Clsx from 'clsx';
import Link from '@docusaurus/Link';
import Heading from '@theme/Heading';
import Translate from '@docusaurus/Translate';
import Styles from './styles.module.css';

function Feature({Svg, title, description, to, external = false}) {
    const card = (
        <article className={Styles.featureCard}>
            <div className={Styles.featureVisual}>
                <Svg className={Styles.featureSvg} role="img"/>
            </div>
            <div className={Styles.featureBody}>
                <Heading as="h3" className={Styles.featureTitle}>{title}</Heading>
                {description && <div className={Styles.featureDescription}>{description}</div>}
            </div>
        </article>
    );

    return (
        <div className={Clsx('col col--4', Styles.featureColumn)}>
            {to ? (
                <Link
                    className={Styles.featureCardLink}
                    to={to}
                    {...(external ? {target: '_blank', rel: 'noreferrer'} : {})}>
                    {card}
                </Link>
            ) : card}
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
                        title={<Translate id="feature.cpuFast.title">Low CPU usage</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/raspberry-pi.svg').default}
                        title={<Translate id="feature.ramEfficient.title">Low RAM usage</Translate>}
                    />
                    <Feature
                        Svg={require('@site/static/svg/key.svg').default}
                        title={<Translate id="feature.secure.title">Modern cryptography</Translate>}
                        description={<Translate id="feature.secure.description">Noise IK, X25519 and ChaCha20-Poly1305.</Translate>}
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/cable.svg').default}
                        title={<Translate id="feature.multiTransport.title">Three connection options</Translate>}
                        description={<Translate id="feature.multiTransport.description">Choose UDP, TCP or WebSocket, including WSS.</Translate>}
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/monitor.svg').default}
                        title={<Translate id="feature.platforms.title">Linux, macOS and Windows</Translate>}
                        description={<Translate id="feature.platforms.description">Clients support amd64 and arm64. The server supports Linux.</Translate>}
                        to="/docs/QuickStart"
                    />
                    <Feature
                        Svg={require('@site/static/svg/github.svg').default}
                        title={<Translate id="feature.openSource.title">Open source</Translate>}
                        description={<Translate id="feature.openSource.description">Source code available under the AGPLv3 license.</Translate>}
                        to="https://github.com/NLipatov/TunGo"
                        external
                    />
                </div>
            </div>
        </section>
    );
}
