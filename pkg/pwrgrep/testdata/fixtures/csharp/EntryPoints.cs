[ApiController]
public class UsersController : ControllerBase
{
    [HttpGet("users/{id}")]
    // ruleid: cs-entry-points
    public IActionResult Get(int id) { return Ok(); }

    // ok: cs-entry-points
    public IActionResult Helper() { return Ok(); }

    [Fact]
    // ruleid: cs-entry-points
    public void StartsUp() { }

    // ruleid: cs-entry-points
    static void Main(string[] args)
    {
        var app = WebApplication.Create(args);
        // ruleid: cs-entry-points
        app.MapGet("/hello", () => "hi");
    }
}
