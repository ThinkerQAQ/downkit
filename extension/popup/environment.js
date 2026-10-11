(function (root) {
  "use strict";

  const api = root.DownKitPopup;
  let tools = [];
  const navigation = [];

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

  function formatModelSize(value) {
    const bytes = Number(value) || 0;
    if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GiB`;
    return `${Math.max(1, Math.round(bytes / 1024 ** 2))} MiB`;
  }

  function modelOptionLabel(model) {
    const state = model.active ? "当前" : model.installed ? "已下载" : model.recommended ? "推荐" : "";
    return [model.id, model.language, model.variant, formatModelSize(model.sizeBytes), state].filter(Boolean).join(" · ");
  }

  function modelActionLabel(model) {
    if (model.active) return "当前模型";
    return model.installed ? "设为当前模型" : "下载并使用";
  }

  function renderModelManager(body, tool) {
    const models = Array.isArray(tool.models) ? tool.models : [];
    if (!models.length) return;
    const manager = document.createElement("div");
    manager.className = "model-manager";
    const title = document.createElement("strong");
    const toolName = tool.displayName || tool.name || "本地";
    title.textContent = `${toolName} 模型目录`;
    const select = document.createElement("select");
    select.setAttribute("aria-label", "选择 Whisper 模型");
    for (const model of models) {
      const option = document.createElement("option");
      option.value = model.id;
      option.textContent = modelOptionLabel(model);
      select.appendChild(option);
    }
    const initial = models.find(model => model.active) || models.find(model => model.recommended) || models[0];
    select.value = initial.id;
    const detail = document.createElement("small");
    const licenseLink = document.createElement("a");
    licenseLink.target = "_blank";
    licenseLink.rel = "noreferrer";
    licenseLink.hidden = true;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "primary";

    function selectedModel() {
      return models.find(model => model.id === select.value) || models[0];
    }
    function update() {
      const model = selectedModel();
      detail.textContent = `${model.source} · ${model.language} · ${model.variant} · ${formatModelSize(model.sizeBytes)}`;
      licenseLink.hidden = !model.licenseUrl;
      licenseLink.href = model.licenseUrl || "#";
      licenseLink.textContent = model.license ? `查看 ${model.license} 使用条款` : "查看模型许可";
      button.textContent = modelActionLabel(model);
      button.disabled = Boolean(model.active);
    }
    select.addEventListener("change", update);
    button.addEventListener("click", async () => {
      const model = selectedModel();
      const requiresAcceptance = !model.installed && Boolean(model.requiresLicenseAcceptance);
      const confirmation = requiresAcceptance
        ? `该模型受 ${model.license || "单独"} 使用条款约束。请先点击条款链接阅读。\n\n确认你已阅读并接受条款，并下载 ${model.id}（${formatModelSize(model.sizeBytes)}）？`
        : `下载 ${model.id}（${formatModelSize(model.sizeBytes)}）并设为当前模型？`;
      if (!model.installed && root.confirm && !root.confirm(confirmation)) return;
      button.disabled = true;
      button.textContent = model.installed ? "正在切换…" : "正在下载…";
      api.setMessage("environmentMessage", `正在准备 ${toolName} 模型 ${model.id}，请勿关闭扩展弹窗…`);
      try {
        await api.send("bridge.tool.action", { tool: tool.name, toolAction: "install-model", model: model.id, acceptLicense: requiresAcceptance });
        api.setMessage("environmentMessage", `${toolName} 模型 ${model.id} 已可用并设为当前模型。`, "ok");
        await refresh();
      } catch (error) {
        api.setMessage("environmentMessage", error.message || String(error), "error");
        button.disabled = false;
        update();
      }
    });
    manager.append(title, select, button, detail, licenseLink);
    body.appendChild(manager);
    update();
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
        navigation.push({ type: "edit", id: tool.name });
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
      navigation.pop();
      renderTools(tools);
    });
    saveButton.addEventListener("click", async () => {
      saveButton.disabled = true;
      cancelButton.disabled = true;
      try {
        await saveToolConfig(tool, card);
          navigation.pop();
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

  const categoryMeta = {
    appearance: { title: "界面与交互", description: "语言、导航方式与工作台偏好", icon: "appearance" },
    runtime: { title: "连接与运行时", description: "浏览器连接、Bridge 与网络", icon: "runtime" },
    capability: { title: "下载能力", description: "媒体嗅探、下载与处理", icon: "capability" },
    dependency: { title: "外部组件", description: "FFmpeg、yt-dlp、识别和翻译", icon: "dependency" },
    other: { title: "其他组件", description: "其他可选模块", icon: "settings" }
  };

  function symbol(name, className = "ui-icon") {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    svg.classList.add(className);
    svg.setAttribute("aria-hidden", "true");
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", "#i-" + name);
    svg.appendChild(use);
    return svg;
  }

  function settingsRow({ title, subtitle, icon, trailing, health, action }) {
    const row = document.createElement("button");
    row.type = "button";
    row.className = "settings-row";
    const symbolWrap = document.createElement("span");
    symbolWrap.className = "settings-row-symbol";
    symbolWrap.appendChild(symbol(icon || "settings"));
    const copy = document.createElement("span");
    copy.className = "settings-row-copy";
    const name = document.createElement("strong");
    name.textContent = title;
    const secondary = document.createElement("small");
    secondary.textContent = subtitle || "";
    copy.append(name, secondary);
    row.append(symbolWrap, copy);
    if (trailing) {
      const badge = document.createElement("span");
      badge.className = "settings-row-trailing" + (health ? " " + cardState(health) : "");
      badge.textContent = trailing;
      row.appendChild(badge);
    }
    row.appendChild(symbol("chevron", "settings-chevron"));
    row.addEventListener("click", action);
    return row;
  }

  function settingsHeading(title, description) {
    const back = document.getElementById("settingsBack");
    back.hidden = navigation.length === 0;
    document.getElementById("settingsTitle").textContent = title;
    document.getElementById("settingsSubtitle").textContent = description;
  }

  function enter(type, id) {
    navigation.push({ type, id });
    renderTools(tools);
  }

  function goBack() {
    if (!navigation.length) return;
    navigation.pop();
    renderTools(tools);
  }

  function addSection(list, label) {
    const section = document.createElement("h3");
    section.className = "settings-section-label";
    section.textContent = label;
    list.appendChild(section);
  }

  function toolActions(body, tool, health) {
    const actions = Array.isArray(tool.actions) ? tool.actions : [];
    if (!actions.length) return;
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
        api.setMessage("environmentMessage", "正在执行 " + (tool.displayName || tool.name) + "：" + (action.label || action.id) + "…");
        try {
          await api.send("bridge.tool.action", { tool: tool.name, toolAction: action.id });
          const success = action.id === "restart" ? "Bridge 已重启并重新连接。"
            : action.id === "open-output" ? "已打开下载目录。"
            : (tool.displayName || tool.name) + " 已安装。";
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

  function renderDetail(list, tool, editing) {
    const health = tool.health || {};
    const card = document.createElement("section");
    card.className = "settings-detail-card";
    card.dataset.toolName = tool.name || "";
    const header = document.createElement("div");
    header.className = "settings-tool-header";
    const status = document.createElement("div");
    status.className = "settings-tool-status";
    const dot = document.createElement("span");
    dot.className = "dot " + cardState(health);
    const summary = document.createElement("span");
    summary.textContent = health.summary || health.status || "等待检测";
    status.append(dot, summary);
    if (health.version) {
      const version = document.createElement("small");
      version.textContent = health.version;
      status.append(version);
    }
    header.appendChild(status);
    if (!editing) renderToolToggle(header, tool, card);
    const body = document.createElement("div");
    body.className = "settings-detail-body";
    if (editing) {
      const help = document.createElement("p");
      help.className = "settings-help";
      help.textContent = "修改后保存到 Data/config.toml，所有入口共用。";
      body.appendChild(help);
      renderConfigEditor(body, tool);
      renderEditControls(body, tool, card, true);
    } else {
      if (tool.description) {
        const description = document.createElement("p");
        description.className = "settings-help";
        description.textContent = tool.description;
        body.appendChild(description);
      }
      const detail = [health.detail, health.path].filter(Boolean).join(" · ");
      if (detail) {
        const line = document.createElement("p");
        line.className = "tool-detail";
        line.textContent = detail;
        body.appendChild(line);
      }
      renderConfigView(body, tool);
      renderModelManager(body, tool);
      if (visibleConfigFields(tool, true).length) renderEditControls(body, tool, card, false);
      toolActions(body, tool, health);
    }
    card.append(header, body);
    list.appendChild(card);
  }

  async function refreshAbout() {
    const appVersion = document.getElementById("aboutAppVersion");
    const bridgeVersion = document.getElementById("aboutBridgeVersion");
    appVersion.textContent = "读取中…";
    bridgeVersion.textContent = "读取中…";
    try {
      const info = await api.send("ui.about");
      appVersion.textContent = info.extensionVersion ? "v" + info.extensionVersion : "未知";
      bridgeVersion.textContent = info.bridgeVersion ? "v" + info.bridgeVersion : "未连接";
    } catch {
      appVersion.textContent = "未连接";
      bridgeVersion.textContent = "未连接";
    }
  }

  function renderTools(nextTools) {
    tools = Array.isArray(nextTools) ? nextTools : [];
    const list = document.getElementById("toolList");
    const preferences = document.getElementById("settingsInterface");
    const about = document.getElementById("settingsAbout");
    if (!list || !preferences || !about) return;
    list.replaceChildren();
    list.hidden = false;
    preferences.hidden = true;
    about.hidden = true;
    const frame = navigation[navigation.length - 1];
    if (!frame) {
      settingsHeading("设置", "下载、连接、组件与界面偏好");
      addSection(list, "偏好设置");
      list.appendChild(settingsRow({
        title: categoryMeta.appearance.title, subtitle: categoryMeta.appearance.description,
        icon: "appearance", action: () => enter("category", "appearance")
      }));
      addSection(list, "系统与组件");
      for (const kind of ["runtime", "capability", "dependency", "other"]) {
        const count = tools.filter(tool => normalizedKind(tool) === kind).length;
        if (!count) continue;
        list.appendChild(settingsRow({
          title: categoryMeta[kind].title, subtitle: categoryMeta[kind].description,
          icon: categoryMeta[kind].icon, trailing: String(count) + " 项",
          action: () => enter("category", kind)
        }));
      }
      addSection(list, "其他");
      list.appendChild(settingsRow({
        title: "关于", subtitle: "版本与应用信息",
        icon: "about", action: () => enter("about", "downkit")
      }));
      return;
    }

    if (frame.type === "about") {
      settingsHeading("关于", "DownKit 版本与应用信息");
      list.hidden = true;
      about.hidden = false;
      refreshAbout();
      return;
    }

    if (frame.type === "category") {
      const group = categoryMeta[frame.id] || categoryMeta.other;
      settingsHeading(group.title, group.description);
      if (frame.id === "appearance") {
        list.hidden = true;
        preferences.hidden = false;
        return;
      }
      const groupTools = tools.filter(tool => normalizedKind(tool) === frame.id);
      for (const tool of groupTools) {
        const health = tool.health || {};
        list.appendChild(settingsRow({
          title: tool.displayName || tool.name, subtitle: health.summary || tool.description,
          icon: group.icon,
          trailing: health.status === "ready" ? "就绪" : (health.version || health.status || "未知"),
          health, action: () => enter("tool", tool.name)
        }));
      }
      if (!groupTools.length) addSection(list, "暂无可用组件");
      return;
    }

    const tool = tools.find(item => item.name === frame.id);
    if (!tool) {
      navigation.pop();
      renderTools(tools);
      return;
    }
    const editing = frame.type === "edit";
    settingsHeading(editing ? "编辑 " + (tool.displayName || tool.name) : (tool.displayName || tool.name),
      editing ? "保存后更新 TOML 配置" : (tool.description || "查看组件状态与设置"));
    renderDetail(list, tool, editing);
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
    document.getElementById("settingsBack").addEventListener("click", goBack);
  }

  root.DownKitEnvironment = { init, activate: refresh, renderTools };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = { cardState, normalizedKind, visibleConfigFields, editorValue, toggleMetadata, formatModelSize, modelOptionLabel, modelActionLabel };
  }
})(globalThis);
