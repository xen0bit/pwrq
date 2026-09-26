namespace Demo
{
    // ruleid: cs-types
    public class Server
    {
        // ruleid: cs-types
        public string Name { get; set; }
        // ruleid: cs-types
        public int Port { get; init; }
        // ok: cs-types
        private int count;
    }

    // ruleid: cs-types
    interface IShape { double Area(); }

    // ruleid: cs-types
    enum Color { Red }

    // ruleid: cs-types
    struct Point { public int X; }

    // ruleid: cs-types
    record Person(string Name);
}
