public class Variables {
    // ruleid: java-variables
    private static final int LIMIT = 10;
    // ruleid: java-variables
    private String name;

    void f() {
        // ruleid: java-variables
        var list = new java.util.ArrayList<String>();
        // ruleid: java-variables
        int x = 1;
        // ok: java-variables
        x = 2;
    }
}
