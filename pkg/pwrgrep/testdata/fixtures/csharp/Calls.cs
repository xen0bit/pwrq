public class Calls
{
    void F()
    {
        // ruleid: cs-calls
        Console.WriteLine("hi");
        // ruleid: cs-calls
        Helper();
        // ruleid: cs-calls
        var o = new StringBuilder();
        // ok: cs-calls
        int x = 1 + 2;
    }

    void Helper() { }
}
