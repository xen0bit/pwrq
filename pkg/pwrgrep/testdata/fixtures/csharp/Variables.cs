public class Variables
{
    // ruleid: cs-variables
    private const int Limit = 10;
    // ruleid: cs-variables
    private string name;

    void F()
    {
        // ruleid: cs-variables
        var list = new List<string>();
        // ruleid: cs-variables
        int x = 1;
        // ok: cs-variables
        x = 2;
    }
}
