function render(el, name) {
  // ruleid: javascript-insecure-document-method
  el.innerHTML = "<b>" + name + "</b>";
  // ruleid: javascript-insecure-document-method
  el.outerHTML = name;
  // ruleid: javascript-insecure-document-method
  document.write(name);

  // A constant is written by the program, not by whoever sent the request.
  // ok: javascript-insecure-document-method
  el.innerHTML = "<b>static</b>";
  // ok: javascript-insecure-document-method
  document.write("<hr>");
  // ok: javascript-insecure-document-method
  el.textContent = name;
}
