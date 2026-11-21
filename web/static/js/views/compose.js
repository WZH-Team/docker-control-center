// Compose 项目视图：导入、启停重启、更新（含旧镜像清理）、移除。
import { api } from "../api.js";
import { t, errorMessage } from "../i18n.js";
import {
  esc, toast, confirmDialog, promptDialog, spinner, emptyState, stateBadge
} from "../ui.js";
import { openJobModal } from "../jobs.js";

export async function renderCompose(view, ctx) {
  view.innerHTML = `
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <div class="toolbar ml-auto">
        <label class="flex cursor-pointer items-center gap-2 text-sm text-slate-500">
          <input type="checkbox" id="compose-auto" class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500" checked />
          <span>${esc(t("common.autoRefresh"))}</span>
        </label>
        <button type="button" id="compose-import" class="btn btn-primary">
          <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15"/></svg>
          ${esc(t("compose.import"))}
        </button>
        <button type="button" id="compose-refresh" class="btn btn-secondary">${esc(t("common.refresh"))}</button>
      </div>
    </div>
    <div id="compose-body">${spinner()}</div>`;

  async function load() {
    const body = document.getElementById("compose-body");
    if (!body) return;
    body.innerHTML = spinner();
    try {
      const res = await api.get("/api/compose/projects");
      const projects = res.projects || [];
      if (projects.length === 0) {
        body.innerHTML = `
          <div class="card">
            <div class="flex flex-col items-center gap-3 py-16">
              ${emptyState(t("compose.empty"))}
              <button type="button" id="compose-import-empty" class="btn btn-primary">${esc(t("compose.import"))}</button>
            </div>
          </div>`;
        document.getElementById("compose-import-empty").addEventListener("click", importProject);
        return;
      }
      body.innerHTML = `<div class="grid grid-cols-1 gap-4 xl:grid-cols-2">${projects.map(cardHTML).join("")}</div>`;
      bindCardEvents();
    } catch (err) {
      body.innerHTML = `<div class="p-8 text-center text-sm text-rose-600">${esc(errorMessage(err))}</div>`;
    }
  }

  function cardHTML(p) {
    return `
      <div class="card flex flex-col">
        <div class="card-header">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="truncate text-base font-semibold text-slate-800">${esc(p.name)}</span>
              ${p.valid
                ? `<span class="badge bg-emerald-100 text-emerald-700">${esc(t("common.success"))}</span>`
                : `<span class="badge bg-rose-100 text-rose-700">${esc(t("compose.invalid"))}</span>`}
            </div>
            <div class="mt-1 truncate font-mono text-xs text-slate-400" title="${esc(p.absPath)}">${esc(p.absPath)}</div>
            ${!p.valid && p.error ? `<div class="mt-1 text-xs text-rose-600">${esc(p.error)}</div>` : ""}
          </div>
          <div class="flex shrink-0 items-center gap-1.5">
            <span class="badge bg-brand-50 text-brand-700">${esc(t("common.running"))}: ${esc(p.running)}</span>
            <span class="badge bg-slate-100 text-slate-600">${esc(t("common.stopped"))}: ${esc(p.stopped)}</span>
          </div>
        </div>

        <div class="flex-1 px-5 py-4">
          <h4 class="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-400">${esc(t("compose.services"))}</h4>
          ${(p.containers || []).length
            ? `<div class="space-y-1.5">${p.containers.map((c) => `
                <div class="flex items-center justify-between rounded-lg bg-slate-50 px-3 py-2">
                  <span class="truncate font-mono text-xs text-slate-700">${esc(c.name)}</span>
                  ${stateBadge(c.state)}
                </div>`).join("")}</div>`
            : `<div class="rounded-lg bg-slate-50 px-3 py-4 text-center text-xs text-slate-400">${esc(t("common.empty"))}</div>`}
        </div>

        <div class="flex flex-wrap items-center gap-2 border-t border-slate-100 px-5 py-4">
          <button type="button" data-action="start" data-path="${esc(p.absPath)}" class="btn btn-primary btn-xs">${esc(t("compose.start"))}</button>
          <button type="button" data-action="stop" data-path="${esc(p.absPath)}" class="btn btn-secondary btn-xs">${esc(t("compose.stop"))}</button>
          <button type="button" data-action="restart" data-path="${esc(p.absPath)}" class="btn btn-secondary btn-xs">${esc(t("compose.restart"))}</button>
          <button type="button" data-action="update" data-path="${esc(p.absPath)}" data-name="${esc(p.name)}" class="btn btn-secondary btn-xs">${esc(t("compose.update"))}</button>
          <div class="ml-auto">
            <button type="button" data-remove="${esc(p.absPath)}" data-name="${esc(p.name)}" class="btn btn-ghost btn-xs text-rose-600 hover:bg-rose-50">${esc(t("compose.remove"))}</button>
          </div>
        </div>
      </div>`;
  }

  function bindCardEvents() {
    document.querySelectorAll("#compose-body [data-action]").forEach((btn) => {
      btn.addEventListener("click", () => runAction(btn.dataset.action, btn.dataset.path, btn.dataset.name || btn.dataset.path));
    });
    document.querySelectorAll("#compose-body [data-remove]").forEach((btn) => {
      btn.addEventListener("click", () => removeProject(btn.dataset.remove, btn.dataset.name));
    });
  }

  async function runAction(action, path, name) {
    try {
      let deleteOldImages = false;
      if (action === "update") {
        const result = await confirmDialog({
          title: t("compose.updateConfirmTitle"),
          message: t("compose.updateConfirmMsg"),
          detail: path,
          confirmText: t("compose.update"),
          checkboxLabel: t("compose.deleteOldImages")
        });
        if (!result.ok) return;
        deleteOldImages = result.checked;
      }
      const res = await api.post("/api/compose/action", { path, action, deleteOldImages });
      toast(t("compose.actionStarted", { action: t(`compose.${action}`) }), "success");
      openJobModal(res.jobId, {
        title: `${t(`compose.${action}`)} · ${name}`,
        onDone: load
      });
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  }

  async function removeProject(path, name) {
    const { ok } = await confirmDialog({
      title: t("compose.removeConfirmTitle"),
      message: t("compose.removeConfirmMsg", { name }),
      danger: true,
      confirmText: t("compose.remove")
    });
    if (!ok) return;
    try {
      await api.del("/api/compose/projects", { path });
      toast(t("compose.removed"), "success");
      load();
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  }

  async function importProject() {
    const path = await promptDialog({
      title: t("compose.importTitle"),
      message: t("compose.importHint"),
      placeholder: t("compose.pathPlaceholder"),
      confirmText: t("compose.import"),
      validate: (v) => (v ? null : t("common.required"))
    });
    if (!path) return;
    try {
      await api.post("/api/compose/projects", { path });
      toast(t("compose.imported"), "success");
      load();
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  }

  document.getElementById("compose-import").addEventListener("click", importProject);
  document.getElementById("compose-refresh").addEventListener("click", load);

  // 自动刷新（10s）。
  let timer = null;
  const setAuto = (on) => {
    if (timer) clearInterval(timer);
    timer = null;
    if (on) timer = setInterval(load, 10000);
  };
  document.getElementById("compose-auto").addEventListener("change", (e) => setAuto(e.target.checked));
  setAuto(true);

  await load();
  return () => { if (timer) clearInterval(timer); };
}
