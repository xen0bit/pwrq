def f(xs):
    # ruleid: python-calls
    print(xs)
    # ruleid: python-calls
    os.path.join("a", "b")
    # ok: python-calls
    return xs[0]
