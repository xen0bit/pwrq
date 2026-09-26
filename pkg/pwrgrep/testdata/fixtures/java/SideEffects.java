import java.nio.file.Files;
import java.nio.file.Path;
import java.net.Socket;
import java.sql.Statement;

public class SideEffects {
    void f(Statement stmt) throws Exception {
        // ruleid: java-side-effects
        Runtime.getRuntime().exec("ls");
        // ruleid: java-side-effects
        Files.delete(Path.of("tmp"));
        // ruleid: java-side-effects
        new Socket("example.com", 80);
        // ruleid: java-side-effects
        stmt.executeQuery("SELECT 1");
        // ok: java-side-effects
        System.getenv("HOME");
    }
}
