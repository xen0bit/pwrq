public class Calls {
    void f() {
        // ruleid: java-calls
        System.out.println("hi");
        // ruleid: java-calls
        helper();
        // ruleid: java-calls
        Object o = new Object();
        // ok: java-calls
        int x = 1 + 2;
    }

    void helper() {}
}
