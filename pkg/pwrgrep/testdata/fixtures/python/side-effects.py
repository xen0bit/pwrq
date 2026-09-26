import os
import shutil
import subprocess

import requests


def f(cursor):
    # ruleid: python-side-effects
    os.system("ls")
    # ruleid: python-side-effects
    subprocess.run(["ls", "-l"])
    # ruleid: python-side-effects
    shutil.rmtree("tmp")
    # ruleid: python-side-effects
    with open("out.txt", "w") as out:
        pass
    # ok: python-side-effects
    with open("in.txt") as f:
        pass
    # ruleid: python-side-effects
    requests.get("https://example.com")
    # ruleid: python-side-effects
    cursor.execute("SELECT 1")
    # ok: python-side-effects
    os.getenv("HOME")
