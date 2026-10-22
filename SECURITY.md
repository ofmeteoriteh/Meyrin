# Security Policy / 安全策略

Reports are accepted in **English or Chinese**.

报告接受**英文或中文**。

## Supported versions / 支持的版本

Only the latest commit on `main` is supported.

只支持 `main` 分支的最新提交。

## Reporting a vulnerability / 报告漏洞

**Do not open a public issue for security vulnerabilities.**

**不要为安全漏洞开公开 issue。**

Use GitHub's private vulnerability reporting:

改用 GitHub 的私密漏洞报告功能：

1. Open the **Security** tab of this repository. / 进入本仓库的 **Security** 标签页。
2. Click **Report a vulnerability**. / 点击 **Report a vulnerability**。

This keeps the report private until a fix is available.

这样在修复发布前，报告将保持私密。

> **Note:** This is a hobby project maintained in spare time. While security reports are taken seriously, **response is not guaranteed**.

> **注意：** 这是一个利用业余时间维护的兴趣项目。安全报告会被认真对待，但**不保证响应**。

## Scope / 范围

Meyrin reads API keys from `.env`, sends prompts and collected answers to third-party model APIs, commits the results to this repository, and publishes them as a static site.
Security-sensitive areas include, but are not limited to:

Meyrin 从 `.env` 读取 API 密钥，向第三方模型 API 发送提示词与收集到的回答，把结果提交进本仓库，并发布为静态站。
安全敏感面包括但不限于：

- **Credentials** — keys for the models under test and separate judge keys are kept in plaintext in `.env`, which is git-ignored.

  凭据 — 受测模型的密钥与独立的评审密钥以明文保存在 `.env`，该文件被 git 忽略。
- **Redaction** — keys must never appear in logs, error messages or the `request` field of committed results; `Authorization` and key headers are redacted.

  脱敏 — 密钥不得出现在日志、错误信息与入库结果的 `request` 字段中；`Authorization` 与携带密钥的请求头一律脱敏。
- **Outbound requests** — HTTPS calls to the vendor endpoints in `docs/api-endpoints.md`, to the judge endpoints configured in `.env`, and to kernel.org.

  外发请求 — 发往 `docs/api-endpoints.md` 中各厂商端点、`.env` 中配置的评审端点与 kernel.org 的请求。
- **Untrusted model output on the site** — answers are rendered from Markdown without raw HTML and with dangerous links filtered; everything else is escaped by `html/template`.

  站点上的不可信模型输出 — 回答按 Markdown 渲染，不输出原始 HTML、过滤危险链接；其余文字一律经 `html/template` 转义。
- **CI and Pages** — workflows build and publish from committed files only and use no API keys.

  CI 与 Pages — workflow 只基于入库文件构建与发布，不使用任何 API 密钥。

## Out of scope / 不在范围内

- Vulnerabilities in vendor APIs or in the relay serving a subscription judge (report upstream).

  厂商 API 或订阅评审所经中转服务的漏洞（向上游报告）。
- Content of model answers themselves, which is third-party output recorded as is.

  模型回答本身的内容，它们是按原样记录的第三方输出。
- Running Meyrin with keys or relays you do not control.

  使用不受你控制的密钥或中转服务运行 Meyrin。

## Disclaimer / 免责声明

This software is provided "as is", without warranty of any kind.
Any commercial entity using this software is solely responsible for its own compliance with applicable laws and regulations, including the EU Cyber Resilience Act (CRA).

本软件按"原样"提供，不附带任何形式的担保。
任何使用本软件的商业实体，须自行负责遵守适用的法律法规，包括欧盟《网络弹性法案》（CRA）。
