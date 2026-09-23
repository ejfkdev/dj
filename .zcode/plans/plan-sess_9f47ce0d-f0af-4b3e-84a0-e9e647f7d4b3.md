# 验证 pydj/jsdj 是否真的更强，并把优点吸收进 Go dj

## 背景（已完成的侦查结论）

两个重写项目都是 Go dj 的忠实移植（26 个插件一一对应，同样"抓取 + 2xx 验证"策略），没有 AST/无头浏览器/字典爆破等新机制。它们相对 Go 版的**真实差异**目前锁定为 5 处（其中 2 处已读 Go 源码确认）：

| # | 差异 | Go 现状（已核实） | 影响 |
|---|---|---|---|
| 1 | **CDN 前缀正则**：pydj 用 `[a-zA-Z0-9.-]*`，Go 用 `//[a-zA-Z0-9][-a-zA-Z0-9]*\.[a-zA-Z0-9]+`（只允许**两级域名**） | `pkg/plugins/urlpattern.go:25` | 3+ 级 CDN 主机（`cdn.foo.example.com`）匹配失败 → 少一批裸文件名补全 → **可能少提取 JS** |
| 2 | **source map 探测**：pydj/jsdj await；Go 是 `go p.probeSourceMap(...)` 不跟踪 + 主循环 `time.Sleep(500ms)` 轮询判空退出 | `pkg/extractor/pipeline.go:1466`（fire-and-forget）、`:302`（500ms settle） | 可能在探测回队前退出 → sourceMap/source 计数不稳定（jsCount 影响待实测）；sleep 也拖慢 |
| 3 | 跨域 source map 保留（Go 丢弃 CDN 上的 .map） | 待核实 | sourceMapCount 偏低 |
| 4 | 输出顺序确定性（jsdj 按插件注册序；Go 的 jsURLs 顺序每次运行都不同，harness 里已观察到） | 已实测到 Go 顺序随机 | 可复现性 |
| 5 | 缓存根命名一致性修复（pydj 声称修了 `https_x` vs `https_x_`） | 待核实 | 缓存复用 |

反向：Go 有两个 jsdj 没有的能力——**内置 uTLS**（真实站点上 jsdj 缺 TLS sidecar 会被 Cloudflare 类站点挡）与**完整 UmiJS 插件**（jsdj 里是空实现）。所以"重写更强"很可能是**特定站点 + 特定维度**的结论，不是全面超越。

## 第一步：实测验证（先量后改）

**目标站点集**（本地 harness 夹具，逐站独立起服，全部 `--no-cache`，统一 `-c 16 -t 30`）：

- 挑 24 个最能暴露差异的夹具：Go 已知 FAIL/WARN 的（d2admin、snowpack、turbopack 15.5.23、vue-pure-admin、webpack-mf、mf-vite、emp、ng-alain、gatsby、stencil、qwik、astro、one、sveltekit、es-module-shims、icejs、mf-runtime、module-federation）+ 4 个 Go PASS 对照（vite 8.3.0、rspack 2.2.3、webpack 5.110.3、turbopack 16.3.5）+ 2 个 Go 独有优势对照（umi、任意 Cloudflare 类真实站点可选）
- 三方命令：Go `bin/dj --no-cache -c 16 -t 30 -f json`、pydj `.venv/bin/pydj --no-cache -c 16 -t 30 -f json`、jsdj `node dist/cli/index.js --json --no-cache -c 16 -t 30`
- 每站记录：`summary.jsCount`、jsURLs 集合、判定（对照 compare.json 的 expected：漏多少）、墙钟耗时（各跑 2 次取较小值；顺序取平均）
- **另做针对性微实验**：造一个含 3 级 CDN 主机前缀 + 裸文件名的合成页面，验证差异 #1 的因果（Go 应漏、pydj/jsdj 应命中）

**判定标准**：若三方 jsCount/漏提数在统计意义上无差别 → 直接结论"重写并未更强"，只吸收 #2/#4 这类确定性/性能改进；若有差 → 定位到具体夹具与具体机制，只吸收被证明有效的那几条。

## 第二步：按实测结果实施 Go 优化（预期候选，全部以实测为准）

1. `urlpattern.go` CDN 主机正则改为支持多级域名（对齐 pydj 的 `[a-zA-Z0-9.-]*`，保持原有 quote/路径约束）
2. source map 探测纳入生命周期管理：用 `jsWg`（或独立 WaitGroup）跟踪探测 goroutine，主循环退出前等待；相应删掉/缩短 `time.Sleep(500ms)` 轮询（预期：计数稳定 + 提速）
3. 跨域 `.map` 保留（若核实 Go 确实丢弃）
4. 输出确定性：最终 `jsURLs` 排序（或按发现来源稳定排序）——写进 README 的"确定性"说明
5. 缓存根命名一致性（若核实有 bug）

**不做**（超出范围或收益不明）：引入 AST/无头浏览器、字典爆破、更换 HTTP 栈。

## 第三步：验证与收尾

- 每项改动后跑 harness 受影响框架的 `retest.sh`；全部完成后跑**全量 retest**，对比基线 **428 PASS / 119 WARN / 23 FAIL**（期望 FAIL 数下降、WARN 下降，且 FAIL 集合里能被修复的项消失）
- 提速对比：同一夹具集在新旧 Go 二进制下各跑一轮，报耗时差
- 若覆盖提升：更新 dj README 的框架版本表/总数（若新增机制改变了某框架结果）与"How it works"
- 提交（遵从不写目标站 URL 的规范）；是否发版（v0.6.3）由你决定
- 更新 memory：新增"pydj/jsdj 对照与吸收的改进"条目

## 风险

- 三方对同一站点的"发现"定义一致（2xx 验证），但 pydj/jsdj 缺 uTLS，真实反爬站点会天然吃亏 → 基准一律用本地夹具，避免把传输层差异误判成提取能力差异
- Go 输出顺序改变属可见行为变化（harness 与 diff 更稳，但用户脚本若依赖顺序会受影响，README 需注明）
- 若实测显示重写并无优势，则只落地 #2/#4 两项确定性/性能改进并如实说明