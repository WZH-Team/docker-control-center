// 闀滃儚瑙嗗浘锛氬垪琛ㄣ€佸閫夊垹闄ゃ€佹媺鍙栥€佹洿鏂般€佽鎯呫€?
import { api } from "../api.js";
import { t, errorMessage } from "../i18n.js";
import {
  esc, fmtTime, fmtBytes, shortId, toast, openModal, confirmDialog, promptDialog,
  spinner, emptyState, copyText, kv
} from "../ui.js";
import { openJobModal } from "../jobs.js";

export async function renderImages(view, ctx) {
  view.innerHTML = `
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <div class="text-sm font-medium text-slate-600" id="img-count"></div>
      <div class="toolbar ml-auto">
        <label class="flex cursor-pointer items-center gap-2 text-sm text-slate-500">
          <input type="checkbox" id="img-auto" class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500" checked />
          <span>${esc(t("common.autoRefresh"))}</span>
        </label>
        <input id="img-search" class="input w-52" type="search" placeholder="${esc(t("common.search"))}" />
        <button type="button" id="img-pull" class="btn btn-primary">
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5M16.5 12L12 16.5m0 0L7.5 12m4.5 4.5V3"/></svg>
          ${esc(t("images.pull"))}
        </button>
        <button type="button" id="img-del-selected" class="btn btn-danger">
          ${esc(t("images.deleteSelected"))}
        </button>
        <button type="button" id="img-refresh" class="btn btn-secondary">
          ${esc(t("common.refresh"))}
        </button>
      </div>
    </div>
    <div class="card overflow-hidden">
      <div id="img-body">${spinner()}</div>
    </div>`;

  let data = [];
  const selected = new Set();

  async function load() {
    const body = document.getElementById("img-body");
    if (!body) return;
    body.innerHTML = spinner();
    try {
      const res = await api.get("/api/images");
      data = res.images || [];
      renderTable();
    } catch (err) {
      body.innerHTML = `<div class="p-8 text-center text-sm text-rose-600">${esc(errorMessage(err))}</div>`;
    }
  }

  function filtered() {
    const q = document.getElementById("img-search").value.trim().toLowerCase();
    if (!q) return data;
    return data.filter((img) =>
      [img.id, ...(img.repoTags || [])].join(" ").toLowerCase().includes(q)
    );
  }

  function syncSelectionUI() {
    const btn = document.getElementById("img-del-selected");
    btn.textContent = selected.size > 0
      ? `${t("images.deleteSelected")} (${selected.size})`
      : t("images.deleteSelected");
    btn.disabled = selected.size === 0;
  }

  function renderTable() {
    const rows = filtered();
    // 鑷姩鍒锋柊鍚庝繚鐣欐湁鏁堥€変腑椤癸紝娓呯悊宸叉秷澶辩殑鏉＄洰銆?
    const validIds = new Set(data.map((r) => r.id));
    for (const id of [...selected]) {
      if (!validIds.has(id)) selected.delete(id);
    }
    const allChecked = rows.length > 0 && rows.every((r) => selected.has(r.id));

    const countEl = document.getElementById("img-count");
    if (countEl) countEl.textContent = t("common.total", { n: rows.length });
    const body = document.getElementById("img-body");
    if (!body) return;

    if (rows.length === 0) {
      body.innerHTML = emptyState(data.length === 0 ? t("images.empty") : t("common.empty"));
      syncSelectionUI();
      return;
    }

    body.innerHTML = `
      <div class="overflow-x-auto">
        <table class="w-full min-w-[980px]">
          <thead class="bg-slate-50">
            <tr>
              <th class="th w-10">
                <input type="checkbox" id="img-select-all" title="${esc(t("common.selectAll"))}"
                       class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500"
                       ${allChecked ? "checked" : ""} />
              </th>
              <th class="th">${esc(t("images.repoTags"))}</th>
              <th class="th">${esc(t("common.id"))}</th>
              <th class="th">${esc(t("common.size"))}</th>
              <th class="th">${esc(t("common.created"))}</th>
              <th class="th">${esc(t("images.inUse"))}</th>
              <th class="th text-right">${esc(t("common.actions"))}</th>
            </tr>
          </thead>
          <tbody>
            ${rows.map(rowHTML).join("")}
          </tbody>
        </table>
      </div>`;

    document.getElementById("img-select-all").addEventListener("change", (e) => {
      const visible = filtered();
      if (e.target.checked) visible.forEach((r) => selected.add(r.id));
      else visible.forEach((r) => selected.delete(r.id));
      renderTable();
    });
    body.querySelectorAll("[data-check]").forEach((cb) => {
      cb.addEventListener("change", () => {
        if (cb.checked) selected.add(cb.dataset.check);
        else selected.delete(cb.dataset.check);
        syncSelectionUI();
      });
    });
    body.querySelectorAll("[data-act]").forEach((btn) => {
      btn.addEventListener("click", () => handleAction(btn.dataset.act, btn.dataset.ref, btn.dataset.id));
    });
    body.querySelectorAll("[data-detail]").forEach((btn) => {
      btn.addEventListener("click", () => showDetail(btn.dataset.detail));
    });
    body.querySelectorAll("[data-copy]").forEach((btn) => {
      btn.addEventListener("click", () => copyText(btn.dataset.copy));
    });
    syncSelectionUI();
  }

  function rowHTML(img) {
    const ref = (img.repoTags && img.repoTags[0]) || img.id;
    const tags = img.repoTags || [];
    const inUse = (img.inUseBy || []).length;
    return `
      <tr class="tr">
        <td class="td">
          <input type="checkbox" data-check="${esc(img.id)}" class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500"
                 ${selected.has(img.id) ? "checked" : ""} />
        </td>
        <td class="td">
          ${tags.length
            ? tags.map((tag) => `<div class="font-mono text-xs text-slate-700">${esc(tag)}</div>`).join("")
            : `<span class="badge bg-amber-100 text-amber-700">${esc(t("images.dangling"))}</span>`}
        </td>
        <td class="td">
          <button type="button" data-copy="${esc(img.id)}" class="font-mono text-xs text-slate-500 hover:text-brand-700">${esc(img.idShort)}</button>
        </td>
        <td class="td whitespace-nowrap">${esc(fmtBytes(img.size))}</td>
        <td class="td whitespace-nowrap text-xs text-slate-500">${esc(fmtTime(img.created))}</td>
        <td class="td">
          ${inUse
            ? `<span class="badge bg-rose-100 text-rose-700">${esc(t("images.inUse"))} 路 ${inUse}</span>`
            : `<span class="badge bg-slate-100 text-slate-500">${esc(t("images.unused"))}</span>`}
        </td>
        <td class="td">
          <div class="flex items-center justify-end gap-1">
            <button type="button" data-detail="${esc(img.id)}" class="btn btn-secondary btn-xs">${esc(t("common.details"))}</button>
            ${tags.length ? `<button type="button" data-act="update" data-ref="${esc(ref)}" class="btn btn-secondary btn-xs">${esc(t("images.update"))}</button>` : ""}
            <button type="button" data-act="remove" data-ref="${esc(img.id)}" class="btn btn-danger btn-xs">${esc(t("images.delete"))}</button>
          </div>
        </td>
      </tr>`;
  }

  async function handleAction(act, ref, id) {
    try {
      if (act === "remove") {
        const { ok, checked } = await confirmDialog({
          title: t("images.deleteConfirmTitle"),
          message: t("images.deleteConfirmMsg", { n: 1 }),
          detail: ref,
          danger: true,
          confirmText: t("images.delete"),
          checkboxLabel: t("common.force")
        });
        if (!ok) return;
        const res = await api.del("/api/images", { refs: [ref], force: checked });
        reportDeleteResults(res.results);
        load();
        return;
      }

      if (act === "update") {
        const { ok } = await confirmDialog({
          title: t("images.updateConfirmTitle"),
          message: t("images.updateConfirmMsg", { ref }),
          detail: `docker pull ${ref}`,
          confirmText: t("images.update")
        });
        if (!ok) return;
        const res = await api.post("/api/images/update", { ref });
        toast(t("images.updateStarted"), "success");
        openJobModal(res.jobId, { title: `${t("images.update")} 路 ${ref}`, onDone: load });
      }
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  }

  function reportDeleteResults(results) {
    const failed = (results || []).filter((r) => !r.ok);
    const okCount = (results || []).filter((r) => r.ok).length;
    if (okCount > 0) toast(t("images.deleted", { n: okCount }), "success");
    if (failed.length > 0) {
      toast(`${t("images.deletePartial")}: ${failed.map((f) => f.ref).join(", ")}`, "error", 6000);
    }
  }

  async function showDetail(id) {
    const wrap = document.createElement("div");
    wrap.innerHTML = spinner();
    openModal({ title: t("common.details"), body: wrap, size: "max-w-xl" });
    try {
      const d = await api.get(`/api/images/detail?ref=${encodeURIComponent(id)}`);
      wrap.innerHTML = `
        <dl class="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2">
          ${kv(t("common.id"), `<button type="button" data-copy="${esc(d.id)}" class="font-mono text-xs text-brand-700 hover:underline">${esc(d.id)}</button>`)}
          ${kv(t("common.size"), esc(fmtBytes(d.size)))}
          ${kv(t("images.architecture"), esc(d.architecture || "-"))}
          ${kv(t("images.os"), esc(d.os || "-"))}
          ${kv(t("images.author"), esc(d.author || "-"))}
          ${kv(t("images.layers"), esc(d.layers ?? "-"))}
          ${kv(t("common.created"), esc(d.created || "-"))}
          ${kv(t("images.comment"), esc(d.comment || "-"))}
        </dl>
        <div class="mt-5">
          <h4 class="mb-2 text-sm font-semibold text-slate-700">${esc(t("images.repoTags"))}</h4>
          <div class="flex flex-wrap gap-1.5">${(d.repoTags || []).length
            ? d.repoTags.map((x) => `<span class="badge bg-brand-100 text-brand-800">${esc(x)}</span>`).join("")
            : `<span class="text-xs text-slate-400">-</span>`}</div>
        </div>
        <div class="mt-4">
          <h4 class="mb-2 text-sm font-semibold text-slate-700">${esc(t("images.repoDigests"))}</h4>
          <div class="space-y-1">${(d.repoDigests || []).length
            ? d.repoDigests.map((x) => `<div class="break-all font-mono text-xs text-slate-600">${esc(x)}</div>`).join("")
            : `<span class="text-xs text-slate-400">-</span>`}</div>
        </div>
        <div class="mt-4">
          <h4 class="mb-2 text-sm font-semibold text-slate-700">${esc(t("images.inUseBy"))}</h4>
          ${(d.inUseBy || []).length
            ? d.inUseBy.map((c) => `<span class="badge bg-slate-100 text-slate-600 mr-1.5">${esc(c.name)}</span>`).join("")
            : `<span class="text-xs text-slate-400">${esc(t("images.unused"))}</span>`}
        </div>`;
      wrap.querySelectorAll("[data-copy]").forEach((btn) =>
        btn.addEventListener("click", () => copyText(btn.dataset.copy)));
    } catch (err) {
      wrap.innerHTML = `<p class="text-sm text-rose-600">${esc(errorMessage(err))}</p>`;
    }
  }

  // 鎷夊彇闀滃儚銆?
  document.getElementById("img-pull").addEventListener("click", async () => {
    const ref = await promptDialog({
      title: t("images.pullTitle"),
      placeholder: t("images.pullPlaceholder"),
      confirmText: t("images.pull"),
      validate: (v) => (v ? null : t("common.required"))
    });
    if (!ref) return;
    try {
      const res = await api.post("/api/images/pull", { ref });
      toast(t("images.pullStarted"), "success");
      openJobModal(res.jobId, { title: `${t("images.pull")} 路 ${ref}`, onDone: load });
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  });

  // 澶氶€夊垹闄ゃ€?
  document.getElementById("img-del-selected").addEventListener("click", async () => {
    const refs = [...selected];
    if (refs.length === 0) return;
    const { ok, checked } = await confirmDialog({
      title: t("images.deleteConfirmTitle"),
      message: t("images.deleteConfirmMsg", { n: refs.length }),
      danger: true,
      confirmText: t("images.delete"),
      checkboxLabel: t("common.force")
    });
    if (!ok) return;
    try {
      const res = await api.del("/api/images", { refs, force: checked });
      reportDeleteResults(res.results);
      load();
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  });

  document.getElementById("img-refresh").addEventListener("click", load);
  document.getElementById("img-search").addEventListener("input", renderTable);

  // 鑷姩鍒锋柊锛?0s锛夛紝鍒锋柊淇濈暀閫変腑椤广€?
  let timer = null;
  const setAuto = (on) => {
    if (timer) clearInterval(timer);
    timer = null;
    if (on) timer = setInterval(load, 10000);
  };
  document.getElementById("img-auto").addEventListener("change", (e) => setAuto(e.target.checked));
  setAuto(true);

  await load();
  return () => { if (timer) clearInterval(timer); };
}
