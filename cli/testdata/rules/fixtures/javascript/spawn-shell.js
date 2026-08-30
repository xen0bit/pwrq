const cp = require("child_process");

function run(cmd) {
  // ruleid: javascript-spawn-shell
  cp.spawn(cmd, { shell: true });
  // ruleid: javascript-spawn-shell
  cp.spawnSync(cmd, { shell: "/bin/sh" });

  // ok: javascript-spawn-shell
  cp.spawnSync(cmd, { shell: false });
  // ok: javascript-spawn-shell
  cp.spawn(cmd, { stdio: "inherit" });
}
