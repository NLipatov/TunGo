import Clsx from 'clsx';
import Link from '@docusaurus/Link';
import UseDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Translate, {translate} from '@docusaurus/Translate';
import Layout from '@theme/Layout';
import Features from '@site/src/components/features';
import Heading from '@theme/Heading';
import Styles from './index.module.css';
import Footer from '../components/footer/footer';

const highlights = [
  {
    label: 'Dataplane',
    value: '0 allocs/op',
    note: 'Full-cycle UDP/TCP packet path stays allocation-free in current benchmark set.',
  },
  {
    label: 'Memory',
    value: '5-15 MB RSS',
    note: 'Lean enough for small VPS instances and constrained edge deployments.',
  },
  {
    label: 'Transports',
    value: 'UDP, TCP, WS',
    note: 'High-performance default path plus fallback and stealth-oriented options.',
  },
];

function HomepageHeader() {
  const {siteConfig} = UseDocusaurusContext();
  return (
    <header className={Clsx(Styles.heroBanner)}>
      <div className={Clsx('container', Styles.heroGrid)}>
        <div className={Styles.heroCopy}>
          <span className={Styles.eyebrow}>TunGo VPN</span>
          <Heading as="h1" className={Styles.heroTitle}>
            Lean userspace VPN for fast, readable, modern networking
          </Heading>
          <p className={Styles.heroSubtitle}>
            {siteConfig.tagline}. TunGo focuses on a small hot path, explicit control plane, and performance that is
            easy to reason about from the code up.
          </p>
          <div className={Styles.buttons}>
            <Link className="button button--primary button--lg" to="/docs/QuickStart">
              <Translate id="homepage.cta">Get started in minutes</Translate>
            </Link>
            <Link className={Clsx('button button--secondary button--lg', Styles.secondaryCta)} to="/benchmarks">
              View benchmarks
            </Link>
          </div>
        </div>

        <div className={Styles.heroPanel}>
          <div className={Styles.heroPanelHeader}>
            <span className={Styles.heroPanelTag}>Why it feels light</span>
            <span className={Styles.heroPanelValue}>Go, not baggage</span>
          </div>
          <div className={Styles.heroPanelGrid}>
            {highlights.map((item) => (
              <div key={item.label} className={Styles.heroMetric}>
                <span className={Styles.heroMetricLabel}>{item.label}</span>
                <strong className={Styles.heroMetricValue}>{item.value}</strong>
                <p className={Styles.heroMetricNote}>{item.note}</p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </header>
  );
}

// noinspection JSUnusedGlobalSymbols
export default function Home() {
  const {siteConfig} = UseDocusaurusContext();
  return (
    <Layout
        title={translate({id: 'homepage.title', message: 'Minimalistic, Fast & Secure Open Source VPN'})}
        description={translate({id: 'homepage.description', message: 'Secure your connection with TunGo: lightweight, fast, open-source VPN built in Go using modern cryptography.'})}>
      <HomepageHeader />
      <main>
        <Features />
        <section className={Styles.benchmarkTeaser}>
          <div className={Clsx('container', Styles.teaserShell)}>
            <div>
              <p className={Styles.teaserEyebrow}>Performance transparency</p>
              <Heading as="h2" className={Styles.teaserTitle}>
                Benchmark numbers are part of the product story
              </Heading>
              <p className={Styles.teaserText}>
                The docs site now includes a dedicated benchmark dashboard with dataplane throughput, latency, repository
                lookup costs, and scaling behaviour. That makes performance claims auditable instead of decorative.
              </p>
            </div>
            <div className={Styles.teaserCard}>
              <div className={Styles.teaserMetric}>
                <span>Full-cycle UDP/TCP</span>
                <strong>~2.5-2.9 us</strong>
              </div>
              <div className={Styles.teaserMetric}>
                <span>1400B dataplane throughput</span>
                <strong>~0.49-0.56 GB/s</strong>
              </div>
              <div className={Styles.teaserMetric}>
                <span>Repository fast paths</span>
                <strong>~4-15 ns</strong>
              </div>
              <Link className="button button--primary button--lg" to="/benchmarks">
                Open benchmark dashboard
              </Link>
            </div>
          </div>
        </section>
      </main>
      <Footer />
    </Layout>
  );
}
