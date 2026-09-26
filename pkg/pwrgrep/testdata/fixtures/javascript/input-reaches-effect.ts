import { exec } from "child_process";

function run() {
  const cmd = process.env.CMD;
  const full = cmd + " --verbose";
  // ruleid: js-input-reaches-effect
  exec(full);
  // ok: js-input-reaches-effect
  exec("ls");
}

function handle(req, res, db) {
  const name = req.query.name;
  // ruleid: js-input-reaches-effect
  db.query("SELECT * FROM users WHERE name = '" + name + "'");
  // ok: js-input-reaches-effect
  db.query("SELECT 1");
}
