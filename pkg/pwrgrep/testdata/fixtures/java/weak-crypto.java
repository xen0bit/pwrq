import java.security.MessageDigest;
import javax.crypto.Cipher;

public class WeakCrypto {
    byte[] fingerprint(byte[] data) throws Exception {
        // ruleid: java-weak-crypto
        MessageDigest md = MessageDigest.getInstance("MD5");
        return md.digest(data);
    }

    byte[] tag(byte[] data) throws Exception {
        // ruleid: java-weak-crypto
        MessageDigest md = MessageDigest.getInstance("SHA-1");
        return md.digest(data);
    }

    Cipher book() throws Exception {
        // ruleid: java-weak-crypto
        return Cipher.getInstance("AES/ECB/PKCS5Padding");
    }

    byte[] digest(byte[] data) throws Exception {
        // ok: java-weak-crypto
        MessageDigest md = MessageDigest.getInstance("SHA-256");
        return md.digest(data);
    }

    Cipher sealed() throws Exception {
        // ok: java-weak-crypto
        return Cipher.getInstance("AES/GCM/NoPadding");
    }
}
