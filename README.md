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
- Automatic Source Map discovery
- Custom User-Agent and proxy support
- Multiple output formats: text, JSON, markdown

## Installation

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
| `--debug` | Enable debug output |
| `-f <format>` | Output format: `text` (default), `json`, `md` |
| `--cache` | Enable caching (enabled by default) |
| `--cache=false` | Disable caching |
| `--useragent=<UA>` | Custom User-Agent string |
| `--proxy=<URL>` | HTTP proxy URL |
| `-h` | Show help information |

### Examples

```bash
# Custom User-Agent
dj --useragent="Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) ..." https://example.com

# Use proxy
dj --proxy="http://127.0.0.1:7890" https://example.com

# Enable debug mode
dj --debug https://example.com
```

### Tested websites

| Website | JS Count |
|---------|----------|
| [docs.qq.com](https://docs.qq.com) | ~3270 |
| [vue.ruoyi.vip](https://vue.ruoyi.vip) | ~68 |
| [gitee.com](https://gitee.com) | ~63 |
| [nuxt.com.cn](https://nuxt.com.cn) | ~160 |
| [chat.z.ai](https://chat.z.ai) | ~539 |
| [show.cool-admin.com/login](https://show.cool-admin.com/login) | ~120 |
| [demo.1panel.cn](https://demo.1panel.cn) | ~161 |
| [mail.qq.com](https://mail.qq.com) | ~418 |
| [chat.deepseek.com](https://chat.deepseek.com) | ~634 |

```bash
dj https://docs.qq.com
dj https://vue.ruoyi.vip
dj https://gitee.com
dj https://nuxt.com.cn
dj https://chat.z.ai
dj https://show.cool-admin.com/login
dj https://demo.1panel.cn
```

## Supported patterns and frameworks (16 plugins)

| Framework/Tool | Features |
|----------------|----------|
| **HTMLScript** | Parse `<script src>` tags to extract directly referenced JS |
| **DynamicImport** | `import()` dynamic loading, `import(/* webpackChunkName */)` comments |
| **Webpack** | `__webpack_require__.e()` dynamic loading, chunk map detection, webpackChunk global, string chunk ID mapping |
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

## How it works

1. Download the target webpage HTML
2. Launch plugin analysis - each URL is processed concurrently by a goroutine:
   - Download JS content
   - Detect Content-Type (skip static resources returning HTML)
   - Dispatch to all plugins for pattern matching
3. Plugins discover new JS URLs or path fragments, add to processing queue
4. Probe for Source Map files (via `sourceMappingURL` or HTTP header)
5. Collect all discovered JS URLs and output

## Output formats

### Text (default)

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
    "sourceMapCount": 1
  },
  "jsURLs": [
    "https://example.com/js/main.js",
    "https://example.com/js/chunk-abc123.js"
  ],
  "cacheBase": "/tmp/ejfkdev/dj/example.com",
  "cacheDirs": {
    "js": "/tmp/ejfkdev/dj/example.com/js",
    "sourceMap": "/tmp/ejfkdev/dj/example.com/source_map",
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
├── html/                  # Original HTML
└── meta.json             # Site metadata
```

## FAQ

**Why aren't some dynamically loaded JS files being extracted?**

This tool uses static analysis of JS code to detect dynamic loading patterns. If a website uses special loading methods, they may not be covered. If you find a website whose dynamic JS cannot be extracted, feel free to submit an [Issue](https://github.com/ejfkdev/dj/issues) with the site URL and any relevant code clues.

## License

[MPL 2.0 (Mozilla Public License)](LICENSE)
