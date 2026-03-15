import React from 'react';
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
            <span className={Styles.legendPeers}>{point.peers} peers</span>
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

function GridTable({columns, header, rows, ariaLabel, className}) {
  const templateColumns = columns.join(' ');
  const lastRowIndex = rows.length;
  const lastColumnIndex = header.length - 1;

  return (
    <div className={Styles.tableWrap}>
      <div
        className={`${Styles.matrixTable}${className ? ` ${className}` : ''}`}
        style={{gridTemplateColumns: templateColumns}}
        role="table"
        aria-label={ariaLabel}
      >
        {header.map((cell, index) => (
          <div
            key={`head-${cell}`}
            className={`${Styles.matrixCell} ${Styles.matrixHead}`}
            style={{borderRight: index === lastColumnIndex ? 0 : undefined}}
            role="columnheader"
          >
            {cell}
          </div>
        ))}
        {rows.map((row, rowIndex) =>
          row.map((cell, columnIndex) => (
            <div
              key={`row-${rowIndex}-col-${columnIndex}`}
              className={Styles.matrixCell}
              style={{
                borderRight: columnIndex === lastColumnIndex ? 0 : undefined,
                borderBottom: rowIndex === lastRowIndex - 1 ? 0 : undefined,
              }}
              role="cell"
            >
              {cell}
            </div>
          )),
        )}
      </div>
    </div>
  );
}

function FullCycleTable() {
  const rows = benchmarkSnapshot.fullCycle1400.map((entry) => [
    <>
      <span className={Styles.tablePrimary}>{entry.transport}</span>
      <span className={Styles.tableSecondary}>{entry.direction}</span>
    </>,
    formatNs(entry.ns),
    formatThroughput(entry.throughput),
    String(entry.allocs),
  ]);

  return (
    <GridTable
      ariaLabel="1400-byte full-cycle dataplane benchmark results"
      className={Styles.fullCycleTable}
      columns={['1.35fr', '0.9fr', '1fr', '0.7fr']}
      header={['Path', 'Latency', 'Throughput', 'Allocs/op']}
      rows={rows}
    />
  );
}

function FastPathTable() {
  const peerCounts = benchmarkSnapshot.repository.fastPath[0].series.map((entry) => entry.peers);
  const rows = [
    ...benchmarkSnapshot.repository.fastPath.map((row) => [
      row.label,
      ...row.series.map((point) => formatNs(point.ns)),
    ]),
    ['Miss path', ...benchmarkSnapshot.repository.missPath.map((point) => formatNs(point.ns))],
  ];

  return (
    <GridTable
      ariaLabel="Repository lookup and miss-path benchmark results"
      className={Styles.fastPathTable}
      columns={[
        '1.5fr',
        '0.8fr',
        '0.8fr',
        '0.9fr',
        '0.95fr',
      ]}
      header={['Lookup', ...peerCounts.map((count) => `${count} peers`)]}
      rows={rows}
    />
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
            <Heading as="h1" className={Styles.title}>
              Benchmark snapshot
            </Heading>
            <p className={Styles.lead}>Manual snapshot measured on {benchmarkSnapshot.machine} with {benchmarkSnapshot.goVersion}.</p>
          </div>
        </section>

        <section className={Styles.metrics}>
          <MetricCard
            className={Styles.metricCardPrimary}
            label="Throughput"
            value={formatThroughput(bestFullCycle.throughput)}
            note="Best full-cycle path"
          />
          <MetricCard
            label="Latency"
            value={formatNs(lowestLatency.ns)}
            note="Lowest full-cycle path"
          />
          <MetricCard
            label="Fast-path lookup"
            value="~4-15 ns"
            note="Flat through 10k peers"
          />
          <MetricCard
            label="Allocs/op"
            value="0"
            note="Hot path"
          />
        </section>

        <section className={Styles.section}>
          <div className={Styles.splitSection}>
            <div className={Styles.splitCopy}>
              <Heading as="h2" className={Styles.sectionTitle}>
                1400B full-cycle
              </Heading>
              <p className={Styles.sectionText}>
                Encrypt, lookup, validate, decrypt, handoff. Upper-bound for the dataplane core, not end-to-end VPN throughput.
              </p>
            </div>
            <div className={Styles.splitTable}>
              <FullCycleTable />
            </div>
          </div>
        </section>

        <section className={Styles.section}>
          <div className={Styles.sectionHeader}>
            <Heading as="h2" className={Styles.sectionTitle}>
              Multi-peer UDP scaling
            </Heading>
            <p className={Styles.sectionText}>
              Aggregate throughput with work spread across many peers, not one serialized send lane.
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
          </div>
        </section>

        <section className={Styles.section}>
          <div className={Styles.sectionHeader}>
            <Heading as="h2" className={Styles.sectionTitle}>
              Lookup and serialization
            </Heading>
            <p className={Styles.sectionText}>
              Internal-IP, allowed-host, and route-ID lookups stay flat. Misses and per-peer serialization are the real pressure points.
            </p>
          </div>
          <FastPathTable />
          <div className={Styles.summaryRow}>
            <MetricCard
              className={Styles.metricCardMuted}
              label="Egress lane"
              value={`~${benchmarkSnapshot.egress.uncontendedNs.toFixed(1)} ns -> ~${benchmarkSnapshot.egress.contendedNs.toFixed(0)} ns`}
              note="Uncontended to contended sends"
            />
            <MetricCard
              className={Styles.metricCardMuted}
              label="Miss path"
              value="Linear"
              note="~35 ns at 1 peer -> ~89.5 μs at 10k peers"
            />
          </div>
        </section>
      </main>
    </Layout>
  );
}
