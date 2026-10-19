#!/usr/bin/env node
/**
 * ds-search-cli 启动垫片
 * 定位本平台二进制(由 optionalDependencies 平台子包提供)并透传执行。
 * 模式同 esbuild / rollup / turbo:npm 安装时自动选装匹配平台的子包,
 * 本文件只做定位 + spawn,不参与编译。
 */
'use strict';

const path = require('path');
const { spawnSync } = require('child_process');

// GOOS/npm platform 与 GOARCH/npm arch 都是标准命名,直接拼接。
const SUBPKG_PREFIX = 'ds-search-cli-';
const platformDir = `${process.platform}-${process.arch}`;

// 平台子包双轨 scoped 优先 + unscoped 兜底。
// 背景:unscoped "ds-search-cli-win32-x64" 被 npmjs spam 检测稳定 E403,
// 已迁至个人 scope;早期已发行的其余 5 个 unscoped 子包保持可解析。
const SCOPE = '@zhangshichun';
const subPkgDir = `${SUBPKG_PREFIX}${platformDir}`;

const EXE_NAME = process.platform === 'win32' ? 'ds-search.exe' : 'ds-search';

function fail(msg, code = 1) {
  process.stderr.write(`ds-search: error: ${msg}\n`);
  process.exit(code);
}

function findBinary() {
  // 平台子包固定与主包同版本,由 npm 的 semver 等号(主包 optionalDependencies
  // 写死版本)保证。失败时给可操作提示而不是栈。
  let pkgJsonPath = null;
  for (const name of [`${SCOPE}/${subPkgDir}`, subPkgDir]) {
    try {
      pkgJsonPath = require.resolve(`${name}/package.json`);
      break;
    } catch (e) { /* 候选下一个 */ }
  }
  if (!pkgJsonPath) {
    fail(
      `unsupported or missing platform package "${SCOPE}/${subPkgDir}" (or "${subPkgDir}").\n` +
        `  npm 安装平台不匹配时(如 freebsd)需手动安装:\n` +
        `  1. 从 GitHub Releases 下载对应平台二进制;2. 放入 PATH。`,
    );
  }
  // 平台子包布局: <subpkg>/package.json + <subpkg>/bin/ds-search(.exe)
  return path.join(path.dirname(pkgJsonPath), 'bin', EXE_NAME);
}

function main() {
  const bin = findBinary();

  // 透传所有参数与 stdio,退出码原样透传。
  const result = spawnSync(bin, process.argv.slice(2), {
    stdio: ['inherit', 'inherit', 'inherit'],
    env: process.env,
    windowsHide: true,
  });

  if (result.error) {
    // ENOENT/EACCES: 子包文件可能被杀毒软件/CI 清理,或子包没随主包安装。
    fail(
      `cannot run ${bin}: ${result.error.message}\n` +
        `  try: npm i -g ds-search-cli --force  或安装指定平台子包: npm i -g --force --os ${process.platform} --cpu ${process.arch} ds-search-cli`,
    );
  }
  if (result.signal) {
    process.kill(process.pid, result.signal);
    return;
  }
  process.exit(result.status === null ? 1 : result.status);
}

main();