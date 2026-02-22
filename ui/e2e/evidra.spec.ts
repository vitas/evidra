import { expect, test, type Frame, type Locator, type Page } from "@playwright/test";

const username = process.env.ARGO_USERNAME || "admin";
const password = process.env.ARGO_PASSWORD || "";
const tabName = process.env.EVIDRA_TAB_NAME || "Evidra";
const extensionPath = process.env.EVIDRA_EXTENSION_PATH || "/evidra-evidence";
const standaloneURL = process.env.EVIDRA_STANDALONE_URL || "http://localhost:8080/ui/";
let useStandalone = false;

test.beforeEach(async ({ page }) => {
  if (useStandalone) {
    return;
  }
  try {
    await loginToArgo(page);
  } catch (err) {
    if (await canReachStandalone()) {
      useStandalone = true;
      return;
    }
    throw err;
  }
});

test("UC-001 S1 happy path: list changes and open one", async ({ page }) => {
  const frame = await openExplorer(page);

  await frame.getByTestId("find-changes-button").click();
  await expect(frame.getByTestId("change-item").first()).toBeVisible();
  await frame.getByTestId("change-item").first().click();

  await expect(frame.getByTestId("root-cause-banner")).toBeVisible();
  await expect(frame.getByTestId("selected-change-header").first()).toBeVisible();
  await expect(frame.getByTestId("timeline-item").first()).toBeVisible();

  await expect(frame.getByTestId("toggle-raw-evidence")).toContainText("Show raw evidence");
  await expect(frame.getByTestId("payload-viewer")).toHaveCount(0);

  await frame.getByTestId("toggle-raw-evidence").click();
  await expect(frame.getByTestId("payload-viewer")).toContainText("{");
});

test("UC-002 S2 search by correlation key", async ({ page }) => {
  const frame = await openExplorer(page);

  await frame.getByTestId("external-change-id-input").fill("CHG777000");
  await frame.getByTestId("external-change-id-input").press("Enter");

  await expect(frame.getByTestId("change-item").first()).toBeVisible();
  await frame.getByTestId("change-item").first().click();

  await expect(frame.getByTestId("root-cause-banner")).toContainText("CHG777000");
  await expect(frame.getByText("CHG777000").first()).toBeVisible();

  const permalink = await frame.getByTestId("change-permalink").innerText();
  expect(permalink).toContain("/ui/explorer/change/chg_");
});

test("UC-003 S3 empty-state clarity", async ({ page }) => {
  const frame = await openExplorer(page);
  await waitForSubjectReady(frame);

  await frame.getByTestId("external-change-id-input").fill(`NO-HIT-${Date.now()}`);
  await frame.getByTestId("external-change-id-input").press("Enter");
  let state = await waitForChangesState(frame);
  if (state === "error") {
    await frame.getByTestId("external-change-id-input").press("Enter");
    state = await waitForChangesState(frame);
  }
  expect(state).toBe("empty");

  const emptyState = frame.getByTestId("changes-empty-state");
  await expect(emptyState).toHaveAttribute("data-state", "empty", { timeout: 20_000 });
  await expect(emptyState).toContainText("No changes found. Adjust filters or search by correlation key to find production changes.");
  await expect(emptyState).not.toContainText("Searching changes...");
});

test("UC-004 S4 export evidence", async ({ page }) => {
  const frame = await openExplorer(page);

  await frame.getByTestId("find-changes-button").click();
  await expect(frame.getByTestId("change-item").first()).toBeVisible();
  await frame.getByTestId("change-item").first().click();

  await frame.getByTestId("export-change-button").click();
  const exportStatus = frame.getByTestId("export-status");
  await expect(exportStatus).toContainText("Export");
  await expect(exportStatus).toContainText(/pending|completed/);
  await expect(exportStatus).toContainText("completed", { timeout: 30_000 });
  await expect(exportStatus).toContainText(/Export\s+\S+:/);
  await expect(frame.getByTestId("download-export-button")).toHaveCount(1);
});

test("UC-005 S5 responsive containment smoke", async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 700 });
  const frame = await openExplorer(page);
  await waitForSubjectReady(frame);

  await frame.getByTestId("external-change-id-input").fill("");
  await frame.getByTestId("ticket-id-input").fill("");
  const advanced = frame.getByTestId("advanced-correlation");
  const isAdvancedOpen = await advanced.evaluate((el) => (el as HTMLDetailsElement).open);
  if (!isAdvancedOpen) {
    await advanced.locator("summary").click();
  }
  await frame.getByTestId("approval-reference-input").fill("");
  await frame.getByTestId("correlation-key-select").selectOption("");
  await frame.getByTestId("correlation-value-input").fill("");

  await frame.getByTestId("find-changes-button").click();
  let state = await waitForChangesState(frame);
  if (state !== "loaded") {
    await frame.getByTestId("find-changes-button").click();
    state = await waitForChangesState(frame);
  }
  expect(state).toBe("loaded");
  await expect(frame.getByTestId("change-item").first()).toBeVisible();

  const explorerFrame = await waitForExplorerFrame(page);
  const horizontalOverflowPx = await explorerFrame.evaluate(() => {
    const root = document.documentElement;
    const body = document.body;
    const rootOverflow = Math.max(0, root.scrollWidth - root.clientWidth);
    const bodyOverflow = Math.max(0, body.scrollWidth - body.clientWidth);
    return Math.max(rootOverflow, bodyOverflow);
  });
  expect(horizontalOverflowPx).toBeLessThanOrEqual(8);

  await explorerFrame.evaluate(() => {
    document.documentElement.scrollTop = 1000;
  });
  await expect(frame.getByTestId("find-changes-button")).toBeVisible();

  const geometry = await explorerFrame.evaluate(() => {
    const changes = document.querySelector<HTMLElement>('[data-testid="changes-panel"]');
    const timeline = document.querySelector<HTMLElement>('[data-testid="timeline-panel"]');
    if (!changes || !timeline) return null;
    const a = changes.getBoundingClientRect();
    const b = timeline.getBoundingClientRect();
    return {
      sideBySide: b.left >= a.right - 1,
      stacked: b.top >= a.bottom - 1,
    };
  });
  expect(geometry).not.toBeNull();
  expect(geometry?.sideBySide || geometry?.stacked).toBeTruthy();
});

test("UC-006 S6 root cause banner shows immediate incident summary", async ({ page }) => {
  const frame = await openExplorer(page);

  await frame.getByTestId("find-changes-button").click();
  await expect(frame.getByTestId("change-item").first()).toBeVisible();
  await frame.getByTestId("change-item").first().click();

  const banner = frame.getByTestId("root-cause-banner");
  await expect(banner).toBeVisible();
  await expect(frame.getByTestId("root-cause-text")).toContainText("Root cause");
  await expect(banner).toContainText("CHG777000");
  await expect(banner).toContainText("OPS-900");
  await expect(banner).toContainText("Time to detect:");

  await frame.getByTestId("copy-permalink-button").click();
  await frame.getByTestId("export-change-button").click();
  await expect(frame.getByTestId("export-status")).toContainText(/Export\s+\S+:/);
});

test("UC-007 S7 selected change header uses primary and secondary fields", async ({ page }) => {
  const frame = await openExplorer(page);

  await frame.getByTestId("find-changes-button").click();
  await expect(frame.getByTestId("change-item").first()).toBeVisible();
  await frame.getByTestId("change-item").first().click();

  const header = frame.getByTestId("selected-change-header").first();
  await expect(header).toContainText("Application:");
  await expect(header).toContainText("Revision:");

  const details = frame.locator(".selected-change-header__secondary");
  await expect(details).toBeVisible();
  await expect(details).not.toHaveAttribute("open", "true");

  await details.locator("summary").click();
  await expect(details).toHaveAttribute("open", "");
  await expect(details).toContainText("Project:");
  await expect(details).toContainText("Cluster:");
  await expect(details).toContainText("Namespace:");

  if ((await frame.getByTestId("copy-revision-button").count()) > 0) {
    await frame.getByTestId("copy-revision-button").click();
    await expect(frame.getByTestId("copy-revision-button")).toBeVisible();
  }
});

test("UC-008 S8 subject selector shows structured display without raw cluster URL", async ({ page }) => {
  const frame = await openExplorer(page);
  await waitForSubjectReady(frame);

  const subjectSelect = frame.getByTestId("subject-select");
  const trigger = subjectSelect.locator(".subject-select-trigger");
  await expect(trigger).toBeVisible();

  await trigger.click();
  await expect(frame.getByTestId("subject-search")).toBeVisible();

  const optionTitles = frame.locator(".subject-option .subject-title");
  await expect(optionTitles.first()).toBeVisible();
  const optionSubtitles = frame.locator(".subject-option .subject-subtitle");
  await expect(optionSubtitles.first()).toBeVisible();

  const optionsText = (await frame.locator(".subject-option").allTextContents()).join(" ");
  expect(optionsText).not.toContain("https://kubernetes.default.svc");
  if (optionsText.includes("kubernetes.default.svc")) {
    throw new Error("subject option text leaked raw cluster URL");
  }

  if (optionsText.includes("in-cluster")) {
    await expect(frame.locator(".subject-option .subject-subtitle").filter({ hasText: "in-cluster" }).first()).toBeVisible();
  }
});

async function loginToArgo(page: Page): Promise<void> {
  await gotoWithRetry(page, "/");

  const usernameInput = page.locator('input[name="username"], input#username');
  if ((await usernameInput.count()) > 0 && (await usernameInput.first().isVisible())) {
    await usernameInput.first().fill(username);
    await page.locator('input[name="password"], input#password').first().fill(password);
    const submit = page.getByRole("button", { name: /sign in|login/i });
    await submit.first().click();
    await page.waitForLoadState("networkidle");
  }
}

async function openExplorer(page: Page): Promise<Locator> {
  if (useStandalone) {
    return openStandalone(page);
  }
  for (let attempt = 0; attempt < 8; attempt += 1) {
    await gotoWithRetry(page, "/");
    const tab = page.getByRole("link", { name: new RegExp(`^${escapeRegex(tabName)}$`) });
    if ((await tab.count()) > 0) {
      await tab.first().click();
    } else {
      await page.waitForTimeout(1000);
      if ((await tab.count()) > 0) {
        await tab.first().click();
      } else {
        await gotoWithRetry(page, extensionPath);
      }
    }

    const frameHost = page.locator('iframe[title="Evidra Evidence Explorer"]');
    if ((await frameHost.count()) > 0) {
      await expect(frameHost).toBeVisible({ timeout: 30_000 });
      const frame = page.frameLocator('iframe[title="Evidra Evidence Explorer"]');
      await expect(frame.getByTestId("filters-panel")).toBeVisible();
      return frame.locator(":root");
    }
    await page.waitForTimeout(500);
  }
  if (await canReachStandalone()) {
    useStandalone = true;
    return openStandalone(page);
  }
  throw new Error("Evidra iframe not found");
}

async function waitForSubjectReady(frame: Locator): Promise<void> {
  const subjectRoot = frame.getByTestId("subject-select");
  const trigger = subjectRoot.locator(".subject-select-trigger");
  await expect(trigger).toBeVisible();
  await expect.poll(async () => {
    const title = await subjectRoot.locator(".subject-title").first().textContent().catch(() => "");
    return (title || "").trim().length > 0;
  }).toBeTruthy();
}

async function waitForChangesState(frame: Locator): Promise<"loaded" | "empty" | "error"> {
  const timeoutAt = Date.now() + 20_000;
  while (Date.now() < timeoutAt) {
    const state = await frame.evaluate(() => {
      const items = document.querySelectorAll('[data-testid="change-item"]').length;
      if (items > 0) return "loaded";
      const empty = document.querySelector<HTMLElement>('[data-testid="changes-empty-state"]');
      const mode = empty?.getAttribute("data-state");
      if (mode === "empty" || mode === "error") return mode;
      return "";
    });
    if (state === "loaded" || state === "empty" || state === "error") return state;
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  return "error";
}

async function waitForExplorerFrame(page: Page): Promise<Frame> {
  if (useStandalone) {
    return page.mainFrame();
  }
  const iframe = page.locator('iframe[title="Evidra Evidence Explorer"]');
  await expect(iframe).toBeVisible({ timeout: 30_000 });
  const handle = await iframe.elementHandle();
  if (!handle) {
    throw new Error("Explorer iframe handle not found");
  }
  const frame = await handle.contentFrame();
  if (!frame) {
    throw new Error("Explorer iframe content frame not found");
  }
  return frame;
}

function escapeRegex(input: string): string {
  return input.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function gotoWithRetry(page: Page, path: string): Promise<void> {
  let lastErr: unknown;
  for (let i = 0; i < 6; i += 1) {
    try {
      await page.goto(path, { waitUntil: "domcontentloaded", timeout: 15_000 });
      return;
    } catch (err) {
      lastErr = err;
      const msg = String((err as Error)?.message || "");
      if (
        !msg.includes("ERR_CONNECTION_RESET") &&
        !msg.includes("ERR_CONNECTION_REFUSED") &&
        !msg.includes("ERR_TIMED_OUT") &&
        !msg.includes("Timeout")
      ) {
        throw err;
      }
      await page.waitForTimeout(1000);
    }
  }
  throw lastErr;
}

async function openStandalone(page: Page): Promise<Locator> {
  await page.goto(standaloneURL, { waitUntil: "domcontentloaded", timeout: 30_000 });
  const root = page.locator("html");
  await expect(root.getByTestId("filters-panel")).toBeVisible();
  return root;
}

async function canReachStandalone(): Promise<boolean> {
  try {
    const res = await fetch(standaloneURL, { method: "GET" });
    return res.ok;
  } catch {
    return false;
  }
}
