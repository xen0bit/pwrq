import os
import sys

from flask import request


def f(parser):
    # ruleid: python-external-input
    home = os.environ["HOME"]
    # ruleid: python-external-input
    user = os.getenv("USER")
    # ruleid: python-external-input
    first = sys.argv[1]
    # ruleid: python-external-input
    args = parser.parse_args()
    # ruleid: python-external-input
    line = input("> ")
    # ruleid: python-external-input
    with open("config.json") as f:
        pass
    # ok: python-external-input
    with open("out.txt", "w") as out:
        pass
    # ruleid: python-external-input
    q = request.args.get("q")
    # ok: python-external-input
    local = "constant"
