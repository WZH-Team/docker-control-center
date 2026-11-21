// 登录视图。
import { api } from "../api.js";
import { t, errorMessage } from "../i18n.js";
import { esc } from "../ui.js";

export function renderLogin(container, onSuccess) {
  container.innerHTML = `
    <form id="login-form" class="space-y-5" autocomplete="off">
      <div>
        <label class="label" for="login-username">${esc(t("login.username"))}</label>
        <input id="login-username" name="username" class="input" type="text" required maxlength="128"
               autocomplete="username" placeholder="admin" />
      </div>
      <div>
        <label class="label" for="login-password">${esc(t("login.password"))}</label>
        <input id="login-password" name="password" class="input" type="password" required maxlength="256"
               autocomplete="current-password" placeholder="••••••••" />
      </div>
      <p class="hidden rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-600" id="login-error"></p>
      <button type="submit" id="login-submit" class="btn btn-primary w-full py-2.5">
        ${esc(t("login.submit"))}
      </button>
    </form>`;

  const form = document.getElementById("login-form");
  const errEl = document.getElementById("login-error");
  const btn = document.getElementById("login-submit");

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    errEl.classList.add("hidden");
    btn.disabled = true;
    btn.textContent = t("login.loggingIn");

    try {
      const data = await api.post("/api/login", {
        username: document.getElementById("login-username").value.trim(),
        password: document.getElementById("login-password").value
      });
      onSuccess(data);
    } catch (err) {
      errEl.textContent = errorMessage(err);
      errEl.classList.remove("hidden");
      btn.disabled = false;
      btn.textContent = t("login.submit");
      document.getElementById("login-password").value = "";
      document.getElementById("login-password").focus();
    }
  });
}
