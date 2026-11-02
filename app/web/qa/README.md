# Isolated browser verification

Run `node qa/browser-check.mjs` from `app/web` with Playwright available. Set `QA_NODE_MODULES` to an external dependency directory when Playwright is not installed in this project. Chromium must already be installed for Playwright.

The script starts local Vite, supplies all API/SSE data through browser fixtures and saves desktop/mobile screenshots plus `qa/artifacts/results.json`. Every API call is intercepted; unexpected API routes fail verification instead of contacting a robot, Miniserver or updater. No fixtures are imported by shipped application code.

Use `QA_START_SERVER=0` to reuse a running server, `QA_ORIGIN` to select its address and `QA_OUTPUT` to change the artifact directory. `QA_CHROME_PATH` can select an existing Chrome executable instead of Playwright's bundled Chromium.
