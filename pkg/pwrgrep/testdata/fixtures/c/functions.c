struct point { int x; };

// ok: c-functions
int prototype(int a);

// ruleid: c-functions
int add(int a, int b) { return a + b; }

// ruleid: c-functions
static void helper(void) {}

// ruleid: c-functions
char *name(struct point *p) { return 0; }
