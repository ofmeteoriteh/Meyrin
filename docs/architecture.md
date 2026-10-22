# 架构

> v0.1 · draft · 2026-10-22

Meyrin 是本仓库测试规范的执行基底：按 [SPEC](SPEC.md) 调用受测模型、计算程序指标、调用评审模型、汇总统计并生成静态站。
测试规则以 SPEC 为唯一事实来源，本文只记录实现结构；两者冲突时以 SPEC 为准。
开发顺序见 [dev-guide](dev-guide.md)。

## 流水线

```text
data/questions.jsonl ─┐
configs/models.yaml ──┼─> run ─────> responses.jsonl ─┬─> metrics ─> metrics.jsonl ──┐
.env ─────────────────┘                               └─> judge ───> judgments.jsonl ─┤
                                                                       human/ ─────────┤
                                                                                       v
                                                             stats ─> summary.json ─> site ─> _site/
```

每一步只读取前一步写入 `results/<ROUND>/` 的文件，可单独重跑。
`run` 与 `judge` 访问模型 API，只在本地运行；其余步骤离线。

## 目录结构

```text
Meyrin/
├── cmd/meyrin/            # 入口
├── mdtable/               # 按锚点行读取 Markdown 表格
├── bank/                  # 题库加载、校验、从 SPEC 生成
├── config/                # 模型清单、端点文档与 .env 读取
├── provider/              # 四类协议的 adapter 与 HTTP 客户端
├── round/                 # 结果目录：manifest 与回答记录
├── runner/                # 调度、重试、断点续跑
├── metrics/               # 程序指标
├── judge/                 # 评审请求、输出校验、计票
├── stats/                 # α、PSA、bootstrap
├── report/                # 汇总 summary.json
├── site/                  # 静态站生成
├── data/
│   ├── questions.jsonl
│   └── dict/              # OpenCC 单字表与 Unicode emoji 数据，固定版本
├── prompts/judge-system.md
├── configs/models.yaml
├── results/<ROUND>/       # 每轮结果，入库
└── docs/
```

- 入口放在 `cmd/meyrin/`：`go build` 与 `go install` 以 import 路径最后一段命名可执行文件，入口放在模块根会产出 `Meyrin` 而非 `meyrin`。
- 领域包直接放在顶层，不设 `internal/`：Meyrin 只交付一个二进制，不供其他模块 import。
- SPEC 与 `docs/api-endpoints.md` 中的表格由 `mdtable` 直接读取：题库的风格标签、模型清单的厂商名单、端点与协议都以文档为准，代码不另存副本。锚点行或表头改动时解析报错，不会按错列读取。
- `_site/` 是 `meyrin site` 的默认输出目录，与 `actions/upload-pages-artifact` 的默认 `path` 一致，不入库。

## Provider 协议

每类协议一个 adapter，厂商与协议的对应以 [api-endpoints](api-endpoints.md)「协议」表为准。
`config` 包在运行时读取该文件的端点表与协议表，端点不在别处另存。

- adapter 统一输出 `content`、`reasoning`、`finish_reason`、`usage`、`model_returned`，对应 SPEC「结果格式」的同名字段。
- `base_url` 与密钥页只取自 `docs/api-endpoints.md`。Google 使用 AI Studio 密钥与 Interactions API。
- 受测请求只含 `messages`、模型 ID、`max_tokens`，以及厂商支持时的 `store: false`；采样与推理参数不传。
- `reasoning` 只收厂商默认返回的推理正文；需另行请求摘要或只返回加密内容的厂商，`reasoning` 留空。`usage` 中的推理 token 数同理，厂商不提供则不记。
- 多轮题的历史只回传 `content`。厂商要求回传推理签名或禁止回传推理内容的，以官方文档为准，处理方式记入 `manifest.json`。

## 模型清单

`configs/models.yaml` 按厂商列出凭据变量名、模型 ID 与可选代理；评审席位列出厂商、调用方式、独立凭据变量名与模型 ID，订阅方式另列地址变量名。受测端点、鉴权头与协议取自 `docs/api-endpoints.md`，订阅评审的地址只写在 `.env`。

- 受测模型 ID 由作者实测选定。
- 评审模型取冻结时各系列的最新版本：Gemini Flash（订阅）、DeepSeek Flash（官方 API）。
- DeepSeek 的 `deepseek-flash` 是原地更新的别名，由作者手动跟进；每轮以 `model_returned` 记录实际版本。
- `meyrin models check` 核对清单的厂商与评审席位与 SPEC「受测模型：」「评审模型：」两表逐行一致，每家厂商在端点文档中有端点与协议，凭据变量名唯一且与 `.env.example` 一致。

## 结果目录

`results/<ROUND>/` 入库，冻结后不手改：

| 文件 | 内容 |
|---|---|
| `manifest.json` | SPEC 版本、题库哈希、模型清单、评审 prompt 哈希、库与字典版本、随机种子与 bootstrap 次数、时间窗口、第 7 题参考值、各厂商多轮与推理处理方式 |
| `responses.jsonl` | 受测回答，字段见 SPEC「结果格式」 |
| `metrics.jsonl` | 程序指标 |
| `judgments.jsonl` | 评审输出 |
| `human/` | 人工核查与复核 |
| `summary.json` | 汇总、α、PSA 与置信区间 |

## runner

- 每家厂商一个串行工作者，各家之间并行；代理按厂商配置，不读取环境中的 `HTTPS_PROXY`。
- 一段对话的各轮全部成功后一次写入 `responses.jsonl`，中途失败则整段不写；断点续跑以 `(provider, question_id, run)` 为键跳过已有记录。
- 429、5xx、网络故障，以及厂商在响应体中报告的临时故障（如 `network_error`、`insufficient_system_resource`），以同一请求体指数退避重试：2 秒起、上限 60 秒、最多 6 次，`Retry-After` 更长时取之。
- 截断的结果作废，输出上限从 8192 翻倍重跑该轮，到 65536 仍截断则该对话记为失败；实际上限写入 `request`，重跑次数计入 `retries`。
- 内容审核类结束原因（如 `sensitive`、`content_filter`）照收，不重跑。
- 第 7 题参考值在本轮首次运行时从 kernel.org 的 `releases.json` 取 `latest_stable` 写入 `manifest.json`；获取失败则中止，以 `-kernel` 手填。
- 同一轮内厂商的模型、端点与协议不得更改，续跑时与 `manifest.json` 核对；更换须新开一轮。
- `-dry-run` 不发请求、不读凭据，输出每家每题每轮的完整 `request`，多轮题的上一轮回答以占位文字代替。

## metrics

逐条实现 SPEC「程序指标」，每个指标有表驱动测试，至少覆盖：

- `**粗体**` 开头的行与 `---` 分隔线不计入 `list_items`，`1、` 计入。
- 代码块内的内容不参与代码块外的指标。
- 第 2 题：英文缩写与小数点不切句；末句没有句末标点仍计一句。
- `words_cjk`：`502 Bad Gateway` 计 3。
- 第 28 题：纯繁体、纯简体、混用、无法判定各一例。
- 第 35 题：两轮分别检查。

## judge

- `prompts/judge-system.md` 含 SPEC 的风格标签定义与归属规则，固定部分在前以利用 prompt caching；文件哈希写入 `manifest.json`，内容修改后重评全部结果。
- 输入按 SPEC「评审输入」组织，去除模型名称与厂商信息，打乱顺序。
- 输出按 SPEC「评审输出」的 schema 校验；失败以同一请求重试，超过上限标记无效并计入报告。支持结构化输出的厂商使用结构化输出。
- 两个评审都评全部回答，不回避自家厂商；自评偏差按 SPEC「一致性检验」量化。
- 订阅评审走 OpenAI Chat Completions 协议：地址变量是以 `/v1` 结尾的 base URL，请求发往其后的 `/chat/completions`。
- 订阅评审在使用前自检：发送一段含客套话的固定文本要求原样复述，并比对输入 token 数，确认请求内容未被改写、未被注入额外的 system prompt。

## stats

- Krippendorff's α 取名义尺度，支持缺失值；以 Krippendorff 公开示例与 Python `krippendorff` 包生成的对照值做 golden 测试，Python 不进入运行时依赖。
- PSA 按 SPEC 公式实现，附手算用例。
- bootstrap 以回答为单位重采样，固定种子。
- 按 SPEC「一致性检验」的区间规则输出判定，并输出两个评审在 Google 与 DeepSeek 回答上的自评偏差及其基线。

## site

- `html/template` 生成纯静态站，原生 JS 只用于表格排序与筛选，样式支持亮色与暗色。
- 页面：首页、题目页、模型页、回答详情、可靠性页、规范页（渲染 `docs/SPEC.md`）。
- 模型回答是不可信内容：Markdown 渲染后经 HTML 清洗，禁止原始 HTML 与脚本。

## 第三方数据

`data/dict/` 的文件固定版本，许可写入 `NOTICE`：

| 数据 | 来源 | 许可 |
|---|---|---|
| `STCharacters.txt`、`TSCharacters.txt` | OpenCC | Apache-2.0 |
| emoji 数据文件 | Unicode | Unicode License V3，版权与许可声明随文件分发 |

## 安全模型

- 凭据只放 `.env`，仓库提交只含变量名与占位符的 `.env.example`。
- 日志、错误信息与 `request` 记录中不出现 key，`Authorization` 等请求头一律脱敏。
- CI 与 Pages 构建不调用任何模型 API，workflow 中不使用 API key。
- 受测模型不接收工具、system prompt 与联网能力，Meyrin 不做循环执行。
