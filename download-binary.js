#!/usr/bin/env node

const https = require('https');
const http = require('http');
const fs = require('fs');
const path = require('path');
const os = require('os');
const zlib = require('zlib');

const REPO = 'yuanguangshan/sourcepack';
const VERSION = require('./package.json').version;

const platform = os.platform();
const arch = os.arch();

const archMap = { x64: 'amd64', arm64: 'arm64' };
const mappedArch = archMap[arch];

if (!mappedArch) {
  console.error(`Unsupported architecture: ${arch}`);
  process.exit(1);
}

const ext = platform === 'win32' ? '.zip' : '.tar.gz';
const archiveName = `sourcepack_${VERSION}_${platform}_${mappedArch}${ext}`;
const downloadUrl = `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;

const binName = platform === 'win32' ? 'sourcepack.exe' : `sourcepack-${platform}-${mappedArch}`;
const binDir = __dirname;

function download(url) {
  return new Promise((resolve, reject) => {
    const mod = url.startsWith('https') ? https : http;
    mod.get(url, { headers: { 'User-Agent': 'node' } }, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        return resolve(download(res.headers.location));
      }
      if (res.statusCode !== 200) {
        return reject(new Error(`Download failed: HTTP ${res.statusCode}`));
      }
      resolve(res);
    }).on('error', reject);
  });
}

async function main() {
  console.log(`Downloading sourcepack v${VERSION} for ${platform}-${mappedArch}...`);

  const res = await download(downloadUrl);
  const chunks = [];
  for await (const chunk of res) chunks.push(chunk);
  const buf = Buffer.concat(chunks);

  if (platform === 'win32') {
    // For zip, use Node's built-in zlib (no external deps)
    // Unfortunately Node doesn't ship with a zip extractor,
    // so we use a simple approach: extract via tar on CI, or
    // fall back to PowerShell on Windows
    const { execSync } = require('child_process');
    const tmpFile = path.join(binDir, archiveName);
    fs.writeFileSync(tmpFile, buf);
    execSync(`powershell -Command "Expand-Archive -Path '${tmpFile}' -DestinationPath '${binDir}' -Force"`, { stdio: 'inherit' });
    // Move binary out of subdirectory if wrapped
    const wrapped = path.join(binDir, `sourcepack_${VERSION}_${platform}_${mappedArch}`, 'sourcepack.exe');
    const target = path.join(binDir, binName);
    if (fs.existsSync(wrapped)) fs.renameSync(wrapped, target);
    fs.unlinkSync(tmpFile);
    try { fs.rmdirSync(path.join(binDir, `sourcepack_${VERSION}_${platform}_${mappedArch}`)); } catch {}
  } else {
    // tar.gz — use pipe
    const { execSync } = require('child_process');
    const tmpFile = path.join(binDir, archiveName);
    fs.writeFileSync(tmpFile, buf);
    execSync(`tar xzf "${tmpFile}" -C "${binDir}" --strip-components=1`, { stdio: 'inherit' });
    // Rename sourcepack to sourcepack-{platform}-{arch}
    const extracted = path.join(binDir, 'sourcepack');
    const target = path.join(binDir, binName);
    if (fs.existsSync(extracted)) fs.renameSync(extracted, target);
    fs.unlinkSync(tmpFile);
  }

  // Make binary executable
  const target = path.join(binDir, binName);
  if (fs.existsSync(target)) {
    fs.chmodSync(target, 0o755);
    console.log(`✓ sourcepack v${VERSION} installed successfully.`);
  } else {
    console.error(`Error: expected binary not found at ${target}`);
    process.exit(1);
  }
}

main().catch(err => {
  console.error(`Failed to download sourcepack: ${err.message}`);
  console.error(`You can manually download from: https://github.com/${REPO}/releases/tag/v${VERSION}`);
  process.exit(1);
});
