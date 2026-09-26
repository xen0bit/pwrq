#include <pthread.h>
#include <unistd.h>

static pthread_mutex_t lock;

void f(void *(*work)(void *)) {
    pthread_t t;
    // ruleid: c-concurrency
    pthread_create(&t, NULL, work, NULL);
    // ruleid: c-concurrency
    pthread_mutex_lock(&lock);
    // ruleid: c-concurrency
    pid_t p = fork();
    // ok: c-concurrency
    sleep(1);
}
