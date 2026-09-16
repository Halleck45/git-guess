'use strict';
// Fetches the git-guess binary matching this package's version from the
// GitHub release, verifies it against checksums.txt, and caches it per user.
const fs = require('fs');
const os = require('os');
const path = require('path');
const crypto = require('crypto');
const pkg = require('../package.json');

const REPO = 'Halleck45/git-guess';

function target() {
  const goos = { linux: 'linux', darwin: 'darwin', win32: 'windows' }[process.platform];
  const goarch = { x64: 'amd64', arm64: 'arm64' }[process.arch];
  if (!goos || !goarch) throw new Error(`unsupported platform ${process.platform}/${process.arch}`);
  return { goos, goarch, ext: goos === 'windows' ? '.exe' : '' };
}

function cacheDir() {
  if (process.env.GIT_GUESS_CACHE) return process.env.GIT_GUESS_CACHE;
  if (process.platform === 'win32') {
    return path.join(process.env.LOCALAPPDATA || path.join(os.homedir(), 'AppData', 'Local'), 'git-guess');
  }
  return path.join(process.env.XDG_CACHE_HOME || path.join(os.homedir(), '.cache'), 'git-guess');
}

function baseUrl() {
  if (process.env.GIT_GUESS_DOWNLOAD_BASE) return process.env.GIT_GUESS_DOWNLOAD_BASE.replace(/\/$/, '');
  return `https://github.com/${REPO}/releases/download/v${pkg.version}`;
}

async function get(url) {
  const r = await fetch(url, { headers: { 'user-agent': `git-guess-npm/${pkg.version}` }, redirect: 'follow' });
  if (!r.ok) throw new Error(`GET ${url}: HTTP ${r.status}`);
  return Buffer.from(await r.arrayBuffer());
}

function verify(body, checksums, name) {
  const line = checksums.split('\n').map((l) => l.trim()).find((l) => l.endsWith(' ' + name));
  if (!line) throw new Error(`${name} is not listed in checksums.txt`);
  const want = line.split(/\s+/)[0];
  const got = crypto.createHash('sha256').update(body).digest('hex');
  if (want !== got) throw new Error(`checksum mismatch for ${name}`);
}

async function ensureBinary() {
  const t = target();
  const dir = path.join(cacheDir(), pkg.version);
  const bin = path.join(dir, 'git-guess' + t.ext);
  if (fs.existsSync(bin)) return bin;
  const name = `git-guess_${t.goos}_${t.goarch}${t.ext}`;
  const base = baseUrl();
  const [body, sums] = await Promise.all([get(`${base}/${name}`), get(`${base}/checksums.txt`)]);
  verify(body, sums.toString('utf8'), name);
  fs.mkdirSync(dir, { recursive: true });
  const tmp = `${bin}.${process.pid}.tmp`;
  fs.writeFileSync(tmp, body, { mode: 0o755 });
  fs.renameSync(tmp, bin);
  return bin;
}

module.exports = { ensureBinary };
