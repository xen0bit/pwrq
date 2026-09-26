#include <stdio.h>
#include <stdlib.h>

int main(int argc, char **argv) {
    char line[128];
    // ruleid: c-external-input
    char *home = getenv("HOME");
    // ruleid: c-external-input
    char *first = argv[1];
    // ruleid: c-external-input
    fgets(line, sizeof line, stdin);
    // ok: c-external-input
    char *local = "constant";
    return 0;
}
