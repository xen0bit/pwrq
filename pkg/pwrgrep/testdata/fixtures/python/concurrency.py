import asyncio
import threading


def f(work):
    # ruleid: python-concurrency
    t = threading.Thread(target=work)
    # ruleid: python-concurrency
    lock = threading.Lock()
    # ok: python-concurrency
    t.start()


async def g(coro):
    # ruleid: python-concurrency
    task = asyncio.create_task(coro)
    # ok: python-concurrency
    await task
