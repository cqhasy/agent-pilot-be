import { el, state } from "../state.js";
import {
    bindContextActions,
    hideSelectionQuickAction,
    renderContextBox,
    showRewriteProposal,
    showSelectionQuickAction
} from "../ui.js";

function getEditorSelectionRange() {
    const selection = window.getSelection();
    if (!selection || selection.rangeCount === 0) return null;
    const range = selection.getRangeAt(0);
    if (!el.editor.contains(range.startContainer) || !el.editor.contains(range.endContainer) || range.collapsed) {
        return null;
    }
    return range.cloneRange();
}

export function refreshContextBox() {
    renderContextBox();
    bindContextActions(removeTarget, updateTargetNote);
}

export function updateEditorSelectionCache() {
    if (state.uiMode !== "doc") return;
    const range = getEditorSelectionRange();
    if (!range) {
        hideSelectionQuickAction();
        return;
    }
    const text = (window.getSelection()?.toString() || "").trim();
    if (!text) {
        hideSelectionQuickAction();
        return;
    }
    state.selectedEditorText = text;
    state.currentSelectionRange = range;
    showSelectionQuickAction(range);
}

export function markSelectionAsTarget() {
    if (state.uiMode !== "doc") return;
    const range = state.currentSelectionRange || getEditorSelectionRange();
    if (!range) return;
    const selectedText = (window.getSelection()?.toString() || state.selectedEditorText || "").trim();
    if (!selectedText) return;

    const wrapper = document.createElement("span");
    wrapper.className = "rewrite-target";
    wrapper.dataset.targetId = `t_${++state.targetSeq}`;
    wrapper.appendChild(range.extractContents());
    range.insertNode(wrapper);

    state.selectedTargets.push({
        id: wrapper.dataset.targetId,
        element: wrapper,
        originalText: selectedText,
        note: ""
    });
    window.getSelection()?.removeAllRanges();
    state.selectedEditorText = "";
    state.currentSelectionRange = null;
    hideSelectionQuickAction();
    refreshContextBox();
}

export function buildTargetPrompt(targets) {
    const details = targets.map((item, idx) => {
        const note = (item.note || "").trim() || "保持语义不变，提升表达清晰度";
        return `- 片段${idx + 1}: ${item.originalText}\n  建议：${note}`;
    }).join("\n");
    return `请结合以下局部片段和建议进行改写：\n${details}`;
}

export function applyReplacements(data, targets) {
    let replacements = Array.isArray(data?.suggestedReplacements) ? data.suggestedReplacements : [];
    if (!replacements.length && data?.suggestedReplacement) replacements = [data.suggestedReplacement];
    if (!replacements.length) return false;

    targets.forEach((target, idx) => {
        const replacement = (replacements[idx] || "").trim();
        if (!replacement || !target.element?.parentNode) return;
        const range = document.createRange();
        range.selectNode(target.element);
        showRewriteProposal(range, target.originalText, replacement);
    });
    state.selectedTargets = [];
    refreshContextBox();
    return true;
}

function removeTarget(targetId) {
    const idx = state.selectedTargets.findIndex((x) => x.id === targetId);
    if (idx < 0) return;
    const item = state.selectedTargets[idx];
    if (item.element?.parentNode) {
        item.element.replaceWith(document.createTextNode(item.originalText));
    }
    state.selectedTargets.splice(idx, 1);
    refreshContextBox();
}

function updateTargetNote(targetId, note) {
    const item = state.selectedTargets.find((x) => x.id === targetId);
    if (!item) return;
    item.note = note;
}
