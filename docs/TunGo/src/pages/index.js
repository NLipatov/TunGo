import Clsx from 'clsx';
import Link from '@docusaurus/Link';
import UseDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Features from '@site/src/components/features';
import Heading from '@theme/Heading';
import Styles from './index.module.css';
import Footer from "../components/footer/footer";

function HomepageHeader() {
  const {siteConfig} = UseDocusaurusContext();
  return (
    <header className={Clsx('hero hero--primary', Styles.heroBanner)}>
      <div className="container">
        <Heading as="h1" className="hero__title" style={{color: "white"}}>
          {siteConfig.title}
        </Heading>
        <p className="hero__subtitle" style={{color: "white"}}>{siteConfig.tagline}</p>
        <div className={Styles.buttons}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/QuickStart">
              Get started in minutes ⏱️
          </Link>
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
        title={`${siteConfig.title} — Minimalistic, Fast & Secure Open Source VPN`}
        description={`Secure your connection with ${siteConfig.title}: lightweight, fast, open-source VPN built in Go using modern cryptography.`}>
      <HomepageHeader />
      <main>
        <Features />
      </main>
        <Footer/>
    </Layout>
  );
}
