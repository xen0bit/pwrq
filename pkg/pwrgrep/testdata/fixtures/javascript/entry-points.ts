import express from "express";

const app = express();

// ruleid: js-entry-points
app.get("/users/:id", (req, res) => {
  res.send(req.params.id);
});

// ruleid: js-entry-points
router.post("/login", login);

// ok: js-entry-points
cache.get("key", 1);

// ruleid: js-entry-points
describe("server", () => {
  // ruleid: js-entry-points
  it("starts", () => {});
});

// ruleid: js-entry-points
export default app;

// ruleid: js-entry-points
if (require.main === module) {
  app.listen(3000);
}
