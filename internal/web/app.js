(function () {
  "use strict";

  var FETCH_CREDENTIALS = { credentials: "include" };

  function copyTextFallback(text) {
    return new Promise(function (resolve, reject) {
      var textarea = document.createElement("textarea");
      textarea.value = text;
      textarea.setAttribute("readonly", "");
      textarea.style.position = "fixed";
      textarea.style.left = "-9999px";
      document.body.appendChild(textarea);
      textarea.select();
      textarea.setSelectionRange(0, text.length);
      var ok = false;
      try {
        ok = document.execCommand("copy");
      } catch (e) {
        ok = false;
      }
      document.body.removeChild(textarea);
      if (ok) {
        resolve();
      } else {
        reject(new Error("copy failed"));
      }
    });
  }

  function copyTextToClipboard(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).catch(function () {
        return copyTextFallback(text);
      });
    }
    return copyTextFallback(text);
  }

  var authPanel = (function () {
    var els = {
      loginView: document.getElementById("view-login"),
      appRoot: document.getElementById("app-root"),
      form: document.getElementById("login-form"),
      username: document.getElementById("login-username"),
      password: document.getElementById("login-password"),
      passwordToggle: document.getElementById("login-password-toggle"),
      error: document.getElementById("login-error"),
      submit: document.getElementById("login-submit"),
    };

    var authenticated = false;
    var onAuthenticated = null;

    function showLogin(message) {
      authenticated = false;
      els.appRoot.hidden = true;
      els.loginView.hidden = false;
      if (message) {
        els.error.textContent = message;
        els.error.hidden = false;
      } else {
        els.error.hidden = true;
        els.error.textContent = "";
      }
    }

    function showApp() {
      authenticated = true;
      els.loginView.hidden = true;
      els.appRoot.hidden = false;
      els.error.hidden = true;
      if (typeof onAuthenticated === "function") {
        onAuthenticated();
        onAuthenticated = null;
      }
    }

    function setSubmitting(loading) {
      els.submit.disabled = loading;
      els.username.disabled = loading;
      els.password.disabled = loading;
      if (els.passwordToggle) {
        els.passwordToggle.disabled = loading;
      }
    }

    function setPasswordVisible(visible) {
      els.password.type = visible ? "text" : "password";
      if (!els.passwordToggle) {
        return;
      }
      els.passwordToggle.classList.toggle("is-visible", visible);
      els.passwordToggle.setAttribute("aria-pressed", visible ? "true" : "false");
      els.passwordToggle.setAttribute("aria-label", visible ? "隐藏密码" : "显示密码");
      els.passwordToggle.title = visible ? "隐藏密码" : "显示密码";
    }

    function login(username, password) {
      setSubmitting(true);
      els.error.hidden = true;
      return fetch("/api/auth/login", Object.assign({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username, password: password }),
      }, FETCH_CREDENTIALS)).then(function (res) {
        return res.json().then(function (body) {
          if (!res.ok) {
            throw new Error(body && body.error ? body.error : "登录失败 (" + res.status + ")");
          }
          return body;
        });
      }).then(function () {
        showApp();
      }).catch(function (err) {
        els.error.textContent = err.message || "登录失败";
        els.error.hidden = false;
      }).finally(function () {
        setSubmitting(false);
      });
    }

    function checkSession() {
      return fetch("/api/auth/me", FETCH_CREDENTIALS).then(function (res) {
        if (res.status === 401) {
          showLogin();
          return false;
        }
        if (!res.ok) {
          showLogin("无法验证登录状态");
          return false;
        }
        showApp();
        return true;
      }).catch(function () {
        showLogin("无法连接服务器");
        return false;
      });
    }

    function requireLogin(message) {
      showLogin(message || "登录已过期，请重新登录");
    }

    function isAuthenticated() {
      return authenticated;
    }

    els.form.addEventListener("submit", function (event) {
      event.preventDefault();
      login(els.username.value.trim(), els.password.value);
    });

    if (els.passwordToggle) {
      els.passwordToggle.addEventListener("click", function () {
        setPasswordVisible(els.password.type === "password");
      });
    }

    return {
      checkSession: checkSession,
      requireLogin: requireLogin,
      isAuthenticated: isAuthenticated,
      setOnAuthenticated: function (fn) {
        onAuthenticated = fn;
      },
    };
  })();

  var views = {
    chat: document.getElementById("view-chat"),
    settings: document.getElementById("view-settings"),
  };

  var SESSION_URL_PARAM = "session";
  var VIEW_URL_PARAM = "view";
  var SETTINGS_TAB_PARAM = "tab";

  function readUrlState() {
    var params = new URLSearchParams(window.location.search);
    var settingsTab = params.get(SETTINGS_TAB_PARAM) || "llm";
    var view = params.get(VIEW_URL_PARAM);
    if (!view) {
      view = settingsTab !== "llm" ? "settings" : "chat";
    }
    return {
      sessionId: params.get(SESSION_URL_PARAM),
      view: view,
      settingsTab: settingsTab,
    };
  }

  function writeUrlState(patch, replace) {
    var url = new URL(window.location.href);
    if (patch.sessionId !== undefined) {
      if (patch.sessionId) {
        url.searchParams.set(SESSION_URL_PARAM, patch.sessionId);
      } else {
        url.searchParams.delete(SESSION_URL_PARAM);
      }
    }
    if (patch.view !== undefined) {
      if (patch.view && patch.view !== "chat") {
        url.searchParams.set(VIEW_URL_PARAM, patch.view);
      } else {
        url.searchParams.delete(VIEW_URL_PARAM);
        url.searchParams.delete(SETTINGS_TAB_PARAM);
      }
    }
    if (patch.settingsTab !== undefined) {
      if (patch.settingsTab && patch.settingsTab !== "llm") {
        url.searchParams.set(SETTINGS_TAB_PARAM, patch.settingsTab);
      } else {
        url.searchParams.delete(SETTINGS_TAB_PARAM);
      }
    }
    var next = url.pathname + url.search;
    var method = replace ? "replaceState" : "pushState";
    window.history[method]({ sessionId: patch.sessionId, view: patch.view }, "", next);
  }

  function isLlmConfigured(cfg) {
    if (!cfg || !cfg.llm || !cfg.llm.active) {
      return false;
    }
    var inst = (cfg.llm.providers || {})[cfg.llm.active];
    if (!inst || !inst.enabled) {
      return false;
    }
    if (cfg.llm.active === "ollama") {
      return true;
    }
    return !!inst.api_key;
  }

  function isProviderInUse(cfg, code) {
    if (!cfg || !cfg.llm || cfg.llm.active !== code) {
      return false;
    }
    var inst = (cfg.llm.providers || {})[code];
    return !!(inst && inst.enabled);
  }

  function activateView(name) {
    Object.keys(views).forEach(function (key) {
      var view = views[key];
      var isActive = key === name;
      view.classList.toggle("active", isActive);
      view.hidden = !isActive;
    });

    writeUrlState({ view: name }, true);

    if (name === "settings") {
      settingsPanel.ensureLoaded();
    } else if (name === "chat" && chatPanel && chatPanel.refreshLlmState) {
      chatPanel.refreshLlmState();
    }
  }

  function openLlmSettings() {
    activateView("settings");
    settingsPanel.activateTab("llm");
  }

  document.querySelectorAll("[data-view]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      activateView(btn.dataset.view);
    });
  });

  document.getElementById("btn-back-from-settings").addEventListener("click", function () {
    activateView("chat");
  });

  function apiFetch(path, options) {
    options = options || {};
    if (!options.credentials) {
      options.credentials = "include";
    }
    return fetch(path, options).then(function (res) {
      if (res.status === 401 && authPanel.isAuthenticated()) {
        authPanel.requireLogin();
      }
      return res.json().then(function (body) {
        if (!res.ok) {
          var err = new Error(body && body.error ? body.error : "请求失败 (" + res.status + ")");
          err.status = res.status;
          throw err;
        }
        return body;
      });
    });
  }

  function formatRelativeTime(iso) {
    if (!iso) {
      return "";
    }
    var then = new Date(iso).getTime();
    var diff = Date.now() - then;
    var mins = Math.floor(diff / 60000);
    if (mins < 1) {
      return "刚刚";
    }
    if (mins < 60) {
      return mins + " 分钟前";
    }
    var hours = Math.floor(mins / 60);
    if (hours < 24) {
      return hours + " 小时前";
    }
    var days = Math.floor(hours / 24);
    if (days < 30) {
      return days + " 天前";
    }
    return new Date(iso).toLocaleDateString("zh-CN");
  }

  function escapeHtml(text) {
    return String(text)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  var confirmDialog = (function () {
    var els = {
      root: document.getElementById("confirm-dialog"),
      backdrop: document.getElementById("confirm-dialog-backdrop"),
      icon: document.getElementById("confirm-dialog-icon"),
      title: document.getElementById("confirm-dialog-title"),
      message: document.getElementById("confirm-dialog-message"),
      detail: document.getElementById("confirm-dialog-detail"),
      cancelBtn: document.getElementById("confirm-dialog-cancel"),
      confirmBtn: document.getElementById("confirm-dialog-confirm"),
    };
    var open = false;
    var pendingResolve = null;

    function setVisible(visible) {
      open = visible;
      els.root.hidden = !visible;
      document.body.classList.toggle("provider-modal-open", visible);
    }

    function finish(result) {
      if (!open) {
        return;
      }
      setVisible(false);
      var resolve = pendingResolve;
      pendingResolve = null;
      if (resolve) {
        resolve(result);
      }
    }

    function openDialog(options) {
      options = options || {};
      if (!els.root) {
        return Promise.resolve(!!options.defaultConfirm);
      }
      if (open) {
        finish(false);
      }

      els.title.textContent = options.title || "确认操作";
      if (options.messageHtml) {
        els.message.innerHTML = options.messageHtml;
      } else {
        els.message.textContent = options.message || "";
      }
      if (options.detail) {
        els.detail.textContent = options.detail;
        els.detail.hidden = false;
      } else {
        els.detail.textContent = "";
        els.detail.hidden = true;
      }
      els.cancelBtn.textContent = options.cancelLabel || "取消";
      els.confirmBtn.textContent = options.confirmLabel || "确认";
      els.confirmBtn.className = options.danger === false ? "btn btn-primary" : "btn btn-danger-solid";

      setVisible(true);
      window.setTimeout(function () {
        els.cancelBtn.focus();
      }, 0);

      return new Promise(function (resolve) {
        pendingResolve = resolve;
      });
    }

    els.cancelBtn.addEventListener("click", function () {
      finish(false);
    });
    els.confirmBtn.addEventListener("click", function () {
      finish(true);
    });
    if (els.backdrop) {
      els.backdrop.addEventListener("click", function () {
        finish(false);
      });
    }
    document.addEventListener("keydown", function (event) {
      if (!open) {
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        finish(false);
      }
    });

    return { open: openDialog };
  })();

  var sessionSidebar = (function () {
    var els = {
      list: document.getElementById("session-list"),
      newBtn: document.getElementById("btn-new-chat"),
      title: document.getElementById("chat-title"),
    };
    var sessions = [];
    var activeId = null;
    var onSelect = null;
    var onDelete = null;

    function render() {
      els.list.innerHTML = "";
      if (!sessions.length) {
        var empty = document.createElement("p");
        empty.className = "session-empty";
        empty.textContent = "暂无历史对话";
        els.list.appendChild(empty);
        return;
      }

      sessions.forEach(function (item) {
        var row = document.createElement("div");
        row.className = "session-row";

        var btn = document.createElement("button");
        btn.type = "button";
        btn.className = "session-item";
        btn.dataset.sessionId = item.id;
        btn.setAttribute("role", "option");
        if (item.id === activeId) {
          btn.classList.add("active");
          btn.setAttribute("aria-selected", "true");
        }

        var title = document.createElement("span");
        title.className = "session-item-title";
        title.textContent = item.title || "新对话";

        var meta = document.createElement("span");
        meta.className = "session-item-meta";
        meta.textContent = formatRelativeTime(item.updated_at);

        btn.appendChild(title);
        btn.appendChild(meta);
        btn.addEventListener("click", function () {
          if (onSelect) {
            onSelect(item.id);
          }
        });
        row.appendChild(btn);

        var delBtn = document.createElement("button");
        delBtn.type = "button";
        delBtn.className = "session-delete";
        delBtn.title = "删除对话";
        delBtn.setAttribute("aria-label", "删除 " + (item.title || "新对话"));
        delBtn.innerHTML =
          '<svg width="12" height="12" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>';
        delBtn.addEventListener("click", function (event) {
          event.stopPropagation();
          var label = item.title || "新对话";
          confirmDialog
            .open({
              title: "删除对话",
              messageHtml:
                '确定删除对话 <strong>「' +
                escapeHtml(label) +
                "」</strong> 吗？",
              detail: "此操作不可恢复，对话记录与工作区文件将被永久删除。",
              confirmLabel: "删除",
              cancelLabel: "取消",
              danger: true,
            })
            .then(function (confirmed) {
              if (confirmed && onDelete) {
                onDelete(item.id);
              }
            });
        });
        row.appendChild(delBtn);

        els.list.appendChild(row);
      });
    }

    function setActive(id) {
      activeId = id;
      render();
      var current = null;
      for (var i = 0; i < sessions.length; i++) {
        if (sessions[i].id === id) {
          current = sessions[i];
          break;
        }
      }
      if (current) {
        els.title.textContent = current.title || "新对话";
      }
    }

    function setTitle(title) {
      els.title.textContent = title || "新对话";
    }

    function refresh() {
      return apiFetch("/api/sessions").then(function (data) {
        sessions = data.sessions || [];
        render();
        if (activeId) {
          setActive(activeId);
        }
      });
    }

    els.newBtn.addEventListener("click", function () {
      if (window.__chatPanelNewSession) {
        window.__chatPanelNewSession();
      }
    });

    return {
      refresh: refresh,
      setActive: setActive,
      setTitle: setTitle,
      onSelect: function (fn) {
        onSelect = fn;
      },
      onDelete: function (fn) {
        onDelete = fn;
      },
    };
  })();

  function formatInlineMarkdown(escaped) {
    return escaped
      .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
      .replace(/\*([^*\n]+)\*/g, "<em>$1</em>")
      .replace(/`([^`\n]+)`/g, '<code class="md-inline-code">$1</code>')
      .replace(/\[([^\]]+)\]\((https?:\/\/[^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
  }

  function isMarkdownTableRow(line) {
    return line.trim().charAt(0) === "|";
  }

  function isMarkdownTableSeparator(line) {
    if (!isMarkdownTableRow(line)) {
      return false;
    }
    return line.replace(/\|/g, "").replace(/[\s:-]/g, "") === "";
  }

  function isMarkdownHorizontalRule(line) {
    return /^(\*{3,}|-{3,}|_{3,})\s*$/.test((line || "").trim());
  }

  function parseMarkdownTableCells(line) {
    return line
      .trim()
      .replace(/^\|/, "")
      .replace(/\|$/, "")
      .split("|")
      .map(function (cell) {
        return cell.trim();
      });
  }

  function renderMarkdownTable(tableLines) {
    var headerCells = parseMarkdownTableCells(tableLines[0]);
    var html = ['<table class="md-table"><thead><tr>'];
    headerCells.forEach(function (cell) {
      html.push("<th>" + formatInlineMarkdown(escapeHtml(cell)) + "</th>");
    });
    html.push("</tr></thead><tbody>");
    for (var i = 2; i < tableLines.length; i++) {
      if (!isMarkdownTableRow(tableLines[i]) || isMarkdownTableSeparator(tableLines[i])) {
        continue;
      }
      var cells = parseMarkdownTableCells(tableLines[i]);
      html.push("<tr>");
      cells.forEach(function (cell) {
        html.push("<td>" + formatInlineMarkdown(escapeHtml(cell)) + "</td>");
      });
      html.push("</tr>");
    }
    html.push("</tbody></table>");
    return html.join("");
  }

  function renderMarkdownLines(lines) {
    var html = [];
    var inList = false;
    var index = 0;

    function closeList() {
      if (inList) {
        html.push("</ul>");
        inList = false;
      }
    }

    while (index < lines.length) {
      var line = lines[index];
      var trimmed = line.trim();

      if (
        isMarkdownTableRow(line) &&
        index + 1 < lines.length &&
        isMarkdownTableSeparator(lines[index + 1])
      ) {
        closeList();
        var tableLines = [];
        while (index < lines.length && isMarkdownTableRow(lines[index])) {
          tableLines.push(lines[index]);
          index++;
        }
        html.push(renderMarkdownTable(tableLines));
        continue;
      }

      var listMatch = trimmed.match(/^([-*]|\d+\.)\s+(.+)$/);
      if (listMatch) {
        if (!inList) {
          html.push('<ul class="md-list">');
          inList = true;
        }
        html.push("<li>" + formatInlineMarkdown(escapeHtml(listMatch[2])) + "</li>");
        index++;
        continue;
      }

      closeList();

      if (!trimmed) {
        if (index < lines.length - 1) {
          html.push('<div class="md-spacer"></div>');
        }
        index++;
        continue;
      }

      if (isMarkdownHorizontalRule(trimmed)) {
        closeList();
        html.push('<hr class="md-hr" />');
        index++;
        continue;
      }

      var headingMatch = trimmed.match(/^(#{1,4})\s+(.+)$/);
      if (headingMatch) {
        var level = headingMatch[1].length;
        html.push(
          '<h' +
            level +
            ' class="md-heading md-h' +
            level +
            '">' +
            formatInlineMarkdown(escapeHtml(headingMatch[2])) +
            "</h" +
            level +
            ">"
        );
        index++;
        continue;
      }

      html.push('<p class="md-p">' + formatInlineMarkdown(escapeHtml(line)) + "</p>");
      index++;
    }

    closeList();
    return html;
  }

  function renderMarkdownBlock(text) {
    if (!text) {
      return "";
    }
    var parts = String(text).split(/(```[\s\S]*?```)/g);
    var html = [];

    parts.forEach(function (part) {
      if (!part) {
        return;
      }
      if (part.indexOf("```") === 0) {
        var match = part.match(/^```(\w*)\n?([\s\S]*?)```$/);
        if (match) {
          var lang = match[1] ? ' data-lang="' + escapeHtml(match[1]) + '"' : "";
          html.push(
            '<pre class="md-code-block"' +
              lang +
              "><code>" +
              escapeHtml(match[2].replace(/\n$/, "")) +
              "</code></pre>"
          );
        }
        return;
      }

      html.push.apply(html, renderMarkdownLines(part.split("\n")));
    });

    return html.join("");
  }

  function formatMessageTime(iso) {
    if (!iso) {
      return "";
    }
    var date = new Date(iso);
    if (isNaN(date.getTime())) {
      return "";
    }
    return date.toLocaleString("zh-CN", {
      month: "numeric",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  function sessionShareUrl(sessionId) {
    var url = new URL(window.location.href);
    url.search = "";
    url.searchParams.set(SESSION_URL_PARAM, sessionId);
    return url.toString();
  }

  var PREVIEW_WINDOW = "databuff-file-preview";

  var filesPanel = (function () {
    var els = {
      root: document.getElementById("workspace-root"),
      copyPathBtn: document.getElementById("workspace-copy-path"),
      tree: document.getElementById("file-tree"),
      uploadBtn: document.getElementById("workspace-upload-file"),
      fileInput: document.getElementById("workspace-file-input"),
      newFileBtn: document.getElementById("workspace-new-file"),
      newFileModal: document.getElementById("workspace-new-file-modal"),
      newFileBackdrop: document.getElementById("workspace-new-file-backdrop"),
      newFileClose: document.getElementById("workspace-new-file-close"),
      newFileCancel: document.getElementById("workspace-new-file-cancel"),
      newFileForm: document.getElementById("workspace-new-file-form"),
      newFileInput: document.getElementById("workspace-new-file-input"),
      newFileSub: document.getElementById("workspace-new-file-sub"),
      newFileError: document.getElementById("workspace-new-file-error"),
      newFileSubmit: document.getElementById("workspace-new-file-submit"),
    };
    var currentPath = "";
    var activeFile = "";
    var sessionId = "";
    var workspaceRootPath = "";
    var newFileModalOpen = false;
    var creatingFile = false;
    var uploadingFiles = false;
    var maxWorkspaceUploadFiles = 10;

    function showEmptySession() {
      workspaceRootPath = "";
      els.root.textContent = "";
      els.root.title = "";
      if (els.copyPathBtn) {
        els.copyPathBtn.hidden = true;
      }
      if (els.newFileBtn) {
        els.newFileBtn.disabled = true;
      }
      if (els.uploadBtn) {
        els.uploadBtn.disabled = true;
      }
      els.tree.innerHTML = '<div class="file-tree-empty">' +
        '<span class="file-tree-empty-icon" aria-hidden="true">' +
        '<svg width="28" height="28" viewBox="0 0 16 16" fill="none">' +
        '<path d="M3 2.5h6.5L13 6v6.5a1 1 0 01-1 1H3a1 1 0 01-1-1V3.5a1 1 0 011-1z" stroke="currentColor" stroke-width="1.2"/>' +
        '<path d="M9.5 2.5V6H13" stroke="currentColor" stroke-width="1.2"/>' +
        "</svg></span>" +
        "<p class=\"file-tree-empty-title\">选择或新建会话</p>" +
        "<p class=\"file-tree-empty-desc\">工作区文件将显示在这里</p></div>";
    }

    function createFileTreeEmpty(title, desc) {
      var empty = document.createElement("div");
      empty.className = "file-tree-empty";
      empty.innerHTML =
        '<span class="file-tree-empty-icon" aria-hidden="true">' +
        '<svg width="28" height="28" viewBox="0 0 16 16" fill="none">' +
        '<path d="M2 4.5h5l1.5 1.5H14v7a1 1 0 01-1 1H2a1 1 0 01-1-1v-8a1 1 0 011-1z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round"/>' +
        "</svg></span>" +
        '<p class="file-tree-empty-title">' + escapeHtml(title) + "</p>" +
        '<p class="file-tree-empty-desc">' + escapeHtml(desc) + "</p>";
      return empty;
    }

    function sessionQuery(prefix) {
      if (!sessionId) {
        return "";
      }
      var sep = prefix === "?" ? "?" : "&";
      return sep + "session_id=" + encodeURIComponent(sessionId);
    }

    function showTreeLoading() {
      els.tree.innerHTML = '<p class="file-tree-loading">加载中…</p>';
    }

    function renderTree(data) {
      els.tree.innerHTML = "";
      currentPath = data.path || "";

      if (data.parent !== undefined && data.path !== "") {
        var up = document.createElement("button");
        up.type = "button";
        up.className = "file-tree-item dir";
        up.innerHTML = '<span class="file-tree-icon">↩</span>..';
        up.addEventListener("click", function () {
          loadTree(data.parent || "");
        });
        els.tree.appendChild(up);
      }

      var entries = data.entries || [];
      if (!entries.length && !data.path) {
        els.tree.appendChild(createFileTreeEmpty("目录为空", "上传或新建文件后在此管理"));
        return;
      }

      entries.forEach(function (entry) {
        var row = document.createElement("div");
        row.className = "file-tree-row";

        var btn = document.createElement("button");
        btn.type = "button";
        btn.className = "file-tree-item" + (entry.is_dir ? " dir" : "");
        if (!entry.is_dir) {
          btn.dataset.path = entry.path;
        }
        if (entry.path === activeFile) {
          btn.classList.add("active");
        }
        var icon = entry.is_dir ? "📁" : "📄";
        btn.innerHTML = '<span class="file-tree-icon">' + icon + "</span>" + entry.name;
        btn.addEventListener("click", function () {
          if (entry.is_dir) {
            loadTree(entry.path);
          } else {
            openFilePreview(entry.path);
          }
        });
        row.appendChild(btn);

        if (!entry.is_dir) {
          var del = document.createElement("button");
          del.type = "button";
          del.className = "file-tree-delete";
          del.title = "删除文件";
          del.setAttribute("aria-label", "删除 " + entry.name);
          del.innerHTML =
            '<svg width="12" height="12" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>';
          del.addEventListener("click", function (event) {
            event.stopPropagation();
            deleteFile(entry.path, entry.name);
          });
          row.appendChild(del);
        }

        els.tree.appendChild(row);
      });
    }

    function markActiveFile(path) {
      activeFile = path;
      els.tree.querySelectorAll(".file-tree-item[data-path]").forEach(function (btn) {
        btn.classList.toggle("active", btn.dataset.path === path);
      });
    }

    function joinPath(dir, name) {
      if (!dir) {
        return name;
      }
      return dir + "/" + name;
    }

    function showModal(visible) {
      if (!els.newFileModal) {
        return;
      }
      newFileModalOpen = visible;
      els.newFileModal.hidden = !visible;
      document.body.classList.toggle("provider-modal-open", visible);
    }

    function setNewFileError(message) {
      if (!els.newFileError) {
        return;
      }
      if (message) {
        els.newFileError.textContent = message;
        els.newFileError.hidden = false;
      } else {
        els.newFileError.textContent = "";
        els.newFileError.hidden = true;
      }
    }

    function validateFileName(name) {
      if (!name) {
        return "请输入文件名";
      }
      if (name.indexOf("/") !== -1 || name.indexOf("\\") !== -1) {
        return "文件名不能包含路径分隔符";
      }
      if (name === "." || name === "..") {
        return "文件名无效";
      }
      return "";
    }

    function currentDirLabel() {
      return currentPath ? currentPath : "根目录";
    }

    function openNewFileModal() {
      if (!sessionId) {
        return;
      }
      setNewFileError("");
      if (els.newFileSub) {
        els.newFileSub.textContent = "当前目录：" + currentDirLabel();
      }
      if (els.newFileInput) {
        els.newFileInput.value = "untitled.txt";
      }
      if (els.newFileSubmit) {
        els.newFileSubmit.disabled = false;
        els.newFileSubmit.textContent = "创建";
      }
      showModal(true);
      window.setTimeout(function () {
        if (els.newFileInput) {
          els.newFileInput.focus();
          els.newFileInput.select();
        }
      }, 0);
    }

    function closeNewFileModal() {
      if (!newFileModalOpen || creatingFile) {
        return;
      }
      showModal(false);
      setNewFileError("");
    }

    function submitNewFile(event) {
      if (event) {
        event.preventDefault();
      }
      if (!sessionId || creatingFile) {
        return;
      }

      var name = els.newFileInput ? els.newFileInput.value.trim() : "";
      var validationError = validateFileName(name);
      if (validationError) {
        setNewFileError(validationError);
        if (els.newFileInput) {
          els.newFileInput.focus();
        }
        return;
      }

      var path = joinPath(currentPath, name);
      creatingFile = true;
      setNewFileError("");
      if (els.newFileSubmit) {
        els.newFileSubmit.disabled = true;
        els.newFileSubmit.textContent = "创建中…";
      }

      apiFetch("/api/workspace/file" + sessionQuery("?"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: path, content: "" }),
      })
        .then(function () {
          showModal(false);
          return loadTree(currentPath);
        })
        .then(function () {
          openFilePreview(path);
        })
        .catch(function (err) {
          setNewFileError(err.message || "创建失败");
          if (els.newFileInput) {
            els.newFileInput.focus();
          }
        })
        .finally(function () {
          creatingFile = false;
          if (els.newFileSubmit) {
            els.newFileSubmit.disabled = false;
            els.newFileSubmit.textContent = "创建";
          }
        });
    }

    function createFile() {
      openNewFileModal();
    }

    function uploadFiles(files) {
      if (!sessionId || !files || !files.length || uploadingFiles) {
        return Promise.resolve();
      }
      if (files.length > maxWorkspaceUploadFiles) {
        window.alert("最多上传 " + maxWorkspaceUploadFiles + " 个文件");
        return Promise.resolve();
      }

      uploadingFiles = true;
      if (els.uploadBtn) {
        els.uploadBtn.disabled = true;
      }

      var fd = new FormData();
      Array.prototype.forEach.call(files, function (file) {
        fd.append("file", file);
      });

      var q = sessionQuery("?");
      if (currentPath) {
        q += (q ? "&" : "?") + "path=" + encodeURIComponent(currentPath);
      }

      return fetch("/api/workspace/upload" + q, Object.assign({
        method: "POST",
        body: fd,
      }, FETCH_CREDENTIALS))
        .then(function (res) {
          if (res.status === 401) {
            authPanel.requireLogin();
          }
          return res.json().then(function (body) {
            if (!res.ok) {
              throw new Error(body && body.error ? body.error : "上传失败 (" + res.status + ")");
            }
            return body;
          });
        })
        .then(function () {
          if (els.fileInput) {
            els.fileInput.value = "";
          }
          return loadTree(currentPath);
        })
        .catch(function (err) {
          window.alert("上传失败: " + err.message);
        })
        .finally(function () {
          uploadingFiles = false;
          if (els.uploadBtn) {
            els.uploadBtn.disabled = !sessionId;
          }
        });
    }

    function triggerUploadPicker() {
      if (!sessionId || uploadingFiles || !els.fileInput) {
        return;
      }
      els.fileInput.click();
    }

    function deleteFile(path, name) {
      if (!sessionId) {
        return;
      }
      if (!window.confirm('确定删除文件 "' + name + '"？')) {
        return;
      }
      apiFetch(
        "/api/workspace/file" +
          sessionQuery("?") +
          "&path=" +
          encodeURIComponent(path),
        { method: "DELETE" }
      )
        .then(function () {
          if (activeFile === path) {
            activeFile = "";
          }
          return loadTree(currentPath);
        })
        .catch(function (err) {
          window.alert("删除失败: " + err.message);
        });
    }

    function openFilePreview(path) {
      markActiveFile(path);
      var url =
        "/preview.html?path=" +
        encodeURIComponent(path) +
        "&session_id=" +
        encodeURIComponent(sessionId);
      var tab = window.open(url, PREVIEW_WINDOW, "noopener,noreferrer");
      if (tab) {
        tab.focus();
      }
    }

    function loadTree(path) {
      if (!sessionId) {
        showEmptySession();
        return Promise.resolve();
      }
      showTreeLoading();
      var q = sessionQuery("?");
      if (path) {
        q += (q ? "&" : "?") + "path=" + encodeURIComponent(path);
      }
      return apiFetch("/api/workspace/tree" + q)
        .then(renderTree)
        .catch(function (err) {
          els.tree.innerHTML =
            '<p class="file-tree-empty">加载失败: ' + err.message + "</p>";
        });
    }

    function refreshRoot() {
      if (!sessionId) {
        showEmptySession();
        return Promise.resolve();
      }
      return apiFetch("/api/workspace" + sessionQuery("?"))
        .then(function (data) {
          if (data.root) {
            workspaceRootPath = data.root;
            var short = data.root;
            if (short.length > 28) {
              short = "…" + short.slice(-26);
            }
            els.root.textContent = short;
            els.root.title = data.root;
            if (els.copyPathBtn) {
              els.copyPathBtn.hidden = false;
            }
          }
        })
        .catch(function () {
          /* ignore */
        });
    }

    function setSession(id) {
      sessionId = id || "";
      currentPath = "";
      activeFile = "";
      if (els.newFileBtn) {
        els.newFileBtn.disabled = !sessionId;
      }
      if (els.uploadBtn) {
        els.uploadBtn.disabled = !sessionId;
      }
      if (!sessionId) {
        showEmptySession();
        return Promise.resolve();
      }
      return refreshRoot().then(function () {
        return loadTree("");
      });
    }

    function copyWorkspacePath() {
      if (!workspaceRootPath || !els.copyPathBtn) {
        return;
      }
      copyTextToClipboard(workspaceRootPath).then(function () {
        els.copyPathBtn.classList.add("is-copied");
        els.copyPathBtn.title = "已复制路径";
        setTimeout(function () {
          els.copyPathBtn.classList.remove("is-copied");
          els.copyPathBtn.title = "复制路径";
        }, 1600);
      });
    }

    function init() {
      showEmptySession();
      if (els.uploadBtn) {
        els.uploadBtn.addEventListener("click", triggerUploadPicker);
      }
      if (els.fileInput) {
        els.fileInput.addEventListener("change", function () {
          uploadFiles(els.fileInput.files);
        });
      }
      if (els.tree) {
        els.tree.addEventListener("dragover", function (event) {
          if (!sessionId || uploadingFiles) {
            return;
          }
          event.preventDefault();
          els.tree.classList.add("is-dragover");
        });
        els.tree.addEventListener("dragleave", function (event) {
          if (event.currentTarget.contains(event.relatedTarget)) {
            return;
          }
          els.tree.classList.remove("is-dragover");
        });
        els.tree.addEventListener("drop", function (event) {
          event.preventDefault();
          els.tree.classList.remove("is-dragover");
          if (!sessionId || uploadingFiles) {
            return;
          }
          uploadFiles(event.dataTransfer.files);
        });
      }
      if (els.newFileBtn) {
        els.newFileBtn.addEventListener("click", createFile);
      }
      if (els.copyPathBtn) {
        els.copyPathBtn.addEventListener("click", copyWorkspacePath);
      }
      if (els.newFileBackdrop) {
        els.newFileBackdrop.addEventListener("click", closeNewFileModal);
      }
      if (els.newFileClose) {
        els.newFileClose.addEventListener("click", closeNewFileModal);
      }
      if (els.newFileCancel) {
        els.newFileCancel.addEventListener("click", closeNewFileModal);
      }
      if (els.newFileForm) {
        els.newFileForm.addEventListener("submit", submitNewFile);
      }
      if (els.newFileInput) {
        els.newFileInput.addEventListener("input", function () {
          setNewFileError("");
        });
      }
      document.addEventListener("keydown", function (event) {
        if (event.key === "Escape" && newFileModalOpen && !creatingFile) {
          closeNewFileModal();
        }
      });
    }

    return { init: init, setSession: setSession, refresh: function () {
      return setSession(sessionId);
    } };
  })();

  var PROVIDER_BRANDS = {
    openai: { color: "#10a37f", label: "AI" },
    anthropic: { color: "#d97757", label: "An" },
    deepseek: { color: "#4d6bfe", label: "DS" },
    moonshot: { color: "#1a1a1a", label: "月" },
    zhipu: { color: "#3b6cff", label: "智" },
    bailian: { color: "#ff6a00", label: "阿" },
    qianfan: { color: "#2932e1", label: "百" },
    minimax: { color: "#6b4eff", label: "MM" },
    ollama: { color: "#f8fafc", label: "🦙", light: true },
    openrouter: { color: "#6366f1", label: "OR" },
    groq: { color: "#f55036", label: "Gq" },
    together: { color: "#0ea5e9", label: "Tg" },
    custom: { color: "#64748b", label: "✦" },
  };

  var settingsPanel = (function () {
    var loaded = false;
    var loading = false;
    var providers = [];
    var config = null;
    var selectedCode = "";
    var drafts = {};
    var searchQuery = "";
    var modalOpen = false;
    var activeTab = "llm";
    var onConfigSaved = null;
    var editingHostId = null;
    var hostModalOpen = false;
    var hostSearchQuery = "";

    function newHostId() {
      return "host-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 10);
    }

    var els = {
      loading: document.getElementById("settings-loading"),
      error: document.getElementById("settings-error"),
      tabBtns: document.querySelectorAll("[data-settings-tab]"),
      tabLlm: document.getElementById("settings-tab-llm"),
      tabHosts: document.getElementById("settings-tab-hosts"),
      cards: document.getElementById("provider-cards"),
      providerEmpty: document.getElementById("provider-empty"),
      providerSearch: document.getElementById("provider-search"),
      bodyLlm: document.getElementById("settings-body-llm"),
      bodyHosts: document.getElementById("settings-body-hosts"),
      hostCards: document.getElementById("host-cards"),
      hostEmpty: document.getElementById("host-empty"),
      btnAddHost: document.getElementById("btn-add-host"),
      modal: document.getElementById("provider-modal"),
      modalBackdrop: document.getElementById("provider-modal-backdrop"),
      modalClose: document.getElementById("provider-modal-close"),
      modalAvatar: document.getElementById("provider-modal-avatar"),
      modalTitle: document.getElementById("provider-modal-title"),
      modalSub: document.getElementById("provider-modal-sub"),
      form: document.getElementById("settings-form"),
      apiKey: document.getElementById("field-api-key"),
      apiKeyHint: document.getElementById("field-api-key-hint"),
      baseUrl: document.getElementById("field-base-url"),
      model: document.getElementById("field-model"),
      processor: document.getElementById("field-response-processor"),
      testBtn: document.getElementById("btn-test"),
      saveBtn: document.getElementById("btn-save"),
      testResult: document.getElementById("test-result"),
      saveStatus: document.getElementById("save-status"),
      hostModal: document.getElementById("host-modal"),
      hostModalBackdrop: document.getElementById("host-modal-backdrop"),
      hostModalClose: document.getElementById("host-modal-close"),
      hostModalTitle: document.getElementById("host-modal-title"),
      hostModalSub: document.getElementById("host-modal-sub"),
      hostForm: document.getElementById("host-form"),
      hostName: document.getElementById("field-host-name"),
      hostAddr: document.getElementById("field-host-addr"),
      hostPort: document.getElementById("field-host-port"),
      hostUser: document.getElementById("field-host-user"),
      hostPassword: document.getElementById("field-host-password"),
      hostPasswordHint: document.getElementById("field-host-password-hint"),
      hostSaveStatus: document.getElementById("host-save-status"),
      hostSaveBtn: document.getElementById("btn-host-save"),
      hostDeleteBtn: document.getElementById("btn-host-delete"),
      hostCancelBtn: document.getElementById("btn-host-cancel"),
    };

    function show(el, visible) {
      if (!el) {
        return;
      }
      el.hidden = !visible;
    }

    function setError(message) {
      if (!message) {
        show(els.error, false);
        els.error.textContent = "";
        return;
      }
      els.error.textContent = message;
      show(els.error, true);
    }

    function hostsList() {
      if (!config || !config.ssh || !config.ssh.hosts) {
        return [];
      }
      return config.ssh.hosts;
    }

    function hostById(id) {
      var list = hostsList();
      for (var i = 0; i < list.length; i++) {
        if (list[i].id === id) {
          return list[i];
        }
      }
      return null;
    }

    function hostDisplayName(h) {
      if (h.name) {
        return h.name;
      }
      return h.host || "未命名主机";
    }

    function hostEndpoint(h) {
      var port = h.port || 22;
      var addr = h.host || "";
      if (port && port !== 22) {
        return addr + ":" + port;
      }
      return addr;
    }

    function activateTab(tab) {
      activeTab = tab === "hosts" ? "hosts" : "llm";
      if (els.tabBtns) {
        els.tabBtns.forEach(function (btn) {
          var isActive = btn.dataset.settingsTab === activeTab;
          btn.classList.toggle("active", isActive);
          btn.setAttribute("aria-selected", isActive ? "true" : "false");
        });
      }
      setSettingsBodyVisible(loaded && !loading);
      writeUrlState({ settingsTab: activeTab }, true);
    }

    function setSettingsBodyVisible(visible) {
      if (els.bodyLlm) {
        show(els.bodyLlm, visible && activeTab === "llm");
      }
      if (els.bodyHosts) {
        show(els.bodyHosts, visible && activeTab === "hosts");
      }
      if (els.tabLlm) {
        els.tabLlm.hidden = !visible || activeTab !== "llm";
      }
      if (els.tabHosts) {
        els.tabHosts.hidden = !visible || activeTab !== "hosts";
      }
    }

    function apiFetch(path, options) {
      options = options || {};
      if (!options.credentials) {
        options.credentials = "include";
      }
      return fetch(path, options).then(function (res) {
        if (res.status === 401 && authPanel.isAuthenticated()) {
          authPanel.requireLogin();
        }
        return res.json().then(function (body) {
          if (!res.ok) {
            var err = new Error(body && body.error ? body.error : "请求失败 (" + res.status + ")");
            err.status = res.status;
            throw err;
          }
          return body;
        });
      });
    }

    function providerByCode(code) {
      for (var i = 0; i < providers.length; i++) {
        if (providers[i].provider_code === code) {
          return providers[i];
        }
      }
      return null;
    }

    function instanceFor(code) {
      if (!config || !config.llm || !config.llm.providers) {
        return {};
      }
      return config.llm.providers[code] || {};
    }

    function defaultDraft(code) {
      var catalog = providerByCode(code);
      var inst = instanceFor(code);
      var processor = inst.response_processor || "";
      if (!processor && catalog) {
        processor = catalog.default_wire_api === "anthropic" ? "anthropic_messages" : "openai_compat";
      }
      return {
        api_key: inst.api_key || "",
        base_url: inst.base_url || (catalog ? catalog.default_base_url : "") || "",
        model: inst.model || (catalog ? catalog.default_model : "") || "",
        response_processor: processor || "openai_compat",
      };
    }

    function getDraft(code) {
      if (!drafts[code]) {
        drafts[code] = defaultDraft(code);
      }
      return drafts[code];
    }

    function hasConfiguredApiKey(code) {
      var draft = getDraft(code);
      if (draft.api_key) {
        return true;
      }
      var inst = instanceFor(code);
      return !!inst.api_key;
    }

    function updateApiKeyHint(code) {
      if (!els.apiKeyHint) {
        return;
      }
      var inputValue = els.apiKey ? els.apiKey.value : "";
      if (inputValue) {
        els.apiKeyHint.textContent = "将保存此密钥";
        els.apiKeyHint.classList.remove("is-configured");
      } else if (hasConfiguredApiKey(code)) {
        els.apiKeyHint.textContent = "留空则保留已保存的 API Key";
        els.apiKeyHint.classList.add("is-configured");
      } else {
        els.apiKeyHint.textContent = "密钥仅保存在本地";
        els.apiKeyHint.classList.remove("is-configured");
      }
    }

    function saveDraftFromForm() {
      if (!selectedCode) {
        return;
      }
      drafts[selectedCode] = {
        api_key: els.apiKey.value,
        base_url: els.baseUrl.value.trim(),
        model: els.model.value.trim(),
        response_processor: els.processor.value,
      };
    }

    function fillForm(code) {
      var draft = getDraft(code);
      els.apiKey.value = draft.api_key || "";
      els.baseUrl.value = draft.base_url;
      els.model.value = draft.model;
      els.processor.value = draft.response_processor || "openai_compat";
      updateApiKeyHint(code);
    }

    function updateModalHeader(code) {
      var catalog = providerByCode(code);
      if (!catalog || !els.modalTitle) {
        return;
      }
      var brand = providerBrand(code);
      var draft = getDraft(code);
      els.modalTitle.textContent = catalog.display_name;
      els.modalSub.textContent = draft.model || catalog.default_model || "";
      if (els.modalAvatar) {
        els.modalAvatar.className = "provider-card-avatar" + (brand.light ? " avatar-light" : "");
        els.modalAvatar.style.background = brand.color;
        els.modalAvatar.textContent = brand.label;
      }
    }

    function openProviderModal(code) {
      if (!code) {
        return;
      }
      if (modalOpen && selectedCode && selectedCode !== code) {
        saveDraftFromForm();
      }
      selectedCode = code;
      fillForm(code);
      updateModalHeader(code);
      hideTestResult();
      hideSaveStatus();
      modalOpen = true;
      show(els.modal, true);
      document.body.classList.add("provider-modal-open");
      window.setTimeout(function () {
        els.apiKey.focus();
      }, 50);
    }

    function closeProviderModal() {
      if (!modalOpen) {
        return;
      }
      saveDraftFromForm();
      modalOpen = false;
      show(els.modal, false);
      if (!hostModalOpen) {
        document.body.classList.remove("provider-modal-open");
      }
      renderCards();
    }

    function providerBrand(code) {
      var brand = PROVIDER_BRANDS[code];
      if (brand) {
        return brand;
      }
      var catalog = providerByCode(code);
      var name = catalog ? catalog.display_name : code;
      return {
        color: "#94a3b8",
        label: name.slice(0, 2),
      };
    }

    function matchesSearch(p) {
      if (!searchQuery) {
        return true;
      }
      var q = searchQuery.toLowerCase();
      var draft = getDraft(p.provider_code);
      return (
        p.display_name.toLowerCase().indexOf(q) !== -1 ||
        p.provider_code.toLowerCase().indexOf(q) !== -1 ||
        (p.default_model || "").toLowerCase().indexOf(q) !== -1 ||
        (draft.model || "").toLowerCase().indexOf(q) !== -1
      );
    }

    function setBtnLabel(btn, text) {
      var label = btn.querySelector(".btn-label");
      if (label) {
        label.textContent = text;
      } else {
        btn.textContent = text;
      }
    }

    function renderCards() {
      els.cards.innerHTML = "";
      var visible = 0;
      providers.forEach(function (p) {
        if (!matchesSearch(p)) {
          return;
        }
        visible += 1;
        var brand = providerBrand(p.provider_code);
        var draft = getDraft(p.provider_code);

        var card = document.createElement("button");
        card.type = "button";
        card.className = "provider-card";
        card.dataset.code = p.provider_code;
        if (isProviderInUse(config, p.provider_code)) {
          card.classList.add("active-provider");
        }

        var top = document.createElement("div");
        top.className = "provider-card-top";

        var avatar = document.createElement("span");
        avatar.className = "provider-card-avatar" + (brand.light ? " avatar-light" : "");
        avatar.style.background = brand.color;
        avatar.textContent = brand.label;
        top.appendChild(avatar);

        var body = document.createElement("span");
        body.className = "provider-card-body";

        var name = document.createElement("span");
        name.className = "provider-card-name";
        name.textContent = p.display_name;

        var meta = document.createElement("span");
        meta.className = "provider-card-meta";
        var metaParts = [draft.model || p.default_model || p.default_wire_api];
        if (hasConfiguredApiKey(p.provider_code)) {
          metaParts.push("API Key 已配置");
        }
        meta.textContent = metaParts.join(" · ");

        body.appendChild(name);
        body.appendChild(meta);
        top.appendChild(body);
        card.appendChild(top);

        var hint = document.createElement("span");
        hint.className = "provider-card-hint";
        hint.textContent = "点击配置 →";
        card.appendChild(hint);

        if (isProviderInUse(config, p.provider_code)) {
          var badge = document.createElement("span");
          badge.className = "provider-card-badge";
          badge.textContent = "使用中";
          card.appendChild(badge);
        }

        card.addEventListener("click", function () {
          openProviderModal(p.provider_code);
        });

        els.cards.appendChild(card);
      });
      if (els.providerEmpty) {
        show(els.providerEmpty, visible === 0);
      }
    }

    function matchesHostSearch(h) {
      if (!hostSearchQuery) {
        return true;
      }
      var q = hostSearchQuery.toLowerCase();
      return (
        (h.name || "").toLowerCase().indexOf(q) !== -1 ||
        (h.host || "").toLowerCase().indexOf(q) !== -1 ||
        (h.user || "").toLowerCase().indexOf(q) !== -1
      );
    }

    function renderHostCards() {
      if (!els.hostCards) {
        return;
      }
      els.hostCards.innerHTML = "";
      var list = hostsList();
      var visible = 0;
      list.forEach(function (h) {
        if (!matchesHostSearch(h)) {
          return;
        }
        visible += 1;

        var card = document.createElement("div");
        card.className = "host-card";
        card.dataset.id = h.id;

        var top = document.createElement("div");
        top.className = "host-card-top";

        var icon = document.createElement("span");
        icon.className = "host-card-icon";
        icon.innerHTML =
          '<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true"><rect x="2" y="3" width="12" height="10" rx="1.5" stroke="currentColor" stroke-width="1.3"/><path d="M5 6h6M5 9h4" stroke="currentColor" stroke-width="1.3" stroke-linecap="round"/></svg>';
        top.appendChild(icon);

        var body = document.createElement("div");
        body.className = "host-card-body";

        var name = document.createElement("span");
        name.className = "host-card-name";
        name.textContent = hostDisplayName(h);

        var meta = document.createElement("span");
        meta.className = "host-card-meta";
        var metaParts = [(h.user || "—") + "@" + hostEndpoint(h)];
        if (h.password_configured) {
          metaParts.push("密码已配置");
        }
        meta.textContent = metaParts.join(" · ");

        body.appendChild(name);
        body.appendChild(meta);
        top.appendChild(body);
        card.appendChild(top);

        var actions = document.createElement("div");
        actions.className = "host-card-actions";

        var editBtn = document.createElement("button");
        editBtn.type = "button";
        editBtn.className = "btn btn-secondary btn-sm";
        editBtn.textContent = "编辑";
        editBtn.addEventListener("click", function () {
          openHostModal(h.id);
        });

        var delBtn = document.createElement("button");
        delBtn.type = "button";
        delBtn.className = "btn btn-danger btn-ghost btn-sm";
        delBtn.textContent = "删除";
        delBtn.addEventListener("click", function () {
          deleteHost(h.id);
        });

        actions.appendChild(editBtn);
        actions.appendChild(delBtn);
        card.appendChild(actions);

        els.hostCards.appendChild(card);
      });

      if (els.hostEmpty) {
        show(els.hostEmpty, visible === 0);
      }
    }

    function hideHostSaveStatus() {
      if (!els.hostSaveStatus) {
        return;
      }
      show(els.hostSaveStatus, false);
      els.hostSaveStatus.className = "save-status";
      els.hostSaveStatus.textContent = "";
    }

    function showHostSaveStatus(ok, message) {
      if (!els.hostSaveStatus) {
        return;
      }
      els.hostSaveStatus.className = "save-status " + (ok ? "save-status-ok" : "save-status-fail");
      els.hostSaveStatus.textContent = message;
      show(els.hostSaveStatus, true);
    }

    function updateHostPasswordHint() {
      if (!els.hostPasswordHint) {
        return;
      }
      var inputValue = els.hostPassword ? els.hostPassword.value : "";
      var configured = editingHostId && hostById(editingHostId) && hostById(editingHostId).password_configured;
      if (inputValue) {
        els.hostPasswordHint.textContent = "将保存此密码";
        els.hostPasswordHint.classList.remove("is-configured");
      } else if (configured) {
        els.hostPasswordHint.textContent = "留空则保留已保存的密码";
        els.hostPasswordHint.classList.add("is-configured");
      } else {
        els.hostPasswordHint.textContent = "密码仅保存在本地";
        els.hostPasswordHint.classList.remove("is-configured");
      }
    }

    function openHostModal(hostId) {
      editingHostId = hostId || null;
      hideHostSaveStatus();
      var isEdit = !!hostId;
      var h = isEdit ? hostById(hostId) : null;

      if (els.hostModalTitle) {
        els.hostModalTitle.textContent = isEdit ? "编辑主机" : "添加主机";
      }
      if (els.hostModalSub) {
        els.hostModalSub.textContent = isEdit ? hostDisplayName(h) : "配置 SSH 登录信息";
      }
      if (els.hostName) {
        els.hostName.value = h && h.name ? h.name : "";
      }
      if (els.hostAddr) {
        els.hostAddr.value = h && h.host ? h.host : "";
      }
      if (els.hostPort) {
        els.hostPort.value = h && h.port ? String(h.port) : "";
      }
      if (els.hostUser) {
        els.hostUser.value = h && h.user ? h.user : "";
      }
      if (els.hostPassword) {
        els.hostPassword.value = "";
      }
      updateHostPasswordHint();
      show(els.hostDeleteBtn, isEdit);

      hostModalOpen = true;
      show(els.hostModal, true);
      document.body.classList.add("provider-modal-open");
      window.setTimeout(function () {
        if (els.hostAddr) {
          els.hostAddr.focus();
        }
      }, 50);
    }

    function closeHostModal() {
      if (!hostModalOpen) {
        return;
      }
      hostModalOpen = false;
      editingHostId = null;
      show(els.hostModal, false);
      if (!modalOpen) {
        document.body.classList.remove("provider-modal-open");
      }
      hideHostSaveStatus();
    }

    function buildHostSavePayload(hosts) {
      var out = JSON.parse(JSON.stringify(config));
      if (!out.ssh) {
        out.ssh = { hosts: [] };
      }
      out.ssh.hosts = hosts;
      return out;
    }

    function saveHosts(hosts, successMessage) {
      return apiFetch("/api/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(buildHostSavePayload(hosts)),
      }).then(function (saved) {
        config = saved;
        renderHostCards();
        return successMessage;
      });
    }

    function readHostForm() {
      var portRaw = els.hostPort ? els.hostPort.value.trim() : "";
      var port = portRaw ? parseInt(portRaw, 10) : 0;
      if (isNaN(port) || port < 1) {
        port = 0;
      }
      return {
        id: editingHostId || newHostId(),
        name: els.hostName ? els.hostName.value.trim() : "",
        host: els.hostAddr ? els.hostAddr.value.trim() : "",
        port: port,
        user: els.hostUser ? els.hostUser.value.trim() : "",
        password: els.hostPassword ? els.hostPassword.value : "",
      };
    }

    function onHostSave(event) {
      event.preventDefault();
      hideHostSaveStatus();
      var entry = readHostForm();
      if (!entry.host) {
        showHostSaveStatus(false, "请填写主机地址");
        return;
      }
      if (!entry.user) {
        showHostSaveStatus(false, "请填写登录用户名");
        return;
      }

      var hosts = hostsList().slice();
      var idx = -1;
      for (var i = 0; i < hosts.length; i++) {
        if (hosts[i].id === entry.id) {
          idx = i;
          break;
        }
      }
      var record = {
        id: entry.id,
        name: entry.name,
        host: entry.host,
        user: entry.user,
      };
      if (entry.port) {
        record.port = entry.port;
      }
      if (entry.password) {
        record.password = entry.password;
      } else if (idx >= 0 && hosts[idx].password_configured) {
        record.password = "";
      }
      if (idx >= 0) {
        hosts[idx] = record;
      } else {
        hosts.push(record);
      }

      els.hostSaveBtn.disabled = true;
      setBtnLabel(els.hostSaveBtn, "保存中…");

      saveHosts(hosts, "主机已保存")
        .then(function (msg) {
          showHostSaveStatus(true, msg);
          window.setTimeout(closeHostModal, 700);
        })
        .catch(function (err) {
          showHostSaveStatus(false, "保存失败: " + err.message);
        })
        .finally(function () {
          els.hostSaveBtn.disabled = false;
          setBtnLabel(els.hostSaveBtn, "保存");
        });
    }

    function deleteHost(hostId) {
      var h = hostById(hostId);
      if (!h) {
        return;
      }
      var label = hostDisplayName(h);
      if (!window.confirm("确定删除主机「" + label + "」？")) {
        return;
      }
      var hosts = hostsList().filter(function (item) {
        return item.id !== hostId;
      });
      saveHosts(hosts)
        .then(function () {
          if (hostModalOpen && editingHostId === hostId) {
            closeHostModal();
          }
        })
        .catch(function (err) {
          setError("删除失败: " + err.message);
        });
    }

    function onHostDelete() {
      if (!editingHostId) {
        return;
      }
      deleteHost(editingHostId);
    }

    function hideTestResult() {
      show(els.testResult, false);
      els.testResult.className = "test-result";
      els.testResult.textContent = "";
    }

    function hideSaveStatus() {
      show(els.saveStatus, false);
      els.saveStatus.className = "save-status";
      els.saveStatus.textContent = "";
    }

    function showTestResult(ok, message, detail) {
      els.testResult.className = "test-result " + (ok ? "test-result-ok" : "test-result-fail");
      els.testResult.innerHTML = "";
      var title = document.createElement("p");
      title.className = "test-result-title";
      title.textContent = (ok ? "✓ " : "✗ ") + message;
      els.testResult.appendChild(title);
      if (detail) {
        var body = document.createElement("pre");
        body.className = "test-result-body";
        body.textContent = detail;
        els.testResult.appendChild(body);
      }
      show(els.testResult, true);
    }

    function showSaveStatus(ok, message) {
      els.saveStatus.className = "save-status " + (ok ? "save-status-ok" : "save-status-fail");
      els.saveStatus.textContent = message;
      show(els.saveStatus, true);
    }

    function buildTestPayload() {
      saveDraftFromForm();
      var draft = getDraft(selectedCode);
      var catalog = providerByCode(selectedCode);
      var inst = instanceFor(selectedCode);
      var payload = {
        provider_code: selectedCode,
        model: draft.model,
        base_url: draft.base_url,
        response_processor: draft.response_processor,
      };
      if (draft.api_key) {
        payload.api_key = draft.api_key;
      } else if (inst.api_key) {
        payload.api_key = inst.api_key;
      }
      if (catalog) {
        payload.wire_api = inst.wire_api || catalog.default_wire_api || "";
      }
      return payload;
    }

    function buildSavePayload() {
      saveDraftFromForm();
      var out = JSON.parse(JSON.stringify(config));
      if (!out.llm) {
        out.llm = { active: selectedCode, providers: {} };
      }
      if (!out.llm.providers) {
        out.llm.providers = {};
      }
      out.llm.active = selectedCode;

      Object.keys(drafts).forEach(function (code) {
        var draft = drafts[code];
        var catalog = providerByCode(code);
        var prev = out.llm.providers[code] || {};
        var entry = {
          enabled: code === selectedCode ? true : !!prev.enabled,
          model: draft.model,
          base_url: draft.base_url,
          response_processor: draft.response_processor,
        };
        if (catalog) {
          entry.wire_api = prev.wire_api || catalog.default_wire_api || "";
        } else {
          entry.wire_api = prev.wire_api || "";
        }
        if (draft.api_key) {
          entry.api_key = draft.api_key;
        } else if (prev.api_key) {
          entry.api_key = prev.api_key;
        }
        if (prev.timeout_sec) {
          entry.timeout_sec = prev.timeout_sec;
        }
        out.llm.providers[code] = entry;
      });

      return out;
    }

    function onTest() {
      if (!selectedCode) {
        return;
      }
      hideTestResult();
      hideSaveStatus();
      els.testBtn.disabled = true;
      setBtnLabel(els.testBtn, "测试中…");

      apiFetch("/api/llm/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(buildTestPayload()),
      })
        .then(function (res) {
          if (res.success) {
            var detail =
              "延迟: " +
              (res.latency_ms != null ? res.latency_ms + " ms" : "—") +
              "\n解析器: " +
              (res.processor_used || "—") +
              "\n回复: " +
              (res.content || "");
            showTestResult(true, "连接成功", detail);
          } else {
            showTestResult(false, "连接失败", res.error || "未知错误");
          }
        })
        .catch(function (err) {
          showTestResult(false, "连接失败", err.message);
        })
        .finally(function () {
          els.testBtn.disabled = false;
          setBtnLabel(els.testBtn, "测试连接");
        });
    }

    function onSave(event) {
      event.preventDefault();
      if (!selectedCode) {
        return;
      }
      hideSaveStatus();
      els.saveBtn.disabled = true;
      setBtnLabel(els.saveBtn, "保存中…");

      apiFetch("/api/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(buildSavePayload()),
      })
        .then(function (saved) {
          config = saved;
          drafts = {};
          getDraft(selectedCode);
          renderCards();
          showSaveStatus(true, "配置已保存，已切换为当前提供商");
          if (typeof onConfigSaved === "function") {
            onConfigSaved(saved);
          }
          window.setTimeout(closeProviderModal, 900);
        })
        .catch(function (err) {
          showSaveStatus(false, "保存失败: " + err.message);
        })
        .finally(function () {
          els.saveBtn.disabled = false;
          setBtnLabel(els.saveBtn, "保存并启用");
        });
    }

    function load() {
      if (loading) {
        return;
      }
      loading = true;
      show(els.loading, true);
      setSettingsBodyVisible(false);
      setError("");

      Promise.all([apiFetch("/api/providers"), apiFetch("/api/config")])
        .then(function (results) {
          providers = results[0].providers || [];
          config = results[1];
          drafts = {};

          selectedCode =
            (config.llm && config.llm.active) ||
            (providers[0] && providers[0].provider_code) ||
            "";

          providers.forEach(function (p) {
            getDraft(p.provider_code);
          });

          renderCards();
          renderHostCards();
          loaded = true;
        })
        .catch(function (err) {
          setError("加载设置失败: " + err.message);
        })
        .finally(function () {
          loading = false;
          show(els.loading, false);
          setSettingsBodyVisible(true);
        });
    }

    if (els.form) {
      els.form.addEventListener("submit", onSave);
    }
    if (els.apiKey) {
      els.apiKey.addEventListener("input", function () {
        if (selectedCode) {
          updateApiKeyHint(selectedCode);
        }
      });
    }
    if (els.testBtn) {
      els.testBtn.addEventListener("click", onTest);
    }
    if (els.providerSearch) {
      els.providerSearch.addEventListener("input", function () {
        searchQuery = els.providerSearch.value.trim();
        renderCards();
      });
    }
    if (els.modalBackdrop) {
      els.modalBackdrop.addEventListener("click", closeProviderModal);
    }
    if (els.modalClose) {
      els.modalClose.addEventListener("click", closeProviderModal);
    }
    if (els.tabBtns) {
      els.tabBtns.forEach(function (btn) {
        btn.addEventListener("click", function () {
          activateTab(btn.dataset.settingsTab);
        });
      });
    }
    if (els.btnAddHost) {
      els.btnAddHost.addEventListener("click", function () {
        openHostModal(null);
      });
    }
    if (els.hostForm) {
      els.hostForm.addEventListener("submit", onHostSave);
    }
    if (els.hostPassword) {
      els.hostPassword.addEventListener("input", updateHostPasswordHint);
    }
    if (els.hostModalBackdrop) {
      els.hostModalBackdrop.addEventListener("click", closeHostModal);
    }
    if (els.hostModalClose) {
      els.hostModalClose.addEventListener("click", closeHostModal);
    }
    if (els.hostCancelBtn) {
      els.hostCancelBtn.addEventListener("click", closeHostModal);
    }
    if (els.hostDeleteBtn) {
      els.hostDeleteBtn.addEventListener("click", onHostDelete);
    }
    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && modalOpen) {
        closeProviderModal();
      }
      if (event.key === "Escape" && hostModalOpen) {
        closeHostModal();
      }
    });

    return {
      ensureLoaded: function () {
        if (!loaded) {
          load();
        }
      },
      activateTab: activateTab,
      initTabFromUrl: function (tab) {
        activateTab(tab === "hosts" ? "hosts" : "llm");
      },
      setOnConfigSaved: function (fn) {
        onConfigSaved = fn;
      },
    };
  })();

  var SESSION_STORAGE_KEY = "databuff-diag.session_id";

  var chatPanel = (function () {
    var sessionId = null;
    var session = null;
    var config = null;
    var streaming = false;
    var initialized = false;
    var pendingAttachments = [];
    var maxAttachments = 5;
    var activeAbortController = null;
    var activeStreamState = null;

    var els = {
      policy: document.getElementById("field-policy-mode"),
      error: document.getElementById("chat-error"),
      llmNotice: document.getElementById("chat-llm-notice"),
      llmNoticeLink: document.getElementById("chat-llm-notice-link"),
      messages: document.getElementById("chat-messages"),
      approvals: document.getElementById("chat-approvals"),
      form: document.getElementById("chat-form"),
      input: document.getElementById("chat-input"),
      send: document.getElementById("chat-send"),
      attachBtn: document.getElementById("chat-attach-btn"),
      fileInput: document.getElementById("chat-file-input"),
      attachments: document.getElementById("chat-attachments"),
      statusDot: document.getElementById("chat-status-dot"),
      copyLink: document.getElementById("btn-copy-session-link"),
    };

    var EMPTY_SUGGESTIONS = [
      "检查 Docker 容器健康状态",
      "分析最近的服务错误日志",
      "排查端口连通性问题",
    ];

    var INPUT_MAX_HEIGHT = 160;
    var INPUT_MAX_CHARS = 100000;

    function inputCharCount(text) {
      return Array.from(text).length;
    }

    function resetInputHeight() {
      els.input.style.height = "";
      els.input.style.overflowY = "";
    }

    function resizeInput() {
      var input = els.input;
      input.style.height = "auto";
      var next = Math.min(input.scrollHeight, INPUT_MAX_HEIGHT);
      input.style.height = next + "px";
      input.style.overflowY = input.scrollHeight > INPUT_MAX_HEIGHT ? "auto" : "hidden";
    }

    var POLICY_LABELS = {
      all_approval: "全部审批",
      write_approval: "写入审批",
      open: "全部开放",
    };

    var RISK_LABELS = {
      readonly: "只读",
      write: "写入",
      dangerous: "危险",
      blocked: "已拦截",
    };

    function show(el, visible) {
      if (!el) {
        return;
      }
      el.hidden = !visible;
    }

    function setError(message) {
      if (!message) {
        show(els.error, false);
        els.error.textContent = "";
        return;
      }
      els.error.textContent = message;
      show(els.error, true);
    }

    function formatFileSize(bytes) {
      if (bytes < 1024) {
        return bytes + " B";
      }
      if (bytes < 1024 * 1024) {
        return (bytes / 1024).toFixed(1) + " KB";
      }
      return (bytes / (1024 * 1024)).toFixed(1) + " MB";
    }

    function isImageMime(mime) {
      return mime && mime.indexOf("image/") === 0;
    }

    function renderPendingAttachments() {
      els.attachments.innerHTML = "";
      if (!pendingAttachments.length) {
        show(els.attachments, false);
        return;
      }
      show(els.attachments, true);
      pendingAttachments.forEach(function (att, index) {
        var chip = document.createElement("div");
        chip.className = "chat-attachment-chip" + (isImageMime(att.mime_type) ? " is-image" : "");

        if (isImageMime(att.mime_type) && att.preview_url) {
          var thumb = document.createElement("img");
          thumb.className = "chat-attachment-thumb";
          thumb.src = att.preview_url;
          thumb.alt = att.filename;
          chip.appendChild(thumb);
        }

        var name = document.createElement("span");
        name.className = "chat-attachment-name";
        name.textContent = att.filename;
        name.title = att.filename;

        var size = document.createElement("span");
        size.className = "chat-attachment-size";
        size.textContent = formatFileSize(att.size);

        var removeBtn = document.createElement("button");
        removeBtn.type = "button";
        removeBtn.className = "chat-attachment-remove";
        removeBtn.setAttribute("aria-label", "移除 " + att.filename);
        removeBtn.textContent = "×";
        removeBtn.addEventListener("click", function () {
          if (att.preview_url) {
            URL.revokeObjectURL(att.preview_url);
          }
          pendingAttachments.splice(index, 1);
          renderPendingAttachments();
        });

        chip.appendChild(name);
        chip.appendChild(size);
        chip.appendChild(removeBtn);
        els.attachments.appendChild(chip);
      });
    }

    function clearPendingAttachments() {
      pendingAttachments.forEach(function (att) {
        if (att.preview_url) {
          URL.revokeObjectURL(att.preview_url);
        }
      });
      pendingAttachments = [];
      renderPendingAttachments();
    }

    function uploadFiles(files) {
      if (!files || !files.length) {
        return Promise.resolve();
      }
      var remaining = maxAttachments - pendingAttachments.length;
      if (remaining <= 0) {
        setError("最多上传 " + maxAttachments + " 个文件");
        return Promise.resolve();
      }

      var toUpload = Array.prototype.slice.call(files, 0, remaining);
      if (files.length > remaining) {
        setError("最多上传 " + maxAttachments + " 个文件，已忽略多余文件");
      }

      setError("");
      els.attachBtn.disabled = true;

      var chain = Promise.resolve();
      toUpload.forEach(function (file) {
        chain = chain.then(function () {
          var fd = new FormData();
          fd.append("file", file);
          return fetch("/api/upload", Object.assign({
            method: "POST",
            body: fd,
          }, FETCH_CREDENTIALS)).then(function (res) {
            if (res.status === 401) {
              authPanel.requireLogin();
            }
            if (!res.ok) {
              return res.json().then(function (body) {
                throw new Error(body && body.error ? body.error : "上传失败 (" + res.status + ")");
              });
            }
            return res.json();
          }).then(function (result) {
            var uploaded = result.files ? result.files : [result];
            uploaded.forEach(function (item) {
              var entry = {
                id: item.id,
                filename: item.filename,
                mime_type: item.mime_type,
                size: item.size,
                url: item.url,
              };
              if (isImageMime(item.mime_type)) {
                entry.preview_url = URL.createObjectURL(file);
              }
              pendingAttachments.push(entry);
            });
            renderPendingAttachments();
          });
        });
      });

      return chain.catch(function (err) {
        setError(err.message);
      }).finally(function () {
        els.attachBtn.disabled = streaming;
      });
    }

    function buildMessageAttachments(msg) {
      if (!msg.attachments || !msg.attachments.length) {
        return null;
      }
      var wrap = document.createElement("div");
      wrap.className = "chat-msg-attachments";
      msg.attachments.forEach(function (att) {
        if (isImageMime(att.mime_type)) {
          var img = document.createElement("img");
          img.className = "chat-msg-attachment-img";
          img.src = "/api/attachments/" + encodeURIComponent(att.id);
          img.alt = att.filename;
          img.loading = "lazy";
          wrap.appendChild(img);
        } else {
          var link = document.createElement("a");
          link.className = "chat-msg-attachment-file";
          link.href = "/api/attachments/" + encodeURIComponent(att.id);
          link.target = "_blank";
          link.rel = "noopener";
          link.textContent = att.filename + " (" + formatFileSize(att.size) + ")";
          wrap.appendChild(link);
        }
      });
      return wrap;
    }

    var KNOWN_SHELL_COMMANDS = {
      awk: 1, bash: 1, cat: 1, curl: 1, df: 1, dig: 1, docker: 1, du: 1, echo: 1, env: 1,
      false: 1, find: 1, free: 1, grep: 1, head: 1, host: 1, hostname: 1, id: 1, journalctl: 1,
      kubectl: 1, ls: 1, mount: 1, netstat: 1, nginx: 1, ping: 1, printenv: 1, ps: 1, pwd: 1,
      sed: 1, sh: 1, sort: 1, ss: 1, stat: 1, systemctl: 1, tail: 1, tee: 1, test: 1, top: 1,
      true: 1, uname: 1, uniq: 1, uptime: 1, wc: 1, which: 1, whoami: 1, xargs: 1,
    };

    function looksLikeTreeDiagram(text) {
      return /├──|└──|│/.test(text);
    }

    function plausibleCommandLine(line) {
      if (looksLikeTreeDiagram(line)) {
        return false;
      }
      if (/[;|&<>$()`]/.test(line)) {
        return true;
      }
      var words = line.trim().split(/\s+/).filter(Boolean);
      if (words.length === 0) {
        return false;
      }
      if (words.length > 1) {
        return true;
      }
      var word = words[0];
      if (/[/.]/.test(word)) {
        return true;
      }
      return !!KNOWN_SHELL_COMMANDS[word.toLowerCase()];
    }

    function looksLikeShellCommand(cmd) {
      if (!cmd || looksLikeTreeDiagram(cmd)) {
        return false;
      }
      var lines = cmd
        .trim()
        .split("\n")
        .map(function (line) {
          return line.trim();
        })
        .filter(Boolean);
      if (!lines.length) {
        return false;
      }
      return lines.some(plausibleCommandLine);
    }

    function parseCommand(text) {
      if (!text) {
        return null;
      }
      var trimmed = text.trim();
      var jsonMatch = trimmed.match(/\{\s*"tool"\s*:\s*"shell"\s*,\s*"command"\s*:\s*"([^"]+)"\s*\}/);
      if (jsonMatch) {
        return looksLikeShellCommand(jsonMatch[1]) ? jsonMatch[1] : null;
      }
      var fenceMatch = trimmed.match(/```(?:json|bash|sh|shell)?\s*\n([\s\S]+?)```/);
      if (fenceMatch && fenceMatch[1].trim()) {
        var fenced = fenceMatch[1].trim();
        return looksLikeShellCommand(fenced) ? fenced : null;
      }
      return null;
    }

    function trimTrailingFenceOpener(text) {
      return (text || "").replace(/\n?```(?:json|bash|sh|shell)?\s*\n?\s*$/s, "").trim();
    }

    function sanitizeAssistantContent(content) {
      if (!content) {
        return "";
      }
      var cleaned = content;
      for (var i = 0; i < 3; i++) {
        var next = trimTrailingFenceOpener(cleaned);
        next = next.replace(/^```(?:json|bash|sh|shell)?\s*$/gm, "").trim();
        if (next === cleaned) {
          break;
        }
        cleaned = next;
      }
      return cleaned;
    }

    function proposalText(fullText, cmd) {
      if (!fullText || !cmd) {
        return sanitizeAssistantContent(fullText);
      }
      var jsonIdx = fullText.search(/\{\s*"tool"\s*:\s*"shell"/);
      if (jsonIdx >= 0) {
        var beforeJson = trimTrailingFenceOpener(fullText.slice(0, jsonIdx));
        return sanitizeAssistantContent(beforeJson || "将执行命令");
      }
      var fenceRe = /```(?:json|bash|sh|shell)?\s*\n([\s\S]+?)```/g;
      var match;
      while ((match = fenceRe.exec(fullText)) !== null) {
        var fenced = match[1].trim();
        if (fenced === cmd || fenced.indexOf(cmd) >= 0) {
          var beforeFence = trimTrailingFenceOpener(fullText.slice(0, match.index));
          return sanitizeAssistantContent(beforeFence || "将执行命令");
        }
      }
      return sanitizeAssistantContent(fullText);
    }

    function proposedCommand(msg, index, messages) {
      if (!msg) {
        return null;
      }
      if (msg.command) {
        return msg.command;
      }
      var cmd = parseCommand(msg.content);
      if (cmd) {
        return cmd;
      }
      var leadMatch = (msg.content || "").match(/^将执行命令：(.+)$/s);
      if (leadMatch) {
        return leadMatch[1].trim();
      }
      if (index != null && messages && messages[index + 1]) {
        var next = messages[index + 1];
        if (next.role === "tool" && next.command) {
          return next.command;
        }
      }
      return null;
    }

    function assistantDisplayText(msg, cmd) {
      var content = sanitizeAssistantContent(msg.content || "");
      if (!cmd) {
        return content;
      }
      if (content === "将执行命令" || content === "将执行命令：") {
        return content;
      }
      if (content.indexOf("将执行命令：") === 0) {
        return "将执行命令";
      }
      return content;
    }

    function executionScopeLabel(command, msg) {
      if (msg && (msg.tool_name === "read" || msg.tool_name === "write" || msg.tool_name === "edit")) {
        return "本机文件";
      }
      if (!command) {
        return "本机";
      }
      if (/^\s*ssh\b/.test(command)) {
        return "远程";
      }
      if (msg && msg.tool_kind === "ssh") {
        return "远程";
      }
      if (/^\s*(ssh|sshpass)\b/.test(command)) {
        return "远程";
      }
      return "本机";
    }

    function executionScopeHint(command, msg) {
      if (msg && (msg.tool_name === "read" || msg.tool_name === "write" || msg.tool_name === "edit")) {
        return "本地文件操作";
      }
      if (/^\s*ssh\b/.test(command || "")) {
        return "在远程主机执行";
      }
      if (msg && msg.tool_kind === "ssh") {
        return "在远程主机执行";
      }
      if (/^\s*(ssh|sshpass)\b/.test(command || "")) {
        return "在远程主机执行";
      }
      return "在本机执行";
    }

    function toolExecutionWarning(msg, index, messages) {
      if (!msg || !msg.command) {
        return null;
      }
      if (/^\s*(ssh|sshpass)\b/.test(msg.command) || /^\s*ssh\b/.test(msg.command)) {
        return null;
      }
      for (var i = index - 1; i >= 0; i--) {
        var prior = messages[i];
        if (prior.role === "assistant") {
          continue;
        }
        if (prior.role !== "user") {
          break;
        }
        var text = prior.content || "";
        if (/\bssh\b/i.test(text) || /\d{1,3}(?:\.\d{1,3}){3}/.test(text)) {
          return "此命令在本机执行，无法直接连接您提到的远程主机。";
        }
        break;
      }
      return null;
    }

    function roleLabel(role, msg) {
      if (role === "user") {
        return "你";
      }
      if (role === "assistant") {
        return "助手";
      }
      if (role === "tool") {
        return toolRoleLabel(msg);
      }
      return "系统";
    }

    function toolRoleLabel(msg) {
      var name = msg && msg.tool_name;
      if (name === "read") {
        return "读取";
      }
      if (name === "write") {
        return "写入";
      }
      if (name === "edit") {
        return "编辑";
      }
      if (name === "ssh") {
        return "SSH";
      }
      return "Shell";
    }

    function toolActionLabel(msg) {
      var name = msg && msg.tool_name;
      if (name === "read" || name === "write" || name === "edit") {
        return "操作";
      }
      return "命令";
    }

    function toolAvatarText(msg) {
      var name = msg && msg.tool_name;
      if (name === "read") {
        return "读";
      }
      if (name === "write") {
        return "写";
      }
      if (name === "edit") {
        return "编";
      }
      if (name === "ssh") {
        return "远";
      }
      return "$";
    }

    function createRoleAvatar(role, msg) {
      var avatar = document.createElement("div");
      avatar.className = "chat-msg-avatar";
      avatar.setAttribute("aria-hidden", "true");
      if (role === "assistant") {
        avatar.classList.add("chat-msg-avatar-brand");
        var img = document.createElement("img");
        img.src = "/favicon.svg";
        img.alt = "";
        img.width = 18;
        img.height = 18;
        avatar.appendChild(img);
        return avatar;
      }
      if (role === "tool") {
        avatar.textContent = toolAvatarText(msg);
        return avatar;
      }
      avatar.textContent = role === "user" ? "我" : "!";
      return avatar;
    }

    function roleClass(role) {
      if (role === "user") {
        return "chat-msg-user";
      }
      if (role === "tool") {
        return "chat-msg-tool";
      }
      if (role === "system") {
        return "chat-msg-system";
      }
      return "chat-msg-assistant";
    }

    var TOOL_STDOUT_MAX_LINES = 20;

    function buildToolOutputSection(label, text, outputClass, collapseAfterLines) {
      var section = document.createElement("div");
      section.className = "tool-section";
      var sectionLabel = document.createElement("div");
      sectionLabel.className = "tool-section-label";
      sectionLabel.textContent = label;
      var pre = document.createElement("pre");
      pre.className = "tool-output " + outputClass;
      pre.textContent = text;
      section.appendChild(sectionLabel);
      section.appendChild(pre);
      attachCollapsibleToolOutput(section, pre, collapseAfterLines);
      return section;
    }

    function attachCollapsibleToolOutput(section, pre, maxLines) {
      var lineCount = (pre.textContent || "").split("\n").length;
      if (lineCount <= maxLines) {
        return;
      }
      section.classList.add("tool-section-collapsible", "is-collapsed");
      var toggle = document.createElement("button");
      toggle.type = "button";
      toggle.className = "tool-output-toggle";
      toggle.textContent = "展开全部（" + lineCount + " 行）";
      toggle.addEventListener("click", function () {
        var collapsed = section.classList.toggle("is-collapsed");
        toggle.textContent = collapsed
          ? "展开全部（" + lineCount + " 行）"
          : "收起输出";
      });
      section.appendChild(toggle);
    }

    function buildToolMessageBody(msg, index, messages) {
      var body = document.createElement("div");
      body.className = "chat-tool-body";

      if (msg.command) {
        var warning = toolExecutionWarning(msg, index, messages);
        if (warning) {
          var warn = document.createElement("div");
          warn.className = "tool-scope-warning";
          warn.textContent = warning;
          body.appendChild(warn);
        }

        var cmdSection = document.createElement("div");
        cmdSection.className = "tool-section";
        cmdSection.innerHTML =
          '<div class="tool-section-label">' + escapeHtml(toolActionLabel(msg)) + "</div>" +
          '<pre class="tool-output tool-cmd">' +
          escapeHtml(msg.command) +
          "</pre>";
        body.appendChild(cmdSection);
      }

      if (msg.exit_code != null) {
        var status = document.createElement("div");
        status.className = "tool-status";
        var badge = document.createElement("span");
        badge.className =
          "tool-exit-badge" + (msg.exit_code === 0 ? " is-success" : " is-error");
        badge.textContent = "退出码 " + msg.exit_code;
        status.appendChild(badge);
        body.appendChild(status);
      }

      if (msg.stdout) {
        body.appendChild(
          buildToolOutputSection("标准输出", msg.stdout, "tool-stdout", TOOL_STDOUT_MAX_LINES)
        );
      }

      if (msg.stderr) {
        var errSection = document.createElement("div");
        errSection.className = "tool-section";
        errSection.innerHTML =
          '<div class="tool-section-label">标准错误</div>' +
          '<pre class="tool-output tool-stderr">' +
          escapeHtml(msg.stderr) +
          "</pre>";
        body.appendChild(errSection);
      }

      if (!msg.command && !msg.stdout && !msg.stderr && msg.content) {
        var fallback = document.createElement("pre");
        fallback.className = "tool-output";
        fallback.textContent = msg.content;
        body.appendChild(fallback);
      }

      return body;
    }

    function countFollowingToolMessages(index, messages) {
      var count = 0;
      for (var i = index + 1; i < messages.length; i++) {
        if (messages[i].role === "tool") {
          count++;
        } else {
          break;
        }
      }
      return count;
    }

    function toolCallSummaryLabel(tc) {
      var name = (tc && tc.name) || "tool";
      var args = {};
      try {
        args = JSON.parse((tc && tc.arguments) || "{}");
      } catch (_e) {
        args = {};
      }
      if (name === "bash" && args.command) {
        return "bash · " + args.command;
      }
      if (name === "read" && args.path) {
        var readLabel = "read · " + args.path;
        if (args.offset || args.limit) {
          readLabel += " (";
          if (args.offset) {
            readLabel += "offset=" + args.offset;
          }
          if (args.limit) {
            readLabel += (args.offset ? ", " : "") + "limit=" + args.limit;
          }
          readLabel += ")";
        }
        return readLabel;
      }
      if (name === "write" && args.path) {
        return "write · " + args.path;
      }
      if (name === "edit" && args.path) {
        return "edit · " + args.path;
      }
      if (name === "ssh" && args.remote_command) {
        return "ssh · " + args.remote_command;
      }
      return name;
    }

    function buildToolCallsSummary(msg) {
      var calls = (msg && msg.tool_calls) || [];
      if (!calls.length) {
        return "";
      }
      var items = calls.map(function (tc) {
        return "<li>" + escapeHtml(toolCallSummaryLabel(tc)) + "</li>";
      });
      return (
        '<div class="chat-tool-calls-summary">' +
        '<div class="chat-tool-calls-label">调用 ' +
        calls.length +
        " 个工具</div>" +
        '<ul class="chat-tool-calls-list">' +
        items.join("") +
        "</ul></div>"
      );
    }

    function shouldShowProposalBlock(msg, index, messages) {
      var cmd = proposedCommand(msg, index, messages);
      if (!cmd) {
        return false;
      }
      var state = proposalState(msg, index, messages);
      if (state === "pending") {
        return true;
      }
      if (msg.tool_calls && msg.tool_calls.length > 0) {
        return false;
      }
      if (state === "proposed") {
        var content = (msg.content || "").trim();
        if (content.length > 300 && !isCommandPending(cmd)) {
          return false;
        }
        return true;
      }
      if (state === "executed" || state === "rejected") {
        return countFollowingToolMessages(index, messages) === 0;
      }
      return false;
    }

    function buildProposalBlock(cmd, risk, state) {
      var titles = {
        proposed: "命令提议",
        pending: "待您审批",
        executed: "已自动执行",
        rejected: "已拒绝执行",
      };
      var title = titles[state] || titles.proposed;
      var html =
        '<div class="chat-proposal' +
        (state === "executed" ? " is-executed" : "") +
        (state === "rejected" ? " is-rejected" : "") +
        '">' +
        '<div class="chat-proposal-title">' +
        escapeHtml(title) +
        "</div>" +
        '<div class="chat-proposal-cmd">' +
        escapeHtml(cmd) +
        "</div>";
      if (risk) {
        html +=
          '<div class="chat-proposal-risk">风险: ' +
          escapeHtml(RISK_LABELS[risk] || risk) +
          "</div>";
      }
      html += "</div>";
      return html;
    }

    function hasPendingApprovals() {
      return !!(session && session.pending_approvals && session.pending_approvals.length > 0);
    }

    function isCommandPending(cmd) {
      if (!cmd || !session || !session.pending_approvals) {
        return false;
      }
      return session.pending_approvals.some(function (approval) {
        return approval.command === cmd;
      });
    }

    function isRejectionToolMessage(msg) {
      return !!(msg && msg.content && msg.content.indexOf("Status: rejected by user") !== -1);
    }

    function proposalState(msg, index, messages) {
      var cmd = proposedCommand(msg, index, messages);
      if (!cmd) {
        return null;
      }
      if (isCommandPending(cmd)) {
        return "pending";
      }
      var next = messages[index + 1];
      if (next && next.role === "tool" && next.command === cmd) {
        if (isRejectionToolMessage(next)) {
          return "rejected";
        }
        return "executed";
      }
      return "proposed";
    }

    function updateComposerState() {
      var pending = hasPendingApprovals();
      els.input.disabled = pending || streaming;
      els.attachBtn.disabled = pending || streaming;
      if (els.input) {
        els.input.placeholder = pending
          ? "请先处理下方待审批命令…"
          : "描述环境问题或排查需求…";
      }
    }

    function renderMessage(msg, index, messages) {
      var wrap = document.createElement("article");
      wrap.className = "chat-msg " + roleClass(msg.role);
      wrap.dataset.messageId = msg.id || "";

      var avatar = createRoleAvatar(msg.role, msg);

      var content = document.createElement("div");
      content.className = "chat-msg-content";

      var meta = document.createElement("div");
      meta.className = "chat-msg-meta";
      var timeText = formatMessageTime(msg.timestamp);
      var roleBadge = '<span class="chat-role-badge">' + escapeHtml(roleLabel(msg.role, msg)) + "</span>";
      if (msg.role === "tool") {
        var scopeLabel = executionScopeLabel(msg.command, msg);
        meta.innerHTML =
          roleBadge +
          ' <span class="tool-scope-badge" title="' +
          escapeHtml(executionScopeHint(msg.command, msg)) +
          '">' +
          escapeHtml(scopeLabel) +
          "</span>" +
          (timeText ? '<span class="chat-msg-time">' + escapeHtml(timeText) + "</span>" : "");
      } else {
        meta.innerHTML =
          roleBadge +
          (timeText ? '<span class="chat-msg-time">' + escapeHtml(timeText) + "</span>" : "");
      }

      var body = document.createElement("div");
      body.className = "chat-msg-body";

      if (msg.role === "tool") {
        body.appendChild(buildToolMessageBody(msg, index, messages));
      } else {
        var attEl = buildMessageAttachments(msg);
        if (attEl) {
          body.appendChild(attEl);
        }
        if (msg.content) {
          var textNode = document.createElement("div");
          textNode.className = "chat-msg-text";
          if (msg.role === "assistant" || msg.role === "system") {
            textNode.className += " md-content";
            var displayContent =
              msg.role === "assistant"
                ? assistantDisplayText(msg, proposedCommand(msg, index, messages))
                : msg.content;
            if (displayContent) {
              textNode.innerHTML = renderMarkdownBlock(displayContent);
              body.appendChild(textNode);
            }
          } else {
            textNode.textContent = msg.content;
            body.appendChild(textNode);
          }
        }
        if (msg.role === "assistant" && msg.tool_calls && msg.tool_calls.length > 0) {
          body.insertAdjacentHTML("beforeend", buildToolCallsSummary(msg));
        }
        if (msg.role === "assistant") {
          var cmd = proposedCommand(msg, index, messages);
          if (cmd && shouldShowProposalBlock(msg, index, messages)) {
            var state = proposalState(msg, index, messages);
            body.insertAdjacentHTML(
              "beforeend",
              buildProposalBlock(cmd, msg.risk, state)
            );
          }
        }
      }

      content.appendChild(meta);
      content.appendChild(body);
      wrap.appendChild(avatar);
      wrap.appendChild(content);
      return wrap;
    }

    function renderEmptyState() {
      var empty = document.createElement("div");
      empty.className = "chat-empty";

      var icon = document.createElement("div");
      icon.className = "chat-empty-icon";
      icon.setAttribute("aria-hidden", "true");
      icon.innerHTML =
        '<svg width="32" height="32" viewBox="0 0 24 24" fill="none">' +
        '<path d="M12 3l8 4.5v9L12 21l-8-4.5v-9L12 3z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>' +
        '<path d="M12 12l8-4.5M12 12v9M12 12L4 7.5" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>' +
        "</svg>";

      var title = document.createElement("p");
      title.className = "chat-empty-title";
      title.textContent = "开始环境诊断";

      var desc = document.createElement("p");
      desc.className = "chat-empty-desc";
      desc.textContent = "描述你遇到的问题，或从下方快捷入口开始排查";

      var suggestions = document.createElement("div");
      suggestions.className = "chat-empty-suggestions";
      EMPTY_SUGGESTIONS.forEach(function (text) {
        var chip = document.createElement("button");
        chip.type = "button";
        chip.className = "chat-suggestion-chip";
        chip.textContent = text;
        chip.addEventListener("click", function () {
          if (streaming) {
            return;
          }
          sendMessage(text);
        });
        suggestions.appendChild(chip);
      });

      empty.appendChild(icon);
      empty.appendChild(title);
      empty.appendChild(desc);
      empty.appendChild(suggestions);
      return empty;
    }

    function renderMessages() {
      els.messages.innerHTML = "";
      if (!session || !session.messages || session.messages.length === 0) {
        els.messages.appendChild(renderEmptyState());
        return;
      }
      session.messages.forEach(function (msg, index) {
        els.messages.appendChild(renderMessage(msg, index, session.messages));
      });
      els.messages.scrollTop = els.messages.scrollHeight;
    }

    function appendNewMessages(messages) {
      if (!messages || !messages.length) {
        return;
      }
      var empty = els.messages.querySelector(".chat-empty");
      if (empty) {
        empty.remove();
      }
      var knownIds = {};
      els.messages.querySelectorAll("[data-message-id]").forEach(function (el) {
        if (el.dataset.messageId) {
          knownIds[el.dataset.messageId] = true;
        }
      });
      var toAppend = [];
      messages.forEach(function (msg) {
        if (msg.id && knownIds[msg.id]) {
          return;
        }
        toAppend.push(msg);
      });
      if (!toAppend.length) {
        return;
      }
      var streamingEl = document.getElementById("chat-streaming-msg");
      var allMessages = session && session.messages ? session.messages : messages;
      toAppend.forEach(function (msg) {
        var index = allMessages.indexOf(msg);
        if (index < 0) {
          index = allMessages.length - 1;
        }
        var el = renderMessage(msg, index, allMessages);
        if (streamingEl) {
          els.messages.insertBefore(el, streamingEl);
        } else {
          els.messages.appendChild(el);
        }
      });
      els.messages.scrollTop = els.messages.scrollHeight;
    }

    function syncSessionIncremental(next) {
      session = next;
      sessionId = next.id;
      if (next.policy_mode) {
        els.policy.value = next.policy_mode;
      }
      renderApprovals();
      appendNewMessages(next.messages);
      updateComposerState();
    }

    function mergeAssistantIntoSession(content) {
      if (!content || !session) {
        return;
      }
      if (!session.messages) {
        session.messages = [];
      }
      var last = session.messages[session.messages.length - 1];
      if (last && last.role === "assistant" && last.content === content) {
        return;
      }
      session.messages.push({ role: "assistant", content: content });
    }

    function renderApprovals() {
      els.approvals.innerHTML = "";
      if (!session || !session.pending_approvals || session.pending_approvals.length === 0) {
        updateComposerState();
        return;
      }

      session.pending_approvals.forEach(function (approval) {
        var card = document.createElement("div");
        card.className = "chat-approval-card";
        card.dataset.approvalId = approval.id;

        var header = document.createElement("div");
        header.className = "chat-approval-header";

        var title = document.createElement("span");
        title.className = "chat-approval-title";
        title.textContent = "待审批命令";

        var risk = document.createElement("span");
        risk.className = "chat-approval-risk";
        risk.textContent = RISK_LABELS[approval.risk] || approval.risk || "未知";

        header.appendChild(title);
        header.appendChild(risk);

        var cmd = document.createElement("pre");
        cmd.className = "chat-approval-cmd";
        cmd.textContent = approval.command;

        var actions = document.createElement("div");
        actions.className = "chat-approval-actions";

        var approveBtn = document.createElement("button");
        approveBtn.type = "button";
        approveBtn.className = "btn btn-approve";
        approveBtn.textContent = "批准执行";
        approveBtn.addEventListener("click", function () {
          handleApproval(approval.id, true);
        });

        var rejectBtn = document.createElement("button");
        rejectBtn.type = "button";
        rejectBtn.className = "btn btn-reject";
        rejectBtn.textContent = "拒绝";
        rejectBtn.addEventListener("click", function () {
          handleApproval(approval.id, false);
        });

        actions.appendChild(approveBtn);
        actions.appendChild(rejectBtn);

        card.appendChild(header);
        card.appendChild(cmd);
        card.appendChild(actions);
        els.approvals.appendChild(card);
      });
      updateComposerState();
    }

    function sessionTitleFromMessages(messages) {
      if (!messages) {
        return "新对话";
      }
      for (var i = 0; i < messages.length; i++) {
        if (messages[i].role === "user") {
          var t = (messages[i].content || "").trim().replace(/\n/g, " ");
          if (!t && messages[i].attachments && messages[i].attachments.length) {
            t = "[" + messages[i].attachments[0].filename + "]";
          }
          if (t) {
            if (t.length > 48) {
              return t.slice(0, 48) + "…";
            }
            return t;
          }
        }
      }
      return "新对话";
    }

    function updateSessionLink(id) {
      if (!els.copyLink) {
        return;
      }
      if (id) {
        els.copyLink.hidden = false;
        els.copyLink.dataset.sessionId = id;
        els.copyLink.title = "复制会话链接";
      } else {
        els.copyLink.hidden = true;
        els.copyLink.dataset.sessionId = "";
      }
    }

    function applySession(next, urlOptions) {
      session = next;
      sessionId = next.id;
      try {
        localStorage.setItem(SESSION_STORAGE_KEY, sessionId);
      } catch (_e) {
        /* ignore */
      }
      if (next.policy_mode) {
        els.policy.value = next.policy_mode;
      }
      sessionSidebar.setActive(sessionId);
      sessionSidebar.setTitle(sessionTitleFromMessages(next.messages));
      updateSessionLink(sessionId);
      writeUrlState({ sessionId: sessionId }, urlOptions && urlOptions.replace);
      try {
        renderMessages();
        renderApprovals();
      } catch (err) {
        setError("消息渲染失败: " + (err && err.message ? err.message : String(err)));
        throw err;
      }
      updateComposerState();
      filesPanel.setSession(sessionId);
    }

    function updateLlmNotice() {
      if (!els.llmNotice) {
        return;
      }
      show(els.llmNotice, !isLlmConfigured(config));
    }

    function refreshLlmState() {
      return loadConfig().then(function (cfg) {
        updateLlmNotice();
        return cfg;
      });
    }

    function onConfigSaved(cfg) {
      config = cfg;
      updateLlmNotice();
    }

    function loadConfig() {
      return apiFetch("/api/config").then(function (cfg) {
        config = cfg;
        if (cfg.policy && cfg.policy.default) {
          els.policy.value = cfg.policy.default;
        } else if (!session) {
          els.policy.value = "open";
        }
        updateLlmNotice();
        return cfg;
      });
    }

    function createSession(policyMode, urlOptions) {
      return apiFetch("/api/sessions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ policy_mode: policyMode }),
      }).then(function (created) {
        applySession(created, urlOptions);
        return created;
      });
    }

    function updateSessionPolicy(policyMode) {
      if (!sessionId) {
        return Promise.resolve(null);
      }
      return apiFetch("/api/sessions/" + encodeURIComponent(sessionId), {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ policy_mode: policyMode }),
      }).then(function (updated) {
        session = updated;
        if (updated.policy_mode) {
          els.policy.value = updated.policy_mode;
        }
        return updated;
      });
    }

    function loadSession(id, urlOptions) {
      return apiFetch("/api/sessions/" + encodeURIComponent(id)).then(function (loaded) {
        applySession(loaded, urlOptions);
        return loaded;
      });
    }

    function deleteSession(id) {
      if (streaming) {
        return Promise.resolve();
      }
      return apiFetch("/api/sessions/" + encodeURIComponent(id), {
        method: "DELETE",
      }).then(function () {
        var wasActive = id === sessionId;
        setError("");
        return sessionSidebar.refresh().then(function () {
          if (!wasActive) {
            return null;
          }
          return apiFetch("/api/sessions").then(function (data) {
            var remaining = data.sessions || [];
            if (remaining.length > 0) {
              return loadSession(remaining[0].id);
            }
            return createSession(els.policy.value);
          });
        });
      });
    }

    function ensureSession() {
      if (sessionId) {
        return Promise.resolve(session);
      }
      return createSession(els.policy.value);
    }

    function navigateFromUrl() {
      var urlState = readUrlState();
      if (urlState.view === "settings") {
        activateView("settings");
        settingsPanel.initTabFromUrl(urlState.settingsTab);
      } else {
        activateView("chat");
      }

      if (!urlState.sessionId) {
        return Promise.resolve(null);
      }
      if (urlState.sessionId === sessionId && session) {
        return Promise.resolve(session);
      }
      setError("");
      return loadSession(urlState.sessionId, { replace: true }).catch(function (err) {
        setError("加载会话失败: " + err.message);
        writeUrlState({ sessionId: null }, true);
        return createSession(els.policy.value, { replace: true });
      });
    }

    function init() {
      if (initialized) {
        return;
      }
      initialized = true;

      sessionSidebar.onSelect(function (id) {
        if (id === sessionId || streaming) {
          return;
        }
        setError("");
        activateView("chat");
        loadSession(id).catch(function (err) {
          setError("加载会话失败: " + err.message);
        });
      });

      sessionSidebar.onDelete(function (id) {
        if (streaming) {
          return;
        }
        deleteSession(id).catch(function (err) {
          setError("删除会话失败: " + err.message);
        });
      });

      window.__chatPanelNewSession = function () {
        if (streaming) {
          return;
        }
        setError("");
        activateView("chat");
        createSession(els.policy.value).then(function () {
          sessionSidebar.refresh();
        });
      };

      if (els.copyLink) {
        els.copyLink.addEventListener("click", function () {
          var id = els.copyLink.dataset.sessionId || sessionId;
          if (!id) {
            return;
          }
          var link = sessionShareUrl(id);
          copyTextToClipboard(link).then(function () {
            els.copyLink.classList.add("is-copied");
            els.copyLink.title = "已复制链接";
            setTimeout(function () {
              els.copyLink.classList.remove("is-copied");
              els.copyLink.title = "复制会话链接";
            }, 1600);
          });
        });
      }

      window.addEventListener("popstate", function () {
        if (streaming) {
          return;
        }
        navigateFromUrl();
      });

      resizeInput();

      var urlState = readUrlState();
      if (urlState.view === "settings") {
        activateView("settings");
        settingsPanel.initTabFromUrl(urlState.settingsTab);
      }

      sessionSidebar.refresh();

      var bootstrapId = urlState.sessionId;
      if (!bootstrapId) {
        try {
          bootstrapId = localStorage.getItem(SESSION_STORAGE_KEY);
        } catch (_e) {
          /* ignore */
        }
      }

      loadConfig()
        .then(function () {
          if (bootstrapId) {
            return loadSession(bootstrapId, { replace: true }).catch(function (err) {
              setError("加载会话失败: " + (err && err.message ? err.message : String(err)));
              throw err;
            });
          }
          return createSession(els.policy.value, { replace: true });
        })
        .then(function () {
          return sessionSidebar.refresh();
        })
        .catch(function (err) {
          setError("初始化会话失败: " + err.message);
        });

      if (els.llmNoticeLink) {
        els.llmNoticeLink.addEventListener("click", function () {
          openLlmSettings();
        });
      }
    }

    function persistPolicyDefault(mode) {
      if (!config) {
        return Promise.resolve();
      }
      var payload = JSON.parse(JSON.stringify(config));
      if (!payload.policy) {
        payload.policy = {};
      }
      payload.policy.default = mode;
      return apiFetch("/api/config", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      }).then(function (saved) {
        config = saved;
      });
    }

    function onPolicyChange() {
      var mode = els.policy.value;
      setError("");
      persistPolicyDefault(mode)
        .then(function () {
          return updateSessionPolicy(mode);
        })
        .then(function (updated) {
          if (!updated) {
            return;
          }
          applySession(updated);
        })
        .catch(function (err) {
          setError("更新策略失败: " + err.message);
        });
    }

    function setStreaming(active) {
      streaming = active;
      els.send.classList.toggle("is-streaming", active);
      els.send.setAttribute("aria-label", active ? "停止回复" : "发送");
      if (els.statusDot) {
        els.statusDot.classList.toggle("is-busy", active);
        els.statusDot.title = active ? "回复中" : "就绪";
      }
      updateComposerState();
    }

    function cancelStreaming() {
      if (!streaming || !activeAbortController) {
        return;
      }
      activeAbortController.abort();
    }

    function cleanupAfterAbort() {
      if (!activeStreamState) {
        return;
      }
      var state = activeStreamState;
      if (state.textEl && state.fullText) {
        finalizeStreamingAssistant(state.textEl, state.fullText);
        mergeAssistantIntoSession(state.fullText);
      } else {
        var streamWrap = document.getElementById("chat-streaming-msg");
        if (streamWrap) {
          streamWrap.remove();
        }
      }
      activeStreamState = null;
    }

    function removeStreamingAssistant() {
      var streamWrap = document.getElementById("chat-streaming-msg");
      if (streamWrap) {
        streamWrap.remove();
      }
    }

    function ensureStreamingCursor(body) {
      if (!body || body.querySelector(".chat-cursor")) {
        return;
      }
      var cursor = document.createElement("span");
      cursor.className = "chat-cursor";
      cursor.setAttribute("aria-hidden", "true");
      body.appendChild(cursor);
    }

    function appendStreamingAssistant() {
      var wrap = document.createElement("article");
      wrap.className = "chat-msg chat-msg-assistant streaming is-thinking";
      wrap.id = "chat-streaming-msg";

      var avatar = createRoleAvatar("assistant");

      var content = document.createElement("div");
      content.className = "chat-msg-content";

      var meta = document.createElement("div");
      meta.className = "chat-msg-meta";
      meta.innerHTML = '<span class="chat-role-badge">助手</span>';

      var body = document.createElement("div");
      body.className = "chat-msg-body";
      body.innerHTML =
        '<div class="chat-msg-thinking" aria-live="polite">' +
          '<span class="chat-thinking-dots" aria-hidden="true"><span></span><span></span><span></span></span>' +
          '<span class="chat-thinking-label">正在思考…</span>' +
        "</div>" +
        '<div class="chat-msg-text md-content chat-stream-text" hidden></div>';

      content.appendChild(meta);
      content.appendChild(body);
      wrap.appendChild(avatar);
      wrap.appendChild(content);
      els.messages.appendChild(wrap);
      els.messages.scrollTop = els.messages.scrollHeight;
      return body.querySelector(".chat-stream-text");
    }

    function updateStreamingAssistantText(textEl, fullText) {
      if (!textEl) {
        return;
      }
      var cmd = parseCommand(fullText);
      var displayText = cmd ? proposalText(fullText, cmd) : sanitizeAssistantContent(fullText);
      if (!displayText.trim()) {
        return;
      }
      var streamWrap = document.getElementById("chat-streaming-msg");
      if (streamWrap) {
        var thinking = streamWrap.querySelector(".chat-msg-thinking");
        if (thinking) {
          thinking.remove();
        }
        streamWrap.classList.remove("is-thinking");
        streamWrap.classList.add("has-content");
        ensureStreamingCursor(streamWrap.querySelector(".chat-msg-body"));
      }
      textEl.hidden = false;
      textEl.innerHTML = renderMarkdownBlock(displayText);
      els.messages.scrollTop = els.messages.scrollHeight;
    }

    function consumeSSE(reader, onEvent) {
      var decoder = new TextDecoder();
      var buffer = "";

      function parseBlock(block) {
        if (!block.trim()) {
          return null;
        }
        var lines = block.split("\n");
        var eventName = "message";
        var dataLines = [];
        lines.forEach(function (line) {
          if (line.indexOf("event:") === 0) {
            eventName = line.slice(6).trim();
          } else if (line.indexOf("data:") === 0) {
            dataLines.push(line.slice(5).trim());
          }
        });
        if (!dataLines.length) {
          return null;
        }
        return onEvent(eventName, dataLines.join("\n"));
      }

      function pump() {
        return reader.read().then(function (result) {
          if (result.done) {
            return null;
          }
          buffer += decoder.decode(result.value, { stream: true });
          var parts = buffer.split("\n\n");
          buffer = parts.pop() || "";

          var chain = Promise.resolve(null);
          parts.forEach(function (block) {
            chain = chain.then(function () {
              return parseBlock(block);
            });
          });
          return chain.then(function () {
            return pump();
          });
        });
      }

      return pump();
    }

    function finalizeStreamingAssistant(textEl, fullText, options) {
      var streamWrap = document.getElementById("chat-streaming-msg");
      if (!streamWrap) {
        return;
      }
      streamWrap.classList.remove("streaming", "is-thinking", "has-content");
      var body = streamWrap.querySelector(".chat-msg-body");
      var thinking = body.querySelector(".chat-msg-thinking");
      if (thinking) {
        thinking.remove();
      }
      var cursor = body.querySelector(".chat-cursor");
      if (cursor) {
        cursor.remove();
      }
      var textNode = textEl || body.querySelector(".chat-stream-text");
      if (!textNode) {
        textNode = document.createElement("div");
        body.appendChild(textNode);
      }
      textNode.className = "chat-msg-text md-content";
      var cmd = parseCommand(fullText);
      var displayText = cmd ? proposalText(fullText, cmd) : sanitizeAssistantContent(fullText);
      textNode.innerHTML = renderMarkdownBlock(displayText);
      if (cmd) {
        var state = "proposed";
        if (options && options.pending) {
          state = "pending";
        } else if (options && options.executed) {
          state = "executed";
        }
        if (state !== "pending") {
          body.insertAdjacentHTML(
            "beforeend",
            buildProposalBlock(cmd, options && options.risk, state)
          );
        }
      }
      streamWrap.removeAttribute("id");
    }

    function commandPendingInList(cmd, approvals) {
      if (!cmd || !approvals || !approvals.length) {
        return false;
      }
      return approvals.some(function (approval) {
        return approval.command === cmd;
      });
    }

    function sendMessage(text, attachmentIds, attachmentMeta) {
      if (hasPendingApprovals()) {
        setError("请先处理待审批命令");
        return;
      }
      setError("");
      setStreaming(true);
      activeAbortController = new AbortController();
      activeStreamState = { textEl: null, fullText: "" };

      var userMsg = {
        role: "user",
        content: text,
        attachments: attachmentMeta || [],
      };
      if (!session) {
        session = { messages: [] };
      }
      if (!session.messages) {
        session.messages = [];
      }
      session.messages.push(userMsg);
      renderMessages();

      ensureSession()
        .then(function () {
          return fetch("/api/chat", Object.assign({
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              session_id: sessionId,
              message: text,
              attachments: attachmentIds || [],
            }),
            signal: activeAbortController.signal,
          }, FETCH_CREDENTIALS));
        })
        .then(function (res) {
          if (res.status === 401) {
            authPanel.requireLogin();
            throw new Error("未登录");
          }
          if (!res.ok) {
            return res.json().then(function (body) {
              throw new Error(body && body.error ? body.error : "聊天请求失败 (" + res.status + ")");
            });
          }
          if (!res.body) {
            throw new Error("浏览器不支持流式响应");
          }

          var textEl = null;
          var fullText = "";
          var reloadedSession = null;

          function syncStreamState() {
            activeStreamState = { textEl: textEl, fullText: fullText };
          }

          function startAssistantStream() {
            if (textEl && fullText) {
              finalizeStreamingAssistant(textEl, fullText);
              mergeAssistantIntoSession(fullText);
            } else {
              removeStreamingAssistant();
            }
            textEl = appendStreamingAssistant();
            fullText = "";
            syncStreamState();
          }

          function updateStreamingStatus(label) {
            var streamWrap = document.getElementById("chat-streaming-msg");
            if (!streamWrap) {
              return;
            }
            var thinkingLabel = streamWrap.querySelector(".chat-thinking-label");
            if (thinkingLabel) {
              thinkingLabel.textContent = label || "正在思考…";
            }
          }

          return consumeSSE(res.body.getReader(), function (eventName, data) {
            if (eventName === "turn_start") {
              startAssistantStream();
              return null;
            }
            if (eventName === "executing") {
              if (!textEl) {
                textEl = appendStreamingAssistant();
                syncStreamState();
              }
              var execPayload = JSON.parse(data);
              updateStreamingStatus(
                execPayload.command
                  ? "正在执行：" + execPayload.command
                  : "正在执行命令…"
              );
              return null;
            }
            if (eventName === "session") {
              syncSessionIncremental(JSON.parse(data));
              return null;
            }
            if (eventName === "chunk") {
              if (!textEl) {
                textEl = appendStreamingAssistant();
              }
              var chunk = JSON.parse(data);
              if (chunk.content) {
                fullText += chunk.content;
                syncStreamState();
                updateStreamingAssistantText(textEl, fullText);
              }
              return null;
            }
            if (eventName === "error") {
              var errPayload = JSON.parse(data);
              throw new Error(errPayload.error || "流式响应错误");
            }
            if (eventName === "done") {
              var donePayload = JSON.parse(data);
              if (textEl && fullText) {
                var doneCmd = parseCommand(fullText);
                finalizeStreamingAssistant(textEl, fullText, {
                  pending: commandPendingInList(doneCmd, donePayload.pending_approvals),
                  risk: doneCmd && donePayload.pending_approvals
                    ? (donePayload.pending_approvals.find(function (a) {
                        return a.command === doneCmd;
                      }) || {}).risk
                    : null,
                });
              } else {
                removeStreamingAssistant();
              }
              if (donePayload.session_id) {
                sessionId = donePayload.session_id;
                try {
                  localStorage.setItem(SESSION_STORAGE_KEY, sessionId);
                } catch (_e) {
                  /* ignore */
                }
                updateSessionLink(sessionId);
                writeUrlState({ sessionId: sessionId }, true);
              }
              if (donePayload.pending_approvals) {
                if (!session) {
                  session = { messages: [], pending_approvals: [] };
                }
                session.pending_approvals = donePayload.pending_approvals;
                renderApprovals();
              }
              reloadedSession = loadSession(sessionId);
              return reloadedSession;
            }
            return null;
          }).then(function () {
            return reloadedSession;
          });
        })
        .then(function (loaded) {
          if (loaded) {
            applySession(loaded);
          }
          return sessionSidebar.refresh();
        })
        .catch(function (err) {
          if (err.name === "AbortError") {
            cleanupAfterAbort();
            return null;
          }
          var streamWrap = document.getElementById("chat-streaming-msg");
          if (streamWrap) {
            streamWrap.remove();
          }
          setError(err.message);
        })
        .finally(function () {
          activeAbortController = null;
          activeStreamState = null;
          setStreaming(false);
        });
    }

    function setApprovalLoading(approvalId, loading) {
      if (!els.approvals) {
        return;
      }
      var card = els.approvals.querySelector('[data-approval-id="' + approvalId + '"]');
      if (!card) {
        return;
      }
      card.classList.toggle("is-processing", loading);
      card.querySelectorAll("button").forEach(function (btn) {
        btn.disabled = loading;
      });
      var status = card.querySelector(".chat-approval-status");
      if (loading) {
        if (!status) {
          status = document.createElement("div");
          status.className = "chat-approval-status";
          status.textContent = "正在执行命令…";
          card.appendChild(status);
        }
      } else if (status) {
        status.remove();
      }
    }

    function handleApproval(approvalId, approved) {
      if (!sessionId || streaming) {
        return;
      }
      setError("");
      setStreaming(true);
      activeAbortController = new AbortController();
      activeStreamState = { textEl: null, fullText: "" };
      setApprovalLoading(approvalId, true);

      function runApproval() {
        fetch("/api/sessions/" + encodeURIComponent(sessionId) + "/approve", Object.assign({
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ approval_id: approvalId, approved: approved }),
          signal: activeAbortController.signal,
        }, FETCH_CREDENTIALS))
          .then(function (res) {
            if (res.status === 401) {
              authPanel.requireLogin();
              throw new Error("未登录");
            }
            if (!res.ok) {
              return res.json().then(function (body) {
                throw new Error(body && body.error ? body.error : "审批请求失败 (" + res.status + ")");
              });
            }
            if (!res.body) {
              throw new Error("浏览器不支持流式响应");
            }

            var textEl = null;
            var fullText = "";

            function syncStreamState() {
              activeStreamState = { textEl: textEl, fullText: fullText };
            }

            function startAssistantStream() {
              if (textEl && fullText) {
                finalizeStreamingAssistant(textEl, fullText);
                mergeAssistantIntoSession(fullText);
              } else {
                removeStreamingAssistant();
              }
              textEl = appendStreamingAssistant();
              fullText = "";
              syncStreamState();
            }

            return consumeSSE(res.body.getReader(), function (eventName, data) {
              if (eventName === "started" || eventName === "executing") {
                return null;
              }
              if (eventName === "session") {
                syncSessionIncremental(JSON.parse(data));
                return null;
              }
              if (eventName === "turn_start") {
                startAssistantStream();
                return null;
              }
              if (eventName === "chunk") {
                if (!textEl) {
                  startAssistantStream();
                }
                var chunk = JSON.parse(data);
                if (chunk.content) {
                  fullText += chunk.content;
                  syncStreamState();
                  updateStreamingAssistantText(textEl, fullText);
                }
                return null;
              }
              if (eventName === "error") {
                var errPayload = JSON.parse(data);
                throw new Error(errPayload.error || "审批流式响应错误");
              }
              if (eventName === "done") {
                if (textEl && fullText) {
                  finalizeStreamingAssistant(textEl, fullText);
                } else {
                  removeStreamingAssistant();
                }
                var donePayload = JSON.parse(data);
                if (donePayload.pending_approvals) {
                  if (!session) {
                    session = { messages: [], pending_approvals: [] };
                  }
                  session.pending_approvals = donePayload.pending_approvals;
                  renderApprovals();
                }
                return loadSession(sessionId).then(function (loaded) {
                  applySession(loaded);
                  return sessionSidebar.refresh();
                });
              }
              return null;
            });
          })
          .catch(function (err) {
            if (err.name === "AbortError") {
              cleanupAfterAbort();
              setApprovalLoading(approvalId, false);
              return null;
            }
            setError("审批操作失败: " + err.message);
            setApprovalLoading(approvalId, false);
          })
          .finally(function () {
            activeAbortController = null;
            activeStreamState = null;
            setStreaming(false);
          });
      }

      requestAnimationFrame(function () {
        requestAnimationFrame(runApproval);
      });
    }

    function onSubmit(event) {
      event.preventDefault();
      if (streaming) {
        cancelStreaming();
        return;
      }
      if (hasPendingApprovals()) {
        setError("请先处理待审批命令");
        return;
      }
      var text = els.input.value.trim();
      if (inputCharCount(text) > INPUT_MAX_CHARS) {
        setError("消息不能超过 " + INPUT_MAX_CHARS + " 个字符");
        return;
      }
      var attachmentIds = pendingAttachments.map(function (att) {
        return att.id;
      });
      var attachmentMeta = pendingAttachments.map(function (att) {
        return {
          id: att.id,
          filename: att.filename,
          mime_type: att.mime_type,
          size: att.size,
        };
      });
      if (!text && !attachmentIds.length) {
        return;
      }
      els.input.value = "";
      resetInputHeight();
      clearPendingAttachments();
      sendMessage(text, attachmentIds, attachmentMeta);
    }

    els.form.addEventListener("submit", onSubmit);
    els.policy.addEventListener("change", onPolicyChange);

    els.attachBtn.addEventListener("click", function () {
      if (!streaming) {
        els.fileInput.click();
      }
    });

    els.fileInput.addEventListener("change", function () {
      uploadFiles(els.fileInput.files);
      els.fileInput.value = "";
    });

    els.form.addEventListener("dragover", function (event) {
      event.preventDefault();
    });

    els.form.addEventListener("drop", function (event) {
      event.preventDefault();
      if (streaming) {
        return;
      }
      uploadFiles(event.dataTransfer.files);
    });

    els.input.addEventListener("input", resizeInput);

    els.input.addEventListener("keydown", function (event) {
      if (event.key === "Enter" && !event.shiftKey) {
        event.preventDefault();
        els.form.requestSubmit();
      }
    });

    return {
      init: init,
      refreshLlmState: refreshLlmState,
      onConfigSaved: onConfigSaved,
    };
  })();

  authPanel.setOnAuthenticated(function () {
    chatPanel.init();
    filesPanel.init();
    settingsPanel.setOnConfigSaved(chatPanel.onConfigSaved);
  });
  authPanel.checkSession();
})();
