import java.nio.file.Files;
import java.nio.file.Path;

public class ExternalInput {
    // ruleid: java-external-input
    public String find(@RequestParam String name) {
        return name;
    }

    void f(HttpServletRequest request) throws Exception {
        // ruleid: java-external-input
        String home = System.getenv("HOME");
        // ruleid: java-external-input
        String config = Files.readString(Path.of("config.json"));
        // ruleid: java-external-input
        String id = request.getParameter("id");
        // ok: java-external-input
        String local = "constant";
    }
}
