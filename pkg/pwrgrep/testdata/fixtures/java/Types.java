// ruleid: java-types
public class Types {
    // ruleid: java-types
    static class Inner {}
}

// ruleid: java-types
interface Shape {
    double area();
}

// ruleid: java-types
enum Color { RED }

// ruleid: java-types
record Point(int x, int y) {}

// ruleid: java-types
class Use {
    // ok: java-types
    Shape s = null;
}
