# 官方 API 入口

各厂商的密钥页、请求端点、鉴权头与文档。端点是完整请求地址，路径即协议；模型 ID 与推理参数以各家文档为准。

| 厂商 | 密钥页 | 端点 | 鉴权头 | 文档 |
|---|---|---|---|---|
| OpenAI | https://platform.openai.com/api-keys | `https://api.openai.com/v1/responses` | `Authorization: Bearer` | https://developers.openai.com/api/docs |
| Google | https://aistudio.google.com/api-keys | `https://generativelanguage.googleapis.com/v1beta/interactions` | `x-goog-api-key` | https://ai.google.dev/gemini-api/docs |
| Anthropic | https://platform.claude.com/settings/keys | `https://api.anthropic.com/v1/messages` | `x-api-key` | https://platform.claude.com/docs/en/api/overview |
| xAI | https://console.x.ai/team/api-keys | `https://api.x.ai/v1/responses` | `Authorization: Bearer` | https://docs.x.ai/overview |
| Meta | https://dev.meta.ai/api-keys | `https://api.meta.ai/v1/responses` | `Authorization: Bearer` | https://dev.meta.ai/docs/overview |
| DeepSeek | https://platform.deepseek.com/api_keys | `https://api.deepseek.com/chat/completions` | `Authorization: Bearer` | https://api-docs.deepseek.com/ |
| 智谱 | https://bigmodel.cn/apikey/platform | `https://open.bigmodel.cn/api/paas/v4/chat/completions` | `Authorization: Bearer` | https://docs.bigmodel.cn |
| 阿里云 | https://platform.qianwenai.com/home/api-keys | `https://maas.qianwenaiapi.com/compatible-mode/v1/chat/completions` | `Authorization: Bearer` | https://platform.qianwenai.com/docs/developer-guides/getting-started/introduction |
| 腾讯 | https://console.cloud.tencent.com/tokenhub/apikey | `https://tokenhub.tencentmaas.com/v1/responses` | `Authorization: Bearer` | https://cloud.tencent.com/document/product/1823/132252 |
| 小米 | https://platform.xiaomimimo.com/console/api-keys | `https://api.xiaomimimo.com/v1/chat/completions` | `api-key` | https://mimo.mi.com/docs/zh-CN/quick-start/summary/first-api-call |
| 月之暗面 | https://platform.kimi.com/console/api-keys | `https://api.moonshot.cn/v1/chat/completions` | `Authorization: Bearer` | https://platform.kimi.com/docs/overview |
| 字节跳动 | https://ark.volcengine.com/region:cn-beijing/apiKey | `https://ark.cn-beijing.volces.com/api/v3/responses` | `Authorization: Bearer` | https://docs.volcengine.com/docs/ark/compatible-with-openai-sdk |
| MiniMax | https://platform.minimax.cn/console/access | `https://api.minimax.cn/anthropic/v1/messages` | `Authorization: Bearer` | https://platform.minimax.cn/docs |

## 协议

| 协议 | 厂商 |
|---|---|
| OpenAI Responses | OpenAI、xAI、Meta、腾讯、字节跳动 |
| OpenAI Chat Completions | DeepSeek、智谱、阿里云、小米、月之暗面 |
| Anthropic Messages | Anthropic、MiniMax |
| Gemini Interactions | Google |

## 备注

- 未走 OpenAI 兼容接口的三家，依据规范「兼容层功能不全时改用原生 API」：
  - Anthropic：兼容层「is not considered a long-term or production-ready solution」，且「the OpenAI SDK doesn't return Claude's thinking」，`response_format` 被忽略，评审所需的结构化输出只在原生 API 提供。
  - Google：兼容层「is still in beta」，官方「recommend that you call the Gemini API directly」，未列出的参数被静默忽略。
  - MiniMax：官方文档将 Anthropic SDK 接入标为「推荐」。
- 阿里云：千问 AI 平台与百炼的密钥通用。
- 小米：Token Plan 的 Key（`tp-` 开头）用另一组地址，不用于本项目。
- DeepSeek：`deepseek-flash` 是会原地更新的别名，文档未列带日期的快照。
- 月之暗面：`platform.moonshot.cn` 跳转到 `platform.kimi.com`。
