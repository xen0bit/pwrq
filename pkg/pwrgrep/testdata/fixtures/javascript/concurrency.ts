async function f(a, b) {
  // ruleid: js-concurrency
  setTimeout(() => {}, 10);
  // ruleid: js-concurrency
  const w = new Worker("worker.js");
  // ruleid: js-concurrency
  await Promise.all([a, b]);
  // ok: js-concurrency
  await a;
}
