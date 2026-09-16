import Clsx from 'clsx';
import Link from '@docusaurus/Link';
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
            <Translate id="homepage.heroTitle">Fast, lightweight VPN</Translate>
          </Heading>
          <div className={Styles.buttons}>
            <Link className="button button--primary button--lg" to="/docs/QuickStart">
              <Translate id="homepage.cta">Install</Translate>
            </Link>
          </div>
        </div>
      </div>
    </header>
  );
}

// noinspection JSUnusedGlobalSymbols
export default function Home() {
  return (
    <Layout
        title={translate({id: 'homepage.title', message: 'Fast, lightweight VPN'})}
        description={translate({id: 'homepage.description', message: 'TunGo is an open-source VPN written in Go with modern cryptography. The server supports Linux; clients support Linux, macOS and Windows.'})}>
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
