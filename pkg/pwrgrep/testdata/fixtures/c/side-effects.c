#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

void f(void) {
    // ruleid: c-side-effects
    system("ls");
    // ruleid: c-side-effects
    FILE *out = fopen("out.txt", "w");
    // ok: c-side-effects
    FILE *in = fopen("in.txt", "r");
    // ruleid: c-side-effects
    unlink("tmp");
    // ruleid: c-side-effects
    int s = socket(AF_INET, SOCK_STREAM, 0);
}
