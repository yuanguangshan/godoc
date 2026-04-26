#!/usr/bin/env node

const { spawn } = require('child_process');
const path = require('path');
const os = require('os');
const fs = require('fs');

const platform = os.platform();
const arch = os.arch();

const archMap = { x64: 'amd64', arm64: 'arm64' };
const mappedArch = archMap[arch];

if (!mappedArch) {
  console.error(`Unsupported architecture: ${arch}`);
  process.exit(1);
}

const binaryName = platform === 'win32'
  ? `godoc-win-${mappedArch}.exe`
  : `godoc-${platform}-${mappedArch}`;
const binaryPath = path.join(__dirname, binaryName);

if (!fs.existsSync(binaryPath)) {
  console.error(`Error: Binary not found at ${binaryPath}`);
  console.error(`Platform: ${platform}, Arch: ${arch}`);
  console.error(`Try running: npm rebuild aidocs`);
  process.exit(1);
}

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: 'inherit'
});

child.on('exit', (code) => {
  process.exit(code);
});
