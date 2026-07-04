(function () {
  "use strict";

  var LINT_DEBOUNCE_MS = 1500;
  var EDITOR_LINE_HEIGHT = 12 * 1.6;

  var els = {
    path: document.getElementById("file-preview-path"),
    meta: document.getElementById("file-preview-meta"),
    tbody: document.getElementById("file-preview-tbody"),
    table: document.getElementById("file-preview-table"),
    editorWrap: document.getElementById("file-preview-editor-wrap"),
    editorGutter: document.getElementById("file-preview-editor-gutter"),
    editor: document.getElementById("file-preview-editor"),
    editBtn: document.getElementById("file-preview-edit"),
    cancelEditBtn: document.getElementById("file-preview-cancel-edit"),
    saveBtn: document.getElementById("file-preview-save"),
    closeBtn: document.getElementById("file-preview-close"),
    footer: document.getElementById("file-preview-footer"),
    diagnostics: document.getElementById("file-preview-diagnostics"),
  };

  var filePath = new URLSearchParams(window.location.search).get("path") || "";
  var sessionId = new URLSearchParams(window.location.search).get("session_id") || "";
  var savedContent = "";
  var saving = false;
  var linting = false;
  var editable = false;
  var canEdit = false;
  var currentDiagnostics = [];
  var lintDebounceTimer = null;

  function fileName(fullPath) {
    return fullPath.split("/").pop() || fullPath || "预览";
  }

  function setPath(fullPath) {
    els.path.textContent = fullPath;
    els.path.title = fullPath;
    document.title = fileName(fullPath);
  }

  function setMeta(text) {
    els.meta.textContent = text;
  }

  function isShellFile() {
    return CodeView.detectLanguage(filePath) === "shell";
  }

  function hideDiagnostics() {
    if (!els.footer) {
      return;
    }
    els.footer.hidden = true;
    if (els.diagnostics) {
      els.diagnostics.innerHTML = "";
    }
  }

  function scrollEditorToLine(line) {
    if (!els.editor) {
      return;
    }
    var target = Math.max(0, (line - 1) * EDITOR_LINE_HEIGHT - els.editor.clientHeight / 2);
    els.editor.scrollTop = target;
    if (els.editorGutter) {
      els.editorGutter.scrollTop = els.editor.scrollTop;
    }
  }

  function scrollToLine(line) {
    if (editable && els.editor) {
      var lines = els.editor.value.split("\n");
      var pos = 0;
      for (var i = 0; i < line - 1 && i < lines.length; i++) {
        pos += lines[i].length + 1;
      }
      els.editor.focus();
      els.editor.setSelectionRange(pos, pos);
      scrollEditorToLine(line);
      return;
    }

    var row = els.tbody.querySelector('tr[data-line="' + line + '"]');
    if (row) {
      row.scrollIntoView({ block: "center", behavior: "smooth" });
      row.classList.add("code-line-error-focus");
      window.setTimeout(function () {
        row.classList.remove("code-line-error-focus");
      }, 1800);
    }
  }

  function showDiagnostics(diagnostics) {
    if (!els.footer || !els.diagnostics || !diagnostics.length) {
      hideDiagnostics();
      return;
    }

    els.footer.hidden = false;
    els.diagnostics.innerHTML = "";

    diagnostics.forEach(function (diag) {
      var item = document.createElement("button");
      item.type = "button";
      item.className = "preview-diagnostic-item";
      item.innerHTML =
        '<span class="preview-diagnostic-pos">第 ' +
        diag.line +
        " 行，第 " +
        diag.column +
        ' 列</span><span class="preview-diagnostic-msg">' +
        CodeView.escapeHtml(diag.message) +
        "</span>";
      item.addEventListener("click", function () {
        scrollToLine(diag.line);
      });
      els.diagnostics.appendChild(item);
    });
  }

  function updateEditorGutter() {
    if (!editable || !els.editorGutter || !els.editor) {
      return;
    }

    var lines = els.editor.value.split("\n");
    var lineCount = Math.max(1, lines.length);
    var errorLines = {};
    currentDiagnostics.forEach(function (diag) {
      errorLines[diag.line] = true;
    });

    var html = "";
    for (var i = 1; i <= lineCount; i++) {
      html +=
        '<div class="preview-editor-line' +
        (errorLines[i] ? " preview-editor-line-error" : "") +
        '">' +
        i +
        "</div>";
    }
    els.editorGutter.innerHTML = html;
    els.editorGutter.scrollTop = els.editor.scrollTop;
  }

  function updateToolbar() {
    if (els.editBtn) {
      els.editBtn.hidden = !canEdit || editable;
    }
    if (els.cancelEditBtn) {
      els.cancelEditBtn.hidden = !editable;
    }
    if (els.saveBtn) {
      els.saveBtn.hidden = !editable;
    }
  }

  function stopLintSchedule() {
    if (lintDebounceTimer) {
      window.clearTimeout(lintDebounceTimer);
      lintDebounceTimer = null;
    }
  }

  function applyLintResult(diagnostics) {
    currentDiagnostics = diagnostics || [];
    updateEditorGutter();
    showDiagnostics(currentDiagnostics);
    updateDirtyState();
  }

  function runLint() {
    if (!editable || !isShellFile() || linting || !sessionId || !filePath || !els.editor) {
      return Promise.resolve();
    }

    linting = true;
    return fetch(
      "/api/workspace/lint?path=" +
        encodeURIComponent(filePath) +
        "&session_id=" +
        encodeURIComponent(sessionId),
      {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: els.editor.value }),
      }
    )
      .then(function (res) {
        if (res.status === 401) {
          throw new Error("未登录，请先在主页面登录");
        }
        return res.json().then(function (body) {
          if (!res.ok) {
            throw new Error(body && body.error ? body.error : "请求失败 (" + res.status + ")");
          }
          return body;
        });
      })
      .then(function (data) {
        applyLintResult(data.diagnostics || []);
      })
      .catch(function (err) {
        console.warn("语法检查失败:", err.message);
      })
      .finally(function () {
        linting = false;
      });
  }

  function scheduleLint() {
    if (!editable || !isShellFile()) {
      return;
    }
    if (lintDebounceTimer) {
      window.clearTimeout(lintDebounceTimer);
    }
    lintDebounceTimer = window.setTimeout(function () {
      lintDebounceTimer = null;
      runLint();
    }, LINT_DEBOUNCE_MS);
  }

  function startLintSchedule() {
    stopLintSchedule();
    if (!editable || !isShellFile()) {
      return;
    }
    runLint();
  }

  function setEditMode(on) {
    editable = on;
    if (els.editorWrap) {
      els.editorWrap.hidden = !on;
    }
    if (els.table) {
      els.table.hidden = on;
    }
    if (on) {
      updateEditorGutter();
      startLintSchedule();
    } else {
      stopLintSchedule();
    }
    updateToolbar();
  }

  function showLoading() {
    setPath(filePath);
    setMeta("加载中…");
    hideDiagnostics();
    canEdit = false;
    setEditMode(false);
    updateToolbar();
    els.tbody.innerHTML =
      '<tr><td class="code-line-num">1</td><td class="code-line-content code-line-loading">加载中…</td></tr>';
  }

  function showError(message) {
    setPath(filePath);
    setMeta("错误");
    hideDiagnostics();
    canEdit = false;
    setEditMode(false);
    updateToolbar();
    els.tbody.innerHTML =
      '<tr><td class="code-line-num">1</td><td class="code-line-content code-line-error">' +
      CodeView.escapeHtml(message) +
      "</td></tr>";
  }

  function buildMeta(lang, rendered, truncated, diagnostics, dirty) {
    var label = CodeView.LANG_LABELS[lang] || lang;
    var parts = [label, rendered.lineCount + " 行"];
    if (truncated) {
      parts.push("只读");
    } else if (editable && dirty) {
      parts.push("未保存");
    } else if (editable) {
      parts.push("编辑中");
    }
    if (lang === "shell" && !truncated) {
      if (diagnostics && diagnostics.length) {
        parts.push(diagnostics.length + " 处语法错误");
      } else {
        parts.push("语法正确");
      }
    }
    return parts.join(" · ");
  }

  function updateDirtyState() {
    if (!editable || !els.editor) {
      return;
    }
    var dirty = els.editor.value !== savedContent;
    var lang = CodeView.detectLanguage(filePath);
    var lineCount = els.editor.value.split("\n").length;
    setMeta(buildMeta(lang, { lineCount: lineCount }, false, currentDiagnostics, dirty));
    if (els.saveBtn) {
      els.saveBtn.disabled = saving;
      els.saveBtn.textContent = dirty ? "保存" : isShellFile() ? "检查" : "保存";
      els.saveBtn.title = dirty ? "保存 (Ctrl+S)" : isShellFile() ? "检查语法 (Ctrl+S)" : "保存 (Ctrl+S)";
    }
  }

  function renderPreview(content, truncated, diagnostics) {
    diagnostics = diagnostics || [];
    currentDiagnostics = diagnostics;
    setEditMode(false);

    var lang = CodeView.detectLanguage(filePath);
    var formatted = CodeView.maybeFormatContent(content, lang);
    var rendered = CodeView.renderCodeTable(formatted, lang, diagnostics);

    setPath(filePath);
    setMeta(buildMeta(lang, rendered, truncated, diagnostics, false));
    els.tbody.innerHTML = "";
    els.tbody.appendChild(rendered.fragment);
    showDiagnostics(diagnostics);
    updateToolbar();

    if (diagnostics.length) {
      scrollToLine(diagnostics[0].line);
    }
  }

  function openFile(content, truncated, diagnostics, allowEdit) {
    savedContent = content;
    canEdit = !!allowEdit && !truncated;
    renderPreview(content, truncated, diagnostics);
  }

  function enterEditMode() {
    if (!canEdit || editable) {
      return;
    }
    setEditMode(true);
    if (els.editor) {
      els.editor.value = savedContent;
      els.editor.focus();
    }
    updateEditorGutter();
    updateDirtyState();
    showDiagnostics(currentDiagnostics);
  }

  function exitEditMode() {
    if (!editable) {
      return;
    }
    if (els.editor && els.editor.value !== savedContent) {
      if (!window.confirm("有未保存的更改，确定返回预览？")) {
        return;
      }
    }
    renderPreview(savedContent, false, currentDiagnostics);
  }

  function saveFile() {
    if (!editable || saving || !sessionId || !filePath) {
      return Promise.resolve();
    }
    if (els.editor.value === savedContent) {
      return runLint();
    }

    saving = true;
    if (els.saveBtn) {
      els.saveBtn.disabled = true;
      els.saveBtn.textContent = "保存中…";
    }

    return fetch(
      "/api/workspace/file?path=" +
        encodeURIComponent(filePath) +
        "&session_id=" +
        encodeURIComponent(sessionId),
      {
        method: "PUT",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: els.editor.value }),
      }
    )
      .then(function (res) {
        if (res.status === 401) {
          throw new Error("未登录，请先在主页面登录");
        }
        return res.json().then(function (body) {
          if (!res.ok) {
            throw new Error(body && body.error ? body.error : "请求失败 (" + res.status + ")");
          }
          return body;
        });
      })
      .then(function (data) {
        savedContent = data.content || els.editor.value;
        applyLintResult(data.diagnostics || []);
      })
      .catch(function (err) {
        window.alert("保存失败: " + err.message);
      })
      .finally(function () {
        saving = false;
        updateDirtyState();
      });
  }

  if (els.editBtn) {
    els.editBtn.addEventListener("click", enterEditMode);
  }

  if (els.cancelEditBtn) {
    els.cancelEditBtn.addEventListener("click", exitEditMode);
  }

  if (els.saveBtn) {
    els.saveBtn.addEventListener("click", function () {
      saveFile();
    });
  }

  if (els.editor) {
    els.editor.addEventListener("input", function () {
      updateEditorGutter();
      updateDirtyState();
      scheduleLint();
    });
    els.editor.addEventListener("scroll", function () {
      if (els.editorGutter) {
        els.editorGutter.scrollTop = els.editor.scrollTop;
      }
    });
  }

  els.closeBtn.addEventListener("click", function () {
    if (editable && els.editor && els.editor.value !== savedContent) {
      if (!window.confirm("有未保存的更改，确定关闭？")) {
        return;
      }
    }
    window.close();
  });

  document.addEventListener("keydown", function (event) {
    if ((event.ctrlKey || event.metaKey) && event.key === "s") {
      if (editable) {
        event.preventDefault();
        saveFile();
      }
      return;
    }
    if (event.key === "Escape") {
      if (editable) {
        exitEditMode();
        return;
      }
      els.closeBtn.click();
    }
  });

  if (!filePath) {
    showError("未指定文件路径");
    return;
  }
  if (!sessionId) {
    showError("未指定会话");
    return;
  }

  showLoading();

  fetch(
    "/api/workspace/file?path=" +
      encodeURIComponent(filePath) +
      "&session_id=" +
      encodeURIComponent(sessionId),
    { credentials: "include" }
  )
    .then(function (res) {
      if (res.status === 401) {
        throw new Error("未登录，请先在主页面登录");
      }
      return res.json().then(function (body) {
        if (!res.ok) {
          throw new Error(body && body.error ? body.error : "请求失败 (" + res.status + ")");
        }
        return body;
      });
    })
    .then(function (data) {
      var allowEdit = !data.truncated && !data.read_only;
      openFile(data.content || "", !!data.truncated, data.diagnostics || [], allowEdit);
    })
    .catch(function (err) {
      showError("无法预览: " + err.message);
    });
})();
