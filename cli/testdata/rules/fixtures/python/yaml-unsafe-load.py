import yaml


def load_config(text):
    # ruleid: python-yaml-unsafe-load
    return yaml.load(text, Loader=yaml.Loader)


def load_stream(text):
    # ruleid: python-yaml-unsafe-load
    return yaml.load_all(text, Loader=yaml.UnsafeLoader)


def load_fast(text):
    # ruleid: python-yaml-unsafe-load
    return yaml.unsafe_load(text)


def load_safely(text):
    # ok: python-yaml-unsafe-load
    return yaml.safe_load(text)


def load_declared(text):
    # ok: python-yaml-unsafe-load
    return yaml.load(text, Loader=yaml.SafeLoader)
