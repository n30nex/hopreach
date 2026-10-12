import test from "node:test";
import assert from "node:assert/strict";
import NoSkipsReporter from "../offline-fixtures/no-skips-reporter.js";

test("offline E2E cannot pass with a skipped replay or scope scenario", async (t) => {
  const error = t.mock.method(console, "error", () => {});
  const reporter = new NoSkipsReporter();
  assert.deepEqual(await reporter.onEnd({ status: "passed" }), { status: "passed" });
  assert.deepEqual(await reporter.onEnd({ status: "failed" }), { status: "failed" });
  reporter.onTestEnd({ titlePath: () => ["chromium", "replay"] }, { status: "skipped" });
  assert.deepEqual(await reporter.onEnd({ status: "passed" }), { status: "failed" });
  assert.match(error.mock.calls[0].arguments[0], /chromium > replay/);
});
