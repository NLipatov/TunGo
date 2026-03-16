export const benchmarkSnapshot = {
  generatedAt: '2026-03-15',
  machine: 'Apple M4 Pro',
  goVersion: 'Go 1.26',
  fullCycle1400: [
    {id: 'udp-c2s', transport: 'UDP', direction: 'Client -> Server', ns: 2658, throughput: 534.15, allocs: 0},
    {id: 'udp-s2c', transport: 'UDP', direction: 'Server -> Client', ns: 2649, throughput: 536.08, allocs: 0},
    {id: 'tcp-c2s', transport: 'TCP', direction: 'Client -> Server', ns: 2622, throughput: 541.49, allocs: 0},
    {id: 'tcp-s2c', transport: 'TCP', direction: 'Server -> Client', ns: 2633, throughput: 539.26, allocs: 0},
  ],
  parallelPeers1400: [
    {
      id: 'udp-c2s-parallel',
      label: 'UDP Client -> Server',
      series: [
        {peers: 1, ns: 3337, throughput: 425.57},
        {peers: 64, ns: 328.4, throughput: 4324.34},
        {peers: 1024, ns: 332.4, throughput: 4272.50},
      ],
    },
    {
      id: 'udp-s2c-parallel',
      label: 'UDP Server -> Client',
      series: [
        {peers: 1, ns: 3334, throughput: 425.89},
        {peers: 64, ns: 349.4, throughput: 4063.60},
        {peers: 1024, ns: 355.9, throughput: 3990.34},
      ],
    },
  ],
  repository: {
    fastPath: [
      {
        id: 'exact-internal',
        label: 'Exact internal lookup',
        series: [
          {peers: 1, ns: 8.677},
          {peers: 100, ns: 9.000},
          {peers: 1000, ns: 9.341},
          {peers: 10000, ns: 9.303},
        ],
      },
      {
        id: 'allowed-host',
        label: 'Allowed host lookup',
        series: [
          {peers: 1, ns: 13.49},
          {peers: 100, ns: 14.95},
          {peers: 1000, ns: 13.40},
          {peers: 10000, ns: 14.11},
        ],
      },
      {
        id: 'route-id',
        label: 'Route ID lookup',
        series: [
          {peers: 1, ns: 3.920},
          {peers: 100, ns: 6.378},
          {peers: 1000, ns: 6.032},
          {peers: 10000, ns: 6.604},
        ],
      },
    ],
    missPath: [
      {peers: 1, ns: 35.39},
      {peers: 100, ns: 699.2},
      {peers: 1000, ns: 9014},
      {peers: 10000, ns: 89484},
    ],
  },
  egress: {
    uncontendedNs: 4.706,
    contendedNs: 80.15,
  },
};
