import org.junit.Test;

@RestController
public class EntryPoints {
    @GetMapping("/users/{id}")
    // ruleid: java-entry-points
    public String user(@PathVariable String id) {
        return id;
    }

    // ruleid: java-entry-points
    public static void main(String[] args) {}

    @Test
    // ruleid: java-entry-points
    public void startsUp() {}

    // ruleid: java-entry-points
    protected void doGet(HttpServletRequest req, HttpServletResponse resp) {}

    // ok: java-entry-points
    public void helper() {}
}
