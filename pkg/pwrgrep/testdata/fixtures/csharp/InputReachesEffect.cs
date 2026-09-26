public class InputReachesEffect : ControllerBase
{
    void Run()
    {
        var cmd = Environment.GetEnvironmentVariable("CMD");
        var full = cmd + " --verbose";
        // ruleid: cs-input-reaches-effect
        Process.Start(full);
        // ok: cs-input-reaches-effect
        Process.Start("ls");
    }

    void Handle(SqlConnection conn)
    {
        var name = Request.Query["name"];
        // ruleid: cs-input-reaches-effect
        var bad = new SqlCommand("SELECT * FROM users WHERE name = '" + name + "'", conn);
        // ok: cs-input-reaches-effect
        var good = new SqlCommand("SELECT 1", conn);
    }
}
