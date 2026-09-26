#include <unistd.h>

void f(int fd) {
    // ruleid: c-swallowed-errors
    (void)close(fd);
    // ruleid: c-swallowed-errors
    if (write(fd, "x", 1) < 0) {}
    // ok: c-swallowed-errors
    if (write(fd, "y", 1) < 0) {
        _exit(1);
    }
    // ok: c-swallowed-errors
    if (fd > 2) {}
}
