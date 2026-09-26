function f(x, promise) {
  try {
    x();
    // ruleid: js-swallowed-errors
  } catch (e) {}
  try {
    x();
    // ruleid: js-swallowed-errors
  } catch {}
  try {
    x();
    // ok: js-swallowed-errors
  } catch (e) {
    throw new Error("failed", { cause: e });
  }
  // ruleid: js-swallowed-errors
  promise.catch(() => {});
}
