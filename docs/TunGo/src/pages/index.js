import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/features';
import Heading from '@theme/Heading';
import styles from './index.module.css';
import Footer from "../components/footer/footer";

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <Heading as="h1" className="hero__title" style={{color: "white"}}>
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle" style={{color: "white"}}>{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/QuickStart">
              Set up your TunGo VPN tunnel in minutes ⏱️
          </Link>
        </div>
      </div>
    </header>
  );
}

// noinspection JSUnusedGlobalSymbols
export default function Home() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
        title={`${siteConfig.title} — Minimalistic, Fast & Secure Open Source VPN`}
        description={`Secure your connection with ${siteConfig.title}: lightweight, fast, open-source VPN built in Go using modern cryptography.`}>
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
        <Footer/>
    </Layout>
  );
}
