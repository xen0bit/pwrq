public class Functions {
    // ruleid: java-functions
    public Functions(String name) {}

    // ruleid: java-functions
    public String user(String id) throws Exception {
        // ok: java-functions
        if (id == null) {
            return "";
        }
        return id;
    }

    // ruleid: java-functions
    void run() {}

    // ruleid: java-functions
    private static <T> T first(java.util.List<T> xs) {
        return xs.get(0);
    }
}
