import java.util.concurrent.Executors;

public class Concurrency {
    void f(java.util.List<Integer> xs) {
        // ruleid: java-concurrency
        new Thread(() -> {}).start();
        // ruleid: java-concurrency
        var pool = Executors.newFixedThreadPool(4);
        // ruleid: java-concurrency
        synchronized (this) {
            xs.clear();
        }
        // ok: java-concurrency
        xs.stream().count();
    }
}
