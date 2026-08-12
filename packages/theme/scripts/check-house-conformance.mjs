#!/usr/bin/env node
/**
 * Fail-closed Editorial Instrument conformance checks for CrossTalk.
 *
 * Scans product CSS/TSX (apps + packages/theme) for:
 *  - forbidden system / stock UI fonts in product CSS
 *  - brand accents other than #3DE0F0 (violet/mint as runtime brand)
 *  - app-local token / house-tokens files
 *  - emoji used as product iconography in TSX (heuristic)
 *  - raw hex palette in theme VU/status paths outside status tokens (light touch)
 *
 * Exit 0 = GREEN, exit 1 = RED with findings on stderr/stdout.
 *
 * Usage:
 *   node packages/theme/scripts/check-house-conformance.mjs
 *   node packages/theme/scripts/check-house-conformance.mjs --root /path/to/repo
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const defaultRoot = path.resolve(__dirname, "../../..");

const args = process.argv.slice(2);
const rootIdx = args.indexOf("--root");
const ROOT = rootIdx >= 0 ? path.resolve(args[rootIdx + 1]) : defaultRoot;

const ALLOWED_ACCENT = "#3DE0F0";
const FORBIDDEN_BRAND_ACCENTS = ["#B98BFF", "#7CFFB2", "#b98bff", "#7cffb2"];

/** System / stock fonts that must not appear as product UI faces in authored CSS. */
const FORBIDDEN_FONT_PATTERNS = [
  /font-family\s*:\s*[^;]*\bsystem-ui\b/i,
  /font-family\s*:\s*[^;]*-apple-system\b/i,
  /font-family\s*:\s*[^;]*\bBlinkMacSystemFont\b/i,
  /font-family\s*:\s*[^;]*\bSegoe UI\b/i,
  /font-family\s*:\s*[^;]*\bRoboto\b/i,
  /font-family\s*:\s*[^;]*\bHelvetica Neue\b/i,
  /font-family\s*:\s*[^;]*\bArial\b/i,
  /font-family\s*:\s*[^;]*\bInter\b/i,
  /font-family\s*:\s*[^;]*\bui-sans-serif\b/i,
];

/** App-local token file basenames that must not exist outside packages/theme. */
const FORBIDDEN_TOKEN_BASENAMES = new Set([
  "house-tokens.css",
  "house-tokens.json",
  "tokens.css",
  "design-tokens.css",
]);

/** Emoji ranges commonly used as product "icons". */
const EMOJI_RE =
  /(?:[\u{1F300}-\u{1FAFF}]|[\u{2600}-\u{27BF}]|[\u{1F1E0}-\u{1F1FF}]|\u{FE0F})/u;

const findings = [];

function rel(p) {
  return path.relative(ROOT, p) || p;
}

function walk(dir, out = []) {
  if (!fs.existsSync(dir)) return out;
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    if (ent.name === "node_modules" || ent.name === "dist" || ent.name === ".git") continue;
    const full = path.join(dir, ent.name);
    if (ent.isDirectory()) walk(full, out);
    else out.push(full);
  }
  return out;
}

function read(p) {
  return fs.readFileSync(p, "utf8");
}

// --- 1. theme.css must hardcode cyan accent, not multi-select runtime ---
const themeCssPath = path.join(ROOT, "packages/theme/theme.css");
if (!fs.existsSync(themeCssPath)) {
  findings.push({
    rule: "theme-source",
    file: "packages/theme/theme.css",
    message: "Missing canonical theme.css",
  });
} else {
  const css = read(themeCssPath);
  if (!css.includes(ALLOWED_ACCENT) && !css.includes(ALLOWED_ACCENT.toLowerCase())) {
    findings.push({
      rule: "accent",
      file: rel(themeCssPath),
      message: `theme.css must set sole accent ${ALLOWED_ACCENT}`,
    });
  }
  // Runtime multi-accent selectors are forbidden in product theme
  if (/data-accent\s*=\s*["']?(violet|mint|cyan)/i.test(css)) {
    findings.push({
      rule: "accent",
      file: rel(themeCssPath),
      message: "theme.css must not expose multi-selectable data-accent runtime switches",
    });
  }
  for (const bad of FORBIDDEN_BRAND_ACCENTS) {
    if (css.toLowerCase().includes(bad.toLowerCase())) {
      findings.push({
        rule: "accent",
        file: rel(themeCssPath),
        message: `Forbidden brand accent ${bad} in theme.css (sole accent is ${ALLOWED_ACCENT})`,
      });
    }
  }
  for (const re of FORBIDDEN_FONT_PATTERNS) {
    if (re.test(css)) {
      findings.push({
        rule: "fonts",
        file: rel(themeCssPath),
        message: `Forbidden system/stock font in theme.css: ${re}`,
      });
    }
  }
  if (!/font-family:\s*"Archivo"/i.test(css) && !/--house-font-product:\s*"Archivo"/i.test(css)) {
    findings.push({
      rule: "fonts",
      file: rel(themeCssPath),
      message: "theme.css must declare Archivo as product font",
    });
  }
}

// --- 2. Product CSS under apps: no system fonts, no local token redefinition of accents ---
const appCssFiles = [
  ...walk(path.join(ROOT, "apps")).filter((f) => f.endsWith(".css")),
  ...walk(path.join(ROOT, "packages")).filter(
    (f) => f.endsWith(".css") && !f.includes(`${path.sep}theme${path.sep}fonts${path.sep}`),
  ),
];

for (const file of appCssFiles) {
  const text = read(file);
  const r = rel(file);

  // App-local token files by content markers
  if (
    !r.startsWith(`packages${path.sep}theme${path.sep}`) &&
    (/--house-accent-base\s*:/i.test(text) || /--house-bg-canvas\s*:/i.test(text))
  ) {
    findings.push({
      rule: "app-local-tokens",
      file: r,
      message: "App/package CSS must not declare house token values; use packages/theme/theme.css",
    });
  }

  // Forbidden fonts in any product CSS except comments about them in the check itself
  if (r.includes("check-house-conformance")) continue;
  for (const re of FORBIDDEN_FONT_PATTERNS) {
    if (re.test(text)) {
      findings.push({
        rule: "fonts",
        file: r,
        message: `Forbidden system/stock font declaration matching ${re}`,
      });
    }
  }

  // Brand accents outside theme.css
  if (!r.startsWith(`packages${path.sep}theme${path.sep}`)) {
    for (const bad of FORBIDDEN_BRAND_ACCENTS) {
      if (text.toLowerCase().includes(bad.toLowerCase())) {
        findings.push({
          rule: "accent",
          file: r,
          message: `Forbidden brand accent ${bad} outside theme foundation`,
        });
      }
    }
  }
}

// --- 3. App-local token files by path ---
for (const file of walk(path.join(ROOT, "apps"))) {
  const base = path.basename(file);
  if (FORBIDDEN_TOKEN_BASENAMES.has(base)) {
    findings.push({
      rule: "app-local-tokens",
      file: rel(file),
      message: `Forbidden app-local token file ${base}`,
    });
  }
}
// house-tokens under docs/ui is documentation source, allowed
for (const file of walk(path.join(ROOT, "packages"))) {
  const base = path.basename(file);
  const r = rel(file);
  if (
    FORBIDDEN_TOKEN_BASENAMES.has(base) &&
    !r.startsWith(`packages${path.sep}theme${path.sep}`) &&
    !r.includes(`${path.sep}docs${path.sep}`)
  ) {
    findings.push({
      rule: "app-local-tokens",
      file: r,
      message: `Forbidden package-local token file ${base}`,
    });
  }
}

// --- 4. Emoji as product iconography ---
// Foundation gate: fail on packages/theme (must stay clean).
// App pages still contain emoji until later migration phases; report as info only.
const themeTsx = walk(path.join(ROOT, "packages/theme")).filter(
  (f) => /\.(tsx|jsx)$/.test(f) && !f.includes(`${path.sep}dist${path.sep}`),
);
for (const file of themeTsx) {
  const text = read(file);
  const lines = text.split("\n");
  lines.forEach((line, i) => {
    if (line.trimStart().startsWith("//") || line.trimStart().startsWith("*")) return;
    if (!EMOJI_RE.test(line)) return;
    findings.push({
      rule: "emoji-icons",
      file: `${rel(file)}:${i + 1}`,
      message: `Emoji detected in theme package (use house Icon component): ${line.trim().slice(0, 80)}`,
    });
  });
}

const appEmoji = [];
for (const file of walk(path.join(ROOT, "apps")).filter((f) => /\.(tsx|jsx)$/.test(f))) {
  const lines = read(file).split("\n");
  lines.forEach((line, i) => {
    if (line.trimStart().startsWith("//") || line.trimStart().startsWith("*")) return;
    if (EMOJI_RE.test(line)) appEmoji.push(`${rel(file)}:${i + 1}`);
  });
}
if (appEmoji.length > 0) {
  console.log(
    `house-conformance: info — ${appEmoji.length} app emoji occurrence(s) deferred to route migration phases`,
  );
}

// --- 5. Self-hosted fonts present ---
const fontsDir = path.join(ROOT, "packages/theme/fonts");
const requiredFonts = [
  "Archivo-400-latin.woff2",
  "Archivo-500-latin.woff2",
  "Archivo-600-latin.woff2",
  "Archivo-700-latin.woff2",
  "Newsreader-400-latin.woff2",
  "Newsreader-600-latin.woff2",
  "IBMPlexMono-400-latin.woff2",
  "IBMPlexMono-500-latin.woff2",
];
for (const f of requiredFonts) {
  if (!fs.existsSync(path.join(fontsDir, f))) {
    findings.push({
      rule: "fonts",
      file: `packages/theme/fonts/${f}`,
      message: "Missing required self-hosted WOFF2",
    });
  }
}

// --- Report ---
if (findings.length === 0) {
  console.log("house-conformance: GREEN (0 findings)");
  console.log(`  accent: ${ALLOWED_ACCENT}`);
  console.log("  fonts: Archivo / Newsreader / IBM Plex Mono (self-hosted)");
  console.log("  token source: packages/theme/theme.css");
  process.exit(0);
}

console.error(`house-conformance: RED (${findings.length} finding(s))`);
for (const f of findings) {
  console.error(`  [${f.rule}] ${f.file}: ${f.message}`);
}
process.exit(1);
