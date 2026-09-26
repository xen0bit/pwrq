#include <pthread.h>
#include <signal.h>
#include <stdlib.h>

static void on_int(int sig) {}
static void cleanup(void) {}
static void *work(void *arg) { return arg; }

// ruleid: c-entry-points
int main(int argc, char **argv) {
    pthread_t t;
    // ruleid: c-entry-points
    signal(SIGINT, on_int);
    // ok: c-entry-points
    signal(SIGPIPE, SIG_IGN);
    // ruleid: c-entry-points
    atexit(cleanup);
    // ruleid: c-entry-points
    pthread_create(&t, NULL, work, NULL);
    return 0;
}
