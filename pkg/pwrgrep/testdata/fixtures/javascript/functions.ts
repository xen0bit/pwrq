// ruleid: js-functions
function helper(a, b) {
  return a;
}

// ruleid: js-functions
export async function fetchIt(url: string): Promise<string> {
  return url;
}

// ruleid: js-functions
const double = (x: number) => x * 2;

// ruleid: js-functions
const later = async (x) => x;

class Server {
  // ruleid: js-functions
  constructor(port: number) {}

  // ruleid: js-functions
  start(port) {
    // ok: js-functions
    if (port) {
      return port;
    }
  }

  // ruleid: js-functions
  private static async stop(): Promise<void> {}
}

// ok: js-functions
helper(1, 2);
