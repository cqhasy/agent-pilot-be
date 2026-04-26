import { getJSON, postJSON } from "./api.js";
import { el, state } from "./state.js";
import {
    renderConversation,
    renderLayout,
    renderSessions,
    renderUserCard,
    clearChatTaskPanel,
    setChatTaskList
} from "./ui.js";
import {
    applyReplacements,
    buildTargetPrompt,
    markSelectionAsTarget,
    refreshContextBox,
    updateEditorSelectionCache
} from "./features/doc-rewrite.js";

async function fetchSessions() {
    const res = await getJSON("/api/sessions");
    state.sessions = res.data || [];
    renderSessions();
    el.sessionList.querySelectorAll(".session-item").forEach((item) => {
        item.onclick = () => loadSession(item.dataset.sessionId);
    });
}

async function loadSession(id) {
    state.currentSessionId = id;
    renderSessions();
    el.sessionList.querySelectorAll(".session-item").forEach((item) => {
        item.onclick = () => loadSession(item.dataset.sessionId);
    });
    const res = await getJSON(`/api/session/detail?sessionId=${encodeURIComponent(id)}`);
    const data = res.data;
    state.conversation = data.messages || [];
    renderConversation(state.conversation);
    state.workspace = data.session.workspace || "chat";
    state.uiMode = state.workspace === "doc" ? "doc" : "chat";
    if (state.uiMode === "doc") el.editor.innerText = data.session.document || "";
    renderLayout();
    state.selectedTargets = [];
    refreshContextBox();
    // 会话加载时：任务拆解只在 AI 助手框展示
    if (Array.isArray(data.session.latestTaskPlan) && data.session.latestTaskPlan.length) {
        setChatTaskList(data.session.latestTaskPlan);
    } else {
        clearChatTaskPanel();
    }
}

async function sendMessage(message, options = {}) {
    // 流式接口：任务拆解/回复动态展示
    clearChatTaskPanel();
    renderConversation([...state.conversation, { role: "user", content: message }]);

    const payload = {
        sessionId: state.currentSessionId,
        message,
        currentDoc: el.editor.innerText,
        selectedText: options.selectedText || "",
        selectedTexts: options.selectedTexts || []
    };

    let assistantText = "";
    // 先追加一个空 assistant bubble，用 delta 填充
    const tempConversation = [...state.conversation, { role: "user", content: message }, { role: "assistant", content: "" }];
    renderConversation(tempConversation);

    const res = await fetch("/api/v1/chat/stream", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
    });
    if (!res.ok || !res.body) {
        // fallback 非流式
        const fallback = await postJSON("/api/chat", payload);
        return applyFinalResponse(fallback.data, options);
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder("utf-8");
    let buffer = "";
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
            const data = JSON.parse(dataLine);
            if (event === "task_plan") {
                setChatTaskList(Array.isArray(data) ? data : []);
            } else if (event === "assistant_delta") {
                assistantText += data.delta || "";
                tempConversation[tempConversation.length - 1].content = assistantText;
                renderConversation(tempConversation);
            } else if (event === "final") {
                return applyFinalResponse(data, options);
            }
        }
    }
}

function applyFinalResponse(data, options) {
    state.currentSessionId = data.session.id;
    state.conversation = data.conversation || [];
    state.uiMode = data.uiMode || "chat";
    state.workspace = data.session.workspace || (data.uiMode === "doc" ? "doc" : "chat");
    renderConversation(state.conversation);
    // 对话框展示任务拆解（可为空）
    if (data.taskPlan && data.taskPlan.length) {
        setChatTaskList(data.taskPlan);
    } else {
        clearChatTaskPanel();
    }
    renderLayout();
    if (state.uiMode === "doc" && options.applyDocument !== false) el.editor.innerText = data.newDocument || "";
    fetchSessions();
    return data;
}

async function sendFromInput() {
    const text = el.chatInput.value.trim();
    if (!text) return;
    const targets = state.uiMode === "doc" ? state.selectedTargets.filter((t) => t.element?.parentNode) : [];
    const composed = targets.length ? `${text}\n\n${buildTargetPrompt(targets)}` : text;
    el.chatInput.value = "";
    const selectedTexts = targets.map((t) => (t.element.textContent || t.originalText || "").trim());
    const data = await sendMessage(composed, { selectedText: selectedTexts[0] || "", selectedTexts, applyDocument: !targets.length });
    if (targets.length) {
        const ok = applyReplacements(data, targets);
        if (!ok) alert("AI 未返回片段替换建议，请重试。");
    }
}

async function rewriteSelectionNow() {
    const targets = state.selectedTargets.filter((t) => t.element?.parentNode);
    if (!targets.length) {
        markSelectionAsTarget();
    }
    const finalTargets = state.selectedTargets.filter((t) => t.element?.parentNode);
    if (!finalTargets.length) {
        alert("请先在文档中选中文本并点击“✓ 加入改写”。");
        return;
    }
    const selectedTexts = finalTargets.map((t) => (t.element.textContent || t.originalText || "").trim());
    const prompt = buildTargetPrompt(finalTargets);
    const data = await sendMessage(prompt, { selectedText: selectedTexts[0], selectedTexts, applyDocument: false });
    const ok = applyReplacements(data, finalTargets);
    if (!ok) alert("AI 本次未返回可替换内容，请重试。");
}

async function saveToLark() {
    const title = prompt("请输入飞书文档标题", "AI协作文档");
    if (!title) return;
    const res = await postJSON("/api/save-lark", { sessionId: state.currentSessionId, title, content: el.editor.innerText });
    alert(res.data?.message || "已完成保存请求");
}

async function loginWithFeishu() {
    const returnTo = encodeURIComponent(window.location.pathname);
    const res = await getJSON(`/api/auth/feishu/login?returnTo=${returnTo}`);
    const payload = res.data?.data ?? res.data;
    if (!payload?.authUrl) {
        alert(res.data?.message || "飞书登录初始化失败，请检查环境变量");
        return;
    }
    window.location.href = payload.authUrl;
}

async function fetchMe() {
    const res = await getJSON("/api/auth/me");
    if (!res.ok) {
        renderUserCard(null);
        return;
    }
    const payload = res.data?.data ?? res.data;
    renderUserCard(payload);
}

async function logout() {
    const res = await postJSON("/api/auth/logout", {});
    if (!res.ok) {
        alert(res.data?.message || "退出失败");
        return;
    }
    localStorage.removeItem("authToken");
    renderUserCard(null);
    el.accountMenu?.classList.add("hidden");
    alert("已退出登录");
}

function handleLoginResultFromQuery() {
    const query = new URLSearchParams(window.location.search);
    const token = query.get("token");
    const login = query.get("login");
    const message = query.get("message");
    if (token) {
        localStorage.setItem("authToken", token);
        alert("飞书登录成功");
    }
    if (login === "success") alert("飞书登录成功");
    if (login === "failed") alert(`飞书登录失败：${message || "未知错误"}`);
    if (token || login) window.history.replaceState({}, "", window.location.pathname);
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
    el.editor.innerText = "当 AI 识别到文档任务时，这里会自动打开。";
    renderSessions();
    clearChatTaskPanel();
}

function wireEvents() {
    el.sendBtn.onclick = sendFromInput;
    el.newSessionBtn.onclick = resetSession;
    el.feishuLoginBtn.onclick = () => {
        // 已登录时作为菜单开关；未登录时走登录
        if (!el.feishuLoginBtn.classList.contains("logged-in")) {
            loginWithFeishu();
            return;
        }
        el.accountMenu?.classList.toggle("hidden");
    };
    el.reloginBtn && (el.reloginBtn.onclick = loginWithFeishu);
    el.logoutBtn && (el.logoutBtn.onclick = logout);
    el.copyOpenIdBtn && (el.copyOpenIdBtn.onclick = async () => {
        const res = await getJSON("/api/auth/me");
        if (!res.ok) return;
        const payload = res.data?.data ?? res.data;
        const openId = payload?.openId || "";
        if (!openId) return;
        await navigator.clipboard.writeText(openId);
        alert("已复制 OpenID");
    });
    document.addEventListener("click", (e) => {
        if (!el.accountMenu || el.accountMenu.classList.contains("hidden")) return;
        const target = e.target;
        if (target === el.feishuLoginBtn || el.feishuLoginBtn.contains(target)) return;
        if (el.accountMenu.contains(target)) return;
        el.accountMenu.classList.add("hidden");
    });
    el.toggleTaskBtn && (el.toggleTaskBtn.onclick = () => {
        if (el.chatTaskList.style.display === "none") {
            el.chatTaskList.style.display = "";
            el.toggleTaskBtn.textContent = "收起";
        } else {
            el.chatTaskList.style.display = "none";
            el.toggleTaskBtn.textContent = "展开";
        }
    });
    el.saveLarkBtn.onclick = saveToLark;
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
    handleLoginResultFromQuery();
    await Promise.all([fetchMe(), fetchSessions()]);
}

bootstrap();
