import React from 'react';
import Clsx from 'clsx';
import Translate, {translate} from '@docusaurus/Translate';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import Styles from './benchmarks.module.css';
import {benchmarkSnapshot} from '@site/src/data/benchmarks';

function formatNs(ns) {
  if (ns >= 1000) {
    return `~${(ns / 1000).toFixed(1)} μs`;
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

function translateTransport(id) {
  const transportById = {
    'udp-c2s': translate({id: 'bench.transport.udp', message: 'UDP'}),
    'udp-s2c': translate({id: 'bench.transport.udp', message: 'UDP'}),
    'tcp-c2s': translate({id: 'bench.transport.tcp', message: 'TCP'}),
    'tcp-s2c': translate({id: 'bench.transport.tcp', message: 'TCP'}),
  };

  return transportById[id] || id.toUpperCase();
}

function translateDirection(id) {
  const directionById = {
    'udp-c2s': translate({id: 'bench.direction.clientServer', message: 'Client -> Server'}),
    'tcp-c2s': translate({id: 'bench.direction.clientServer', message: 'Client -> Server'}),
    'udp-s2c': translate({id: 'bench.direction.serverClient', message: 'Server -> Client'}),
    'tcp-s2c': translate({id: 'bench.direction.serverClient', message: 'Server -> Client'}),
  };

  return directionById[id] || id;
}

function translateParallelLabel(id) {
  const labelById = {
    'udp-c2s-parallel': translate({
      id: 'bench.parallel.clientServer',
      message: 'UDP Client -> Server',
    }),
    'udp-s2c-parallel': translate({
      id: 'bench.parallel.serverClient',
      message: 'UDP Server -> Client',
    }),
  };

  return labelById[id] || id;
}

function translateLookupLabel(id) {
  const labelById = {
    'exact-internal': translate({
      id: 'bench.lookup.exactInternal',
      message: 'Exact internal lookup',
    }),
    'allowed-host': translate({
      id: 'bench.lookup.allowedHost',
      message: 'Allowed host lookup',
    }),
    'route-id': translate({
      id: 'bench.lookup.routeId',
      message: 'Route ID lookup',
    }),
  };

  return labelById[id] || id;
}

function buildPolyline(series, valueKey, width, height, padding) {
  const values = series.map((point) => point[valueKey]);
  const peerLogs = series.map((point) => Math.log10(point.peers));
  const min = Math.min(...values);
  const max = Math.max(...values);
  const minPeerLog = Math.min(...peerLogs);
  const maxPeerLog = Math.max(...peerLogs);
  const innerWidth = width - padding * 2;
  const innerHeight = height - padding * 2;

  return series
    .map((point) => {
      const peerLog = Math.log10(point.peers);
      const x =
        maxPeerLog === minPeerLog
          ? padding + innerWidth / 2
          : padding + ((peerLog - minPeerLog) / (maxPeerLog - minPeerLog)) * innerWidth;
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
  const peerLogs = series.map((point) => Math.log10(point.peers));
  const min = Math.min(...values);
  const max = Math.max(...values);
  const minPeerLog = Math.min(...peerLogs);
  const maxPeerLog = Math.max(...peerLogs);
  const innerWidth = width - padding * 2;
  const innerHeight = height - padding * 2;

  return series.map((point) => {
    const peerLog = Math.log10(point.peers);
    const x =
      maxPeerLog === minPeerLog
        ? padding + innerWidth / 2
        : padding + ((peerLog - minPeerLog) / (maxPeerLog - minPeerLog)) * innerWidth;
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
            <span className={Styles.legendPeers}>
              {translate(
                {
                  id: 'bench.legend.peers',
                  message: '{count} peers',
                },
                {count: point.peers},
              )}
            </span>
            <span className={Styles.legendValue}>{formatter(point[valueKey])}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function MetricCard({label, value, note, className}) {
  return (
    <div className={`${Styles.metricCard}${className ? ` ${className}` : ''}`}>
      <div className={Styles.metricLabel}>{label}</div>
      <div className={Styles.metricValue}>{value}</div>
      <div className={Styles.metricNote}>{note}</div>
    </div>
  );
}

function DataTable({header, rows, ariaLabel, className}) {
  return (
    <div className={Styles.tableWrap}>
      <table className={`${Styles.dataTable}${className ? ` ${className}` : ''}`} aria-label={ariaLabel}>
        <thead>
          <tr>
            {header.map((cell) => (
              <th key={cell} scope="col">
                {cell}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, rowIndex) => (
            <tr key={`row-${rowIndex}`}>
              {row.map((cell, columnIndex) => (
                <td key={`row-${rowIndex}-col-${columnIndex}`}>{cell}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function FullCycleTable() {
  const rows = benchmarkSnapshot.fullCycle1400.map((entry) => [
    <>
      <span className={Styles.tablePrimary}>{translateTransport(entry.id)}</span>
      <span className={Styles.tableSecondary}>{translateDirection(entry.id)}</span>
    </>,
    formatNs(entry.ns),
    formatThroughput(entry.throughput),
    String(entry.allocs),
  ]);

  return (
    <DataTable
      ariaLabel={translate({
        id: 'bench.table.fullCycleAria',
        message: '1400-byte full-cycle dataplane benchmark results',
      })}
      className={Styles.fullCycleTable}
      header={[
        translate({id: 'bench.table.path', message: 'Path'}),
        translate({id: 'bench.table.latency', message: 'Latency'}),
        translate({id: 'bench.table.throughput', message: 'Throughput'}),
        translate({id: 'bench.table.allocs', message: 'Allocs/op'}),
      ]}
      rows={rows}
    />
  );
}

function FastPathTable() {
  const peerCounts = benchmarkSnapshot.repository.fastPath[0].series.map((entry) => entry.peers);
  const rows = [
    ...benchmarkSnapshot.repository.fastPath.map((row) => [
      translateLookupLabel(row.id),
      ...row.series.map((point) => formatNs(point.ns)),
    ]),
    [
      translate({id: 'bench.lookup.missPath', message: 'Miss path'}),
      ...benchmarkSnapshot.repository.missPath.map((point) => formatNs(point.ns)),
    ],
  ];

  return (
    <DataTable
      ariaLabel={translate({
        id: 'bench.table.lookupAria',
        message: 'Repository lookup and miss-path benchmark results',
      })}
      className={Styles.fastPathTable}
      header={[
        translate({id: 'bench.table.lookup', message: 'Lookup'}),
        ...peerCounts.map((count) =>
          translate(
            {
              id: 'bench.table.peerCount',
              message: '{count} peers',
            },
            {count},
          ),
        ),
      ]}
      rows={rows}
    />
  );
}

export default function BenchmarksPage() {
  const bestFullCycle = [...benchmarkSnapshot.fullCycle1400].sort((a, b) => b.throughput - a.throughput)[0];
  const lowestLatency = [...benchmarkSnapshot.fullCycle1400].sort((a, b) => a.ns - b.ns)[0];

  return (
    <Layout
      title={translate({id: 'bench.page.title', message: 'Benchmarks'})}
      description={translate({
        id: 'bench.page.description',
        message: 'Benchmark dashboard for TunGo dataplane throughput, latency, repository lookup costs, and egress contention.',
      })}
    >
      <main className={Styles.page}>
        <section className={Clsx('container', Styles.hero)}>
          <div className={Styles.heroIntro}>
            <Heading as="h1" className={Styles.title}>
              <Translate id="bench.hero.title">Benchmark snapshot</Translate>
            </Heading>
            <p className={Styles.lead}>
              {translate(
                {
                  id: 'bench.hero.lead',
                  message: 'Manual snapshot measured on {machine} with {goVersion}.',
                },
                {machine: benchmarkSnapshot.machine, goVersion: benchmarkSnapshot.goVersion},
              )}
            </p>
          </div>
        </section>

        <section className={Clsx('container', Styles.metrics)}>
          <MetricCard
            className={Styles.metricCardPrimary}
            label={translate({id: 'bench.metric.throughput', message: 'Throughput'})}
            value={formatThroughput(bestFullCycle.throughput)}
            note={translate({id: 'bench.metric.throughputNote', message: 'Best full-cycle path'})}
          />
          <MetricCard
            label={translate({id: 'bench.metric.latency', message: 'Latency'})}
            value={formatNs(lowestLatency.ns)}
            note={translate({id: 'bench.metric.latencyNote', message: 'Lowest full-cycle path'})}
          />
          <MetricCard
            label={translate({id: 'bench.metric.lookup', message: 'Fast-path lookup'})}
            value="~4-15 ns"
            note={translate({id: 'bench.metric.lookupNote', message: 'Flat through 10k peers'})}
          />
          <MetricCard
            label={translate({id: 'bench.metric.allocs', message: 'Allocs/op'})}
            value="0"
            note={translate({id: 'bench.metric.allocsNote', message: 'Hot path'})}
          />
        </section>

        <section className={Clsx('container', Styles.section)}>
          <div className={Styles.splitSection}>
            <div className={Styles.splitCopy}>
              <Heading as="h2" className={Styles.sectionTitle}>
                <Translate id="bench.section.fullCycle.title">Full-cycle dataplane</Translate>
              </Heading>
              <p className={Styles.sectionText}>
                <Translate id="bench.section.fullCycle.text">
                  Encrypt, lookup, validate, decrypt, handoff. Upper-bound for the dataplane core, not end-to-end VPN throughput.
                </Translate>
              </p>
            </div>
            <div className={Styles.splitTable}>
              <FullCycleTable />
            </div>
          </div>
        </section>

        <section className={Clsx('container', Styles.section)}>
          <div className={Styles.sectionHeader}>
            <Heading as="h2" className={Styles.sectionTitle}>
              <Translate id="bench.section.scaling.title">Multi-peer UDP scaling</Translate>
            </Heading>
            <p className={Styles.sectionText}>
              <Translate id="bench.section.scaling.text">
                Aggregate throughput with work spread across many peers, not one serialized send lane.
              </Translate>
            </p>
          </div>

          <div className={Styles.chartGrid}>
            {benchmarkSnapshot.parallelPeers1400.map((entry) => (
              <div key={`${entry.id}-throughput`} className={Styles.chartCard}>
                <div className={Styles.chartHeader}>
                  <Heading as="h3" className={Styles.chartTitle}>
                    {translateParallelLabel(entry.id)}
                  </Heading>
                  <span className={Styles.chartMetric}>
                    <Translate id="bench.chart.aggregateThroughput">Aggregate throughput</Translate>
                  </span>
                </div>
                <Sparkline
                  series={entry.series}
                  valueKey="throughput"
                  color="#009fc9"
                  formatter={(value) => formatThroughput(value)}
                />
              </div>
            ))}
          </div>
        </section>

        <section className={Clsx('container', Styles.section)}>
          <div className={Styles.splitSection}>
            <div className={Styles.splitCopy}>
              <Heading as="h2" className={Styles.sectionTitle}>
                <Translate id="bench.section.lookup.title">Lookup and serialization</Translate>
              </Heading>
              <p className={Styles.sectionText}>
                <Translate id="bench.section.lookup.text">
                  Internal-IP, allowed-host, and route-ID lookups stay flat. Misses and per-peer serialization are the real pressure points.
                </Translate>
              </p>
            </div>
            <div className={Styles.splitTable}>
              <FastPathTable />
            </div>
          </div>
          <div className={Styles.summaryRow}>
            <MetricCard
              className={Styles.metricCardMuted}
              label={translate({id: 'bench.summary.egress', message: 'Egress lane'})}
              value={`~${benchmarkSnapshot.egress.uncontendedNs.toFixed(1)} ns -> ~${benchmarkSnapshot.egress.contendedNs.toFixed(0)} ns`}
              note={translate({id: 'bench.summary.egressNote', message: 'Uncontended to contended sends'})}
            />
            <MetricCard
              className={Styles.metricCardMuted}
              label={translate({id: 'bench.summary.missPath', message: 'Miss path'})}
              value={translate({id: 'bench.summary.linear', message: 'Linear'})}
              note={translate({id: 'bench.summary.missPathNote', message: '~35 ns at 1 peer -> ~89.5 μs at 10k peers'})}
            />
          </div>
        </section>
      </main>
    </Layout>
  );
}
