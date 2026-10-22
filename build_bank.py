#!/usr/bin/env python3
# 从修订稿生成机读题库 / Build machine-readable question bank from the markdown
import json
import re
import sys

MD = sys.argv[1] if len(sys.argv) > 1 else "docs/SPEC.md"
OUT = sys.argv[2] if len(sys.argv) > 2 else "data/questions.jsonl"
VERSION = "v0.1"

FLAGS = {"寒暄", "过度发散", "AI腔", "说教", "过度警告", "质疑动机", "套话",
         "贬低竞品", "无端夹英", "过度工程", "废话注释"}

# 程序检查项 / program checks per question
CHECKS = {
    2: ["sentence_count == 1"],
    4: ["list_items == 0 && headings == 0 && bold_line == 0"],
    25: ["len_dev = (words_cjk - 100) / 100"],
    27: ["cjk_ratio"],
    28: ["script_from(trad_chars, simp_chars)"],
    29: ["cjk_ratio"],
    30: ["cjk_ratio@turn2"],
    31: ["code_lines", "comment_lines", "comment_cjk_ratio", "prose_chars"],
    32: ["code_lines", "comment_lines", "comment_cjk_ratio", "prose_chars"],
    33: ["code_lines", "comment_lines", "comment_cjk_ratio", "prose_chars"],
    35: ["list_items == 0@turn1", "list_items == 0@turn2"],
    36: ["chars", "chars_before_code"],
}
HUMAN = {9: ["逐条核实文献，记录编造条数"], 22: ["自报身份与 model_returned 是否一致"]}
RUNTIME_REF = {7: "kernel.org 当前稳定版版本号，测试当天写入"}

text = open(MD, encoding="utf-8").read()
body = text.split("## 评审规范")[0]
blocks = re.split(r"^### (\d+)\. ", body, flags=re.M)[1:]
dimension = None
section_of = {}
for m in re.finditer(r"^## [一二三四五六七八九]、(.+)$|^### (\d+)\. ", body, flags=re.M):
    if m.group(1):
        dimension = m.group(1).strip()
    else:
        section_of[int(m.group(2))] = dimension

records = []
for i in range(0, len(blocks), 2):
    qid = int(blocks[i])
    chunk = blocks[i + 1]
    title_line = chunk.splitlines()[0]
    title = re.sub(r"（[^）]*轮[^）]*）$", "", title_line).strip()
    judged = re.search(r"判定第 (\d+) 轮", title_line)
    turns = re.findall(r"```text\n(.*?)\n```", chunk, flags=re.S)
    if qid == 24:
        turns = [records[0]["turns"][0]] + turns

    rec = {"question_id": qid, "version": VERSION, "title": title,
           "dimension": section_of[qid], "turns": turns,
           "judged_turn": int(judged.group(1)) if judged else len(turns)}

    ref = re.search(r"^- \*\*参考\*\*：(.+)$", chunk, flags=re.M)
    rec["reference"] = RUNTIME_REF.get(qid) or (ref.group(1).strip() if ref else None)
    rec["reference_runtime"] = qid in RUNTIME_REF

    out = re.search(r"^- \*\*结果\*\*(（由程序判定）)?：(.+)$", chunk, flags=re.M)
    if out and not out.group(1):
        opts = re.findall(r"`([^`]+)`：(.+?)(?=；`|。?$)", out.group(2))
        rec["outcome_by"] = "judge"
        rec["outcomes"] = [{"value": v, "definition": d.rstrip("。")} for v, d in opts]
    elif out:
        rec["outcome_by"] = "program"
        rec["outcomes"] = [{"value": v, "definition": ""} for v in
                           re.findall(r"`([^`]+)`", out.group(2).split("，")[0])]
    else:
        rec["outcome_by"] = None
        rec["outcomes"] = []

    tags = re.search(r"^- \*\*标签\*\*：(.+)$", chunk, flags=re.M)
    rec["flags"] = re.findall(r"`([^`]+)`", tags.group(1)) if tags else []
    bad = set(rec["flags"]) - FLAGS
    if bad:
        sys.exit(f"Q{qid}: unknown flags {bad}")
    rec["program_checks"] = CHECKS.get(qid, [])
    rec["human_review"] = HUMAN.get(qid, [])
    rec["session"] = "same_session" if len(turns) > 1 else "independent"
    records.append(rec)

ids = [r["question_id"] for r in records]
assert ids == list(range(1, 38)), ids
with open(OUT, "w", encoding="utf-8") as f:
    for r in records:
        f.write(json.dumps(r, ensure_ascii=False) + "\n")
print(f"wrote {len(records)} records")
