import Clsx from 'clsx';
import Link from '@docusaurus/Link';
import UseDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Translate, {translate} from '@docusaurus/Translate';
import Layout from '@theme/Layout';
import Features from '@site/src/components/features';
import Heading from '@theme/Heading';
import Styles from './index.module.css';
import Footer from '../components/footer/footer';

function HomepageHeader() {
  return (
    <header className={Clsx(Styles.heroBanner)}>
      <div className={Clsx('container', Styles.heroGrid)}>
        <div className={Styles.heroCopy}>
          <Heading as="h1" className={Styles.heroTitle}>
            <Translate id="homepage.heroTitle.prefix">Fast, lightweight</Translate>{' '}
            <span className={Styles.noBreak}>
              <Translate id="homepage.heroTitle.suffix">userspace VPN</Translate>
            </span>
          </Heading>
          <div className={Styles.buttons}>
            <Link className="button button--primary button--lg" to="/docs/QuickStart">
              <Translate id="homepage.cta">Get started in minutes</Translate>
            </Link>
            <Link className={Clsx('button button--secondary button--lg', Styles.secondaryCta)} to="/benchmarks">
              <Translate id="homepage.benchmarksCta">View benchmarks</Translate>
            </Link>
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
        </main>
        <Footer />
      </div>
    </Layout>
  );
}
