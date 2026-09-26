# ruleid: python-variables
LIMIT = 10


def f(self, d):
    # ruleid: python-variables
    x = 1
    # ruleid: python-variables
    a, b = 1, 2
    # ruleid: python-variables
    name: str = "x"
    # ok: python-variables
    self.port = x
    # ok: python-variables
    d["k"] = a
    # ok: python-variables
    x == b
