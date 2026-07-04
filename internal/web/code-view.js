(function (global) {
  "use strict";

  function escapeHtml(text) {
    return String(text)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  var LANG_LABELS = {
    javascript: "JavaScript",
    typescript: "TypeScript",
    python: "Python",
    go: "Go",
    json: "JSON",
    yaml: "YAML",
    markdown: "Markdown",
    html: "HTML",
    css: "CSS",
    shell: "Shell",
    sql: "SQL",
    rust: "Rust",
    java: "Java",
    ruby: "Ruby",
    php: "PHP",
    toml: "TOML",
    dockerfile: "Dockerfile",
    makefile: "Makefile",
    plain: "Text",
  };

  var LANG_KEYWORDS = {
    javascript:
      /\b(const|let|var|function|return|if|else|for|while|do|switch|case|break|continue|class|extends|import|export|from|default|async|await|new|this|typeof|instanceof|in|of|try|catch|finally|throw|null|undefined|true|false|void|delete|yield)\b/g,
    typescript:
      /\b(const|let|var|function|return|if|else|for|while|do|switch|case|break|continue|class|extends|import|export|from|default|async|await|new|this|typeof|instanceof|in|of|try|catch|finally|throw|null|undefined|true|false|void|delete|yield|interface|type|enum|implements|namespace|readonly|public|private|protected|as|is|keyof|infer)\b/g,
    python:
      /\b(def|class|return|if|elif|else|for|while|break|continue|import|from|as|with|try|except|finally|raise|pass|lambda|yield|global|nonlocal|assert|del|in|is|not|and|or|True|False|None|async|await)\b/g,
    go: /\b(package|import|func|return|if|else|for|range|switch|case|default|break|continue|var|const|type|struct|interface|map|chan|go|defer|select|fallthrough|nil|true|false|make|new|len|cap)\b/g,
    rust:
      /\b(fn|let|mut|const|static|if|else|match|for|while|loop|break|continue|return|struct|enum|impl|trait|use|mod|pub|crate|self|super|where|async|await|move|ref|true|false|Some|None|Ok|Err)\b/g,
    java:
      /\b(class|interface|enum|extends|implements|import|package|public|private|protected|static|final|void|return|if|else|for|while|do|switch|case|break|continue|new|this|super|try|catch|finally|throw|throws|true|false|null)\b/g,
    ruby:
      /\b(def|class|module|end|if|elsif|else|unless|case|when|while|until|for|break|next|redo|retry|return|yield|raise|rescue|ensure|true|false|nil|self|super|and|or|not|in|require|include|extend)\b/g,
    php:
      /\b(function|class|interface|trait|extends|implements|namespace|use|public|private|protected|static|final|var|return|if|else|elseif|foreach|for|while|do|switch|case|break|continue|new|try|catch|finally|throw|true|false|null|echo|print)\b/g,
    sql:
      /\b(SELECT|FROM|WHERE|JOIN|LEFT|RIGHT|INNER|OUTER|ON|GROUP|BY|ORDER|HAVING|LIMIT|OFFSET|INSERT|INTO|VALUES|UPDATE|SET|DELETE|CREATE|TABLE|INDEX|ALTER|DROP|AS|AND|OR|NOT|NULL|IS|IN|BETWEEN|LIKE|DISTINCT|UNION|ALL|CASE|WHEN|THEN|ELSE|END|PRIMARY|KEY|FOREIGN|REFERENCES|CONSTRAINT|DEFAULT|AUTO_INCREMENT)\b/gi,
    shell:
      /\b(if|then|else|elif|fi|for|do|done|while|until|case|esac|function|return|exit|export|local|readonly|shift|set|unset|source|echo|printf|cd|pwd|test|true|false)\b/g,
    dockerfile:
      /\b(FROM|RUN|CMD|LABEL|MAINTAINER|EXPOSE|ENV|ADD|COPY|ENTRYPOINT|VOLUME|USER|WORKDIR|ARG|ONBUILD|STOPSIGNAL|HEALTHCHECK|SHELL)\b/g,
  };

  function detectLanguage(path) {
    var base = path.split("/").pop() || path;
    var lower = base.toLowerCase();
    if (lower === "dockerfile") {
      return "dockerfile";
    }
    if (lower === "makefile") {
      return "makefile";
    }
    var ext = (base.split(".").pop() || "").toLowerCase();
    var map = {
      js: "javascript",
      jsx: "javascript",
      mjs: "javascript",
      cjs: "javascript",
      ts: "typescript",
      tsx: "typescript",
      py: "python",
      pyw: "python",
      go: "go",
      mod: "go",
      sum: "plain",
      json: "json",
      yaml: "yaml",
      yml: "yaml",
      md: "markdown",
      markdown: "markdown",
      html: "html",
      htm: "html",
      xml: "xml",
      svg: "xml",
      css: "css",
      scss: "css",
      less: "css",
      sh: "shell",
      bash: "shell",
      zsh: "shell",
      sql: "sql",
      rs: "rust",
      java: "java",
      rb: "ruby",
      php: "php",
      toml: "toml",
    };
    return map[ext] || "plain";
  }

  function maybeFormatContent(content, lang) {
    if (lang === "json") {
      try {
        return JSON.stringify(JSON.parse(content), null, 2);
      } catch (_e) {
        /* keep original */
      }
    }
    return content;
  }

  function applyKeywordHighlight(text, lang) {
    var pattern = LANG_KEYWORDS[lang];
    if (!pattern) {
      return text;
    }
    pattern.lastIndex = 0;
    return text.replace(pattern, function (match) {
      return '<span class="tok-keyword">' + match + "</span>";
    });
  }

  function highlightLine(line, lang) {
    if (!line) {
      return '<span class="code-line-empty">&nbsp;</span>';
    }

    var segments = [];
    var plainStart = 0;
    var i = 0;
    var len = line.length;

    function pushPlain(end) {
      if (end > plainStart) {
        segments.push({ type: "plain", text: line.slice(plainStart, end) });
        plainStart = end;
      }
    }

    while (i < len) {
      var ch = line[i];
      var next = line[i + 1];

      if (lang !== "json" && ch === "/" && next === "/") {
        pushPlain(i);
        segments.push({ type: "comment", text: line.slice(i) });
        plainStart = len;
        break;
      }
      if (
        lang !== "json" &&
        ch === "#" &&
        (lang === "python" ||
          lang === "shell" ||
          lang === "yaml" ||
          lang === "toml" ||
          lang === "dockerfile" ||
          lang === "makefile")
      ) {
        pushPlain(i);
        segments.push({ type: "comment", text: line.slice(i) });
        plainStart = len;
        break;
      }
      if (ch === '"' || ch === "'" || ch === "`") {
        var quote = ch;
        var j = i + 1;
        while (j < len) {
          if (line[j] === "\\") {
            j += 2;
            continue;
          }
          if (line[j] === quote) {
            j += 1;
            break;
          }
          j += 1;
        }
        pushPlain(i);
        segments.push({ type: "string", text: line.slice(i, j) });
        i = j;
        plainStart = j;
        continue;
      }
      i += 1;
    }

    pushPlain(len);

    if (!segments.length) {
      return '<span class="code-line-empty">&nbsp;</span>';
    }

    var html = "";
    segments.forEach(function (seg) {
      var escaped = escapeHtml(seg.text);
      if (seg.type === "comment") {
        html += '<span class="tok-comment">' + escaped + "</span>";
      } else if (seg.type === "string") {
        html += '<span class="tok-string">' + escaped + "</span>";
      } else {
        escaped = escaped.replace(/\b(\d+(?:\.\d+)?)\b/g, '<span class="tok-number">$1</span>');
        if (lang === "markdown" && /^#{1,6}\s/.test(seg.text)) {
          html += '<span class="tok-heading">' + applyKeywordHighlight(escaped, lang) + "</span>";
        } else if (lang === "html" || lang === "xml") {
          escaped = escaped.replace(/(&lt;\/?)([\w:-]+)/g, '$1<span class="tok-tag">$2</span>');
          html += applyKeywordHighlight(escaped, lang);
        } else if (lang === "css") {
          escaped = escaped.replace(/([\w-]+)(\s*:)/g, '<span class="tok-property">$1</span>$2');
          html += applyKeywordHighlight(escaped, lang);
        } else if (lang === "dockerfile") {
          html += applyKeywordHighlight(escaped, "dockerfile");
        } else {
          html += applyKeywordHighlight(escaped, lang);
        }
      }
    });
    return html;
  }

  function renderCodeTable(content, lang, diagnostics) {
    diagnostics = diagnostics || [];
    var byLine = {};
    diagnostics.forEach(function (diag) {
      var line = diag.line;
      if (!byLine[line]) {
        byLine[line] = [];
      }
      byLine[line].push(diag);
    });

    var lines = String(content).split("\n");
    if (lines.length && lines[lines.length - 1] === "" && content.slice(-1) === "\n") {
      lines.pop();
    }
    var tbody = document.createDocumentFragment();
    lines.forEach(function (line, idx) {
      var lineNum = idx + 1;
      var lineDiags = byLine[lineNum] || [];
      var tr = document.createElement("tr");
      tr.dataset.line = String(lineNum);
      if (lineDiags.length) {
        tr.className = "code-line-has-error";
        tr.title = lineDiags
          .map(function (diag) {
            return "第 " + diag.line + " 行，第 " + diag.column + " 列：" + diag.message;
          })
          .join("\n");
      }

      var numTd = document.createElement("td");
      numTd.className = "code-line-num" + (lineDiags.length ? " code-line-num-error" : "");
      numTd.textContent = String(lineNum);

      var codeTd = document.createElement("td");
      codeTd.className = "code-line-content" + (lineDiags.length ? " code-line-content-error" : "");
      if (lineDiags.length && lineDiags[0].column > 0) {
        codeTd.dataset.errorCol = String(lineDiags[0].column);
      }
      codeTd.innerHTML = '<code class="lang-' + lang + '">' + highlightLine(line, lang) + "</code>";

      tr.appendChild(numTd);
      tr.appendChild(codeTd);
      tbody.appendChild(tr);
    });
    return { fragment: tbody, lineCount: lines.length, diagnostics: diagnostics };
  }

  global.CodeView = {
    LANG_LABELS: LANG_LABELS,
    detectLanguage: detectLanguage,
    maybeFormatContent: maybeFormatContent,
    renderCodeTable: renderCodeTable,
    escapeHtml: escapeHtml,
  };
})(window);
