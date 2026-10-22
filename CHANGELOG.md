# Changelog / 更新日志

## v0.1 (2026-10-22)

**Initial test set and harness** — the 37-question style specification with its machine-readable bank, and the Go harness that runs it end to end: calling the models, program metrics, judging, reliability statistics and the static site.
**初版测试集与测试基底** — 37 题的风格测试规范与机读题库，以及端到端执行它的 Go 基底：调用受测模型、程序指标、评审、可靠性统计与静态站。

- **Added:** `docs/SPEC.md` and `data/questions.jsonl`; `meyrin bank build` and `meyrin bank check` keep the bank byte-identical to the spec
  新增：`docs/SPEC.md` 与 `data/questions.jsonl`；`meyrin bank build` 与 `meyrin bank check` 保证题库与规范逐字节一致
- **Added:** `meyrin models check` for `configs/models.yaml` against the spec, `docs/api-endpoints.md` and `.env.example`
  新增：`meyrin models check` 核对 `configs/models.yaml` 与规范、`docs/api-endpoints.md`、`.env.example`
- **Added:** `meyrin run` with four protocol adapters, per-vendor workers, retries, truncation reruns and resume; each round keeps a copy of the bank
  新增：`meyrin run`，含四类协议 adapter、按厂商的工作者、重试、截断重跑与断点续跑；每轮保存题库副本
- **Added:** `meyrin metrics` with pinned OpenCC ver.1.4.2 and Unicode Emoji 18.0 data under `data/dict/`
  新增：`meyrin metrics`，使用 `data/dict/` 中固定版本的 OpenCC ver.1.4.2 与 Unicode Emoji 18.0 数据
- **Added:** `meyrin judge` with `prompts/judge-system.md`, output validation, quoted-evidence checks and a self-check for the subscription judge
  新增：`meyrin judge`，含 `prompts/judge-system.md`、输出校验、证据原文核对与订阅评审自检
- **Added:** `meyrin stats` for Krippendorff's α, positive specific agreement, bootstrap intervals, self-preference bias and the human review list
  新增：`meyrin stats`，计算 Krippendorff's α、positive specific agreement、bootstrap 区间、自评偏差与人工复核清单
- **Added:** `meyrin site` and the `pages.yml` workflow publishing the static site
  新增：`meyrin site` 与发布静态站的 `pages.yml` workflow
