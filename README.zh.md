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
- 自动发现 Source Map 并**还原原始源码**（优先 sourcesContent，缺失时用 mappings VLQ 回退）
- **缓存复用**：第二次运行同一站点时从本地缓存恢复，零网络请求
- TLS 指纹伪装，**随机化浏览器指纹**（Chrome、Firefox、Safari、Edge、iOS），绕过 Cloudflare 等 WAF
- HTTP/2 和 HTTP/1.1 协议自动协商
- SOCKS5/HTTP/HTTPS 代理支持，支持认证
- 环境变量代理配置（`HTTPS_PROXY`、`ALL_PROXY`、`NO_PROXY` 等）
- 自定义 User-Agent 和浏览器请求头模拟
- 多种输出格式：text、json、markdown

## 支持的框架与加载模型

dj 的提取器已在 **73 类框架 / 加载模型**、**562 个实测版本**的真实构建产物上验证。

| 分类 | 框架 |
|------|------|
| 打包器（14） | `webpack`、`webpack4`、`vite`、`rollup`、`esbuild`、`parcel`、`rsbuild`、`rspack`、`farm`、`mako`、`rolldown`、`systemjs`、`snowpack`、`turbopack` |
| 微前端框架（17） | `qiankun`、`single-spa`、`micro-app`、`wujie`、`garfish`、`icestark`、`piral`、`luigi`、`emp`、`hel-micro`、`native-federation`、`module-federation`、`mf-runtime`、`mf-vite`、`mf-rsbuild`、`webpack-mf`、`vite-plugin-federation` |
| 元框架 / SSR（25） | `nuxt`、`sveltekit`、`angular`、`astro`、`qwik`、`solidstart`、`tanstack-start`、`marko`、`react-router`、`remix2`、`redwood`、`analog`、`vike`、`hono`、`one`、`fresh`、`stencil`、`storybook`、`gatsby`、`cra`、`umi`、`icejs`、`vue-cli`、`modernjs`、`bun` |
| Rust / WASM 前端（3） | `leptos`、`lustre`、`trunk` |
| 后台模板（12） | `ant-design-pro`、`amis`、`ng-alain`、`jeecg`、`d2admin`、`pig-ui`、`tdesign-starter`、`vue-element-admin`、`vue-pure-admin`、`vue-vben-admin`、`ruoyi-vue2`、`ruoyi-vue3` |
| 规范 / 垫片（1） | `es-module-shims` |

## 版本明细（点击展开）

<details><summary><b>打包器</b>（14 类，展开查看已验证版本）</summary>

| 框架 | 说明 | 已验证版本 |
|------|------|------------|
| **webpack** | webpack 5 运行时 chunk 映射 | v5.0.0 ~ v5.109.2（18 个版本） |
| **webpack4** | webpack 4 JSONP 运行时 | v4.33.0 ~ v4.47.0（15 个版本） |
| **vite** | dynamic import / modulepreload / prefetch | v3.0.9 ~ v8.2.1（13 个版本） |
| **rollup** | 多入口 + 动态导入 | v2.0.6 ~ v4.62.4（18 个版本） |
| **esbuild** | ESBuild 产物 | v0.14.54 ~ v0.28.2（15 个版本） |
| **parcel** | Parcel 产物 | v2.0.1 ~ v2.16.4（4 个版本） |
| **rsbuild** | Rsbuild / Rspack 生态 | v1.0.19 ~ v1.7.6（7 个版本） |
| **rspack** | runtime hash 映射 | v1.0.14 ~ v2.1.10（10 个版本） |
| **farm** | Rust 打包器 | v0.1.1 ~ v1.0.5（5 个版本） |
| **mako** | 字节 Mako | v0.4.17 ~ v0.11.15（8 个版本） |
| **rolldown** | Rolldown | v0.9.2 ~ v1.2.4（10 个版本） |
| **systemjs** | SystemJS 规范产物 | v0.21.6 ~ v6.15.1（17 个版本） |
| **snowpack** | 无打包 ESM 产物 | v2.18.5 ~ v3.8.8（10 个版本） |
| **turbopack** | Next.js Turbopack | v15.3.9 ~ v16.3.1（7 个版本） |

</details>

<details><summary><b>微前端框架</b>（17 类，展开查看已验证版本）</summary>

| 框架 | 说明 | 已验证版本 |
|------|------|------------|
| **qiankun** | qiankun 2.x（含 vite-plugin-qiankun） | v2.1.1 ~ v2.10.16（10 个版本） |
| **single-spa** | 根应用 | v4.4.4 ~ v6.0.3（12 个版本） |
| **micro-app** | micro-app | v0.1.0 ~ v0.8.11（8 个版本） |
| **wujie** | wujie | v1.0.29 ~ v2.1.0（3 个版本） |
| **garfish** | Garfish | v1.0.26 ~ v1.19.12（10 个版本） |
| **icestark** | ICEstark | v1.0.0 ~ v2.8.4（11 个版本） |
| **piral** | Piral | v1.1.0 ~ v1.12.2（12 个版本） |
| **luigi** | Luigi | v0.0.9 ~ v1.7.11（10 个版本） |
| **emp** | @efox/emp | v1.0.34 ~ v1.10.2（11 个版本） |
| **hel-micro** | hel-micro | v3.5.16 ~ v4.15.3（10 个版本） |
| **native-federation** | Native Federation | v0.9.1 ~ v4.4.1（11 个版本） |
| **module-federation** | @module-federation/enhanced | v0.0.17 ~ v2.8.2（11 个版本） |
| **mf-runtime** | MF 纯运行时 | v2.1.0 ~ v2.8.2（8 个版本） |
| **mf-vite** | MF Vite 集成 | v1.13.7 ~ v1.20.7（8 个版本） |
| **mf-rsbuild** | MF Rsbuild 插件 | v2.1.0 ~ v2.8.2（8 个版本） |
| **webpack-mf** | webpack 官方 MF | v5.98.0 ~ v5.108.4（10 个版本） |
| **vite-plugin-federation** | Origin.js vite 联邦 | v0.0.3 ~ v1.4.1（6 个版本） |

</details>

<details><summary><b>元框架 / SSR</b>（25 类，展开查看已验证版本）</summary>

| 框架 | 说明 | 已验证版本 |
|------|------|------------|
| **nuxt** | Nuxt | v3.0.0 ~ v4.5.2（8 个版本） |
| **sveltekit** | SvelteKit | v2.0.8 ~ v2.70.2（10 个版本） |
| **angular** | Angular | v17.0.10 ~ v20.3.34（10 个版本） |
| **astro** | Astro | v3.6.5 ~ v7.2.2（4 个版本） |
| **qwik** | Qwik | v1.9.1 ~ v1.20.0（12 个版本） |
| **solidstart** | SolidStart | v1.1.7 ~ v2.0.0（4 个版本） |
| **tanstack-start** | TanStack Start | v1.111.15 ~ v1.168.46（8 个版本） |
| **marko** | Marko | v0.1.16 ~ v0.11.9（5 个版本） |
| **react-router** | React Router 框架模式 | v7.0.2 ~ v8.3.0（8 个版本） |
| **remix2** | Remix | v2.10.3 ~ v2.17.5（8 个版本） |
| **redwood** | Redwood | v6.6.4 ~ v8.9.0（3 个版本） |
| **analog** | Analog | v0.2.45 ~ v2.7.0（4 个版本） |
| **vike** | Vike | v0.4.249 ~ v0.4.260（12 个版本） |
| **hono** | Hono JSX | v4.2.9 ~ v4.13.2（12 个版本） |
| **one** | One | v1.17.11 ~ v1.24.5（8 个版本） |
| **fresh** | Deno Fresh | v1.4.3 ~ v2.3.3（7 个版本） |
| **stencil** | Stencil | v4.33.1 ~ v4.44.0（12 个版本） |
| **storybook** | Storybook 静态导出 | v9.0.18 ~ v10.5.9（8 个版本） |
| **gatsby** | Gatsby | v4.25.9 ~ v5.16.1（5 个版本） |
| **cra** | Create React App | v5.0.0 ~ v5.0.1（2 个版本） |
| **umi** | UmiJS 4 | v4.0.90 ~ v4.7.6（8 个版本） |
| **icejs** | ICE.js | v3.0.6 ~ v3.6.5（7 个版本） |
| **vue-cli** | Vue CLI | v4.0.5 ~ v5.0.9（7 个版本） |
| **modernjs** | Modern.js | v2.0.2 ~ v3.8.2（11 个版本） |
| **bun** | Bun 打包器 | v0.6.14 ~ v1.3.14（7 个版本） |

</details>

<details><summary><b>Rust / WASM 前端</b>（3 类，展开查看已验证版本）</summary>

| 框架 | 说明 | 已验证版本 |
|------|------|------------|
| **leptos** | Leptos | v0.4.10 ~ v0.8.20（5 个版本） |
| **lustre** | Gleam Lustre | v5.4.0 ~ v5.7.1（4 个版本） |
| **trunk** | Trunk（wasm-bindgen） | v0.17.5 ~ v0.21.14（5 个版本） |

</details>

<details><summary><b>后台模板</b>（12 类，展开查看已验证版本）</summary>

| 框架 | 说明 | 已验证版本 |
|------|------|------------|
| **ant-design-pro** | Ant Design Pro | v4.4.0 ~ v6.0.3（5 个版本） |
| **amis** | 百度 amis | v1.9.0 ~ v6.13.0（4 个版本） |
| **ng-alain** | NG-Alain | v17.3.1 ~ v21.3.0（5 个版本） |
| **jeecg** | JeecgBoot 前端 | v3.3.0 ~ v3.4.3（2 个版本） |
| **d2admin** | D2Admin | v1.25.0 |
| **pig-ui** | Pig UI | v4.1.0 |
| **tdesign-starter** | TDesign Starter | v0.11.0 ~ v0.14.0（2 个版本） |
| **vue-element-admin** | vue-element-admin | v4.3.1 ~ v4.4.0（2 个版本） |
| **vue-pure-admin** | vue-pure-admin | v6.3.0 ~ v7.0.0（2 个版本） |
| **vue-vben-admin** | Vue Vben Admin | v5.3.2 ~ v5.4.8（2 个版本） |
| **ruoyi-vue2** | RuoYi-Vue2 | v3.8.9 ~ v3.9.2（3 个版本） |
| **ruoyi-vue3** | RuoYi-Vue3 | v3.9.1 ~ v3.9.2（2 个版本） |

</details>

<details><summary><b>规范 / 垫片</b>（1 类，展开查看已验证版本）</summary>

| 框架 | 说明 | 已验证版本 |
|------|------|------------|
| **es-module-shims** | importmap 裸说明符 | v2.1.2 ~ v2.8.4（8 个版本） |

</details>


## 安装

### Homebrew（macOS）

```bash
brew install ejfkdev/tap/dj
```

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
| `-v, --version` | 打印版本并退出 |
| `-d, --debug` | 启用调试输出 |
| `-f, --format <fmt>` | 输出格式：`md`（默认）、`json`、`text`（纯一行一个 URL 列表） |
| `--no-cache` | 禁用缓存读取（下载的文件仍会保存到磁盘）；`--cache` / `--cache=false` 旧语法保留兼容 |
| `--useragent=<UA>` | 自定义 User-Agent |
| `--ua=<UA>` | `--useragent` 的短别名 |
| `-x, --proxy <URL>` | 代理地址（http/https/socks5），优先级高于环境变量 |
| `--cookie=<cookies>` | 注入 Cookie 绕过 Cloudflare（如 `"cf_clearance=xxx"`） |
| `-H, --header <K: V>` | 自定义 HTTP 请求头，可重复指定（curl 风格） |
| `--no-random-tls` | 关闭随机化 TLS 指纹（使用固定 Chrome 指纹） |
| `-o, --output <dir>` | 输出目录（将所有文件保存一份到此目录：js/、html/、source_map/、sources/，不含站点子目录） |
| `-t, --timeout <secs>` | 单个 HTTP 请求超时秒数（默认 30） |
| `-c, --concurrency <N>` | 全局 HTTP 并发上限——下载、探测、HEAD/RSC 共享同一预算（默认 8） |
| `-h, --help` | 显示帮助信息 |

### 示例

```bash
# 自定义 User-Agent
dj --useragent="Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X) ..." https://example.com

# 使用 HTTP 代理
dj --proxy="http://127.0.0.1:7890" https://example.com

# 使用 SOCKS5 代理（短参数 -x）
dj -x socks5://127.0.0.1:1080 https://example.com

# 使用 HTTPS 代理
dj --proxy="https://proxy.example.com:443" https://example.com

# 带认证的代理
dj --proxy="socks5://user:pass@127.0.0.1:1080" https://example.com

# 使用环境变量代理（HTTPS_PROXY、HTTP_PROXY、ALL_PROXY）
HTTPS_PROXY=http://127.0.0.1:7890 dj https://example.com

# 跳过指定主机的代理
ALL_PROXY=socks5://127.0.0.1:1080 NO_PROXY=localhost,example.com dj https://example.com

# 注入 Cookie 绕过 Cloudflare
dj --cookie="cf_clearance=xxx" https://example.com

# 启用调试模式
dj --debug https://example.com

# 保存到自定义输出目录（不含站点子目录）
dj -o ./output https://example.com

# 设置单请求超时秒数（默认 30）
dj -t 60 https://example.com

# 组合使用：全新扫描、输出到指定目录、带代理和超时
dj --no-cache -o ./output -x socks5://127.0.0.1:1080 -t 60 https://example.com
```

<details>
<summary>📊 测试网站（点击展开）</summary>
> 测试快照：dj v0.5.21，2026-08-19。⏱ = 超时未完成（重测中）。


**框架 / 后台管理**

| 站点 | JS | 站点 | JS |
|------|----|------|----|
| [vue.ruoyi.vip](https://vue.ruoyi.vip) | 74 | [demo.1panel.cn](https://demo.1panel.cn) | 590 |
| [show.cool-admin.com/login](https://show.cool-admin.com/login) | 135 | [ant.design](https://ant.design) | 2541 |
| [arco.design](https://arco.design) | 11 | [vuejs.org](https://vuejs.org) | 60 |
| [react.dev](https://react.dev) | 39 | [svelte.dev](https://svelte.dev) | 71 |
| [angular.io](https://angular.io) | 290 | [nuxt.com.cn](https://nuxt.com.cn) | 176 |

**AI / 云平台**

| 站点 | JS | 站点 | JS |
|------|----|------|----|
| [chat.deepseek.com](https://chat.deepseek.com) | 41 | [chat.z.ai](https://chat.z.ai) | 168 |
| [kimi.moonshot.cn](https://kimi.moonshot.cn) | 484 | [cloud.tencent.com](https://cloud.tencent.com) | 107 |
| [docs.qq.com](https://docs.qq.com) | 9 | [www.aliyun.com](https://www.aliyun.com) | 270 |

**企业 / 协作**

| 站点 | JS | 站点 | JS |
|------|----|------|----|
| [feishu.cn](https://www.feishu.cn) | 69 | [dingtalk.com](https://www.dingtalk.com) | 44 |
| [youzan.com](https://www.youzan.com) | ⏱ | [kingdee.com](https://www.kingdee.com) | 167 |
| [chanjet.com](https://www.chanjet.com) | 27 | [landray.com.cn](https://www.landray.com.cn) | 46 |

**电商 / 门户**

| 站点 | JS | 站点 | JS |
|------|----|------|----|
| [gitee.com](https://gitee.com) | 260 | [baidu.com](https://www.baidu.com) | 506 |
| [meituan.com](https://www.meituan.com) | 177 | [pinduoduo.com](https://www.pinduoduo.com) | 11 |
| [bilibili.com](https://www.bilibili.com) | 369 | [juejin.cn](https://www.juejin.cn) | 102 |

**政务 / 高校**

| 站点 | JS | 站点 | JS |
|------|----|------|----|
| [shanghai.gov.cn](https://www.shanghai.gov.cn) | 131 | [xinhuanet.com](https://www.xinhuanet.com) | 92 |
| [zju.edu.cn](https://www.zju.edu.cn) | 72 | [tsinghua.edu.cn](https://www.tsinghua.edu.cn) | 37 |
| [chaoxing.com](https://www.chaoxing.com) | 36 | [www.people.com.cn](https://www.people.com.cn) | 78 |

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

## 工作原理

1. 下载目标网页 HTML
2. 启动插件分析流程，每个 URL 并行由 goroutine 处理：
   - 下载 JS 内容
   - 检测 Content-Type（跳过 HTML 响应的静态资源）
   - 分发给所有插件进行模式匹配
3. 插件发现新的 JS URL 或路径片段，添加到待处理队列
4. 探测 Source Map 文件（通过 `sourceMappingURL` 或 HTTP 头）
5. 从 Source Map 还原原始源码：
   - 优先：从 `sourcesContent` 字段提取完整原始源码
   - 回退：`sourcesContent` 缺失时解析 `mappings`（VLQ 解码）重组
   - 还原出的文件按原始目录结构写入 `sources/`
6. 收集所有发现的 JS URL 并输出

### 缓存复用

启用缓存时（默认），第二次运行同一站点会完全跳过网络请求：
- 从 `meta.json` 加载之前发现的 JS URL 列表
- 从本地缓存恢复 source map 和源码
- 使用 `--no-cache` 可强制全量重新扫描（文件仍会保存到磁盘，只是不从缓存读取）

### 输出目录

使用 `-o/--output` 可将所有文件额外保存一份到指定目录（不含站点子目录层级）：

```
<output_dir>/
├── js/                    # 下载的 JS 文件
├── source_map/            # Source Map 文件
├── sources/               # 还原的原始源码（保留目录结构）
├── html/                  # 原始 HTML
└── meta.json             # 站点元数据
```

缓存目录始终正常写入；`-o` 是在缓存基础上额外写一份副本。

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
| **Qiankun** | single-spa 微前端子应用：探测 `entry`/`proEntry` HTML 目录入口，再提取子应用脚本、modulepreload 与内联 `import()` |

Source Map 支持：通过 `sourceMappingURL` 注释、HTTP 响应头或内联 data URI 自动探测。

## 输出格式
`-f json` output includes a `jsDetails` array with per-JS provenance
(discovered-by plugin, source URL, inline flag); `meta.json` records the same
via `source_url` / `from_plugin`.


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
├── sources/               # 还原出的原始源码（保留原始目录结构）
├── html/                  # 原始 HTML
└── meta.json             # 站点元数据（JS URL、source map 路径、还原源码列表、缓存目录）
```

## 常见问题

**为什么有些动态加载的 JS 没有被提取到？**

本工具通过静态分析 JS 代码来探测动态加载模式。如果网站使用特殊的加载方式，可能无法覆盖。如果你发现某个网站的动态加载 JS 无法被提取，欢迎提交 [Issue](https://github.com/ejfkdev/dj/issues)，并提供网站地址和相关代码线索。

## 许可证

[MPL 2.0 (Mozilla Public License)](LICENSE)
