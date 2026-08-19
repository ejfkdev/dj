# dj - Dynamic JS File Extractor

[中文](./README.zh.md) | English

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8.svg?style=flat-square)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MPL%202.0-blue.svg?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/ejfkdev/dj?style=flat-square)](https://github.com/ejfkdev/dj/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/ejfkdev/dj/build.yml?style=flat-square)](https://github.com/ejfkdev/dj/actions)
[![Stars](https://img.shields.io/github/stars/ejfkdev/dj?style=flat-square)](https://github.com/ejfkdev/dj/stargazers)
[![Forks](https://img.shields.io/github/forks/ejfkdev/dj?style=flat-square)](https://github.com/ejfkdev/dj/network/members)
[![Issues](https://img.shields.io/github/issues/ejfkdev/dj?style=flat-square)](https://github.com/ejfkdev/dj/issues)
[![Downloads](https://img.shields.io/github/downloads/ejfkdev/dj/total?style=flat-square)](https://github.com/ejfkdev/dj/releases)

`dj` intelligently detects dynamically loaded JavaScript files by statically analyzing website HTML and JS code, including webpack chunks, import() lazy loading, and more.

## Features

- Deep analysis of website HTML and JS to extract dynamically loaded JavaScript files
- Smart detection of dynamic loading patterns: import(), require(), webpack chunks, vite preload, etc.
- Support for multiple frontend framework chunk mappings: Next.js, Nuxt.js, Vite, SvelteKit, Webpack, and more
- Automatic Source Map discovery and **original source code restoration** (from `sourcesContent`, with `mappings` VLQ fallback)
- **Cache reuse**: second run on the same site restores results from local cache with zero network requests
- TLS fingerprint spoofing with **randomized browser fingerprints** (Chrome, Firefox, Safari, Edge, iOS) to bypass Cloudflare and other WAFs
- HTTP/2 and HTTP/1.1 protocol auto-negotiation
- SOCKS5/HTTP/HTTPS proxy support with authentication
- Environment variable proxy configuration (`HTTPS_PROXY`, `ALL_PROXY`, `NO_PROXY`, etc.)
- Custom User-Agent and browser-like request headers
- Multiple output formats: text, JSON, markdown

## Supported Frameworks & Loading Models

dj's extractor is verified against real build output across **73 frameworks / loading models** and **562 versions**.

| Category | Frameworks |
|----------|------------|
| Bundlers (14) | `webpack`, `webpack4`, `vite`, `rollup`, `esbuild`, `parcel`, `rsbuild`, `rspack`, `farm`, `mako`, `rolldown`, `systemjs`, `snowpack`, `turbopack` |
| Microfrontend frameworks (17) | `qiankun`, `single-spa`, `micro-app`, `wujie`, `garfish`, `icestark`, `piral`, `luigi`, `emp`, `hel-micro`, `native-federation`, `module-federation`, `mf-runtime`, `mf-vite`, `mf-rsbuild`, `webpack-mf`, `vite-plugin-federation` |
| Meta frameworks / SSR (25) | `nuxt`, `sveltekit`, `angular`, `astro`, `qwik`, `solidstart`, `tanstack-start`, `marko`, `react-router`, `remix2`, `redwood`, `analog`, `vike`, `hono`, `one`, `fresh`, `stencil`, `storybook`, `gatsby`, `cra`, `umi`, `icejs`, `vue-cli`, `modernjs`, `bun` |
| Rust / WASM frontends (3) | `leptos`, `lustre`, `trunk` |
| Admin templates (12) | `ant-design-pro`, `amis`, `ng-alain`, `jeecg`, `d2admin`, `pig-ui`, `tdesign-starter`, `vue-element-admin`, `vue-pure-admin`, `vue-vben-admin`, `ruoyi-vue2`, `ruoyi-vue3` |
| Spec / polyfills (1) | `es-module-shims` |

## Version details (click to expand)

<details><summary><b>Bundlers</b> (14 — expand for verified versions)</summary>

| Framework | Notes | Verified versions |
|-----------|-------|-------------------|
| **webpack** | webpack 5 runtime chunk maps | v5.0.0 ~ v5.109.2（18 个版本） |
| **webpack4** | webpack 4 JSONP runtime | v4.33.0 ~ v4.47.0（15 个版本） |
| **vite** | dynamic import / modulepreload / prefetch | v3.0.9 ~ v8.2.1（13 个版本） |
| **rollup** | multi-entry + dynamic import | v2.0.6 ~ v4.62.4（18 个版本） |
| **esbuild** | esbuild output | v0.14.54 ~ v0.28.2（15 个版本） |
| **parcel** | parcel output | v2.0.1 ~ v2.16.4（4 个版本） |
| **rsbuild** | rsbuild / rspack ecosystem | v1.0.19 ~ v1.7.6（7 个版本） |
| **rspack** | runtime hash maps | v1.0.14 ~ v2.1.10（10 个版本） |
| **farm** | Rust bundler | v0.1.1 ~ v1.0.5（5 个版本） |
| **mako** | ByteDance mako | v0.4.17 ~ v0.11.15（8 个版本） |
| **rolldown** | rolldown | v0.9.2 ~ v1.2.4（10 个版本） |
| **systemjs** | SystemJS output | v0.21.6 ~ v6.15.1（17 个版本） |
| **snowpack** | unbundled ESM output | v2.18.5 ~ v3.8.8（10 个版本） |
| **turbopack** | Next.js turbopack | v15.3.9 ~ v16.3.1（7 个版本） |

</details>

<details><summary><b>Microfrontend frameworks</b> (17 — expand for verified versions)</summary>

| Framework | Notes | Verified versions |
|-----------|-------|-------------------|
| **qiankun** | qiankun 2.x (incl. vite-plugin-qiankun) | v2.1.1 ~ v2.10.16（10 个版本） |
| **single-spa** | root config | v4.4.4 ~ v6.0.3（12 个版本） |
| **micro-app** | micro-app | v0.1.0 ~ v0.8.11（8 个版本） |
| **wujie** | wujie | v1.0.29 ~ v2.1.0（3 个版本） |
| **garfish** | garfish | v1.0.26 ~ v1.19.12（10 个版本） |
| **icestark** | icestark | v1.0.0 ~ v2.8.4（11 个版本） |
| **piral** | piral | v1.1.0 ~ v1.12.2（12 个版本） |
| **luigi** | luigi | v0.0.9 ~ v1.7.11（10 个版本） |
| **emp** | @efox/emp | v1.0.34 ~ v1.10.2（11 个版本） |
| **hel-micro** | hel-micro | v3.5.16 ~ v4.15.3（10 个版本） |
| **native-federation** | native federation | v0.9.1 ~ v4.4.1（11 个版本） |
| **module-federation** | @module-federation/enhanced | v0.0.17 ~ v2.8.2（11 个版本） |
| **mf-runtime** | MF runtime only | v2.1.0 ~ v2.8.2（8 个版本） |
| **mf-vite** | MF vite integration | v1.13.7 ~ v1.20.7（8 个版本） |
| **mf-rsbuild** | MF rsbuild plugin | v2.1.0 ~ v2.8.2（8 个版本） |
| **webpack-mf** | webpack built-in MF | v5.98.0 ~ v5.108.4（10 个版本） |
| **vite-plugin-federation** | Origin.js federation | v0.0.3 ~ v1.4.1（6 个版本） |

</details>

<details><summary><b>Meta frameworks / SSR</b> (25 — expand for verified versions)</summary>

| Framework | Notes | Verified versions |
|-----------|-------|-------------------|
| **nuxt** | Nuxt | v3.0.0 ~ v4.5.2（8 个版本） |
| **sveltekit** | SvelteKit | v2.0.8 ~ v2.70.2（10 个版本） |
| **angular** | Angular | v17.0.10 ~ v20.3.34（10 个版本） |
| **astro** | Astro | v3.6.5 ~ v7.2.2（4 个版本） |
| **qwik** | Qwik | v1.9.1 ~ v1.20.0（12 个版本） |
| **solidstart** | SolidStart | v1.1.7 ~ v2.0.0（4 个版本） |
| **tanstack-start** | TanStack Start | v1.111.15 ~ v1.168.46（8 个版本） |
| **marko** | Marko | v0.1.16 ~ v0.11.9（5 个版本） |
| **react-router** | React Router framework mode | v7.0.2 ~ v8.3.0（8 个版本） |
| **remix2** | Remix | v2.10.3 ~ v2.17.5（8 个版本） |
| **redwood** | Redwood | v6.6.4 ~ v8.9.0（3 个版本） |
| **analog** | Analog | v0.2.45 ~ v2.7.0（4 个版本） |
| **vike** | Vike | v0.4.249 ~ v0.4.260（12 个版本） |
| **hono** | Hono JSX | v4.2.9 ~ v4.13.2（12 个版本） |
| **one** | One | v1.17.11 ~ v1.24.5（8 个版本） |
| **fresh** | Deno Fresh | v1.4.3 ~ v2.3.3（7 个版本） |
| **stencil** | Stencil | v4.33.1 ~ v4.44.0（12 个版本） |
| **storybook** | Storybook static export | v9.0.18 ~ v10.5.9（8 个版本） |
| **gatsby** | Gatsby | v4.25.9 ~ v5.16.1（5 个版本） |
| **cra** | Create React App | v5.0.0 ~ v5.0.1（2 个版本） |
| **umi** | UmiJS 4 | v4.0.90 ~ v4.7.6（8 个版本） |
| **icejs** | ICE.js | v3.0.6 ~ v3.6.5（7 个版本） |
| **vue-cli** | Vue CLI | v4.0.5 ~ v5.0.9（7 个版本） |
| **modernjs** | Modern.js | v2.0.2 ~ v3.8.2（11 个版本） |
| **bun** | Bun bundler | v0.6.14 ~ v1.3.14（7 个版本） |

</details>

<details><summary><b>Rust / WASM frontends</b> (3 — expand for verified versions)</summary>

| Framework | Notes | Verified versions |
|-----------|-------|-------------------|
| **leptos** | Leptos | v0.4.10 ~ v0.8.20（5 个版本） |
| **lustre** | Gleam Lustre | v5.4.0 ~ v5.7.1（4 个版本） |
| **trunk** | Trunk (wasm-bindgen) | v0.17.5 ~ v0.21.14（5 个版本） |

</details>

<details><summary><b>Admin templates</b> (12 — expand for verified versions)</summary>

| Framework | Notes | Verified versions |
|-----------|-------|-------------------|
| **ant-design-pro** | Ant Design Pro | v4.4.0 ~ v6.0.3（5 个版本） |
| **amis** | Baidu amis | v1.9.0 ~ v6.13.0（4 个版本） |
| **ng-alain** | NG-Alain | v17.3.1 ~ v21.3.0（5 个版本） |
| **jeecg** | JeecgBoot | v3.3.0 ~ v3.4.3（2 个版本） |
| **d2admin** | D2Admin | v1.25.0 |
| **pig-ui** | Pig UI | v4.1.0 |
| **tdesign-starter** | TDesign Starter | v0.11.0 ~ v0.14.0（2 个版本） |
| **vue-element-admin** | vue-element-admin | v4.3.1 ~ v4.4.0（2 个版本） |
| **vue-pure-admin** | vue-pure-admin | v6.3.0 ~ v7.0.0（2 个版本） |
| **vue-vben-admin** | Vue Vben Admin | v5.3.2 ~ v5.4.8（2 个版本） |
| **ruoyi-vue2** | RuoYi-Vue2 | v3.8.9 ~ v3.9.2（3 个版本） |
| **ruoyi-vue3** | RuoYi-Vue3 | v3.9.1 ~ v3.9.2（2 个版本） |

</details>

<details><summary><b>Spec / polyfills</b> (1 — expand for verified versions)</summary>

| Framework | Notes | Verified versions |
|-----------|-------|-------------------|
| **es-module-shims** | importmap bare specifiers | v2.1.2 ~ v2.8.4（8 个版本） |

</details>


## Installation

### Homebrew (macOS)

```bash
brew install ejfkdev/tap/dj
```

### go install (recommended)

```bash
go install github.com/ejfkdev/dj@latest
```

### Build from source

```bash
git clone https://github.com/ejfkdev/dj.git
cd dj
go build -ldflags="-X main.version=1.0.0" -o dj .
```

### Download prebuilt binaries

Visit the [Releases](https://github.com/ejfkdev/dj/releases) page to download binaries for your platform.

## Usage

```bash
dj [options] <URL>
```

### Basic usage

```bash
# Extract JS URLs (real-time output)
dj https://example.com

# Output in JSON format
dj -f json https://example.com

# Output in Markdown format
dj -f md https://example.com
```

### Command line options

| Option | Description |
|--------|-------------|
| `-v, --version` | Print version and exit |
| `-d, --debug` | Enable debug output |
| `-f, --format <fmt>` | Output format: `md` (default), `json`, `text` (bare URL list) |
| `--no-cache` | Disable cache reads (still saves downloaded files to disk); `--cache` / `--cache=false` kept for backward compatibility |
| `--useragent=<UA>` | Custom User-Agent string |
| `--ua=<UA>` | Short alias for `--useragent` |
| `-x, --proxy <URL>` | Proxy URL (http/https/socks5), overrides environment variables |
| `--cookie=<cookies>` | Cookies for bypassing Cloudflare (e.g., `"cf_clearance=xxx"`) |
| `-H, --header <K: V>` | Custom HTTP header, repeatable (curl-style) |
| `--no-random-tls` | Disable randomized TLS fingerprint (use fixed Chrome) |
| `-o, --output <dir>` | Output directory (saves a copy of all files: js/, html/, source_map/, sources/) without site subdir |
| `-t, --timeout <secs>` | Per-request timeout in seconds (default: 30) |
| `-c, --concurrency <N>` | Max concurrent HTTP requests — downloads, probes, HEAD/RSC share this budget (default: 8) |
| `-h, --help` | Show help information |

### Examples

```bash
# Custom User-Agent
dj --useragent="Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) ..." https://example.com

# Use HTTP proxy
dj --proxy="http://127.0.0.1:7890" https://example.com

# Use SOCKS5 proxy (short form -x)
dj -x socks5://127.0.0.1:1080 https://example.com

# Use HTTPS proxy
dj --proxy="https://proxy.example.com:443" https://example.com

# Proxy with authentication
dj --proxy="socks5://user:pass@127.0.0.1:1080" https://example.com

# Use environment variable proxy (HTTPS_PROXY, HTTP_PROXY, ALL_PROXY)
HTTPS_PROXY=http://127.0.0.1:7890 dj https://example.com

# Bypass proxy for specific hosts
ALL_PROXY=socks5://127.0.0.1:1080 NO_PROXY=localhost,example.com dj https://example.com

# Inject cookies for Cloudflare bypass
dj --cookie="cf_clearance=xxx" https://example.com

# Enable debug mode
dj --debug https://example.com

# Save files to a custom output directory (without site subdir)
dj -o ./output https://example.com

# Set per-request timeout (default: 30 seconds)
dj -t 60 https://example.com

# Combine: fresh scan, save to output dir, with proxy and timeout
dj --no-cache -o ./output -x socks5://127.0.0.1:1080 -t 60 https://example.com
```

### Tested websites

<details>
<summary>📊 Tested websites (click to expand)</summary>

**Framework / Admin**

| Site | JS | Site | JS |
|-----|----|-----|----|
| [vue.ruoyi.vip](https://vue.ruoyi.vip) | 70 | [demo.1panel.cn](https://demo.1panel.cn) | 524 |
| [show.cool-admin.com/login](https://show.cool-admin.com/login) | 135 | [ant.design](https://ant.design) | 2536 |
| [arco.design](https://arco.design) | 461 | [vuejs.org](https://vuejs.org) | 17 |
| [react.dev](https://react.dev) | 34 | [svelte.dev](https://svelte.dev) | 74 |
| [angular.io](https://angular.io) | 289 | [nuxt.com.cn](https://nuxt.com.cn) | 163 |

**AI / Cloud**

| Site | JS | Site | JS |
|-----|----|-----|----|
| [chat.deepseek.com](https://chat.deepseek.com) | 41 | [chat.z.ai](https://chat.z.ai) | 167 |
| [kimi.moonshot.cn](https://kimi.moonshot.cn) | 383 | [cloud.tencent.com](https://cloud.tencent.com) | 1532 |
| [docs.qq.com](https://docs.qq.com) | 3617 | [www.aliyun.com](https://www.aliyun.com) | 59 |

**Enterprise / Collaboration**

| Site | JS | Site | JS |
|-----|----|-----|----|
| [feishu.cn](https://www.feishu.cn) | 460 | [dingtalk.com](https://www.dingtalk.com) | 22 |
| [youzan.com](https://www.youzan.com) | 332 | [kingdee.com](https://www.kingdee.com) | 34 |
| [chanjet.com](https://www.chanjet.com) | 20 | [landray.com.cn](https://www.landray.com.cn) | 11 |

**E-commerce / Portal**

| Site | JS | Site | JS |
|-----|----|-----|----|
| [gitee.com](https://gitee.com) | 100 | [baidu.com](https://www.baidu.com) | 314 |
| [meituan.com](https://www.meituan.com) | 109 | [pinduoduo.com](https://www.pinduoduo.com) | 7 |
| [bilibili.com](https://www.bilibili.com) | 44 | [juejin.cn](https://www.juejin.cn) | 102 |

**Government / University**

| Site | JS | Site | JS |
|-----|----|-----|----|
| [shanghai.gov.cn](https://www.shanghai.gov.cn) | 42 | [xinhuanet.com](https://www.xinhuanet.com) | 12 |
| [zju.edu.cn](https://www.zju.edu.cn) | 6 | [tsinghua.edu.cn](https://www.tsinghua.edu.cn) | 17 |
| [chaoxing.com](https://www.chaoxing.com) | 53 | [www.people.com.cn](https://www.people.com.cn) | 4 |

</details>

```bash
dj https://docs.qq.com
dj https://vue.ruoyi.vip
dj https://gitee.com
dj https://nuxt.com.cn
dj https://chat.z.ai
dj https://show.cool-admin.com/login
dj https://demo.1panel.cn
```

## Supported patterns and frameworks (17 plugins)

| Framework/Tool | Features |
|----------------|----------|
| **HTMLScript** | Parse `<script src>` tags to extract directly referenced JS |
| **DynamicImport** | `import()` dynamic loading, `import(/* webpackChunkName */)` comments |
| **Webpack** | `__webpack_require__.e()` dynamic loading, chunk map detection, webpackChunk global, string chunk ID mapping, webpack 4 `HASH.TIMESTAMP` fingerprint + `{"chunk-xxx":1}` existence markers |
| **Next.js** | App Router / Pages Router chunk detection, build manifest, flight chunk |
| **Nuxt.js** | `/_nuxt/` path pattern, build assets |
| **Vite** | `__vitePreload()`, modulepreload, lazy loading chunks |
| **SvelteKit** | `/_app/immutable/nodes/` and `/_app/immutable/chunks/` paths |
| **RequireJS** | `require()` / `define()` dependency loading, data-main |
| **Module Federation** | `__webpack_require__.federation` remote modules, `manifest.json` parsing |
| **ModuleFederationManifest** | Module Federation `manifest.json` shared/exposes module extraction |
| **HelMicro** | metadata.json component config, CDN prefix |
| **ESMImport** | Static `import` declaration extraction |
| **ScriptCreate** | `document.createElement('script')` dynamic loading |
| **ModernJS** | ByteDance ModernJS route manifest, b.p publicPath |
| **URLPattern** | General URL pattern matching and path probing |
| **SourceMap** | `.map` file detection (via `sourceMappingURL`, HTTP header, or inline data URI) |
| **Qiankun** | single-spa microfrontend apps: `entry`/`proEntry` HTML directory detection, then sub-app scripts, modulepreload, and inline `import()` extraction |
| **UniversalURL** | Encoding-aware fallback: decodes JS escapes / URL encoding / Unicode / HTML entities, then captures `<script src>`, `import()`, `require()`, and loose `.js` string matches. Useful for `document.write` injection, custom loaders, and other patterns other plugins miss. |

## How it works

1. Download the target webpage HTML
2. Launch plugin analysis - each URL is processed concurrently by a goroutine:
   - Download JS content
   - Detect Content-Type (skip static resources returning HTML)
   - Dispatch to all plugins for pattern matching
3. Plugins discover new JS URLs or path fragments, add to processing queue
4. Probe for Source Map files (via `sourceMappingURL` or HTTP header)
5. Restore original source code from Source Maps:
   - Priority: extract from `sourcesContent` field (complete original source)
   - Fallback: reconstruct from `mappings` (VLQ decoding) when `sourcesContent` is missing
   - Restored files preserve original directory structure under `sources/`
6. Collect all discovered JS URLs and output

### Cache reuse

When caching is enabled (default), the second run on the same site skips network requests entirely:
- Loads previously discovered JS URLs from `meta.json`
- Restores source maps and source code from local cache
- Use `--no-cache` to force a full re-scan (files are still saved to disk, just not read from cache)

## Output formats
`-f json` output includes a `jsDetails` array with per-JS provenance
(discovered-by plugin, source URL, inline flag); `meta.json` records the same
via `source_url` / `from_plugin`.


### Markdown (default)

```
https://example.com/js/main.js
https://example.com/js/chunk-abc123.js
https://example.com/js/async-def456.js
```

### JSON

```json
{
  "summary": {
    "jsCount": 3,
    "sourceMapCount": 1,
    "sourceCount": 15
  },
  "jsURLs": [
    "https://example.com/js/main.js",
    "https://example.com/js/chunk-abc123.js"
  ],
  "cacheBase": "/tmp/ejfkdev/dj/example.com",
  "cacheDirs": {
    "js": "/tmp/ejfkdev/dj/example.com/js",
    "sourceMap": "/tmp/ejfkdev/dj/example.com/source_map",
    "source": "/tmp/ejfkdev/dj/example.com/sources",
    "html": "/tmp/ejfkdev/dj/example.com/html/web.html"
  }
}
```

## Caching

Caching is enabled by default. Cache is stored in the system temp directory:

| OS | Cache directory |
|----|-----------------|
| Linux/Mac | `/tmp/ejfkdev/dj/` |
| Windows | `%TEMP%\ejfkdev\dj\` |

Cache structure:

```
<temp_dir>/ejfkdev/dj/<origin>/
├── js/                    # Downloaded JS files
├── source_map/            # Source Map files
├── sources/               # Restored original source code (preserves directory structure)
├── html/                  # Original HTML
└── meta.json             # Site metadata (JS URLs, source map paths, restored sources, cache dirs)
```

With `-o/--output`, files are also written to the specified directory **without** the `<origin>` subdirectory level:

```
<output_dir>/
├── js/
├── source_map/
├── sources/
├── html/
└── meta.json
```

The cache directory is always written normally; `-o` adds a second copy.

## FAQ

**Why aren't some dynamically loaded JS files being extracted?**

This tool uses static analysis of JS code to detect dynamic loading patterns. If a website uses special loading methods, they may not be covered. If you find a website whose dynamic JS cannot be extracted, feel free to submit an [Issue](https://github.com/ejfkdev/dj/issues) with the site URL and any relevant code clues.

## License

[MPL 2.0 (Mozilla Public License)](LICENSE)
