[![License](https://img.shields.io/badge/license-Apache--2.0%20%7C%20CC%20BY--NC%204.0-lightgrey?style=flat-square)](#license--disclaimer)
![Benchmark](https://img.shields.io/badge/benchmark-LLM%20style-4da3ff?style=flat-square)

English | [中文](README.zh-CN.md)

<p align="center"><img src="assets/meyrin-icon.svg" width="120" alt="Meyrin logo"></p>

# Meyrin

> A black-box benchmark for comparing the response style of LLMs through their official APIs.

## Introduction

Meyrin compares how different models answer, not whether they answer correctly.
The test set has 37 low-difficulty prompts across nine dimensions: format and expression, honesty and accuracy, stance and sycophancy, boundaries and moralizing, emotion and persona, creative writing, multilingual, code, and interaction.

Each prompt is sent verbatim to every model through its official API, with no system prompt, no tools, and no web search.
Responses are labeled by blinded LLM judges into per-question outcome options and 11 style tags, together with program-computed metrics.
Judge agreement is reported as Krippendorff's α (outcomes) and positive specific agreement (tags), each with a bootstrap confidence interval.

## Prerequisites

- Go 1.27+

## Example

Regenerate the machine-readable question bank from the specification:

```bash
go run ./cmd/meyrin bank build
```

```text
wrote 37 questions to data/questions.jsonl
```

## Commands

Run from the repository root; paths default to the repository layout.

| Command | Purpose |
|---|---|
| `go run ./cmd/meyrin bank build` | Regenerate `data/questions.jsonl` from `docs/SPEC.md` |
| `go run ./cmd/meyrin bank check` | Verify that the committed bank matches the spec byte for byte |
| `go run ./cmd/meyrin models check` | Verify `configs/models.yaml` against the spec, `docs/api-endpoints.md`, and `.env.example` |
| `go run ./cmd/meyrin run -round <ROUND> -dry-run` | Print every request without sending it or reading credentials |
| `go run ./cmd/meyrin run -round <ROUND> [-runs 3] [-only q1,q24] [-vendors <A,B>]` | Call the models under test and append results to `results/<ROUND>/`; rerunning resumes |
| `go run ./cmd/meyrin metrics -round <ROUND>` | Compute program metrics for the round's responses into `results/<ROUND>/metrics.jsonl` |
| `go run ./cmd/meyrin judge -round <ROUND> -dry-run` | Print every judge request without sending it or reading credentials |
| `go run ./cmd/meyrin judge -round <ROUND> [-judges <A,B>]` | Have the judge models label the round's answers into `results/<ROUND>/judgments.jsonl`; rerunning resumes |
| `go run ./cmd/meyrin stats -round <ROUND>` | Tally votes and compute reliability, self-preference bias and the review list into `results/<ROUND>/summary.json` |
| `go run ./cmd/meyrin site` | Render every summarized round and the spec into the static site under `_site/` |
| `go test ./...` | Run the unit tests |

## Security

- **Credentials** — API keys for the models under test and separate judge keys live in plaintext in `.env`, which is git-ignored; keys are redacted from logs, errors and recorded requests.
- **Outbound requests** — `meyrin run` and `meyrin judge` call the vendor endpoints in `docs/api-endpoints.md`, the judge endpoints in `.env`, and kernel.org.
- **Untrusted content** — model answers are third-party output; the site renders them without raw HTML and with dangerous links filtered.
- **CI and Pages** — workflows use committed files only and no API keys.

See [SECURITY.md](SECURITY.md) to report a vulnerability.

## Documentation

| Document | Purpose |
|---|---|
| [docs/SPEC.md](docs/SPEC.md) | Test specification: prompts, outcome options, style tags, judging, reliability thresholds |
| [docs/api-endpoints.md](docs/api-endpoints.md) | Official API key pages, base URLs, and documentation for each vendor |
| [docs/architecture.md](docs/architecture.md) | Pipeline, package layout, provider protocols, result files, security model |
| [docs/dev-guide.md](docs/dev-guide.md) | Environment, development order, spec and bank sync, CI |

## Honest Limitations

- Results describe the API version of a model only. Some vendors serve different model versions in their apps, and the API side may include a content-moderation layer.
- Blinding is incomplete: a response that states its own identity cannot be anonymized.

## License & Disclaimer

Released under the [Apache License 2.0](LICENSE) and the [Creative Commons Attribution-NonCommercial 4.0 International license](LICENSE-DATA), by path:

| Path | License |
|---|---|
| `docs/SPEC.md`, `data/questions.jsonl`, `prompts/judge-system.md` | CC BY-NC 4.0 ([LICENSE-DATA](LICENSE-DATA)) |
| `data/dict/STCharacters.txt`, `data/dict/TSCharacters.txt` | Apache-2.0, from OpenCC ([NOTICE](NOTICE)) |
| `data/dict/emoji-test.txt` | Unicode License V3 ([NOTICE](NOTICE)) |
| All other paths | Apache-2.0 ([LICENSE](LICENSE)) |

This project is developed and maintained by individual contributors on a voluntary, non-commercial basis.

This software is provided "as is", without warranty of any kind, express or implied. The authors are not liable for any damage arising from its use, including but not limited to data loss, system damage, or security incidents. See the [LICENSE](LICENSE) file for the full terms.

`meyrin bank`, `meyrin models`, `meyrin metrics`, `meyrin stats` and `meyrin site` read and write repository files only; they make no network requests. `meyrin run` sends the prompts in `data/questions.jsonl` to the vendor APIs configured in `configs/models.yaml`, using keys from `.env`; keys never appear in results or logs, and requests opt out of server-side storage where the API allows it. `meyrin judge` sends those prompts and the collected responses, without vendor or model names, to the judge models configured in `configs/models.yaml`, using the separate judge keys and addresses in `.env`. Running the test procedure in `docs/SPEC.md` sends prompts to third-party model APIs with credentials supplied by the runner, and the responses are third-party content governed by each vendor's terms. `meyrin site` publishes the committed answers as they are.

The authors have no obligation to provide support, updates, or security fixes. To report a vulnerability, follow [SECURITY.md](SECURITY.md).

Any commercial entity using this software is solely responsible for its own compliance with applicable laws and regulations, including but not limited to the EU Cyber Resilience Act (CRA) and any other regional requirements.
