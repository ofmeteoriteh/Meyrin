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

- Python 3 (standard library only), for regenerating the question bank

## Example

Regenerate the machine-readable question bank from the specification:

```bash
python3 build_bank.py docs/SPEC.md data/questions.jsonl
```

```text
wrote 37 records
```

## Documentation

| Document | Purpose |
|---|---|
| [docs/SPEC.md](docs/SPEC.md) | Test specification: prompts, outcome options, style tags, judging, reliability thresholds |
| [docs/api-endpoints.md](docs/api-endpoints.md) | Official API key pages, base URLs, and documentation for each vendor |

## Honest Limitations

- Results describe the API version of a model only. Some vendors serve different model versions in their apps, and the API side may include a content-moderation layer.
- Blinding is incomplete: a response that states its own identity cannot be anonymized.

## License & Disclaimer

Released under the [Apache License 2.0](LICENSE) and the [Creative Commons Attribution-NonCommercial 4.0 International license](LICENSE-DATA), by path:

| Path | License |
|---|---|
| `build_bank.py`, `docs/api-endpoints.md`, `assets/` | Apache-2.0 ([LICENSE](LICENSE)) |
| `docs/SPEC.md`, `data/questions.jsonl` | CC BY-NC 4.0 ([LICENSE-DATA](LICENSE-DATA)) |

This project is developed and maintained by individual contributors on a voluntary, non-commercial basis.

This software is provided "as is", without warranty of any kind, express or implied. The authors are not liable for any damage arising from its use, including but not limited to data loss, system damage, or security incidents. See the [LICENSE](LICENSE) file for the full terms.

`build_bank.py` reads `docs/SPEC.md` and writes `data/questions.jsonl`; it makes no network requests. Running the test procedure in `docs/SPEC.md` sends prompts to third-party model APIs with credentials supplied by the runner, and the responses are third-party content governed by each vendor's terms.

Any commercial entity using this software is solely responsible for its own compliance with applicable laws and regulations, including but not limited to the EU Cyber Resilience Act (CRA) and any other regional requirements.
