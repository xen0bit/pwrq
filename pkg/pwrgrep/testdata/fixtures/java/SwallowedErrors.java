public class SwallowedErrors {
    void f() {
        try {
            run();
            // ruleid: java-swallowed-errors
        } catch (Exception e) {}
        try {
            run();
            // ok: java-swallowed-errors
        } catch (RuntimeException e) {
            throw new IllegalStateException(e);
        }
    }

    void run() {}
}
