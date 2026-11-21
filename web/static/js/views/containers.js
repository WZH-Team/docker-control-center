// 容器视图：列表、启停重启、删除、更新（latest）、详情。
import { api } from "../api.js";
import { t, errorMessage } from "../i18n.js";
import {
  esc, fmtTime, shortId, toast, openModal, confirmDialog,
  stateBadge, spinner, emptyState, copyText, kv
} from "../ui.js";
import { openJobModal } from "../jobs.js";

export async function renderContainers(view, ctx) {
  view.innerHTML = `
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <div class="text-sm font-medium text-slate-600" id="ctr-count"></div>
      <div class="toolbar ml-auto">
        <label class="flex cursor-pointer items-center gap-2 text-sm text-slate-500">
          <input type="checkbox" id="ctr-auto" class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500" checked />
          <span>${esc(t("common.autoRefresh"))}</span>
        </label>
        <input id="ctr-search" class="input w-52" type="search" placeholder="${esc(t("common.search"))}" />
        <button type="button" id="ctr-refresh" class="btn btn-secondary">
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182"/></svg>
          ${esc(t("common.refresh"))}
        </button>
      </div>
    </div>
    <div class="card overflow-hidden">
      <div id="ctr-body">${spinner()}</div>
    </div>`;

  let data = [];

  async function load() {
    const body = document.getElementById("ctr-body");
    if (!body) return;
    body.innerHTML = spinner();
    try {
      const res = await api.get("/api/containers");
      data = res.containers || [];
      renderTable();
    } catch (err) {
      body.innerHTML = `<div class="p-8 text-center text-sm text-rose-600">${esc(errorMessage(err))}</div>`;
    }
  }

  function filtered() {
    const q = document.getElementById("ctr-search").value.trim().toLowerCase();
    if (!q) return data;
    return data.filter((c) =>
      [c.name, c.image, c.id, c.state, c.status].join(" ").toLowerCase().includes(q)
    );
  }

  function renderTable() {
    const rows = filtered();
    const countEl = document.getElementById("ctr-count");
    if (countEl) countEl.textContent = t("common.total", { n: rows.length });
    const body = document.getElementById("ctr-body");
    if (!body) return;

    if (rows.length === 0) {
      body.innerHTML = emptyState(data.length === 0 ? t("containers.empty") : t("common.empty"));
      return;
    }

    body.innerHTML = `
      <div class="overflow-x-auto">
        <table class="w-full min-w-[980px]">
          <thead class="bg-slate-50">
            <tr>
              <th class="th">${esc(t("common.name"))}</th>
              <th class="th">${esc(t("common.id"))}</th>
              <th class="th">${esc(t("containers.image"))}</th>
              <th class="th">${esc(t("common.status"))}</th>
              <th class="th">${esc(t("common.created"))}</th>
              <th class="th text-right">${esc(t("common.actions"))}</th>
            </tr>
          </thead>
          <tbody>
            ${rows.map(rowHTML).join("")}
          </tbody>
        </table>
      </div>`;

    body.querySelectorAll("[data-act]").forEach((btn) => {
      btn.addEventListener("click", () => handleAction(btn.dataset.act, btn.dataset.id, btn.dataset.name, btn.dataset.image));
    });
    body.querySelectorAll("[data-detail]").forEach((btn) => {
      btn.addEventListener("click", () => showDetail(btn.dataset.detail));
    });
    body.querySelectorAll("[data-copy]").forEach((btn) => {
      btn.addEventListener("click", () => copyText(btn.dataset.copy));
    });
  }

  function rowHTML(c) {
    const running = c.state === "running";
    return `
      <tr class="tr">
        <td class="td">
          <div class="flex items-center gap-2">
            <button type="button" data-detail="${esc(c.id)}" class="font-medium text-brand-700 hover:underline">${esc(c.name)}</button>
            ${c.composeProject ? `<span class="badge bg-cyan-100 text-cyan-700">${esc(c.composeProject)}</span>` : ""}
            ${c.isLatest ? `<span class="badge bg-brand-100 text-brand-700">latest</span>` : ""}
          </div>
        </td>
        <td class="td">
          <button type="button" data-copy="${esc(c.id)}" class="font-mono text-xs text-slate-500 hover:text-brand-700" title="${esc(t("common.copy"))}">${esc(c.idShort)}</button>
        </td>
        <td class="td"><span class="font-mono text-xs text-slate-600">${esc(c.image)}</span></td>
        <td class="td">
          ${stateBadge(c.state)}
          <div class="mt-1 whitespace-nowrap text-xs text-slate-400">${esc(c.status)}</div>
        </td>
        <td class="td whitespace-nowrap text-xs text-slate-500">${esc(fmtTime(c.created))}</td>
        <td class="td">
          <div class="flex items-center justify-end gap-1">
            ${running
              ? `<button type="button" data-act="stop" data-id="${esc(c.id)}" data-name="${esc(c.name)}" class="btn btn-secondary btn-xs">${esc(t("containers.stop"))}</button>
                 <button type="button" data-act="restart" data-id="${esc(c.id)}" data-name="${esc(c.name)}" class="btn btn-secondary btn-xs">${esc(t("containers.restart"))}</button>`
              : `<button type="button" data-act="start" data-id="${esc(c.id)}" data-name="${esc(c.name)}" class="btn btn-primary btn-xs">${esc(t("containers.start"))}</button>`}
            ${c.isLatest ? `<button type="button" data-act="update" data-id="${esc(c.id)}" data-name="${esc(c.name)}" data-image="${esc(c.image)}" class="btn btn-secondary btn-xs">${esc(t("containers.update"))}</button>` : ""}
            <button type="button" data-act="remove" data-id="${esc(c.id)}" data-name="${esc(c.name)}" class="btn btn-danger btn-xs">${esc(t("containers.remove"))}</button>
          </div>
        </td>
      </tr>`;
  }

  async function handleAction(act, id, name, image) {
    try {
      if (act === "remove") {
        const { ok } = await confirmDialog({
          title: t("containers.removeConfirmTitle"),
          message: t("containers.removeConfirmMsg", { name }),
          danger: true,
          confirmText: t("containers.remove")
        });
        if (!ok) return;
        await api.del(`/api/containers/${encodeURIComponent(id)}?force=true`);
        toast(t("common.success"), "success");
        load();
        return;
      }

      if (act === "update") {
        const { ok } = await confirmDialog({
          title: t("containers.updateConfirmTitle"),
          message: t("containers.updateConfirmMsg", { image }),
          detail: `docker pull ${image}`,
          confirmText: t("containers.update")
        });
        if (!ok) return;
        const res = await api.post(`/api/containers/${encodeURIComponent(id)}/update`);
        toast(t("containers.updated"), "success");
        openJobModal(res.jobId, { title: `${t("containers.update")} · ${name}`, onDone: load });
        return;
      }

      await api.post(`/api/containers/${encodeURIComponent(id)}/${act}`);
      toast(t("common.success"), "success");
      load();
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  }

  async function showDetail(id) {
    const wrap = document.createElement("div");
    wrap.innerHTML = spinner();
    const { close } = openModal({ title: t("containers.detail"), body: wrap, size: "max-w-2xl" });
    try {
      const d = await api.get(`/api/containers/${encodeURIComponent(id)}`);
      wrap.innerHTML = detailHTML(d);
      wrap.querySelectorAll("[data-copy]").forEach((btn) =>
        btn.addEventListener("click", () => copyText(btn.dataset.copy)));
    } catch (err) {
      wrap.innerHTML = `<p class="text-sm text-rose-600">${esc(errorMessage(err))}</p>`;
    }
    return close;
  }

  function detailHTML(d) {
    const mapRows = (obj) => Object.entries(obj || {})
      .map(([k, v]) => `<div class="flex gap-2 border-b border-slate-100 py-1 text-xs last:border-0">
          <span class="w-40 shrink-0 break-all font-mono text-slate-500">${esc(k)}</span>
          <span class="min-w-0 flex-1 break-all font-mono text-slate-700">${esc(v)}</span>
        </div>`)
      .join("") || `<div class="text-xs text-slate-400">-</div>`;

    return `
      <dl class="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2">
        ${kv(t("common.name"), `<span class="font-medium">${esc(d.name)}</span>`)}
        ${kv(t("common.id"), `<button type="button" data-copy="${esc(d.id)}" class="font-mono text-xs text-brand-700 hover:underline">${esc(d.id)}</button>`)}
        ${kv(t("containers.image"), esc(d.image))}
        ${kv(t("containers.imageId"), `<span class="font-mono text-xs">${esc(shortId(d.imageId))}</span>`)}
        ${kv(t("common.status"), stateBadge(d.state) + ` <span class="text-xs text-slate-400">${esc(d.status)}</span>`)}
        ${kv(t("common.created"), esc(fmtTime(d.created)))}
        ${kv(t("containers.command"), `<span class="font-mono text-xs">${esc(d.command || "-")}</span>`)}
        ${kv(t("containers.restartPolicy"), esc(d.restartPolicy || "-"))}
        ${kv(t("containers.startedAt"), esc(fmtTime(d.startedAt)))}
        ${kv(t("containers.finishedAt"), esc(fmtTime(d.finishedAt)))}
        ${kv(t("containers.exitCode"), esc(d.exitCode ?? 0))}
        ${kv(t("containers.restartCount"), esc(d.restartCount ?? 0))}
        ${kv(t("containers.workingDir"), `<span class="font-mono text-xs">${esc(d.workingDir || "-")}</span>`)}
        ${kv(t("containers.user"), esc(d.user || "-"))}
        ${kv(t("containers.platform"), esc(d.platform || "-"))}
      </dl>

      <div class="mt-5">
        <h4 class="mb-2 text-sm font-semibold text-slate-700">${esc(t("containers.ports"))}</h4>
        ${(d.ports || []).length
          ? `<div class="space-y-1">${d.ports.map((p) => `
              <div class="rounded-lg bg-slate-50 px-3 py-1.5 font-mono text-xs text-slate-600">
                ${esc(p.ip || "0.0.0.0")}:${esc(p.publicPort || "-")} → ${esc(p.privatePort)}/${esc(p.type)}
              </div>`).join("")}</div>`
          : `<div class="text-xs text-slate-400">-</div>`}
      </div>

      <div class="mt-5">
        <h4 class="mb-2 text-sm font-semibold text-slate-700">${esc(t("containers.mounts"))}</h4>
        ${(d.mounts || []).length
          ? `<div class="space-y-1">${d.mounts.map((m) => `
              <div class="rounded-lg bg-slate-50 px-3 py-1.5 font-mono text-xs text-slate-600">
                <span class="badge bg-slate-200 text-slate-600 mr-2">${esc(m.type)}</span>
                ${esc(m.name || m.source)} → ${esc(m.destination)} ${m.rw ? "" : "(ro)"}
              </div>`).join("")}</div>`
          : `<div class="text-xs text-slate-400">-</div>`}
      </div>

      <div class="mt-5">
        <h4 class="mb-2 text-sm font-semibold text-slate-700">${esc(t("containers.networks"))}</h4>
        ${(d.networks || []).length
          ? `<div class="space-y-1">${d.networks.map((n) => `
              <div class="rounded-lg bg-slate-50 px-3 py-1.5 font-mono text-xs text-slate-600">
                ${esc(n.name)} · ${esc(n.ip || "-")} ${n.aliases && n.aliases.length ? `· ${esc(n.aliases.join(", "))}` : ""}
              </div>`).join("")}</div>`
          : `<div class="text-xs text-slate-400">-</div>`}
      </div>

      <details class="mt-5">
        <summary class="cursor-pointer text-sm font-semibold text-slate-700">${esc(t("containers.env"))}</summary>
        <div class="mt-2 rounded-lg border border-slate-100 p-2">
          ${(d.env || []).length ? mapRows(Object.fromEntries(d.env.map((e) => {
            const i = e.indexOf("=");
            return i > 0 ? [e.slice(0, i), e.slice(i + 1)] : [e, ""];
          }))) : `<div class="text-xs text-slate-400">${esc(t("containers.noEnv"))}</div>`}
        </div>
      </details>

      <details class="mt-3">
        <summary class="cursor-pointer text-sm font-semibold text-slate-700">${esc(t("containers.labels"))}</summary>
        <div class="mt-2 rounded-lg border border-slate-100 p-2">${mapRows(d.labels)}</div>
      </details>`;
  }

  document.getElementById("ctr-refresh").addEventListener("click", load);
  document.getElementById("ctr-search").addEventListener("input", renderTable);

  // 自动刷新（10s），与主流 Docker 面板习惯一致。
  let timer = null;
  const setAuto = (on) => {
    if (timer) clearInterval(timer);
    timer = null;
    if (on) timer = setInterval(load, 10000);
  };
  document.getElementById("ctr-auto").addEventListener("change", (e) => setAuto(e.target.checked));
  setAuto(true);

  await load();
  return () => { if (timer) clearInterval(timer); };
}
