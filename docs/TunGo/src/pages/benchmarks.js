import React from 'react';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Link from '@docusaurus/Link';
import Styles from './benchmarks.module.css';
import {benchmarkSnapshot} from '@site/src/data/benchmarks';

function formatNs(ns) {
  if (ns >= 1000) {
    return `~${(ns / 1000).toFixed(1)} us`;
  }
  if (ns >= 100) {
    return `~${Math.round(ns)} ns`;
  }
  return `~${ns.toFixed(1)} ns`;
}

function formatThroughput(value) {
  const gbit = (value * 8) / 1000;
  if (gbit >= 1) {
    return `~${gbit.toFixed(1)} Gbit/s`;
  }
  return `~${Math.round(value * 8)} Mbit/s`;
}

function buildPolyline(series, valueKey, width, height, padding) {
  const values = series.map((point) => point[valueKey]);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const innerWidth = width - padding * 2;
  const innerHeight = height - padding * 2;

  return series
    .map((point, index) => {
      const x = padding + (innerWidth * index) / Math.max(series.length - 1, 1);
      const y =
        max === min
          ? padding + innerHeight / 2
          : padding + innerHeight - ((point[valueKey] - min) / (max - min)) * innerHeight;
      return `${x},${y}`;
    })
    .join(' ');
}

function buildDots(series, valueKey, width, height, padding) {
  const values = series.map((point) => point[valueKey]);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const innerWidth = width - padding * 2;
  const innerHeight = height - padding * 2;

  return series.map((point, index) => {
    const x = padding + (innerWidth * index) / Math.max(series.length - 1, 1);
    const y =
      max === min
        ? padding + innerHeight / 2
        : padding + innerHeight - ((point[valueKey] - min) / (max - min)) * innerHeight;
    return {x, y, label: point.peers};
  });
}

function Sparkline({series, valueKey, color, formatter}) {
  const width = 520;
  const height = 180;
  const padding = 18;
  const polyline = buildPolyline(series, valueKey, width, height, padding);
  const dots = buildDots(series, valueKey, width, height, padding);

  return (
    <div className={Styles.sparklineShell}>
      <svg viewBox={`0 0 ${width} ${height}`} className={Styles.sparkline} role="img" aria-hidden="true">
        <rect x="0" y="0" width={width} height={height} rx="18" className={Styles.chartFrame} />
        <polyline points={polyline} className={Styles.chartLine} style={{stroke: color}} />
        {dots.map((dot) => (
          <g key={`${valueKey}-${dot.label}`}>
            <circle cx={dot.x} cy={dot.y} r="5" className={Styles.chartDot} style={{fill: color}} />
            <text x={dot.x} y={height - 6} textAnchor="middle" className={Styles.chartLabel}>
              {dot.label}
            </text>
          </g>
        ))}
      </svg>
      <div className={Styles.sparklineLegend}>
        {series.map((point) => (
          <div key={`${valueKey}-${point.peers}`} className={Styles.legendEntry}>
            <span className={Styles.legendPeers}>{point.peers} peers</span>
            <span className={Styles.legendValue}>{formatter(point[valueKey])}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function MetricCard({label, value, note}) {
  return (
    <div className={Styles.metricCard}>
      <div className={Styles.metricLabel}>{label}</div>
      <div className={Styles.metricValue}>{value}</div>
      <div className={Styles.metricNote}>{note}</div>
    </div>
  );
}

function FullCycleTable() {
  return (
    <div className={Styles.tableWrap}>
      <table className={Styles.table}>
        <thead>
          <tr>
            <th>Path</th>
            <th>Latency</th>
            <th>Throughput</th>
            <th>Allocs/op</th>
          </tr>
        </thead>
        <tbody>
          {benchmarkSnapshot.fullCycle1400.map((entry) => (
            <tr key={entry.id}>
              <td>
                <span className={Styles.tablePrimary}>{entry.transport}</span>
                <span className={Styles.tableSecondary}>{entry.direction}</span>
              </td>
              <td>{formatNs(entry.ns)}</td>
              <td>{formatThroughput(entry.throughput)}</td>
              <td>{entry.allocs}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function FastPathTable() {
  const peerCounts = benchmarkSnapshot.repository.fastPath[0].series.map((entry) => entry.peers);
  return (
    <div className={Styles.tableWrap}>
      <table className={Styles.table}>
        <thead>
          <tr>
            <th>Lookup</th>
            {peerCounts.map((count) => (
              <th key={count}>{count} peers</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {benchmarkSnapshot.repository.fastPath.map((row) => (
            <tr key={row.id}>
              <td>{row.label}</td>
              {row.series.map((point) => (
                <td key={`${row.id}-${point.peers}`}>{formatNs(point.ns)}</td>
              ))}
            </tr>
          ))}
          <tr>
            <td>Miss path</td>
            {benchmarkSnapshot.repository.missPath.map((point) => (
              <td key={`miss-${point.peers}`}>{formatNs(point.ns)}</td>
            ))}
          </tr>
        </tbody>
      </table>
    </div>
  );
}

export default function BenchmarksPage() {
  const bestFullCycle = [...benchmarkSnapshot.fullCycle1400].sort((a, b) => b.throughput - a.throughput)[0];
  const lowestLatency = [...benchmarkSnapshot.fullCycle1400].sort((a, b) => a.ns - b.ns)[0];

  return (
    <Layout
      title="Benchmarks"
      description="Benchmark dashboard for TunGo dataplane throughput, latency, repository lookup costs, and egress contention."
    >
      <main className={Styles.page}>
        <section className={Styles.hero}>
          <div className={Styles.heroIntro}>
            <p className={Styles.eyebrow}>Performance Benchmarks</p>
            <Heading as="h1" className={Styles.title}>
              Userspace dataplane numbers, without the marketing fog
            </Heading>
            <p className={Styles.lead}>
              TunGo&apos;s benchmark suite currently covers full-cycle UDP/TCP packet processing, repository lookups,
              egress serialization, and many-peer scaling. The numbers below are benchmark snapshots from{' '}
              <strong>{benchmarkSnapshot.machine}</strong> on <strong>{benchmarkSnapshot.goVersion}</strong>.
            </p>
            <p className={Styles.scope}>{benchmarkSnapshot.scope}</p>
          </div>
          <div className={Styles.metaPanel}>
            <div>
              <span className={Styles.metaLabel}>Snapshot date</span>
              <span className={Styles.metaValue}>{benchmarkSnapshot.generatedAt}</span>
            </div>
            <div>
              <span className={Styles.metaLabel}>Benchmark scope</span>
              <span className={Styles.metaValue}>Dataplane core and scaling paths</span>
            </div>
            <div>
              <span className={Styles.metaLabel}>Reproduce</span>
              <code className={Styles.metaCode}>go test -run ^$ -bench FullCycle -benchmem</code>
            </div>
          </div>
        </section>

        <section className={Styles.metrics}>
          <MetricCard
            label="Best 1400B full-cycle throughput"
            value={formatThroughput(bestFullCycle.throughput)}
            note={`${bestFullCycle.transport} ${bestFullCycle.direction}, 1400B payload`}
          />
          <MetricCard
            label="Lowest 1400B full-cycle latency"
            value={formatNs(lowestLatency.ns)}
            note={`${lowestLatency.transport} ${lowestLatency.direction}, 1400B payload`}
          />
          <MetricCard
            label="Repository fast-path lookup"
            value="~4-15 ns"
            note="Route ID, exact internal, and allowed-host hits stay flat through 10k peers"
          />
          <MetricCard
            label="Egress serialization cost"
            value="~4.7 ns -> ~82 ns"
            note="From uncontended send path to contested parallel sends on one egress lane"
          />
        </section>

        <section className={Styles.section}>
          <div className={Styles.sectionHeader}>
            <div>
              <p className={Styles.sectionEyebrow}>Full-cycle dataplane</p>
              <Heading as="h2" className={Styles.sectionTitle}>
                1400B snapshot
              </Heading>
            </div>
            <p className={Styles.sectionText}>
              These numbers cover encrypt, route/peer lookup, validation, decrypt, and handoff to an in-memory sink.
              They are useful as an upper-bound for the dataplane core, not as end-to-end VPN throughput claims.
            </p>
          </div>
          <FullCycleTable />
        </section>

        <section className={Styles.section}>
          <div className={Styles.sectionHeader}>
            <div>
              <p className={Styles.sectionEyebrow}>Parallel UDP scaling</p>
              <Heading as="h2" className={Styles.sectionTitle}>
                One peer is not the same as many peers
              </Heading>
            </div>
            <p className={Styles.sectionText}>
              The parallel-peer benchmarks highlight the difference between single-flow serialization and multi-peer
              aggregate throughput. The charts below are not the same dataset as the 1400B full-cycle snapshot above:
              they show how UDP scales when work is spread across many peers instead of one peer&apos;s serialized send
              lane.
            </p>
          </div>

          <div className={Styles.chartGrid}>
            {benchmarkSnapshot.parallelPeers1400.map((entry) => (
              <div key={`${entry.id}-throughput`} className={Styles.chartCard}>
                <div className={Styles.chartHeader}>
                  <Heading as="h3" className={Styles.chartTitle}>
                    {entry.label}
                  </Heading>
                  <span className={Styles.chartMetric}>Aggregate throughput</span>
                </div>
                <Sparkline
                  series={entry.series}
                  valueKey="throughput"
                  color="#009fc9"
                  formatter={(value) => formatThroughput(value)}
                />
              </div>
            ))}

            {benchmarkSnapshot.parallelPeers1400.map((entry) => (
              <div key={`${entry.id}-latency`} className={Styles.chartCard}>
                <div className={Styles.chartHeader}>
                  <Heading as="h3" className={Styles.chartTitle}>
                    {entry.label}
                  </Heading>
                  <span className={Styles.chartMetric}>Latency</span>
                </div>
                <Sparkline
                  series={entry.series}
                  valueKey="ns"
                  color="#475569"
                  formatter={(value) => formatNs(value)}
                />
              </div>
            ))}
          </div>
        </section>

        <section className={Styles.section}>
          <div className={Styles.sectionHeader}>
            <div>
              <p className={Styles.sectionEyebrow}>Repository behavior</p>
              <Heading as="h2" className={Styles.sectionTitle}>
                Fast paths are flat. Misses are not.
              </Heading>
            </div>
            <p className={Styles.sectionText}>
              Internal-IP, allowed-host, and route-ID lookups stay effectively constant out to 10k peers. The miss path
              is intentionally shown beside them because it is the part that already behaves like a real scalability
              constraint.
            </p>
          </div>
          <FastPathTable />
        </section>

        <section className={Styles.section}>
          <div className={Styles.callout}>
            <Heading as="h2" className={Styles.calloutTitle}>
              What these benchmarks already tell us
            </Heading>
            <div className={Styles.calloutGrid}>
              <div>
                <span className={Styles.calloutLabel}>Strong dataplane core</span>
                <p>
                  Full-cycle UDP and TCP packet processing stays in the low-microsecond range for 1400-byte packets and
                  remains allocation-free on the hot path.
                </p>
              </div>
              <div>
                <span className={Styles.calloutLabel}>Real architectural pressure points</span>
                <p>
                  The relevant bottlenecks are no longer basic crypto or packet plumbing. They are repository misses and
                  per-peer serialization under contention.
                </p>
              </div>
            </div>
            <Link className="button button--primary button--lg" to="/docs/QuickStart">
              Back to docs
            </Link>
          </div>
        </section>
      </main>
    </Layout>
  );
}
