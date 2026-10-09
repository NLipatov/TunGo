const fs = require('node:fs/promises');
const path = require('node:path');

// Last revision before ADR-001 was removed from the documentation.
const archiveRoot =
  'https://github.com/NLipatov/TunGo/blob/6077246b2e790ea34e52800d3304e9245267941b/docs/TunGo';

module.exports = function legacyDocRedirects({i18n: {currentLocale}}) {
  const archivedDocs =
    currentLocale === 'en'
      ? 'docs'
      : `i18n/${currentLocale}/docusaurus-plugin-content-docs/current`;

  return {
    name: 'legacy-doc-redirects',
    async postBuild({outDir, siteConfig}) {
      const redirects = [
        [
          'docs/introduction to TunGo',
          new URL(siteConfig.baseUrl, siteConfig.url).href,
        ],
        [
          'docs/Architecture/ADR/ADR-001',
          `${archiveRoot}/${archivedDocs}/Architecture/ADR/ADR-001.md`,
        ],
      ];

      // GitHub Pages serves these static redirects; they are not sitemap routes.
      for (const [source, destination] of redirects) {
        const directory = path.join(outDir, source);
        await fs.mkdir(directory, {recursive: true});
        await fs.writeFile(
          path.join(directory, 'index.html'),
          `<!doctype html>
<html lang="${currentLocale}">
<head>
  <meta charset="utf-8">
  <meta http-equiv="refresh" content="0; url=${destination}">
  <link rel="canonical" href="${destination}">
  <title>Redirecting</title>
</head>
<body><a href="${destination}">${destination}</a></body>
</html>
`,
          {flag: 'wx'},
        );
      }
    },
  };
};
