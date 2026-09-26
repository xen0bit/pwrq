import { exec } from "child_process";
import fs from "fs";

async function f(db, el) {
  // ruleid: js-side-effects
  exec("ls");
  // ruleid: js-side-effects
  fs.writeFileSync("out.txt", "");
  // ruleid: js-side-effects
  await fetch("https://example.com");
  // ruleid: js-side-effects
  db.query("SELECT 1");
  // ruleid: js-side-effects
  el.innerHTML = "<b>hi</b>";
  // ok: js-side-effects
  console.log("hi");
}
