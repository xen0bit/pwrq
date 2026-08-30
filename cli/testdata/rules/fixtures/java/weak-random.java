import java.security.SecureRandom;
import java.util.Random;

public class WeakRandom {
    int sessionId() {
        // ruleid: java-weak-random
        return new Random().nextInt();
    }

    double jitter() {
        // ruleid: java-weak-random
        return Math.random();
    }

    byte[] token() {
        byte[] out = new byte[32];
        // ok: java-weak-random
        new SecureRandom().nextBytes(out);
        return out;
    }
}
