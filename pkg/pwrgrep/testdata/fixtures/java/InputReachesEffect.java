import java.sql.Statement;

public class InputReachesEffect {
    void run() throws Exception {
        String cmd = System.getenv("CMD");
        String full = cmd + " --verbose";
        // ruleid: java-input-reaches-effect
        Runtime.getRuntime().exec(full);
        // ok: java-input-reaches-effect
        Runtime.getRuntime().exec("ls");
    }

    void handle(HttpServletRequest request, Statement stmt) throws Exception {
        String name = request.getParameter("name");
        // ruleid: java-input-reaches-effect
        stmt.executeQuery("SELECT * FROM users WHERE name = '" + name + "'");
        // ok: java-input-reaches-effect
        stmt.executeQuery("SELECT 1");
    }
}
