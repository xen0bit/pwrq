import hashlib


def fingerprint(data):
    # ruleid: python-weak-hash
    return hashlib.md5(data).hexdigest()


def tag(data):
    # ruleid: python-weak-hash
    return hashlib.sha1(data).hexdigest()


def checksum(data):
    # A hash used to shard or deduplicate is not a signature, and the standard
    # library has a way to say so.
    # ok: python-weak-hash
    return hashlib.md5(data, usedforsecurity=False).hexdigest()


def digest(data):
    # ok: python-weak-hash
    return hashlib.sha256(data).hexdigest()
