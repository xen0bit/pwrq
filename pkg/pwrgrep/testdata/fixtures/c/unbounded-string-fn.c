#include <stdio.h>
#include <string.h>

void read_name(char *dst, const char *src, size_t n)
{
    char buf[64];
    /* ruleid: c-unbounded-string-fn */
    gets(buf);
    /* ruleid: c-unbounded-string-fn */
    strcpy(dst, src);
    /* ruleid: c-unbounded-string-fn */
    strcat(dst, src);
    /* ruleid: c-unbounded-string-fn */
    sprintf(dst, "%s", src);

    /* ok: c-unbounded-string-fn */
    fgets(dst, n, stdin);
    /* ok: c-unbounded-string-fn */
    snprintf(dst, n, "%s", src);
}
