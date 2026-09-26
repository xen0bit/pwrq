def f(x):
    # ruleid: python-swallowed-errors
    try:
        x()
    except ValueError:
        pass
    # ruleid: python-swallowed-errors
    try:
        x()
    except KeyError:
        raise
    except:
        # nothing to be done
        pass
    # ok: python-swallowed-errors
    try:
        x()
    except KeyError as e:
        raise RuntimeError("missing") from e
