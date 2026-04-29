import { el, state } from "./state.js";

export function escapeHtml(value) {
  return (value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

export function renderLayout() {
  const isDoc = state.uiMode === "doc";
  el.main.classList.toggle("doc-mode", isDoc);
  el.workspaceTag.textContent = `当前模式：${state.workspace === "doc" ? "文档" : state.workspace === "ppt" ? "PPT" : "聊天"}`;
  if (!isDoc) {
    hideSelectionQuickAction();
    state.selectedTargets = [];
    renderContextBox();
  }
}

export function renderSessions() {
  el.sessionList.innerHTML = "";
  state.sessions.forEach((s) => {
    const div = document.createElement("div");
    div.className =
      "session-item" + (s.id === state.currentSessionId ? " active" : "");
    div.innerHTML = `<strong>${escapeHtml(s.title)}</strong><div style="font-size:12px;color:#667;margin-top:4px;">${new Date(s.updatedAt).toLocaleString()}</div>`;
    div.dataset.sessionId = s.id;
    el.sessionList.appendChild(div);
  });
}

export function renderConversation(messages) {
  el.chatLog.innerHTML = "";
  (messages || []).forEach((m) => {
    const div = document.createElement("div");
    div.className = "bubble " + (m.role === "user" ? "user" : "assistant");
    div.textContent = m.content;
    el.chatLog.appendChild(div);
  });
  el.chatLog.scrollTop = el.chatLog.scrollHeight;
}

export function renderTaskPlan(tasks) {}

export function clearChatTaskPanel() {
  el.chatTaskList.innerHTML = "";
  el.chatTaskPanel.classList.add("hidden");
  if (el.toggleTaskBtn) el.toggleTaskBtn.textContent = "收起";
}

export function addChatTaskItem(text) {
  if (!text) return;
  el.chatTaskPanel.classList.remove("hidden");
  const div = document.createElement("div");
  div.className = "chat-task-item";
  div.textContent = text;
  el.chatTaskList.appendChild(div);
}

export function setChatTaskList(tasks) {
  el.chatTaskList.innerHTML = "";
  if (!tasks || !tasks.length) {
    el.chatTaskPanel.classList.add("hidden");
    return;
  }
  el.chatTaskPanel.classList.remove("hidden");
  tasks.forEach((t) => addChatTaskItem(t));
}

export function renderUserCard(user) {
  if (!user) {
    el.userCard.classList.add("hidden");
    el.emailLoginBtn.classList.remove("logged-in");
    el.emailLoginBtn.textContent = "邮箱登录";
    el.authForms?.classList.remove("hidden");
    el.accountSummary?.classList.add("hidden");
    el.accountMenu?.classList.add("hidden");
    return;
  }

  const name = user.userName || user.name || user.email || "用户";
  el.emailLoginBtn.classList.add("logged-in");
  el.emailLoginBtn.innerHTML = `
    <span class="account-pill">
      <span class="avatar">${user.avatar ? `<img src="${escapeHtml(user.avatar)}" alt="avatar" />` : ""}</span>
      <span class="account-name">${escapeHtml(name)}</span>
    </span>
  `;
  el.authForms?.classList.add("hidden");
  if (el.accountSummary) {
    el.accountSummary.classList.remove("hidden");
    el.accountSummary.innerHTML = `
      <div class="name">${escapeHtml(name)}</div>
      <div>${escapeHtml(user.email || "")}</div>
    `;
  }
}

export function renderContextBox() {
  if (state.uiMode !== "doc" || !state.selectedTargets.length) {
    el.contextBox.classList.add("hidden");
    el.contextBox.innerHTML = "";
    return;
  }

  el.contextBox.classList.remove("hidden");
  el.contextBox.innerHTML = state.selectedTargets
    .map((item, idx) => {
      const preview = escapeHtml((item.originalText || "").slice(0, 40));
      return `
      <div class="context-item" data-target-id="${item.id}">
        <div class="context-title">
          <span>选区 ${idx + 1}: ${preview}${item.originalText.length > 40 ? "..." : ""}</span>
          <button type="button" class="chip-remove" data-remove-target-id="${item.id}">移除</button>
        </div>
        <div class="context-note-label">备注</div>
        <textarea class="context-note-input" data-note-target-id="${item.id}" placeholder="补充你的修改要求">${escapeHtml(item.note || "")}</textarea>
      </div>
    `;
    })
    .join("");
}

export function bindContextActions(onRemove, onUpdateNote) {
  el.contextBox.querySelectorAll("[data-remove-target-id]").forEach((btn) => {
    btn.onclick = () => onRemove(btn.getAttribute("data-remove-target-id"));
  });
  el.contextBox.querySelectorAll("[data-note-target-id]").forEach((input) => {
    input.oninput = () =>
      onUpdateNote(input.getAttribute("data-note-target-id"), input.value);
  });
}

export function showSelectionQuickAction(range) {
  const rect = range.getBoundingClientRect();
  if (!rect || (!rect.width && !rect.height)) return;
  el.selectionQuickAction.classList.remove("hidden");
  el.selectionQuickAction.style.top = `${Math.max(8, rect.top - 44 + window.scrollY)}px`;
  el.selectionQuickAction.style.left = `${rect.left + window.scrollX}px`;
}

export function hideSelectionQuickAction() {
  el.selectionQuickAction.classList.add("hidden");
}

export function showRewriteProposal(range, originalText, replacementText) {
  if (!range || !originalText.trim() || !replacementText.trim()) return;
  const wrapper = document.createElement("span");
  wrapper.className = "rewrite-proposal";
  wrapper.setAttribute("contenteditable", "false");

  const actions = document.createElement("div");
  actions.className = "rewrite-actions";
  actions.innerHTML = `<button class="accept" type="button">接受</button><button class="reject" type="button">拒绝</button>`;
  const oldNode = document.createElement("span");
  oldNode.className = "rewrite-old";
  oldNode.innerHTML = escapeHtml(originalText);
  const newNode = document.createElement("span");
  newNode.className = "rewrite-new";
  newNode.innerHTML = escapeHtml(replacementText);

  wrapper.appendChild(actions);
  wrapper.appendChild(oldNode);
  wrapper.appendChild(newNode);

  range.deleteContents();
  range.insertNode(wrapper);

  const apply = (accept) => {
    const textNode = document.createTextNode(
      accept ? replacementText : originalText,
    );
    wrapper.replaceWith(textNode);
  };
  actions.querySelector(".accept").onclick = () => apply(true);
  actions.querySelector(".reject").onclick = () => apply(false);
}
