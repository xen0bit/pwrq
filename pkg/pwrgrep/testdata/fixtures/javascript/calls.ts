function f(xs) {
  // ruleid: js-calls
  console.log(xs);
  // ruleid: js-calls
  const d = new Date();
  // ok: js-calls
  return xs[0];
}
