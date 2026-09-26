// ruleid: c-types
struct point { int x; int y; };

// ruleid: c-types
typedef unsigned long size;

// ruleid: c-types
enum color { RED, GREEN };

// ruleid: c-types
union value { int i; float f; };

// ok: c-types
struct point origin;
