import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

const FeatureList = [
  {
    title: 'Easy to Use',
    Svg: require('@site/static/img/peg.svg').default,
    description: (
      <>
        It&apos;s easy to use. Like peg.
      </>
    ),
  },
  {
    title: 'Focus on What Matters',
    Svg: require('@site/static/img/baloons.svg').default,
    description: (
      <>
        It&apos;s fast lightweight reliable and secure.
      </>
    ),
  },
  {
    title: 'Powered by Go',
    Svg: require('@site/static/img/gopher.svg').default,
    description: (
      <>
          High performance, simple syntax, and efficient concurrency support made it a perfect choice for TunGo.
      </>
    ),
  },
];

function Feature({Svg, title, description}) {
  return (
    <div className={clsx('col col--4')}>
      <div className="text--center">
        <Svg className={styles.featureSvg} role="img" />
      </div>
      <div className="text--center padding-horiz--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures() {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
