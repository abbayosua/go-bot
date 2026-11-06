// Thin native shell: starts the Go-Bot binary next to the app (or connects to
// one already running) and shows its web UI. No forked logic: every feature
// is the same API and UI the browser gets.
const { app, BrowserWindow, shell } = require("electron");
const { spawn } = require("child_process");
const path = require("path");
const fs = require("fs");

const PORT = process.env.GOBOT_PORT || "7417";
const ADDR = `http://127.0.0.1:${PORT}`;
let child = null;
let window = null;

function binaryPath() {
  const local = path.join(__dirname, "..", "bin", "go-bot");
  if (fs.existsSync(local)) return local;
  const packaged = path.join(process.resourcesPath || "", "go-bot");
  if (packaged && fs.existsSync(packaged)) return packaged;
  return null;
}

function startBackend() {
  const binary = binaryPath();
  if (!binary) return;
  child = spawn(binary, ["--addr", `127.0.0.1:${PORT}`], { stdio: "ignore" });
  child.on("error", () => { child = null; });
}

async function waitForBackend(tries = 60) {
  for (let i = 0; i < tries; i++) {
    try {
      const response = await fetch(`${ADDR}/api/health`);
      if (response.ok) return true;
    } catch (error) {
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
  }
  return false;
}

async function createWindow() {
  window = new BrowserWindow({
    width: 1180,
    height: 800,
    title: "Go-Bot",
    autoHideMenuBar: true,
  });
  window.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url);
    return { action: "deny" };
  });
  await window.loadURL(ADDR);
}

app.whenReady().then(async () => {
  startBackend();
  await waitForBackend();
  await createWindow();
  app.on("activate", async () => {
    if (BrowserWindow.getAllWindows().length === 0) await createWindow();
  });
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") app.quit();
});

app.on("quit", () => {
  if (child) child.kill();
});
