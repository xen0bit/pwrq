// ruleid: c-variables
#define LIMIT 10

// ruleid: c-variables
static int counter = 0;

void f(void) {
    // ruleid: c-variables
    int x = 1;
    // ruleid: c-variables
    char buf[64];
    // ruleid: c-variables
    char *p;
    // ok: c-variables
    x = 2;
}
