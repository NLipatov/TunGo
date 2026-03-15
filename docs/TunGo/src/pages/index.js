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
    note: 'Allocation-free packet path',
  },
  {
    label: 'Memory',
    value: '5-15 MB RSS',
    note: 'Lean enough for small VPS and edge',
  },
  {
    label: 'Transports',
    value: 'UDP, TCP, WS',
    note: 'Fast path plus fallback and stealth',
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
            Compact userspace VPN with a measured hot path
          </Heading>
          <p className={Styles.heroSubtitle}>
            {siteConfig.tagline}. TunGo keeps the dataplane small, the control plane explicit, and the performance story
            visible in code and benchmarks.
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
            <span className={Styles.heroPanelTag}>Snapshot</span>
            <span className={Styles.heroPanelValue}>Current posture</span>
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
      <div className={Styles.homeShell}>
        <HomepageHeader />
        <main className={Styles.homeMain}>
          <Features />
          <section className={Styles.benchmarkTeaser}>
            <div className={Clsx('container', Styles.teaserShell)}>
              <div className={Styles.teaserCopy}>
                <p className={Styles.teaserEyebrow}>Benchmark transparency</p>
                <Heading as="h2" className={Styles.teaserTitle}>
                  Performance claims link to measurements
                </Heading>
                <p className={Styles.teaserText}>
                  The benchmark dashboard tracks dataplane throughput, latency, lookup cost, and scaling behaviour.
                </p>
              </div>
              <div className={Styles.teaserMetrics}>
                <div className={Styles.teaserMetric}>
                  <span>1400B full-cycle latency</span>
                  <strong>~2.5-2.9 us</strong>
                </div>
                <div className={Styles.teaserMetric}>
                  <span>1400B full-cycle throughput</span>
                  <strong>~4.0-4.5 Gbit/s</strong>
                </div>
                <div className={Styles.teaserMetric}>
                  <span>Repository fast paths</span>
                  <strong>~4-15 ns</strong>
                </div>
              </div>
              <Link className={Clsx('button button--primary button--lg', Styles.teaserButton)} to="/benchmarks">
                Open benchmark dashboard
              </Link>
            </div>
          </section>
        </main>
        <Footer />
      </div>
    </Layout>
  );
}
