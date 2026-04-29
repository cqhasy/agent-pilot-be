import { getJSON, postJSON } from "./api.js";
import {
  applyReplacements,
  buildTargetPrompt,
  markSelectionAsTarget,
  refreshContextBox,
  updateEditorSelectionCache,
} from "./features/doc-rewrite.js";
import { el, state } from "./state.js";
import {
  clearChatTaskPanel,
  renderConversation,
  renderLayout,
  renderSessions,
  renderUserCard,
  setChatTaskList,
} from "./ui.js";

async function fetchSessions() {
  const res = await getJSON("/api/sessions");
  state.sessions = res.data?.data || res.data || [];
  renderSessions();
  el.sessionList.querySelectorAll(".session-item").forEach((item) => {
    item.onclick = () => loadSession(item.dataset.sessionId);
  });
}

async function loadSession(id) {
  state.currentSessionId = id;
  renderSessions();
  const res = await getJSON(
    `/api/session/detail?sessionId=${encodeURIComponent(id)}`,
  );
  const data = res.data?.data || res.data;
  state.conversation = data.messages || [];
  renderConversation(state.conversation);
  state.workspace = data.session?.workspace || "chat";
  state.uiMode = state.workspace === "doc" ? "doc" : "chat";
  if (state.uiMode === "doc")
    el.editor.innerText = data.session?.document || "";
  renderLayout();
  state.selectedTargets = [];
  refreshContextBox();
  if (
    Array.isArray(data.session?.latestTaskPlan) &&
    data.session.latestTaskPlan.length
  ) {
    setChatTaskList(data.session.latestTaskPlan);
  } else {
    clearChatTaskPanel();
  }
}

async function sendMessage(message) {
  clearChatTaskPanel();
  const displayConversation = [
    ...state.conversation,
    { role: "user", content: message },
    { role: "assistant", content: "" },
  ];
  renderConversation(displayConversation);
  const assistantBubble = el.chatLog.lastElementChild;

  const res = await fetch("/api/v1/chat/stream", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message }),
  });
  if (!res.ok || !res.body) {
    if (assistantBubble) assistantBubble.textContent = "请求失败，请稍后重试。";
    return {};
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";
  let assistantText = "";
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split("\n\n");
    buffer = parts.pop() || "";
    for (const part of parts) {
      const lines = part.split("\n");
      let event = "";
      let dataLine = "";
      for (const line of lines) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        if (line.startsWith("data:")) dataLine = line.slice(5).trim();
      }
      if (!event || !dataLine) continue;
      if (event === "message") {
        assistantText += dataLine;
        if (assistantBubble) assistantBubble.textContent = assistantText;
        el.chatLog.scrollTop = el.chatLog.scrollHeight;
      } else if (event === "done") {
        state.conversation = [
          ...state.conversation,
          { role: "user", content: message },
          { role: "assistant", content: assistantText },
        ];
        return { content: assistantText };
      } else if (event === "error") {
        if (assistantBubble) assistantBubble.textContent = `错误: ${dataLine}`;
        return {};
      }
    }
  }
  return { content: assistantText };
}

async function sendFromInput() {
  const text = el.chatInput.value.trim();
  if (!text) return;
  const targets =
    state.uiMode === "doc"
      ? state.selectedTargets.filter((t) => t.element?.parentNode)
      : [];
  const composed = targets.length
    ? `${text}\n\n${buildTargetPrompt(targets)}`
    : text;
  el.chatInput.value = "";
  const data = await sendMessage(composed);
  if (targets.length) {
    const ok = applyReplacements(data, targets);
    if (!ok) alert("AI 暂未返回可应用的改写结果。");
  }
}

async function rewriteSelectionNow() {
  const targets = state.selectedTargets.filter((t) => t.element?.parentNode);
  if (!targets.length) markSelectionAsTarget();
  const finalTargets = state.selectedTargets.filter(
    (t) => t.element?.parentNode,
  );
  if (!finalTargets.length) {
    alert("请先选中文档中的内容。");
    return;
  }
  const data = await sendMessage(buildTargetPrompt(finalTargets));
  const ok = applyReplacements(data, finalTargets);
  if (!ok) alert("AI 暂未返回可应用的改写结果。");
}

async function saveDocument() {
  alert("保存文档功能还没有接入新的后端接口。");
}

function extractPayload(res) {
  return res.data?.data ?? res.data;
}

async function loginWithEmail(event) {
  event?.preventDefault();
  const res = await postJSON("/api/auth/email/login", {
    userEmail: el.loginEmailInput.value.trim(),
    password: el.loginPasswordInput.value,
  });
  if (!res.ok) {
    alert(res.data?.message || "登录失败");
    return;
  }
  const user = extractPayload(res);
  localStorage.setItem("authToken", user.token);
  renderUserCard(user);
  el.accountMenu?.classList.add("hidden");
}

async function registerWithEmail(event) {
  event?.preventDefault();
  const res = await postJSON("/api/auth/email/register", {
    username: el.registerNameInput.value.trim(),
    userEmail: el.registerEmailInput.value.trim(),
    password: el.registerPasswordInput.value,
    confirmPassword: el.registerConfirmPasswordInput.value,
    code: el.registerCodeInput.value.trim(),
  });
  if (!res.ok) {
    alert(res.data?.message || "注册失败");
    return;
  }
  const user = extractPayload(res);
  localStorage.setItem("authToken", user.token);
  renderUserCard(user);
  el.accountMenu?.classList.add("hidden");
}

async function sendRegisterCode() {
  const email = el.registerEmailInput.value.trim();
  if (!email) {
    alert("请先填写邮箱");
    return;
  }
  const res = await postJSON("/api/auth/email/send", { userEmail: email });
  if (!res.ok) {
    alert(res.data?.message || "验证码发送失败");
    return;
  }
  alert("验证码已发送");
}

function toggleAuthMode() {
  const registerVisible = !el.emailRegisterForm.classList.contains("hidden");
  el.emailRegisterForm.classList.toggle("hidden", registerVisible);
  el.emailLoginForm.classList.toggle("hidden", !registerVisible);
  el.toggleAuthModeBtn.textContent = registerVisible
    ? "创建账号"
    : "已有账号，去登录";
}

async function fetchMe() {
  const res = await getJSON("/api/auth/me");
  if (!res.ok) {
    renderUserCard(null);
    return;
  }
  renderUserCard(extractPayload(res));
}

async function logout() {
  const res = await postJSON("/api/auth/email/logout", {});
  if (!res.ok) {
    alert(res.data?.message || "退出失败");
    return;
  }
  localStorage.removeItem("authToken");
  renderUserCard(null);
}

function resetSession() {
  state.currentSessionId = "";
  state.conversation = [];
  state.uiMode = "chat";
  state.workspace = "chat";
  state.selectedTargets = [];
  renderLayout();
  renderConversation([]);
  refreshContextBox();
  el.editor.innerText = "在这里编辑 AI 生成的文档内容。";
  renderSessions();
  clearChatTaskPanel();
}

function wireEvents() {
  el.sendBtn.onclick = sendFromInput;
  el.newSessionBtn.onclick = resetSession;
  el.emailLoginBtn.onclick = () => el.accountMenu?.classList.toggle("hidden");
  el.emailLoginForm.onsubmit = loginWithEmail;
  el.emailRegisterForm.onsubmit = registerWithEmail;
  el.sendCodeBtn.onclick = sendRegisterCode;
  el.toggleAuthModeBtn.onclick = toggleAuthMode;
  el.logoutBtn.onclick = logout;
  el.reloginBtn.onclick = () => {
    localStorage.removeItem("authToken");
    renderUserCard(null);
    el.accountMenu?.classList.remove("hidden");
  };
  document.addEventListener("click", (e) => {
    if (!el.accountMenu || el.accountMenu.classList.contains("hidden")) return;
    const target = e.target;
    if (target === el.emailLoginBtn || el.emailLoginBtn.contains(target))
      return;
    if (el.accountMenu.contains(target)) return;
    el.accountMenu.classList.add("hidden");
  });
  el.toggleTaskBtn &&
    (el.toggleTaskBtn.onclick = () => {
      if (el.chatTaskList.style.display === "none") {
        el.chatTaskList.style.display = "";
        el.toggleTaskBtn.textContent = "收起";
      } else {
        el.chatTaskList.style.display = "none";
        el.toggleTaskBtn.textContent = "展开";
      }
    });
  el.saveLarkBtn.onclick = saveDocument;
  el.rewriteSelectionBtn.onclick = rewriteSelectionNow;
  el.markSelectionBtn.onmousedown = (e) => e.preventDefault();
  el.markSelectionBtn.onclick = markSelectionAsTarget;
  document.addEventListener("selectionchange", updateEditorSelectionCache);
  el.editor.addEventListener("mouseup", updateEditorSelectionCache);
  el.editor.addEventListener("keyup", updateEditorSelectionCache);
}

async function bootstrap() {
  wireEvents();
  renderLayout();
  await Promise.all([fetchMe(), fetchSessions()]);
}

bootstrap();
