// 表格排序与筛选；页面在脚本不可用时照常显示 / Table sorting and filtering; pages work without the script
"use strict";

document.querySelectorAll("th[data-sort]").forEach((th) => {
  th.addEventListener("click", () => {
    const table = th.closest("table");
    const body = table.tBodies[0];
    const index = Array.from(th.parentNode.children).indexOf(th);
    const numeric = th.dataset.sort === "num";
    const ascending = th.dataset.dir !== "asc";
    th.dataset.dir = ascending ? "asc" : "desc";
    const key = (row) => {
      const text = row.children[index] ? row.children[index].textContent.trim() : "";
      const n = parseFloat(text);
      return numeric ? (Number.isNaN(n) ? -Infinity : n) : text;
    };
    const rows = Array.from(body.rows);
    rows.sort((a, b) => {
      const x = key(a);
      const y = key(b);
      const order = numeric ? x - y : String(x).localeCompare(String(y), "zh-CN");
      return ascending ? order : -order;
    });
    rows.forEach((row) => body.appendChild(row));
  });
});

document.querySelectorAll("input[data-filter]").forEach((input) => {
  const table = document.getElementById(input.dataset.filter);
  if (!table) {
    return;
  }
  input.addEventListener("input", () => {
    const query = input.value.trim().toLowerCase();
    Array.from(table.tBodies[0].rows).forEach((row) => {
      row.hidden = query !== "" && !row.textContent.toLowerCase().includes(query);
    });
  });
});
