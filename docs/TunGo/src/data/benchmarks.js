export const benchmarkSnapshot = {
  generatedAt: '2026-03-15',
  machine: 'Apple M4 Pro',
  goVersion: 'Go 1.26',
  scope:
    'In-memory userspace dataplane benchmarks. These numbers exclude TUN, socket, kernel, NAT, firewall, and real network overhead.',
  fullCycle1400: [
    {id: 'udp-c2s', transport: 'UDP', direction: 'Client -> Server', ns: 2579, throughput: 550.56, allocs: 0},
    {id: 'udp-s2c', transport: 'UDP', direction: 'Server -> Client', ns: 2874, throughput: 494.10, allocs: 0},
    {id: 'tcp-c2s', transport: 'TCP', direction: 'Client -> Server', ns: 2515, throughput: 564.56, allocs: 0},
    {id: 'tcp-s2c', transport: 'TCP', direction: 'Server -> Client', ns: 2564, throughput: 553.73, allocs: 0},
  ],
  parallelPeers1400: [
    {
      id: 'udp-c2s-parallel',
      label: 'UDP Client -> Server',
      series: [
        {peers: 1, ns: 4245, throughput: 334.55},
        {peers: 64, ns: 532.6, throughput: 2666.21},
        {peers: 1024, ns: 404.9, throughput: 3506.79},
      ],
    },
    {
      id: 'udp-s2c-parallel',
      label: 'UDP Server -> Client',
      series: [
        {peers: 1, ns: 3182, throughput: 446.21},
        {peers: 64, ns: 511.3, throughput: 2777.08},
        {peers: 1024, ns: 480.6, throughput: 2954.59},
      ],
    },
  ],
  repository: {
    fastPath: [
      {
        id: 'exact-internal',
        label: 'Exact internal lookup',
        series: [
          {peers: 1, ns: 8.704},
          {peers: 100, ns: 9.235},
          {peers: 1000, ns: 9.413},
          {peers: 10000, ns: 9.505},
        ],
      },
      {
        id: 'allowed-host',
        label: 'Allowed host lookup',
        series: [
          {peers: 1, ns: 13.66},
          {peers: 100, ns: 13.89},
          {peers: 1000, ns: 13.55},
          {peers: 10000, ns: 15.47},
        ],
      },
      {
        id: 'route-id',
        label: 'Route ID lookup',
        series: [
          {peers: 1, ns: 3.894},
          {peers: 100, ns: 5.760},
          {peers: 1000, ns: 6.270},
          {peers: 10000, ns: 6.371},
        ],
      },
    ],
    missPath: [
      {peers: 1, ns: 35.96},
      {peers: 100, ns: 704.0},
      {peers: 1000, ns: 9001},
      {peers: 10000, ns: 92342},
    ],
  },
  egress: {
    uncontendedNs: 4.666,
    contendedNs: 82.36,
  },
};
