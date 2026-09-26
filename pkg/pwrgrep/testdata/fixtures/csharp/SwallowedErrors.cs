public class SwallowedErrors
{
    void F()
    {
        try { Run(); }
        // ruleid: cs-swallowed-errors
        catch (IOException e) { }
        try { Run(); }
        // ruleid: cs-swallowed-errors
        catch { }
        try { Run(); }
        // ok: cs-swallowed-errors
        catch (Exception e) { throw new InvalidOperationException("failed", e); }
    }

    void Run() { }
}
