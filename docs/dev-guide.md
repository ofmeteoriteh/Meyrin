# 开发指南

> v0.1 · draft · 2026-10-22

实现结构见 [architecture](architecture.md)，测试规则见 [SPEC](SPEC.md)。

## 环境

- Go 1.27+，单二进制，标准库优先。
- 引入第三方依赖时在 PR 描述中说明理由。
- 凭据按 `.env.example` 写入仓库根的 `.env`。

## 开发顺序

全部工作在一个 `dev/<topic>` 分支上进行，能完整跑通一轮后以 merge commit 合入 `main`。

| 顺序 | 命令 | 完成标准 |
|---|---|---|
| 1 | `meyrin bank build`、`meyrin bank check`、`meyrin models check` | `bank build` 输出与 `data/questions.jsonl` 逐字节一致 |
| 2 | `meyrin run --round <ROUND> [--dry-run] [--runs 3] [--only q1,q24]` | `--dry-run` 输出全部厂商与题目的请求；每家真实调用 1 题成功 |
| 3 | `meyrin metrics --round <ROUND>` | architecture「metrics」列出的边界用例通过 |
| 4 | `meyrin judge --round <ROUND>` | 对第 2 步的回答完成评审；订阅评审通过自检；schema 校验与计票逻辑有测试 |
| 5 | `meyrin stats --round <ROUND>` | α 与 golden 值一致；`summary.json` 生成 |
| 6 | `meyrin site [--out _site/]` | 回答中的 `<script>` 被清洗；Pages 部署成功 |

之后的试跑、校准与冻结由作者主导，按 SPEC「校准与冻结」执行。

## 规范与题库

- `data/questions.jsonl` 由 `meyrin bank build` 从 `docs/SPEC.md` 生成，不手改。
- SPEC、题库与 `prompts/judge-system.md` 同版本维护，任一处修改须同步其余两处。

## CI

`ci.yml` 在 `main` 与 `dev/**` 上运行 `gofmt` 检查、`go vet`、`go build`、`go test ./...`、`meyrin bank check` 与 `meyrin models check`，不使用任何 API key。
`pages.yml` 在 `main` 更新时以 `meyrin site` 生成站点并发布到 GitHub Pages，同样不调用模型 API；仓库的 Pages 来源须设为 GitHub Actions。
