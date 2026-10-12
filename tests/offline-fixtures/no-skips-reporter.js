// Synthetic data makes the live-mesh absence guards unexpected in this suite.
class NoSkipsReporter {
  constructor() { this.skipped = []; }
  onTestEnd(test, result) {
    if (result.status === "skipped") this.skipped.push(test.titlePath().join(" > "));
  }
  async onEnd(result) {
    if (this.skipped.length) {
      console.error("Unexpected offline E2E skips:\n" + this.skipped.join("\n"));
      return { status: "failed" };
    }
    return { status: result.status };
  }
}
module.exports = NoSkipsReporter;
