public class SideEffects
{
    async Task F(HttpClient client, SqlConnection conn)
    {
        // ruleid: cs-side-effects
        Process.Start("ls");
        // ruleid: cs-side-effects
        File.Delete("tmp");
        // ruleid: cs-side-effects
        await client.GetAsync("https://example.com");
        // ruleid: cs-side-effects
        var cmd = new SqlCommand("SELECT 1", conn);
        // ruleid: cs-side-effects
        cmd.ExecuteReader();
        // ok: cs-side-effects
        Console.WriteLine("hi");
    }
}
