// ruleid: js-types
class Server {}

// ruleid: js-types
export class Child extends Server {}

// ruleid: js-types
interface Shape {
  area(): number;
}

// ruleid: js-types
type ID = string;

// ruleid: js-types
enum Color {
  Red,
}

// ok: js-types
const s = new Server();
