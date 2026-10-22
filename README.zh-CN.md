[![License](https://img.shields.io/badge/license-Apache--2.0%20%7C%20CC%20BY--NC%204.0-lightgrey?style=flat-square)](#许可与免责声明)
![Benchmark](https://img.shields.io/badge/benchmark-LLM%20style-4da3ff?style=flat-square)

[English](README.md) | 中文

<p align="center"><img src="assets/meyrin-icon.svg" width="120" alt="Meyrin logo"></p>

# Meyrin

> 通过官方 API 对比大模型回答风格的黑盒测试。

## 简介

Meyrin 比较不同模型"怎么回答"，而不是"答得对不对"。
测试集共 37 道低难度题目，分为九个维度：格式与表达、诚实与准确、立场与谄媚、边界与说教、情绪与人格、创作、多语言、代码、交互。

每道题逐字发送给每个模型的官方 API，不带 system prompt、不带工具、不联网。
回答由盲评的 LLM 评审标注为各题的结果项与 11 个风格标签，另有程序计算的指标。
评审一致性用 Krippendorff's α（结果项）与 positive specific agreement（标签）报告，均附 bootstrap 置信区间。

## Prerequisites

- Go 1.27+

## 示例

从规范重新生成机读题库：

```bash
go run ./cmd/meyrin bank build
```

```text
wrote 37 questions to data/questions.jsonl
```

## 命令

在仓库根目录运行，路径默认按仓库结构取。

| 命令 | 用途 |
|---|---|
| `go run ./cmd/meyrin bank build` | 从 `docs/SPEC.md` 重新生成 `data/questions.jsonl` |
| `go run ./cmd/meyrin bank check` | 校验入库题库与规范逐字节一致 |
| `go run ./cmd/meyrin models check` | 核对 `configs/models.yaml` 与规范、`docs/api-endpoints.md`、`.env.example` 一致 |
| `go run ./cmd/meyrin run -round <ROUND> -dry-run` | 输出全部请求，不发送、不读取凭据 |
| `go run ./cmd/meyrin run -round <ROUND> [-runs 3] [-only q1,q24] [-vendors <A,B>]` | 调用受测模型，结果追加到 `results/<ROUND>/`；重复执行即续跑 |
| `go test ./...` | 运行单元测试 |

## 文档

| 文档 | 用途 |
|---|---|
| [docs/SPEC.md](docs/SPEC.md) | 测试规范：题目、结果项、风格标签、评审、一致性阈值 |
| [docs/api-endpoints.md](docs/api-endpoints.md) | 各厂商的官方 API 密钥页、base_url 与文档 |
| [docs/architecture.md](docs/architecture.md) | 流水线、包结构、provider 协议、结果文件、安全模型 |
| [docs/dev-guide.md](docs/dev-guide.md) | 环境、开发顺序、规范与题库同步、CI |

## 已知局限

- 结论仅代表模型的 API 版本。部分厂商 App 与 API 的模型版本不同，API 侧也可能存在内容审核层。
- 盲评不彻底：回答中自报身份的内容无法匿名化。

## 许可与免责声明

按路径分别以 [Apache License 2.0](LICENSE) 和 [知识共享 署名-非商业性使用 4.0 国际许可协议](LICENSE-DATA) 发布：

| 路径 | 许可 |
|---|---|
| `docs/SPEC.md`、`data/questions.jsonl` | CC BY-NC 4.0（[LICENSE-DATA](LICENSE-DATA)） |
| 其余全部路径 | Apache-2.0（[LICENSE](LICENSE)） |

本项目由个人贡献者出于自愿、以非商业方式开发和维护。

本软件按"现状"提供，不附带任何明示或暗示的担保。作者不对使用本软件造成的任何损害负责，包括但不限于数据丢失、系统损坏或安全事件。完整条款见 [LICENSE](LICENSE)。

`meyrin bank` 与 `meyrin models` 只读写仓库内的文件，不发起任何网络请求。`meyrin run` 用 `.env` 中的密钥把 `data/questions.jsonl` 的提示词发给 `configs/models.yaml` 配置的厂商 API；密钥不会出现在结果与日志中，API 支持时请求会关闭服务端存储。执行 `docs/SPEC.md` 中的测试流程会用运行者提供的凭据向第三方模型 API 发送提示词，返回的回答属第三方内容，受各厂商条款约束。

任何商业实体使用本软件，须自行负责遵守适用的法律法规，包括但不限于欧盟《网络弹性法案》（CRA）及其他地区性要求。
