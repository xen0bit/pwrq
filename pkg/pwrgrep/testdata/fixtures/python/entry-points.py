import click
from flask import Flask

app = Flask(__name__)


@app.route("/hello")
# ruleid: python-entry-points
def hello():
    return "hi"


@app.get("/items/{id}")
# ruleid: python-entry-points
async def item(id):
    pass


@click.command()
# ruleid: python-entry-points
def cli():
    pass


# ruleid: python-entry-points
def test_hello():
    assert hello() == "hi"


# ok: python-entry-points
def helper():
    pass


# ruleid: python-entry-points
if __name__ == "__main__":
    cli()
