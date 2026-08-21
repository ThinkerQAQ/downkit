(function (root) {
  "use strict";

  const api = root.DownKitPopup;
  let tools = [];
  const editingTools = new Set();
  const expandedTools = new Set();
  let initializedExpansion = false;

  function setIndicator(state, text) {
    const indicator = document.getElementById("bridgeIndicator");
    indicator.className = `indicator ${state}`;
    indicator.textContent = text;
  }

  function cardState(health) {
    if (health && health.status === "disabled") return "disabled";
    if (health && health.ok) return "ok";
    if (health && ["missing", "incompatible", "error"].includes(health.status)) return "error";
    return "checking";
  }

  function inputFor(field, value) {
    const input = document.createElement(field.type === "select" ? "select" : "input");
    if (field.type === "select") {
      for (const option of field.options || []) {
        const node = document.createElement("option");
        node.value = option.value ?? "";
        node.textContent = option.label || option.value || "";
        input.appendChild(node);
      }
    } else {
      input.type = field.type === "integer" ? "number" : field.type === "boolean" ? "checkbox" : "text";
      input.placeholder = field.placeholder || "";
      input.autocomplete = "off";
      input.readOnly = Boolean(field.readOnly);
      if (field.min !== undefined && field.min !== null) input.min = field.min;
      if (field.max !== undefined && field.max !== null) input.max = field.max;
    }
    if (field.type === "boolean") input.checked = Boolean(value ?? field.default);
    else input.value = value ?? field.default ?? "";
    if (!field.readOnly) input.dataset.configKey = field.key;
    return input;
  }

  const groupDefinitions = {
    runtime: { title: "连接与运行时", description: "浏览器到本机服务的可信连接" },
    capability: { title: "下载能力", description: "发现、解析和下载媒体" },
    dependency: { title: "外部组件", description: "按需参与封装和页面解析" },
    other: { title: "其他 Tool", description: "尚未分类的能力" }
  };

  function normalizedKind(tool) {
    const kind = String(tool && tool.kind || "").toLowerCase();
    if (kind === "runtime" || kind === "infra") return "runtime";
    if (kind === "capability" || kind === "core" || kind === "platform") return "capability";
    if (kind === "dependency") return "dependency";
    return "other";
  }

  function visibleConfigFields(tool, editing) {
    const config = tool && tool.config || {};
    const schema = Array.isArray(config.schema) ? config.schema : [];
    if (editing) return schema.filter(field => !field.readOnly);
    if (normalizedKind(tool) !== "dependency" || !(tool.health && tool.health.ok)) return schema;
    return schema.filter(field => field.type !== "file");
  }

  function currentConfigValues(card, selectedTool) {
    const config = {};
    const extensionScope = Boolean(selectedTool && selectedTool.config && selectedTool.config.scope === "extension");
    for (const tool of tools) {
      const view = tool.config || {};
      if (extensionScope && tool.name !== selectedTool.name) continue;
      if (!extensionScope && view.scope === "extension") continue;
      const toggle = view.toggle;
      if (toggle && toggle.key) config[toggle.key] = Boolean(view.values && view.values[toggle.key]);
      for (const field of Array.isArray(view.schema) ? view.schema : []) {
        if (!field.readOnly) config[field.key] = (view.values && view.values[field.key]) ?? field.default ?? "";
      }
    }
    card.querySelectorAll("[data-config-key]").forEach(input => {
      config[input.dataset.configKey] = input.type === "checkbox"
        ? input.checked
        : input.type === "number" ? Number(input.value) : input.value.trim();
    });
    return config;
  }

  function saveToolConfig(tool, card) {
    const local = tool.config && tool.config.scope === "extension";
    return api.send("tool.config.save", {
      tool: tool.name,
      config: currentConfigValues(card, local ? tool : null)
    });
  }

  function toggleMetadata(tool) {
    const config = tool && tool.config || {};
    const toggle = config.toggle;
    return toggle && toggle.key ? toggle : null;
  }

  function renderToolToggle(summary, tool, card) {
    const toggle = toggleMetadata(tool);
    if (!toggle) return;
    const config = tool.config || {};
    const control = document.createElement("label");
    control.className = "tool-toggle";
    control.title = toggle.description || toggle.label || "切换 Tool";
    const input = document.createElement("input");
    input.type = "checkbox";
    input.checked = Boolean(config.values && config.values[toggle.key]);
    input.dataset.configKey = toggle.key;
    input.setAttribute("role", "switch");
    input.setAttribute("aria-label", toggle.label || `切换 ${tool.displayName || tool.name}`);
    const track = document.createElement("span");
    track.className = "tool-toggle-track";
    control.append(input, track);
    control.addEventListener("click", event => event.stopPropagation());
    input.addEventListener("change", async () => {
      input.disabled = true;
      try {
        await saveToolConfig(tool, card);
        api.setMessage("environmentMessage", `${tool.displayName || tool.name} 已${input.checked ? "开启" : "关闭"}。`, "ok");
        await refresh();
      } catch (error) {
        input.checked = !input.checked;
        input.disabled = false;
        api.setMessage("environmentMessage", error.message || String(error), "error");
      }
    });
    summary.appendChild(control);
  }

  function displayValue(field, value) {
    if (field.type === "boolean") return value === false ? "关闭" : "开启";
    if (field.type === "select") {
      const option = (field.options || []).find(item => String(item.value ?? "") === String(value ?? ""));
      return option && (option.label || option.value) || value || "未设置";
    }
    return value === undefined || value === null || value === "" ? "未设置" : String(value);
  }

  function editorValue(tool, field) {
    const config = tool && tool.config || {};
    const configured = config.values && config.values[field.key];
    if (configured !== undefined && configured !== null && configured !== "") return configured;
    if (field.type === "file") return tool && tool.health && tool.health.path || "";
    return configured ?? field.default ?? "";
  }

  function renderConfigView(body, tool) {
    const config = tool.config || {};
    const schema = visibleConfigFields(tool, false).filter(field => !field.readOnly);
    if (!schema.length) return;
    const values = document.createElement("div");
    values.className = "tool-values";
    for (const field of schema) {
      const item = document.createElement("div");
      item.className = `tool-value${field.type === "directory" || field.type === "file" ? " wide" : ""}`;
      const label = document.createElement("span");
      label.className = "tool-value-label";
      label.textContent = field.label || field.key;
      const data = document.createElement("span");
      data.className = "tool-value-data";
      data.textContent = displayValue(field, config.values && config.values[field.key]);
      data.title = data.textContent;
      item.append(label, data);
      values.appendChild(item);
    }
    body.appendChild(values);
  }

  function renderConfigEditor(body, tool) {
    const config = tool.config || {};
    const schema = visibleConfigFields(tool, true);
    if (!schema.length) return;
    const fields = document.createElement("div");
    fields.className = "tool-fields";
    for (const field of schema) {
      const label = document.createElement("label");
      label.className = `tool-field${field.type === "directory" || field.type === "file" ? " wide" : ""}${field.type === "boolean" ? " boolean" : ""}`;
      const input = inputFor(field, editorValue(tool, field));
      if (field.type === "boolean") label.append(input, document.createTextNode(field.label || field.key));
      else label.append(document.createTextNode(field.label || field.key), input);
      if (field.description) {
        const help = document.createElement("small");
        help.textContent = field.description;
        label.appendChild(help);
      }
      fields.appendChild(label);
    }
    body.appendChild(fields);
  }

  function renderEditControls(container, tool, card, editing) {
    const controls = document.createElement("div");
    controls.className = "tool-edit-controls";
    if (!editing) {
      const editButton = document.createElement("button");
      editButton.type = "button";
      editButton.className = "secondary tool-edit";
      editButton.textContent = "编辑";
      editButton.title = `编辑 ${tool.displayName || tool.name} 配置`;
      editButton.setAttribute("aria-label", editButton.title);
      editButton.addEventListener("click", () => {
        expandedTools.add(tool.name);
        editingTools.add(tool.name);
        renderTools(tools);
      });
      controls.appendChild(editButton);
      container.appendChild(controls);
      return;
    }

    const saveButton = document.createElement("button");
    saveButton.type = "button";
    saveButton.className = "primary";
    saveButton.textContent = "保存";
    const cancelButton = document.createElement("button");
    cancelButton.type = "button";
    cancelButton.className = "secondary";
    cancelButton.textContent = "取消";
    cancelButton.addEventListener("click", () => {
      editingTools.delete(tool.name);
      renderTools(tools);
    });
    saveButton.addEventListener("click", async () => {
      saveButton.disabled = true;
      cancelButton.disabled = true;
      try {
        await saveToolConfig(tool, card);
        editingTools.delete(tool.name);
        api.setMessage("environmentMessage", `${tool.displayName || tool.name} 配置已保存。`, "ok");
        await refresh();
      } catch (error) {
        api.setMessage("environmentMessage", error.message || String(error), "error");
        saveButton.disabled = false;
        cancelButton.disabled = false;
      }
    });
    controls.append(cancelButton, saveButton);
    container.appendChild(controls);
  }

  function groupContainer(list, containers, tool) {
    const key = normalizedKind(tool);
    if (containers.has(key)) return containers.get(key);
    const definition = groupDefinitions[key];
    const section = document.createElement("section");
    section.className = "tool-group";
    section.dataset.toolGroup = key;
    const heading = document.createElement("div");
    heading.className = "tool-group-heading";
    const title = document.createElement("h3");
    title.textContent = definition.title;
    const description = document.createElement("span");
    description.textContent = definition.description;
    heading.append(title, description);
    const cards = document.createElement("div");
    cards.className = "tool-group-list";
    section.append(heading, cards);
    list.appendChild(section);
    containers.set(key, cards);
    return cards;
  }

  function renderTools(nextTools) {
    const list = document.getElementById("toolList");
    if (!initializedExpansion) {
      for (const tool of Array.isArray(nextTools) ? nextTools : []) {
        if (tool.config && tool.config.defaultExpanded) expandedTools.add(tool.name);
      }
      initializedExpansion = true;
    }
    list.replaceChildren();
    tools = Array.isArray(nextTools) ? nextTools : [];
    const containers = new Map();

    for (const tool of tools) {
      const health = tool.health || {};
      const card = document.createElement("details");
      card.className = `tool-card ${cardState(health)}`;
      card.dataset.toolName = tool.name || "";
      const editing = editingTools.has(tool.name);
      card.open = editing || expandedTools.has(tool.name);
      card.addEventListener("toggle", () => {
        if (card.open) expandedTools.add(tool.name);
        else expandedTools.delete(tool.name);
      });

      const summary = document.createElement("summary");
      const dot = document.createElement("span");
      dot.className = "dot";
      const main = document.createElement("span");
      main.className = "tool-card-main";
      const title = document.createElement("span");
      title.className = "tool-card-title";
      const strong = document.createElement("strong");
      strong.textContent = tool.displayName || tool.name;
      const kind = document.createElement("span");
      kind.className = "tool-kind";
      kind.textContent = normalizedKind(tool);
      title.append(strong, kind);
      const headline = document.createElement("span");
      headline.className = "tool-summary";
      headline.textContent = health.summary || "等待检测";
      main.append(title, headline);
      const status = document.createElement("span");
      status.className = "tool-health";
      status.textContent = health.version || health.status || "unknown";
      summary.append(dot, main, status);
      renderToolToggle(summary, tool, card);
      const editableSchema = visibleConfigFields(tool, true);

      const body = document.createElement("div");
      body.className = "tool-card-body";
      if (tool.description || editableSchema.length) {
        const bodyHeader = document.createElement("div");
        bodyHeader.className = "tool-card-body-header";
        if (tool.description) {
          const description = document.createElement("p");
          description.className = "tool-description";
          description.textContent = tool.description;
          bodyHeader.appendChild(description);
        }
        if (editableSchema.length) renderEditControls(bodyHeader, tool, card, editing);
        body.appendChild(bodyHeader);
      }
      const detailText = [health.detail, health.path].filter(Boolean).join(" · ");
      if (detailText) {
        const detail = document.createElement("p");
        detail.className = "tool-detail";
        detail.textContent = detailText;
        body.appendChild(detail);
      }

      if (editing) renderConfigEditor(body, tool);
      else renderConfigView(body, tool);
      const actions = Array.isArray(tool.actions) ? tool.actions : [];
      if (actions.length && !editing) {
        const actionRow = document.createElement("div");
        actionRow.className = "tool-actions";
        for (const action of actions) {
          const button = document.createElement("button");
          button.type = "button";
          button.className = health.ok ? "secondary" : "primary";
          button.textContent = action.label || action.id;
          button.title = action.description || "";
          button.addEventListener("click", async () => {
            button.disabled = true;
            api.setMessage("environmentMessage", `正在执行 ${tool.displayName || tool.name}：${action.label || action.id}…`);
            try {
              await api.send("bridge.tool.action", { tool: tool.name, toolAction: action.id });
              const success = action.id === "restart"
                ? "Bridge 已重启并重新连接。"
                : action.id === "open-output"
                  ? "已打开下载目录。"
                : `${tool.displayName || tool.name} 已安装。`;
              api.setMessage("environmentMessage", success, "ok");
              await refresh();
            } catch (error) {
              api.setMessage("environmentMessage", error.message || String(error), "error");
            } finally {
              button.disabled = false;
            }
          });
          actionRow.appendChild(button);
        }
        body.appendChild(actionRow);
      }
      card.append(summary, body);
      groupContainer(list, containers, tool).appendChild(card);
    }

    if (!tools.length) {
      const empty = document.createElement("p");
      empty.className = "empty";
      empty.textContent = "没有可用的 Tool 描述。";
      list.appendChild(empty);
    }
  }

  async function refresh() {
    setIndicator("checking", "环境检测中");
    api.setMessage("environmentMessage", "");
    try {
      const result = await api.send("bridge.tools");
      renderTools(result.tools);
      const requiredTools = tools.filter(tool => tool.required);
      const requiredReady = requiredTools.length > 0 && requiredTools.every(tool => tool.health && tool.health.ok);
      setIndicator(requiredReady ? "ok" : "error", requiredReady ? "环境正常" : "环境异常");
    } catch (error) {
      setIndicator("error", "环境异常");
      api.setMessage("environmentMessage", error.message || String(error), "error");
    }
  }

  function init() {
    document.getElementById("refreshEnvironment").addEventListener("click", refresh);
  }

  root.DownKitEnvironment = { init, activate: refresh, renderTools };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = { cardState, normalizedKind, visibleConfigFields, editorValue, toggleMetadata };
  }
})(globalThis);
