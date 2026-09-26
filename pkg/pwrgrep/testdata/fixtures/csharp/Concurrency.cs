public class Concurrency
{
    async Task F(Task a, Task b)
    {
        // ruleid: cs-concurrency
        Task.Run(() => Work());
        // ruleid: cs-concurrency
        var both = Task.WhenAll(a, b);
        // ruleid: cs-concurrency
        lock (this)
        {
            Work();
        }
        // ok: cs-concurrency
        Work();
    }

    void Work() { }
}
