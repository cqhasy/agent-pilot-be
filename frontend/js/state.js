export const state = {
    currentSessionId: "",
    sessions: [],
    conversation: [],
    uiMode: "chat",
    workspace: "chat",
    selectedTargets: [],
    selectedEditorText: "",
    currentSelectionRange: null,
    targetSeq: 0
};

export const el = {
    main: document.getElementById("main"),
    sessionList: document.getElementById("sessionList"),
    chatLog: document.getElementById("chatLog"),
    chatInput: document.getElementById("chatInput"),
    sendBtn: document.getElementById("sendBtn"),
    chatTaskPanel: document.getElementById("chatTaskPanel"),
    chatTaskList: document.getElementById("chatTaskList"),
    toggleTaskBtn: document.getElementById("toggleTaskBtn"),
    newSessionBtn: document.getElementById("newSessionBtn"),
    feishuLoginBtn: document.getElementById("feishuLoginBtn"),
    reloginBtn: document.getElementById("reloginBtn"),
    logoutBtn: document.getElementById("logoutBtn"),
    copyOpenIdBtn: document.getElementById("copyOpenIdBtn"),
    accountMenu: document.getElementById("accountMenu"),
    accountSummary: document.getElementById("accountSummary"),
    userCard: document.getElementById("userCard"),
    editorPanel: document.getElementById("editorPanel"),
    editor: document.getElementById("editor"),
    rewriteSelectionBtn: document.getElementById("rewriteSelectionBtn"),
    saveLarkBtn: document.getElementById("saveLarkBtn"),
    workspaceTag: document.getElementById("workspaceTag"),
    contextBox: document.getElementById("contextBox"),
    selectionQuickAction: document.getElementById("selectionQuickAction"),
    markSelectionBtn: document.getElementById("markSelectionBtn")
};
