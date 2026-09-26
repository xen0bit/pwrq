#include <stdio.h>

void f(void) {
    // ok: c-calls
    int x = 1;
    // ruleid: c-calls
    printf("%d\n", x);
    // ruleid: c-calls
    helper();
}
