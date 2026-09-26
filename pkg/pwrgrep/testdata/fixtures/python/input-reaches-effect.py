import os
import subprocess
import sys

from flask import request


def run():
    cmd = os.environ["CMD"]
    full = cmd + " --verbose"
    # ruleid: python-input-reaches-effect
    os.system(full)
    # ok: python-input-reaches-effect
    os.system("ls")
    # ruleid: python-input-reaches-effect
    subprocess.run(["ls", sys.argv[1]])


def handle(cursor):
    name = request.args.get("name")
    # ruleid: python-input-reaches-effect
    cursor.execute("SELECT * FROM users WHERE name = '" + name + "'")
    # ok: python-input-reaches-effect
    cursor.execute("SELECT 1")
