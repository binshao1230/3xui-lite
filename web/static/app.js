const state = {
  token: localStorage.getItem("token") || "",
  inbounds: [],
  theme: localStorage.getItem("theme") === "light" ? "light" : "dark",
};

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => [...document.querySelectorAll(sel)];

function toast(msg, type = "ok") {
  const el = $("#toast");
  if (!el) {
    console.warn("[toast]", msg);
    return;
  }
  el.textContent = msg || "";
  el.className = `toast ${type}`;
  // 确保可见（覆盖 hidden）
  el.classList.remove("hidden");
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.add("hidden"), 3200);
}

function copyText(text) {
  if (!text) return Promise.resolve(false);
  try {
    // HTTP / 非安全上下文下 navigator.clipboard 可能为 undefined
    const clip = navigator.clipboard;
    if (clip && typeof clip.writeText === "function") {
      return clip.writeText(text).then(() => true).catch(() => fallbackCopy(text));
    }
  } catch (_) {
    /* fall through */
  }
  return Promise.resolve(fallbackCopy(text));
}

function fallbackCopy(text) {
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.left = "-9999px";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch (_) {
    return false;
  }
}

// ----- theme -----
function applyTheme(theme, persist = true) {
  const t = theme === "light" ? "light" : "dark";
  state.theme = t;
  document.documentElement.setAttribute("data-theme", t);
  if (persist) {
    try {
      localStorage.setItem("theme", t);
    } catch (_) {}
  }
  $$("[data-theme-set]").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.themeSet === t);
  });
}

function bindThemeToggles() {
  $$("[data-theme-set]").forEach((btn) => {
    if (btn.dataset.bound) return;
    btn.dataset.bound = "1";
    btn.addEventListener("click", () => {
      applyTheme(btn.dataset.themeSet);
      toast(btn.dataset.themeSet === "light" ? "已切换浅色主题" : "已切换深色主题");
    });
  });
  applyTheme(state.theme, false);
}

bindThemeToggles();

async function api(path, opts = {}) {
  const headers = { "Content-Type": "application/json", ...(opts.headers || {}) };
  if (state.token) headers.Authorization = `Bearer ${state.token}`;
  const res = await fetch(path, { ...opts, headers });
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("application/json")) {
    const data = await res.json();
    if (res.status === 401) {
      logout(false);
      throw new Error(data.msg || "未登录");
    }
    return data;
  }
  return { ok: res.ok, text: await res.text() };
}

function showLogin() {
  $("#view-login").classList.remove("hidden");
  $("#view-main").classList.add("hidden");
}

function showMain() {
  $("#view-login").classList.add("hidden");
  $("#view-main").classList.remove("hidden");
  loadDashboard();
  loadInbounds();
}

function logout(callApi = true) {
  if (callApi && state.token) api("/api/logout", { method: "POST" }).catch(() => {});
  state.token = "";
  localStorage.removeItem("token");
  showLogin();
}

// ----- tabs -----
$$(".nav").forEach((btn) => {
  btn.addEventListener("click", () => {
    $$(".nav").forEach((b) => b.classList.remove("active"));
    btn.classList.add("active");
    $$(".tab").forEach((t) => t.classList.add("hidden"));
    $(`#tab-${btn.dataset.tab}`).classList.remove("hidden");
    if (btn.dataset.tab === "dashboard") loadDashboard();
    if (btn.dataset.tab === "inbounds") loadInbounds();
    if (btn.dataset.tab === "xray") loadLogs();
    if (btn.dataset.tab === "settings") loadHost();
  });
});

// ----- login -----
$("#btn-login").onclick = async () => {
  $("#login-err").textContent = "";
  try {
    const data = await api("/api/login", {
      method: "POST",
      body: JSON.stringify({
        username: $("#login-user").value.trim(),
        password: $("#login-pass").value,
      }),
    });
    if (!data.ok) {
      $("#login-err").textContent = data.msg || "登录失败";
      return;
    }
    state.token = data.token;
    localStorage.setItem("token", data.token);
    showMain();
  } catch (e) {
    $("#login-err").textContent = e.message;
  }
};

$("#btn-logout").onclick = () => logout(true);

// ----- dashboard -----
async function loadDashboard() {
  try {
    const res = await api("/api/status");
    if (!res.ok) return;
    const d = res.data;
    const coreName = d.activeCore === "singbox" ? "sing-box" : "Xray";
    const running = d.coreRunning ?? d.xrayRunning;
    $("#stats").innerHTML = `
      <div class="stat"><div class="label">当前内核</div>
        <div class="value" style="font-size:1.15rem">${esc(coreName)}</div></div>
      <div class="stat"><div class="label">内核状态</div>
        <div class="value ${running ? "ok" : "bad"}">${running ? "运行中" : "已停止"}</div></div>
      <div class="stat"><div class="label">内核版本</div><div class="value" style="font-size:.95rem">${esc(d.coreVersion || d.xrayVersion || "")}</div></div>
      <div class="stat"><div class="label">运行时长</div><div class="value">${fmtDur(d.uptime)}</div></div>
      <div class="stat"><div class="label">入站数</div><div class="value">${d.inboundCount}</div></div>
      <div class="stat"><div class="label">客户端数</div><div class="value">${d.clientCount}</div></div>
      <div class="stat"><div class="label">Xray</div><div class="value" style="font-size:.9rem">${d.xrayAvailable === false ? "未安装" : (d.xrayRunning ? "运行" : "待命")}</div></div>
      <div class="stat"><div class="label">sing-box</div><div class="value" style="font-size:.9rem">${d.singboxAvailable === false ? "未安装" : (d.singboxRunning ? "运行" : "待命")}</div></div>
    `;
  } catch (e) {
    toast(e.message, "err");
  }
}

// ----- inbounds -----
async function loadInbounds() {
  try {
    const res = await api("/api/inbounds");
    if (!res.ok) return;
    state.inbounds = res.data || [];
    renderInbounds();
  } catch (e) {
    toast(e.message, "err");
  }
}

function protoLabel(p) {
  return { vless: "VLESS", ss2022: "SS2022", relay: "中转" }[p] || p;
}

function renderInbounds() {
  const box = $("#inbound-list");
  if (!state.inbounds.length) {
    box.innerHTML = `<p class="muted">暂无入站，点击右上角新建。</p>`;
    return;
  }
  box.innerHTML = state.inbounds
    .map((ib) => {
      const clients = (ib.clients || [])
        .map(
          (c) => `
        <div class="client-row">
          <div>
            <strong>${esc(c.email)}</strong>
            <span class="badge ${c.enable ? "on" : "off"}">${c.enable ? "启用" : "停用"}</span>
            <div class="mono muted">${ib.protocol === "vless" ? esc(c.uuid) : esc(c.password || "")}</div>
          </div>
          <div class="row gap">
            ${
              ib.protocol !== "relay"
                ? `<button class="btn sm" data-link="${ib.id}:${c.id}">链接</button>
            <button class="btn sm primary" data-qr="${ib.id}:${c.id}">二维码</button>`
                : ""
            }
            <button class="btn sm danger" data-del-client="${ib.id}:${c.id}">删除</button>
          </div>
        </div>`
        )
        .join("");

      return `
      <div class="inbound">
        <div class="head">
          <div>
            <strong>${esc(ib.remark || "未命名")}</strong>
            <span class="badge proto">${protoLabel(ib.protocol)}</span>
            <span class="badge ${ib.enable ? "on" : "off"}">${ib.enable ? "启用" : "停用"}</span>
            <div class="muted" style="margin-top:.35rem">
              ${esc(ib.listen)}:${ib.port}
              ${ib.protocol === "vless" ? ` · ${esc(ib.network)}/${esc(ib.security)}` : ""}
            </div>
          </div>
          <div class="row gap">
            ${ib.protocol !== "relay" ? `<button class="btn sm" data-add-client="${ib.id}">+ 客户端</button>` : ""}
            <button class="btn sm" data-edit-ib="${ib.id}">编辑</button>
            <button class="btn sm danger" data-del-ib="${ib.id}">删除</button>
          </div>
        </div>
        ${ib.protocol !== "relay" ? `<div class="clients">${clients || '<p class="muted">暂无客户端</p>'}</div>` : ""}
      </div>`;
    })
    .join("");

  box.querySelectorAll("[data-del-ib]").forEach((b) => {
    b.onclick = async () => {
      if (!confirm("确认删除该入站？")) return;
      await api(`/api/inbounds/${b.dataset.delIb}`, { method: "DELETE" });
      toast("已删除");
      loadInbounds();
      loadDashboard();
    };
  });
  box.querySelectorAll("[data-edit-ib]").forEach((b) => {
    b.onclick = () => openInboundModal(Number(b.dataset.editIb));
  });
  box.querySelectorAll("[data-add-client]").forEach((b) => {
    b.onclick = () => openClientModal(Number(b.dataset.addClient));
  });
  box.querySelectorAll("[data-del-client]").forEach((b) => {
    b.onclick = async () => {
      const [id, cid] = b.dataset.delClient.split(":");
      if (!confirm("删除客户端？")) return;
      await api(`/api/inbounds/${id}/clients/${cid}`, { method: "DELETE" });
      toast("已删除客户端");
      loadInbounds();
    };
  });
  box.querySelectorAll("[data-link]").forEach((b) => {
    b.onclick = async () => {
      const [id, cid] = b.dataset.link.split(":");
      await showShareModal(id, cid, false);
    };
  });
  box.querySelectorAll("[data-qr]").forEach((b) => {
    b.onclick = async () => {
      const [id, cid] = b.dataset.qr.split(":");
      await showShareModal(id, cid, true);
    };
  });
}

async function showShareModal(inboundId, clientId, preferQR) {
  try {
    const res = await api(`/api/inbounds/${inboundId}/clients/${clientId}/link`);
    if (!res || res.ok === false || !res.link) {
      const detail = res?.msg || "无链接";
      toast(detail, "err");
      // 仍弹出说明，避免“完全没反应”
      $("#modal-title").textContent = "无法生成链接";
      $("#modal-body").innerHTML = `
        <p class="err">${esc(detail)}</p>
        <p class="muted" style="font-size:.9rem;line-height:1.5">
          请到「设置」填写公网域名/IP；并确认客户端有 UUID（VLESS）或密码（SS2022）。
        </p>
        <pre class="logbox" style="max-height:160px">${esc(JSON.stringify(res || {}, null, 2))}</pre>
        <button class="btn" id="share-close">关闭</button>`;
      $("#share-close").onclick = closeModal;
      openModal();
      return;
    }
    // 优先用 URL 出图（带 token，兼容 HTTP 非安全上下文）
    const tokenQ = encodeURIComponent(state.token || "");
    const qrUrl = `/api/inbounds/${inboundId}/clients/${clientId}/qrcode?size=280&token=${tokenQ}&t=${Date.now()}`;
    const qrData = res.qrcodeData || "";
    await copyText(res.link);
    $("#modal-title").textContent = preferQR ? "分享二维码" : "分享链接 / 二维码";
    $("#modal-body").innerHTML = `
      <div style="text-align:center">
        <div class="qr-wrap">
          <img id="share-qr" src="${qrData || qrUrl}" alt="qrcode" width="256" height="256"
               onerror="this.onerror=null;this.src='${qrUrl}'" />
        </div>
        <div class="row gap" style="justify-content:center;margin:.75rem 0">
          <a class="btn sm" id="btn-dl-qr" href="${qrUrl}" target="_blank" rel="noopener">打开二维码图</a>
          <button class="btn sm primary" id="btn-copy-link">复制链接</button>
        </div>
        <p class="muted" style="font-size:.8rem;margin:0 0 .35rem">主机: ${esc(res.host || "")}</p>
        <textarea id="share-link-text" readonly rows="5" style="width:100%;margin-top:.25rem;font-family:ui-monospace,Consolas,monospace;font-size:.8rem">${esc(res.link)}</textarea>
        <p class="muted" style="font-size:.8rem;margin-top:.5rem">可扫码或全选复制上方链接；设置里可固定公网 IP/域名</p>
        <button class="btn" id="share-close" style="margin-top:.75rem">关闭</button>
      </div>`;
    $("#btn-copy-link")?.addEventListener("click", async () => {
      const ok = await copyText(res.link);
      toast(ok ? "链接已复制" : "复制失败，请手动全选复制", ok ? "ok" : "err");
      const ta = $("#share-link-text");
      if (ta) {
        ta.focus();
        ta.select();
      }
    });
    $("#share-close").onclick = closeModal;
    openModal();
    toast(preferQR ? "二维码已生成" : "链接已生成");
  } catch (e) {
    console.error(e);
    toast(e.message || "打开分享失败", "err");
    alert("分享失败: " + (e.message || e));
  }
}

$("#btn-new-inbound").onclick = () => openInboundModal(null);

// One-click setup (dashboard + inbounds)
document.addEventListener("click", async (e) => {
  const btn = e.target.closest("[data-quick]");
  if (!btn) return;
  const preset = btn.dataset.quick;
  btn.disabled = true;
  try {
    const res = await api("/api/quick", {
      method: "POST",
      body: JSON.stringify({ preset, addClient: true }),
    });
    if (!res.ok) {
      toast(res.msg || "生成失败", "err");
      return;
    }
    const tip = res.warn ? `已生成但内核警告: ${res.warn}` : "一键配置已生成并应用";
    toast(tip, res.warn ? "err" : "ok");
    const el = $("#quick-result");
    if (el) {
      el.innerHTML = res.link
        ? `已创建 <strong>${esc(res.data?.remark || "")}</strong> · 端口 ${res.data?.port}<br>分享链接：<span class="mono">${esc(res.link)}</span>`
        : `已创建 ${esc(res.data?.remark || "")} · 端口 ${res.data?.port}`;
    }
    if (res.link && res.client?.id && res.data?.id) {
      await showShareModal(res.data.id, res.client.id, true);
    } else if (res.link) {
      await copyText(res.link);
      prompt("一键配置完成，分享链接（已尝试复制）:", res.link);
    }
    loadInbounds();
    loadDashboard();
  } catch (err) {
    toast(err.message, "err");
  } finally {
    btn.disabled = false;
  }
});

$("#btn-goto-inbounds")?.addEventListener("click", () => {
  $$(".nav").forEach((b) => b.classList.remove("active"));
  document.querySelector('.nav[data-tab="inbounds"]')?.classList.add("active");
  $$(".tab").forEach((t) => t.classList.add("hidden"));
  $("#tab-inbounds").classList.remove("hidden");
  loadInbounds();
});

function openInboundModal(id) {
  const ib = id ? state.inbounds.find((x) => x.id === id) : null;
  const isEdit = !!ib;
  $("#modal-title").textContent = isEdit ? "编辑入站（可手动改）" : "手动新建入站";
  let settings = {};
  try {
    settings = JSON.parse(ib?.settings || "{}");
  } catch (_) {}

  $("#modal-body").innerHTML = `
    <div class="form-grid">
      ${
        !isEdit
          ? `<div class="field"><label>先选预设自动填充（仍可改）</label>
        <div class="row gap" style="flex-wrap:wrap">
          <button type="button" class="btn sm" data-fill="vless-reality">填充 Reality</button>
          <button type="button" class="btn sm" data-fill="vless-tcp">填充 VLESS TCP</button>
          <button type="button" class="btn sm" data-fill="vless-ws">填充 WS</button>
          <button type="button" class="btn sm" data-fill="ss2022">填充 SS2022</button>
        </div></div>`
          : ""
      }
      <div class="field"><label>备注</label><input id="f-remark" value="${escAttr(ib?.remark || "")}" /></div>
      <div class="field"><label>协议</label>
        <select id="f-protocol" ${isEdit ? "disabled" : ""}>
          <option value="vless" ${ib?.protocol === "vless" ? "selected" : ""}>VLESS</option>
          <option value="ss2022" ${ib?.protocol === "ss2022" ? "selected" : ""}>Shadowsocks 2022</option>
          <option value="relay" ${ib?.protocol === "relay" ? "selected" : ""}>中转 (dokodemo-door)</option>
        </select>
      </div>
      <div class="field"><label>监听地址</label><input id="f-listen" value="${escAttr(ib?.listen || "0.0.0.0")}" /></div>
      <div class="field"><label>端口</label><input id="f-port" type="number" value="${ib?.port || 443}" /></div>
      <div class="field" data-vless><label>传输</label>
        <select id="f-network">
          <option value="tcp">TCP</option><option value="ws">WebSocket</option><option value="grpc">gRPC</option>
        </select>
      </div>
      <div class="field" data-vless><label>安全</label>
        <select id="f-security">
          <option value="none">none</option><option value="tls">TLS</option><option value="reality">Reality</option>
        </select>
      </div>
      <div class="field" data-vless data-reality><label>Reality PrivateKey</label><input id="f-rpriv" value="${escAttr(settings.realityPrivateKey || "")}" /></div>
      <div class="field" data-vless data-reality><label>Reality PublicKey (分享用)</label><input id="f-rpub" value="${escAttr(settings.realityPublicKey || "")}" /></div>
      <div class="field" data-vless data-reality><label>ServerNames (逗号分隔)</label><input id="f-sni" value="${escAttr((settings.serverNames || []).join(",") || "www.microsoft.com")}" /></div>
      <div class="field" data-vless data-reality><label>Dest</label><input id="f-dest" value="${escAttr(settings.dest || "www.microsoft.com:443")}" /></div>
      <div class="field" data-vless data-reality><label>ShortIds (逗号分隔)</label><input id="f-sid" value="${escAttr((settings.shortIds || [""]).join(","))}" /></div>
      <div class="field" data-vless data-flow><label>默认 Flow</label><input id="f-flow" value="${escAttr(settings.flow || "")}" placeholder="xtls-rprx-vision" /></div>
      <div class="field" data-vless data-ws><label>WS Path</label><input id="f-path" value="${escAttr(settings.path || "/")}" /></div>
      <div class="field" data-vless data-ws><label>WS Host</label><input id="f-host" value="${escAttr(settings.host || "")}" /></div>
      <div class="field" data-ss><label>加密方式</label>
        <select id="f-method">
          <option>2022-blake3-aes-128-gcm</option>
          <option>2022-blake3-aes-256-gcm</option>
          <option>2022-blake3-chacha20-poly1305</option>
        </select>
      </div>
      <div class="field" data-ss><label>服务端密码 (base64 key)</label>
        <div class="row gap">
          <input id="f-sspass" style="flex:1" value="${escAttr(settings.password || "")}" placeholder="自动生成或手动填写" />
          <button type="button" class="btn sm" id="btn-gen-ss">随机</button>
        </div>
      </div>
      <div class="field" data-relay><label>目标地址</label><input id="f-raddr" value="${escAttr(settings.address || "")}" placeholder="1.2.3.4" /></div>
      <div class="field" data-relay><label>目标端口</label><input id="f-rport" type="number" value="${settings.port || 443}" /></div>
      <div class="field" data-relay><label>网络</label><input id="f-rnet" value="${escAttr(settings.network || "tcp,udp")}" /></div>
      <div class="field"><label>高级：Settings JSON（可选覆盖）</label>
        <textarea id="f-settings-json" rows="4" style="width:100%;font-family:ui-monospace,Consolas,monospace;font-size:.8rem">${escAttr(ib?.settings && ib.settings !== "{}" ? ib.settings : "")}</textarea>
        <div class="muted" style="font-size:.75rem;margin-top:.25rem">留空则用上方表单字段；填写则优先使用 JSON</div>
      </div>
      <div class="field"><label><input type="checkbox" id="f-enable" ${ib?.enable !== false ? "checked" : ""} /> 启用</label></div>
      <div class="row gap" style="margin-top:.5rem">
        <button class="btn primary" id="f-save">保存</button>
        <button class="btn" id="f-cancel">取消</button>
      </div>
    </div>`;

  if (ib) {
    $("#f-network").value = ib.network || "tcp";
    $("#f-security").value = ib.security || "none";
    if (settings.method) $("#f-method").value = settings.method;
  }

  const syncFields = () => {
    const p = $("#f-protocol").value;
    const sec = $("#f-security")?.value;
    const net = $("#f-network")?.value;
    $$("[data-vless]").forEach((el) => (el.style.display = p === "vless" ? "" : "none"));
    $$("[data-ss]").forEach((el) => (el.style.display = p === "ss2022" ? "" : "none"));
    $$("[data-relay]").forEach((el) => (el.style.display = p === "relay" ? "" : "none"));
    $$("[data-reality]").forEach((el) => (el.style.display = p === "vless" && sec === "reality" ? "" : "none"));
    $$("[data-ws]").forEach((el) => (el.style.display = p === "vless" && net === "ws" ? "" : "none"));
  };
  $("#f-protocol").onchange = syncFields;
  $("#f-security").onchange = syncFields;
  $("#f-network").onchange = syncFields;
  syncFields();

  const applyDraft = (d) => {
    if (!d) return;
    $("#f-remark").value = d.remark || "";
    $("#f-protocol").value = d.protocol || "vless";
    $("#f-listen").value = d.listen || "0.0.0.0";
    $("#f-port").value = d.port || 443;
    if ($("#f-network")) $("#f-network").value = d.network || "tcp";
    if ($("#f-security")) $("#f-security").value = d.security || "none";
    const st = d.settings || {};
    if ($("#f-flow")) $("#f-flow").value = st.flow || "";
    if ($("#f-path")) $("#f-path").value = st.path || "/";
    if ($("#f-host")) $("#f-host").value = st.host || "";
    if ($("#f-rpriv")) $("#f-rpriv").value = st.realityPrivateKey || "";
    if ($("#f-rpub")) $("#f-rpub").value = st.realityPublicKey || "";
    if ($("#f-sni")) $("#f-sni").value = (st.serverNames || []).join(",") || "www.microsoft.com";
    if ($("#f-dest")) $("#f-dest").value = st.dest || "";
    if ($("#f-sid")) $("#f-sid").value = (st.shortIds || []).join(",");
    if ($("#f-method") && st.method) $("#f-method").value = st.method;
    if ($("#f-sspass")) $("#f-sspass").value = st.password || "";
    $("#f-settings-json").value = JSON.stringify(st, null, 2);
    syncFields();
    toast(d.tips || "已自动填充，可继续修改", "ok");
  };

  $$("[data-fill]").forEach((b) => {
    b.onclick = async () => {
      const res = await api(`/api/gen/defaults?preset=${encodeURIComponent(b.dataset.fill)}`);
      if (!res.ok) {
        toast(res.msg || "填充失败", "err");
        return;
      }
      applyDraft(res.data);
    };
  });

  $("#btn-gen-ss")?.addEventListener("click", async () => {
    const res = await api("/api/gen/defaults?preset=ss2022");
    if (res.ok && res.data?.settings?.password) {
      $("#f-sspass").value = res.data.settings.password;
      if (!$("#f-port").value) $("#f-port").value = res.data.port;
    }
  });

  $("#f-cancel").onclick = closeModal;
  $("#f-save").onclick = async () => {
    const protocol = $("#f-protocol").value;
    let settings = {};
    const rawJson = $("#f-settings-json")?.value?.trim();
    if (rawJson) {
      try {
        settings = JSON.parse(rawJson);
      } catch {
        toast("Settings JSON 格式错误", "err");
        return;
      }
    } else if (protocol === "vless") {
      settings = {
        flow: $("#f-flow").value.trim(),
        path: $("#f-path").value.trim(),
        host: $("#f-host").value.trim(),
        realityPrivateKey: $("#f-rpriv").value.trim(),
        realityPublicKey: $("#f-rpub").value.trim(),
        serverNames: $("#f-sni").value.split(",").map((s) => s.trim()).filter(Boolean),
        dest: $("#f-dest").value.trim(),
        shortIds: $("#f-sid").value.split(",").map((s) => s.trim()),
      };
    } else if (protocol === "ss2022") {
      settings = {
        method: $("#f-method").value,
        password: $("#f-sspass").value.trim(),
        network: "tcp,udp",
      };
    } else {
      settings = {
        address: $("#f-raddr").value.trim(),
        port: Number($("#f-rport").value),
        network: $("#f-rnet").value.trim() || "tcp,udp",
      };
    }
    const body = {
      remark: $("#f-remark").value.trim(),
      enable: $("#f-enable").checked,
      protocol,
      listen: $("#f-listen").value.trim() || "0.0.0.0",
      port: Number($("#f-port").value),
      network: $("#f-network")?.value || "tcp",
      security: $("#f-security")?.value || "none",
      settings,
    };
    const res = isEdit
      ? await api(`/api/inbounds/${id}`, { method: "PUT", body: JSON.stringify(body) })
      : await api("/api/inbounds", { method: "POST", body: JSON.stringify(body) });
    if (!res.ok) {
      toast(res.msg || "失败", "err");
      return;
    }
    if (res.warn) toast(res.warn, "err");
    else toast("已保存");
    closeModal();
    loadInbounds();
    loadDashboard();
  };

  openModal();
}

function openClientModal(inboundId) {
  const ib = state.inbounds.find((x) => x.id === inboundId);
  $("#modal-title").textContent = `添加客户端 · ${ib?.remark || inboundId}`;
  $("#modal-body").innerHTML = `
    <div class="form-grid">
      <div class="field"><label>邮箱/备注</label><input id="c-email" placeholder="user1" /></div>
      ${
        ib?.protocol === "vless"
          ? `<div class="field"><label>UUID (空则自动生成)</label><input id="c-uuid" /></div>
             <div class="field"><label>Flow</label><input id="c-flow" placeholder="xtls-rprx-vision" /></div>`
          : `<div class="field"><label>密码 (空则自动生成)</label><input id="c-pass" /></div>`
      }
      <div class="row gap">
        <button class="btn primary" id="c-save">添加</button>
        <button class="btn" id="c-cancel">取消</button>
      </div>
    </div>`;
  $("#c-cancel").onclick = closeModal;
  $("#c-save").onclick = async () => {
    const body = { email: $("#c-email").value.trim() };
    if (ib?.protocol === "vless") {
      body.uuid = $("#c-uuid").value.trim();
      body.flow = $("#c-flow").value.trim();
    } else {
      body.password = $("#c-pass").value.trim();
    }
    const res = await api(`/api/inbounds/${inboundId}/clients`, {
      method: "POST",
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      toast(res.msg || "失败", "err");
      return;
    }
    toast(res.warn || "已添加", res.warn ? "err" : "ok");
    closeModal();
    loadInbounds();
  };
  openModal();
}

function openModal() {
  $("#modal").classList.remove("hidden");
}
function closeModal() {
  $("#modal").classList.add("hidden");
}
$("#modal-close").onclick = closeModal;
$("#modal").addEventListener("click", (e) => {
  if (e.target.id === "modal") closeModal();
});

// ----- core -----
async function loadCorePanel() {
  try {
    const res = await api("/api/core");
    if (!res.ok) return;
    const d = res.data || {};
    const active = d.activeCore || "xray";
    const label = active === "singbox" ? "sing-box" : "Xray";
    $("#core-active-label").textContent = label;
    $("#core-detail").textContent =
      `Xray: ${d.xrayAvailable ? d.xrayVersion : "未安装"} · ` +
      `sing-box: ${d.singboxAvailable ? d.singboxVersion : "未安装"} · ` +
      `状态: ${d.coreRunning ? "运行中" : "已停止"}`;
    $("#btn-core-xray").classList.toggle("primary", active === "xray");
    $("#btn-core-singbox").classList.toggle("primary", active === "singbox");
    $("#btn-core-xray").disabled = d.xrayAvailable === false;
    $("#btn-core-singbox").disabled = d.singboxAvailable === false;
  } catch (e) {
    toast(e.message, "err");
  }
}

async function switchCore(name) {
  const res = await api("/api/core", {
    method: "POST",
    body: JSON.stringify({ core: name }),
  });
  if (!res.ok) {
    toast(res.msg || "切换失败", "err");
    return;
  }
  toast(res.warn || `已切换到 ${name === "singbox" ? "sing-box" : "Xray"}`, res.warn ? "err" : "ok");
  loadCorePanel();
  loadLogs();
  loadDashboard();
}

$("#btn-core-xray").onclick = () => switchCore("xray");
$("#btn-core-singbox").onclick = () => switchCore("singbox");

$("#btn-xray-restart").onclick = async () => {
  const res = await api("/api/core/restart", { method: "POST" });
  toast(res.ok ? "已重启" : res.msg || "失败", res.ok ? "ok" : "err");
  loadLogs();
  loadCorePanel();
  loadDashboard();
};
$("#btn-xray-start").onclick = async () => {
  const res = await api("/api/core/start", { method: "POST" });
  toast(res.ok ? "已启动" : res.msg || "失败", res.ok ? "ok" : "err");
  loadCorePanel();
  loadDashboard();
};
$("#btn-xray-stop").onclick = async () => {
  const res = await api("/api/core/stop", { method: "POST" });
  toast(res.ok ? "已停止" : res.msg || "失败", res.ok ? "ok" : "err");
  loadCorePanel();
  loadDashboard();
};
$("#btn-xray-config").onclick = async () => {
  const res = await fetch("/api/core/config", {
    headers: { Authorization: `Bearer ${state.token}` },
  });
  const text = await res.text();
  $("#xray-config").classList.remove("hidden");
  $("#xray-config").textContent = text;
};
async function loadLogs() {
  await loadCorePanel();
  const res = await api("/api/core/logs");
  if (res.ok) {
    const lines = res.data || [];
    let t = `[core=${res.core || "?"}] ` + (lines.join("\n") || "(暂无日志)");
    if (res.lastError) t = "[last error] " + res.lastError + "\n\n" + t;
    $("#xray-logs").textContent = t;
  }
}

// ----- settings -----
async function loadHost() {
  const res = await api("/api/settings/host");
  if (res.ok) {
    // 优先显示已保存；否则用检测到的公网 Host 填入提示
    $("#public-host").value = res.host || res.detectHost || "";
    const tip = $("#host-detect-tip");
    if (tip) {
      tip.textContent = res.detectHost
        ? `当前访问检测到主机: ${res.detectHost}（未设置时分享链接会自动用它）`
        : "";
    }
  }
}
$("#btn-save-host").onclick = async () => {
  const res = await api("/api/settings/host", {
    method: "POST",
    body: JSON.stringify({ host: $("#public-host").value.trim() }),
  });
  toast(res.ok ? "已保存" : res.msg || "失败", res.ok ? "ok" : "err");
};
$("#btn-pass").onclick = async () => {
  const res = await api("/api/password", {
    method: "POST",
    body: JSON.stringify({
      oldPassword: $("#old-pass").value,
      newPassword: $("#new-pass").value,
    }),
  });
  toast(res.ok ? "密码已更新" : res.msg || "失败", res.ok ? "ok" : "err");
};

// ----- online update -----
state.updateInfo = null;

async function checkUpdate() {
  const box = $("#update-info");
  const btn = $("#btn-do-update");
  if (box) box.textContent = "检查中...";
  if (btn) btn.disabled = true;
  try {
    const res = await api("/api/update/check");
    if (!res.ok) {
      if (box) box.textContent = res.msg || "检查失败";
      toast(res.msg || "检查失败", "err");
      return;
    }
    state.updateInfo = res.data;
    const d = res.data || {};
    const lines = [
      `当前版本: v${d.current || "?"}`,
      `最新版本: v${d.latest || "?"}`,
      d.hasUpdate ? "状态: 发现新版本" : "状态: 已是最新",
      d.assetName ? `安装包: ${d.assetName}` : "",
      d.reason ? `说明: ${d.reason}` : "",
      d.htmlUrl ? `Release: ${d.htmlUrl}` : "",
      "",
      d.body ? String(d.body).slice(0, 800) : "",
    ].filter((x) => x !== undefined);
    if (box) box.textContent = lines.join("\n");
    if (btn) {
      btn.disabled = !(d.hasUpdate && d.canUpdate);
    }
    toast(d.hasUpdate ? `发现新版本 v${d.latest}` : "已是最新版本");
  } catch (e) {
    if (box) box.textContent = e.message || String(e);
    toast(e.message || "检查失败", "err");
  }
}

async function doUpdate() {
  if (!confirm("确定从 GitHub 下载并更新？更新过程中面板会短暂重启。")) return;
  const box = $("#update-info");
  const btn = $("#btn-do-update");
  if (btn) btn.disabled = true;
  if (box) box.textContent = (box.textContent || "") + "\n\n正在下载并更新，请稍候...";
  try {
    const res = await api("/api/update", { method: "POST" });
    if (!res.ok) {
      toast(res.msg || "更新失败", "err");
      if (box) box.textContent += "\n失败: " + (res.msg || "");
      if (btn) btn.disabled = false;
      return;
    }
    toast(res.msg || "更新完成，正在重启…");
    if (box) box.textContent += "\n" + (res.msg || "ok") + "\n服务即将重启，10 秒后自动刷新…";
    // wait for service restart then reload
    let n = 0;
    const timer = setInterval(async () => {
      n++;
      try {
        const r = await fetch("/api/status", {
          headers: { Authorization: `Bearer ${state.token}` },
        });
        if (r.ok) {
          clearInterval(timer);
          toast("更新成功，页面刷新");
          location.reload();
        }
      } catch (_) {}
      if (n > 30) {
        clearInterval(timer);
        toast("请手动刷新页面", "err");
      }
    }, 2000);
  } catch (e) {
    // 面板退出时 fetch 可能失败，进入轮询
    if (box) box.textContent += "\n连接中断（可能正在重启），等待恢复…";
    let n = 0;
    const timer = setInterval(async () => {
      n++;
      try {
        const r = await fetch("/api/status", {
          headers: { Authorization: `Bearer ${state.token}` },
        });
        if (r.ok) {
          clearInterval(timer);
          location.reload();
        }
      } catch (_) {}
      if (n > 30) {
        clearInterval(timer);
        toast(e.message || "更新后请手动刷新", "err");
      }
    }, 2000);
  }
}

$("#btn-check-update")?.addEventListener("click", checkUpdate);
$("#btn-do-update")?.addEventListener("click", doUpdate);

// ----- utils -----
function esc(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}
function escAttr(s) {
  return esc(s).replace(/'/g, "&#39;");
}
function fmtDur(sec) {
  sec = Number(sec) || 0;
  if (sec < 60) return sec + "s";
  if (sec < 3600) return Math.floor(sec / 60) + "m";
  if (sec < 86400) return Math.floor(sec / 3600) + "h";
  return Math.floor(sec / 86400) + "d";
}

// boot
applyTheme(state.theme, false);
if (state.token) {
  api("/api/status")
    .then((r) => {
      if (r.ok) showMain();
      else showLogin();
    })
    .catch(() => showLogin());
} else {
  showLogin();
}
