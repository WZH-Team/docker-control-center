// 存储卷视图：列表、多选删除、详情。
import { api } from "../api.js";
import { t, errorMessage } from "../i18n.js";
import {
  esc, fmtTime, fmtBytes, toast, openModal, confirmDialog,
  spinner, emptyState, copyText, kv
} from "../ui.js";

export async function renderVolumes(view, ctx) {
  view.innerHTML = `
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <div class="text-sm font-medium text-slate-600" id="vol-count"></div>
      <div class="toolbar ml-auto">
        <label class="flex cursor-pointer items-center gap-2 text-sm text-slate-500">
          <input type="checkbox" id="vol-auto" class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500" checked />
          <span>${esc(t("common.autoRefresh"))}</span>
        </label>
        <input id="vol-search" class="input w-52" type="search" placeholder="${esc(t("common.search"))}" />
        <button type="button" id="vol-del-selected" class="btn btn-danger">${esc(t("volumes.deleteSelected"))}</button>
        <button type="button" id="vol-refresh" class="btn btn-secondary">${esc(t("common.refresh"))}</button>
      </div>
    </div>
    <div class="card overflow-hidden">
      <div id="vol-body">${spinner()}</div>
    </div>`;

  let data = [];
  const selected = new Set();

  async function load() {
    const body = document.getElementById("vol-body");
    if (!body) return;
    body.innerHTML = spinner();
    try {
      const res = await api.get("/api/volumes");
      data = res.volumes || [];
      renderTable();
    } catch (err) {
      body.innerHTML = `<div class="p-8 text-center text-sm text-rose-600">${esc(errorMessage(err))}</div>`;
    }
  }

  function filtered() {
    const q = document.getElementById("vol-search").value.trim().toLowerCase();
    if (!q) return data;
    return data.filter((v) => [v.name, v.driver, v.mountpoint].join(" ").toLowerCase().includes(q));
  }

  function syncSelectionUI() {
    const btn = document.getElementById("vol-del-selected");
    btn.textContent = selected.size > 0
      ? `${t("volumes.deleteSelected")} (${selected.size})`
      : t("volumes.deleteSelected");
    btn.disabled = selected.size === 0;
  }

  function renderTable() {
    const rows = filtered();
    // 自动刷新后保留有效选中项，清理已消失的条目。
    const validNames = new Set(data.map((r) => r.name));
    for (const name of [...selected]) {
      if (!validNames.has(name)) selected.delete(name);
    }
    const allChecked = rows.length > 0 && rows.every((r) => selected.has(r.name));

    const countEl = document.getElementById("vol-count");
    if (countEl) countEl.textContent = t("common.total", { n: rows.length });
    const body = document.getElementById("vol-body");
    if (!body) return;

    if (rows.length === 0) {
      body.innerHTML = emptyState(data.length === 0 ? t("volumes.empty") : t("common.empty"));
      syncSelectionUI();
      return;
    }

    body.innerHTML = `
      <div class="overflow-x-auto">
        <table class="w-full min-w-[980px]">
          <thead class="bg-slate-50">
            <tr>
              <th class="th w-10">
                <input type="checkbox" id="vol-select-all" title="${esc(t("common.selectAll"))}"
                       class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500"
                       ${allChecked ? "checked" : ""} />
              </th>
              <th class="th">${esc(t("common.name"))}</th>
              <th class="th">${esc(t("volumes.driver"))}</th>
              <th class="th">${esc(t("volumes.mountpoint"))}</th>
              <th class="th">${esc(t("volumes.refCount"))}</th>
              <th class="th">${esc(t("common.created"))}</th>
              <th class="th text-right">${esc(t("common.actions"))}</th>
            </tr>
          </thead>
          <tbody>
            ${rows.map(rowHTML).join("")}
          </tbody>
        </table>
      </div>`;

    document.getElementById("vol-select-all").addEventListener("change", (e) => {
      const visible = filtered();
      if (e.target.checked) visible.forEach((r) => selected.add(r.name));
      else visible.forEach((r) => selected.delete(r.name));
      renderTable();
    });
    body.querySelectorAll("[data-check]").forEach((cb) => {
      cb.addEventListener("change", () => {
        if (cb.checked) selected.add(cb.dataset.check);
        else selected.delete(cb.dataset.check);
        syncSelectionUI();
      });
    });
    body.querySelectorAll("[data-detail]").forEach((btn) =>
      btn.addEventListener("click", () => showDetail(btn.dataset.detail)));
    body.querySelectorAll("[data-remove]").forEach((btn) =>
      btn.addEventListener("click", () => removeOne(btn.dataset.remove)));
    body.querySelectorAll("[data-copy]").forEach((btn) =>
      btn.addEventListener("click", () => copyText(btn.dataset.copy)));
    syncSelectionUI();
  }

  function rowHTML(v) {
    return `
      <tr class="tr">
        <td class="td">
          <input type="checkbox" data-check="${esc(v.name)}" class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500"
                 ${selected.has(v.name) ? "checked" : ""} />
        </td>
        <td class="td">
          <button type="button" data-detail="${esc(v.name)}" class="font-medium text-brand-700 hover:underline">${esc(v.name)}</button>
        </td>
        <td class="td"><span class="badge bg-slate-100 text-slate-600">${esc(v.driver)}</span></td>
        <td class="td">
          <button type="button" data-copy="${esc(v.mountpoint)}" class="max-w-xs truncate font-mono text-xs text-slate-500 hover:text-brand-700">${esc(v.mountpoint)}</button>
        </td>
        <td class="td">${esc(v.refCount ?? "-")}</td>
        <td class="td whitespace-nowrap text-xs text-slate-500">${esc(fmtTime(v.createdAt))}</td>
        <td class="td">
          <div class="flex items-center justify-end gap-1">
            <button type="button" data-detail="${esc(v.name)}" class="btn btn-secondary btn-xs">${esc(t("common.details"))}</button>
            <button type="button" data-remove="${esc(v.name)}" class="btn btn-danger btn-xs">${esc(t("common.delete"))}</button>
          </div>
        </td>
      </tr>`;
  }

  async function removeOne(name) {
    const { ok } = await confirmDialog({
      title: t("volumes.deleteConfirmTitle"),
      message: t("volumes.deleteConfirmMsg", { n: 1 }),
      detail: name,
      danger: true,
      confirmText: t("common.delete")
    });
    if (!ok) return;
    try {
      const res = await api.del("/api/volumes", { names: [name], force: false });
      reportResults(res.results);
      load();
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  }

  function reportResults(results) {
    const failed = (results || []).filter((r) => !r.ok);
    const okCount = (results || []).filter((r) => r.ok).length;
    if (okCount > 0) toast(t("volumes.deleted", { n: okCount }), "success");
    if (failed.length > 0) {
      toast(failed.map((f) => `${f.ref}: ${f.error}`).join("; "), "error", 6000);
    }
  }

  async function showDetail(name) {
    const wrap = document.createElement("div");
    wrap.innerHTML = spinner();
    openModal({ title: t("volumes.detail"), body: wrap, size: "max-w-xl" });
    try {
      const res = await api.get(`/api/volumes/detail?name=${encodeURIComponent(name)}`);
      const v = res.volume || {};
      wrap.innerHTML = `
        <dl class="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2">
          ${kv(t("common.name"), `<span class="font-medium">${esc(v.name)}</span>`)}
          ${kv(t("volumes.driver"), esc(v.driver))}
          ${kv(t("volumes.mountpoint"), `<button type="button" data-copy="${esc(v.mountpoint)}" class="break-all font-mono text-xs text-brand-700 hover:underline">${esc(v.mountpoint)}</button>`)}
          ${kv(t("volumes.scope"), esc(v.scope || "-"))}
          ${kv(t("common.created"), esc(v.createdAt || "-"))}
          ${kv(t("common.size"), v.size ? esc(fmtBytes(v.size)) : "-")}
        </dl>
        <div class="mt-5">
          <h4 class="mb-2 text-sm font-semibold text-slate-700">${esc(t("common.details"))}</h4>
          <div class="rounded-lg border border-slate-100 p-3">
            ${Object.entries(v.labels || {}).map(([k, val]) => `
              <div class="flex gap-2 border-b border-slate-100 py-1 text-xs last:border-0">
                <span class="w-48 shrink-0 break-all font-mono text-slate-500">${esc(k)}</span>
                <span class="min-w-0 flex-1 break-all font-mono text-slate-700">${esc(val)}</span>
              </div>`).join("") || `<div class="text-xs text-slate-400">-</div>`}
          </div>
        </div>`;
      wrap.querySelectorAll("[data-copy]").forEach((btn) =>
        btn.addEventListener("click", () => copyText(btn.dataset.copy)));
    } catch (err) {
      wrap.innerHTML = `<p class="text-sm text-rose-600">${esc(errorMessage(err))}</p>`;
    }
  }

  document.getElementById("vol-del-selected").addEventListener("click", async () => {
    const names = [...selected];
    if (names.length === 0) return;
    const { ok, checked } = await confirmDialog({
      title: t("volumes.deleteConfirmTitle"),
      message: t("volumes.deleteConfirmMsg", { n: names.length }),
      danger: true,
      confirmText: t("common.delete"),
      checkboxLabel: t("common.force")
    });
    if (!ok) return;
    try {
      const res = await api.del("/api/volumes", { names, force: checked });
      reportResults(res.results);
      load();
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  });

  document.getElementById("vol-refresh").addEventListener("click", load);
  document.getElementById("vol-search").addEventListener("input", renderTable);

  // 自动刷新（10s），刷新保留选中项。
  let timer = null;
  const setAuto = (on) => {
    if (timer) clearInterval(timer);
    timer = null;
    if (on) timer = setInterval(load, 10000);
  };
  document.getElementById("vol-auto").addEventListener("change", (e) => setAuto(e.target.checked));
  setAuto(true);

  await load();
  return () => { if (timer) clearInterval(timer); };
}
