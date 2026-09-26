import fs from "fs";

function f(req, res) {
  // ruleid: js-external-input
  const home = process.env.HOME;
  // ruleid: js-external-input
  const args = process.argv;
  // ruleid: js-external-input
  const config = fs.readFileSync("config.json");
  // ruleid: js-external-input
  const id = req.params.id;
  // ruleid: js-external-input
  const q = location.search;
  // ok: js-external-input
  const local = "constant";
}
