#include <stdlib.h>
#include <unistd.h>

int main(int argc, char **argv) {
    char *cmd = argv[1];
    // ruleid: c-input-reaches-effect
    system(cmd);
    // ok: c-input-reaches-effect
    system("ls");
    const char *home = getenv("HOME");
    char *p;
    p = home;
    // ruleid: c-input-reaches-effect
    unlink(p);
    return 0;
}
