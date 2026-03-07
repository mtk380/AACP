#!/usr/bin/env node
const fs = require("node:fs/promises");
const path = require("node:path");
const { chromium } = require("playwright");

const baseURL = process.env.AACP_UI_SCREENSHOT_URL || "http://127.0.0.1:8899";
const outDir = process.env.AACP_UI_SCREENSHOT_OUT || "docs/screenshots";

async function capture() {
  const outputDir = path.resolve(outDir);
  await fs.mkdir(outputDir, { recursive: true });

  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1600, height: 1200 },
  });
  const page = await context.newPage();

  await page.goto(baseURL, { waitUntil: "networkidle", timeout: 60000 });
  await page.waitForSelector("#btn-run-story-demo", { timeout: 30000 });
  await page.waitForTimeout(600);

  await page.screenshot({
    path: path.join(outputDir, "story-overview.png"),
    fullPage: true,
  });

  await page.locator("#btn-run-story-demo").click();
  await page.waitForSelector(".story-step", { timeout: 120000 });
  await page.waitForTimeout(1200);

  await page.screenshot({
    path: path.join(outputDir, "story-after-run.png"),
    fullPage: true,
  });

  const firstStep = page.locator(".story-step").first();
  await firstStep.locator("summary").click();
  await page.waitForTimeout(500);

  await firstStep.screenshot({
    path: path.join(outputDir, "story-step-detail.png"),
  });

  await browser.close();
  console.log(`screenshots generated in ${outputDir}`);
}

capture().catch((err) => {
  console.error(err);
  process.exit(1);
});
