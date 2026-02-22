(function registerEvidraExtension(windowObj) {
  var cfg = windowObj.__EVIDRA_EXTENSION_CONFIG__ || {};
  var evidraBaseURL = (cfg.evidraBaseURL || "http://localhost:8080").replace(/\/$/, "");
  var apiBaseURL = (cfg.apiBaseURL || evidraBaseURL).replace(/\/$/, "");
  var authMode = cfg.authMode || "none";
  var authToken = cfg.authToken || "";
  var panelTitle = cfg.title || "Evidra";
  var panelPath = cfg.path || "/evidra-evidence";
  var panelIcon = cfg.icon || "fa fa-file-alt";

  function styleObj(obj) {
    return obj;
  }

  function EvidenceFrame() {
    var qs = new URLSearchParams();
    qs.set("embedded", "argocd");
    qs.set("api_base", apiBaseURL);
    qs.set("auth_mode", authMode);
    if (authMode === "bearer" && authToken) {
      qs.set("auth_token", authToken);
    }
    var src = evidraBaseURL + "/ui/?" + qs.toString();
    return windowObj.React.createElement(
      "div",
      {
        style: styleObj({
          width: "100%",
          height: "calc(100vh - 160px)",
          minHeight: "560px",
          background: "#fff",
          overflow: "hidden",
        }),
      },
      windowObj.React.createElement("iframe", {
        title: "Evidra Evidence Explorer",
        src: src,
        style: styleObj({
          border: 0,
          width: "100%",
          height: "100%",
          display: "block",
          background: "#fff",
        }),
        referrerPolicy: "no-referrer",
      }),
    );
  }

  var api = windowObj.extensionsAPI;
  if (!api || typeof api.registerSystemLevelExtension !== "function") {
    console.warn("Evidra extension: Argo CD extensions API not available.");
    return;
  }

  try {
    api.registerSystemLevelExtension(EvidenceFrame, panelTitle, panelPath, panelIcon);
    return;
  } catch (err1) {
    // fallback
  }

  try {
    var ext = {
      title: panelTitle,
      component: EvidenceFrame,
      path: panelPath,
    };
    if (panelIcon) {
      ext.icon = panelIcon;
    }
    api.registerSystemLevelExtension(ext);
    return;
  } catch (err2) {
    // fallback
  }

  try {
    api.registerSystemLevelExtension(EvidenceFrame, panelTitle);
  } catch (err3) {
    console.error("Evidra extension: failed to register system-level extension", err3);
  }
})(window);
