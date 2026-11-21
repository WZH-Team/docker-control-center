// 系统日志视图：查看日志尾部内容。
import { api } from "../api.js";
import { t, errorMessage } from "../i18n.js";
import { esc, spinner, emptyState, toast } from "../ui.js";

export async function renderLogs(view, ctx) {
  view.innerHTML = `
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <div class="toolbar ml-auto">
        <label class="flex items-center gap-2 text-sm text-slate-500">
          <span>${esc(t("logs.lines"))}</span>
          <select id="log-lines" class="input w-auto py-1.5 text-sm">
            <option value="100">100</option>
            <option value="200" selected>200</option>
            <option value="500">500</option>
            <option value="1000">1000</option>
          </select>
        </label>
        <label class="flex cursor-pointer items-center gap-2 text-sm text-slate-500">
          <input type="checkbox" id="log-auto" class="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500" />
          <span>${esc(t("common.autoRefresh"))}</span>
        </label>
        <button type="button" id="log-refresh" class="btn btn-secondary">${esc(t("common.refresh"))}</button>
      </div>
    </div>
    <div class="card overflow-hidden">
      <div class="flex flex-wrap items-center gap-3 border-b border-slate-100 px-5 py-3">
        <div class="text-sm text-slate-500">${esc(t("logs.path"))}:</div>
        <div class="break-all font-mono text-xs text-slate-600" id="log-path">-</div>
      </div>
      <div id="log-body" class="p-4">${spinner()}</div>
    </div>`;

  let timer = null;

  async function load() {
    const body = document.getElementById("log-body");
    const lines = document.getElementById("log-lines").value;
    try {
      const res = await api.get(`/api/system/logs?lines=${encodeURIComponent(lines)}`);
      document.getElementById("log-path").textContent = res.path || "-";
      if (!res.enabled) {
        body.innerHTML = emptyState(t("logs.disabled"));
        return;
      }
      const content = (res.lines || []).join("\n");
      body.innerHTML = content.trim()
        ? `<pre class="code-block max-h-[65vh] whitespace-pre-wrap break-all">${esc(content)}</pre>`
        : emptyState(t("logs.empty"));
    } catch (err) {
      body.innerHTML = `<div class="p-6 text-center text-sm text-rose-600">${esc(errorMessage(err))}</div>`;
    }
  }

  function setAuto(on) {
    if (timer) clearInterval(timer);
    timer = null;
    if (on) timer = setInterval(load, 10000);
  }

  document.getElementById("log-refresh").addEventListener("click", load);
  document.getElementById("log-lines").addEventListener("change", load);
  document.getElementById("log-auto").addEventListener("change", (e) => setAuto(e.target.checked));

  await load();
  return () => { if (timer) clearInterval(timer); };
}
