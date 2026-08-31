function notify(win, message) {
  // ruleid: javascript-wildcard-postmessage
  win.postMessage(message, "*");

  // ok: javascript-wildcard-postmessage
  win.postMessage(message, "https://example.com");
}
