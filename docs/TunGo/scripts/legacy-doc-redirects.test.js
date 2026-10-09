const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const buildRoot = path.resolve(__dirname, '..', 'build');
const archiveRoot =
  'https://github.com/NLipatov/TunGo/blob/6077246b2e790ea34e52800d3304e9245267941b/docs/TunGo';

const redirects = [
  ['docs/introduction to TunGo', 'https://tungo.ethacore.com/'],
  ['ru/docs/introduction to TunGo', 'https://tungo.ethacore.com/ru/'],
  [
    'docs/Architecture/ADR/ADR-001',
    `${archiveRoot}/docs/Architecture/ADR/ADR-001.md`,
  ],
  ...['fr', 'zh-Hans'].map((locale) => [
    `${locale}/docs/Architecture/ADR/ADR-001`,
    `${archiveRoot}/i18n/${locale}/docusaurus-plugin-content-docs/current/Architecture/ADR/ADR-001.md`,
  ]),
];

for (const [source, destination] of redirects) {
  test(`${source} redirects immediately to its replacement`, () => {
    const html = fs.readFileSync(
      path.join(buildRoot, source, 'index.html'),
      'utf8',
    );
    const refresh = html.match(
      /<meta\s+http-equiv="refresh"\s+content="([^"]+)"/i,
    )?.[1];
    const canonical = html.match(
      /<link\s+rel="canonical"\s+href="([^"]+)"/i,
    )?.[1];

    assert.equal(refresh, `0; url=${destination}`);
    assert.equal(canonical, destination);
    assert.ok(
      html.includes(`<a href="${destination}">`),
      'A fallback link is available',
    );

    const localeRoot = source.startsWith('docs/')
      ? buildRoot
      : path.join(buildRoot, source.split('/')[0]);
    const sitemap = fs.readFileSync(
      path.join(localeRoot, 'sitemap.xml'),
      'utf8',
    );
    assert.ok(
      !sitemap.includes(encodeURI(source)),
      'Redirect URLs must not enter the sitemap',
    );
    assert.ok(
      !sitemap.includes(source),
      'Redirect URLs must not enter the sitemap unencoded',
    );

    const target = new URL(destination);
    if (target.hostname === 'tungo.ethacore.com') {
      assert.ok(
        fs.existsSync(path.join(buildRoot, target.pathname, 'index.html')),
      );
    }
  });
}

test('removed benchmark pages stay absent', () => {
  for (const locale of ['', 'ar', 'ja']) {
    assert.equal(
      fs.existsSync(path.join(buildRoot, locale, 'benchmarks')),
      false,
    );
    const sitemap = fs.readFileSync(
      path.join(buildRoot, locale, 'sitemap.xml'),
      'utf8',
    );
    assert.ok(!sitemap.includes('/benchmarks'));
  }
});
