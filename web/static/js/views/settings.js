// 设置视图：系统信息、配置查看、凭据修改、配置重载。
import { api } from "../api.js";
import { t, errorMessage } from "../i18n.js";
import { esc, fmtDuration, toast, kv, spinner, confirmDialog } from "../ui.js";

export async function renderSettings(view, ctx) {
  view.innerHTML = `
    <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <div class="card">
        <div class="card-header"><h3 class="card-title">${esc(t("settings.system"))}</h3></div>
        <div class="px-5 py-4" id="sys-info">${spinner()}</div>
      </div>

      <div class="card">
        <div class="card-header">
          <h3 class="card-title">${esc(t("settings.server"))}</h3>
          <button type="button" id="cfg-reload" class="btn btn-secondary btn-xs">${esc(t("settings.reload"))}</button>
        </div>
        <div class="px-5 py-4" id="cfg-info">${spinner()}</div>
      </div>

      <div class="card">
        <div class="card-header"><h3 class="card-title">${esc(t("settings.credentials"))}</h3></div>
        <div class="px-5 py-4">
          <form id="cred-form" class="space-y-4">
            <div>
              <label class="label" for="cred-username">${esc(t("settings.newUsername"))}</label>
              <input id="cred-username" class="input" type="text" required maxlength="128" autocomplete="off" />
            </div>
            <div>
              <label class="label" for="cred-password">${esc(t("settings.newPassword"))}</label>
              <input id="cred-password" class="input" type="password" required minlength="8" maxlength="256" autocomplete="new-password" />
            </div>
            <div>
              <label class="label" for="cred-password2">${esc(t("settings.confirmPassword"))}</label>
              <input id="cred-password2" class="input" type="password" required minlength="8" maxlength="256" autocomplete="new-password" />
            </div>
            <p class="hidden rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-600" id="cred-error"></p>
            <div class="flex items-center gap-2">
              <button type="submit" class="btn btn-primary">${esc(t("common.save"))}</button>
            </div>
          </form>
        </div>
      </div>

      <div class="card">
        <div class="card-header"><h3 class="card-title">${esc(t("settings.composeProjects"))}</h3></div>
        <div class="px-5 py-4" id="compose-info">${spinner()}</div>
      </div>
    </div>`;

  // 系统信息。
  try {
    const status = await api.get("/api/status");
    const app = status.app || {};
    const docker = status.docker || {};
    const startedAt = app.startedAt ? new Date(app.startedAt).getTime() : Date.now();
    document.getElementById("sys-info").innerHTML = `
      <dl class="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2">
        ${kv(t("settings.version"), esc(app.version || "-"))}
        ${kv(t("settings.platform"), esc(app.platform || "-"))}
        ${kv(t("settings.uptime"), esc(fmtDuration((Date.now() - startedAt) / 1000)))}
        ${kv(t("settings.listen"), `<span class="font-mono text-xs">${esc(app.listen || "-")}</span>`)}
        ${kv(t("settings.configPath"), `<span class="break-all font-mono text-xs">${esc(app.configPath || "-")}</span>`)}
        ${kv(t("docker.version"), esc(docker.version?.Version || "-"))}
        ${kv(t("docker.apiVersion"), esc(docker.version?.ApiVersion || "-"))}
        ${kv(t("docker.composeAvailable"), docker.composeAvailable ? esc(t("common.yes")) : esc(t("docker.composeMissing")))}
      </dl>`;
  } catch (err) {
    document.getElementById("sys-info").innerHTML = `<p class="text-sm text-rose-600">${esc(errorMessage(err))}</p>`;
  }

  // 配置信息。
  async function loadConfig() {
    try {
      const cfg = await api.get("/api/config/view");
      document.getElementById("cfg-info").innerHTML = `
        <dl class="grid grid-cols-1 gap-x-6 gap-y-4 sm:grid-cols-2">
          ${kv(t("settings.port"), esc(cfg.server?.port ?? "-"))}
          ${kv(t("settings.host"), esc(cfg.server?.host || "*"))}
          ${kv(t("settings.ssl"), cfg.ssl?.enabled ? esc(t("settings.sslEnabled")) : esc(t("settings.sslDisabled")))}
          ${kv(t("settings.keepHTTP"), cfg.ssl?.keepHTTP ? esc(t("common.yes")) : esc(t("common.no")))}
          ${kv(t("settings.logging"), (cfg.logging?.enabled ? esc(t("settings.loggingOn")) : esc(t("settings.loggingOff"))) + ` <span class="text-xs text-slate-400">· ${esc((cfg.logging?.level || "").toUpperCase())}</span>`)}
          ${kv(t("logs.path"), `<span class="break-all font-mono text-xs">${esc(cfg.logging?.file || "-")}</span>`)}
          ${kv(t("settings.dockerHost"), `<span class="font-mono text-xs">${esc(cfg.docker?.host || "(default)")}</span>`)}
          ${kv(t("settings.certDir"), `<span class="break-all font-mono text-xs">${esc(cfg.ssl?.certDir || "-")}</span>`)}
        </dl>
        <p class="mt-4 rounded-lg bg-brand-50 px-3 py-2 text-xs text-brand-800">${esc(t("settings.reloadHint"))}</p>`;

      const projects = cfg.composeProjects || [];
      document.getElementById("compose-info").innerHTML = projects.length
        ? `<div class="space-y-2">${projects.map((p) => `
            <div class="rounded-lg bg-slate-50 px-3 py-2">
              <div class="text-sm font-medium text-slate-700">${esc(p.name || "-")}</div>
              <div class="break-all font-mono text-xs text-slate-500">${esc(p.absPath || p.path)}</div>
            </div>`).join("")}</div>`
        : `<div class="text-sm text-slate-400">${esc(t("compose.empty"))}</div>`;
    } catch (err) {
      document.getElementById("cfg-info").innerHTML = `<p class="text-sm text-rose-600">${esc(errorMessage(err))}</p>`;
    }
  }
  await loadConfig();

  // 重新加载配置。
  document.getElementById("cfg-reload").addEventListener("click", async () => {
    try {
      await api.post("/api/config/reload");
      toast(t("settings.reloadDone"), "success");
      loadConfig();
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  });

  // 修改凭据。
  const form = document.getElementById("cred-form");
  const errEl = document.getElementById("cred-error");
  document.getElementById("cred-username").value = ctx.session?.username || "";
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    errEl.classList.add("hidden");

    const username = document.getElementById("cred-username").value.trim();
    const password = document.getElementById("cred-password").value;
    const password2 = document.getElementById("cred-password2").value;

    if (password.length < 8) {
      errEl.textContent = t("settings.passwordTooShort");
      errEl.classList.remove("hidden");
      return;
    }
    if (password !== password2) {
      errEl.textContent = t("settings.passwordMismatch");
      errEl.classList.remove("hidden");
      return;
    }

    const { ok } = await confirmDialog({
      title: t("settings.changeCredentials"),
      message: t("settings.credentialsSaved"),
      confirmText: t("common.save")
    });
    if (!ok) return;

    try {
      await api.post("/api/config/auth", { username, password });
      toast(t("settings.credentialsSaved"), "success");
      form.reset();
    } catch (err) {
      errEl.textContent = errorMessage(err);
      errEl.classList.remove("hidden");
    }
  });
}
