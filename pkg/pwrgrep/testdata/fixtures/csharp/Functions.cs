public class Functions : Base
{
    // ruleid: cs-functions
    public Functions(string name) : base(name) { }

    // ruleid: cs-functions
    public string User(int id, string q)
    {
        // ok: cs-functions
        if (id > 0) { return q; }
        return "";
    }

    // ruleid: cs-functions
    void Run() { }

    // ruleid: cs-functions
    public async Task<int> FetchAsync() { return 1; }
}
