# dj - 动态加载 JS 文件提取工具

[English](./README.md) | 中文

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8.svg?style=flat-square)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MPL%202.0-blue.svg?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/ejfkdev/dj?style=flat-square)](https://github.com/ejfkdev/dj/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/ejfkdev/dj/build.yml?style=flat-square)](https://github.com/ejfkdev/dj/actions)
[![Stars](https://img.shields.io/github/stars/ejfkdev/dj?style=flat-square)](https://github.com/ejfkdev/dj/stargazers)
[![Forks](https://img.shields.io/github/forks/ejfkdev/dj?style=flat-square)](https://github.com/ejfkdev/dj/network/members)
[![Issues](https://img.shields.io/github/issues/ejfkdev/dj?style=flat-square)](https://github.com/ejfkdev/dj/issues)
[![Downloads](https://img.shields.io/github/downloads/ejfkdev/dj/total?style=flat-square)](https://github.com/ejfkdev/dj/releases)

`dj` 通过静态分析网站 HTML 和 JS 代码，智能探测由 JS 触发的动态加载文件，包括 webpack chunk、import() 懒加载等。

## 功能特性

- 深度分析网站 HTML 和 JS，提取动态加载的 JavaScript 文件
- 智能探测动态加载模式：import()、require()、webpack chunk、vite preload 等
- 支持多种前端框架的 chunk 映射：Next.js、Nuxt.js、Vite、SvelteKit、Webpack 等
- 自动发现 Source Map 关联和位置
- 支持自定义 User-Agent 和代理
- 多种输出格式：text、json、markdown

## 安装

### go install（推荐）

```bash
go install github.com/ejfkdev/dj@latest
```

### 从源码编译

```bash
git clone https://github.com/ejfkdev/dj.git
cd dj
go build -ldflags="-X main.version=1.0.0" -o dj .
```

### 下载预编译版本

前往 [Releases](https://github.com/ejfkdev/dj/releases) 页面下载对应平台的二进制文件。

## 使用方法

```bash
dj [选项] <URL>
```

### 基本用法

```bash
# 提取 JS URL（实时输出）
dj https://example.com

# 输出 JSON 格式
dj -f json https://example.com

# 输出 Markdown 格式
dj -f md https://example.com
```

### 命令行选项

| 选项 | 说明 |
|------|------|
| `--debug` | 启用调试输出 |
| `-f <format>` | 输出格式：`text`（默认）、`json`、`md` |
| `--cache` | 启用缓存（默认启用） |
| `--cache=false` | 禁用缓存 |
| `--useragent=<UA>` | 自定义 User-Agent |
| `--proxy=<URL>` | HTTP 代理地址 |
| `-h` | 显示帮助信息 |

### 示例

```bash
# 自定义 User-Agent
dj --useragent="Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) ..." https://example.com

# 使用代理
dj --proxy="http://127.0.0.1:7890" https://example.com

# 启用调试模式
dj --debug https://example.com
```

### 测试网站

| 网站 | JS 数量 |
|------|---------|
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

## 工作原理

1. 下载目标网页 HTML
2. 启动插件分析流程，每个 URL 并行由 goroutine 处理：
   - 下载 JS 内容
   - 检测 Content-Type（跳过 HTML 响应的静态资源）
   - 分发给所有插件进行模式匹配
3. 插件发现新的 JS URL 或路径片段，添加到待处理队列
4. 探测 Source Map 文件（通过 `sourceMappingURL` 或 HTTP 头）
5. 收集所有发现的 JS URL 并输出

支持的动态加载模式和框架（共 16 个插件）：

| 框架/工具 | 支持的功能 |
|-----------|-----------|
| **HTMLScript** | 解析 `<script src>` 标签提取直接引用的 JS |
| **DynamicImport** | `import()` 动态加载、`import(/* webpackChunkName */)` 注释 |
| **Webpack** | `__webpack_require__.e()` 动态加载、chunk map 探测、webpackChunk 全局变量、字符串 chunk ID 映射 |
| **Next.js** | App Router / Pages Router chunk 探测、build manifest、flight chunk |
| **Nuxt.js** | `/_nuxt/` 路径模式、build assets |
| **Vite** | `__vitePreload()`、modulepreload、懒加载 chunk |
| **SvelteKit** | `/_app/immutable/nodes/` 和 `/_app/immutable/chunks/` 路径 |
| **RequireJS** | `require()` / `define()` 依赖加载、data-main |
| **Module Federation** | `__webpack_require__.federation` 远程模块、`manifest.json` 解析 |
| **ModuleFederationManifest** | Module Federation `manifest.json` 中的 shared/exposes 模块提取 |
| **HelMicro** | metadata.json 组件配置、CDN prefix |
| **ESMImport** | 静态 `import` 声明提取 |
| **ScriptCreate** | `document.createElement('script')` 动态加载 |
| **ModernJS** | ByteDance ModernJS route manifest、b.p publicPath |
| **URLPattern** | 通用 URL 模式匹配和路径探测 |
| **SourceMap** | `.map` 文件探测（通过 `sourceMappingURL`、HTTP 头或内联 data URI） |

Source Map 支持：通过 `sourceMappingURL` 注释、HTTP 响应头或内联 data URI 自动探测。

## 输出格式

### Text（默认）

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

## 缓存

默认启用缓存。缓存在系统临时目录中：

| 系统 | 缓存目录 |
|------|----------|
| Linux/Mac | `/tmp/ejfkdev/dj/` |
| Windows | `%TEMP%\ejfkdev\dj\` |

缓存结构：

```
<temp_dir>/ejfkdev/dj/<origin>/
├── js/                    # 下载的 JS 文件
├── source_map/            # Source Map 文件
├── html/                  # 原始 HTML
└── meta.json             # 站点元数据
```

## 常见问题

**为什么有些动态加载的 JS 没有被提取到？**

本工具通过静态分析 JS 代码来探测动态加载模式。如果网站使用特殊的加载方式，可能无法覆盖。如果你发现某个网站的动态加载 JS 无法被提取，欢迎提交 [Issue](https://github.com/ejfkdev/dj/issues)，并提供网站地址和相关代码线索。

## 许可证

[MPL 2.0 (Mozilla Public License)](LICENSE)
