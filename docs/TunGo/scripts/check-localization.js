const fs = require('node:fs');
const path = require('node:path');

const sourceLocale = 'en';
const projectRoot = path.resolve(__dirname, '..');
const i18nRoot = path.join(projectRoot, 'i18n');
const docsRoot = path.join(projectRoot, 'docs');
const icuPluralPattern = /\{\s*[\w.]+\s*,\s*plural\s*,/;
const errors = [];

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function listRelativeFiles(rootPath) {
  const files = [];

  function walk(currentPath, currentPrefix = '') {
    for (const entry of fs.readdirSync(currentPath, {withFileTypes: true})) {
      const relativePath = path.join(currentPrefix, entry.name);
      const fullPath = path.join(currentPath, entry.name);

      if (entry.isDirectory()) {
        walk(fullPath, relativePath);
      } else {
        files.push(relativePath);
      }
    }
  }

  walk(rootPath);
  return files.sort();
}

const locales = fs
  .readdirSync(i18nRoot)
  .filter((locale) => fs.statSync(path.join(i18nRoot, locale)).isDirectory())
  .sort();

const sourceCodeJsonPath = path.join(i18nRoot, sourceLocale, 'code.json');
const sourceCodeMessages = readJson(sourceCodeJsonPath);
const sourceCustomCodeKeys = Object.keys(sourceCodeMessages)
  .filter((key) => !key.startsWith('theme.'))
  .sort();

for (const locale of locales) {
  const codeJsonPath = path.join(i18nRoot, locale, 'code.json');

  if (!fs.existsSync(codeJsonPath)) {
    errors.push(`Missing code translations file for locale "${locale}": ${path.relative(projectRoot, codeJsonPath)}`);
    continue;
  }

  const messages = readJson(codeJsonPath);
  const pluralCategories = new Intl.PluralRules(locale).resolvedOptions().pluralCategories;

  for (const [key, entry] of Object.entries(messages)) {
    const message = entry?.message;

    if (typeof message !== 'string') {
      continue;
    }

    if (icuPluralPattern.test(message)) {
      errors.push(
        `${path.relative(projectRoot, codeJsonPath)}:${key} uses ICU plural syntax. ` +
          'Docusaurus expects pipe-separated plural forms selected through usePluralMessage()/usePluralForm().',
      );
    }

    if (key.endsWith('.plurals')) {
      const forms = message.split('|').length;

      if (pluralCategories.length > 1 && forms < 2) {
        errors.push(
          `${path.relative(projectRoot, codeJsonPath)}:${key} defines only ${forms} plural form(s) ` +
            `for locale "${locale}". Provide at least singular and plural variants.`,
        );
      }
    }
  }

  if (locale !== sourceLocale) {
    const localeCustomKeys = Object.keys(messages)
      .filter((key) => !key.startsWith('theme.'))
      .sort();
    const missingCustomKeys = sourceCustomCodeKeys.filter((key) => !localeCustomKeys.includes(key));
    const extraCustomKeys = localeCustomKeys.filter((key) => !sourceCustomCodeKeys.includes(key));

    for (const missingKey of missingCustomKeys) {
      errors.push(
        `${path.relative(projectRoot, codeJsonPath)} is missing custom translation key "${missingKey}" present in ${sourceLocale}/code.json.`,
      );
    }

    for (const extraKey of extraCustomKeys) {
      errors.push(
        `${path.relative(projectRoot, codeJsonPath)} has extra custom translation key "${extraKey}" missing from ${sourceLocale}/code.json.`,
      );
    }
  }
}

const sourceNavbarJsonPath = path.join(i18nRoot, sourceLocale, 'docusaurus-theme-classic', 'navbar.json');
const sourceNavbarKeys = Object.keys(readJson(sourceNavbarJsonPath)).sort();

for (const locale of locales.filter((locale) => locale !== sourceLocale)) {
  const navbarJsonPath = path.join(i18nRoot, locale, 'docusaurus-theme-classic', 'navbar.json');

  if (!fs.existsSync(navbarJsonPath)) {
    errors.push(`Missing navbar translations file for locale "${locale}": ${path.relative(projectRoot, navbarJsonPath)}`);
    continue;
  }

  const navbarKeys = Object.keys(readJson(navbarJsonPath)).sort();
  const missingNavbarKeys = sourceNavbarKeys.filter((key) => !navbarKeys.includes(key));
  const extraNavbarKeys = navbarKeys.filter((key) => !sourceNavbarKeys.includes(key));

  for (const missingKey of missingNavbarKeys) {
    errors.push(
      `${path.relative(projectRoot, navbarJsonPath)} is missing navbar translation key "${missingKey}" present in ${sourceLocale}/docusaurus-theme-classic/navbar.json.`,
    );
  }

  for (const extraKey of extraNavbarKeys) {
    errors.push(
      `${path.relative(projectRoot, navbarJsonPath)} has extra navbar translation key "${extraKey}" missing from ${sourceLocale}/docusaurus-theme-classic/navbar.json.`,
    );
  }
}

const sourceDocsFiles = listRelativeFiles(docsRoot);

for (const locale of locales.filter((locale) => locale !== sourceLocale)) {
  const localeDocsRoot = path.join(i18nRoot, locale, 'docusaurus-plugin-content-docs', 'current');

  if (!fs.existsSync(localeDocsRoot)) {
    errors.push(`Missing localized docs tree for locale "${locale}": ${path.relative(projectRoot, localeDocsRoot)}`);
    continue;
  }

  const localeDocsFiles = listRelativeFiles(localeDocsRoot);
  const missingDocsFiles = sourceDocsFiles.filter((file) => !localeDocsFiles.includes(file));
  const extraDocsFiles = localeDocsFiles.filter((file) => !sourceDocsFiles.includes(file));

  for (const missingFile of missingDocsFiles) {
    errors.push(`${path.relative(projectRoot, localeDocsRoot)} is missing docs file "${missingFile}" from source docs.`);
  }

  for (const extraFile of extraDocsFiles) {
    errors.push(`${path.relative(projectRoot, localeDocsRoot)} has extra docs file "${extraFile}" that is absent in source docs.`);
  }
}

if (errors.length > 0) {
  console.error('Localization validation failed:\n');
  for (const error of errors) {
    console.error(`- ${error}`);
  }
  process.exit(1);
}

console.log('Localization validation passed.');
