# ruleid: python-types
class Plain:
    pass


# ruleid: python-types
class Server(Base, Mixin):
    pass


# ok: python-types
server = Server()
