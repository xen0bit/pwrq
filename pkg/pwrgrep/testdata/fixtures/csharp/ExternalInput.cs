public class ExternalInput : ControllerBase
{
    // ruleid: cs-external-input
    public IActionResult Find([FromQuery] string name) { return Ok(name); }

    void F()
    {
        // ruleid: cs-external-input
        var home = Environment.GetEnvironmentVariable("HOME");
        // ruleid: cs-external-input
        var line = Console.ReadLine();
        // ruleid: cs-external-input
        var config = File.ReadAllText("config.json");
        // ruleid: cs-external-input
        var q = Request.Query["q"];
        // ok: cs-external-input
        var local = "constant";
    }
}
