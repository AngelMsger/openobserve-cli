'use strict';

const path = require('path');
const test = require('node:test');
const assert = require('node:assert/strict');

const { assetName, binPath, welcomeText } = require('./install.js');

test('selects Windows release assets for both supported architectures', () => {
  assert.equal(assetName('win32', 'x64'), 'openobserve-cli-windows-amd64.exe');
  assert.equal(assetName('win32', 'arm64'), 'openobserve-cli-windows-arm64.exe');
});

test('uses an exe launcher target on Windows', () => {
  assert.equal(path.basename(binPath('win32').file), 'openobserve-cli.exe');
});

test('rejects unsupported Windows architectures', () => {
  assert.throws(() => assetName('win32', 'ia32'), /unsupported platform win32\/ia32/);
});

test('welcome text recommends valid OpenObserve commands', () => {
  assert.match(welcomeText(), /openobserve-cli stream list/);
  assert.match(welcomeText(), /openobserve-cli search run/);
});
