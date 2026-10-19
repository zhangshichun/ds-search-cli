#!/usr/bin/env node
/**
 * pack-npm.mjs — 从 dist/bin 组装 npm 发布产物
 *
 * 输入:  dist/bin/<platform>-<arch>/ds-search(.exe)(build-release.ps1 产物)
 * 输出:  dist/npm/
 *          ds-search-cli/                 主包(package.json + bin.js + README)
 *          ds-search-cli-<platform>-<arch>/   6 个平台子包(package.json + bin/二进制)
 *
 * 发布: 发布后主包 optionalDependencies 指同版本平台子包;
 * npm install 时按 os/cpu 自动只装匹配子包,bin.js 垫片定位二进制。
 *
 * 用法: node scripts/pack-npm.mjs [--out dist/npm]
 */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');

const args = process.argv.slice(2);
const outIdx = args.indexOf('--out');
const outDir = path.resolve(root, outIdx >= 0 ? args[outIdx + 1] : 'dist/npm');

const version = fs.readFileSync(path.join(root, 'VERSION'), 'utf8').trim();
if (!version) throw new Error('VERSION 为空');

const MAIN_PKG = 'ds-search-cli';
const DESCRIPTION = 'DeepSeek Search/Fetch CLI - web search & page fetch for AI agents (ds-search)';
const LICENSE = 'MIT';
const HOMEPAGE = 'https://github.com/zhangshichun/ds-search-cli';

// win32-x64 子包走个人 scope:unscoped "ds-search-cli-win32-x64" 被 npmjs spam
// 检测稳定 E403(其余平台名都能过);scoped 名绑定账号归属,不经那道评分。
// esbuild/@swc 平台子包正是此布局。bin.js 垫片按 scoped 优先、unscoped 兜底。
const SCOPE = 'zhangshichun';

// 写入主包与平台子包 manifest.repository 的 git 地址(spam 评分的重要真伪信号)。
const REPO = { type: 'git', url: HOMEPAGE.replace('https://', 'git+https://') + '.git' };

/** 平台包安装后的真实 npm 包名(scoped 带 @scope/ 前缀);manifest.name 与冒烟布局共用。 */
const packageName = (pl) => (pl.scoped ? `@${SCOPE}/` : '') + `${MAIN_PKG}-${pl.npm}`;

// 产物命名统一为 npm 生态标准(platform-arch,如 win32-x64),与 bin.js 的
// process.platform/-arch 拼接规则一致;GOOS/goarch 仅在构建脚本里使用。
const PLATFORMS = [
  { go: 'windows-amd64', npm: 'win32-x64', os: 'win32', cpu: 'x64', scoped: true },
  { go: 'windows-arm64', npm: 'win32-arm64', os: 'win32', cpu: 'arm64' },
  { go: 'linux-amd64', npm: 'linux-x64', os: 'linux', cpu: 'x64' },
  { go: 'linux-arm64', npm: 'linux-arm64', os: 'linux', cpu: 'arm64' },
  { go: 'darwin-amd64', npm: 'darwin-x64', os: 'darwin', cpu: 'x64' },
  { go: 'darwin-arm64', npm: 'darwin-arm64', os: 'darwin', cpu: 'arm64' },
];

const EXE = (goPlatform) => (goPlatform.startsWith('windows') ? 'ds-search.exe' : 'ds-search');

/** 生成主包 manifest。 */
function mainPackageJson(dependencies) {
  return {
    name: MAIN_PKG,
    version,
    description: DESCRIPTION,
    license: LICENSE,
    homepage: HOMEPAGE,
    // bin 由 npx/npm 全局安装后直接可执行。
    bin: { 'ds-search': 'bin.js' },
    files: ['bin.js', 'README.md', 'LICENSE'],
    optionalDependencies: dependencies,
    keywords: ['deepseek', 'search', 'web-search', 'fetch', 'cli', 'ai', 'agent', 'rag', 'api'],
    repository: REPO,
    engines: { node: '>=16' },
    preferGlobal: true,
  };
}

function copy(src, dest) {
  fs.mkdirSync(path.dirname(dest), { recursive: true });
  fs.copyFileSync(src, dest);
}

/** 平台子包 README:npmjs spam 反垃圾对"无文档纯二进制包"评分极低,必须有它。 */
function subReadme(subName, pl) {
  return `# ${subName}

Prebuilt \`${EXE(pl.go)}\` binary ([DeepSeek web search/fetch CLI](https://api-docs.deepseek.com/zh-cn/guides/search/)) for **${pl.go}**.

**This package is installed automatically** as an optional dependency of [\`${MAIN_PKG}\`](${HOMEPAGE}) — global install gives you the \`ds-search\` command:

\`\`\`bash
npm i -g ${MAIN_PKG}
ds-search search "DeepSeek V4 release" --jsonl --fields title,url,summary
ds-search fetch "https://example.com/article" --raw
\`\`\`

You normally don't need to install this package directly. ${LICENSE} license. Source & docs: ${HOMEPAGE}
`;
}

// 清理现有同名目录:遇到终端/资源管理器占用目录句柄(EBUSY)不致命 —
// 跳过删除,后续以覆写方式重建(删目录才需要句柄空闲,写文件不受影响)。
if (fs.existsSync(outDir)) {
  for (const entry of fs.readdirSync(outDir)) {
    try {
      fs.rmSync(path.join(outDir, entry), { recursive: true, force: true });
    } catch (e) {
      console.warn(`  ! 跳过删除 ${entry}(${e.code ?? e.message})— 目录可能被某窗口占用,内容将直接覆写`);
    }
  }
}

const optionalDeps = {};
const licenseSrc = path.join(root, 'LICENSE');
for (const pl of PLATFORMS) {
  const binSrc = path.join(root, 'dist', 'bin', pl.go, EXE(pl.go));
  if (!fs.existsSync(binSrc)) {
    console.error(`✗ 缺少二进制 ${binSrc} — 先跑 scripts/build-release.ps1`);
    process.exit(1);
  }

  // dist 本地目录名保持无 @ 前缀(publish 脚本按目录扫描,路径也更直观);
  // manifest.name 才是 npm 包名 — scoped 平台带 @scope/。
  const subDirName = `${MAIN_PKG}-${pl.npm}`;
  const subName = packageName(pl);
  const subDir = path.join(outDir, subDirName);
  fs.mkdirSync(path.join(subDir, 'bin'), { recursive: true });
  // os/cpu 声明让 npm 安装时只装匹配平台的子包;其余元数据帮助 npmjs 的 spam
  // 检测把它识别为正常包(纯二进制+284B manifest 的包会被反垃圾拦截)。
  const manifest = {
    name: subName,
    version,
    description: `${DESCRIPTION} — ${pl.go} binary`,
    license: LICENSE,
    homepage: HOMEPAGE,
    // repository 是 npmjs spam 评分的重要真伪信号(无出处的新名字更容易被拦)。
    repository: REPO,
    os: [pl.os],
    cpu: [pl.cpu],
    files: ['bin', 'README.md', 'LICENSE'],
    'funding': false,
  };
  fs.writeFileSync(path.join(subDir, 'package.json'), JSON.stringify(manifest, null, 2) + '\n');
  fs.writeFileSync(path.join(subDir, 'README.md'), subReadme(subName, pl));
  copy(binSrc, path.join(subDir, 'bin', EXE(pl.go)));
  if (fs.existsSync(licenseSrc)) copy(licenseSrc, path.join(subDir, 'LICENSE'));
  optionalDeps[subName] = version;
  console.log(`  + ${subName} (${EXE(pl.go)})`);
}

// 主包
const mainDir = path.join(outDir, MAIN_PKG);
const readmeSrc = path.join(root, 'README.md');
console.log(`  + ${MAIN_PKG}`);
fs.mkdirSync(mainDir, { recursive: true });
fs.writeFileSync(path.join(mainDir, 'package.json'), JSON.stringify(mainPackageJson(optionalDeps), null, 2) + '\n');
copy(path.join(root, 'npm', 'bin.js'), path.join(mainDir, 'bin.js'));
if (fs.existsSync(readmeSrc)) copy(readmeSrc, path.join(mainDir, 'README.md'));

console.log(`\n✓ npm 产物就绪: ${outDir}`);
console.log('  发布: pwsh scripts/publish-npm.ps1  (自动扫描 dist/npm、先子包后主包,含幂等跳过与 2FA/信任窗口交互)');

// 冒烟布局:按 npm 安装后的 node_modules 结构复制一份,验证 bin.js 垫片能定位
// 二进制(发布链路自检)。只复制主包 + 当前平台子包 — 真实安装也只拉一个平台包,
// 垫片解析只查本机平台那条 optionalDeps 路径,复制其余平台毫无验证价值。
const smokeDir = path.resolve(outDir, '..', 'npm-install-test', 'node_modules');
fs.rmSync(smokeDir, { recursive: true, force: true });
fs.mkdirSync(smokeDir, { recursive: true });
fs.cpSync(path.join(outDir, MAIN_PKG), path.join(smokeDir, MAIN_PKG), { recursive: true });
const localPl = PLATFORMS.find((pl) => pl.os === process.platform && pl.cpu === process.arch);
if (localPl) {
  // scoped 包(本机 win32-x64 即是)按真实安装形态落 node_modules/@<scope>/ 下。
  fs.cpSync(path.join(outDir, `${MAIN_PKG}-${localPl.npm}`), path.join(smokeDir, packageName(localPl)), { recursive: true });
} else {
  console.warn(`  ! PLATFORMS 无本机平台(${process.platform}-${process.arch}),冒烟布局略过平台包`);
}
console.log(`✓ 冒烟布局: ${smokeDir}`);
console.log(`  自检: node ${path.join(smokeDir, MAIN_PKG, 'bin.js')} --help`);