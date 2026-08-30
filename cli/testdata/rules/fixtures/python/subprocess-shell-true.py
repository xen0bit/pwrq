import subprocess


def run(cmd):
    # ruleid: python-subprocess-shell-true
    subprocess.run(cmd, shell=True)


def check(cmd):
    # ruleid: python-subprocess-shell-true
    return subprocess.check_output(cmd, shell=True, timeout=5)


def fixed():
    # A constant command line has nothing for a shell to reinterpret, and the
    # original excludes it.
    # ok: python-subprocess-shell-true
    subprocess.call("ls -l", shell=True)


def safe(cmd):
    # ok: python-subprocess-shell-true
    subprocess.run(cmd, shell=False)
