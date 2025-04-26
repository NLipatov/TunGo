import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

const FeatureList = [
  {
    title: 'Fast',
    Svg: require('@site/static/svg/racing-car.svg').default,
    description: (
      <>
          <strong>No allocations</strong> and practically no CPU time
      </>
    ),
  },
  {
    title: 'Open Sourced',
    Svg: require('@site/static/svg/github.svg').default,
    description: (
      <>
        TunGo is an <strong>MIT licensed</strong> open source project 
      </>
    ),
  },
  {
    title: 'Secure',
    Svg: require('@site/static/svg/key.svg').default,
    description: (
      <>
          ChaCha20 used to bidirectional tunnel traffic encryption
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
